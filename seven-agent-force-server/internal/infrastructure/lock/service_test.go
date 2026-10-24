package lock

import (
	"context"
	"testing"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestRedisServiceLockAndReplayProtection(t *testing.T) {
	mini := miniredis.RunT(t)
	provider := cacheinfra.NewProvider(config.CacheConfig{
		Enabled: true,
		Codec:   "sonic",
		Redis: config.RedisCacheConfig{
			Enabled: true,
			Mode:    config.RedisCacheModeSingle,
			Single:  config.RedisSingleConfig{Addr: mini.Addr()},
		},
	})
	service := NewRedisService(provider)
	ctx := context.Background()

	token, ok, err := service.TryLock(ctx, "seven:lock:test", time.Minute)
	if err != nil {
		t.Fatalf("try lock: %v", err)
	}
	if !ok || token == "" {
		t.Fatalf("unexpected lock result: ok=%v token=%s", ok, token)
	}

	_, ok, err = service.TryLock(ctx, "seven:lock:test", time.Minute)
	if err != nil {
		t.Fatalf("try second lock: %v", err)
	}
	if ok {
		t.Fatal("expected second lock to be rejected")
	}

	if refreshed, err := service.Refresh(ctx, "seven:lock:test", token, 2*time.Minute); err != nil || !refreshed {
		t.Fatalf("refresh lock: refreshed=%v err=%v", refreshed, err)
	}
	if unlocked, err := service.Unlock(ctx, "seven:lock:test", token); err != nil || !unlocked {
		t.Fatalf("unlock lock: unlocked=%v err=%v", unlocked, err)
	}

	first, err := service.CheckAndSetNonce(ctx, "seven:nonce:test", time.Minute)
	if err != nil {
		t.Fatalf("set nonce first: %v", err)
	}
	second, err := service.CheckAndSetNonce(ctx, "seven:nonce:test", time.Minute)
	if err != nil {
		t.Fatalf("set nonce second: %v", err)
	}
	if !first || second {
		t.Fatalf("unexpected nonce results: first=%v second=%v", first, second)
	}
}
