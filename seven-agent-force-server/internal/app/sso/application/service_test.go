package application

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	ssoinfra "github.com/CaixyPromise/seven-agent/force/internal/app/sso/infrastructure"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/key"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/jmoiron/sqlx"
)

func TestBuildDiscoveryDocumentUsesSingleSSOPrefix(t *testing.T) {
	service := &Service{cfg: config.SSOConfig{
		BaseURL: "http://127.0.0.1:8888/sso",
		Issuer:  "http://127.0.0.1:8888/sso",
	}}

	doc, err := service.BuildDiscoveryDocument(context.Background())
	if err != nil {
		t.Fatalf("BuildDiscoveryDocument() error = %v", err)
	}

	if got := doc["authorization_endpoint"]; got != "http://127.0.0.1:8888/sso/oauth2/authorize" {
		t.Fatalf("unexpected authorization endpoint: %v", got)
	}
	if got := doc["jwks_uri"]; got != "http://127.0.0.1:8888/sso/.well-known/jwks.json" {
		t.Fatalf("unexpected jwks uri: %v", got)
	}
	if got := doc["scopes_supported"]; got == nil {
		t.Fatalf("expected scopes_supported in discovery document")
	}
}

func TestActiveSessionCandidateRemainsNarrowAndIsNotDomainSession(t *testing.T) {
	typeOfCandidate := reflect.TypeOf(ActiveSessionCandidate{})
	fields := make([]string, 0, typeOfCandidate.NumField())
	for index := 0; index < typeOfCandidate.NumField(); index++ {
		fields = append(fields, typeOfCandidate.Field(index).Name)
	}
	want := []string{"SessionID", "UserID", "ClientID", "ACR", "AMR", "ExpiresAt"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("DG6.2 candidate fields=%v want=%v", fields, want)
	}
	if typeOfCandidate == reflect.TypeOf(domain.Session{}) {
		t.Fatal("DG6.2 candidate must not be domain.Session")
	}
}

func TestBuildDiscoveryDocumentAppendsSSOPrefixWhenMissing(t *testing.T) {
	service := &Service{cfg: config.SSOConfig{
		BaseURL: "http://127.0.0.1:8888",
		Issuer:  "http://127.0.0.1:8888/sso",
	}}

	doc, err := service.BuildDiscoveryDocument(context.Background())
	if err != nil {
		t.Fatalf("BuildDiscoveryDocument() error = %v", err)
	}

	if got := doc["token_endpoint"]; got != "http://127.0.0.1:8888/sso/oauth2/token" {
		t.Fatalf("unexpected token endpoint: %v", got)
	}
}

func TestInt64ValueParsesStringClaims(t *testing.T) {
	const expected int64 = 2034963667420737538
	if got := int64Value("2034963667420737538"); got != expected {
		t.Fatalf("int64Value(string) = %d, want %d", got, expected)
	}
	if got := int64Value([]byte("2034963667420737538")); got != expected {
		t.Fatalf("int64Value([]byte) = %d, want %d", got, expected)
	}
}

func TestAuthenticateClientRejectsSecretForPublicClient(t *testing.T) {
	service := &Service{}
	client := &domain.Client{ClientAuthMethod: "none"}
	if err := service.authenticateClient(client, "wrong-secret"); err == nil {
		t.Fatalf("authenticateClient() expected public client secret to be rejected")
	}
}

func TestAuthenticateClientRejectsInconsistentClientTypeAndAuthMethod(t *testing.T) {
	service := &Service{}
	cases := []struct {
		name   string
		client *domain.Client
		secret string
	}{
		{
			name: "public client with secret auth",
			client: &domain.Client{
				ClientType:       "public",
				ClientAuthMethod: "client_secret_basic",
				SecretHashes:     []string{"hashed-secret"},
			},
			secret: "raw-secret",
		},
		{
			name: "confidential client with none auth",
			client: &domain.Client{
				ClientType:       "confidential",
				ClientAuthMethod: "none",
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := service.authenticateClient(tt.client, tt.secret); err == nil {
				t.Fatalf("authenticateClient() accepted inconsistent client type/auth method")
			}
		})
	}
}

func TestAuthenticateClientAcceptsProductClientTypeAliases(t *testing.T) {
	service := &Service{}
	cases := []struct {
		name   string
		client *domain.Client
	}{
		{
			name: "first party spa maps to public none",
			client: &domain.Client{
				ClientType:       "FIRST_PARTY_SPA",
				ClientAuthMethod: "none",
			},
		},
		{
			name: "third party spa maps to public none",
			client: &domain.Client{
				ClientType:       "THIRD_PARTY_SPA",
				ClientAuthMethod: "none",
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := service.authenticateClient(tt.client, ""); err != nil {
				t.Fatalf("authenticateClient() rejected product client type alias: %v", err)
			}
		})
	}
}

func TestAuthenticateClientRejectsPlaintextSecretFallback(t *testing.T) {
	service := &Service{}
	client := &domain.Client{
		ClientAuthMethod: "client_secret_basic",
		SecretHashes:     []string{"plain-secret"},
	}
	if err := service.authenticateClient(client, "plain-secret"); err == nil {
		t.Fatal("authenticateClient() accepted plaintext client secret fallback")
	}
}

func TestValidateAuthorizeRequestRejectsDisallowedGrantAndMissingPKCEMethod(t *testing.T) {
	service := &Service{}
	client := &domain.Client{
		ClientID:         "client-a",
		GrantTypes:       []string{"refresh_token"},
		Scopes:           []string{"openid"},
		RequirePKCE:      true,
		RedirectURIs:     []string{"https://app.example.com/callback"},
		ClientAuthMethod: "none",
	}
	if err := service.validateAuthorizeRequest(client, "code", "https://app.example.com/callback", []string{"openid"}, "challenge", "S256", ""); err == nil {
		t.Fatal("validateAuthorizeRequest() accepted authorization_code when client grantTypes does not allow it")
	}

	client.GrantTypes = []string{"authorization_code"}
	if err := service.validateAuthorizeRequest(client, "code", "https://app.example.com/callback", []string{"openid"}, "challenge", "", ""); err == nil {
		t.Fatal("validateAuthorizeRequest() accepted PKCE challenge without explicit S256 method")
	}
	if err := service.validateAuthorizeRequest(client, "code", "https://app.example.com/callback", []string{"openid"}, "short", "S256", ""); err == nil {
		t.Fatal("validateAuthorizeRequest() accepted PKCE challenge shorter than RFC 7636 minimum length")
	}

	client.RequirePKCE = false
	if err := service.validateAuthorizeRequest(client, "code", "https://app.example.com/callback", []string{"openid"}, "", "", ""); err == nil {
		t.Fatal("validateAuthorizeRequest() accepted missing PKCE for public client")
	}
}

func TestValidateAuthorizeRequestRejectsInconsistentClientTypeAndAuthMethod(t *testing.T) {
	service := &Service{}
	base := domain.Client{
		ClientID:     "client-a",
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		RedirectURIs: []string{"https://app.example.com/callback"},
	}
	cases := []struct {
		name   string
		mutate func(*domain.Client)
	}{
		{
			name: "public client with secret auth",
			mutate: func(client *domain.Client) {
				client.ClientType = "public"
				client.ClientAuthMethod = "client_secret_basic"
			},
		},
		{
			name: "confidential client with none auth",
			mutate: func(client *domain.Client) {
				client.ClientType = "confidential"
				client.ClientAuthMethod = "none"
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			client := base
			tt.mutate(&client)
			if err := service.validateAuthorizeRequest(&client, "code", "https://app.example.com/callback", []string{"openid"}, "challenge", "S256", ""); err == nil {
				t.Fatal("validateAuthorizeRequest() accepted inconsistent client type/auth method")
			}
		})
	}
}

