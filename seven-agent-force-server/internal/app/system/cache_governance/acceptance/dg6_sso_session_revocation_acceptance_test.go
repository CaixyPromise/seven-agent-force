package acceptance

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authapp "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/application"
	authdomain "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authinfra "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/infrastructure"
	ssoapp "github.com/CaixyPromise/seven-agent/force/internal/app/sso/application"
	ssodomain "github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	ssoinfra "github.com/CaixyPromise/seven-agent/force/internal/app/sso/infrastructure"
	cachegovapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/application"
	cachegovinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/infrastructure"
	userapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/application"
	userdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	userinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/infrastructure"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

// TestDG6UpdateLockStateRevokesRealSSOBearerAndCookieSession proves the
// security-critical user-status path against each guarded governance
// database. The test uses a freshly generated test-only RSA key and a
// BootstrapFirstPartySession-created session/refresh-family; it neither reads
// local SSO keys nor sends a token, cookie, or DSN to logs.
func TestDG6UpdateLockStateRevokesRealSSOBearerAndCookieSession(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply after the isolated migration path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)

	fixture := dg6SeedAuthorizationFixture(t, ctx, target)
	clientID := fmt.Sprintf("dg6-sso-%d", fixture.userID)
	t.Cleanup(func() {
		dg5ResetAcceptanceOutbox(t, context.Background(), target)
		dg6DeleteSSOFixture(t, context.Background(), target, fixture.userID, clientID)
		dg6DeleteAuthorizationFixture(t, context.Background(), target, fixture)
	})

	users, err := userinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatalf("create isolated user repository: %v", err)
	}
	userService := userapp.NewService(users, userdomain.NewService(), nil, nil, userapp.WithTransactor(target.provider.Transactor()))
	authorizationRepo, err := authinfra.NewRepository(target.provider, userService)
	if err != nil {
		t.Fatalf("create isolated authorization repository: %v", err)
	}
	ids, err := xid.New(81)
	if err != nil {
		t.Fatalf("create isolated identifier generator: %v", err)
	}
	ssoRepository, err := ssoinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatalf("create isolated SSO repository: %v", err)
	}
	prefix := fmt.Sprintf("seven-dg62-sso-%s-%d", target.dialect, time.Now().UTC().UnixNano())
	manager, governed, redisProvider := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "sso")
	t.Cleanup(func() { _ = redisProvider.Close() })
	targeted, ok := manager.(cacheinfra.TargetedGovernedCache)
	if !ok {
		t.Fatal("DG6.2 SSO acceptance manager is missing targeted cache")
	}
	outbox := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), ids.NextID)
	targeted.SetTargetFreshnessGate(outbox)
	governed.SetFanoutHealthy(true)
	rabbit, err := rabbitinfra.New(dg5AcceptanceRabbitConfig(target.cfg))
	if err != nil {
		t.Fatalf("connect local RabbitMQ for DG6.2 SSO acceptance: %v", err)
	}
	t.Cleanup(func() { _ = rabbit.Close() })
	generation := cachegovinfra.NewGenerationAdapter(governed)
	broker, err := cachegovinfra.NewFanoutAdapter(rabbit, generation, prefix+"-fanout", true)
	if err != nil {
		t.Fatal(err)
	}
	registrar := cachegovapp.NewTargetedService(outbox, generation, broker, outbox, "dg62-sso-relay-"+target.dialect)
	if !registrar.Enabled() {
		t.Fatal("DG6.2 SSO targeted registrar disabled")
	}
	dg6InsertFirstPartyClient(t, ctx, ssoRepository, clientID)
	ssoService := ssoapp.NewService(
		dg6SSOTestConfig(clientID),
		ids,
		ssoRepository,
		ssoinfra.NewAuthSessionCache(nil),
		dg6TestJWTService(t),
		nil,
		userService,
		userService,
	)
	ssoService.BindTransactor(target.provider.Transactor())
	ssoService.BindActiveSessionValidityInvalidations(registrar)
	ssoService.BindActiveSessionValidityCache(ssoinfra.NewActiveSessionValidityCache(targeted))
	authorizationService := authapp.NewService(
		target.cfg.Authorization,
		nil,
		target.provider.Transactor(),
		authorizationRepo,
		authdomain.NewService(),
		ids,
		ssoService,
		ssoService,
		nil,
		nil,
	)
	userService.BindRoleAssignments(authorizationService)
	userService.BindSessions(ssoService)

	bootstrapped, err := ssoService.BootstrapFirstPartySession(ctx, ssofacade.BootstrapSessionCommand{
		UserID:       fixture.userID,
		ClientID:     clientID,
		PlatformCode: "DG6",
	})
	if err != nil {
		t.Fatalf("bootstrap isolated first-party session: %v", err)
	}
	if bootstrapped == nil || strings.TrimSpace(bootstrapped.AccessToken) == "" || strings.TrimSpace(bootstrapped.SessionCookieHeaderValue) == "" {
		t.Fatal("bootstrap must return an access token and an HttpOnly session cookie")
	}
	sessionID := dg6CookieSessionID(t, bootstrapped.SessionCookieHeaderValue, "dg6_session")
	secondSession, err := ssoService.BootstrapFirstPartySession(ctx, ssofacade.BootstrapSessionCommand{
		UserID:       fixture.userID,
		ClientID:     clientID,
		PlatformCode: "DG6",
	})
	if err != nil || secondSession == nil || strings.TrimSpace(secondSession.AccessToken) == "" {
		t.Fatalf("bootstrap second isolated first-party session: result=%v err=%v", secondSession != nil, err)
	}

	if contextBeforeLock, accessErr := authorizationService.BuildContextFromAccessToken(ctx, bootstrapped.AccessToken, "dg6-bearer"); accessErr != nil || contextBeforeLock == nil || contextBeforeLock.UserID != fixture.userID {
		t.Fatalf("pre-lock bearer authorization context user=%d err=%v", dg6ContextUserID(contextBeforeLock), accessErr)
	}
	if contextBeforeLock, cookieErr := authorizationService.BuildContextFromSession(ctx, sessionID, "dg6-cookie"); cookieErr != nil || contextBeforeLock == nil || contextBeforeLock.UserID != fixture.userID {
		t.Fatalf("pre-lock cookie authorization context user=%d err=%v", dg6ContextUserID(contextBeforeLock), cookieErr)
	}

	if err := userService.UpdateLockState(ctx, userfacade.UpdateLockStateCommand{
		UserID:     fixture.userID,
		Status:     userdomain.UserStatusDisabled,
		UnsealTime: nil,
	}); err != nil {
		t.Fatalf("lock user and revoke real SSO sessions: %v", err)
	}
	dg62AssertPendingTargetEvents(t, ctx, target, 2)
	dg6AssertSSOSessionAndRefreshRevoked(t, ctx, target, fixture.userID, 2)
	if _, err := authorizationService.BuildContextFromAccessToken(ctx, bootstrapped.AccessToken, "dg6-bearer"); err == nil {
		t.Fatal("locked user bearer was still accepted after committed session revocation")
	}
	if _, err := authorizationService.BuildContextFromSession(ctx, sessionID, "dg6-cookie"); err == nil {
		t.Fatal("locked user session cookie was still accepted after committed session revocation")
	}
}

