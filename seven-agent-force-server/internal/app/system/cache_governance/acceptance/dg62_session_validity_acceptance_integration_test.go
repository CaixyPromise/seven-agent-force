package acceptance

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	ssoapp "github.com/CaixyPromise/seven-agent/force/internal/app/sso/application"
	ssodomain "github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	ssoinfra "github.com/CaixyPromise/seven-agent/force/internal/app/sso/infrastructure"
	cachegovapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/application"
	cachegovinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/infrastructure"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/bytedance/sonic"
)

// TestDG62TargetedSessionRevocationPreRelay is deliberately an application
// path test: B source-loads a real session snapshot, A invokes real
// RevokeSession, and B must reject before relay/consumer runs. It also proves
// the single target event count and that a different warm session remains
// usable rather than being class-wide flushed.
func TestDG62TargetedSessionRevocationPreRelay(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	defer target.db.Close()
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)
	prefix := fmt.Sprintf("seven-dg62-%s-%d", target.dialect, time.Now().UnixNano())
	mgrA, governedA, redisA := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "a")
	defer redisA.Close()
	mgrB, governedB, redisB := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "b")
	defer redisB.Close()
	targetedA := mgrA.(cacheinfra.TargetedGovernedCache)
	targetedCacheB := mgrB.(cacheinfra.TargetedGovernedCache)
	ids, err := xid.New(92)
	if err != nil {
		t.Fatal(err)
	}
	outbox := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), ids.NextID)
	targetedA.SetTargetFreshnessGate(outbox)
	targetedCacheB.SetTargetFreshnessGate(outbox)
	// Keep the source-adjacent gate healthy while the relay is deliberately
	// paused below: the pre-relay assertion must exercise the PENDING target
	// fence, rather than passing only because RabbitMQ is declared unhealthy.
	governedA.SetFanoutHealthy(true)
	governedB.SetFanoutHealthy(true)
	rabbitA, err := rabbitinfra.New(dg5AcceptanceRabbitConfig(target.cfg))
	if err != nil {
		t.Fatalf("connect local RabbitMQ instance A: %v", err)
	}
	defer rabbitA.Close()
	rabbitB, err := rabbitinfra.New(dg5AcceptanceRabbitConfig(target.cfg))
	if err != nil {
		t.Fatalf("connect local RabbitMQ instance B: %v", err)
	}
	defer rabbitB.Close()
	generationA := cachegovinfra.NewGenerationAdapter(governedA)
	generationB := cachegovinfra.NewGenerationAdapter(governedB)
	brokerA, err := cachegovinfra.NewFanoutAdapter(rabbitA, generationA, prefix+"-a", true)
	if err != nil {
		t.Fatal(err)
	}
	brokerB, err := cachegovinfra.NewFanoutAdapter(rabbitB, generationB, prefix+"-b", true)
	if err != nil {
		t.Fatal(err)
	}
	recordingBrokerA := newDG62RecordingTargetedFanout(brokerA)
	registrar := cachegovapp.NewTargetedService(outbox, generationA, recordingBrokerA, outbox, "dg62-relay-"+target.dialect+"-a")
	if !registrar.Enabled() {
		t.Fatal("targeted registrar disabled")
	}
	targetedB := cachegovapp.NewTargetedService(outbox, generationB, brokerB, outbox, "dg62-relay-"+target.dialect+"-b")
	if !targetedB.Enabled() {
		t.Fatal("targeted consumer service disabled")
	}
	cacheServiceA := cachegovapp.NewService(outbox, generationA, brokerA, outbox, "dg62-cache-relay-"+target.dialect+"-a")
	cacheServiceA.BindTargeted(registrar)
	cacheServiceB := cachegovapp.NewService(outbox, generationB, brokerB, outbox, "dg62-cache-relay-"+target.dialect+"-b")
	cacheServiceB.BindTargeted(targetedB)
	repo, err := ssoinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()
	sessionA := "dg62-a-" + fmt.Sprint(base.UnixNano())
	sessionB := "dg62-b-" + fmt.Sprint(base.UnixNano())
	disabledClientID := "dg62-disabled-client-" + fmt.Sprint(base.UnixNano())
	sessionC := "dg62-c-" + fmt.Sprint(base.UnixNano())
	// These de-identified sentinels must stay out of every v2 transport/cache
	// representation; failures intentionally do not log their values.
	for _, item := range []ssodomain.Session{{SessionID: sessionA, UserID: 92001, ClientID: "dg62-client", PlatformCode: "dg62", LoginMethod: "PASSWORD", LoginIP: "dg62-ip-sentinel", UserAgent: "dg62-agent-sentinel", MetadataJSON: `{"marker":"dg62-session-metadata-sentinel"}`, ACR: "pwd", AMR: []string{"pwd"}, LoginAt: base, ExpiresAt: base.Add(time.Hour), Status: ssodomain.SessionStatusActive}, {SessionID: sessionB, UserID: 92002, ClientID: "dg62-client", PlatformCode: "dg62", LoginMethod: "PASSWORD", LoginIP: "dg62-ip-sentinel-b", UserAgent: "dg62-agent-sentinel-b", MetadataJSON: `{"marker":"dg62-session-metadata-sentinel-b"}`, ACR: "pwd", AMR: []string{"pwd"}, LoginAt: base, ExpiresAt: base.Add(time.Hour), Status: ssodomain.SessionStatusActive}} {
		if err := repo.InsertSession(ctx, &item); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.InsertRefreshTokenFamily(ctx, &ssodomain.RefreshTokenFamily{FamilyID: "dg62-family-" + fmt.Sprint(base.UnixNano()), SessionID: sessionA, ClientID: "dg62-client", UserID: 92001, CurrentTokenHash: "dg62-token-sentinel", PreviousTokenHash: "dg62-cookie-sentinel", MetadataJSON: `{"marker":"dg62-refresh-metadata-sentinel"}`, ExpiresAt: base.Add(time.Hour), Status: ssodomain.RefreshFamilyStatusActive}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_refresh_token_family WHERE sessionId=?`, `DELETE FROM sys_sso_refresh_token_family WHERE "sessionId"=?`, sessionA)
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_session WHERE sessionId IN (?, ?, ?)`, `DELETE FROM sys_sso_session WHERE "sessionId" IN (?, ?, ?)`, sessionA, sessionB, sessionC)
		dg6ExecFixture(t, context.Background(), target, `DELETE FROM sys_sso_client WHERE clientId=?`, `DELETE FROM sys_sso_client WHERE "clientId"=?`, disabledClientID)
		dg5ResetAcceptanceOutbox(t, context.Background(), target)
	}()
	serviceA := ssoapp.NewService(config.SSOConfig{}, ids, repo, ssoinfra.NewAuthSessionCache(nil), nil, nil, nil, nil)
	serviceA.BindTransactor(target.provider.Transactor())
	serviceA.BindActiveSessionValidityInvalidations(registrar)
	serviceA.BindActiveSessionValidityCache(ssoinfra.NewActiveSessionValidityCache(targetedA))
	serviceB := ssoapp.NewService(config.SSOConfig{}, ids, repo, ssoinfra.NewAuthSessionCache(nil), nil, nil, nil, nil)
	serviceB.BindTransactor(target.provider.Transactor())
	serviceB.BindActiveSessionValidityInvalidations(registrar)
	recordingB := newDG62RecordingTargetedCache(targetedCacheB)
	serviceB.BindActiveSessionValidityCache(ssoinfra.NewActiveSessionValidityCache(recordingB))
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionA); err != nil || item == nil {
		t.Fatalf("warm B session A: %v", err)
	}
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionB); err != nil || item == nil {
		t.Fatalf("warm B session B: %v", err)
	}
	requestA, _ := cachepolicy.ActiveSessionValidityReadRequest(sessionA)
	requestB, _ := cachepolicy.ActiveSessionValidityReadRequest(sessionB)
	recordingB.ResetLoaderCalls()
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionB); err != nil || item == nil {
		t.Fatalf("B pre-mutation session B read: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestB); calls != 0 {
		t.Fatalf("unrelated B session source-loaded before mutation: calls=%d", calls)
	}
	if revoked, err := serviceA.RevokeSession(ctx, sessionA); err != nil || !revoked {
		t.Fatalf("A real revoke=%v err=%v", revoked, err)
	}
	dg62AssertPendingTargetEvents(t, ctx, target, 1)
	dg62AssertSessionBoundaryIsOpaque(t, ctx, target, redisB, prefix, sessionA, "dg62-token-sentinel", "dg62-cookie-sentinel", "dg62-ip-sentinel", "dg62-agent-sentinel", "dg62-session-metadata-sentinel", "dg62-refresh-metadata-sentinel")
	recordingB.ResetLoaderCalls()
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionA); err != nil || item != nil {
		t.Fatalf("B accepted revoked pre-relay session: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestA); calls != 1 {
		t.Fatalf("revoked B session did not source-load behind pending target fence: calls=%d", calls)
	}
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionB); err != nil || item == nil {
		t.Fatalf("unrelated session was flushed: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestB); calls != 0 {
		t.Fatalf("A target invalidation forced unrelated B session source-load: calls=%d", calls)
	}

	// Now start the real B fanout consumer and relay A's durable event. The
	// receiver must evict its exact old L1 entry before ACK; a later B read
	// cannot silently reuse the revoked session snapshot.
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	defer consumerCancel()
	governedB.SetFanoutHealthy(false)
	go func() {
		_ = brokerB.ConsumeMixed(consumerCtx, cacheServiceB.HandleFanout, cacheServiceB.HandleTargetedFanout)
	}()
	waitDG5(t, "DG6.2 targeted B RabbitMQ fanout consumer healthy", func() bool {
		return governedB.GovernedStatus().FanoutHealthy
	})
	if err := cacheServiceA.RelayOutbox(ctx, 10); err != nil {
		t.Fatalf("relay committed DG6.2 target invalidation: %v", err)
	}
	waitDG5(t, "DG6.2 targeted fanout is completed", func() bool {
		return dg5OutboxStatusCount(t, ctx, target, "DONE") == 1
	})
	dg62AssertPublishedTargetedEnvelopeOpaque(t, recordingBrokerA, sessionA, "dg62-token-sentinel", "dg62-cookie-sentinel", "dg62-ip-sentinel", "dg62-agent-sentinel", "dg62-session-metadata-sentinel", "dg62-refresh-metadata-sentinel")
	for _, broker := range []*cachegovinfra.FanoutAdapter{brokerA, brokerB} {
		count, err := broker.DeadLetterCount(ctx)
		if err != nil || count != 0 {
			t.Fatal("DG6.2 valid targeted fanout unexpectedly produced a broker diagnostic")
		}
	}
	recordingB.ResetLoaderCalls()
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionA); err != nil || item != nil {
		t.Fatalf("B reused revoked L1 after targeted fanout: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestA); calls != 1 {
		t.Fatalf("targeted fanout did not force revoked B session source-load: calls=%d", calls)
	}
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionB); err != nil || item == nil {
		t.Fatalf("targeted fanout flushed unrelated session: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestB); calls != 0 {
		t.Fatalf("targeted fanout forced unrelated B session source-load: calls=%d", calls)
	}

	// A real managed OAuth-client disable is another session-validity fact
	// mutation. It must register C's exact target in the same transaction while
	// preserving B's unrelated warm candidate as a cache hit.
	if err := repo.InsertClient(ctx, &ssodomain.Client{ClientID: disabledClientID, ClientName: "DG6.2 Disabled Client", ClientType: "CONFIDENTIAL", ClientAuthMethod: "client_secret_basic", GrantTypes: []string{"refresh_token"}, Scopes: []string{"openid"}, RequirePKCE: true, AccessTokenTTLSec: 60, RefreshTokenTTLSec: 60, Status: ssodomain.ClientStatusActive, MetadataJSON: `{"managedBy":"hub_control","ownerNodeCode":"dg62-node"}`}, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertSession(ctx, &ssodomain.Session{SessionID: sessionC, UserID: 92003, ClientID: disabledClientID, PlatformCode: "dg62", LoginMethod: "PASSWORD", ACR: "pwd", AMR: []string{"pwd"}, LoginAt: base, ExpiresAt: base.Add(time.Hour), Status: ssodomain.SessionStatusActive}); err != nil {
		t.Fatal(err)
	}
	requestC, _ := cachepolicy.ActiveSessionValidityReadRequest(sessionC)
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionC); err != nil || item == nil {
		t.Fatalf("warm B client-disable session: item=%v err=%v", item, err)
	}
	if err := serviceA.SetManagedClientStatus(ctx, ssofacade.ManagedClientStatusCommand{ClientID: disabledClientID, OwnerNodeCode: "dg62-node", Status: ssodomain.ClientStatusDisabled}); err != nil {
		t.Fatalf("real managed client disable: %v", err)
	}
	dg62AssertPendingTargetEvents(t, ctx, target, 1)
	recordingB.ResetLoaderCalls()
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionC); err != nil || item != nil {
		t.Fatalf("B accepted session after real client disable: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestC); calls != 1 {
		t.Fatalf("client-disabled target did not source-load behind pending fence: calls=%d", calls)
	}
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionB); err != nil || item == nil {
		t.Fatalf("client-disable invalidation flushed unrelated B session: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestB); calls != 0 {
		t.Fatalf("client-disable invalidation forced unrelated B session source-load: calls=%d", calls)
	}

	// A stopped real fanout consumer makes RabbitMQ delivery health uncertain.
	// Redis remains available and session B is still locally warm, so the source
	// callback is direct evidence that the target layer refuses that L1 value.
	consumerCancel()
	waitDG5(t, "DG6.2 targeted B RabbitMQ fanout consumer stopped", func() bool {
		return !governedB.GovernedStatus().FanoutHealthy
	})
	recordingB.ResetLoaderCalls()
	if item, err := serviceB.ResolveActiveSessionForCandidateUse(ctx, sessionB); err != nil || item == nil {
		t.Fatalf("B did not authority-read while RabbitMQ health was uncertain: item=%v err=%v", item, err)
	}
	if calls := recordingB.LoaderCalls(requestB); calls != 1 {
		t.Fatalf("unhealthy RabbitMQ reused warm B session: source-loads=%d", calls)
	}
}

