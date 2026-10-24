package acceptance

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ssoapp "github.com/CaixyPromise/seven-agent/force/internal/app/sso/application"
	ssodomain "github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssoinfra "github.com/CaixyPromise/seven-agent/force/internal/app/sso/infrastructure"
	cachegovapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/application"
	cachegovinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/infrastructure"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

// TestDG62RefreshReusePunishmentCommitsBeforeUnauthorized is the RED/GREEN
// regression for a subtle transactional rule: once previous-token reuse is
// confirmed, session/family punishment and its exact target event must commit
// before the public OAuth Unauthorized response is returned.
func TestDG62RefreshReusePunishmentCommitsBeforeUnauthorized(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	defer target.db.Close()
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)
	prefix := fmt.Sprintf("seven-dg62-reuse-%s-%d", target.dialect, time.Now().UTC().UnixNano())
	manager, governed, redisProvider := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "reuse")
	defer redisProvider.Close()
	targeted, ok := manager.(cacheinfra.TargetedGovernedCache)
	if !ok {
		t.Fatal("DG6.2 refresh-reuse manager is missing targeted cache")
	}
	ids, err := xid.New(94)
	if err != nil {
		t.Fatal(err)
	}
	outbox := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), ids.NextID)
	targeted.SetTargetFreshnessGate(outbox)
	governed.SetFanoutHealthy(true)
	rabbit, err := rabbitinfra.New(dg5AcceptanceRabbitConfig(target.cfg))
	if err != nil {
		t.Fatalf("connect local RabbitMQ: %v", err)
	}
	defer rabbit.Close()
	generation := cachegovinfra.NewGenerationAdapter(governed)
	broker, err := cachegovinfra.NewFanoutAdapter(rabbit, generation, prefix+"-fanout", true)
	if err != nil {
		t.Fatal(err)
	}
	registrar := cachegovapp.NewTargetedService(outbox, generation, broker, outbox, "dg62-reuse-relay-"+target.dialect)
	if !registrar.Enabled() {
		t.Fatal("DG6.2 refresh-reuse registrar disabled")
	}
	repo, err := ssoinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatal(err)
	}
	jwt := dg62RefreshReuseJWT(t)
	base := time.Now().UTC()
	clientID := "dg62-reuse-client-" + fmt.Sprint(base.UnixNano())
	sessionID := "dg62-reuse-session-" + fmt.Sprint(base.UnixNano())
	familyID := "dg62-reuse-family-" + fmt.Sprint(base.UnixNano())
	revokeSessionID := "dg62-revoke-error-session-" + fmt.Sprint(base.UnixNano())
	revokeFamilyID := "dg62-revoke-error-family-" + fmt.Sprint(base.UnixNano())
	revokeToken := "dg62-revoke-error-token-" + fmt.Sprint(base.UnixNano())
	service := ssoapp.NewService(config.SSOConfig{Issuer: "https://dg62.example/sso"}, ids, repo, ssoinfra.NewAuthSessionCache(nil), jwt, nil, nil, nil)
	service.BindTransactor(target.provider.Transactor())
	service.BindActiveSessionValidityInvalidations(registrar)
	defer func() {
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_audit_log WHERE clientId=?`, `DELETE FROM sys_sso_audit_log WHERE "clientId"=?`, clientID)
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_refresh_token_family WHERE familyId IN (?, ?)`, `DELETE FROM sys_sso_refresh_token_family WHERE "familyId" IN (?, ?)`, familyID, revokeFamilyID)
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_session WHERE sessionId IN (?, ?)`, `DELETE FROM sys_sso_session WHERE "sessionId" IN (?, ?)`, sessionID, revokeSessionID)
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_client WHERE clientId=?`, `DELETE FROM sys_sso_client WHERE "clientId"=?`, clientID)
		dg5ResetAcceptanceOutbox(t, context.Background(), target)
	}()
	if err := repo.InsertClient(ctx, &ssodomain.Client{ClientID: clientID, ClientName: "DG6.2 reuse client", ClientType: "PUBLIC", ClientAuthMethod: "none", GrantTypes: []string{"refresh_token"}, Scopes: []string{"openid", "offline_access"}, AccessTokenTTLSec: 60, RefreshTokenTTLSec: 60, Status: ssodomain.ClientStatusActive}, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertSession(ctx, &ssodomain.Session{SessionID: sessionID, UserID: 95101, ClientID: clientID, PlatformCode: "dg62", LoginMethod: "PASSWORD", ACR: "pwd", AMR: []string{"pwd"}, LoginAt: base.Add(-time.Minute), ExpiresAt: base.Add(time.Hour), Status: ssodomain.SessionStatusActive}); err != nil {
		t.Fatal(err)
	}
	refreshToken, err := jwt.Sign(ctx, map[string]any{
		"iss": "https://dg62.example/sso", "sub": "95101", "uid": int64(95101), "aud": []string{clientID}, "client_id": clientID, "sid": sessionID,
		"iat": base.Add(-time.Minute).Unix(), "exp": base.Add(10 * time.Minute).Unix(), "scope": "openid offline_access", "token_type": "refresh_token", "jti": "dg62-reuse-token-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertRefreshTokenFamily(ctx, &ssodomain.RefreshTokenFamily{FamilyID: familyID, SessionID: sessionID, ClientID: clientID, UserID: 95101, CurrentTokenHash: "replacement-token-hash", PreviousTokenHash: ssoinfra.BuildTokenHash(refreshToken), RotatedAt: ptrTime(base.Add(-time.Minute)), ExpiresAt: base.Add(time.Hour), Status: ssodomain.RefreshFamilyStatusActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExchangeRefreshToken(ctx, clientID, "", refreshToken); err == nil {
		t.Fatal("refresh reuse unexpectedly accepted")
	}
	dg62AssertPendingTargetEvents(t, ctx, target, 1)
	dg62AssertRefreshReusePunishmentCommitted(t, ctx, target, sessionID, familyID)

	// RevokeToken must surface a target-fence failure. It cannot reply as if the
	// opaque refresh token had been revoked while a transaction rolls back the
	// family/session facts or omits its required target event.
	dg5ResetAcceptanceOutbox(t, ctx, target)
	if err := repo.InsertSession(ctx, &ssodomain.Session{SessionID: revokeSessionID, UserID: 95102, ClientID: clientID, PlatformCode: "dg62", LoginMethod: "PASSWORD", ACR: "pwd", AMR: []string{"pwd"}, LoginAt: base.Add(-time.Minute), ExpiresAt: base.Add(time.Hour), Status: ssodomain.SessionStatusActive}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertRefreshTokenFamily(ctx, &ssodomain.RefreshTokenFamily{FamilyID: revokeFamilyID, SessionID: revokeSessionID, ClientID: clientID, UserID: 95102, CurrentTokenHash: ssoinfra.BuildTokenHash(revokeToken), ExpiresAt: base.Add(time.Hour), Status: ssodomain.RefreshFamilyStatusActive}); err != nil {
		t.Fatal(err)
	}
	blockedRequest, _ := cachepolicy.ActiveSessionValidityReadRequest(revokeSessionID)
	blocker, err := outbox.AcquireTargetedMutation(ctx, blockedRequest.Entry.DataClass, blockedRequest.TargetKind, blockedRequest.TargetDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	if err := service.RevokeToken(ctx, revokeToken, "refresh_token"); err == nil {
		t.Fatal("RevokeToken swallowed exact target fence failure")
	}
	if count := dg62TargetedOutboxEventCount(t, ctx, target); count != 0 {
		t.Fatalf("failed RevokeToken left target outbox rows: %d", count)
	}
	dg62AssertRevokeTokenRollback(t, ctx, target, revokeSessionID, revokeFamilyID)
}

// TestDG62TokenRevocationReturnsAuthorityReadFailures is the RED/GREEN
// regression for RFC-compatible invalid-token no-ops: only a completed lookup
// with a nil family is a no-op. A database/authority read failure must reach
// the caller instead of pretending the token had already been revoked.
func TestDG62TokenRevocationReturnsAuthorityReadFailures(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	defer target.db.Close()
	assertDG5AcceptanceSchema(t, ctx, target)
	ids, err := xid.New(94)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := ssoinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatal(err)
	}
	jwt := dg62RefreshReuseJWT(t)
	base := time.Now().UTC()
	clientID := "dg62-revoke-read-error-client-" + fmt.Sprint(base.UnixNano())
	sessionID := "dg62-revoke-read-error-session-" + fmt.Sprint(base.UnixNano())
	service := func(repository ssoapp.SessionRepository) *ssoapp.Service {
		return ssoapp.NewService(config.SSOConfig{Issuer: "https://dg62.example/sso"}, ids, repository, ssoinfra.NewAuthSessionCache(nil), jwt, nil, nil, nil)
	}
	defer func() {
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_audit_log WHERE clientId=?`, `DELETE FROM sys_sso_audit_log WHERE "clientId"=?`, clientID)
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_session WHERE sessionId=?`, `DELETE FROM sys_sso_session WHERE "sessionId"=?`, sessionID)
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_client WHERE clientId=?`, `DELETE FROM sys_sso_client WHERE "clientId"=?`, clientID)
	}()
	if err := repo.InsertClient(ctx, &ssodomain.Client{ClientID: clientID, ClientName: "DG6.2 revocation read error", ClientType: "PUBLIC", ClientAuthMethod: "none", GrantTypes: []string{"refresh_token"}, Scopes: []string{"openid", "offline_access"}, AccessTokenTTLSec: 60, RefreshTokenTTLSec: 60, Status: ssodomain.ClientStatusActive}, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertSession(ctx, &ssodomain.Session{SessionID: sessionID, UserID: 95103, ClientID: clientID, PlatformCode: "dg62", LoginMethod: "PASSWORD", ACR: "pwd", AMR: []string{"pwd"}, LoginAt: base.Add(-time.Minute), ExpiresAt: base.Add(time.Hour), Status: ssodomain.SessionStatusActive}); err != nil {
		t.Fatal(err)
	}
	refreshToken, err := jwt.Sign(ctx, map[string]any{
		"iss": "https://dg62.example/sso", "sub": "95103", "uid": int64(95103), "aud": []string{clientID}, "client_id": clientID, "sid": sessionID,
		"iat": base.Add(-time.Minute).Unix(), "exp": base.Add(10 * time.Minute).Unix(), "scope": "openid offline_access", "token_type": "refresh_token", "jti": "dg62-revoke-read-error-refresh",
	})
	if err != nil {
		t.Fatal(err)
	}
	accessToken, err := jwt.Sign(ctx, map[string]any{
		"iss": "https://dg62.example/sso", "sub": "95103", "uid": int64(95103), "aud": []string{clientID}, "client_id": clientID, "sid": sessionID,
		"iat": base.Add(-time.Minute).Unix(), "exp": base.Add(10 * time.Minute).Unix(), "scope": "openid", "token_type": "access_token", "jti": "dg62-revoke-read-error-access",
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("completed refresh-family non-match is a no-op", func(t *testing.T) {
		if err := service(repo).RevokeToken(ctx, refreshToken, "refresh_token"); err != nil {
			t.Fatalf("RevokeToken() non-match error = %v", err)
		}
		if err := service(repo).RevokeTokenForClient(ctx, clientID, "", refreshToken, "refresh_token"); err != nil {
			t.Fatalf("RevokeTokenForClient() non-match error = %v", err)
		}
	})
	t.Run("cryptographically invalid access token is a no-op", func(t *testing.T) {
		if err := service(repo).RevokeToken(ctx, "not-a-jwt", "access_token"); err != nil {
			t.Fatalf("RevokeToken() invalid token error = %v", err)
		}
	})
	t.Run("bare refresh current-family lookup failure", func(t *testing.T) {
		want := errors.New("injected current family lookup failure")
		got := service(dg62FaultingRepository{SessionRepository: repo, currentFamilyErr: want}).RevokeToken(ctx, refreshToken, "refresh_token")
		if !errors.Is(got, want) {
			t.Fatalf("RevokeToken() error = %v, want current-family authority error", got)
		}
	})
	t.Run("client refresh previous-family lookup failure", func(t *testing.T) {
		want := errors.New("injected previous family lookup failure")
		got := service(dg62FaultingRepository{SessionRepository: repo, previousFamilyErr: want}).RevokeTokenForClient(ctx, clientID, "", refreshToken, "refresh_token")
		if !errors.Is(got, want) {
			t.Fatalf("RevokeTokenForClient() error = %v, want previous-family authority error", got)
		}
	})
	t.Run("bare access session lookup failure", func(t *testing.T) {
		want := errors.New("injected active session lookup failure")
		got := service(dg62FaultingRepository{SessionRepository: repo, sessionErr: want}).RevokeToken(ctx, accessToken, "access_token")
		if !errors.Is(got, want) {
			t.Fatalf("RevokeToken() error = %v, want active-session authority error", got)
		}
	})
	t.Run("client access session lookup failure", func(t *testing.T) {
		want := errors.New("injected client access session lookup failure")
		got := service(dg62FaultingRepository{SessionRepository: repo, sessionErr: want}).RevokeTokenForClient(ctx, clientID, "", accessToken, "access_token")
		if !errors.Is(got, want) {
			t.Fatalf("RevokeTokenForClient() error = %v, want active-session authority error", got)
		}
	})
	dg62AssertSessionStillActive(t, ctx, target, sessionID)
}

// dg62FaultingRepository delegates every real repository operation except the
// exact authority reads under test. The fixture and client/session lookup stay
// on the real isolated database for both supported dialects.
type dg62FaultingRepository struct {
	ssoapp.SessionRepository
	currentFamilyErr  error
	previousFamilyErr error
	sessionErr        error
}

func (r dg62FaultingRepository) FindRefreshFamilyByCurrentHash(ctx context.Context, hash string) (*ssodomain.RefreshTokenFamily, error) {
	if r.currentFamilyErr != nil {
		return nil, r.currentFamilyErr
	}
	return r.SessionRepository.FindRefreshFamilyByCurrentHash(ctx, hash)
}

func (r dg62FaultingRepository) FindRefreshFamilyByPreviousHash(ctx context.Context, hash string) (*ssodomain.RefreshTokenFamily, error) {
	if r.previousFamilyErr != nil {
		return nil, r.previousFamilyErr
	}
	return r.SessionRepository.FindRefreshFamilyByPreviousHash(ctx, hash)
}

func (r dg62FaultingRepository) FindSessionBySessionID(ctx context.Context, sessionID string) (*ssodomain.Session, error) {
	if r.sessionErr != nil {
		return nil, r.sessionErr
	}
	return r.SessionRepository.FindSessionBySessionID(ctx, sessionID)
}

func dg62AssertSessionStillActive(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, sessionID string) {
	t.Helper()
	query := `SELECT status FROM sys_sso_session WHERE sessionId=?`
	if target.dialect == "postgres" {
		query = `SELECT status FROM sys_sso_session WHERE "sessionId"=?`
	}
	var status int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), sessionID).Scan(&status); err != nil || status != ssodomain.SessionStatusActive {
		t.Fatalf("authority read failure changed session fact: status=%d err=%v", status, err)
	}
}

func dg62RefreshReuseJWT(t *testing.T) *jwtinfra.Service {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePath := filepath.Join(t.TempDir(), "private.pem")
	publicPath := filepath.Join(t.TempDir(), "public.pem")
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}), 0o600); err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := keyring.NewLocalProvider(config.KeysConfig{Provider: "local", JWT: config.JWTKeysConfig{Algorithm: "RS256", Active: config.JWTKeySourceConfig{KID: "dg62-reuse", PrivateKeySource: "file:" + privatePath, PublicKeySource: "file:" + publicPath}}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := jwtinfra.New(keys, "RS256")
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func dg62AssertRefreshReusePunishmentCommitted(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, sessionID, familyID string) {
	t.Helper()
	query := `SELECT status FROM sys_sso_session WHERE sessionId=?`
	if target.dialect == "postgres" {
		query = `SELECT status FROM sys_sso_session WHERE "sessionId"=?`
	}
	var sessionStatus int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), sessionID).Scan(&sessionStatus); err != nil || sessionStatus != ssodomain.SessionStatusRevoked {
		t.Fatalf("refresh-reuse session punishment was not committed: status=%d err=%v", sessionStatus, err)
	}
	query = `SELECT status, reuseDetected FROM sys_sso_refresh_token_family WHERE familyId=?`
	if target.dialect == "postgres" {
		query = `SELECT status, "reuseDetected" FROM sys_sso_refresh_token_family WHERE "familyId"=?`
	}
	var familyStatus, reuseDetected int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), familyID).Scan(&familyStatus, &reuseDetected); err != nil || familyStatus != ssodomain.RefreshFamilyStatusRevoked || reuseDetected != 1 {
		t.Fatalf("refresh-reuse family punishment was not committed: status=%d reuse=%d err=%v", familyStatus, reuseDetected, err)
	}
}

func dg62AssertRevokeTokenRollback(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, sessionID, familyID string) {
	t.Helper()
	sessionQuery := `SELECT status FROM sys_sso_session WHERE sessionId=?`
	familyQuery := `SELECT status FROM sys_sso_refresh_token_family WHERE familyId=?`
	if target.dialect == "postgres" {
		sessionQuery = `SELECT status FROM sys_sso_session WHERE "sessionId"=?`
		familyQuery = `SELECT status FROM sys_sso_refresh_token_family WHERE "familyId"=?`
	}
	var sessionStatus, familyStatus int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(sessionQuery), sessionID).Scan(&sessionStatus); err != nil || sessionStatus != ssodomain.SessionStatusActive {
		t.Fatalf("failed RevokeToken changed session fact: status=%d err=%v", sessionStatus, err)
	}
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(familyQuery), familyID).Scan(&familyStatus); err != nil || familyStatus != ssodomain.RefreshFamilyStatusActive {
		t.Fatalf("failed RevokeToken changed refresh family: status=%d err=%v", familyStatus, err)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