func TestValidateAccessTokenRejectsWrongIssuer(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://evil.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-a",
	})
	if _, err := service.ValidateAccessToken(context.Background(), token); err == nil {
		t.Fatal("ValidateAccessToken() accepted token with wrong issuer")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestValidateAccessTokenRejectsWrongAudienceBeforeSessionLookup(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"other-client"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-a",
	})

	if _, err := service.ValidateAccessToken(context.Background(), token); err == nil {
		t.Fatal("ValidateAccessToken() accepted token whose audience does not contain client_id")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestValidateAccessTokenRejectsSessionClaimMismatchBeforeTouch(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "9999",
		"uid":        int64(9999),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-a",
	})
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})

	if _, err := service.ValidateAccessToken(context.Background(), token); err == nil {
		t.Fatal("ValidateAccessToken() accepted token whose user claim does not match the active session")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestValidateAccessTokenRejectsUIDSubjectMismatchBeforeSessionLookup(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "9999",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-a",
	})

	if _, err := service.ValidateAccessToken(context.Background(), token); err == nil {
		t.Fatal("ValidateAccessToken() accepted token with mismatched uid/sub claims")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeRefreshTokenRejectsClientWithoutRefreshGrant(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})

	if _, err := service.ExchangeRefreshToken(context.Background(), "client-a", "", "unparsed.refresh.token"); err == nil {
		t.Fatal("ExchangeRefreshToken() accepted refresh_token grant for a client without refresh_token grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeDoesNotIssueRefreshWithoutRefreshGrant(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	codeVerifier := "valid-code-verifier-abcdefghijklmnopqrstuvwxyz"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:                  10,
		Code:                "code-a",
		ClientID:            "client-a",
		UserID:              1001,
		SessionID:           "session-a",
		RedirectURI:         redirectURI,
		Scopes:              []string{"openid", "offline_access"},
		CodeChallenge:       sha256Sum(codeVerifier),
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(time.Minute),
		Status:              domain.CodeStatusActive,
		CreateTime:          now,
		UpdateTime:          now,
	})
	expectConsumeAuthorizationCode(mock, "code-a")
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	expectSSOAuditLogWithTrace(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultSuccess, "exchanged", "trace-service-auth-code", `"grantType":"authorization_code"`, `"refreshIssued":false`)

	bundle, err := service.ExchangeAuthorizationCode(xcontext.WithTraceID(context.Background(), "trace-service-auth-code"), "client-a", "", "code-a", redirectURI, codeVerifier)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	if bundle == nil {
		t.Fatal("ExchangeAuthorizationCode() returned nil bundle")
	}
	if bundle.RefreshToken != "" || bundle.RefreshTokenExpiresAt != nil {
		t.Fatal("ExchangeAuthorizationCode() issued refresh token without refresh_token grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestInsertSSOAuditLogKeepsExplicitTraceID(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	expectSSOAuditLogWithTrace(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultSuccess, "exchanged", "explicit-trace", `"grantType":"authorization_code"`)

	service.insertSSOAuditLog(xcontext.WithTraceID(context.Background(), "context-trace"), domain.AuditLog{
		EventType:  "TOKEN_EXCHANGED",
		ClientID:   "client-a",
		Result:     ssoAuditResultSuccess,
		ReasonCode: "exchanged",
		DetailJSON: `{"grantType":"authorization_code"}`,
		TraceID:    "explicit-trace",
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCompleteInteractiveAuthenticationWritesAuditTraceID(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManager())
	service.cfg.LoginTransactionTTLSeconds = 300
	service.cfg.SessionIdleTimeoutSeconds = 1800
	service.cfg.SessionCookie.Name = "sso_session"

	now := time.Now().UTC()
	snapshot := &domain.AuthorizationSessionSnapshot{
		LoginTransactionID: "login-txn-a",
		ClientID:           "client-a",
		RedirectURI:        "https://app.example.com/callback",
		Scopes:             []string{"openid"},
		State:              "state-a",
		DeviceID:           "device-a",
		TenantID:           "tenant-a",
		LoginIP:            "203.0.113.10",
		UserAgent:          "Mozilla/5.0",
		TraceID:            "trace-interactive-login",
		ExpiresAt:          timePtr(now.Add(5 * time.Minute)),
	}
	if err := service.cache.SaveSession(context.Background(), snapshot, time.Minute); err != nil {
		t.Fatalf("save auth session: %v", err)
	}
	mock.ExpectExec("INSERT INTO sys_sso_session").
		WithArgs(
			sqlmock.AnyArg(),
			int64(1001),
			"client-a",
			nil,
			"device-a",
			"203.0.113.10",
			"Mozilla/5.0",
			"AAL2",
			sqlmock.AnyArg(),
			"LOCAL",
			nil,
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			domain.SessionStatusActive,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectClientLookup(mock, domain.Client{
		ID:                 1,
		ClientID:           "client-a",
		ClientName:         "client a",
		ClientType:         "public",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code"},
		Scopes:             []string{"openid"},
		AccessTokenTTLSec:  300,
		RefreshTokenTTLSec: 3600,
		Status:             domain.ClientStatusActive,
	})
	mock.ExpectExec("INSERT INTO sys_sso_authorization_code").
		WithArgs(sqlmock.AnyArg(), "client-a", int64(1001), sqlmock.AnyArg(), "https://app.example.com/callback", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "AAL2", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), domain.CodeStatusActive, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO sys_sso_audit_log").
		WithArgs(
			"INTERACTIVE_LOGIN_COMPLETED",
			"client-a",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"device-a",
			"tenant-a",
			"203.0.113.10",
			"Mozilla/5.0",
			ssoAuditResultSuccess,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"trace-interactive-login",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := service.CompleteInteractiveAuthentication(context.Background(), ssofacade.CompleteInteractiveAuthenticationCommand{
		LoginTransactionID: "login-txn-a",
		UserID:             1001,
		ACR:                "AAL2",
		AMR:                []string{"TOTP"},
		RequestContext:     &ssofacade.RequestContext{TraceID: "trace-completion-login"},
	})
	if err != nil {
		t.Fatalf("CompleteInteractiveAuthentication() error = %v", err)
	}
	if result == nil || !result.Authenticated {
		t.Fatalf("CompleteInteractiveAuthentication() result = %#v, want authenticated", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCompleteInteractiveAuthenticationStoresExternalLoginSource(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManager())
	service.cfg.LoginTransactionTTLSeconds = 300
	service.cfg.SessionIdleTimeoutSeconds = 1800
	service.cfg.SessionCookie.Name = "sso_session"

	now := time.Now().UTC()
	snapshot := &domain.AuthorizationSessionSnapshot{
		LoginTransactionID: "login-txn-external",
		ClientID:           "client-a",
		RedirectURI:        "https://app.example.com/callback",
		Scopes:             []string{"openid"},
		State:              "state-a",
		DeviceID:           "device-a",
		LoginIP:            "203.0.113.10",
		UserAgent:          "Mozilla/5.0",
		ExpiresAt:          timePtr(now.Add(5 * time.Minute)),
	}
	if err := service.cache.SaveSession(context.Background(), snapshot, time.Minute); err != nil {
		t.Fatalf("save auth session: %v", err)
	}
	mock.ExpectExec("INSERT INTO sys_sso_session").
		WithArgs(
			sqlmock.AnyArg(),
			int64(1001),
			"client-a",
			nil,
			"device-a",
			"203.0.113.10",
			"Mozilla/5.0",
			"LEVEL_1",
			jsonArrayExactMatcher{items: []string{"mfa", "oauth", "oauth:github"}},
			"EXTERNAL_OAUTH",
			"github",
			int64(501),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			domain.SessionStatusActive,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectClientLookup(mock, domain.Client{
		ID:                 1,
		ClientID:           "client-a",
		ClientName:         "client a",
		ClientType:         "public",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code"},
		Scopes:             []string{"openid"},
		AccessTokenTTLSec:  300,
		RefreshTokenTTLSec: 3600,
		Status:             domain.ClientStatusActive,
	})
	mock.ExpectExec("INSERT INTO sys_sso_authorization_code").
		WithArgs(sqlmock.AnyArg(), "client-a", int64(1001), sqlmock.AnyArg(), "https://app.example.com/callback", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "LEVEL_1", jsonArrayExactMatcher{items: []string{"mfa", "oauth", "oauth:github"}}, sqlmock.AnyArg(), sqlmock.AnyArg(), domain.CodeStatusActive, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO sys_sso_audit_log").
		WithArgs(
			"INTERACTIVE_LOGIN_COMPLETED",
			"client-a",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"device-a",
			sqlmock.AnyArg(),
			"203.0.113.10",
			"Mozilla/5.0",
			ssoAuditResultSuccess,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := service.CompleteInteractiveAuthentication(context.Background(), ssofacade.CompleteInteractiveAuthenticationCommand{
		LoginTransactionID:   "login-txn-external",
		UserID:               1001,
		LoginMethod:          "EXTERNAL_OAUTH",
		ExternalProviderCode: " GitHub ",
		ExternalIdentityID:   501,
		AMR:                  []string{"oauth", "oauth:GitHub", "mfa", "mfa"},
	})
	if err != nil {
		t.Fatalf("CompleteInteractiveAuthentication() error = %v", err)
	}
	if result == nil || !result.Authenticated {
		t.Fatalf("CompleteInteractiveAuthentication() result = %#v, want authenticated", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCompleteInteractiveAuthenticationRejectsExternalLoginWithoutProviderCode(t *testing.T) {
	service, _ := newTokenValidationTestService(t)
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManager())

	_, err := service.CompleteInteractiveAuthentication(context.Background(), ssofacade.CompleteInteractiveAuthenticationCommand{
		LoginTransactionID: "login-txn-empty-provider",
		UserID:             1001,
		LoginMethod:        "EXTERNAL_OAUTH",
	})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error for missing external provider, got %v", err)
	}
}

func TestCanonicalExternalProviderCodeAcceptsManagedHubProvider(t *testing.T) {
	got, err := canonicalExternalProviderCode(" Hub:Node-A ")
	if err != nil {
		t.Fatalf("canonicalExternalProviderCode() error = %v", err)
	}
	if got != "hub:node-a" {
		t.Fatalf("canonicalExternalProviderCode() = %q, want hub:node-a", got)
	}
}

func TestCanonicalExternalProviderCodeRejectsInvalidManagedHubOwner(t *testing.T) {
	for _, providerCode := range []string{"hub:", "hub:-node", "hub:node/a"} {
		if _, err := canonicalExternalProviderCode(providerCode); err == nil {
			t.Fatalf("canonicalExternalProviderCode(%q) accepted invalid managed owner", providerCode)
		}
	}
}

func TestBootstrapFirstPartySessionStoresExternalLoginSource(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.cfg.DefaultFirstPartyClientID = "first-party"
	service.cfg.SessionIdleTimeoutSeconds = 1800
	service.cfg.SessionCookie.Name = "sso_session"
	service.cfg.RefreshCookie.Name = "sso_refresh"

	expectClientLookup(mock, domain.Client{
		ID:                 1,
		ClientID:           "first-party",
		ClientName:         "first party",
		ClientType:         "public",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid", "profile", "email", "offline_access"},
		AccessTokenTTLSec:  300,
		RefreshTokenTTLSec: 3600,
		Status:             domain.ClientStatusActive,
	})
	mock.ExpectExec("INSERT INTO sys_sso_session").
		WithArgs(
			sqlmock.AnyArg(),
			int64(1001),
			"first-party",
			nil,
			"device-a",
			"203.0.113.10",
			"Mozilla/5.0",
			"LEVEL_1",
			jsonArrayExactMatcher{items: []string{"oauth", "oauth:google"}},
			"EXTERNAL_OAUTH",
			"google",
			int64(701),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			domain.SessionStatusActive,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectInsertRefreshFamily(mock)

	result, err := service.BootstrapFirstPartySession(context.Background(), ssofacade.BootstrapSessionCommand{
		UserID:               1001,
		LoginMethod:          "EXTERNAL_OAUTH",
		ExternalProviderCode: "google",
		ExternalIdentityID:   701,
		RequestContext: &ssofacade.RequestContext{
			DeviceID:  "device-a",
			LoginIP:   "203.0.113.10",
			UserAgent: "Mozilla/5.0",
		},
	})
	if err != nil {
		t.Fatalf("BootstrapFirstPartySession() error = %v", err)
	}
	if result == nil || result.AccessToken == "" || result.RefreshCookieHeaderValue == "" {
		t.Fatalf("BootstrapFirstPartySession() result = %#v, want token and refresh cookie", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCreateAuthorizationSessionPersistsRequestTraceID(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManager())
	service.cfg.LoginTransactionTTLSeconds = 300
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code"},
		Scopes:           []string{"openid"},
		Status:           domain.ClientStatusActive,
		RedirectURIs:     []string{"https://app.example.com/callback"},
	})

	created, err := service.CreateAuthorizationSession(context.Background(), ssofacade.CreateAuthorizationSessionRequest{
		ClientID:            "client-a",
		ResponseType:        "code",
		RedirectURI:         "https://app.example.com/callback",
		Scopes:              []string{"openid"},
		CodeChallenge:       strings.Repeat("a", 43),
		CodeChallengeMethod: "S256",
		RequestContext: &ssofacade.RequestContext{
			TraceID: " trace-auth-session ",
		},
	})
	if err != nil {
		t.Fatalf("CreateAuthorizationSession() error = %v", err)
	}
	if created == nil || created.TraceID != "trace-auth-session" {
		t.Fatalf("created auth session trace = %#v, want trace-auth-session", created)
	}
	loaded, err := service.GetAuthorizationSession(context.Background(), created.LoginTransactionID)
	if err != nil {
		t.Fatalf("GetAuthorizationSession() error = %v", err)
	}
	if loaded == nil || loaded.TraceID != "trace-auth-session" {
		t.Fatalf("loaded auth session trace = %#v, want trace-auth-session", loaded)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeWritesAuditWithoutTokenMaterial(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	codeVerifier := "valid-code-verifier-abcdefghijklmnopqrstuvwxyz"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:                  10,
		Code:                "raw-code-a",
		ClientID:            "client-a",
		UserID:              1001,
		SessionID:           "session-a",
		RedirectURI:         redirectURI,
		Scopes:              []string{"openid", "offline_access", "profile"},
		CodeChallenge:       sha256Sum(codeVerifier),
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(time.Minute),
		Status:              domain.CodeStatusActive,
		CreateTime:          now,
		UpdateTime:          now,
	})
	expectConsumeAuthorizationCode(mock, "raw-code-a")
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	expectInsertRefreshFamily(mock)
	expectSSOAuditLog(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultSuccess, "exchanged", `"grantType":"authorization_code"`, `"refreshIssued":true`, `"scopeCount":3`)

	bundle, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-a", redirectURI, codeVerifier)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	if bundle == nil || bundle.AccessToken == "" || bundle.RefreshToken == "" || bundle.IDToken == "" {
		t.Fatalf("ExchangeAuthorizationCode() did not mint expected token bundle: %#v", bundle)
	}
	refreshPrincipal, err := service.parseRefreshToken(context.Background(), bundle.RefreshToken)
	if err != nil {
		t.Fatalf("parseRefreshToken() rejected minted refresh token: %v", err)
	}
	if refreshPrincipal.UserID != 1001 || refreshPrincipal.Subject != "1001" || refreshPrincipal.ClientID != "client-a" || refreshPrincipal.SessionID != "session-a" {
		t.Fatalf("minted refresh token principal = %#v", refreshPrincipal)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsPKCEFailureWithoutConsumingCode(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:                  10,
		Code:                "raw-code-a",
		ClientID:            "client-a",
		UserID:              1001,
		SessionID:           "session-a",
		RedirectURI:         redirectURI,
		Scopes:              []string{"openid", "offline_access", "profile"},
		CodeChallenge:       sha256Sum("valid-code-verifier-abcdefghijklmnopqrstuvwxyz"),
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(time.Minute),
		Status:              domain.CodeStatusActive,
		CreateTime:          now,
		UpdateTime:          now,
	})
	expectSSOAuditLog(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "pkce_failed", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":3`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-a", redirectURI, "wrong-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted wrong PKCE verifier")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsRedirectMismatchWithoutConsumingCode(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:          10,
		Code:        "raw-code-a",
		ClientID:    "client-a",
		UserID:      1001,
		SessionID:   "session-a",
		RedirectURI: redirectURI,
		Scopes:      []string{"openid", "offline_access", "profile"},
		ExpiresAt:   now.Add(time.Minute),
		Status:      domain.CodeStatusActive,
		CreateTime:  now,
		UpdateTime:  now,
	})
	expectSSOAuditLog(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "redirect_mismatch", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":3`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-a", "https://evil.example/callback", "valid-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted mismatched redirect_uri")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsReplayAfterConditionalConsumeFailure(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	codeVerifier := "valid-code-verifier-abcdefghijklmnopqrstuvwxyz"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:                  10,
		Code:                "raw-code-a",
		ClientID:            "client-a",
		UserID:              1001,
		SessionID:           "session-a",
		RedirectURI:         redirectURI,
		Scopes:              []string{"openid", "offline_access", "profile"},
		CodeChallenge:       sha256Sum(codeVerifier),
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(time.Minute),
		Status:              domain.CodeStatusActive,
		CreateTime:          now,
		UpdateTime:          now,
	})
	expectConsumeAuthorizationCodeResult(mock, "raw-code-a", 0)
	expectSSOAuditLog(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "code_replay", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":3`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-a", redirectURI, codeVerifier)
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted replayed authorization code")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsInvalidCodeWithoutTokenMaterial(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	redirectURI := "https://app.example.com/callback"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectMissingAuthorizationCodeLookup(mock, "raw-code-missing")
	expectSSOAuditLogWithoutSubject(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "invalid_code", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":0`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-missing", redirectURI, "valid-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted missing authorization code")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsClientMismatchWithoutVictimUserBinding(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:          10,
		Code:        "raw-code-b",
		ClientID:    "client-b",
		UserID:      2002,
		SessionID:   "session-b",
		RedirectURI: redirectURI,
		Scopes:      []string{"openid", "offline_access", "profile"},
		ExpiresAt:   now.Add(time.Minute),
		Status:      domain.CodeStatusActive,
		CreateTime:  now,
		UpdateTime:  now,
	})
	expectSSOAuditLogWithoutSubject(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "client_mismatch", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":0`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-b", redirectURI, "valid-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted authorization code owned by another client")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsClientAuthenticationFailureWithoutSecret(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	redirectURI := "https://app.example.com/callback"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "confidential",
		ClientAuthMethod: "client_secret_basic",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectSSOAuditLogWithoutSubject(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "invalid_client", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":0`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "plain-secret", "raw-code-a", redirectURI, "valid-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted invalid client_secret_basic credentials")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsUnknownClientWithoutCodeLookup(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	redirectURI := "https://app.example.com/callback"
	expectMissingClientLookup(mock, "missing-client")
	expectSSOAuditLogWithoutSubject(mock, "TOKEN_EXCHANGED", "missing-client", ssoAuditResultFailure, "unknown_client", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":0`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "missing-client", "", "raw-code-a", redirectURI, "valid-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted unknown client")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeAuditsGrantNotAllowedWithoutCodeLookup(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	redirectURI := "https://app.example.com/callback"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectSSOAuditLogWithoutSubject(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "grant_not_allowed", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":0`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-a", redirectURI, "valid-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted client without authorization_code grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeRejectsPublicClientCodeMissingPKCEChallenge(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:          10,
		Code:        "raw-code-a",
		ClientID:    "client-a",
		UserID:      1001,
		SessionID:   "session-a",
		RedirectURI: redirectURI,
		Scopes:      []string{"openid", "offline_access", "profile"},
		ExpiresAt:   now.Add(time.Minute),
		Status:      domain.CodeStatusActive,
		CreateTime:  now,
		UpdateTime:  now,
	})
	expectSSOAuditLog(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "pkce_required", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":3`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-a", redirectURI, "valid-code-verifier-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted public-client code without PKCE challenge")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeAuthorizationCodeRejectsUnsupportedStoredPKCEMethod(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	redirectURI := "https://app.example.com/callback"
	codeVerifier := "valid-code-verifier-abcdefghijklmnopqrstuvwxyz"
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectAuthorizationCodeLookup(mock, domain.AuthorizationCode{
		ID:                  10,
		Code:                "raw-code-a",
		ClientID:            "client-a",
		UserID:              1001,
		SessionID:           "session-a",
		RedirectURI:         redirectURI,
		Scopes:              []string{"openid", "offline_access", "profile"},
		CodeChallenge:       sha256Sum(codeVerifier),
		CodeChallengeMethod: "plain",
		ExpiresAt:           now.Add(time.Minute),
		Status:              domain.CodeStatusActive,
		CreateTime:          now,
		UpdateTime:          now,
	})
	expectSSOAuditLog(mock, "TOKEN_EXCHANGED", "client-a", ssoAuditResultFailure, "pkce_failed", `"grantType":"authorization_code"`, `"refreshIssued":false`, `"scopeCount":3`)

	_, err := service.ExchangeAuthorizationCode(context.Background(), "client-a", "", "raw-code-a", redirectURI, codeVerifier)
	if err == nil {
		t.Fatal("ExchangeAuthorizationCode() accepted authorization code with unsupported stored PKCE method")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestParseRefreshTokenRejectsWrongIssuerAudienceExpiryAndType(t *testing.T) {
	service, _ := newTokenValidationTestService(t)
	now := time.Now().UTC()
	baseClaims := map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-a",
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "wrong issuer", mutate: func(claims map[string]any) { claims["iss"] = "https://evil.example.com/sso" }},
		{name: "wrong audience", mutate: func(claims map[string]any) { claims["aud"] = []string{"other-client"} }},
		{name: "expired", mutate: func(claims map[string]any) { claims["exp"] = now.Add(-time.Minute).Unix() }},
		{name: "wrong type", mutate: func(claims map[string]any) { claims["token_type"] = "access_token" }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			claims := cloneClaims(baseClaims)
			tt.mutate(claims)
			token := signSSOTestToken(t, service.jwt, claims)
			if _, err := service.parseRefreshToken(context.Background(), token); err == nil {
				t.Fatalf("parseRefreshToken() accepted %s", tt.name)
			}
		})
	}
}

func TestParseRefreshTokenRejectsUIDSubjectMismatch(t *testing.T) {
	service, _ := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "9999",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-a",
	})
	if _, err := service.parseRefreshToken(context.Background(), token); err == nil {
		t.Fatal("parseRefreshToken() accepted token with mismatched uid/sub claims")
	}
}

func TestParseAccessTokenForRevocationRejectsUIDSubjectMismatch(t *testing.T) {
	service, _ := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "9999",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-a",
	})
	if _, err := service.parseAccessTokenForRevocation(context.Background(), token); err == nil {
		t.Fatal("parseAccessTokenForRevocation() accepted token with mismatched uid/sub claims")
	}
}

func TestExchangeRefreshTokenRejectsPreviousTokenWithinSkewWithoutReplayPunishment(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.cfg.RefreshReplayClockSkewSec = 30
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-a",
	})
	hash := ssoinfra.BuildTokenHash(token)
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectMissingRefreshFamilyByCurrentHash(mock, hash)
	rotatedAt := now.Add(-5 * time.Second)
	expectRefreshFamilyByPreviousHash(mock, hash, domain.RefreshTokenFamily{
		ID:                10,
		FamilyID:          "family-a",
		SessionID:         "session-a",
		ClientID:          "client-a",
		UserID:            1001,
		CurrentTokenHash:  "next-token-hash",
		PreviousTokenHash: hash,
		RotatedAt:         &rotatedAt,
		ExpiresAt:         now.Add(time.Hour),
		Status:            domain.RefreshFamilyStatusActive,
		CreateTime:        now,
		UpdateTime:        now,
	})
	expectSSOAuditLog(mock, "TOKEN_REFRESH_REUSE_DETECTED", "client-a", ssoAuditResultFailure, "rotation_skew_replay", `"grantType":"refresh_token"`, `"punished":false`)

	_, err := service.ExchangeRefreshToken(context.Background(), "client-a", "", token)
	if err == nil {
		t.Fatal("ExchangeRefreshToken() accepted previous refresh token inside replay skew")
	}
	if strings.Contains(err.Error(), "reuse") {
		t.Fatalf("ExchangeRefreshToken() punished in-flight previous token as replay: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeRefreshTokenPunishesPreviousTokenBeyondSkewAsReplay(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.cfg.RefreshReplayClockSkewSec = 30
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-a",
	})
	hash := ssoinfra.BuildTokenHash(token)
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectMissingRefreshFamilyByCurrentHash(mock, hash)
	rotatedAt := now.Add(-time.Minute)
	expectRefreshFamilyByPreviousHash(mock, hash, domain.RefreshTokenFamily{
		ID:                10,
		FamilyID:          "family-a",
		SessionID:         "session-a",
		ClientID:          "client-a",
		UserID:            1001,
		CurrentTokenHash:  "next-token-hash",
		PreviousTokenHash: hash,
		RotatedAt:         &rotatedAt,
		ExpiresAt:         now.Add(time.Hour),
		Status:            domain.RefreshFamilyStatusActive,
		CreateTime:        now,
		UpdateTime:        now,
	})
	expectMarkRefreshFamilyReuseDetected(mock, "family-a")
	expectRevokeRefreshFamiliesBySessionID(mock, "session-a")
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	expectRevokeRefreshFamiliesBySessionID(mock, "session-a")
	expectRevokeSession(mock, "session-a")
	expectSSOAuditLog(mock, "TOKEN_REFRESH_REUSE_DETECTED", "client-a", ssoAuditResultFailure, "reuse_detected", `"grantType":"refresh_token"`, `"punished":true`)

	_, err := service.ExchangeRefreshToken(context.Background(), "client-a", "", token)
	if err == nil {
		t.Fatal("ExchangeRefreshToken() accepted replayed previous refresh token beyond skew")
	}
	if !strings.Contains(err.Error(), "reuse") {
		t.Fatalf("ExchangeRefreshToken() error = %v, want reuse punishment", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExchangeRefreshTokenWritesSuccessAuditWithoutTokenMaterial(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-success",
	})
	hash := ssoinfra.BuildTokenHash(token)
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectRefreshFamilyByCurrentHash(mock, hash, domain.RefreshTokenFamily{
		ID:               10,
		FamilyID:         "family-a",
		SessionID:        "session-a",
		ClientID:         "client-a",
		UserID:           1001,
		CurrentTokenHash: hash,
		ExpiresAt:        now.Add(time.Hour),
		Status:           domain.RefreshFamilyStatusActive,
		MetadataJSON:     metadataJSON(map[string]any{"scopes": []string{"openid", "offline_access"}}),
		CreateTime:       now,
		UpdateTime:       now,
	})
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	expectRotateRefreshFamily(mock, "family-a", hash, 1)
	expectSSOAuditLog(mock, "TOKEN_REFRESHED", "client-a", ssoAuditResultSuccess, "refreshed", `"grantType":"refresh_token"`, `"scopeCount":2`)

	bundle, err := service.ExchangeRefreshToken(context.Background(), "client-a", "", token)
	if err != nil {
		t.Fatalf("ExchangeRefreshToken() error = %v", err)
	}
	if bundle == nil || bundle.AccessToken == "" || bundle.RefreshToken == "" || bundle.IDToken == "" {
		t.Fatalf("ExchangeRefreshToken() did not mint replacement tokens: %#v", bundle)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestIntrospectTokenForClientReturnsActiveForCurrentRefreshToken(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-a",
	})
	hash := ssoinfra.BuildTokenHash(token)
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectRefreshFamilyByCurrentHash(mock, hash, domain.RefreshTokenFamily{
		ID:               10,
		FamilyID:         "family-a",
		SessionID:        "session-a",
		ClientID:         "client-a",
		UserID:           1001,
		CurrentTokenHash: hash,
		ExpiresAt:        now.Add(time.Hour),
		Status:           domain.RefreshFamilyStatusActive,
		CreateTime:       now,
		UpdateTime:       now,
	})
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	expectSSOAuditLog(mock, ssoAuditEventTokenIntrospected, "client-a", ssoAuditResultSuccess, "active", `"active":true`, `"tokenType":"refresh_token"`)

	result, err := service.IntrospectTokenForClient(context.Background(), "client-a", "", token, "refresh_token")
	if err != nil {
		t.Fatalf("IntrospectTokenForClient() error = %v", err)
	}
	if result["active"] != true || result["token_type"] != "refresh_token" || result["client_id"] != "client-a" || result["sid"] != "session-a" {
		t.Fatalf("unexpected refresh introspection result: %v", result)
	}
	if result["token"] != nil || result["refresh_token"] != nil {
		t.Fatalf("introspection leaked token material: %v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestIntrospectTokenForClientReturnsInactiveForRotatedRefreshToken(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-a",
	})
	hash := ssoinfra.BuildTokenHash(token)
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectMissingRefreshFamilyByCurrentHash(mock, hash)
	expectSSOAuditLog(mock, ssoAuditEventTokenIntrospected, "client-a", ssoAuditResultSuccess, "inactive", `"active":false`, `"tokenTypeHint":"refresh_token"`)

	result, err := service.IntrospectTokenForClient(context.Background(), "client-a", "", token, "refresh_token")
	if err != nil {
		t.Fatalf("IntrospectTokenForClient() error = %v", err)
	}
	if result["active"] != false || len(result) != 1 {
		t.Fatalf("rotated refresh token leaked active claims: %v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetUserInfoWritesAuditWithoutBearerToken(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.profiles = staticProfileFacade{profile: &userfacade.UserProfile{
		UserID:      1001,
		AccountName: "alice",
		NickName:    "Alice",
		Email:       "alice@example.com",
		Enabled:     true,
	}}
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile email",
		"token_type": "access_token",
		"jti":        "access-token-userinfo",
	})
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	expectSSOAuditLog(mock, "USERINFO_ACCESSED", "client-a", ssoAuditResultSuccess, "accessed", `"scopeCount":3`)

	result, err := service.GetUserInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("GetUserInfo() error = %v", err)
	}
	if result["email"] != "alice@example.com" {
		t.Fatalf("GetUserInfo() missing email claim: %v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetUserInfoAuditsMissingProfileWithoutBearerToken(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.profiles = staticProfileFacade{}
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-userinfo-missing-profile",
	})
	expectActiveSessionLookup(mock, "session-a", domain.Session{
		ID:        1,
		SessionID: "session-a",
		UserID:    1001,
		ClientID:  "client-a",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	expectSSOAuditLog(mock, "USERINFO_ACCESSED", "client-a", ssoAuditResultFailure, "profile_missing", `"scopeCount":2`)

	if _, err := service.GetUserInfo(context.Background(), token); err == nil {
		t.Fatal("GetUserInfo() accepted missing profile")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeTokenForClientNoOpsMismatchedRefreshTokenClient(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-b"},
		"client_id":  "client-b",
		"sid":        "session-b",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-b",
	})
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectSSOAuditLog(mock, ssoAuditEventTokenRevoked, "client-a", ssoAuditResultSuccess, "cross_client_noop", `"outcome":"cross_client_noop"`, `"tokenTypeHint":"refresh_token"`)

	if err := service.RevokeTokenForClient(context.Background(), "client-a", "", token, "refresh_token"); err != nil {
		t.Fatalf("RevokeTokenForClient() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeTokenForClientRevokesMatchingRefreshTokenClient(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-a",
	})
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectRefreshFamilyByCurrentHash(mock, ssoinfra.BuildTokenHash(token), domain.RefreshTokenFamily{
		ID:               10,
		FamilyID:         "family-a",
		SessionID:        "session-a",
		ClientID:         "client-a",
		UserID:           1001,
		CurrentTokenHash: ssoinfra.BuildTokenHash(token),
		ExpiresAt:        now.Add(time.Hour),
		Status:           domain.RefreshFamilyStatusActive,
		CreateTime:       now,
		UpdateTime:       now,
	})
	expectRevokeRefreshFamiliesBySessionID(mock, "session-a")
	expectRevokeSession(mock, "session-a")
	expectSSOAuditLog(mock, ssoAuditEventTokenRevoked, "client-a", ssoAuditResultSuccess, "revoked", `"outcome":"revoked"`, `"tokenTypeHint":"refresh_token"`)

	if err := service.RevokeTokenForClient(context.Background(), "client-a", "", token, "refresh_token"); err != nil {
		t.Fatalf("RevokeTokenForClient() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeTokenForClientAuditsMissingRefreshFamilyAsNoop(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-a"},
		"client_id":  "client-a",
		"sid":        "session-a",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid offline_access",
		"token_type": "refresh_token",
		"jti":        "refresh-token-missing-family",
	})
	hash := ssoinfra.BuildTokenHash(token)
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		Status:           domain.ClientStatusActive,
	})
	expectMissingRefreshFamilyByCurrentHash(mock, hash)
	expectMissingRefreshFamilyByPreviousHash(mock, hash)
	expectSSOAuditLog(mock, ssoAuditEventTokenRevoked, "client-a", ssoAuditResultSuccess, "invalid_token_noop", `"outcome":"invalid_token_noop"`, `"tokenTypeHint":"refresh_token"`)

	if err := service.RevokeTokenForClient(context.Background(), "client-a", "", token, "refresh_token"); err != nil {
		t.Fatalf("RevokeTokenForClient() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeTokenForClientNoOpsMismatchedAccessTokenClient(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	token := signSSOTestToken(t, service.jwt, map[string]any{
		"iss":        "https://auth.example.com/sso",
		"sub":        "1001",
		"uid":        int64(1001),
		"aud":        []string{"client-b"},
		"client_id":  "client-b",
		"sid":        "session-b",
		"iat":        now.Unix(),
		"exp":        now.Add(10 * time.Minute).Unix(),
		"scope":      "openid profile",
		"token_type": "access_token",
		"jti":        "access-token-b",
	})
	expectClientLookup(mock, domain.Client{
		ID:               1,
		ClientID:         "client-a",
		ClientName:       "client a",
		ClientType:       "public",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "profile"},
		Status:           domain.ClientStatusActive,
	})
	expectSSOAuditLog(mock, ssoAuditEventTokenRevoked, "client-a", ssoAuditResultSuccess, "invalid_token_noop", `"outcome":"invalid_token_noop"`, `"tokenTypeHint":"access_token"`)

	if err := service.RevokeTokenForClient(context.Background(), "client-a", "", token, "access_token"); err != nil {
		t.Fatalf("RevokeTokenForClient() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByUserIDReturnsRevokedCount(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	expectListSessionsByUserID(mock, 1001, domain.Session{
		ID:        20,
		SessionID: "session-a",
		ClientID:  "client-a",
		UserID:    1001,
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}, domain.Session{
		ID:        21,
		SessionID: "session-b",
		ClientID:  "client-b",
		UserID:    1001,
		LoginAt:   now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}, domain.Session{
		ID:        22,
		SessionID: "session-c",
		ClientID:  "client-c",
		UserID:    1001,
		LoginAt:   now.Add(-3 * time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	})
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), int64(1001), sqlmock.AnyArg(), domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), int64(1001), sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 3))
	expectSSOAuditLogWithoutClient(mock, "USER_SESSIONS_REVOKED", int64(1001), ssoAuditResultSuccess, "revoked",
		`"operation":"revoke_user_sessions"`,
		`"revokedCount":3`,
		`"activeSessionCountBefore":3`,
	)

	revokedCount, err := service.RevokeSessionsByUserID(context.Background(), 1001)
	if err != nil {
		t.Fatalf("RevokeSessionsByUserID() error = %v", err)
	}
	if revokedCount != 3 {
		t.Fatalf("RevokeSessionsByUserID() revokedCount = %d, want 3", revokedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByUserIDAtOrBeforeUsesCutoffForSQLAndWatermark(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	mini := miniredis.RunT(t)
	cacheConfig := config.CacheConfig{Enabled: true, Codec: "sonic", Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "seven", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	manager, err := cache.NewDefaultManager(cacheConfig, cache.NewProvider(cacheConfig))
	if err != nil {
		t.Fatalf("cache manager: %v", err)
	}
	service.cache = ssoinfra.NewAuthSessionCache(manager)
	cutoff := time.Date(2026, 7, 11, 12, 0, 0, 123000000, time.UTC)
	mock.ExpectExec(`UPDATE sys_sso_refresh_token_family`).
		WithArgs(cutoff, domain.RefreshFamilyStatusRevoked, cutoff, int64(1001), cutoff, domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE sys_sso_session`).
		WithArgs(cutoff, domain.SessionStatusRevoked, cutoff, int64(1001), cutoff, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 3))

	changed, err := service.RevokeSessionsByUserIDAtOrBefore(context.Background(), 1001, cutoff)
	if err != nil || changed != 3 {
		t.Fatalf("RevokeSessionsByUserIDAtOrBefore()=%d err=%v", changed, err)
	}
	watermark, err := service.cache.UserRevokedAt(context.Background(), 1001)
	if err != nil || watermark == nil || !watermark.Equal(cutoff) {
		t.Fatalf("revocation watermark=%v err=%v want %s", watermark, err, cutoff)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUserRevocationWatermarkRejectsAtOrBeforeAndPreservesLaterSessions(t *testing.T) {
	cutoff := time.Date(2026, 7, 11, 12, 0, 0, 123000000, time.UTC)
	if !revokedByUserWatermark(cutoff.Add(-time.Nanosecond), cutoff) {
		t.Fatal("session before cutoff survived")
	}
	if !revokedByUserWatermark(cutoff, cutoff) {
		t.Fatal("session at cutoff survived")
	}
	if revokedByUserWatermark(cutoff.Add(time.Nanosecond), cutoff) {
		t.Fatal("session after cutoff was revoked")
	}
}

func TestUserRevocationWatermarkUsesImmutableSessionCreateTime(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManager())
	cutoff := time.Now().UTC().Add(-time.Minute)
	loginAt := cutoff.Add(-time.Hour)
	createTime := cutoff.Add(time.Second)
	expiresAt := cutoff.Add(24 * time.Hour)
	if err := service.cache.MarkUserRevoked(context.Background(), 1001, cutoff); err != nil {
		t.Fatalf("mark user revoked: %v", err)
	}
	mock.ExpectQuery("SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson").
		WithArgs("late-created-session").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson",
			"loginMethod", "externalProviderCode", "externalIdentityId", "loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}).AddRow(1, "late-created-session", 1001, "console", nil, nil, nil, nil, nil, "[]", nil, nil, nil,
			loginAt, loginAt, expiresAt, nil, domain.SessionStatusActive, nil, createTime, createTime))

	session, err := service.ResolveActiveSession(context.Background(), "late-created-session")
	if err != nil {
		t.Fatalf("resolve late-created session: %v", err)
	}
	if session == nil {
		t.Fatal("session created after cutoff was rejected because its login time predates acceptance")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionWritesLifecycleAuditSnapshot(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	session := domain.Session{
		ID:        20,
		SessionID: "session-a",
		ClientID:  "client-a",
		UserID:    1001,
		DeviceID:  "device-a",
		LoginIP:   "127.0.0.1",
		UserAgent: "unit-test",
		LoginAt:   now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		Status:    domain.SessionStatusActive,
	}
	expectActiveSessionLookup(mock, "session-a", session)
	expectRevokeRefreshFamiliesBySessionID(mock, "session-a")
	expectRevokeSession(mock, "session-a")
	expectSSOAuditLog(mock, "SESSION_REVOKED", "client-a", ssoAuditResultSuccess, "revoked",
		`"operation":"revoke_session"`,
		`"sessionId":"session-a"`,
		`"deviceId":"device-a"`,
		`"statusBefore":0`,
		`"revoked":true`,
		`"revokedCount":1`,
	)

	revoked, err := service.RevokeSession(context.Background(), "session-a")
	if err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if !revoked {
		t.Fatal("RevokeSession() revoked = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeManagedSessionMutatesWithoutSSOAudit(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	expectRevokeRefreshFamiliesBySessionID(mock, "managed-session-a")
	expectRevokeSession(mock, "managed-session-a")

	revoked, err := service.RevokeManagedSession(context.Background(), "managed-session-a")
	if err != nil {
		t.Fatalf("RevokeManagedSession() error = %v", err)
	}
	if !revoked {
		t.Fatal("RevokeManagedSession() revoked = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("managed revocation emitted an ordinary SSO audit or missed mutation: %v", err)
	}
}

func TestRevokeSessionsByUserIDWritesLifecycleAuditSnapshot(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	expectListSessionsByUserID(mock, 1001,
		domain.Session{
			ID:        20,
			SessionID: "session-a",
			ClientID:  "client-a",
			UserID:    1001,
			DeviceID:  "device-a",
			LoginIP:   "127.0.0.1",
			UserAgent: "unit-test-a",
			LoginAt:   now.Add(-time.Minute),
			ExpiresAt: now.Add(time.Hour),
			Status:    domain.SessionStatusActive,
		},
		domain.Session{
			ID:        21,
			SessionID: "session-b",
			ClientID:  "client-b",
			UserID:    1001,
			DeviceID:  "device-b",
			LoginIP:   "127.0.0.2",
			UserAgent: "unit-test-b",
			LoginAt:   now.Add(-2 * time.Minute),
			ExpiresAt: now.Add(time.Hour),
			Status:    domain.SessionStatusActive,
		},
	)
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), int64(1001), sqlmock.AnyArg(), domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), int64(1001), sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))
	expectSSOAuditLogWithoutClient(mock, "USER_SESSIONS_REVOKED", int64(1001), ssoAuditResultSuccess, "revoked",
		`"operation":"revoke_user_sessions"`,
		`"revokedCount":2`,
		`"activeSessionCountBefore":2`,
		`"sessionId":"session-a"`,
		`"sessionId":"session-b"`,
		`"deviceId":"device-a"`,
		`"deviceId":"device-b"`,
	)

	revokedCount, err := service.RevokeSessionsByUserID(context.Background(), 1001)
	if err != nil {
		t.Fatalf("RevokeSessionsByUserID() error = %v", err)
	}
	if revokedCount != 2 {
		t.Fatalf("RevokeSessionsByUserID() revokedCount = %d, want 2", revokedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByExternalProviderRevokesActiveSessionsOnly(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	expectListActiveSessionsByExternalProvider(mock, "github",
		domain.Session{
			ID:                   20,
			SessionID:            "session-provider-a",
			ClientID:             "client-a",
			UserID:               1001,
			DeviceID:             "device-a",
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
			ExternalIdentityID:   501,
		},
		domain.Session{
			ID:                   21,
			SessionID:            "session-provider-b",
			ClientID:             "client-b",
			UserID:               1002,
			DeviceID:             "device-b",
			LoginAt:              now.Add(-2 * time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
			ExternalIdentityID:   502,
		},
	)
	expectRevokeRefreshFamiliesByExternalProvider(mock, "github", 2)
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), "github", sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))
	expectSSOAuditLogWithSubjectAndTrace(mock, "EXTERNAL_PROVIDER_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"operation":"revoke_external_provider_sessions"`,
		`"providerCode":"github"`,
		`"revokedCount":2`,
		`"sessionId":"session-provider-a"`,
		`"sessionId":"session-provider-b"`,
	)

	revokedCount, err := service.RevokeSessionsByExternalProvider(context.Background(), " GitHub ")
	if err != nil {
		t.Fatalf("RevokeSessionsByExternalProvider() error = %v", err)
	}
	if revokedCount != 2 {
		t.Fatalf("RevokeSessionsByExternalProvider() revokedCount = %d, want 2", revokedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByExternalProviderPagesSessionEffectsWithBoundedAuditSnapshot(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	firstPage := make([]domain.Session, sessionRevocationPageSize)
	for index := range firstPage {
		firstPage[index] = domain.Session{
			ID:                   int64(index + 1),
			SessionID:            fmt.Sprintf("session-provider-%03d", index+1),
			ClientID:             "client-a",
			UserID:               int64(1000 + index),
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
		}
	}
	cutoff := now.Add(time.Second)
	mock.ExpectQuery("SELECT CURRENT_TIMESTAMP").
		WillReturnRows(sqlmock.NewRows([]string{"cutoff"}).AddRow(cutoff))
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs("github", domain.SessionStatusActive, cutoff, int64(0), sessionRevocationPageSize).
		WillReturnRows(newSessionRows(firstPage...))
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs("github", domain.SessionStatusActive, cutoff, int64(sessionRevocationPageSize), sessionRevocationPageSize).
		WillReturnRows(newSessionRows(domain.Session{
			ID:                   int64(sessionRevocationPageSize + 1),
			SessionID:            "session-provider-101",
			ClientID:             "client-a",
			UserID:               1101,
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
		}))
	mock.ExpectExec("(?s)UPDATE sys_sso_refresh_token_family.*externalProviderCode = \\?.*createTime <= \\?").
		WithArgs(cutoff, domain.RefreshFamilyStatusRevoked, cutoff, domain.RefreshFamilyStatusActive, "github", cutoff, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 101))
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(cutoff, domain.SessionStatusRevoked, cutoff, "github", cutoff, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 101))
	expectSSOAuditLogWithSubjectAndTrace(mock, "EXTERNAL_PROVIDER_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"providerCode":"github"`,
		`"revokedCount":101`,
		`"activeSessionCountBefore":101`,
		`"sessionSnapshotCount":100`,
		`"sessionSnapshotsTruncated":true`,
		`"sessionId":"session-provider-001"`,
		`"sessionId":"session-provider-100"`,
	)

	revokedCount, err := service.RevokeSessionsByExternalProvider(context.Background(), "github")
	if err != nil {
		t.Fatalf("RevokeSessionsByExternalProvider() error=%v", err)
	}
	if revokedCount != 101 {
		t.Fatalf("revokedCount=%d, want 101", revokedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByExternalIdentityRevokesActiveSessionsOnly(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	expectListActiveSessionsByExternalIdentity(mock, 501,
		domain.Session{
			ID:                   20,
			SessionID:            "session-identity-a",
			ClientID:             "client-a",
			UserID:               1001,
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
			ExternalIdentityID:   501,
		},
	)
	expectRevokeRefreshFamiliesByExternalIdentity(mock, 501, 1)
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), int64(501), sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSSOAuditLogWithSubjectAndTrace(mock, "EXTERNAL_IDENTITY_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"operation":"revoke_external_identity_sessions"`,
		`"externalIdentityId":501`,
		`"revokedCount":1`,
		`"sessionId":"session-identity-a"`,
	)

	revokedCount, err := service.RevokeSessionsByExternalIdentity(context.Background(), 501)
	if err != nil {
		t.Fatalf("RevokeSessionsByExternalIdentity() error = %v", err)
	}
	if revokedCount != 1 {
		t.Fatalf("RevokeSessionsByExternalIdentity() revokedCount = %d, want 1", revokedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByExternalProviderRevokesRefreshFamiliesForMatchedSessions(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Now().UTC()
	expectListActiveSessionsByExternalProvider(mock, "google",
		domain.Session{
			ID:                   30,
			SessionID:            "session-google-a",
			ClientID:             "client-a",
			UserID:               2001,
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "google",
			ExternalIdentityID:   701,
		},
	)
	expectRevokeRefreshFamiliesByExternalProvider(mock, "google", 2)
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), "google", sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))
	expectSSOAuditLogWithSubjectAndTrace(mock, "EXTERNAL_PROVIDER_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"providerCode":"google"`,
		`"sessionId":"session-google-a"`,
	)

	revokedCount, err := service.RevokeSessionsByExternalProvider(context.Background(), "google")
	if err != nil {
		t.Fatalf("RevokeSessionsByExternalProvider() error = %v", err)
	}
	if revokedCount != 2 {
		t.Fatalf("RevokeSessionsByExternalProvider() revokedCount = %d, want set-based revoke count 2", revokedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByExternalProviderDoesNotMarkWholeUserRevoked(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	cacheLayer := &ssoTestCacheLayer{values: map[string][]byte{}}
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManagerWithLayer(cacheLayer))
	now := time.Now().UTC()
	if _, err := service.cache.AllowTouch(context.Background(), "session-provider-a", time.Minute); err != nil {
		t.Fatalf("seed session touch cache: %v", err)
	}
	expectListActiveSessionsByExternalProvider(mock, "github",
		domain.Session{
			ID:                   20,
			SessionID:            "session-provider-a",
			ClientID:             "client-a",
			UserID:               1001,
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
			ExternalIdentityID:   501,
		},
	)
	expectRevokeRefreshFamiliesByExternalProvider(mock, "github", 1)
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), "github", sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSSOAuditLogWithSubjectAndTrace(mock, "EXTERNAL_PROVIDER_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"providerCode":"github"`,
		`"sessionId":"session-provider-a"`,
	)

	if _, err := service.RevokeSessionsByExternalProvider(context.Background(), "github"); err != nil {
		t.Fatalf("RevokeSessionsByExternalProvider() error = %v", err)
	}
	if cacheLayer.hasKeyContaining("session:revoked:user") {
		t.Fatal("RevokeSessionsByExternalProvider() marked whole user revoked")
	}
	if cacheLayer.hasKeyContaining("session-touch") {
		t.Fatal("RevokeSessionsByExternalProvider() left per-session touch cache behind")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByPlatformCodeRevokesRefreshFamiliesAndClearsSessionTouch(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	cacheLayer := &ssoTestCacheLayer{values: map[string][]byte{}}
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManagerWithLayer(cacheLayer))
	now := time.Now().UTC()
	if _, err := service.cache.AllowTouch(context.Background(), "session-platform-a", time.Minute); err != nil {
		t.Fatalf("seed session touch cache: %v", err)
	}
	expectListActiveSessionsByPlatformCode(mock, "seven-admin",
		domain.Session{
			ID:           40,
			SessionID:    "session-platform-a",
			ClientID:     "authorization-console",
			PlatformCode: "seven-admin",
			UserID:       3001,
			LoginAt:      now.Add(-time.Minute),
			ExpiresAt:    now.Add(time.Hour),
			Status:       domain.SessionStatusActive,
			LoginMethod:  "PASSWORD",
		},
	)
	expectRevokeRefreshFamiliesByPlatformCode(mock, "seven-admin", 1)
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), "seven-admin", sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSSOAuditLogWithSubjectAndTrace(mock, "PLATFORM_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"platformCode":"seven-admin"`,
		`"sessionId":"session-platform-a"`,
	)

	revokedCount, err := service.RevokeSessionsByPlatformCode(context.Background(), " seven-admin ")
	if err != nil {
		t.Fatalf("RevokeSessionsByPlatformCode() error = %v", err)
	}
	if revokedCount != 1 {
		t.Fatalf("RevokeSessionsByPlatformCode() revokedCount = %d, want 1", revokedCount)
	}
	if cacheLayer.hasKeyContaining("session:revoked:user") {
		t.Fatal("RevokeSessionsByPlatformCode() marked whole user revoked")
	}
	if cacheLayer.hasKeyContaining("session-touch") {
		t.Fatal("RevokeSessionsByPlatformCode() left per-session touch cache behind")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByPlatformLoginMethodScopesProviderAndClearsSessionTouch(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	cacheLayer := &ssoTestCacheLayer{values: map[string][]byte{}}
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManagerWithLayer(cacheLayer))
	now := time.Now().UTC()
	if _, err := service.cache.AllowTouch(context.Background(), "session-github-a", time.Minute); err != nil {
		t.Fatalf("seed session touch cache: %v", err)
	}
	expectListActiveSessionsByPlatformLoginMethod(mock, "seven-admin", "EXTERNAL_OAUTH", "github",
		domain.Session{
			ID:                   41,
			SessionID:            "session-github-a",
			ClientID:             "authorization-console",
			PlatformCode:         "seven-admin",
			UserID:               3001,
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
		},
	)
	expectRevokeRefreshFamiliesByPlatformLoginMethod(mock, "seven-admin", "EXTERNAL_OAUTH", "github", 1)
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), "seven-admin", "EXTERNAL_OAUTH", "github", sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSSOAuditLogWithSubjectAndTrace(mock, "PLATFORM_LOGIN_METHOD_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"platformCode":"seven-admin"`,
		`"loginMethod":"EXTERNAL_OAUTH"`,
		`"externalProviderCode":"github"`,
		`"sessionId":"session-github-a"`,
	)

	revokedCount, err := service.RevokeSessionsByPlatformLoginMethod(context.Background(), " seven-admin ", " external_oauth ", " github ")
	if err != nil {
		t.Fatalf("RevokeSessionsByPlatformLoginMethod() error = %v", err)
	}
	if revokedCount != 1 {
		t.Fatalf("RevokeSessionsByPlatformLoginMethod() revokedCount = %d, want 1", revokedCount)
	}
	if cacheLayer.hasKeyContaining("session:revoked:user") {
		t.Fatal("RevokeSessionsByPlatformLoginMethod() marked whole user revoked")
	}
	if cacheLayer.hasKeyContaining("session-touch") {
		t.Fatal("RevokeSessionsByPlatformLoginMethod() left per-session touch cache behind")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRevokeSessionsByExternalIdentityDoesNotMarkWholeUserRevoked(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	cacheLayer := &ssoTestCacheLayer{values: map[string][]byte{}}
	service.cache = ssoinfra.NewAuthSessionCache(newSSOTestCacheManagerWithLayer(cacheLayer))
	now := time.Now().UTC()
	if _, err := service.cache.AllowTouch(context.Background(), "session-identity-a", time.Minute); err != nil {
		t.Fatalf("seed session touch cache: %v", err)
	}
	expectListActiveSessionsByExternalIdentity(mock, 501,
		domain.Session{
			ID:                   20,
			SessionID:            "session-identity-a",
			ClientID:             "client-a",
			UserID:               1001,
			LoginAt:              now.Add(-time.Minute),
			ExpiresAt:            now.Add(time.Hour),
			Status:               domain.SessionStatusActive,
			LoginMethod:          "EXTERNAL_OAUTH",
			ExternalProviderCode: "github",
			ExternalIdentityID:   501,
		},
	)
	expectRevokeRefreshFamiliesByExternalIdentity(mock, 501, 1)
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), int64(501), sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSSOAuditLogWithSubjectAndTrace(mock, "EXTERNAL_IDENTITY_SESSIONS_REVOKED", nil, nil, nil, ssoAuditResultSuccess, "revoked", "",
		`"externalIdentityId":501`,
		`"sessionId":"session-identity-a"`,
	)

	if _, err := service.RevokeSessionsByExternalIdentity(context.Background(), 501); err != nil {
		t.Fatalf("RevokeSessionsByExternalIdentity() error = %v", err)
	}
	if cacheLayer.hasKeyContaining("session:revoked:user") {
		t.Fatal("RevokeSessionsByExternalIdentity() marked whole user revoked")
	}
	if cacheLayer.hasKeyContaining("session-touch") {
		t.Fatal("RevokeSessionsByExternalIdentity() left per-session touch cache behind")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestVerifyPKCERejectsInvalidVerifierSyntax(t *testing.T) {
	shortVerifier := "short"
	if verifyPKCE(sha256Sum(shortVerifier), shortVerifier) {
		t.Fatal("verifyPKCE() accepted verifier shorter than RFC 7636 minimum length")
	}
	invalidCharVerifier := strings.Repeat("a", 42) + "%"
	if verifyPKCE(sha256Sum(invalidCharVerifier), invalidCharVerifier) {
		t.Fatal("verifyPKCE() accepted verifier with characters outside RFC 7636 unreserved set")
	}
}

func TestNormalizeScopesRequiresExplicitOpenID(t *testing.T) {
	service := &Service{}
	client := &domain.Client{Scopes: []string{"openid", "profile", "email"}}
	if _, err := service.normalizeScopes(client, nil); err == nil {
		t.Fatalf("normalizeScopes() expected empty scopes to be rejected")
	}
	if _, err := service.normalizeScopes(client, []string{"profile"}); err == nil {
		t.Fatalf("normalizeScopes() expected missing openid to be rejected")
	}
}

type sqlmockProvider struct {
	db *sqlx.DB
}

func (p sqlmockProvider) Driver() string               { return "sqlmock" }
func (p sqlmockProvider) Dialect() string              { return "mysql" }
func (p sqlmockProvider) DB() *sql.DB                  { return p.db.DB }
func (p sqlmockProvider) SQLX() *sqlx.DB               { return p.db }
func (p sqlmockProvider) Close() error                 { return nil }
func (p sqlmockProvider) Transactor() store.Transactor { return nil }
func (p sqlmockProvider) Configured() bool             { return true }

type staticProfileFacade struct {
	profile *userfacade.UserProfile
	err     error
}

func (f staticProfileFacade) GetProfileByUserID(context.Context, int64) (*userfacade.UserProfile, error) {
	return f.profile, f.err
}

func (f staticProfileFacade) UpdateSelfProfile(context.Context, userfacade.UpdateSelfProfileCommand) error {
	return nil
}

func (f staticProfileFacade) CommitCurrentUserAvatar(context.Context, int64, int64) (string, error) {
	return "", nil
}

func (f staticProfileFacade) UpdateSelfEmail(context.Context, userfacade.UpdateSelfEmailCommand) error {
	return nil
}

func (f staticProfileFacade) SyncExternalProfile(context.Context, userfacade.SyncExternalProfileCommand) error {
	return nil
}

func newTokenValidationTestService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	repo, err := ssoinfra.NewRepository(sqlmockProvider{db: sqlx.NewDb(rawDB, "sqlmock")})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	return &Service{
		cfg: config.SSOConfig{
			Issuer:                     "https://auth.example.com/sso",
			SessionTouchThrottleSecond: 30,
		},
		repository: repo,
		cache:      ssoinfra.NewAuthSessionCache(nil),
		jwt:        newSSOTestJWTService(t),
		password:   passwordService,
	}, mock
}

func newSSOTestJWTService(t *testing.T) *jwtinfra.Service {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "jwt-private.pem")
	publicPath := filepath.Join(dir, "jwt-public.pem")
	writeSSOTestPEM(t, privatePath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey))
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	writeSSOTestPEM(t, publicPath, "PUBLIC KEY", publicDER)
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
		t.Fatalf("new local key provider: %v", err)
	}
	service, err := jwtinfra.New(keys, "RS256")
	if err != nil {
		t.Fatalf("new jwt service: %v", err)
	}
	return service
}

func writeSSOTestPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	block := &pem.Block{Type: blockType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}
}

func signSSOTestToken(t *testing.T, service *jwtinfra.Service, claims map[string]any) string {
	t.Helper()
	token, err := service.Sign(context.Background(), claims)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestClientCapabilitiesOnlyExposeSupportedV1Options(t *testing.T) {
	service := &Service{}
	capabilities := service.ClientCapabilities(context.Background())

	if got := strings.Join(capabilities["clientTypes"].([]string), ","); got != "PUBLIC,CONFIDENTIAL" {
		t.Fatalf("unexpected client types: %s", got)
	}
	if got := strings.Join(capabilities["clientAuthMethods"].([]string), ","); got != "none,client_secret_basic" {
		t.Fatalf("unexpected auth methods: %s", got)
	}
	if got := strings.Join(capabilities["grantTypes"].([]string), ","); got != "authorization_code,refresh_token" {
		t.Fatalf("unexpected grant types: %s", got)
	}
	if got := strings.Join(capabilities["codeChallengeMethods"].([]string), ","); got != "S256" {
		t.Fatalf("unexpected challenge methods: %s", got)
	}
	if got := strings.Join(capabilities["signingAlgorithms"].([]string), ","); got != "RS256" {
		t.Fatalf("unexpected signing algorithms: %s", got)
	}
	encoded := mustJSON(capabilities)
	for _, unsupported := range []string{"implicit", "client_credentials", "password", "HS256", "plain", "private_key_jwt"} {
		if strings.Contains(encoded, unsupported) {
			t.Fatalf("capabilities exposed unsupported option %q in %s", unsupported, encoded)
		}
	}
}

func TestListClientsReturnsStableAdminPage(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sys_sso_client c WHERE c\\.isDeleted = 0").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT c\\.id, c\\.clientId, c\\.clientName, c\\.clientType").
		WithArgs(200, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
			"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
			"status", "metadataJson", "activeRedirectCount", "activeSecretCount", "createTime", "updateTime",
		}).AddRow(
			int64(7), "authorization-console", "Authorization Console", "PUBLIC", "none",
			`["authorization_code","refresh_token"]`, `["openid","profile"]`,
			1, 0, 1, 1800, 2592000, domain.ClientStatusActive, `{"seed":true}`, 2, 0, now, now,
		))

	page, err := service.ListClients(context.Background(), ssofacade.ClientAdminQuery{Current: -1, PageSize: 500})
	if err != nil {
		t.Fatalf("ListClients() error = %v", err)
	}
	if page.Current != 1 || page.PageSize != 200 || page.Total != 1 || len(page.Records) != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	item := page.Records[0]
	if item.ClientID != "authorization-console" || item.ActiveRedirectURICount != 2 || item.ActiveSecretCount != 0 {
		t.Fatalf("unexpected client record: %#v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetClientNeverReturnsSecretHash(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Date(2026, 6, 18, 10, 30, 0, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)

	expectClientDetailLookup(mock, domain.Client{
		ID:                  11,
		ClientID:            "demo-client",
		ClientName:          "Demo Client",
		ClientType:          "CONFIDENTIAL",
		ClientAuthMethod:    "client_secret_basic",
		GrantTypes:          []string{"authorization_code", "refresh_token"},
		Scopes:              []string{"openid", "email"},
		RequirePKCE:         true,
		AccessTokenTTLSec:   1800,
		RefreshTokenTTLSec:  2592000,
		Status:              domain.ClientStatusActive,
		ActiveRedirectCount: 1,
		ActiveSecretCount:   1,
		CreateTime:          now,
		UpdateTime:          now,
	})
	mock.ExpectQuery("SELECT id, clientId, redirectUri, postLogoutRedirectUri, status, createTime, updateTime").
		WithArgs("demo-client", domain.ClientStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "redirectUri", "postLogoutRedirectUri", "status", "createTime", "updateTime"}).
			AddRow(int64(20), "demo-client", "https://demo.example/callback", nil, domain.ClientStatusActive, now, now))
	mock.ExpectQuery("SELECT id, clientId, secretHint, expiresAt, status, createTime, updateTime").
		WithArgs("demo-client").
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "secretHint", "expiresAt", "status", "createTime", "updateTime"}).
			AddRow(int64(30), "demo-client", "sec_****abcd", expiresAt, domain.ClientStatusActive, now, now))

	detail, err := service.GetClient(context.Background(), " demo-client ")
	if err != nil {
		t.Fatalf("GetClient() error = %v", err)
	}
	if detail.ClientID != "demo-client" || len(detail.RedirectURIs) != 1 || len(detail.Secrets) != 1 {
		t.Fatalf("unexpected detail: %#v", detail)
	}
	encoded := mustJSON(detail)
	for _, forbidden := range []string{"secretHash", "argon2id", "plain-secret", "sec_live_"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("detail leaked forbidden secret marker %q in %s", forbidden, encoded)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListClientSecretsRequiresExistingClient(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	mock.ExpectQuery("SELECT c\\.id, c\\.clientId, c\\.clientName, c\\.clientType").
		WithArgs("missing-client").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
			"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
			"status", "metadataJson", "activeRedirectCount", "activeSecretCount", "createTime", "updateTime",
		}))

	_, err := service.ListClientSecrets(context.Background(), " missing-client ")
	if err == nil {
		t.Fatal("expected not found error")
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeNotFound {
		t.Fatalf("expected not found error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCreateClientRejectsPublicClientWithSecretAuth(t *testing.T) {
	service := &Service{}
	_, err := service.CreateClient(context.Background(), 1001, ssofacade.ClientAdminSaveRequest{
		ClientID:           "demo-client",
		ClientName:         "Demo Client",
		ClientType:         "PUBLIC",
		ClientAuthMethod:   "client_secret_basic",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid", "email"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
	}, validSSOClientProof("SSO_CLIENT_CREATE", "unused"))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error, got %v", err)
	}
}

func TestCreateClientRejectsConfidentialClientWithoutSecretAuth(t *testing.T) {
	service := &Service{}
	_, err := service.CreateClient(context.Background(), 1001, ssofacade.ClientAdminSaveRequest{
		ClientID:           "demo-client",
		ClientName:         "Demo Client",
		ClientType:         "CONFIDENTIAL",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid", "email"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
	}, validSSOClientProof("SSO_CLIENT_CREATE", "unused"))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error, got %v", err)
	}
}

