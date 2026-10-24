package lock

import (
	"context"
	"strconv"
	"testing"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func BenchmarkDistributedLockTryLockUnlock(b *testing.B) {
	mini := miniredis.RunT(b)
	cfg := config.CacheConfig{
		Enabled: true,
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
	keys := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		keys[i] = "seven:lock:bench:" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		token, ok, err := service.TryLock(ctx, keys[i], time.Minute)
		if err != nil {
			b.Fatalf("try lock: %v", err)
		}
		if !ok {
			b.Fatal("expected lock success")
		}
		unlocked, err := service.Unlock(ctx, keys[i], token)
		if err != nil {
			b.Fatalf("unlock: %v", err)
		}
		if !unlocked {
			b.Fatal("expected unlock success")
		}
	}
}
