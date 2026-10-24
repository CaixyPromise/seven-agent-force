package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	executionaccessfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	artifactdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/domain"
	artifactinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/infrastructure"
	continuationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	continuationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	deliveryfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	machineidem "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/machineidempotency"
	processorapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/application"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	processorinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
	reviewrunfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	dispatchapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	dispatchinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/infrastructure"
	registryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/application"
	workspaceinputapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
)

type MachineHandler struct {
	interactionSuspend  *continuationapp.InteractionSuspendService
	interactions        interactionfacade.MachineControl
	executionAccess     executionaccessfacade.MachineService
	completionChecks    completioncheck.MachineControl
	completionRevisions completioncheck.RevisionMachineControl
	dispatch            *dispatchapp.Service
	store               *dispatchinfra.Store
	tasks               *taskinfra.Store
	registry            *registryapp.Service
	postprocess         *artifactinfra.Store
	processors          *processorapp.Service
	continuation        *continuationapp.Service
	delivery            *deliveryapp.MachineService
	deliveryTools       deliveryfacade.MachineToolService
	idempotency         *machineidem.Middleware
	workspaceInput      *workspaceinputapp.Service
	workspaceRetention  taskfacade.WorkspaceRetentionAuthority
	reviewRuns          reviewrunfacade.MachineControl
	idGen               IDGenerator
}

type IDGenerator interface{ NextID() int64 }

func (h *MachineHandler) SetWorkspaceInput(service *workspaceinputapp.Service) {
	h.workspaceInput = service
}

func (h *MachineHandler) SetExecutionAccess(service executionaccessfacade.MachineService) {
	if h != nil {
		h.executionAccess = service
	}
}

func (h *MachineHandler) SetDeliveryTools(service deliveryfacade.MachineToolService) {
	if h != nil {
		h.deliveryTools = service
	}
}

// SetWorkspaceRetention binds the Task facade rather than exposing Worker
// storage to the controller. The Worker receives only a safe eligibility fact.
func (h *MachineHandler) SetWorkspaceRetention(authority taskfacade.WorkspaceRetentionAuthority) {
	h.workspaceRetention = authority
}

// SetReviewRunControl binds the fenced Slice 4 machine surface. These routes
// remain Worker-machine-only; normal Task users cannot inspect or submit
// leases, permits, containers, raw reports, or Provider diagnostics.
func (h *MachineHandler) SetReviewRunControl(control reviewrunfacade.MachineControl) {
	if h != nil {
		h.reviewRuns = control
	}
}