// dg62RecordingTargetedCache preserves every real cache operation and only
// counts execution of the actual SSO source-loader callback by target digest.
// It is test-only: it cannot fabricate an L1/L2 result or modify a delegate
// return value.
type dg62RecordingTargetedCache struct {
	cacheinfra.TargetedGovernedCache
	mu    sync.Mutex
	loads map[string]int
}

func newDG62RecordingTargetedCache(delegate cacheinfra.TargetedGovernedCache) *dg62RecordingTargetedCache {
	return &dg62RecordingTargetedCache{TargetedGovernedCache: delegate, loads: make(map[string]int)}
}

func (r *dg62RecordingTargetedCache) GetOrLoadTargeted(ctx context.Context, request cachepolicy.TargetedReadRequest, dest any, loader cacheinfra.TargetedLoader) (bool, error) {
	if r == nil || r.TargetedGovernedCache == nil {
		return false, cacheinfra.ErrClassifiedCacheRequestInvalid
	}
	return r.TargetedGovernedCache.GetOrLoadTargeted(ctx, request, dest, func(loadCtx context.Context) (cachepolicy.TargetedCacheableValue, error) {
		r.mu.Lock()
		r.loads[request.TargetDigest]++
		r.mu.Unlock()
		return loader(loadCtx)
	})
}

