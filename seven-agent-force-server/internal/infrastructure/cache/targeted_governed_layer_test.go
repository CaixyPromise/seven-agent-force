package cache

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

type trustedTargetGate struct{}

func (trustedTargetGate) AcquireTargetedRead(context.Context, cachepolicy.DataClass, string, string) (cachepolicy.FreshnessLease, error) {
	return trustedCacheFreshnessLease{}, nil
}
func (trustedTargetGate) AcquireTargetedMutation(context.Context, cachepolicy.DataClass, string, string) (cachepolicy.FreshnessLease, error) {
	return trustedCacheFreshnessLease{}, nil
}

type untrustedTargetGate struct{}

type untrustedTargetLease struct{}

func (untrustedTargetLease) Trusted() bool { return false }
func (untrustedTargetLease) Release()      {}

func (untrustedTargetGate) AcquireTargetedRead(context.Context, cachepolicy.DataClass, string, string) (cachepolicy.FreshnessLease, error) {
	return untrustedTargetLease{}, nil
}
func (untrustedTargetGate) AcquireTargetedMutation(context.Context, cachepolicy.DataClass, string, string) (cachepolicy.FreshnessLease, error) {
	return untrustedTargetLease{}, nil
}

func TestTargetedGovernedLayerFanoutEvictsOnlyExactSessionL1(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Codec: "sonic", L1: config.CacheL1Config{Enabled: true, MaxCost: 1 << 20, NumCounters: 1000, BufferItems: 64, DefaultTTL: time.Minute}, Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "dg62", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	mgr, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatal(err)
	}
	governed := mgr.(TargetedGovernedCache)
	base := mgr.(*manager).governed.(*governedLayer)
	governed.SetTargetFreshnessGate(trustedTargetGate{})
	base.SetFanoutHealthy(true)
	requestA, _ := cachepolicy.ActiveSessionValidityReadRequest("session-a")
	requestB, _ := cachepolicy.ActiveSessionValidityReadRequest("session-b")
	ctx := context.Background()
	loadsA, loadsB := 0, 0
	loadA := func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loadsA++
		return cachepolicy.TargetedCacheableValue{Value: map[string]string{"value": "a"}, Cacheable: true, ExpiresAt: time.Now().Add(time.Minute)}, nil
	}
	loadB := func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loadsB++
		return cachepolicy.TargetedCacheableValue{Value: map[string]string{"value": "b"}, Cacheable: true, ExpiresAt: time.Now().Add(time.Minute)}, nil
	}
	var a, b map[string]string
	if ok, err := governed.GetOrLoadTargeted(ctx, requestA, &a, loadA); err != nil || !ok {
		t.Fatalf("warm a ok=%v err=%v", ok, err)
	}
	if ok, err := governed.GetOrLoadTargeted(ctx, requestB, &b, loadB); err != nil || !ok {
		t.Fatalf("warm b ok=%v err=%v", ok, err)
	}
	if _, err := governed.AdvanceTargetGeneration(ctx, "event-a", requestA); err != nil {
		t.Fatal(err)
	}
	governed.EvictTargetLocalAndResolve("event-a", requestA)
	if ok, err := governed.GetOrLoadTargeted(ctx, requestA, &a, loadA); err != nil || !ok || loadsA != 2 {
		t.Fatalf("revoked target reused old L1: ok=%v loads=%d err=%v", ok, loadsA, err)
	}
	if ok, err := governed.GetOrLoadTargeted(ctx, requestB, &b, loadB); err != nil || !ok || loadsB != 1 {
		t.Fatalf("unrelated target was flushed: ok=%v loads=%d err=%v", ok, loadsB, err)
	}
}

