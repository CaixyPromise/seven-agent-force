package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Handler struct{ service *application.Service }
type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func New(service *application.Service) *Handler { return &Handler{service: service} }
func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.GET("/workforce/hook-packages", wrap("workforce:hook-package:list", h.listPackages))
	router.POST("/workforce/hook-packages", wrap("workforce:hook-package:create", h.createPackage))
	router.GET("/workforce/hook-packages/:id", wrap("workforce:hook-package:query", h.getPackage))
	router.POST("/workforce/hook-packages/:id/revisions", wrap("workforce:hook-package:update", h.createRevision))
	router.PUT("/workforce/hook-revisions/:id", wrap("workforce:hook-package:update", h.updateRevision))
	router.POST("/workforce/hook-revisions/:id/archive-import", wrap("workforce:hook-package:update", h.importArchive))
	router.POST("/workforce/hook-revisions/:id/validate", wrap("workforce:hook-package:validate", h.validate))
	router.POST("/workforce/hook-revisions/:id/test-runs", wrap("workforce:hook-package:test", h.test))
	router.POST("/workforce/hook-revisions/:id/publish", wrap("workforce:hook-package:publish", h.publish))
	router.GET("/workforce/hook-package-releases", wrap("workforce:hook-package:list", h.listReleases))
	router.GET("/workforce/hook-audit", wrap("workforce:hook-audit:list", h.listAudit))
	router.GET("/workforce/harness-channels", wrap("workforce:result-processor:author", h.listHarnessChannels))
}

func (h *Handler) listHarnessChannels(ctx context.Context, c *app.RequestContext) {
	items, err := h.service.ListHarnessChannels(ctx)
	write(c, items, err)
}
func (h *Handler) createRevision(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	item, err := h.service.CreateRevision(ctx, tenant, pathID(c, "id"), actor(c))
	write(c, item, err)
}
func (h *Handler) listPackages(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	items, err := h.service.ListPackages(ctx, tenant)
	write(c, items, err)
}
func (h *Handler) getPackage(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	item, err := h.service.GetPackage(ctx, tenant, pathID(c, "id"))
	write(c, item, err)
}
func (h *Handler) createPackage(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	var request application.CreatePackageCommand
	if !decode(c, &request, 1<<20) {
		return
	}
	item, err := h.service.CreatePackage(ctx, tenant, actor(c), request)
	write(c, item, err)
}
func (h *Handler) updateRevision(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64               `json:"expectedRevision,string"`
		Manifest         domain.Manifest     `json:"manifest"`
		Files            []domain.SourceFile `json:"files"`
	}
	if !decode(c, &request, 8<<20) {
		return
	}
	item, err := h.service.UpdateRevision(ctx, tenant, pathID(c, "id"), request.ExpectedRevision, request.Manifest, request.Files)
	write(c, item, err)
}
func (h *Handler) importArchive(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	expected, _ := strconv.ParseInt(c.Query("expectedRevision"), 10, 64)
	body := c.Request.Body()
	if len(body) > domain.MaxArchiveBytes {
		problem(c, http.StatusRequestEntityTooLarge, "HOOK_ARCHIVE_TOO_LARGE", "archive exceeds 8 MiB")
		return
	}
	item, err := h.service.ImportArchive(ctx, tenant, pathID(c, "id"), expected, body)
	write(c, item, err)
}
func (h *Handler) validate(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	result, err := h.service.Validate(ctx, tenant, pathID(c, "id"))
	write(c, result, err)
}
func (h *Handler) test(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	var request application.TestCommand
	if !decode(c, &request, 1<<20) {
		return
	}
	result, err := h.service.Test(ctx, tenant, pathID(c, "id"), actor(c), request)
	write(c, result, err)
}
func (h *Handler) publish(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	var request struct {
		Version string `json:"version"`
	}
	if !decode(c, &request, 4096) {
		return
	}
	result, err := h.service.Publish(ctx, tenant, pathID(c, "id"), actor(c), request.Version)
	write(c, result, err)
}
func (h *Handler) listReleases(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	items, err := h.service.ListReleases(ctx, tenant)
	write(c, items, err)
}
func (h *Handler) listAudit(ctx context.Context, c *app.RequestContext) {
	tenant, ok := scope(c)
	if !ok {
		return
	}
	items, err := h.service.ListAudit(ctx, tenant)
	write(c, items, err)
}
func scope(c *app.RequestContext) (int64, bool) {
	value, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil || value.OrgID < 1 {
		problem(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return 0, false
	}
	return value.OrgID, true
}

func pathID(c *app.RequestContext, name string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(string(c.Param(name))), 10, 64)
	if err != nil || value < 1 {
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
func decode(c *app.RequestContext, target any, max int) bool {
	if len(c.Request.Body()) > max {
		problem(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds the endpoint limit")
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		problem(c, http.StatusBadRequest, "INVALID_JSON", "request body is not valid for this operation")
		return false
	}
	return true
}
func write(c *app.RequestContext, value any, err error) {
	if err == nil {
		response.Success(c, value)
		return
	}
	switch {
	case errors.Is(err, infrastructure.ErrNotFound):
		problem(c, 404, "NOT_FOUND", "Hook resource not found")
	case errors.Is(err, infrastructure.ErrConflict):
		problem(c, 409, "HOOK_REVISION_CONFLICT", err.Error())
	default:
		problem(c, 400, "HOOK_REQUEST_INVALID", err.Error())
	}
}
func problem(c *app.RequestContext, status int, code, message string) {
	details := map[string]string{"reasonCode": code}
	var err *apperrors.AppError
	switch status {
	case 403:
		err = apperrors.DataScopeDenied(message)
	case 404:
		err = apperrors.NotFound(message)
	case 409:
		err = apperrors.ObjectState(message)
	case 500:
		err = apperrors.System(message)
	default:
		err = apperrors.Params(message)
	}
	response.Error(c, err.WithDetails(details))
}
