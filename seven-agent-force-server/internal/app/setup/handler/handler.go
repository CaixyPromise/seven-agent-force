package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime"
	"strings"

	setupdomain "github.com/CaixyPromise/seven-agent/force/internal/app/setup/domain"
	setupfacade "github.com/CaixyPromise/seven-agent/force/internal/app/setup/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
)

const (
	headerSetupToken   = "X-Setup-Token"
	headerDeviceID     = "X-Device-Id"
	headerTenantID     = "X-Tenant-Id"
	headerTraceID      = "X-Trace-Id"
	headerSecFetchSite = "Sec-Fetch-Site"
	headerSecFetchMode = "Sec-Fetch-Mode"
)

type Service interface {
	GetSetupStatus(ctx context.Context) (*setupfacade.SetupStatusDTO, error)
	CreateOwner(ctx context.Context, request setupfacade.SetupOwnerRequestDTO, setupToken string, requestContext *ssofacade.RequestContext) (*setupfacade.OwnerBootstrapResult, error)
}

type Handler struct {
	service Service
	cfg     config.SetupConfig
}

func NewHandler(service Service, cfg config.SetupConfig) *Handler {
	return &Handler{service: service, cfg: cfg}
}

func (c *Handler) Status(ctx context.Context, reqCtx *app.RequestContext) {
	if !c.validateRequestOrigin(reqCtx) {
		response.Error(reqCtx, invalidSetupOrigin())
		return
	}
	result, err := c.service.GetSetupStatus(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) CreateOwner(ctx context.Context, reqCtx *app.RequestContext) {
	if !c.validateRequestOrigin(reqCtx) {
		response.Error(reqCtx, invalidSetupOrigin())
		return
	}
	if !isJSONContentType(reqCtx) {
		response.Error(reqCtx, apperrors.Params("请求类型必须为 application/json"))
		return
	}
	var request setupfacade.SetupOwnerRequestDTO
	if len(reqCtx.Request.Body()) == 0 || json.Unmarshal(reqCtx.Request.Body(), &request) != nil {
		response.Error(reqCtx, apperrors.Params("请求体格式错误"))
		return
	}
	setupToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerSetupToken)))
	result, err := c.service.CreateOwner(ctx, request, setupToken, resolveRequestContext(reqCtx))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if result.SessionCookieHeaderValue != "" {
		reqCtx.Response.Header.Add("Set-Cookie", result.SessionCookieHeaderValue)
	}
	if result.RefreshCookieHeaderValue != "" {
		reqCtx.Response.Header.Add("Set-Cookie", result.RefreshCookieHeaderValue)
	}
	response.Success(reqCtx, result.Owner)
}

func (c *Handler) validateRequestOrigin(reqCtx *app.RequestContext) bool {
	if reqCtx == nil {
		return true
	}
	return setupdomain.ValidateOrigin(setupdomain.OriginCheckInput{
		Origin:                string(reqCtx.Request.Header.Peek("Origin")),
		Referer:               string(reqCtx.Request.Header.Peek("Referer")),
		SecFetchSite:          string(reqCtx.Request.Header.Peek(headerSecFetchSite)),
		SecFetchMode:          string(reqCtx.Request.Header.Peek(headerSecFetchMode)),
		AllowedOriginPatterns: c.cfg.AllowedOriginPatterns,
		RequireOriginHeader:   c.cfg.RequireOriginHeader,
	})
}

func isJSONContentType(reqCtx *app.RequestContext) bool {
	contentType := strings.TrimSpace(string(reqCtx.Request.Header.ContentType()))
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func resolveRequestContext(reqCtx *app.RequestContext) *ssofacade.RequestContext {
	if reqCtx == nil {
		return &ssofacade.RequestContext{TenantID: "default", TraceID: uuid.NewString()}
	}
	userAgent := strings.TrimSpace(string(reqCtx.UserAgent()))
	deviceID := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerDeviceID)))
	if deviceID == "" && userAgent != "" {
		sum := sha256.Sum256([]byte(strings.ToLower(userAgent)))
		deviceID = "ua:" + hex.EncodeToString(sum[:])
	}
	tenantID := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerTenantID)))
	if tenantID == "" {
		tenantID = "default"
	}
	traceID := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerTraceID)))
	if traceID == "" {
		traceID = xcontext.TraceID(reqCtx)
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	return &ssofacade.RequestContext{
		DeviceID:  deviceID,
		TenantID:  tenantID,
		LoginIP:   xcontext.ResolveClientIP(reqCtx),
		UserAgent: userAgent,
		TraceID:   traceID,
	}
}

func invalidSetupOrigin() error {
	return apperrors.New(apperrors.CodeNoAuth, apperrors.KindForbidden, "初始化请求来源不可信，请刷新页面后重试")
}
