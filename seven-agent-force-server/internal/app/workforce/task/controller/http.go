package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"

	postprocess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/infrastructure"
	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	processorapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Handler struct {
	service      *application.Service
	store        *infrastructure.Store
	artifacts    *artifactstore.Store
	postprocess  *postprocess.Store
	processors   *processorapp.Service
	interactions interactionfacade.TaskSummaryReader
}

func (h *Handler) SetInteractionSummaries(reader interactionfacade.TaskSummaryReader) {
	h.interactions = reader
}

// SetResultProcessorService keeps the Task read surface dependent on the
// Result Processor's small audit facade rather than its authoring store.
func (h *Handler) SetResultProcessorService(service *processorapp.Service) {
	if h != nil {
		h.processors = service
	}
}

func New(service *application.Service, store *infrastructure.Store, artifacts *artifactstore.Store, postprocessStores ...*postprocess.Store) *Handler {
	var postprocessStore *postprocess.Store
	if len(postprocessStores) > 0 {
		postprocessStore = postprocessStores[0]
	}
	return &Handler{service: service, store: store, artifacts: artifacts, postprocess: postprocessStore}
}

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

// independentReviewSummary exposes only this Task's review switch and current
// ReviewRun status. Project-specific instructions live in the immutable
// ProjectVersion and runtime identities remain control-plane diagnostics.
type independentReviewSummary struct {
	Required bool   `json:"required"`
	Status   string `json:"status,omitempty"`
}

type taskResponse struct {
	domain.Task
	IndependentReview      independentReviewSummary `json:"independentReview"`
	PendingActionCount     int                      `json:"pendingActionCount"`
	HasUnreadUpdates       bool                     `json:"hasUnreadUpdates"`
	ActivitySummary        string                   `json:"activitySummary,omitempty"`
	FirstPendingActionKind string                   `json:"firstPendingActionKind,omitempty"`
	FirstPendingActionID   int64                    `json:"firstPendingActionId,string,omitempty"`
}

// MarshalJSON preserves Task's established, flat creator-facing wire shape.
//
// response.Success first passes its argument through the Framework JSON
// compatibility normalizer. That normalizer deliberately walks exported struct
// fields to stringify int64 values, but does not flatten anonymous embedded
// structs. Without this explicit marshaler it therefore produces
// {"Task":{...},"independentReview":{...}}, which silently breaks the
// existing Task API contract. Implementing json.Marshaler makes the normalizer
// preserve this value and lets encoding/json apply Go's normal embedded-field
// semantics, while retaining all of Task's decimal-string JSON tags.
func (r taskResponse) MarshalJSON() ([]byte, error) {
	type wire taskResponse
	return json.Marshal(wire(r))
}

func taskCreatorResponse(task domain.Task) taskResponse {
	required := task.ReviewEnabled
	task.Instruction = continuationfacade.PublicInstruction(task.Instruction)
	// Never serialize legacy Task-owned infrastructure selection through public
	// Task endpoints. Project-first Tasks expose business state here; when an
	// Assignment exists, Execution.Resolved is the read-only diagnostic
	// projection of the actual immutable Runtime/Provider/Worker selection.
	task.ExecutionSnapshot = nil
	task.HarnessAlias = ""
	task.HarnessReleaseID = 0
	return taskResponse{Task: task, IndependentReview: independentReviewSummary{Required: required, Status: task.IndependentReviewStatus}}
}

func taskCreatorResponses(tasks []domain.Task) []taskResponse {
	result := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, taskCreatorResponse(task))
	}
	return result
}

