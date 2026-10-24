package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	setupdomain "github.com/CaixyPromise/seven-agent/force/internal/app/setup/domain"
	ssoapp "github.com/CaixyPromise/seven-agent/force/internal/app/sso/application"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	limiterinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/limiter"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type Handler struct {
	service *ssoapp.Service
	cfg     ConfigView
	auth    authorizationfacade.AuthFacade
	limiter limiterinfra.Limiter
}

type ConfigView struct {
	Enabled                 bool
	FrontendPrimaryEnabled  bool
	ResourceServerEnabled   bool
	Issuer                  string
	BaseURL                 string
	FrontendLoginURL        string
	DefaultFirstPartyClient string
	SessionCookieName       string
	RefreshCookieName       string
	RefreshCookieSecure     bool
	TokenRateLimit          int64
	TokenRateLimitWindow    time.Duration
	UserInfoRateLimit       int64
	UserInfoRateLimitWindow time.Duration
	RateLimitFailClosed     bool
}

func NewHandler(service *ssoapp.Service, cfg ConfigView) *Handler {
	return &Handler{service: service, cfg: cfg}
}

func (c *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c == nil {
		return
	}
	c.auth = auth
}

func (c *Handler) BindLimiter(limiter limiterinfra.Limiter) {
	if c == nil {
		return
	}
	c.limiter = limiter
}

func (c *Handler) RuntimeConfig(ctx context.Context, reqCtx *app.RequestContext) {
	issuer := strings.TrimSpace(c.cfg.Issuer)
	if issuer == "" {
		issuer = strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/")
	}
	response.Success(reqCtx, map[string]any{
		"enabled":                   c.cfg.Enabled,
		"frontendPrimaryEnabled":    c.cfg.FrontendPrimaryEnabled,
		"resourceServerEnabled":     c.cfg.ResourceServerEnabled,
		"issuer":                    issuer,
		"defaultFirstPartyClientId": c.cfg.DefaultFirstPartyClient,
	})
}

func (c *Handler) Discovery(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.service.BuildDiscoveryDocument(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	reqCtx.JSON(http.StatusOK, result)
}

func (c *Handler) JWKS(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.service.BuildJwksDocument(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	reqCtx.JSON(http.StatusOK, result)
}

func (c *Handler) Authorize(ctx context.Context, reqCtx *app.RequestContext) {
	clientID := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("client_id")))
	redirectURI := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("redirect_uri")))
	responseType := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("response_type")))
	scope := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("scope")))
	state := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("state")))
	nonce := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("nonce")))
	codeChallenge := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("code_challenge")))
	codeChallengeMethod := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("code_challenge_method")))
	prompt := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("prompt")))
	if responseType != "code" {
		c.writeAuthorizeError(ctx, reqCtx, clientID, redirectURI, "unsupported_response_type", "仅支持 response_type=code", state)
		return
	}
	sessionID := readCookie(reqCtx, c.cfg.SessionCookieName)
	if sessionID != "" {
		if session, err := c.service.ResolveActiveSessionForCandidateUse(ctx, sessionID); err == nil && session != nil {
			redirectURL, redirectErr := c.service.AuthorizeWithActiveSession(ctx, clientID, redirectURI, scope, state, nonce, codeChallenge, codeChallengeMethod, session)
			if redirectErr == nil && strings.TrimSpace(redirectURL) != "" {
				reqCtx.Response.Header.Set("Location", redirectURL)
				reqCtx.SetStatusCode(http.StatusFound)
				return
			}
		}
	}
	if strings.EqualFold(prompt, "none") {
		c.writeAuthorizeError(ctx, reqCtx, clientID, redirectURI, "login_required", "用户尚未登录", state)
		return
	}
	session, err := c.service.CreateAuthorizationSession(ctx, ssofacade.CreateAuthorizationSessionRequest{
		ClientID:            clientID,
		ResponseType:        responseType,
		RedirectURI:         redirectURI,
		Scopes:              splitScope(scope),
		State:               state,
		Nonce:               nonce,
		Prompt:              prompt,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		RequestContext:      resolveRequestContext(reqCtx),
	})
	if err != nil {
		c.writeAuthorizeApplicationError(ctx, reqCtx, clientID, redirectURI, err, state)
		return
	}
	loginURL := strings.TrimSpace(c.cfg.FrontendLoginURL)
	if loginURL == "" {
		loginURL = "/login"
	}
	redirectURL := loginURL
	if parsed, err := url.Parse(loginURL); err == nil {
		query := parsed.Query()
		query.Set("loginTransactionId", session.LoginTransactionID)
		if continueURL := buildAuthorizeContinueURL(c.cfg.BaseURL, reqCtx); continueURL != "" {
			query.Set("continue", continueURL)
		}
		parsed.RawQuery = query.Encode()
		redirectURL = parsed.String()
	}
	reqCtx.Response.Header.Set("Location", redirectURL)
	reqCtx.SetStatusCode(http.StatusFound)
}

func (c *Handler) writeAuthorizeError(ctx context.Context, reqCtx *app.RequestContext, clientID, redirectURI, code, description, state string) {
	if c.service.CanRedirectToClient(ctx, clientID, redirectURI) {
		c.redirectAuthorizeError(reqCtx, redirectURI, code, description, state)
		return
	}
	reqCtx.JSON(http.StatusBadRequest, map[string]any{
		"error":             code,
		"error_description": description,
	})
}