func TestGlobalRefreshEpochEvictsTargetedSessionL1WithoutTargetEnumeration(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Codec: "sonic", L1: config.CacheL1Config{Enabled: true, MaxCost: 1 << 20, NumCounters: 1000, BufferItems: 64, DefaultTTL: time.Minute}, Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "dg63-v2", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	mgr, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatal(err)
	}
	targeted := mgr.(TargetedGovernedCache)
	refresh := mgr.(GlobalRefreshGovernedCache)
	base := mgr.(*manager).governed.(*governedLayer)
	targeted.SetTargetFreshnessGate(trustedTargetGate{})
	base.SetFanoutHealthy(true)
	request, _ := cachepolicy.ActiveSessionValidityReadRequest("session-global-refresh")
	loads := 0
	loader := func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loads++
		return cachepolicy.TargetedCacheableValue{Value: map[string]int{"generation": loads}, Cacheable: true, ExpiresAt: time.Now().Add(time.Minute)}, nil
	}
	var value map[string]int
	if found, err := targeted.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found {
		t.Fatalf("warm targeted read found=%v err=%v", found, err)
	}
	if found, err := targeted.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 1 {
		t.Fatalf("targeted L1 did not warm: found=%v loads=%d err=%v", found, loads, err)
	}
	refresh.MarkGlobalRefreshDirty("refresh-v3-targeted")
	if found, err := targeted.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 2 {
		t.Fatalf("pending global refresh reused targeted cache: found=%v loads=%d err=%v", found, loads, err)
	}
	if _, err := refresh.AdvanceGlobalRefresh(context.Background(), "refresh-v3-targeted"); err != nil {
		t.Fatal(err)
	}
	refresh.EvictAllGovernedLocal("refresh-v3-targeted")
	if found, err := targeted.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 3 {
		t.Fatalf("global fanout did not evict targeted L1: found=%v loads=%d err=%v", found, loads, err)
	}
}

func TestTargetedGovernedLayerDirtyOverflowStaysFailClosed(t *testing.T) {
	request, _ := cachepolicy.ActiveSessionValidityReadRequest("session-overflow")
	layer := &targetedGovernedLayer{parent: &governedLayer{}, dirty: make(map[string]map[string]struct{}), unsafe: make(map[string]bool)}
	for index := 0; index <= targetedWriterDirtyEventCap; index++ {
		layer.MarkTargetLocalDirty("event-"+strconv.Itoa(index), request)
	}
	if !layer.overflow || layer.dirtyCount != 0 || len(layer.dirty) != 0 {
		t.Fatalf("overflow did not release IDs fail-closed: overflow=%v count=%d dirty=%d", layer.overflow, layer.dirtyCount, len(layer.dirty))
	}
	layer.EvictTargetLocalAndResolve("event-0", request)
	if !layer.overflow {
		t.Fatal("fanout unexpectedly cleared targeted overflow")
	}
}

func TestTargetedGovernedLayerNilReceiverLoadsAuthorityWithoutCache(t *testing.T) {
	request, _ := cachepolicy.ActiveSessionValidityReadRequest("session-nil")
	var layer *targetedGovernedLayer
	loads := 0
	var result map[string]string
	found, err := layer.GetOrLoadTargeted(context.Background(), request, &result, func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loads++
		return cachepolicy.TargetedCacheableValue{Value: map[string]string{"value": "authority"}, ExpiresAt: time.Now().Add(time.Minute)}, nil
	})
	if err != nil || !found || loads != 1 || result["value"] != "authority" {
		t.Fatalf("nil layer did not use authority: found=%v loads=%d result=%v err=%v", found, loads, result, err)
	}
}

