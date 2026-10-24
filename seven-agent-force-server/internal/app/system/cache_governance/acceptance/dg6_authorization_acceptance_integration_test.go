package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authapp "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/application"
	authdomain "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	authinfra "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/infrastructure"
	cachegovapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/application"
	cachegovfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	cachegovinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/infrastructure"
	userapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/application"
	userdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/infrastructure"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

const dg6AuthorizationAcceptanceEnv = "DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE"

// TestDG6AuthorizationSnapshotCatalogContract protects the policy boundary
// before the real two-instance test is enabled. Authorization snapshots are
// deliberately private, per-user, feature-versioned cache candidates; this
// test must remain independent of a database so an accidental catalog
// broadening is caught by ordinary focused CI.
func TestDG6AuthorizationSnapshotCatalogContract(t *testing.T) {
	fingerprint := dg6FeatureFingerprint("dg6-contract-feature-set")
	contextA, ok := cachepolicy.AuthorizationContextReadRequest(1001, fingerprint)
	if !ok || !contextA.Valid() {
		t.Fatal("authorization context request must be an approved catalog candidate")
	}
	contextB, ok := cachepolicy.AuthorizationContextReadRequest(1002, fingerprint)
	if !ok || !contextB.Valid() {
		t.Fatal("a distinct authorization user must be an approved catalog candidate")
	}
	menuA, ok := cachepolicy.AuthorizationMenuReadRequest(1001, fingerprint)
	if !ok || !menuA.Valid() {
		t.Fatal("authorization menu request must be an approved catalog candidate")
	}
	if contextA.Entry.Exposure != "PRIVATE" || contextA.Entry.Sensitivity != "RESTRICTED" || contextA.MaxStale != 0 {
		t.Fatalf("authorization context must remain private/restricted with a zero stale budget: %#v", contextA.Entry)
	}
	if contextA.KeyMaterial() == contextB.KeyMaterial() || contextA.KeyMaterial() == menuA.KeyMaterial() {
		t.Fatal("authorization cache candidates must isolate user and projection identity")
	}
	if strings.Contains(contextA.KeyMaterial(), contextA.Target) || strings.Contains(contextA.TargetDigest(), contextA.Target) {
		t.Fatal("cache key and durable target digest must not expose the raw authorization target")
	}

	if _, ok := cachepolicy.AuthorizationContextReadRequest(0, fingerprint); ok {
		t.Fatal("zero user identifier must not become a cache candidate")
	}
	if _, ok := cachepolicy.AuthorizationContextReadRequest(1001, "not-a-feature-digest"); ok {
		t.Fatal("unversioned authorization snapshots must not become cache candidates")
	}
	forged := contextA
	forged.Target = "user:1001:features:" + fingerprint + ":session:authority"
	if forged.Valid() {
		t.Fatal("session authority must not be smuggled into the authorization snapshot catalog")
	}
}

// TestDG6AuthorizationInvalidationEnvelopeIsContentFree makes the durable
// cache protocol prove that authorization invalidation events contain only a
// reviewed class digest. Identity, bearer/cookie material, permission detail,
// and temporary-grant contents are never eligible transport fields.
func TestDG6AuthorizationInvalidationEnvelopeIsContentFree(t *testing.T) {
	event, err := cachepolicy.NewInvalidationEnvelope("dg6-contract-event", cachepolicy.DataClassAuthorizationContext)
	if err != nil {
		t.Fatalf("create authorization invalidation envelope: %v", err)
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("encode authorization invalidation envelope: %v", err)
	}
	serialized := strings.ToLower(string(payload))
	for _, forbidden := range []string{
		"user:1001", "permission", "temporary", "grant", "bearer", "token", "cookie", "session", "password", "roleid",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("authorization invalidation payload leaked forbidden content %q: %s", forbidden, serialized)
		}
	}
	if event.TargetDigest != cachepolicy.ClassTargetDigest(cachepolicy.DataClassAuthorizationContext) {
		t.Fatal("authorization mutations must use the class digest rather than enumerating users")
	}
}

