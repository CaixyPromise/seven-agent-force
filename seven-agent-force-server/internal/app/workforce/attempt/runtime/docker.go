package runtime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	providerproxy "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/proxy"
	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	reviewfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/supervisorwire"
)

type Docker struct {
	cleanupJournal           *resourcejournal.Journal
	cleanupCommand           resourcejournal.Command
	cfg                      config.WorkerConfig
	workspace                *PersistentWorkspace
	providerProxy            config.ProviderProxyConfig
	providerSSRF             config.ProviderSSRFConfig
	waitProviderProxy        func(context.Context, string) error
	commandText              func(context.Context, string, ...string) (string, error)
	providerProxyControlExec func(context.Context, string, []byte) ([]byte, error)
}

var (
	ErrWorkspacePrepareTimeout  = errors.New("workspace prepare timeout")
	ErrSupervisorStartupTimeout = errors.New("supervisor startup timeout")
)

func NewDocker(cfg config.WorkerConfig, providerProxy config.ProviderProxyConfig, providerSSRF ...config.ProviderSSRFConfig) *Docker {
	var ssrf config.ProviderSSRFConfig
	if len(providerSSRF) > 0 {
		ssrf = providerSSRF[0]
	}
	return &Docker{
		cfg: cfg, providerProxy: providerProxy, providerSSRF: ssrf,
		waitProviderProxy: waitUnixSocket, commandText: commandText,
		providerProxyControlExec: execProviderProxyControl,
	}
}

// BindPersistentWorkspace lets the Worker startup sequence share one
// preflighted, canonical workspaceRoot with the Docker runtime. It is kept
// separate from Docker ownership so an API process never has to own a Docker
// socket or a host workspace.
func (d *Docker) BindPersistentWorkspace(workspace *PersistentWorkspace) {
	if d != nil {
		d.workspace = workspace
	}
}

func (d *Docker) persistentWorkspace() (*PersistentWorkspace, error) {
	if d == nil {
		return nil, errors.New("WORKSPACE_RUNTIME_UNAVAILABLE")
	}
	if d.workspace != nil {
		return d.workspace, nil
	}
	workspace, err := OpenPersistentWorkspace(d.cfg.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	d.workspace = workspace
	return workspace, nil
}

type Workspace struct {
	TaskRoot    string
	InputRoot   string
	AttemptRoot string
	Repo        string
	Output      string
	State       string
	Runtime     string
	Temporary   string
	SpecPath    string
}

type RunResult struct {
	InteractionSuspended bool
	ContainerID          string
	ExitCode             int
	CompletionRaw        []byte
	EvaluationCycleID    int64
	SessionDigest        string
	TerminalDirective    string
	TerminalDecision     string
	TerminalReasonCode   string
}

type ProviderRuntimeIdentity struct {
	ModelCredentialID    int64
	CredentialVersion    int64
	Protocol             string
	PrimaryModel         string
	ExecutionDeadline    string
	ExpectedProxyDigest  string
	ExpectedProxyVersion string
}

type HookMount struct {
	Source string
	Digest string
}

// ResultProcessorMount is a Worker-owned read-only materialization of the
// exact Result Processor release frozen in the Task snapshot. The Supervisor
// never resolves authoring defaults or object storage keys itself.
type ResultProcessorMount struct {
	Source    string
	Digest    string
	ReleaseID int64
}

type EventSink func(eventType string, payload any)

// RuntimeGate keeps the Docker adapter mechanical. Server-owned state
// transitions remain behind the Worker Machine API callbacks.
type RuntimeGate struct {
	DrainInteractions  func(context.Context) error
	SuspendInteraction func(context.Context, *ExecutionChannel) error
	InteractionSession func(context.Context, *ExecutionChannel) (func(), error)
	ReadEvaluation     func(context.Context, processorfacade.EvaluationReplayIdentity) (processorfacade.EvaluationReplay, error)
	ProviderProxyReady func(dispatchdomain.ProviderProxyReady) error
	ContainerCreated   func(containerID string) error
	SupervisorReady    func(dispatchdomain.SupervisorReady) error
	AcquireRunPermit   func() (dispatchdomain.RunPermit, error)
	HarnessStarted     func(dispatchdomain.HarnessStarted) error
	// EvaluationReady records the durable boundary between a finished Agent
	// round and a protected Result Processor invocation.  It must complete
	// before CONFIG/SECRET material can be requested for the cycle.
	EvaluationReady       func(cycleID int64, cycleNumber int) error
	CheckEvaluation       func(context.Context, int64) error
	MaterializeEvaluation func(cycleID int64, cycleNumber int) (processorfacade.Materialization, error)
	PersistEvaluation     func(candidate processorfacade.EvaluationCandidate) (processorfacade.EvaluationDirective, error)
	// DeliveryStarted is transport-only observation after the Server has
	// durably accepted HARNESS_STARTED. Its failure must never stop the live
	// Runtime: the Worker closes the consumer channel and lets RabbitMQ
	// redeliver, then ACK_ALREADY_RUNNING settles the duplicate delivery.
	DeliveryStarted func() error
}

type providerProxyRuntime struct {
	NetworkName             string
	ContainerID             string
	ContainerName           string
	ImageDigest             string
	ExecutionIdentityDigest string
	ControlSocket           string
	cleanup                 func()
}

func waitUnixSocket(ctx context.Context, path string) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := os.Stat(path)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type RecoveryProbe struct {
	TaskID   int64
	Evidence dispatchdomain.RuntimeEvidence
}

type ReattachGate struct {
	DrainInteractions     func(context.Context) error
	SuspendInteraction    func(context.Context, *ExecutionChannel) error
	InteractionSession    func(context.Context, *ExecutionChannel) (func(), error)
	ReadEvaluation        func(context.Context, processorfacade.EvaluationReplayIdentity) (processorfacade.EvaluationReplay, error)
	AcquireRunPermit      func() (dispatchdomain.RunPermit, error)
	HarnessStarted        func(dispatchdomain.HarnessStarted) error
	EvaluationReady       func(cycleID int64, cycleNumber int) error
	CheckEvaluation       func(context.Context, int64) error
	MaterializeEvaluation func(cycleID int64, cycleNumber int) (processorfacade.Materialization, error)
	PersistEvaluation     func(candidate processorfacade.EvaluationCandidate) (processorfacade.EvaluationDirective, error)
}

// TerminateRuntimeForLoss is the overlap gate for a replacement Attempt. It
// only acts on the exact Server-recorded managed container and confirms that
// the old Runtime is no longer present before the Worker reports host/runtime
// loss. A mismatched container ID fails closed instead of being killed or
// allowing another generation to start beside it.
func (d *Docker) TerminateRuntimeForLoss(ctx context.Context, taskID int64, attemptID int64, containerName string, expectedContainerID string, generation int64, ownerEpoch int64) error {
	return d.cleanupLost(expectedContainerID, map[string]string{
		"seven.managed": "true", "seven.worker.id": strconv.FormatInt(d.cfg.ID, 10),
		"seven.task.id": strconv.FormatInt(taskID, 10), "seven.attempt.id": strconv.FormatInt(attemptID, 10),
		"seven.attempt.generation": strconv.FormatInt(generation, 10),
	})
}

func (d *Docker) TerminateProviderProxyForLoss(ctx context.Context, taskID int64, attemptID int64, containerName string, expectedContainerID string, imageDigest string, identityDigest string, generation, ownerEpoch int64) error {
	if containerName == "" && expectedContainerID == "" {
		return nil
	}
	if err := d.cleanupLost(expectedContainerID, map[string]string{
		"seven.managed": "true", "seven.worker.id": strconv.FormatInt(d.cfg.ID, 10),
		"seven.task.id": strconv.FormatInt(taskID, 10), "seven.attempt.id": strconv.FormatInt(attemptID, 10),
		"seven.attempt.generation":          strconv.FormatInt(generation, 10),
		"seven.provider.proxy.image.digest": imageDigest, "seven.provider.proxy.identity.digest": identityDigest,
	}); err != nil {
		return err
	}
	if d.cleanupJournal != nil {
		return d.cleanupJournal.CleanupMatching(map[string]string{"seven.attempt.id": strconv.FormatInt(attemptID, 10)}, func(c context.Context, r resourcejournal.Resource) error {
			return resourcejournal.Remove(c, d.cleanupCommand, r)
		})
	}
	return nil
}

func (d *Docker) interruptProviderProxy(ctx context.Context, taskID int64, attemptID int64, generation, ownerEpoch int64) error {
	response, err := d.callProviderProxyControl(ctx, taskID, attemptID, generation, map[string]any{"schemaVersion": 1, "command": "interrupt", "attemptId": strconv.FormatInt(attemptID, 10), "generation": strconv.FormatInt(generation, 10), "ownerEpoch": strconv.FormatInt(ownerEpoch, 10)})
	if err != nil {
		return err
	}
	if !response.OK {
		return errors.New(response.ReasonCode)
	}
	return nil
}

func (d *Docker) InspectRuntimeForRecovery(ctx context.Context, taskID int64, attemptID int64, assignmentID int64, nodeID int64, hostBootID string, containerName string, expectedContainerID string, imageDigest string, hookDigest string, identityDigest string, generation, ownerEpoch int64) (RecoveryProbe, error) {
	output, err := exec.CommandContext(ctx, "docker", "container", "inspect", containerName).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return RecoveryProbe{}, errors.New("RUNTIME_MISSING")
		}
		return RecoveryProbe{}, err
	}
	var rows []struct {
		ID    string `json:"Id"`
		Image string `json:"Image"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal(output, &rows) != nil || len(rows) != 1 || rows[0].ID == "" || !rows[0].State.Running {
		return RecoveryProbe{}, errors.New("RUNTIME_MISSING")
	}
	if expectedContainerID != "" && rows[0].ID != expectedContainerID {
		return RecoveryProbe{}, errors.New("RUNTIME_IDENTITY_DRIFT")
	}
	expected := map[string]string{
		"seven.managed": "true", "seven.task.id": strconv.FormatInt(taskID, 10), "seven.attempt.id": strconv.FormatInt(attemptID, 10),
		"seven.execution.node.id": strconv.FormatInt(nodeID, 10), "seven.assignment.id": strconv.FormatInt(assignmentID, 10),
		"seven.worker.host-boot-id": hostBootID, "seven.attempt.generation": strconv.FormatInt(generation, 10),
		"seven.execution.identity.digest": identityDigest,
		"seven.harness.image.digest":      imageDigest, "seven.hook.configuration.digest": hookDigest,
	}
	for key, value := range expected {
		if rows[0].Config.Labels[key] != value {
			return RecoveryProbe{}, fmt.Errorf("RUNTIME_IDENTITY_DRIFT: %s", key)
		}
	}
	labelOwner, parseErr := strconv.ParseInt(rows[0].Config.Labels["seven.owner.epoch"], 10, 64)
	if parseErr != nil || labelOwner < 1 || labelOwner > ownerEpoch {
		return RecoveryProbe{}, errors.New("RUNTIME_IDENTITY_DRIFT: seven.owner.epoch")
	}
	stateResponse, err := callSupervisorContext(ctx, rows[0].ID, map[string]any{"command": "state"})
	if err != nil || !stateResponse.OK {
		return RecoveryProbe{}, errors.New("RUNTIME_UDS_UNAVAILABLE")
	}
	if stateResponse.State.TaskID != taskID || stateResponse.State.AttemptID != attemptID || stateResponse.State.Generation != generation || stateResponse.State.OwnerEpoch != ownerEpoch {
		return RecoveryProbe{}, errors.New("RUNTIME_IDENTITY_DRIFT")
	}
	labelBody, _ := json.Marshal(rows[0].Config.Labels)
	labelSum := sha256.Sum256(labelBody)
	workerID, parseWorkerErr := strconv.ParseInt(rows[0].Config.Labels["seven.worker.id"], 10, 64)
	if parseWorkerErr != nil || workerID <= 0 {
		return RecoveryProbe{}, errors.New("RUNTIME_IDENTITY_DRIFT")
	}
	return RecoveryProbe{TaskID: taskID, Evidence: dispatchdomain.RuntimeEvidence{
		AttemptID: attemptID, ExecutionNodeID: nodeID, AssignmentID: assignmentID,
		WorkerID: workerID, HostBootID: hostBootID, Generation: generation, OwnerEpoch: ownerEpoch,
		ContainerID: rows[0].ID, ContainerName: containerName, LabelsDigest: hex.EncodeToString(labelSum[:]),
		ExecutionIdentityDigest: identityDigest, HarnessImageDigest: imageDigest, HookConfigurationDigest: hookDigest,
		SupervisorProtocol: "seven.supervisor-control/v2", SupervisorState: stateResponse.State.State,
		HarnessPID: stateResponse.State.HarnessPID, RunNonceDigest: stateResponse.State.RunNonceDigest,
		LastJournalSequence:      stateResponse.State.LastJournalSequence,
		AdapterSessionGeneration: stateResponse.State.SessionGeneration, RoundKey: stateResponse.State.CurrentRound,
	}}, nil
}

func (d *Docker) AdoptOwner(ctx context.Context, containerID string, taskID int64, attemptID int64, generation int64, permit dispatchdomain.ReattachPermit) (dispatchdomain.ReattachAck, error) {
	response, err := callSupervisorContext(ctx, containerID, map[string]any{
		"schemaVersion": 2, "command": "adopt-owner", "taskId": strconv.FormatInt(taskID, 10), "attemptId": strconv.FormatInt(attemptID, 10), "generation": strconv.FormatInt(generation, 10),
		"previousOwnerEpoch": strconv.FormatInt(permit.OldOwnerEpoch, 10), "ownerEpoch": strconv.FormatInt(permit.NewOwnerEpoch, 10),
		"reattachPermitId": strconv.FormatInt(permit.PermitID, 10), "reattachPermitToken": permit.PermitToken,
		"expiresAt": permit.ExpiresAt, "leaseExpiresAt": permit.LeaseExpiresAt,
		"leaseReceiptDigest": authorityReceipt(attemptID, permit.NewOwnerEpoch, permit.LeaseExpiresAt),
	})
	if err != nil {
		return dispatchdomain.ReattachAck{}, err
	}
	if !response.OK {
		return dispatchdomain.ReattachAck{}, fmt.Errorf("%s: %s", response.ReasonCode, response.Error)
	}
	// Read the same exact container after adoption; an ACK alone cannot prove
	// the native session/round survived the owner transfer.
	if !permit.InteractionReplayRequested {
		return response.ReattachAck, nil
	}
	state, err := callSupervisorContext(ctx, containerID, map[string]any{"command": "state"})
	if err != nil {
		return dispatchdomain.ReattachAck{}, err
	}
	return bindAdoptedInteractionState(containerID, taskID, attemptID, generation, permit, response.ReattachAck, state)
}

func (d *Docker) InspectProviderProxyForRecovery(ctx context.Context, taskID int64, attemptID int64, hostBootID string, containerName string, expectedContainerID string, imageDigest string, identityDigest string, generation, ownerEpoch int64) (dispatchdomain.ProviderProxyEvidence, error) {
	if containerName == "" || expectedContainerID == "" || imageDigest == "" || len(identityDigest) != 64 {
		return dispatchdomain.ProviderProxyEvidence{}, errors.New("PROVIDER_PROXY_RUNTIME_EVIDENCE_INCOMPLETE")
	}
	output, err := exec.CommandContext(ctx, "docker", "container", "inspect", containerName).Output()
	if err != nil {
		return dispatchdomain.ProviderProxyEvidence{}, errors.New("PROVIDER_PROXY_RUNTIME_MISSING")
	}
	var rows []struct {
		ID    string `json:"Id"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal(output, &rows) != nil || len(rows) != 1 || rows[0].ID != expectedContainerID || !rows[0].State.Running {
		return dispatchdomain.ProviderProxyEvidence{}, errors.New("PROVIDER_PROXY_RUNTIME_MISSING")
	}
	expected := map[string]string{
		"seven.managed": "true", "seven.task.id": strconv.FormatInt(taskID, 10), "seven.attempt.id": strconv.FormatInt(attemptID, 10),
		"seven.attempt.generation":  strconv.FormatInt(generation, 10),
		"seven.worker.host-boot-id": hostBootID, "seven.provider.proxy.image.digest": imageDigest,
		"seven.provider.proxy.identity.digest": identityDigest,
	}
	for key, value := range expected {
		if rows[0].Config.Labels[key] != value {
			return dispatchdomain.ProviderProxyEvidence{}, fmt.Errorf("PROVIDER_PROXY_IDENTITY_DRIFT: %s", key)
		}
	}
	labelOwner, parseErr := strconv.ParseInt(rows[0].Config.Labels["seven.owner.epoch"], 10, 64)
	if parseErr != nil || labelOwner < 1 || labelOwner > ownerEpoch {
		return dispatchdomain.ProviderProxyEvidence{}, errors.New("PROVIDER_PROXY_IDENTITY_DRIFT: seven.owner.epoch")
	}
	state, err := d.callProviderProxyControl(ctx, taskID, attemptID, generation, map[string]any{"schemaVersion": 1, "command": "state", "attemptId": strconv.FormatInt(attemptID, 10), "generation": strconv.FormatInt(generation, 10), "ownerEpoch": strconv.FormatInt(ownerEpoch, 10)})
	if err != nil || !state.OK || state.OwnerEpoch != ownerEpoch || state.ControlState != "READY" {
		return dispatchdomain.ProviderProxyEvidence{}, errors.New("PROVIDER_PROXY_UDS_UNAVAILABLE")
	}
	labelBody, _ := json.Marshal(rows[0].Config.Labels)
	labelSum := sha256.Sum256(labelBody)
	return dispatchdomain.ProviderProxyEvidence{ContainerID: rows[0].ID, ContainerName: containerName, LabelsDigest: hex.EncodeToString(labelSum[:]), ImageDigest: imageDigest, ExecutionIdentityDigest: identityDigest, OwnerEpoch: ownerEpoch, ControlState: state.ControlState}, nil
}