func (r *dg62RecordingTargetedCache) ResetLoaderCalls() {
	r.mu.Lock()
	r.loads = make(map[string]int)
	r.mu.Unlock()
}

func (r *dg62RecordingTargetedCache) LoaderCalls(request cachepolicy.TargetedReadRequest) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loads[request.TargetDigest]
}

// dg62RecordingTargetedFanout forwards the confirmed real RabbitMQ publish
// unchanged, retaining only the test's outgoing strict-Sonic envelope so the
// acceptance can inspect it without reading or printing a broker delivery.
type dg62RecordingTargetedFanout struct {
	cachepolicy.TargetedFanoutPort
	mu       sync.Mutex
	payloads [][]byte
}

func newDG62RecordingTargetedFanout(delegate cachepolicy.TargetedFanoutPort) *dg62RecordingTargetedFanout {
	return &dg62RecordingTargetedFanout{TargetedFanoutPort: delegate}
}

func (r *dg62RecordingTargetedFanout) PublishTargeted(ctx context.Context, event cachepolicy.TargetedInvalidationEnvelope) error {
	payload, err := sonic.Marshal(event)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.payloads = append(r.payloads, append([]byte(nil), payload...))
	r.mu.Unlock()
	return r.TargetedFanoutPort.PublishTargeted(ctx, event)
}