func (h *Handler) taskResponsesWithSummaries(ctx context.Context, c *app.RequestContext, tasks []domain.Task) ([]taskResponse, error) {
	results := taskCreatorResponses(tasks)
	if h.interactions == nil || len(tasks) == 0 {
		return results, nil
	}
	user, ok := securitycontext.CurrentUserID(c)
	if !ok {
		return nil, errors.New("authentication is required")
	}
	ids := make([]int64, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}
	summaries, err := h.interactions.TaskSummaries(ctx, tasks[0].TenantID, user, ids, securitycontext.HasPermission(c, "workforce:task:continue"))
	if err != nil {
		return nil, err
	}
	for i := range results {
		summary := summaries[results[i].ID]
		results[i].PendingActionCount = summary.PendingActionCount
		results[i].HasUnreadUpdates = summary.HasUnreadUpdates
		results[i].ActivitySummary = summary.ActivitySummary
		results[i].FirstPendingActionKind = summary.FirstPendingActionKind
		results[i].FirstPendingActionID = summary.FirstPendingActionID
	}
	return results, nil
}

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.POST("/workforce/projects/:projectId/tasks", wrap("workforce:task:create", h.createProjectTask))
	router.GET("/workforce/projects/:projectId/runtime-options", wrap("workforce:task:create", h.runtimeOptions))
	router.GET("/workforce/projects/:projectId/tasks", wrap("workforce:task:list", h.listProjectTasks))
	router.GET("/workforce/tasks", wrap("workforce:task:list", h.list))
	router.GET("/workforce/backlog", wrap("workforce:task:list", h.backlog))
	router.GET("/workforce/tasks/:id", wrap("workforce:task:query", h.get))
	router.GET("/workforce/tasks/:id/independent-review", wrap("workforce:task:query", h.independentReview))
	router.PATCH("/workforce/tasks/:id/metadata", wrap("workforce:task:metadata:update", h.updateMetadata))
	router.POST("/workforce/tasks/:id/cancel", wrap("workforce:task:cancel", h.cancel))
	router.GET("/workforce/tasks/:id/events", wrap("workforce:task:query", h.events))
	router.GET("/workforce/tasks/:id/events/stream", wrap("workforce:task:query", h.eventStream))
	router.GET("/workforce/tasks/:id/artifacts", wrap("workforce:artifact:list", h.listArtifacts))
	router.GET("/workforce/tasks/:id/artifacts/:artifactId/download", wrap("workforce:artifact:download", h.downloadArtifact))
	router.GET("/workforce/tasks/:id/recovery", wrap("workforce:task:query", h.recovery))
	router.GET("/workforce/tasks/:id/evaluations", wrap("workforce:task:query", h.evaluations))
	router.GET("/workforce/tasks/:id/continuations", wrap("workforce:task:query", h.continuations))
	router.GET("/workforce/tasks/:id/result-outputs", wrap("workforce:task:query", h.resultOutputs))
	router.GET("/workforce/tasks/:id/finalization-runs", wrap("workforce:task:query", h.finalizationRuns))
	router.GET("/workforce/finalization-runs/:runId", wrap("workforce:task:query", h.finalizationRun))
	router.POST("/workforce/finalization-runs/:runId/retry", wrap("workforce:finalization:retry", h.retryFinalization))
	router.GET("/workforce/artifact-upload-attempts/:uploadId", wrap("workforce:task:query", h.uploadAttempt))
	router.POST("/workforce/artifact-upload-attempts/:uploadId/retry", wrap("workforce:artifact-upload:retry", h.retryUpload))
}

func (h *Handler) runtimeOptions(ctx context.Context, c *app.RequestContext) {
	projectID, err := positivePathID(c, "projectId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "project id must be a positive decimal integer")
		return
	}
	credentialID, err := strconv.ParseInt(strings.TrimSpace(c.Query("modelCredentialId")), 10, 64)
	if err != nil || credentialID < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "model credential id must be a positive decimal integer")
		return
	}
	tenantID, ok := tenantScope(c)
	if !ok {
		return
	}
	result, err := h.service.ListRuntimeOptions(ctx, tenantID, projectID, credentialID)
	if err != nil {
		switch {
		case errors.Is(err, routingfacade.ErrRuntimeSelectionRequired):
			writeError(c, http.StatusBadRequest, "TASK_RUNTIME_SELECTION_REQUIRED", "runtime selection is required")
		case errors.Is(err, routingfacade.ErrRuntimeSelectionUnavailable):
			writeError(c, http.StatusConflict, "TASK_RUNTIME_SELECTION_UNAVAILABLE", "runtime options are unavailable")
		case errors.Is(err, routingfacade.ErrRuntimeSelectionIncompatible):
			writeError(c, http.StatusConflict, "TASK_RUNTIME_SELECTION_INCOMPATIBLE", "runtime options are incompatible")
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "runtime options could not be loaded")
		}
		return
	}
	response.Success(c, result)
}