// TestDG6AuthorizationGovernanceAcceptance uses two independent L1 instances,
// the real guarded MySQL/PostgreSQL governance database, local Redis, and
// RabbitMQ. It exercises the shared protocol using the two DG6 authorization
// data classes: a committed durable mutation must force B to reload v2 before
// relay; a rollback must append no event; and any Rabbit/Redis/freshness doubt
// must make B source-authoritative. Application-level role/menu/user fixtures
// are intentionally a separate concern from this cache-protocol acceptance.
func TestDG6AuthorizationGovernanceAcceptance(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply after the isolated migration path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)

	prefix := fmt.Sprintf("seven-dg6-%s-%d", target.dialect, time.Now().UTC().UnixNano())
	_, governedA, redisA := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "a")
	_, governedB, redisB := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "b")
	t.Cleanup(func() { _ = redisA.Close() })
	// redisB is deliberately closed during the final fail-closed assertion.

	rabbitCfg := dg5AcceptanceRabbitConfig(target.cfg)
	rabbitA, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ for DG6 instance A: %v", err)
	}
	t.Cleanup(func() { _ = rabbitA.Close() })
	rabbitB, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ for DG6 instance B: %v", err)
	}
	t.Cleanup(func() { _ = rabbitB.Close() })

	generationA := cachegovinfra.NewGenerationAdapter(governedA)
	generationB := cachegovinfra.NewGenerationAdapter(governedB)
	brokerA, err := cachegovinfra.NewFanoutAdapter(rabbitA, generationA, prefix+"-fanout-a", true)
	if err != nil {
		t.Fatalf("create DG6 RabbitMQ fanout A: %v", err)
	}
	brokerB, err := cachegovinfra.NewFanoutAdapter(rabbitB, generationB, prefix+"-fanout-b", true)
	if err != nil {
		t.Fatalf("create DG6 RabbitMQ fanout B: %v", err)
	}
	idsA, err := xid.New(71)
	if err != nil {
		t.Fatalf("create DG6 isolated instance-A id generator: %v", err)
	}
	idsB, err := xid.New(72)
	if err != nil {
		t.Fatalf("create DG6 isolated instance-B id generator: %v", err)
	}
	outboxA := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), idsA.NextID)
	outboxB := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), idsB.NextID)
	governedA.SetFreshnessGate(outboxA)
	governedB.SetFreshnessGate(outboxB)
	relayA := cachegovapp.NewService(outboxA, generationA, brokerA, outboxA, "dg6-relay-"+target.dialect+"-a")
	relayB := cachegovapp.NewService(outboxB, generationB, brokerB, outboxB, "dg6-relay-"+target.dialect+"-b")
	if !relayA.Enabled() || !relayB.Enabled() {
		t.Fatal("DG6 relay services must be fully configured")
	}
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	t.Cleanup(consumerCancel)
	startDG5Consumer(t, consumerCtx, brokerA, relayA, governedA)
	startDG5Consumer(t, consumerCtx, brokerB, relayB, governedB)

	request, ok := cachepolicy.AuthorizationContextReadRequest(1001, dg6FeatureFingerprint("dg6-acceptance-features"))
	if !ok {
		t.Fatal("construct DG6 authorization cache request")
	}
	var authority atomic.Value
	authority.Store("v1")
	if got := dg6LoadAuthorizationSnapshot(t, ctx, governedA, request, &authority); got != "v1" {
		t.Fatalf("warm A authorization context = %q, want v1", got)
	}
	if got := dg6LoadAuthorizationSnapshot(t, ctx, governedB, request, &authority); got != "v1" {
		t.Fatalf("warm B authorization context = %q, want v1", got)
	}

	beforeOutbox := dg5OutboxOwnerCount(t, ctx, target)
	err = cachegovfacade.RunInvalidatedMutationClasses(ctx,
		target.provider.Transactor().WithinTransaction,
		target.provider.Transactor(),
		relayA,
		[]cachepolicy.DataClass{cachepolicy.DataClassAuthorizationContext, cachepolicy.DataClassAuthorizationMenus},
		func(context.Context) (bool, error) {
			authority.Store("v2")
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("commit DG6 authorization mutation with durable invalidation: %v", err)
	}
	if after := dg5OutboxOwnerCount(t, ctx, target); after != beforeOutbox+2 {
		t.Fatalf("committed DG6 authorization mutation outbox rows=%d, want %d", after, beforeOutbox+2)
	}
	dg6AssertAuthorizationOutboxContentFree(t, ctx, target)
	// No relay has run. B's old L1/L2 candidate must be rejected by the real
	// source-adjacent fence and reloaded from authority as v2.
	if got := dg6LoadAuthorizationSnapshot(t, ctx, governedB, request, &authority); got != "v2" {
		t.Fatalf("B returned stale authorization snapshot before relay: got %q, want v2", got)
	}

	dg5RelayAll(t, ctx, relayA)
	waitDG5(t, "DG6 B fanout converges after durable authorization relay", func() bool {
		return governedB.GovernedStatus().FanoutHealthy && dg5OutboxStatusCount(t, ctx, target, "DONE") >= 2
	})
	if got := dg6LoadAuthorizationSnapshot(t, ctx, governedB, request, &authority); got != "v2" {
		t.Fatalf("B authorization snapshot after relay = %q, want v2", got)
	}

	beforeRollback := dg5OutboxOwnerCount(t, ctx, target)
	rollback := errors.New("DG6 acceptance rollback")
	err = cachegovfacade.RunInvalidatedMutationClasses(ctx,
		target.provider.Transactor().WithinTransaction,
		target.provider.Transactor(),
		relayA,
		[]cachepolicy.DataClass{cachepolicy.DataClassAuthorizationContext, cachepolicy.DataClassAuthorizationMenus},
		func(context.Context) (bool, error) { return false, rollback },
	)
	if !errors.Is(err, rollback) {
		t.Fatalf("force DG6 authorization rollback: %v", err)
	}
	if after := dg5OutboxOwnerCount(t, ctx, target); after != beforeRollback {
		t.Fatalf("rolled-back DG6 authorization mutation left outbox rows: before=%d after=%d", beforeRollback, after)
	}

	// A broker health doubt cannot authorize from a warmed B L1. This models a
	// consumer connection whose confirmed delivery state is unknown.
	authority.Store("v3-rabbit-fallback")
	governedB.SetFanoutHealthy(false)
	if got := dg6LoadAuthorizationSnapshot(t, ctx, governedB, request, &authority); got != "v3-rabbit-fallback" {
		t.Fatalf("Rabbit uncertainty reused an authorization snapshot: got %q", got)
	}
	governedB.SetFanoutHealthy(true)

	// Closing only B's test Redis client must also bypass B's old L1; this
	// does not stop or modify the local Redis service and keeps the acceptance
	// harness isolated from unrelated users.
	authority.Store("v4-redis-fallback")
	if err := redisB.Close(); err != nil {
		t.Fatalf("close DG6 B Redis client for fail-closed probe: %v", err)
	}
	if got := dg6LoadAuthorizationSnapshot(t, ctx, governedB, request, &authority); got != "v4-redis-fallback" {
		t.Fatalf("Redis uncertainty reused an authorization snapshot: got %q", got)
	}
}

