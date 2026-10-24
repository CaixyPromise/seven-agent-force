package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	hubfacade "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/facade"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

const maxBodyBytes = 1 << 20

type RouteWrapper func(permission, description string, handler app.HandlerFunc) app.HandlerFunc

type Handler struct {
	service hubfacade.NodeAdminFacade
	wrap    RouteWrapper
}

func New(service hubfacade.NodeAdminFacade) *Handler { return &Handler{service: service} }

func (h *Handler) BindRouteWrapper(wrapper RouteWrapper) { h.wrap = wrapper }

func (h *Handler) wrapped(permission, description string, handler app.HandlerFunc) app.HandlerFunc {
	if h.wrap == nil {
		return handler
	}
	return h.wrap(permission, description, handler)
}

func (h *Handler) Mount(router route.IRouter) {
	group := router.Group("/system/hub/nodes")
	group.GET("", h.wrapped("system:hub-node:list", "查询Hub节点列表", h.pageNodes))
	group.POST("", h.wrapped("system:hub-node:add", "创建Hub节点", h.saveNode))
	group.GET("/:nodeCode", h.wrapped("system:hub-node:query", "查询Hub节点", h.getNode))
	group.PUT("/:nodeCode", h.wrapped("system:hub-node:edit", "编辑Hub节点", h.updateNode))
	group.POST("/:nodeCode/copy", h.wrapped("system:hub-node:add", "复制Hub节点", h.copyNode))
	group.PUT("/:nodeCode/status", h.wrapped("system:hub-node:status", "启停Hub节点", h.setNodeStatus))
	group.POST("/:nodeCode/connection-test", h.wrapped("system:hub-node:test", "测试Hub节点连接", h.testConnection))
	group.GET("/:nodeCode/users", h.wrapped("system:hub-node:user:list", "查询Node用户", h.listUsers))
	group.GET("/:nodeCode/users/:userId", h.wrapped("system:hub-node:user:query", "查询Node用户详情", h.getUser))
	group.PUT("/:nodeCode/users/:userId/status", h.wrapped("system:hub-node:user:status", "修改Node用户状态", h.setUserStatus))
	group.GET("/:nodeCode/users/:userId/sessions", h.wrapped("system:hub-node:session:list", "查询Node用户会话", h.listSessions))
	group.POST("/:nodeCode/users/:userId/sessions/revoke", h.wrapped("system:hub-node:session:revoke", "撤销Node用户会话", h.revokeSessions))
	group.GET("/:nodeCode/login-policy", h.wrapped("system:hub-node:policy:query", "查询Node登录策略", h.getLoginPolicy))
	group.POST("/:nodeCode/login-policy/apply", h.wrapped("system:hub-node:policy:apply", "应用Node登录策略", h.applyLoginPolicy))
	group.GET("/:nodeCode/federation", h.wrapped("system:hub-node:federation:query", "查询Node联邦连接", h.getFederation))
	group.POST("/:nodeCode/federation/provision", h.wrapped("system:hub-node:federation:apply", "编排Node联邦连接", h.provisionFederation))
}

