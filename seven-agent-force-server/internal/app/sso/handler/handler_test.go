package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	externaldomain "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	externaldrivers "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/infrastructure/drivers"
	ssoapp "github.com/CaixyPromise/seven-agent/force/internal/app/sso/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssoinfra "github.com/CaixyPromise/seven-agent/force/internal/app/sso/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	limiterinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/limiter"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/jmoiron/sqlx"
)

func TestManagedHubGenericOIDCDriverExchangesThroughRealSSOHandler(t *testing.T) {
	const (
		issuer       = "https://hub.example.com/"
		clientID     = "hub-node-order-admin"
		clientSecret = "managed-secret"
		redirectURI  = "https://node.example.com/login/external/hub:order-admin/callback"
		codeVerifier = "valid-code-verifier-abcdefghijklmnopqrstuvwxyz"
		nonce        = "managed-nonce"
	)
	fixture, secretHash := newSSOHandlerProtocolFixture(t, issuer, clientSecret)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{Issuer: issuer, BaseURL: "https://hub.example.com"})
	engine.GET("/.well-known/openid-configuration", handler.Discovery)
	engine.GET("/.well-known/jwks.json", handler.JWKS)
	engine.GET("/sso/.well-known/jwks.json", handler.JWKS)
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	client := domain.Client{
		ID: 1, ClientID: clientID, ClientName: "Managed Node", ClientType: "confidential", ClientAuthMethod: "client_secret_basic",
		GrantTypes: []string{"authorization_code", "refresh_token"}, Scopes: []string{"openid", "profile", "email"}, RequirePKCE: true, Status: domain.ClientStatusActive,
	}
	fixture.expectConfidentialClientLookup(client, secretHash)
	code := domain.AuthorizationCode{
		ID: 10, Code: "managed-code", ClientID: clientID, UserID: 1001, SessionID: "managed-session", RedirectURI: redirectURI,
		Scopes: []string{"openid", "profile", "email"}, CodeChallenge: ssoHandlerTestPKCEChallenge(codeVerifier), CodeChallengeMethod: "S256",
		Nonce: nonce, ExpiresAt: now.Add(5 * time.Minute), Status: domain.CodeStatusActive, CreateTime: now, UpdateTime: now,
	}
	fixture.expectAuthorizationCodeLookup(code)
	fixture.expectConsumeAuthorizationCode(code.Code)
	fixture.expectActiveSessionLookup(code.SessionID, domain.Session{
		ID: 20, SessionID: code.SessionID, ClientID: clientID, UserID: code.UserID, LoginAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Status: domain.SessionStatusActive,
	})
	fixture.expectSSOAuditLog("TOKEN_EXCHANGED", clientID, "SUCCESS", "exchanged", `"grantType":"authorization_code"`, `"refreshIssued":false`)

	transport := &hertzEngineRoundTripper{engine: engine}
	driver := externaldrivers.NewOIDCDriver(externaldrivers.WithOIDCHTTPClient(&http.Client{Transport: transport}))
	discovery, err := driver.Discover(context.Background(), issuer)
	if err != nil {
		t.Fatalf("discover real Hub SSO: %v", err)
	}
	provider := externaldomain.Provider{
		ProviderCode: "hub:order-admin", ProtocolType: externaldomain.ProtocolTypeOIDC, Issuer: discovery.Issuer,
		AuthorizationEndpoint: discovery.AuthorizationEndpoint, TokenEndpoint: discovery.TokenEndpoint, JWKSURI: discovery.JWKSURI,
		ClientID: clientID, RedirectURI: redirectURI, TokenEndpointAuthMethod: externaldomain.TokenEndpointAuthMethodClientSecretBasic,
	}
	result, err := driver.ExchangeCode(context.Background(), provider, externaldrivers.TokenExchangeRequest{
		Code: code.Code, CodeVerifier: codeVerifier, RedirectURI: redirectURI, ClientSecret: clientSecret,
		Nonce: nonce, ExpectedIssuer: issuer, Scopes: []string{"openid", "profile", "email"},
	})
	if err != nil {
		t.Fatalf("exchange through real Hub handler: %v", err)
	}
	if !transport.tokenUsedBasic || result.TokenSet.IDToken == "" || result.ExpectedIssuer != issuer {
		t.Fatalf("protocol result basic=%v issuer=%q idToken=%v", transport.tokenUsedBasic, result.ExpectedIssuer, result.TokenSet.IDToken != "")
	}
	if transport.tokenForm.Get("client_id") != "" || transport.tokenForm.Get("client_secret") != "" {
		t.Fatalf("managed token request leaked credentials into form: %v", transport.tokenForm)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

const ssoHandlerTestRedirectURI = "http://127.0.0.1:5177/sso/callback"

func ssoHandlerTestPKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestTokenRouteAuthorizationCodeMintsDisposableRefreshToken(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	codeVerifier := "valid-code-verifier-abcdefghijklmnopqrstuvwxyz"
	code := domain.AuthorizationCode{
		ID:                  10,
		Code:                "code-live-route",
		ClientID:            "client-route",
		UserID:              1001,
		SessionID:           "session-route",
		RedirectURI:         ssoHandlerTestRedirectURI,
		Scopes:              []string{"openid", "offline_access", "profile"},
		CodeChallenge:       ssoHandlerTestPKCEChallenge(codeVerifier),
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(5 * time.Minute),
		Status:              domain.CodeStatusActive,
		CreateTime:          now,
		UpdateTime:          now,
	}
	session := domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectClientLookup(domain.Client{
		ID:               1,
		ClientID:         "client-route",
		ClientName:       "route client",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	fixture.expectAuthorizationCodeLookup(code)
	fixture.expectConsumeAuthorizationCode(code.Code)
	fixture.expectActiveSessionLookup(session.SessionID, session)
	fixture.expectInsertRefreshFamily()
	fixture.expectSSOAuditLogWithTrace("TOKEN_EXCHANGED", "client-route", "SUCCESS", "exchanged", "0123456789abcdef0123456789abcdef", `"grantType":"authorization_code"`, `"refreshIssued":true`)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", "client-route")
	form.Set("code", code.Code)
	form.Set("redirect_uri", ssoHandlerTestRedirectURI)
	form.Set("code_verifier", codeVerifier)
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
		ut.Header{Key: "X-Trace-Id", Value: "0123456789abcdef0123456789abcdef"},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal token response: %v", err)
	}
	if body["token_type"] != "Bearer" {
		t.Fatalf("token_type=%v, want Bearer", body["token_type"])
	}
	if body["access_token"] == "" || body["refresh_token"] == "" {
		t.Fatalf("token response missing access or refresh token: %v", body)
	}
	if setCookie := recorder.Header().Get("Set-Cookie"); setCookie == "" {
		t.Fatal("token response did not set refresh cookie")
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteAuthorizationCodeOmitsRefreshTokenBodyForTrustedFirstPartyClient(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	codeVerifier := "valid-code-verifier-abcdefghijklmnopqrstuvwxyz"
	code := domain.AuthorizationCode{
		ID:                  10,
		Code:                "code-live-route",
		ClientID:            "client-route",
		UserID:              1001,
		SessionID:           "session-route",
		RedirectURI:         ssoHandlerTestRedirectURI,
		Scopes:              []string{"openid", "offline_access", "profile"},
		CodeChallenge:       ssoHandlerTestPKCEChallenge(codeVerifier),
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(5 * time.Minute),
		Status:              domain.CodeStatusActive,
		CreateTime:          now,
		UpdateTime:          now,
	}
	session := domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectClientLookup(domain.Client{
		ID:                1,
		ClientID:          "client-route",
		ClientName:        "route client",
		ClientType:        "public",
		ClientAuthMethod:  "none",
		GrantTypes:        []string{"authorization_code", "refresh_token"},
		Scopes:            []string{"openid", "offline_access", "profile"},
		TrustedFirstParty: true,
		Status:            domain.ClientStatusActive,
	})
	fixture.expectAuthorizationCodeLookup(code)
	fixture.expectConsumeAuthorizationCode(code.Code)
	fixture.expectActiveSessionLookup(session.SessionID, session)
	fixture.expectInsertRefreshFamily()
	fixture.expectSSOAuditLogWithTrace("TOKEN_EXCHANGED", "client-route", "SUCCESS", "exchanged", "0123456789abcdef0123456789abcdef", `"grantType":"authorization_code"`, `"refreshIssued":true`)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", "client-route")
	form.Set("code", code.Code)
	form.Set("redirect_uri", ssoHandlerTestRedirectURI)
	form.Set("code_verifier", codeVerifier)
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
		ut.Header{Key: "X-Trace-Id", Value: "0123456789abcdef0123456789abcdef"},
	)

	body := decodeTokenRouteBody(t, recorder)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body["access_token"] == "" {
		t.Fatalf("token response missing access token: %v", body)
	}
	if body["refresh_token"] != nil {
		t.Fatalf("trusted first-party authorization_code response leaked refresh_token body: %v", body)
	}
	if setCookie := recorder.Header().Get("Set-Cookie"); setCookie == "" || !strings.Contains(setCookie, "seven_refresh_token=") {
		t.Fatalf("trusted first-party authorization_code response did not set refresh cookie: %q", setCookie)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteRejectsSameRefreshTokenReplayAfterRotation(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	refreshToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-route"},
		"client_id":  "client-route",
		"sid":        "session-route",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-route",
	})
	hash := ssoinfra.BuildTokenHash(refreshToken)
	rotatedAt := now.Add(-5 * time.Second)
	fixture.expectClientLookup(domain.Client{
		ID:               1,
		ClientID:         "client-route",
		ClientName:       "route client",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	fixture.expectMissingRefreshFamilyByCurrentHash(hash)
	fixture.expectRefreshFamilyByPreviousHash(hash, domain.RefreshTokenFamily{
		ID:                30,
		FamilyID:          "family-route",
		SessionID:         "session-route",
		ClientID:          "client-route",
		UserID:            1001,
		CurrentTokenHash:  "next-refresh-token-hash",
		PreviousTokenHash: hash,
		RotatedAt:         &rotatedAt,
		ExpiresAt:         now.Add(time.Hour),
		Status:            domain.RefreshFamilyStatusActive,
		CreateTime:        now,
		UpdateTime:        now,
	})
	fixture.expectSSOAuditLog("TOKEN_REFRESH_REUSE_DETECTED", "client-route", "FAILURE", "rotation_skew_replay", `"grantType":"refresh_token"`, `"punished":false`)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")
	form.Set("refresh_token", refreshToken)
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
	)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal token response: %v", err)
	}
	if body["error"] != "invalid_grant" {
		t.Fatalf("error=%v, want invalid_grant", body["error"])
	}
	if body["access_token"] != nil || body["refresh_token"] != nil || body["id_token"] != nil {
		t.Fatalf("replay response minted token fields: %v", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteSameRefreshTokenConditionalRaceMintsAtMostOneReplacement(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	refreshToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-route"},
		"client_id":  "client-route",
		"sid":        "session-route",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-route-race",
	})
	hash := ssoinfra.BuildTokenHash(refreshToken)
	family := domain.RefreshTokenFamily{
		ID:               31,
		FamilyID:         "family-route-race",
		SessionID:        "session-route",
		ClientID:         "client-route",
		UserID:           1001,
		CurrentTokenHash: hash,
		ExpiresAt:        now.Add(time.Hour),
		Status:           domain.RefreshFamilyStatusActive,
		MetadataJSON:     mustSSOHandlerTestJSON([]string{"openid", "offline_access"}),
		CreateTime:       now,
		UpdateTime:       now,
	}
	session := domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	for _, affectedRows := range []int64{1, 0} {
		fixture.expectClientLookup(domain.Client{
			ID:               1,
			ClientID:         "client-route",
			ClientName:       "route client",
			ClientType:       "public",
			ClientAuthMethod: "none",
			GrantTypes:       []string{"authorization_code", "refresh_token"},
			Scopes:           []string{"openid", "offline_access", "profile"},
			Status:           domain.ClientStatusActive,
		})
		fixture.expectRefreshFamilyByCurrentHash(hash, family)
		fixture.expectActiveSessionLookup(session.SessionID, session)
		fixture.expectRotateRefreshFamily(family.FamilyID, hash, affectedRows)
		if affectedRows == 1 {
			fixture.expectSSOAuditLog("TOKEN_REFRESHED", "client-route", "SUCCESS", "refreshed", `"grantType":"refresh_token"`, `"rotated":true`)
		} else {
			fixture.expectSSOAuditLog("TOKEN_REFRESHED", "client-route", "FAILURE", "rotation_conflict", `"grantType":"refresh_token"`, `"rotated":false`)
		}
	}

	first := performRefreshTokenRequest(engine, refreshToken)
	second := performRefreshTokenRequest(engine, refreshToken)

	firstBody := decodeTokenRouteBody(t, first)
	secondBody := decodeTokenRouteBody(t, second)
	if first.Code != http.StatusOK {
		t.Fatalf("first refresh status=%d body=%s", first.Code, first.Body.String())
	}
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("second refresh status=%d body=%s", second.Code, second.Body.String())
	}
	if firstBody["access_token"] == "" || firstBody["refresh_token"] == "" {
		t.Fatalf("first refresh did not mint replacement tokens: %v", firstBody)
	}
	if secondBody["access_token"] != nil || secondBody["refresh_token"] != nil || secondBody["id_token"] != nil {
		t.Fatalf("conditional race loser minted token fields: %v", secondBody)
	}
	if secondBody["error"] != "invalid_grant" {
		t.Fatalf("second error=%v, want invalid_grant", secondBody["error"])
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteRefreshGrantRateLimitPreservesInitialOAuthErrorThenLimits(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	limiter := newFakeRouteLimiter(2)
	handler := NewHandler(fixture.service, ConfigView{
		TokenRateLimit:       2,
		TokenRateLimitWindow: time.Minute,
	})
	handler.BindLimiter(limiter)
	engine.POST("/sso/oauth2/token", handler.Token)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")

	first := performTokenFormRequest(engine, form)
	second := performTokenFormRequest(engine, form)
	third := performTokenFormRequest(engine, form)

	if first.Code != http.StatusBadRequest || decodeTokenRouteBody(t, first)["error"] != "invalid_request" {
		t.Fatalf("first refresh request should preserve OAuth invalid_request, status=%d body=%s", first.Code, first.Body.String())
	}
	if second.Code != http.StatusBadRequest || decodeTokenRouteBody(t, second)["error"] != "invalid_request" {
		t.Fatalf("second refresh request should preserve OAuth invalid_request, status=%d body=%s", second.Code, second.Body.String())
	}
	assertOAuthRateLimited(t, third)
	if !limiter.sawKeyPrefix("sso:token:refresh_token:client:client-route") {
		t.Fatalf("expected refresh token route limiter key by client/grant, keys=%v", limiter.keys)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteLimiterFailureCanFailClosed(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	limiter := &strictFailingRouteLimiter{}
	handler := NewHandler(fixture.service, ConfigView{
		TokenRateLimit:       1,
		TokenRateLimitWindow: time.Minute,
		RateLimitFailClosed:  true,
	})
	handler.BindLimiter(limiter)
	engine.POST("/sso/oauth2/token", handler.Token)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")

	recorder := performTokenFormRequest(engine, form)
	assertOAuthRateLimited(t, recorder)
	if !limiter.strictCalled {
		t.Fatalf("expected SSO token fail-closed config to use strict limiter override")
	}
	if limiter.strictFailOpen {
		t.Fatalf("expected SSO token strict limiter override to force fail-open=false")
	}
	if limiter.allowCalled {
		t.Fatalf("expected SSO token strict limiter override to avoid ordinary Allow and prevent double counting")
	}
}

func TestTokenRouteRefreshCookieFallbackRequiresTrustedFirstPartyClient(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
		FrontendLoginURL:    "https://console.example.com/login",
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	refreshToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-route"},
		"client_id":  "client-route",
		"sid":        "session-route",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-cookie-fallback",
	})
	fixture.expectClientLookup(domain.Client{
		ID:                1,
		ClientID:          "client-route",
		ClientName:        "route client",
		ClientType:        "public",
		ClientAuthMethod:  "none",
		GrantTypes:        []string{"authorization_code", "refresh_token"},
		Scopes:            []string{"openid", "offline_access", "profile"},
		TrustedFirstParty: false,
		Status:            domain.ClientStatusActive,
	})

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
		ut.Header{Key: "Origin", Value: "https://console.example.com"},
		ut.Header{Key: "Sec-Fetch-Site", Value: "same-origin"},
		ut.Header{Key: "Cookie", Value: "seven_refresh_token=" + refreshToken},
	)

	body := decodeTokenRouteBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["error"] != "invalid_request" {
		t.Fatalf("cookie fallback for non trusted first-party client should be invalid_request, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body["access_token"] != nil || body["refresh_token"] != nil || body["id_token"] != nil {
		t.Fatalf("cookie fallback denial leaked token fields: %v", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteRefreshCookieFallbackAllowsTrustedFirstPartyClient(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
		FrontendLoginURL:    "https://console.example.com/login",
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	refreshToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-route"},
		"client_id":  "client-route",
		"sid":        "session-route",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-cookie-trusted",
	})
	hash := ssoinfra.BuildTokenHash(refreshToken)
	family := domain.RefreshTokenFamily{
		ID:               32,
		FamilyID:         "family-route-cookie-trusted",
		SessionID:        "session-route",
		ClientID:         "client-route",
		UserID:           1001,
		CurrentTokenHash: hash,
		ExpiresAt:        now.Add(time.Hour),
		Status:           domain.RefreshFamilyStatusActive,
		MetadataJSON:     mustSSOHandlerTestJSON([]string{"openid", "offline_access"}),
		CreateTime:       now,
		UpdateTime:       now,
	}
	session := domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectClientLookup(domain.Client{
		ID:                1,
		ClientID:          "client-route",
		ClientName:        "route client",
		ClientType:        "public",
		ClientAuthMethod:  "none",
		GrantTypes:        []string{"authorization_code", "refresh_token"},
		Scopes:            []string{"openid", "offline_access", "profile"},
		TrustedFirstParty: true,
		Status:            domain.ClientStatusActive,
	})
	fixture.expectRefreshFamilyByCurrentHash(hash, family)
	fixture.expectActiveSessionLookup(session.SessionID, session)
	fixture.expectRotateRefreshFamily(family.FamilyID, hash, 1)
	fixture.expectSSOAuditLog("TOKEN_REFRESHED", "client-route", "SUCCESS", "refreshed", `"grantType":"refresh_token"`, `"rotated":true`)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
		ut.Header{Key: "Origin", Value: "https://console.example.com"},
		ut.Header{Key: "Sec-Fetch-Site", Value: "same-origin"},
		ut.Header{Key: "Cookie", Value: "seven_refresh_token=" + refreshToken},
	)

	body := decodeTokenRouteBody(t, recorder)
	if recorder.Code != http.StatusOK {
		t.Fatalf("trusted first-party cookie fallback status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body["access_token"] == "" {
		t.Fatalf("trusted first-party cookie fallback did not mint access token: %v", body)
	}
	if body["refresh_token"] != nil {
		t.Fatalf("trusted first-party cookie fallback leaked refresh_token in response body: %v", body)
	}
	if setCookie := recorder.Header().Get("Set-Cookie"); setCookie == "" || !strings.Contains(setCookie, "seven_refresh_token=") {
		t.Fatalf("trusted first-party cookie fallback did not rotate refresh cookie: %q", setCookie)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteRefreshCookieFallbackRejectsCrossSiteOrigin(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
		FrontendLoginURL:    "https://console.example.com/login",
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	refreshToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-route"},
		"client_id":  "client-route",
		"sid":        "session-route",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-cookie-cross-site",
	})

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
		ut.Header{Key: "Origin", Value: "https://evil.example.net"},
		ut.Header{Key: "Sec-Fetch-Site", Value: "cross-site"},
		ut.Header{Key: "Cookie", Value: "seven_refresh_token=" + refreshToken},
	)

	body := decodeTokenRouteBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["error"] != "invalid_request" {
		t.Fatalf("cross-site cookie fallback should be invalid_request, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body["access_token"] != nil || body["refresh_token"] != nil || body["id_token"] != nil {
		t.Fatalf("cross-site cookie fallback denial leaked token fields: %v", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTokenRouteRefreshCookieFallbackRejectsMissingOrigin(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		RefreshCookieName:   "seven_refresh_token",
		RefreshCookieSecure: false,
		FrontendLoginURL:    "https://console.example.com/login",
	})
	engine.POST("/sso/oauth2/token", handler.Token)

	now := time.Now().UTC()
	refreshToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-route"},
		"client_id":  "client-route",
		"sid":        "session-route",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-cookie-missing-origin",
	})

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
		ut.Header{Key: "Sec-Fetch-Site", Value: "same-origin"},
		ut.Header{Key: "Cookie", Value: "seven_refresh_token=" + refreshToken},
	)

	body := decodeTokenRouteBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["error"] != "invalid_request" {
		t.Fatalf("missing-origin cookie fallback should be invalid_request, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body["access_token"] != nil || body["refresh_token"] != nil || body["id_token"] != nil {
		t.Fatalf("missing-origin cookie fallback denial leaked token fields: %v", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRefreshCookieFallbackAllowedOriginsNormalizeConfiguredURLs(t *testing.T) {
	handler := NewHandler(nil, ConfigView{
		FrontendLoginURL: "https://console.example.com/login?next=/home",
		BaseURL:          "https://auth.example.com/api/sso",
		Issuer:           "https://auth.example.com/api/sso",
	})

	origins := handler.refreshCookieFallbackAllowedOrigins()
	want := []string{"https://console.example.com", "https://auth.example.com"}
	if len(origins) != len(want) {
		t.Fatalf("origins=%v, want %v", origins, want)
	}
	for i := range want {
		if origins[i] != want[i] {
			t.Fatalf("origins=%v, want %v", origins, want)
		}
	}
}

func TestUserInfoRouteRateLimitPreservesInitialInvalidTokenThenLimits(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	limiter := newFakeRouteLimiter(2)
	handler := NewHandler(fixture.service, ConfigView{
		UserInfoRateLimit:       2,
		UserInfoRateLimitWindow: time.Minute,
	})
	handler.BindLimiter(limiter)
	engine.GET("/sso/oauth2/userinfo", handler.UserInfo)

	first := performUserInfoRequest(engine, "Bearer invalid-token-live")
	second := performUserInfoRequest(engine, "Bearer invalid-token-live")
	third := performUserInfoRequest(engine, "Bearer invalid-token-live")

	if first.Code != http.StatusUnauthorized || decodeTokenRouteBody(t, first)["error"] != "invalid_token" {
		t.Fatalf("first userinfo request should preserve invalid_token, status=%d body=%s", first.Code, first.Body.String())
	}
	if second.Code != http.StatusUnauthorized || decodeTokenRouteBody(t, second)["error"] != "invalid_token" {
		t.Fatalf("second userinfo request should preserve invalid_token, status=%d body=%s", second.Code, second.Body.String())
	}
	assertOAuthRateLimited(t, third)
	if !limiter.sawKeyPrefix("sso:userinfo:bearer:") {
		t.Fatalf("expected userinfo limiter key to use bearer digest, keys=%v", limiter.keys)
	}
	for _, key := range limiter.keys {
		if strings.Contains(key, "invalid-token-live") {
			t.Fatalf("limiter key leaked raw bearer token: %s", key)
		}
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUserInfoRouteLimiterFailureDefaultsFailOpen(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		UserInfoRateLimit:       1,
		UserInfoRateLimitWindow: time.Minute,
	})
	handler.BindLimiter(failingRouteLimiter{})
	engine.GET("/sso/oauth2/userinfo", handler.UserInfo)

	recorder := performUserInfoRequest(engine, "Bearer invalid-token-live")
	if recorder.Code != http.StatusUnauthorized || decodeTokenRouteBody(t, recorder)["error"] != "invalid_token" {
		t.Fatalf("limiter backend failure should fail open to OAuth invalid_token, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUserInfoRouteLimiterFailureCanFailClosed(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	limiter := &strictFailingRouteLimiter{}
	handler := NewHandler(fixture.service, ConfigView{
		UserInfoRateLimit:       1,
		UserInfoRateLimitWindow: time.Minute,
		RateLimitFailClosed:     true,
	})
	handler.BindLimiter(limiter)
	engine.GET("/sso/oauth2/userinfo", handler.UserInfo)

	recorder := performUserInfoRequest(engine, "Bearer invalid-token-live")
	assertOAuthRateLimited(t, recorder)
	if !limiter.strictCalled {
		t.Fatalf("expected SSO fail-closed config to use strict limiter override")
	}
	if limiter.strictFailOpen {
		t.Fatalf("expected strict limiter override to force fail-open=false")
	}
	if limiter.allowCalled {
		t.Fatalf("expected strict limiter override to avoid ordinary Allow and prevent double counting")
	}
}

func TestAnonymousOAuthRoutesFailClosedWithProtocolErrors(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{})
	engine.POST("/sso/oauth2/token", handler.Token)
	engine.GET("/sso/oauth2/userinfo", handler.UserInfo)
	engine.POST("/sso/oauth2/revoke", handler.Revoke)
	engine.POST("/sso/oauth2/introspect", handler.Introspect)

	tests := []struct {
		name       string
		method     string
		path       string
		form       url.Values
		headers    []ut.Header
		wantStatus int
		wantError  string
	}{
		{
			name:       "token_missing_client",
			method:     http.MethodPost,
			path:       "/sso/oauth2/token",
			form:       url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"refresh-route"}},
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid_client",
		},
		{
			name:       "token_malformed_basic_client_auth",
			method:     http.MethodPost,
			path:       "/sso/oauth2/token",
			form:       url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"refresh-route"}},
			headers:    []ut.Header{{Key: "Authorization", Value: "Basic not-base64"}},
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid_client",
		},
		{
			name:       "userinfo_missing_bearer",
			method:     http.MethodGet,
			path:       "/sso/oauth2/userinfo",
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid_token",
		},
		{
			name:       "revoke_missing_client",
			method:     http.MethodPost,
			path:       "/sso/oauth2/revoke",
			form:       url.Values{"token": {"opaque-route-token"}},
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid_client",
		},
		{
			name:       "revoke_malformed_basic_client_auth",
			method:     http.MethodPost,
			path:       "/sso/oauth2/revoke",
			form:       url.Values{"token": {"opaque-route-token"}},
			headers:    []ut.Header{{Key: "Authorization", Value: "Basic not-base64"}},
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid_client",
		},
		{
			name:       "introspect_missing_client",
			method:     http.MethodPost,
			path:       "/sso/oauth2/introspect",
			form:       url.Values{"token": {"opaque-route-token"}},
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid_client",
		},
		{
			name:       "introspect_malformed_basic_client_auth",
			method:     http.MethodPost,
			path:       "/sso/oauth2/introspect",
			form:       url.Values{"token": {"opaque-route-token"}},
			headers:    []ut.Header{{Key: "Authorization", Value: "Basic not-base64"}},
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid_client",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performOAuthRouteRequest(engine, tt.method, tt.path, tt.form, tt.headers...)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status=%d, want %d body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			body := decodeTokenRouteBody(t, recorder)
			if body["error"] != tt.wantError {
				t.Fatalf("error=%v, want %s body=%v", body["error"], tt.wantError, body)
			}
			for _, field := range []string{"access_token", "refresh_token", "id_token", "token"} {
				if body[field] != nil {
					t.Fatalf("anonymous protocol error leaked %s: %v", field, body)
				}
			}
			if body["code"] != nil || body["data"] != nil {
				t.Fatalf("anonymous OAuth route returned platform envelope instead of OAuth error: %v", body)
			}
		})
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("anonymous protocol guard should not reach repository calls: %v", err)
	}
}

func TestIntrospectRouteRequiresClientAuthentication(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{})
	engine.POST("/sso/oauth2/introspect", handler.Introspect)

	form := url.Values{}
	form.Set("token", "opaque-or-jwt")
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/introspect",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
	)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeTokenRouteBody(t, recorder)
	if body["error"] != "invalid_client" {
		t.Fatalf("error=%v, want invalid_client", body["error"])
	}
}

func TestIntrospectRouteReturnsActiveClaimsForOwnAccessToken(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{})
	engine.POST("/sso/oauth2/introspect", handler.Introspect)

	now := time.Now().UTC()
	accessToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-route"},
		"client_id":  "client-route",
		"sid":        "session-route",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-route",
	})
	session := domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectClientLookup(domain.Client{
		ID:               1,
		ClientID:         "client-route",
		ClientName:       "route client",
		ClientType:       "public",
		ClientAuthMethod: "none",
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	fixture.expectActiveSessionLookup(session.SessionID, session)
	fixture.expectTouchSession(session.SessionID)
	fixture.expectSSOAuditLog("TOKEN_INTROSPECTED", "client-route", "SUCCESS", "active", `"active":true`, `"tokenType":"access_token"`)

	form := url.Values{}
	form.Set("client_id", "client-route")
	form.Set("token", accessToken)
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/introspect",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeTokenRouteBody(t, recorder)
	if body["active"] != true {
		t.Fatalf("active=%v, want true body=%v", body["active"], body)
	}
	if body["client_id"] != "client-route" || body["sub"] != "1001" || body["sid"] != "session-route" {
		t.Fatalf("missing introspection claims: %v", body)
	}
	if body["token"] != nil || body["access_token"] != nil {
		t.Fatalf("introspection leaked token material: %v", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestIntrospectRouteReturnsInactiveForOtherClientToken(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{})
	engine.POST("/sso/oauth2/introspect", handler.Introspect)

	now := time.Now().UTC()
	accessToken := fixture.signToken(t, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-other"},
		"client_id":  "client-other",
		"sid":        "session-other-client",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-other-client",
	})
	fixture.expectClientLookup(domain.Client{
		ID:               1,
		ClientID:         "client-route",
		ClientName:       "route client",
		ClientType:       "public",
		ClientAuthMethod: "none",
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	fixture.expectSSOAuditLog("TOKEN_INTROSPECTED", "client-route", "SUCCESS", "inactive", `"active":false`)

	form := url.Values{}
	form.Set("client_id", "client-route")
	form.Set("token", accessToken)
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/introspect",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeTokenRouteBody(t, recorder)
	if body["active"] != false {
		t.Fatalf("active=%v, want false body=%v", body["active"], body)
	}
	if len(body) != 1 {
		t.Fatalf("inactive response leaked claims: %v", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestLogoutAllRouteReturnsRevokedCount(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:    1001,
			Username:  "alice",
			SessionID: "session-route",
		})
	})
	handler := NewHandler(fixture.service, ConfigView{
		SessionCookieName: "seven_session",
		RefreshCookieName: "seven_refresh_token",
	})
	engine.POST("/sso/logout-all", handler.LogoutAll)
	now := time.Now().UTC()
	fixture.expectListActiveSessionsByUserIDForRevocation(1001, domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}, domain.Session{
		ID:        21,
		SessionID: "session-other",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	fixture.expectRevokeRefreshFamiliesByUserID(1001)
	fixture.expectRevokeSessionsByUserID(1001, 2)

	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/logout-all", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal logout-all response: %v", err)
	}
	if body.Data["revoked"] != true || body.Data["revokedCount"] != float64(2) {
		t.Fatalf("logout-all response data = %#v, want revoked true and count 2", body.Data)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestLogoutRouteReturnsRevokedCount(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{
		SessionCookieName: "seven_session",
		RefreshCookieName: "seven_refresh_token",
	})
	engine.POST("/sso/logout", handler.Logout)
	now := time.Now().UTC()
	fixture.expectActiveSessionLookup("session-route", domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	fixture.expectRevokeRefreshFamiliesBySessionID("session-route")
	fixture.expectRevokeSession("session-route", 1)

	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/logout", nil,
		ut.Header{Key: "Cookie", Value: "seven_session=session-route"},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal logout response: %v", err)
	}
	if body["revoked"] != true || body["revokedCount"] != float64(1) {
		t.Fatalf("logout response = %#v, want revoked true and count 1", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDeleteSessionRouteReturnsRevokedCount(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:    1001,
			Username:  "alice",
			SessionID: "session-current",
		})
	})
	handler := NewHandler(fixture.service, ConfigView{SessionCookieName: "seven_session"})
	engine.DELETE("/sso/sessions/:sessionId", handler.DeleteSession)
	now := time.Now().UTC()
	session := domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectActiveSessionLookup("session-route", session)
	fixture.expectActiveSessionLookup("session-route", session)
	fixture.expectRevokeRefreshFamiliesBySessionID("session-route")
	fixture.expectRevokeSession("session-route", 1)

	recorder := ut.PerformRequest(engine.Engine, http.MethodDelete, "/sso/sessions/session-route", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal delete-session response: %v", err)
	}
	if body.Data["revoked"] != true || body.Data["revokedCount"] != float64(1) {
		t.Fatalf("delete-session response data = %#v, want revoked true and count 1", body.Data)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDeleteDeviceRouteReturnsRevokedCount(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:    1001,
			Username:  "alice",
			SessionID: "session-current",
		})
	})
	handler := NewHandler(fixture.service, ConfigView{SessionCookieName: "seven_session"})
	engine.DELETE("/sso/devices/:deviceId", handler.DeleteDevice)
	now := time.Now().UTC()
	session := domain.Session{
		ID:        20,
		SessionID: "session-device-a",
		ClientID:  "client-route",
		UserID:    1001,
		DeviceID:  "device-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectListSessionsByUserID(1001, session, domain.Session{
		ID:        21,
		SessionID: "session-device-b",
		ClientID:  "client-route",
		UserID:    1001,
		DeviceID:  "device-b",
		LoginAt:   now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	fixture.expectActiveSessionLookup("session-device-a", session)
	fixture.expectRevokeRefreshFamiliesBySessionID("session-device-a")
	fixture.expectRevokeSession("session-device-a", 1)

	recorder := ut.PerformRequest(engine.Engine, http.MethodDelete, "/sso/devices/device-a", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal delete-device response: %v", err)
	}
	if body.Data["revoked"] != true || body.Data["revokedCount"] != float64(1) {
		t.Fatalf("delete-device response data = %#v, want revoked true and count 1", body.Data)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestAdminRevocationRoutesRequireStepUpBeforeMutation(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		register    func(*Handler, *server.Hertz)
		wantBinding string
	}{
		{
			name:   "kick_session",
			method: http.MethodPost,
			path:   "/sso/admin/users/1001/sessions/session-route/kick",
			register: func(handler *Handler, engine *server.Hertz) {
				engine.POST("/sso/admin/users/:userId/sessions/:sessionId/kick", handler.AdminKickUserSession)
			},
			wantBinding: "sso:user:1001|session:session-route|force-logout",
		},
		{
			name:   "logout_all",
			method: http.MethodPost,
			path:   "/sso/admin/users/1001/logout-all",
			register: func(handler *Handler, engine *server.Hertz) {
				engine.POST("/sso/admin/users/:userId/logout-all", handler.AdminLogoutAllUserSessions)
			},
			wantBinding: "sso:user:1001|logout-all",
		},
		{
			name:   "kick_device",
			method: http.MethodPost,
			path:   "/sso/admin/users/1001/devices/device-a/kick",
			register: func(handler *Handler, engine *server.Hertz) {
				engine.POST("/sso/admin/users/:userId/devices/:deviceId/kick", handler.AdminKickUserDevice)
			},
			wantBinding: "sso:user:1001|device:device-a|force-logout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSSOHandlerTestFixture(t)
			engine := newSSOAdminRouteTestEngine()
			auth := &fakeSSOAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{
				ChallengeIdentifier:        "challenge-" + tt.name,
				ChallengeState:             "PENDING",
				EffectiveTimeToLiveSeconds: 300,
				RequiredAssuranceLevel:     "AAL2",
				ResolvedAssuranceLevel:     "AAL2",
				ActualChallengeTypeNames:   []string{"TIME_BASED_ONE_TIME_PASSWORD"},
			}}
			handler := NewHandler(fixture.service, ConfigView{SessionCookieName: "seven_session"})
			handler.BindAuthorization(auth)
			tt.register(handler, engine)

			recorder := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil)
			assertSSOBusinessCode(t, recorder, apperrors.CodeChallengeRequired)
			if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionAdminForceLogout) {
				t.Fatalf("challenge businessAction=%q", auth.lastChallenge.BusinessAction)
			}
			if auth.lastChallenge.OperationBinding != tt.wantBinding {
				t.Fatalf("challenge operationBinding=%q, want %q", auth.lastChallenge.OperationBinding, tt.wantBinding)
			}
			if err := fixture.mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("sql expectations: %v", err)
			}
		})
	}
}