// TestDG6RealAuthorizationServicesRejectWarmPeerBeforeRelay closes the
// application-level counterpart of the generic cache-protocol acceptance. It
// deliberately keeps the relay idle. B is the first reader under a unique
// Redis prefix, so the test proves B source-loads both context/menu into its
// own L1/L2 layers. It then removes only B's exact L2 payloads and proves B's
// next real reads leave L2 absent: a source reload would restore it, so those
// reads distinguish a real B-L1 hit. A then performs a real role mutation. B
// must observe the committed authority result while the durable invalidations
// remain non-DONE; a direct protocol call or synthetic authority value is not
// part of this test.
func TestDG6RealAuthorizationServicesRejectWarmPeerBeforeRelay(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply after the isolated migration path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)
	fixture := dg6SeedAuthorizationFixture(t, ctx, target)
	t.Cleanup(func() { dg6DeleteAuthorizationFixture(t, context.Background(), target, fixture) })

	prefix := fmt.Sprintf("seven-dg6-real-auth-%s-%d", target.dialect, time.Now().UTC().UnixNano())
	managerA, governedA, redisA := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "a")
	managerB, governedB, redisB := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "b")
	t.Cleanup(func() { _ = redisA.Close() })
	t.Cleanup(func() { _ = redisB.Close() })

	rabbitCfg := dg5AcceptanceRabbitConfig(target.cfg)
	rabbitA, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ for real authorization instance A: %v", err)
	}
	t.Cleanup(func() { _ = rabbitA.Close() })
	rabbitB, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ for real authorization instance B: %v", err)
	}
	t.Cleanup(func() { _ = rabbitB.Close() })

	generationA := cachegovinfra.NewGenerationAdapter(governedA)
	generationB := cachegovinfra.NewGenerationAdapter(governedB)
	brokerA, err := cachegovinfra.NewFanoutAdapter(rabbitA, generationA, prefix+"-fanout-a", true)
	if err != nil {
		t.Fatalf("create real authorization fanout A: %v", err)
	}
	brokerB, err := cachegovinfra.NewFanoutAdapter(rabbitB, generationB, prefix+"-fanout-b", true)
	if err != nil {
		t.Fatalf("create real authorization fanout B: %v", err)
	}
	// The actual broker topology is available, but consumers are intentionally
	// not started: this is the pre-relay counterexample. Marking the local
	// health state ready allows the real governed layers to admit their warm
	// candidates; the non-DONE source event remains the freshness authority.
	generationA.SetFanoutHealthy(true)
	generationB.SetFanoutHealthy(true)
	idsA, err := xid.New(81)
	if err != nil {
		t.Fatalf("create real authorization instance-A id generator: %v", err)
	}
	idsB, err := xid.New(82)
	if err != nil {
		t.Fatalf("create real authorization instance-B id generator: %v", err)
	}
	outboxA := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), idsA.NextID)
	outboxB := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), idsB.NextID)
	governedA.SetFreshnessGate(outboxA)
	governedB.SetFreshnessGate(outboxB)
	relayA := cachegovapp.NewService(outboxA, generationA, brokerA, outboxA, "dg6-real-auth-relay-"+target.dialect+"-a")
	relayB := cachegovapp.NewService(outboxB, generationB, brokerB, outboxB, "dg6-real-auth-relay-"+target.dialect+"-b")
	if !relayA.Enabled() || !relayB.Enabled() {
		t.Fatal("real authorization cache relays must be configured")
	}

	users, err := userinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatalf("create real authorization user repository: %v", err)
	}
	userService := userapp.NewService(users, userdomain.NewService(), nil, nil, userapp.WithTransactor(target.provider.Transactor()))
	authRepoA, err := authinfra.NewRepository(target.provider, userService)
	if err != nil {
		t.Fatalf("create real authorization repository A: %v", err)
	}
	authRepoB, err := authinfra.NewRepository(target.provider, userService)
	if err != nil {
		t.Fatalf("create real authorization repository B: %v", err)
	}
	serviceA := authapp.NewService(target.cfg.Authorization, managerA, target.provider.Transactor(), authRepoA, authdomain.NewService(), idsA, nil, nil, nil, nil)
	recordingB := newRecordingGovernedManager(managerB, governedB)
	serviceB := authapp.NewService(target.cfg.Authorization, recordingB, target.provider.Transactor(), authRepoB, authdomain.NewService(), idsB, nil, nil, nil, nil)
	serviceA.BindCacheInvalidations(relayA)
	serviceB.BindCacheInvalidations(relayB)

	// NewService receives no feature set in this isolated fixture, so mirror
	// the production service's stable nil-feature fingerprint exactly.
	featureFingerprint := cachepolicy.EventDigest("authorization-features:none")
	contextRequest, ok := cachepolicy.AuthorizationContextReadRequest(fixture.userID, featureFingerprint)
	if !ok {
		t.Fatal("construct real authorization context cache request")
	}
	menuRequest, ok := cachepolicy.AuthorizationMenuReadRequest(fixture.userID, featureFingerprint)
	if !ok {
		t.Fatal("construct real authorization menu cache request")
	}
	// This prefix is unique to this test invocation. Proving both payloads are
	// absent before B reads prevents an L2 hit from disguising B's L1 as warm.
	dg6AssertNoAuthorizationCandidate(t, ctx, managerB, redisB, contextRequest)
	dg6AssertNoAuthorizationCandidate(t, ctx, managerB, redisB, menuRequest)

	contextB, err := serviceB.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-real-auth")
	if err != nil || !dg6HasPermission(contextB.Permissions, fixture.readPermission) {
		t.Fatalf("source-load real authorization context B=%#v err=%v", contextB, err)
	}
	menusB, err := serviceB.GetCurrentUserMenus(ctx, fixture.userID)
	if err != nil || len(menusB) != 1 || menusB[0].Permission != fixture.readPermission {
		t.Fatalf("source-load real authorization menus B=%#v err=%v", menusB, err)
	}
	dg6AssertWarmAuthorizationCandidate(t, ctx, managerB, redisB, contextRequest)
	dg6AssertWarmAuthorizationCandidate(t, ctx, managerB, redisB, menuRequest)
	if loads := recordingB.LoaderCalls(); loads != 2 {
		t.Fatalf("B initial source load callback count=%d want=2", loads)
	}

	// Delete only this unique test's L2 payloads. The generation and B's service
	// (including its Ristretto L1) remain untouched. A source reload below is
	// cacheable and would repopulate L2, so persistent absence distinguishes a
	// genuine B-L1 hit without a production inspection hook.
	dg6DeleteAuthorizationCandidate(t, ctx, managerB, redisB, contextRequest)
	dg6DeleteAuthorizationCandidate(t, ctx, managerB, redisB, menuRequest)
	dg6AssertNoAuthorizationCandidate(t, ctx, managerB, redisB, contextRequest)
	dg6AssertNoAuthorizationCandidate(t, ctx, managerB, redisB, menuRequest)
	recordingB.ResetObservations()

	contextB, err = serviceB.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-real-auth")
	if err != nil || !dg6HasPermission(contextB.Permissions, fixture.readPermission) {
		t.Fatalf("B L1-hit authorization context=%#v err=%v", contextB, err)
	}
	menusB, err = serviceB.GetCurrentUserMenus(ctx, fixture.userID)
	if err != nil || len(menusB) != 1 || menusB[0].Permission != fixture.readPermission {
		t.Fatalf("B L1-hit authorization menus=%#v err=%v", menusB, err)
	}
	dg6AssertNoAuthorizationCandidate(t, ctx, managerB, redisB, contextRequest)
	dg6AssertNoAuthorizationCandidate(t, ctx, managerB, redisB, menuRequest)
	if loads := recordingB.LoaderCalls(); loads != 0 {
		t.Fatalf("B pre-mutation reads re-executed real source loader=%d want=0", loads)
	}
	dg6AssertGovernedReadResults(t, recordingB, contextRequest.Entry.DataClass, menuRequest.Entry.DataClass)

	before := dg5OutboxOwnerCount(t, ctx, target)
	beforePending := dg5OutboxStatusCount(t, ctx, target, "PENDING")
	disabled, dataScope := 1, 1
	if _, err := serviceA.UpdateRole(ctx, authfacade.RoleCommand{
		ID: fixture.roleID, Name: fixture.roleName, Code: fixture.roleCode, Type: "CUSTOM",
		Status: &disabled, DataScope: &dataScope, OperatorID: fixture.userID,
	}); err != nil {
		t.Fatalf("real application role mutation: %v", err)
	}
	if after := dg5OutboxOwnerCount(t, ctx, target); after != before+2 {
		t.Fatalf("real role mutation outbox rows=%d want=%d", after, before+2)
	}
	if pending := dg5OutboxStatusCount(t, ctx, target, "PENDING"); pending != beforePending+2 {
		t.Fatalf("real role mutation pending outbox rows=%d want=%d before any relay", pending, beforePending+2)
	}

	// No relay is invoked before these reads. B must acquire the source fence,
	// see the two PENDING invalidations, and re-execute each real source loader
	// instead of silently reusing its proven-warm L1 entries.
	recordingB.ResetObservations()
	contextB, err = serviceB.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-real-auth")
	if err != nil {
		t.Fatalf("peer B context after committed role mutation: %v", err)
	}
	if dg6HasPermission(contextB.Permissions, fixture.readPermission) {
		t.Fatalf("peer B returned stale permission before relay: %#v", contextB.Permissions)
	}
	menusB, err = serviceB.GetCurrentUserMenus(ctx, fixture.userID)
	if err != nil {
		t.Fatalf("peer B menus after committed role mutation: %v", err)
	}
	if len(menusB) != 0 {
		t.Fatalf("peer B returned stale menu before relay: %#v", menusB)
	}
	if loads := recordingB.LoaderCalls(); loads != 2 {
		t.Fatalf("B post-mutation reads executed real source loader=%d want=2", loads)
	}
	dg6AssertGovernedReadResults(t, recordingB, contextRequest.Entry.DataClass, menuRequest.Entry.DataClass)
	if pending := dg5OutboxStatusCount(t, ctx, target, "PENDING"); pending != beforePending+2 {
		t.Fatalf("B authority reload unexpectedly relayed invalidations: pending=%d want=%d", pending, beforePending+2)
	}
}

