package handler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

const maxAuthorizationHeaderBytes = 8 * 1024

// Handler exposes the Bearer-protected versioned Node HTTP contract.
type Handler struct {
	queries  nodefacade.QueryFacade
	commands nodefacade.CommandFacade
	bearer   [sha256.Size]byte
}

// New creates a Node HTTP handler and retains only a hash of the Bearer.
func New(queries nodefacade.QueryFacade, commands nodefacade.CommandFacade, bearer string) *Handler {
	return &Handler{queries: queries, commands: commands, bearer: sha256.Sum256([]byte(strings.TrimSpace(bearer)))}
}

// Mount registers all nine /internal/node/v1 routes.
func (h *Handler) Mount(router route.IRouter) {
	if h == nil || router == nil {
		return
	}
	group := router.Group("/internal/node/v1")
	group.Use(h.requireBearer)
	group.GET("/descriptor", h.describe)
	group.GET("/users", h.listUsers)
	group.GET("/users/:userId", h.getUser)
	group.PUT("/users/:userId/status", h.setUserStatus)
	group.GET("/users/:userId/sessions", h.listUserSessions)
	group.POST("/users/:userId/sessions/revoke", h.revokeUserSessions)
	group.GET("/login-policy", h.getLoginPolicy)
	group.POST("/login-policy/apply", h.applyLoginPolicy)
	group.PUT("/hub-connection", h.applyHubConnection)
}

func (h *Handler) requireBearer(ctx context.Context, reqCtx *app.RequestContext) {
	xcontext.EnsureTraceID(reqCtx)
	values := headerValues(reqCtx, "Authorization")
	if len(values) != 1 || len(values[0]) > maxAuthorizationHeaderBytes {
		h.writeError(reqCtx, apperrors.Unauthorized("Node管理Bearer无效"))
		reqCtx.Abort()
		return
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(values[0]), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		h.writeError(reqCtx, apperrors.Unauthorized("Node管理Bearer无效"))
		reqCtx.Abort()
		return
	}
	candidate := sha256.Sum256([]byte(strings.TrimSpace(token)))
	if subtle.ConstantTimeCompare(candidate[:], h.bearer[:]) != 1 {
		h.writeError(reqCtx, apperrors.Unauthorized("Node管理Bearer无效"))
		reqCtx.Abort()
		return
	}
	reqCtx.Next(ctx)
}

func (h *Handler) describe(ctx context.Context, c *app.RequestContext) {
	result, err := h.queries.Describe(ctx)
	h.write(c, result, err)
}

func (h *Handler) listUsers(ctx context.Context, c *app.RequestContext) {
	current, err := queryInt64(c, "current", 1)
	if err != nil {
		h.writeError(c, err)
		return
	}
	size, err := queryInt64(c, "size", 20)
	if err != nil {
		h.writeError(c, err)
		return
	}
	var status *int
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			h.writeError(c, apperrors.Params("status格式错误"))
			return
		}
		status = &value
	}
	result, err := h.queries.ListUsers(ctx, nodefacade.UserPageQuery{Current: current, Size: size, Keyword: c.Query("keyword"), Status: status})
	h.write(c, result, err)
}

func (h *Handler) getUser(ctx context.Context, c *app.RequestContext) {
	userID, err := pathID(c)
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.queries.GetUser(ctx, userID)
	h.write(c, result, err)
}

func (h *Handler) setUserStatus(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Status *int   `json:"status"`
		Reason string `json:"reason"`
	}
	if err := bindJSON(c, &request); err != nil {
		h.writeError(c, err)
		return
	}
	if request.Status == nil {
		h.writeError(c, apperrors.Params("status不能为空"))
		return
	}
	key, err := writeMetadata(c, request.Reason)
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.commands.SetUserStatus(ctx, nodefacade.SetUserStatusCommand{UserID: c.Param("userId"), Status: *request.Status, Reason: request.Reason, IdempotencyKey: key})
	h.write(c, result, err)
}

func (h *Handler) listUserSessions(ctx context.Context, c *app.RequestContext) {
	userID, err := pathID(c)
	if err != nil {
		h.writeError(c, err)
		return
	}
	current, err := queryInt64(c, "current", 1)
	if err != nil {
		h.writeError(c, err)
		return
	}
	size, err := queryInt64(c, "size", 20)
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.queries.ListUserSessions(ctx, userID, nodefacade.SessionPageQuery{Current: current, Size: size})
	h.write(c, result, err)
}