func dg6SSOTestConfig(clientID string) config.SSOConfig {
	return config.SSOConfig{
		Issuer:                    "https://dg6.test.invalid/sso",
		DefaultFirstPartyClientID: clientID,
		SessionIdleTimeoutSeconds: 1800,
		SessionCookie:             config.SSOCookieConfig{Name: "dg6_session", Path: "/"},
		RefreshCookie:             config.SSORefreshCookieConfig{Name: "dg6_refresh", Path: "/", HTTPOnly: true},
	}
}

func dg6TestJWTService(t *testing.T) *jwtinfra.Service {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate DG6 test RSA key: %v", err)
	}
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "dg6-test-private.pem")
	publicPath := filepath.Join(dir, "dg6-test-public.pem")
	dg6WritePEM(t, privatePath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey))
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal DG6 test public key: %v", err)
	}
	dg6WritePEM(t, publicPath, "PUBLIC KEY", publicDER)
	keys, err := keyring.NewLocalProvider(config.KeysConfig{
		Provider: "local",
		JWT: config.JWTKeysConfig{
			Algorithm: "RS256",
			Active: config.JWTKeySourceConfig{
				KID:              "dg6-test-active",
				PrivateKeySource: "file:" + privatePath,
				PublicKeySource:  "file:" + publicPath,
			},
		},
	})
	if err != nil {
		t.Fatalf("create DG6 test signing provider: %v", err)
	}
	service, err := jwtinfra.New(keys, "RS256")
	if err != nil {
		t.Fatalf("create DG6 test jwt service: %v", err)
	}
	return service
}

