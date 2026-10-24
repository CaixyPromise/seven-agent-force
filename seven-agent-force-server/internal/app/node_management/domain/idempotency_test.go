package domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	nodeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestCommandAndPreparedKeysShareClusterHashTag(t *testing.T) {
	metadata := CommandMetadata{
		NodeCode:       "order-admin",
		Method:         "PUT",
		Path:           "/users/2001/status",
		IdempotencyKey: "raw-command-key",
		RequestDigest:  "digest-a",
	}
	commandKey := commandCacheKey(metadata)
	preparedKey := preparedCacheKey(metadata)
	commandTag := redisHashTag(commandKey)
	preparedTag := redisHashTag(preparedKey)
	if commandTag == "" || commandTag != preparedTag {
		t.Fatalf("command key %q and prepared key %q must share a non-empty hash tag", commandKey, preparedKey)
	}
	if commandKey == preparedKey {
		t.Fatal("command and prepared keys must remain distinct")
	}
	if strings.Contains(commandKey, metadata.IdempotencyKey) || strings.Contains(preparedKey, metadata.IdempotencyKey) {
		t.Fatal("Redis keys must not expose the raw idempotency key")
	}

	otherNode := metadata
	otherNode.NodeCode = "billing-admin"
	otherScope := metadata
	otherScope.Path = "/users/3001/status"
	if redisHashTag(commandCacheKey(otherNode)) == commandTag {
		t.Fatal("different nodes must not share a command hash tag")
	}
	if redisHashTag(commandCacheKey(otherScope)) == commandTag {
		t.Fatal("different command scopes must not share a command hash tag")
	}
}

func TestCommandScopeHashIsOpaqueAndDistinctByIdempotencyKey(t *testing.T) {
	first := CommandMetadata{
		NodeCode:       "order-admin",
		Method:         "PUT",
		Path:           "/internal/node/v1/users/2001/status",
		IdempotencyKey: "raw-command-key-a",
	}
	second := first
	second.IdempotencyKey = "raw-command-key-b"

	firstHash := CommandScopeHash(first)
	secondHash := CommandScopeHash(second)
	if len(firstHash) != sha256.Size*2 || firstHash == secondHash {
		t.Fatalf("scope hashes first=%q second=%q", firstHash, secondHash)
	}
	if strings.Contains(firstHash, first.IdempotencyKey) || strings.Contains(secondHash, second.IdempotencyKey) {
		t.Fatal("command scope hash exposed raw idempotency key")
	}
}

func redisHashTag(key string) string {
	start := strings.IndexByte(key, '{')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(key[start+1:], '}')
	if end <= 0 {
		return ""
	}
	return key[start+1 : start+1+end]
}

func TestCommandCoordinatorReplaysCompletedResult(t *testing.T) {
	coordinator, _, _ := newRedisCoordinator(t)
	request := CommandMetadata{NodeCode: "order-admin", Method: "PUT", Path: "/users/2001/status", IdempotencyKey: "cmd-1", RequestDigest: "digest-a"}
	var calls atomic.Int64
	op := func(context.Context) ([]byte, error) {
		calls.Add(1)
		return []byte(`{"changedCount":1}`), nil
	}

	first, replayed, err := coordinator.Execute(context.Background(), request, op)
	if err != nil || replayed || string(first) != `{"changedCount":1}` {
		t.Fatalf("first execute result=%s replayed=%v err=%v", first, replayed, err)
	}
	second, replayed, err := coordinator.Execute(context.Background(), request, op)
	if err != nil || !replayed || string(second) != string(first) {
		t.Fatalf("replay result=%s replayed=%v err=%v", second, replayed, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("operation calls=%d want 1", calls.Load())
	}
}

func TestCommandCoordinatorRejectsConcurrentAndConflictingClaims(t *testing.T) {
	coordinator, _, _ := newRedisCoordinator(t)
	request := CommandMetadata{NodeCode: "order-admin", Method: "POST", Path: "/login-policy/apply", IdempotencyKey: "cmd-2", RequestDigest: "digest-a"}
	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, _ = coordinator.Execute(context.Background(), request, func(context.Context) ([]byte, error) {
			close(started)
			<-release
			return []byte(`{"changedCount":1}`), nil
		})
	}()
	<-started

	_, _, err := coordinator.Execute(context.Background(), request, func(context.Context) ([]byte, error) {
		t.Fatal("concurrent operation must not run")
		return nil, nil
	})
	assertCode(t, err, apperrors.CodeObjectStateInvalid)
	if RetryAfter(err) <= 0 {
		t.Fatalf("in-progress error must carry retry-after: %v", err)
	}

	conflict := request
	conflict.RequestDigest = "digest-b"
	_, _, err = coordinator.Execute(context.Background(), conflict, func(context.Context) ([]byte, error) {
		t.Fatal("conflicting operation must not run")
		return nil, nil
	})
	assertCode(t, err, apperrors.CodeObjectStateInvalid)
	close(release)
	wg.Wait()
}