func NewMachine(dispatch *dispatchapp.Service, store *dispatchinfra.Store, tasks *taskinfra.Store, registry *registryapp.Service, postprocess *artifactinfra.Store, processors *processorapp.Service, continuation *continuationapp.Service, delivery *deliveryapp.MachineService, idempotency *machineidem.Middleware, idGen IDGenerator) *MachineHandler {
	return &MachineHandler{dispatch: dispatch, store: store, tasks: tasks, registry: registry, postprocess: postprocess, processors: processors, continuation: continuation, delivery: delivery, idempotency: idempotency, idGen: idGen}
}
func (h *MachineHandler) Mount(r route.IRouter) {
	h.mountInteractions(r)
	h.mountExecutionAccess(r)
	h.mountInteractionSuspend(r)
	r.POST("/internal/workforce/v1/attempts/:attemptId/completion-check-revisions/apply", h.applyCompletionRevision)
	wrap := func(route string, ttl time.Duration, handler app.HandlerFunc) app.HandlerFunc {
		return h.idempotency.Wrap(route, ttl, handler)
	}
	r.POST("/internal/workforce/v1/assignments/:assignmentId/claim", wrap("/assignments/{assignmentId}/claim", 2*time.Minute, h.claim))
	r.POST("/internal/workforce/v1/review-runs/:runId/claims", wrap("/review-runs/{runId}/claims", 2*time.Minute, h.claimReviewRun))
	r.POST("/internal/workforce/v1/review-runs/:runId/recover", wrap("/review-runs/{runId}/recover", 2*time.Minute, h.recoverReviewRun))
	r.PUT("/internal/workforce/v1/review-runs/:runId/lease", wrap("/review-runs/{runId}/lease", 2*time.Minute, h.renewReviewLease))
	r.POST("/internal/workforce/v1/review-runs/:runId/launch-permits", wrap("/review-runs/{runId}/launch-permits", 2*time.Minute, h.reviewLaunchPermit))
	r.POST("/internal/workforce/v1/review-runs/:runId/containers", wrap("/review-runs/{runId}/containers", 24*time.Hour, h.reviewContainerCreated))
	r.POST("/internal/workforce/v1/review-runs/:runId/reports", wrap("/review-runs/{runId}/reports", 24*time.Hour, h.submitReviewReport))
	r.POST("/internal/workforce/v1/review-runs/:runId/failures", wrap("/review-runs/{runId}/failures", 24*time.Hour, h.failReviewRun))
	r.POST("/internal/workforce/v1/tasks/:taskId/workspace-gc-eligibility", h.workspaceGCEligibility)
	r.POST("/internal/workforce/v1/assignments/:assignmentId/release", wrap("/assignments/{assignmentId}/release", 24*time.Hour, h.releaseAssignment))
	r.POST("/internal/workforce/v1/attempts/:attemptId/launch-permits", wrap("/attempts/{attemptId}/launch-permits", 2*time.Minute, h.launch))
	r.POST("/internal/workforce/v1/attempts/:attemptId/containers", wrap("/attempts/{attemptId}/containers", 24*time.Hour, h.containerCreated))
	r.POST("/internal/workforce/v1/attempts/:attemptId/provider-proxy-ready", wrap("/attempts/{attemptId}/provider-proxy-ready", 24*time.Hour, h.providerProxyReady))
	r.POST("/internal/workforce/v1/attempts/:attemptId/supervisor-ready", wrap("/attempts/{attemptId}/supervisor-ready", 24*time.Hour, h.supervisorReady))
	r.POST("/internal/workforce/v1/attempts/:attemptId/run-permits", wrap("/attempts/{attemptId}/run-permits", time.Minute, h.runPermit))
	r.POST("/internal/workforce/v1/attempts/:attemptId/harness-started", wrap("/attempts/{attemptId}/harness-started", 24*time.Hour, h.harnessStarted))
	r.PUT("/internal/workforce/v1/attempts/:attemptId/lease", wrap("/attempts/{attemptId}/lease", 2*time.Minute, h.renewLease))
	r.POST("/internal/workforce/v1/attempts/:attemptId/reattach-permits", wrap("/attempts/{attemptId}/reattach-permits", 3*time.Minute, h.reattachPermit))
	r.POST("/internal/workforce/v1/attempts/:attemptId/reattach-acks", wrap("/attempts/{attemptId}/reattach-acks", 24*time.Hour, h.reattachAck))
	r.POST("/internal/workforce/v1/attempts/:attemptId/runtime-losses", wrap("/attempts/{attemptId}/runtime-losses", 24*time.Hour, h.runtimeLoss))
	r.GET("/internal/workforce/v1/attempts/:attemptId/control", h.control)
	r.POST("/internal/workforce/v1/attempts/:attemptId/events/batch", wrap("/attempts/{attemptId}/events/batch", 24*time.Hour, h.facts))
	r.POST("/internal/workforce/v1/attempts/:attemptId/journal-facts/batch", wrap("/attempts/{attemptId}/journal-facts/batch", 24*time.Hour, h.journalFacts))
	r.POST("/internal/workforce/v1/attempts/:attemptId/completion-proposals", wrap("/attempts/{attemptId}/completion-proposals", 24*time.Hour, h.completion))
	r.POST("/internal/workforce/v1/attempts/:attemptId/checks/batch", wrap("/attempts/{attemptId}/checks/batch", 24*time.Hour, h.checks))
	r.POST("/internal/workforce/v1/attempts/:attemptId/processor-materializations", wrap("/attempts/{attemptId}/processor-materializations", 2*time.Minute, h.processorMaterialization))
	r.POST("/internal/workforce/v1/attempts/:attemptId/result-evaluations", wrap("/attempts/{attemptId}/result-evaluations", 24*time.Hour, h.resultEvaluation))
	// Never replay a cached response here: every read checks current authority.
	r.POST("/internal/workforce/v1/attempts/:attemptId/result-evaluations/read", h.readEvaluation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/completion-check-results", wrap("/attempts/{attemptId}/completion-check-results", 24*time.Hour, h.recordCompletionCheck))
	r.POST("/internal/workforce/v1/attempts/:attemptId/completion-check-results/read", h.readCompletionCheck)
	r.POST("/internal/workforce/v1/attempts/:attemptId/continuation-checkpoints", wrap("/attempts/{attemptId}/continuation-checkpoints", 24*time.Hour, h.continuationCheckpoint))
	r.POST("/internal/workforce/v1/attempts/:attemptId/continuation-checkpoint-keys", wrap("/attempts/{attemptId}/continuation-checkpoint-keys", 24*time.Hour, h.continuationCheckpointKey))
	r.POST("/internal/workforce/v1/continuation-restores/:restoreRunId/key", wrap("/continuation-restores/{restoreRunId}/key", 24*time.Hour, h.continuationRestoreKey))
	r.POST("/internal/workforce/v1/continuation-restores/:restoreRunId/results", wrap("/continuation-restores/{restoreRunId}/results", 24*time.Hour, h.continuationRestoreResult))
	r.POST("/internal/workforce/v1/result-materialization-runs/:runId/claims", wrap("/result-materialization-runs/{runId}/claims", 24*time.Hour, h.claimMaterialization))
	r.POST("/internal/workforce/v1/result-materialization-runs/:runId/freeze", wrap("/result-materialization-runs/{runId}/freeze", 24*time.Hour, h.freezeMaterialization))
	r.POST("/internal/workforce/v1/result-materialization-runs/:runId/failures", wrap("/result-materialization-runs/{runId}/failures", 24*time.Hour, h.failMaterialization))
	r.POST("/internal/workforce/v1/result-processor-fixture-runs/:runId/claims", wrap("/result-processor-fixture-runs/{runId}/claims", 24*time.Hour, h.fixtureClaim))
	r.POST("/internal/workforce/v1/result-processor-fixture-runs/:runId/results", wrap("/result-processor-fixture-runs/{runId}/results", 24*time.Hour, h.fixtureResult))
	r.GET("/internal/workforce/v1/attempts/:attemptId/evidence", h.evidence)
	r.POST("/internal/workforce/v1/attempts/:attemptId/artifacts/batch", wrap("/attempts/{attemptId}/artifacts/batch", 24*time.Hour, h.artifacts))
	r.POST("/internal/workforce/v1/attempts/:attemptId/outcome", wrap("/attempts/{attemptId}/outcome", 24*time.Hour, h.outcome))
	r.POST("/internal/workforce/v1/finalization-runs/:runId/claims", wrap("/finalization-runs/{runId}/claims", 24*time.Hour, h.claimFinalization))
	r.POST("/internal/workforce/v1/finalization-runs/:runId/freeze", wrap("/finalization-runs/{runId}/freeze", 24*time.Hour, h.freezeFinalization))
	r.POST("/internal/workforce/v1/finalization-runs/:runId/failures", wrap("/finalization-runs/{runId}/failures", 24*time.Hour, h.failFinalization))
	r.POST("/internal/workforce/v1/artifact-upload-attempts/:uploadId/claims", wrap("/artifact-upload-attempts/{uploadId}/claims", 24*time.Hour, h.claimUpload))
	r.POST("/internal/workforce/v1/artifact-upload-attempts/:uploadId/verifications", wrap("/artifact-upload-attempts/{uploadId}/verifications", 24*time.Hour, h.verifyUpload))
	r.POST("/internal/workforce/v1/artifact-upload-attempts/:uploadId/failures", wrap("/artifact-upload-attempts/{uploadId}/failures", 24*time.Hour, h.failUpload))
	r.POST("/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/claims", wrap("/delivery-attempts/{deliveryAttemptId}/claims", 24*time.Hour, h.claimDelivery))
	r.POST("/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/credential-materializations", wrap("/delivery-attempts/{deliveryAttemptId}/credential-materializations", 2*time.Minute, h.deliveryCredential))
	r.POST("/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/observations", wrap("/delivery-attempts/{deliveryAttemptId}/observations", 2*time.Minute, h.prepareDelivery))
	r.POST("/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/results", wrap("/delivery-attempts/{deliveryAttemptId}/results", 24*time.Hour, h.resultDelivery))
	r.POST("/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/reconciliations", wrap("/delivery-attempts/{deliveryAttemptId}/reconciliations", 24*time.Hour, h.reconcileDelivery))
	r.POST("/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/failures", wrap("/delivery-attempts/{deliveryAttemptId}/failures", 24*time.Hour, h.failDelivery))
	r.POST("/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/claims", wrap("/source-ingestion-runs/{sourceIngestionRunId}/claims", 24*time.Hour, h.claimSourceIngestion))
	r.POST("/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/credential-materializations", wrap("/source-ingestion-runs/{sourceIngestionRunId}/credential-materializations", 2*time.Minute, h.sourceIngestionCredential))
	r.POST("/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/observations", wrap("/source-ingestion-runs/{sourceIngestionRunId}/observations", 24*time.Hour, h.observeSourceIngestion))
	r.POST("/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/results", wrap("/source-ingestion-runs/{sourceIngestionRunId}/results", 24*time.Hour, h.resultSourceIngestion))
	r.POST("/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/failures", wrap("/source-ingestion-runs/{sourceIngestionRunId}/failures", 24*time.Hour, h.failSourceIngestion))
}

type workspaceGCEligibilityRequest struct {
	domain.MachineEnvelope
}

// workspaceGCEligibility is intentionally read-only. The Worker uses the
// Server's durable state to decide whether it may remove a same-host cache;
// it cannot manufacture an eligible state from local files.
func (h *MachineHandler) workspaceGCEligibility(ctx context.Context, c *app.RequestContext) {
	var req workspaceGCEligibilityRequest
	if !decode(c, &req, 8<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	taskID := pathID(c, "taskId")
	if taskID < 1 {
		writeError(c, http.StatusBadRequest, "WORKSPACE_RETENTION_TASK_ID_INVALID", "task identity is invalid")
		return
	}
	if h.workspaceRetention == nil {
		writeError(c, http.StatusInternalServerError, "WORKSPACE_RETENTION_AUTHORITY_UNAVAILABLE", "workspace retention authority is unavailable")
		return
	}
	eligibility, err := h.workspaceRetention.WorkspaceGCEligibility(ctx, taskID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "WORKSPACE_RETENTION_AUTHORITY_UNAVAILABLE", "workspace retention authority is unavailable")
		return
	}
	if eligibility.TaskID != taskID {
		writeError(c, http.StatusInternalServerError, "WORKSPACE_RETENTION_IDENTITY_CONFLICT", "workspace retention identity is invalid")
		return
	}
	response.Success(c, eligibility)
}

func sourceAuthority(e domain.MachineEnvelope) workspaceinputapp.MachineAuthority {
	return workspaceinputapp.MachineAuthority{WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID, Generation: e.Generation}
}
func (h *MachineHandler) claimSourceIngestion(ctx context.Context, c *app.RequestContext) {
	var req postprocessClaimRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.workspaceInput == nil {
		return
	}
	claim, err := h.workspaceInput.ClaimIngestion(ctx, sourceAuthority(req.MachineEnvelope), pathID(c, "sourceIngestionRunId"))
	if err != nil {
		writeSourceIngestionError(c, err)
		return
	}
	response.Success(c, claim)
}

type sourceIngestionLeaseRequest struct {
	domain.MachineEnvelope
	LeaseToken string `json:"leaseToken"`
}

func (h *MachineHandler) sourceIngestionCredential(ctx context.Context, c *app.RequestContext) {
	var req sourceIngestionLeaseRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.workspaceInput == nil {
		return
	}
	credential, err := h.workspaceInput.MaterializeCredential(ctx, sourceAuthority(req.MachineEnvelope), pathID(c, "sourceIngestionRunId"), req.LeaseToken)
	if err != nil {
		writeSourceIngestionError(c, err)
		return
	}
	response.Success(c, credential)
}

type sourceIngestionObservationRequest struct {
	domain.MachineEnvelope
	LeaseToken     string `json:"leaseToken"`
	ResolvedCommit string `json:"resolvedCommit"`
}

func (h *MachineHandler) observeSourceIngestion(ctx context.Context, c *app.RequestContext) {
	var req sourceIngestionObservationRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.workspaceInput == nil {
		return
	}
	if err := h.workspaceInput.FreezeSourceCommit(ctx, sourceAuthority(req.MachineEnvelope), pathID(c, "sourceIngestionRunId"), req.LeaseToken, req.ResolvedCommit); err != nil {
		writeSourceIngestionError(c, err)
		return
	}
	response.Success(c, map[string]any{"resolvedCommit": req.ResolvedCommit})
}

type sourceIngestionResultRequest struct {
	domain.MachineEnvelope
	LeaseToken string                                  `json:"leaseToken"`
	Result     workspaceinputapp.SourceIngestionResult `json:"result"`
}

func (h *MachineHandler) resultSourceIngestion(ctx context.Context, c *app.RequestContext) {
	var req sourceIngestionResultRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.workspaceInput == nil {
		return
	}
	result, err := h.workspaceInput.CompleteIngestion(ctx, sourceAuthority(req.MachineEnvelope), pathID(c, "sourceIngestionRunId"), req.LeaseToken, req.Result)
	if err != nil {
		writeSourceIngestionError(c, err)
		return
	}
	response.Success(c, result)
}

type sourceIngestionFailureRequest struct {
	domain.MachineEnvelope
	LeaseToken string `json:"leaseToken"`
	ReasonCode string `json:"reasonCode"`
}

func (h *MachineHandler) failSourceIngestion(ctx context.Context, c *app.RequestContext) {
	var req sourceIngestionFailureRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.workspaceInput == nil {
		return
	}
	if err := h.workspaceInput.FailIngestion(ctx, sourceAuthority(req.MachineEnvelope), pathID(c, "sourceIngestionRunId"), req.LeaseToken, req.ReasonCode); err != nil {
		writeSourceIngestionError(c, err)
		return
	}
	response.Success(c, map[string]any{"recorded": true})
}
func writeSourceIngestionError(c *app.RequestContext, err error) {
	if errors.Is(err, workspaceinputapp.ErrNotFound) {
		writeError(c, 404, "SOURCE_INGESTION_NOT_FOUND", "source ingestion work was not found")
		return
	}
	code := strings.SplitN(strings.TrimSpace(err.Error()), ":", 2)[0]
	if code == "" || len(code) > 128 {
		code = "SOURCE_INGESTION_IDENTITY_CONFLICT"
	}
	writeError(c, 409, code, "source ingestion request was rejected")
}

func deliveryAuthority(e domain.MachineEnvelope) deliveryapp.MachineAuthority {
	return deliveryapp.MachineAuthority{WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID, Generation: e.Generation}
}
func (h *MachineHandler) claimDelivery(ctx context.Context, c *app.RequestContext) {
	var req postprocessClaimRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.delivery == nil {
		return
	}
	claim, err := h.delivery.Claim(ctx, deliveryAuthority(req.MachineEnvelope), pathID(c, "deliveryAttemptId"))
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	response.Success(c, claim)
}

type deliveryCredentialRequest struct {
	domain.MachineEnvelope
	LeaseToken string `json:"leaseToken"`
}

func (h *MachineHandler) deliveryCredential(ctx context.Context, c *app.RequestContext) {
	var req deliveryCredentialRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.delivery == nil {
		return
	}
	value, err := h.delivery.MaterializeCredential(ctx, deliveryAuthority(req.MachineEnvelope), pathID(c, "deliveryAttemptId"), req.LeaseToken)
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	response.Success(c, value)
}

type deliveryPrepareRequest struct {
	domain.MachineEnvelope
	LeaseToken     string `json:"leaseToken"`
	DeliveryCommit string `json:"deliveryCommit"`
}

func (h *MachineHandler) prepareDelivery(ctx context.Context, c *app.RequestContext) {
	var req deliveryPrepareRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.delivery == nil {
		return
	}
	result, err := h.delivery.PreparePush(ctx, deliveryAuthority(req.MachineEnvelope), pathID(c, "deliveryAttemptId"), req.LeaseToken, req.DeliveryCommit)
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	response.Success(c, result)
}

type deliveryResultRequest struct {
	domain.MachineEnvelope
	LeaseToken        string                     `json:"leaseToken"`
	PushAuthorization string                     `json:"pushAuthorization"`
	Result            deliveryapp.DeliveryResult `json:"result"`
}

func (h *MachineHandler) resultDelivery(ctx context.Context, c *app.RequestContext) {
	var req deliveryResultRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.delivery == nil {
		return
	}
	if err := h.delivery.RecordResult(ctx, deliveryAuthority(req.MachineEnvelope), pathID(c, "deliveryAttemptId"), req.LeaseToken, req.PushAuthorization, req.Result); err != nil {
		writeDeliveryError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

type deliveryReconciliationRequest struct {
	domain.MachineEnvelope
	LeaseToken string                     `json:"leaseToken"`
	Result     deliveryapp.DeliveryResult `json:"result"`
}

func (h *MachineHandler) reconcileDelivery(ctx context.Context, c *app.RequestContext) {
	var req deliveryReconciliationRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.delivery == nil {
		return
	}
	if err := h.delivery.RecordReconciliation(ctx, deliveryAuthority(req.MachineEnvelope), pathID(c, "deliveryAttemptId"), req.LeaseToken, req.Result); err != nil {
		writeDeliveryError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

type deliveryFailureRequest struct {
	domain.MachineEnvelope
	LeaseToken string `json:"leaseToken"`
	ReasonCode string `json:"reasonCode"`
}

func (h *MachineHandler) failDelivery(ctx context.Context, c *app.RequestContext) {
	var req deliveryFailureRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.delivery == nil {
		return
	}
	if err := h.delivery.RecordFailure(ctx, deliveryAuthority(req.MachineEnvelope), pathID(c, "deliveryAttemptId"), req.LeaseToken, req.ReasonCode); err != nil {
		writeDeliveryError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}
func writeDeliveryError(c *app.RequestContext, err error) {
	status, reason := deliveryErrorStatus(err)
	message := "delivery operation failed"
	if status == http.StatusNotFound {
		message = "delivery resource was not found"
	} else if status == http.StatusConflict {
		message = "delivery identity or state is invalid"
	}
	writeError(c, status, reason, message)
}

func deliveryErrorStatus(err error) (int, string) {
	if errors.Is(err, deliveryapp.ErrNotFound) {
		return http.StatusNotFound, deliveryapp.ErrNotFound.Error()
	}
	for _, known := range []error{
		deliverydomain.ErrInvalidTarget,
		deliverydomain.ErrPolicyDenied,
		deliverydomain.ErrApprovalRequired,
		deliverydomain.ErrIdentityConflict,
		deliverydomain.ErrRemoteChanged,
		deliverydomain.ErrBaseChanged,
		deliverydomain.ErrBranchCreateForbidden,
	} {
		if errors.Is(err, known) {
			return http.StatusConflict, known.Error()
		}
	}
	code := strings.TrimSpace(err.Error())
	switch code {
	case "CONTINUATION_CHECKPOINT_FORMAT_UNSUPPORTED",
		"DELIVERY_READBACK_REQUIRED",
		"DELIVERY_ATTEMPT_NOT_RUNNING",
		"DELIVERY_LEASE_INVALID",
		"DELIVERY_PUSH_AUTHORIZATION_INVALID",
		"DELIVERY_OWNER_STALE",
		"DELIVERY_GENERATION_STALE":
		return http.StatusConflict, code
	default:
		// SQL, object-store, encryption and other infrastructure failures are
		// retryable server failures. Do not expose their raw text as a reasonCode
		// and do not let the Worker ACK them as business conflicts.
		return http.StatusInternalServerError, "DELIVERY_PERSIST_FAILED"
	}
}

func (h *MachineHandler) claimMaterialization(ctx context.Context, c *app.RequestContext) {
	var req postprocessClaimRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.continuation == nil {
		return
	}
	claim, err := h.continuation.ClaimMaterialization(ctx, continuationapp.MachineAuthority{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, Generation: req.Generation}, pathID(c, "runId"))
	if err != nil {
		writeContinuationError(c, err)
		return
	}
	response.Success(c, claim)
}

type materializationFreezeRequest struct {
	domain.MachineEnvelope
	LeaseToken string                                   `json:"leaseToken"`
	Result     continuationdomain.MaterializationFreeze `json:"result"`
}

func (h *MachineHandler) freezeMaterialization(ctx context.Context, c *app.RequestContext) {
	var req materializationFreezeRequest
	if !decode(c, &req, 1<<20) || !h.auth(ctx, c, req.MachineEnvelope) || h.continuation == nil {
		return
	}
	err := h.continuation.FreezeMaterialization(ctx, continuationapp.MachineAuthority{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, Generation: req.Generation}, pathID(c, "runId"), req.LeaseToken, req.Result)
	if err != nil {
		writeContinuationError(c, err)
		return
	}
	response.Success(c, map[string]any{"frozen": true})
}

type materializationFailureRequest struct {
	domain.MachineEnvelope
	LeaseToken string `json:"leaseToken"`
	ReasonCode string `json:"reasonCode"`
}

func (h *MachineHandler) failMaterialization(ctx context.Context, c *app.RequestContext) {
	var req materializationFailureRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.continuation == nil {
		return
	}
	err := h.continuation.FailMaterialization(ctx, continuationapp.MachineAuthority{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, Generation: req.Generation}, pathID(c, "runId"), req.LeaseToken, req.ReasonCode)
	if err != nil {
		writeContinuationError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

type continuationKeyRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken  string `json:"attemptLeaseToken"`
	CheckpointIdentity string `json:"checkpointIdentity"`
}

func (h *MachineHandler) continuationCheckpointKey(ctx context.Context, c *app.RequestContext) {
	var req continuationKeyRequest
	if !decode(c, &req, 16<<10) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) || h.continuation == nil {
		return
	}
	materialization, err := h.continuation.IssueCheckpointKey(ctx, continuationapp.MachineAuthority{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, AttemptID: pathID(c, "attemptId"), LeaseToken: req.AttemptLeaseToken, Generation: req.Generation, OwnerEpoch: req.OwnerEpoch}, req.CheckpointIdentity)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, materialization)
}

type continuationRestoreKeyRequest struct {
	domain.MachineEnvelope
	AttemptID         int64  `json:"attemptId,string"`
	AttemptLeaseToken string `json:"attemptLeaseToken"`
}

func (h *MachineHandler) continuationRestoreKey(ctx context.Context, c *app.RequestContext) {
	var req continuationRestoreKeyRequest
	if !decode(c, &req, 16<<10) || !h.auth(ctx, c, req.MachineEnvelope) || h.continuation == nil {
		return
	}
	materialization, err := h.continuation.ResolveRestoreKey(ctx, continuationapp.MachineAuthority{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, AttemptID: req.AttemptID, LeaseToken: req.AttemptLeaseToken, Generation: req.Generation, OwnerEpoch: req.OwnerEpoch}, pathID(c, "restoreRunId"))
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, materialization)
}

type continuationRestoreResultRequest struct {
	domain.MachineEnvelope
	AttemptID         int64  `json:"attemptId,string"`
	AttemptLeaseToken string `json:"attemptLeaseToken"`
	RestoreRunID      int64  `json:"restoreRunId,string"`
	ResultDigest      string `json:"resultDigest"`
	BaseCommit        string `json:"baseCommit"`
	ResultCommit      string `json:"resultCommit"`
	ResultTree        string `json:"resultTree"`
}

func (h *MachineHandler) continuationRestoreResult(ctx context.Context, c *app.RequestContext) {
	var req continuationRestoreResultRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if req.RestoreRunID != pathID(c, "restoreRunId") {
		writeDispatchError(c, dispatchapp.ErrIdentityConflict)
		return
	}
	if err := h.store.CompleteContinuationRestore(ctx, req.MachineEnvelope, req.AttemptID, req.AttemptLeaseToken, req.RestoreRunID, req.ResultDigest, req.BaseCommit, req.ResultCommit, req.ResultTree); err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

type providerProxyReadyRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string                    `json:"attemptLeaseToken"`
	Ready             domain.ProviderProxyReady `json:"ready"`
}

func (h *MachineHandler) providerProxyReady(ctx context.Context, c *app.RequestContext) {
	var req providerProxyReadyRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.dispatch.RecordProviderProxyReady(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken, req.Ready); err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

type postprocessClaimRequest struct{ domain.MachineEnvelope }

func (h *MachineHandler) claimFinalization(ctx context.Context, c *app.RequestContext) {
	var req postprocessClaimRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	claim, err := h.postprocess.ClaimFinalization(ctx, req.MachineEnvelope, pathID(c, "runId"))
	if err != nil {
		writePostprocessError(c, err)
		return
	}
	response.Success(c, claim)
}

type freezeRequest struct {
	domain.MachineEnvelope
	LeaseToken string                      `json:"leaseToken"`
	Result     artifactdomain.FreezeResult `json:"result"`
}

func (h *MachineHandler) freezeFinalization(ctx context.Context, c *app.RequestContext) {
	var req freezeRequest
	if !decode(c, &req, 2<<20) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if _, err := h.postprocess.CompleteFreeze(ctx, req.MachineEnvelope, pathID(c, "runId"), req.LeaseToken, req.Result); err != nil {
		writePostprocessError(c, err)
		return
	}
	response.Success(c, map[string]any{"frozen": true})
}

type postprocessFailureRequest struct {
	domain.MachineEnvelope
	LeaseToken string `json:"leaseToken"`
	ReasonCode string `json:"reasonCode"`
}

func (h *MachineHandler) failFinalization(ctx context.Context, c *app.RequestContext) {
	var req postprocessFailureRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.postprocess.FailFinalization(ctx, req.MachineEnvelope, pathID(c, "runId"), req.LeaseToken, req.ReasonCode); err != nil {
		writePostprocessError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

func (h *MachineHandler) claimUpload(ctx context.Context, c *app.RequestContext) {
	var req postprocessClaimRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	claim, err := h.postprocess.ClaimUpload(ctx, req.MachineEnvelope, pathID(c, "uploadId"))
	if err != nil {
		writePostprocessError(c, err)
		return
	}
	response.Success(c, claim)
}

type uploadVerificationRequest struct {
	domain.MachineEnvelope
	LeaseToken string `json:"leaseToken"`
}

func (h *MachineHandler) verifyUpload(ctx context.Context, c *app.RequestContext) {
	var req uploadVerificationRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.postprocess.VerifyUpload(ctx, req.MachineEnvelope, pathID(c, "uploadId"), req.LeaseToken); err != nil {
		writePostprocessError(c, err)
		return
	}
	response.Success(c, map[string]any{"verified": true})
}

func (h *MachineHandler) failUpload(ctx context.Context, c *app.RequestContext) {
	var req postprocessFailureRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.postprocess.FailUpload(ctx, req.MachineEnvelope, pathID(c, "uploadId"), req.LeaseToken, req.ReasonCode); err != nil {
		writePostprocessError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

type releaseAssignmentRequest struct {
	domain.MachineEnvelope
	ReasonCode string `json:"reasonCode"`
}

func (h *MachineHandler) releaseAssignment(ctx context.Context, c *app.RequestContext) {
	var req releaseAssignmentRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.dispatch.ReleaseAssignment(ctx, req.MachineEnvelope, pathID(c, "assignmentId"), req.ReasonCode); err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"released": true})
}

type claimRequest struct {
	domain.MachineEnvelope
	ExecutionRequirementDigest string `json:"executionRequirementDigest"`
}

func (h *MachineHandler) claim(ctx context.Context, c *app.RequestContext) {
	var req claimRequest
	if !decode(c, &req, 1<<20) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	contract, err := h.dispatch.Claim(ctx, req.MachineEnvelope, pathID(c, "assignmentId"), req.ExecutionRequirementDigest)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"contract": contract})
}

type reviewClaimRequest struct{ domain.MachineEnvelope }

func (h *MachineHandler) claimReviewRun(ctx context.Context, c *app.RequestContext) {
	var req reviewClaimRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.reviewRuns == nil {
		writeError(c, http.StatusServiceUnavailable, "REVIEW_RUN_CONTROL_UNAVAILABLE", "independent review control is unavailable")
		return
	}
	claim, err := h.reviewRuns.Claim(ctx, reviewMachineEnvelope(req.MachineEnvelope), pathID(c, "runId"))
	if err != nil {
		writeReviewRunError(c, err)
		return
	}
	response.Success(c, map[string]any{"claim": claim})
}

type reviewRecoveryRequest struct {
	domain.MachineEnvelope
	ContainerID             string `json:"containerId"`
	ContainerName           string `json:"containerName"`
	ImageDigest             string `json:"imageDigest"`
	ExecutionIdentityDigest string `json:"executionIdentityDigest"`
}

func (h *MachineHandler) recoverReviewRun(ctx context.Context, c *app.RequestContext) {
	var req reviewRecoveryRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.reviewRuns == nil {
		writeError(c, http.StatusServiceUnavailable, "REVIEW_RUN_CONTROL_UNAVAILABLE", "independent review control is unavailable")
		return
	}
	claim, err := h.reviewRuns.Recover(ctx, reviewMachineEnvelope(req.MachineEnvelope), pathID(c, "runId"), reviewrunfacade.ContainerRegistration{ContainerID: req.ContainerID, ContainerName: req.ContainerName, ImageDigest: req.ImageDigest, ExecutionIdentityDigest: req.ExecutionIdentityDigest})
	if err != nil {
		writeReviewRunError(c, err)
		return
	}
	response.Success(c, map[string]any{"claim": claim})
}

type reviewLeaseRequest struct {
	domain.MachineEnvelope
	ReviewLeaseToken string `json:"reviewLeaseToken"`
}

func (h *MachineHandler) renewReviewLease(ctx context.Context, c *app.RequestContext) {
	var req reviewLeaseRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.reviewRuns == nil {
		writeError(c, http.StatusServiceUnavailable, "REVIEW_RUN_CONTROL_UNAVAILABLE", "independent review control is unavailable")
		return
	}
	expiresAt, err := h.reviewRuns.RenewLease(ctx, reviewMachineEnvelope(req.MachineEnvelope), pathID(c, "runId"), req.ReviewLeaseToken)
	if err != nil {
		writeReviewRunError(c, err)
		return
	}
	receipt := sha256.Sum256([]byte(fmt.Sprintf("seven.review-lease/v1\x00%d\x00%d\x00%s", pathID(c, "runId"), req.OwnerEpoch, expiresAt)))
	response.Success(c, map[string]any{"expiresAt": expiresAt, "receiptDigest": hex.EncodeToString(receipt[:])})
}

func (h *MachineHandler) reviewLaunchPermit(ctx context.Context, c *app.RequestContext) {
	var req reviewLeaseRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.reviewRuns == nil {
		writeError(c, http.StatusServiceUnavailable, "REVIEW_RUN_CONTROL_UNAVAILABLE", "independent review control is unavailable")
		return
	}
	permit, err := h.reviewRuns.AcquireLaunchPermit(ctx, reviewMachineEnvelope(req.MachineEnvelope), pathID(c, "runId"), req.ReviewLeaseToken)
	if err != nil {
		writeReviewRunError(c, err)
		return
	}
	response.Success(c, map[string]any{"permit": permit})
}

type reviewContainerRequest struct {
	domain.MachineEnvelope
	ReviewLeaseToken        string `json:"reviewLeaseToken"`
	PermitID                int64  `json:"permitId,string"`
	PermitToken             string `json:"permitToken"`
	ContainerID             string `json:"containerId"`
	ContainerName           string `json:"containerName"`
	ImageDigest             string `json:"imageDigest"`
	ExecutionIdentityDigest string `json:"executionIdentityDigest"`
}

func (h *MachineHandler) reviewContainerCreated(ctx context.Context, c *app.RequestContext) {
	var req reviewContainerRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.reviewRuns == nil {
		writeError(c, http.StatusServiceUnavailable, "REVIEW_RUN_CONTROL_UNAVAILABLE", "independent review control is unavailable")
		return
	}
	err := h.reviewRuns.RecordContainer(ctx, reviewMachineEnvelope(req.MachineEnvelope), pathID(c, "runId"), reviewrunfacade.ContainerRegistration{ContainerID: req.ContainerID, ContainerName: req.ContainerName, ImageDigest: req.ImageDigest, ExecutionIdentityDigest: req.ExecutionIdentityDigest}, req.ReviewLeaseToken, req.PermitID, req.PermitToken)
	if err != nil {
		writeReviewRunError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

type reviewReportRequest struct {
	domain.MachineEnvelope
	ReviewLeaseToken string                     `json:"reviewLeaseToken"`
	PermitID         int64                      `json:"permitId,string"`
	PermitToken      string                     `json:"permitToken"`
	Report           json.RawMessage            `json:"report"`
	Usage            reviewrunfacade.UsageFacts `json:"usage"`
}

func (h *MachineHandler) submitReviewReport(ctx context.Context, c *app.RequestContext) {
	var req reviewReportRequest
	if !decode(c, &req, 384<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.reviewRuns == nil {
		writeError(c, http.StatusServiceUnavailable, "REVIEW_RUN_CONTROL_UNAVAILABLE", "independent review control is unavailable")
		return
	}
	result, err := h.reviewRuns.SubmitReport(ctx, reviewMachineEnvelope(req.MachineEnvelope), pathID(c, "runId"), reviewrunfacade.ReportSubmission{LeaseToken: req.ReviewLeaseToken, PermitID: req.PermitID, PermitToken: req.PermitToken, Report: req.Report, Usage: req.Usage})
	if err != nil {
		writeReviewRunError(c, err)
		return
	}
	response.Success(c, map[string]any{"result": result})
}

type reviewFailureRequest struct {
	domain.MachineEnvelope
	ReviewLeaseToken string `json:"reviewLeaseToken"`
	ReasonCode       string `json:"reasonCode"`
}

func (h *MachineHandler) failReviewRun(ctx context.Context, c *app.RequestContext) {
	var req reviewFailureRequest
	if !decode(c, &req, 64<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.reviewRuns == nil {
		writeError(c, http.StatusServiceUnavailable, "REVIEW_RUN_CONTROL_UNAVAILABLE", "independent review control is unavailable")
		return
	}
	if err := h.reviewRuns.RecordTransientFailure(ctx, reviewMachineEnvelope(req.MachineEnvelope), pathID(c, "runId"), req.ReviewLeaseToken, req.ReasonCode); err != nil {
		writeReviewRunError(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

func reviewMachineEnvelope(e domain.MachineEnvelope) reviewrunfacade.MachineEnvelope {
	return reviewrunfacade.MachineEnvelope{SchemaVersion: e.SchemaVersion, WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID, RequestID: e.RequestID, Generation: e.Generation, OwnerEpoch: e.OwnerEpoch}
}

type leaseRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken   string `json:"attemptLeaseToken"`
	ActiveElapsedMillis *int64 `json:"activeElapsedMillis,string,omitempty"`
}

func (h *MachineHandler) launch(ctx context.Context, c *app.RequestContext) {
	var req leaseRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	permit, replay, err := h.dispatch.AcquireLaunchPermit(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"permit": permit, "replayed": replay})
}

type containerCreatedRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken       string `json:"attemptLeaseToken"`
	ContainerID             string `json:"containerId"`
	ImageDigest             string `json:"imageDigest"`
	ExecutionIdentityDigest string `json:"executionIdentityDigest"`
}

func (h *MachineHandler) containerCreated(ctx context.Context, c *app.RequestContext) {
	var req containerCreatedRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.dispatch.RecordContainerCreated(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken, req.ContainerID, req.ImageDigest, req.ExecutionIdentityDigest); err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

type supervisorReadyRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string                 `json:"attemptLeaseToken"`
	Ready             domain.SupervisorReady `json:"ready"`
}

func (h *MachineHandler) supervisorReady(ctx context.Context, c *app.RequestContext) {
	var req supervisorReadyRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.dispatch.RecordSupervisorReady(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken, req.Ready); err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

func (h *MachineHandler) runPermit(ctx context.Context, c *app.RequestContext) {
	var req leaseRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	permit, err := h.dispatch.AcquireRunPermit(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"permit": permit})
}

type harnessStartedRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string                `json:"attemptLeaseToken"`
	Started           domain.HarnessStarted `json:"started"`
}

func (h *MachineHandler) harnessStarted(ctx context.Context, c *app.RequestContext) {
	var req harnessStartedRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if err := h.dispatch.RecordHarnessStarted(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken, req.Started); err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

func (h *MachineHandler) renewLease(ctx context.Context, c *app.RequestContext) {
	var req leaseRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	expiresAt, err := h.dispatch.RenewExecutionLease(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken, req.ActiveElapsedMillis)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	receipt := sha256.Sum256([]byte(fmt.Sprintf("seven.execution-lease/v1\x00%d\x00%d\x00%s", pathID(c, "attemptId"), req.OwnerEpoch, expiresAt)))
	response.Success(c, map[string]any{"expiresAt": expiresAt, "receiptDigest": hex.EncodeToString(receipt[:])})
}

type reattachPermitRequest struct {
	domain.MachineEnvelope
	Evidence domain.RuntimeEvidence `json:"evidence"`
}

func (h *MachineHandler) reattachPermit(ctx context.Context, c *app.RequestContext) {
	var req reattachPermitRequest
	if !decode(c, &req, 256<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	permit, err := h.dispatch.AcquireReattachPermit(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.Evidence)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"permit": permit})
}

type reattachAckRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string             `json:"attemptLeaseToken"`
	Ack               domain.ReattachAck `json:"ack"`
}

func (h *MachineHandler) reattachAck(ctx context.Context, c *app.RequestContext) {
	var req reattachAckRequest
	if !decode(c, &req, 256<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	result, err := h.dispatch.CommitReattach(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken, req.Ack)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, result)
}

type runtimeLossRequest struct {
	domain.MachineEnvelope
	ReasonCode     string `json:"reasonCode"`
	EvidenceDigest string `json:"evidenceDigest,omitempty"`
}

func (h *MachineHandler) runtimeLoss(ctx context.Context, c *app.RequestContext) {
	var req runtimeLossRequest
	if !decode(c, &req, 128<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	result, err := h.dispatch.ReportRuntimeLoss(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.ReasonCode, req.EvidenceDigest)
	if err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, result)
}

type factsRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string        `json:"attemptLeaseToken"`
	Facts             []domain.Fact `json:"facts"`
}

func (h *MachineHandler) facts(ctx context.Context, c *app.RequestContext) {
	h.recordFacts(ctx, c, false)
}

func (h *MachineHandler) journalFacts(ctx context.Context, c *app.RequestContext) {
	h.recordFacts(ctx, c, true)
}

func (h *MachineHandler) recordFacts(ctx context.Context, c *app.RequestContext, journal bool) {
	var req factsRequest
	if !decode(c, &req, 1<<20) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if len(req.Facts) < 1 || len(req.Facts) > 128 {
		writeError(c, 400, "FACT_BATCH_INVALID", "facts must contain 1-128 items")
		return
	}
	record := h.store.RecordFacts
	if journal {
		record = h.store.RecordJournalFacts
	}
	if err := record(ctx, req.MachineEnvelope, pathID(c, "attemptId"), req.AttemptLeaseToken, req.Facts); err != nil {
		writeDispatchError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

type completionRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string          `json:"attemptLeaseToken"`
	Completion        json.RawMessage `json:"completion"`
}

func (h *MachineHandler) completion(ctx context.Context, c *app.RequestContext) {
	var req completionRequest
	if !decode(c, &req, 1<<20) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	attemptCtx, _ := h.store.AttemptContext(ctx, pathID(c, "attemptId"))
	if len(req.Completion) == 0 || !json.Valid(req.Completion) {
		writeError(c, 400, "COMPLETION_INVALID", "completion must be valid JSON")
		return
	}
	if err := h.tasks.RecordCompletion(ctx, attemptCtx.TaskID, pathID(c, "attemptId"), req.Completion); err != nil {
		writeError(c, 500, "COMPLETION_PERSIST_FAILED", "completion could not be persisted")
		return
	}
	response.Success(c, map[string]any{"validationStatus": taskdomain.CompletionProposalAccepted})
}

type checkInput struct {
	Ordinal  int    `json:"ordinal"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
	Status   string `json:"status"`
}
type checksRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string       `json:"attemptLeaseToken"`
	Checks            []checkInput `json:"checks"`
}

func (h *MachineHandler) checks(ctx context.Context, c *app.RequestContext) {
	var req checksRequest
	if !decode(c, &req, 1<<20) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	attemptCtx, _ := h.store.AttemptContext(ctx, pathID(c, "attemptId"))
	if len(req.Checks) > 32 {
		writeError(c, 400, "CHECK_BATCH_INVALID", "too many checks")
		return
	}
	for _, v := range req.Checks {
		if len(v.Output) > 128<<10 {
			writeError(c, 400, "CHECK_OUTPUT_TOO_LARGE", "check output exceeds limit")
			return
		}
		if err := h.tasks.RecordCheck(ctx, attemptCtx.TaskID, pathID(c, "attemptId"), v.Ordinal, v.Name, v.Command, v.ExitCode, v.Output, v.Status); err != nil && !strings.Contains(err.Error(), "duplicate key") {
			writeError(c, 500, "CHECK_PERSIST_FAILED", "check could not be persisted")
			return
		}
	}
	response.Success(c, map[string]any{"ack": true})
}

// Processor materialization is deliberately a separate, short-lived Machine
// call. The Worker asks for it only after the Agent process group has stopped;
// values are returned once and must be passed through the Supervisor's private
// control path rather than Docker environment or Attempt spec.
type processorMaterializationRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string `json:"attemptLeaseToken"`
	CycleID           int64  `json:"cycleId,string"`
	CycleNumber       int    `json:"cycleNumber"`
}

func (h *MachineHandler) processorMaterialization(ctx context.Context, c *app.RequestContext) {
	var req processorMaterializationRequest
	if !decode(c, &req, 64<<10) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	if h.processors == nil {
		writeError(c, 503, "RESULT_PROCESSOR_UNAVAILABLE", "result processor runtime is unavailable")
		return
	}
	materialization, err := h.processors.MaterializeEvaluationEnvironment(ctx, processorIdentity(req.MachineEnvelope), pathID(c, "attemptId"), req.AttemptLeaseToken, req.CycleID, req.CycleNumber)
	if err != nil {
		writeProcessorError(c, err)
		return
	}
	response.Success(c, materialization)
}

type resultEvaluationRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string                              `json:"attemptLeaseToken"`
	Candidate         processorfacade.EvaluationCandidate `json:"candidate"`
}

func (h *MachineHandler) resultEvaluation(ctx context.Context, c *app.RequestContext) {
	var req resultEvaluationRequest
	if !decode(c, &req, 2<<20) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	if h.processors == nil {
		writeError(c, 503, "RESULT_PROCESSOR_UNAVAILABLE", "result processor runtime is unavailable")
		return
	}
	directive, err := h.processors.PersistEvaluation(ctx, processorIdentity(req.MachineEnvelope), pathID(c, "attemptId"), req.AttemptLeaseToken, req.Candidate)
	if err != nil {
		writeProcessorError(c, err)
		return
	}
	response.Success(c, directive)
}

type continuationCheckpointRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string                                          `json:"attemptLeaseToken"`
	Checkpoint        processorfacade.ContinuationCheckpointCandidate `json:"checkpoint"`
}

// continuationCheckpoint records only deterministic object keys and verified
// digests.  The objects themselves were uploaded/read back by the Worker and
// never traverse the Machine API.
func (h *MachineHandler) continuationCheckpoint(ctx context.Context, c *app.RequestContext) {
	var req continuationCheckpointRequest
	if !decode(c, &req, 128<<10) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	if h.processors == nil {
		writeError(c, 503, "RESULT_PROCESSOR_UNAVAILABLE", "result processor runtime is unavailable")
		return
	}
	view, err := h.processors.RecordContinuationCheckpoint(ctx, processorIdentity(req.MachineEnvelope), pathID(c, "attemptId"), req.AttemptLeaseToken, req.Checkpoint)
	if err != nil {
		writeProcessorError(c, err)
		return
	}
	response.Success(c, view)
}

type fixtureClaimRequest struct{ domain.MachineEnvelope }

func (h *MachineHandler) fixtureClaim(ctx context.Context, c *app.RequestContext) {
	var req fixtureClaimRequest
	if !decode(c, &req, 16<<10) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.processors == nil {
		writeError(c, 503, "RESULT_PROCESSOR_UNAVAILABLE", "result processor runtime is unavailable")
		return
	}
	claim, err := h.processors.ClaimFixtureRun(ctx, processorIdentity(req.MachineEnvelope), pathID(c, "runId"))
	if err != nil {
		writeProcessorError(c, err)
		return
	}
	response.Success(c, claim)
}

type fixtureResultRequest struct {
	domain.MachineEnvelope
	Result processorfacade.FixtureResult `json:"result"`
}

func (h *MachineHandler) fixtureResult(ctx context.Context, c *app.RequestContext) {
	var req fixtureResultRequest
	if !decode(c, &req, 2<<20) || !h.auth(ctx, c, req.MachineEnvelope) {
		return
	}
	if h.processors == nil {
		writeError(c, 503, "RESULT_PROCESSOR_UNAVAILABLE", "result processor runtime is unavailable")
		return
	}
	view, err := h.processors.RecordFixtureResult(ctx, processorIdentity(req.MachineEnvelope), pathID(c, "runId"), req.Result)
	if err != nil {
		writeProcessorError(c, err)
		return
	}
	response.Success(c, view)
}

func processorIdentity(e domain.MachineEnvelope) processorfacade.MachineIdentity {
	return processorfacade.MachineIdentity{WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID, Generation: e.Generation, OwnerEpoch: e.OwnerEpoch}
}

func (h *MachineHandler) evidence(ctx context.Context, c *app.RequestContext) {
	e, lease, ok := h.headerEnvelope(c)
	if !ok || !h.authAttempt(ctx, c, e, lease) {
		return
	}
	v, _ := h.store.AttemptContext(ctx, pathID(c, "attemptId"))
	checks, err := h.tasks.CheckEvidence(ctx, v.TaskID, pathID(c, "attemptId"))
	if err != nil {
		writeError(c, 500, "EVIDENCE_READ_FAILED", "evidence unavailable")
		return
	}
	events, err := h.tasks.Events(ctx, v.TaskID, 0, 1000)
	if err != nil {
		writeError(c, 500, "EVIDENCE_READ_FAILED", "evidence unavailable")
		return
	}
	response.Success(c, map[string]any{"checks": checks, "events": events})
}
func (h *MachineHandler) control(ctx context.Context, c *app.RequestContext) {
	e, lease, ok := h.headerEnvelope(c)
	if !ok || !h.authAttempt(ctx, c, e, lease) {
		return
	}
	v, _ := h.store.AttemptContext(ctx, pathID(c, "attemptId"))
	cancelling, err := h.tasks.IsCancelling(ctx, v.TaskID)
	if err != nil {
		writeError(c, 500, "CONTROL_READ_FAILED", "attempt control unavailable")
		return
	}
	response.Success(c, map[string]any{"cancelling": cancelling, "serverTime": time.Now().UTC().Format(time.RFC3339Nano)})
}

type artifactInput struct {
	ID        int64           `json:"id,string,omitempty"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	ObjectKey string          `json:"objectKey"`
	SHA256    string          `json:"sha256"`
	SizeBytes int64           `json:"sizeBytes"`
	MediaType string          `json:"mediaType"`
	Status    string          `json:"status"`
	Metadata  json.RawMessage `json:"metadata"`
}
type artifactsRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string          `json:"attemptLeaseToken"`
	Artifacts         []artifactInput `json:"artifacts"`
}

func (h *MachineHandler) artifacts(ctx context.Context, c *app.RequestContext) {
	var req artifactsRequest
	if !decode(c, &req, 1<<20) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	v, _ := h.store.AttemptContext(ctx, pathID(c, "attemptId"))
	for _, a := range req.Artifacts {
		prefix := fmt.Sprintf("tasks/%d/attempts/%d/", v.TaskID, pathID(c, "attemptId"))
		if !strings.HasPrefix(a.ObjectKey, prefix) || len(a.SHA256) != 64 || a.SizeBytes < 0 {
			writeError(c, 400, "ARTIFACT_IDENTITY_INVALID", "artifact identity is invalid")
			return
		}
		id := a.ID
		if id == 0 {
			id = h.idGen.NextID()
		}
		if err := h.tasks.RecordArtifact(ctx, taskdomain.Artifact{ID: id, TaskID: v.TaskID, AttemptID: pathID(c, "attemptId"), Type: a.Type, Name: a.Name, ObjectKey: a.ObjectKey, SHA256: a.SHA256, SizeBytes: a.SizeBytes, MediaType: a.MediaType, Status: a.Status, Metadata: a.Metadata}); err != nil {
			writeError(c, 500, "ARTIFACT_PERSIST_FAILED", "artifact metadata could not be persisted")
			return
		}
	}
	response.Success(c, map[string]any{"ack": true})
}

type outcomeRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string `json:"attemptLeaseToken"`
	Outcome           string `json:"outcome"`
	ReasonCode        string `json:"reasonCode"`
	Message           string `json:"message"`
	ResultCommit      string `json:"resultCommit"`
	ResultTree        string `json:"resultTree"`
	Summary           string `json:"summary"`
}

func (h *MachineHandler) outcome(ctx context.Context, c *app.RequestContext) {
	var req outcomeRequest
	if !decode(c, &req, 256<<10) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	if req.Outcome != "EXECUTION_COMPLETED" && req.Outcome != "FAILED" && req.Outcome != "CANCELLED" && req.Outcome != "TIMED_OUT" {
		writeError(c, 400, "OUTCOME_INVALID", "outcome must be EXECUTION_COMPLETED, FAILED, CANCELLED, or TIMED_OUT")
		return
	}
	err := h.store.FinalizeOutcome(ctx, pathID(c, "attemptId"), domain.Outcome{
		Outcome: req.Outcome, ReasonCode: req.ReasonCode, Message: req.Message, ResultCommit: req.ResultCommit, ResultTree: req.ResultTree, Summary: req.Summary,
		WorkerID: req.WorkerID, WorkerInstanceID: req.InstanceID, HostBootID: req.HostBootID, Generation: req.Generation,
	})
	if err != nil {
		writeError(c, 409, "OUTCOME_REJECTED", err.Error())
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

func (h *MachineHandler) auth(ctx context.Context, c *app.RequestContext, e domain.MachineEnvelope) bool {
	auth := strings.TrimSpace(string(c.Request.Header.Peek("Authorization")))
	if !strings.HasPrefix(auth, "Bearer ") || h.registry.Authenticate(ctx, e.WorkerID, strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))) != nil {
		writeError(c, 401, "WORKER_UNAUTHORIZED", "worker credential is invalid")
		return false
	}
	return true
}
func (h *MachineHandler) authAttempt(ctx context.Context, c *app.RequestContext, e domain.MachineEnvelope, lease string) bool {
	if !h.auth(ctx, c, e) {
		return false
	}
	v, err := h.store.AttemptContext(ctx, pathID(c, "attemptId"))
	if err != nil || !v.Authorizes(e, lease, time.Now()) {
		writeError(c, 409, "ATTEMPT_IDENTITY_CONFLICT", "attempt identity is invalid")
		return false
	}
	return true
}
func (h *MachineHandler) headerEnvelope(c *app.RequestContext) (domain.MachineEnvelope, string, bool) {
	generation := int64(0)
	_ = json.Unmarshal(c.Request.Header.Peek("X-Seven-Generation"), &generation)
	ownerEpoch := int64(0)
	_ = json.Unmarshal(c.Request.Header.Peek("X-Seven-Owner-Epoch"), &ownerEpoch)
	workerID, _ := strconv.ParseInt(string(c.Request.Header.Peek("X-Seven-Worker-ID")), 10, 64)
	requestID := strings.TrimSpace(string(c.Request.Header.Peek("X-Seven-Request-ID")))
	e := domain.MachineEnvelope{SchemaVersion: 1, WorkerID: workerID, InstanceID: string(c.Request.Header.Peek("X-Seven-Worker-Instance")), HostBootID: string(c.Request.Header.Peek("X-Seven-Host-Boot-ID")), RequestID: requestID, Generation: generation, OwnerEpoch: ownerEpoch}
	lease := string(c.Request.Header.Peek("X-Seven-Attempt-Lease"))
	if e.WorkerID <= 0 || e.InstanceID == "" || e.HostBootID == "" || e.RequestID == "" || len(e.RequestID) > 128 || generation < 1 || ownerEpoch < 1 || lease == "" {
		writeError(c, 400, "INVALID_MACHINE_ENVELOPE", "machine headers are invalid")
		return e, lease, false
	}
	return e, lease, true
}
func decode(c *app.RequestContext, dst any, limit int) bool {
	body := c.Request.Body()
	if len(body) == 0 || len(body) > limit {
		writeError(c, 413, "REQUEST_TOO_LARGE", "request body exceeds limit")
		return false
	}
	d := json.NewDecoder(io.LimitReader(bytes.NewReader(body), int64(limit)+1))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		writeError(c, 400, "INVALID_JSON", "request body is invalid")
		return false
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(c, 400, "INVALID_JSON", "request body has trailing data")
		return false
	}
	return true
}
func writeDispatchError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, dispatchapp.ErrNotFound):
		writeError(c, 404, "ASSIGNMENT_NOT_FOUND", err.Error())
	case errors.Is(err, dispatchapp.ErrNotClaimable):
		writeError(c, 409, "ASSIGNMENT_NOT_CLAIMABLE", err.Error())
	case errors.Is(err, dispatchapp.ErrStaleInstance):
		writeError(c, 409, "STALE_WORKER_INSTANCE", err.Error())
	default:
		writeError(c, 409, "EXECUTION_IDENTITY_CONFLICT", err.Error())
	}
}

func writeReviewRunError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, reviewrunfacade.ErrNotFound):
		writeError(c, http.StatusNotFound, "REVIEW_RUN_NOT_FOUND", "independent review run was not found")
	case reviewrunfacade.IsConflict(err):
		code := "REVIEW_RUN_FENCE_REJECTED"
		if stage := reviewrunfacade.FenceStage(err); stage != "" {
			code += "_" + stage
		}
		writeError(c, http.StatusConflict, code, "independent review run identity is no longer current")
	case errors.Is(err, reviewrunfacade.ErrInvalidReport):
		writeError(c, http.StatusConflict, "REVIEW_REPORT_REJECTED", "independent review report was rejected")
	default:
		writeError(c, http.StatusInternalServerError, "REVIEW_RUN_CONTROL_FAILED", "independent review control failed")
	}
}
func writePostprocessError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, artifactinfra.ErrNotFound):
		writeError(c, 404, "POSTPROCESS_WORK_NOT_FOUND", "post-process work was not found")
	default:
		writeError(c, 409, "POSTPROCESS_IDENTITY_CONFLICT", "post-process identity or state is invalid")
	}
}
func writeProcessorError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, processorinfra.ErrNotFound):
		writeError(c, 404, "RESULT_PROCESSOR_NOT_FOUND", "result processor execution resource was not found")
	case errors.Is(err, processorinfra.ErrConflict):
		writeError(c, 409, "RESULT_PROCESSOR_IDENTITY_CONFLICT", "result processor execution identity or state is invalid")
	default:
		// Deliberately return the stable reason prefix only. Secret resolver
		// errors and processor values must not leak through a Machine response.
		code := "RESULT_PROCESSOR_REJECTED"
		if prefix := strings.SplitN(err.Error(), ":", 2)[0]; len(prefix) <= 128 && (strings.HasPrefix(prefix, "RESULT_") || strings.HasPrefix(prefix, "CHECKPOINT_") || strings.HasPrefix(prefix, "CONTINUATION_")) {
			code = prefix
		}
		writeError(c, 409, code, "result processor request was rejected")
	}
}

