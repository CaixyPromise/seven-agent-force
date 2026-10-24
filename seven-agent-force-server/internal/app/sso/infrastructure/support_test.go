package infrastructure

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/key"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestBuildRefreshCookieFallsBackFromHostPrefixWhenInsecure(t *testing.T) {
	cfg := config.SSORefreshCookieConfig{
		Name:     "__Host-seven_sso_rt",
		Path:     "/",
		SameSite: "Lax",
		Secure:   false,
		HTTPOnly: true,
	}

	header := BuildRefreshCookie(cfg, "refresh-token", time.Unix(1713830400, 0).UTC())
	if !strings.HasPrefix(header, "seven_sso_rt=") {
		t.Fatalf("BuildRefreshCookie() = %q, want downgraded non-__Host cookie name", header)
	}
}

func TestBuildExpiredRefreshCookiesClearsConfiguredAndFallbackNames(t *testing.T) {
	cfg := config.SSORefreshCookieConfig{
		Name:     "__Host-seven_sso_rt",
		Path:     "/",
		SameSite: "Lax",
		Secure:   false,
		HTTPOnly: true,
	}

	headers := BuildExpiredRefreshCookies(cfg)
	if len(headers) != 2 {
		t.Fatalf("BuildExpiredRefreshCookies() len = %d, want 2", len(headers))
	}
	if !strings.HasPrefix(headers[0], "seven_sso_rt=") {
		t.Fatalf("first expired cookie = %q, want downgraded cookie name", headers[0])
	}
	if !strings.HasPrefix(headers[1], "__Host-seven_sso_rt=") {
		t.Fatalf("second expired cookie = %q, want configured cookie name", headers[1])
	}
}

func TestMarkUserRevokedRetainsAtomicMaximum(t *testing.T) {
	ctx := context.Background()
	manager, mini := newSupportRedisManager(t)
	cache := NewAuthSessionCache(manager)
	first := time.Date(2026, 7, 11, 12, 0, 0, 123, time.UTC)
	newer := first.Add(2 * time.Minute)
	older := first.Add(-2 * time.Minute)

	if err := cache.MarkUserRevoked(ctx, 2001, first); err != nil {
		t.Fatalf("first watermark: %v", err)
	}
	assertUserRevokedAt(t, cache, 2001, first)
	if err := cache.MarkUserRevoked(ctx, 2001, newer); err != nil {
		t.Fatalf("newer watermark: %v", err)
	}
	assertUserRevokedAt(t, cache, 2001, newer)
	if err := cache.MarkUserRevoked(ctx, 2001, older); err != nil {
		t.Fatalf("older watermark replay: %v", err)
	}
	assertUserRevokedAt(t, cache, 2001, newer)
	if ttl := mini.TTL(cache.revokedMarkerKey(2001)); ttl != revokedMarkerTTL {
		t.Fatalf("watermark TTL=%s want %s", ttl, revokedMarkerTTL)
	}
}

func TestMarkUserRevokedConcurrentOrderingRetainsMaximum(t *testing.T) {
	ctx := context.Background()
	manager, _ := newSupportRedisManager(t)
	cache := NewAuthSessionCache(manager)
	base := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	values := []time.Time{base.Add(time.Second), base.Add(4 * time.Second), base.Add(2 * time.Second), base.Add(3 * time.Second)}

	start := make(chan struct{})
	errs := make(chan error, len(values))
	var wg sync.WaitGroup
	for _, value := range values {
		value := value
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- cache.MarkUserRevoked(ctx, 2001, value)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent watermark: %v", err)
		}
	}
	assertUserRevokedAt(t, cache, 2001, base.Add(4*time.Second))
}

func TestMarkUserRevokedAdvancesLegacyVariableWidthTimestamp(t *testing.T) {
	ctx := context.Background()
	manager, _ := newSupportRedisManager(t)
	cache := NewAuthSessionCache(manager)
	legacy := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	if err := manager.SetString(ctx, cache.revokedMarkerKey(2001), strconvTime(legacy), revokedMarkerTTL); err != nil {
		t.Fatalf("seed legacy watermark: %v", err)
	}
	newer := legacy.Add(123 * time.Nanosecond)
	if err := cache.MarkUserRevoked(ctx, 2001, newer); err != nil {
		t.Fatalf("advance legacy watermark: %v", err)
	}
	assertUserRevokedAt(t, cache, 2001, newer)
}

func TestMarkUserRevokedFailsClosedWhenCacheUnavailable(t *testing.T) {
	cache := NewAuthSessionCache(cacheinfra.NewManager("disabled", key.NewBuilder("seven")))
	if err := cache.MarkUserRevoked(context.Background(), 2001, time.Now()); err == nil {
		t.Fatal("missing Redis primitive must fail closed")
	}
}

func newSupportRedisManager(t *testing.T) (cacheinfra.Manager, *miniredis.Miniredis) {
	t.Helper()
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Codec: "sonic", Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "seven", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	manager, err := cacheinfra.NewDefaultManager(cfg, cacheinfra.NewProvider(cfg))
	if err != nil {
		t.Fatalf("new cache manager: %v", err)
	}
	return manager, mini
}

func assertUserRevokedAt(t *testing.T, cache *AuthSessionCache, userID int64, want time.Time) {
	t.Helper()
	got, err := cache.UserRevokedAt(context.Background(), userID)
	if err != nil || got == nil || !got.Equal(want) {
		t.Fatalf("watermark=%v want %s err=%v", got, want, err)
	}
}