func (d *Docker) AdoptProviderProxyOwner(ctx context.Context, taskID int64, attemptID int64, workerInstanceID string, generation int64, permit dispatchdomain.ReattachPermit) (dispatchdomain.ProviderProxyReattachAck, error) {
	response, err := d.callProviderProxyControl(ctx, taskID, attemptID, generation, map[string]any{
		"schemaVersion": 1, "command": "adopt-owner", "attemptId": strconv.FormatInt(attemptID, 10), "generation": strconv.FormatInt(generation, 10),
		"expectedOwnerEpoch": strconv.FormatInt(permit.OldOwnerEpoch, 10), "ownerEpoch": strconv.FormatInt(permit.NewOwnerEpoch, 10), "workerInstanceId": workerInstanceID,
		"leaseToken": permit.LeaseToken, "leaseExpiresAt": permit.LeaseExpiresAt,
		"reattachPermitId": strconv.FormatInt(permit.PermitID, 10), "reattachPermitToken": permit.PermitToken,
		"reattachPermitExpiresAt": permit.ExpiresAt,
	})
	if err != nil {
		return dispatchdomain.ProviderProxyReattachAck{}, err
	}
	if !response.OK {
		return dispatchdomain.ProviderProxyReattachAck{}, errors.New(response.ReasonCode)
	}
	return dispatchdomain.ProviderProxyReattachAck{PermitID: response.ReattachPermitID, PermitDigest: response.ReattachPermitDigest, OldOwnerEpoch: permit.OldOwnerEpoch, NewOwnerEpoch: response.OwnerEpoch, ControlState: response.ControlState}, nil
}

func (d *Docker) RenewProviderProxyAuthority(ctx context.Context, taskID int64, attempt domain.Attempt, expiresAt string) error {
	response, err := d.callProviderProxyControl(ctx, taskID, attempt.ID, attempt.Generation, map[string]any{
		"schemaVersion": 1, "command": "renew-authority", "attemptId": strconv.FormatInt(attempt.ID, 10), "generation": strconv.FormatInt(attempt.Generation, 10),
		"ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10), "leaseToken": attempt.LeaseToken, "leaseExpiresAt": expiresAt,
	})
	if err != nil {
		return err
	}
	if !response.OK {
		return errors.New(response.ReasonCode)
	}
	return nil
}

type providerProxyControlResponse struct {
	OK                   bool   `json:"ok"`
	ReasonCode           string `json:"reasonCode"`
	OwnerEpoch           int64  `json:"ownerEpoch,string"`
	WorkerInstanceID     string `json:"workerInstanceId"`
	LeaseExpiresAt       string `json:"leaseExpiresAt"`
	ReattachPermitID     int64  `json:"reattachPermitId,string"`
	ReattachPermitDigest string `json:"reattachPermitDigest"`
	ControlState         string `json:"controlState"`
}

func (d *Docker) callProviderProxyControl(ctx context.Context, taskID int64, attemptID int64, generation int64, request map[string]any) (providerProxyControlResponse, error) {
	var response providerProxyControlResponse
	body, err := json.Marshal(request)
	if err != nil {
		return response, err
	}
	controlCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	containerName := sanitizeName("seven-provider-proxy-" + strconv.FormatInt(attemptID, 10))
	output, err := d.providerProxyControlExec(controlCtx, containerName, body)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return response, fmt.Errorf("Provider Proxy control exec: %w: %s", err, boundedText(exitErr.Stderr, 4096))
		}
		return response, err
	}
	if err = decodeProviderProxyControlResponse(output, &response); err != nil {
		return response, err
	}
	return response, nil
}

func execProviderProxyControl(ctx context.Context, containerName string, body []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "/usr/local/bin/seven-provider-proxy", "control")
	command.Stdin = bytes.NewReader(body)
	return command.Output()
}

func decodeProviderProxyControlResponse(body []byte, response *providerProxyControlResponse) error {
	decoder := json.NewDecoder(io.LimitReader(strings.NewReader(string(body)), 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(response); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("Provider Proxy control response contains trailing data")
	}
	return nil
}

func (d *Docker) RenewAuthority(ctx context.Context, containerID string, taskID int64, attempt domain.Attempt, expiresAt, receiptDigest string) error {
	response, err := callSupervisorContext(ctx, containerID, map[string]any{
		"schemaVersion": 2, "command": "renew-authority", "taskId": strconv.FormatInt(taskID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10),
		"generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10),
		"leaseExpiresAt": expiresAt, "leaseReceiptDigest": receiptDigest,
	})
	if err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("%s: %s", response.ReasonCode, response.Error)
	}
	return nil
}

func (d *Docker) ExistingWorkspace(item domain.WorkItem, attempt domain.Attempt) (Workspace, error) {
	workspace, err := d.persistentWorkspace()
	if err != nil {
		return Workspace{}, err
	}
	return workspace.Attempt(item.TaskID, attempt.ID)
}

func (d *Docker) harnessTimeout(item domain.WorkItem) time.Duration {
	return time.Duration(item.HarnessTimeoutSeconds) * time.Second
}

