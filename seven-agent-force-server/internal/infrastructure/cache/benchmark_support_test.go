package cache

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

const (
	benchmarkRedisAddrEnv = "SEVEN_CACHE_BENCH_REDIS_ADDR"
	benchmarkRedisDBEnv   = "SEVEN_CACHE_BENCH_REDIS_DB"
)

func newBenchmarkManager(b *testing.B) (Manager, func()) {
	b.Helper()

	cfg := config.CacheConfig{
		Enabled: true,
		Prefix:  "seven-bench",
		Codec:   "sonic",
		L1: config.CacheL1Config{
			Enabled:     true,
			MaxCost:     1024 * 1024,
			NumCounters: 10000,
			BufferItems: 64,
			DefaultTTL:  30 * time.Second,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven-bench",
			Database:  0,
		},
	}

	var cleanup func()
	if addr := os.Getenv(benchmarkRedisAddrEnv); addr != "" {
		cfg.Redis.Single.Addr = addr
		cfg.Redis.Database = benchmarkRedisDB()
		cleanup = func() {}
	} else {
		mini := miniredis.RunT(b)
		cfg.Redis.Single.Addr = mini.Addr()
		cleanup = mini.Close
	}

	provider := NewProvider(cfg)
	if provider.Configured() {
		if err := provider.Client().FlushDB(context.Background()).Err(); err != nil {
			b.Fatalf("flush benchmark redis db before run: %v", err)
		}
	}
	manager, err := NewDefaultManager(cfg, provider)
	if err != nil {
		_ = provider.Close()
		cleanup()
		b.Fatalf("new default manager: %v", err)
	}

	return manager, func() {
		if provider.Configured() {
			_ = provider.Client().FlushDB(context.Background()).Err()
		}
		_ = provider.Close()
		cleanup()
	}
}

func benchmarkRedisDB() int {
	value := os.Getenv(benchmarkRedisDBEnv)
	if value == "" {
		return 7
	}
	db, err := strconv.Atoi(value)
	if err != nil || db < 0 {
		return 7
	}
	return db
}
