package infrastructure

import (
	"context"
	"strings"
	"testing"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestLoginFailureStateStoreReadsJavaKeysAndLegacyKeys(t *testing.T) {
	manager := newTestCacheManager(t)
	store := NewLoginFailureStateStore(manager)
	ctx := context.Background()

	if err := store.SaveFailureCount(ctx, "alice", 3); err != nil {
		t.Fatalf("save failure count: %v", err)
	}
	count, err := store.GetFailureCount(ctx, "alice")
	if err != nil {
		t.Fatalf("get failure count: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected primary failure count 3, got %d", count)
	}

	if err := store.DeleteFailureCount(ctx, "alice"); err != nil {
		t.Fatalf("delete failure count: %v", err)
	}
	legacyKey := manager.Builder().Build("login", "failure", "alice")
	if err := manager.SetString(ctx, legacyKey, "5", time.Minute); err != nil {
		t.Fatalf("set legacy failure count: %v", err)
	}
	count, err = store.GetFailureCount(ctx, "alice")
	if err != nil {
		t.Fatalf("get legacy failure count: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected legacy failure count 5, got %d", count)
	}

	if err := store.SaveLockUntil(ctx, "alice", 1234567890, 1); err != nil {
		t.Fatalf("save lock until: %v", err)
	}
	unlockAt, err := store.GetLockUntil(ctx, "alice")
	if err != nil {
		t.Fatalf("get lock until: %v", err)
	}
	if unlockAt == nil || *unlockAt != 1234567890 {
		t.Fatalf("expected java-style lock until, got %#v", unlockAt)
	}

	if err := store.DeleteLock(ctx, "alice"); err != nil {
		t.Fatalf("delete lock: %v", err)
	}
	legacyLockKey := manager.Builder().Build("login", "lock", "alice")
	if err := manager.SetString(ctx, legacyLockKey, "22334455", time.Minute); err != nil {
		t.Fatalf("set legacy lock until: %v", err)
	}
	unlockAt, err = store.GetLockUntil(ctx, "alice")
	if err != nil {
		t.Fatalf("get legacy lock until: %v", err)
	}
	if unlockAt == nil || *unlockAt != 22334455 {
		t.Fatalf("expected legacy lock until, got %#v", unlockAt)
	}

	if err := store.DeleteFailureCount(ctx, "alice"); err != nil {
		t.Fatalf("delete failure count again: %v", err)
	}
	if err := store.DeleteLock(ctx, "alice"); err != nil {
		t.Fatalf("delete lock again: %v", err)
	}
	if err := store.SaveCaptchaFailureCount(ctx, "alice", 4); err != nil {
		t.Fatalf("save captcha failure count: %v", err)
	}
	captchaCount, err := store.GetCaptchaFailureCount(ctx, "alice")
	if err != nil {
		t.Fatalf("get captcha failure count: %v", err)
	}
	if captchaCount != 4 {
		t.Fatalf("expected captcha failure count 4, got %d", captchaCount)
	}
	if err := store.DeleteCaptchaFailureCount(ctx, "alice"); err != nil {
		t.Fatalf("delete captcha failure count: %v", err)
	}
	captchaCount, err = store.GetCaptchaFailureCount(ctx, "alice")
	if err != nil {
		t.Fatalf("get captcha failure count after delete: %v", err)
	}
	if captchaCount != 0 {
		t.Fatalf("expected captcha failure count reset to 0, got %d", captchaCount)
	}
	if err := manager.SetString(ctx, captchaFailureKeyPrefix+"alice", "6", time.Minute); err != nil {
		t.Fatalf("set legacy captcha failure count: %v", err)
	}
	captchaCount, err = store.GetCaptchaFailureCount(ctx, "alice")
	if err != nil {
		t.Fatalf("get legacy captcha failure count: %v", err)
	}
	if captchaCount != 6 {
		t.Fatalf("expected legacy captcha failure count 6, got %d", captchaCount)
	}
	if err := store.DeleteCaptchaFailureCount(ctx, "alice"); err != nil {
		t.Fatalf("delete legacy captcha failure count: %v", err)
	}
	if value, found, err := manager.GetString(ctx, captchaFailureKeyPrefix+"alice"); err != nil || found || value != "" {
		t.Fatalf("expected legacy captcha failure key deleted, found=%v value=%q err=%v", found, value, err)
	}
}

func TestLoginFailureStateStoreDeleteRemovesNewAndLegacyKeys(t *testing.T) {
	manager := newTestCacheManager(t)
	store := NewLoginFailureStateStore(manager)
	ctx := context.Background()

	if err := store.SaveFailureCount(ctx, "bob", 3); err != nil {
		t.Fatalf("save primary failure key: %v", err)
	}
	if err := manager.SetString(ctx, loginFailureKeyPrefix+"bob", "2", time.Minute); err != nil {
		t.Fatalf("set java failure key: %v", err)
	}
	if err := manager.SetString(ctx, manager.Builder().Build("login", "failure", "bob"), "7", time.Minute); err != nil {
		t.Fatalf("set legacy failure key: %v", err)
	}
	if err := store.DeleteFailureCount(ctx, "bob"); err != nil {
		t.Fatalf("delete failure count: %v", err)
	}
	if value, found, err := manager.GetString(ctx, store.failureKey("bob")); err != nil || found || value != "" {
		t.Fatalf("expected primary failure key deleted, found=%v value=%q err=%v", found, value, err)
	}
	if value, found, err := manager.GetString(ctx, loginFailureKeyPrefix+"bob"); err != nil || found || value != "" {
		t.Fatalf("expected java failure key deleted, found=%v value=%q err=%v", found, value, err)
	}
	if value, found, err := manager.GetString(ctx, manager.Builder().Build("login", "failure", "bob")); err != nil || found || value != "" {
		t.Fatalf("expected legacy failure key deleted, found=%v value=%q err=%v", found, value, err)
	}
}

func TestLoginFailureStateStoreDoesNotWriteRawAccountKeys(t *testing.T) {
	manager, mini := newTestCacheManagerWithServer(t)
	store := NewLoginFailureStateStore(manager)
	ctx := context.Background()
	account := "alice@example.com"

	if err := store.SaveFailureCount(ctx, account, 3); err != nil {
		t.Fatalf("save failure count: %v", err)
	}
	if err := store.SaveLockUntil(ctx, account, time.Now().UTC().Add(time.Hour).UnixMilli(), 1); err != nil {
		t.Fatalf("save lock until: %v", err)
	}
	if err := store.SaveCaptchaFailureCount(ctx, account, 2); err != nil {
		t.Fatalf("save captcha failure count: %v", err)
	}

	for _, key := range mini.Keys() {
		if strings.Contains(key, account) {
			t.Fatalf("login punishment cache key leaked raw account %q in key %q", account, key)
		}
	}
	if count, err := store.GetFailureCount(ctx, account); err != nil || count != 3 {
		t.Fatalf("expected hashed failure count 3, got count=%d err=%v", count, err)
	}
	if count, err := store.GetCaptchaFailureCount(ctx, account); err != nil || count != 2 {
		t.Fatalf("expected hashed captcha count 2, got count=%d err=%v", count, err)
	}
	if unlockAt, err := store.GetLockUntil(ctx, account); err != nil || unlockAt == nil {
		t.Fatalf("expected hashed lock value, got value=%#v err=%v", unlockAt, err)
	}
}

func TestLoginFailureStateStoreDoesNotWriteRawContextSignalKeys(t *testing.T) {
	manager, mini := newTestCacheManagerWithServer(t)
	store := NewLoginFailureStateStore(manager)
	ctx := context.Background()
	clientIP := "203.0.113.10"
	deviceID := "device-browser-1"

	if err := store.SaveContextFailureCount(ctx, "ip", clientIP, 3); err != nil {
		t.Fatalf("save IP context failure count: %v", err)
	}
	if err := store.SaveContextFailureCount(ctx, "device", deviceID, 4); err != nil {
		t.Fatalf("save device context failure count: %v", err)
	}
	if err := store.SaveContextFailureCount(ctx, "ip_device", clientIP+"|"+deviceID, 5); err != nil {
		t.Fatalf("save IP/device context failure count: %v", err)
	}

	for _, key := range mini.Keys() {
		if strings.Contains(key, clientIP) || strings.Contains(key, deviceID) {
			t.Fatalf("login punishment context key leaked raw signal in key %q", key)
		}
	}
	if count, err := store.GetContextFailureCount(ctx, "ip", clientIP); err != nil || count != 3 {
		t.Fatalf("expected hashed IP context count 3, got count=%d err=%v", count, err)
	}
	if count, err := store.GetContextFailureCount(ctx, "device", deviceID); err != nil || count != 4 {
		t.Fatalf("expected hashed device context count 4, got count=%d err=%v", count, err)
	}
	if count, err := store.GetContextFailureCount(ctx, "ip_device", clientIP+"|"+deviceID); err != nil || count != 5 {
		t.Fatalf("expected hashed IP/device context count 5, got count=%d err=%v", count, err)
	}
}

func newTestCacheManager(t *testing.T) cacheinfra.Manager {
	t.Helper()
	manager, _ := newTestCacheManagerWithServer(t)
	return manager
}

func newTestCacheManagerWithServer(t *testing.T) (cacheinfra.Manager, *miniredis.Miniredis) {
	t.Helper()
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
		t.Fatalf("new default manager: %v", err)
	}
	return manager, mini
}