func (h *Handler) listProjectTasks(ctx context.Context, c *app.RequestContext) {
	projectID, err := positivePathID(c, "projectId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "project id must be a positive decimal integer")
		return
	}
	tenantID, ok := tenantScope(c)
	if !ok {
		return
	}
	filter, err := taskListFilter(c, tenantID, projectID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "TASK_LIST_FILTER_INVALID", err.Error())
		return
	}
	page, err := h.service.ListPage(ctx, filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "failed to list project tasks")
		return
	}
	items, err := h.taskResponsesWithSummaries(ctx, c, page.Items)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "INTERACTION_UNAVAILABLE", "task updates could not be loaded")
		return
	}
	response.Success(c, map[string]any{"items": items, "nextCursor": decimalCursor(page.NextCursor)})
}

func (h *Handler) createProjectTask(ctx context.Context, c *app.RequestContext) {
	if len(c.Request.Body()) > 128<<10 {
		writeError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds 128 KiB")
		return
	}
	projectID, err := positivePathID(c, "projectId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "project id must be a positive decimal integer")
		return
	}
	var request taskfacade.ProjectTaskRequest
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "INVALID_JSON", "request body must contain exactly one JSON object")
		return
	}
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED", "an unambiguous organization scope is required")
		return
	}
	creator, ok := securitycontext.CurrentUserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "TASK_CREATOR_IDENTITY_REQUIRED", "an authenticated user identity is required")
		return
	}
	task, replay, err := h.service.CreateProjectTask(ctx, tenantID, creator, projectID, string(c.Request.Header.Peek("Idempotency-Key")), request)
	if err != nil {
		status, code, message := taskCreateError(err)
		if projectCode := projectapp.ErrorCode(err); projectCode != "" {
			status = http.StatusBadRequest
			if projectCode == "PROJECT_VERSION_CONFLICT" {
				status = http.StatusConflict
			} else if projectCode == "PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED" {
				status = http.StatusForbidden
			}
			code, message = projectCode, projectCode
		}
		writeError(c, status, code, message)
		return
	}
	response.Success(c, map[string]any{"task": taskCreatorResponse(task), "replayed": replay})
}

func (h *Handler) resultOutputs(ctx context.Context, c *app.RequestContext) {
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	if h.processors == nil {
		writeError(c, http.StatusServiceUnavailable, "RESULT_PROCESSOR_UNAVAILABLE", "result output view is unavailable")
		return
	}
	items, err := h.processors.ListResultOutputs(ctx, task.TenantID, task.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "failed to resolve result outputs")
		return
	}
	response.Success(c, items)
}

func (h *Handler) evaluations(ctx context.Context, c *app.RequestContext) {
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	if h.processors == nil {
		writeError(c, http.StatusServiceUnavailable, "RESULT_PROCESSOR_UNAVAILABLE", "result processor audit is unavailable")
		return
	}
	items, err := h.processors.ListEvaluations(ctx, task.TenantID, task.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "failed to resolve result evaluations")
		return
	}
	response.Success(c, items)
}

func (h *Handler) continuations(ctx context.Context, c *app.RequestContext) {
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	if h.processors == nil {
		writeError(c, http.StatusServiceUnavailable, "RESULT_PROCESSOR_UNAVAILABLE", "result processor audit is unavailable")
		return
	}
	items, err := h.processors.ListContinuationCheckpoints(ctx, task.TenantID, task.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "failed to resolve continuation checkpoints")
		return
	}
	response.Success(c, items)
}

func (h *Handler) recovery(ctx context.Context, c *app.RequestContext) {
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	runs, err := h.postprocessRuns(ctx, task.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "failed to resolve recovery evidence")
		return
	}
	response.Success(c, map[string]any{
		"taskId": strconv.FormatInt(task.ID, 10), "status": task.Status, "execution": task.Execution,
		"cancelRequestId": strconv.FormatInt(task.CancelRequestID, 10), "cancelGeneration": strconv.FormatInt(task.CancelGeneration, 10),
		"cancelRequestedBy": strconv.FormatInt(task.CancelRequestedBy, 10), "cancelRequestedAt": task.CancelRequestedAt,
		"cancelReason": task.CancelReason, "finalizationRuns": runs,
	})
}