func (d *Docker) Resume(ctx context.Context, item domain.WorkItem, attempt domain.Attempt, containerID string, startNeeded bool, sink EventSink, gate ReattachGate, cancelling func() bool) (RunResult, error) {
	ctx, executionSession := executionSessionScope(ctx, item.TaskID, attempt.ID, attempt.Generation, attempt.OwnerEpoch, sink)
	defer executionSession.Close()
	workspace, err := d.ExistingWorkspace(item, attempt)
	if err != nil {
		return RunResult{}, err
	}
	// Once the new owner has adopted the Runtime it also owns terminal cleanup.
	// Keep this deferred so cancellation, a missing completion file, or another
	// early return cannot leave an exited managed container behind.
	defer d.cleanupContainer(containerID, item.TaskID, attempt.ID)
	runCtx, runCancel, budgetErr := ExecutionBudgetContext(ctx, d.harnessTimeout(item), BudgetIdentity{TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch}, true)
	if budgetErr != nil {
		return RunResult{}, budgetErr
	}
	defer runCancel()
	defer watchInteractionBudget(runCtx, &ExecutionChannel{session: executionSession, container: containerID})()
	// Historical structured facts are recovered from the digest-addressed
	// Runtime journal. Following Docker logs only from this adoption instant
	// prevents replaying old stdout as new random facts.
	logsSince := time.Now().UTC().Format(time.RFC3339Nano)
	cmd := exec.CommandContext(runCtx, "docker", "logs", "--since", logsSince, "-f", containerID)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return RunResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return RunResult{}, err
	}
	if err = cmd.Start(); err != nil {
		return RunResult{}, err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go scanLogs(stdout, "stdout", sink, &wg)
	go scanLogs(stderr, "stderr", sink, &wg)
	if startNeeded {
		if gate.AcquireRunPermit == nil || gate.HarnessStarted == nil {
			return RunResult{}, errors.New("reattach gate is incomplete")
		}
		permit, permitErr := gate.AcquireRunPermit()
		if permitErr != nil {
			return RunResult{}, permitErr
		}
		started, startErr := startHarness(runCtx, containerID, item.TaskID, attempt, permit)
		if startErr != nil {
			return RunResult{}, startErr
		}
		if startErr = gate.HarnessStarted(started); startErr != nil {
			return RunResult{}, startErr
		}
		if startErr = confirmHarnessRunning(runCtx, containerID, item.TaskID, attempt, permit); startErr != nil {
			return RunResult{}, startErr
		}
	}
	if gate.InteractionSession != nil {
		closeInteractions, interactionErr := gate.InteractionSession(runCtx, &ExecutionChannel{session: executionSession, container: containerID})
		if interactionErr != nil {
			return RunResult{}, interactionErr
		}
		if closeInteractions != nil {
			defer closeInteractions()
		}
	}
	if gate.EvaluationReady == nil || gate.MaterializeEvaluation == nil || gate.PersistEvaluation == nil {
		return RunResult{}, errors.New("reattach Evaluation gate is incomplete")
	}
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		ticker := time.NewTicker(d.cfg.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if cancelling() {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
					_ = interruptSupervisor(stopCtx, containerID, item.TaskID, attempt)
					_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "10", containerID).Run()
					stopCancel()
					return
				}
			}
		}
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	evaluationErr := d.driveResultEvaluations(runCtx, containerID, item, attempt, workspace, RuntimeGate{
		DrainInteractions:     gate.DrainInteractions,
		SuspendInteraction:    gate.SuspendInteraction,
		ReadEvaluation:        gate.ReadEvaluation,
		EvaluationReady:       gate.EvaluationReady,
		CheckEvaluation:       gate.CheckEvaluation,
		MaterializeEvaluation: gate.MaterializeEvaluation,
		PersistEvaluation:     gate.PersistEvaluation,
	})
	if evaluationErr != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = interruptSupervisor(stopCtx, containerID, item.TaskID, attempt)
		_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "10", containerID).Run()
		stopCancel()
	}
	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-runCtx.Done():
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = interruptSupervisor(stopCtx, containerID, item.TaskID, attempt)
		_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "10", containerID).Run()
		stopCancel()
		waitErr = <-waitDone
	}
	wg.Wait()
	runtimeErr := runCtx.Err()
	runCancel()
	<-cancelDone
	if errors.Is(runtimeErr, context.DeadlineExceeded) {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = interruptSupervisor(stopCtx, containerID, item.TaskID, attempt)
		_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "10", containerID).Run()
		stopCancel()
		return RunResult{ContainerID: containerID}, context.DeadlineExceeded
	}
	if evaluationErr != nil {
		if errors.Is(evaluationErr, ErrInteractionSuspended) {
			return RunResult{ContainerID: containerID, InteractionSuspended: true}, nil
		}
		return RunResult{ContainerID: containerID}, evaluationErr
	}
	exitText, inspectErr := commandText(context.Background(), "docker", "inspect", "--format", "{{.State.ExitCode}}", containerID)
	if inspectErr != nil {
		return RunResult{ContainerID: containerID}, inspectErr
	}
	exitCode, _ := strconv.Atoi(exitText)
	if waitErr != nil && exitCode == 0 {
		return RunResult{ContainerID: containerID}, waitErr
	}
	completion, completionErr := os.ReadFile(filepath.Join(workspace.Output, "completion.json"))
	if completionErr == nil {
		metadata, err := readCompletionEvaluationMetadata(completion)
		if err != nil {
			return RunResult{ContainerID: containerID, ExitCode: exitCode}, err
		}
		return RunResult{ContainerID: containerID, ExitCode: exitCode, CompletionRaw: completion, EvaluationCycleID: metadata.EvaluationCycleID, SessionDigest: metadata.SessionDigest}, nil
	}
	terminal, terminalErr := readEvaluationTerminal(filepath.Join(workspace.Output, "evaluation-terminal.json"))
	if terminalErr == nil {
		return RunResult{ContainerID: containerID, ExitCode: exitCode, TerminalDirective: terminal.Directive, TerminalDecision: terminal.Decision, TerminalReasonCode: terminal.ReasonCode, EvaluationCycleID: terminal.EvaluationCycleID, SessionDigest: terminal.SessionDigest}, nil
	}
	if exitCode == 0 {
		return RunResult{ContainerID: containerID}, errors.New("completion proposal or evaluation terminal is missing")
	}
	return RunResult{ContainerID: containerID, ExitCode: exitCode}, nil
}

