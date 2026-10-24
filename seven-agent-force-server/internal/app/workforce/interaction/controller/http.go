package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
	"io"
	"strconv"
)

type Handler struct{ service *application.Service }

func New(s *application.Service) *Handler { return &Handler{service: s} }

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func (h *Handler) Mount(r route.IRouter, wrap PermissionWrapper) {
	if wrap == nil {
		panic("interaction permission wrapper is required")
	}
	r.GET("/workforce/tasks/:taskId/conversation", wrap("workforce:task:query", h.conversation))
	r.GET("/workforce/tasks/:taskId/interactions", wrap("workforce:task:query", h.questions))
	r.GET("/workforce/tasks/:taskId/conversation/:itemId", wrap("workforce:task:query", h.item))
	r.POST("/workforce/tasks/:taskId/interactions/:interactionId/responses", wrap("workforce:task:continue", h.respond))
	r.POST("/workforce/tasks/:taskId/messages", wrap("workforce:task:continue", h.message))
	r.POST("/workforce/tasks/:taskId/conversation/read", wrap("workforce:task:query", h.read))
}
func positive(s string) (int64, error) {
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != s {
		return 0, domain.ErrInvalid
	}
	return n, nil
}
func scope(c *app.RequestContext) (facade.User, error) {
	org, e := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if e != nil {
		return facade.User{}, domain.ErrNotFound
	}
	user, ok := securitycontext.CurrentUserID(c)
	if !ok {
		return facade.User{}, domain.ErrNotFound
	}
	task, e := positive(c.Param("taskId"))
	return facade.User{TenantID: org.OrgID, TaskID: task, UserID: user, CanContinue: securitycontext.HasPermission(c, "workforce:task:continue")}, e
}
func decode(c *app.RequestContext, v any) error {
	if len(c.Request.Body()) > 32<<10 {
		return domain.ErrInvalid
	}
	body := bytes.TrimSpace(c.Request.Body())
	if len(body) == 0 || body[0] != '{' {
		return domain.ErrInvalid
	}
	check := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	check.UseNumber()
	if !uniqueJSON(check, 0) {
		return domain.ErrInvalid
	}
	if _, e := check.Token(); !errors.Is(e, io.EOF) {
		return domain.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return domain.ErrInvalid
	}
	if e := d.Decode(&struct{}{}); !errors.Is(e, io.EOF) {
		return domain.ErrInvalid
	}
	return nil
}

// Reject duplicate object keys before encoding/json can silently discard an
// earlier authority/version/idempotency field. Request bytes are already bounded.
func uniqueJSON(d *json.Decoder, depth int) bool {
	if depth > 32 {
		return false
	}
	token, e := d.Token()
	if e != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return false
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !uniqueJSON(d, depth+1) {
				return false
			}
		}
		close, e := d.Token()
		return e == nil && close == json.Delim('}')
	case '[':
		for d.More() {
			if !uniqueJSON(d, depth+1) {
				return false
			}
		}
		close, e := d.Token()
		return e == nil && close == json.Delim(']')
	}
	return false
}
func problem(c *app.RequestContext, e error) {
	if errors.Is(e, continuation.ErrInteractionSuspendInProgress) {
		c.Response.Header.Set("Retry-After", "1")
		response.ErrorWithReason(c, 409, apperrors.ObjectState("任务正在暂停，请稍后重试"), continuation.ErrInteractionSuspendInProgress.Error(), map[string]any{"retryable": true})
		return
	}
	status, code := 500, domain.ErrUnavailable.Error()
	publicErr := error(apperrors.System("任务交互暂时不可用"))
	switch e {
	case domain.ErrInvalid:
		status, code = 400, e.Error()
		publicErr = apperrors.Params("任务交互请求无效")
	case domain.ErrNotFound:
		status, code = 404, e.Error()
		publicErr = apperrors.NotFound("任务交互不存在")
	case domain.ErrStale, domain.ErrConflict, domain.ErrNotAllowed, domain.ErrFenced, domain.ErrBusy:
		status, code = 409, e.Error()
		publicErr = apperrors.ObjectState("任务交互状态已变化")
	}
	response.ErrorWithReason(c, status, publicErr, code, nil)
}
func (h *Handler) conversation(ctx context.Context, c *app.RequestContext) {
	u, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	q := application.Query{Cursor: c.Query("cursor")}
	if v := c.Query("limit"); v != "" {
		q.Limit, e = strconv.Atoi(v)
		if e != nil || q.Limit < 1 {
			problem(c, domain.ErrInvalid)
			return
		}
	}
	if v := c.Query("before"); v != "" {
		q.Before, e = positive(v)
		if e != nil {
			problem(c, e)
			return
		}
	}
	if v := c.Query("after"); v != "" {
		q.Changes = true
		if v != "0" {
			q.After, e = positive(v)
			if e != nil {
				problem(c, e)
				return
			}
		}
	}
	out, e := h.service.Conversation(ctx, u, q)
	if e != nil {
		problem(c, e)
		return
	}
	response.Success(c, out)
}
func (h *Handler) item(ctx context.Context, c *app.RequestContext) {
	u, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	id, e := positive(c.Param("itemId"))
	if e != nil {
		problem(c, e)
		return
	}
	out, e := h.service.ConversationItem(ctx, u, id)
	if e != nil {
		problem(c, e)
		return
	}
	response.Success(c, out)
}
func (h *Handler) respond(ctx context.Context, c *app.RequestContext) {
	u, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	id, e := positive(c.Param("interactionId"))
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.ResponseRequest
	if e = decode(c, &req); e != nil {
		problem(c, e)
		return
	}
	out, e := h.service.Respond(ctx, u, id, req)
	if e != nil {
		problem(c, e)
		return
	}
	c.Response.Header.Set("Idempotency-Replayed", strconv.FormatBool(out.Replayed))
	response.Success(c, out)
}
func (h *Handler) message(ctx context.Context, c *app.RequestContext) {
	u, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.MessageRequest
	if e = decode(c, &req); e != nil {
		problem(c, e)
		return
	}
	out, e := h.service.PostMessage(ctx, u, req)
	if e != nil {
		problem(c, e)
		return
	}
	c.Response.Header.Set("Idempotency-Replayed", strconv.FormatBool(out.Replayed))
	response.Success(c, out)
}
func (h *Handler) read(ctx context.Context, c *app.RequestContext) {
	u, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	var req struct {
		Watermark *int64 `json:"watermark,string"`
	}
	if e = decode(c, &req); e != nil {
		problem(c, e)
		return
	}
	if req.Watermark == nil {
		problem(c, domain.ErrInvalid)
		return
	}
	if e = h.service.MarkRead(ctx, u, *req.Watermark); e != nil {
		problem(c, e)
		return
	}
	response.Success(c, map[string]bool{"saved": true})
}