func dg6WritePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("write ephemeral DG6 test key: %v", err)
	}
}

func dg6InsertFirstPartyClient(t *testing.T, ctx context.Context, repository *ssoinfra.Repository, clientID string) {
	t.Helper()
	if err := repository.InsertClient(ctx, &ssodomain.Client{
		ClientID:           clientID,
		ClientName:         "DG6 Isolated SSO Acceptance",
		ClientType:         "PUBLIC",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid", "profile", "email", "offline_access"},
		TrustedFirstParty:  true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             ssodomain.ClientStatusActive,
	}, 0); err != nil {
		t.Fatalf("insert isolated first-party SSO client: %v", err)
	}
}

func dg6CookieSessionID(t *testing.T, headerValue, expectedName string) string {
	t.Helper()
	first, _, _ := strings.Cut(strings.TrimSpace(headerValue), ";")
	name, value, found := strings.Cut(first, "=")
	if !found || strings.TrimSpace(name) != expectedName || strings.TrimSpace(value) == "" {
		t.Fatal("bootstrap did not return the expected isolated session cookie")
	}
	return strings.TrimSpace(value)
}

func dg6ContextUserID(value *securitycontext.UserContext) int64 {
	if value == nil {
		return 0
	}
	return value.UserID
}

func dg6AssertSSOSessionAndRefreshRevoked(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, userID int64, want int) {
	t.Helper()
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	sessionQuery := `SELECT COUNT(*) FROM sys_sso_session WHERE userId=? AND status=1 AND revokedAt IS NOT NULL AND isDeleted=0`
	refreshQuery := `SELECT COUNT(*) FROM sys_sso_refresh_token_family WHERE userId=? AND status=1 AND revokedAt IS NOT NULL AND isDeleted=0`
	if target.dialect == "postgres" {
		sessionQuery = `SELECT COUNT(*) FROM sys_sso_session WHERE "userId"=? AND status=1 AND "revokedAt" IS NOT NULL AND "isDeleted"=0`
		refreshQuery = `SELECT COUNT(*) FROM sys_sso_refresh_token_family WHERE "userId"=? AND status=1 AND "revokedAt" IS NOT NULL AND "isDeleted"=0`
	}
	var sessions, refreshFamilies int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(sessionQuery), userID).Scan(&sessions); err != nil {
		t.Fatalf("read isolated SSO session revocation: %v", err)
	}
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(refreshQuery), userID).Scan(&refreshFamilies); err != nil {
		t.Fatalf("read isolated SSO refresh-family revocation: %v", err)
	}
	if sessions != want || refreshFamilies != want {
		t.Fatalf("lock must revoke every bootstrap session/family: sessions=%d refreshFamilies=%d want=%d", sessions, refreshFamilies, want)
	}
}

func dg6DeleteSSOFixture(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, userID int64, clientID string) {
	t.Helper()
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_sso_audit_log WHERE userId=?`, `DELETE FROM sys_sso_audit_log WHERE "userId"=?`, userID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_sso_refresh_token_family WHERE userId=?`, `DELETE FROM sys_sso_refresh_token_family WHERE "userId"=?`, userID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_sso_session WHERE userId=?`, `DELETE FROM sys_sso_session WHERE "userId"=?`, userID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_sso_client WHERE clientId=?`, `DELETE FROM sys_sso_client WHERE "clientId"=?`, clientID)
}