func TestCreateClientRejectsMissingOpenIDScope(t *testing.T) {
	service := &Service{}
	_, err := service.CreateClient(context.Background(), 1001, ssofacade.ClientAdminSaveRequest{
		ClientID:           "demo-client",
		ClientName:         "Demo Client",
		ClientType:         "PUBLIC",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"email"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
	}, validSSOClientProof("SSO_CLIENT_CREATE", "unused"))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error, got %v", err)
	}
}

func TestManagedClientFacadeUsesSSOValidationWithoutInteractiveProof(t *testing.T) {
	service := &Service{}
	_, err := service.UpsertManagedClient(context.Background(), ssofacade.ManagedClientCommand{
		ClientID:    "hub-node-order-admin",
		ClientName:  "Hub Node Order Admin",
		RedirectURI: "https://user:pass@node.example.com/callback",
	})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected SSO redirect validation error without step-up, got %v", err)
	}
}

func TestGenericAdminGuardRejectsHubManagedClientNamespaceAndMetadata(t *testing.T) {
	for _, client := range []*domain.Client{
		{ClientID: "hub-node-order-admin", MetadataJSON: `{}`},
		{ClientID: "custom-client", MetadataJSON: `{"managedBy":"hub_control","ownerNodeCode":"order-admin"}`},
	} {
		if err := guardGenericClientMutation(client); err == nil {
			t.Fatalf("generic admin accepted managed client: %#v", client)
		}
	}
	if err := guardGenericClientMutation(&domain.Client{ClientID: "ordinary-client", MetadataJSON: `{"display":"ordinary"}`}); err != nil {
		t.Fatalf("ordinary client rejected: %v", err)
	}
}

