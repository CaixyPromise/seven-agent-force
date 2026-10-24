package cache

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/key"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/l1"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/bytedance/sonic"
)

func TestGovernedLayerRequiresHealthyFanoutAndUsesGenerationToEvict(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{
		Enabled: true,
		Prefix:  "seven",
		Codec:   "sonic",
		L1: config.CacheL1Config{
			Enabled:     true,
			MaxCost:     1024 * 1024,
			NumCounters: 1000,
			BufferItems: 64,
			DefaultTTL:  time.Minute,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven",
			Single:    config.RedisSingleConfig{Addr: mini.Addr()},
		},
	}
	cacheManager, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatalf("new cache manager: %v", err)
	}
	governed, ok := cacheManager.(GovernedCache)
	if !ok {
		t.Fatal("default manager must expose the classified DG5 cache layer")
	}
	governed.SetFreshnessGate(trustedCacheFreshnessGate{})

	request, ok := cachepolicy.ConfigReadRequest("SEVEN_FRONTEND_METADATA.title", "org:1", "anonymous")
	if !ok {
		t.Fatal("catalogued config read was rejected")
	}
	ctx := context.Background()
	loads := 0
	load := func(value string) ClassifiedLoader {
		return func(context.Context) (cachepolicy.CacheableValue, error) {
			loads++
			return cachepolicy.CacheableValue{Value: map[string]string{"value": value}, Cacheable: true}, nil
		}
	}

	// RabbitMQ fanout starts untrusted, so L1/L2 cannot be used and the source
	// of truth remains authoritative.
	var first map[string]string
	found, err := governed.GetOrLoadClassified(ctx, request, &first, load("one"))
	if err != nil || !found || first["value"] != "one" || loads != 1 {
		t.Fatalf("untrusted read=%v value=%v loads=%d err=%v", found, first, loads, err)
	}
	var second map[string]string
	found, err = governed.GetOrLoadClassified(ctx, request, &second, load("two"))
	if err != nil || !found || second["value"] != "two" || loads != 2 {
		t.Fatalf("untrusted second read reused cache: found=%v value=%v loads=%d err=%v", found, second, loads, err)
	}

	governed.SetFanoutHealthy(true)
	var cached map[string]string
	found, err = governed.GetOrLoadClassified(ctx, request, &cached, load("three"))
	if err != nil || !found || cached["value"] != "three" || loads != 3 {
		t.Fatalf("initial trusted read=%v value=%v loads=%d err=%v", found, cached, loads, err)
	}
	var l1Hit map[string]string
	found, err = governed.GetOrLoadClassified(ctx, request, &l1Hit, load("four"))
	if err != nil || !found || l1Hit["value"] != "three" || loads != 3 {
		t.Fatalf("L1 did not preserve generation-safe value: found=%v value=%v loads=%d err=%v", found, l1Hit, loads, err)
	}

	governed.MarkLocalDirty("event-1", request.Entry.DataClass)
	var writerRead map[string]string
	found, err = governed.GetOrLoadClassified(ctx, request, &writerRead, load("five"))
	if err != nil || !found || writerRead["value"] != "five" || loads != 4 {
		t.Fatalf("writer read-your-write used stale cache: found=%v value=%v loads=%d err=%v", found, writerRead, loads, err)
	}
	if _, err := governed.AdvanceGeneration(ctx, "event-1", request.Entry.DataClass); err != nil {
		t.Fatalf("advance generation: %v", err)
	}
	governed.EvictLocalAndResolve("event-1", request.Entry.DataClass)
	var afterFanout map[string]string
	found, err = governed.GetOrLoadClassified(ctx, request, &afterFanout, load("six"))
	if err != nil || !found || afterFanout["value"] != "six" || loads != 5 {
		t.Fatalf("generation eviction did not require a fresh source read: found=%v value=%v loads=%d err=%v", found, afterFanout, loads, err)
	}

	// A missing generation must establish a new random epoch rather than
	// returning to generation=1, otherwise a surviving old L2 payload could be
	// accepted after an epoch loss.
	layer := cacheManager.(*manager).governed.(*governedLayer)
	mini.Del(layer.generationKey(request.Entry.DataClass))
	var afterEpochLoss map[string]string
	found, err = governed.GetOrLoadClassified(ctx, request, &afterEpochLoss, load("seven"))
	if err != nil || !found || afterEpochLoss["value"] != "seven" || loads != 6 {
		t.Fatalf("generation epoch recovery reused an old L2 payload: found=%v value=%v loads=%d err=%v", found, afterEpochLoss, loads, err)
	}

	for _, key := range mini.Keys() {
		if strings.Contains(strings.ToLower(key), "themeprimarycolor") || strings.Contains(key, "org:1") {
			t.Fatalf("DG5 Redis key leaked raw cache material: %q", key)
		}
	}
}