// ReadRuntimeJournalFacts returns the Supervisor-owned facts that may have
// been produced while no Worker process was attached. The stable factId and
// payload digest make replay across repeated owner transfers idempotent at the
// Server. Harness-start is already a dedicated Machine transition and is not
// replayed through the generic fact endpoint.
func (d *Docker) ReadRuntimeJournalFacts(item domain.WorkItem, attempt domain.Attempt) ([]dispatchdomain.Fact, error) {
	workspace, err := d.ExistingWorkspace(item, attempt)
	if err != nil {
		return nil, err
	}
	journalPath := filepath.Join(workspace.State, "runtime-journal.jsonl")
	file, err := os.Open(journalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 8<<20 {
		return nil, errors.New("RUNTIME_JOURNAL_TOO_LARGE")
	}
	scanner := bufio.NewScanner(io.LimitReader(file, 8<<20))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	facts := make([]dispatchdomain.Fact, 0, 32)
	var previous int64
	for scanner.Scan() {
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		var row supervisorwire.JournalEntry
		if err := decoder.Decode(&row); err != nil {
			return nil, fmt.Errorf("decode Runtime journal: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, errors.New("RUNTIME_JOURNAL_TRAILING_DATA")
		}
		if row.SchemaVersion != 1 || row.Sequence <= previous || row.FactID != fmt.Sprintf("jrn_%d_%d", attempt.ID, row.Sequence) || !json.Valid(row.Payload) {
			return nil, errors.New("RUNTIME_JOURNAL_IDENTITY_INVALID")
		}
		previous = row.Sequence
		sum := sha256.Sum256(row.Payload)
		if hex.EncodeToString(sum[:]) != row.PayloadDigest {
			return nil, errors.New("RUNTIME_JOURNAL_DIGEST_MISMATCH")
		}
		if dispatchdomain.ReplayableJournalFact(row.Type) {
			facts = append(facts, dispatchdomain.Fact{FactID: row.FactID, ProducerSequence: row.Sequence, EventType: row.Type, Payload: row.Payload, PayloadDigest: row.PayloadDigest})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return facts, nil
}

func authorityReceipt(attemptID int64, ownerEpoch int64, expiresAt string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("seven.execution-lease/v1\x00%d\x00%d\x00%s", attemptID, ownerEpoch, expiresAt)))
	return hex.EncodeToString(sum[:])
}

func (d *Docker) Prepare(ctx context.Context, item domain.WorkItem, attempt domain.Attempt) (Workspace, string, error) {
	return Workspace{}, "", errors.New("TASK_INPUT_BUNDLE_REQUIRED")
}

func (d *Docker) PrepareWorkspace(item domain.WorkItem, attempt domain.Attempt) (Workspace, error) {
	storage, err := d.persistentWorkspace()
	if err != nil {
		return Workspace{}, err
	}
	return storage.PrepareAttempt(item.TaskID, attempt.ID)
}

func (d *Docker) Run(ctx context.Context, item domain.WorkItem, attempt domain.Attempt, workspace Workspace, hooks *HookMount, processor *ResultProcessorMount, provider *ProviderRuntimeIdentity, sink EventSink, gate RuntimeGate, cancelling func() bool) (RunResult, error) {
	ctx, executionSession := executionSessionScope(ctx, item.TaskID, attempt.ID, attempt.Generation, attempt.OwnerEpoch, sink)
	defer executionSession.Close()
	containerTaskRoot := filepath.Join("/workspace", strconv.FormatInt(item.TaskID, 10))
	containerAttemptRoot := filepath.Join(containerTaskRoot, "attempts", strconv.FormatInt(attempt.ID, 10))
	spec := newAttemptSpecPayload(item, attempt, containerAttemptRoot)
	completionInstruction, completionInputErr := d.materializeCompletionCheckInput(item, attempt, workspace, containerTaskRoot)
	if completionInputErr != nil {
		return RunResult{}, completionInputErr
	}
	stageInstruction, stageErr := d.materializeStageInput(item, attempt, workspace, containerTaskRoot, completionInstruction)
	if stageErr != nil {
		return RunResult{}, stageErr
	}
	completionInstruction = stageInstruction
	spec["instruction"] = completionInstruction
	if correctionEntry, correctionErr := d.materializeReviewCorrection(item, attempt, workspace, containerTaskRoot); correctionErr != nil {
		return RunResult{}, correctionErr
	} else if correctionEntry != nil {
		spec["reviewCorrection"] = correctionEntry
		if reportPath, ok := correctionEntry["reportPath"].(string); ok {
			instruction, bindErr := bindReviewCorrectionInstruction(completionInstruction, reportPath)
			if bindErr != nil {
				return RunResult{}, bindErr
			}
			spec["instruction"] = instruction
		} else if strings.Contains(item.Instruction, continuationfacade.ReviewReportPathPlaceholder) {
			return RunResult{}, errors.New("INDEPENDENT_REVIEW_CORRECTION_PATH_INVALID")
		}
	}
	if hooks != nil {
		spec["hooks"] = map[string]any{"configurationDigest": hooks.Digest, "root": filepath.Join("/opt/seven-worker/managed-hooks", hooks.Digest), "spec": filepath.Join("/opt/seven-worker/managed-hooks", hooks.Digest, "hooks.json")}
	}
	if processor == nil || processor.ReleaseID == 0 || processor.Digest == "" {
		return RunResult{}, errors.New("RESULT_PROCESSOR_MOUNT_REQUIRED")
	}
	processorRoot := filepath.Join("/opt/seven-worker/result-processors", processor.Digest)
	spec["resultProcessor"] = map[string]any{
		"releaseId":             strconv.FormatInt(processor.ReleaseID, 10),
		"digest":                processor.Digest,
		"root":                  processorRoot,
		"bootstrap":             "/opt/seven-worker/result-runtime/bootstrap.py",
		"runtime":               "/opt/seven-worker/result-runtime",
		"pythonRuntimeIdentity": item.ExecutionSnapshot.PythonRuntimeIdentity,
		"bootstrapDigest":       item.ExecutionSnapshot.BootstrapDigest,
		"sdkDigest":             item.ExecutionSnapshot.SDKDigest,
		"moduleIndexDigest":     item.ExecutionSnapshot.ModuleIndexDigest,
		"parameters":            item.ExecutionSnapshot.ResultProcessorParameters,
		"maxEvaluationCycles":   item.ExecutionSnapshot.MaxEvaluationCycles,
	}
	if provider != nil {
		spec["provider"] = map[string]any{"model": provider.PrimaryModel, "protocol": provider.Protocol, "listen": "127.0.0.1:8787", "proxyBaseUrl": "http://seven-provider-proxy:8080"}
	}
	if item.ContinuationRestore != nil {
		spec["continuation"] = map[string]any{
			"sourceTurnId":         strconv.FormatInt(item.ContinuationRestore.SourceTurnID, 10),
			"targetTurnId":         strconv.FormatInt(item.ContinuationRestore.TargetTurnID, 10),
			"checkpointId":         strconv.FormatInt(item.ContinuationRestore.CheckpointID, 10),
			"sessionLineageDigest": item.ContinuationRestore.SessionLineageDigest,
			"resumeNonce":          item.ContinuationRestore.ResumeNonce,
			"resumeNonceDigest":    item.ContinuationRestore.ResumeNonceDigest,
		}
	}
	body, _ := json.MarshalIndent(spec, "", "  ")
	storage, err := d.persistentWorkspace()
	if err != nil {
		return RunResult{}, err
	}
	if err := storage.WriteAtomic(workspace.SpecPath, body, 0o644); err != nil {
		return RunResult{}, err
	}
	containerSpecPath := filepath.Join(containerTaskRoot, "input", "attempts", strconv.FormatInt(attempt.ID, 10), "attempt.json")
	containerName := attempt.ContainerName
	if containerName == "" {
		containerName = sanitizeName("seven-attempt-" + strconv.FormatInt(attempt.ID, 10) + "-g" + strconv.FormatInt(attempt.Generation, 10))
	}
	imageSelector := item.HarnessImageDigest
	if imageSelector == "" {
		return RunResult{}, errors.New("admitted Harness image digest is missing")
	}
	networkName := "none"
	var cleanupProvider func()
	if provider != nil {
		if gate.ProviderProxyReady == nil {
			return RunResult{}, errors.New("provider proxy runtime gate is incomplete")
		}
		proxyRuntime, err := d.startProviderProxy(ctx, item, attempt, *provider)
		if err != nil {
			return RunResult{}, err
		}
		networkName, cleanupProvider = proxyRuntime.NetworkName, proxyRuntime.cleanup
		defer cleanupProvider()
		if err = gate.ProviderProxyReady(dispatchdomain.ProviderProxyReady{
			ContainerID: proxyRuntime.ContainerID, ContainerName: proxyRuntime.ContainerName, ImageDigest: proxyRuntime.ImageDigest,
			ExecutionIdentityDigest: proxyRuntime.ExecutionIdentityDigest, ControlState: "READY",
		}); err != nil {
			return RunResult{}, err
		}
	}
	args := []string{
		"create", "--name", containerName, "--network", networkName, "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--cpus", "1.0",
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,readonly", workspace.InputRoot, filepath.Join(containerTaskRoot, "input")),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s", workspace.AttemptRoot, containerAttemptRoot),
		"-e", "SEVEN_TASK_ROOT=" + containerTaskRoot,
		"-e", "SEVEN_ATTEMPT_ROOT=" + containerAttemptRoot,
		"-e", "SEVEN_ATTEMPT_SPEC=" + containerSpecPath,
		"-e", "SEVEN_TASK_ID=" + strconv.FormatInt(item.TaskID, 10),
		"-e", "SEVEN_ATTEMPT_ID=" + strconv.FormatInt(attempt.ID, 10),
		"-e", "SEVEN_ATTEMPT_GENERATION=" + strconv.FormatInt(attempt.Generation, 10),
		"-e", "SEVEN_WORKSPACE=" + filepath.Join(containerAttemptRoot, "repo"),
		"-e", "SEVEN_OUTPUT=" + filepath.Join(containerAttemptRoot, "output"),
		"-e", "SEVEN_STATE=" + filepath.Join(containerAttemptRoot, "state"),
		// Keep the socket basename short: Linux sockaddr_un is limited to about
		// 108 bytes and the contract deliberately includes both stable IDs.
		"-e", "SEVEN_CONTROL_SOCKET=" + filepath.Join(containerAttemptRoot, "runtime", "s"),
		"-e", "TMPDIR=" + filepath.Join(containerAttemptRoot, "tmp"),
	}
	if item.ExecutionSnapshot.StagePurpose == "PLAN" {
		args = append(args, "-e", "SEVEN_PLANNING_SOURCE="+filepath.Join(containerTaskRoot, "input", "attempts", strconv.FormatInt(attempt.ID, 10), "planning-source"))
	}
	for _, key := range sortedLabelKeys(attempt.RuntimeLabels) {
		args = append(args, "--label", key+"="+attempt.RuntimeLabels[key])
	}
	if len(attempt.RuntimeLabels) == 0 {
		args = append(args, "--label", "seven.managed=true", "--label", "seven.task.id="+strconv.FormatInt(item.TaskID, 10), "--label", "seven.attempt.id="+strconv.FormatInt(attempt.ID, 10))
	}
	if hooks != nil {
		args = append(args, "--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,readonly", hooks.Source, filepath.Join("/opt/seven-worker/managed-hooks", hooks.Digest)), "-e", "SEVEN_HOOK_CONFIGURATION_ROOT="+filepath.Join("/opt/seven-worker/managed-hooks", hooks.Digest), "-e", "SEVEN_HOOK_CONFIGURATION_DIGEST="+hooks.Digest)
	}
	args = append(args, "--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,readonly", processor.Source, processorRoot), "-e", "SEVEN_RESULT_PROCESSOR_ROOT="+processorRoot, "-e", "SEVEN_RESULT_PROCESSOR_DIGEST="+processor.Digest)
	args = append(args, imageSelector, "supervise", "--spec", containerSpecPath)
	createCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	containerID, running, err := inspectExistingRuntime(createCtx, containerName, imageSelector, attempt.RuntimeLabels)
	if err != nil {
		return RunResult{}, err
	}
	if containerID == "" {
		containerID, err = d.commandText(createCtx, "docker", args...)
		if err != nil {
			return RunResult{}, fmt.Errorf("docker create: %w", err)
		}
	}
	defer d.cleanupContainer(containerID, item.TaskID, attempt.ID)
	if gate.ContainerCreated == nil || gate.SupervisorReady == nil || gate.AcquireRunPermit == nil || gate.HarnessStarted == nil || gate.DeliveryStarted == nil {
		return RunResult{ContainerID: containerID}, errors.New("runtime gate is incomplete")
	}
	if err := gate.ContainerCreated(containerID); err != nil {
		return RunResult{ContainerID: containerID}, err
	}

	startupCtx, startupCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer startupCancel()
	if !running {
		// Start without attaching. Docker Desktop's `docker start` does not
		// expose a signal-proxy switch, and `start -a` makes the Worker process
		// group own PID 1's lifetime. A detached start followed by an
		// independent logs follower keeps the Runtime alive when the Worker is
		// killed while preserving the same bounded log stream.
		if output, startErr := exec.CommandContext(startupCtx, "docker", runtimeStartCommand(containerID)...).CombinedOutput(); startErr != nil {
			if errors.Is(startupCtx.Err(), context.DeadlineExceeded) {
				return RunResult{ContainerID: containerID}, fmt.Errorf("%w: %v", ErrSupervisorStartupTimeout, startErr)
			}
			return RunResult{ContainerID: containerID}, fmt.Errorf("start Attempt container: %w: %s", startErr, strings.TrimSpace(string(output)))
		}
	}
	// Log following belongs to the Runtime, not the startup deadline. Keep it
	// under a separate context so a valid long-running Harness is not killed two
	// minutes after the Supervisor became READY.
	logCtx, logCancel := context.WithCancel(ctx)
	defer logCancel()
	cmd := exec.CommandContext(logCtx, "docker", runtimeAttachCommand(containerID)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return RunResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return RunResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return RunResult{}, err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go scanLogs(stdout, "stdout", sink, &wg)
	go scanLogs(stderr, "stderr", sink, &wg)
	abortStartup := func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), resourcejournal.Timeout)
		_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "1", containerID).Run()
		stopCancel()
		logCancel()
		_ = cmd.Wait()
		wg.Wait()
	}
	startupFailure := func(err error) error {
		if errors.Is(startupCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrSupervisorStartupTimeout, err)
		}
		return err
	}
	if err := waitSupervisorReady(startupCtx, containerID, item.TaskID, attempt); err != nil {
		abortStartup()
		return RunResult{ContainerID: containerID}, startupFailure(err)
	}
	ready := dispatchdomain.SupervisorReady{ContainerID: containerID, ControlProtocol: "seven.supervisor-control/v2", ExecutionIdentity: attempt.RuntimeLabels["seven.execution.identity.digest"]}
	if err := gate.SupervisorReady(ready); err != nil {
		abortStartup()
		return RunResult{ContainerID: containerID}, startupFailure(err)
	}
	permit, err := gate.AcquireRunPermit()
	if err != nil {
		abortStartup()
		return RunResult{ContainerID: containerID}, startupFailure(err)
	}
	started, err := startHarness(startupCtx, containerID, item.TaskID, attempt, permit)
	if err != nil {
		abortStartup()
		return RunResult{ContainerID: containerID}, startupFailure(err)
	}
	if err := gate.HarnessStarted(started); err != nil {
		abortStartup()
		return RunResult{ContainerID: containerID}, startupFailure(err)
	}
	if err := confirmHarnessRunning(startupCtx, containerID, item.TaskID, attempt, permit); err != nil {
		abortStartup()
		return RunResult{ContainerID: containerID}, startupFailure(err)
	}
	startupCancel()
	// At this point the Server is the authoritative owner of a RUNNING
	// Attempt. Broker ACK is deliberately not part of the Runtime safety
	// boundary: an uncertain ACK is reconciled by redelivery and must not
	// terminate the Harness that has already started.
	_ = gate.DeliveryStarted()
	executionCtx, executionCancel, budgetErr := ExecutionBudgetContext(ctx, d.harnessTimeout(item), BudgetIdentity{TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch}, item.InteractionBudgetRestored)
	if budgetErr != nil {
		return RunResult{}, budgetErr
	}
	defer executionCancel()
	if gate.InteractionSession != nil {
		closeInteractions, interactionErr := gate.InteractionSession(executionCtx, &ExecutionChannel{session: executionSession, container: containerID})
		if interactionErr != nil {
			return RunResult{}, interactionErr
		}
		if closeInteractions != nil {
			defer closeInteractions()
		}
	}
	defer watchInteractionBudget(executionCtx, &ExecutionChannel{session: executionSession, container: containerID})()
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		ticker := time.NewTicker(d.cfg.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-executionCtx.Done():
				return
			case <-ticker.C:
				if cancelling() {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
					_ = interruptSupervisor(stopCtx, containerID, item.TaskID, attempt)
					_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "10", containerID).Run()
					stopCancel()
					return
				}
			}
		}
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	evaluationErr := d.driveResultEvaluations(executionCtx, containerID, item, attempt, workspace, gate)
	if evaluationErr != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = interruptSupervisor(stopCtx, containerID, item.TaskID, attempt)
		_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "10", containerID).Run()
		stopCancel()
	}
	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-executionCtx.Done():
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = interruptSupervisor(stopCtx, containerID, item.TaskID, attempt)
		_ = exec.CommandContext(stopCtx, "docker", "stop", "--time", "10", containerID).Run()
		stopCancel()
		logCancel()
		waitErr = <-waitDone
	}
	wg.Wait()
	runtimeErr := executionCtx.Err()
	executionCancel()
	<-cancelDone
	if errors.Is(runtimeErr, context.DeadlineExceeded) {
		return RunResult{ContainerID: containerID}, context.DeadlineExceeded
	}
	if evaluationErr != nil {
		if errors.Is(evaluationErr, ErrInteractionSuspended) {
			return RunResult{ContainerID: containerID, InteractionSuspended: true}, nil
		}
		return RunResult{ContainerID: containerID}, evaluationErr
	}
	exitText, inspectErr := commandText(context.Background(), "docker", "inspect", "--format", "{{.State.ExitCode}}", containerID)
	if inspectErr != nil {
		return RunResult{ContainerID: containerID}, inspectErr
	}
	exitCode, _ := strconv.Atoi(exitText)
	if waitErr != nil && exitCode == 0 {
		return RunResult{ContainerID: containerID}, waitErr
	}
	completion, completionErr := os.ReadFile(filepath.Join(workspace.Output, "completion.json"))
	if completionErr == nil {
		metadata, err := readCompletionEvaluationMetadata(completion)
		if err != nil {
			return RunResult{ContainerID: containerID, ExitCode: exitCode}, err
		}
		return RunResult{ContainerID: containerID, ExitCode: exitCode, CompletionRaw: completion, EvaluationCycleID: metadata.EvaluationCycleID, SessionDigest: metadata.SessionDigest}, nil
	}
	terminal, terminalErr := readEvaluationTerminal(filepath.Join(workspace.Output, "evaluation-terminal.json"))
	if terminalErr == nil {
		return RunResult{ContainerID: containerID, ExitCode: exitCode, TerminalDirective: terminal.Directive, TerminalDecision: terminal.Decision, TerminalReasonCode: terminal.ReasonCode, EvaluationCycleID: terminal.EvaluationCycleID, SessionDigest: terminal.SessionDigest}, nil
	}
	if exitCode == 0 {
		return RunResult{ContainerID: containerID, ExitCode: exitCode}, errors.New("completion proposal or evaluation terminal is missing")
	}
	return RunResult{ContainerID: containerID, ExitCode: exitCode}, nil
}