func (r *dg62RecordingTargetedFanout) Payloads() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([][]byte, len(r.payloads))
	for index := range r.payloads {
		result[index] = append([]byte(nil), r.payloads[index]...)
	}
	return result
}

func dg62AssertPublishedTargetedEnvelopeOpaque(t *testing.T, fanout *dg62RecordingTargetedFanout, forbidden ...string) {
	t.Helper()
	if fanout == nil {
		t.Fatal("DG6.2 broker opacity assertion requires the real publish observer")
	}
	payloads := fanout.Payloads()
	if len(payloads) != 1 || dg62ContainsForbidden(string(payloads[0]), forbidden) {
		t.Fatal("DG6.2 targeted RabbitMQ envelope exposed a prohibited session field")
	}
}

func dg62AssertPendingTargetEvents(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, want int) {
	t.Helper()
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	query := `SELECT COUNT(*) FROM sys_outbox_event WHERE eventOwner=? AND scopeId=? AND eventType=? AND status='PENDING'`
	if target.dialect == "postgres" {
		query = `SELECT COUNT(*) FROM sys_outbox_event WHERE "eventOwner"=? AND "scopeId"=? AND "eventType"=? AND status='PENDING'`
	}
	var count int
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.TargetedCacheInvalidationEventType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("PENDING targeted events=%d want=%d", count, want)
	}
}