func TestGlobalRefreshEpochEvictsAllGovernedV1Candidates(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Prefix: "seven", Codec: "sonic", L1: config.CacheL1Config{Enabled: true, MaxCost: 1024 * 1024, NumCounters: 1000, BufferItems: 64, DefaultTTL: time.Minute}, Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "seven", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	manager, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	governed := manager.(GovernedCache)
	refresh, ok := manager.(GlobalRefreshGovernedCache)
	if !ok {
		t.Fatal("default manager must expose global refresh governance")
	}
	governed.SetFreshnessGate(trustedCacheFreshnessGate{})
	governed.SetFanoutHealthy(true)
	request, ok := cachepolicy.ConfigReadRequest("SEVEN_FRONTEND_METADATA.title", "org:1", "anonymous")
	if !ok {
		t.Fatal("catalog request rejected")
	}
	loads := 0
	load := func(value string) ClassifiedLoader {
		return func(context.Context) (cachepolicy.CacheableValue, error) {
			loads++
			return cachepolicy.CacheableValue{Value: map[string]string{"value": value}, Cacheable: true}, nil
		}
	}
	var first, warm map[string]string
	if found, err := governed.GetOrLoadClassified(context.Background(), request, &first, load("before")); err != nil || !found {
		t.Fatalf("first governed read found=%v err=%v", found, err)
	}
	if found, err := governed.GetOrLoadClassified(context.Background(), request, &warm, load("unexpected")); err != nil || !found || warm["value"] != "before" || loads != 1 {
		t.Fatalf("warm candidate did not hit L1: found=%v value=%v loads=%d err=%v", found, warm, loads, err)
	}
	refresh.MarkGlobalRefreshDirty("refresh-1")
	var pending map[string]string
	if found, err := governed.GetOrLoadClassified(context.Background(), request, &pending, load("pending-source")); err != nil || !found || pending["value"] != "pending-source" || loads != 2 {
		t.Fatalf("pending global refresh returned stale candidate: found=%v value=%v loads=%d err=%v", found, pending, loads, err)
	}
	if _, err := refresh.AdvanceGlobalRefresh(context.Background(), "refresh-1"); err != nil {
		t.Fatalf("advance global refresh epoch: %v", err)
	}
	refresh.EvictAllGovernedLocal("refresh-1")
	var after map[string]string
	if found, err := governed.GetOrLoadClassified(context.Background(), request, &after, load("after")); err != nil || !found || after["value"] != "after" || loads != 3 {
		t.Fatalf("global refresh fanout did not evict governed L1: found=%v value=%v loads=%d err=%v", found, after, loads, err)
	}
}

func TestGovernedLayerRejectsL1DecodeWhenFanoutTurnsUnhealthy(t *testing.T) {
	layer, request, codec := newBlockingGovernedLayer(t)
	ctx := context.Background()
	var loads atomic.Int32
	load := func(value string) ClassifiedLoader {
		return func(context.Context) (cachepolicy.CacheableValue, error) {
			loads.Add(1)
			return cachepolicy.CacheableValue{Value: map[string]string{"value": value}, Cacheable: true}, nil
		}
	}

	var primed map[string]string
	if found, err := layer.GetOrLoadClassified(ctx, request, &primed, load("one")); err != nil || !found || primed["value"] != "one" {
		t.Fatalf("prime governed L1: found=%v value=%v err=%v", found, primed, err)
	}

	codec.arm()
	result := make(chan governedReadResult, 1)
	go func() {
		var value map[string]string
		found, err := layer.GetOrLoadClassified(ctx, request, &value, load("two"))
		result <- governedReadResult{found: found, value: value, err: err}
	}()
	<-codec.entered

	transitioned := make(chan struct{})
	go func() {
		layer.SetFanoutHealthy(false)
		close(transitioned)
	}()
	select {
	case <-transitioned:
		// The health transition must not wait for a slow Sonic decode.
	case <-time.After(time.Second):
		t.Fatal("fanout fail-closed transition was blocked by L1 decode")
	}
	codec.release()
	got := <-result
	if got.err != nil || !got.found || got.value["value"] != "two" || loads.Load() != 2 {
		t.Fatalf("stale L1 escaped after fanout outage: found=%v value=%v loads=%d err=%v", got.found, got.value, loads.Load(), got.err)
	}
}