func (d *Docker) materializeReviewCorrection(item domain.WorkItem, attempt domain.Attempt, workspace Workspace, containerTaskRoot string) (map[string]any, error) {
	if item.ReviewCorrection == nil {
		return nil, nil
	}
	correction := item.ReviewCorrection
	if correction.SourceReviewRunID < 1 || (correction.RevisionKind != "AUTOMATIC" && correction.RevisionKind != "HUMAN_AUTHORIZED") {
		return nil, errors.New("INDEPENDENT_REVIEW_CORRECTION_IDENTITY_INVALID")
	}
	containerReportPath := filepath.Join(containerTaskRoot, "input", "attempts", strconv.FormatInt(attempt.ID, 10), "review", "review-report.json")
	entry := map[string]any{"sourceReviewRunId": strconv.FormatInt(correction.SourceReviewRunID, 10), "revisionKind": correction.RevisionKind, "reasonCode": correction.ReasonCode}
	if correction.ReportDigest == "" {
		if correction.RevisionKind != "HUMAN_AUTHORIZED" || !safeCorrectionReasonCode(correction.ReasonCode) {
			return nil, errors.New("INDEPENDENT_REVIEW_CORRECTION_IDENTITY_INVALID")
		}
		return entry, nil
	}
	canonical, digest, canonicalErr := reviewfacade.CanonicalizeCorrection(correction.Report)
	validVerdict := correction.Verdict == "REVISE" || (correction.RevisionKind == "HUMAN_AUTHORIZED" && correction.Verdict == "REJECT")
	if canonicalErr != nil || digest != correction.ReportDigest || !validVerdict || correction.ReportSchema != "review-report/v1" {
		return nil, errors.New("INDEPENDENT_REVIEW_CORRECTION_IDENTITY_INVALID")
	}
	hostReportPath := filepath.Join(workspace.InputRoot, "attempts", strconv.FormatInt(attempt.ID, 10), "review", "review-report.json")
	storage, storageErr := d.persistentWorkspace()
	if storageErr != nil {
		return nil, storageErr
	}
	if storageErr = storage.ensureDirectory(filepath.Dir(hostReportPath), 0o700); storageErr != nil {
		return nil, storageErr
	}
	if storageErr = storage.WriteAtomic(hostReportPath, canonical, 0o444); storageErr != nil {
		return nil, storageErr
	}
	entry["verdict"] = correction.Verdict
	entry["reportPath"] = containerReportPath
	entry["reportDigest"] = correction.ReportDigest
	return entry, nil
}

func safeCorrectionReasonCode(value string) bool {
	switch strings.TrimSpace(value) {
	case "INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED",
		"INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS",
		"INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE",
		"INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT",
		"INDEPENDENT_REVIEW_REJECTED",
		"INDEPENDENT_REVIEW_REPORT_INVALID",
		"INDEPENDENT_REVIEW_RETRY_EXHAUSTED":
		return true
	default:
		return false
	}
}

func bindReviewCorrectionInstruction(instruction, reportPath string) (string, error) {
	if strings.Count(instruction, continuationfacade.ReviewReportPathPlaceholder) != 1 || !filepath.IsAbs(reportPath) {
		return "", errors.New("INDEPENDENT_REVIEW_CORRECTION_PATH_INVALID")
	}
	return strings.Replace(instruction, continuationfacade.ReviewReportPathPlaceholder, reportPath, 1), nil
}

func newAttemptSpecPayload(item domain.WorkItem, attempt domain.Attempt, containerAttemptRoot string) map[string]any {
	spec := map[string]any{
		"schemaVersion": 1, "taskId": strconv.FormatInt(item.TaskID, 10), "projectVersionId": strconv.FormatInt(item.ProjectVersionID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10),
		"generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10),
		"authorityExpiresAt": attempt.LeaseExpiresAt,
		"instruction":        item.Instruction, "workspace": filepath.Join(containerAttemptRoot, "repo"),
		"output": filepath.Join(containerAttemptRoot, "output"), "state": filepath.Join(containerAttemptRoot, "state"),
		"runtime": filepath.Join(containerAttemptRoot, "runtime"), "temporary": filepath.Join(containerAttemptRoot, "tmp"),
	}
	// These fields exist only for verified cold restore input. A nil RawMessage
	// inside a map otherwise serializes as an explicit JSON null on fresh starts.
	if item.InteractionResponse != nil {
		spec["interactionResponse"] = item.InteractionResponse
	}
	if item.InteractionQuestionRequest != nil {
		spec["interactionQuestionRequest"] = item.InteractionQuestionRequest
	}
	if item.OperationToolResult != nil {
		spec["operationToolResult"] = item.OperationToolResult
	}
	if len(item.AllowedAdapterTools) > 0 {
		spec["allowedAdapterTools"] = append([]string(nil), item.AllowedAdapterTools...)
	}
	return spec
}

func runtimeStartCommand(containerID string) []string {
	return []string{"start", containerID}
}

func runtimeAttachCommand(containerID string) []string {
	return []string{"logs", "-f", containerID}
}

type evaluationTerminal struct {
	Directive         string `json:"directive"`
	Decision          string `json:"decision"`
	ReasonCode        string `json:"reasonCode"`
	Message           string `json:"message"`
	EvaluationCycleID int64  `json:"evaluationCycleId,string"`
	SessionDigest     string `json:"sessionDigest"`
}

type completionEvaluationMetadata struct {
	EvaluationCycleID int64  `json:"evaluationCycleId,string"`
	SessionDigest     string `json:"sessionDigest"`
}

func readCompletionEvaluationMetadata(body []byte) (completionEvaluationMetadata, error) {
	var metadata completionEvaluationMetadata
	if len(body) == 0 || !json.Valid(body) || json.Unmarshal(body, &metadata) != nil || metadata.EvaluationCycleID <= 0 {
		return completionEvaluationMetadata{}, errors.New("COMPLETION_EVALUATION_IDENTITY_INVALID")
	}
	return metadata, nil
}

func readEvaluationTerminal(path string) (evaluationTerminal, error) {
	// Docker Desktop bind mounts can publish the final file a few milliseconds
	// after the container wait returns. The Supervisor writes the terminal
	// before exiting, so this bounded read-after-exit retry observes that same
	// immutable file; it never reruns Evaluation or accepts a different fact.
	var lastErr error
	for attempt := 0; attempt < 40; attempt++ {
		value, err := readEvaluationTerminalOnce(path)
		if err == nil {
			return value, nil
		}
		lastErr = err
		time.Sleep(25 * time.Millisecond)
	}
	return evaluationTerminal{}, lastErr
}