// TestDG6AuthorizationReadFixtureAgainstBothDialects drives real repository
// queries over a deliberately tiny, dialect-safe authorization fixture. It
// complements the two-instance protocol test above: role/permission/menu,
// temporary grant state, expiry/revoke, and a locked user must affect the
// authority result rather than depend on a best-effort local delete.
func TestDG6AuthorizationReadFixtureAgainstBothDialects(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(dg6AuthorizationAcceptanceEnv)), dg5AcceptanceApply) {
		t.Skip("set DG6_AUTHORIZATION_GOVERNANCE_ACCEPTANCE=apply after the isolated migration path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	assertDG5AcceptanceSchema(t, ctx, target)
	fixture := dg6SeedAuthorizationFixture(t, ctx, target)
	t.Cleanup(func() { dg6DeleteAuthorizationFixture(t, context.Background(), target, fixture) })

	users, err := userinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatalf("create DG6 real user repository: %v", err)
	}
	userService := userapp.NewService(users, userdomain.NewService(), nil, nil, userapp.WithTransactor(target.provider.Transactor()))
	authRepo, err := authinfra.NewRepository(target.provider, userService)
	if err != nil {
		t.Fatalf("create DG6 real authorization repository: %v", err)
	}
	ids, err := xid.New(73)
	if err != nil {
		t.Fatalf("create DG6 real authorization id generator: %v", err)
	}
	service := authapp.NewService(target.cfg.Authorization, nil, target.provider.Transactor(), authRepo, authdomain.NewService(), ids, nil, nil, nil, nil)

	contextV1, err := service.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-fixture")
	if err != nil || !dg6HasPermission(contextV1.Permissions, fixture.readPermission) {
		t.Fatalf("real fixture initial authorization context=%#v err=%v", contextV1, err)
	}
	menus, err := service.GetCurrentUserMenus(ctx, fixture.userID)
	if err != nil || len(menus) != 1 || menus[0].Permission != fixture.readPermission {
		t.Fatalf("real fixture initial authorization menus=%#v err=%v", menus, err)
	}

	dg6ExecFixture(t, ctx, target,
		`INSERT INTO sys_user_permission (id,userId,permissionId,type,expireTime,source,reason,createTime,updateTime,isDeleted) VALUES (?,?,?,TRUE,?,'DG6','acceptance',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0)`,
		`INSERT INTO sys_user_permission (id,"userId","permissionId",type,"expireTime",source,reason,"createTime","updateTime","isDeleted") VALUES (?,?,?,TRUE,?,'DG6','acceptance',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,FALSE)`,
		fixture.tempGrantID, fixture.userID, fixture.tempPermissionID, time.Now().UTC().Add(time.Hour))
	withTemporary, err := service.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-fixture")
	if err != nil || !dg6HasPermission(withTemporary.Permissions, fixture.tempPermission) {
		t.Fatalf("active temporary grant must be present in authoritative context: %#v err=%v", withTemporary, err)
	}
	dg6ExecFixture(t, ctx, target,
		`DELETE FROM sys_user_permission WHERE id=?`, `DELETE FROM sys_user_permission WHERE id=?`, fixture.tempGrantID)
	afterRevoke, err := service.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-fixture")
	if err != nil || dg6HasPermission(afterRevoke.Permissions, fixture.tempPermission) {
		t.Fatalf("revoked temporary grant must not survive authority read: %#v err=%v", afterRevoke, err)
	}

	dg6ExecFixture(t, ctx, target,
		`UPDATE sys_role_permission SET permissionId=? WHERE id=?`, `UPDATE sys_role_permission SET "permissionId"=? WHERE id=?`, fixture.tempPermissionID, fixture.rolePermissionID)
	afterRoleChange, err := service.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-fixture")
	if err != nil || !dg6HasPermission(afterRoleChange.Permissions, fixture.tempPermission) {
		t.Fatalf("role permission mutation was not reflected by authority context: %#v err=%v", afterRoleChange, err)
	}
	dg6ExecFixture(t, ctx, target, `UPDATE sys_menu SET status=1 WHERE id=?`, `UPDATE sys_menu SET status=1 WHERE id=?`, fixture.menuID)
	menus, err = service.GetCurrentUserMenus(ctx, fixture.userID)
	if err != nil || len(menus) != 0 {
		t.Fatalf("disabled menu must not survive authority menu read: %#v err=%v", menus, err)
	}
	dg6ExecFixture(t, ctx, target, `UPDATE sys_user SET status=1,unsealTime=? WHERE id=?`, `UPDATE sys_user SET status=1,"unsealTime"=? WHERE id=?`, time.Now().UTC().Add(time.Hour), fixture.userID)
	if _, err := service.BuildUserContext(ctx, fixture.userID, "", nil, nil, "dg6-fixture"); err == nil {
		t.Fatal("locked user must not receive an authorization context from source")
	}
}