func TestGovernedLayerRejectsL1DecodeWhenGenerationAdvances(t *testing.T) {
	layer, request, codec := newBlockingGovernedLayer(t)
	ctx := context.Background()
	var loads atomic.Int32
	load := func(value string) ClassifiedLoader {
		return func(context.Context) (cachepolicy.CacheableValue, error) {
			loads.Add(1)
			return cachepolicy.CacheableValue{Value: map[string]string{"value": value}, Cacheable: true}, nil
		}
	}

	var primed map[string]string
	if found, err := layer.GetOrLoadClassified(ctx, request, &primed, load("one")); err != nil || !found || primed["value"] != "one" {
		t.Fatalf("prime governed L1: found=%v value=%v err=%v", found, primed, err)
	}

	codec.arm()
	result := make(chan governedReadResult, 1)
	go func() {
		var value map[string]string
		found, err := layer.GetOrLoadClassified(ctx, request, &value, load("two"))
		result <- governedReadResult{found: found, value: value, err: err}
	}()
	<-codec.entered

	advanced := make(chan error, 1)
	go func() {
		_, err := layer.AdvanceGeneration(ctx, "fanout-event-1", request.Entry.DataClass)
		advanced <- err
	}()
	select {
	case err := <-advanced:
		if err != nil {
			t.Fatalf("advance generation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("generation advance was blocked by L1 decode")
	}
	codec.release()
	got := <-result
	if got.err != nil || !got.found || got.value["value"] != "two" || loads.Load() != 2 {
		t.Fatalf("stale L1 escaped after generation advance: found=%v value=%v loads=%d err=%v", got.found, got.value, loads.Load(), got.err)
	}
}

func TestGovernedLayerKeepsClassUntrustedUntilEveryGenerationAdvanceCompletes(t *testing.T) {
	layer, request, _ := newBlockingGovernedLayer(t)
	class := request.Entry.DataClass
	layer.beginGenerationAdvance(class)
	layer.beginGenerationAdvance(class)
	if layer.trustedForRead(class) {
		t.Fatal("class became trusted while two generation advances were in flight")
	}
	layer.completeGenerationAdvance(class)
	if layer.trustedForRead(class) {
		t.Fatal("class became trusted after only one of two generation advances completed")
	}
	layer.completeGenerationAdvance(class)
	if !layer.trustedForRead(class) {
		t.Fatal("class remained untrusted after every generation advance completed")
	}
}

func TestGovernedLayerBoundsWriterDirtyTrackingFailClosed(t *testing.T) {
	layer, request, _ := newBlockingGovernedLayer(t)
	class := request.Entry.DataClass
	for index := 0; index < 101; index++ {
		layer.MarkLocalDirty("writer-event-"+strconv.Itoa(index), class)
	}
	if tracked := len(layer.dirty[class]); tracked > 100 {
		t.Fatalf("writer dirty tracking grew past the bounded relay window: tracked=%d", tracked)
	}
	for index := 0; index < 101; index++ {
		layer.EvictLocalAndResolve("writer-event-"+strconv.Itoa(index), class)
	}
	if layer.trustedForRead(class) {
		t.Fatal("writer dirty overflow recovered to a trusted cache without a restart-safe recovery barrier")
	}
}

func TestGovernedLayerRecoversOnlyAfterSuccessfulRetryFollowingGenerationFailure(t *testing.T) {
	layer, request, _ := newBlockingGovernedLayer(t)
	class := request.Entry.DataClass
	var warmed map[string]string
	if found, err := layer.GetOrLoadClassified(context.Background(), request, &warmed, func(context.Context) (cachepolicy.CacheableValue, error) {
		return cachepolicy.CacheableValue{Value: map[string]string{"value": "warm"}, Cacheable: true}, nil
	}); err != nil || !found || warmed["value"] != "warm" {
		t.Fatalf("establish trusted freshness read: found=%v value=%v err=%v", found, warmed, err)
	}
	if _, err := layer.AdvanceGeneration(context.Background(), "primed-generation-event", class); err != nil {
		t.Fatalf("prime generation advance: %v", err)
	}
	if status := layer.GovernedStatus(); !status.ReadTrusted {
		t.Fatalf("primed governed status was not trusted: %+v", status)
	}
	failedCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := layer.AdvanceGeneration(failedCtx, "failed-generation-event", class); err == nil {
		t.Fatal("cancelled generation advance unexpectedly succeeded")
	}
	if layer.trustedForRead(class) {
		t.Fatal("generation failure made the class trusted before durable retry")
	}
	if status := layer.GovernedStatus(); status.ReadTrusted || status.UnsafeClasses != 1 || status.TransitioningClasses != 0 {
		t.Fatalf("health overstated failed generation freshness: %+v", status)
	}
	if _, err := layer.AdvanceGeneration(context.Background(), "recovered-generation-event", class); err != nil {
		t.Fatalf("successful retry: %v", err)
	}
	if !layer.trustedForRead(class) {
		t.Fatal("successful retry did not clear the failed generation state")
	}
	if status := layer.GovernedStatus(); !status.ReadTrusted || status.UnsafeClasses != 0 || status.TransitioningClasses != 0 {
		t.Fatalf("health did not recover after generation retry: %+v", status)
	}
}

func TestGovernedLayerBypassesCandidatesWhenGlobalFreshnessCannotBeConfirmed(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{
		Enabled: true,
		Codec:   "sonic",
		L1:      config.CacheL1Config{Enabled: true, MaxCost: 1024 * 1024, NumCounters: 1000, BufferItems: 64, DefaultTTL: time.Minute},
		Redis:   config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "seven", Single: config.RedisSingleConfig{Addr: mini.Addr()}},
	}
	manager, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatalf("new governed manager: %v", err)
	}
	governed := manager.(GovernedCache)
	governed.SetFanoutHealthy(true)
	governed.SetFreshnessGate(trustedCacheFreshnessGate{})
	request, ok := cachepolicy.ConfigReadRequest("SEVEN_FRONTEND_METADATA.title", "org:1", "anonymous")
	if !ok {
		t.Fatal("catalogued request rejected")
	}
	loads := 0
	load := func(value string) ClassifiedLoader {
		return func(context.Context) (cachepolicy.CacheableValue, error) {
			loads++
			return cachepolicy.CacheableValue{Value: map[string]string{"value": value}, Cacheable: true}, nil
		}
	}
	var warm map[string]string
	if found, err := governed.GetOrLoadClassified(context.Background(), request, &warm, load("v1")); err != nil || !found || warm["value"] != "v1" {
		t.Fatalf("warm cache: found=%v value=%v err=%v", found, warm, err)
	}
	governed.SetFreshnessGate(unavailableCacheFreshnessGate{})
	var fresh map[string]string
	if found, err := governed.GetOrLoadClassified(context.Background(), request, &fresh, load("v2")); err != nil || !found || fresh["value"] != "v2" || loads != 2 {
		t.Fatalf("unconfirmed freshness reused a cache candidate: found=%v value=%v loads=%d err=%v", found, fresh, loads, err)
	}
	status := governed.GovernedStatus()
	if status.FreshnessHealthy || status.ReadTrusted {
		t.Fatalf("unconfirmed global freshness overstated cache health: %+v", status)
	}
}