func (h *Handler) finalizationRuns(ctx context.Context, c *app.RequestContext) {
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	runs, err := h.postprocessRuns(ctx, task.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "failed to list finalization runs")
		return
	}
	response.Success(c, runs)
}

func (h *Handler) finalizationRun(ctx context.Context, c *app.RequestContext) {
	if h.postprocess == nil {
		writeError(c, http.StatusServiceUnavailable, "POSTPROCESS_UNAVAILABLE", "post-process evidence is unavailable")
		return
	}
	runID, err := positivePathID(c, "runId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "finalization run id must be a positive decimal integer")
		return
	}
	item, err := h.postprocess.GetFinalizationRun(ctx, runID)
	if err != nil {
		writePostprocessError(c, err)
		return
	}
	if _, err = h.tenantTaskByID(ctx, c, item.TaskID); err != nil {
		writeDomainError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) uploadAttempt(ctx context.Context, c *app.RequestContext) {
	if h.postprocess == nil {
		writeError(c, http.StatusServiceUnavailable, "POSTPROCESS_UNAVAILABLE", "post-process evidence is unavailable")
		return
	}
	uploadID, err := positivePathID(c, "uploadId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "upload attempt id must be a positive decimal integer")
		return
	}
	item, err := h.postprocess.GetUploadAttempt(ctx, uploadID)
	if err != nil {
		writePostprocessError(c, err)
		return
	}
	if _, err = h.tenantTaskByID(ctx, c, item.TaskID); err != nil {
		writeDomainError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) retryFinalization(ctx context.Context, c *app.RequestContext) {
	if h.postprocess == nil {
		writeError(c, http.StatusServiceUnavailable, "POSTPROCESS_UNAVAILABLE", "post-process work is unavailable")
		return
	}
	runID, err := positivePathID(c, "runId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "finalization run id must be a positive decimal integer")
		return
	}
	item, err := h.postprocess.GetFinalizationRun(ctx, runID)
	if err != nil {
		writePostprocessError(c, err)
		return
	}
	if _, err = h.tenantTaskByID(ctx, c, item.TaskID); err != nil {
		writeDomainError(c, err)
		return
	}
	actor, _ := securitycontext.CurrentUserID(c)
	if err = h.postprocess.RetryFinalization(ctx, item.ID, actor, retryReason(c)); err != nil {
		writePostprocessError(c, err)
		return
	}
	updated, _ := h.postprocess.GetFinalizationRun(ctx, item.ID)
	response.Success(c, updated)
}

func (h *Handler) retryUpload(ctx context.Context, c *app.RequestContext) {
	if h.postprocess == nil {
		writeError(c, http.StatusServiceUnavailable, "POSTPROCESS_UNAVAILABLE", "post-process work is unavailable")
		return
	}
	uploadID, err := positivePathID(c, "uploadId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "upload attempt id must be a positive decimal integer")
		return
	}
	item, err := h.postprocess.GetUploadAttempt(ctx, uploadID)
	if err != nil {
		writePostprocessError(c, err)
		return
	}
	if _, err = h.tenantTaskByID(ctx, c, item.TaskID); err != nil {
		writeDomainError(c, err)
		return
	}
	actor, _ := securitycontext.CurrentUserID(c)
	if err = h.postprocess.RetryUpload(ctx, item.ID, actor, retryReason(c)); err != nil {
		writePostprocessError(c, err)
		return
	}
	updated, _ := h.postprocess.GetUploadAttempt(ctx, item.ID)
	response.Success(c, updated)
}

func retryReason(c *app.RequestContext) string {
	var request struct {
		Reason string `json:"reason"`
	}
	if len(c.Request.Body()) > 0 {
		_ = json.Unmarshal(c.Request.Body(), &request)
	}
	return strings.TrimSpace(request.Reason)
}

func (h *Handler) postprocessRuns(ctx context.Context, taskID int64) (any, error) {
	if h.postprocess == nil {
		return []any{}, nil
	}
	return h.postprocess.ListFinalizationRuns(ctx, taskID)
}

func (h *Handler) tenantTaskByID(ctx context.Context, c *app.RequestContext, taskID int64) (domain.Task, error) {
	tenantID, ok := tenantScope(c)
	if !ok {
		return domain.Task{}, infrastructure.ErrNotFound
	}
	task, err := h.service.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if task.TenantID != tenantID {
		return domain.Task{}, infrastructure.ErrNotFound
	}
	return task, nil
}

func writePostprocessError(c *app.RequestContext, err error) {
	if errors.Is(err, postprocess.ErrNotFound) {
		writeError(c, http.StatusNotFound, "POSTPROCESS_NOT_FOUND", "post-process record was not found")
		return
	}
	if errors.Is(err, postprocess.ErrConflict) {
		writeError(c, http.StatusConflict, "POSTPROCESS_RETRY_CONFLICT", "post-process record cannot be retried from its current state")
		return
	}
	writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "post-process operation failed")
}

func (h *Handler) backlog(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED", "an unambiguous organization scope is required")
		return
	}
	item, err := h.service.Backlog(ctx, tenantID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "failed to resolve backlog")
		return
	}
	response.Success(c, item)
}