func TestManagedClientCreationPersistsAuthoritativeOwnerMetadata(t *testing.T) {
	service, mock := newManagedClientTestService(t)
	expectManagedClientCreate(mock, "hub-node-order-admin", "order-admin", true)

	result, err := service.UpsertManagedClient(context.Background(), managedClientCommand("order-admin", false))
	if err != nil {
		t.Fatalf("UpsertManagedClient() error=%v", err)
	}
	if result.ClientID != "hub-node-order-admin" || result.ClientSecret == "" {
		t.Fatalf("managed result=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestManagedClientRejectsUnmanagedOrForeignOwnershipCollision(t *testing.T) {
	for _, metadata := range []string{`{"display":"admin client"}`, `{"managedBy":"hub_control","ownerNodeCode":"other-node"}`} {
		t.Run(metadata, func(t *testing.T) {
			service, mock := newManagedClientTestService(t)
			mock.ExpectBegin()
			expectManagedClientLookup(mock, "hub-node-order-admin", metadata, 1)
			mock.ExpectRollback()
			_, err := service.UpsertManagedClient(context.Background(), managedClientCommand("order-admin", false))
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeObjectStateInvalid {
				t.Fatalf("expected ownership collision, got %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("sql expectations: %v", err)
			}
		})
	}
}

func TestManagedClientReplayReturnsNoSecretAndRotationDisablesPrevious(t *testing.T) {
	service, mock := newManagedClientTestService(t)
	metadata := managedClientMetadataJSON("order-admin")
	for _, rotate := range []bool{false, true} {
		mock.ExpectBegin()
		expectManagedClientLookup(mock, "hub-node-order-admin", metadata, 1)
		expectManagedClientProfileUpdate(mock, "hub-node-order-admin", metadata)
		expectManagedRedirectReplace(mock, "hub-node-order-admin")
		mock.ExpectQuery("SELECT COUNT\\(\\*\\).*sys_sso_client_secret").WithArgs("hub-node-order-admin", domain.ClientStatusActive, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		if rotate {
			expectManagedSecretInsert(mock, "hub-node-order-admin")
			mock.ExpectExec("(?s)UPDATE sys_sso_client_secret.*id <>").
				WithArgs(domain.ClientStatusDisabled, int64(0), sqlmock.AnyArg(), "hub-node-order-admin", sqlmock.AnyArg(), domain.ClientStatusActive).
				WillReturnResult(sqlmock.NewResult(0, 73))
		}
		mock.ExpectCommit()
		result, err := service.UpsertManagedClient(context.Background(), managedClientCommand("order-admin", rotate))
		if err != nil {
			t.Fatalf("rotate=%v error=%v", rotate, err)
		}
		if rotate == (result.ClientSecret == "") {
			t.Fatalf("rotate=%v secret returned=%v", rotate, result.ClientSecret != "")
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestManagedClientStatusRequiresExactOwnershipAndRevokesDisabledClientSessions(t *testing.T) {
	for _, status := range []int{domain.ClientStatusDisabled, domain.ClientStatusActive} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			service, mock := newManagedClientTestService(t)
			mock.ExpectBegin()
			currentStatus := domain.ClientStatusActive
			if status == domain.ClientStatusActive {
				currentStatus = domain.ClientStatusDisabled
			}
			expectManagedClientLookupWithStatus(mock, "hub-node-order-admin", managedClientMetadataJSON("order-admin"), currentStatus)
			mock.ExpectExec("UPDATE sys_sso_client.*SET status").WithArgs(status, int64(0), sqlmock.AnyArg(), "hub-node-order-admin", status).WillReturnResult(sqlmock.NewResult(0, 1))
			if status == domain.ClientStatusDisabled {
				mock.ExpectExec("UPDATE sys_sso_session").WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), "hub-node-order-admin", sqlmock.AnyArg(), domain.SessionStatusActive).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()

			if err := service.SetManagedClientStatus(context.Background(), ssofacade.ManagedClientStatusCommand{ClientID: "hub-node-order-admin", OwnerNodeCode: "order-admin", Status: status}); err != nil {
				t.Fatalf("SetManagedClientStatus() error=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("sql expectations: %v", err)
			}
		})
	}
}

func TestManagedClientStatusRejectsForeignOwnerAndAllowsMissingClient(t *testing.T) {
	t.Run("foreign owner", func(t *testing.T) {
		service, mock := newManagedClientTestService(t)
		mock.ExpectBegin()
		expectManagedClientLookupWithStatus(mock, "hub-node-order-admin", managedClientMetadataJSON("other-node"), domain.ClientStatusActive)
		mock.ExpectRollback()
		err := service.SetManagedClientStatus(context.Background(), ssofacade.ManagedClientStatusCommand{ClientID: "hub-node-order-admin", OwnerNodeCode: "order-admin", Status: domain.ClientStatusDisabled})
		if apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
			t.Fatalf("foreign owner error=%v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})
	t.Run("missing client", func(t *testing.T) {
		service, mock := newManagedClientTestService(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT c\\.id, c\\.clientId, c\\.clientName, c\\.clientType.*LIMIT 1 FOR UPDATE").WithArgs("hub-node-order-admin").WillReturnRows(managedClientRows())
		mock.ExpectCommit()
		if err := service.SetManagedClientStatus(context.Background(), ssofacade.ManagedClientStatusCommand{ClientID: "hub-node-order-admin", OwnerNodeCode: "order-admin", Status: domain.ClientStatusDisabled}); err != nil {
			t.Fatalf("missing managed client must be a no-op: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})
}

func TestManagedClientOuterTransactionRollbackAllowsReplay(t *testing.T) {
	service, mock := newManagedClientTestService(t)
	expectManagedClientCreate(mock, "hub-node-order-admin", "order-admin", false)
	transactor := service.transactor
	err := transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		result, upsertErr := service.UpsertManagedClient(txCtx, managedClientCommand("order-admin", false))
		if upsertErr != nil || result.ClientSecret == "" {
			t.Fatalf("managed upsert result=%+v err=%v", result, upsertErr)
		}
		return errors.New("injected Hub metadata persistence failure")
	})
	if err == nil {
		t.Fatal("outer transaction must roll back")
	}
	expectManagedClientCreate(mock, "hub-node-order-admin", "order-admin", true)
	if result, replayErr := service.UpsertManagedClient(context.Background(), managedClientCommand("order-admin", false)); replayErr != nil || result.ClientSecret == "" {
		t.Fatalf("replay result=%+v err=%v", result, replayErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func newManagedClientTestService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	db := sqlx.NewDb(rawDB, "sqlmock")
	repository, err := ssoinfra.NewRepository(sqlmockProvider{db: db})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	idGen, err := xid.New(1)
	if err != nil {
		t.Fatalf("new id generator: %v", err)
	}
	service := &Service{repository: repository, password: passwordService, idGen: idGen}
	service.BindTransactor(store.NewSQLXTransactor(db))
	return service, mock
}

func managedClientCommand(owner string, rotate bool) ssofacade.ManagedClientCommand {
	return ssofacade.ManagedClientCommand{ClientID: "hub-node-" + owner, ClientName: "Hub Node Order Admin", RedirectURI: "https://node.example.com/callback", RotateSecret: rotate, OwnerNodeCode: owner}
}

func managedClientMetadataJSON(owner string) string {
	return `{"managedBy":"hub_control","ownerNodeCode":"` + owner + `"}`
}

func expectManagedClientCreate(mock sqlmock.Sqlmock, clientID, owner string, commit bool) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT c\\.id, c\\.clientId, c\\.clientName, c\\.clientType.*LIMIT 1 FOR UPDATE").WithArgs(clientID).WillReturnRows(managedClientRows())
	metadata := managedClientMetadataJSON(owner)
	mock.ExpectExec("INSERT INTO sys_sso_client").WithArgs(
		clientID, "Hub Node Order Admin", "CONFIDENTIAL", "client_secret_basic",
		`["authorization_code","refresh_token"]`, `["email","openid","profile"]`,
		1, 0, 0, 1800, 2592000, domain.ClientStatusActive, metadata, int64(0), int64(0),
	).WillReturnResult(sqlmock.NewResult(1, 1))
	expectManagedRedirectReplace(mock, clientID)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\).*sys_sso_client_secret").WithArgs(clientID, domain.ClientStatusActive, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectManagedSecretInsert(mock, clientID)
	if commit {
		mock.ExpectCommit()
	} else {
		mock.ExpectRollback()
	}
}

func expectManagedClientLookup(mock sqlmock.Sqlmock, clientID, metadata string, activeSecrets int64) {
	expectManagedClientLookupWithStatusAndSecretCount(mock, clientID, metadata, domain.ClientStatusActive, activeSecrets)
}

func expectManagedClientLookupWithStatus(mock sqlmock.Sqlmock, clientID, metadata string, status int) {
	expectManagedClientLookupWithStatusAndSecretCount(mock, clientID, metadata, status, 1)
}

func expectManagedClientLookupWithStatusAndSecretCount(mock sqlmock.Sqlmock, clientID, metadata string, status int, activeSecrets int64) {
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT c\\.id, c\\.clientId, c\\.clientName, c\\.clientType.*LIMIT 1 FOR UPDATE").WithArgs(clientID).WillReturnRows(managedClientRows().AddRow(
		int64(11), clientID, "Existing Client", "CONFIDENTIAL", "client_secret_basic",
		`["authorization_code","refresh_token"]`, `["openid"]`, 1, 0, 0, 1800, 2592000,
		status, metadata, 1, activeSecrets, now, now,
	))
}

func managedClientRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
		"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
		"status", "metadataJson", "activeRedirectCount", "activeSecretCount", "createTime", "updateTime",
	})
}

func expectManagedClientProfileUpdate(mock sqlmock.Sqlmock, clientID, metadata string) {
	mock.ExpectExec("UPDATE sys_sso_client.*SET clientName").WithArgs(
		"Hub Node Order Admin", "CONFIDENTIAL", "client_secret_basic",
		`["authorization_code","refresh_token"]`, `["email","openid","profile"]`,
		1, 0, 0, 1800, 2592000, metadata, int64(0), clientID,
	).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectManagedRedirectReplace(mock sqlmock.Sqlmock, clientID string) {
	mock.ExpectExec("DELETE FROM sys_sso_client_redirect_uri").WithArgs(clientID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE sys_sso_client_redirect_uri").WithArgs(int64(0), sqlmock.AnyArg(), clientID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO sys_sso_client_redirect_uri").WithArgs(clientID, "https://node.example.com/callback", nil, domain.ClientStatusActive, int64(0), sqlmock.AnyArg(), int64(0), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectManagedSecretInsert(mock sqlmock.Sqlmock, clientID string) {
	mock.ExpectExec("INSERT INTO sys_sso_client_secret").WithArgs(sqlmock.AnyArg(), clientID, sqlmock.AnyArg(), sqlmock.AnyArg(), nil, domain.ClientStatusActive, int64(0), int64(0)).WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestUpdateClientRequiresMatchingStepUpProof(t *testing.T) {
	service := &Service{}
	request := validClientAdminUpdateRequest()
	binding, err := BuildClientAdminUpdateOperationBinding("demo-client", request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	_, err = service.UpdateClient(context.Background(), 1001, "demo-client", request, validSSOClientProof("SSO_CLIENT_UPDATE", binding+"-wrong"))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden proof mismatch, got %v", err)
	}
}

func TestUpdateClientRejectsProofForOldPayload(t *testing.T) {
	service := &Service{}
	oldRequest := validClientAdminUpdateRequest()
	oldRequest.ClientName = "Old Name"
	oldBinding, err := BuildClientAdminUpdateOperationBinding("demo-client", oldRequest)
	if err != nil {
		t.Fatalf("build old binding: %v", err)
	}
	newRequest := validClientAdminUpdateRequest()
	newRequest.ClientName = "New Name"
	_, err = service.UpdateClient(context.Background(), 1001, "demo-client", newRequest, validSSOClientProof("SSO_CLIENT_UPDATE", oldBinding))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden old-payload proof, got %v", err)
	}
}

func TestDisableClientRevokesSessionsByDefault(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	now := time.Date(2026, 6, 18, 16, 0, 0, 0, time.UTC)
	request := ssofacade.ClientStatusRequest{Status: domain.ClientStatusDisabled}
	binding, err := BuildClientStatusOperationBinding("demo-client", request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	expectClientDetailLookup(mock, domain.Client{
		ID:                 11,
		ClientID:           "demo-client",
		ClientName:         "Demo Client",
		ClientType:         "PUBLIC",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid", "email"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             domain.ClientStatusActive,
		CreateTime:         now,
		UpdateTime:         now,
	})
	expectListActiveSessionsByClientID(mock, "demo-client")
	mock.ExpectExec("UPDATE sys_sso_client").
		WithArgs(domain.ClientStatusDisabled, int64(1001), sqlmock.AnyArg(), "demo-client", domain.ClientStatusDisabled).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), "demo-client", sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))

	err = service.UpdateClientStatus(context.Background(), 1001, "demo-client", request, validSSOClientProof("SSO_CLIENT_STATUS_CHANGE", binding))
	if err != nil {
		t.Fatalf("UpdateClientStatus() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestValidateRedirectURIRejectsHostConfusion(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		profile string
	}{
		{name: "scheme relative", raw: "//evil.example/callback", profile: "prod"},
		{name: "fragment", raw: "https://internal.example/callback#fragment", profile: "prod"},
		{name: "userinfo", raw: "https://user:pass@internal.example/callback", profile: "prod"},
		{name: "encoded slash host", raw: "https://internal.example%2fevil.example/callback", profile: "prod"},
		{name: "double encoded slash host", raw: "https://internal.example%252fevil.example/callback", profile: "prod"},
		{name: "backslash confusion", raw: "https://internal.example\\evil.example/callback", profile: "prod"},
		{name: "double encoded backslash host", raw: "https://internal.example%255cevil.example/callback", profile: "prod"},
		{name: "crlf", raw: "https://internal.example/%0d%0aLocation:%20https://evil.example", profile: "prod"},
		{name: "prod http", raw: "http://internal.example/callback", profile: "prod"},
		{name: "wildcard", raw: "https://*.internal.example/callback", profile: "prod"},
		{name: "localhost no port", raw: "http://localhost/callback", profile: "dev"},
		{name: "trailing dot host", raw: "https://internal.example./callback", profile: "prod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateRedirectURIForClient(tt.raw, tt.profile); err == nil {
				t.Fatalf("expected %q to be rejected", tt.raw)
			}
		})
	}
	if err := validateRedirectURIForClient("http://127.0.0.1:5177/callback", "dev"); err != nil {
		t.Fatalf("expected dev localhost redirect to pass: %v", err)
	}
	if err := validateRedirectURIForClient("http://node-a.localhost:18080/callback", "test"); err != nil {
		t.Fatalf("expected reserved .localhost redirect to pass in test: %v", err)
	}
	if err := validateRedirectURIForClient("https://internal.example/callback", "prod"); err != nil {
		t.Fatalf("expected https redirect to pass: %v", err)
	}
}

func TestUpdateClientRedirectURIsRequiresMatchingStepUpProof(t *testing.T) {
	service := &Service{}
	request := ssofacade.ClientRedirectURIUpdateRequest{RedirectURIs: []string{"https://demo.example/callback"}}
	binding, err := BuildClientRedirectURIsOperationBinding("demo-client", request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	_, err = service.UpdateClientRedirectURIs(context.Background(), 1001, "demo-client", request, validSSOClientProof("SSO_CLIENT_REDIRECT_EDIT", binding+"-wrong"))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden proof mismatch, got %v", err)
	}
}

func TestNormalizeClientRedirectURIsRejectsOversizedArrays(t *testing.T) {
	redirects := make([]string, 101)
	postLogouts := make([]string, 101)
	for index := 0; index < 101; index++ {
		redirects[index] = fmt.Sprintf("https://client-%03d.example/callback", index)
		postLogouts[index] = fmt.Sprintf("https://client-%03d.example/logout", index)
	}
	if _, err := normalizeClientRedirectURIsForAdmin(ssofacade.ClientRedirectURIUpdateRequest{
		RedirectURIs: redirects,
	}, "prod"); err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("oversized redirectUris err=%v", err)
	}
	if _, err := normalizeClientRedirectURIsForAdmin(ssofacade.ClientRedirectURIUpdateRequest{
		RedirectURIs:           []string{"https://client.example/callback"},
		PostLogoutRedirectURIs: postLogouts,
	}, "prod"); err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("oversized postLogoutRedirectUris err=%v", err)
	}
	if _, err := BuildClientRedirectURIsOperationBinding("demo-client", ssofacade.ClientRedirectURIUpdateRequest{
		RedirectURIs: redirects,
	}); err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("oversized binding input err=%v", err)
	}
}

func TestNormalizeClientRedirectURIsPreservesIndependentSetReadbackDeterministically(t *testing.T) {
	items, err := normalizeClientRedirectURIsForAdmin(ssofacade.ClientRedirectURIUpdateRequest{
		RedirectURIs: []string{
			"https://z.example/callback",
			"https://a.example/callback",
		},
		PostLogoutRedirectURIs: []string{
			"https://z.example/logout",
			"https://a.example/logout",
		},
	}, "prod")
	if err != nil {
		t.Fatalf("normalize redirects: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("normalized rows=%d, want 2 physical rows", len(items))
	}
	if items[0].RedirectURI != "https://a.example/callback" || items[0].PostLogoutRedirectURI != "https://a.example/logout" ||
		items[1].RedirectURI != "https://z.example/callback" || items[1].PostLogoutRedirectURI != "https://z.example/logout" {
		t.Fatalf("normalized rows=%#v", items)
	}
	redirectSet := []string{items[0].RedirectURI, items[1].RedirectURI}
	postLogoutSet := []string{items[0].PostLogoutRedirectURI, items[1].PostLogoutRedirectURI}
	if !reflect.DeepEqual(redirectSet, []string{"https://a.example/callback", "https://z.example/callback"}) ||
		!reflect.DeepEqual(postLogoutSet, []string{"https://a.example/logout", "https://z.example/logout"}) {
		t.Fatalf("readback sets redirect=%v postLogout=%v", redirectSet, postLogoutSet)
	}
}

func TestNormalizeClientRedirectURIsRejectsUnrepresentablePostLogoutCardinalityBeforeSQL(t *testing.T) {
	_, err := normalizeClientRedirectURIsForAdmin(ssofacade.ClientRedirectURIUpdateRequest{
		RedirectURIs: []string{
			"https://a.example/callback",
			"https://b.example/callback",
		},
		PostLogoutRedirectURIs: []string{
			"https://a.example/logout",
			"https://b.example/logout",
			"https://c.example/logout",
		},
	}, "prod")
	if err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("unrepresentable cardinality err=%v", err)
	}
}

func TestGenerateClientSecretReturnsPlaintextOnceAndStoresHash(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	request := ssofacade.ClientSecretGenerateRequest{ExpiresInDays: 30, Reason: "rotate"}
	binding, err := BuildClientSecretGenerateOperationBinding("demo-client", request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	now := time.Now().UTC()
	expectClientDetailLookup(mock, domain.Client{
		ID:                  10,
		ClientID:            "demo-client",
		ClientName:          "Demo",
		ClientType:          "CONFIDENTIAL",
		ClientAuthMethod:    "client_secret_basic",
		GrantTypes:          []string{"authorization_code", "refresh_token"},
		Scopes:              []string{"openid"},
		RequirePKCE:         true,
		AccessTokenTTLSec:   1800,
		RefreshTokenTTLSec:  2592000,
		Status:              domain.ClientStatusActive,
		ActiveSecretCount:   1,
		ActiveRedirectCount: 1,
		CreateTime:          now,
		UpdateTime:          now,
	})
	hashArg := &capturingBcryptHashArg{}
	mock.ExpectExec("INSERT INTO sys_sso_client_secret").
		WithArgs(sqlmock.AnyArg(), "demo-client", hashArg, sqlmock.AnyArg(), sqlmock.AnyArg(), domain.ClientStatusActive, int64(1001), int64(1001)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	response, err := service.GenerateClientSecret(context.Background(), 1001, "demo-client", request, validSSOClientProof("SSO_CLIENT_SECRET_GENERATE", binding))
	if err != nil {
		t.Fatalf("GenerateClientSecret() error = %v", err)
	}
	if !strings.HasPrefix(response.ClientSecret, "sec_live_") {
		t.Fatalf("generated secret prefix = %q", response.ClientSecret)
	}
	if response.SecretHint == "" || strings.Contains(response.SecretHint, response.ClientSecret) {
		t.Fatalf("unsafe secret hint: %q", response.SecretHint)
	}
	if hashArg.value == "" || strings.Contains(hashArg.value, response.ClientSecret) {
		t.Fatalf("stored hash is empty or contains plaintext: %q", hashArg.value)
	}
	if err := service.password.Verify(context.Background(), response.ClientSecret, hashArg.value); err != nil {
		t.Fatalf("stored hash does not verify generated secret: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGenerateClientSecretRejectsPublicClient(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	request := ssofacade.ClientSecretGenerateRequest{Reason: "public"}
	binding, err := BuildClientSecretGenerateOperationBinding("demo-client", request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	now := time.Now().UTC()
	expectClientDetailLookup(mock, domain.Client{
		ID:                 10,
		ClientID:           "demo-client",
		ClientName:         "Demo",
		ClientType:         "PUBLIC",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             domain.ClientStatusActive,
		CreateTime:         now,
		UpdateTime:         now,
	})
	_, err = service.GenerateClientSecret(context.Background(), 1001, "demo-client", request, validSSOClientProof("SSO_CLIENT_SECRET_GENERATE", binding))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error for public client, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDisableLastSecretRejectsForActiveConfidentialClient(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	request := ssofacade.ClientSecretStatusRequest{Reason: "last-secret"}
	binding, err := BuildClientSecretStatusOperationBinding("demo-client", 99, request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	now := time.Now().UTC()
	expectClientDetailLookup(mock, domain.Client{
		ID:                 10,
		ClientID:           "demo-client",
		ClientName:         "Demo",
		ClientType:         "CONFIDENTIAL",
		ClientAuthMethod:   "client_secret_basic",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             domain.ClientStatusActive,
		CreateTime:         now,
		UpdateTime:         now,
	})
	mock.ExpectQuery("SELECT id, clientId, secretHint, expiresAt, status, createTime, updateTime").
		WithArgs("demo-client").
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "secretHint", "expiresAt", "status", "createTime", "updateTime"}).
			AddRow(int64(99), "demo-client", "sec_****abcd", nil, domain.ClientStatusActive, now, now))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)").
		WithArgs("demo-client", domain.ClientStatusActive, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	err = service.DisableClientSecret(context.Background(), 1001, "demo-client", 99, request, validSSOClientProof("SSO_CLIENT_SECRET_DISABLE", binding))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error for last secret, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDisableClientSecretAllowsNoActiveSecretWhenExplicit(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	request := ssofacade.ClientSecretStatusRequest{Reason: "planned outage", AllowNoActiveSecret: true}
	binding, err := BuildClientSecretStatusOperationBinding("demo-client", 99, request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	now := time.Now().UTC()
	expectClientDetailLookup(mock, domain.Client{
		ID:                 10,
		ClientID:           "demo-client",
		ClientName:         "Demo",
		ClientType:         "CONFIDENTIAL",
		ClientAuthMethod:   "client_secret_basic",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             domain.ClientStatusActive,
		CreateTime:         now,
		UpdateTime:         now,
	})
	mock.ExpectQuery("SELECT id, clientId, secretHint, expiresAt, status, createTime, updateTime").
		WithArgs("demo-client").
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "secretHint", "expiresAt", "status", "createTime", "updateTime"}).
			AddRow(int64(99), "demo-client", "sec_****abcd", nil, domain.ClientStatusActive, now, now))
	mock.ExpectExec("UPDATE sys_sso_client_secret").
		WithArgs(domain.ClientStatusDisabled, int64(1001), sqlmock.AnyArg(), int64(99), "demo-client", domain.ClientStatusDisabled).
		WillReturnResult(sqlmock.NewResult(0, 1))
	err = service.DisableClientSecret(context.Background(), 1001, "demo-client", 99, request, validSSOClientProof("SSO_CLIENT_SECRET_DISABLE", binding))
	if err != nil {
		t.Fatalf("DisableClientSecret() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDisableClientSecretIsIdempotentWhenAlreadyDisabled(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	request := ssofacade.ClientSecretStatusRequest{Reason: "already disabled"}
	binding, err := BuildClientSecretStatusOperationBinding("demo-client", 99, request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	now := time.Now().UTC()
	expectClientDetailLookup(mock, domain.Client{
		ID:                 10,
		ClientID:           "demo-client",
		ClientName:         "Demo",
		ClientType:         "CONFIDENTIAL",
		ClientAuthMethod:   "client_secret_basic",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid"},
		RequirePKCE:        true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             domain.ClientStatusActive,
		CreateTime:         now,
		UpdateTime:         now,
	})
	mock.ExpectQuery("SELECT id, clientId, secretHint, expiresAt, status, createTime, updateTime").
		WithArgs("demo-client").
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "secretHint", "expiresAt", "status", "createTime", "updateTime"}).
			AddRow(int64(99), "demo-client", "sec_****abcd", nil, domain.ClientStatusDisabled, now, now).
			AddRow(int64(100), "demo-client", "sec_****efgh", nil, domain.ClientStatusActive, now, now))
	err = service.DisableClientSecret(context.Background(), 1001, "demo-client", 99, request, validSSOClientProof("SSO_CLIENT_SECRET_DISABLE", binding))
	if err != nil {
		t.Fatalf("DisableClientSecret() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDisableClientSecretRequiresMatchingStepUpProof(t *testing.T) {
	service := &Service{}
	request := ssofacade.ClientSecretStatusRequest{Reason: "mismatch"}
	binding, err := BuildClientSecretStatusOperationBinding("demo-client", 99, request)
	if err != nil {
		t.Fatalf("build binding: %v", err)
	}
	err = service.DisableClientSecret(context.Background(), 1001, "demo-client", 99, request, validSSOClientProof("SSO_CLIENT_SECRET_DISABLE", binding+"-wrong"))
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden proof mismatch, got %v", err)
	}
}

func TestDisabledClientTokenRefreshRevokeIntrospectFailClosed(t *testing.T) {
	service, mock := newTokenValidationTestService(t)
	disabled := domain.Client{
		ID:               10,
		ClientID:         "disabled-client",
		ClientName:       "Disabled",
		ClientType:       "PUBLIC",
		ClientAuthMethod: "none",
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		Scopes:           []string{"openid", "offline_access"},
		RequirePKCE:      true,
		Status:           domain.ClientStatusDisabled,
	}
	for i := 0; i < 3; i++ {
		expectClientLookup(mock, disabled)
	}
	if _, err := service.ExchangeRefreshToken(context.Background(), "disabled-client", "", "refresh-token"); err == nil {
		t.Fatal("expected refresh token exchange to fail for disabled client")
	}
	if err := service.RevokeTokenForClient(context.Background(), "disabled-client", "", "token", "access_token"); err == nil {
		t.Fatal("expected revoke to fail for disabled client")
	}
	if _, err := service.IntrospectTokenForClient(context.Background(), "disabled-client", "", "token", "access_token"); err == nil {
		t.Fatal("expected introspect to fail for disabled client")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func validClientAdminSaveRequest() ssofacade.ClientAdminSaveRequest {
	return ssofacade.ClientAdminSaveRequest{
		ClientID:           "demo-client",
		ClientName:         "Demo Client",
		ClientType:         "PUBLIC",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"refresh_token", "authorization_code"},
		Scopes:             []string{"email", "openid"},
		RequirePKCE:        true,
		TrustedFirstParty:  true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
	}
}

func validClientAdminUpdateRequest() ssofacade.UpdateClientAdminRequest {
	request := validClientAdminSaveRequest()
	return ssofacade.UpdateClientAdminRequest{
		ClientName:         request.ClientName,
		ClientType:         request.ClientType,
		ClientAuthMethod:   request.ClientAuthMethod,
		GrantTypes:         request.GrantTypes,
		Scopes:             request.Scopes,
		RequirePKCE:        request.RequirePKCE,
		RequireConsent:     request.RequireConsent,
		TrustedFirstParty:  request.TrustedFirstParty,
		AccessTokenTTLSec:  request.AccessTokenTTLSec,
		RefreshTokenTTLSec: request.RefreshTokenTTLSec,
		MetadataJSON:       request.MetadataJSON,
	}
}

func validSSOClientProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        action,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
}

func expectClientDetailLookup(mock sqlmock.Sqlmock, client domain.Client) {
	mock.ExpectQuery("SELECT c\\.id, c\\.clientId, c\\.clientName, c\\.clientType").
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
			mustJSON(client.GrantTypes),
			mustJSON(client.Scopes),
			boolToIntForTest(client.RequirePKCE),
			boolToIntForTest(client.RequireConsent),
			boolToIntForTest(client.TrustedFirstParty),
			client.AccessTokenTTLSec,
			client.RefreshTokenTTLSec,
			client.Status,
			nullableStringValue(client.MetadataJSON),
			client.ActiveRedirectCount,
			client.ActiveSecretCount,
			client.CreateTime,
			client.UpdateTime,
		))
}

func boolToIntForTest(value bool) int {
	if value {
		return 1
	}
	return 0
}

func expectActiveSessionLookup(mock sqlmock.Sqlmock, sessionID string, session domain.Session) {
	mock.ExpectQuery("SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson").
		WithArgs(sessionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson",
			"loginMethod", "externalProviderCode", "externalIdentityId",
			"loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}).AddRow(
			session.ID,
			session.SessionID,
			session.UserID,
			session.ClientID,
			nullableStringValue(session.PlatformCode),
			nullableStringValue(session.DeviceID),
			nullableStringValue(session.LoginIP),
			nullableStringValue(session.UserAgent),
			nullableStringValue(session.ACR),
			mustJSON(session.AMR),
			nullableStringValue(session.LoginMethod),
			nullableStringValue(session.ExternalProviderCode),
			nullableInt64Value(session.ExternalIdentityID),
			session.LoginAt,
			nil,
			session.ExpiresAt,
			nil,
			session.Status,
			nullableStringValue(session.MetadataJSON),
			session.LoginAt,
			session.LoginAt,
		))
}

func expectListSessionsByUserID(mock sqlmock.Sqlmock, userID int64, sessions ...domain.Session) {
	expectSessionRevocationCutoff(mock)
	rows := newSessionRows(sessions...)
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs(userID, domain.SessionStatusActive, sqlmock.AnyArg(), int64(0), sessionRevocationPageSize).
		WillReturnRows(rows)
}

func expectListActiveSessionsByExternalProvider(mock sqlmock.Sqlmock, providerCode string, sessions ...domain.Session) {
	expectSessionRevocationCutoff(mock)
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs(providerCode, domain.SessionStatusActive, sqlmock.AnyArg(), int64(0), sessionRevocationPageSize).
		WillReturnRows(newSessionRows(sessions...))
}

func expectListActiveSessionsByPlatformCode(mock sqlmock.Sqlmock, platformCode string, sessions ...domain.Session) {
	expectSessionRevocationCutoff(mock)
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs(platformCode, domain.SessionStatusActive, sqlmock.AnyArg(), int64(0), sessionRevocationPageSize).
		WillReturnRows(newSessionRows(sessions...))
}

func expectListActiveSessionsByPlatformLoginMethod(mock sqlmock.Sqlmock, platformCode, loginMethod, providerCode string, sessions ...domain.Session) {
	expectSessionRevocationCutoff(mock)
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs(platformCode, loginMethod, providerCode, domain.SessionStatusActive, sqlmock.AnyArg(), int64(0), sessionRevocationPageSize).
		WillReturnRows(newSessionRows(sessions...))
}

func expectListActiveSessionsByExternalIdentity(mock sqlmock.Sqlmock, identityID int64, sessions ...domain.Session) {
	expectSessionRevocationCutoff(mock)
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs(identityID, domain.SessionStatusActive, sqlmock.AnyArg(), int64(0), sessionRevocationPageSize).
		WillReturnRows(newSessionRows(sessions...))
}

func expectListActiveSessionsByClientID(mock sqlmock.Sqlmock, clientID string, sessions ...domain.Session) {
	expectSessionRevocationCutoff(mock)
	mock.ExpectQuery("(?s)SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson.*id >.*ORDER BY id ASC.*LIMIT").
		WithArgs(clientID, domain.SessionStatusActive, sqlmock.AnyArg(), int64(0), sessionRevocationPageSize).
		WillReturnRows(newSessionRows(sessions...))
}

func expectSessionRevocationCutoff(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT CURRENT_TIMESTAMP").
		WillReturnRows(sqlmock.NewRows([]string{"cutoff"}).AddRow(time.Now().UTC()))
}

func newSessionRows(sessions ...domain.Session) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson",
		"loginMethod", "externalProviderCode", "externalIdentityId",
		"loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
	})
	for _, session := range sessions {
		rows.AddRow(
			session.ID,
			session.SessionID,
			session.UserID,
			session.ClientID,
			nullableStringValue(session.PlatformCode),
			nullableStringValue(session.DeviceID),
			nullableStringValue(session.LoginIP),
			nullableStringValue(session.UserAgent),
			nullableStringValue(session.ACR),
			mustJSON(session.AMR),
			nullableStringValue(session.LoginMethod),
			nullableStringValue(session.ExternalProviderCode),
			nullableInt64Value(session.ExternalIdentityID),
			session.LoginAt,
			session.LastAccessAt,
			session.ExpiresAt,
			session.RevokedAt,
			session.Status,
			nullableStringValue(session.MetadataJSON),
			session.CreateTime,
			session.UpdateTime,
		)
	}
	return rows
}