func readEvaluationTerminalOnce(path string) (evaluationTerminal, error) {
	var value evaluationTerminal
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 || len(body) > 64<<10 {
		return value, errors.New("evaluation terminal is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return value, errors.New("evaluation terminal contains trailing data")
	}
	valid := (value.Directive == processorfacade.DirectiveWaitForUser && value.Decision == "WAIT_USER") ||
		(value.Directive == processorfacade.DirectiveStopManualReview && value.Decision == "MANUAL_REVIEW") ||
		(value.Directive == processorfacade.DirectiveStopRejected && value.Decision == "REJECT")
	if !valid || value.ReasonCode == "" || len(value.ReasonCode) > 128 || value.EvaluationCycleID <= 0 {
		return value, errors.New("evaluation terminal identity is invalid")
	}
	return value, nil
}

func interruptSupervisor(ctx context.Context, containerID string, taskID int64, attempt domain.Attempt) error {
	_, err := callSupervisorContextWithTimeout(ctx, containerID, map[string]any{
		"schemaVersion": 1, "command": "interrupt", "taskId": strconv.FormatInt(taskID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10),
		"generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10),
	}, 5*time.Second)
	return err
}

func inspectExistingRuntime(ctx context.Context, name, image string, expectedLabels map[string]string) (string, bool, error) {
	expectedImageID, err := commandText(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", image)
	if err != nil {
		return "", false, fmt.Errorf("resolve frozen runtime image: %w", err)
	}
	output, err := exec.CommandContext(ctx, "docker", "container", "inspect", name).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("inspect deterministic runtime: %w", err)
	}
	var rows []struct {
		ID    string `json:"Id"`
		Image string `json:"Image"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal(output, &rows) != nil || len(rows) != 1 || rows[0].ID == "" {
		return "", false, errors.New("RUNTIME_IDENTITY_INVALID")
	}
	if rows[0].Image != expectedImageID {
		return "", false, errors.New("RUNTIME_IMAGE_IDENTITY_CONFLICT")
	}
	for key, value := range expectedLabels {
		if rows[0].Config.Labels[key] != value {
			return "", false, fmt.Errorf("RUNTIME_LABEL_IDENTITY_CONFLICT: %s", key)
		}
	}
	return rows[0].ID, rows[0].State.Running, nil
}

type supervisorResponse struct {
	Capture                InteractionCapture            `json:"capture"`
	ActivityGap            bool                          `json:"activityGap"`
	ActivityDroppedThrough int64                         `json:"activityDroppedThrough,string"`
	Notifications          []InteractionNotification     `json:"notifications"`
	Through                int64                         `json:"through,string"`
	HasMore                bool                          `json:"hasMore"`
	OK                     bool                          `json:"ok"`
	ReasonCode             string                        `json:"reasonCode"`
	Error                  string                        `json:"error"`
	State                  supervisorStateSnapshot       `json:"state"`
	HarnessStarted         dispatchdomain.HarnessStarted `json:"harnessStarted"`
	ReattachAck            dispatchdomain.ReattachAck    `json:"reattachAck"`
	Candidate              supervisorEvaluationCandidate `json:"candidate"`
}

type supervisorEvaluationCandidate struct {
	CycleID       int64  `json:"cycleId,string"`
	CycleNumber   int    `json:"cycleNumber"`
	CandidateFile string `json:"candidateFile"`
	ContractSHA   string `json:"contractSha256"`
	SessionDigest string `json:"sessionDigest"`
}

type supervisorStateSnapshot struct {
	SuspendControlKey        string   `json:"suspendControlKey"`
	ActiveElapsedMillis      int64    `json:"activeElapsedMillis,string"`
	InteractionWaitingMillis int64    `json:"interactionWaitingMillis,string"`
	InteractionWaiting       bool     `json:"interactionWaiting"`
	CurrentRound             string   `json:"currentRound"`
	SessionGeneration        int64    `json:"sessionGeneration,string"`
	InteractionActive        bool     `json:"interactionActive"`
	PendingInteractions      []string `json:"pendingInteractions"`
	PendingTools             []string `json:"pendingTools"`
	ObservationSequence      int64    `json:"observationSequence,string"`
	ObservationGap           bool     `json:"observationGap"`
	State                    string   `json:"state"`
	TaskID                   int64    `json:"taskId,string"`
	AttemptID                int64    `json:"attemptId,string"`
	Generation               int64    `json:"generation,string"`
	OwnerEpoch               int64    `json:"ownerEpoch,string"`
	HarnessPID               int      `json:"harnessPid"`
	RunNonceDigest           string   `json:"runNonceDigest"`
	LastJournalSequence      int64    `json:"lastJournalSequence,string"`
	EvaluationCycleID        int64    `json:"evaluationCycleId,string"`
	EvaluationCycle          int      `json:"evaluationCycle"`
	EvaluationCandidate      string   `json:"evaluationCandidate"`
	EvaluationDigest         string   `json:"evaluationCandidateDigest"`
	SessionDigest            string   `json:"sessionDigest"`
}

func waitSupervisorReady(ctx context.Context, containerID string, taskID int64, attempt domain.Attempt) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := callSupervisorContext(ctx, containerID, map[string]any{"command": "state"})
		if err == nil && response.OK && response.State.State == "READY" && response.State.TaskID == taskID && response.State.AttemptID == attempt.ID && response.State.Generation == attempt.Generation && response.State.OwnerEpoch == attempt.OwnerEpoch {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("SUPERVISOR_READY_TIMEOUT")
		case <-ticker.C:
		}
	}
}

func startHarness(ctx context.Context, containerID string, taskID int64, attempt domain.Attempt, permit dispatchdomain.RunPermit) (dispatchdomain.HarnessStarted, error) {
	response, err := callSupervisorContextWithTimeout(ctx, containerID, map[string]any{
		"schemaVersion": 2, "command": "start-harness", "taskId": strconv.FormatInt(taskID, 10),
		"attemptId": strconv.FormatInt(attempt.ID, 10), "generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10),
		"runPermitId": strconv.FormatInt(permit.PermitID, 10), "runPermitToken": permit.PermitToken,
		"runNonce": permit.RunNonce, "expiresAt": permit.ExpiresAt,
	}, 35*time.Second)
	if err != nil {
		return dispatchdomain.HarnessStarted{}, err
	}
	if !response.OK {
		return dispatchdomain.HarnessStarted{}, fmt.Errorf("%s: %s", response.ReasonCode, response.Error)
	}
	return response.HarnessStarted, nil
}

func confirmHarnessRunning(ctx context.Context, containerID string, taskID int64, attempt domain.Attempt, permit dispatchdomain.RunPermit) error {
	request := map[string]any{
		"schemaVersion": 2, "command": "confirm-running", "taskId": strconv.FormatInt(taskID, 10),
		"attemptId": strconv.FormatInt(attempt.ID, 10), "generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10),
		"runPermitId": strconv.FormatInt(permit.PermitID, 10), "runNonce": permit.RunNonce,
	}
	var lastErr error
	for retry := 0; retry < 3; retry++ {
		response, err := callSupervisorContext(ctx, containerID, request)
		if err == nil && response.OK {
			return nil
		}
		if err == nil {
			err = fmt.Errorf("%s: %s", response.ReasonCode, response.Error)
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(retry+1) * 100 * time.Millisecond):
		}
	}
	return fmt.Errorf("confirm Server-authorized RUNNING with Supervisor: %w", lastErr)
}

// driveResultEvaluations is the Worker-side half of the durable Evaluation
// handshake.  It never decides a Result Contract itself: the protected
// bootstrap writes a candidate, Server persists it, and only the returned
// directive is sent back to the Supervisor.
func (d *Docker) driveResultEvaluations(ctx context.Context, containerID string, item domain.WorkItem, attempt domain.Attempt, workspace Workspace, gate RuntimeGate) error {
	if gate.EvaluationReady == nil || gate.MaterializeEvaluation == nil || gate.PersistEvaluation == nil {
		return errors.New("Result Processor evaluation gate is incomplete")
	}
	seenReady := map[int64]bool{}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := callSupervisorContext(ctx, containerID, map[string]any{
			"schemaVersion": 2, "command": "evaluation-state", "taskId": strconv.FormatInt(item.TaskID, 10),
			"attemptId": strconv.FormatInt(attempt.ID, 10), "generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10),
		})
		if err != nil {
			// Persistent control must not hide reconnect identity/fencing or
			// protocol failures behind the legacy Docker polling fallback.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// evaluation-state is a read. Its short RPC deadline must not
			// terminate a still-valid execution budget or replay any mutation.
			// A late response is discarded by the request identity; query again
			// while the outer budget and runtime authority remain valid.
			if errors.Is(err, context.DeadlineExceeded) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					continue
				}
			}
			if _, persistent := ctx.Value(executionSessionKey{}).(*ExecutionSession); persistent && !retryableEvaluationACKDisconnect(ctx, err) {
				return err
			}
			running, inspectErr := dockerContainerRunning(ctx, containerID)
			if inspectErr == nil && !running {
				return drainTerminalInteractions(ctx, gate, "TERMINAL")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				continue
			}
		}
		if !response.OK {
			return fmt.Errorf("%s: %s", response.ReasonCode, response.Error)
		}
		state := response.State
		switch state.State {
		case "PAUSING_PREPARE", "INTERACTION_SUSPENDED":
			if gate.SuspendInteraction == nil {
				return errors.New("INTERACTION_SUSPEND_UNAVAILABLE")
			}
			session, ok := ctx.Value(executionSessionKey{}).(*ExecutionSession)
			if !ok || session == nil {
				return errors.New("INTERACTION_SESSION_UNAVAILABLE")
			}
			if err := gate.SuspendInteraction(ctx, &ExecutionChannel{session: session, container: containerID}); err != nil {
				if errors.Is(err, ErrInteractionSuspendAborted) {
					continue
				}
				return err
			}
			return ErrInteractionSuspended
		case "EVALUATION_PENDING_ACK":
			directive, err := recoverPendingEvaluation(ctx, d.cfg.WorkspaceRoot, item, attempt, state, gate)
			if err != nil {
				return err
			}
			if err = PersistAuthoritativeEvaluation(d.cfg.WorkspaceRoot, item.TaskID, attempt, state.EvaluationCycleID, directive); err != nil {
				return err
			}
			if err = drainTerminalInteractions(ctx, gate, directive.Directive); err != nil {
				return err
			}
			if err = continueProcessorEvaluation(ctx, containerID, item.TaskID, attempt, directive); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if retryableEvaluationACKDisconnect(ctx, err) {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-ticker.C:
						continue
					}
				}
				return err
			}
			if directive.Directive == processorfacade.DirectiveResumeSameSession {
				continue
			}
			return nil
		case "EVALUATION_READY":
			if state.EvaluationCycleID < 0 || state.EvaluationCycle < 1 || state.EvaluationCycle > item.ExecutionSnapshot.MaxEvaluationCycles {
				return errors.New("EVALUATION_STATE_IDENTITY_INVALID")
			}
			materialization, materializeErr := gate.MaterializeEvaluation(state.EvaluationCycleID, state.EvaluationCycle)
			if materializeErr != nil {
				return materializeErr
			}
			if materialization.CycleID <= 0 || (state.EvaluationCycleID > 0 && materialization.CycleID != state.EvaluationCycleID) {
				return errors.New("EVALUATION_MATERIALIZATION_IDENTITY_INVALID")
			}
			state.EvaluationCycleID = materialization.CycleID
			if state.State == "EVALUATION_READY" && !seenReady[state.EvaluationCycleID] {
				// This is the explicit boundary that stops new Provider invocations
				// before any cycle-scoped CONFIG/SECRET material is requested.
				if err := gate.EvaluationReady(state.EvaluationCycleID, state.EvaluationCycle); err != nil {
					return err
				}
				seenReady[state.EvaluationCycleID] = true
			}
			if gate.CheckEvaluation != nil {
				if err := gate.CheckEvaluation(ctx, state.EvaluationCycleID); err != nil {
					return err
				}
			}
			candidateResponse, runErr := runProcessorEvaluation(ctx, containerID, item.TaskID, attempt, state.EvaluationCycleID, state.EvaluationCycle, materialization)
			if runErr != nil {
				return runErr
			}
			candidate, candidateErr := readProcessorCandidate(workspace, item, attempt, state, candidateResponse, materialization)
			if candidateErr != nil {
				return candidateErr
			}
			// Freeze the already redacted exact request before the uncertain
			// Server commit/response boundary. Recovery never needs secrets.
			if err := persistPendingCandidate(d.cfg.WorkspaceRoot, item.TaskID, attempt, candidate); err != nil {
				return err
			}
			directive, persistErr := gate.PersistEvaluation(candidate)
			if persistErr != nil {
				return persistErr
			}
			if err := PersistAuthoritativeEvaluation(d.cfg.WorkspaceRoot, item.TaskID, attempt, candidate.CycleID, directive); err != nil {
				return err
			}
			if err := drainTerminalInteractions(ctx, gate, directive.Directive); err != nil {
				return err
			}
			if err := continueProcessorEvaluation(ctx, containerID, item.TaskID, attempt, directive); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if retryableEvaluationACKDisconnect(ctx, err) {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-ticker.C:
						continue
					}
				}
				return err
			}
			switch directive.Directive {
			case processorfacade.DirectiveResumeSameSession:
				continue
			case processorfacade.DirectiveProposeCompletion, processorfacade.DirectiveWaitForUser, processorfacade.DirectiveStopRejected, processorfacade.DirectiveStopManualReview:
				return nil
			default:
				return errors.New("RESULT_PROCESSOR_DIRECTIVE_INVALID")
			}
		case "COMPLETED", "WAITING_USER", "MANUAL_REVIEW", "REJECTED", "FAILED", "CANCELLED":
			return drainTerminalInteractions(ctx, gate, "TERMINAL")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func drainTerminalInteractions(ctx context.Context, gate RuntimeGate, directive string) error {
	if directive == processorfacade.DirectiveResumeSameSession {
		return nil
	}
	if gate.DrainInteractions == nil {
		return errors.New("INTERACTION_DRAIN_UNAVAILABLE")
	}
	drain, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return gate.DrainInteractions(drain)
}

func runProcessorEvaluation(ctx context.Context, containerID string, taskID int64, attempt domain.Attempt, cycleID int64, cycleNumber int, materialization processorfacade.Materialization) (supervisorEvaluationCandidate, error) {
	response, err := callSupervisorContextWithTimeout(ctx, containerID, map[string]any{
		"schemaVersion": 2, "command": "run-evaluation", "taskId": strconv.FormatInt(taskID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10),
		"generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10), "cycleId": strconv.FormatInt(cycleID, 10), "cycleNumber": cycleNumber,
		"materialization": materialization,
	}, 35*time.Second)
	if err != nil {
		return supervisorEvaluationCandidate{}, err
	}
	if !response.OK {
		return supervisorEvaluationCandidate{}, fmt.Errorf("%s: %s", response.ReasonCode, response.Error)
	}
	if response.Candidate.CycleID != cycleID || response.Candidate.CycleNumber != cycleNumber || response.Candidate.CandidateFile != strconv.FormatInt(cycleID, 10)+".result.json" || len(response.Candidate.ContractSHA) != 64 {
		return supervisorEvaluationCandidate{}, errors.New("RESULT_PROCESSOR_CANDIDATE_IDENTITY_INVALID")
	}
	return response.Candidate, nil
}

// An uncertain ACK is never resent here. The caller re-enters state lookup,
// reconnect fencing and Server authority recovery. Business/protocol errors
// and expired contexts are deliberately not retryable.
func retryableEvaluationACKDisconnect(ctx context.Context, err error) bool {
	return ctx.Err() == nil && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
}

func continueProcessorEvaluation(ctx context.Context, containerID string, taskID int64, attempt domain.Attempt, directive processorfacade.EvaluationDirective) error {
	response, err := callSupervisorContext(ctx, containerID, map[string]any{
		"schemaVersion": 2, "command": "continue-evaluation", "taskId": strconv.FormatInt(taskID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10),
		"generation": strconv.FormatInt(attempt.Generation, 10), "ownerEpoch": strconv.FormatInt(attempt.OwnerEpoch, 10), "cycleId": strconv.FormatInt(directive.CycleID, 10),
		"directive": directive.Directive, "decision": directive.Decision, "reasonCode": directive.ReasonCode,
		"message": directive.Message, "managedFeedback": directive.ManagedFeedback, "resumeNonce": directive.ResumeNonce,
	})
	if err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("%s: %s", response.ReasonCode, response.Error)
	}
	return nil
}

func readProcessorCandidate(workspace Workspace, item domain.WorkItem, attempt domain.Attempt, state supervisorStateSnapshot, response supervisorEvaluationCandidate, materialization processorfacade.Materialization) (processorfacade.EvaluationCandidate, error) {
	if response.CandidateFile != strconv.FormatInt(state.EvaluationCycleID, 10)+".result.json" || !insideEvaluationRuntime(workspace.Runtime, response.CandidateFile) {
		return processorfacade.EvaluationCandidate{}, errors.New("RESULT_PROCESSOR_CANDIDATE_PATH_INVALID")
	}
	path := filepath.Join(workspace.Runtime, "evaluations", response.CandidateFile)
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 || len(body) > processordomain.MaxContractBytes || !json.Valid(body) {
		return processorfacade.EvaluationCandidate{}, errors.New("RESULT_PROCESSOR_CANDIDATE_UNAVAILABLE")
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != response.ContractSHA && !isSafeProcessorRejection(body) {
		return processorfacade.EvaluationCandidate{}, errors.New("RESULT_PROCESSOR_CANDIDATE_DIGEST_MISMATCH")
	}
	// A processor receives cycle-scoped secret values only after the Agent has
	// stopped.  Its Result Contract, workspace and all later checkpoint/output
	// paths are therefore a hard redaction boundary.  Do this check before the
	// candidate ever crosses the Worker -> Server Machine API boundary.  An
	// explicit REJECT is safer than a manual-review checkpoint because a
	// checkpoint could preserve the leaked value for a later restore.
	exposed, scanErr := processorSecretExposure(body, workspace.Repo, materialization.RedactionHint)
	if scanErr != nil {
		// A scan error must not become an unchecked checkpoint or user output.
		// It is represented as a safe terminal contract, never by including the
		// scanner error (which may contain a filesystem path) in operator data.
		safeBody, _, err := writeSafeProcessorRejection(path, "PROCESSOR_SECRET_SCAN_UNAVAILABLE", "结果处理脚本的受保护值检查未完成，任务已安全停止。")
		if err != nil {
			return processorfacade.EvaluationCandidate{}, err
		}
		return evaluationCandidateFromBody(workspace, item, attempt, state, response, safeBody)
	}
	if exposed {
		safeBody, _, err := writeSafeProcessorRejection(path, "PROCESSOR_SECRET_EXPOSURE", "结果处理脚本输出了受保护的值，内容已被隔离。")
		if err != nil {
			return processorfacade.EvaluationCandidate{}, err
		}
		return evaluationCandidateFromBody(workspace, item, attempt, state, response, safeBody)
	}
	return evaluationCandidateFromBody(workspace, item, attempt, state, response, body)
}

func evaluationCandidateFromBody(workspace Workspace, item domain.WorkItem, attempt domain.Attempt, state supervisorStateSnapshot, response supervisorEvaluationCandidate, body []byte) (processorfacade.EvaluationCandidate, error) {
	contract, err := processordomain.ValidateContract(body)
	if err != nil {
		return processorfacade.EvaluationCandidate{}, fmt.Errorf("RESULT_PROCESSOR_CONTRACT_INVALID: %w", err)
	}
	contractDigest, err := processordomain.ContractDigest(contract)
	if err != nil {
		return processorfacade.EvaluationCandidate{}, err
	}
	treeDigest, err := candidateWorkspaceDigest(workspace.Repo)
	if err != nil {
		return processorfacade.EvaluationCandidate{}, err
	}
	findingsDigest, err := candidateFindingsDigest(body)
	if err != nil {
		return processorfacade.EvaluationCandidate{}, err
	}
	return processorfacade.EvaluationCandidate{
		CycleID: response.CycleID, CycleNumber: response.CycleNumber, SessionDigest: response.SessionDigest,
		CandidateTreeDigest: treeDigest, FindingsDigest: findingsDigest, Contract: body, ContractDigest: contractDigest,
		ProcessorReleaseID: item.ExecutionSnapshot.ResultProcessorReleaseID, ProcessorDigest: item.ExecutionSnapshot.ResultProcessorDigest,
		EvaluationStartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// processorSecretExposure detects exact materialized SECRET values without
// serializing them into an error, event or audit field.  The bounded walk also
// prevents a malicious repository from turning this safety check into an
// unbounded post-processing scan.  Symlinks are deliberately not followed.
func processorSecretExposure(contract []byte, workspaceRoot string, secretValues []string) (bool, error) {
	secrets := normalizedSecretValues(secretValues)
	if len(secrets) == 0 {
		return false, nil
	}
	if containsAnySecret(contract, secrets) {
		return true, nil
	}
	var total int64
	found := false
	err := filepath.WalkDir(workspaceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || found {
			return walkErr
		}
		relative, err := filepath.Rel(workspaceRoot, path)
		if err != nil || relative == "." {
			return err
		}
		if relative == ".git" || strings.HasPrefix(relative, ".git"+string(os.PathSeparator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > 64<<20 {
			// The later trusted tree digest has the same bounded ceiling.  Treat a
			// too-large candidate tree as not scannable; the tree digest will fail
			// closed without exposing a secret value.
			return errors.New("RESULT_PROCESSOR_TREE_TOO_LARGE")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if containsAnySecret(body, secrets) {
			found = true
			return errors.New("RESULT_PROCESSOR_SECRET_EXPOSURE")
		}
		return nil
	})
	if found {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

func normalizedSecretValues(values []string) [][]byte {
	seen := make(map[string]struct{}, len(values))
	result := make([][]byte, 0, len(values))
	for _, value := range values {
		if value == "" || len(value) > 64<<10 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, []byte(value))
	}
	sort.Slice(result, func(i, j int) bool { return len(result[i]) > len(result[j]) })
	return result
}

func containsAnySecret(body []byte, secrets [][]byte) bool {
	for _, secret := range secrets {
		if len(secret) != 0 && bytes.Contains(body, secret) {
			return true
		}
	}
	return false
}

func writeSafeProcessorRejection(path, reasonCode, message string) ([]byte, string, error) {
	body, err := json.Marshal(map[string]any{
		"decision": "REJECT", "reasonCode": reasonCode, "message": message,
	})
	if err != nil {
		return nil, "", err
	}
	if err := atomicPrivateWriteFile(path, body); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}

// isSafeProcessorRejection permits a reattached Worker to replay an already
// quarantined candidate after the first owner has atomically replaced the raw
// private file.  It can only make the Attempt fail; it can never convert an
// untrusted result into PASS/REVISE or bypass the Server's contract checks.
func isSafeProcessorRejection(body []byte) bool {
	contract, err := processordomain.ValidateContract(body)
	if err != nil || contract.Decision != "REJECT" {
		return false
	}
	return contract.ReasonCode == "PROCESSOR_SECRET_EXPOSURE" || contract.ReasonCode == "PROCESSOR_SECRET_SCAN_UNAVAILABLE"
}

func atomicPrivateWriteFile(path string, body []byte) error {
	if len(body) > processordomain.MaxContractBytes {
		return errors.New("RESULT_PROCESSOR_CANDIDATE_TOO_LARGE")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func insideEvaluationRuntime(runtimeRoot, filename string) bool {
	if filepath.Base(filename) != filename || !strings.HasSuffix(filename, ".result.json") {
		return false
	}
	return filepath.Clean(filepath.Join(runtimeRoot, "evaluations", filename)) != filepath.Clean(runtimeRoot)
}

func candidateFindingsDigest(contract []byte) (string, error) {
	var value struct {
		Findings json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal(contract, &value); err != nil {
		return "", err
	}
	if len(value.Findings) == 0 {
		value.Findings = json.RawMessage("[]")
	}
	var normalized any
	if err := json.Unmarshal(value.Findings, &normalized); err != nil {
		return "", err
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func candidateWorkspaceDigest(root string) (string, error) {
	entries := make([]string, 0, 256)
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." {
			return err
		}
		if relative == ".git" || strings.HasPrefix(relative, ".git"+string(os.PathSeparator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if len(entries) > 10_000 || total > 64<<20 {
			return errors.New("RESULT_PROCESSOR_TREE_TOO_LARGE")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		entries = append(entries, filepath.ToSlash(relative)+"\x00"+strconv.FormatInt(int64(info.Mode().Perm()), 8)+"\x00"+hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func dockerContainerRunning(ctx context.Context, containerID string) (bool, error) {
	text, err := commandText(ctx, "docker", "inspect", "--format", "{{.State.Running}}", containerID)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(text) == "true", nil
}

func callSupervisorContext(ctx context.Context, containerID string, request any) (supervisorResponse, error) {
	return callSupervisorContextWithTimeout(ctx, containerID, request, 5*time.Second)
}

func callSupervisorContextWithTimeout(ctx context.Context, containerID string, request any, timeout time.Duration) (supervisorResponse, error) {
	if session, ok := ctx.Value(executionSessionKey{}).(*ExecutionSession); ok {
		controlCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return session.Call(controlCtx, containerID, request)
	}
	var response supervisorResponse
	body, err := json.Marshal(request)
	if err != nil {
		return response, err
	}
	controlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(controlCtx, "docker", "exec", "-i", containerID, "/usr/local/bin/seven-worker-cli", "control")
	command.Stdin = strings.NewReader(string(body))
	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return response, fmt.Errorf("Supervisor control exec: %w: %s", err, boundedText(exitErr.Stderr, 4096))
		}
		return response, err
	}
	decoder := json.NewDecoder(io.LimitReader(strings.NewReader(string(output)), 64<<10))
	if err := decoder.Decode(&response); err != nil {
		return response, err
	}
	return response, nil
}

func boundedText(value []byte, limit int) string {
	if len(value) > limit {
		value = value[:limit]
	}
	return strings.TrimSpace(string(value))
}

func (d *Docker) startProviderProxy(ctx context.Context, item domain.WorkItem, attempt domain.Attempt, provider ProviderRuntimeIdentity) (providerProxyRuntime, error) {
	if strings.TrimSpace(d.providerProxy.Image) == "" {
		return providerProxyRuntime{}, errors.New("provider proxy image is not configured")
	}
	if provider.ExpectedProxyDigest == "" || provider.ExpectedProxyDigest != d.providerProxy.ImageDigest || provider.ExpectedProxyVersion == "" || provider.ExpectedProxyVersion != d.providerProxy.Version {
		return providerProxyRuntime{}, errors.New("Provider Proxy immutable identity does not match TaskExecutionSnapshot")
	}
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 10*time.Second)
	resolvedDigest, inspectErr := d.commandText(inspectCtx, "docker", "image", "inspect", "--format", "{{.Id}}", d.providerProxy.Image)
	inspectCancel()
	if inspectErr != nil || resolvedDigest != provider.ExpectedProxyDigest {
		return providerProxyRuntime{}, errors.New("configured Provider Proxy image does not resolve to the frozen digest")
	}
	controlDirectory := providerProxyControlDirectory(d.providerProxy.ControlRoot, item.TaskID, attempt.ID)
	if err := os.MkdirAll(controlDirectory, 0o700); err != nil {
		return providerProxyRuntime{}, fmt.Errorf("create provider proxy control directory: %w", err)
	}
	var directoryRecord resourcejournal.Resource
	if d.cleanupJournal != nil {
		var err error
		directoryRecord, err = d.cleanupJournal.TrackDirectory(controlDirectory, map[string]string{"seven.task.id": strconv.FormatInt(item.TaskID, 10), "seven.attempt.id": strconv.FormatInt(attempt.ID, 10)})
		if err != nil {
			return providerProxyRuntime{}, err
		}
	}
	cleanupFiles := func() {
		if d.cleanupJournal != nil {
			_ = d.cleanupJournal.Cleanup(directoryRecord.Key, nil)
			return
		}
		_ = os.RemoveAll(controlDirectory)
	}
	specPath := filepath.Join(controlDirectory, "provider.json")
	gatewayBaseURL, err := gatewayURLForContainer(d.cfg.ServerBaseURL)
	if err != nil {
		cleanupFiles()
		return providerProxyRuntime{}, err
	}
	deadline := provider.ExecutionDeadline
	spec := providerproxy.Config{
		GatewayBaseURL: gatewayBaseURL, WorkerTokenFile: "/run/seven/worker.token",
		WorkerID: d.cfg.ID, WorkerInstanceID: attempt.WorkerInstance, HostBootID: attempt.HostBootID,
		TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch,
		AttemptLeaseToken: attempt.LeaseToken, ModelCredentialID: provider.ModelCredentialID,
		CredentialVersion: provider.CredentialVersion, ModelProtocol: provider.Protocol, PrimaryModel: provider.PrimaryModel,
		ExecutionDeadline: deadline, ControlSocket: "/run/seven/control/provider.sock",
		MaxRequestBytes: d.providerSSRF.MaxRequestBytes,
	}
	body, err := json.Marshal(spec)
	if err != nil {
		cleanupFiles()
		return providerProxyRuntime{}, err
	}
	// The parent directory is host-only 0700. The bind-mounted file is 0444 so
	// the image's fixed non-root user can read it without running the proxy as
	// the host Worker UID; the Agent container never receives this mount.
	if err := os.WriteFile(specPath, body, 0o444); err != nil {
		cleanupFiles()
		return providerProxyRuntime{}, fmt.Errorf("write provider proxy specification: %w", err)
	}
	socketDirectory := filepath.Join(controlDirectory, "socket")
	// Docker Desktop does not support chmod on a Unix socket inside a bind
	// mount. The owner-only parent remains the mandatory access boundary there;
	// Linux additionally enforces the socket's own 0600 mode in the proxy.
	if err := os.MkdirAll(socketDirectory, 0o700); err != nil {
		cleanupFiles()
		return providerProxyRuntime{}, fmt.Errorf("create provider proxy socket directory: %w", err)
	}
	if os.Getuid() == 0 {
		cleanupFiles()
		return providerProxyRuntime{}, errors.New("seven-worker must run as a non-root host user for Provider Proxy file isolation")
	}
	attemptIdentity := strconv.FormatInt(attempt.ID, 10)
	networkName := sanitizeName("seven-provider-" + attemptIdentity)
	proxyName := sanitizeName("seven-provider-proxy-" + attemptIdentity)
	cleanup := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), resourcejournal.Timeout)
		defer cleanupCancel()
		_, containerErr := d.commandText(cleanupCtx, "docker", "rm", "-f", proxyName)
		_, networkErr := d.commandText(cleanupCtx, "docker", "network", "rm", networkName)
		if d.cleanupJournal != nil || (containerErr == nil && networkErr == nil) {
			cleanupFiles()
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := d.commandText(commandCtx, "docker", "network", "create", "--internal", "--label", "seven.managed=true", "--label", "seven.worker.id="+strconv.FormatInt(d.cfg.ID, 10), "--label", "seven.task.id="+strconv.FormatInt(item.TaskID, 10), "--label", "seven.attempt.id="+attemptIdentity, networkName); err != nil {
		cleanupFiles()
		return providerProxyRuntime{}, fmt.Errorf("create provider private network: %w", err)
	}
	identityBody, _ := json.Marshal([]any{d.cfg.ID, attempt.WorkerInstance, attempt.HostBootID, item.TaskID, attempt.ID, attempt.Generation, attempt.OwnerEpoch, provider.ModelCredentialID, provider.CredentialVersion, provider.ExpectedProxyDigest})
	identitySum := sha256.Sum256(identityBody)
	proxyIdentity := hex.EncodeToString(identitySum[:])
	proxyID, err := d.commandText(commandCtx, "docker", "create", "--name", proxyName,
		"--network", "bridge", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "64", "--memory", "128m", "--cpus", "0.5", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--add-host", "host.docker.internal:host-gateway",
		"--label", "seven.managed=true", "--label", "seven.task.id="+strconv.FormatInt(item.TaskID, 10), "--label", "seven.attempt.id="+attemptIdentity,
		"--label", "seven.attempt.generation="+strconv.FormatInt(attempt.Generation, 10),
		"--label", "seven.owner.epoch="+strconv.FormatInt(attempt.OwnerEpoch, 10),
		"--label", "seven.provider.proxy.image.digest="+provider.ExpectedProxyDigest,
		"--label", "seven.provider.proxy.identity.digest="+proxyIdentity,
		"--label", "seven.worker.id="+strconv.FormatInt(d.cfg.ID, 10),
		"--label", "seven.worker.instance="+attempt.WorkerInstance,
		"--label", "seven.worker.host-boot-id="+attempt.HostBootID,
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/run/seven/provider.json,readonly", specPath),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/run/seven/worker.token,readonly", d.cfg.TokenFile),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/run/seven/control", socketDirectory),
		d.providerProxy.Image, "--spec", "/run/seven/provider.json")
	if err != nil {
		cleanup()
		return providerProxyRuntime{}, fmt.Errorf("create provider proxy container: %w", err)
	}
	if _, err := d.commandText(commandCtx, "docker", "network", "connect", "--alias", "seven-provider-proxy", networkName, proxyID); err != nil {
		cleanup()
		return providerProxyRuntime{}, fmt.Errorf("connect provider proxy private network: %w", err)
	}
	if _, err := d.commandText(commandCtx, "docker", "start", proxyID); err != nil {
		cleanup()
		return providerProxyRuntime{}, fmt.Errorf("start provider proxy container: %w", err)
	}
	controlSocket := filepath.Join(socketDirectory, "provider.sock")
	if err := d.waitProviderProxy(commandCtx, controlSocket); err != nil {
		cleanup()
		return providerProxyRuntime{}, fmt.Errorf("wait provider proxy control socket: %w", err)
	}
	return providerProxyRuntime{NetworkName: networkName, ContainerID: proxyID, ContainerName: proxyName, ImageDigest: provider.ExpectedProxyDigest, ExecutionIdentityDigest: proxyIdentity, ControlSocket: controlSocket, cleanup: cleanup}, nil
}

func gatewayURLForContainer(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("Framework Gateway base URL is invalid")
	}
	hostname := parsed.Hostname()
	if hostname == "127.0.0.1" || hostname == "localhost" || hostname == "::1" {
		port := parsed.Port()
		parsed.Host = "host.docker.internal"
		if port != "" {
			parsed.Host += ":" + port
		}
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// providerProxyControlDirectory keeps the host-visible Unix-domain socket path
// below the shortest commonly deployed AF_UNIX limit (macOS is approximately
// 104 bytes). Full Task and Attempt identity remains in the immutable spec,
// Docker labels, and every control request; this path is only a deterministic
// local locator and is not an authorization boundary.
func providerProxyControlDirectory(root string, taskID int64, attemptID int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d", taskID, attemptID)))
	return filepath.Join(root, "p-"+hex.EncodeToString(sum[:12]))
}

func providerProxyControlSocket(root string, taskID int64, attemptID int64) string {
	return filepath.Join(providerProxyControlDirectory(root, taskID, attemptID), "socket", "provider.sock")
}

func (d *Docker) Validate(ctx context.Context, image, repo, command string) (int, string, error) {
	validationCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(validationCtx, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--cpus", "1.0", "--entrypoint", "/bin/sh", "--mount", fmt.Sprintf("type=bind,src=%s,dst=/workspace/repo,readonly", repo), "-w", "/workspace/repo", image, "-lc", command)
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return 1, bounded(output), err
		}
	}
	return exitCode, bounded(output), nil
}

func scanLogs(reader io.Reader, stream string, sink EventSink, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(io.LimitReader(reader, 16<<20))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var event map[string]any
		if json.Unmarshal(line, &event) == nil {
			eventType, _ := event["type"].(string)
			if eventType == "" {
				eventType = "harness.event"
			}
			sink(eventType, event)
		} else {
			sink("harness.log", map[string]any{"stream": stream, "line": string(line)})
		}
	}
	if err := scanner.Err(); err != nil {
		sink("harness.log_error", map[string]any{"stream": stream, "error": err.Error()})
	}
}

func git(ctx context.Context, repo string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	command.Env = safeGitEnvironment()
	return command.CombinedOutput()
}

func safeGitEnvironment(extra ...string) []string {
	environment := make([]string, 0, len(os.Environ())+len(extra)+4)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") && !strings.HasPrefix(value, "SSH_ASKPASS=") {
			environment = append(environment, value)
		}
	}
	environment = append(environment,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/bin/false", "SSH_ASKPASS=/bin/false",
	)
	return append(environment, extra...)
}

func gitText(ctx context.Context, repo string, args ...string) (string, error) {
	output, err := git(ctx, repo, args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, bounded(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func commandText(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", name, err, bounded(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func bounded(value []byte) string {
	if len(value) > 8192 {
		value = value[:8192]
	}
	return strings.TrimSpace(string(value))
}

func sanitizeName(value string) string {
	value = strings.ToLower(value)
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, value)
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}

func sortedLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
