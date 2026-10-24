package cache

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestDefaultManagerSupportsRedisKVAndTwoLevelCache(t *testing.T) {
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
			DefaultTTL:  30 * time.Second,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven",
			Single: config.RedisSingleConfig{
				Addr: mini.Addr(),
			},
		},
	}

	provider := NewProvider(cfg)
	manager, err := NewDefaultManager(cfg, provider)
	if err != nil {
		t.Fatalf("new default manager: %v", err)
	}

	type payload struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	ctx := context.Background()
	cacheKey := manager.Builder().Build("sso", "client", 1)

	if err := manager.Set(ctx, cacheKey, payload{ID: 12, Name: "alpha"}, time.Minute); err != nil {
		t.Fatalf("set cache value: %v", err)
	}
	var loaded payload
	hit, err := manager.Get(ctx, cacheKey, &loaded)
	if err != nil {
		t.Fatalf("get cache value: %v", err)
	}
	if !hit || loaded.ID != 12 || loaded.Name != "alpha" {
		t.Fatalf("unexpected get result: hit=%v value=%+v", hit, loaded)
	}

	loaderCalls := 0
	var twoLevel payload
	hit, err = manager.GetOrLoad(ctx, manager.Builder().Build("dict", "config", "hot"), &twoLevel, time.Minute, func(context.Context) (any, error) {
		loaderCalls++
		return payload{ID: 18, Name: "cached"}, nil
	})
	if err != nil {
		t.Fatalf("get or load cache value: %v", err)
	}
	if !hit || twoLevel.Name != "cached" {
		t.Fatalf("unexpected get or load result: hit=%v value=%+v", hit, twoLevel)
	}

	var second payload
	hit, err = manager.GetOrLoad(ctx, manager.Builder().Build("dict", "config", "hot"), &second, time.Minute, func(context.Context) (any, error) {
		loaderCalls++
		return payload{ID: 19, Name: "miss"}, nil
	})
	if err != nil {
		t.Fatalf("get or load cached value: %v", err)
	}
	if !hit || second.Name != "cached" {
		t.Fatalf("unexpected cached result: hit=%v value=%+v", hit, second)
	}
	if loaderCalls != 1 {
		t.Fatalf("expected loader called once, got %d", loaderCalls)
	}
}

func TestDefaultManagerSupportsHashAndCompareDelete(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{
		Enabled: true,
		Codec:   "sonic",
		L1: config.CacheL1Config{
			Enabled: false,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven",
			Single: config.RedisSingleConfig{
				Addr: mini.Addr(),
			},
		},
	}

	manager, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatalf("new default manager: %v", err)
	}

	ctx := context.Background()
	hashKey := "seven:test:hash"
	if err := manager.HSet(ctx, hashKey, map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatalf("hset: %v", err)
	}
	values, err := manager.HGetAllDel(ctx, hashKey)
	if err != nil {
		t.Fatalf("hgetalldel: %v", err)
	}
	if values["a"] != "1" || values["b"] != "2" {
		t.Fatalf("unexpected hash values: %+v", values)
	}

	compareKey := "seven:test:compare"
	if err := manager.Set(ctx, compareKey, map[string]any{"value": "x"}, time.Minute); err != nil {
		t.Fatalf("set compare key: %v", err)
	}
	deleted, err := manager.CompareAndDelete(ctx, compareKey, map[string]any{"value": "x"})
	if err != nil {
		t.Fatalf("compare and delete: %v", err)
	}
	if !deleted {
		t.Fatal("expected compare delete to succeed")
	}
}

func TestDefaultManagerHealthExposesSafeDG5FreshnessState(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{
		Enabled: true,
		Codec:   "sonic",
		Governance: config.CacheGovernanceConfig{
			Enabled: true,
		},
		L1: config.CacheL1Config{
			Enabled:     true,
			MaxCost:     1024 * 1024,
			NumCounters: 1000,
			BufferItems: 64,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven",
			Single:    config.RedisSingleConfig{Addr: mini.Addr()},
		},
	}
	manager, err := NewDefaultManager(cfg, NewProvider(cfg))
	if err != nil {
		t.Fatalf("new default manager: %v", err)
	}
	governed, ok := manager.(GovernedCache)
	if !ok {
		t.Fatal("default manager lacks governed cache surface")
	}
	governed.SetFanoutHealthy(true)
	health := manager.Health(context.Background())
	if !health.Governance.Enabled || !health.Governance.FanoutHealthy || health.Governance.ReadTrusted {
		t.Fatalf("expected health to show governed cache warming but not Redis-trusted yet: %+v", health.Governance)
	}
}