// taskCreateError is the only Create endpoint error-to-wire mapping. Unknown
// persistence/provider errors must never be rendered as client-visible prose:
// their messages can contain SQL diagnostics, configuration facts, or secret
// hints that are not part of the public Task contract.
func taskCreateError(err error) (status int, code, message string) {
	status, code, message = http.StatusInternalServerError, "TASK_CREATION_FAILED", "task creation could not be completed"
	if application.IsConflict(err) {
		return http.StatusConflict, "IDEMPOTENCY_CONFLICT", "task creation conflicts with an existing request"
	}
	if stableCode := application.ErrorCode(err); stableCode != "" {
		// RESULT_PROCESSOR_LEGACY_TASK_ACTIVE is the lone currently exposed
		// creation-state conflict. Every other documented Task creation code
		// is a caller-correctable selection or payload validation failure. In
		// particular, do not let normal priority/timeout/hook/idempotency
		// validation fall through as a 500 merely because its original prose
		// came from application code.
		if stableCode == "RESULT_PROCESSOR_LEGACY_TASK_ACTIVE" {
			return http.StatusConflict, stableCode, application.SafeErrorMessage(stableCode)
		}
		return http.StatusBadRequest, stableCode, application.SafeErrorMessage(stableCode)
	}
	return status, code, message
}

func (h *Handler) updateMetadata(ctx context.Context, c *app.RequestContext) {
	if len(c.Request.Body()) > 64<<10 {
		writeError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds 64 KiB")
		return
	}
	var request application.MetadataRequest
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "INVALID_JSON", "request body must contain exactly one JSON object")
		return
	}
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	actor, _ := securitycontext.CurrentUserID(c)
	updated, err := h.service.UpdateMetadata(ctx, task.TenantID, task.ID, actor, request)
	if err != nil {
		if errors.Is(err, domain.ErrTaskMetadataConflict) {
			response.ErrorWithStatus(c, http.StatusConflict, apperrors.ObjectState("task information changed; reload before saving").WithDetails(map[string]string{"reasonCode": "TASK_METADATA_CONFLICT"}))
			return
		}
		if code := application.ErrorCode(err); code != "" {
			writeError(c, http.StatusBadRequest, code, err.Error())
			return
		}
		writeDomainError(c, err)
		return
	}
	response.Success(c, taskCreatorResponse(updated))
}

func (h *Handler) list(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED", "an unambiguous organization scope is required")
		return
	}
	filter, err := taskListFilter(c, tenantID, 0)
	if err != nil {
		writeError(c, http.StatusBadRequest, "TASK_LIST_FILTER_INVALID", err.Error())
		return
	}
	page, err := h.service.ListPage(ctx, filter)
	if err != nil {
		writeError(c, 500, "DATABASE_ERROR", "failed to list tasks")
		return
	}
	results, err := h.taskResponsesWithSummaries(ctx, c, page.Items)
	if err != nil {
		writeError(c, 503, "INTERACTION_UNAVAILABLE", "task updates could not be loaded")
		return
	}
	response.Success(c, map[string]any{"items": results, "nextCursor": decimalCursor(page.NextCursor)})
}