func expectTouchSession(mock sqlmock.Sqlmock, sessionID string) {
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sessionID, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectClientLookup(mock sqlmock.Sqlmock, client domain.Client) {
	mock.ExpectQuery("SELECT id, clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson").
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
			mustJSON(client.GrantTypes),
			mustJSON(client.Scopes),
			0,
			0,
			0,
			300,
			3600,
			client.Status,
			nil,
		))
	redirectRows := sqlmock.NewRows([]string{"redirectUri", "postLogoutRedirectUri"})
	maxRedirectRows := maxInt(len(client.RedirectURIs), len(client.PostLogoutRedirects))
	for i := 0; i < maxRedirectRows; i++ {
		var redirectURI any
		if i < len(client.RedirectURIs) {
			redirectURI = client.RedirectURIs[i]
		}
		var postLogoutRedirect any
		if i < len(client.PostLogoutRedirects) {
			postLogoutRedirect = client.PostLogoutRedirects[i]
		}
		redirectRows.AddRow(redirectURI, postLogoutRedirect)
	}
	mock.ExpectQuery("SELECT redirectUri, postLogoutRedirectUri").
		WithArgs(client.ClientID).
		WillReturnRows(redirectRows)
	mock.ExpectQuery("SELECT secretHash").
		WithArgs(client.ClientID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"secretHash"}))
}

