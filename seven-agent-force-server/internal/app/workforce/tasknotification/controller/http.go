package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc
type UpdateWrapper func(app.HandlerFunc) app.HandlerFunc

func (h *Handler) Mount(router route.IRouter, query PermissionWrapper, update UpdateWrapper) {
	router.GET("/workforce/task-notifications/readiness", query("workforce:task:query", h.readiness))
	router.GET("/workforce/tasks/:taskId/notification-subscription", query("workforce:task:query", h.getSubscription))
	router.PUT("/workforce/tasks/:taskId/notification-subscription", update(h.updateSubscription))
	router.GET("/workforce/tasks/:taskId/notification-records", query("workforce:task:query", h.listRecords))
}

func (h *Handler) readiness(ctx context.Context, c *app.RequestContext) {
	userID, ok := securitycontext.CurrentUserID(c)
	if !ok || userID <= 0 {
		writeError(c, http.StatusUnauthorized, "TASK_NOTIFICATION_RECIPIENT_INVALID", "an authenticated user identity is required")
		return
	}
	organization, scopeErr := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if scopeErr != nil || organization.OrgID <= 0 {
		writeError(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED", "an unambiguous organization scope is required")
		return
	}
	item, err := h.service.ResolveReadiness(ctx, organization.OrgID, userID)
	writeResult(c, item, err)
}

func (h *Handler) getSubscription(ctx context.Context, c *app.RequestContext) {
	tenantID, actorID, taskID, ok := scope(c)
	if !ok {
		return
	}
	item, err := h.service.GetSubscription(ctx, tenantID, taskID, actorID, securitycontext.HasPermission(c, "workforce:task-notification:operate"))
	writeResult(c, item, err)
}

func (h *Handler) updateSubscription(ctx context.Context, c *app.RequestContext) {
	tenantID, actorID, taskID, ok := scope(c)
	if !ok {
		return
	}
	if len(c.Request.Body()) > 64<<10 {
		writeError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds 64 KiB")
		return
	}
	var request struct {
		Enabled         bool                      `json:"enabled"`
		Events          []domain.FactType         `json:"events"`
		Channels        []domain.ChannelSelection `json:"channels"`
		ExpectedVersion int64                     `json:"expectedVersion,string"`
	}
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
	item, err := h.service.UpdateSubscription(ctx, tenantID, taskID, actorID, request.ExpectedVersion,
		securitycontext.HasPermission(c, "workforce:task-notification:operate"),
		domain.SubscriptionCommand{Enabled: request.Enabled, Events: request.Events, Channels: request.Channels})
	writeResult(c, item, err)
}

func (h *Handler) listRecords(ctx context.Context, c *app.RequestContext) {
	tenantID, actorID, taskID, ok := scope(c)
	if !ok {
		return
	}
	items, err := h.service.ListRecords(ctx, tenantID, taskID, actorID, securitycontext.HasPermission(c, "workforce:task-notification:operate"))
	writeResult(c, items, err)
}

func scope(c *app.RequestContext) (int64, int64, int64, bool) {
	organization, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil || organization.OrgID <= 0 {
		writeError(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED", "an unambiguous organization scope is required")
		return 0, 0, 0, false
	}
	actorID, ok := securitycontext.CurrentUserID(c)
	if !ok || actorID <= 0 {
		writeError(c, http.StatusUnauthorized, "TASK_NOTIFICATION_RECIPIENT_INVALID", "an authenticated user identity is required")
		return 0, 0, 0, false
	}
	raw := string(c.Param("taskId"))
	if raw == "" || strings.TrimSpace(raw) != raw || (len(raw) > 1 && raw[0] == '0') {
		writeError(c, http.StatusBadRequest, "TASK_NOTIFICATION_FACT_INVALID", "taskId must be a positive decimal int64")
		return 0, 0, 0, false
	}
	taskID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || taskID <= 0 {
		writeError(c, http.StatusBadRequest, "TASK_NOTIFICATION_FACT_INVALID", "taskId must be a positive decimal int64")
		return 0, 0, 0, false
	}
	return organization.OrgID, actorID, taskID, true
}

func writeResult(c *app.RequestContext, value any, err error) {
	if err == nil {
		response.Success(c, value)
		return
	}
	switch {
	case errors.Is(err, infrastructure.ErrNotFound):
		writeError(c, http.StatusNotFound, "TASK_NOTIFICATION_NOT_FOUND", "Task notification settings were not found")
	case errors.Is(err, application.ErrForbidden):
		response.Error(c, apperrors.PermissionDenied("workforce:task-notification:operate"))
	case errors.Is(err, domain.ErrSubscriptionConflict):
		writeError(c, http.StatusConflict, "TASK_NOTIFICATION_SUBSCRIPTION_CONFLICT", "settings changed; reload before saving")
	case errors.Is(err, domain.ErrFactInvalid), errors.Is(err, domain.ErrRecipientInvalid):
		writeError(c, http.StatusBadRequest, domain.ErrorCode(err), "Task notification settings are invalid")
	default:
		writeError(c, http.StatusInternalServerError, "DATABASE_ERROR", "Task notification operation failed")
	}
}

func writeError(c *app.RequestContext, status int, code, message string) {
	response.ErrorWithStatus(c, status, apperrors.ObjectState(message).WithDetails(map[string]string{"reasonCode": code}))
}
