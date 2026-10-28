package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	artifactapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/application"
	artifactdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/domain"
	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	continuationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	hookdistribution "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/distribution"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/imageadmission"
	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/messaging"
	processorcheckpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	processordistribution "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/distribution"
	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	processorinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
	reviewrundomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	reviewruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/runtime"
	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/ids"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
	registrydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/hostinfo"
)

var errReconciliationRequired = errors.New("RECONCILIATION_REQUIRED")

type worker struct {
	cfg                workforceconfig.Config
	logger             *zap.Logger
	control            *control.Client
	runtime            *attemptruntime.Docker
	reviewRuntime      *reviewruntime.Docker
	artifacts          *artifactstore.Store
	hooks              *hookdistribution.Materializer
	processors         *processordistribution.Materializer
	instance           string
	hostBoot           string
	active             atomic.Int64
	activeReviews      atomic.Int64
	heartbeat          atomic.Int64
	consumerState      atomic.Value
	directives         chan string
	consumerRestart    chan struct{}
	inventoryWake      chan struct{}
	inventoryVersion   atomic.Int64
	refreshGeneration  atomic.Int64
	attempts           sync.WaitGroup
	finalizationSlots  chan struct{}
	uploadSlots        chan struct{}
	fixtureSlots       chan struct{}
	deliverySlots      chan struct{}
	sourceSlots        chan struct{}
	agentSlots         chan struct{}
	reviewSlots        chan struct{}
	containerSlots     chan struct{}
	faults             *acceptanceFaultInjector
	workspace          *attemptruntime.PersistentWorkspace
	cleanupJournal     *resourcejournal.Journal
	cleanupReconciling atomic.Bool
	interactions       *interactionTransportHub
	interactionBudgets *privateInteractionBudgets
}

type workspaceGCAuthority struct{ control *control.Client }

func (a workspaceGCAuthority) Eligibility(ctx context.Context, taskID int64) (attemptruntime.GCEligibility, error) {
	if a.control == nil {
		return attemptruntime.GCEligibility{}, errors.New("WORKSPACE_RETENTION_AUTHORITY_UNAVAILABLE")
	}
	eligibility, err := a.control.WorkspaceGCEligibility(ctx, taskID)
	if err != nil {
		return attemptruntime.GCEligibility{}, err
	}
	return attemptruntime.GCEligibility{
		TaskID:                 eligibility.TaskID,
		Terminal:               eligibility.Terminal,
		TerminalAt:             eligibility.TerminalAt,
		ActiveReferences:       eligibility.ActiveReferences,
		PortableStateAvailable: eligibility.PortableStateAvailable,
		ReasonCode:             eligibility.ReasonCode,
	}, nil
}

func main() {
	configDir := flag.String("config-dir", "./configs", "Worker configuration directory")
	showVersion := flag.Bool("version", false, "print build metadata and exit")
	admitOnly := flag.Bool("admit-only", false, "inspect configured protocol images and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.String())
		return
	}
	cfg, err := workforceconfig.LoadWorker(*configDir)
	if err != nil {
		fatal("load Worker configuration", err)
	}
	if err := cfg.Validate(); err != nil {
		fatal("validate Worker configuration", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger, err := zap.NewProduction()
	if err != nil {
		fatal("create logger", err)
	}
	defer logger.Sync()
	persistentWorkspace, err := attemptruntime.OpenPersistentWorkspace(cfg.Worker.WorkspaceRoot)
	if err != nil {
		fatal("preflight persistent Worker workspace", err)
	}
	// The canonical path protects all later path containment checks from an
	// ambiguous configured symlink while retaining the existing
	// worker.workspaceRoot configuration name and development default.
	cfg.Worker.WorkspaceRoot = persistentWorkspace.Root()
	unlock, err := acquireWorkerLock(cfg.Worker.WorkspaceRoot)
	if err != nil {
		fatal("acquire Worker single-instance lock", err)
	}
	defer unlock()
	cleanupJournal, err := resourcejournal.Open(filepath.Join(cfg.Worker.WorkspaceRoot, ".seven", "resource-journal"))
	if err != nil {
		fatal("open Worker-private resource journal", err)
	}
	cleanupJournal.SetContainerLimit(cfg.Worker.MaxTotalContainers)
	workspaceRecords, err := persistentWorkspace.Scan(ctx)
	if err != nil {
		fatal("scan persistent Worker workspace", err)
	}
	for _, record := range workspaceRecords {
		logger.Info("persistent workspace recovery scan",
			zap.Int64("task_id", record.TaskID), zap.Int64("task_turn_id", record.TaskTurnID),
			zap.Int64("finalization_run_id", record.FinalizationRunID), zap.String("state", string(record.State)), zap.String("reason_code", record.ReasonCode))
	}
	if *admitOnly {
		images, inspectErr := inspectImages(ctx, cfg)
		if inspectErr != nil {
			fatal("inspect Worker protocol images", inspectErr)
		}
		body, _ := json.MarshalIndent(images, "", "  ")
		fmt.Println(string(body))
		return
	}
	token, err := readToken(cfg.Worker.TokenFile)
	if err != nil {
		fatal("read Worker token", err)
	}
	hostBootID, err := hostinfo.HostBootID(ctx)
	if err != nil {
		fatal("derive host boot identity", err)
	}
	hostBootID = acceptanceHostBootID(hostBootID)
	instanceID := ids.New("wki")
	client, err := control.New(cfg.Worker.ServerBaseURL, token, cfg.Worker.ID, instanceID, hostBootID)
	if err != nil {
		fatal("create Worker Machine API client", err)
	}
	egressReady := deliveryEgressEnforced(ctx, cfg)
	capabilities, _ := json.Marshal(map[string]any{
		"workerProtocol": "seven.worker/v1", "imageContracts": []string{"seven.worker-image/v1"}, "attemptControl": []int{1}, "supervisorControl": []int{2},
		"gitDelivery": true, "customDelivery": true, "deliveryEgressEnforced": egressReady,
		"planningSource":            "v1",
		"independentReviewConsumer": map[bool]string{true: "v1", false: ""}[cfg.Reviewer.Enabled],
	})
	registration, err := client.Register(ctx, control.RegisterInput{
		StartedAt: time.Now().UTC().Format(time.RFC3339), Version: buildinfo.Version,
		Revision: buildinfo.Commit, BuildDate: buildinfo.BuildDate,
		MaxActiveAttempts: cfg.Worker.MaxActiveContainers, MaxActiveReviews: cfg.Worker.MaxActiveReviews, MaxTotalContainers: cfg.Worker.MaxTotalContainers,
		Capabilities: capabilities,
	})
	if err != nil {
		fatal("register Worker", err)
	}
	images, err := inspectImages(ctx, cfg)
	if err != nil {
		fatal("inspect Worker protocol images", err)
	}
	inventoryDigest := canonicalInventoryDigest(images)
	initialInventoryVersion := int64(registration.InventoryVersion) + 1
	if err := client.PublishInventory(ctx, initialInventoryVersion, inventoryDigest, images); err != nil {
		fatal("publish Worker image inventory", err)
	}
	cfg.RabbitMQ.TaskQueue = registration.Queue.Queue
	cfg.RabbitMQ.Declare = false
	cfg.RabbitMQ.Prefetch = cfg.Worker.MaxTotalContainers
	if cfg.RabbitMQ.Prefetch < 1 {
		cfg.RabbitMQ.Prefetch = 1
	}
	if cfg.RabbitMQ.Prefetch > 64 {
		cfg.RabbitMQ.Prefetch = 64
	}
	broker, err := messaging.Open(cfg.RabbitMQ)
	if err != nil {
		fatal("open dedicated RabbitMQ consumer", err)
	}
	defer broker.Close()
	artifacts, err := artifactstore.Open(ctx, cfg.Artifact)
	if err != nil {
		fatal("open artifact data plane", err)
	}
	deliveryConcurrency := cfg.Worker.MaxActiveDeliveries
	if deliveryConcurrency < 1 {
		deliveryConcurrency = 1
	}
	w := &worker{
		cfg: cfg, logger: logger, control: client, artifacts: artifacts,
		runtime:  attemptruntime.NewDocker(cfg.Worker, cfg.ProviderProxy, cfg.ProviderSSRF),
		instance: instanceID, hostBoot: hostBootID,
		directives: make(chan string, 4), consumerRestart: make(chan struct{}, 1), inventoryWake: make(chan struct{}, 1),
		finalizationSlots: make(chan struct{}, 1), uploadSlots: make(chan struct{}, 2), fixtureSlots: make(chan struct{}, 1), deliverySlots: make(chan struct{}, deliveryConcurrency), sourceSlots: make(chan struct{}, 1),
		faults: newAcceptanceFaultInjector(), workspace: persistentWorkspace, cleanupJournal: cleanupJournal,
		agentSlots: make(chan struct{}, cfg.Worker.MaxActiveContainers), reviewSlots: make(chan struct{}, cfg.Worker.MaxActiveReviews), containerSlots: make(chan struct{}, cfg.Worker.MaxTotalContainers),
	}
	w.runtime.BindPersistentWorkspace(persistentWorkspace)
	w.interactionBudgets, err = initPrivateInteractionBudgets(filepath.Join(cfg.Worker.WorkspaceRoot, ".interaction-budgets"))
	if err != nil {
		fatal("open private interaction budgets", err)
	}
	defer w.interactionBudgets.root.Close()
	w.interactions, err = newInteractionTransportHub(ctx, filepath.Join(cfg.Worker.WorkspaceRoot, ".interaction-spool"), client)
	if err != nil {
		fatal("open private interaction spool", err)
	}
	defer w.interactions.Close()
	go func() {
		select {
		case <-ctx.Done():
		case failure, ok := <-w.interactions.Errors():
			if !ok {
				return
			}
			// Losing reliable interaction storage is not an observational warning:
			// stop intake/execution and retain the spool for authoritative recovery.
			w.logger.Error("interaction delivery blocked; stopping Worker", zap.Int64("attempt_id", failure.AttemptID), zap.Error(failure.Err))
			stop()
		}
	}()
	w.runtime.BindResourceJournal(cleanupJournal)
	w.reviewRuntime = reviewruntime.NewDocker(cfg.Worker, cfg.ProviderProxy)
	w.reviewRuntime.BindPersistentWorkspace(persistentWorkspace)
	w.reviewRuntime.BindResourceJournal(cleanupJournal)
	w.cleanupReconciling.Store(true)
	if err := w.recoverResourceJournal(ctx, registration.Reconciliation); err != nil {
		fatal("reconcile private resource journal", err)
	}
	w.inventoryVersion.Store(initialInventoryVersion)
	initialConsumerState := "CONSUMING"
	if registration.DesiredState != registrydomain.DesiredEnabled || registration.Reconciliation.Required {
		initialConsumerState = "STOPPED"
	}
	w.consumerState.Store(initialConsumerState)
	if cfg.ManagedHooks.Enabled {
		w.hooks = hookdistribution.NewRemote(cfg.ManagedHooks.CacheRoot, func(ctx context.Context, key string) (io.ReadCloser, error) {
			return artifacts.Get(ctx, key)
		})
	}
	// Result Processor releases are mandatory for P0.9 Tasks and use a
	// separate Worker-owned cache. This remains available for tasks without a
	// Hook configuration, so managedHooks.enabled does not become a hidden dependency
	// of Result Processing.
	w.processors = processordistribution.NewRemote(filepath.Join(cfg.Worker.WorkspaceRoot, ".seven", "result-processors"), func(ctx context.Context, key string) (io.ReadCloser, error) {
		return artifacts.Get(ctx, key)
	})
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	// Recovery is fenced by the newly registered Worker session, which must be
	// visible to ReviewRun control before any local container is considered.
	if err := w.sendHeartbeat(heartbeatCtx); err != nil {
		fatal("publish initial Worker heartbeat", err)
	}
	go w.heartbeatLoop(heartbeatCtx)
	go w.inventoryLoop(heartbeatCtx)
	go w.workspaceMaintenanceLoop(heartbeatCtx)
	w.recoverReviewContainers(heartbeatCtx)
	go w.reviewRuntimeMaintenanceLoop(heartbeatCtx)
	if registration.Reconciliation.Required {
		if err := w.reconcileRuntimes(ctx, registration.Reconciliation); err != nil {
			fatal("reconcile Worker runtimes", err)
		}
		if registration.DesiredState == registrydomain.DesiredEnabled {
			w.consumerState.Store("CONSUMING")
		}
	}
	w.cleanupReconciling.Store(false)
	go w.resourceCleanupLoop(heartbeatCtx)
	logger.Info("Worker registered", zap.Int64("worker_id", cfg.Worker.ID), zap.String("instance_id", instanceID), zap.String("host_boot_id", hostBootID), zap.String("queue", registration.Queue.Queue), zap.Int("admitted_images", len(images)))
	go w.consumePostprocess(ctx, broker, registration.Queue.PostprocessQueue)
	err = w.consume(ctx, broker)
	heartbeatCancel()
	deregisterCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = client.Deregister(deregisterCtx, "PROCESS_EXIT")
	cancel()
	if err != nil && ctx.Err() == nil {
		fatal("consume assignments", err)
	}
}

// workspaceMaintenanceLoop keeps the Worker-local, same-host cache bounded
// without making it part of the durable Task state machine. Server authority
// is consulted for every candidate, so a connectivity failure only retains
// data; it never authorizes deletion.
func (w *worker) workspaceMaintenanceLoop(ctx context.Context) {
	w.collectExpiredWorkspaces(ctx)
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.collectExpiredWorkspaces(ctx)
		}
	}
}

// reviewRuntimeMaintenanceLoop is intentionally local-only: it removes
// labelled report/relay volumes that no longer have a validated Reviewer
// container. It never creates, claims, or transitions a Server ReviewRun.
func (w *worker) reviewRuntimeMaintenanceLoop(ctx context.Context) {
	if w == nil || w.reviewRuntime == nil || !w.cfg.Reviewer.Enabled {
		return
	}
	clean := func() {
		containers, err := w.reviewRuntime.ManagedContainers(ctx)
		if err != nil {
			w.logger.Warn("scan independent review runtime cleanup", zap.Error(err))
			return
		}
		if err = w.reviewRuntime.CleanupOrphanReportVolumes(ctx, containers); err != nil {
			w.logger.Warn("clean orphan independent review report volumes", zap.Error(err))
		}
		if err = w.reviewRuntime.CleanupOrphanProviderRelays(ctx, containers); err != nil {
			w.logger.Warn("clean orphan independent review provider relays", zap.Error(err))
		}
	}
	clean()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clean()
		}
	}
}

func (w *worker) collectExpiredWorkspaces(parent context.Context) {
	if w == nil || w.workspace == nil || w.control == nil || w.logger == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	report, err := w.workspace.GarbageCollect(ctx, workspaceGCAuthority{control: w.control}, func(audit attemptruntime.GCAudit) {
		w.logger.Info("persistent workspace retention",
			zap.Int64("task_id", audit.TaskID), zap.String("disposition", audit.Disposition), zap.String("reason_code", audit.ReasonCode))
	})
	if err != nil {
		w.logger.Warn("persistent workspace retention failed closed", zap.Error(err))
		return
	}
	if report.Deleted > 0 || report.Skipped > 0 {
		w.logger.Info("persistent workspace retention completed", zap.Int("deleted", report.Deleted), zap.Int("skipped", report.Skipped))
	}
}