func (c *Handler) writeAuthorizeApplicationError(ctx context.Context, reqCtx *app.RequestContext, clientID, redirectURI string, err error, state string) {
	appErr := apperrors.From(err)
	code := "invalid_request"
	if appErr != nil {
		switch {
		case strings.Contains(appErr.Message(), "response_type"):
			code = "unsupported_response_type"
		case strings.Contains(appErr.Message(), "scope") || strings.Contains(appErr.Message(), "openid"):
			code = "invalid_scope"
		}
	}
	c.writeAuthorizeError(ctx, reqCtx, clientID, redirectURI, code, err.Error(), state)
}

func writeAuthorizeLoginError(reqCtx *app.RequestContext, err error) {
	appErr := apperrors.From(err)
	code := "invalid_request"
	if appErr != nil {
		switch {
		case strings.Contains(appErr.Message(), "response_type"):
			code = "unsupported_response_type"
		case strings.Contains(appErr.Message(), "scope") || strings.Contains(appErr.Message(), "openid"):
			code = "invalid_scope"
		}
	}
	reqCtx.JSON(http.StatusBadRequest, map[string]any{
		"error":             code,
		"error_description": err.Error(),
	})
}

func (c *Handler) AuthorizeLogin(ctx context.Context, reqCtx *app.RequestContext) {
	sessionID := readCookie(reqCtx, c.cfg.SessionCookieName)
	clientID := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("client_id")))
	redirectURI := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("redirect_uri")))
	responseType := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("response_type")))
	scope := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("scope")))
	state := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("state")))
	nonce := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("nonce")))
	codeChallenge := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("code_challenge")))
	codeChallengeMethod := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("code_challenge_method")))
	prompt := strings.TrimSpace(string(reqCtx.QueryArgs().Peek("prompt")))
	if responseType != "code" {
		reqCtx.JSON(http.StatusBadRequest, map[string]any{"error": "unsupported_response_type", "error_description": "仅支持 response_type=code"})
		return
	}
	if sessionID != "" {
		if session, err := c.service.ResolveActiveSessionForCandidateUse(ctx, sessionID); err == nil && session != nil {
			if redirectURL, err := c.service.AuthorizeWithActiveSession(ctx, clientID, redirectURI, scope, state, nonce, codeChallenge, codeChallengeMethod, session); err == nil {
				response.Success(reqCtx, map[string]any{"redirectUrl": redirectURL})
				return
			}
		}
	}
	session, err := c.service.CreateAuthorizationSession(ctx, ssofacade.CreateAuthorizationSessionRequest{
		ClientID:            clientID,
		ResponseType:        responseType,
		RedirectURI:         redirectURI,
		Scopes:              splitScope(scope),
		State:               state,
		Nonce:               nonce,
		Prompt:              prompt,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		RequestContext:      resolveRequestContext(reqCtx),
	})
	if err != nil {
		writeAuthorizeLoginError(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{
		"loginTransactionId": session.LoginTransactionID,
		"expiresIn":          300,
	})
}

