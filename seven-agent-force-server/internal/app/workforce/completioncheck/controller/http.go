package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
	"io"
	"net/http"
	"strconv"
)

type Handler struct {
	service   *application.Service
	revisions *application.RevisionService
}

func New(s *application.Service) *Handler                                { return &Handler{service: s} }
func (h *Handler) WithRevisions(s *application.RevisionService) *Handler { h.revisions = s; return h }

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func (h *Handler) Mount(r route.IRouter, wrap PermissionWrapper) {
	r.GET("/workforce/tasks/:taskId/completion-check", wrap("workforce:task:query", h.get))
	r.POST("/workforce/tasks/:taskId/completion-check-decisions", wrap("workforce:task:continue", h.decide))
	r.POST("/workforce/tasks/:taskId/completion-check-revisions", wrap("workforce:task:continue", h.requestRevision))
	r.GET("/workforce/tasks/:taskId/completion-check-revisions/pending", wrap("workforce:task:query", h.pendingRevision))
	r.GET("/workforce/tasks/:taskId/completion-check-revisions", wrap("workforce:task:query", h.revisionView))
}
func scope(c *app.RequestContext) (int64, int64, int64, error) {
	org, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil {
		return 0, 0, 0, facade.ErrNotFound
	}
	user, ok := securitycontext.CurrentUserID(c)
	if !ok {
		return 0, 0, 0, facade.ErrNotFound
	}
	task, err := strconv.ParseInt(c.Param("taskId"), 10, 64)
	if err != nil || task <= 0 {
		return 0, 0, 0, facade.ErrInvalid
	}
	return org.OrgID, task, user, nil
}
func (h *Handler) get(ctx context.Context, c *app.RequestContext) {
	tenant, task, _, err := scope(c)
	if err != nil {
		problem(c, err)
		return
	}
	value, err := h.service.Summary(ctx, tenant, task)
	if err != nil {
		problem(c, err)
		return
	}
	response.Success(c, value)
}
func (h *Handler) decide(ctx context.Context, c *app.RequestContext) {
	tenant, task, user, err := scope(c)
	if err != nil {
		problem(c, err)
		return
	}
	if len(c.Request.Body()) > 128<<10 {
		problem(c, facade.ErrInvalid)
		return
	}
	var d application.Decision
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&d); err != nil {
		problem(c, facade.ErrInvalid)
		return
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		problem(c, facade.ErrInvalid)
		return
	}
	d.TenantID, d.TaskID, d.ActorID = tenant, task, user
	result, err := h.service.Decide(ctx, d)
	if err != nil {
		problem(c, err)
		return
	}
	c.Response.Header.Set("Idempotency-Replayed", fmt.Sprint(result.Replayed))
	response.Success(c, result)
}
func problem(c *app.RequestContext, err error) {
	status, code := http.StatusInternalServerError, "COMPLETION_CHECK_UNAVAILABLE"
	publicErr := error(apperrors.System("完成检查暂时不可用"))
	switch {
	case errors.Is(err, facade.ErrRevisionUnavailable):
		status, code = http.StatusServiceUnavailable, facade.ErrRevisionUnavailable.Error()
		publicErr = apperrors.ServiceUnavailable("完成检查暂时不可用")
	case errors.Is(err, facade.ErrStale):
		status, code = http.StatusConflict, "COMPLETION_CHECK_STALE"
		publicErr = apperrors.ObjectState("完成检查状态已更新")
	case errors.Is(err, facade.ErrActionNotAllowed):
		status, code = http.StatusConflict, "COMPLETION_CHECK_ACTION_NOT_ALLOWED"
		publicErr = apperrors.ObjectState("当前不能执行该完成检查操作")
	case errors.Is(err, facade.ErrIdempotencyConflict):
		status, code = http.StatusConflict, "COMPLETION_CHECK_IDEMPOTENCY_CONFLICT"
		publicErr = apperrors.ObjectState("完成检查请求与已提交内容冲突")
	case errors.Is(err, facade.ErrInvalid):
		status, code = http.StatusBadRequest, facade.ErrInvalid.Error()
		publicErr = apperrors.Params("完成检查请求无效")
	case errors.Is(err, facade.ErrNotFound):
		status, code = http.StatusNotFound, facade.ErrNotFound.Error()
		publicErr = apperrors.NotFound("完成检查不存在")
	case errors.Is(err, facade.ErrConflict):
		status, code = http.StatusConflict, facade.ErrConflict.Error()
		publicErr = apperrors.ObjectState("完成检查记录不一致")
	}
	response.ErrorWithReason(c, status, publicErr, code, nil)
}