func TestTargetedGovernedLayerDoesNotPersistNegativeOrExpiredAuthorityResult(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Codec: "sonic", L1: config.CacheL1Config{Enabled: true, MaxCost: 1 << 20, NumCounters: 1000, BufferItems: 64, DefaultTTL: time.Minute}, Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "dg62-negative", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	mgr, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatal(err)
	}
	governed := mgr.(TargetedGovernedCache)
	base := mgr.(*manager).governed.(*governedLayer)
	governed.SetTargetFreshnessGate(trustedTargetGate{})
	base.SetFanoutHealthy(true)
	request, _ := cachepolicy.ActiveSessionValidityReadRequest("session-negative")
	loads := 0
	loader := func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loads++
		if loads == 1 {
			return cachepolicy.TargetedCacheableValue{}, nil
		}
		return cachepolicy.TargetedCacheableValue{Value: map[string]string{"value": "expired"}, Cacheable: true, ExpiresAt: time.Now().Add(-time.Second)}, nil
	}
	var value map[string]string
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || found {
		t.Fatalf("negative result found=%v err=%v", found, err)
	}
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 2 {
		t.Fatalf("expired result found=%v loads=%d err=%v", found, loads, err)
	}
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loads++
		return cachepolicy.TargetedCacheableValue{Value: map[string]string{"value": "fresh"}, Cacheable: true, ExpiresAt: time.Now().Add(time.Minute)}, nil
	}); err != nil || !found || loads != 3 {
		t.Fatalf("expired result was retained: found=%v loads=%d err=%v", found, loads, err)
	}
}

func TestTargetedGovernedLayerUntrustedFenceBypassesWarmL1(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Codec: "sonic", L1: config.CacheL1Config{Enabled: true, MaxCost: 1 << 20, NumCounters: 1000, BufferItems: 64, DefaultTTL: time.Minute}, Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "dg62-untrusted", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	mgr, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatal(err)
	}
	governed := mgr.(TargetedGovernedCache)
	base := mgr.(*manager).governed.(*governedLayer)
	request, _ := cachepolicy.ActiveSessionValidityReadRequest("session-untrusted")
	loads := 0
	loader := func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loads++
		return cachepolicy.TargetedCacheableValue{Value: map[string]int{"generation": loads}, Cacheable: true, ExpiresAt: time.Now().Add(time.Minute)}, nil
	}
	governed.SetTargetFreshnessGate(trustedTargetGate{})
	base.SetFanoutHealthy(true)
	var value map[string]int
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 1 {
		t.Fatalf("warm targeted read found=%v loads=%d err=%v", found, loads, err)
	}
	// A lost instance fanout health signal must never reuse the warm L1 value.
	// The only permitted response is an authority load.
	base.SetFanoutHealthy(false)
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 2 || value["generation"] != 2 {
		t.Fatalf("unhealthy fanout reused warm L1: found=%v loads=%d value=%v err=%v", found, loads, value, err)
	}
	base.SetFanoutHealthy(true)
	governed.SetTargetFreshnessGate(untrustedTargetGate{})
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 3 || value["generation"] != 3 {
		t.Fatalf("untrusted target fence reused warm L1: found=%v loads=%d value=%v err=%v", found, loads, value, err)
	}
}

func TestTargetedGovernedLayerUnavailableRedisBypassesWarmL1(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Codec: "sonic", L1: config.CacheL1Config{Enabled: true, MaxCost: 1 << 20, NumCounters: 1000, BufferItems: 64, DefaultTTL: time.Minute}, Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "dg62-redis-unavailable", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	mgr, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatal(err)
	}
	governed := mgr.(TargetedGovernedCache)
	base := mgr.(*manager).governed.(*governedLayer)
	governed.SetTargetFreshnessGate(trustedTargetGate{})
	base.SetFanoutHealthy(true)
	request, _ := cachepolicy.ActiveSessionValidityReadRequest("session-redis-unavailable")
	loads := 0
	loader := func(context.Context) (cachepolicy.TargetedCacheableValue, error) {
		loads++
		return cachepolicy.TargetedCacheableValue{Value: map[string]int{"generation": loads}, Cacheable: true, ExpiresAt: time.Now().Add(time.Minute)}, nil
	}
	var value map[string]int
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 1 {
		t.Fatalf("warm targeted read found=%v loads=%d err=%v", found, loads, err)
	}
	mini.Close()
	if found, err := governed.GetOrLoadTargeted(context.Background(), request, &value, loader); err != nil || !found || loads != 2 || value["generation"] != 2 {
		t.Fatalf("unavailable Redis reused warm L1: found=%v loads=%d value=%v err=%v", found, loads, value, err)
	}
}
