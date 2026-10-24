package acceptance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cachegovapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/application"
	cachegovdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/domain"
	cachegovinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/infrastructure"
	configapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/application"
	configdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	configinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/infrastructure"
	dictapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/application"
	dictdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	dictfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/facade"
	dictinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/infrastructure"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	msgoutbox "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/outbox"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	sharedconfig "github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/bytedance/sonic"
	"github.com/jmoiron/sqlx"
	amqp "github.com/rabbitmq/amqp091-go"
	redisclient "github.com/redis/go-redis/v9"
)

const dg5AcceptanceApply = "apply"

// TestDG5CacheGovernanceAcceptance uses the real config/dict application
// services, two independent Ristretto instances, the local Redis service, the
// shared sys_outbox_event table, and local RabbitMQ fanout. It is opt-in so
// normal tests never touch an external service; openDG5IsolatedDatabase still
// rejects any database other than the two governance names before a write.
func TestDG5CacheGovernanceAcceptance(t *testing.T) {
	if strings.ToLower(strings.TrimSpace(os.Getenv(dg5AcceptanceEnv))) != dg5AcceptanceApply {
		t.Skip("set DG5_CACHE_GOVERNANCE_ACCEPTANCE=apply after the isolated migration path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)
	dg5ResetAcceptanceFixtures(t, ctx, target)

	baseID := time.Now().UTC().UnixNano()
	prefix := fmt.Sprintf("seven-dg5-%s-%d", target.dialect, baseID)
	managerA, governedA, redisA := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "a")
	managerB, governedB, redisB := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "b")
	t.Cleanup(func() { _ = redisB.Close() })
	t.Cleanup(func() { _ = redisA.Close() })

	rabbitCfg := dg5AcceptanceRabbitConfig(target.cfg)
	rabbitA, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ for DG5 instance A: %v", err)
	}
	t.Cleanup(func() { _ = rabbitA.Close() })
	rabbitB, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ for DG5 instance B: %v", err)
	}
	t.Cleanup(func() { _ = rabbitB.Close() })

	generationA := cachegovinfra.NewGenerationAdapter(governedA)
	generationB := cachegovinfra.NewGenerationAdapter(governedB)
	dg5AssertVersionedFanoutTopologyUpgradesLegacyQueue(t, rabbitA, generationA, rabbitCfg, prefix+"-fanout-a")
	brokerA, err := cachegovinfra.NewFanoutAdapter(rabbitA, generationA, prefix+"-fanout-a", true)
	if err != nil {
		t.Fatalf("create DG5 RabbitMQ fanout A: %v", err)
	}
	brokerB, err := cachegovinfra.NewFanoutAdapter(rabbitB, generationB, prefix+"-fanout-b", true)
	if err != nil {
		t.Fatalf("create DG5 RabbitMQ fanout B: %v", err)
	}
	// Queues use opaque, run-specific instance digests and expire through the
	// DG5 topology policy. Acceptance never receives a generic destructive
	// broker capability merely to tidy test state.

	outboxIDsA, err := xid.New(67)
	if err != nil {
		t.Fatalf("create DG5 isolated instance-A outbox id generator: %v", err)
	}
	outboxIDsB, err := xid.New(68)
	if err != nil {
		t.Fatalf("create DG5 isolated instance-B outbox id generator: %v", err)
	}
	outboxA := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), outboxIDsA.NextID)
	outboxB := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), outboxIDsB.NextID)
	// Acceptance composes two module-like instances directly, so install the
	// same source-adjacent gate that Module.Install wires in production before
	// any classified read can become eligible.
	governedA.SetFreshnessGate(outboxA)
	governedB.SetFreshnessGate(outboxB)
	dg5AssertStrictScopeClaimFence(t, ctx, target, outboxA, outboxIDsA.NextID)
	relayA := cachegovapp.NewService(outboxA, generationA, brokerA, outboxA, "dg5-relay-"+target.dialect+"-a")
	relayB := cachegovapp.NewService(outboxB, generationB, brokerB, outboxB, "dg5-relay-"+target.dialect+"-b")
	if !relayA.Enabled() || !relayB.Enabled() {
		t.Fatal("DG5 relay services must be fully configured")
	}
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	t.Cleanup(consumerCancel)
	startDG5Consumer(t, consumerCtx, brokerA, relayA, governedA)
	startDG5Consumer(t, consumerCtx, brokerB, relayB, governedB)
	waitDG5(t, "both DG5 fanout consumers become healthy", func() bool {
		return governedA.GovernedStatus().FanoutHealthy && governedB.GovernedStatus().FanoutHealthy
	})

	configRepo, err := configinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatalf("create config repository: %v", err)
	}
	dictRepo, err := dictinfra.NewRepository(target.provider)
	if err != nil {
		t.Fatalf("create dictionary repository: %v", err)
	}
	configA := configapp.NewService(target.provider.Transactor(), configRepo, configinfra.NewCacheStore(managerA, true), configdomain.NewService(), dg5AcceptanceSecretCipher{}, nil, nil, relayA)
	configB := configapp.NewService(target.provider.Transactor(), configRepo, configinfra.NewCacheStore(managerB, true), configdomain.NewService(), dg5AcceptanceSecretCipher{}, nil, nil, relayB)
	dictA := dictapp.NewService(target.provider.Transactor(), dictRepo, dictinfra.NewCacheStore(managerA, true), dictdomain.NewService(), relayA)
	dictB := dictapp.NewService(target.provider.Transactor(), dictRepo, dictinfra.NewCacheStore(managerB, true), dictdomain.NewService(), relayB)
	configWriter := configapp.Actor{UserID: baseID + 1, IsAdmin: true, Authenticated: true, AccountID: baseID + 1, ScopeID: "dg5-admin", AuthzVersion: 1}
	dictWriter := dictapp.Actor{UserID: baseID + 1, IsAdmin: true, Authenticated: true, AccountID: baseID + 1, ScopeID: "dg5-admin", AuthzVersion: 1}

	groupID, configID := ensureDG5ConfigFixture(t, ctx, configA, configRepo, configWriter)
	typeID, itemID := ensureDG5DictionaryFixture(t, ctx, dictA, dictRepo, dictWriter, baseID)
	dg5AssertSingleActiveDictionaryFixture(t, ctx, dictRepo, typeID)
	dg5RelayAll(t, ctx, relayA)
	waitDG5(t, "writer dirty state resolves after initial relay", func() bool {
		return governedA.GovernedStatus().DirtyClasses == 0
	})
	dg5AssertOversizedDurableOutboxIsTerminalWithoutGeneration(t, ctx, target, relayA, outboxIDsA.NextID, managerA, redisA)

	configV1 := fmt.Sprintf("#%06x", baseID&0xffffff)
	updateDG5ConfigValue(t, ctx, configA, configRepo, configWriter, configID, configV1)
	dg5RelayAll(t, ctx, relayA)
	waitDG5(t, "initial config invalidation delivery", func() bool {
		value, readErr := dg5ReadConfig(ctx, configA, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
		return readErr == nil && value == configV1
	})
	if value, err := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); err != nil || value != configV1 {
		t.Fatalf("second instance initial config read value=%q err=%v", value, err)
	}

	dictV1 := fmt.Sprintf("DG5 label %d", baseID)
	updateDG5DictItemLabel(t, ctx, dictA, dictRepo, dictWriter, itemID, dictV1)
	dg5RelayAll(t, ctx, relayA)
	if label, readErr := dg5ReadDictItemLabel(ctx, dictB, "gender", fmt.Sprintf("dg5_%d", baseID)); readErr != nil || label != dictV1 {
		t.Fatalf("initial dictionary read after invalidation label=%q err=%v", label, readErr)
	}
	dg5MeasureCacheBudget(t, ctx, target, configB, dictB)
	dg5AssertDictRequiredLoginMutationBypassesWarmCache(t, ctx, dictA, dictB, dictRepo, dictWriter, typeID)
	dg5RelayAll(t, ctx, relayA)

	keysBeforeScope := dg5RedisKeyCount(t, ctx, redisA, prefix)
	for _, actor := range []configapp.Actor{
		{Authenticated: true, AccountID: baseID + 11, ScopeID: "org:one", AuthzVersion: 2},
		{Authenticated: true, AccountID: baseID + 12, ScopeID: "org:two", AuthzVersion: 2},
	} {
		if value, readErr := dg5ReadConfig(ctx, configB, actor, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != configV1 {
			t.Fatalf("scope-isolated config read value=%q err=%v", value, readErr)
		}
	}
	if keysAfterScope := dg5RedisKeyCount(t, ctx, redisA, prefix); keysAfterScope < keysBeforeScope+2 {
		t.Fatalf("scope/identity-separated reads did not create independent opaque cache entries: before=%d after=%d governed=%+v health=%+v outbox(pending=%d processing=%d failed=%d)", keysBeforeScope, keysAfterScope, governedB.GovernedStatus(), managerB.Health(ctx).Governance, dg5OutboxStatusCount(t, ctx, target, "PENDING"), dg5OutboxStatusCount(t, ctx, target, "PROCESSING"), dg5OutboxStatusCount(t, ctx, target, "FAILED"))
	}
	dg5AssertOpaqueRedisKeys(t, ctx, redisA, prefix, "themePrimaryColor", "org:one", configV1)

	// Two independent application instances append different domain mutations
	// concurrently through distinct Snowflake nodes. This is a real write-path
	// probe, not merely concurrent relay claiming: IDs must remain unique and
	// both durable invalidations must converge through the shared outbox.
	configConcurrent := fmt.Sprintf("#%06x", (baseID+41)&0xffffff)
	dictConcurrent := fmt.Sprintf("DG5 concurrent %d", baseID)
	dg5AssertConcurrentCrossInstanceMutations(t, ctx, target, configA, configRepo, configWriter, configID, configConcurrent, dictB, dictRepo, dictWriter, itemID, dictConcurrent)
	dg5RelayConcurrently(t, ctx, relayA, relayB)
	waitDG5(t, "cross-instance concurrent config convergence", func() bool {
		value, readErr := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
		return readErr == nil && value == configConcurrent
	})
	waitDG5(t, "cross-instance concurrent dictionary convergence", func() bool {
		label, readErr := dg5ReadDictItemLabel(ctx, dictA, "gender", fmt.Sprintf("dg5_%d", baseID))
		return readErr == nil && label == dictConcurrent
	})
	configV1, dictV1 = configConcurrent, dictConcurrent

	dg5AssertWarmClassifiedCandidate(t, ctx, managerB, governedB, redisB, cachepolicy.ConfigReadRequest, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
	configV2 := fmt.Sprintf("#%06x", (baseID+1)&0xffffff)
	updateDG5ConfigValue(t, ctx, configA, configRepo, configWriter, configID, configV2)
	if value, readErr := dg5ReadConfig(ctx, configA, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != configV2 {
		t.Fatalf("writer read-your-write value=%q err=%v", value, readErr)
	}
	// A committed business mutation is the linearization point for classified
	// reads. B has an old warm L1/L2 entry, while the durable relay is still
	// deliberately paused; it must nevertheless bypass that candidate and read
	// the post-commit authority instead of returning v1.
	if value, readErr := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != configV2 {
		t.Fatalf("second instance returned stale value before durable relay: value=%q err=%v", value, readErr)
	}
	dg5AssertConcurrentPostCommitConfigReads(t, ctx, configB, configV2)
	dg5RelayAll(t, ctx, relayA)
	waitDG5(t, "second instance config convergence", func() bool {
		value, readErr := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
		return readErr == nil && value == configV2
	})
	dg5AssertSiblingOuterTransactionCoalescesMutationFence(t, ctx, target, configA, configB, configRepo, configWriter, configID, relayA, baseID)

	dg5AssertWarmClassifiedCandidate(t, ctx, managerB, governedB, redisB, cachepolicy.DictReadRequest, "gender")
	dictV2 := fmt.Sprintf("DG5 revised %d", baseID)
	updateDG5DictItemLabel(t, ctx, dictA, dictRepo, dictWriter, itemID, dictV2)
	if label, readErr := dg5ReadDictItemLabel(ctx, dictA, "gender", fmt.Sprintf("dg5_%d", baseID)); readErr != nil || label != dictV2 {
		t.Fatalf("dictionary writer read-your-write label=%q err=%v", label, readErr)
	}
	// The same strong boundary applies to eligible dictionaries. B has a warm
	// pre-update cache candidate and the relay remains intentionally paused;
	// it must observe the committed value through the authoritative fallback.
	if label, readErr := dg5ReadDictItemLabel(ctx, dictB, "gender", fmt.Sprintf("dg5_%d", baseID)); readErr != nil || label != dictV2 {
		t.Fatalf("second instance returned stale dictionary before durable relay: label=%q err=%v", label, readErr)
	}
	dg5RelayConcurrently(t, ctx, relayA, relayB)
	waitDG5(t, "concurrent workers fenced and completed dictionary invalidation", func() bool {
		return dg5OutboxStatusCount(t, ctx, target, "PENDING") == 0 && dg5OutboxStatusCount(t, ctx, target, "PROCESSING") == 0
	})
	waitDG5(t, "second instance dictionary convergence", func() bool {
		label, readErr := dg5ReadDictItemLabel(ctx, dictB, "gender", fmt.Sprintf("dg5_%d", baseID))
		return readErr == nil && label == dictV2
	})

	dg5AssertTransactionRollbackLeavesNoOutboxOrWriterDirty(t, ctx, target, configA, configRepo, configWriter, groupID, governedA, baseID)
	dg5AssertSensitiveConfigDoesNotEnterOutboxOrCache(t, ctx, target, configA, configB, configWriter, groupID, redisA, prefix, baseID)
	// Drain the intentional sensitive-fixture invalidation before installing the
	// confirmation-unknown publisher. This makes the following restart test
	// prove the precise config V3 event rather than an unrelated earlier row.
	dg5RelayAll(t, ctx, relayA)

	// Simulate a positive broker delivery whose publisher confirmation becomes
	// unknown. The durable row must stay FAILED; a freshly constructed relay
	// then replays it, yielding only an extra eviction and a final DONE state.
	configV3 := fmt.Sprintf("#%06x", (baseID+2)&0xffffff)
	updateDG5ConfigValue(t, ctx, configA, configRepo, configWriter, configID, configV3)
	faultRelay := cachegovapp.NewService(outboxA, generationA, &dg5PublishThenUnknown{delegate: brokerA}, outboxA, "dg5-relay-"+target.dialect+"-fault")
	if err := faultRelay.RelayOutbox(ctx, 100); err != nil {
		t.Fatalf("simulate publish confirmation unknown: %v", err)
	}
	if dg5OutboxStatusCount(t, ctx, target, "FAILED") == 0 {
		t.Fatal("confirmation-unknown event was incorrectly marked complete")
	}
	restartRelay := cachegovapp.NewService(outboxA, generationA, brokerA, outboxA, "dg5-relay-"+target.dialect+"-restarted")
	waitDG5(t, "outbox restart replay after confirmation unknown", func() bool {
		if relayErr := restartRelay.RelayOutbox(ctx, 100); relayErr != nil {
			return false
		}
		return dg5OutboxStatusCount(t, ctx, target, "FAILED") == 0
	})
	waitDG5(t, "replayed config convergence", func() bool {
		value, readErr := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
		return readErr == nil && value == configV3
	})

	// Delete only the opaque generation key in this test's private Redis
	// prefix. The next read must establish a fresh epoch, evict stale L1, and
	// read the source even while old payload keys still exist.
	if err := redisA.Client().Del(ctx, managerA.Builder().Build("dg5", "generation", cachepolicy.ClassTargetDigest(cachepolicy.DataClassConfigPublicScalar))).Err(); err != nil {
		t.Fatalf("delete DG5 test generation key: %v", err)
	}
	configV4 := fmt.Sprintf("#%06x", (baseID+3)&0xffffff)
	updateDG5ConfigValue(t, ctx, configA, configRepo, configWriter, configID, configV4)
	if value, readErr := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != configV4 {
		t.Fatalf("Redis epoch recovery reused stale L1/L2 value=%q err=%v", value, readErr)
	}
	dg5RelayAll(t, ctx, relayA)

	dg5AssertConsumerRedelivery(t, ctx, target.cfg, prefix, rabbitCfg, rabbitA, outboxA, brokerA)
	dg5AssertHostileFanoutIsRejectedAndObservable(t, ctx, rabbitA, brokerA, brokerB, governedA, governedB)
	dg5AssertOversizedFanoutIsRejectedAndObservable(t, ctx, rabbitA, brokerA, brokerB, governedA, governedB)
	_ = dg5AssertRestartAndPausedRelayUseFreshnessFence(t, ctx, rabbitB, brokerB, relayA, relayB, governedB, configA, configB, configRepo, configWriter, configID, baseID)

	// Prove RabbitMQ degradation independently while B still has a healthy
	// Redis generation path and a warm candidate. It must nevertheless bypass
	// L1/L2 because a broker-unhealthy instance cannot trust an old local view.
	waitDG5(t, "RabbitMQ outage precondition has healthy Redis and fanout", func() bool {
		status := governedB.GovernedStatus()
		return status.FanoutHealthy && status.RedisHealthy && status.FreshnessHealthy && status.ReadTrusted
	})
	if err := rabbitB.Close(); err != nil {
		t.Fatalf("close instance B RabbitMQ client for degradation test: %v", err)
	}
	waitDG5(t, "RabbitMQ consumer outage marks B untrusted", func() bool {
		return !governedB.GovernedStatus().FanoutHealthy
	})
	configV6 := fmt.Sprintf("#%06x", (baseID+6)&0xffffff)
	updateDG5ConfigValue(t, ctx, configA, configRepo, configWriter, configID, configV6)
	if value, readErr := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != configV6 {
		t.Fatalf("RabbitMQ outage did not fall back to source-of-truth: value=%q err=%v", value, readErr)
	}
	rabbitDegraded := managerB.Health(ctx).Governance
	if rabbitDegraded.FanoutHealthy || rabbitDegraded.ReadTrusted {
		t.Fatalf("RabbitMQ degradation was not observable through safe cache health: %+v", rabbitDegraded)
	}
	dg5RelayAll(t, ctx, relayA)
	if err := brokerB.Reconnect(ctx); err != nil {
		t.Fatalf("reconnect instance B RabbitMQ client after independent outage: %v", err)
	}
	rabbitRecoveredCtx, rabbitRecoveredCancel := context.WithCancel(ctx)
	t.Cleanup(rabbitRecoveredCancel)
	startDG5Consumer(t, rabbitRecoveredCtx, brokerB, relayB, governedB)
	waitDG5(t, "RabbitMQ instance B recovers before Redis outage", func() bool {
		return governedB.GovernedStatus().FanoutHealthy
	})

	// A local Redis loss while RabbitMQ remains live makes B's generation
	// freshness untrusted before the next read. The independent RabbitMQ probe
	// above has already restored its own connection, so these results cannot be
	// attributed to a broker outage.
	if err := redisB.Close(); err != nil {
		t.Fatalf("close instance B Redis client for degradation test: %v", err)
	}
	if label, readErr := dg5ReadDictItemLabel(ctx, dictB, "gender", fmt.Sprintf("dg5_%d", baseID)); readErr != nil || label != dictV2 {
		t.Fatalf("Redis outage did not fall back to source-of-truth: label=%q err=%v", label, readErr)
	}
	redisDegraded := managerB.Health(ctx).Governance
	if redisDegraded.RedisHealthy || redisDegraded.ReadTrusted {
		t.Fatalf("Redis degradation was not observable through safe cache health: %+v", redisDegraded)
	}
	configV7 := fmt.Sprintf("#%06x", (baseID+7)&0xffffff)
	updateDG5ConfigValue(t, ctx, configA, configRepo, configWriter, configID, configV7)
	if value, readErr := dg5ReadConfig(ctx, configB, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != configV7 {
		t.Fatalf("Redis-outage instance returned stale value while relay was paused: value=%q err=%v", value, readErr)
	}
	dg5RelayAll(t, ctx, relayA)

	if typeID <= 0 {
		t.Fatal("dictionary type fixture must remain valid")
	}
	t.Logf("DG5 real acceptance completed against isolated %s with two L1 instances, Redis, and RabbitMQ", target.dialect)
}

// dg5AssertVersionedFanoutTopologyUpgradesLegacyQueue declares the exact v2
// durable queue shape before bounded diagnostic-queue retention. It then
// creates a v3 adapter for the same stable instance identity. RabbitMQ refuses
// to alter durable queue arguments in place, so a current topology must use
// new source and diagnostic queue names rather than depending on a destructive
// QueueDelete or failing every upgraded instance closed forever.
func dg5AssertVersionedFanoutTopologyUpgradesLegacyQueue(t *testing.T, client *rabbitinfra.Client, generation cachepolicy.GenerationPort, rabbitCfg sharedconfig.RabbitMQConfig, instanceID string) {
	t.Helper()
	if client == nil || generation == nil {
		t.Fatal("DG5 RabbitMQ topology upgrade probe requires a real client and generation adapter")
	}
	digest := cachepolicy.EventDigest(instanceID)
	legacyQueue := "seven.cache-governance.dg5.v2." + digest[:24]
	legacyDeadLetterQueue := legacyQueue + ".dlq"
	if err := client.DeclareQueue(legacyQueue, rabbitinfra.QueueOptions{
		Expires: 30 * time.Minute,
	}, cachegovinfra.FanoutExchange, ""); err != nil {
		t.Fatalf("declare v2 DG5 source queue for upgrade probe: %v", err)
	}
	if err := client.DeclareQueue(legacyDeadLetterQueue, rabbitinfra.QueueOptions{}, cachegovinfra.FanoutDeadLetterExchange, legacyQueue+".dead"); err != nil {
		t.Fatalf("declare v2 DG5 diagnostic queue for upgrade probe: %v", err)
	}
	// The historical v2 diagnostic intentionally had no expiry. It is created
	// only to prove that V3 gets a distinct durable name; delete these two exact
	// run-scoped test queues on completion so the acceptance harness never turns
	// its legacy probe into a permanent local-broker leak. This uses the AMQP
	// library only in test code and does not restore a generic production queue
	// deletion capability.
	t.Cleanup(func() { dg5DeleteExactTestQueues(t, rabbitCfg, legacyQueue, legacyDeadLetterQueue) })
	adapter, err := cachegovinfra.NewFanoutAdapter(client, generation, instanceID, true)
	if err != nil {
		t.Fatalf("upgrade stable DG5 instance across immutable legacy queue arguments: %v", err)
	}
	if adapter.QueueName() == legacyQueue || !strings.Contains(adapter.QueueName(), "."+cachegovinfra.FanoutTopologyQueueVersion+".") {
		t.Fatalf("DG5 topology did not version immutable source queue: current=%q legacy=%q", adapter.QueueName(), legacyQueue)
	}
	if adapter.DeadLetterQueueName() == legacyDeadLetterQueue || !strings.HasPrefix(adapter.DeadLetterQueueName(), adapter.QueueName()+".") {
		t.Fatalf("DG5 topology did not version immutable diagnostic queue: current=%q legacy=%q", adapter.DeadLetterQueueName(), legacyDeadLetterQueue)
	}
}

func dg5DeleteExactTestQueues(t *testing.T, cfg sharedconfig.RabbitMQConfig, queues ...string) {
	t.Helper()
	endpoint := fmt.Sprintf(
		"amqp://%s:%s@%s:%d/%s",
		url.QueryEscape(cfg.Username),
		url.QueryEscape(cfg.Password),
		cfg.Host,
		cfg.Port,
		strings.TrimPrefix(url.PathEscape(cfg.VHost), "/"),
	)
	conn, err := amqp.Dial(endpoint)
	if err != nil {
		t.Errorf("connect exact DG5 test-queue cleanup channel: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()
	channel, err := conn.Channel()
	if err != nil {
		t.Errorf("open exact DG5 test-queue cleanup channel: %v", err)
		return
	}
	defer func() { _ = channel.Close() }()
	for _, queue := range queues {
		if _, err := channel.QueueDelete(queue, false, false, false); err != nil {
			t.Errorf("delete exact DG5 test queue %q: %v", queue, err)
		}
	}
}

func assertDG5AcceptanceSchema(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) {
	t.Helper()
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := target.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sys_outbox_event`).Scan(&count); err != nil {
		t.Fatalf("DG5 acceptance requires the isolated latest schema: %v", err)
	}
}

// dg5ResetAcceptanceOutbox removes only prior DG5 system-global test work
// from the already-guarded governance database. It neither truncates the
// shared outbox nor touches another owner/scope, so an old failed test cannot
// consume the bounded relay's 100-row budget or make the next run's fanout
// observations depend on historical events.
func dg5ResetAcceptanceOutbox(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) {
	t.Helper()
	if target == nil || target.provider == nil {
		t.Fatal("DG5 acceptance outbox reset requires an isolated database target")
	}
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	exec := target.provider.SQLX()
	statement := dg5OutboxStatement(target,
		`DELETE FROM sys_outbox_event WHERE eventOwner=? AND scopeId=?`,
		`DELETE FROM sys_outbox_event WHERE "eventOwner"=? AND "scopeId"=?`)
	if _, err := exec.ExecContext(ctx, exec.Rebind(statement), cachegovdomain.OutboxOwner, cachegovdomain.ScopeID); err != nil {
		t.Fatalf("reset prior isolated DG5 outbox rows: %v", err)
	}
	var remaining int
	statement = dg5OutboxStatement(target,
		`SELECT COUNT(1) FROM sys_outbox_event WHERE eventOwner=? AND scopeId=?`,
		`SELECT COUNT(1) FROM sys_outbox_event WHERE "eventOwner"=? AND "scopeId"=?`)
	if err := exec.QueryRowxContext(ctx, exec.Rebind(statement), cachegovdomain.OutboxOwner, cachegovdomain.ScopeID).Scan(&remaining); err != nil {
		t.Fatalf("verify isolated DG5 outbox reset: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("isolated DG5 outbox reset left %d system-global rows", remaining)
	}
}

// dg5ResetAcceptanceFixtures removes only rows created by earlier DG5
// acceptance runs. It is deliberately narrower than a schema reset: the exact
// governance database guard has already passed, the parent code is fixed, and
// the child key/value must carry the DG5-owned prefix. This keeps payload
// measurements repeatable without treating the isolated database as a global
// cache or data clearing target.
func dg5ResetAcceptanceFixtures(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) {
	t.Helper()
	if target == nil || target.provider == nil {
		t.Fatal("DG5 fixture reset requires an isolated database target")
	}
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	exec := target.provider.SQLX()
	dg5DeleteFixtureChildren(t, ctx, exec, target,
		`SELECT id FROM sys_dict_type WHERE dictCode=? AND isDeleted=0`,
		`SELECT id FROM sys_dict_type WHERE "dictCode"=? AND "isDeleted"=FALSE`,
		`DELETE FROM sys_dict_item WHERE dictTypeId=? AND itemValue LIKE ?`,
		`DELETE FROM sys_dict_item WHERE "dictTypeId"=? AND "itemValue" LIKE ?`,
		"gender", "dg5_")
	dg5DeleteFixtureChildren(t, ctx, exec, target,
		`SELECT id FROM sys_config_group WHERE groupCode=? AND isDeleted=0`,
		`SELECT id FROM sys_config_group WHERE "groupCode"=? AND "isDeleted"=FALSE`,
		`DELETE FROM sys_config WHERE groupId=? AND configKey LIKE ?`,
		`DELETE FROM sys_config WHERE "groupId"=? AND "configKey" LIKE ?`,
		"SEVEN_FRONTEND_METADATA", "dg5_")
}

func dg5DeleteFixtureChildren(t *testing.T, ctx context.Context, exec *sqlx.DB, target *dg5IsolatedDatabase, mysqlParent, postgresParent, mysqlDelete, postgresDelete, parentCode, prefix string) {
	t.Helper()
	if exec == nil || target == nil {
		t.Fatal("DG5 fixture cleanup requires a guarded SQL executor")
	}
	parentQuery := dg5OutboxStatement(target, mysqlParent, postgresParent)
	var parentID int64
	err := exec.QueryRowxContext(ctx, exec.Rebind(parentQuery), parentCode).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		t.Fatalf("find DG5 fixture parent %q: %v", parentCode, err)
	}
	deleteQuery := dg5OutboxStatement(target, mysqlDelete, postgresDelete)
	if _, err := exec.ExecContext(ctx, exec.Rebind(deleteQuery), parentID, prefix+"%"); err != nil {
		t.Fatalf("delete prior DG5 fixture rows for %q: %v", parentCode, err)
	}
}

func newDG5AcceptanceManager(t *testing.T, ctx context.Context, cfg sharedconfig.Config, prefix, instance string) (cacheinfra.Manager, cacheinfra.GovernedCache, cacheinfra.Provider) {
	t.Helper()
	cacheCfg := cfg.Cache
	cacheCfg.Enabled = true
	cacheCfg.Codec = "sonic"
	cacheCfg.L1.Enabled = true
	cacheCfg.Redis.Enabled = true
	cacheCfg.Redis.KeyPrefix = prefix
	cacheCfg.Redis.ClientName = "seven-dg5-" + instance
	provider := cacheinfra.NewProvider(cacheCfg)
	if !provider.Configured() || provider.Client() == nil {
		t.Fatal("DG5 acceptance Redis provider is not configured")
	}
	if err := provider.Ping(ctx); err != nil {
		t.Fatalf("ping local Redis for DG5 instance %s: %v", instance, err)
	}
	manager, err := cacheinfra.NewDefaultManager(cacheCfg, provider)
	if err != nil {
		t.Fatalf("create DG5 cache manager %s: %v", instance, err)
	}
	governed, ok := manager.(cacheinfra.GovernedCache)
	if !ok {
		t.Fatal("DG5 cache manager is missing classified cache layer")
	}
	return manager, governed, provider
}

func dg5AcceptanceRabbitConfig(cfg sharedconfig.Config) sharedconfig.RabbitMQConfig {
	rabbitCfg := cfg.RabbitMQ
	rabbitCfg.URL = ""
	rabbitCfg.Enabled = true
	rabbitCfg.Declare = true
	rabbitCfg.Host = "127.0.0.1"
	rabbitCfg.Port = 5672
	if strings.TrimSpace(rabbitCfg.Username) == "" {
		rabbitCfg.Username = "guest"
	}
	if rabbitCfg.Prefetch <= 0 {
		rabbitCfg.Prefetch = 10
	}
	return rabbitCfg
}

func startDG5Consumer(t *testing.T, ctx context.Context, broker *cachegovinfra.FanoutAdapter, service *cachegovapp.Service, governed cacheinfra.GovernedCache) {
	t.Helper()
	go func() { _ = broker.Consume(ctx, service.HandleFanout) }()
	waitDG5(t, "DG5 RabbitMQ fanout consumer healthy", func() bool {
		return governed.GovernedStatus().FanoutHealthy
	})
}

// dg5AssertOversizedDurableOutboxIsTerminalWithoutGeneration injects a
// de-sensitised oversized body into only the already-guarded governance test
// database. It models a compromised owner/type-matching database writer and
// proves the bounded SQL projection marks the row DEAD without handing its
// body to Sonic, Redis generation, or RabbitMQ.
func dg5AssertOversizedDurableOutboxIsTerminalWithoutGeneration(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, relay *cachegovapp.Service, nextID func() int64, manager cacheinfra.Manager, redis cacheinfra.Provider) {
	t.Helper()
	if target == nil || relay == nil || nextID == nil || manager == nil || redis == nil || redis.Client() == nil {
		t.Fatal("DG5 bounded durable outbox probe requires isolated database, relay, id generator, and Redis manager")
	}
	id := nextID()
	if id <= 0 {
		t.Fatal("DG5 bounded durable outbox probe received an invalid id")
	}
	eventID := fmt.Sprintf("dg5-bounded-outbox-%d", time.Now().UnixNano())
	aggregateID := cachepolicy.ClassTargetDigest(cachepolicy.DataClassConfigPublicScalar)
	generationKey := manager.Builder().Build("dg5", "generation", aggregateID)
	generationBefore, generationBeforeErr := redis.Client().Get(ctx, generationKey).Result()
	if generationBeforeErr != nil && !errors.Is(generationBeforeErr, redisclient.Nil) {
		t.Fatalf("read DG5 generation before bounded durable outbox probe: err=%v", generationBeforeErr)
	}
	exec := target.provider.SQLX()
	insert := dg5OutboxStatement(target,
		`INSERT INTO sys_outbox_event (id, eventId, eventOwner, scopeId, eventType, aggregateType, aggregateId, payload, status, retryCount, nextRetryAt, errorMsg, leaseOwner, leaseToken, leaseUntil, createTime, updateTime)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', 0, NULL, NULL, NULL, NULL, NULL, ?, ?)`,
		`INSERT INTO sys_outbox_event (id, "eventId", "eventOwner", "scopeId", "eventType", "aggregateType", "aggregateId", payload, status, "retryCount", "nextRetryAt", "errorMsg", "leaseOwner", "leaseToken", "leaseUntil", "createTime", "updateTime")
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', 0, NULL, NULL, NULL, NULL, NULL, ?, ?)`)
	now := time.Now().UTC()
	if _, err := exec.ExecContext(ctx, exec.Rebind(insert), id, eventID, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.CacheInvalidationEventType, cachepolicy.CacheInvalidationAggregate, aggregateID, strings.Repeat("x", cachepolicy.MaxInvalidationEnvelopeBytes+1), now, now); err != nil {
		t.Fatalf("insert guarded oversized DG5 durable outbox probe: %v", err)
	}
	// A DEAD DG5 row intentionally keeps its data class source-only under the
	// zero-stale fence. This acceptance-only adversarial row therefore has to
	// be removed exactly after its terminal result is verified; arrange the
	// same narrowly scoped cleanup on every early test failure as well.
	deleteProbe := func(requireOne bool) {
		deleteSQL := dg5OutboxStatement(target,
			`DELETE FROM sys_outbox_event WHERE id=? AND eventOwner=? AND scopeId=? AND eventType=? AND aggregateType=? AND aggregateId=?`,
			`DELETE FROM sys_outbox_event WHERE id=? AND "eventOwner"=? AND "scopeId"=? AND "eventType"=? AND "aggregateType"=? AND "aggregateId"=?`)
		result, deleteErr := exec.ExecContext(context.Background(), exec.Rebind(deleteSQL), id, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.CacheInvalidationEventType, cachepolicy.CacheInvalidationAggregate, aggregateID)
		if deleteErr != nil {
			if requireOne {
				t.Fatalf("remove guarded oversized DG5 durable outbox probe: %v", deleteErr)
			}
			t.Errorf("cleanup guarded oversized DG5 durable outbox probe: %v", deleteErr)
			return
		}
		if requireOne {
			rows, rowsErr := result.RowsAffected()
			if rowsErr != nil || rows != 1 {
				t.Fatalf("remove guarded oversized DG5 durable outbox probe affected=%d err=%v, want exactly one", rows, rowsErr)
			}
		}
	}
	t.Cleanup(func() { deleteProbe(false) })
	if err := relay.RelayOutbox(ctx, 100); err != nil {
		t.Fatalf("relay guarded oversized DG5 durable outbox probe: %v", err)
	}
	query := dg5OutboxStatement(target,
		`SELECT status, errorMsg FROM sys_outbox_event WHERE id=?`,
		`SELECT status, "errorMsg" FROM sys_outbox_event WHERE id=?`)
	var status string
	var reason sql.NullString
	if err := exec.QueryRowxContext(ctx, exec.Rebind(query), id).Scan(&status, &reason); err != nil {
		t.Fatalf("read guarded oversized DG5 durable outbox result: %v", err)
	}
	if status != "DEAD" || reason.String != "cache invalidation payload exceeds protocol limit" {
		t.Fatalf("oversized durable DG5 outbox row was not safely terminal: status=%q reason=%q", status, reason.String)
	}
	generationAfter, generationAfterErr := redis.Client().Get(ctx, generationKey).Result()
	if generationBeforeErr == nil {
		if generationAfterErr != nil || generationAfter != generationBefore {
			t.Fatalf("oversized durable DG5 outbox row advanced generation: unchanged=%t err=%v", generationAfter == generationBefore, generationAfterErr)
		}
	} else if !errors.Is(generationAfterErr, redisclient.Nil) {
		t.Fatalf("oversized durable DG5 outbox row created a generation epoch: err=%v", generationAfterErr)
	}
	deleteProbe(true)
}

func ensureDG5ConfigFixture(t *testing.T, ctx context.Context, service *configapp.Service, repo *configinfra.Repository, actor configapp.Actor) (int64, int64) {
	t.Helper()
	group, err := repo.FindGroupByCode(ctx, "SEVEN_FRONTEND_METADATA")
	if err != nil {
		t.Fatalf("find DG5 config group: %v", err)
	}
	if group == nil {
		groupID, addErr := service.AddConfigGroup(ctx, actor, configfacade.ConfigGroupAddRequest{GroupCode: "SEVEN_FRONTEND_METADATA", GroupName: "DG5 frontend metadata"})
		if addErr != nil {
			t.Fatalf("create DG5 config group: %v", addErr)
		}
		group, err = repo.FindGroupByID(ctx, groupID)
		if err != nil || group == nil {
			t.Fatalf("reload DG5 config group: group=%#v err=%v", group, err)
		}
	}
	if group.Status != 1 || group.IsDeleted != 0 {
		t.Fatalf("DG5 config group is not readable: status=%d deleted=%d", group.Status, group.IsDeleted)
	}
	item, err := repo.FindConfigByGroupAndKey(ctx, group.ID, "themePrimaryColor", false)
	if err != nil {
		t.Fatalf("find DG5 config fixture: %v", err)
	}
	if item == nil {
		schema := configdomain.CurrentScalarSchemaVersion
		configID, addErr := service.AddConfig(ctx, actor, configfacade.ConfigAddRequest{
			GroupID:       group.ID,
			ConfigKey:     "themePrimaryColor",
			ConfigValue:   "#102030",
			ValueType:     "STRING",
			UIWidget:      "INPUT",
			Exposure:      "PUBLIC",
			Sensitivity:   "NORMAL",
			EffectType:    "realtime",
			SchemaVersion: &schema,
		})
		if addErr != nil {
			t.Fatalf("create DG5 config fixture: %v", addErr)
		}
		return group.ID, configID
	}
	if item.Exposure != "PUBLIC" || item.Sensitivity != "NORMAL" || item.SchemaVersion != configdomain.CurrentScalarSchemaVersion || item.IsEnabled != 1 || item.IsDeleted != 0 {
		t.Fatalf("existing DG5 config fixture is not catalog eligible")
	}
	return group.ID, item.ID
}

func ensureDG5DictionaryFixture(t *testing.T, ctx context.Context, service *dictapp.Service, repo *dictinfra.Repository, actor dictapp.Actor, baseID int64) (int64, int64) {
	t.Helper()
	typeItem, err := repo.FindTypeByCode(ctx, "gender")
	if err != nil {
		t.Fatalf("find DG5 dict type: %v", err)
	}
	if typeItem == nil {
		status, schema := 1, cachepolicy.SchemaVersionV1
		typeID, addErr := service.AddDictType(ctx, actor, dictfacade.DictTypeAddRequest{
			DictCode:      "gender",
			DictName:      "DG5 Gender",
			Status:        &status,
			ValueType:     "STRING",
			UIWidget:      "SELECT",
			Exposure:      "PUBLIC",
			Sensitivity:   "NORMAL",
			SchemaVersion: &schema,
		})
		if addErr != nil {
			t.Fatalf("create DG5 dict type: %v", addErr)
		}
		typeItem, err = repo.FindTypeByID(ctx, typeID)
		if err != nil || typeItem == nil {
			t.Fatalf("reload DG5 dict type: item=%#v err=%v", typeItem, err)
		}
	}
	if typeItem.Exposure != "PUBLIC" || typeItem.Sensitivity != "NORMAL" || typeItem.SchemaVersion != cachepolicy.SchemaVersionV1 || typeItem.Status != 1 || typeItem.IsDeleted != 0 || typeItem.RequiredLogin != 0 {
		t.Fatalf("existing DG5 dictionary fixture is not catalog eligible")
	}
	status := 1
	itemID, err := service.AddDictItem(ctx, actor, typeItem.ID, dictfacade.DictItemAddRequest{
		ItemValue: fmt.Sprintf("dg5_%d", baseID),
		ItemLabel: "DG5 initial",
		Status:    &status,
	})
	if err != nil {
		t.Fatalf("create DG5 dict item fixture: %v", err)
	}
	return typeItem.ID, itemID
}

func dg5AssertSingleActiveDictionaryFixture(t *testing.T, ctx context.Context, repo *dictinfra.Repository, typeID int64) {
	t.Helper()
	items, err := repo.QueryItems(ctx, dictdomain.DictItemListQuery{DictTypeID: typeID})
	if err != nil {
		t.Fatalf("list DG5 dictionary fixture items: %v", err)
	}
	count := 0
	for _, item := range items {
		if strings.HasPrefix(item.ItemValue, "dg5_") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("DG5 dictionary fixture leaked across runs: active fixture count=%d, want=1", count)
	}
}

func updateDG5ConfigValue(t *testing.T, ctx context.Context, service *configapp.Service, repo *configinfra.Repository, actor configapp.Actor, id int64, value string) {
	t.Helper()
	item, err := repo.FindConfigByID(ctx, id)
	if err != nil || item == nil {
		t.Fatalf("load config before DG5 update: item=%#v err=%v", item, err)
	}
	if err := service.UpdateConfig(ctx, actor, configfacade.ConfigUpdateRequest{ID: id, ConfigValue: &value, Version: &item.Version}); err != nil {
		t.Fatalf("update DG5 config value: %v", err)
	}
}

func updateDG5DictItemLabel(t *testing.T, ctx context.Context, service *dictapp.Service, repo *dictinfra.Repository, actor dictapp.Actor, id int64, label string) {
	t.Helper()
	item, err := repo.FindItemByID(ctx, id)
	if err != nil || item == nil {
		t.Fatalf("load dict item before DG5 update: item=%#v err=%v", item, err)
	}
	if err := service.UpdateDictItem(ctx, actor, dictfacade.DictItemUpdateRequest{ID: id, ItemLabel: &label, Version: &item.Version}); err != nil {
		t.Fatalf("update DG5 dictionary item: %v", err)
	}
}

// dg5AssertDictRequiredLoginMutationBypassesWarmCache warms the eligible
// anonymous dictionary response first, then changes the current source-side
// authorization fact. The second instance must not reuse that old candidate:
// it must observe the required-login rule through the source/freshness path.
func dg5AssertDictRequiredLoginMutationBypassesWarmCache(t *testing.T, ctx context.Context, writer, reader *dictapp.Service, repo *dictinfra.Repository, actor dictapp.Actor, typeID int64) {
	t.Helper()
	before, err := repo.FindTypeByID(ctx, typeID)
	if err != nil || before == nil {
		t.Fatalf("load DG5 dictionary type before required-login mutation: type=%t err=%v", before != nil, err)
	}
	if before.RequiredLogin != 0 {
		t.Fatalf("DG5 dictionary fixture was not anonymously cacheable before required-login probe: requiredLogin=%d", before.RequiredLogin)
	}
	required := 1
	if err := writer.UpdateDictType(ctx, actor, dictfacade.DictTypeUpdateRequest{ID: typeID, RequiredLogin: &required, Version: &before.Version}); err != nil {
		t.Fatalf("require login for DG5 dictionary type: %v", err)
	}
	if response, readErr := reader.GetDictByCodeForClient(ctx, dictapp.Actor{}, "gender"); readErr == nil || response != nil {
		t.Fatalf("anonymous reader reused a warm dictionary cache entry after required-login commit: response=%t err=%v", response != nil, readErr)
	}
	after, err := repo.FindTypeByID(ctx, typeID)
	if err != nil || after == nil {
		t.Fatalf("reload DG5 dictionary type before restoring required-login: type=%t err=%v", after != nil, err)
	}
	public := 0
	if err := writer.UpdateDictType(ctx, actor, dictfacade.DictTypeUpdateRequest{ID: typeID, RequiredLogin: &public, Version: &after.Version}); err != nil {
		t.Fatalf("restore public DG5 dictionary type: %v", err)
	}
	if response, readErr := reader.GetDictByCodeForClient(ctx, dictapp.Actor{}, "gender"); readErr != nil || response == nil {
		t.Fatalf("public reader did not recover through authoritative dictionary path: response=%t err=%v", response != nil, readErr)
	}
}

func dg5ReadConfig(ctx context.Context, service *configapp.Service, actor configapp.Actor, key string) (string, error) {
	item, err := service.GetConfigByKeyForClient(ctx, actor, key)
	if err != nil || item == nil {
		return "", err
	}
	value, ok := item.Value.(string)
	if !ok {
		return "", fmt.Errorf("unexpected config scalar type %T", item.Value)
	}
	return value, nil
}

func dg5ReadDictItemLabel(ctx context.Context, service *dictapp.Service, code, itemValue string) (string, error) {
	response, err := service.GetDictByCodeForClient(ctx, dictapp.Actor{}, code)
	if err != nil || response == nil {
		return "", err
	}
	items := response.Record[code]
	for _, item := range items {
		if item.ItemValue == itemValue {
			return item.ItemLabel, nil
		}
	}
	return "", fmt.Errorf("dictionary item not found in client result of %d entries", len(items))
}

func dg5RelayAll(t *testing.T, ctx context.Context, relay *cachegovapp.Service) {
	t.Helper()
	if err := relay.RelayOutbox(ctx, 100); err != nil {
		t.Fatalf("relay DG5 outbox: %v", err)
	}
}

// dg5RelayConcurrently makes two independent workers race for the same
// cache-governance row. The shared Store's owner/type claim and lease fence
// must make this converge as one completed invalidation (or a harmless
// duplicate broker eviction), never an unclaimed or permanently processing
// row.
func dg5RelayConcurrently(t *testing.T, ctx context.Context, relays ...*cachegovapp.Service) {
	t.Helper()
	var group sync.WaitGroup
	errs := make(chan error, len(relays))
	for _, relay := range relays {
		if relay == nil {
			continue
		}
		group.Add(1)
		go func(worker *cachegovapp.Service) {
			defer group.Done()
			errs <- worker.RelayOutbox(ctx, 100)
		}(relay)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent DG5 relay: %v", err)
		}
	}
}

func dg5AssertConcurrentCrossInstanceMutations(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, configService *configapp.Service, configRepo *configinfra.Repository, configActor configapp.Actor, configID int64, configValue string, dictService *dictapp.Service, dictRepo *dictinfra.Repository, dictActor dictapp.Actor, dictItemID int64, dictLabel string) {
	t.Helper()
	beforeCount, beforeDistinct := dg5OutboxIDStats(t, ctx, target)
	errs := make(chan error, 2)
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		item, err := configRepo.FindConfigByID(ctx, configID)
		if err != nil || item == nil {
			errs <- fmt.Errorf("load concurrent config: item=%t err=%w", item != nil, err)
			return
		}
		errs <- configService.UpdateConfig(ctx, configActor, configfacade.ConfigUpdateRequest{ID: configID, ConfigValue: &configValue, Version: &item.Version})
	}()
	go func() {
		defer group.Done()
		item, err := dictRepo.FindItemByID(ctx, dictItemID)
		if err != nil || item == nil {
			errs <- fmt.Errorf("load concurrent dictionary item: item=%t err=%w", item != nil, err)
			return
		}
		errs <- dictService.UpdateDictItem(ctx, dictActor, dictfacade.DictItemUpdateRequest{ID: dictItemID, ItemLabel: &dictLabel, Version: &item.Version})
	}()
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent cross-instance DG5 mutation: %v", err)
		}
	}
	afterCount, afterDistinct := dg5OutboxIDStats(t, ctx, target)
	if afterCount < beforeCount+2 || afterCount != afterDistinct || beforeCount != beforeDistinct {
		t.Fatalf("concurrent outbox IDs were not unique and durable: before=%d/%d after=%d/%d", beforeCount, beforeDistinct, afterCount, afterDistinct)
	}
}