// dg62AssertSessionBoundaryIsOpaque checks the real durable event and the
// unique Redis namespace without emitting either body. A raw session ID is
// request-local only: it cannot appear in the target digest, cache key, or
// cached projection.
func dg62AssertSessionBoundaryIsOpaque(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, redis cacheinfra.Provider, prefix string, forbidden ...string) {
	t.Helper()
	if redis == nil || redis.Client() == nil {
		t.Fatal("DG6.2 opacity assertion requires configured Redis")
	}
	query := `SELECT aggregateId, payload FROM sys_outbox_event WHERE eventOwner=? AND scopeId=? AND eventType=? AND status='PENDING'`
	if target.dialect == "postgres" {
		query = `SELECT "aggregateId", payload FROM sys_outbox_event WHERE "eventOwner"=? AND "scopeId"=? AND "eventType"=? AND status='PENDING'`
	}
	var aggregateID, payload string
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.TargetedCacheInvalidationEventType).Scan(&aggregateID, &payload); err != nil {
		t.Fatal(err)
	}
	if !cachepolicy.IsDigest(aggregateID) || dg62ContainsForbidden(aggregateID, forbidden) || dg62ContainsForbidden(payload, forbidden) {
		t.Fatal("DG6.2 target outbox exposed a raw session identifier")
	}
	keys, _, err := redis.Client().Scan(ctx, 0, prefix+"*", 100).Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if dg62ContainsForbidden(key, forbidden) {
			t.Fatal("DG6.2 Redis key exposed a prohibited session field")
		}
		payload, getErr := redis.Client().Get(ctx, key).Bytes()
		if getErr == nil && dg62ContainsForbidden(string(payload), forbidden) {
			t.Fatal("DG6.2 Redis payload exposed a prohibited session field")
		}
	}
}

func dg62ContainsForbidden(value string, forbidden []string) bool {
	for _, marker := range forbidden {
		if marker != "" && strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