func expectMissingClientLookup(mock sqlmock.Sqlmock, clientID string) {
	mock.ExpectQuery("SELECT id, clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson").
		WithArgs(clientID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
			"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
			"status", "metadataJson",
		}))
}

func expectAuthorizationCodeLookup(mock sqlmock.Sqlmock, code domain.AuthorizationCode) {
	mock.ExpectQuery("SELECT id, code, clientId, userId, sessionId, redirectUri, scopesJson, codeChallenge, codeChallengeMethod").
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
			mustJSON(code.Scopes),
			nullableStringValue(code.CodeChallenge),
			nullableStringValue(code.CodeChallengeMethod),
			nullableStringValue(code.Nonce),
			nullableStringValue(code.ACR),
			nil,
			code.ExpiresAt,
			nil,
			code.Status,
			nil,
			code.CreateTime,
			code.UpdateTime,
		))
}

func expectMissingAuthorizationCodeLookup(mock sqlmock.Sqlmock, code string) {
	mock.ExpectQuery("SELECT id, code, clientId, userId, sessionId, redirectUri, scopesJson, codeChallenge, codeChallengeMethod").
		WithArgs(code).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "code", "clientId", "userId", "sessionId", "redirectUri", "scopesJson", "codeChallenge", "codeChallengeMethod",
			"nonce", "acr", "amrJson", "expiresAt", "consumedAt", "status", "metadataJson", "createTime", "updateTime",
		}))
}