func (w *worker) consume(ctx context.Context, broker *messaging.Broker) error {
	// Agent Attempts and independent ReviewRuns have their own capacity limits,
	// but every container consumes one slot from this Worker-wide hard ceiling.
	// It is a field rather than a consumer-local channel so restart recovery
	// cannot bypass the same protection while reattaching a ReviewRun.
	for {
		if w.currentConsumerState() != "CONSUMING" {
			select {
			case <-ctx.Done():
				return w.waitForAttempts(ctx.Err())
			case directive := <-w.directives:
				if directive == "CONSUME" {
					w.consumerState.Store("CONSUMING")
				}
				continue
			}
		}
		deliveries, closeConsumer, err := broker.Consume()
		if err != nil {
			w.consumerState.Store("ERROR")
			return err
		}
		consume := true
		for consume {
			select {
			case <-ctx.Done():
				_ = closeConsumer()
				w.consumerState.Store("STOPPED")
				return w.waitForAttempts(ctx.Err())
			case directive := <-w.directives:
				if directive == "DRAIN" || directive == "STOP" {
					_ = closeConsumer()
					w.consumerState.Store("STOPPED")
					consume = false
				}
			case <-w.consumerRestart:
				// An ACK result was uncertain after Server RUNNING was already
				// persisted. Closing the channel is the RabbitMQ recovery
				// boundary: unacked deliveries are redelivered and the Machine
				// API returns ACK_ALREADY_RUNNING without restarting Harness.
				_ = closeConsumer()
				consume = false
			case delivery, ok := <-deliveries:
				if !ok {
					w.consumerState.Store("ERROR")
					return errors.New("RabbitMQ delivery channel closed")
				}
				select {
				case w.containerSlots <- struct{}{}:
					w.attempts.Add(1)
					go func(delivery amqp.Delivery) {
						defer w.attempts.Done()
						defer func() { <-w.containerSlots }()
						w.handleDelivery(context.Background(), delivery)
					}(delivery)
				case <-ctx.Done():
					_ = delivery.Nack(false, true)
					return ctx.Err()
				}
			}
		}
	}
}

func (w *worker) waitForAttempts(shutdownErr error) error {
	done := make(chan struct{})
	go func() {
		w.attempts.Wait()
		close(done)
	}()
	select {
	case <-done:
		return shutdownErr
	case <-time.After(120 * time.Second):
		// Active containers are intentionally left intact. A new Worker instance
		// on the same host boot will reconcile them through Supervisor v2.
		return shutdownErr
	}
}

func (w *worker) consumePostprocess(ctx context.Context, broker *messaging.Broker, queue string) {
	for ctx.Err() == nil {
		deliveries, closeConsumer, err := broker.ConsumeQueue(queue, 4)
		if err != nil {
			w.logger.Warn("open post-process consumer", zap.Error(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		consume := true
		for consume {
			select {
			case <-ctx.Done():
				_ = closeConsumer()
				return
			case delivery, ok := <-deliveries:
				if !ok {
					consume = false
					continue
				}
				if w.currentConsumerState() != "CONSUMING" {
					_ = delivery.Nack(false, true)
					_ = closeConsumer()
					consume = false
					continue
				}
				w.attempts.Add(1)
				go func(d amqp.Delivery) { defer w.attempts.Done(); w.handlePostprocess(context.Background(), d) }(delivery)
			}
		}
	}
}

func (w *worker) handlePostprocess(ctx context.Context, delivery amqp.Delivery) {
	var header struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		MessageID     int64  `json:"messageId,string"`
		WorkerID      int64  `json:"workerId,string"`
		Generation    int64  `json:"generation,string"`
	}
	if len(delivery.Body) > 64<<10 || json.Unmarshal(delivery.Body, &header) != nil || header.SchemaVersion != 1 || header.MessageID == 0 || header.WorkerID != w.cfg.Worker.ID || header.Generation < 1 {
		_ = delivery.Reject(false)
		return
	}
	var err error
	var resourceID int64
	switch header.Kind {
	case artifactdomain.FinalizationMessageKind:
		var message artifactdomain.WorkMessage
		if strictJSON(delivery.Body, &message) != nil || message.ResourceID == 0 {
			_ = delivery.Reject(false)
			return
		}
		resourceID = message.ResourceID
		w.finalizationSlots <- struct{}{}
		defer func() { <-w.finalizationSlots }()
		err = w.runFinalization(ctx, message)
	case artifactdomain.UploadMessageKind:
		var message artifactdomain.WorkMessage
		if strictJSON(delivery.Body, &message) != nil || message.ResourceID == 0 {
			_ = delivery.Reject(false)
			return
		}
		resourceID = message.ResourceID
		w.uploadSlots <- struct{}{}
		defer func() { <-w.uploadSlots }()
		err = w.runArtifactUpload(ctx, message)
	case processorinfra.FixtureMessageKind:
		var message processorinfra.FixtureMessage
		if strictJSON(delivery.Body, &message) != nil || message.RunID == 0 {
			_ = delivery.Reject(false)
			return
		}
		resourceID = message.RunID
		w.fixtureSlots <- struct{}{}
		defer func() { <-w.fixtureSlots }()
		err = w.runProcessorFixture(ctx, message)
	case continuationdomain.MaterializationMessageKind:
		var message continuationdomain.MaterializationMessage
		if strictJSON(delivery.Body, &message) != nil || message.ResourceID == 0 {
			_ = delivery.Reject(false)
			return
		}
		resourceID = message.ResourceID
		w.finalizationSlots <- struct{}{}
		defer func() { <-w.finalizationSlots }()
		err = w.runMaterialization(ctx, message)
	case deliverydomain.MessageKind:
		var message deliverydomain.Message
		if strictJSON(delivery.Body, &message) != nil || message.ResourceID == 0 {
			_ = delivery.Reject(false)
			return
		}
		resourceID = message.ResourceID
		w.deliverySlots <- struct{}{}
		defer func() { <-w.deliverySlots }()
		err = w.runDelivery(ctx, message)
	case workspaceinputdomain.SourceIngestionMessageKind:
		var message workspaceinputdomain.SourceIngestionMessage
		if strictJSON(delivery.Body, &message) != nil || message.ResourceID == 0 {
			_ = delivery.Reject(false)
			return
		}
		resourceID = message.ResourceID
		w.sourceSlots <- struct{}{}
		defer func() { <-w.sourceSlots }()
		err = w.runSourceIngestion(ctx, message)
	default:
		_ = delivery.Reject(false)
		return
	}
	if err == nil {
		_ = delivery.Ack(false)
		return
	}
	var apiErr *control.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.StatusCode == 409 || apiErr.ReasonCode == "POSTPROCESS_IDENTITY_CONFLICT") {
		w.logger.Warn("post-process work was fenced by Server",
			zap.String("kind", header.Kind),
			zap.Int64("resource_id", resourceID),
			zap.Int("status_code", apiErr.StatusCode),
			zap.String("reason_code", apiErr.ReasonCode))
		// The Server is the work-state authority. Terminal or stale duplicate
		// notifications are safe to acknowledge and must not hot-loop.
		_ = delivery.Ack(false)
		return
	}
	w.logger.Warn("post-process work failed", zap.String("kind", header.Kind), zap.Int64("resource_id", resourceID), zap.Error(err))
	select {
	case <-ctx.Done():
		_ = delivery.Nack(false, true)
		return
	case <-time.After(time.Second):
	}
	_ = delivery.Nack(false, true)
}

type frozenEvidence struct {
	taskID    int64
	attemptID int64
	checks    []taskinfra.CheckEvidence
	events    []taskdomain.Event
}

func (e frozenEvidence) CheckEvidence(_ context.Context, taskID int64, attemptID int64) ([]taskinfra.CheckEvidence, error) {
	if taskID != e.taskID || attemptID != e.attemptID {
		return nil, errors.New("finalization evidence identity mismatch")
	}
	return append([]taskinfra.CheckEvidence(nil), e.checks...), nil
}
func (e frozenEvidence) Events(_ context.Context, taskID int64, afterID int64, limit int) ([]taskdomain.Event, error) {
	if taskID != e.taskID {
		return nil, errors.New("finalization evidence identity mismatch")
	}
	result := make([]taskdomain.Event, 0, len(e.events))
	for _, event := range e.events {
		if event.ID > afterID {
			result = append(result, event)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}
func (e frozenEvidence) RecordArtifact(context.Context, taskdomain.Artifact) error {
	return errors.New("FinalizationRun cannot publish artifact metadata directly")
}

func (w *worker) runFinalization(ctx context.Context, message artifactdomain.WorkMessage) error {
	claim, err := w.control.ClaimFinalization(ctx, message)
	if err != nil {
		return err
	}
	if w.faults.consume("fail-finalization-once") {
		return w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FAULT_INJECTED_FINALIZATION")
	}
	var item taskdomain.WorkItem
	var attempt taskdomain.Attempt
	var checks []taskinfra.CheckEvidence
	var events []taskdomain.Event
	var resultContract processordomain.ResultContract
	if strictJSON(claim.WorkItem, &item) != nil || strictJSON(claim.Attempt, &attempt) != nil || json.Unmarshal(claim.Checks, &checks) != nil || json.Unmarshal(claim.Events, &events) != nil || json.Unmarshal(claim.ResultContract, &resultContract) != nil {
		_ = w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_INPUT_INVALID")
		return nil
	}
	if item.TaskID != claim.TaskID || item.TaskTurnID < 1 || item.TaskTurnID != claim.TaskTurnID || claim.TaskTurnID < 1 || attempt.ID != claim.AttemptID {
		_ = w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_INPUT_IDENTITY_INVALID")
		return nil
	}
	if _, contractErr := processordomain.ValidateContract(claim.ResultContract); contractErr != nil || resultContract.Decision != "PASS" || claim.EvaluationCycleID == 0 {
		_ = w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_INPUT_INVALID")
		return nil
	}
	attempt.TaskID = item.TaskID
	workspace, workspaceErr := w.runtime.ExistingWorkspace(item, attempt)
	if workspaceErr != nil {
		_ = w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_WORKSPACE_PATH_INVALID")
		return nil
	}
	if info, statErr := os.Stat(workspace.Repo); statErr != nil || !info.IsDir() {
		_ = w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_SOURCE_LOST")
		return nil
	}
	resultWorkspace, resultPathErr := w.workspace.Result(item.TaskID, item.TaskTurnID, claim.RunID)
	if resultPathErr != nil {
		_ = w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_RESULT_PATH_INVALID")
		return nil
	}
	spoolParent := resultWorkspace.TurnRoot
	if err = w.workspace.PrepareResultParent(resultWorkspace); err != nil {
		_ = w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_SPOOL_UNAVAILABLE")
		return nil
	}
	finalizer := artifactapp.NewFinalizer(frozenEvidence{taskID: item.TaskID, attemptID: attempt.ID, checks: checks, events: events}, w.artifacts)
	frozen, freezeErr := finalizer.Freeze(ctx, item, attempt, workspace.Repo, claim.Completion, artifactapp.ResultPlan{Outputs: resultContract.Outputs, Actions: resultContract.Actions, ContinuationIncludes: resultContract.Continuation.Includes}, spoolParent, claim.RunID)
	if freezeErr != nil {
		if reportErr := w.control.FailFinalization(context.Background(), message, claim.LeaseToken, "FINALIZATION_FAILED"); reportErr != nil {
			return reportErr
		}
		return nil
	}
	result := artifactdomain.FreezeResult{ResultCommit: frozen.Commit, ResultTree: frozen.Tree, ManifestDigest: frozen.ManifestDigest, SpoolIdentity: frozen.SpoolIdentity, Artifacts: make([]artifactdomain.FrozenArtifact, 0, len(frozen.Artifacts)), ReviewEvidence: make([]artifactdomain.ReviewEvidence, 0, len(frozen.ReviewEvidence))}
	for _, artifact := range frozen.Artifacts {
		result.Artifacts = append(result.Artifacts, artifactdomain.FrozenArtifact{Type: artifact.Type, Name: artifact.Name, SourcePath: artifact.SourcePath, MediaType: artifact.MediaType, Path: artifact.Path, SHA256: artifact.SHA256, SizeBytes: artifact.SizeBytes})
	}
	for _, evidence := range frozen.ReviewEvidence {
		result.ReviewEvidence = append(result.ReviewEvidence, artifactdomain.ReviewEvidence{Path: evidence.Path, LineCount: evidence.LineCount})
	}
	return w.control.FreezeFinalization(ctx, message, claim.LeaseToken, result)
}

func (w *worker) runArtifactUpload(ctx context.Context, message artifactdomain.WorkMessage) error {
	claim, err := w.control.ClaimUpload(ctx, message)
	if err != nil {
		return err
	}
	var spoolRoot string
	if claim.SpoolKind == "MATERIALIZATION" {
		spoolRoot = filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "materialization", strconv.FormatInt(claim.MaterializationRunID, 10))
	} else {
		resultWorkspace, pathErr := w.workspace.Result(claim.TaskID, claim.TaskTurnID, claim.FinalizationRunID)
		if pathErr != nil {
			_ = w.control.FailUpload(context.Background(), message, claim.LeaseToken, "ARTIFACT_SPOOL_IDENTITY_INVALID")
			return nil
		}
		spoolRoot = resultWorkspace.Root
	}
	root, err := filepath.Abs(spoolRoot)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.Clean(claim.SpoolRelativePath)))
	if err != nil || filepath.Dir(path) != root {
		_ = w.control.FailUpload(context.Background(), message, claim.LeaseToken, "ARTIFACT_SPOOL_IDENTITY_INVALID")
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil || int64(len(body)) != claim.SizeBytes || sha256Hex(body) != claim.SHA256 {
		_ = w.control.FailUpload(context.Background(), message, claim.LeaseToken, "ARTIFACT_SPOOL_DIGEST_MISMATCH")
		return nil
	}
	if w.faults.consume("fail-upload-before-write-once") {
		return w.control.FailUpload(context.Background(), message, claim.LeaseToken, "FAULT_INJECTED_UPLOAD_BEFORE_WRITE")
	}
	if err = w.artifacts.Put(ctx, claim.ObjectKey, claim.MediaType, bytes.NewReader(body), int64(len(body))); err != nil {
		_ = w.control.FailUpload(context.Background(), message, claim.LeaseToken, "ARTIFACT_UPLOAD_FAILED")
		return nil
	}
	if w.faults.consume("fail-readback-once") {
		return w.control.FailUpload(context.Background(), message, claim.LeaseToken, "FAULT_INJECTED_READBACK")
	}
	object, err := w.artifacts.Get(ctx, claim.ObjectKey)
	if err != nil {
		_ = w.control.FailUpload(context.Background(), message, claim.LeaseToken, "ARTIFACT_READBACK_FAILED")
		return nil
	}
	readback, readErr := io.ReadAll(io.LimitReader(object, claim.SizeBytes+1))
	closeErr := object.Close()
	if readErr != nil || closeErr != nil || int64(len(readback)) != claim.SizeBytes || sha256Hex(readback) != claim.SHA256 {
		_ = w.control.FailUpload(context.Background(), message, claim.LeaseToken, "ARTIFACT_READBACK_DIGEST_MISMATCH")
		return nil
	}
	return w.control.VerifyUpload(ctx, message, claim.LeaseToken)
}

func (w *worker) runMaterialization(ctx context.Context, message continuationdomain.MaterializationMessage) error {
	claim, err := w.control.ClaimMaterialization(ctx, message)
	if err != nil {
		return err
	}
	fail := func(reason string) error {
		if reportErr := w.control.FailMaterialization(context.Background(), message, claim.LeaseToken, reason); reportErr != nil {
			return reportErr
		}
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(claim.Key)
	if err != nil || len(key) != 32 || claim.Restore.EncryptionAlgorithm != "AES-256-GCM-CHUNKED" || len(claim.Restore.CheckpointIdentity) != 64 {
		return fail("RESULT_MATERIALIZATION_KEY_INVALID")
	}
	spoolRoot := filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "materialization", strconv.FormatInt(claim.RunID, 10))
	if err = os.RemoveAll(spoolRoot); err != nil {
		return fail("RESULT_MATERIALIZATION_SPOOL_UNAVAILABLE")
	}
	if err = os.MkdirAll(spoolRoot, 0o700); err != nil {
		return fail("RESULT_MATERIALIZATION_SPOOL_UNAVAILABLE")
	}
	workspaceRoot := filepath.Join(spoolRoot, "workspace")
	if claim.Restore.FormatVersion != 2 {
		return fail("CONTINUATION_CHECKPOINT_FORMAT_UNSUPPORTED")
	}
	if err = w.restoreBundleOverlay(ctx, bundleOverlayRestore{
		CheckpointIdentity: claim.Restore.CheckpointIdentity,
		BundleObjectKey:    claim.Restore.RepositoryBundleObjectKey, BundleObjectSHA256: claim.Restore.RepositoryBundleObjectSHA256, BundleContentSHA256: claim.Restore.RepositoryBundleContentSHA256, BundleSizeBytes: claim.Restore.RepositoryBundleSizeBytes,
		OverlayObjectKey: claim.Restore.WorkspaceOverlayObjectKey, OverlayObjectSHA256: claim.Restore.WorkspaceOverlayObjectSHA256, OverlayContentSHA256: claim.Restore.WorkspaceOverlayContentSHA256, OverlaySizeBytes: claim.Restore.WorkspaceOverlaySizeBytes,
		ExpectedCommit: claim.ExpectedCommit, ExpectedTree: claim.ExpectedTree,
	}, key, spoolRoot, workspaceRoot); err != nil {
		w.logger.Warn("restore materialization source", zap.Int64("run_id", claim.RunID), zap.Error(err))
		return fail("RESULT_MATERIALIZATION_SOURCE_TAMPERED")
	}
	var name, mediaType, artifactPath string
	var selectionManifest json.RawMessage
	var selectionManifestDigest string
	var selectedFileCount int
	switch claim.Kind {
	case continuationdomain.MaterializationDeclaredOutput, continuationdomain.MaterializationZIP, continuationdomain.MaterializationTarGZ:
		if claim.OutputName == "" {
			return fail("RESULT_MATERIALIZATION_DECLARATION_INVALID")
		}
		if claim.OutputKind == "ARCHIVE" || claim.Kind == continuationdomain.MaterializationZIP || claim.Kind == continuationdomain.MaterializationTarGZ {
			format := claim.ArchiveFormat
			if claim.Kind == continuationdomain.MaterializationZIP || claim.Kind == continuationdomain.MaterializationTarGZ {
				format = claim.Kind
			}
			var selection processordomain.PathSelection
			decoder := json.NewDecoder(strings.NewReader(claim.SelectionSpec))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&selection) != nil || len(selection.Includes) == 0 {
				return fail("RESULT_MATERIALIZATION_DECLARATION_INVALID")
			}
			name = claim.OutputName
			if format == "ZIP" {
				name += ".zip"
				mediaType = "application/zip"
			} else if format == "TAR_GZ" {
				name += ".tar.gz"
				mediaType = "application/gzip"
			} else {
				return fail("RESULT_MATERIALIZATION_DECLARATION_INVALID")
			}
			artifactPath = filepath.Join(spoolRoot, name)
			var entries []processorcheckpoint.SelectionEntry
			if _, entries, err = processorcheckpoint.PackSelectionArchive(ctx, workspaceRoot, selection, format, artifactPath); err != nil {
				return fail("RESULT_MATERIALIZATION_PACK_FAILED")
			}
			selectionManifest, err = json.Marshal(entries)
			if err != nil {
				return fail("RESULT_MATERIALIZATION_PACK_FAILED")
			}
			selectionManifestDigest = sha256Hex(selectionManifest)
			selectedFileCount = len(entries)
			break
		}
		if claim.OutputSourcePath == "" {
			return fail("RESULT_MATERIALIZATION_DECLARATION_INVALID")
		}
		root, _ := filepath.Abs(workspaceRoot)
		source, absErr := filepath.Abs(filepath.Join(root, filepath.Clean(claim.OutputSourcePath)))
		if absErr != nil || (source != root && !strings.HasPrefix(source, root+string(os.PathSeparator))) {
			return fail("RESULT_MATERIALIZATION_PATH_INVALID")
		}
		info, statErr := os.Lstat(source)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return fail("RESULT_MATERIALIZATION_SOURCE_MISSING")
		}
		name = claim.OutputName
		if info.IsDir() {
			return fail("RESULT_MATERIALIZATION_ARCHIVE_FORMAT_REQUIRED")
		} else if info.Mode().IsRegular() && info.Size() <= processorcheckpoint.MaxFileBytes {
			mediaType = "application/octet-stream"
			artifactPath = filepath.Join(spoolRoot, name)
			if err = copyMaterializationFile(source, artifactPath, processorcheckpoint.MaxFileBytes); err != nil {
				return fail("RESULT_MATERIALIZATION_PACK_FAILED")
			}
		} else {
			return fail("RESULT_MATERIALIZATION_FILE_TYPE_FORBIDDEN")
		}
	case continuationdomain.MaterializationPatch:
		if claim.ExpectedBaseCommit == "" || claim.ExpectedCommit == "" {
			return fail("RESULT_MATERIALIZATION_IDENTITY_MISMATCH")
		}
		name, mediaType, artifactPath = "result.patch", "text/x-diff", filepath.Join(spoolRoot, "result.patch")
		if err = runMaterializationCommand(ctx, artifactPath, processorcheckpoint.MaxArchiveBytes, "git", "-C", workspaceRoot, "diff", "--binary", "--full-index", claim.ExpectedBaseCommit, claim.ExpectedCommit, "--"); err != nil {
			return fail("RESULT_MATERIALIZATION_PATCH_FAILED")
		}
	case continuationdomain.MaterializationGitBundle, continuationdomain.MaterializationBundle:
		if claim.ExpectedCommit == "" {
			return fail("RESULT_MATERIALIZATION_IDENTITY_MISMATCH")
		}
		name, mediaType, artifactPath = "result.bundle", "application/x-git-bundle", filepath.Join(spoolRoot, "result.bundle")
		ref := "refs/seven/materialization/" + strconv.FormatInt(claim.RunID, 10)
		if output, refErr := exec.CommandContext(ctx, "git", "-C", workspaceRoot, "update-ref", ref, claim.ExpectedCommit).CombinedOutput(); refErr != nil || len(output) > 8<<10 {
			return fail("RESULT_MATERIALIZATION_BUNDLE_FAILED")
		}
		if output, bundleErr := exec.CommandContext(ctx, "git", "-C", workspaceRoot, "bundle", "create", artifactPath, ref).CombinedOutput(); bundleErr != nil || len(output) > 8<<10 {
			return fail("RESULT_MATERIALIZATION_BUNDLE_FAILED")
		}
	default:
		return fail("RESULT_MATERIALIZATION_KIND_INVALID")
	}
	info, err := os.Stat(artifactPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > processorcheckpoint.MaxArchiveBytes {
		return fail("RESULT_MATERIALIZATION_RESULT_INVALID")
	}
	digest, err := digestMaterializationFile(artifactPath)
	if err != nil {
		return fail("RESULT_MATERIALIZATION_RESULT_INVALID")
	}
	spoolIdentity := sha256Hex([]byte(strings.Join([]string{strconv.FormatInt(claim.RunID, 10), claim.Kind, name, digest, fmt.Sprint(info.Size())}, "\x00")))
	return w.control.FreezeMaterialization(ctx, message, claim.LeaseToken, continuationdomain.MaterializationFreeze{Name: name, MediaType: mediaType, Path: filepath.Base(artifactPath), SHA256: digest, SizeBytes: info.Size(), SpoolIdentity: spoolIdentity, SelectionManifest: selectionManifest, SelectionManifestDigest: selectionManifestDigest, SelectedFileCount: selectedFileCount})
}

