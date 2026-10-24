package handler

import (
	"context"
	"strconv"
	"strings"

	loginfacade "github.com/CaixyPromise/seven-agent/force/internal/app/login/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type Handler struct {
	facade loginfacade.PasswordFlowFacade
}

func NewHandler(facade loginfacade.PasswordFlowFacade) *Handler {
	return &Handler{facade: facade}
}

func (c *Handler) PasswordState(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.PasswordStateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.GetPasswordState(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) Password(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.PasswordSubmitRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.SubmitPassword(ctx, request)
	if err != nil {
		annotatePasswordPunishmentAudit(reqCtx, request, nil, err)
		response.Error(reqCtx, err)
		return
	}
	annotatePasswordPunishmentAudit(reqCtx, request, result, nil)
	writeLoginCookies(reqCtx, result.SessionCookieHeaderValue, result.RefreshCookieHeaderValue)
	response.Success(reqCtx, result)
}

func (c *Handler) RegisterState(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.RegisterStateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.GetRegisterState(ctx, request)
	if err != nil {
		writeRetryAfter(reqCtx, err)
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) RegisterEmailCode(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.RegisterEmailCodeRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.SendRegisterEmailCode(ctx, request)
	if err != nil {
		writeRetryAfter(reqCtx, err)
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) Register(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.RegisterSubmitRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.SubmitRegister(ctx, request)
	if err != nil {
		writeRetryAfter(reqCtx, err)
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) StartPasskey(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.PasskeyStartRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.StartPasskey(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func writeRetryAfter(reqCtx *app.RequestContext, err error) {
	if reqCtx == nil {
		return
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Kind() != apperrors.KindRateLimited {
		return
	}
	if details, ok := appErr.Details().(map[string]any); ok {
		if seconds, ok := details["retryAfterSeconds"].(int); ok && seconds > 0 {
			reqCtx.Response.Header.Set("Retry-After", strconv.Itoa(seconds))
			return
		}
	}
	reqCtx.Response.Header.Set("Retry-After", "60")
}

func (c *Handler) VerifyPasskey(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.PasskeyVerifyRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.VerifyPasskey(ctx, request)
	if err != nil {
		annotatePasskeyPunishmentAudit(reqCtx, request, nil, err)
		response.Error(reqCtx, err)
		return
	}
	annotatePasskeyPunishmentAudit(reqCtx, request, result, nil)
	writeLoginCookies(reqCtx, result.SessionCookieHeaderValue, result.RefreshCookieHeaderValue)
	response.Success(reqCtx, result)
}

func (c *Handler) VerifyTotp(ctx context.Context, reqCtx *app.RequestContext) {
	var request loginfacade.TotpVerifyRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.RequestContext = resolveRequestContext(reqCtx)
	result, err := c.facade.VerifyTotp(ctx, request)
	if err != nil {
		annotateTotpPunishmentAudit(reqCtx, request, nil, err)
		response.Error(reqCtx, err)
		return
	}
	annotateTotpPunishmentAudit(reqCtx, request, result, nil)
	writeLoginCookies(reqCtx, result.SessionCookieHeaderValue, result.RefreshCookieHeaderValue)
	response.Success(reqCtx, result)
}

func resolveRequestContext(reqCtx *app.RequestContext) *loginfacade.RequestContext {
	if reqCtx == nil {
		return nil
	}
	return &loginfacade.RequestContext{
		LoginIP:   reqCtx.ClientIP(),
		UserAgent: string(reqCtx.UserAgent()),
		DeviceID:  string(reqCtx.Request.Header.Peek("X-Device-Id")),
		TenantID:  string(reqCtx.Request.Header.Peek("X-Tenant-Id")),
		TraceID:   string(reqCtx.Request.Header.Peek("X-Trace-Id")),
		Host:      strings.TrimSpace(string(reqCtx.Host())),
		Origin:    strings.TrimSpace(string(reqCtx.GetHeader("Origin"))),
		Referer:   strings.TrimSpace(string(reqCtx.GetHeader("Referer"))),
	}
}

func writeLoginCookies(reqCtx *app.RequestContext, sessionCookie, refreshCookie string) {
	if reqCtx == nil {
		return
	}
	if sessionCookie = stringTrim(sessionCookie); sessionCookie != "" {
		reqCtx.Response.Header.Add("Set-Cookie", sessionCookie)
	}
	if refreshCookie = stringTrim(refreshCookie); refreshCookie != "" {
		reqCtx.Response.Header.Add("Set-Cookie", refreshCookie)
	}
}

func stringTrim(value string) string {
	return strings.TrimSpace(value)
}

func annotatePasswordPunishmentAudit(reqCtx *app.RequestContext, request loginfacade.PasswordSubmitRequest, result *loginfacade.PasswordSubmitResult, err error) {
	if reqCtx == nil {
		return
	}
	audit := securitycontext.LoginPunishmentAudit{
		LoginTransactionID: request.LoginTransactionID,
		AccountFingerprint: securitycontext.LoginAccountFingerprint(request.UserAccount),
		Outcome:            "unknown",
	}
	if err != nil {
		appErr := apperrors.From(err)
		audit.Outcome = "rejected"
		if appErr != nil {
			audit.Code = appErr.Code()
		}
		securitycontext.SetLoginPunishmentAudit(reqCtx, audit)
		return
	}
	if result == nil {
		securitycontext.SetLoginPunishmentAudit(reqCtx, audit)
		return
	}
	audit.CaptchaRequired = result.CaptchaRequired
	audit.TotpRequired = result.TotpRequired
	audit.Locked = result.Locked
	audit.LockExpiresAt = result.LockExpiresAt
	if result.Captcha != nil {
		audit.ChallengeIdentifier = result.Captcha.ChallengeIdentifier
	}
	switch {
	case result.Authenticated:
		audit.Outcome = "authenticated"
	case result.Locked:
		audit.Outcome = "locked"
	case result.CaptchaRejected:
		audit.Outcome = "captcha_failed"
	case result.CaptchaRequired && strings.TrimSpace(request.CaptchaCode) != "":
		audit.Outcome = "rejected"
	case result.CaptchaRequired:
		audit.Outcome = "captcha_required"
	case result.TotpRequired:
		audit.Outcome = "mfa_required"
	default:
		audit.Outcome = "rejected"
	}
	securitycontext.SetLoginPunishmentAudit(reqCtx, audit)
}

func annotateTotpPunishmentAudit(reqCtx *app.RequestContext, request loginfacade.TotpVerifyRequest, result *loginfacade.TotpVerifyResult, err error) {
	audit := baseLoginPunishmentAudit(reqCtx, request.LoginTransactionID, request.UserAccount, err)
	if audit == nil {
		return
	}
	if err == nil && result != nil {
		audit.Locked = result.Locked
		audit.LockExpiresAt = result.LockExpiresAt
		if result.Authenticated {
			audit.Outcome = "authenticated"
		} else if result.Locked {
			audit.Outcome = "locked"
		} else {
			audit.Outcome = "rejected"
		}
	}
	securitycontext.SetLoginPunishmentAudit(reqCtx, *audit)
}

func annotatePasskeyPunishmentAudit(reqCtx *app.RequestContext, request loginfacade.PasskeyVerifyRequest, result *loginfacade.PasskeyVerifyResult, err error) {
	audit := baseLoginPunishmentAudit(reqCtx, request.LoginTransactionID, request.UserAccount, err)
	if audit == nil {
		return
	}
	if err == nil && result != nil {
		audit.Locked = result.Locked
		audit.LockExpiresAt = result.LockExpiresAt
		if result.Authenticated {
			audit.Outcome = "authenticated"
		} else if result.Locked {
			audit.Outcome = "locked"
		} else {
			audit.Outcome = "rejected"
		}
	}
	securitycontext.SetLoginPunishmentAudit(reqCtx, *audit)
}

func baseLoginPunishmentAudit(reqCtx *app.RequestContext, loginTransactionID, userAccount string, err error) *securitycontext.LoginPunishmentAudit {
	if reqCtx == nil {
		return nil
	}
	audit := &securitycontext.LoginPunishmentAudit{
		LoginTransactionID: loginTransactionID,
		AccountFingerprint: securitycontext.LoginAccountFingerprint(userAccount),
		Outcome:            "unknown",
	}
	if err != nil {
		appErr := apperrors.From(err)
		audit.Outcome = "rejected"
		if appErr != nil {
			audit.Code = appErr.Code()
		}
	}
	return audit
}