func nullableStringValue(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableInt64Value(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func expectConsumeAuthorizationCode(mock sqlmock.Sqlmock, code string) {
	expectConsumeAuthorizationCodeResult(mock, code, 1)
}

func expectConsumeAuthorizationCodeResult(mock sqlmock.Sqlmock, code string, rowsAffected int64) {
	mock.ExpectExec("UPDATE sys_sso_authorization_code").
		WithArgs(sqlmock.AnyArg(), domain.CodeStatusConsumed, sqlmock.AnyArg(), code, domain.CodeStatusActive).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
}

func expectInsertRefreshFamily(mock sqlmock.Sqlmock) {
	mock.ExpectExec("INSERT INTO sys_sso_refresh_token_family").
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

func expectRefreshFamilyByCurrentHash(mock sqlmock.Sqlmock, hash string, family domain.RefreshTokenFamily) {
	mock.ExpectQuery("SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected").
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
			0,
			nil,
			family.ExpiresAt,
			nil,
			family.Status,
			family.MetadataJSON,
			family.CreateTime,
			family.UpdateTime,
		))
}

func expectRotateRefreshFamily(mock sqlmock.Sqlmock, familyID string, previousHash string, affectedRows int64) {
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(previousHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), familyID, previousHash, domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func expectMissingRefreshFamilyByCurrentHash(mock sqlmock.Sqlmock, hash string) {
	mock.ExpectQuery("SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected").
		WithArgs(hash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "familyId", "sessionId", "clientId", "userId", "currentTokenHash", "previousTokenHash",
			"reuseDetected", "rotatedAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}))
}

func expectRefreshFamilyByPreviousHash(mock sqlmock.Sqlmock, hash string, family domain.RefreshTokenFamily) {
	previousHash := family.PreviousTokenHash
	if previousHash == "" {
		previousHash = hash
	}
	mock.ExpectQuery("SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected").
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

func expectMissingRefreshFamilyByPreviousHash(mock sqlmock.Sqlmock, hash string) {
	mock.ExpectQuery("SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected").
		WithArgs(hash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "familyId", "sessionId", "clientId", "userId", "currentTokenHash", "previousTokenHash",
			"reuseDetected", "rotatedAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
		}))
}

func expectMarkRefreshFamilyReuseDetected(mock sqlmock.Sqlmock, familyID string) {
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), familyID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectRevokeRefreshFamiliesBySessionID(mock sqlmock.Sqlmock, sessionID string) {
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), sessionID, domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectRevokeRefreshFamiliesByExternalProvider(mock sqlmock.Sqlmock, providerCode string, affectedRows int64) {
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), domain.RefreshFamilyStatusActive, providerCode, sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func expectRevokeRefreshFamiliesByPlatformCode(mock sqlmock.Sqlmock, platformCode string, affectedRows int64) {
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), domain.RefreshFamilyStatusActive, platformCode, sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func expectRevokeRefreshFamiliesByPlatformLoginMethod(mock sqlmock.Sqlmock, platformCode, loginMethod, providerCode string, affectedRows int64) {
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), domain.RefreshFamilyStatusActive, platformCode, loginMethod, providerCode, sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func expectRevokeRefreshFamiliesByExternalIdentity(mock sqlmock.Sqlmock, identityID int64, affectedRows int64) {
	mock.ExpectExec("UPDATE sys_sso_refresh_token_family").
		WithArgs(sqlmock.AnyArg(), domain.RefreshFamilyStatusRevoked, sqlmock.AnyArg(), domain.RefreshFamilyStatusActive, identityID, sqlmock.AnyArg(), domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, affectedRows))
}

func expectRevokeSession(mock sqlmock.Sqlmock, sessionID string) {
	mock.ExpectExec("UPDATE sys_sso_session").
		WithArgs(sqlmock.AnyArg(), domain.SessionStatusRevoked, sqlmock.AnyArg(), sessionID, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectSSOAuditLog(mock sqlmock.Sqlmock, eventType, clientID, result, reasonCode string, detailFragments ...string) {
	expectSSOAuditLogWithTrace(mock, eventType, clientID, result, reasonCode, "", detailFragments...)
}

func expectSSOAuditLogWithTrace(mock sqlmock.Sqlmock, eventType, clientID, result, reasonCode, traceID string, detailFragments ...string) {
	expectSSOAuditLogWithSubjectAndTrace(mock, eventType, clientID, sqlmock.AnyArg(), sqlmock.AnyArg(), result, reasonCode, traceID, detailFragments...)
}

func expectSSOAuditLogWithoutSubject(mock sqlmock.Sqlmock, eventType, clientID, result, reasonCode string, detailFragments ...string) {
	expectSSOAuditLogWithSubjectAndTrace(mock, eventType, clientID, nil, nil, result, reasonCode, "", detailFragments...)
}

func expectSSOAuditLogWithoutClient(mock sqlmock.Sqlmock, eventType string, userID int64, result, reasonCode string, detailFragments ...string) {
	expectSSOAuditLogWithSubjectAndTrace(mock, eventType, nil, userID, nil, result, reasonCode, "", detailFragments...)
}

func expectSSOAuditLogWithSubjectAndTrace(mock sqlmock.Sqlmock, eventType string, clientIDArg driver.Value, userIDArg driver.Value, sessionIDArg driver.Value, result, reasonCode, traceID string, detailFragments ...string) {
	var traceArg driver.Value = sqlmock.AnyArg()
	if strings.TrimSpace(traceID) != "" {
		traceArg = traceID
	}
	mock.ExpectExec("INSERT INTO sys_sso_audit_log").
		WithArgs(
			eventType,
			clientIDArg,
			userIDArg,
			sessionIDArg,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			result,
			reasonCode,
			auditDetailMatcher{fragments: detailFragments},
			traceArg,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

type auditDetailMatcher struct {
	fragments []string
}

func (m auditDetailMatcher) Match(value driver.Value) bool {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	default:
		return false
	}
	if strings.Contains(text, "access-token") || strings.Contains(text, "refresh-token") || strings.Contains(text, "plain-secret") ||
		strings.Contains(text, "raw-code") || strings.Contains(text, "raw-code-verifier") {
		return false
	}
	for _, fragment := range m.fragments {
		if !strings.Contains(text, fragment) {
			return false
		}
	}
	return true
}

type jsonArrayContainsMatcher struct {
	items []string
}

func (m jsonArrayContainsMatcher) Match(value driver.Value) bool {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	default:
		return false
	}
	var values []string
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return false
	}
	set := make(map[string]struct{}, len(values))
	for _, item := range values {
		set[item] = struct{}{}
	}
	for _, item := range m.items {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

type jsonArrayExactMatcher struct {
	items []string
}

func (m jsonArrayExactMatcher) Match(value driver.Value) bool {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	default:
		return false
	}
	var values []string
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return false
	}
	if len(values) != len(m.items) {
		return false
	}
	expected := append([]string(nil), m.items...)
	sort.Strings(expected)
	sort.Strings(values)
	for i := range expected {
		if values[i] != expected[i] {
			return false
		}
	}
	return true
}

type capturingBcryptHashArg struct {
	value string
}

func (m *capturingBcryptHashArg) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	if !strings.HasPrefix(raw, "$2") || strings.Contains(raw, "sec_live_") {
		return false
	}
	m.value = raw
	return true
}

func cloneClaims(claims map[string]any) map[string]any {
	copied := make(map[string]any, len(claims))
	for key, value := range claims {
		copied[key] = value
	}
	return copied
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func newSSOTestCacheManager() cache.Manager {
	return newSSOTestCacheManagerWithLayer(&ssoTestCacheLayer{values: map[string][]byte{}})
}

func newSSOTestCacheManagerWithLayer(layer *ssoTestCacheLayer) cache.Manager {
	return cache.NewManager(
		"sso-test",
		key.NewBuilder("sso-test"),
		cache.WithKVLayer("memory", layer),
		cache.WithPrimitiveLayer("memory", layer),
	)
}

type ssoTestCacheLayer struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (l *ssoTestCacheLayer) Get(_ context.Context, cacheKey string, dest any) (bool, error) {
	l.mu.Lock()
	raw, ok := l.values[cacheKey]
	l.mu.Unlock()
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (l *ssoTestCacheLayer) GetString(_ context.Context, cacheKey string) (string, bool, error) {
	l.mu.Lock()
	raw, ok := l.values[cacheKey]
	l.mu.Unlock()
	if !ok {
		return "", false, nil
	}
	return string(raw), true, nil
}

func (l *ssoTestCacheLayer) GetBytes(_ context.Context, cacheKey string) ([]byte, bool, error) {
	l.mu.Lock()
	raw, ok := l.values[cacheKey]
	l.mu.Unlock()
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), raw...), true, nil
}

func (l *ssoTestCacheLayer) Set(_ context.Context, cacheKey string, value any, _ time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.values[cacheKey] = raw
	l.mu.Unlock()
	return nil
}

func (l *ssoTestCacheLayer) SetString(_ context.Context, cacheKey string, value string, _ time.Duration) error {
	l.mu.Lock()
	l.values[cacheKey] = []byte(value)
	l.mu.Unlock()
	return nil
}

func (l *ssoTestCacheLayer) SetMaxTimestamp(_ context.Context, cacheKey string, value time.Time, _ time.Duration) (bool, error) {
	encoded := value.UTC().Format("2006-01-02T15:04:05.000000000Z")
	l.mu.Lock()
	defer l.mu.Unlock()
	if current, ok := l.values[cacheKey]; ok && string(current) >= encoded {
		return false, nil
	}
	l.values[cacheKey] = []byte(encoded)
	return true, nil
}

func (l *ssoTestCacheLayer) SetBytes(_ context.Context, cacheKey string, value []byte, _ time.Duration) error {
	l.mu.Lock()
	l.values[cacheKey] = append([]byte(nil), value...)
	l.mu.Unlock()
	return nil
}

func (l *ssoTestCacheLayer) Delete(_ context.Context, cacheKey string) error {
	l.mu.Lock()
	delete(l.values, cacheKey)
	l.mu.Unlock()
	return nil
}

func (l *ssoTestCacheLayer) Exists(_ context.Context, cacheKey string) (bool, error) {
	l.mu.Lock()
	_, ok := l.values[cacheKey]
	l.mu.Unlock()
	return ok, nil
}

func (l *ssoTestCacheLayer) hasKeyContaining(fragment string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for cacheKey := range l.values {
		if strings.Contains(cacheKey, fragment) {
			return true
		}
	}
	return false
}

func (l *ssoTestCacheLayer) Expire(context.Context, string, time.Duration) error {
	return nil
}

func (l *ssoTestCacheLayer) GetDel(ctx context.Context, cacheKey string, dest any) (bool, error) {
	hit, err := l.Get(ctx, cacheKey, dest)
	if err != nil || !hit {
		return hit, err
	}
	return true, l.Delete(ctx, cacheKey)
}

func (l *ssoTestCacheLayer) GetDelString(ctx context.Context, cacheKey string) (string, bool, error) {
	value, hit, err := l.GetString(ctx, cacheKey)
	if err != nil || !hit {
		return value, hit, err
	}
	return value, true, l.Delete(ctx, cacheKey)
}

func (l *ssoTestCacheLayer) CompareAndDelete(context.Context, string, any) (bool, error) {
	return false, nil
}

func (l *ssoTestCacheLayer) CompareAndDeleteString(ctx context.Context, cacheKey string, expected string) (bool, error) {
	value, hit, err := l.GetString(ctx, cacheKey)
	if err != nil || !hit || value != expected {
		return false, err
	}
	return true, l.Delete(ctx, cacheKey)
}

func (l *ssoTestCacheLayer) SetNX(ctx context.Context, cacheKey string, value any, ttl time.Duration) (bool, error) {
	l.mu.Lock()
	_, exists := l.values[cacheKey]
	l.mu.Unlock()
	if exists {
		return false, nil
	}
	return true, l.Set(ctx, cacheKey, value, ttl)
}

func (l *ssoTestCacheLayer) SetNXString(ctx context.Context, cacheKey string, value string, ttl time.Duration) (bool, error) {
	l.mu.Lock()
	_, exists := l.values[cacheKey]
	l.mu.Unlock()
	if exists {
		return false, nil
	}
	return true, l.SetString(ctx, cacheKey, value, ttl)
}

func (l *ssoTestCacheLayer) SetNXBytes(ctx context.Context, cacheKey string, value []byte, ttl time.Duration) (bool, error) {
	l.mu.Lock()
	_, exists := l.values[cacheKey]
	l.mu.Unlock()
	if exists {
		return false, nil
	}
	return true, l.SetBytes(ctx, cacheKey, value, ttl)
}

func (l *ssoTestCacheLayer) Incr(ctx context.Context, cacheKey string, ttl time.Duration) (int64, error) {
	raw, hit, err := l.GetString(ctx, cacheKey)
	if err != nil {
		return 0, err
	}
	var next int64 = 1
	if hit {
		parsed, _ := strconv.ParseInt(raw, 10, 64)
		next = parsed + 1
	}
	return next, l.SetString(ctx, cacheKey, strconv.FormatInt(next, 10), ttl)
}

func (l *ssoTestCacheLayer) DeleteMany(ctx context.Context, cacheKeys ...string) error {
	for _, cacheKey := range cacheKeys {
		if err := l.Delete(ctx, cacheKey); err != nil {
			return err
		}
	}
	return nil
}

func TestScopeSliceParsesSpaceDelimitedScopeClaim(t *testing.T) {
	got := scopeSlice("openid profile email")
	want := []string{"openid", "profile", "email"}
	if len(got) != len(want) {
		t.Fatalf("scopeSlice() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopeSlice()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
