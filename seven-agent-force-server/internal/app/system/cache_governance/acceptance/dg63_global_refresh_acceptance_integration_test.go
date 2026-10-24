package acceptance

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cachegovapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/application"
	cachegovinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/infrastructure"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

// TestDG63GlobalRefreshAcceptance uses both isolated SQL dialects (selected
// one per run), two independent governed L1 instances, real local Redis and
// RabbitMQ, and the shared durable sys_outbox_event table. It intentionally
// leaves the relay paused between the application Refresh commit and B's
// reads, proving the V3 freshness fence forces source-only behavior before
// any broker delivery exists.
func TestDG63GlobalRefreshAcceptance(t *testing.T) {
	if strings.ToLower(strings.TrimSpace(os.Getenv(dg5AcceptanceEnv))) != dg5AcceptanceApply {
		t.Skip("set DG5_CACHE_GOVERNANCE_ACCEPTANCE=apply after isolated migration verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	assertDG5AcceptanceSchema(t, ctx, target)
	dg5ResetAcceptanceOutbox(t, ctx, target)

	prefix := fmt.Sprintf("seven-dg63-%s-%d", target.dialect, time.Now().UnixNano())
	managerA, governedA, redisA := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "a")
	managerB, governedB, redisB := newDG5AcceptanceManager(t, ctx, target.cfg, prefix, "b")
	t.Cleanup(func() { _ = redisA.Close() })
	t.Cleanup(func() { _ = redisB.Close() })
	targetedA := managerA.(cacheinfra.TargetedGovernedCache)
	targetedB := managerB.(cacheinfra.TargetedGovernedCache)

	rabbitCfg := dg5AcceptanceRabbitConfig(target.cfg)
	rabbitA, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ A: %v", err)
	}
	t.Cleanup(func() { _ = rabbitA.Close() })
	rabbitB, err := rabbitinfra.New(rabbitCfg)
	if err != nil {
		t.Fatalf("connect local RabbitMQ B: %v", err)
	}
	t.Cleanup(func() { _ = rabbitB.Close() })
	generationA := cachegovinfra.NewGenerationAdapter(governedA)
	generationB := cachegovinfra.NewGenerationAdapter(governedB)
	brokerA, err := cachegovinfra.NewFanoutAdapter(rabbitA, generationA, prefix+"-a", true)
	if err != nil {
		t.Fatalf("create A fanout adapter: %v", err)
	}
	brokerB, err := cachegovinfra.NewFanoutAdapter(rabbitB, generationB, prefix+"-b", true)
	if err != nil {
		t.Fatalf("create B fanout adapter: %v", err)
	}
	idsA, err := xid.New(83)
	if err != nil {
		t.Fatal(err)
	}
	idsB, err := xid.New(84)
	if err != nil {
		t.Fatal(err)
	}
	outboxA := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), idsA.NextID)
	outboxB := cachegovinfra.NewOutboxAdapter(target.provider.SQLX(), idsB.NextID)
	governedA.SetFreshnessGate(outboxA)
	governedB.SetFreshnessGate(outboxB)
	targetedA.SetTargetFreshnessGate(outboxA)
	targetedB.SetTargetFreshnessGate(outboxB)
	relayA := cachegovapp.NewService(outboxA, generationA, brokerA, outboxA, "dg63-relay-a")
	relayB := cachegovapp.NewService(outboxB, generationB, brokerB, outboxB, "dg63-relay-b")
	refreshA := cachegovapp.NewRefreshService(target.provider.Transactor(), outboxA, generationA, brokerA, outboxA)
	refreshB := cachegovapp.NewRefreshService(target.provider.Transactor(), outboxB, generationB, brokerB, outboxB)
	relayA.BindRefresh(refreshA)
	relayB.BindRefresh(refreshB)
	consumerCtx, stopConsumers := context.WithCancel(ctx)
	consumerDone := make(chan struct{}, 2)
	go func() {
		defer func() { consumerDone <- struct{}{} }()
		_ = brokerA.ConsumeGoverned(consumerCtx, relayA.HandleFanout, relayA.HandleTargetedFanout, relayA.HandleRefreshFanout)
	}()
	go func() {
		defer func() { consumerDone <- struct{}{} }()
		_ = brokerB.ConsumeGoverned(consumerCtx, relayB.HandleFanout, relayB.HandleTargetedFanout, relayB.HandleRefreshFanout)
	}()
	// This cleanup runs before the RabbitMQ clients' earlier cleanups. It is a
	// regression guard for the exact race-hang diagnosis: consumer cancellation
	// must return before Client.Close performs its bounded connection shutdown.
	t.Cleanup(func() {
		stopConsumers()
		for index := 0; index < 2; index++ {
			select {
			case <-consumerDone:
			case <-time.After(2 * time.Second):
				t.Errorf("DG6.3 RabbitMQ consumer %d did not stop after context cancellation", index+1)
			}
		}
	})
	waitDG5(t, "DG6.3 consumers healthy", func() bool {
		return governedA.GovernedStatus().FanoutHealthy && governedB.GovernedStatus().FanoutHealthy
	})

	configRequest, _ := cachepolicy.ConfigReadRequest("SEVEN_FRONTEND_METADATA.title", "dg63", "anonymous")
	sessionRequest, _ := cachepolicy.ActiveSessionValidityReadRequest("dg63-session")
	var configLoads atomic.Int32
	configLoader := func(value string) cacheinfra.ClassifiedLoader {
		return func(context.Context) (cachepolicy.CacheableValue, error) {
			configLoads.Add(1)
			return cachepolicy.CacheableValue{Value: map[string]string{"value": value}, Cacheable: true}, nil
		}
	}
	var sessionLoads atomic.Int32
	sessionLoader := func(value string) cacheinfra.TargetedLoader {
		return func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
			sessionLoads.Add(1)
			return cachepolicy.TargetedCacheableValue{Value: map[string]string{"value": value}, Cacheable: true, ExpiresAt: time.Now().UTC().Add(time.Minute)}, nil
		}
	}
	warmV1V2 := func(manager cacheinfra.Manager, value string) {
		var configValue, sessionValue map[string]string
		if found, err := manager.(cacheinfra.GovernedCache).GetOrLoadClassified(ctx, configRequest, &configValue, configLoader(value)); err != nil || !found {
			t.Fatalf("warm V1 found=%v err=%v", found, err)
		}
		if found, err := manager.(cacheinfra.TargetedGovernedCache).GetOrLoadTargeted(ctx, sessionRequest, &sessionValue, sessionLoader(value)); err != nil || !found {
			t.Fatalf("warm V2 found=%v err=%v", found, err)
		}
	}
	warmV1V2(managerB, "before")
	warmV1V2(managerB, "unexpected")
	if configLoads.Load() != 1 || sessionLoads.Load() != 1 {
		t.Fatalf("B was not genuinely warm: v1=%d v2=%d", configLoads.Load(), sessionLoads.Load())
	}

	if result, err := refreshA.Refresh(ctx); err != nil || result.State != "PENDING" {
		t.Fatalf("submit protected refresh: result=%+v err=%v", result, err)
	}
	// A double click is deliberately coalesced against the same durable active
	// operation; it must not create a second global refresh event.
	if result, err := refreshA.Refresh(ctx); err != nil || result.State != "PENDING" {
		t.Fatalf("coalesce concurrent protected refresh: result=%+v err=%v", result, err)
	}
	if pending := dg5OutboxStatusCount(t, ctx, target, "PENDING"); pending != 1 {
		t.Fatalf("expected exactly one pending V3 refresh before relay, got %d", pending)
	}
	// No RelayOutbox call has occurred. The second instance must source-load
	// both V1 and V2 now; returning either original warm value would violate the
	// global V3 zero-stale fence.
	var pendingConfig, pendingSession map[string]string
	if found, err := governedB.GetOrLoadClassified(ctx, configRequest, &pendingConfig, configLoader("after-pending")); err != nil || !found || pendingConfig["value"] != "after-pending" || configLoads.Load() != 2 {
		t.Fatalf("B returned V1 stale candidate before relay: found=%v value=%v loads=%d err=%v", found, pendingConfig, configLoads.Load(), err)
	}
	if found, err := targetedB.GetOrLoadTargeted(ctx, sessionRequest, &pendingSession, sessionLoader("after-pending")); err != nil || !found || pendingSession["value"] != "after-pending" || sessionLoads.Load() != 2 {
		t.Fatalf("B returned V2 stale candidate before relay: found=%v value=%v loads=%d err=%v", found, pendingSession, sessionLoads.Load(), err)
	}

	if err := relayA.RelayOutbox(ctx, 10); err != nil {
		t.Fatalf("relay V3 refresh: %v", err)
	}
	waitDG5(t, "DG6.3 V3 fanout evicts both governed L1 instances", func() bool {
		return dg5OutboxStatusCount(t, ctx, target, "PENDING") == 0 && governedA.GovernedStatus().GlobalRefreshEvictions > 0 && governedB.GovernedStatus().GlobalRefreshEvictions > 0
	})
	var afterConfig, afterSession map[string]string
	if found, err := governedB.GetOrLoadClassified(ctx, configRequest, &afterConfig, configLoader("after-fanout")); err != nil || !found || afterConfig["value"] != "after-fanout" || configLoads.Load() != 3 {
		t.Fatalf("V3 fanout did not evict B V1 L1: found=%v value=%v loads=%d err=%v", found, afterConfig, configLoads.Load(), err)
	}
	if found, err := targetedB.GetOrLoadTargeted(ctx, sessionRequest, &afterSession, sessionLoader("after-fanout")); err != nil || !found || afterSession["value"] != "after-fanout" || sessionLoads.Load() != 3 {
		t.Fatalf("V3 fanout did not evict B V2 L1: found=%v value=%v loads=%d err=%v", found, afterSession, sessionLoads.Load(), err)
	}
	dg63AssertGlobalFenceLinearizesOverlappingCandidateReads(t, ctx, target, outboxA, outboxB, relayA, governedB, targetedB, configRequest, sessionRequest, configLoader, sessionLoader)
	dg63AssertInvalidV3DeadRecoversCandidateEligibility(t, ctx, target, outboxA, relayA, governedB, targetedB, configRequest, sessionRequest, configLoader, sessionLoader, &configLoads, &sessionLoads)
	dg63AssertV3OutboxIsContentFree(t, ctx, target)
	dg63AssertExpiredV3LeaseIsReclaimedAfterRelayRestart(t, ctx, target, outboxA, relayB)
	dg63AssertRabbitOutageFailsClosedThenRecovers(t, ctx, target, outboxA, relayA, relayB, rabbitA, governedB, configRequest, configLoader, &configLoads)
	dg63AssertRedisOutageFailsClosed(t, ctx, governedB, redisB, configRequest, configLoader, &configLoads)
}

