package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	routing "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	project "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	tf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	a "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/application"
	d "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
	"io"
	"strconv"
	"strings"
)

type Handler struct{ service *a.Service }

func New(s *a.Service) *Handler { return &Handler{s} }

type Wrap func(string, app.HandlerFunc) app.HandlerFunc

func (h *Handler) Mount(r route.IRouter, wrap Wrap) {
	r.POST("/workforce/projects/:projectId/work-items", wrap("workforce:work-item:create", wrap("workforce:task:create", h.create)))
	r.GET("/workforce/projects/:projectId/work-items", wrap("workforce:work-item:query", h.list))
	r.GET("/workforce/work-items/:id", wrap("workforce:work-item:query", h.get))
	r.POST("/workforce/work-items/:id/plan-decisions", wrap("workforce:work-item:plan:approve", wrap("workforce:task:create", h.approve)))
	r.POST("/workforce/work-items/:id/plan-edits", wrap("workforce:work-item:plan:approve", h.edit))
	r.POST("/workforce/work-items/:id/plan-revisions", wrap("workforce:work-item:plan:approve", wrap("workforce:task:create", h.revise)))
	r.POST("/workforce/work-items/:id/cancel", wrap("workforce:work-item:cancel", h.cancel))
}
func identity(c *app.RequestContext) (int64, int64, bool) {
	scope, e := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	actor, ok := securitycontext.CurrentUserID(c)
	if e != nil || scope.OrgID < 1 || !ok || actor < 1 {
		write(c, nil, d.ErrNotFound)
		return 0, 0, false
	}
	return scope.OrgID, actor, true
}
func id(c *app.RequestContext, k string) int64 {
	s := c.Param(k)
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 1 || strconv.FormatInt(n, 10) != s {
		return 0
	}
	return n
}
func decode(c *app.RequestContext, v any) bool {
	b := c.Request.Body()
	if len(b) > 256*1024 {
		write(c, nil, d.ErrInvalid)
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil || dec.Decode(new(any)) != io.EOF {
		write(c, nil, d.ErrInvalid)
		return false
	}
	return true
}
func key(c *app.RequestContext) string {
	return strings.TrimSpace(string(c.GetHeader("Idempotency-Key")))
}
func (h *Handler) create(ctx context.Context, c *app.RequestContext) {
	t, u, ok := identity(c)
	if !ok {
		return
	}
	var r d.Create
	if !decode(c, &r) {
		return
	}
	i, e := h.service.Create(ctx, t, u, id(c, "projectId"), key(c), r)
	filter(c, &i)
	write(c, i, e)
}
func (h *Handler) get(ctx context.Context, c *app.RequestContext) {
	t, _, ok := identity(c)
	if !ok {
		return
	}
	i, e := h.service.Get(ctx, t, id(c, "id"))
	filter(c, &i)
	write(c, i, e)
}
func (h *Handler) list(ctx context.Context, c *app.RequestContext) {
	t, _, ok := identity(c)
	if !ok {
		return
	}
	items, e := h.service.List(ctx, t, id(c, "projectId"))
	for n := range items {
		filter(c, &items[n])
	}
	write(c, map[string]any{"items": items}, e)
}
func (h *Handler) approve(ctx context.Context, c *app.RequestContext) {
	t, u, ok := identity(c)
	if !ok {
		return
	}
	var r a.Approve
	if !decode(c, &r) {
		return
	}
	i, e := h.service.Approve(ctx, t, u, id(c, "id"), key(c), r)
	filter(c, &i)
	write(c, i, e)
}
func (h *Handler) edit(ctx context.Context, c *app.RequestContext) {
	t, u, ok := identity(c)
	if !ok {
		return
	}
	var r a.EditPlan
	if !decode(c, &r) {
		return
	}
	i, e := h.service.Edit(ctx, t, u, id(c, "id"), key(c), r)
	filter(c, &i)
	write(c, i, e)
}
func (h *Handler) revise(ctx context.Context, c *app.RequestContext) {
	t, u, ok := identity(c)
	if !ok {
		return
	}
	var r a.RevisePlan
	if !decode(c, &r) {
		return
	}
	i, e := h.service.Revise(ctx, t, u, id(c, "id"), key(c), r)
	filter(c, &i)
	write(c, i, e)
}
func (h *Handler) cancel(ctx context.Context, c *app.RequestContext) {
	t, u, ok := identity(c)
	if !ok {
		return
	}
	var r struct {
		ExpectedVersion int64 `json:"expectedVersion,string"`
	}
	if !decode(c, &r) {
		return
	}
	i, e := h.service.Cancel(ctx, t, u, id(c, "id"), r.ExpectedVersion)
	filter(c, &i)
	write(c, i, e)
}
func filter(c *app.RequestContext, item *d.Item) {
	if !securitycontext.HasPermission(c, "workforce:model-credential:usage") {
		item.Usage = nil
	}
	allowed := item.AvailableActions[:0]
	for _, action := range item.AvailableActions {
		if action == "CANCEL" && securitycontext.HasPermission(c, "workforce:work-item:cancel") {
			allowed = append(allowed, action)
		}
		if action == "EDIT_PLAN" && securitycontext.HasPermission(c, "workforce:work-item:plan:approve") {
			allowed = append(allowed, action)
		}
		if (action == "APPROVE_PLAN" || action == "REVISE_PLAN") && securitycontext.HasPermission(c, "workforce:work-item:plan:approve") && securitycontext.HasPermission(c, "workforce:task:create") {
			allowed = append(allowed, action)
		}
	}
	item.AvailableActions = allowed
}
func write(c *app.RequestContext, v any, e error) {
	if e == nil {
		response.Success(c, v)
		return
	}
	status, code := 503, "WORK_ITEM_UNAVAILABLE"
	switch {
	case errors.Is(e, d.ErrNotFound):
		status, code = 404, e.Error()
	case errors.Is(e, d.ErrConflict):
		status, code = 409, e.Error()
	case errors.Is(e, d.ErrInvalid), errors.Is(e, d.ErrPlan):
		status, code = 422, e.Error()
	case errors.Is(e, tf.ErrPlanningHooksUnsupported):
		status, code = 422, e.Error()
	case errors.Is(e, project.ErrArchived), errors.Is(e, project.ErrConflict), errors.Is(e, project.ErrSourceUnavailable):
		status, code = 409, e.Error()
	case errors.Is(e, project.ErrInvalid), errors.Is(e, routing.ErrRuntimeSelectionRequired), errors.Is(e, routing.ErrRuntimeSelectionUnavailable), errors.Is(e, routing.ErrRuntimeSelectionIncompatible):
		status, code = 422, e.Error()
	}
	var publicErr error
	switch status {
	case 404:
		publicErr = apperrors.NotFound("工作单不存在")
	case 409:
		publicErr = apperrors.ObjectState("工作单状态已变化")
	case 422:
		publicErr = apperrors.Params("工作单配置无效")
	default:
		publicErr = apperrors.ServiceUnavailable("工作单服务暂时不可用")
	}
	response.ErrorWithReason(c, status, publicErr, code, nil)
}
