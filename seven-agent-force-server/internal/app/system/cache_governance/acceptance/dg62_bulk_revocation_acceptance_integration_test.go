package acceptance

import (
	"context"
	"fmt"
	"os"
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
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

const dg62BulkSessionCount = 101

// TestDG62BulkRevocationRegistersEveryTargetPastAuditSample exercises each
// bulk SSO mutation through the real application/repository/outbox path. The
// 101-row fixture crosses both the keyset page and bounded-audit-sample limit;
// the assertion is on durable v2 targets, never on the audit representation.
func TestDG62BulkRevocationRegistersEveryTargetPastAuditSample(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	defer target.db.Close()
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)
	prefix := fmt.Sprintf("seven-dg62-bulk-%s-%d", target.dialect, time.Now().UTC().UnixNano())
	manager, governed, redisProvider := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "bulk")
	defer redisProvider.Close()
	targeted, ok := manager.(cacheinfra.TargetedGovernedCache)
	if !ok {
		t.Fatal("DG6.2 bulk acceptance manager is missing targeted cache")
	}
	ids, err := xid.New(93)
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
	registrar := cachegovapp.NewTargetedService(outbox, generation, broker, outbox, "dg62-bulk-relay-"+target.dialect)
	if !registrar.Enabled() {
		t.Fatal("DG6.2 bulk targeted registrar disabled")
	}
	repo, err := ssoinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatal(err)
	}
	service := ssoapp.NewService(config.SSOConfig{}, ids, repo, ssoinfra.NewAuthSessionCache(nil), nil, nil, nil, nil)
	service.BindTransactor(target.provider.Transactor())
	service.BindActiveSessionValidityInvalidations(registrar)

	// Keep the fixture cleanup ahead of the deferred DB close. t.Cleanup runs
	// after ordinary defers, which would otherwise attempt cleanup on a closed
	// isolated database handle.
	defer func() {
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_audit_log WHERE clientId=?`, `DELETE FROM sys_sso_audit_log WHERE "clientId"=?`, "dg62-bulk-client")
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_session WHERE sessionId LIKE ?`, `DELETE FROM sys_sso_session WHERE "sessionId" LIKE ?`, "dg62-bulk-%")
		dg5ResetAcceptanceOutbox(t, context.Background(), target)
	}()

	tests := []struct {
		name     string
		session  func(int) ssodomain.Session
		revoke   func(context.Context) (int64, error)
		mysql    string
		postgres string
		args     []any
	}{
		{
			name: "platform",
			session: func(index int) ssodomain.Session {
				return dg62BulkSession("platform", index, "dg62-platform", "PASSWORD", "", 0)
			},
			revoke: func(callCtx context.Context) (int64, error) {
				return service.RevokeSessionsByPlatformCode(callCtx, "dg62-platform")
			},
			mysql:    `SELECT COUNT(*) FROM sys_sso_session WHERE platformCode=? AND status=1 AND isDeleted=0`,
			postgres: `SELECT COUNT(*) FROM sys_sso_session WHERE "platformCode"=? AND status=1 AND "isDeleted"=0`,
			args:     []any{"dg62-platform"},
		},
		{
			name: "login_method",
			session: func(index int) ssodomain.Session {
				return dg62BulkSession("login", index, "dg62-login-platform", "EXTERNAL_OAUTH", "dg62-login-provider", 0)
			},
			revoke: func(callCtx context.Context) (int64, error) {
				return service.RevokeSessionsByPlatformLoginMethod(callCtx, "dg62-login-platform", "EXTERNAL_OAUTH", "dg62-login-provider")
			},
			mysql:    `SELECT COUNT(*) FROM sys_sso_session WHERE platformCode=? AND loginMethod=? AND externalProviderCode=? AND status=1 AND isDeleted=0`,
			postgres: `SELECT COUNT(*) FROM sys_sso_session WHERE "platformCode"=? AND "loginMethod"=? AND "externalProviderCode"=? AND status=1 AND "isDeleted"=0`,
			args:     []any{"dg62-login-platform", "EXTERNAL_OAUTH", "dg62-login-provider"},
		},
		{
			name: "provider",
			session: func(index int) ssodomain.Session {
				return dg62BulkSession("provider", index, "dg62-provider-platform", "EXTERNAL_OAUTH", "dg62-provider", int64(880000+index))
			},
			revoke: func(callCtx context.Context) (int64, error) {
				return service.RevokeSessionsByExternalProvider(callCtx, "dg62-provider")
			},
			mysql:    `SELECT COUNT(*) FROM sys_sso_session WHERE externalProviderCode=? AND status=1 AND isDeleted=0`,
			postgres: `SELECT COUNT(*) FROM sys_sso_session WHERE "externalProviderCode"=? AND status=1 AND "isDeleted"=0`,
			args:     []any{"dg62-provider"},
		},
		{
			name: "identity",
			session: func(index int) ssodomain.Session {
				return dg62BulkSession("identity", index, "dg62-identity-platform", "EXTERNAL_OAUTH", "dg62-identity-provider", 887766)
			},
			revoke: func(callCtx context.Context) (int64, error) {
				return service.RevokeSessionsByExternalIdentity(callCtx, 887766)
			},
			mysql:    `SELECT COUNT(*) FROM sys_sso_session WHERE externalIdentityId=? AND status=1 AND isDeleted=0`,
			postgres: `SELECT COUNT(*) FROM sys_sso_session WHERE "externalIdentityId"=? AND status=1 AND "isDeleted"=0`,
			args:     []any{int64(887766)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dg5ResetAcceptanceOutbox(t, ctx, target)
			for index := 1; index <= dg62BulkSessionCount; index++ {
				item := test.session(index)
				if err := repo.InsertSession(ctx, &item); err != nil {
					t.Fatal(err)
				}
			}
			revoked, err := test.revoke(ctx)
			if err != nil || revoked != dg62BulkSessionCount {
				t.Fatalf("bulk %s revoke=%d err=%v", test.name, revoked, err)
			}
			dg62AssertPendingTargetEvents(t, ctx, target, dg62BulkSessionCount)
			dg62AssertBulkRevoked(t, ctx, target, test.mysql, test.postgres, test.args...)
			dg6ExecFixture(t, ctx, target, `DELETE FROM sys_sso_session WHERE sessionId LIKE ?`, `DELETE FROM sys_sso_session WHERE "sessionId" LIKE ?`, "dg62-bulk-"+test.name+"-%")
		})
	}

	t.Run("overlapping_bulk_fences_have_a_global_order", func(t *testing.T) {
		requestA, _ := cachepolicy.ActiveSessionValidityReadRequest("dg62-order-a-" + prefix)
		requestB, _ := cachepolicy.ActiveSessionValidityReadRequest("dg62-order-b-" + prefix)
		first, err := outbox.BeginTargetedMutationFence(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Release()
		if err := first.AcquireTargetedMutation(ctx, requestA.Entry.DataClass, requestA.TargetKind, requestA.TargetDigest); err != nil {
			t.Fatal(err)
		}
		if err := first.AcquireTargetedMutation(ctx, requestB.Entry.DataClass, requestB.TargetKind, requestB.TargetDigest); err != nil {
			t.Fatal(err)
		}
		second, err := outbox.BeginTargetedMutationFence(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Release()
		acquired := make(chan error, 1)
		go func() {
			// This competing bulk transaction requests B then A, the reverse of
			// the first. It must wait at the one batch lock, never form a
			// digest-lock cycle with the first transaction.
			if err := second.AcquireTargetedMutation(ctx, requestB.Entry.DataClass, requestB.TargetKind, requestB.TargetDigest); err != nil {
				acquired <- err
				return
			}
			acquired <- second.AcquireTargetedMutation(ctx, requestA.Entry.DataClass, requestA.TargetKind, requestA.TargetDigest)
		}()
		select {
		case err := <-acquired:
			t.Fatalf("reverse-order bulk fence bypassed serialization: %v", err)
		case <-time.After(150 * time.Millisecond):
		}
		first.Release()
		select {
		case err := <-acquired:
			if err != nil {
				t.Fatalf("serialized reverse-order bulk fence: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("serialized reverse-order bulk fence did not acquire after release")
		}
	})

	t.Run("blocked_target_times_out_and_rolls_back_all_prior_outbox_rows", func(t *testing.T) {
		dg5ResetAcceptanceOutbox(t, ctx, target)
		const blockedKind = "blocked"
		for index := 1; index <= dg62BulkSessionCount; index++ {
			item := dg62BulkSession(blockedKind, index, "dg62-bulk-blocked", "PASSWORD", "", 0)
			if err := repo.InsertSession(ctx, &item); err != nil {
				t.Fatal(err)
			}
		}
		blockedRequest, _ := cachepolicy.ActiveSessionValidityReadRequest(fmt.Sprintf("dg62-bulk-%s-%03d", blockedKind, 51))
		blocker, err := outbox.AcquireTargetedMutation(ctx, blockedRequest.Entry.DataClass, blockedRequest.TargetKind, blockedRequest.TargetDigest)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Release()
		started := time.Now()
		revoked, err := service.RevokeSessionsByPlatformCode(ctx, "dg62-bulk-blocked")
		if err == nil || revoked != 0 || time.Since(started) < 1500*time.Millisecond {
			t.Fatalf("blocked target did not fail closed at the advisory-lock timeout: revoked=%d elapsed=%s err=%v", revoked, time.Since(started), err)
		}
		if count := dg62TargetedOutboxEventCount(t, ctx, target); count != 0 {
			t.Fatalf("timed-out bulk revocation left partial target outbox rows: %d", count)
		}
		dg62AssertBulkStillActive(t, ctx, target, "dg62-bulk-blocked")
		dg6ExecFixture(t, ctx, target, `DELETE FROM sys_sso_session WHERE sessionId LIKE ?`, `DELETE FROM sys_sso_session WHERE "sessionId" LIKE ?`, "dg62-bulk-blocked-%")
	})
}

func dg62BulkSession(kind string, index int, platform, loginMethod, provider string, identityID int64) ssodomain.Session {
	now := time.Now().UTC()
	return ssodomain.Session{SessionID: fmt.Sprintf("dg62-bulk-%s-%03d", kind, index), UserID: int64(940000 + index), ClientID: "dg62-bulk-client", PlatformCode: platform, LoginMethod: loginMethod, ExternalProviderCode: provider, ExternalIdentityID: identityID, ACR: "pwd", AMR: []string{"pwd"}, LoginAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Status: ssodomain.SessionStatusActive}
}

func dg62AssertBulkRevoked(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, mysql, postgres string, args ...any) {
	t.Helper()
	query := mysql
	if target.dialect == "postgres" {
		query = postgres
	}
	var count int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != dg62BulkSessionCount {
		t.Fatalf("bulk durable revoked count=%d want=%d", count, dg62BulkSessionCount)
	}
}

func dg62TargetedOutboxEventCount(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) int {
	t.Helper()
	query := `SELECT COUNT(*) FROM sys_outbox_event WHERE eventOwner=? AND scopeId=? AND eventType=?`
	if target.dialect == "postgres" {
		query = `SELECT COUNT(*) FROM sys_outbox_event WHERE "eventOwner"=? AND "scopeId"=? AND "eventType"=?`
	}
	var count int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.TargetedCacheInvalidationEventType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func dg62AssertBulkStillActive(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, platform string) {
	t.Helper()
	query := `SELECT COUNT(*) FROM sys_sso_session WHERE platformCode=? AND status=0 AND isDeleted=0`
	if target.dialect == "postgres" {
		query = `SELECT COUNT(*) FROM sys_sso_session WHERE "platformCode"=? AND status=0 AND "isDeleted"=0`
	}
	var count int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), platform).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != dg62BulkSessionCount {
		t.Fatalf("timed-out bulk revocation changed durable session facts: active=%d want=%d", count, dg62BulkSessionCount)
	}
}