func writeContinuationError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, continuationdomain.ErrCheckpointUnavailable):
		writeError(c, 404, "CONTINUATION_RESOURCE_NOT_FOUND", "continuation resource was not found")
	case errors.Is(err, continuationdomain.ErrCheckpointExpired),
		errors.Is(err, continuationdomain.ErrMaterializationExpired),
		errors.Is(err, continuationdomain.ErrMaterializationInvalid),
		errors.Is(err, continuationdomain.ErrContinuationIdentity),
		errors.Is(err, continuationdomain.ErrContinuationNotAllowed):
		writeError(c, 409, err.Error(), "continuation identity or state is invalid")
	default:
		// Persistence and object-store failures are retryable infrastructure
		// errors. Returning 500 keeps the post-process delivery unacked so the
		// worker can retry instead of silently dropping the materialization.
		writeError(c, 500, "CONTINUATION_PERSIST_FAILED", "continuation state could not be persisted")
	}
}

func writeError(c *app.RequestContext, status int, code, message string) {
	var e *apperrors.AppError
	switch status {
	case 401:
		e = apperrors.Unauthorized(message)
	case 404:
		e = apperrors.NotFound(message)
	case 409:
		e = apperrors.ObjectState(message)
	case 500:
		e = apperrors.System(message)
	default:
		e = apperrors.Params(message)
	}
	response.Error(c, e.WithDetails(map[string]any{"reasonCode": code}))
	c.Response.SetStatusCode(status)
}

func pathID(c *app.RequestContext, name string) int64 {
	raw := c.Param(name)
	if raw == "" || strings.TrimSpace(raw) != raw || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}
