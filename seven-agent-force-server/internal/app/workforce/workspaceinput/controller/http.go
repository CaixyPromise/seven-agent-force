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

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Handler struct{ service *application.Service }
type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func New(service *application.Service) *Handler { return &Handler{service: service} }
func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.POST("/workforce/workspace-sources/uploads", wrap("workforce:source:create", h.uploadSource))
	router.GET("/workforce/workspace-source-revisions/:id", wrap("workforce:source:query", h.sourceRevision))
	router.GET("/workforce/source-ingestion-runs/:id", wrap("workforce:source:query", h.ingestionRun))
	router.POST("/workforce/reference-assets", wrap("workforce:reference:create", h.reference))
	router.GET("/workforce/reference-assets/:id", wrap("workforce:source:query", h.referenceView))
	router.GET("/workforce/tasks/:id/input", wrap("workforce:task:query", h.taskInput))
	router.GET("/workforce/tasks/:id/git-plan", wrap("workforce:task:query", h.gitPlan))
}
func (h *Handler) taskInput(ctx context.Context, c *app.RequestContext) {
	ctx = authenticatedContext(ctx, c)
	item, err := h.service.GetTaskInput(ctx, tenant(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) gitPlan(ctx context.Context, c *app.RequestContext) {
	ctx = authenticatedContext(ctx, c)
	item, err := h.service.GetGitPlan(ctx, tenant(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) uploadSource(ctx context.Context, c *app.RequestContext) {
	ctx = authenticatedContext(ctx, c)
	var request application.UploadSourceRequest
	if !decode(c, &request, 32<<10) {
		return
	}
	revision, run, replayed, err := h.service.CreateUploadedSource(ctx, tenant(c), actor(c), request)
	if err == nil {
		c.Response.Header.Set("Idempotency-Replayed", fmt.Sprint(replayed))
	}
	write(c, map[string]any{"sourceRevision": revision, "ingestionRun": run}, err)
}
func (h *Handler) sourceRevision(ctx context.Context, c *app.RequestContext) {
	ctx = authenticatedContext(ctx, c)
	item, err := h.service.GetRevision(ctx, tenant(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) ingestionRun(ctx context.Context, c *app.RequestContext) {
	ctx = authenticatedContext(ctx, c)
	item, err := h.service.GetRun(ctx, tenant(c), pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) reference(ctx context.Context, c *app.RequestContext) {
	ctx = authenticatedContext(ctx, c)
	var request application.ReferenceRequest
	if !decode(c, &request, 32<<10) {
		return
	}
	item, replayed, err := h.service.CreateReference(ctx, tenant(c), actor(c), request)
	if err == nil {
		c.Response.Header.Set("Idempotency-Replayed", fmt.Sprint(replayed))
	}
	write(c, item, err)
}
func (h *Handler) referenceView(ctx context.Context, c *app.RequestContext) {
	ctx = authenticatedContext(ctx, c)
	item, err := h.service.GetReference(ctx, tenant(c), pathID(c, "id"))
	write(c, item, err)
}

func authenticatedContext(ctx context.Context, c *app.RequestContext) context.Context {
	return securitycontext.WithUser(ctx, securitycontext.Get(c))
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
	case errors.Is(err, domain.ErrTaskInputInvalid), errors.Is(err, domain.ErrSourceNotAvailable), errors.Is(err, domain.ErrReferenceNotAvailable):
		problem(c, http.StatusBadRequest, err.Error())
	case err.Error() == "IDEMPOTENCY_CONFLICT":
		problem(c, http.StatusConflict, err.Error())
	default:
		problem(c, http.StatusInternalServerError, err.Error())
	}
}
func problem(c *app.RequestContext, status int, reason string) {
	var e *apperrors.AppError
	switch status {
	case http.StatusBadRequest:
		e = apperrors.Params("任务输入不合法")
	case http.StatusNotFound:
		e = apperrors.NotFound("任务输入不存在")
	case http.StatusConflict:
		e = apperrors.ObjectState("任务输入请求冲突")
	default:
		e = apperrors.System("任务输入操作失败")
	}
	response.Error(c, e.WithDetails(map[string]string{"reasonCode": reason}))
}