func TestAdminRevocationRoutesRejectInvalidProofBeforeMutation(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		register    func(*Handler, *server.Hertz)
		wantBinding string
	}{
		{
			name:   "kick_session",
			method: http.MethodPost,
			path:   "/sso/admin/users/1001/sessions/session-route/kick",
			register: func(handler *Handler, engine *server.Hertz) {
				engine.POST("/sso/admin/users/:userId/sessions/:sessionId/kick", handler.AdminKickUserSession)
			},
			wantBinding: "sso:user:1001|session:session-route|force-logout",
		},
		{
			name:   "logout_all",
			method: http.MethodPost,
			path:   "/sso/admin/users/1001/logout-all",
			register: func(handler *Handler, engine *server.Hertz) {
				engine.POST("/sso/admin/users/:userId/logout-all", handler.AdminLogoutAllUserSessions)
			},
			wantBinding: "sso:user:1001|logout-all",
		},
		{
			name:   "kick_device",
			method: http.MethodPost,
			path:   "/sso/admin/users/1001/devices/device-a/kick",
			register: func(handler *Handler, engine *server.Hertz) {
				engine.POST("/sso/admin/users/:userId/devices/:deviceId/kick", handler.AdminKickUserDevice)
			},
			wantBinding: "sso:user:1001|device:device-a|force-logout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSSOHandlerTestFixture(t)
			engine := newSSOAdminRouteTestEngine()
			auth := &fakeSSOAuthFacade{verifyErr: apperrors.Forbidden("step-up proof无效或已过期")}
			handler := NewHandler(fixture.service, ConfigView{SessionCookieName: "seven_session"})
			handler.BindAuthorization(auth)
			tt.register(handler, engine)

			recorder := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil, ssoProofHeaders("bad-proof", "flow-bad")...)
			assertSSOBusinessCode(t, recorder, apperrors.CodeForbidden)
			assertSSOStepUpVerifyRequest(t, auth.lastValidate, "flow-bad", tt.wantBinding)
			if err := fixture.mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("sql expectations: %v", err)
			}
		})
	}
}