func TestCommandCoordinatorFailsClosedAndReleasesFailedOwner(t *testing.T) {
	unavailable := NewCommandCoordinator(nodeinfra.NewCommandStore(cacheinfra.NewManager("disabled", nil)))
	request := CommandMetadata{NodeCode: "order-admin", Method: "PUT", Path: "/users/2001/status", IdempotencyKey: "cmd-3", RequestDigest: "digest-a"}
	mutated := false
	_, _, err := unavailable.Execute(context.Background(), request, func(context.Context) ([]byte, error) {
		mutated = true
		return nil, nil
	})
	assertCode(t, err, apperrors.CodeServiceUnavailable)
	if mutated {
		t.Fatal("operation ran while Redis was unavailable")
	}

	coordinator, _, _ := newRedisCoordinator(t)
	wantErr := errors.New("mutation failed")
	_, _, err = coordinator.Execute(context.Background(), request, func(context.Context) ([]byte, error) { return nil, wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("first error=%v want %v", err, wantErr)
	}
	_, replayed, err := coordinator.Execute(context.Background(), request, func(context.Context) ([]byte, error) { return []byte(`{}`), nil })
	if err != nil || replayed {
		t.Fatalf("retry replayed=%v err=%v", replayed, err)
	}
}

func TestCommandCoordinatorRetryAfterResultWriteFailureDoesNotCreateSecondBusinessEffect(t *testing.T) {
	manager, _ := newRedisManager(t)
	failing := &failReplaceManager{Manager: manager, fail: true}
	coordinator := NewCommandCoordinator(nodeinfra.NewCommandStore(failing))
	request := CommandMetadata{NodeCode: "order-admin", Method: "PUT", Path: "/users/2001/status", IdempotencyKey: "cmd-crash", RequestDigest: "digest-a"}
	state := 0
	businessEffects := 0
	operation := func(context.Context) ([]byte, error) {
		if state != 1 {
			state = 1
			businessEffects++
		}
		return []byte(`{"changedCount":1}`), nil
	}

	_, _, err := coordinator.Execute(context.Background(), request, operation)
	assertCode(t, err, apperrors.CodeServiceUnavailable)
	if state != 1 || businessEffects != 1 {
		t.Fatalf("first attempt state=%d effects=%d", state, businessEffects)
	}
	if err := manager.Delete(context.Background(), commandCacheKey(request)); err != nil {
		t.Fatalf("expire abandoned owner: %v", err)
	}
	_, replayed, err := coordinator.Execute(context.Background(), request, operation)
	if err != nil || replayed {
		t.Fatalf("retry replayed=%v err=%v", replayed, err)
	}
	if businessEffects != 1 {
		t.Fatalf("retry created duplicate business effect: %d", businessEffects)
	}
}

func TestCommandPreparationIsStableFor24HoursAndFailsClosed(t *testing.T) {
	coordinator, _, mini := newRedisCoordinator(t)
	request := CommandMetadata{NodeCode: "order-admin", Method: "POST", Path: "/users/2001/sessions/revoke", IdempotencyKey: "cmd-prepared", RequestDigest: "digest-a"}
	prepareCalls := 0
	first, err := coordinator.Prepare(context.Background(), request, func(context.Context) ([]byte, error) {
		prepareCalls++
		return []byte(`["accepted-session"]`), nil
	})
	if err != nil || string(first) != `["accepted-session"]` {
		t.Fatalf("first preparation=%s err=%v", first, err)
	}
	second, err := coordinator.Prepare(context.Background(), request, func(context.Context) ([]byte, error) {
		prepareCalls++
		return []byte(`["later-session"]`), nil
	})
	if err != nil || string(second) != string(first) || prepareCalls != 1 {
		t.Fatalf("replayed preparation=%s calls=%d err=%v", second, prepareCalls, err)
	}
	if ttl := mini.TTL(preparedCacheKey(request)); ttl != 24*time.Hour {
		t.Fatalf("prepared TTL=%s want 24h", ttl)
	}

	conflict := request
	conflict.RequestDigest = "digest-b"
	_, err = coordinator.Prepare(context.Background(), conflict, func(context.Context) ([]byte, error) {
		t.Fatal("conflicting preparation must not run")
		return nil, nil
	})
	assertCode(t, err, apperrors.CodeObjectStateInvalid)

	unavailable := NewCommandCoordinator(nodeinfra.NewCommandStore(cacheinfra.NewManager("disabled", nil)))
	prepareCalls = 0
	_, err = unavailable.Prepare(context.Background(), request, func(context.Context) ([]byte, error) {
		prepareCalls++
		return nil, nil
	})
	assertCode(t, err, apperrors.CodeServiceUnavailable)
	if prepareCalls != 0 {
		t.Fatal("preparation read failure did not fail closed")
	}
}

func TestCommandCompletionRefreshesPreparedTTLAndPrepareDetectsCompletedFirst(t *testing.T) {
	coordinator, manager, mini := newRedisCoordinator(t)
	request := CommandMetadata{NodeCode: "order-admin", Method: "POST", Path: "/users/2001/sessions/revoke", IdempotencyKey: "cmd-expiry-gap", RequestDigest: "digest-a"}
	prepareCalls := 0
	if _, err := coordinator.Prepare(context.Background(), request, func(context.Context) ([]byte, error) {
		prepareCalls++
		return []byte(`{"cutoff":"2026-07-11T12:00:00Z"}`), nil
	}); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	mini.FastForward(23 * time.Hour)
	if _, _, err := coordinator.Execute(context.Background(), request, func(context.Context) ([]byte, error) {
		return []byte(`{"changedCount":2}`), nil
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	preparedTTL := mini.TTL(preparedCacheKey(request))
	completedTTL := mini.TTL(commandCacheKey(request))
	if preparedTTL != completedTTL || preparedTTL != 24*time.Hour {
		t.Fatalf("prepared TTL=%s completed TTL=%s want aligned 24h", preparedTTL, completedTTL)
	}

	if err := manager.Delete(context.Background(), preparedCacheKey(request)); err != nil {
		t.Fatalf("delete prepared fixture: %v", err)
	}
	payload, err := coordinator.Prepare(context.Background(), request, func(context.Context) ([]byte, error) {
		prepareCalls++
		return []byte(`{"cutoff":"later"}`), nil
	})
	if err != nil {
		t.Fatalf("prepare completed command: %v", err)
	}
	if payload != nil || prepareCalls != 1 {
		t.Fatalf("completed command invoked preparer: payload=%s calls=%d", payload, prepareCalls)
	}
}

func newRedisCoordinator(t *testing.T) (*Coordinator, cacheinfra.Manager, *miniredis.Miniredis) {
	t.Helper()
	manager, mini := newRedisManager(t)
	return NewCommandCoordinator(nodeinfra.NewCommandStore(manager)), manager, mini
}

func newRedisManager(t *testing.T) (cacheinfra.Manager, *miniredis.Miniredis) {
	t.Helper()
	mini := miniredis.RunT(t)
	cfg := config.CacheConfig{Enabled: true, Codec: "sonic", Redis: config.RedisCacheConfig{Enabled: true, Mode: config.RedisCacheModeSingle, KeyPrefix: "seven", Single: config.RedisSingleConfig{Addr: mini.Addr()}}}
	manager, err := cacheinfra.NewDefaultManager(cfg, cacheinfra.NewProvider(cfg))
	if err != nil {
		t.Fatalf("new cache manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.DeleteMany(context.Background()) })
	return manager, mini
}

type failReplaceManager struct {
	cacheinfra.Manager
	fail bool
}

func (m *failReplaceManager) CompareAndSetStringAndExpire(ctx context.Context, key, expected, replacement, expiryKey string, ttl time.Duration) (bool, error) {
	if m.fail {
		m.fail = false
		return false, errors.New("redis write failed")
	}
	return m.Manager.CompareAndSetStringAndExpire(ctx, key, expected, replacement, expiryKey, ttl)
}

func (m *failReplaceManager) CompareAndSetString(ctx context.Context, key, expected, replacement string, ttl time.Duration) (bool, error) {
	if m.fail {
		m.fail = false
		return false, errors.New("redis write failed")
	}
	return m.Manager.CompareAndSetString(ctx, key, expected, replacement, ttl)
}

func assertCode(t *testing.T, err error, want int) {
	t.Helper()
	if got := apperrors.From(err).Code(); got != want {
		t.Fatalf("code=%d want %d err=%v", got, want, err)
	}
}
