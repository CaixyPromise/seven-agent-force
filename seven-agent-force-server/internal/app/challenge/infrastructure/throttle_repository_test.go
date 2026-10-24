package infrastructure

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestThrottleRepositoryRecordsLocksClearsAndHashesKeys(t *testing.T) {
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{
		Enabled: true,
		Prefix:  "seven-test",
		Codec:   "sonic",
		L1: config.CacheL1Config{
			Enabled: false,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven-test",
			Single: config.RedisSingleConfig{
				Addr: mini.Addr(),
			},
		},
	}
	manager, err := cacheinfra.NewDefaultManager(cfg, cacheinfra.NewProvider(cfg))
	if err != nil {
		t.Fatalf("new cache manager: %v", err)
	}
	repo := NewThrottleRepository(manager)
	ctx := context.Background()
	keys := []domain.ChallengeThrottleKey{{
		Dimension: "email-target-action-factor",
		Value:     "alice@example.com|CONFIG_SENSITIVE_REVEAL|EMAIL_ONE_TIME_PASSWORD",
	}}

	if decision, err := repo.RecordFailure(ctx, keys, 2, time.Minute, time.Hour); err != nil || decision != nil {
		t.Fatalf("first failure should not lock: decision=%+v err=%v", decision, err)
	}
	decision, err := repo.RecordFailure(ctx, keys, 2, time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("second failure: %v", err)
	}
	if decision == nil || !decision.Locked || decision.Dimension != "email-target-action-factor" {
		t.Fatalf("expected locked decision, got %+v", decision)
	}
	locked, err := repo.CheckLocked(ctx, keys)
	if err != nil {
		t.Fatalf("check locked: %v", err)
	}
	if locked == nil || !locked.Locked {
		t.Fatalf("expected lock to be visible, got %+v", locked)
	}
	for _, key := range mini.Keys() {
		if strings.Contains(key, "alice@example.com") {
			t.Fatalf("throttle key leaked raw email: %s", key)
		}
	}
	if err := repo.ClearFailures(ctx, keys); err != nil {
		t.Fatalf("clear failures: %v", err)
	}
	locked, err = repo.CheckLocked(ctx, keys)
	if err != nil {
		t.Fatalf("check cleared lock: %v", err)
	}
	if locked != nil {
		t.Fatalf("expected lock to be cleared, got %+v", locked)
	}
}