func copyMaterializationFile(source, destination string, limit int64) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(in, limit+1))
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written > limit {
		_ = os.Remove(destination + ".tmp")
		return errors.New("RESULT_MATERIALIZATION_FILE_TOO_LARGE")
	}
	return os.Rename(destination+".tmp", destination)
}

func runMaterializationCommand(ctx context.Context, destination string, limit int64, name string, args ...string) error {
	file, err := os.OpenFile(destination+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	limited := &materializationLimitWriter{writer: file, remaining: limit}
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout, command.Stderr = limited, io.Discard
	err = command.Run()
	syncErr := file.Sync()
	closeErr := file.Close()
	if err != nil || limited.exceeded || syncErr != nil || closeErr != nil {
		_ = os.Remove(destination + ".tmp")
		return errors.New("RESULT_MATERIALIZATION_COMMAND_FAILED")
	}
	return os.Rename(destination+".tmp", destination)
}

type materializationLimitWriter struct {
	writer    io.Writer
	remaining int64
	exceeded  bool
}

func (w *materializationLimitWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		w.exceeded = true
		return 0, errors.New("limit exceeded")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}
func digestMaterializationFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err = io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func sha256Hex(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

func (w *worker) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	if len(delivery.Body) > 384<<10 {
		w.logger.Warn("reject oversized Worker assignment message", zap.String("message_id", delivery.MessageId))
		_ = delivery.Reject(false)
		return
	}
	header, err := deliveryHeader(delivery.Body)
	if err != nil || header.SchemaVersion != 1 {
		w.logger.Warn("reject invalid Worker assignment message", zap.String("message_id", delivery.MessageId))
		_ = delivery.Reject(false)
		return
	}
	if header.Kind == reviewrundomain.AssignmentMessageKind {
		w.handleReviewDelivery(ctx, delivery)
		return
	}
	var message dispatchdomain.AssignmentMessage
	if len(delivery.Body) > 64<<10 || strictJSON(delivery.Body, &message) != nil || message.SchemaVersion != 1 || message.Kind != dispatchdomain.AssignmentSchema || message.AssignmentID == 0 || message.WorkerID != w.cfg.Worker.ID || message.Generation < 1 {
		w.logger.Warn("reject invalid assignment message", zap.String("message_id", delivery.MessageId))
		_ = delivery.Reject(false)
		return
	}
	if w.currentConsumerState() != "CONSUMING" {
		if err := w.control.ReleaseAssignment(ctx, message, "WORKER_DRAINING"); err != nil {
			w.logger.Warn("release unclaimed Assignment while draining", zap.Int64("assignment_id", message.AssignmentID), zap.Error(err))
			_ = delivery.Nack(false, true)
			return
		}
		_ = delivery.Ack(false)
		return
	}
	select {
	case w.agentSlots <- struct{}{}:
		defer func() { <-w.agentSlots }()
	case <-ctx.Done():
		_ = delivery.Nack(false, true)
		return
	}
	w.active.Add(1)
	deliverySettled, err := w.process(ctx, message, func() error { return delivery.Ack(false) })
	w.active.Add(-1)
	if deliverySettled {
		if err != nil {
			w.logger.Error("assignment failed after RUNNING delivery was detached", zap.Int64("assignment_id", message.AssignmentID), zap.Int64("task_id", message.TaskID), zap.Error(err))
		}
		return
	}
	if terminalMachineError(err) {
		_ = delivery.Ack(false)
		return
	}
	if errors.Is(err, errReconciliationRequired) {
		w.logger.Error("assignment requires operator reconciliation", zap.Int64("assignment_id", message.AssignmentID), zap.Int64("task_id", message.TaskID))
		_ = delivery.Nack(false, true)
		return
	}
	w.logger.Error("assignment processing did not reach a persisted terminal state", zap.Int64("assignment_id", message.AssignmentID), zap.Int64("task_id", message.TaskID), zap.Error(err))
	select {
	case <-ctx.Done():
		_ = delivery.Nack(false, true)
		return
	case <-time.After(time.Second):
	}
	_ = delivery.Nack(false, true)
}

func (w *worker) process(ctx context.Context, message dispatchdomain.AssignmentMessage, acknowledge func() error) (bool, error) {
	contract, err := w.control.Claim(ctx, message)
	if err != nil {
		return false, err
	}
	switch contract.Directive {
	case dispatchdomain.DirectiveAckAlreadyRunning, dispatchdomain.DirectiveAckTerminal, dispatchdomain.DirectiveRejectStaleAssignment:
		if err := acknowledge(); err != nil {
			return false, fmt.Errorf("ack Assignment directive %s: %w", contract.Directive, err)
		}
		return true, nil
	case dispatchdomain.DirectiveReconcileLiveRuntime:
		return false, errReconciliationRequired
	case dispatchdomain.DirectiveStartNewRuntime, dispatchdomain.DirectiveResumeAuthorizedRuntime:
	default:
		return false, fmt.Errorf("unknown Claim directive %q", contract.Directive)
	}
	item, attempt := contract.WorkItem, contract.Attempt
	attempt.LeaseToken = contract.AttemptLeaseToken
	attempt.LeaseExpiresAt = contract.AttemptLeaseExpiresAt
	attempt.ExecutionNodeID = contract.ExecutionNodeID
	attempt.AssignmentID = contract.AssignmentID
	attempt.WorkerID = w.cfg.Worker.ID
	attempt.WorkerInstance = w.instance
	attempt.HostBootID = w.hostBoot
	attempt.OwnerEpoch = contract.OwnerEpoch
	leaseCtx, stopLease := context.WithCancel(ctx)
	defer stopLease()
	var activeBudget atomic.Pointer[context.Context]
	var activeContainer atomic.Value
	go w.renewLeaseLoop(leaseCtx, item.TaskID, attempt, func() string {
		value := activeContainer.Load()
		if value == nil {
			return ""
		}
		return value.(string)
	}, func() *int64 { return currentBudgetMillis(&activeBudget) })
	var eventSequence atomic.Int64
	nextEvent := func(eventType string, payload any) error {
		return w.control.AddEvent(context.Background(), attempt, eventType, payload, eventSequence.Add(1))
	}
	cancelling := func() bool {
		value, readErr := w.control.Control(context.Background(), attempt)
		return readErr == nil && value
	}
	var workspace attemptruntime.Workspace
	var baseCommit string
	if item.ContinuationRestore != nil {
		workspace, baseCommit, err = w.restoreContinuation(ctx, &item, attempt)
	} else {
		workspace, baseCommit, err = w.restoreTaskInput(ctx, item, attempt)
	}
	if err != nil {
		if errors.Is(err, errColdRestoreConfirmationPending) {
			// Local installation is durable, but Server confirmation is unknown.
			// Redelivery reconciles the same restore; it must not fail the Task.
			return false, err
		}
		if errors.Is(err, attemptruntime.ErrWorkspacePrepareTimeout) {
			return false, w.retryInfrastructureLoss(ctx, item, attempt, dispatchdomain.ReasonWorkspacePrepareInfrastructure, err)
		}
		return false, w.fail(ctx, attempt, "WORKSPACE_PREPARE_FAILED", err)
	}
	if err = w.ensurePlanningSource(ctx, item, workspace); err != nil {
		return false, w.fail(ctx, attempt, "PLANNING_SOURCE_PREPARE_FAILED", err)
	}
	item.ResolvedBaseCommit = baseCommit
	if err := nextEvent("workspace.ready", map[string]any{"baseCommit": baseCommit}); err != nil {
		return false, w.fail(ctx, attempt, "WORKSPACE_FACT_PERSIST_FAILED", err)
	}
	// Baseline and Harness share one execution budget. A check cannot add
	// another five minutes before the Agent's independent timeout starts.
	executionBudget := time.Duration(item.HarnessTimeoutSeconds) * time.Second
	budgetParent := ctx
	if w.interactionBudgets != nil {
		budgetParent = attemptruntime.WithBudgetRestoreProvider(ctx, w.interactionBudgets.Restore)
	}
	executionCtx, cancelExecution, budgetErr := withExecutionBudget(budgetParent, executionBudget, attemptruntime.BudgetIdentity{TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch}, item.InteractionBudgetRestored, item.ExecutionActiveMilliseconds)
	if budgetErr != nil {
		return false, w.fail(ctx, attempt, "BUDGET_RECOVERY_EVIDENCE_REQUIRED", budgetErr)
	}
	defer cancelExecution()
	budgetContext := context.Context(executionCtx)
	activeBudget.Store(&budgetContext)
	stopBudgetSave, budgetErr := w.interactionBudgets.persist(executionCtx, cancelExecution)
	if budgetErr != nil {
		return false, w.fail(ctx, attempt, "BUDGET_PERSIST_FAILED", budgetErr)
	}
	defer stopBudgetSave()
	// A cold network-tool resume must settle the persisted Operation against
	// the restored workspace before either baseline checks or the Adapter can
	// run. The resulting standalone tool output is then injected into the new
	// native turn; the old JSON-RPC request identity is never reused.
	if err := w.prepareColdExecutionOperation(executionCtx, &item, attempt, workspace); err != nil {
		return false, w.fail(ctx, attempt, "OPERATION_RESTORE_FAILED", err)
	}
	if err := w.runCompletionCheck(executionCtx, item, attempt, workspace, "BASELINE", 0); err != nil {
		return false, w.fail(ctx, attempt, "CHECK_BASELINE_RECORD_FAILED", err)
	}
	if cancelling() {
		return false, w.cancel(ctx, attempt)
	}
	permit, _, err := w.control.LaunchPermit(ctx, attempt.ID, attempt.Generation, attempt.OwnerEpoch, attempt.LeaseToken)
	if err != nil {
		return false, w.fail(ctx, attempt, "LAUNCH_PERMIT_FAILED", err)
	}
	attempt.ContainerName = permit.ExpectedContainerName
	attempt.RuntimeLabels = permit.Labels
	var hookMount *attemptruntime.HookMount
	if len(item.ExecutionSnapshot.PackageReleases) > 0 {
		if w.hooks == nil {
			return false, w.fail(ctx, attempt, "HOOK_MATERIALIZER_UNAVAILABLE", errors.New("managed Hook materializer is disabled"))
		}
		materialized, materializeErr := w.hooks.Materialize(ctx, item.TenantID, item.ExecutionSnapshot)
		if materializeErr != nil {
			return false, w.fail(ctx, attempt, "HOOK_MATERIALIZATION_FAILED", materializeErr)
		}
		hookMount = &attemptruntime.HookMount{Source: materialized.Source, Digest: materialized.Digest}
	}
	if w.processors == nil {
		return false, w.fail(ctx, attempt, "RESULT_PROCESSOR_MATERIALIZER_UNAVAILABLE", errors.New("Result Processor materializer is unavailable"))
	}
	processorMount, materializeErr := w.processors.Materialize(
		ctx,
		item.ExecutionSnapshot.ResultProcessorReleaseID,
		item.ExecutionSnapshot.ResultProcessorDigest,
		item.ResultProcessorArchiveObjectKey,
	)
	if materializeErr != nil {
		return false, w.fail(ctx, attempt, "RESULT_PROCESSOR_MATERIALIZATION_FAILED", materializeErr)
	}
	sink := func(eventType string, payload any) {
		if eventErr := nextEvent(eventType, payload); eventErr != nil {
			w.logger.Warn("persist runtime event", zap.Int64("task_id", item.TaskID), zap.Int64("attempt_id", attempt.ID), zap.Error(eventErr))
		}
	}
	deliverySettled := false
	gate := attemptruntime.RuntimeGate{
		DrainInteractions: func(drainCtx context.Context) error {
			if w.interactions == nil {
				return errInteractionTransport
			}
			return w.interactions.Drain(drainCtx, attempt)
		},
		SuspendInteraction: func(suspendCtx context.Context, channel *attemptruntime.ExecutionChannel) error {
			return w.suspendInteractionRuntime(suspendCtx, item, attempt, workspace, channel)
		},
		InteractionSession: func(sessionCtx context.Context, channel *attemptruntime.ExecutionChannel) (func(), error) {
			if w.interactions == nil {
				return nil, errInteractionTransport
			}
			var closePump func()
			var err error
			if item.SourceExecutionOperationID > 0 {
				closePump, err = w.interactions.RegisterColdResume(sessionCtx, item.TaskID, attempt, channel, item.SourceExecutionOperationID, item.SourceAdapterSessionGeneration)
			} else {
				closePump, err = w.interactions.Register(sessionCtx, item.TaskID, attempt, channel)
			}
			if err != nil {
				return nil, err
			}
			stopOperations := w.startExecutionOperationLoop(sessionCtx, item, attempt, workspace, channel)
			stopBudgetObservations := w.observeAcceptedBudgetQuestions(sessionCtx, attempt, channel)
			return func() {
				stopOperations()
				stopBudgetObservations()
				if closePump != nil {
					closePump()
				}
			}, nil
		},
		CheckEvaluation: func(checkCtx context.Context, cycleID int64) error {
			return w.runCompletionCheck(checkCtx, item, attempt, workspace, "CURRENT", cycleID)
		},
		ProviderProxyReady: func(ready dispatchdomain.ProviderProxyReady) error {
			return w.control.ProviderProxyReady(context.Background(), attempt, ready)
		},
		ContainerCreated: func(containerID string) error {
			activeContainer.Store(containerID)
			return w.control.ContainerCreated(context.Background(), attempt, containerID, item.HarnessImageDigest, permit.ExecutionIdentityDigest)
		},
		SupervisorReady: func(ready dispatchdomain.SupervisorReady) error {
			return w.control.SupervisorReady(context.Background(), attempt, ready)
		},
		AcquireRunPermit: func() (dispatchdomain.RunPermit, error) {
			return w.control.RunPermit(context.Background(), attempt)
		},
		HarnessStarted: func(started dispatchdomain.HarnessStarted) error {
			return w.control.HarnessStarted(context.Background(), attempt, started)
		},
		EvaluationReady: func(cycleID int64, cycleNumber int) error {
			return nextEvent("task.validating", map[string]any{
				"cycleId":     strconv.FormatInt(cycleID, 10),
				"cycleNumber": cycleNumber,
			})
		},
		MaterializeEvaluation: func(cycleID int64, cycleNumber int) (processorfacade.Materialization, error) {
			return w.control.ProcessorMaterialization(context.Background(), attempt, cycleID, cycleNumber)
		},
		ReadEvaluation: func(ctx context.Context, pending processorfacade.EvaluationReplayIdentity) (processorfacade.EvaluationReplay, error) {
			return w.control.ReadEvaluation(ctx, attempt, pending)
		},
		PersistEvaluation: func(candidate processorfacade.EvaluationCandidate) (processorfacade.EvaluationDirective, error) {
			return w.control.ResultEvaluation(context.Background(), attempt, candidate)
		},
		DeliveryStarted: func() error {
			// Once HARNESS_STARTED is durably accepted, this handler must not
			// NACK the original delivery even when ACK is uncertain. Closing
			// its channel causes RabbitMQ to redeliver and the repeated Claim
			// is settled by ACK_ALREADY_RUNNING.
			deliverySettled = true
			if err := acknowledge(); err != nil {
				select {
				case w.consumerRestart <- struct{}{}:
				default:
				}
				_ = nextEvent("assignment.delivery_ack_uncertain", map[string]any{"messageId": message.MessageID, "reasonCode": "BROKER_ACK_UNCERTAIN"})
				return fmt.Errorf("ack Assignment after RUNNING: %w", err)
			}
			_ = nextEvent("assignment.delivery_acked", map[string]any{"messageId": message.MessageID})
			return nil
		},
	}
	providerRuntime, err := providerRuntimeIdentity(item)
	if err != nil {
		return false, w.fail(ctx, attempt, "PROVIDER_RUNTIME_IDENTITY_INVALID", err)
	}
	runResult, err := w.runtime.Run(executionCtx, item, attempt, workspace, hookMount, &attemptruntime.ResultProcessorMount{Source: processorMount.Source, Digest: processorMount.Digest, ReleaseID: processorMount.ReleaseID}, providerRuntime, sink, gate, cancelling)
	if cancelling() {
		return deliverySettled, w.cancel(context.Background(), attempt)
	}
	if err != nil {
		if !deliverySettled && errors.Is(err, attemptruntime.ErrSupervisorStartupTimeout) {
			return false, w.retryInfrastructureLoss(ctx, item, attempt, dispatchdomain.ReasonSupervisorStartupInfrastructure, err)
		}
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") {
			return deliverySettled, w.timeout(ctx, attempt, "HARNESS_TIMEOUT", err)
		}
		return deliverySettled, w.fail(ctx, attempt, "ATTEMPT_RUNTIME_FAILED", err)
	}
	return deliverySettled, w.finishAttempt(ctx, item, attempt, workspace, runResult, cancelling)
}