func (h *Handler) pageNodes(ctx context.Context, c *app.RequestContext) {
	current, err := queryInt(c, "current", 1)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	size, err := queryInt(c, "size", 20)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	var status *int
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil {
			h.write(c, nil, apperrors.Params("status格式错误"))
			return
		}
		status = &v
	}
	result, err := h.service.PageNodes(traceContext(ctx, c), hubfacade.NodePageQuery{Current: current, Size: size, Keyword: c.Query("keyword"), Status: status})
	h.write(c, result, err)
}
func (h *Handler) getNode(ctx context.Context, c *app.RequestContext) {
	result, err := h.service.GetNode(traceContext(ctx, c), c.Param("nodeCode"))
	h.write(c, result, err)
}
func (h *Handler) saveNode(ctx context.Context, c *app.RequestContext) {
	var command hubfacade.SaveNodeCommand
	if err := decode(c, &command); err != nil {
		h.write(c, nil, err)
		return
	}
	result, err := h.service.SaveNode(traceContext(ctx, c), command)
	h.write(c, result, err)
}
func (h *Handler) updateNode(ctx context.Context, c *app.RequestContext) {
	var command hubfacade.SaveNodeCommand
	if err := decode(c, &command); err != nil {
		h.write(c, nil, err)
		return
	}
	command.OriginalNodeCode = c.Param("nodeCode")
	result, err := h.service.SaveNode(traceContext(ctx, c), command)
	h.write(c, result, err)
}
func (h *Handler) copyNode(ctx context.Context, c *app.RequestContext) {
	var command hubfacade.CopyNodeCommand
	if err := decode(c, &command); err != nil {
		h.write(c, nil, err)
		return
	}
	result, err := h.service.CopyNode(traceContext(ctx, c), c.Param("nodeCode"), command)
	h.write(c, result, err)
}
func (h *Handler) setNodeStatus(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Status *int `json:"status"`
	}
	if err := decode(c, &body); err != nil {
		h.write(c, nil, err)
		return
	}
	if body.Status == nil {
		h.write(c, nil, apperrors.Params("status不能为空"))
		return
	}
	err := h.service.SetNodeStatus(traceContext(ctx, c), hubfacade.SetNodeStatusCommand{NodeCode: c.Param("nodeCode"), Status: *body.Status})
	h.write(c, nil, err)
}
func (h *Handler) testConnection(ctx context.Context, c *app.RequestContext) {
	result, err := h.service.TestConnection(traceContext(ctx, c), c.Param("nodeCode"))
	h.write(c, result, err)
}
func (h *Handler) listUsers(ctx context.Context, c *app.RequestContext) {
	current, err := queryInt64(c, "current", 1)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	size, err := queryInt64(c, "size", 20)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	var status *int
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil {
			h.write(c, nil, apperrors.Params("status格式错误"))
			return
		}
		status = &v
	}
	result, err := h.service.ListNodeUsers(traceContext(ctx, c), c.Param("nodeCode"), nodefacade.UserPageQuery{Current: current, Size: size, Keyword: c.Query("keyword"), Status: status})
	h.write(c, result, err)
}
func (h *Handler) getUser(ctx context.Context, c *app.RequestContext) {
	result, err := h.service.GetNodeUser(traceContext(ctx, c), c.Param("nodeCode"), c.Param("userId"))
	h.write(c, result, err)
}
func (h *Handler) setUserStatus(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Status *int   `json:"status"`
		Reason string `json:"reason"`
	}
	if err := decode(c, &body); err != nil {
		h.write(c, nil, err)
		return
	}
	if body.Status == nil {
		h.write(c, nil, apperrors.Params("status不能为空"))
		return
	}
	key, err := idempotencyKey(c)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	err = h.service.SetNodeUserStatus(traceContext(ctx, c), hubfacade.NodeUserStatusCommand{NodeCode: c.Param("nodeCode"), UserID: c.Param("userId"), Status: *body.Status, Reason: body.Reason, IdempotencyKey: key})
	h.write(c, nil, err)
}
func (h *Handler) listSessions(ctx context.Context, c *app.RequestContext) {
	current, err := queryInt64(c, "current", 1)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	size, err := queryInt64(c, "size", 20)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	result, err := h.service.ListNodeUserSessions(traceContext(ctx, c), c.Param("nodeCode"), c.Param("userId"), nodefacade.SessionPageQuery{Current: current, Size: size})
	h.write(c, result, err)
}
func (h *Handler) revokeSessions(ctx context.Context, c *app.RequestContext) {
	var body struct {
		All         bool     `json:"all"`
		SessionRefs []string `json:"sessionRefs"`
		Reason      string   `json:"reason"`
	}
	if err := decode(c, &body); err != nil {
		h.write(c, nil, err)
		return
	}
	key, err := idempotencyKey(c)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	err = h.service.RevokeNodeUserSessions(traceContext(ctx, c), hubfacade.RevokeNodeSessionsCommand{NodeCode: c.Param("nodeCode"), UserID: c.Param("userId"), All: body.All, SessionRefs: body.SessionRefs, Reason: body.Reason, IdempotencyKey: key})
	h.write(c, nil, err)
}
func (h *Handler) getLoginPolicy(ctx context.Context, c *app.RequestContext) {
	result, err := h.service.GetNodeLoginPolicy(traceContext(ctx, c), c.Param("nodeCode"))
	h.write(c, result, err)
}
func (h *Handler) applyLoginPolicy(ctx context.Context, c *app.RequestContext) {
	var command nodefacade.ApplyLoginPolicyCommand
	if err := decode(c, &command); err != nil {
		h.write(c, nil, err)
		return
	}
	key, err := idempotencyKey(c)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	command.IdempotencyKey = key
	err = h.service.ApplyNodeLoginPolicy(traceContext(ctx, c), c.Param("nodeCode"), command)
	h.write(c, nil, err)
}
func (h *Handler) getFederation(ctx context.Context, c *app.RequestContext) {
	result, err := h.service.GetFederationStatus(traceContext(ctx, c), c.Param("nodeCode"))
	h.write(c, result, err)
}
func (h *Handler) provisionFederation(ctx context.Context, c *app.RequestContext) {
	var command hubfacade.ProvisionConnectionCommand
	if err := decode(c, &command); err != nil {
		h.write(c, nil, err)
		return
	}
	key, err := idempotencyKey(c)
	if err != nil {
		h.write(c, nil, err)
		return
	}
	command.NodeCode = c.Param("nodeCode")
	command.IdempotencyKey = key
	err = h.service.ProvisionNodeConnection(traceContext(ctx, c), command)
	h.write(c, nil, err)
}