// dg5AssertConcurrentPostCommitConfigReads races independent B readers only
// after A's UpdateConfig returned. Every candidate read is therefore after the
// business commit linearization point and must see v2 even before the relay
// advances Redis or publishes fanout.
func dg5AssertConcurrentPostCommitConfigReads(t *testing.T, ctx context.Context, service *configapp.Service, want string) {
	t.Helper()
	if service == nil {
		t.Fatal("DG5 concurrent read probe requires a config service")
	}
	const readers = 12
	start := make(chan struct{})
	errs := make(chan error, readers)
	var group sync.WaitGroup
	for index := 0; index < readers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			value, err := dg5ReadConfig(ctx, service, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
			if err != nil || value != want {
				errs <- fmt.Errorf("post-commit reader returned value=%q err=%w", value, err)
			}
		}()
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// dg5AssertSiblingOuterTransactionCoalescesMutationFence executes two config
// mutations as siblings of one real outer MySQL/PostgreSQL transaction. The
// second service call receives the original transaction context, so this is
// the regression boundary for transaction-scoped resource sharing: a second
// non-reentrant advisory lock would otherwise self-timeout before commit.
func dg5AssertSiblingOuterTransactionCoalescesMutationFence(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, writer, reader *configapp.Service, repo *configinfra.Repository, actor configapp.Actor, configID int64, relay *cachegovapp.Service, baseID int64) {
	t.Helper()
	if target == nil || writer == nil || reader == nil || repo == nil || relay == nil {
		t.Fatal("DG5 sibling outer-transaction probe requires isolated database, config services, repository, and relay")
	}
	beforeCount, beforeDistinct := dg5OutboxIDStats(t, ctx, target)
	first := fmt.Sprintf("#%06x", (baseID+21)&0xffffff)
	second := fmt.Sprintf("#%06x", (baseID+22)&0xffffff)
	if err := target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		item, loadErr := repo.FindConfigByID(txCtx, configID)
		if loadErr != nil || item == nil {
			return fmt.Errorf("load first sibling config mutation: item=%t err=%v", item != nil, loadErr)
		}
		if updateErr := writer.UpdateConfig(txCtx, actor, configfacade.ConfigUpdateRequest{ID: configID, ConfigValue: &first, Version: &item.Version}); updateErr != nil {
			return fmt.Errorf("first sibling config mutation: %w", updateErr)
		}
		item, loadErr = repo.FindConfigByID(txCtx, configID)
		if loadErr != nil || item == nil {
			return fmt.Errorf("load second sibling config mutation: item=%t err=%v", item != nil, loadErr)
		}
		if updateErr := writer.UpdateConfig(txCtx, actor, configfacade.ConfigUpdateRequest{ID: configID, ConfigValue: &second, Version: &item.Version}); updateErr != nil {
			return fmt.Errorf("second sibling config mutation: %w", updateErr)
		}
		return nil
	}); err != nil {
		t.Fatalf("DG5 sibling outer transaction self-lock or mutation failure: %v", err)
	}
	afterCount, afterDistinct := dg5OutboxIDStats(t, ctx, target)
	if afterCount < beforeCount+2 || afterCount != afterDistinct || beforeCount != beforeDistinct {
		t.Fatalf("DG5 sibling mutations did not commit two distinct durable invalidations: before=%d/%d after=%d/%d", beforeCount, beforeDistinct, afterCount, afterDistinct)
	}
	if value, readErr := dg5ReadConfig(ctx, reader, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != second {
		t.Fatalf("DG5 sibling outer transaction returned a stale remote cache value before relay: value=%q err=%v", value, readErr)
	}
	dg5RelayAll(t, ctx, relay)
	waitDG5(t, "sibling outer transaction relay convergence", func() bool {
		value, readErr := dg5ReadConfig(ctx, reader, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
		return readErr == nil && value == second
	})
}

// dg5MeasureCacheBudget records only byte counts from the same eligible
// business responses used by acceptance. It intentionally never logs a
// response body, cache key, scope, or configuration value. Ristretto costs
// the payload bytes supplied by the governed layer, so the reported entry
// counts are a conservative payload-only lower bound.
func dg5MeasureCacheBudget(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, configService *configapp.Service, dictService *dictapp.Service) {
	t.Helper()
	configValue, err := configService.GetConfigByKeyForClient(ctx, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
	if err != nil || configValue == nil {
		t.Fatalf("read classified config for DG5 payload measurement: value=%t err=%v", configValue != nil, err)
	}
	dictValue, err := dictService.GetDictByCodeForClient(ctx, dictapp.Actor{}, "gender")
	if err != nil || dictValue == nil {
		t.Fatalf("read classified dict for DG5 payload measurement: value=%t err=%v", dictValue != nil, err)
	}
	configPayload, err := sonic.Marshal(configValue)
	if err != nil {
		t.Fatalf("Sonic encode DG5 config measurement: %v", err)
	}
	dictPayload, err := sonic.Marshal(dictValue)
	if err != nil {
		t.Fatalf("Sonic encode DG5 dictionary measurement: %v", err)
	}
	envelope, err := cachegovdomain.NewInvalidationEvent("dg5-cache-budget-envelope", cachepolicy.DataClassConfigPublicScalar)
	if err != nil {
		t.Fatalf("create DG5 fanout envelope measurement: %v", err)
	}
	envelopePayload, err := sonic.Marshal(envelope)
	if err != nil {
		t.Fatalf("Sonic encode DG5 fanout envelope measurement: %v", err)
	}
	if len(envelopePayload) > cachegovinfra.MaxFanoutEnvelopeBytes {
		t.Fatalf("DG5 fanout envelope measurement exceeds reviewed limit: bytes=%d limit=%d", len(envelopePayload), cachegovinfra.MaxFanoutEnvelopeBytes)
	}
	maxCost := target.cfg.Cache.L1.MaxCost
	if maxCost <= 0 || int64(len(configPayload)) > maxCost || int64(len(dictPayload)) > maxCost {
		t.Fatalf("DG5 configured Ristretto maxCost cannot hold representative payloads")
	}
	configEntry, configOK := cachepolicy.Entry(cachepolicy.DataClassConfigPublicScalar)
	dictEntry, dictOK := cachepolicy.Entry(cachepolicy.DataClassDictPublicItems)
	if !configOK || !dictOK || configEntry.L1TTL != 30*time.Second || dictEntry.L1TTL != 30*time.Second || configEntry.L2TTL != 5*time.Minute || dictEntry.L2TTL != 5*time.Minute || configEntry.MaxStale != 0 || dictEntry.MaxStale != 0 {
		t.Fatal("DG5 cache timing catalog drifted from the reviewed acceptance values")
	}
	t.Logf("DG5 Sonic payload budget: config=%dB dict=%dB fanoutEnvelope=%dB fanoutEnvelopeLimit=%dB l1MaxCost=%d payloadOnlyEntriesAtLeast(config=%d,dict=%d) l1TTL=30s l2TTL=5m maxStale=0s", len(configPayload), len(dictPayload), len(envelopePayload), cachegovinfra.MaxFanoutEnvelopeBytes, maxCost, maxCost/int64(len(configPayload)), maxCost/int64(len(dictPayload)))
}

// dg5AssertWarmClassifiedCandidate proves the pre-update state includes a
// real governed L2 candidate for B's exact opaque request/generation tuple.
// It intentionally never logs that physical key or any raw target/value. The
// following v2-before-relay assertions therefore exercise invalidation of an
// actual cache candidate, not a source-only false green path.
func dg5AssertWarmClassifiedCandidate(t *testing.T, ctx context.Context, manager cacheinfra.Manager, governed cacheinfra.GovernedCache, provider cacheinfra.Provider, requestFactory func(string, string, string) (cachepolicy.ReadRequest, bool), target string) {
	t.Helper()
	if manager == nil || governed == nil || provider == nil || provider.Client() == nil || requestFactory == nil {
		t.Fatal("DG5 warm-candidate probe requires configured manager, Redis provider, and request factory")
	}
	request, ok := requestFactory(target, "public:global", "anonymous")
	if !ok {
		t.Fatal("DG5 warm-candidate probe could not construct a catalogued request")
	}
	governance := governed.GovernedStatus()
	if !governance.FanoutHealthy || !governance.RedisHealthy || !governance.FreshnessHealthy || !governance.ReadTrusted {
		t.Fatalf("DG5 warm-candidate probe ran without a trusted B cache health state: %+v", governance)
	}
	generationKey := manager.Builder().Build("dg5", "generation", cachepolicy.ClassTargetDigest(request.Entry.DataClass))
	generation, err := provider.Client().Get(ctx, generationKey).Result()
	if err != nil || strings.TrimSpace(generation) == "" {
		t.Fatalf("DG5 warm-candidate generation is unavailable: generation=%t err=%v", strings.TrimSpace(generation) != "", err)
	}
	payloadKey := dg5PayloadKeyForCurrentEpoch(t, ctx, manager, provider, request, generation)
	exists, err := provider.Client().Exists(ctx, payloadKey).Result()
	if err != nil || exists != 1 {
		t.Fatalf("DG5 B instance did not hold the expected opaque L2 cache candidate: exists=%d err=%v", exists, err)
	}
}

// dg5PayloadKeyForCurrentEpoch mirrors the governed layer's opaque payload
// namespace. DG6.3 added the global refresh epoch to that namespace, so an
// acceptance probe must include both independently fenced epochs rather than
// accidentally inspect a pre-DG6.3 key that production no longer uses.
func dg5PayloadKeyForCurrentEpoch(t *testing.T, ctx context.Context, manager cacheinfra.Manager, provider cacheinfra.Provider, request cachepolicy.ReadRequest, generation string) string {
	t.Helper()
	if manager == nil || provider == nil || provider.Client() == nil {
		t.Fatal("DG5 payload-key assertion requires cache manager and Redis provider")
	}
	globalEpochKey := manager.Builder().Build("dg6", "cache-refresh-v3", "epoch")
	globalEpoch, err := provider.Client().Get(ctx, globalEpochKey).Result()
	if err != nil || strings.TrimSpace(globalEpoch) == "" {
		t.Fatalf("DG5 payload-key assertion has no global refresh epoch: epoch=%t err=%v", strings.TrimSpace(globalEpoch) != "", err)
	}
	return manager.Builder().Build("dg5", "payload", request.KeyMaterial(), strings.TrimSpace(generation), strings.TrimSpace(globalEpoch))
}

func dg5RedisKeyCount(t *testing.T, ctx context.Context, provider cacheinfra.Provider, prefix string) int {
	t.Helper()
	keys, err := provider.Client().Keys(ctx, prefix+"*").Result()
	if err != nil {
		t.Fatalf("list DG5 private Redis keys: %v", err)
	}
	return len(keys)
}

func dg5AssertOpaqueRedisKeys(t *testing.T, ctx context.Context, provider cacheinfra.Provider, prefix string, forbidden ...string) {
	t.Helper()
	keys, err := provider.Client().Keys(ctx, prefix+"*").Result()
	if err != nil {
		t.Fatalf("list DG5 Redis keys for opacity check: %v", err)
	}
	for _, key := range keys {
		for _, raw := range forbidden {
			if raw != "" && strings.Contains(strings.ToLower(key), strings.ToLower(raw)) {
				t.Fatal("DG5 Redis key leaked raw cache material")
			}
		}
	}
}

func dg5AssertTransactionRollbackLeavesNoOutboxOrWriterDirty(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, service *configapp.Service, repo *configinfra.Repository, actor configapp.Actor, groupID int64, governed cacheinfra.GovernedCache, baseID int64) {
	t.Helper()
	beforeOutbox := dg5OutboxOwnerCount(t, ctx, target)
	beforeDirty := governed.GovernedStatus().DirtyClasses
	rollback := errors.New("DG5 acceptance rollback")
	err := target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		schema := configdomain.CurrentScalarSchemaVersion
		_, addErr := service.AddConfig(txCtx, actor, configfacade.ConfigAddRequest{
			GroupID:       groupID,
			ConfigKey:     fmt.Sprintf("rollback_%d", baseID),
			ConfigValue:   "rolled-back",
			ValueType:     "STRING",
			UIWidget:      "INPUT",
			Exposure:      "INTERNAL",
			Sensitivity:   "NORMAL",
			EffectType:    "realtime",
			SchemaVersion: &schema,
		})
		if addErr != nil {
			return addErr
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("force DG5 rollback err=%v", err)
	}
	if after := dg5OutboxOwnerCount(t, ctx, target); after != beforeOutbox {
		t.Fatalf("rolled-back mutation left cache outbox rows: before=%d after=%d", beforeOutbox, after)
	}
	if afterDirty := governed.GovernedStatus().DirtyClasses; afterDirty != beforeDirty {
		t.Fatalf("rolled-back mutation dirtied writer L1: before=%d after=%d", beforeDirty, afterDirty)
	}
	item, findErr := repo.FindConfigByGroupAndKey(ctx, groupID, fmt.Sprintf("rollback_%d", baseID), true)
	if findErr != nil || item != nil {
		t.Fatalf("rolled-back config persisted: item=%#v err=%v", item, findErr)
	}
}

// dg5AssertStrictScopeClaimFence attacks the read-to-claim-to-mark boundary
// against a real isolated database row. A cache owner/type row with local
// scope must remain PENDING when the DG5 system:global adapter tries to claim
// or complete it; local/NULL compatibility is never permitted for DG5.
func dg5AssertStrictScopeClaimFence(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, adapter *cachegovinfra.OutboxAdapter, nextID func() int64) {
	t.Helper()
	if adapter == nil || nextID == nil {
		t.Fatal("DG5 scope-fence fixture requires adapter and distributed id generator")
	}
	id := nextID()
	eventID := fmt.Sprintf("dg5-local-scope-%d", time.Now().UnixNano())
	foreign := &msgoutbox.Event{
		ID:            id,
		EventID:       eventID,
		EventOwner:    cachegovdomain.OutboxOwner,
		ScopeID:       "local",
		EventType:     cachegovdomain.EventType,
		AggregateType: "cache-invalidation",
		AggregateID:   "scope-fence-probe",
		Payload:       `{}`,
	}
	if err := target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return msgoutbox.NewStore(target.provider.SQLX()).Append(txCtx, foreign)
	}); err != nil {
		t.Fatalf("create isolated local-scope DG5 probe: %v", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		exec := target.provider.SQLX()
		_, _ = exec.ExecContext(cleanupCtx, exec.Rebind(dg5OutboxStatement(target,
			`DELETE FROM sys_outbox_event WHERE id=? AND eventOwner=? AND scopeId=?`,
			`DELETE FROM sys_outbox_event WHERE id=? AND "eventOwner"=? AND "scopeId"=?`)), id, cachegovdomain.OutboxOwner, "local")
	}()
	lease, claimed, err := adapter.Claim(ctx, id, cachegovdomain.EventType, "dg5-scope-fence")
	if err != nil || claimed || lease != nil {
		t.Fatalf("system:global adapter crossed local claim fence: lease=%+v claimed=%t err=%v", lease, claimed, err)
	}
	applied, err := adapter.Mark(ctx, id, cachegovdomain.EventType, "not-a-lease", "DONE", "", 0, nil)
	if err != nil || applied {
		t.Fatalf("system:global adapter crossed local completion fence: applied=%t err=%v", applied, err)
	}
	var scope, status string
	exec := target.provider.SQLX()
	if err := exec.QueryRowxContext(ctx, exec.Rebind(dg5OutboxStatement(target,
		`SELECT scopeId, status FROM sys_outbox_event WHERE id=?`,
		`SELECT "scopeId", status FROM sys_outbox_event WHERE id=?`)), id).Scan(&scope, &status); err != nil {
		t.Fatalf("read isolated local-scope probe after fenced operations: %v", err)
	}
	if scope != "local" || status != "PENDING" {
		t.Fatalf("local-scope probe changed by DG5 fence check: scope=%q status=%q", scope, status)
	}
}

func dg5AssertSensitiveConfigDoesNotEnterOutboxOrCache(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, writer, reader *configapp.Service, actor configapp.Actor, groupID int64, redis cacheinfra.Provider, prefix string, baseID int64) {
	t.Helper()
	canary := fmt.Sprintf("DG5-SECRET-%d", baseID)
	schema := configdomain.CurrentScalarSchemaVersion
	secretID, err := writer.AddConfig(ctx, actor, configfacade.ConfigAddRequest{
		GroupID:       groupID,
		ConfigKey:     fmt.Sprintf("dg5_secret_%d", baseID),
		ConfigValue:   canary,
		ValueType:     "STRING",
		UIWidget:      "INPUT",
		Exposure:      "INTERNAL",
		Sensitivity:   "SECRET",
		EffectType:    "realtime",
		SchemaVersion: &schema,
	})
	if err != nil || secretID <= 0 {
		t.Fatalf("create DG5 secret fixture id=%d err=%v", secretID, err)
	}
	keysBefore := dg5RedisKeyCount(t, ctx, redis, prefix)
	if _, readErr := reader.GetConfigByKeyForClient(ctx, configapp.Actor{}, "SEVEN_FRONTEND_METADATA."+fmt.Sprintf("dg5_secret_%d", baseID)); readErr == nil {
		t.Fatal("sensitive config became externally readable")
	}
	if keysAfter := dg5RedisKeyCount(t, ctx, redis, prefix); keysAfter != keysBefore {
		t.Fatalf("sensitive config read added classified cache entries: before=%d after=%d", keysBefore, keysAfter)
	}
	exec := target.provider.SQLX()
	rows, queryErr := exec.QueryxContext(ctx, exec.Rebind(dg5OutboxStatement(target,
		`SELECT payload, aggregateId, scopeId, eventType FROM sys_outbox_event WHERE eventOwner = ?`,
		`SELECT payload, "aggregateId", "scopeId", "eventType" FROM sys_outbox_event WHERE "eventOwner" = ?`)), cachegovdomain.OutboxOwner)
	if queryErr != nil {
		t.Fatalf("query DG5 outbox confidentiality: %v", queryErr)
	}
	defer rows.Close()
	for rows.Next() {
		var payload, aggregateID, scopeID, eventType string
		if scanErr := rows.Scan(&payload, &aggregateID, &scopeID, &eventType); scanErr != nil {
			t.Fatalf("scan DG5 outbox confidentiality: %v", scanErr)
		}
		if scopeID != cachegovdomain.ScopeID || eventType != cachegovdomain.EventType {
			t.Fatal("DG5 outbox row escaped strict owner/scope/type routing")
		}
		if strings.Contains(payload, canary) || strings.Contains(aggregateID, canary) {
			t.Fatal("DG5 outbox leaked a sensitive configuration value")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate DG5 outbox confidentiality: %v", err)
	}
}

func dg5OutboxOwnerCount(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) int {
	t.Helper()
	var count int
	exec := target.provider.SQLX()
	if err := exec.QueryRowxContext(ctx, exec.Rebind(dg5OutboxStatement(target,
		`SELECT COUNT(1) FROM sys_outbox_event WHERE eventOwner = ?`,
		`SELECT COUNT(1) FROM sys_outbox_event WHERE "eventOwner" = ?`)), cachegovdomain.OutboxOwner).Scan(&count); err != nil {
		t.Fatalf("count DG5 outbox rows: %v", err)
	}
	return count
}

func dg5OutboxStatusCount(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, status string) int {
	t.Helper()
	var count int
	exec := target.provider.SQLX()
	if err := exec.QueryRowxContext(ctx, exec.Rebind(dg5OutboxStatement(target,
		`SELECT COUNT(1) FROM sys_outbox_event WHERE eventOwner = ? AND status = ?`,
		`SELECT COUNT(1) FROM sys_outbox_event WHERE "eventOwner" = ? AND status = ?`)), cachegovdomain.OutboxOwner, status).Scan(&count); err != nil {
		t.Fatalf("count DG5 outbox status: %v", err)
	}
	return count
}

func dg5OutboxIDStats(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) (int, int) {
	t.Helper()
	var count, distinct int
	exec := target.provider.SQLX()
	if err := exec.QueryRowxContext(ctx, exec.Rebind(dg5OutboxStatement(target,
		`SELECT COUNT(1), COUNT(DISTINCT id) FROM sys_outbox_event WHERE eventOwner = ? AND scopeId = ?`,
		`SELECT COUNT(1), COUNT(DISTINCT id) FROM sys_outbox_event WHERE "eventOwner" = ? AND "scopeId" = ?`)), cachegovdomain.OutboxOwner, cachegovdomain.ScopeID).Scan(&count, &distinct); err != nil {
		t.Fatalf("count distinct DG5 outbox IDs: %v", err)
	}
	return count, distinct
}

func dg5AssertConsumerRedelivery(t *testing.T, ctx context.Context, cfg sharedconfig.Config, prefix string, rabbitCfg sharedconfig.RabbitMQConfig, publisher *rabbitinfra.Client, outbox *cachegovinfra.OutboxAdapter, broker *cachegovinfra.FanoutAdapter) {
	t.Helper()
	manager, governed, redis := newDG5AcceptanceManager(t, ctx, cfg, prefix, "redelivery")
	_ = manager
	t.Cleanup(func() { _ = redis.Close() })
	crashedRabbit, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ redelivery consumer: %v", err)
	}
	t.Cleanup(func() { _ = crashedRabbit.Close() })
	generation := cachegovinfra.NewGenerationAdapter(governed)
	consumer, err := cachegovinfra.NewFanoutAdapter(crashedRabbit, generation, prefix+"-redelivery", true)
	if err != nil {
		t.Fatalf("create DG5 redelivery fanout: %v", err)
	}
	service := cachegovapp.NewService(outbox, generation, consumer, outbox, "dg5-relay-redelivery")
	consumeCtx, cancelCrash := context.WithCancel(ctx)
	t.Cleanup(cancelCrash)
	event, err := cachegovdomain.NewInvalidationEvent(fmt.Sprintf("dg5-redelivery-%d", time.Now().UnixNano()), cachepolicy.DataClassDictPublicItems)
	if err != nil {
		t.Fatalf("create DG5 redelivery event: %v", err)
	}
	var firstAttempts atomic.Int32
	var firstEvicted atomic.Bool
	crashResult := make(chan error, 1)
	go func() {
		crashResult <- consumer.Consume(consumeCtx, func(messageCtx context.Context, received cachegovdomain.InvalidationEvent) error {
			if received.EventID != event.EventID {
				return service.HandleFanout(messageCtx, received)
			}
			if firstAttempts.Add(1) != 1 {
				return errors.New("crashed DG5 consumer received a retry before its connection closed")
			}
			if err := service.HandleFanout(messageCtx, received); err != nil {
				return err
			}
			firstEvicted.Store(true)
			// This is deliberately after the L1 eviction and before the generic
			// transport ACK. Closing the real AMQP connection makes RabbitMQ
			// requeue the unacknowledged delivery; it is not a same-connection
			// NACK/retry simulation.
			if err := crashedRabbit.Close(); err != nil {
				return fmt.Errorf("close DG5 consumer connection before ACK: %w", err)
			}
			return errors.New("simulate consumer process loss before ACK")
		})
	}()
	waitDG5(t, "DG5 redelivery consumer healthy", func() bool { return governed.GovernedStatus().FanoutHealthy })
	if err := broker.Publish(ctx, event); err != nil {
		t.Fatalf("publish DG5 redelivery event with confirm: %v", err)
	}
	waitDG5(t, "consumer connection loss after pre-ACK cache eviction", func() bool {
		return firstEvicted.Load() && firstAttempts.Load() == 1 && !governed.GovernedStatus().FanoutHealthy
	})
	select {
	case consumeErr := <-crashResult:
		if consumeErr != nil && !errors.Is(consumeErr, context.Canceled) {
			t.Fatalf("DG5 crashed consumer returned unexpected error: %v", consumeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DG5 crashed consumer did not stop after its AMQP connection closed")
	}

	// Start a separate real connection on the same durable queue. The generic
	// transport exposes RabbitMQ's Redelivered bit while the test still uses
	// the DG5 Sonic allowlist decoder and the actual cache-governance handler.
	// This makes the assertion specifically about crash-before-ACK recovery,
	// rather than the ordinary retry/NACK path.
	recoveryRabbit, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect replacement RabbitMQ redelivery consumer: %v", err)
	}
	t.Cleanup(func() { _ = recoveryRabbit.Close() })
	recoveryAdapter, err := cachegovinfra.NewFanoutAdapter(recoveryRabbit, generation, prefix+"-redelivery", true)
	if err != nil {
		t.Fatalf("create replacement DG5 redelivery fanout: %v", err)
	}
	recoveryCtx, cancelRecovery := context.WithCancel(ctx)
	t.Cleanup(cancelRecovery)
	var recovered atomic.Bool
	var observedNonRedelivery atomic.Bool
	recoveryResult := make(chan error, 1)
	go func() {
		recoveryResult <- rabbitinfra.ConsumeJSONWithDecoder(recoveryCtx, recoveryRabbit, recoveryAdapter.QueueName(), "dg5-redelivery-recovery", func(payload []byte) (cachepolicy.InvalidationEnvelope, error) {
			if len(payload) == 0 || len(payload) > cachegovinfra.MaxFanoutEnvelopeBytes {
				return cachepolicy.InvalidationEnvelope{}, cachepolicy.ErrInvalidationEnvelope
			}
			return cachepolicy.DecodeInvalidationEnvelope(payload)
		}, nil, func(messageCtx context.Context, delivery rabbitinfra.Delivery[cachepolicy.InvalidationEnvelope]) error {
			if delivery.Message.EventID != event.EventID {
				return service.HandleFanout(messageCtx, delivery.Message)
			}
			if !delivery.Redelivered {
				observedNonRedelivery.Store(true)
				return rabbitinfra.PermanentConsumeError(errors.New("replacement consumer received an unmarked DG5 redelivery"))
			}
			if err := service.HandleFanout(messageCtx, delivery.Message); err != nil {
				return err
			}
			recovered.Store(true)
			return nil
		})
	}()
	waitDG5(t, "replacement consumer receives RabbitMQ-marked redelivery", func() bool {
		return recovered.Load() || observedNonRedelivery.Load()
	})
	if observedNonRedelivery.Load() {
		t.Fatal("DG5 replacement consumer did not receive RabbitMQ Redelivered=true after crash-before-ACK")
	}
	cancelRecovery()
	select {
	case <-recoveryResult:
	case <-time.After(2 * time.Second):
		t.Fatal("DG5 replacement consumer did not stop after recovery assertion")
	}
	t.Log("DG5 crash-before-ACK delivery was requeued to a separate RabbitMQ consumer with Redelivered=true")
	if err := publisher.Publish(ctx, rabbitinfra.PublishOptions{Exchange: cachegovinfra.FanoutExchange, MessageID: fmt.Sprintf("dg5-opaque-%d", time.Now().UnixNano()), Payload: event}); err != nil {
		t.Fatalf("publish confirmed opaque DG5 fanout probe: %v", err)
	}
}

// dg5AssertHostileFanoutIsRejectedAndObservable publishes through the real
// local RabbitMQ exchange. The body is deliberately content-free apart from
// a benign unexpected field: the strict Sonic decoder must reject it before
// application eviction, then expose only an aggregate rejection count.
func dg5AssertHostileFanoutIsRejectedAndObservable(t *testing.T, ctx context.Context, publisher *rabbitinfra.Client, firstBroker, secondBroker *cachegovinfra.FanoutAdapter, first, second cacheinfra.GovernedCache) {
	t.Helper()
	if publisher == nil || firstBroker == nil || secondBroker == nil || first == nil || second == nil {
		t.Fatal("DG5 hostile fanout probe requires two real broker adapters and cache instances")
	}
	event, err := cachegovdomain.NewInvalidationEvent(fmt.Sprintf("dg5-hostile-%d", time.Now().UnixNano()), cachepolicy.DataClassConfigPublicScalar)
	if err != nil {
		t.Fatalf("create hostile DG5 event envelope: %v", err)
	}
	type hostileEnvelope struct {
		SchemaVersion int                   `json:"schemaVersion"`
		EventID       string                `json:"eventId"`
		ScopeID       string                `json:"scopeId"`
		DataClass     cachepolicy.DataClass `json:"dataClass"`
		TargetDigest  string                `json:"targetDigest"`
		Unexpected    string                `json:"unexpected"`
	}
	firstBefore := first.GovernedStatus()
	secondBefore := second.GovernedStatus()
	if err := publisher.Publish(ctx, rabbitinfra.PublishOptions{
		Exchange:  cachegovinfra.FanoutExchange,
		MessageID: event.EventID,
		Payload: hostileEnvelope{
			SchemaVersion: event.SchemaVersion,
			EventID:       event.EventID,
			ScopeID:       event.ScopeID,
			DataClass:     event.DataClass,
			TargetDigest:  event.TargetDigest,
			Unexpected:    "reject-me",
		},
	}); err != nil {
		t.Fatalf("publish hostile DG5 fanout envelope: %v", err)
	}
	waitDG5(t, "hostile DG5 fanout rejection is observable", func() bool {
		firstAfter := first.GovernedStatus()
		secondAfter := second.GovernedStatus()
		firstDLQ, firstErr := firstBroker.DeadLetterCount(ctx)
		secondDLQ, secondErr := secondBroker.DeadLetterCount(ctx)
		return firstErr == nil && secondErr == nil &&
			firstAfter.RejectedFanoutMessages > firstBefore.RejectedFanoutMessages &&
			secondAfter.RejectedFanoutMessages > secondBefore.RejectedFanoutMessages &&
			firstDLQ >= 1 && secondDLQ >= 1
	})
	if firstAfter, secondAfter := first.GovernedStatus(), second.GovernedStatus(); firstAfter.DirtyClasses != firstBefore.DirtyClasses || secondAfter.DirtyClasses != secondBefore.DirtyClasses {
		t.Fatal("hostile fanout payload changed writer-dirty cache state")
	}
}

// dg5AssertOversizedFanoutIsRejectedAndObservable proves that the wire-size
// limit is enforced by actual DG5 consumers before Sonic receives a hostile
// body. Only content-free diagnostics may reach each controlled terminal DLQ.
func dg5AssertOversizedFanoutIsRejectedAndObservable(t *testing.T, ctx context.Context, publisher *rabbitinfra.Client, firstBroker, secondBroker *cachegovinfra.FanoutAdapter, first, second cacheinfra.GovernedCache) {
	t.Helper()
	if publisher == nil || firstBroker == nil || secondBroker == nil || first == nil || second == nil {
		t.Fatal("DG5 oversized fanout probe requires two real broker adapters and cache instances")
	}
	firstBefore := first.GovernedStatus()
	secondBefore := second.GovernedStatus()
	firstDLQBefore, firstDLQErr := firstBroker.DeadLetterCount(ctx)
	secondDLQBefore, secondDLQErr := secondBroker.DeadLetterCount(ctx)
	if firstDLQErr != nil || secondDLQErr != nil {
		t.Fatalf("read DG5 terminal diagnostic counts before oversized probe: first=%v second=%v", firstDLQErr, secondDLQErr)
	}
	if err := publisher.PublishRaw(ctx, rabbitinfra.RawPublishOptions{
		Exchange:  cachegovinfra.FanoutExchange,
		MessageID: fmt.Sprintf("dg5-oversized-%d", time.Now().UnixNano()),
		Body:      make([]byte, cachegovinfra.MaxFanoutEnvelopeBytes+1),
	}); err != nil {
		t.Fatalf("publish oversized DG5 fanout envelope: %v", err)
	}
	waitDG5(t, "oversized DG5 fanout rejection is observable", func() bool {
		firstAfter := first.GovernedStatus()
		secondAfter := second.GovernedStatus()
		firstDLQ, firstErr := firstBroker.DeadLetterCount(ctx)
		secondDLQ, secondErr := secondBroker.DeadLetterCount(ctx)
		return firstErr == nil && secondErr == nil &&
			firstAfter.RejectedFanoutMessages > firstBefore.RejectedFanoutMessages &&
			secondAfter.RejectedFanoutMessages > secondBefore.RejectedFanoutMessages &&
			firstDLQ >= firstDLQBefore+1 && secondDLQ >= secondDLQBefore+1
	})
}

// dg5AssertRestartAndPausedRelayUseFreshnessFence exercises a genuinely
// offline B instance without granting the test a destructive generic queue
// API. Before the relay runs, both an offline B and a reconnected B must read
// the committed value through the source-adjacent outbox fence; RabbitMQ is a
// durable broadcast/recovery path, never the only cross-instance freshness
// mechanism.
func dg5AssertRestartAndPausedRelayUseFreshnessFence(t *testing.T, ctx context.Context, offline *rabbitinfra.Client, offlineBroker *cachegovinfra.FanoutAdapter, producerRelay, offlineRelay *cachegovapp.Service, offlineGoverned cacheinfra.GovernedCache, writer, reader *configapp.Service, repo *configinfra.Repository, actor configapp.Actor, configID, baseID int64) string {
	t.Helper()
	if offline == nil || offlineBroker == nil || producerRelay == nil || offlineRelay == nil || offlineGoverned == nil || writer == nil || reader == nil || repo == nil {
		t.Fatal("DG5 late-instance probe requires real RabbitMQ, Redis, cache, and application services")
	}
	deadLettersBefore, deadLetterErr := offlineBroker.DeadLetterCount(ctx)
	if deadLetterErr != nil || deadLettersBefore < 1 {
		t.Fatalf("DG5 durable terminal diagnostic missing before instance restart: count=%d err=%v", deadLettersBefore, deadLetterErr)
	}

	if err := offline.Close(); err != nil {
		t.Fatalf("close late-instance RabbitMQ client: %v", err)
	}
	waitDG5(t, "late-instance consumer becomes untrusted before paused relay", func() bool {
		return !offlineGoverned.GovernedStatus().FanoutHealthy
	})

	updated := fmt.Sprintf("#%06x", (baseID+5)&0xffffff)
	updateDG5ConfigValue(t, ctx, writer, repo, actor, configID, updated)
	if value, readErr := dg5ReadConfig(ctx, reader, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != updated {
		t.Fatalf("offline instance returned stale value while relay was paused: value=%q err=%v", value, readErr)
	}

	if err := offlineBroker.Reconnect(ctx); err != nil {
		t.Fatalf("reconnect late-instance RabbitMQ client: %v", err)
	}
	waitDG5(t, "terminal DLQ diagnostic survives instance restart", func() bool {
		count, countErr := offlineBroker.DeadLetterCount(ctx)
		return countErr == nil && count >= deadLettersBefore
	})
	rejoinedCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	startDG5Consumer(t, rejoinedCtx, offlineBroker, offlineRelay, offlineGoverned)
	waitDG5(t, "rejoined instance fanout becomes healthy", func() bool {
		return offlineGoverned.GovernedStatus().FanoutHealthy
	})
	if value, readErr := dg5ReadConfig(ctx, reader, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor"); readErr != nil || value != updated {
		t.Fatalf("rejoined instance returned stale value before delayed relay: value=%q err=%v", value, readErr)
	}
	dg5RelayAll(t, ctx, producerRelay)
	waitDG5(t, "rejoined instance converges after relay recovery", func() bool {
		value, readErr := dg5ReadConfig(ctx, reader, configapp.Actor{}, "SEVEN_FRONTEND_METADATA.themePrimaryColor")
		status := offlineGoverned.GovernedStatus()
		return readErr == nil && value == updated && status.FanoutHealthy && status.RedisHealthy && status.FreshnessHealthy && status.ReadTrusted
	})
	return updated
}

func waitDG5(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", description)
}

func dg5OutboxStatement(target *dg5IsolatedDatabase, mysql, postgres string) string {
	if target != nil && target.dialect == "postgres" {
		return postgres
	}
	return mysql
}

type dg5AcceptanceSecretCipher struct{}

func (dg5AcceptanceSecretCipher) EncryptString(_ context.Context, plain string) (configdomain.ConfigSecretValue, error) {
	return configdomain.ConfigSecretValue{Plain: plain, CiphertextB64: "dg5-ciphertext", EDEKB64: "dg5-edek", WrapKeyRef: "dg5-test-key"}, nil
}

func (dg5AcceptanceSecretCipher) DecryptString(_ context.Context, value configdomain.ConfigSecretValue) (string, error) {
	return value.Plain, nil
}

type dg5PublishThenUnknown struct {
	delegate cachegovdomain.FanoutPort
	once     sync.Once
}

func (p *dg5PublishThenUnknown) Enabled() bool {
	return p != nil && p.delegate != nil && p.delegate.Enabled()
}

func (p *dg5PublishThenUnknown) Publish(ctx context.Context, event cachegovdomain.InvalidationEvent) error {
	if p == nil || p.delegate == nil {
		return cachegovdomain.ErrFanoutUnavailable
	}
	if err := p.delegate.Publish(ctx, event); err != nil {
		return err
	}
	unknown := false
	p.once.Do(func() { unknown = true })
	if unknown {
		return errors.New("simulated publisher confirmation unknown after broker delivery")
	}
	return nil
}