func dg63AssertInvalidV3DeadRecoversCandidateEligibility(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, writer *cachegovinfra.OutboxAdapter, relay *cachegovapp.Service, governed cacheinfra.GovernedCache, targeted cacheinfra.TargetedGovernedCache, config cachepolicy.ReadRequest, session cachepolicy.TargetedReadRequest, configLoader func(string) cacheinfra.ClassifiedLoader, sessionLoader func(string) cacheinfra.TargetedLoader, configLoads, sessionLoads *atomic.Int32) {
	t.Helper()
	if governed == nil || targeted == nil || configLoads == nil || sessionLoads == nil {
		t.Fatal("invalid V3 probe requires both governed caches and loader counters")
	}
	query := `UPDATE sys_outbox_event SET payload=? WHERE eventId=? AND eventType=?`
	if target.dialect == "postgres" {
		query = `UPDATE sys_outbox_event SET payload=$1 WHERE "eventId"=$2 AND "eventType"=$3`
	}
	exec := target.provider.SQLX()
	// The preceding deterministic V1/V2 overlap probes each fan out a valid
	// refresh. Warm both candidates again so this check can distinguish a
	// terminal-invalid V3 from an already-missing candidate.
	var baselineConfig, baselineSession map[string]string
	if found, err := governed.GetOrLoadClassified(ctx, config, &baselineConfig, configLoader("before-invalid-v3")); err != nil || !found {
		t.Fatalf("warm V1 before invalid V3: found=%v err=%v", found, err)
	}
	if found, err := targeted.GetOrLoadTargeted(ctx, session, &baselineSession, sessionLoader("before-invalid-v3")); err != nil || !found {
		t.Fatalf("warm V2 before invalid V3: found=%v err=%v", found, err)
	}
	baselineEvictions := governed.GovernedStatus().GlobalRefreshEvictions
	baselineConfigLoads, baselineSessionLoads := configLoads.Load(), sessionLoads.Load()
	for _, invalid := range []struct {
		name    string
		payload string
	}{
		{name: "malformed", payload: `{"schemaVersion":3,"unexpected":true}`},
		{name: "oversized", payload: strings.Repeat("x", cachepolicy.MaxInvalidationEnvelopeBytes+1)},
	} {
		event, err := cachepolicy.NewCacheRefreshEnvelope(fmt.Sprintf("dg63-dead-%s-%d", invalid.name, time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		if err := target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error { return writer.AppendRefresh(txCtx, event) }); err != nil {
			t.Fatalf("append %s V3 fixture: %v", invalid.name, err)
		}
		if _, err := exec.ExecContext(ctx, exec.Rebind(query), invalid.payload, event.EventID, cachepolicy.CacheRefreshEventType); err != nil {
			t.Fatalf("corrupt %s V3 fixture: %v", invalid.name, err)
		}
		if err := relay.RelayOutbox(ctx, 10); err != nil {
			t.Fatalf("relay %s V3: %v", invalid.name, err)
		}
		if status := dg63OutboxEventStatus(t, ctx, target, event.EventID); status != "DEAD" {
			t.Fatalf("%s V3 was not terminal DEAD: %q", invalid.name, status)
		}
		if evictions := governed.GovernedStatus().GlobalRefreshEvictions; evictions != baselineEvictions {
			t.Fatalf("%s V3 advanced global epoch or evicted L1: got %d want %d", invalid.name, evictions, baselineEvictions)
		}
		var cachedConfig, cachedSession map[string]string
		if found, err := governed.GetOrLoadClassified(ctx, config, &cachedConfig, configLoader("invalid-v3-reload")); err != nil || !found || configLoads.Load() != baselineConfigLoads {
			t.Fatalf("%s V3 changed V1 candidate eligibility: found=%v loads=%d baseline=%d err=%v", invalid.name, found, configLoads.Load(), baselineConfigLoads, err)
		}
		if found, err := targeted.GetOrLoadTargeted(ctx, session, &cachedSession, sessionLoader("invalid-v3-reload")); err != nil || !found || sessionLoads.Load() != baselineSessionLoads {
			t.Fatalf("%s V3 changed V2 candidate eligibility: found=%v loads=%d baseline=%d err=%v", invalid.name, found, sessionLoads.Load(), baselineSessionLoads, err)
		}
	}
	// A terminal malformed event is safely ignored by V3 only. Its status must
	// not weaken the V1/V2 predicate, and a later unknown state remains
	// fail-closed through real governed reads rather than lease inspection alone.
	eventID := fmt.Sprintf("dg63-dead-malformed-%d", time.Now().UnixNano())
	statusQuery := `UPDATE sys_outbox_event SET status='UNKNOWN_V3' WHERE eventType=? AND eventOwner=? AND scopeId=? AND status='DEAD' ORDER BY id DESC LIMIT 1`
	if target.dialect == "postgres" {
		statusQuery = `UPDATE sys_outbox_event SET status='UNKNOWN_V3' WHERE id=(SELECT id FROM sys_outbox_event WHERE "eventType"=$1 AND "eventOwner"=$2 AND "scopeId"=$3 AND status='DEAD' ORDER BY id DESC LIMIT 1)`
	}
	if _, err := exec.ExecContext(ctx, exec.Rebind(statusQuery), cachepolicy.CacheRefreshEventType, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal); err != nil {
		t.Fatalf("set unknown V3 status fixture: %v", err)
	}
	// The unknown row is deliberately the latest terminal row. Both actual
	// managed reads must execute their authoritative loaders; direct lease
	// checks alone could not rule out a service-level fallback.
	var unknownConfig, unknownSession map[string]string
	if found, err := governed.GetOrLoadClassified(ctx, config, &unknownConfig, configLoader("unknown-v3-v1")); err != nil || !found || unknownConfig["value"] != "unknown-v3-v1" || configLoads.Load() != baselineConfigLoads+1 {
		t.Fatalf("unknown V3 did not force V1 authority load: found=%v value=%v loads=%d err=%v", found, unknownConfig, configLoads.Load(), err)
	}
	if found, err := targeted.GetOrLoadTargeted(ctx, session, &unknownSession, sessionLoader("unknown-v3-v2")); err != nil || !found || unknownSession["value"] != "unknown-v3-v2" || sessionLoads.Load() != baselineSessionLoads+1 {
		t.Fatalf("unknown V3 did not force V2 authority load: found=%v value=%v loads=%d err=%v", found, unknownSession, sessionLoads.Load(), err)
	}
	if _, err := exec.ExecContext(ctx, exec.Rebind(strings.Replace(statusQuery, "'UNKNOWN_V3'", "'DEAD'", 1)), cachepolicy.CacheRefreshEventType, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal); err != nil {
		t.Fatalf("restore terminal V3 status fixture: %v", err)
	}
	// A valid V3 can recover candidate operation after the invalid diagnostics.
	valid, err := cachepolicy.NewCacheRefreshEnvelope(eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error { return writer.AppendRefresh(txCtx, valid) }); err != nil {
		t.Fatalf("append valid V3 after DEAD diagnostics: %v", err)
	}
	if err := relay.RelayOutbox(ctx, 10); err != nil {
		t.Fatalf("relay valid V3 after DEAD diagnostics: %v", err)
	}
	waitDG5(t, "valid V3 recovers after terminal diagnostics", func() bool {
		return dg63OutboxEventStatus(t, ctx, target, valid.EventID) == "DONE" && governed.GovernedStatus().GlobalRefreshEvictions > baselineEvictions
	})
	var recoveredConfig, recoveredSession map[string]string
	if found, err := governed.GetOrLoadClassified(ctx, config, &recoveredConfig, configLoader("valid-v3-recovery")); err != nil || !found || recoveredConfig["value"] != "valid-v3-recovery" || configLoads.Load() != baselineConfigLoads+2 {
		t.Fatalf("valid V3 did not restore V1 after terminal diagnostics: found=%v value=%v loads=%d err=%v", found, recoveredConfig, configLoads.Load(), err)
	}
	if found, err := targeted.GetOrLoadTargeted(ctx, session, &recoveredSession, sessionLoader("valid-v3-recovery")); err != nil || !found || recoveredSession["value"] != "valid-v3-recovery" || sessionLoads.Load() != baselineSessionLoads+2 {
		t.Fatalf("valid V3 did not restore V2 after terminal diagnostics: found=%v value=%v loads=%d err=%v", found, recoveredSession, sessionLoads.Load(), err)
	}
}

func dg63OutboxEventStatus(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, eventID string) string {
	t.Helper()
	query := `SELECT status FROM sys_outbox_event WHERE eventId=? AND eventType=?`
	if target.dialect == "postgres" {
		query = `SELECT status FROM sys_outbox_event WHERE "eventId"=$1 AND "eventType"=$2`
	}
	var status string
	if err := target.provider.SQLX().QueryRowxContext(ctx, target.provider.SQLX().Rebind(query), eventID, cachepolicy.CacheRefreshEventType).Scan(&status); err != nil {
		t.Fatalf("read V3 event status: %v", err)
	}
	return status
}

// dg63AssertGlobalFenceLinearizesOverlappingCandidateReads deterministically
// holds the exact production candidate-read lease after B has entered its
// protected acceptance interval. A's V3 transaction must remain blocked until
// B releases it; immediately after A commits, B's actual governed read may not
// return the pre-existing candidate. V1 and V2 are exercised independently.
func dg63AssertGlobalFenceLinearizesOverlappingCandidateReads(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, writer, reader *cachegovinfra.OutboxAdapter, relay *cachegovapp.Service, governed cacheinfra.GovernedCache, targeted cacheinfra.TargetedGovernedCache, configRequest cachepolicy.ReadRequest, sessionRequest cachepolicy.TargetedReadRequest, configLoader func(string) cacheinfra.ClassifiedLoader, sessionLoader func(string) cacheinfra.TargetedLoader) {
	t.Helper()
	type probe struct {
		name    string
		acquire func() (cachepolicy.FreshnessLease, error)
		read    func(string) error
	}
	probes := []probe{
		{name: "v1", acquire: func() (cachepolicy.FreshnessLease, error) {
			return reader.AcquireRead(ctx, configRequest.Entry.DataClass)
		}, read: func(value string) error {
			var got map[string]string
			found, err := governed.GetOrLoadClassified(ctx, configRequest, &got, configLoader(value))
			if err != nil || !found || got["value"] != value {
				return fmt.Errorf("V1 found=%v value=%v err=%v", found, got, err)
			}
			return nil
		}},
		{name: "v2", acquire: func() (cachepolicy.FreshnessLease, error) {
			return reader.AcquireTargetedRead(ctx, sessionRequest.Entry.DataClass, sessionRequest.TargetKind, sessionRequest.TargetDigest)
		}, read: func(value string) error {
			var got map[string]string
			found, err := targeted.GetOrLoadTargeted(ctx, sessionRequest, &got, sessionLoader(value))
			if err != nil || !found || got["value"] != value {
				return fmt.Errorf("V2 found=%v value=%v err=%v", found, got, err)
			}
			return nil
		}},
	}
	for _, probe := range probes {
		lease, err := probe.acquire()
		if err != nil || lease == nil || !lease.Trusted() {
			t.Fatalf("%s acquire trusted candidate lease: %v", probe.name, err)
		}
		committed := make(chan error, 1)
		go func(name string) {
			mutation, err := writer.AcquireRefreshMutation(ctx)
			if err == nil {
				event, newErr := cachepolicy.NewCacheRefreshEnvelope("dg63-overlap-" + name + fmt.Sprintf("-%d", time.Now().UnixNano()))
				if newErr != nil {
					err = newErr
				} else {
					err = target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error { return writer.AppendRefresh(txCtx, event) })
				}
				mutation.Release()
			}
			committed <- err
		}(probe.name)
		select {
		case err := <-committed:
			t.Fatalf("%s V3 committed inside B read interval: %v", probe.name, err)
		case <-time.After(150 * time.Millisecond):
		}
		lease.Release()
		if err := <-committed; err != nil {
			t.Fatalf("%s V3 commit after release: %v", probe.name, err)
		}
		if err := probe.read("after-overlap-" + probe.name); err != nil {
			t.Fatalf("%s stale candidate after V3 commit: %v", probe.name, err)
		}
		if err := relay.RelayOutbox(ctx, 10); err != nil {
			t.Fatalf("%s relay overlap V3: %v", probe.name, err)
		}
		waitDG5(t, probe.name+" overlap V3 complete", func() bool { return dg5OutboxStatusCount(t, ctx, target, "PENDING") == 0 })
	}
}