func decode(c *app.RequestContext, target any) error {
	body := c.Request.Body()
	if len(body) == 0 || len(body) > maxBodyBytes {
		return apperrors.Params("请求体不能为空或过大")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return apperrors.Params("请求体格式错误")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return apperrors.Params("请求体格式错误")
	}
	return nil
}
func idempotencyKey(c *app.RequestContext) (string, error) {
	var values []string
	c.Request.Header.VisitAll(func(k, v []byte) {
		if strings.EqualFold(string(k), "Idempotency-Key") {
			values = append(values, string(v))
		}
	})
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" || len(values[0]) > 256 {
		return "", apperrors.Params("Idempotency-Key不能为空且不能超过256字符")
	}
	return strings.TrimSpace(values[0]), nil
}
func queryInt(c *app.RequestContext, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperrors.Params(name + "格式错误")
	}
	return v, nil
}
func queryInt64(c *app.RequestContext, name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, apperrors.Params(name + "格式错误")
	}
	return v, nil
}
func traceContext(ctx context.Context, c *app.RequestContext) context.Context {
	return xcontext.WithTraceID(ctx, xcontext.EnsureTraceID(c))
}

type remoteHTTPError interface {
	error
	RemoteStatusCode() int
	RemoteCode() int
	RemoteMessage() string
	RemoteRetryAfter() int
}

func (h *Handler) write(c *app.RequestContext, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	var remote remoteHTTPError
	if errors.As(err, &remote) {
		traceID := xcontext.EnsureTraceID(c)
		xcontext.SetResponseError(c, remote.RemoteCode(), "Node远端调用失败")
		if retry := remote.RemoteRetryAfter(); retry > 0 {
			c.Header("Retry-After", strconv.Itoa(retry))
		}
		c.Header(xcontext.TraceIDHeader, traceID)
		c.JSON(remote.RemoteStatusCode(), response.Result{Code: remote.RemoteCode(), Data: nil, Message: remote.RemoteMessage(), TraceID: traceID})
		return
	}
	response.Error(c, err)
	status := apperrors.HTTPStatus(err)
	switch apperrors.From(err).Kind() {
	case apperrors.KindParams:
		status = http.StatusBadRequest
	case apperrors.KindAuth:
		status = http.StatusUnauthorized
	case apperrors.KindForbidden:
		status = http.StatusForbidden
	case apperrors.KindObjectState:
		status = http.StatusConflict
	}
	c.Response.SetStatusCode(status)
}