func (h *Handler) revokeUserSessions(ctx context.Context, c *app.RequestContext) {
	var request struct {
		All         bool     `json:"all"`
		SessionRefs []string `json:"sessionRefs"`
		Reason      string   `json:"reason"`
	}
	if err := bindJSON(c, &request); err != nil {
		h.writeError(c, err)
		return
	}
	key, err := writeMetadata(c, request.Reason)
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.commands.RevokeUserSessions(ctx, nodefacade.RevokeUserSessionsCommand{UserID: c.Param("userId"), All: request.All, SessionRefs: request.SessionRefs, Reason: request.Reason, IdempotencyKey: key})
	h.write(c, result, err)
}

func (h *Handler) getLoginPolicy(ctx context.Context, c *app.RequestContext) {
	result, err := h.queries.GetLoginPolicy(ctx)
	h.write(c, result, err)
}

func (h *Handler) applyLoginPolicy(ctx context.Context, c *app.RequestContext) {
	var request nodefacade.ApplyLoginPolicyCommand
	if err := bindJSON(c, &request); err != nil {
		h.writeError(c, err)
		return
	}
	key, err := writeMetadata(c, request.Reason)
	if err != nil {
		h.writeError(c, err)
		return
	}
	request.IdempotencyKey = key
	result, err := h.commands.ApplyLoginPolicy(ctx, request)
	h.write(c, result, err)
}

func (h *Handler) applyHubConnection(ctx context.Context, c *app.RequestContext) {
	var request nodefacade.ApplyHubConnectionCommand
	if err := bindJSON(c, &request); err != nil {
		h.writeError(c, err)
		return
	}
	key, err := writeMetadata(c, request.Reason)
	if err != nil {
		h.writeError(c, err)
		return
	}
	request.IdempotencyKey = key
	result, err := h.commands.ApplyHubConnection(ctx, request)
	h.write(c, result, err)
}

func (h *Handler) write(c *app.RequestContext, data any, err error) {
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, data)
}

func (h *Handler) writeError(c *app.RequestContext, err error) {
	if retry := retryAfter(err); retry > 0 {
		c.Header("Retry-After", strconv.Itoa(retry))
	}
	response.Error(c, err)
	appErr := apperrors.From(err)
	status := apperrors.HTTPStatus(appErr)
	switch appErr.Kind() {
	case apperrors.KindParams:
		status = http.StatusBadRequest
	case apperrors.KindAuth:
		status = http.StatusUnauthorized
	case apperrors.KindForbidden:
		status = http.StatusForbidden
	case apperrors.KindObjectState:
		status = http.StatusConflict
	case apperrors.KindSystem, apperrors.KindOperation:
		status = http.StatusInternalServerError
	}
	c.Response.SetStatusCode(status)
}

func bindJSON(c *app.RequestContext, target any) error {
	if len(c.Request.Body()) == 0 || len(c.Request.Body()) > 1024*1024 {
		return apperrors.Params("请求体不能为空或过大")
	}
	if err := json.Unmarshal(c.Request.Body(), target); err != nil {
		return apperrors.Params("请求体格式错误")
	}
	return nil
}

func writeMetadata(c *app.RequestContext, reason string) (string, error) {
	values := headerValues(c, "Idempotency-Key")
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" || len(values[0]) > 256 {
		return "", apperrors.Params("Idempotency-Key不能为空且不能超过256字符")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 512 {
		return "", apperrors.Params("reason不能为空且不能超过512字符")
	}
	return strings.TrimSpace(values[0]), nil
}

func headerValues(c *app.RequestContext, name string) []string {
	values := make([]string, 0, 1)
	c.Request.Header.VisitAll(func(key, value []byte) {
		if strings.EqualFold(string(key), name) {
			values = append(values, string(value))
		}
	})
	return values
}

func pathID(c *app.RequestContext) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(c.Param("userId")), 10, 64)
	if err != nil || value <= 0 {
		return 0, apperrors.Params("userId格式错误")
	}
	return value, nil
}
func queryInt64(c *app.RequestContext, name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, apperrors.Params(name + "格式错误")
	}
	return value, nil
}
func retryAfter(err error) int {
	details, ok := apperrors.From(err).Details().(map[string]any)
	if !ok {
		return 0
	}
	switch value := details["retryAfterSeconds"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}