type dg6AuthorizationFixture struct {
	userID, roleID, readPermissionID, tempPermissionID, menuID, rolePermissionID, tempGrantID int64
	readPermission, tempPermission, roleName, roleCode                                        string
}

func dg6SeedAuthorizationFixture(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) dg6AuthorizationFixture {
	t.Helper()
	base := time.Now().UTC().UnixNano() / 1000
	f := dg6AuthorizationFixture{userID: base, roleID: base + 1, readPermissionID: base + 2, tempPermissionID: base + 3, menuID: base + 4, rolePermissionID: base + 5, tempGrantID: base + 6, readPermission: "dg6:read:" + fmt.Sprint(base), tempPermission: "dg6:temporary:" + fmt.Sprint(base), roleName: "DG6 role", roleCode: "DG6_" + fmt.Sprint(base)}
	dg6ExecFixture(t, ctx, target, `INSERT INTO sys_user (id,userAccount,nickName,status,userEmail,createTime,updateTime,isDeleted) VALUES (?,?,?,0,'',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0)`, `INSERT INTO sys_user (id,"userAccount","nickName",status,"userEmail","createTime","updateTime","isDeleted") VALUES (?,?,?,0,'',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,FALSE)`, f.userID, "dg6u"+fmt.Sprint(base%100000000), "DG6")
	dg6ExecFixture(t, ctx, target, `INSERT INTO sys_role (id,name,code,dataScope,status,type,createTime,updateTime,isDeleted) VALUES (?,?,?,1,0,3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0)`, `INSERT INTO sys_role (id,name,code,"dataScope",status,type,"createTime","updateTime","isDeleted") VALUES (?,?,?,1,0,3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,FALSE)`, f.roleID, f.roleName, f.roleCode)
	for _, p := range []struct {
		id   int64
		code string
	}{{f.readPermissionID, f.readPermission}, {f.tempPermissionID, f.tempPermission}} {
		dg6ExecFixture(t, ctx, target, `INSERT INTO sys_permission (id,code,name,resourceType,status,createTime,updateTime,isDeleted) VALUES (?,?,?,'API',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0)`, `INSERT INTO sys_permission (id,code,name,"resourceType",status,"createTime","updateTime","isDeleted") VALUES (?,?,?,'API',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,FALSE)`, p.id, p.code, p.code)
	}
	dg6ExecFixture(t, ctx, target, `INSERT INTO sys_menu (id,name,type,permission,status,visible,isFrame,isCache,createTime,updateTime,isDeleted) VALUES (?,?,'C',?,0,1,0,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0)`, `INSERT INTO sys_menu (id,name,type,permission,status,visible,"isFrame","isCache","createTime","updateTime","isDeleted") VALUES (?,?,'C',?,0,TRUE,FALSE,FALSE,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,FALSE)`, f.menuID, "DG6 menu", f.readPermission)
	dg6ExecFixture(t, ctx, target, `INSERT INTO sys_user_role (id,userId,roleId,createTime,updateTime,isDeleted) VALUES (?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0)`, `INSERT INTO sys_user_role (id,"userId","roleId","createTime","updateTime","isDeleted") VALUES (?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,FALSE)`, base+7, f.userID, f.roleID)
	dg6ExecFixture(t, ctx, target, `INSERT INTO sys_role_permission (id,roleId,permissionId,createTime,updateTime) VALUES (?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, `INSERT INTO sys_role_permission (id,"roleId","permissionId","createTime","updateTime") VALUES (?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, f.rolePermissionID, f.roleID, f.readPermissionID)
	dg6ExecFixture(t, ctx, target, `INSERT INTO sys_role_menu (id,roleId,menuId,createTime,updateTime) VALUES (?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, `INSERT INTO sys_role_menu (id,"roleId","menuId","createTime","updateTime") VALUES (?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, base+8, f.roleID, f.menuID)
	return f
}

func dg6DeleteAuthorizationFixture(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, f dg6AuthorizationFixture) {
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_user_permission WHERE userId=?`, `DELETE FROM sys_user_permission WHERE "userId"=?`, f.userID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_role_permission WHERE roleId=?`, `DELETE FROM sys_role_permission WHERE "roleId"=?`, f.roleID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_role_menu WHERE roleId=?`, `DELETE FROM sys_role_menu WHERE "roleId"=?`, f.roleID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_user_role WHERE userId=?`, `DELETE FROM sys_user_role WHERE "userId"=?`, f.userID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_menu WHERE id=?`, `DELETE FROM sys_menu WHERE id=?`, f.menuID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_permission WHERE id=?`, `DELETE FROM sys_permission WHERE id=?`, f.readPermissionID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_permission WHERE id=?`, `DELETE FROM sys_permission WHERE id=?`, f.tempPermissionID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_role WHERE id=?`, `DELETE FROM sys_role WHERE id=?`, f.roleID)
	dg6ExecFixture(t, ctx, target, `DELETE FROM sys_user WHERE id=?`, `DELETE FROM sys_user WHERE id=?`, f.userID)
}

func dg6ExecFixture(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, mysql, postgres string, args ...any) {
	t.Helper()
	statement := mysql
	if target.dialect == "postgres" {
		statement = postgres
	}
	if _, err := target.provider.SQLX().ExecContext(ctx, target.provider.SQLX().Rebind(statement), args...); err != nil {
		t.Fatalf("DG6 fixture statement failed: %v", err)
	}
}
func dg6HasPermission(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type dg6AuthorizationSnapshot struct {
	Version string `json:"version"`
}

func dg6LoadAuthorizationSnapshot(t *testing.T, ctx context.Context, governed cacheinfra.GovernedCache, request cachepolicy.ReadRequest, authority *atomic.Value) string {
	t.Helper()
	if governed == nil || authority == nil {
		t.Fatal("DG6 authorization snapshot loader requires governed cache and authority")
	}
	var result dg6AuthorizationSnapshot
	_, err := governed.GetOrLoadClassified(ctx, request, &result, func(context.Context) (cachepolicy.CacheableValue, error) {
		value, _ := authority.Load().(string)
		return cachepolicy.CacheableValue{Value: dg6AuthorizationSnapshot{Version: value}, Cacheable: true}, nil
	})
	if err != nil {
		t.Fatalf("load DG6 authorization snapshot: %v", err)
	}
	return result.Version
}

func dg6AssertWarmAuthorizationCandidate(t *testing.T, ctx context.Context, manager cacheinfra.Manager, provider cacheinfra.Provider, request cachepolicy.ReadRequest) {
	t.Helper()
	if manager == nil || provider == nil || provider.Client() == nil {
		t.Fatal("real authorization warm-candidate assertion requires cache manager and Redis provider")
	}
	generationKey := manager.Builder().Build("dg5", "generation", cachepolicy.ClassTargetDigest(request.Entry.DataClass))
	generation, err := provider.Client().Get(ctx, generationKey).Result()
	if err != nil || strings.TrimSpace(generation) == "" {
		t.Fatalf("real authorization warm candidate has no Redis generation: generation=%t err=%v", strings.TrimSpace(generation) != "", err)
	}
	payloadKey := dg5PayloadKeyForCurrentEpoch(t, ctx, manager, provider, request, generation)
	exists, err := provider.Client().Exists(ctx, payloadKey).Result()
	if err != nil || exists != 1 {
		t.Fatalf("real authorization warm candidate missing Redis L2 payload: exists=%d err=%v", exists, err)
	}
}

func dg6AssertNoAuthorizationCandidate(t *testing.T, ctx context.Context, manager cacheinfra.Manager, provider cacheinfra.Provider, request cachepolicy.ReadRequest) {
	t.Helper()
	if manager == nil || provider == nil || provider.Client() == nil {
		t.Fatal("real authorization cold-candidate assertion requires cache manager and Redis provider")
	}
	generationKey := manager.Builder().Build("dg5", "generation", cachepolicy.ClassTargetDigest(request.Entry.DataClass))
	generation, err := provider.Client().Get(ctx, generationKey).Result()
	if err == nil && strings.TrimSpace(generation) != "" {
		payloadKey := dg5PayloadKeyForCurrentEpoch(t, ctx, manager, provider, request, generation)
		exists, existsErr := provider.Client().Exists(ctx, payloadKey).Result()
		if existsErr != nil || exists != 0 {
			t.Fatalf("real authorization candidate was warm before B source load: exists=%d err=%v", exists, existsErr)
		}
		return
	}
	if !errors.Is(err, redis.Nil) {
		t.Fatalf("read Redis generation before B source load: %v", err)
	}
}

func dg6DeleteAuthorizationCandidate(t *testing.T, ctx context.Context, manager cacheinfra.Manager, provider cacheinfra.Provider, request cachepolicy.ReadRequest) {
	t.Helper()
	if manager == nil || provider == nil || provider.Client() == nil {
		t.Fatal("real authorization L2-delete assertion requires cache manager and Redis provider")
	}
	generationKey := manager.Builder().Build("dg5", "generation", cachepolicy.ClassTargetDigest(request.Entry.DataClass))
	generation, err := provider.Client().Get(ctx, generationKey).Result()
	if err != nil || strings.TrimSpace(generation) == "" {
		t.Fatalf("read Redis generation before deleting exact authorization L2 payload: generation=%t err=%v", strings.TrimSpace(generation) != "", err)
	}
	payloadKey := dg5PayloadKeyForCurrentEpoch(t, ctx, manager, provider, request, generation)
	deleted, err := provider.Client().Del(ctx, payloadKey).Result()
	if err != nil || deleted != 1 {
		t.Fatalf("delete exact authorization L2 payload: deleted=%d err=%v", deleted, err)
	}
}

// recordingGovernedManager is a test-only observation wrapper. Embedding the
// real Manager preserves every ordinary cache call; every GovernedCache method
// delegates to the real governed layer. Only the application-supplied loader
// callback is counted, so this cannot prime, inspect, or otherwise alter L1.
type recordingGovernedManager struct {
	cacheinfra.Manager
	governed    cacheinfra.GovernedCache
	loaderCalls atomic.Int64
	mu          sync.Mutex
	results     []recordingGovernedResult
}

type recordingGovernedResult struct {
	dataClass cachepolicy.DataClass
	returned  bool
	errNil    bool
}

var _ cacheinfra.Manager = (*recordingGovernedManager)(nil)
var _ cacheinfra.GovernedCache = (*recordingGovernedManager)(nil)

func newRecordingGovernedManager(manager cacheinfra.Manager, governed cacheinfra.GovernedCache) *recordingGovernedManager {
	return &recordingGovernedManager{Manager: manager, governed: governed}
}

func (m *recordingGovernedManager) GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader cacheinfra.ClassifiedLoader) (bool, error) {
	returned, err := m.governed.GetOrLoadClassified(ctx, request, dest, m.record(loader))
	m.recordResult(request.Entry.DataClass, returned, err)
	return returned, err
}

func (m *recordingGovernedManager) GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight cacheinfra.ClassifiedPreflight, loader cacheinfra.ClassifiedLoader) (bool, error) {
	returned, err := m.governed.GetOrLoadClassifiedWithPreflight(ctx, request, dest, preflight, m.record(loader))
	m.recordResult(request.Entry.DataClass, returned, err)
	return returned, err
}

