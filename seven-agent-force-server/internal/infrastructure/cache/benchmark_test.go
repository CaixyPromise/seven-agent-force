package cache

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/key"
)

type benchmarkPayload struct {
	ID     int64             `json:"id"`
	Name   string            `json:"name"`
	Status string            `json:"status"`
	Labels map[string]string `json:"labels"`
}

func BenchmarkKeyBuilderBuild(b *testing.B) {
	builder := key.NewBuilder("seven")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = builder.Build("sso", "client", i, "client:web", "env:dev")
	}
}

func BenchmarkSonicCodecRoundTrip(b *testing.B) {
	codec, err := NewCodec("sonic")
	if err != nil {
		b.Fatalf("new codec: %v", err)
	}
	value := benchmarkPayload{
		ID:     1234567890123,
		Name:   "alpha",
		Status: "enabled",
		Labels: map[string]string{"tenant": "default", "scope": "openid profile"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		payload, err := codec.Marshal(value)
		if err != nil {
			b.Fatalf("marshal: %v", err)
		}
		var decoded benchmarkPayload
		if err := codec.Unmarshal(payload, &decoded); err != nil {
			b.Fatalf("unmarshal: %v", err)
		}
	}
}

func BenchmarkRedisManagerSetGet(b *testing.B) {
	manager, cleanup := newBenchmarkManager(b)
	defer cleanup()

	ctx := context.Background()
	payload := benchmarkPayload{
		ID:     1234567890123,
		Name:   "alpha",
		Status: "enabled",
		Labels: map[string]string{"tenant": "default", "scope": "openid profile"},
	}
	cacheKey := manager.Builder().Build("sso", "client", "bench")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := manager.Set(ctx, cacheKey, payload, time.Minute); err != nil {
			b.Fatalf("set: %v", err)
		}
		var loaded benchmarkPayload
		hit, err := manager.Get(ctx, cacheKey, &loaded)
		if err != nil {
			b.Fatalf("get: %v", err)
		}
		if !hit {
			b.Fatal("expected cache hit")
		}
	}
}

func BenchmarkTwoLevelCacheL1Hit(b *testing.B) {
	manager, cleanup := newBenchmarkManager(b)
	defer cleanup()

	ctx := context.Background()
	cacheKey := manager.Builder().Build("dict", "config", "hot")
	loaderCalls := 0

	var warm benchmarkPayload
	_, err := manager.GetOrLoad(ctx, cacheKey, &warm, time.Minute, func(context.Context) (any, error) {
		loaderCalls++
		return benchmarkPayload{
			ID:     1,
			Name:   "warm",
			Status: "ok",
			Labels: map[string]string{"source": "loader"},
		}, nil
	})
	if err != nil {
		b.Fatalf("warm get or load: %v", err)
	}
	if loaderCalls != 1 {
		b.Fatalf("unexpected loader calls: %d", loaderCalls)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var loaded benchmarkPayload
		hit, err := manager.GetOrLoad(ctx, cacheKey, &loaded, time.Minute, func(context.Context) (any, error) {
			loaderCalls++
			return nil, nil
		})
		if err != nil {
			b.Fatalf("l1 get or load: %v", err)
		}
		if !hit {
			b.Fatal("expected l1 hit")
		}
	}
}

func BenchmarkRedisPrimitiveSetNXStringAndIncr(b *testing.B) {
	manager, cleanup := newBenchmarkManager(b)
	defer cleanup()

	ctx := context.Background()
	setNXKeys := make([]string, b.N)
	incrKeys := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		setNXKeys[i] = manager.Builder().Build("nonce", "bench", i)
		incrKeys[i] = manager.Builder().Build("counter", "bench", i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ok, err := manager.SetNXString(ctx, setNXKeys[i], "1", time.Minute)
		if err != nil {
			b.Fatalf("setnx: %v", err)
		}
		if !ok {
			b.Fatal("expected setnx success")
		}
		if _, err := manager.Incr(ctx, incrKeys[i], time.Minute); err != nil {
			b.Fatalf("incr: %v", err)
		}
	}
}