func TestAdminKickUserSessionRouteReturnsRevokedCount(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{}
	handler := NewHandler(fixture.service, ConfigView{SessionCookieName: "seven_session"})
	handler.BindAuthorization(auth)
	engine.POST("/sso/admin/users/:userId/sessions/:sessionId/kick", handler.AdminKickUserSession)
	now := time.Now().UTC()
	session := domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectActiveSessionLookup("session-route", session)
	fixture.expectActiveSessionLookup("session-route", session)
	fixture.expectRevokeRefreshFamiliesBySessionID("session-route")
	fixture.expectRevokeSession("session-route", 1)

	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/admin/users/1001/sessions/session-route/kick", nil, ssoProofHeaders("proof-token", "flow-kick-session")...)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertSSOStepUpVerifyRequest(t, auth.lastValidate, "flow-kick-session", "sso:user:1001|session:session-route|force-logout")
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal admin kick-session response: %v", err)
	}
	if body.Data["revoked"] != true || body.Data["revokedCount"] != float64(1) {
		t.Fatalf("admin kick-session response data = %#v, want revoked true and count 1", body.Data)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestAdminLogoutAllUserSessionsRouteReturnsRevokedCount(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{}
	handler := NewHandler(fixture.service, ConfigView{SessionCookieName: "seven_session"})
	handler.BindAuthorization(auth)
	engine.POST("/sso/admin/users/:userId/logout-all", handler.AdminLogoutAllUserSessions)
	now := time.Now().UTC()
	fixture.expectListActiveSessionsByUserIDForRevocation(1001, domain.Session{
		ID:        20,
		SessionID: "session-route",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}, domain.Session{
		ID:        21,
		SessionID: "session-other",
		ClientID:  "client-route",
		UserID:    1001,
		LoginAt:   now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	fixture.expectRevokeRefreshFamiliesByUserID(1001)
	fixture.expectRevokeSessionsByUserID(1001, 2)

	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/admin/users/1001/logout-all", nil, ssoProofHeaders("proof-token", "flow-logout-all")...)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertSSOStepUpVerifyRequest(t, auth.lastValidate, "flow-logout-all", "sso:user:1001|logout-all")
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal admin logout-all response: %v", err)
	}
	if body.Data["revoked"] != true || body.Data["revokedCount"] != float64(2) {
		t.Fatalf("admin logout-all response data = %#v, want revoked true and count 2", body.Data)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestAdminKickUserDeviceRouteReturnsRevokedCount(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{}
	handler := NewHandler(fixture.service, ConfigView{SessionCookieName: "seven_session"})
	handler.BindAuthorization(auth)
	engine.POST("/sso/admin/users/:userId/devices/:deviceId/kick", handler.AdminKickUserDevice)
	now := time.Now().UTC()
	session := domain.Session{
		ID:        20,
		SessionID: "session-device-a",
		ClientID:  "client-route",
		UserID:    1001,
		DeviceID:  "device-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	fixture.expectListSessionsByUserID(1001, session, domain.Session{
		ID:        21,
		SessionID: "session-device-b",
		ClientID:  "client-route",
		UserID:    1001,
		DeviceID:  "device-b",
		LoginAt:   now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	fixture.expectActiveSessionLookup("session-device-a", session)
	fixture.expectRevokeRefreshFamiliesBySessionID("session-device-a")
	fixture.expectRevokeSession("session-device-a", 1)

	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/admin/users/1001/devices/device-a/kick", nil, ssoProofHeaders("proof-token", "flow-kick-device")...)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertSSOStepUpVerifyRequest(t, auth.lastValidate, "flow-kick-device", "sso:user:1001|device:device-a|force-logout")
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal admin kick-device response: %v", err)
	}
	if body.Data["revoked"] != true || body.Data["revokedCount"] != float64(1) {
		t.Fatalf("admin kick-device response data = %#v, want revoked true and count 1", body.Data)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSSOProtectedMutationUsesRequestedBusinessAction(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.POST("/sso/admin/test-client/:clientId/status", func(ctx context.Context, reqCtx *app.RequestContext) {
		_, err := handler.ensureProtectedMutation(
			ctx,
			reqCtx,
			challengedomain.BusinessActionSSOClientStatusChange,
			ssoClientOperationBinding(string(reqCtx.Param("clientId")), "status"),
		)
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
		response.Success(reqCtx, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/admin/test-client/demo-client/status", nil, ssoProofHeaders("proof-token", "flow-sso-client-status")...)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertSSOStepUpVerifyRequestForAction(t, auth.lastValidate, challengedomain.BusinessActionSSOClientStatusChange, "flow-sso-client-status", "sso:client:demo-client|status")
}

func TestSSOProtectedMutationRejectsWrongBusinessActionProof(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{verifyToken: &authorizationfacade.StepUpTokenVO{
		BusinessAction:            string(challengedomain.BusinessActionAdminForceLogout),
		OperationBinding:          "sso:client:demo-client|status",
		AuthenticationMethodNames: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.POST("/sso/admin/test-client/:clientId/status", func(ctx context.Context, reqCtx *app.RequestContext) {
		_, err := handler.ensureProtectedMutation(
			ctx,
			reqCtx,
			challengedomain.BusinessActionSSOClientStatusChange,
			ssoClientOperationBinding(string(reqCtx.Param("clientId")), "status"),
		)
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
		response.Success(reqCtx, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/admin/test-client/demo-client/status", nil, ssoProofHeaders("proof-token", "flow-sso-client-status")...)
	assertSSOBusinessCode(t, recorder, apperrors.CodeForbidden)
}

func TestSSOProtectedMutationBindsCanonicalClientPayload(t *testing.T) {
	binding := ssoClientOperationBinding("  authorization-console  ", " secret-generate ")
	if binding != "sso:client:authorization-console|secret-generate" {
		t.Fatalf("canonical client binding=%q", binding)
	}
}

func TestListClientSecretsDoesNotExposeSecretHash(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	now := time.Date(2026, 6, 18, 15, 0, 0, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)
	fixture.expectClientDetailLookup(domain.Client{
		ID:                 99,
		ClientID:           "demo-client",
		ClientName:         "Demo Client",
		ClientType:         "CONFIDENTIAL",
		ClientAuthMethod:   "client_secret_basic",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid", "email"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             domain.ClientStatusActive,
		CreateTime:         now,
		UpdateTime:         now,
	})
	fixture.mock.ExpectQuery("SELECT id, clientId, secretHint, expiresAt, status, createTime, updateTime").
		WithArgs("demo-client").
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "secretHint", "expiresAt", "status", "createTime", "updateTime"}).
			AddRow(int64(100), "demo-client", "sec_****abcd", expiresAt, domain.ClientStatusActive, now, now))

	engine := server.Default()
	handler := NewHandler(fixture.service, ConfigView{})
	engine.GET("/sso/admin/clients/:clientId/secrets", handler.ListClientSecrets)

	recorder := ut.PerformRequest(engine.Engine, http.MethodGet, "/sso/admin/clients/%20demo-client%20/secrets", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"secretHash", "argon2id", "plain-secret", "sec_live_"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("secret list leaked forbidden marker %q in %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "sec_****abcd") {
		t.Fatalf("secret list did not include safe hint: %s", body)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCreateClientReturnsChallengeRequiredWithoutProof(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{
		ChallengeIdentifier:        "challenge-create",
		ChallengeState:             "PENDING",
		EffectiveTimeToLiveSeconds: 300,
		RequiredAssuranceLevel:     "AAL2",
		ResolvedAssuranceLevel:     "AAL2",
		ActualChallengeTypeNames:   []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.POST("/sso/admin/clients", handler.CreateClient)

	body := `{"clientId":"demo-client","clientName":"Demo Client","clientType":"PUBLIC","clientAuthMethod":"none","grantTypes":["authorization_code","refresh_token"],"scopes":["openid","email"],"requirePkce":true,"trustedFirstParty":true,"accessTokenTtlSec":1800,"refreshTokenTtlSec":2592000}`
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/admin/clients", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertSSOBusinessCode(t, recorder, apperrors.CodeChallengeRequired)
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionSSOClientCreate) {
		t.Fatalf("challenge businessAction=%q", auth.lastChallenge.BusinessAction)
	}
	if !strings.HasPrefix(auth.lastChallenge.OperationBinding, "sso:client:demo-client|action:SSO_CLIENT_CREATE|payload:") {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestGenerateClientSecretUsesSecretGenerateBusinessAction(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{
		ChallengeIdentifier:        "challenge-secret-generate",
		ChallengeState:             "PENDING",
		EffectiveTimeToLiveSeconds: 300,
		RequiredAssuranceLevel:     "AAL2",
		ResolvedAssuranceLevel:     "AAL2",
		ActualChallengeTypeNames:   []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.POST("/sso/admin/clients/:clientId/secrets", handler.GenerateClientSecret)

	body := `{"expiresInDays":30,"reason":"rotate"}`
	recorder := ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/admin/clients/demo-client/secrets", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertSSOBusinessCode(t, recorder, apperrors.CodeChallengeRequired)
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionSSOClientSecretGenerate) {
		t.Fatalf("challenge businessAction=%q", auth.lastChallenge.BusinessAction)
	}
	if !strings.HasPrefix(auth.lastChallenge.OperationBinding, "sso:client:demo-client|action:SSO_CLIENT_SECRET_GENERATE|payload:") {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestDisableClientSecretUsesSecretDisableBusinessAction(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{
		ChallengeIdentifier:        "challenge-secret-disable",
		ChallengeState:             "PENDING",
		EffectiveTimeToLiveSeconds: 300,
		RequiredAssuranceLevel:     "AAL2",
		ResolvedAssuranceLevel:     "AAL2",
		ActualChallengeTypeNames:   []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.PUT("/sso/admin/clients/:clientId/secrets/:secretId/status", handler.DisableClientSecret)

	body := `{"reason":"retire"}`
	recorder := ut.PerformRequest(engine.Engine, http.MethodPut, "/sso/admin/clients/demo-client/secrets/99/status", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertSSOBusinessCode(t, recorder, apperrors.CodeChallengeRequired)
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionSSOClientSecretDisable) {
		t.Fatalf("challenge businessAction=%q", auth.lastChallenge.BusinessAction)
	}
	if !strings.HasPrefix(auth.lastChallenge.OperationBinding, "sso:client:demo-client|action:SSO_CLIENT_SECRET_DISABLE|payload:") {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestUpdateClientStatusUsesStatusChangeBusinessAction(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.PUT("/sso/admin/clients/:clientId/status", handler.UpdateClientStatus)

	body := `{"status":1}`
	recorder := ut.PerformRequest(engine.Engine, http.MethodPut, "/sso/admin/clients/demo-client/status", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertSSOBusinessCode(t, recorder, apperrors.CodeChallengeRequired)
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionSSOClientStatusChange) {
		t.Fatalf("challenge businessAction=%q", auth.lastChallenge.BusinessAction)
	}
	if !strings.HasPrefix(auth.lastChallenge.OperationBinding, "sso:client:demo-client|action:SSO_CLIENT_STATUS_CHANGE|payload:") {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestClientMutationRejectsWrongOperationBinding(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{verifyToken: &authorizationfacade.StepUpTokenVO{
		BusinessAction:            string(challengedomain.BusinessActionSSOClientStatusChange),
		OperationBinding:          "sso:client:demo-client|action:SSO_CLIENT_STATUS_CHANGE|payload:old",
		AuthenticationMethodNames: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.PUT("/sso/admin/clients/:clientId/status", handler.UpdateClientStatus)

	body := `{"status":1}`
	recorder := ut.PerformRequest(engine.Engine, http.MethodPut, "/sso/admin/clients/demo-client/status", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, append(ssoProofHeaders("proof-token", "flow-status"), ut.Header{Key: "Content-Type", Value: "application/json"})...)
	assertSSOBusinessCode(t, recorder, apperrors.CodeForbidden)
}

func TestUpdateClientRedirectURIsUsesRedirectEditBusinessAction(t *testing.T) {
	engine := newSSOAdminRouteTestEngine()
	auth := &fakeSSOAuthFacade{}
	handler := NewHandler(nil, ConfigView{})
	handler.BindAuthorization(auth)
	engine.PUT("/sso/admin/clients/:clientId/redirect-uris", handler.UpdateClientRedirectURIs)

	body := `{"redirectUris":["https://demo.example/callback"]}`
	recorder := ut.PerformRequest(engine.Engine, http.MethodPut, "/sso/admin/clients/demo-client/redirect-uris", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	assertSSOBusinessCode(t, recorder, apperrors.CodeChallengeRequired)
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionSSOClientRedirectEdit) {
		t.Fatalf("challenge businessAction=%q", auth.lastChallenge.BusinessAction)
	}
	if !strings.HasPrefix(auth.lastChallenge.OperationBinding, "sso:client:demo-client|action:SSO_CLIENT_REDIRECT_EDIT|payload:") {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestDisabledClientAuthorizeDoesNotRedirectToClient(t *testing.T) {
	fixture := newSSOHandlerTestFixture(t)
	disabled := domain.Client{
		ID:           10,
		ClientID:     "disabled-client",
		ClientName:   "Disabled",
		RedirectURIs: []string{"https://client.example/callback"},
		Status:       domain.ClientStatusDisabled,
	}
	fixture.expectClientLookup(disabled)
	fixture.expectClientLookup(disabled)

	handler := NewHandler(fixture.service, ConfigView{})
	engine := server.Default()
	engine.GET("/sso/oauth2/authorize", handler.Authorize)

	recorder := ut.PerformRequest(engine.Engine, http.MethodGet, "/sso/oauth2/authorize?response_type=code&client_id=disabled-client&redirect_uri=https%3A%2F%2Fclient.example%2Fcallback&scope=openid&state=state-a&code_challenge=abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ&code_challenge_method=S256", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if location := string(recorder.Header().Peek("Location")); strings.Contains(location, "client.example") {
		t.Fatalf("disabled client should not receive redirect, Location=%q", location)
	}
	body := decodeTokenRouteBody(t, recorder)
	if body["error"] == nil {
		t.Fatalf("expected oauth error body, got %s", recorder.Body.String())
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func newSSOAdminRouteTestEngine() *server.Hertz {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:    9001,
			Username:  "operator",
			SessionID: "operator-session",
			Source:    "sso",
			IsAdmin:   true,
		})
		reqCtx.Next(ctx)
	})
	return engine
}

func ssoProofHeaders(proofToken, flowNonce string) []ut.Header {
	return []ut.Header{
		{Key: "Proof-Token", Value: proofToken},
		{Key: "Flow-Nonce", Value: flowNonce},
	}
}

func assertSSOBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected http status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, recorder.Body.String())
	}
	if body.Code != expected {
		t.Fatalf("business code=%d, want %d body=%s", body.Code, expected, recorder.Body.String())
	}
}

func assertSSOStepUpVerifyRequest(t *testing.T, request authorizationfacade.StepUpVerifyRequest, flowNonce, operationBinding string) {
	t.Helper()
	assertSSOStepUpVerifyRequestForAction(t, request, challengedomain.BusinessActionAdminForceLogout, flowNonce, operationBinding)
}

func assertSSOStepUpVerifyRequestForAction(t *testing.T, request authorizationfacade.StepUpVerifyRequest, action challengedomain.BusinessAction, flowNonce, operationBinding string) {
	t.Helper()
	if request.BusinessAction != string(action) {
		t.Fatalf("verify businessAction=%q", request.BusinessAction)
	}
	if request.FlowNonce != flowNonce {
		t.Fatalf("verify flowNonce=%q, want %q", request.FlowNonce, flowNonce)
	}
	if request.OperationBinding != operationBinding {
		t.Fatalf("verify operationBinding=%q, want %q", request.OperationBinding, operationBinding)
	}
	if !request.ConsumeOnce {
		t.Fatalf("verify ConsumeOnce=false, want true")
	}
}

type fakeSSOAuthFacade struct {
	authorizationfacade.AuthFacade
	challenge     *authorizationfacade.StepUpChallengeVO
	verifyToken   *authorizationfacade.StepUpTokenVO
	verifyErr     error
	lastChallenge authorizationfacade.StepUpChallengeRequest
	lastValidate  authorizationfacade.StepUpVerifyRequest
}

func (f *fakeSSOAuthFacade) CreateStepUpChallenge(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	f.lastChallenge = request
	if f.challenge != nil {
		return f.challenge, nil
	}
	return &authorizationfacade.StepUpChallengeVO{
		ChallengeIdentifier:        "challenge-default",
		ChallengeState:             "PENDING",
		EffectiveTimeToLiveSeconds: 300,
		RequiredAssuranceLevel:     "AAL2",
		ResolvedAssuranceLevel:     "AAL2",
		ActualChallengeTypeNames:   []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}, nil
}

func (f *fakeSSOAuthFacade) VerifyStepUp(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
	f.lastValidate = request
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	if f.verifyToken != nil {
		token := *f.verifyToken
		if token.ProofToken == "" {
			token.ProofToken = request.ProofToken
		}
		if token.BusinessAction == "" {
			token.BusinessAction = request.BusinessAction
		}
		if token.FlowNonce == "" {
			token.FlowNonce = request.FlowNonce
		}
		if token.OperationBinding == "" {
			token.OperationBinding = request.OperationBinding
		}
		if token.ChallengeID == "" {
			token.ChallengeID = "challenge-verify"
		}
		if token.TokenUniqueIdentifier == "" {
			token.TokenUniqueIdentifier = "proof-jti-verify"
		}
		if len(token.AuthenticationMethodNames) == 0 {
			token.AuthenticationMethodNames = []string{"TIME_BASED_ONE_TIME_PASSWORD"}
		}
		return &token, nil
	}
	return &authorizationfacade.StepUpTokenVO{
		ProofToken:                request.ProofToken,
		ChallengeID:               "challenge-verify",
		TokenUniqueIdentifier:     "proof-jti-verify",
		BusinessAction:            request.BusinessAction,
		FlowNonce:                 request.FlowNonce,
		OperationBinding:          request.OperationBinding,
		AuthenticationMethodNames: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}, nil
}

type ssoHandlerTestFixture struct {
	service *ssoapp.Service
	mock    sqlmock.Sqlmock
	jwt     *jwtinfra.Service
}

type ssoHandlerSQLMockProvider struct {
	db *sqlx.DB
}

type hertzEngineRoundTripper struct {
	engine         *server.Hertz
	tokenUsedBasic bool
	tokenForm      url.Values
}

func (r *hertzEngineRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	var body *ut.Body
	var payload []byte
	if request.Body != nil {
		var err error
		payload, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		body = &ut.Body{Body: bytes.NewReader(payload), Len: len(payload)}
	}
	headers := make([]ut.Header, 0, len(request.Header))
	for key, values := range request.Header {
		for _, value := range values {
			headers = append(headers, ut.Header{Key: key, Value: value})
		}
	}
	if request.URL.Path == "/sso/oauth2/token" {
		r.tokenUsedBasic = strings.HasPrefix(request.Header.Get("Authorization"), "Basic ")
		r.tokenForm, _ = url.ParseQuery(string(payload))
	}
	recorder := ut.PerformRequest(r.engine.Engine, request.Method, request.URL.RequestURI(), body, headers...)
	responseHeader := make(http.Header)
	recorder.Header().VisitAll(func(key, value []byte) {
		responseHeader.Add(string(key), string(value))
	})
	return &http.Response{
		StatusCode: recorder.Code, Header: responseHeader, Body: io.NopCloser(bytes.NewReader(recorder.Body.Bytes())), Request: request,
	}, nil
}

func newSSOHandlerProtocolFixture(t *testing.T, issuer, clientSecret string) (*ssoHandlerTestFixture, string) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	repo, err := ssoinfra.NewRepository(ssoHandlerSQLMockProvider{db: sqlx.NewDb(rawDB, "sqlmock")})
	if err != nil {
		t.Fatalf("new sso repository: %v", err)
	}
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	secretHash, err := passwordService.Hash(context.Background(), clientSecret)
	if err != nil {
		t.Fatalf("hash managed secret: %v", err)
	}
	jwtService := newSSOHandlerTestJWTService(t)
	service := ssoapp.NewService(config.SSOConfig{Issuer: issuer, BaseURL: strings.TrimSuffix(issuer, "/")}, nil, repo, ssoinfra.NewAuthSessionCache(nil), jwtService, passwordService, nil, nil)
	return &ssoHandlerTestFixture{service: service, mock: mock, jwt: jwtService}, secretHash
}

func (p ssoHandlerSQLMockProvider) Driver() string               { return "sqlmock" }
func (p ssoHandlerSQLMockProvider) Dialect() string              { return "mysql" }
func (p ssoHandlerSQLMockProvider) DB() *sql.DB                  { return p.db.DB }
func (p ssoHandlerSQLMockProvider) SQLX() *sqlx.DB               { return p.db }
func (p ssoHandlerSQLMockProvider) Close() error                 { return nil }
func (p ssoHandlerSQLMockProvider) Transactor() store.Transactor { return nil }
func (p ssoHandlerSQLMockProvider) Configured() bool             { return true }

func newSSOHandlerTestFixture(t *testing.T) *ssoHandlerTestFixture {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	repo, err := ssoinfra.NewRepository(ssoHandlerSQLMockProvider{db: sqlx.NewDb(rawDB, "sqlmock")})
	if err != nil {
		t.Fatalf("new sso repository: %v", err)
	}
	jwtService := newSSOHandlerTestJWTService(t)
	service := ssoapp.NewService(
		config.SSOConfig{
			Issuer:                     "https://auth.example.com/sso",
			SessionTouchThrottleSecond: 30,
			RefreshReplayClockSkewSec:  30,
			RefreshCookie: config.SSORefreshCookieConfig{
				Name:     "seven_refresh_token",
				Path:     "/",
				SameSite: "Lax",
			},
		},
		nil,
		repo,
		ssoinfra.NewAuthSessionCache(nil),
		jwtService,
		nil,
		nil,
		nil,
	)
	return &ssoHandlerTestFixture{service: service, mock: mock, jwt: jwtService}
}

func newSSOHandlerTestJWTService(t *testing.T) *jwtinfra.Service {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "jwt-private.pem")
	publicPath := filepath.Join(dir, "jwt-public.pem")
	writeSSOHandlerTestPEM(t, privatePath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey))
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	writeSSOHandlerTestPEM(t, publicPath, "PUBLIC KEY", publicDER)
	keys, err := keyring.NewLocalProvider(config.KeysConfig{
		Provider: "local",
		JWT: config.JWTKeysConfig{
			Algorithm: "RS256",
			Active: config.JWTKeySourceConfig{
				KID:              "kid-active",
				PrivateKeySource: "file:" + privatePath,
				PublicKeySource:  "file:" + publicPath,
			},
		},
	})
	if err != nil {
		t.Fatalf("new key provider: %v", err)
	}
	service, err := jwtinfra.New(keys, "RS256")
	if err != nil {
		t.Fatalf("new jwt service: %v", err)
	}
	return service
}

func writeSSOHandlerTestPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	block := &pem.Block{Type: blockType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}
}

func (f *ssoHandlerTestFixture) signToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	token, err := f.jwt.Sign(context.Background(), claims)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func (f *ssoHandlerTestFixture) expectClientLookup(client domain.Client) {
	f.mock.ExpectQuery("SELECT id, clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson").
		WithArgs(client.ClientID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
			"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
			"status", "metadataJson",
		}).AddRow(
			client.ID,
			client.ClientID,
			client.ClientName,
			client.ClientType,
			client.ClientAuthMethod,
			mustSSOHandlerTestJSON(client.GrantTypes),
			mustSSOHandlerTestJSON(client.Scopes),
			0,
			0,
			boolToInt(client.TrustedFirstParty),
			300,
			3600,
			client.Status,
			nil,
		))
	f.mock.ExpectQuery("SELECT redirectUri, postLogoutRedirectUri").
		WithArgs(client.ClientID).
		WillReturnRows(sqlmock.NewRows([]string{"redirectUri", "postLogoutRedirectUri"}))
	f.mock.ExpectQuery("SELECT secretHash").
		WithArgs(client.ClientID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"secretHash"}))
}

func (f *ssoHandlerTestFixture) expectConfidentialClientLookup(client domain.Client, secretHash string) {
	f.mock.ExpectQuery("SELECT id, clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson").
		WithArgs(client.ClientID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
			"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec", "status", "metadataJson",
		}).AddRow(client.ID, client.ClientID, client.ClientName, client.ClientType, client.ClientAuthMethod, mustSSOHandlerTestJSON(client.GrantTypes), mustSSOHandlerTestJSON(client.Scopes), 1, 0, 0, 300, 3600, client.Status, nil))
	f.mock.ExpectQuery("SELECT redirectUri, postLogoutRedirectUri").WithArgs(client.ClientID).
		WillReturnRows(sqlmock.NewRows([]string{"redirectUri", "postLogoutRedirectUri"}))
	f.mock.ExpectQuery("SELECT secretHash").WithArgs(client.ClientID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"secretHash"}).AddRow(secretHash))
}

func (f *ssoHandlerTestFixture) expectClientDetailLookup(client domain.Client) {
	f.mock.ExpectQuery("SELECT c\\.id, c\\.clientId, c\\.clientName, c\\.clientType").
		WithArgs(client.ClientID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
			"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
			"status", "metadataJson", "activeRedirectCount", "activeSecretCount", "createTime", "updateTime",
		}).AddRow(
			client.ID,
			client.ClientID,
			client.ClientName,
			client.ClientType,
			client.ClientAuthMethod,
			mustSSOHandlerTestJSON(client.GrantTypes),
			mustSSOHandlerTestJSON(client.Scopes),
			boolToInt(client.RequirePKCE),
			boolToInt(client.RequireConsent),
			boolToInt(client.TrustedFirstParty),
			client.AccessTokenTTLSec,
			client.RefreshTokenTTLSec,
			client.Status,
			nullableSQLString(client.MetadataJSON),
			client.ActiveRedirectCount,
			client.ActiveSecretCount,
			client.CreateTime,
			client.UpdateTime,
		))
}

func (f *ssoHandlerTestFixture) expectAuthorizationCodeLookup(code domain.AuthorizationCode) {
	f.mock.ExpectQuery("SELECT id, code, clientId, userId, sessionId, redirectUri, scopesJson, codeChallenge, codeChallengeMethod").
		WithArgs(code.Code).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "code", "clientId", "userId", "sessionId", "redirectUri", "scopesJson", "codeChallenge", "codeChallengeMethod",
			"nonce", "acr", "amrJson", "expiresAt", "consumedAt", "status", "metadataJson", "createTime", "updateTime",
		}).AddRow(
			code.ID,
			code.Code,
			code.ClientID,
			code.UserID,
			code.SessionID,
			code.RedirectURI,
			mustSSOHandlerTestJSON(code.Scopes),
			code.CodeChallenge,
			code.CodeChallengeMethod,
			nullableSQLString(code.Nonce),
			nullableSQLString(code.ACR),
			nullableSQLString(mustSSOHandlerTestJSON(code.AMR)),
			code.ExpiresAt,
			nil,
			code.Status,
			nil,
			code.CreateTime,
			code.UpdateTime,
		))
}

func (f *ssoHandlerTestFixture) expectRefreshFamilyByCurrentHash(hash string, family domain.RefreshTokenFamily) {
	f.mock.ExpectQuery("SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected").
		WithArgs(hash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "familyId", "sessionId", "clientId", "userId", "currentTokenHash", "previousTokenHash",
			"reuseDetected", "rotatedAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}).AddRow(
			family.ID,
			family.FamilyID,
			family.SessionID,
			family.ClientID,
			family.UserID,
			family.CurrentTokenHash,
			nil,
			boolToInt(family.ReuseDetected),
			family.RotatedAt,
			family.ExpiresAt,
			family.RevokedAt,
			family.Status,
			family.MetadataJSON,
			family.CreateTime,
			family.UpdateTime,
		))
}

func (f *ssoHandlerTestFixture) expectMissingRefreshFamilyByCurrentHash(hash string) {
	f.mock.ExpectQuery("SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected").
		WithArgs(hash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "familyId", "sessionId", "clientId", "userId", "currentTokenHash", "previousTokenHash",
			"reuseDetected", "rotatedAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}))
}

func (f *ssoHandlerTestFixture) expectRefreshFamilyByPreviousHash(hash string, family domain.RefreshTokenFamily) {
	previousHash := family.PreviousTokenHash
	if previousHash == "" {
		previousHash = hash
	}
	f.mock.ExpectQuery("SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected").
		WithArgs(hash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "familyId", "sessionId", "clientId", "userId", "currentTokenHash", "previousTokenHash",
			"reuseDetected", "rotatedAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}).AddRow(
			family.ID,
			family.FamilyID,
			family.SessionID,
			family.ClientID,
			family.UserID,
			family.CurrentTokenHash,
			previousHash,
			boolToInt(family.ReuseDetected),
			family.RotatedAt,
			family.ExpiresAt,
			family.RevokedAt,
			family.Status,
			family.MetadataJSON,
			family.CreateTime,
			family.UpdateTime,
		))
}

func (f *ssoHandlerTestFixture) expectConsumeAuthorizationCode(code string) {
	f.mock.ExpectExec("UPDATE sys_sso_authorization_code").
		WithArgs(sqlmock.AnyArg(), domain.CodeStatusConsumed, sqlmock.AnyArg(), code, domain.CodeStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func (f *ssoHandlerTestFixture) expectActiveSessionLookup(sessionID string, session domain.Session) {
	f.mock.ExpectQuery("SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson").
		WithArgs(sessionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson",
			"loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}).AddRow(
			session.ID,
			session.SessionID,
			session.UserID,
			session.ClientID,
			nullableSQLString(session.PlatformCode),
			nil,
			nil,
			nil,
			nil,
			nil,
			session.LoginAt,
			nil,
			session.ExpiresAt,
			nil,
			session.Status,
			nil,
			session.LoginAt,
			session.LoginAt,
		))
}

func (f *ssoHandlerTestFixture) expectListSessionsByUserID(userID int64, sessions ...domain.Session) {
	rows := sqlmock.NewRows([]string{
		"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson",
		"loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
	})
	for _, session := range sessions {
		rows.AddRow(
			session.ID,
			session.SessionID,
			session.UserID,
			session.ClientID,
			nullableSQLString(session.PlatformCode),
			nullableSQLString(session.DeviceID),
			nullableSQLString(session.LoginIP),
			nullableSQLString(session.UserAgent),
			nullableSQLString(session.ACR),
			mustSSOHandlerTestJSON(session.AMR),
			session.LoginAt,
			nullableSQLTime(session.LastAccessAt),
			session.ExpiresAt,
			nullableSQLTime(session.RevokedAt),
			session.Status,
			nullableSQLString(session.MetadataJSON),
			session.CreateTime,
			session.UpdateTime,
		)
	}
	f.mock.ExpectQuery("SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson").
		WithArgs(userID).
		WillReturnRows(rows)
}

func (f *ssoHandlerTestFixture) expectListActiveSessionsByUserIDForRevocation(userID int64, sessions ...domain.Session) {
	f.mock.ExpectQuery("SELECT CURRENT_TIMESTAMP").
		WillReturnRows(sqlmock.NewRows([]string{"cutoff"}).AddRow(time.Now().UTC()))
	rows := sqlmock.NewRows([]string{
		"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson",
		"loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
	})
	for _, session := range sessions {
		rows.AddRow(
			session.ID,
			session.SessionID,
			session.UserID,
			session.ClientID,
			nullableSQLString(session.PlatformCode),
			nullableSQLString(session.DeviceID),
			nullableSQLString(session.LoginIP),
			nullableSQLString(session.UserAgent),
			nullableSQLString(session.ACR),
			mustSSOHandlerTestJSON(session.AMR),
			session.LoginAt,
			nullableSQLTime(session.LastAccessAt),
			session.ExpiresAt,
			nullableSQLTime(session.RevokedAt),
			session.Status,
			nullableSQLString(session.MetadataJSON),
			session.CreateTime,
			session.UpdateTime,
		)
	}
	f.mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs(userID, domain.SessionStatusActive, sqlmock.AnyArg(), int64(0), 100).
		WillReturnRows(rows)
}

func (f *ssoHandlerTestFixture) expectTouchSession(sessionID string) {
	f.mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sessionID, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func (f *ssoHandlerTestFixture) expectRevokeRefreshFamiliesByUserID(userID int64) {
	f.mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), userID, sqlmock.AnyArg(), domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 3))
}

func (f *ssoHandlerTestFixture) expectRevokeRefreshFamiliesBySessionID(sessionID string) {
	f.mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), sessionID, domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func (f *ssoHandlerTestFixture) expectRevokeSession(sessionID string, affectedRows int64) {
	f.mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), sessionID, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func (f *ssoHandlerTestFixture) expectRevokeSessionsByUserID(userID int64, affectedRows int64) {
	f.mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), userID, sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func (f *ssoHandlerTestFixture) expectSSOAuditLog(eventType, clientID, result, reasonCode string, detailFragments ...string) {
	f.expectSSOAuditLogWithTrace(eventType, clientID, result, reasonCode, "", detailFragments...)
}

func (f *ssoHandlerTestFixture) expectSSOAuditLogWithTrace(eventType, clientID, result, reasonCode, traceID string, detailFragments ...string) {
	var traceArg driver.Value = sqlmock.AnyArg()
	if strings.TrimSpace(traceID) != "" {
		traceArg = traceID
	}
	f.mock.ExpectExec("INSERT INTO sys_sso_audit_log").
		WithArgs(
			eventType,
			clientID,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			result,
			reasonCode,
			ssoHandlerAuditDetailMatcher{fragments: detailFragments},
			traceArg,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

type ssoHandlerAuditDetailMatcher struct {
	fragments []string
}

func (m ssoHandlerAuditDetailMatcher) Match(value driver.Value) bool {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	default:
		return false
	}
	if strings.Contains(text, "access-token") || strings.Contains(text, "refresh-token") || strings.Contains(text, "plain-secret") || strings.Contains(text, "code-live-route") {
		return false
	}
	for _, fragment := range m.fragments {
		if !strings.Contains(text, fragment) {
			return false
		}
	}
	return true
}

func (f *ssoHandlerTestFixture) expectInsertRefreshFamily() {
	f.mock.ExpectExec("INSERT INTO sys_sso_refresh_token_family").
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			domain.RefreshFamilyStatusActive,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func (f *ssoHandlerTestFixture) expectRotateRefreshFamily(familyID string, previousHash string, affectedRows int64) {
	f.mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(previousHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), familyID, previousHash, domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func performRefreshTokenRequest(engine *server.Hertz, refreshToken string) *ut.ResponseRecorder {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", "client-route")
	form.Set("refresh_token", refreshToken)
	return performTokenFormRequest(engine, form)
}

func performTokenFormRequest(engine *server.Hertz, form url.Values) *ut.ResponseRecorder {
	return ut.PerformRequest(engine.Engine, http.MethodPost, "/sso/oauth2/token",
		&ut.Body{Body: strings.NewReader(form.Encode()), Len: len(form.Encode())},
		ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"},
	)
}

func performUserInfoRequest(engine *server.Hertz, authorization string) *ut.ResponseRecorder {
	return ut.PerformRequest(engine.Engine, http.MethodGet, "/sso/oauth2/userinfo", nil,
		ut.Header{Key: "Authorization", Value: authorization},
	)
}

func performOAuthRouteRequest(engine *server.Hertz, method string, path string, form url.Values, headers ...ut.Header) *ut.ResponseRecorder {
	var body *ut.Body
	if form != nil {
		encoded := form.Encode()
		body = &ut.Body{Body: strings.NewReader(encoded), Len: len(encoded)}
		headers = append(headers, ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"})
	}
	return ut.PerformRequest(engine.Engine, method, path, body, headers...)
}

func assertOAuthRateLimited(t *testing.T, recorder *ut.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected HTTP 429, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeTokenRouteBody(t, recorder)
	if body["error"] != "rate_limited" {
		t.Fatalf("expected OAuth rate_limited response, got %v", body)
	}
	if body["access_token"] != nil || body["refresh_token"] != nil || body["id_token"] != nil || body["token"] != nil {
		t.Fatalf("rate-limited response leaked token fields: %v", body)
	}
}

func decodeTokenRouteBody(t *testing.T, recorder *ut.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal token response: %v", err)
	}
	return body
}

func mustSSOHandlerTestJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func nullableSQLString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableSQLTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type fakeRouteLimiter struct {
	limit  int64
	counts map[string]int64
	keys   []string
}

func newFakeRouteLimiter(limit int64) *fakeRouteLimiter {
	return &fakeRouteLimiter{limit: limit, counts: make(map[string]int64)}
}

func (l *fakeRouteLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (limiterinfra.Decision, error) {
	_ = ctx
	_ = window
	if limit <= 0 {
		limit = l.limit
	}
	l.keys = append(l.keys, key)
	l.counts[key]++
	current := l.counts[key]
	if current > limit {
		return limiterinfra.Decision{
			Allowed:    false,
			Key:        key,
			Limit:      limit,
			Current:    current,
			RetryAfter: time.Second,
			ResetAfter: time.Minute,
		}, limiterinfra.ErrRateLimited
	}
	return limiterinfra.Decision{
		Allowed:    true,
		Key:        key,
		Limit:      limit,
		Current:    current,
		Remaining:  limit - current,
		ResetAfter: time.Minute,
	}, nil
}

func (l *fakeRouteLimiter) AllowDefault(ctx context.Context, key string) (limiterinfra.Decision, error) {
	return l.Allow(ctx, key, l.limit, time.Minute)
}

func (l *fakeRouteLimiter) sawKeyPrefix(prefix string) bool {
	for _, key := range l.keys {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

type failingRouteLimiter struct{}

func (failingRouteLimiter) Allow(context.Context, string, int64, time.Duration) (limiterinfra.Decision, error) {
	return limiterinfra.Decision{Allowed: false}, errors.New("limiter down")
}

func (failingRouteLimiter) AllowDefault(context.Context, string) (limiterinfra.Decision, error) {
	return limiterinfra.Decision{Allowed: false}, errors.New("limiter down")
}

type strictFailingRouteLimiter struct {
	strictCalled   bool
	strictFailOpen bool
	allowCalled    bool
}

func (l *strictFailingRouteLimiter) Allow(context.Context, string, int64, time.Duration) (limiterinfra.Decision, error) {
	l.allowCalled = true
	return limiterinfra.Decision{Allowed: true}, nil
}

func (l *strictFailingRouteLimiter) AllowDefault(ctx context.Context, key string) (limiterinfra.Decision, error) {
	return l.Allow(ctx, key, 1, time.Minute)
}

func (l *strictFailingRouteLimiter) AllowWithFailOpen(_ context.Context, _ string, limit int64, window time.Duration, failOpen bool) (limiterinfra.Decision, error) {
	l.strictCalled = true
	l.strictFailOpen = failOpen
	return limiterinfra.Decision{
		Allowed:    false,
		Limit:      limit,
		RetryAfter: window,
		ResetAfter: window,
	}, errors.New("limiter down")
}