func (m *recordingGovernedManager) MarkLocalDirty(eventID string, classes ...cachepolicy.DataClass) {
	m.governed.MarkLocalDirty(eventID, classes...)
}

func (m *recordingGovernedManager) EvictLocalAndResolve(eventID string, classes ...cachepolicy.DataClass) {
	m.governed.EvictLocalAndResolve(eventID, classes...)
}

func (m *recordingGovernedManager) AdvanceGeneration(ctx context.Context, eventID string, class cachepolicy.DataClass) (bool, error) {
	return m.governed.AdvanceGeneration(ctx, eventID, class)
}

func (m *recordingGovernedManager) SetFanoutHealthy(healthy bool) {
	m.governed.SetFanoutHealthy(healthy)
}

func (m *recordingGovernedManager) SetFreshnessGate(gate cachepolicy.FreshnessGate) {
	m.governed.SetFreshnessGate(gate)
}

func (m *recordingGovernedManager) RecordRejectedFanout() {
	m.governed.RecordRejectedFanout()
}

func (m *recordingGovernedManager) GovernedStatus() cacheinfra.GovernedStatus {
	return m.governed.GovernedStatus()
}

func (m *recordingGovernedManager) LoaderCalls() int64 {
	return m.loaderCalls.Load()
}

func (m *recordingGovernedManager) ResetObservations() {
	m.loaderCalls.Store(0)
	m.mu.Lock()
	m.results = nil
	m.mu.Unlock()
}