func decimalCursor(value int64) any {
	if value < 1 {
		return nil
	}
	return strconv.FormatInt(value, 10)
}

func taskListFilter(c *app.RequestContext, tenantID, projectID int64) (infrastructure.ListFilter, error) {
	filter := infrastructure.ListFilter{TenantID: tenantID, ProjectID: projectID, Query: strings.TrimSpace(c.Query("query")), View: strings.TrimSpace(c.Query("view"))}
	if filter.View != "" {
		valid := map[string]bool{"all": true, "active": true, "succeeded": true, "failed": true, "cancelled": true, "attention": true, "waiting": true}
		if !valid[filter.View] {
			return filter, errors.New("view is invalid")
		}
	}
	filter.Limit, _ = strconv.Atoi(c.Query("limit"))
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			return filter, errors.New("cursor must be a positive decimal integer")
		}
		filter.Cursor = value
	}
	parse := func(name string) (*time.Time, error) {
		raw := strings.TrimSpace(c.Query(name))
		if raw == "" {
			return nil, nil
		}
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			if day, e := time.Parse("2006-01-02", raw); e == nil {
				value = day
				if name == "to" {
					value = value.Add(24 * time.Hour)
				}
				err = nil
			}
		}
		if err != nil {
			return nil, errors.New(name + " must be RFC3339 or YYYY-MM-DD")
		}
		value = value.UTC()
		return &value, nil
	}
	var err error
	if filter.From, err = parse("from"); err != nil {
		return filter, err
	}
	if filter.To, err = parse("to"); err != nil {
		return filter, err
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return filter, errors.New("from must be before to")
	}
	return filter, nil
}

func (h *Handler) get(ctx context.Context, c *app.RequestContext) {
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	items, err := h.taskResponsesWithSummaries(ctx, c, []domain.Task{task})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERACTION_SUMMARY_UNAVAILABLE", "task interaction summary could not be loaded")
		return
	}
	response.Success(c, items[0])
}

// independentReview exposes the safe, canonical business result separately
// from the generic Task status. Clients must not infer independent review from
// REVIEWING/MANUAL_REVIEW because those states are also used by other flows.
func (h *Handler) independentReview(ctx context.Context, c *app.RequestContext) {
	task, err := h.tenantTask(ctx, c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	report, err := h.service.IndependentReviewReport(ctx, task)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INDEPENDENT_REVIEW_UNAVAILABLE", "independent review result is unavailable")
		return
	}
	response.Success(c, report)
}

func (h *Handler) cancel(ctx context.Context, c *app.RequestContext) {
	if _, err := h.tenantTask(ctx, c); err != nil {
		writeDomainError(c, err)
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if len(c.Request.Body()) > 0 && json.Unmarshal(c.Request.Body(), &request) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}
	actor, _ := securitycontext.CurrentUserID(c)
	taskID, err := positivePathID(c, "id")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "task id must be a positive decimal integer")
		return
	}
	result, err := h.service.Cancel(ctx, taskID, actor, request.Reason)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	task, _ := h.tenantTask(ctx, c)
	response.Success(c, map[string]any{"task": taskCreatorResponse(task), "cancel": result})
}

func (h *Handler) events(ctx context.Context, c *app.RequestContext) {
	if _, err := h.tenantTask(ctx, c); err != nil {
		writeDomainError(c, err)
		return
	}
	after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
	taskID, err := positivePathID(c, "id")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "task id must be a positive decimal integer")
		return
	}
	events, err := h.service.Events(ctx, taskID, after)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	response.Success(c, events)
}

