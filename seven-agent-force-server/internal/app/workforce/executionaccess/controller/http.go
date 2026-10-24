package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

type Handler struct{ service facade.PublicService }

func New(service facade.PublicService) *Handler { return &Handler{service: service} }

func (h *Handler) Mount(r route.IRouter, wrap PermissionWrapper) {
	if wrap == nil {
		panic("execution access permission wrapper is required")
	}
	r.GET("/workforce/tasks/:taskId/execution-operations/:operationId", wrap("workforce:task:query", h.get))
	r.POST("/workforce/tasks/:taskId/execution-operations/:operationId/decisions", wrap("workforce:task:continue", h.decide))
	r.GET("/workforce/tasks/:taskId/network-grants", wrap("workforce:task:query", h.grants))
	r.POST("/workforce/tasks/:taskId/network-grants/:grantId/revoke", wrap("workforce:task:continue", h.revoke))
}

func (h *Handler) get(ctx context.Context, c *app.RequestContext) {
	u, id, err := publicScope(c, "operationId")
	if err != nil {
		problem(c, err)
		return
	}
	value, err := h.service.Get(ctx, u, id)
	write(c, value, err)
}
func (h *Handler) decide(ctx context.Context, c *app.RequestContext) {
	u, id, err := publicScope(c, "operationId")
	if err != nil {
		problem(c, err)
		return
	}
	var req facade.DecisionRequest
	if err = decode(c, &req, 24<<10); err != nil {
		problem(c, err)
		return
	}
	value, err := h.service.Decide(ctx, u, id, req)
	if err == nil {
		c.Response.Header.Set("Idempotency-Replayed", strconv.FormatBool(value.Replayed))
	}
	write(c, value, err)
}
func (h *Handler) grants(ctx context.Context, c *app.RequestContext) {
	u, _, err := publicScope(c, "")
	if err != nil {
		problem(c, err)
		return
	}
	value, err := h.service.ListGrants(ctx, u)
	write(c, value, err)
}
func (h *Handler) revoke(ctx context.Context, c *app.RequestContext) {
	u, id, err := publicScope(c, "grantId")
	if err != nil {
		problem(c, err)
		return
	}
	var req facade.RevokeRequest
	if err = decode(c, &req, 8<<10); err != nil {
		problem(c, err)
		return
	}
	value, err := h.service.RevokeGrant(ctx, u, id, req)
	if err == nil {
		c.Response.Header.Set("Idempotency-Replayed", strconv.FormatBool(value.Replayed))
	}
	write(c, value, err)
}

func publicScope(c *app.RequestContext, resource string) (facade.User, int64, error) {
	org, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil {
		return facade.User{}, 0, domain.ErrNotFound
	}
	user, ok := securitycontext.CurrentUserID(c)
	if !ok {
		return facade.User{}, 0, domain.ErrNotFound
	}
	task, err := positive(c.Param("taskId"))
	if err != nil {
		return facade.User{}, 0, err
	}
	var id int64
	if resource != "" {
		id, err = positive(c.Param(resource))
		if err != nil {
			return facade.User{}, 0, err
		}
	}
	return facade.User{TenantID: org.OrgID, TaskID: task, UserID: user, CanContinue: securitycontext.HasPermission(c, "workforce:task:continue")}, id, nil
}

func positive(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
		return 0, domain.ErrInvalid
	}
	return value, nil
}

func decode(c *app.RequestContext, target any, limit int) error {
	raw := c.Request.Body()
	if len(raw) == 0 || len(raw) > limit {
		return domain.ErrInvalid
	}
	body := bytes.TrimSpace(raw)
	if len(body) == 0 || body[0] != '{' {
		return domain.ErrInvalid
	}
	check := json.NewDecoder(bytes.NewReader(body))
	check.UseNumber()
	if !uniqueJSON(check, 0) {
		return domain.ErrInvalid
	}
	if _, err := check.Token(); !errors.Is(err, io.EOF) {
		return domain.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return domain.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrInvalid
	}
	return nil
}

func uniqueJSON(d *json.Decoder, depth int) bool {
	if depth > 32 {
		return false
	}
	token, err := d.Token()
	if err != nil {
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
			raw, err := d.Token()
			key, ok := raw.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !uniqueJSON(d, depth+1) {
				return false
			}
		}
		close, err := d.Token()
		return err == nil && close == json.Delim('}')
	case '[':
		for d.More() {
			if !uniqueJSON(d, depth+1) {
				return false
			}
		}
		close, err := d.Token()
		return err == nil && close == json.Delim(']')
	default:
		return false
	}
}

func write(c *app.RequestContext, value any, err error) {
	if err != nil {
		problem(c, err)
		return
	}
	response.Success(c, value)
}
func problem(c *app.RequestContext, err error) {
	status, code := http.StatusInternalServerError, domain.ErrUnavailable.Error()
	publicErr := error(apperrors.System("网络操作暂时不可用"))
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrTargetDenied):
		status, code = http.StatusBadRequest, err.Error()
		publicErr = apperrors.Params("网络操作请求无效")
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, err.Error()
		publicErr = apperrors.NotFound("网络操作不存在")
	case errors.Is(err, domain.ErrStale), errors.Is(err, domain.ErrActionNotAllowed), errors.Is(err, domain.ErrIdempotencyConflict), errors.Is(err, domain.ErrGrantRevoked), errors.Is(err, domain.ErrGrantExpired), errors.Is(err, domain.ErrFenced), errors.Is(err, domain.ErrOutcomeUnknown):
		status, code = http.StatusConflict, err.Error()
		publicErr = apperrors.ObjectState("网络操作状态已变化")
	}
	response.ErrorWithReason(c, status, publicErr, code, nil)
}