func (m *recordingGovernedManager) record(loader cacheinfra.ClassifiedLoader) cacheinfra.ClassifiedLoader {
	if loader == nil {
		return nil
	}
	return func(ctx context.Context) (cachepolicy.CacheableValue, error) {
		m.loaderCalls.Add(1)
		return loader(ctx)
	}
}

func (m *recordingGovernedManager) recordResult(dataClass cachepolicy.DataClass, returned bool, err error) {
	m.mu.Lock()
	m.results = append(m.results, recordingGovernedResult{dataClass: dataClass, returned: returned, errNil: err == nil})
	m.mu.Unlock()
}

func (m *recordingGovernedManager) SnapshotResults() []recordingGovernedResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]recordingGovernedResult(nil), m.results...)
}

func dg6AssertGovernedReadResults(t *testing.T, recorder *recordingGovernedManager, expectedClasses ...cachepolicy.DataClass) {
	t.Helper()
	if recorder == nil {
		t.Fatal("recording governed manager is required")
	}
	results := recorder.SnapshotResults()
	if len(results) != len(expectedClasses) {
		t.Fatalf("governed result count=%d want=%d results=%#v", len(results), len(expectedClasses), results)
	}
	expected := make(map[cachepolicy.DataClass]int, len(expectedClasses))
	for _, class := range expectedClasses {
		expected[class]++
	}
	for _, result := range results {
		if !result.returned || !result.errNil {
			t.Fatalf("governed read did not return trusted candidate: %#v", result)
		}
		expected[result.dataClass]--
	}
	for class, count := range expected {
		if count != 0 {
			t.Fatalf("governed read classes did not match: remaining=%s:%d results=%#v", class, count, results)
		}
	}
}