// handleReviewDelivery is intentionally separate from Agent Attempt handling.
// A ReviewRun has its own lease/permit/attempt lifecycle and can never enter
// the Agent runtime, Coding Session, or mutable Attempt workspace.
func (w *worker) handleReviewDelivery(ctx context.Context, delivery amqp.Delivery) {
	var message reviewrundomain.AssignmentMessage
	if !w.cfg.Reviewer.Enabled || w.reviewRuntime == nil || len(delivery.Body) > 64<<10 || strictJSON(delivery.Body, &message) != nil || message.SchemaVersion != 1 || message.Kind != reviewrundomain.AssignmentMessageKind || message.ReviewRunID < 1 || message.WorkerID != w.cfg.Worker.ID || message.OwnerEpoch < 1 {
		_ = delivery.Reject(false)
		return
	}
	if w.currentConsumerState() != "CONSUMING" {
		_ = delivery.Nack(false, true)
		return
	}
	select {
	case w.reviewSlots <- struct{}{}:
		defer func() { <-w.reviewSlots }()
	case <-ctx.Done():
		_ = delivery.Nack(false, true)
		return
	}
	w.activeReviews.Add(1)
	settled, err := w.processReview(ctx, message, func() error { return delivery.Ack(false) })
	w.activeReviews.Add(-1)
	if settled || terminalMachineError(err) {
		return
	}
	if err != nil {
		w.logger.Warn("independent review delivery failed", zap.Int64("review_run_id", message.ReviewRunID), zap.Error(err))
	}
	_ = delivery.Nack(false, true)
}

func (w *worker) processReview(ctx context.Context, message reviewrundomain.AssignmentMessage, acknowledge func() error) (bool, error) {
	claim, err := w.control.ReviewClaim(ctx, message.ReviewRunID, message.OwnerEpoch)
	if err != nil {
		if terminalMachineError(err) {
			_ = acknowledge()
			return true, nil
		}
		return false, err
	}
	if claim.OwnerEpoch != message.OwnerEpoch || claim.ReviewRunID != message.ReviewRunID || claim.LeaseToken == "" {
		_ = acknowledge()
		return true, nil
	}
	// consume() has already reserved the Reviewer container slot. A provider-
	// enabled ReviewRun also owns its isolated relay sidecar, so reserve the
	// second physical slot before creating either container. This may wait for
	// an Agent to finish, but it never allows the relay to bypass the Worker-
	// wide maxTotalContainers ceiling.
	extraContainerSlots := reviewRuntimeContainerSlots(claim) - 1
	if !w.acquireTotalContainerSlots(ctx, extraContainerSlots) {
		return false, ctx.Err()
	}
	defer w.releaseTotalContainerSlots(extraContainerSlots)
	leaseCtx, cancelLease := context.WithCancel(ctx)
	defer cancelLease()
	leaseLost := make(chan error, 1)
	go w.renewReviewLeaseLoop(leaseCtx, claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, leaseLost)
	runtimeCtx, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()
	runtimeLost := make(chan error, 1)
	go func() {
		select {
		case lost := <-leaseLost:
			runtimeLost <- lost
			if lost != nil {
				cancelRuntime()
			}
		case <-leaseCtx.Done():
		}
	}()
	permit, err := w.control.ReviewLaunchPermit(ctx, claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken)
	if err != nil {
		return false, err
	}
	settled := false
	runResult, err := w.reviewRuntime.Run(runtimeCtx, reviewWorkFromClaim(claim, w.cfg.Worker.ID, w.instance, w.hostBoot), reviewruntime.Gate{ContainerCreated: func(container reviewruntime.Container) error {
		if err := w.control.ReviewContainerCreated(context.Background(), claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, permit, container.ID, container.Name, claim.ReviewerImage, container.ExecutionIdentityDigest); err != nil {
			return err
		}
		if err := acknowledge(); err != nil {
			return err
		}
		settled = true
		return nil
	}})
	select {
	case renewErr := <-runtimeLost:
		if renewErr != nil {
			return settled, renewErr
		}
	default:
	}
	if err != nil {
		if failureErr := w.control.ReviewFailure(context.Background(), claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, reviewFailureReason(err)); failureErr != nil {
			return settled, failureErr
		}
		if !settled {
			if ackErr := acknowledge(); ackErr != nil {
				return false, ackErr
			}
			settled = true
		}
		return settled, nil
	}
	if _, err = w.control.SubmitReviewReport(ctx, claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, permit, runResult.Report, runResult.Usage); err != nil {
		if terminalMachineError(err) {
			return true, nil
		}
		return settled, err
	}
	if !settled {
		if ackErr := acknowledge(); ackErr != nil {
			return false, ackErr
		}
		settled = true
	}
	return settled, nil
}

// recoverReviewContainers never reconstructs a ReviewRun from Docker. It
// merely presents labelled local containers to the Server, which either grants
// a fresh fenced lease for the exact durable identity or tells this Worker to
// remove the stale container. A transient Server error retains local evidence
// and relies on normal lease-expiry reconciliation rather than deleting work.
func (w *worker) recoverReviewContainers(ctx context.Context) {
	if w == nil || w.reviewRuntime == nil || !w.cfg.Reviewer.Enabled {
		return
	}
	containers, err := w.reviewRuntime.ManagedContainers(ctx)
	if err != nil {
		w.logger.Warn("scan managed independent review containers", zap.Error(err))
		return
	}
	if err = w.reviewRuntime.CleanupOrphanReportVolumes(ctx, containers); err != nil {
		w.logger.Warn("clean orphan independent review report volumes", zap.Error(err))
	}
	if err = w.reviewRuntime.CleanupOrphanProviderRelays(ctx, containers); err != nil {
		w.logger.Warn("clean orphan independent review provider relays", zap.Error(err))
	}
	for _, container := range containers {
		if container.WorkerID != w.cfg.Worker.ID {
			_ = w.reviewRuntime.StopAndRemove(context.Background(), container.ID)
			continue
		}
		claim, recoverErr := w.control.RecoverReviewRun(ctx, container.ReviewRunID, container.OwnerEpoch, container.ID, container.Name, container.ImageDigest, container.ExecutionIdentityDigest)
		if recoverErr != nil {
			if terminalMachineError(recoverErr) {
				_ = w.reviewRuntime.StopAndRemove(context.Background(), container.ID)
			} else {
				w.logger.Warn("retain review container pending recovery authority", zap.Int64("review_run_id", container.ReviewRunID), zap.Error(recoverErr))
			}
			continue
		}
		if claim.ReviewRunID != container.ReviewRunID || claim.OwnerEpoch != container.OwnerEpoch || claim.LeaseToken == "" {
			_ = w.reviewRuntime.StopAndRemove(context.Background(), container.ID)
			continue
		}
		reservedSlots := reviewRuntimeContainerSlots(claim)
		if !w.tryReserveTotalContainerSlots(reservedSlots) {
			_ = w.reviewRuntime.StopAndRemove(context.Background(), container.ID)
			_ = w.control.ReviewFailure(context.Background(), claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, "REVIEW_RECOVERY_CAPACITY_EXCEEDED")
			continue
		}
		w.attempts.Add(1)
		go func(claim control.ReviewClaim, container reviewruntime.ManagedContainer, reservedSlots int) {
			defer w.attempts.Done()
			defer w.releaseTotalContainerSlots(reservedSlots)
			w.resumeReviewContainer(ctx, claim, container)
		}(claim, container, reservedSlots)
	}
}

