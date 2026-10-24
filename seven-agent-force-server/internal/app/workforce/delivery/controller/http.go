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

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Handler struct{ service *application.Service }
type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func New(service *application.Service) *Handler { return &Handler{service: service} }
func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.GET("/workforce/tasks/:taskId/deliveries", wrap("workforce:task:query", h.listRequests))
	router.POST("/workforce/tasks/:taskId/delivery-requests", wrap("workforce:delivery-target:operate", h.createRequest))
	router.POST("/workforce/tasks/:taskId/delivery-requests/:requestId/decisions", wrap("workforce:delivery:approve", h.decide))
	router.GET("/workforce/delivery-requests/:id", wrap("workforce:task:query", h.getRequest))
	router.POST("/workforce/delivery-requests/:id/reauthorizations", wrap("workforce:delivery:approve", h.reauthorizeRequest))
	router.POST("/workforce/delivery-requests/:id/cancel", wrap("workforce:delivery:cancel", h.cancel))
	router.POST("/workforce/delivery-requests/:id/retry", wrap("workforce:delivery:retry", h.retry))
	router.POST("/workforce/delivery-requests/:id/reconcile", wrap("workforce:delivery:reconcile", h.reconcile))
	router.POST("/workforce/review-operations/:id/reconcile", wrap("workforce:delivery:reconcile", h.reconcileReview))
	router.POST("/workforce/review-operations/:id/reauthorizations", wrap("workforce:delivery:approve", h.reauthorizeReview))
	router.GET("/workforce/delivery-attempts/:id", wrap("workforce:task:query", h.getAttempt))
}
func (h *Handler) listRequests(ctx context.Context, c *app.RequestContext) {
	items, err := h.service.ListRequests(ctx, tenant(c), pathID(c, "taskId"))
	write(c, items, err)
}
func (h *Handler) getRequest(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.GetRequest(ctx, tenant(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) getAttempt(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.GetAttempt(ctx, tenant(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) createRequest(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Action         domain.Action `json:"action"`
		ResultCommit   string        `json:"resultCommit"`
		IdempotencyKey string        `json:"idempotencyKey"`
	}
	if !decode(c, &body, 64<<10) {
		return
	}
	var item application.RequestView
	var replay bool
	var err error
	if body.ResultCommit != "" {
		if body.Action.Type != "" || body.Action.When != "" || body.Action.TargetID != 0 || body.Action.BaseBranch != "" || body.Action.Branch != "" || body.Action.Operation != "" || len(body.Action.Payload) != 0 {
			problem(c, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		if !securitycontext.HasPermission(c, "workforce:delivery:approve") {
			response.Error(c, apperrors.PermissionDenied("workforce:delivery:approve"))
			return
		}
		item, replay, err = h.service.Restart(ctx, application.Restart{TaskID: pathID(c, "taskId"), TenantID: tenant(c), Actor: actor(c), ResultCommit: body.ResultCommit, IdempotencyKey: body.IdempotencyKey})
	} else {
		item, replay, err = h.service.CreateRequest(ctx, application.CreateRequest{TaskID: pathID(c, "taskId"), TenantID: tenant(c), Actor: actor(c), IdempotencyKey: body.IdempotencyKey, Action: body.Action})
	}
	if err == nil {
		c.Response.Header.Set("Idempotency-Replayed", fmt.Sprint(replay))
	}
	write(c, item, err)
}
func (h *Handler) decide(ctx context.Context, c *app.RequestContext) {
	var body struct {
		ExpectedVersion int64  `json:"expectedVersion,string"`
		Action          string `json:"action"`
		Reason          string `json:"reason"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if !decode(c, &body, 16<<10) {
		return
	}
	item, replayed, err := h.service.Decide(ctx, application.Decision{TenantID: tenant(c), TaskID: pathID(c, "taskId"), RequestID: pathID(c, "requestId"), Actor: actor(c), ExpectedVersion: body.ExpectedVersion, Action: body.Action, Reason: body.Reason, IdempotencyKey: body.IdempotencyKey})
	if err == nil {
		c.Response.Header.Set("Idempotency-Replayed", fmt.Sprint(replayed))
	}
	write(c, item, err)
}
func (h *Handler) reauthorizeRequest(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Reason           string `json:"reason"`
		ProjectVersionID int64  `json:"projectVersionId,string"`
	}
	if !decode(c, &body, 8<<10) {
		return
	}
	item, err := h.service.ReauthorizeRequest(ctx, application.Reauthorization{TenantID: tenant(c), RequestID: pathID(c, "id"), Actor: actor(c), Reason: body.Reason, ProjectVersionID: body.ProjectVersionID})
	write(c, item, err)
}
func (h *Handler) reauthorizeReview(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Reason string `json:"reason"`
	}
	if !decode(c, &body, 8<<10) {
		return
	}
	item, err := h.service.ReauthorizeReview(ctx, application.Reauthorization{TenantID: tenant(c), OperationID: pathID(c, "id"), Actor: actor(c), Reason: body.Reason})
	write(c, item, err)
}
func (h *Handler) cancel(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.Cancel(ctx, tenant(c), actor(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) retry(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.Retry(ctx, tenant(c), actor(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) reconcile(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.Reconcile(ctx, tenant(c), actor(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) reconcileReview(ctx context.Context, c *app.RequestContext) {
	item, err := h.service.ReconcileReview(ctx, tenant(c), actor(c), pathID(c, "id"))
	write(c, item, err)
}
func decode(c *app.RequestContext, target any, limit int) bool {
	if len(c.Request.Body()) > limit {
		problem(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		problem(c, http.StatusBadRequest, "INVALID_REQUEST")
		return false
	}
	if decoder.Decode(&struct{}{}) == nil {
		problem(c, http.StatusBadRequest, "INVALID_REQUEST")
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
func write(c *app.RequestContext, value any, err error) {
	if err == nil {
		response.Success(c, value)
		return
	}
	switch {
	case errors.Is(err, application.ErrNotFound):
		problem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidTarget):
		problem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrPolicyDenied), errors.Is(err, domain.ErrApprovalRequired), errors.Is(err, domain.ErrIdentityConflict), errors.Is(err, domain.ErrRemoteChanged), errors.Is(err, domain.ErrBaseChanged), errors.Is(err, domain.ErrBranchCreateForbidden):
		problem(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrRequestStale), errors.Is(err, domain.ErrDecisionConflict), errors.Is(err, domain.ErrDecisionNotAllowed):
		problem(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrIntentInvalid):
		problem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrIntentNotAvailable), errors.Is(err, domain.ErrResultNotCurrent), errors.Is(err, domain.ErrReauthorization):
		problem(c, http.StatusConflict, err.Error())
	case errors.Is(err, application.ErrReconciliationNotAllowed), errors.Is(err, application.ErrReconciliationInconclusive), errors.Is(err, application.ErrReconciliationEvidenceMismatch):
		problem(c, http.StatusConflict, err.Error())
	default:
		problem(c, http.StatusInternalServerError, "DELIVERY_INTERNAL_ERROR")
	}
}
func problem(c *app.RequestContext, status int, reason string) {
	var err *apperrors.AppError
	switch status {
	case http.StatusBadRequest:
		err = apperrors.Params("交付配置不合法")
	case http.StatusNotFound:
		err = apperrors.NotFound("交付资源不存在")
	case http.StatusConflict:
		err = apperrors.ObjectState("当前状态不允许此交付操作")
	default:
		err = apperrors.System("交付操作失败")
	}
	response.Error(c, err.WithDetails(map[string]string{"reasonCode": reason}))
}
