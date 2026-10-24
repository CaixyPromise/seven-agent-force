package infrastructure

import (
	"context"
	"strings"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
)

const (
	ownerBootstrapLockKey       = "setup:owner:bootstrap:lock"
	ownerSetupTokenNonceKeyPref = "setup:owner:token:nonce:"
)

type StateStore struct {
	cache cacheinfra.Manager
}

func NewStateStore(cache cacheinfra.Manager) *StateStore {
	return &StateStore{cache: cache}
}

func (s *StateStore) ConsumeNonce(ctx context.Context, nonce string, ttl time.Duration) (bool, error) {
	if s == nil || s.cache == nil {
		return false, cacheinfra.ErrCacheLayerUnsupported
	}
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		return false, nil
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return s.cache.SetNXString(ctx, ownerSetupTokenNonceKeyPref+nonce, "1", ttl)
}

func (s *StateStore) AcquireBootstrapLock(ctx context.Context, token string, ttl time.Duration) (bool, error) {
	if s == nil || s.cache == nil {
		return false, cacheinfra.ErrCacheLayerUnsupported
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return s.cache.SetNXString(ctx, ownerBootstrapLockKey, strings.TrimSpace(token), ttl)
}

func (s *StateStore) ReleaseBootstrapLock(ctx context.Context, token string) error {
	if s == nil || s.cache == nil {
		return cacheinfra.ErrCacheLayerUnsupported
	}
	_, err := s.cache.CompareAndDeleteString(ctx, ownerBootstrapLockKey, strings.TrimSpace(token))
	return err
}