func (w *worker) resumeReviewContainer(ctx context.Context, claim control.ReviewClaim, container reviewruntime.ManagedContainer) {
	select {
	case w.reviewSlots <- struct{}{}:
		defer func() { <-w.reviewSlots }()
	case <-ctx.Done():
		return
	}
	w.activeReviews.Add(1)
	defer w.activeReviews.Add(-1)
	leaseCtx, cancelLease := context.WithCancel(ctx)
	defer cancelLease()
	leaseLost := make(chan error, 1)
	go w.renewReviewLeaseLoop(leaseCtx, claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, leaseLost)
	runtimeCtx, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()
	runtimeLost := make(chan error, 1)
	go func() {
		select {
		case lost := <-leaseLost:
			runtimeLost <- lost
			if lost != nil {
				cancelRuntime()
			}
		case <-leaseCtx.Done():
		}
	}()
	permit, err := w.control.ReviewLaunchPermit(ctx, claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken)
	if err != nil {
		w.logger.Warn("recover review launch permit", zap.Int64("review_run_id", claim.ReviewRunID), zap.Error(err))
		_ = w.reviewRuntime.StopAndRemove(context.Background(), container.ID)
		return
	}
	// Recover granted a fresh owner lease only after Server-side comparison of
	// the durable Run and local label identity. Re-recording that exact
	// container consumes the fresh short launch permit and converts it into the
	// lease-bound completion authority used by SubmitReviewReport. A different
	// container is fence-rejected by the Server.
	if err = w.control.ReviewContainerCreated(context.Background(), claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, permit, container.ID, container.Name, container.ImageDigest, container.ExecutionIdentityDigest); err != nil {
		w.logger.Warn("recover review container registration", zap.Int64("review_run_id", claim.ReviewRunID), zap.Error(err))
		_ = w.reviewRuntime.StopAndRemove(context.Background(), container.ID)
		return
	}
	result, err := w.reviewRuntime.Reattach(runtimeCtx, reviewWorkFromClaim(claim, w.cfg.Worker.ID, w.instance, w.hostBoot), container)
	select {
	case renewErr := <-runtimeLost:
		if renewErr != nil {
			return
		}
	default:
	}
	if err != nil {
		if failureErr := w.control.ReviewFailure(context.Background(), claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, reviewFailureReason(err)); failureErr != nil && !terminalMachineError(failureErr) {
			w.logger.Warn("record recovered review failure", zap.Int64("review_run_id", claim.ReviewRunID), zap.Error(failureErr))
		}
		return
	}
	if _, err = w.control.SubmitReviewReport(ctx, claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, permit, result.Report, result.Usage); err != nil && !terminalMachineError(err) {
		w.logger.Warn("submit recovered review report", zap.Int64("review_run_id", claim.ReviewRunID), zap.Error(err))
	}
}

func reviewWorkFromClaim(claim control.ReviewClaim, workerID int64, workerInstanceID, hostBootID string) reviewruntime.Work {
	deadline, _ := time.Parse(time.RFC3339Nano, claim.ExecutionDeadlineAt)
	leaseExpires, _ := time.Parse(time.RFC3339Nano, claim.LeaseExpiresAt)
	return reviewruntime.Work{ReviewRunID: claim.ReviewRunID, ReviewRunAttemptID: claim.ReviewRunAttemptID, WorkerID: workerID, WorkerInstanceID: workerInstanceID, HostBootID: hostBootID, TaskID: claim.TaskID, TaskTurnID: claim.TaskTurnID, FinalizationRunID: claim.FinalizationRunID, ResultCommit: claim.ResultCommit, ResultTree: claim.ResultTree, ReviewerImage: claim.ReviewerImage, OwnerEpoch: claim.OwnerEpoch, HardTimeout: time.Duration(claim.HardTimeoutSeconds) * time.Second, ExecutionDeadline: deadline, LeaseExpiresAt: leaseExpires, LeaseToken: claim.LeaseToken, ReviewContract: claim.ReviewContract, Requirements: claim.Requirements, ProviderProxyImageDigest: claim.ProviderProxyImageDigest, Provider: reviewruntime.ProviderIdentity{CredentialID: claim.ModelCredentialID, CredentialVersion: claim.CredentialVersion, Protocol: claim.ModelProtocol, PrimaryModel: claim.PrimaryModel, StructuredContract: claim.ProviderStructuredContract}}
}

func reviewRuntimeContainerSlots(claim control.ReviewClaim) int {
	if (reviewruntime.ProviderIdentity{CredentialID: claim.ModelCredentialID, CredentialVersion: claim.CredentialVersion, Protocol: claim.ModelProtocol, PrimaryModel: claim.PrimaryModel, StructuredContract: claim.ProviderStructuredContract}).Enabled() {
		return 2
	}
	return 1
}

func (w *worker) acquireTotalContainerSlots(ctx context.Context, count int) bool {
	for index := 0; index < count; index++ {
		if w.containerSlots == nil {
			continue
		}
		select {
		case w.containerSlots <- struct{}{}:
		case <-ctx.Done():
			w.releaseTotalContainerSlots(index)
			return false
		}
	}
	return true
}

func (w *worker) tryReserveTotalContainerSlots(count int) bool {
	for index := 0; index < count; index++ {
		if w.containerSlots == nil {
			continue
		}
		select {
		case w.containerSlots <- struct{}{}:
		default:
			w.releaseTotalContainerSlots(index)
			return false
		}
	}
	return true
}

func (w *worker) releaseTotalContainerSlots(count int) {
	if w == nil || w.containerSlots == nil {
		return
	}
	for index := 0; index < count; index++ {
		<-w.containerSlots
	}
}

func (w *worker) renewReviewLeaseLoop(ctx context.Context, runID, ownerEpoch int64, lease string, lost chan<- error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.control.RenewReviewLease(context.Background(), runID, ownerEpoch, lease); err != nil {
				select {
				case lost <- err:
				default:
				}
				return
			}
		}
	}
}

func reviewFailureReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "REVIEW_RUNTIME_TIMEOUT"
	}
	if errors.Is(err, reviewruntime.ErrResultIdentityInvalid) {
		return "REVIEW_RESULT_IDENTITY_INVALID"
	}
	if errors.Is(err, reviewruntime.ErrReportMissing) {
		return "REVIEW_REPORT_MISSING"
	}
	const providerRejected = "review provider relay rejected: "
	if err != nil {
		message := err.Error()
		if index := strings.Index(message, providerRejected); index >= 0 {
			code := strings.TrimSpace(message[index+len(providerRejected):])
			if safeReviewFailureReason(code) {
				return code
			}
		}
	}
	return "REVIEW_RUNTIME_FAILED"
}

func safeReviewFailureReason(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character == '_' || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9')) {
			return false
		}
	}
	return strings.HasPrefix(value, "PROVIDER_") || strings.HasPrefix(value, "REVIEW_") || strings.HasPrefix(value, "INVOCATION_")
}

// retryInfrastructureLoss is only used after the Worker has confirmed that no
// Harness is running and the Docker adapter has removed any partially-created
// Runtime. The Server owns the bounded three-Attempt budget and keeps the
// frozen TaskExecutionSnapshot when it schedules the replacement generation.
func (w *worker) retryInfrastructureLoss(_ context.Context, item taskdomain.WorkItem, attempt taskdomain.Attempt, reason string, cause error) error {
	evidence := sha256.Sum256([]byte(cause.Error()))
	_, err := w.control.ReportRuntimeLoss(context.Background(), registrydomain.RecoverableRuntime{
		TaskID: item.TaskID, AttemptID: attempt.ID, ExecutionNodeID: attempt.ExecutionNodeID,
		AssignmentID: attempt.AssignmentID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch,
		HostBootID: attempt.HostBootID, ExpectedContainerName: attempt.ContainerName,
		HarnessImageDigest: item.HarnessImageDigest, HookConfigurationDigest: item.ExecutionSnapshot.HookConfigurationDigest,
	}, reason, hex.EncodeToString(evidence[:]))
	if err != nil {
		return fmt.Errorf("%s; persist retryable infrastructure loss: %w", cause, err)
	}
	return &terminalFailure{cause: cause}
}

func (w *worker) finishAttempt(ctx context.Context, item taskdomain.WorkItem, attempt taskdomain.Attempt, workspace attemptruntime.Workspace, runResult attemptruntime.RunResult, cancelling func() bool) error {
	if runResult.InteractionSuspended {
		return nil
	}
	if runResult.ExitCode != 0 {
		return w.fail(ctx, attempt, harnessExitCode(runResult.ExitCode), fmt.Errorf("Harness exited with code %d", runResult.ExitCode))
	}
	// WAIT_USER, MANUAL_REVIEW and REJECT are already atomically persisted by
	// the Result Processor module before Supervisor exits.  They deliberately
	// do not create CompletionProposal/Finalization work and must not be
	// overwritten by the legacy Worker validation path.
	if runResult.TerminalDirective != "" {
		switch runResult.TerminalDirective {
		case processorfacade.DirectiveWaitForUser, processorfacade.DirectiveStopManualReview:
			if err := w.freezeContinuationCheckpoint(ctx, item, attempt, workspace, runResult.TerminalDecision, runResult.EvaluationCycleID, runResult.SessionDigest); err != nil {
				return w.fail(ctx, attempt, "CONTINUATION_CHECKPOINT_FAILED", err)
			}
			if runResult.TerminalDirective == processorfacade.DirectiveWaitForUser {
				if _, err := w.control.ApplyCompletionRevision(ctx, attempt, runResult.EvaluationCycleID); err != nil {
					return err
				}
			}
		case processorfacade.DirectiveStopRejected:
			// REJECT intentionally leaves no resumable checkpoint or trusted
			// result. The Server has already written the stable failure state.
		default:
			return w.fail(ctx, attempt, "RESULT_PROCESSOR_DIRECTIVE_INVALID", errors.New("terminal evaluation directive is invalid"))
		}
		return nil
	}
	if !json.Valid(runResult.CompletionRaw) {
		return w.fail(ctx, attempt, "COMPLETION_INVALID", errors.New("completion proposal is invalid"))
	}
	if err := w.control.Completion(ctx, attempt, runResult.CompletionRaw); err != nil {
		return w.fail(ctx, attempt, "COMPLETION_PERSIST_FAILED", err)
	}
	if cancelling() {
		return w.cancel(ctx, attempt)
	}
	if err := w.freezeContinuationCheckpoint(ctx, item, attempt, workspace, "PASS", runResult.EvaluationCycleID, runResult.SessionDigest); err != nil {
		return w.fail(ctx, attempt, "CONTINUATION_CHECKPOINT_FAILED", err)
	}
	// A pending additive check gets a unique successor only at this genuine,
	// stopped and independently frozen checkpoint. It precedes Finalizer/outbox.
	revision, err := w.control.ApplyCompletionRevision(ctx, attempt, runResult.EvaluationCycleID)
	if err != nil {
		return err
	}
	if revision.Applied {
		return nil
	}
	return w.control.Outcome(ctx, attempt, control.Outcome{Outcome: "EXECUTION_COMPLETED", Summary: "Agent execution passed the immutable Result Processor; trusted result coordination is pending"})
}

// freezeContinuationCheckpoint preserves the only workspace and private
// session evidence before any terminal result/coordinator cleanup can remove
// it.  Object keys are deterministic from the TaskTurn/Attempt/Cycle
// identity, and each object is read back byte-for-byte before the Server sees
// the metadata.
func (w *worker) freezeContinuationCheckpoint(ctx context.Context, item taskdomain.WorkItem, attempt taskdomain.Attempt, workspace attemptruntime.Workspace, decision string, cycleID int64, sessionDigest string, resultIdentity ...string) error {
	if item.TaskTurnID == 0 || cycleID == 0 || (decision != "PASS" && decision != "WAIT_USER" && decision != "MANUAL_REVIEW") {
		return errors.New("CONTINUATION_CHECKPOINT_IDENTITY_INVALID")
	}
	input := processorcheckpoint.Input{
		TaskID: item.TaskID, TaskTurnID: item.TaskTurnID, AttemptID: attempt.ID, Generation: attempt.Generation,
		EvaluationCycleID: cycleID, Decision: decision, SessionDigest: sessionDigest,
		RepoRoot: workspace.Repo, StateRoot: workspace.State,
		BaseCommit: item.ResolvedBaseCommit, ExecutionSnapshotDigest: item.TaskExecutionSnapshotDigest,
		HarnessImageDigest: item.ExecutionSnapshot.HarnessImageDigest, HookConfigurationDigest: item.ExecutionSnapshot.HookConfigurationDigest,
		ResultProcessorDigest: item.ExecutionSnapshot.ResultProcessorDigest, ModelCredentialIdentity: fmt.Sprintf("%d:%d:%s", item.ExecutionSnapshot.ModelCredentialID, item.ExecutionSnapshot.CredentialVersion, item.ExecutionSnapshot.ModelProtocol),
		IsolationProfile: item.ExecutionSnapshot.IsolationProfile,
	}
	contract, err := attemptruntime.ReadAuthoritativeEvaluation(w.cfg.Worker.WorkspaceRoot, item.TaskID, attempt, cycleID, decision)
	if err != nil {
		return err
	}
	input.Outputs = contract.Outputs
	input.ContinuationIncludes = contract.Continuation.Includes
	trustedIdentity, err := artifactapp.ComputeCheckpointIdentityForPlan(ctx, workspace.Repo, item.ResolvedBaseCommit, decision, contract.Outputs, contract.Continuation.Includes)
	if err != nil {
		return fmt.Errorf("RESULT_IDENTITY_RECOMPUTE_FAILED: %w", err)
	}
	input.ResultCommit, input.ResultTree = trustedIdentity.Commit, trustedIdentity.Tree
	if len(resultIdentity) == 2 {
		if resultIdentity[0] != input.ResultCommit || resultIdentity[1] != input.ResultTree {
			return errors.New("CONTINUATION_RESULT_IDENTITY_MISMATCH")
		}
	}
	spoolRoot := filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "continuations", strconv.FormatInt(attempt.ID, 10), strconv.FormatInt(cycleID, 10))
	frozen, err := processorcheckpoint.FreezeToSpool(ctx, input, spoolRoot)
	if err != nil {
		return err
	}
	keyMaterial, err := w.control.ContinuationCheckpointKey(ctx, attempt, frozen.Identity)
	if err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(keyMaterial.PlainKeyBase64)
	if err != nil || len(key) != 32 || keyMaterial.Algorithm != "AES-256-GCM-CHUNKED" || keyMaterial.AADIdentity != frozen.Identity {
		return errors.New("CONTINUATION_ENCRYPTION_MATERIAL_INVALID")
	}
	encryptedBundle, err := encryptCheckpointBlob(frozen.RepositoryBundle, filepath.Join(spoolRoot, "repository-bundle.enc"), key, checkpointAAD(frozen.Identity, "repository-bundle"))
	if err != nil {
		return err
	}
	encryptedOverlay, err := encryptCheckpointBlob(frozen.WorkspaceOverlay, filepath.Join(spoolRoot, "workspace-overlay.enc"), key, checkpointAAD(frozen.Identity, "workspace-overlay"))
	if err != nil {
		return err
	}
	encryptedSession, err := encryptCheckpointBlob(frozen.Session, filepath.Join(spoolRoot, "session.enc"), key, checkpointAAD(frozen.Identity, "session"))
	if err != nil {
		return err
	}
	encryptedManifest, err := encryptCheckpointBlob(frozen.Manifest, filepath.Join(spoolRoot, "manifest.enc"), key, checkpointAAD(frozen.Identity, "manifest"))
	if err != nil {
		return err
	}
	bundleKey, overlayKey, sessionKey, manifestKey := processorcheckpoint.ObjectKeysV2(input, frozen.Identity)
	if err = w.putCheckpointFile(ctx, bundleKey, "application/octet-stream", encryptedBundle); err != nil {
		return fmt.Errorf("repository bundle: %w", err)
	}
	if err = w.putCheckpointFile(ctx, overlayKey, "application/octet-stream", encryptedOverlay); err != nil {
		return fmt.Errorf("workspace overlay: %w", err)
	}
	if err = w.putCheckpointFile(ctx, sessionKey, "application/octet-stream", encryptedSession); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	if err = w.putCheckpointFile(ctx, manifestKey, "application/octet-stream", encryptedManifest); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	envelope, err := json.Marshal(keyMaterial.Envelope)
	if err != nil {
		return err
	}
	_, err = w.control.ContinuationCheckpoint(ctx, attempt, processorfacade.ContinuationCheckpointCandidate{
		FormatVersion:      frozen.FormatVersion,
		CheckpointIdentity: frozen.Identity, EvaluationCycleID: cycleID, Decision: decision,
		RepositoryBundleObjectKey: bundleKey, RepositoryBundleContentSHA256: frozen.RepositoryBundle.SHA256, RepositoryBundleObjectSHA256: encryptedBundle.SHA256, RepositoryBundleSizeBytes: encryptedBundle.SizeBytes,
		WorkspaceOverlayObjectKey: overlayKey, WorkspaceOverlayContentSHA256: frozen.WorkspaceOverlay.SHA256, WorkspaceOverlayObjectSHA256: encryptedOverlay.SHA256, WorkspaceOverlaySizeBytes: encryptedOverlay.SizeBytes,
		SelectionDigest: frozen.Selection.SelectionDigest, GitignoreDigest: frozen.Selection.GitignoreDigest, SelectionPolicyDigest: frozen.Selection.PolicyDigest,
		SessionObjectKey: sessionKey, SessionSHA256: frozen.Session.SHA256, SessionContentSHA256: frozen.Session.SHA256, SessionObjectSHA256: encryptedSession.SHA256, SessionSizeBytes: encryptedSession.SizeBytes,
		ManifestObjectKey: manifestKey, ManifestSHA256: frozen.Manifest.SHA256, ManifestContentSHA256: frozen.Manifest.SHA256, ManifestObjectSHA256: encryptedManifest.SHA256, ManifestSizeBytes: encryptedManifest.SizeBytes,
		TaskExecutionSnapshotDigest: input.ExecutionSnapshotDigest, BaseCommit: input.BaseCommit, ResultCommit: input.ResultCommit, ResultTree: input.ResultTree,
		SessionLineageDigest: sessionDigest,
		EncryptionAlgorithm:  keyMaterial.Algorithm, EncryptionKeyID: keyMaterial.Envelope.KeyID, WrappedDataKey: string(envelope),
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
	})
	return err
}