func TestGovernedLayerPreflightBypassesWarmCandidateBeforeL1OrL2(t *testing.T) {
	layer, request, _ := newBlockingGovernedLayer(t)
	ctx := context.Background()
	loads := 0
	load := func(value string) ClassifiedLoader {
		return func(context.Context) (cachepolicy.CacheableValue, error) {
			loads++
			return cachepolicy.CacheableValue{Value: map[string]string{"value": value}, Cacheable: true}, nil
		}
	}
	var warm map[string]string
	if found, err := layer.GetOrLoadClassified(ctx, request, &warm, load("v1")); err != nil || !found || warm["value"] != "v1" {
		t.Fatalf("warm classified candidate: found=%v value=%v err=%v", found, warm, err)
	}
	preflights := 0
	var fresh map[string]string
	found, err := layer.GetOrLoadClassifiedWithPreflight(ctx, request, &fresh, func(context.Context) (bool, error) {
		preflights++
		return false, nil
	}, load("v2"))
	if err != nil || !found || fresh["value"] != "v2" || loads != 2 || preflights != 1 {
		t.Fatalf("failed source preflight reused an L1/L2 candidate: found=%v value=%v loads=%d preflights=%d err=%v", found, fresh, loads, preflights, err)
	}
}

type governedReadResult struct {
	found bool
	value map[string]string
	err   error
}

