package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	continuationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.GET("/workforce/tasks/:taskId/turns", wrap("workforce:task:query", h.listTurns))
	router.POST("/workforce/tasks/:taskId/turns", wrap("workforce:task:continue", h.createTurn))
	router.GET("/workforce/task-turns/:turnId", wrap("workforce:task:query", h.getTurn))
	router.GET("/workforce/continuation-checkpoints/:checkpointId", wrap("workforce:task:query", h.getCheckpoint))
	router.POST("/workforce/continuation-checkpoints/:checkpointId/approvals", wrap("workforce:task:continuation:approve", h.approve))
	router.POST("/workforce/tasks/:taskId/result-materializations", wrap("workforce:result-output:materialize", h.createMaterialization))
	router.GET("/workforce/tasks/:taskId/result-materializations", wrap("workforce:task:query", h.listMaterializations))
	router.GET("/workforce/result-materialization-runs/:runId", wrap("workforce:task:query", h.getMaterialization))
	router.POST("/workforce/result-materialization-runs/:runId/retry", wrap("workforce:result-output:retry", h.retryMaterialization))
}

func (h *Handler) createMaterialization(ctx context.Context, c *app.RequestContext) {
	var request application.CreateMaterializationRequest
	if !decode(c, &request, 16<<10) {
		return
	}
	request.TaskID, request.TenantID, request.Actor = pathID(c, "taskId"), tenant(c), actor(c)
	item, replay, err := h.service.CreateMaterialization(ctx, request)
	if err != nil {
		problem(c, err)
		return
	}
	c.Response.Header.Set("Idempotency-Replayed", fmt.Sprint(replay))
	response.Success(c, item)
}

func (h *Handler) getMaterialization(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.GetMaterialization(ctx, tenant(c), pathID(c, "runId"))
	if err != nil {
		problem(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) listMaterializations(ctx context.Context, c *app.RequestContext) {
	items, err := h.service.ListMaterializations(ctx, tenant(c), pathID(c, "taskId"))
	if err != nil {
		problem(c, err)
		return
	}
	response.Success(c, items)
}

func (h *Handler) retryMaterialization(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Reason string `json:"reason"`
	}
	if !decode(c, &request, 8<<10) {
		return
	}
	if err := h.service.RetryMaterialization(ctx, tenant(c), actor(c), pathID(c, "runId"), request.Reason); err != nil {
		problem(c, err)
		return
	}
	response.Success(c, map[string]any{"accepted": true})
}

func (h *Handler) createTurn(ctx context.Context, c *app.RequestContext) {
	var request application.CreateNextTurnRequest
	if !decode(c, &request, 32<<10) {
		return
	}
	creatorUserID, ok := securitycontext.CurrentUserID(c)
	if !ok {
		problem(c, domain.ErrContinuationIdentity)
		return
	}
	request.TaskID, request.TenantID, request.CreatorUserID = pathID(c, "taskId"), tenant(c), creatorUserID
	item, replay, err := h.service.CreateNextTurn(ctx, request)
	if err != nil {
		problem(c, err)
		return
	}
	c.Response.Header.Set("Idempotency-Replayed", fmt.Sprint(replay))
	response.Success(c, item)
}

func (h *Handler) approve(ctx context.Context, c *app.RequestContext) {
	var request application.ApproveRequest
	if !decode(c, &request, 8<<10) {
		return
	}
	request.CheckpointID, request.TenantID, request.Actor = pathID(c, "checkpointId"), tenant(c), actor(c)
	item, err := h.service.ApproveManualReviewContinuation(ctx, request)
	if err != nil {
		problem(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) listTurns(ctx context.Context, c *app.RequestContext) {
	items, err := h.service.ListTurns(ctx, tenant(c), pathID(c, "taskId"))
	if err != nil {
		problem(c, err)
		return
	}
	for index := range items {
		items[index].Instruction = continuationfacade.PublicInstruction(items[index].Instruction)
	}
	response.Success(c, items)
}

func (h *Handler) getTurn(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.GetTurn(ctx, tenant(c), pathID(c, "turnId"))
	if err != nil {
		problem(c, err)
		return
	}
	item.Instruction = continuationfacade.PublicInstruction(item.Instruction)
	response.Success(c, item)
}

func (h *Handler) getCheckpoint(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.GetCheckpoint(ctx, tenant(c), pathID(c, "checkpointId"))
	if err != nil {
		problem(c, err)
		return
	}
	response.Success(c, item)
}

func decode(c *app.RequestContext, target any, limit int) bool {
	if len(c.Request.Body()) > limit {
		write(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request is too large")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		write(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return false
	}
	if decoder.Decode(&struct{}{}) == nil {
		write(c, http.StatusBadRequest, "INVALID_REQUEST", "trailing JSON is not allowed")
		return false
	}
	return true
}

func tenant(c *app.RequestContext) int64 {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil {
		return 0
	}
	return scope.OrgID
}

func pathID(c *app.RequestContext, name string) int64 {
	value, err := strconv.ParseInt(string(c.Param(name)), 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}
func actor(c *app.RequestContext) int64 {
	if id, ok := securitycontext.CurrentUserID(c); ok {
		return id
	}
	return 0
}

func problem(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, continuationinfra.ErrNotFound):
		write(c, http.StatusNotFound, "CONTINUATION_NOT_FOUND", "continuation resource was not found")
	case errors.Is(err, domain.ErrTaskTurnConflict), errors.Is(err, domain.ErrTaskTurnAlreadyActive), errors.Is(err, domain.ErrContinuationNotAllowed), errors.Is(err, domain.ErrCheckpointUnavailable), errors.Is(err, domain.ErrCheckpointExpired), errors.Is(err, domain.ErrManualApprovalRequired), errors.Is(err, domain.ErrIdempotencyConflict):
		write(c, http.StatusConflict, err.Error(), "continuation state does not allow this operation")
	case errors.Is(err, domain.ErrContinuationInstruction):
		write(c, http.StatusBadRequest, err.Error(), "continuation request is invalid")
	case errors.Is(err, domain.ErrMaterializationInvalid):
		write(c, http.StatusConflict, err.Error(), "result output cannot be generated from the current task state")
	case errors.Is(err, domain.ErrMaterializationExpired):
		write(c, http.StatusConflict, err.Error(), "the saved result needed to generate this output has expired")
	default:
		write(c, http.StatusInternalServerError, "CONTINUATION_INTERNAL_ERROR", "continuation request failed")
	}
}

func write(c *app.RequestContext, status int, reason, message string) {
	var err *apperrors.AppError
	switch status {
	case http.StatusBadRequest:
		err = apperrors.Params(message)
	case http.StatusNotFound:
		err = apperrors.NotFound(message)
	case http.StatusConflict:
		err = apperrors.ObjectState(message)
	default:
		err = apperrors.System(message)
	}
	err = err.WithDetails(map[string]string{"reasonCode": reason})
	response.Error(c, err)
}