func dg6AssertAuthorizationOutboxContentFree(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) {
	t.Helper()
	if target == nil || target.provider == nil {
		t.Fatal("DG6 authorization outbox probe requires an isolated database target")
	}
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	query := `SELECT payload, aggregateId FROM sys_outbox_event WHERE eventOwner=? AND scopeId=? AND aggregateType=?`
	if target.dialect == "postgres" {
		query = `SELECT payload, "aggregateId" FROM sys_outbox_event WHERE "eventOwner"=? AND "scopeId"=? AND "aggregateType"=?`
	}
	rows, err := target.provider.SQLX().QueryxContext(ctx, target.provider.SQLX().Rebind(query), cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.CacheInvalidationAggregate)
	if err != nil {
		t.Fatalf("read isolated DG6 authorization outbox content: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload, aggregateID string
		if err := rows.Scan(&payload, &aggregateID); err != nil {
			t.Fatalf("scan isolated DG6 authorization outbox content: %v", err)
		}
		content := strings.ToLower(payload + "\n" + aggregateID)
		for _, forbidden := range []string{
			"user:1001", "permission", "temporary", "grant", "bearer", "token", "cookie", "session", "password", "roleid",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("isolated DG6 authorization outbox leaked forbidden content %q", forbidden)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate isolated DG6 authorization outbox content: %v", err)
	}
}

func dg6FeatureFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