func checkpointAAD(identity, blobType string) []byte {
	return []byte("seven.continuation\x00" + identity + "\x00" + blobType)
}

func encryptCheckpointBlob(source processorcheckpoint.SpoolBlob, destination string, key, aad []byte) (processorcheckpoint.SpoolBlob, error) {
	input, err := os.Open(source.Path)
	if err != nil {
		return processorcheckpoint.SpoolBlob{}, err
	}
	defer input.Close()
	temporary := destination + ".tmp"
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return processorcheckpoint.SpoolBlob{}, err
	}
	hash := sha256.New()
	writer := io.MultiWriter(output, hash)
	err = processorcheckpoint.EncryptStream(input, writer, key, aad)
	if err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(temporary)
		if err != nil {
			return processorcheckpoint.SpoolBlob{}, err
		}
		return processorcheckpoint.SpoolBlob{}, closeErr
	}
	if err = os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return processorcheckpoint.SpoolBlob{}, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return processorcheckpoint.SpoolBlob{}, err
	}
	return processorcheckpoint.SpoolBlob{Path: destination, SHA256: hex.EncodeToString(hash.Sum(nil)), SizeBytes: info.Size(), FileCount: source.FileCount}, nil
}

func (w *worker) restoreContinuation(ctx context.Context, item *taskdomain.WorkItem, attempt taskdomain.Attempt) (attemptruntime.Workspace, string, error) {
	restore := item.ContinuationRestore
	if restore == nil || restore.RestoreRunID == 0 || restore.CheckpointID == 0 {
		return attemptruntime.Workspace{}, "", errors.New("CONTINUATION_RESTORE_CONFLICT")
	}
	if restore.EncryptionAlgorithm != "AES-256-GCM-CHUNKED" || len(restore.CheckpointIdentity) != 64 {
		return attemptruntime.Workspace{}, "", errors.New("CONTINUATION_ENCRYPTION_MATERIAL_UNAVAILABLE")
	}
	keyMaterial, err := w.control.ContinuationRestoreKey(ctx, attempt, restore.RestoreRunID)
	if err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	key, err := base64.StdEncoding.DecodeString(keyMaterial.PlainKeyBase64)
	if err != nil || len(key) != 32 || keyMaterial.Algorithm != restore.EncryptionAlgorithm || keyMaterial.AADIdentity != restore.CheckpointIdentity {
		return attemptruntime.Workspace{}, "", errors.New("CONTINUATION_ENCRYPTION_MATERIAL_INVALID")
	}
	defer clear(key)
	workspace, workspaceErr := w.runtime.PrepareWorkspace(*item, attempt)
	if workspaceErr != nil {
		return attemptruntime.Workspace{}, "", workspaceErr
	}
	if restore.FormatVersion != 2 {
		return attemptruntime.Workspace{}, "", errors.New("CONTINUATION_CHECKPOINT_FORMAT_UNSUPPORTED")
	}
	if keyMaterial.InteractionRestoreInput != nil {
		return w.restoreColdInteraction(ctx, item, attempt, workspace, keyMaterial.InteractionRestoreInput, key)
	}
	download := func(key, expected string, size int64) (*os.File, error) {
		if key == "" || len(expected) != 64 || size < 1 {
			return nil, errors.New("CONTINUATION_CHECKPOINT_MISSING")
		}
		object, err := w.artifacts.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		defer object.Close()
		file, err := os.CreateTemp(workspace.AttemptRoot, ".restore-*")
		if err != nil {
			return nil, err
		}
		_ = file.Chmod(0o600)
		hash := sha256.New()
		read, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(object, size+1))
		if copyErr != nil || read != size || hex.EncodeToString(hash.Sum(nil)) != expected {
			file.Close()
			os.Remove(file.Name())
			return nil, errors.New("CONTINUATION_CHECKPOINT_TAMPERED")
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			os.Remove(file.Name())
			return nil, err
		}
		return file, nil
	}
	if err = w.restoreBundleOverlay(ctx, bundleOverlayRestore{
		CheckpointIdentity: restore.CheckpointIdentity,
		BundleObjectKey:    restore.RepositoryBundleObjectKey, BundleObjectSHA256: restore.RepositoryBundleObjectSHA256, BundleContentSHA256: restore.RepositoryBundleContentSHA256, BundleSizeBytes: restore.RepositoryBundleSizeBytes,
		OverlayObjectKey: restore.WorkspaceOverlayObjectKey, OverlayObjectSHA256: restore.WorkspaceOverlayObjectSHA256, OverlayContentSHA256: restore.WorkspaceOverlayContentSHA256, OverlaySizeBytes: restore.WorkspaceOverlaySizeBytes,
		ExpectedCommit: restore.ExpectedResultCommit, ExpectedTree: restore.ExpectedResultTree,
	}, key, workspace.AttemptRoot, workspace.Repo); err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	sessionObject, err := download(restore.SessionObjectKey, restore.SessionObjectSHA256, restore.SessionSizeBytes)
	if err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	defer func() { name := sessionObject.Name(); sessionObject.Close(); os.Remove(name) }()
	sessionPlain, err := decryptCheckpointObject(sessionObject, workspace.AttemptRoot, key, checkpointAAD(restore.CheckpointIdentity, "session"), restore.SessionContentSHA256, processorcheckpoint.MaxSessionArchiveBytes)
	if err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	defer func() { name := sessionPlain.Name(); sessionPlain.Close(); os.Remove(name) }()
	// PrepareWorkspace creates the Attempt-private state directory so the rest
	// of the runtime has stable paths. A continuation restore, however, must
	// replace that empty/pre-launch directory with the frozen session archive;
	// Extract deliberately rejects an existing destination to prevent merges.
	// Removing only this fenced Attempt directory also makes same-assignment
	// redelivery deterministic without inheriting partially restored state.
	if err = restoreSessionArchive(ctx, sessionPlain, workspace.State); err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	if err = prepareRestoredSessionState(ctx, workspace.State, item.TaskID, attempt.ID, restore.SessionLineageDigest); err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	manifestObject, err := download(restore.ManifestObjectKey, restore.ManifestObjectSHA256, restore.ManifestSizeBytes)
	if err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	defer func() { name := manifestObject.Name(); manifestObject.Close(); os.Remove(name) }()
	manifestPlain, err := decryptCheckpointObject(manifestObject, workspace.AttemptRoot, key, checkpointAAD(restore.CheckpointIdentity, "manifest"), restore.ManifestContentSHA256, 1<<20)
	if err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	defer func() { name := manifestPlain.Name(); manifestPlain.Close(); os.Remove(name) }()
	manifestBody, err := io.ReadAll(io.LimitReader(manifestPlain, (1<<20)+1))
	if err != nil || len(manifestBody) > 1<<20 || !validateRestoredCheckpointManifest(manifestBody, restore) {
		return attemptruntime.Workspace{}, "", errors.New("CONTINUATION_IDENTITY_INCOMPATIBLE")
	}
	if err = prepareRestoredBindPermissions(workspace.Repo, workspace.State); err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	baseOutput, err := exec.CommandContext(ctx, "git", "-C", workspace.Repo, "rev-parse", "HEAD").Output()
	if err != nil {
		return attemptruntime.Workspace{}, "", errors.New("CONTINUATION_RESULT_IDENTITY_MISMATCH")
	}
	base := strings.TrimSpace(string(baseOutput))
	if base != restore.ExpectedResultCommit {
		return attemptruntime.Workspace{}, "", errors.New("CONTINUATION_RESULT_IDENTITY_MISMATCH")
	}
	resultDigest := sha256Hex([]byte(strings.Join([]string{strconv.FormatInt(restore.CheckpointID, 10), restore.TaskExecutionSnapshotDigest, base, restore.ExpectedResultCommit, restore.ExpectedResultTree, restore.SessionLineageDigest}, "\x00")))
	// base is the checked-out ResultCommit. The restore acknowledgement also
	// carries the immutable source base separately: conflating the two makes a
	// valid cross-Worker restore fail whenever Turn #1 changed the repository.
	if err = w.control.CompleteContinuationRestore(ctx, attempt, restore.RestoreRunID, resultDigest, restore.ExpectedBaseCommit, restore.ExpectedResultCommit, restore.ExpectedResultTree); err != nil {
		return attemptruntime.Workspace{}, "", err
	}
	return workspace, base, nil
}

func decryptCheckpointObject(source *os.File, directory string, key, aad []byte, expectedContentSHA string, maxBytes int64) (*os.File, error) {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	output, err := os.CreateTemp(directory, ".plain-restore-*")
	if err != nil {
		return nil, err
	}
	_ = output.Chmod(0o600)
	hash := sha256.New()
	if err = processorcheckpoint.DecryptStream(source, io.MultiWriter(output, hash), key, aad); err != nil {
		output.Close()
		os.Remove(output.Name())
		return nil, err
	}
	info, statErr := output.Stat()
	if statErr != nil || info.Size() > maxBytes || hex.EncodeToString(hash.Sum(nil)) != expectedContentSHA {
		output.Close()
		os.Remove(output.Name())
		return nil, errors.New("CONTINUATION_CHECKPOINT_TAMPERED")
	}
	if _, err = output.Seek(0, io.SeekStart); err != nil {
		output.Close()
		os.Remove(output.Name())
		return nil, err
	}
	return output, nil
}