func dg63AssertV3OutboxIsContentFree(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) {
	t.Helper()
	// Earlier probes deliberately leave malformed V3 diagnostics in DEAD.
	// createTime is not a total order on every database, so a broad
	// "not DEAD" lookup can nondeterministically select a test-only UNKNOWN
	// row instead of the successful durable envelope. Assert the last
	// completed V3 operation directly, ordered by the immutable outbox ID.
	query := `SELECT eventId, status, payload FROM sys_outbox_event WHERE eventOwner=? AND scopeId=? AND eventType=? AND status='DONE' ORDER BY id DESC LIMIT 1`
	if target.dialect == "postgres" {
		query = `SELECT "eventId", status, payload FROM sys_outbox_event WHERE "eventOwner"=$1 AND "scopeId"=$2 AND "eventType"=$3 AND status='DONE' ORDER BY id DESC LIMIT 1`
	}
	var eventID, status, payload string
	exec := target.provider.SQLX()
	if err := exec.QueryRowxContext(ctx, exec.Rebind(query), cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.CacheRefreshEventType).Scan(&eventID, &status, &payload); err != nil {
		t.Fatalf("read V3 outbox payload: %v", err)
	}
	if _, err := cachepolicy.DecodeCacheRefreshEnvelope([]byte(payload)); err != nil {
		t.Fatalf("V3 outbox did not retain strict content-free envelope: event=%q status=%q bytes=%d digest=%s err=%v", eventID, status, len(payload), cachepolicy.EventDigest(payload), err)
	}
	for _, forbidden := range []string{"session", "token", "cookie", "loginIP", "userAgent", "metadata", "refreshFamily", "before", "after"} {
		if strings.Contains(strings.ToLower(payload), strings.ToLower(forbidden)) {
			t.Fatalf("V3 outbox payload exposed forbidden content class")
		}
	}
}