type blockingSonicCodec struct {
	mu        sync.Mutex
	armed     bool
	entered   chan struct{}
	releaseCh chan struct{}
}

func (c *blockingSonicCodec) Name() string { return "sonic" }

func (c *blockingSonicCodec) Marshal(value any) ([]byte, error) {
	return sonic.Marshal(value)
}

func (c *blockingSonicCodec) Unmarshal(payload []byte, dest any) error {
	c.mu.Lock()
	armed := c.armed
	if armed {
		c.armed = false
		close(c.entered)
	}
	release := c.releaseCh
	c.mu.Unlock()
	if armed {
		<-release
	}
	return sonic.Unmarshal(payload, dest)
}

func (c *blockingSonicCodec) arm() {
	c.mu.Lock()
	c.armed = true
	c.entered = make(chan struct{})
	c.releaseCh = make(chan struct{})
	c.mu.Unlock()
}

func (c *blockingSonicCodec) release() {
	c.mu.Lock()
	release := c.releaseCh
	c.mu.Unlock()
	close(release)
}

func newBlockingGovernedLayer(t *testing.T) (*governedLayer, cachepolicy.ReadRequest, *blockingSonicCodec) {
	t.Helper()
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{
		Enabled: true,
		Prefix:  "seven",
		L1: config.CacheL1Config{
			Enabled:     true,
			MaxCost:     1024 * 1024,
			NumCounters: 1000,
			BufferItems: 64,
			DefaultTTL:  time.Minute,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven",
			Single:    config.RedisSingleConfig{Addr: mini.Addr()},
		},
	}
	provider := NewProvider(cfg)
	t.Cleanup(func() { _ = provider.Close() })
	store, err := l1.NewStore(cfg)
	if err != nil {
		t.Fatalf("new L1 store: %v", err)
	}
	t.Cleanup(store.Close)
	codec := &blockingSonicCodec{}
	layer := NewGovernedLayer(key.NewBuilder(cfg.Redis.KeyPrefix), store, provider, codec)
	layer.SetFanoutHealthy(true)
	layer.SetFreshnessGate(trustedCacheFreshnessGate{})
	request, ok := cachepolicy.ConfigReadRequest("SEVEN_FRONTEND_METADATA.title", "org:1", "anonymous")
	if !ok {
		t.Fatal("catalogued config read was rejected")
	}
	return layer, request, codec
}

type trustedCacheFreshnessGate struct{}

func (trustedCacheFreshnessGate) AcquireRead(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return trustedCacheFreshnessLease{}, nil
}

func (trustedCacheFreshnessGate) AcquireMutation(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return trustedCacheFreshnessLease{}, nil
}

type unavailableCacheFreshnessGate struct{}

func (unavailableCacheFreshnessGate) AcquireRead(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return nil, errors.New("global freshness unavailable")
}

func (unavailableCacheFreshnessGate) AcquireMutation(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return nil, errors.New("global freshness unavailable")
}

type trustedCacheFreshnessLease struct{}

func (trustedCacheFreshnessLease) Trusted() bool { return true }
func (trustedCacheFreshnessLease) Release()      {}
