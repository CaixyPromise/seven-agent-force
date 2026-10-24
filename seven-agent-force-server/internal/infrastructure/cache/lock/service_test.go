package lock

import (
	"context"
	"testing"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestDistributedLockAndReplayProtection(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{
		Enabled: true,
		Codec:   "sonic",
		Redis: config.RedisCacheConfig{
			Enabled: true,
			Mode:    config.RedisCacheModeSingle,
			Single: config.RedisSingleConfig{
				Addr: mini.Addr(),
			},
		},
	}
	service := NewService(cacheinfra.NewProvider(cfg))
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
		t.Fatalf("try lock second time: %v", err)
	}
	if ok {
		t.Fatal("expected second lock attempt to fail")
	}

	refreshed, err := service.Refresh(ctx, "seven:lock:test", token, 2*time.Minute)
	if err != nil {
		t.Fatalf("refresh lock: %v", err)
	}
	if !refreshed {
		t.Fatal("expected refresh to succeed")
	}

	unlocked, err := service.Unlock(ctx, "seven:lock:test", token)
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if !unlocked {
		t.Fatal("expected unlock to succeed")
	}

	first, err := service.CheckAndSetNonce(ctx, "seven:nonce:test", time.Minute)
	if err != nil {
		t.Fatalf("check nonce first time: %v", err)
	}
	second, err := service.CheckAndSetNonce(ctx, "seven:nonce:test", time.Minute)
	if err != nil {
		t.Fatalf("check nonce second time: %v", err)
	}
	if !first || second {
		t.Fatalf("unexpected nonce results: first=%v second=%v", first, second)
	}
}