// dg63AssertExpiredV3LeaseIsReclaimedAfterRelayRestart uses the same shared
// Outbox store and a separate, already-running instance-B relay. It models a
// process crash after claim but before publish confirmation: expiry permits a
// new relay to claim, advance the idempotent epoch, publish, and complete.
func dg63AssertExpiredV3LeaseIsReclaimedAfterRelayRestart(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, outbox cachepolicy.RefreshOutboxPort, restarted *cachegovapp.Service) {
	t.Helper()
	if outbox == nil || restarted == nil {
		t.Fatal("DG6.3 reclaim probe requires refresh outbox and restarted relay")
	}
	event, err := cachepolicy.NewCacheRefreshEnvelope(fmt.Sprintf("dg63-reclaim-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	if err := target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error { return outbox.AppendRefresh(txCtx, event) }); err != nil {
		t.Fatalf("append guarded V3 reclaim fixture: %v", err)
	}
	query := `UPDATE sys_outbox_event SET status='PROCESSING', leaseOwner='crashed', leaseToken='expired', leaseUntil=?, updateTime=? WHERE eventId=? AND eventType=?`
	if target.dialect == "postgres" {
		query = `UPDATE sys_outbox_event SET status='PROCESSING', "leaseOwner"='crashed', "leaseToken"='expired', "leaseUntil"=$1, "updateTime"=$2 WHERE "eventId"=$3 AND "eventType"=$4`
	}
	exec := target.provider.SQLX()
	now := time.Now().UTC()
	if _, err := exec.ExecContext(ctx, exec.Rebind(query), now.Add(-time.Minute), now, event.EventID, cachepolicy.CacheRefreshEventType); err != nil {
		t.Fatalf("expire guarded V3 lease fixture: %v", err)
	}
	if err := restarted.RelayOutbox(ctx, 10); err != nil {
		t.Fatalf("reclaim V3 relay after simulated crash: %v", err)
	}
	statusQuery := `SELECT status FROM sys_outbox_event WHERE eventId=? AND eventType=?`
	if target.dialect == "postgres" {
		statusQuery = `SELECT status FROM sys_outbox_event WHERE "eventId"=$1 AND "eventType"=$2`
	}
	var status string
	if err := exec.QueryRowxContext(ctx, exec.Rebind(statusQuery), event.EventID, cachepolicy.CacheRefreshEventType).Scan(&status); err != nil || status != "DONE" {
		t.Fatalf("V3 expired lease was not reclaimed to DONE: status=%q err=%v", status, err)
	}
}

// dg63AssertRabbitOutageFailsClosedThenRecovers closes only instance A's real
// AMQP client. It neither alters broker topology nor uses destructive broker
// commands. While confirmation is unavailable, the durable row remains
// FAILED and B must source-load; a healthy independent relay then reclaims it.
func dg63AssertRabbitOutageFailsClosedThenRecovers(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase, outbox cachepolicy.RefreshOutboxPort, failedRelay, recoveredRelay *cachegovapp.Service, failedRabbit *rabbitinfra.Client, governed cacheinfra.GovernedCache, request cachepolicy.ReadRequest, loader func(string) cacheinfra.ClassifiedLoader, loads *atomic.Int32) {
	t.Helper()
	if err := failedRabbit.Close(); err != nil {
		t.Fatalf("close only failed instance RabbitMQ client: %v", err)
	}
	event, err := cachepolicy.NewCacheRefreshEnvelope(fmt.Sprintf("dg63-rabbit-outage-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	if err := target.provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error { return outbox.AppendRefresh(txCtx, event) }); err != nil {
		t.Fatalf("append V3 RabbitMQ outage fixture: %v", err)
	}
	if err := failedRelay.RelayOutbox(ctx, 10); err != nil {
		t.Fatalf("relay should persist, not hide, RabbitMQ confirmation outage: %v", err)
	}
	if failed := dg5OutboxStatusCount(t, ctx, target, "FAILED"); failed != 1 {
		t.Fatalf("RabbitMQ outage did not retain exactly one FAILED V3 event: %d", failed)
	}
	var value map[string]string
	before := loads.Load()
	if found, err := governed.GetOrLoadClassified(ctx, request, &value, loader("rabbit-fallback")); err != nil || !found || value["value"] != "rabbit-fallback" || loads.Load() != before+1 {
		t.Fatalf("RabbitMQ uncertainty returned trusted candidate: found=%v value=%v loads=%d before=%d err=%v", found, value, loads.Load(), before, err)
	}
	time.Sleep(2200 * time.Millisecond) // tolerate the second bounded retry after a repeated local run
	if err := recoveredRelay.RelayOutbox(ctx, 10); err != nil {
		t.Fatalf("healthy independent relay recovery: %v", err)
	}
	if pending, failed := dg5OutboxStatusCount(t, ctx, target, "PENDING"), dg5OutboxStatusCount(t, ctx, target, "FAILED"); pending != 0 || failed != 0 {
		t.Fatalf("RabbitMQ recovery left V3 incomplete: pending=%d failed=%d", pending, failed)
	}
}

// dg63AssertRedisOutageFailsClosed closes only B's already-open real Redis
// client after a cache warm. The governed read must use its authoritative
// loader instead of returning B's former L1/L2 candidate.
func dg63AssertRedisOutageFailsClosed(t *testing.T, ctx context.Context, governed cacheinfra.GovernedCache, redis interface{ Close() error }, request cachepolicy.ReadRequest, loader func(string) cacheinfra.ClassifiedLoader, loads *atomic.Int32) {
	t.Helper()
	if err := redis.Close(); err != nil {
		t.Fatalf("close only B Redis client: %v", err)
	}
	var value map[string]string
	before := loads.Load()
	if found, err := governed.GetOrLoadClassified(ctx, request, &value, loader("redis-fallback")); err != nil || !found || value["value"] != "redis-fallback" || loads.Load() != before+1 {
		t.Fatalf("Redis uncertainty returned trusted candidate: found=%v value=%v loads=%d before=%d err=%v", found, value, loads.Load(), before, err)
	}
}