func (c *Handler) Token(ctx context.Context, reqCtx *app.RequestContext) {
	ctx = withRequestTrace(ctx, reqCtx)
	grantType := strings.TrimSpace(string(reqCtx.FormValue("grant_type")))
	clientID, clientSecret, err := resolveClientAuthentication(reqCtx)
	if err != nil {
		reqCtx.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid_client", "error_description": err.Error()})
		return
	}
	if !c.allowOAuthRoute(ctx, reqCtx, "token", tokenRateLimitKey(grantType, clientID, reqCtx), c.cfg.TokenRateLimit, c.cfg.TokenRateLimitWindow) {
		return
	}
	switch grantType {
	case "authorization_code":
		bundle, bundleErr := c.service.ExchangeAuthorizationCode(
			ctx,
			clientID,
			clientSecret,
			strings.TrimSpace(string(reqCtx.FormValue("code"))),
			strings.TrimSpace(string(reqCtx.FormValue("redirect_uri"))),
			strings.TrimSpace(string(reqCtx.FormValue("code_verifier"))),
		)
		if bundleErr != nil {
			writeTokenError(reqCtx, bundleErr)
			return
		}
		result := map[string]any{
			"access_token": bundle.AccessToken,
			"token_type":   bundle.TokenType,
			"expires_in":   bundle.ExpiresInSeconds,
			"scope":        bundle.Scope,
		}
		if strings.TrimSpace(bundle.IDToken) != "" {
			result["id_token"] = bundle.IDToken
		}
		if strings.TrimSpace(bundle.RefreshToken) != "" {
			if bundle.RefreshTokenBodyAllowed {
				result["refresh_token"] = bundle.RefreshToken
			}
			if bundle.RefreshTokenExpiresAt != nil {
				reqCtx.Response.Header.Add("Set-Cookie", c.service.BuildRefreshCookie(bundle.RefreshToken, *bundle.RefreshTokenExpiresAt))
			}
		}
		reqCtx.JSON(http.StatusOK, result)
	case "refresh_token":
		refreshToken := strings.TrimSpace(string(reqCtx.FormValue("refresh_token")))
		fromCookie := false
		if refreshToken == "" {
			refreshToken = readRefreshCookie(reqCtx, c.cfg.RefreshCookieName, c.cfg.RefreshCookieSecure)
			fromCookie = refreshToken != ""
		}
		if fromCookie {
			if !c.validateRefreshCookieFallbackOrigin(reqCtx) {
				writeTokenError(reqCtx, apperrors.Params("refresh token cookie fallback requires trusted request origin"))
				return
			}
			bundle, bundleErr := c.service.ExchangeRefreshTokenFromCookie(ctx, clientID, clientSecret, refreshToken)
			if bundleErr != nil {
				writeTokenError(reqCtx, bundleErr)
				return
			}
			result := map[string]any{
				"access_token": bundle.AccessToken,
				"token_type":   bundle.TokenType,
				"expires_in":   bundle.ExpiresInSeconds,
				"scope":        bundle.Scope,
			}
			if strings.TrimSpace(bundle.IDToken) != "" {
				result["id_token"] = bundle.IDToken
			}
			if strings.TrimSpace(bundle.RefreshToken) != "" {
				if bundle.RefreshTokenBodyAllowed {
					result["refresh_token"] = bundle.RefreshToken
				}
				if bundle.RefreshTokenExpiresAt != nil {
					reqCtx.Response.Header.Add("Set-Cookie", c.service.BuildRefreshCookie(bundle.RefreshToken, *bundle.RefreshTokenExpiresAt))
				}
			}
			reqCtx.JSON(http.StatusOK, result)
			return
		}
		bundle, bundleErr := c.service.ExchangeRefreshToken(ctx, clientID, clientSecret, refreshToken)
		if bundleErr != nil {
			writeTokenError(reqCtx, bundleErr)
			return
		}
		result := map[string]any{
			"access_token": bundle.AccessToken,
			"token_type":   bundle.TokenType,
			"expires_in":   bundle.ExpiresInSeconds,
			"scope":        bundle.Scope,
		}
		if strings.TrimSpace(bundle.IDToken) != "" {
			result["id_token"] = bundle.IDToken
		}
		if strings.TrimSpace(bundle.RefreshToken) != "" {
			if bundle.RefreshTokenBodyAllowed {
				result["refresh_token"] = bundle.RefreshToken
			}
			if bundle.RefreshTokenExpiresAt != nil {
				reqCtx.Response.Header.Add("Set-Cookie", c.service.BuildRefreshCookie(bundle.RefreshToken, *bundle.RefreshTokenExpiresAt))
			}
		}
		reqCtx.JSON(http.StatusOK, result)
	default:
		reqCtx.JSON(http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
	}
}

func (c *Handler) UserInfo(ctx context.Context, reqCtx *app.RequestContext) {
	ctx = withRequestTrace(ctx, reqCtx)
	if !c.allowOAuthRoute(ctx, reqCtx, "userinfo", userInfoRateLimitKey(reqCtx), c.cfg.UserInfoRateLimit, c.cfg.UserInfoRateLimitWindow) {
		return
	}
	item, err := c.service.GetUserInfo(ctx, extractBearer(reqCtx))
	if err != nil {
		reqCtx.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid_token"})
		return
	}
	reqCtx.JSON(http.StatusOK, item)
}

func (c *Handler) ClientCapabilities(ctx context.Context, reqCtx *app.RequestContext) {
	response.Success(reqCtx, c.service.ClientCapabilities(ctx))
}

func (c *Handler) ListClients(ctx context.Context, reqCtx *app.RequestContext) {
	var query ssofacade.ClientAdminQuery
	if err := httpx.Bind(reqCtx, &query); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.service.ListClients(ctx, query)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) GetClient(ctx context.Context, reqCtx *app.RequestContext) {
	item, err := c.service.GetClient(ctx, strings.TrimSpace(string(reqCtx.Param("clientId"))))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) ListClientRedirectURIs(ctx context.Context, reqCtx *app.RequestContext) {
	items, err := c.service.ListClientRedirectURIs(ctx, strings.TrimSpace(string(reqCtx.Param("clientId"))))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) ListClientSecrets(ctx context.Context, reqCtx *app.RequestContext) {
	items, err := c.service.ListClientSecrets(ctx, strings.TrimSpace(string(reqCtx.Param("clientId"))))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) CreateClient(ctx context.Context, reqCtx *app.RequestContext) {
	var request ssofacade.ClientAdminSaveRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding, err := ssoapp.BuildClientAdminCreateOperationBinding(request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionSSOClientCreate, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := c.service.CreateClient(ctx, actorID, request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) UpdateClient(ctx context.Context, reqCtx *app.RequestContext) {
	clientID := strings.TrimSpace(string(reqCtx.Param("clientId")))
	var request ssofacade.UpdateClientAdminRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding, err := ssoapp.BuildClientAdminUpdateOperationBinding(clientID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionSSOClientUpdate, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := c.service.UpdateClient(ctx, actorID, clientID, request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) UpdateClientStatus(ctx context.Context, reqCtx *app.RequestContext) {
	clientID := strings.TrimSpace(string(reqCtx.Param("clientId")))
	var request ssofacade.ClientStatusRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding, err := ssoapp.BuildClientStatusOperationBinding(clientID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionSSOClientStatusChange, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.service.UpdateClientStatus(ctx, actorID, clientID, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) UpdateClientRedirectURIs(ctx context.Context, reqCtx *app.RequestContext) {
	clientID := strings.TrimSpace(string(reqCtx.Param("clientId")))
	var request ssofacade.ClientRedirectURIUpdateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding, err := ssoapp.BuildClientRedirectURIsOperationBinding(clientID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionSSOClientRedirectEdit, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := c.service.UpdateClientRedirectURIs(ctx, actorID, clientID, request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) GenerateClientSecret(ctx context.Context, reqCtx *app.RequestContext) {
	clientID := strings.TrimSpace(string(reqCtx.Param("clientId")))
	var request ssofacade.ClientSecretGenerateRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding, err := ssoapp.BuildClientSecretGenerateOperationBinding(clientID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionSSOClientSecretGenerate, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := c.service.GenerateClientSecret(ctx, actorID, clientID, request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) DisableClientSecret(ctx context.Context, reqCtx *app.RequestContext) {
	clientID := strings.TrimSpace(string(reqCtx.Param("clientId")))
	secretID, err := strconv.ParseInt(strings.TrimSpace(string(reqCtx.Param("secretId"))), 10, 64)
	if err != nil || secretID <= 0 {
		response.Error(reqCtx, apperrors.Params("secretId无效"))
		return
	}
	var request ssofacade.ClientSecretStatusRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding, err := ssoapp.BuildClientSecretStatusOperationBinding(clientID, secretID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionSSOClientSecretDisable, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.service.DisableClientSecret(ctx, actorID, clientID, secretID, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) validateRefreshCookieFallbackOrigin(reqCtx *app.RequestContext) bool {
	if reqCtx == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Sec-Fetch-Site"))), "cross-site") {
		return false
	}
	return setupdomain.ValidateOrigin(setupdomain.OriginCheckInput{
		Origin:                string(reqCtx.Request.Header.Peek("Origin")),
		Referer:               string(reqCtx.Request.Header.Peek("Referer")),
		SecFetchSite:          string(reqCtx.Request.Header.Peek("Sec-Fetch-Site")),
		SecFetchMode:          string(reqCtx.Request.Header.Peek("Sec-Fetch-Mode")),
		AllowedOriginPatterns: c.refreshCookieFallbackAllowedOrigins(),
		RequireOriginHeader:   true,
	})
}

func (c *Handler) refreshCookieFallbackAllowedOrigins() []string {
	if c == nil {
		return nil
	}
	candidates := []string{c.cfg.FrontendLoginURL, c.cfg.BaseURL, c.cfg.Issuer}
	origins := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		origin := originFromURL(candidate)
		if origin == "" {
			continue
		}
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins
}

func originFromURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return ""
	}
	origin := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" {
		origin += ":" + port
	}
	return origin
}

func (c *Handler) allowOAuthRoute(ctx context.Context, reqCtx *app.RequestContext, routeName string, key string, limit int64, window time.Duration) bool {
	if c == nil || c.limiter == nil || limit <= 0 || window <= 0 {
		return true
	}
	var decision limiterinfra.Decision
	var err error
	if c.cfg.RateLimitFailClosed {
		if strictLimiter, ok := c.limiter.(limiterinfra.FailOpenOverrideLimiter); ok {
			decision, err = strictLimiter.AllowWithFailOpen(ctx, key, limit, window, false)
		} else {
			decision, err = c.limiter.Allow(ctx, key, limit, window)
		}
	} else {
		decision, err = c.limiter.Allow(ctx, key, limit, window)
	}
	if err == nil && decision.Allowed {
		return true
	}
	if err != nil && !errors.Is(err, limiterinfra.ErrRateLimited) && !c.cfg.RateLimitFailClosed {
		return true
	}
	retryAfter := int64(decision.RetryAfter.Seconds())
	if retryAfter <= 0 {
		retryAfter = int64(window.Seconds())
	}
	if retryAfter > 0 {
		reqCtx.Response.Header.Set("Retry-After", strconv.FormatInt(retryAfter, 10))
	}
	reqCtx.JSON(http.StatusTooManyRequests, map[string]any{
		"error":             "rate_limited",
		"error_description": routeName + " rate limit exceeded",
	})
	return false
}

func tokenRateLimitKey(grantType, clientID string, reqCtx *app.RequestContext) string {
	grantType = strings.TrimSpace(grantType)
	if grantType == "" {
		grantType = "unknown"
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = "anonymous"
	}
	return "sso:token:" + grantType + ":client:" + clientID + ":ip:" + rateLimitIP(reqCtx)
}

func userInfoRateLimitKey(reqCtx *app.RequestContext) string {
	bearer := extractBearer(reqCtx)
	if strings.TrimSpace(bearer) != "" {
		return "sso:userinfo:bearer:" + digestForRateLimitKey(bearer)
	}
	return "sso:userinfo:ip:" + rateLimitIP(reqCtx)
}

func digestForRateLimitKey(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func rateLimitIP(reqCtx *app.RequestContext) string {
	value := strings.TrimSpace(xcontext.ResolveClientIP(reqCtx))
	if value == "" {
		return "unknown"
	}
	return value
}

func (c *Handler) Revoke(ctx context.Context, reqCtx *app.RequestContext) {
	ctx = withRequestTrace(ctx, reqCtx)
	clientID, clientSecret, err := resolveClientAuthentication(reqCtx)
	if err != nil {
		reqCtx.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid_client", "error_description": err.Error()})
		return
	}
	if revokeErr := c.service.RevokeTokenForClient(
		ctx,
		clientID,
		clientSecret,
		strings.TrimSpace(string(reqCtx.FormValue("token"))),
		strings.TrimSpace(string(reqCtx.FormValue("token_type_hint"))),
	); revokeErr != nil {
		writeTokenError(reqCtx, revokeErr)
		return
	}
	reqCtx.JSON(http.StatusOK, map[string]any{})
}

func (c *Handler) Introspect(ctx context.Context, reqCtx *app.RequestContext) {
	ctx = withRequestTrace(ctx, reqCtx)
	clientID, clientSecret, err := resolveClientAuthentication(reqCtx)
	if err != nil {
		reqCtx.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid_client", "error_description": err.Error()})
		return
	}
	result, introspectErr := c.service.IntrospectTokenForClient(
		ctx,
		clientID,
		clientSecret,
		strings.TrimSpace(string(reqCtx.FormValue("token"))),
		strings.TrimSpace(string(reqCtx.FormValue("token_type_hint"))),
	)
	if introspectErr != nil {
		writeTokenError(reqCtx, introspectErr)
		return
	}
	reqCtx.JSON(http.StatusOK, result)
}

func withRequestTrace(ctx context.Context, reqCtx *app.RequestContext) context.Context {
	return xcontext.WithTraceID(ctx, xcontext.EnsureTraceID(reqCtx))
}

func (c *Handler) Logout(ctx context.Context, reqCtx *app.RequestContext) {
	revoked := false
	if sessionID := readCookie(reqCtx, c.cfg.SessionCookieName); sessionID != "" {
		revoked, _ = c.service.RevokeSession(ctx, sessionID)
	}
	reqCtx.Response.Header.Add("Set-Cookie", c.service.BuildExpiredSessionCookie())
	for _, cookie := range c.service.BuildExpiredRefreshCookies() {
		reqCtx.Response.Header.Add("Set-Cookie", cookie)
	}
	reqCtx.JSON(http.StatusOK, map[string]any{"revoked": revoked, "revokedCount": boolRevokedCount(revoked)})
}

func (c *Handler) InternalValidate(ctx context.Context, reqCtx *app.RequestContext) {
	principal, err := c.service.ValidateAccessToken(ctx, extractBearer(reqCtx))
	if err != nil {
		appErr := apperrors.From(err)
		xcontext.SetResponseError(reqCtx, appErr.Code(), appErr.Message())
		reqCtx.JSON(http.StatusUnauthorized, response.Result{
			Code:    appErr.Code(),
			Data:    jsoncompat.NormalizeForJSON(appErr.Details()),
			Message: appErr.Message(),
			TraceID: xcontext.TraceID(reqCtx),
		})
		return
	}
	response.Success(reqCtx, principal)
}

func (c *Handler) ListSessions(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, listErr := c.service.ListSessionsByUserID(ctx, userID)
	if listErr != nil {
		response.Error(reqCtx, listErr)
		return
	}
	response.Success(reqCtx, buildSessionViews(items, readCookie(reqCtx, c.cfg.SessionCookieName)))
}

func (c *Handler) DeleteSession(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	targetSessionID := string(reqCtx.Param("sessionId"))
	targetSession, resolveErr := c.service.ResolveActiveSessionRecord(ctx, targetSessionID)
	if resolveErr != nil {
		response.Error(reqCtx, resolveErr)
		return
	}
	if targetSession == nil || targetSession.UserID != userID {
		response.Error(reqCtx, fmt.Errorf("会话不存在或无权操作"))
		return
	}
	ok, revokeErr := c.service.RevokeSession(ctx, targetSessionID)
	if revokeErr != nil {
		response.Error(reqCtx, revokeErr)
		return
	}
	response.Success(reqCtx, map[string]any{"revoked": ok, "revokedCount": boolRevokedCount(ok)})
}

func (c *Handler) ListDevices(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, listErr := c.service.ListSessionsByUserID(ctx, userID)
	if listErr != nil {
		response.Error(reqCtx, listErr)
		return
	}
	response.Success(reqCtx, buildDeviceViews(items, readCookie(reqCtx, c.cfg.SessionCookieName)))
}

func (c *Handler) DeleteDevice(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	deviceID := strings.TrimSpace(string(reqCtx.Param("deviceId")))
	items, listErr := c.service.ListSessionsByUserID(ctx, userID)
	if listErr != nil {
		response.Error(reqCtx, listErr)
		return
	}
	revoked := 0
	for _, item := range items {
		if sessionDeviceKey(item) != deviceID {
			continue
		}
		ok, revokeErr := c.service.RevokeSession(ctx, item.SessionID)
		if revokeErr != nil {
			response.Error(reqCtx, revokeErr)
			return
		}
		if ok {
			revoked++
		}
	}
	response.Success(reqCtx, map[string]any{"revoked": revoked > 0, "revokedCount": revoked})
}

func (c *Handler) LogoutAll(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := resolveCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	revokedCount, revokeErr := c.service.RevokeSessionsByUserID(ctx, userID)
	if revokeErr != nil {
		response.Error(reqCtx, revokeErr)
		return
	}
	reqCtx.Response.Header.Add("Set-Cookie", c.service.BuildExpiredSessionCookie())
	for _, cookie := range c.service.BuildExpiredRefreshCookies() {
		reqCtx.Response.Header.Add("Set-Cookie", cookie)
	}
	response.Success(reqCtx, map[string]any{"revoked": revokedCount > 0, "revokedCount": int(revokedCount)})
}

func (c *Handler) AdminListUserSessions(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parseInt64Path(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, listErr := c.service.ListSessionsByUserID(ctx, userID)
	if listErr != nil {
		response.Error(reqCtx, listErr)
		return
	}
	response.Success(reqCtx, buildSessionViews(items, readCookie(reqCtx, c.cfg.SessionCookieName)))
}

func (c *Handler) AdminKickUserSession(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parseInt64Path(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	targetSessionID := strings.TrimSpace(string(reqCtx.Param("sessionId")))
	if _, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionAdminForceLogout, ssoAdminKickSessionBinding(userID, targetSessionID)); err != nil {
		response.Error(reqCtx, err)
		return
	}
	targetSession, resolveErr := c.service.ResolveActiveSessionRecord(ctx, targetSessionID)
	if resolveErr != nil {
		response.Error(reqCtx, resolveErr)
		return
	}
	if targetSession == nil || targetSession.UserID != userID {
		response.Error(reqCtx, fmt.Errorf("会话不存在或无权操作"))
		return
	}
	ok, revokeErr := c.service.RevokeSession(ctx, targetSessionID)
	if revokeErr != nil {
		response.Error(reqCtx, revokeErr)
		return
	}
	response.Success(reqCtx, map[string]any{"revoked": ok, "revokedCount": boolRevokedCount(ok)})
}

func (c *Handler) AdminLogoutAllUserSessions(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parseInt64Path(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if _, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionAdminForceLogout, ssoAdminLogoutAllBinding(userID)); err != nil {
		response.Error(reqCtx, err)
		return
	}
	revokedCount, revokeErr := c.service.RevokeSessionsByUserID(ctx, userID)
	if revokeErr != nil {
		response.Error(reqCtx, revokeErr)
		return
	}
	response.Success(reqCtx, map[string]any{"revoked": revokedCount > 0, "revokedCount": int(revokedCount)})
}

func (c *Handler) AdminListUserDevices(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parseInt64Path(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, listErr := c.service.ListSessionsByUserID(ctx, userID)
	if listErr != nil {
		response.Error(reqCtx, listErr)
		return
	}
	response.Success(reqCtx, buildDeviceViews(items, readCookie(reqCtx, c.cfg.SessionCookieName)))
}

func (c *Handler) AdminKickUserDevice(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parseInt64Path(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	deviceID := strings.TrimSpace(string(reqCtx.Param("deviceId")))
	if _, err := c.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionAdminForceLogout, ssoAdminKickDeviceBinding(userID, deviceID)); err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, listErr := c.service.ListSessionsByUserID(ctx, userID)
	if listErr != nil {
		response.Error(reqCtx, listErr)
		return
	}
	revoked := 0
	for _, item := range items {
		if sessionDeviceKey(item) != deviceID {
			continue
		}
		ok, revokeErr := c.service.RevokeSession(ctx, item.SessionID)
		if revokeErr != nil {
			response.Error(reqCtx, revokeErr)
			return
		}
		if ok {
			revoked++
		}
	}
	response.Success(reqCtx, map[string]any{"revoked": revoked > 0, "revokedCount": revoked})
}

func (c *Handler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, businessAction challengedomain.BusinessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if c.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildStepUpRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	businessActionValue := string(businessAction)
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseStepUpFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessActionValue)
	if proofToken != "" {
		token, err := c.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{
			ProofToken:       proofToken,
			BusinessAction:   businessActionValue,
			FlowNonce:        flowNonce,
			OperationBinding: operationBinding,
			ConsumeOnce:      true,
		})
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		if token == nil {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof验证失败")
		}
		if !stepUpTokenMatchesProtectedMutation(token, businessActionValue, operationBinding) {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof与当前操作不匹配")
		}
		securitycontext.SetStepUpProofAudit(reqCtx, stepUpProofAuditFromToken(token, businessActionValue, operationBinding))
		return stepUpProofMetadataFromToken(token, businessActionValue, operationBinding), nil
	}
	challenge, err := c.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
		BusinessAction:   businessActionValue,
		FlowNonce:        flowNonce,
		OperationBinding: operationBinding,
	})
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	return stepup.ProofMetadata{}, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"requiredAssuranceLevel":     challenge.RequiredAssuranceLevel,
		"resolvedAssuranceLevel":     challenge.ResolvedAssuranceLevel,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
		"actualChallengeTypeNames":   challenge.ActualChallengeTypeNames,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"businessAction":             businessActionValue,
		"operationBinding":           operationBinding,
	})
}

func stepUpTokenMatchesProtectedMutation(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) bool {
	if token == nil {
		return false
	}
	if strings.TrimSpace(token.BusinessAction) != "" && strings.TrimSpace(token.BusinessAction) != strings.TrimSpace(businessAction) {
		return false
	}
	if strings.TrimSpace(token.OperationBinding) != "" && strings.TrimSpace(token.OperationBinding) != strings.TrimSpace(operationBinding) {
		return false
	}
	return true
}

func buildStepUpRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	user := securitycontext.Require(reqCtx)
	if user.UserID <= 0 {
		return authorizationfacade.RequestScope{}, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return authorizationfacade.RequestScope{
		UserID:    user.UserID,
		Username:  user.Username,
		IPAddress: reqCtx.ClientIP(),
		UserAgent: string(reqCtx.UserAgent()),
		DeviceID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
		SessionID: user.SessionID,
		Source:    user.Source,
	}, nil
}

func stepUpProofAuditFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) securitycontext.StepUpProofAudit {
	if token == nil {
		return securitycontext.StepUpProofAudit{}
	}
	return securitycontext.StepUpProofAudit{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func stepUpProofMetadataFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) stepup.ProofMetadata {
	if token == nil {
		return stepup.ProofMetadata{}
	}
	return stepup.ProofMetadata{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func firstNonBlank(values ...string) string {
	for _, item := range values {
		if value := strings.TrimSpace(item); value != "" {
			return value
		}
	}
	return ""
}

func chooseStepUpFlowNonce(flowNonce, businessAction string) string {
	value := strings.TrimSpace(flowNonce)
	if value != "" {
		return value
	}
	return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func ssoAdminKickSessionBinding(userID int64, sessionID string) string {
	return "sso:user:" + strconv.FormatInt(userID, 10) + "|session:" + strings.TrimSpace(sessionID) + "|force-logout"
}

func ssoAdminLogoutAllBinding(userID int64) string {
	return "sso:user:" + strconv.FormatInt(userID, 10) + "|logout-all"
}

func ssoAdminKickDeviceBinding(userID int64, deviceID string) string {
	return "sso:user:" + strconv.FormatInt(userID, 10) + "|device:" + strings.TrimSpace(deviceID) + "|force-logout"
}

func ssoClientOperationBinding(clientID, operation string) string {
	return "sso:client:" + strings.TrimSpace(clientID) + "|" + strings.TrimSpace(operation)
}

func resolveClientAuthentication(reqCtx *app.RequestContext) (string, string, error) {
	clientID := strings.TrimSpace(string(reqCtx.FormValue("client_id")))
	header := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Authorization")))
	if strings.HasPrefix(header, "Basic ") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(header, "Basic ")))
		if err != nil {
			return "", "", fmt.Errorf("Basic client auth 非法")
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return "", "", fmt.Errorf("Basic client auth 非法")
		}
		clientID = strings.TrimSpace(parts[0])
		return clientID, parts[1], nil
	}
	if clientID == "" {
		return "", "", fmt.Errorf("缺少 client_id")
	}
	return clientID, "", nil
}

func boolRevokedCount(revoked bool) int {
	if revoked {
		return 1
	}
	return 0
}

func resolveRequestContext(reqCtx *app.RequestContext) *ssofacade.RequestContext {
	if reqCtx == nil {
		return nil
	}
	return &ssofacade.RequestContext{
		DeviceID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
		LoginIP:   reqCtx.ClientIP(),
		UserAgent: strings.TrimSpace(string(reqCtx.UserAgent())),
		TraceID:   strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Trace-Id"))),
	}
}

func resolveCurrentUserID(reqCtx *app.RequestContext) (int64, error) {
	if userID, ok := securitycontext.CurrentUserID(reqCtx); ok && userID > 0 {
		return userID, nil
	}
	return 0, apperrors.Unauthorized("未登录或登录信息失效")
}

func parseInt64Path(reqCtx *app.RequestContext, name string) (int64, error) {
	value := strings.TrimSpace(string(reqCtx.Param(name)))
	if value == "" {
		return 0, fmt.Errorf("缺少参数 %s", name)
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("参数 %s 非法", name)
	}
	return id, nil
}

type sessionView struct {
	SessionID      string `json:"sessionId"`
	UserID         int64  `json:"userId"`
	ClientID       string `json:"clientId,omitempty"`
	LoginTime      string `json:"loginTime,omitempty"`
	LastActiveTime string `json:"lastActiveTime,omitempty"`
	ExpireTime     string `json:"expireTime,omitempty"`
	IPAddress      string `json:"ipAddress,omitempty"`
	LoginIP        string `json:"loginIp,omitempty"`
	DeviceID       string `json:"deviceId,omitempty"`
	DeviceInfo     string `json:"deviceInfo,omitempty"`
	UserAgent      string `json:"userAgent,omitempty"`
	CurrentSession bool   `json:"currentSession"`
	Revoked        bool   `json:"revoked"`
}

type deviceView struct {
	DeviceID       string   `json:"deviceId"`
	DeviceInfo     string   `json:"deviceInfo,omitempty"`
	SessionCount   int      `json:"sessionCount"`
	LastActiveTime string   `json:"lastActiveTime,omitempty"`
	CurrentDevice  bool     `json:"currentDevice"`
	IPSamples      []string `json:"ipSamples,omitempty"`
}

func buildSessionViews(items []ssofacade.SessionRecord, currentSessionID string) []sessionView {
	result := make([]sessionView, 0, len(items))
	for _, item := range items {
		result = append(result, sessionView{
			SessionID:      item.SessionID,
			UserID:         item.UserID,
			ClientID:       item.ClientID,
			LoginTime:      formatTimePtr(item.LoginAt),
			LastActiveTime: formatTimePtr(firstTimePtr(item.LastAccessAt, item.LoginAt)),
			ExpireTime:     formatTimePtr(item.ExpiresAt),
			IPAddress:      item.LoginIP,
			LoginIP:        item.LoginIP,
			DeviceID:       sessionDeviceKey(item),
			DeviceInfo:     sessionDeviceInfo(item),
			UserAgent:      item.UserAgent,
			CurrentSession: currentSessionID != "" && item.SessionID == currentSessionID,
			Revoked:        item.RevokedAt != nil || strings.EqualFold(item.Status, "REVOKED"),
		})
	}
	return result
}

func buildDeviceViews(items []ssofacade.SessionRecord, currentSessionID string) []deviceView {
	currentDeviceID := ""
	buckets := make(map[string]*deviceView)
	for _, item := range items {
		deviceID := sessionDeviceKey(item)
		view := buckets[deviceID]
		if view == nil {
			view = &deviceView{
				DeviceID:   deviceID,
				DeviceInfo: sessionDeviceInfo(item),
			}
			buckets[deviceID] = view
		}
		view.SessionCount++
		if item.SessionID == currentSessionID {
			currentDeviceID = deviceID
			view.CurrentDevice = true
		}
		lastActive := firstTimePtr(item.LastAccessAt, item.LoginAt)
		if isAfterFormatted(lastActive, view.LastActiveTime) {
			view.LastActiveTime = formatTimePtr(lastActive)
		}
		if strings.TrimSpace(item.LoginIP) != "" && !containsString(view.IPSamples, item.LoginIP) {
			view.IPSamples = append(view.IPSamples, item.LoginIP)
		}
	}
	result := make([]deviceView, 0, len(buckets))
	for _, item := range buckets {
		if currentDeviceID != "" && item.DeviceID == currentDeviceID {
			item.CurrentDevice = true
		}
		result = append(result, *item)
	}
	return result
}

func sessionDeviceKey(item ssofacade.SessionRecord) string {
	if strings.TrimSpace(item.DeviceID) != "" {
		return strings.TrimSpace(item.DeviceID)
	}
	if strings.TrimSpace(item.UserAgent) != "" {
		return "ua:" + strconv.FormatUint(uint64(fnv32a(item.UserAgent)), 36)
	}
	return "session:" + item.SessionID
}

func sessionDeviceInfo(item ssofacade.SessionRecord) string {
	if strings.TrimSpace(item.UserAgent) != "" {
		return strings.TrimSpace(item.UserAgent)
	}
	if strings.TrimSpace(item.DeviceID) != "" {
		return strings.TrimSpace(item.DeviceID)
	}
	return "未知设备"
}

func firstTimePtr(items ...*time.Time) *time.Time {
	for _, item := range items {
		if item != nil && !item.IsZero() {
			return item
		}
	}
	return nil
}

func isAfterFormatted(candidate *time.Time, current string) bool {
	if candidate == nil {
		return false
	}
	if strings.TrimSpace(current) == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339, current)
	if err != nil {
		return true
	}
	return candidate.After(parsed)
}

func formatTimePtr(item *time.Time) string {
	if item == nil || item.IsZero() {
		return ""
	}
	return item.Format(time.RFC3339)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func fnv32a(value string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(value); i++ {
		hash ^= uint32(value[i])
		hash *= 16777619
	}
	return hash
}

func readCookie(reqCtx *app.RequestContext, name string) string {
	if reqCtx == nil || strings.TrimSpace(name) == "" {
		return ""
	}
	cookie := reqCtx.Request.Header.Cookie(name)
	return strings.TrimSpace(string(cookie))
}

func readRefreshCookie(reqCtx *app.RequestContext, configuredName string, secure bool) string {
	for _, name := range refreshCookieCandidateNames(configuredName, secure) {
		if value := readCookie(reqCtx, name); value != "" {
			return value
		}
	}
	return ""
}

func extractBearer(reqCtx *app.RequestContext) string {
	header := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Authorization")))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func buildAuthorizeContinueURL(baseURL string, reqCtx *app.RequestContext) string {
	if reqCtx == nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	target := base + "/oauth2/authorize"
	if query := strings.TrimSpace(string(reqCtx.URI().QueryString())); query != "" {
		target += "?" + query
	}
	return target
}

func refreshCookieCandidateNames(configuredName string, secure bool) []string {
	name := strings.TrimSpace(configuredName)
	if name == "" {
		return nil
	}
	result := make([]string, 0, 2)
	writeName := name
	if !secure && strings.HasPrefix(name, "__Host-") {
		writeName = strings.TrimPrefix(name, "__Host-")
	}
	if writeName != "" {
		result = append(result, writeName)
	}
	if name != "" && name != writeName {
		result = append(result, name)
	}
	return result
}

func writeTokenError(reqCtx *app.RequestContext, err error) {
	appErr := apperrors.From(err)
	if appErr == nil {
		reqCtx.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid_grant"})
		return
	}
	status := http.StatusUnauthorized
	code := "invalid_grant"
	switch appErr.Kind() {
	case apperrors.KindParams:
		status = http.StatusBadRequest
		code = "invalid_request"
		if strings.Contains(appErr.Message(), "scope") || strings.Contains(appErr.Message(), "openid") {
			code = "invalid_scope"
		}
	case apperrors.KindOperation:
		status = http.StatusConflict
		code = "concurrent_request"
	case apperrors.KindAuth:
		if strings.Contains(strings.ToLower(appErr.Message()), "client") || strings.Contains(appErr.Message(), "客户端") {
			code = "invalid_client"
		}
	}
	reqCtx.JSON(status, map[string]any{
		"error":             code,
		"error_description": appErr.Message(),
	})
}

func splitScope(value string) []string {
	parts := strings.Fields(strings.TrimSpace(value))
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		if strings.TrimSpace(item) != "" {
			result = append(result, strings.TrimSpace(item))
		}
	}
	return result
}

func (c *Handler) redirectAuthorizeError(reqCtx *app.RequestContext, redirectURI, errCode, description, state string) {
	if parsed, err := url.Parse(strings.TrimSpace(redirectURI)); err == nil && strings.TrimSpace(redirectURI) != "" {
		query := parsed.Query()
		query.Set("error", errCode)
		if strings.TrimSpace(description) != "" {
			query.Set("error_description", description)
		}
		if strings.TrimSpace(state) != "" {
			query.Set("state", state)
		}
		parsed.RawQuery = query.Encode()
		reqCtx.Response.Header.Set("Location", parsed.String())
		reqCtx.SetStatusCode(http.StatusFound)
		return
	}
	reqCtx.JSON(http.StatusBadRequest, map[string]any{"error": errCode, "error_description": description})
}
