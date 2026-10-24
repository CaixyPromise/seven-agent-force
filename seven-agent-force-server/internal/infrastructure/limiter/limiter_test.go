package limiter

import (
	"context"
	"errors"
	"testing"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestFixedWindowLimiter(t *testing.T) {
	mini := miniredis.RunT(t)
	cacheCfg := config.CacheConfig{
		Enabled: true,
		Codec:   "sonic",
		Redis: config.RedisCacheConfig{
			Enabled: true,
			Mode:    config.RedisCacheModeSingle,
			Single:  config.RedisSingleConfig{Addr: mini.Addr()},
		},
	}
	provider := cacheinfra.NewProvider(cacheCfg)
	mgr, err := cacheinfra.NewDefaultManager(cacheCfg, provider)
	if err != nil {
		t.Fatalf("build cache manager: %v", err)
	}
	service := New(config.LimiterConfig{
		Enabled:       true,
		KeyPrefix:     "test:limit",
		DefaultLimit:  2,
		DefaultWindow: time.Minute,
	}, mgr)
	ctx := context.Background()

	first, err := service.AllowDefault(ctx, "user:1")
	if err != nil || !first.Allowed || first.Remaining != 1 {
		t.Fatalf("first allow: decision=%+v err=%v", first, err)
	}
	second, err := service.AllowDefault(ctx, "user:1")
	if err != nil || !second.Allowed || second.Remaining != 0 {
		t.Fatalf("second allow: decision=%+v err=%v", second, err)
	}
	third, err := service.AllowDefault(ctx, "user:1")
	if !errors.Is(err, ErrRateLimited) || third.Allowed {
		t.Fatalf("third allow should rate limit: decision=%+v err=%v", third, err)
	}
}

func TestLimiterFailOpenWhenCacheUnavailable(t *testing.T) {
	service := New(config.LimiterConfig{
		Enabled:       true,
		DefaultLimit:  1,
		DefaultWindow: time.Minute,
		FailOpen:      true,
	}, nil)

	decision, err := service.AllowDefault(context.Background(), "user:1")
	if err != nil {
		t.Fatalf("expected fail-open without error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected fail-open decision to allow: %+v", decision)
	}
}

func TestLimiterAllowWithFailOpenOverrideCanFailClosed(t *testing.T) {
	service := New(config.LimiterConfig{
		Enabled:       true,
		DefaultLimit:  1,
		DefaultWindow: time.Minute,
		FailOpen:      true,
	}, nil)

	decision, err := service.AllowWithFailOpen(context.Background(), "user:1", 1, time.Minute, false)
	if !errors.Is(err, cacheinfra.ErrCacheLayerUnsupported) {
		t.Fatalf("expected fail-closed cache error, decision=%+v err=%v", decision, err)
	}
	if decision.Allowed {
		t.Fatalf("expected fail-closed decision to deny: %+v", decision)
	}
}