func (h *Handler) eventStream(ctx context.Context, c *app.RequestContext) {
	if _, err := h.tenantTask(ctx, c); err != nil {
		writeDomainError(c, err)
		return
	}
	after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
	c.Response.Header.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("X-Accel-Buffering", "no")
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		taskID, parseErr := positivePathID(c, "id")
		if parseErr != nil {
			_, _ = fmt.Fprint(c, "event: error\ndata: {\"code\":\"INVALID_ID\"}\n\n")
			_ = c.Flush()
			return
		}
		events, err := h.service.Events(ctx, taskID, after)
		if err != nil {
			_, _ = fmt.Fprintf(c, "event: error\ndata: {\"code\":\"EVENT_STREAM_ERROR\"}\n\n")
			_ = c.Flush()
			return
		}
		for _, event := range events {
			body, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(c, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, body)
			after = event.ID
		}
		if len(events) > 0 {
			_ = c.Flush()
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			_, _ = io.WriteString(c, ": reconnect\n\n")
			_ = c.Flush()
			return
		case <-ticker.C:
		}
	}
}

func (h *Handler) listArtifacts(ctx context.Context, c *app.RequestContext) {
	if _, err := h.tenantTask(ctx, c); err != nil {
		writeDomainError(c, err)
		return
	}
	taskID, err := positivePathID(c, "id")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "task id must be a positive decimal integer")
		return
	}
	items, err := h.service.Artifacts(ctx, taskID)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *Handler) downloadArtifact(ctx context.Context, c *app.RequestContext) {
	if _, err := h.tenantTask(ctx, c); err != nil {
		writeDomainError(c, err)
		return
	}
	taskID, err := positivePathID(c, "id")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "task id must be a positive decimal integer")
		return
	}
	artifactID, err := positivePathID(c, "artifactId")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "artifact id must be a positive decimal integer")
		return
	}
	item, err := h.store.Artifact(ctx, taskID, artifactID)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	object, err := h.artifacts.Get(ctx, item.ObjectKey)
	if err != nil {
		writeError(c, 502, "ARTIFACT_UNAVAILABLE", "artifact content is unavailable")
		return
	}
	defer object.Close()
	c.Response.Header.SetContentType(item.MediaType)
	c.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(item.Name, "\"", "")))
	c.SetStatusCode(consts.StatusOK)
	if _, err := io.Copy(c, object); err != nil {
		return
	}
}

func (h *Handler) tenantTask(ctx context.Context, c *app.RequestContext) (domain.Task, error) {
	tenantID, ok := tenantScope(c)
	if !ok {
		return domain.Task{}, infrastructure.ErrNotFound
	}
	taskID, parseErr := positivePathID(c, "id")
	if parseErr != nil {
		return domain.Task{}, infrastructure.ErrNotFound
	}
	task, err := h.service.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if task.TenantID != tenantID {
		return domain.Task{}, infrastructure.ErrNotFound
	}
	return task, nil
}

func tenantScope(c *app.RequestContext) (int64, bool) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	return scope.OrgID, err == nil && scope.OrgID > 0
}

func positivePathID(c *app.RequestContext, name string) (int64, error) {
	raw := c.Param(name)
	if raw == "" || strings.TrimSpace(raw) != raw || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return 0, errors.New("invalid id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func writeDomainError(c *app.RequestContext, err error) {
	if application.IsNotFound(err) {
		writeError(c, 404, "NOT_FOUND", "resource not found")
		return
	}
	if errors.Is(err, infrastructure.ErrInvalidTransition) {
		writeError(c, 409, "INVALID_STATE", "task state does not allow this operation")
		return
	}
	writeError(c, 500, "INTERNAL_ERROR", "request failed")
}

func writeError(c *app.RequestContext, status int, code, message string) {
	details := map[string]string{"reasonCode": code}
	var err *apperrors.AppError
	switch status {
	case http.StatusUnauthorized:
		err = apperrors.Unauthorized(message)
	case http.StatusForbidden:
		err = apperrors.DataScopeDenied(message)
	case http.StatusNotFound:
		err = apperrors.NotFound(message)
	case http.StatusConflict:
		err = apperrors.ObjectState(message)
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		err = apperrors.ServiceUnavailable(message)
	case http.StatusInternalServerError:
		err = apperrors.System(message)
	default:
		err = apperrors.Params(message)
	}
	response.Error(c, err.WithDetails(details))
}

// compile-time check keeps the streaming assumption explicit across Hertz upgrades.
var _ io.Writer = (*app.RequestContext)(nil)