func validateRestoredCheckpointManifest(body []byte, restore *taskdomain.ContinuationRestore) bool {
	var manifest struct {
		SchemaVersion               int    `json:"schemaVersion"`
		CheckpointIdentity          string `json:"checkpointIdentity"`
		TaskExecutionSnapshotDigest string `json:"taskExecutionSnapshotDigest"`
		SessionDigest               string `json:"sessionDigest"`
		BaseCommit                  string `json:"baseCommit"`
		ResultCommit                string `json:"resultCommit"`
		ResultTree                  string `json:"resultTree"`
		RepositoryBundle            struct {
			SHA256 string `json:"sha256"`
		} `json:"repositoryBundle"`
		WorkspaceOverlay struct {
			SHA256          string `json:"sha256"`
			SelectionDigest string `json:"selectionDigest"`
		} `json:"workspaceOverlay"`
		Session struct {
			SHA256 string `json:"sha256"`
		} `json:"session"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if decoder.Decode(&manifest) != nil {
		return false
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return false
	}
	return manifest.SchemaVersion == 2 && manifest.CheckpointIdentity == restore.CheckpointIdentity && manifest.TaskExecutionSnapshotDigest == restore.TaskExecutionSnapshotDigest && manifest.SessionDigest == restore.SessionLineageDigest && manifest.BaseCommit == restore.ExpectedBaseCommit && manifest.ResultCommit == restore.ExpectedResultCommit && manifest.ResultTree == restore.ExpectedResultTree && manifest.RepositoryBundle.SHA256 == restore.RepositoryBundleContentSHA256 && manifest.WorkspaceOverlay.SHA256 == restore.WorkspaceOverlayContentSHA256 && manifest.WorkspaceOverlay.SelectionDigest == restore.SelectionDigest && manifest.Session.SHA256 == restore.SessionContentSHA256
}

func makeAttemptBindAccessible(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o777)
		}
		return os.Chmod(path, 0o666)
	})
}

// Native session data has the same private contract after restore as on a
// fresh Attempt. Broad repository bind permissions must not reach State.
func prepareRestoredBindPermissions(repo, state string) error {
	if err := makeAttemptBindAccessible(repo); err != nil {
		return err
	}
	return filepath.WalkDir(state, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		mode := os.FileMode(0o600)
		if entry.IsDir() {
			mode = 0o700
		}
		return os.Chmod(path, mode)
	})
}

func (w *worker) putCheckpointFile(ctx context.Context, key, mediaType string, blob processorcheckpoint.SpoolBlob) error {
	file, err := os.Open(blob.Path)
	if err != nil {
		return err
	}
	if err = w.artifacts.Put(ctx, key, mediaType, file, blob.SizeBytes); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	object, err := w.artifacts.Get(ctx, key)
	if err != nil {
		return err
	}
	hash := sha256.New()
	read, readErr := io.Copy(hash, io.LimitReader(object, blob.SizeBytes+1))
	closeErr := object.Close()
	if readErr != nil || closeErr != nil || read != blob.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != blob.SHA256 {
		return errors.New("CONTINUATION_CHECKPOINT_READBACK_MISMATCH")
	}
	return nil
}

func (w *worker) putCheckpointObject(ctx context.Context, key, mediaType string, body []byte, expectedSHA256 string) error {
	if len(body) == 0 || sha256Hex(body) != expectedSHA256 {
		return errors.New("CONTINUATION_CHECKPOINT_DIGEST_INVALID")
	}
	if err := w.artifacts.Put(ctx, key, mediaType, bytes.NewReader(body), int64(len(body))); err != nil {
		return err
	}
	object, err := w.artifacts.Get(ctx, key)
	if err != nil {
		return err
	}
	readback, readErr := io.ReadAll(io.LimitReader(object, int64(len(body))+1))
	closeErr := object.Close()
	if readErr != nil || closeErr != nil || len(readback) != len(body) || sha256Hex(readback) != expectedSHA256 {
		return errors.New("CONTINUATION_CHECKPOINT_READBACK_MISMATCH")
	}
	return nil
}

func (w *worker) reconcileRuntimes(ctx context.Context, reconciliation registrydomain.Reconciliation) error {
	for _, candidate := range reconciliation.Runtimes {
		if candidate.HostBootID != w.hostBoot || candidate.CancellationRequested {
			if candidate.ProviderBacked {
				if err := w.runtime.TerminateProviderProxyForLoss(ctx, candidate.TaskID, candidate.AttemptID, candidate.ProviderProxyContainerName, candidate.ProviderProxyContainerID, candidate.ProviderProxyImageDigest, candidate.ProviderProxyIdentityDigest, candidate.Generation, candidate.OwnerEpoch); err != nil {
					return fmt.Errorf("fence Provider Proxy before host-loss replacement: %w", err)
				}
			}
			if err := w.runtime.TerminateRuntimeForLoss(ctx, candidate.TaskID, candidate.AttemptID, candidate.ExpectedContainerName, candidate.ContainerID, candidate.Generation, candidate.OwnerEpoch); err != nil {
				return fmt.Errorf("fence old Runtime before host-loss replacement: %w", err)
			}
			reason := dispatchdomain.ReasonHostLost
			if candidate.CancellationRequested {
				reason = dispatchdomain.ReasonRuntimeMissing
			}
			if _, err := w.control.ReportRuntimeLoss(ctx, candidate, reason, ""); err != nil {
				return err
			}
			continue
		}
		probe, err := w.runtime.InspectRuntimeForRecovery(ctx, candidate.TaskID, candidate.AttemptID, candidate.AssignmentID, candidate.ExecutionNodeID,
			candidate.HostBootID, candidate.ExpectedContainerName, candidate.ContainerID, candidate.HarnessImageDigest, candidate.HookConfigurationDigest,
			candidate.ExecutionIdentityDigest, candidate.Generation, candidate.OwnerEpoch)
		if err == nil && candidate.ProviderBacked {
			var proxyEvidence dispatchdomain.ProviderProxyEvidence
			proxyEvidence, err = w.runtime.InspectProviderProxyForRecovery(ctx, candidate.TaskID, candidate.AttemptID, candidate.HostBootID,
				candidate.ProviderProxyContainerName, candidate.ProviderProxyContainerID, candidate.ProviderProxyImageDigest,
				candidate.ProviderProxyIdentityDigest, candidate.Generation, candidate.OwnerEpoch)
			if err == nil {
				probe.Evidence.ProviderProxy = &proxyEvidence
			}
		}
		if err != nil {
			reason := dispatchdomain.ReasonRuntimeMissing
			if strings.Contains(err.Error(), "IDENTITY_DRIFT") {
				reason = "RUNTIME_IDENTITY_DRIFT"
			}
			w.logger.Warn("Runtime recovery evidence rejected",
				zap.Int64("task_id", candidate.TaskID), zap.Int64("attempt_id", candidate.AttemptID),
				zap.String("reason_code", reason), zap.Error(err))
			if candidate.ProviderBacked {
				if terminateErr := w.runtime.TerminateProviderProxyForLoss(ctx, candidate.TaskID, candidate.AttemptID, candidate.ProviderProxyContainerName, candidate.ProviderProxyContainerID, candidate.ProviderProxyImageDigest, candidate.ProviderProxyIdentityDigest, candidate.Generation, candidate.OwnerEpoch); terminateErr != nil {
					return fmt.Errorf("fence unrecoverable Provider Proxy before replacement: %w", terminateErr)
				}
			}
			if terminateErr := w.runtime.TerminateRuntimeForLoss(ctx, candidate.TaskID, candidate.AttemptID, candidate.ExpectedContainerName, candidate.ContainerID, candidate.Generation, candidate.OwnerEpoch); terminateErr != nil {
				return fmt.Errorf("fence unrecoverable Runtime before replacement: %w", terminateErr)
			}
			digest := sha256.Sum256([]byte(err.Error()))
			if _, reportErr := w.control.ReportRuntimeLoss(ctx, candidate, reason, hex.EncodeToString(digest[:])); reportErr != nil {
				return fmt.Errorf("%v; report runtime loss: %w", err, reportErr)
			}
			continue
		}
		probe.Evidence.InteractionReplayRequested, err = w.interactions.needsInteractionReplay(ctx, candidate, probe.Evidence)
		if err != nil {
			return err
		}
		permit, err := w.control.AcquireReattachPermit(ctx, candidate, probe.Evidence)
		if err != nil {
			return err
		}
		ack, err := w.runtime.AdoptOwner(ctx, probe.Evidence.ContainerID, candidate.TaskID, candidate.AttemptID, candidate.Generation, permit)
		if err != nil {
			return err
		}
		if candidate.ProviderBacked {
			proxyAck, proxyErr := w.runtime.AdoptProviderProxyOwner(ctx, candidate.TaskID, candidate.AttemptID, w.instance, candidate.Generation, permit)
			if proxyErr != nil {
				return proxyErr
			}
			ack.ProviderProxy = &proxyAck
		}
		result, err := w.control.CommitReattach(ctx, candidate, permit, ack)
		if err != nil {
			return err
		}
		w.attempts.Add(1)
		go func(result dispatchdomain.ReattachResult, containerID string, candidate registrydomain.RecoverableRuntime) {
			defer w.attempts.Done()
			w.resumeReattached(context.Background(), result, containerID, candidate)
		}(result, probe.Evidence.ContainerID, candidate)
	}
	return nil
}

func (w *worker) resumeReattached(ctx context.Context, result dispatchdomain.ReattachResult, containerID string, recovered registrydomain.RecoverableRuntime) {
	// A reattached Agent container did not pass through consume(), so it must
	// acquire both the Agent-specific and shared Review/Agent container limits
	// before it is counted as live again.
	if w.agentSlots != nil {
		select {
		case w.agentSlots <- struct{}{}:
			defer func() { <-w.agentSlots }()
		case <-ctx.Done():
			return
		}
	}
	if w.containerSlots != nil {
		select {
		case w.containerSlots <- struct{}{}:
			defer func() { <-w.containerSlots }()
		case <-ctx.Done():
			return
		}
	}
	w.active.Add(1)
	defer w.active.Add(-1)
	item, attempt := result.Contract.WorkItem, result.Contract.Attempt
	attempt.LeaseToken = result.Contract.AttemptLeaseToken
	attempt.LeaseExpiresAt = result.Contract.AttemptLeaseExpiresAt
	attempt.ExecutionNodeID = result.Contract.ExecutionNodeID
	attempt.AssignmentID = result.Contract.AssignmentID
	attempt.WorkerID, attempt.WorkerInstance, attempt.HostBootID = w.cfg.Worker.ID, w.instance, w.hostBoot
	attempt.OwnerEpoch = result.Contract.OwnerEpoch
	defer func() {
		if err := w.runtime.TerminateRuntimeForLoss(context.Background(), item.TaskID, attempt.ID, recovered.ExpectedContainerName, containerID, attempt.Generation, attempt.OwnerEpoch); err != nil {
			w.logger.Warn("reattached runtime cleanup remains pending", zap.Int64("attempt_id", attempt.ID))
		}
	}()
	leaseCtx, stopLease := context.WithCancel(ctx)
	defer stopLease()
	var activeBudget atomic.Pointer[context.Context]
	go w.renewLeaseLoop(leaseCtx, item.TaskID, attempt, func() string { return containerID }, func() *int64 { return currentBudgetMillis(&activeBudget) }, recovered.ProviderBacked)
	if recovered.ProviderBacked {
		defer func() {
			if err := w.runtime.TerminateProviderProxyForLoss(context.Background(), item.TaskID, attempt.ID, recovered.ProviderProxyContainerName, recovered.ProviderProxyContainerID, recovered.ProviderProxyImageDigest, recovered.ProviderProxyIdentityDigest, attempt.Generation, attempt.OwnerEpoch); err != nil {
				w.logger.Warn("clean reattached Provider Proxy", zap.Int64("attempt_id", attempt.ID), zap.Error(err))
			}
		}()
	}
	journalFacts, journalErr := w.runtime.ReadRuntimeJournalFacts(item, attempt)
	if journalErr != nil {
		if err := w.fail(context.Background(), attempt, "RUNTIME_JOURNAL_REPLAY_FAILED", journalErr); err != nil {
			w.logger.Error("fail Runtime after journal replay error", zap.Int64("attempt_id", attempt.ID), zap.Error(err))
		}
		return
	}
	if err := w.control.ReplayJournalFacts(context.Background(), attempt, journalFacts); err != nil {
		if failErr := w.fail(context.Background(), attempt, "RUNTIME_JOURNAL_REPLAY_FAILED", err); failErr != nil {
			w.logger.Error("fail Runtime after journal persistence error", zap.Int64("attempt_id", attempt.ID), zap.Error(failErr))
		}
		return
	}
	var eventSequence atomic.Int64
	eventSequence.Store(time.Now().UnixNano())
	nextEvent := func(eventType string, payload any) error {
		return w.control.AddEvent(context.Background(), attempt, eventType, payload, eventSequence.Add(1))
	}
	cancelling := func() bool {
		value, readErr := w.control.Control(context.Background(), attempt)
		return readErr == nil && value
	}
	sink := func(eventType string, payload any) {
		if err := nextEvent(eventType, payload); err != nil {
			w.logger.Warn("persist reattached runtime event", zap.Int64("attempt_id", attempt.ID), zap.Error(err))
		}
	}
	gate := attemptruntime.ReattachGate{
		DrainInteractions: func(drainCtx context.Context) error {
			if w.interactions == nil {
				return errInteractionTransport
			}
			return w.interactions.Drain(drainCtx, attempt)
		},
		SuspendInteraction: func(suspendCtx context.Context, channel *attemptruntime.ExecutionChannel) error {
			workspace, err := w.runtime.ExistingWorkspace(item, attempt)
			if err != nil {
				return err
			}
			return w.suspendInteractionRuntime(suspendCtx, item, attempt, workspace, channel)
		},
		InteractionSession: func(sessionCtx context.Context, channel *attemptruntime.ExecutionChannel) (func(), error) {
			if w.interactions == nil {
				return nil, errInteractionTransport
			}
			var closePump func()
			var err error
			if result.InteractionReplayBinding != nil {
				var proof committedInteractionReattach
				proof, err = committedInteractionReplay(result, recovered, containerID, attempt, w.interactionBudgets)
				if err == nil {
					closePump, err = w.interactions.RegisterReattached(sessionCtx, proof, channel)
				}
			} else {
				closePump, err = w.interactions.Register(sessionCtx, item.TaskID, attempt, channel)
			}
			if err != nil {
				return nil, err
			}
			workspace, err := w.runtime.ExistingWorkspace(item, attempt)
			if err != nil {
				if closePump != nil {
					closePump()
				}
				return nil, err
			}
			stopOperations := w.startExecutionOperationLoop(sessionCtx, item, attempt, workspace, channel)
			stopBudgetObservations := w.observeAcceptedBudgetQuestions(sessionCtx, attempt, channel)
			return func() {
				stopOperations()
				stopBudgetObservations()
				if closePump != nil {
					closePump()
				}
			}, nil
		},
		CheckEvaluation: func(checkCtx context.Context, cycleID int64) error {
			workspace, err := w.runtime.ExistingWorkspace(item, attempt)
			if err != nil {
				return err
			}
			return w.runCompletionCheck(checkCtx, item, attempt, workspace, "CURRENT", cycleID)
		},
		AcquireRunPermit: func() (dispatchdomain.RunPermit, error) { return w.control.RunPermit(context.Background(), attempt) },
		HarnessStarted: func(started dispatchdomain.HarnessStarted) error {
			return w.control.HarnessStarted(context.Background(), attempt, started)
		},
		EvaluationReady: func(cycleID int64, cycleNumber int) error {
			return nextEvent("task.validating", map[string]any{
				"cycleId":     strconv.FormatInt(cycleID, 10),
				"cycleNumber": cycleNumber,
			})
		},
		MaterializeEvaluation: func(cycleID int64, cycleNumber int) (processorfacade.Materialization, error) {
			return w.control.ProcessorMaterialization(context.Background(), attempt, cycleID, cycleNumber)
		},
		ReadEvaluation: func(ctx context.Context, pending processorfacade.EvaluationReplayIdentity) (processorfacade.EvaluationReplay, error) {
			return w.control.ReadEvaluation(ctx, attempt, pending)
		},
		PersistEvaluation: func(candidate processorfacade.EvaluationCandidate) (processorfacade.EvaluationDirective, error) {
			return w.control.ResultEvaluation(context.Background(), attempt, candidate)
		},
	}
	budgetParent := ctx
	limit := time.Duration(item.HarnessTimeoutSeconds) * time.Second
	if w.interactionBudgets != nil {
		budgetParent = attemptruntime.WithBudgetRestoreProvider(ctx, w.interactionBudgets.reattachProvider(attemptruntime.BudgetIdentity{TaskID: recovered.TaskID, AttemptID: recovered.AttemptID, Generation: recovered.Generation, OwnerEpoch: recovered.OwnerEpoch}, attemptruntime.BudgetIdentity{TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch}))
	}
	budgetCtx, cancelBudget, budgetErr := withExecutionBudget(budgetParent, limit, attemptruntime.BudgetIdentity{TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch}, true, item.ExecutionActiveMilliseconds)
	if budgetErr != nil {
		_ = w.fail(ctx, attempt, "BUDGET_RECOVERY_EVIDENCE_REQUIRED", budgetErr)
		return
	}
	defer cancelBudget()
	budgetContext := context.Context(budgetCtx)
	activeBudget.Store(&budgetContext)
	stopBudgetSave, budgetErr := w.interactionBudgets.persist(budgetCtx, cancelBudget)
	if budgetErr != nil {
		_ = w.fail(ctx, attempt, "BUDGET_PERSIST_FAILED", budgetErr)
		return
	}
	defer stopBudgetSave()
	runResult, err := w.runtime.Resume(budgetCtx, item, attempt, containerID, result.StartNeeded, sink, gate, cancelling)
	if cancelling() {
		err = w.cancel(context.Background(), attempt)
	} else if err != nil {
		err = w.fail(context.Background(), attempt, "REATTACHED_RUNTIME_FAILED", err)
	} else {
		workspace, workspaceErr := w.runtime.ExistingWorkspace(item, attempt)
		if workspaceErr != nil {
			err = w.fail(context.Background(), attempt, "REATTACHED_WORKSPACE_PATH_INVALID", workspaceErr)
		} else {
			err = w.finishAttempt(ctx, item, attempt, workspace, runResult, cancelling)
		}
	}
	if err != nil {
		w.logger.Error("reattached Attempt failed", zap.Int64("attempt_id", attempt.ID), zap.Error(err))
	}
}

func currentBudgetMillis(pointer *atomic.Pointer[context.Context]) *int64 {
	value := pointer.Load()
	if value == nil {
		return nil
	}
	snapshot, err := attemptruntime.BudgetSnapshot(*value)
	if err != nil {
		return nil
	}
	return &snapshot.ActiveElapsedMillis
}

func (w *worker) renewLeaseLoop(ctx context.Context, taskID int64, attempt taskdomain.Attempt, containerID func() string, activeBudget func() *int64, providerBacked ...bool) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewal, err := w.control.RenewLease(ctx, attempt, activeBudget())
			if err != nil && ctx.Err() == nil {
				w.logger.Warn("renew execution lease", zap.Int64("attempt_id", attempt.ID), zap.Error(err))
				continue
			}
			if id := containerID(); id != "" {
				if err := w.runtime.RenewAuthority(ctx, id, taskID, attempt, renewal.ExpiresAt, renewal.ReceiptDigest); err != nil && ctx.Err() == nil {
					w.logger.Warn("renew Supervisor authority", zap.Int64("attempt_id", attempt.ID), zap.Error(err))
				}
			}
			if len(providerBacked) > 0 && providerBacked[0] {
				if err := w.runtime.RenewProviderProxyAuthority(ctx, taskID, attempt, renewal.ExpiresAt); err != nil && ctx.Err() == nil {
					w.logger.Warn("renew Provider Proxy authority", zap.Int64("attempt_id", attempt.ID), zap.Error(err))
				}
			}
		}
	}
}

func providerRuntimeIdentity(item taskdomain.WorkItem) (*attemptruntime.ProviderRuntimeIdentity, error) {
	if item.ModelCredentialName == "" {
		return nil, nil
	}
	snapshot := item.ExecutionSnapshot
	if snapshot.ModelCredentialID == 0 || snapshot.CredentialVersion == 0 || (snapshot.ModelProtocol != "OPENAI" && snapshot.ModelProtocol != "ANTHROPIC") || snapshot.PrimaryModel == "" || snapshot.ProviderProxyImageDigest == "" || snapshot.ProviderProxyVersion == "" || snapshot.LocalProviderFacadeDigest == "" || snapshot.ProviderGatewayContract == "" || snapshot.ProviderAdapterIdentity == "" {
		return nil, errors.New("frozen model credential execution identity is incomplete")
	}
	if item.PrimaryModel != "" && item.PrimaryModel != snapshot.PrimaryModel {
		return nil, errors.New("Task primary model drifted from the frozen snapshot")
	}
	return &attemptruntime.ProviderRuntimeIdentity{
		ModelCredentialID: snapshot.ModelCredentialID, CredentialVersion: snapshot.CredentialVersion,
		Protocol: snapshot.ModelProtocol, PrimaryModel: snapshot.PrimaryModel,
		ExpectedProxyDigest: snapshot.ProviderProxyImageDigest, ExpectedProxyVersion: snapshot.ProviderProxyVersion,
	}, nil
}

func (w *worker) fail(ctx context.Context, attempt taskdomain.Attempt, code string, cause error) error {
	if cancelling, _ := w.control.Control(context.Background(), attempt); cancelling {
		return w.cancel(ctx, attempt)
	}
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	if err := w.control.Outcome(context.Background(), attempt, control.Outcome{Outcome: "FAILED", ReasonCode: code, Message: message}); err != nil {
		return fmt.Errorf("%s; persist terminal outcome: %w", cause, err)
	}
	return &terminalFailure{cause: cause}
}

func (w *worker) cancel(ctx context.Context, attempt taskdomain.Attempt) error {
	if err := w.control.Outcome(context.Background(), attempt, control.Outcome{Outcome: "CANCELLED", ReasonCode: "USER_CANCELLED", Message: "task cancellation requested"}); err != nil {
		return err
	}
	return &terminalFailure{cause: errors.New("task cancelled")}
}

func (w *worker) timeout(ctx context.Context, attempt taskdomain.Attempt, code string, cause error) error {
	if cancelling, _ := w.control.Control(context.Background(), attempt); cancelling {
		return w.cancel(ctx, attempt)
	}
	if err := w.control.Outcome(context.Background(), attempt, control.Outcome{Outcome: "TIMED_OUT", ReasonCode: code, Message: cause.Error()}); err != nil {
		return err
	}
	return &terminalFailure{cause: cause}
}

func (w *worker) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.sendHeartbeat(ctx); err != nil && ctx.Err() == nil {
			w.logger.Warn("send Worker heartbeat", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *worker) sendHeartbeat(ctx context.Context) error {
	snapshotCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	snapshot, err := hostinfo.Collect(snapshotCtx, w.cfg.Worker.WorkspaceRoot)
	if err != nil {
		return err
	}
	result, err := w.control.Heartbeat(snapshotCtx, control.HeartbeatInput{
		Sequence: w.heartbeat.Add(1), ConsumerState: w.currentConsumerState(),
		DockerReady: snapshot.DockerReady, ActiveAttempts: int(w.active.Load()),
		MaxActiveAttempts: w.cfg.Worker.MaxActiveContainers, Hostname: snapshot.Hostname,
		ReportedIP: snapshot.ReportedIP, PrimaryInterface: snapshot.PrimaryInterface,
		PrimaryMAC: snapshot.PrimaryMAC, Interfaces: snapshot.Interfaces,
		ResourceSnapshot: snapshot.Resources,
	})
	if err != nil {
		return err
	}
	select {
	case w.directives <- result.ConsumerDirective:
	default:
	}
	refreshGeneration := int64(result.RefreshInventoryGeneration)
	if refreshGeneration > w.refreshGeneration.Load() {
		w.refreshGeneration.Store(refreshGeneration)
		select {
		case w.inventoryWake <- struct{}{}:
		default:
		}
	}
	return nil
}

func (w *worker) currentConsumerState() string {
	if w.cleanupReconciling.Load() || (w.cleanupJournal != nil && w.cleanupJournal.Pending() > 0) {
		return "STOPPED"
	}
	if value := w.consumerState.Load(); value != nil {
		return value.(string)
	}
	return "STARTING"
}

func (w *worker) inventoryLoop(ctx context.Context) {
	timer := time.NewTimer(inventoryScanDelay(60*time.Second, time.Now().UnixNano()))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-w.inventoryWake:
		}
		images, err := inspectImages(ctx, w.cfg)
		if err != nil {
			if ctx.Err() == nil {
				w.logger.Warn("refresh Worker image inventory", zap.Error(err))
			}
		} else {
			version := w.inventoryVersion.Add(1)
			if err = w.control.PublishInventory(ctx, version, canonicalInventoryDigest(images), images); err != nil && ctx.Err() == nil {
				w.logger.Warn("publish refreshed Worker image inventory", zap.Int64("version", version), zap.Error(err))
			}
		}
		timer.Reset(inventoryScanDelay(60*time.Second, time.Now().UnixNano()))
	}
}

func inventoryScanDelay(base time.Duration, entropy int64) time.Duration {
	if base <= 0 {
		return base
	}
	// Uniformly select an integer percentage in [-10,+10]. The jitter is not a
	// security primitive; it only prevents a Worker fleet from synchronizing
	// Docker scans and inventory uploads after a common restart.
	percent := entropy%21 - 10
	if percent < -10 {
		percent += 21
	}
	return base + time.Duration(int64(base)*percent/100)
}

func inspectImages(ctx context.Context, cfg workforceconfig.Config) ([]registrydomain.Image, error) {
	candidates, err := discoverProtocolImages(ctx)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		candidates = []protocolImage{{Ref: cfg.Harness.Image, Role: "attempt"}}
	}
	allowed := map[string]bool{}
	for _, digest := range cfg.Worker.ManagedImageDigests {
		allowed[strings.TrimSpace(digest)] = true
	}
	images := make([]registrydomain.Image, 0, len(candidates)+1)
	for _, candidate := range candidates {
		if candidate.Role != "attempt" {
			continue
		}
		// Discovery must remain complete enough to report every protocol image the
		// Docker daemon exposes, but an operator-managed allowlist is an admission
		// boundary, not merely a scheduling preference.  In particular, never run
		// an arbitrary labelled image merely to obtain its dynamic probe evidence.
		// Besides preserving that boundary, this keeps a stale or malicious local
		// image from delaying the Worker before it can publish its inventory.
		if len(allowed) > 0 && !allowed[candidate.Digest] {
			evidence, _ := json.Marshal(map[string]any{
				"reasonCode":  "NOT_ALLOWED_BY_WORKER_POLICY",
				"imageDigest": candidate.Digest,
			})
			images = append(images, registrydomain.Image{
				ImageDigest: candidate.Digest, ImageRef: candidate.Ref, Role: "attempt",
				ContractVersion: "seven.worker-image/v1", AdmissionStatus: "REJECTED",
				AdmissionEvidence: evidence, PresenceStatus: "PRESENT", InventoryVersion: 1,
			})
			continue
		}
		harness := cfg.Harness
		harness.Image = candidate.Ref
		report, inspectErr := imageadmission.Inspect(ctx, harness)
		if inspectErr != nil {
			if errors.Is(inspectErr, imageadmission.ErrInspectionUnavailable) {
				return nil, inspectErr
			}
			evidence, _ := json.Marshal(map[string]any{"reasonCode": "IMAGE_ADMISSION_REJECTED", "diagnostic": inspectErr.Error()})
			images = append(images, registrydomain.Image{ImageDigest: candidate.Digest, ImageRef: candidate.Ref, Role: "attempt", ContractVersion: "seven.worker-image/v1", AdmissionStatus: "REJECTED", AdmissionEvidence: evidence, PresenceStatus: "PRESENT", InventoryVersion: 1})
			continue
		}
		evidence, _ := json.Marshal(report)
		manifest, manifestErr := admittedInteractionInventoryManifest(report)
		if manifestErr != nil {
			return nil, manifestErr
		}
		manifestSum := sha256.Sum256(manifest)
		images = append(images, admittedRegistryImage(candidate.Ref, report, manifest, evidence, hex.EncodeToString(manifestSum[:])))
	}
	if strings.TrimSpace(cfg.ProviderProxy.Image) != "" {
		if proxy, proxyErr := imageadmission.VerifyProviderProxy(ctx, cfg.ProviderProxy); proxyErr == nil {
			proxyEvidence, _ := json.Marshal(proxy)
			proxyManifest, _ := json.Marshal(map[string]any{"version": proxy.Version, "contract": proxy.Contract, "protocol": proxy.Protocol, "manifest": proxy.Manifest})
			proxySum := sha256.Sum256(proxyManifest)
			status := "ADMITTED"
			if len(allowed) > 0 && !allowed[proxy.ImageDigest] {
				status = "REJECTED"
				proxyEvidence, _ = json.Marshal(map[string]any{"reasonCode": "NOT_ALLOWED_BY_WORKER_POLICY", "imageDigest": proxy.ImageDigest})
			}
			images = append(images, registrydomain.Image{ImageDigest: proxy.ImageDigest, ImageRef: proxy.ImageRef, Role: "provider-proxy", ContractVersion: "seven.worker-image/v1", ManifestDigest: hex.EncodeToString(proxySum[:]), Manifest: proxyManifest, Capabilities: json.RawMessage(`{"providerProxy":true,"providerGatewayClient":true}`), AdmissionStatus: status, AdmissionEvidence: proxyEvidence, PresenceStatus: "PRESENT", InventoryVersion: 1})
		}
	}
	if len(images) == 0 {
		return nil, errors.New("no seven.worker-image/v1 image passed local discovery")
	}
	return images, nil
}

func admittedRegistryImage(candidateRef string, report imageadmission.Report, manifest, evidence json.RawMessage, manifestDigest string) registrydomain.Image {
	capabilities, _ := json.Marshal(report.Probe)
	environmentCapabilities, _ := json.Marshal(report.EnvironmentCapabilities)
	executionKinds, _ := json.Marshal(report.ExecutionKinds)
	return registrydomain.Image{
		ImageDigest: report.ImageDigest, ImageRef: candidateRef, Role: "attempt",
		ContractVersion: report.Contract, ManifestDigest: manifestDigest,
		Manifest: manifest, Capabilities: capabilities,
		SchedulingScope: report.SchedulingScope, SchedulingPriority: report.SchedulingPriority,
		EnvironmentCapabilities: environmentCapabilities, ExecutionKinds: executionKinds,
		SchedulingDigest: report.SchedulingDigest, AdmissionStatus: "ADMITTED",
		AdmissionEvidence: evidence, PresenceStatus: "PRESENT", InventoryVersion: 1,
	}
}

type protocolImage struct {
	Digest string
	Ref    string
	Role   string
}

func discoverProtocolImages(ctx context.Context) ([]protocolImage, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "docker", "image", "ls", "--no-trunc", "--quiet", "--filter", "label=io.github.caixypromise.seven-worker.image.contract=seven.worker-image/v1").Output()
	if err != nil {
		return nil, err
	}
	idsSeen := map[string]bool{}
	result := []protocolImage{}
	for _, id := range strings.Fields(string(output)) {
		if idsSeen[id] {
			continue
		}
		idsSeen[id] = true
		inspect, inspectErr := exec.CommandContext(commandCtx, "docker", "image", "inspect", id).Output()
		if inspectErr != nil {
			continue
		}
		var rows []struct {
			ID       string   `json:"Id"`
			RepoTags []string `json:"RepoTags"`
			Config   struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
		}
		if json.Unmarshal(inspect, &rows) != nil || len(rows) != 1 {
			continue
		}
		role := rows[0].Config.Labels["io.github.caixypromise.seven-worker.image.role"]
		if role != "attempt" && role != "provider-proxy" {
			continue
		}
		ref := rows[0].ID
		if len(rows[0].RepoTags) > 0 && rows[0].RepoTags[0] != "<none>:<none>" {
			ref = rows[0].RepoTags[0]
		}
		result = append(result, protocolImage{Digest: rows[0].ID, Ref: ref, Role: role})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Digest < result[j].Digest })
	return result, nil
}

func canonicalInventoryDigest(images []registrydomain.Image) string {
	copyImages := append([]registrydomain.Image(nil), images...)
	sort.Slice(copyImages, func(i, j int) bool { return copyImages[i].ImageDigest < copyImages[j].ImageDigest })
	body, _ := json.Marshal(copyImages)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func harnessExitCode(exitCode int) string {
	switch exitCode {
	case 40:
		return "HOOK_PREPARE_DENIED"
	case 41:
		return "HOOK_PROPOSAL_DENIED"
	case 42:
		return "HOOK_AUDIT_UNAVAILABLE"
	default:
		return "HARNESS_EXIT_NONZERO"
	}
}

type terminalFailure struct{ cause error }

func (e *terminalFailure) Error() string { return e.cause.Error() }
func terminalMachineError(err error) bool {
	var terminal *terminalFailure
	return errors.As(err, &terminal)
}

func strictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

type messageHeader struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
}

// deliveryHeader deliberately reads only the routing header.  A complete
// assignment contains identity and lease-routing fields in addition to this
// header, so decoding the header with strictJSON would reject every valid
// message as having unknown fields before the kind-specific strict decoder
// below gets to validate its full schema.  json.Unmarshal still rejects
// malformed or trailing JSON; the selected full message is then decoded with
// strictJSON and remains the authority for accepting it.
func deliveryHeader(body []byte) (messageHeader, error) {
	var header messageHeader
	if err := json.Unmarshal(body, &header); err != nil {
		return messageHeader{}, err
	}
	return header, nil
}

func acquireWorkerLock(workspaceRoot string) (func(), error) {
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(workspaceRoot, ".seven-worker.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("another seven-worker process owns this workspace root")
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

func readToken(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("Worker token file must be a regular file with mode 0600")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(body))
	if len(token) < 32 {
		return "", errors.New("Worker token is invalid")
	}
	return token, nil
}

func fatal(operation string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", operation, err)
	os.Exit(1)
}
