package infrastructure

import (
	"context"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/bytedance/sonic"
)

type InteractionCacheService struct {
	cache cache.Manager
}

func NewInteractionCacheService(cacheManager cache.Manager) *InteractionCacheService {
	return &InteractionCacheService{cache: cacheManager}
}

func (s *InteractionCacheService) GetInteraction(ctx context.Context, loginTransactionID string) (*domain.InteractionSnapshot, error) {
	if s == nil || s.cache == nil {
		return nil, nil
	}
	raw, found, err := s.cache.GetString(ctx, s.snapshotKey(loginTransactionID))
	if err != nil || !found || strings.TrimSpace(raw) == "" {
		return nil, err
	}
	var snapshot domain.InteractionSnapshot
	if err := sonic.UnmarshalString(raw, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (s *InteractionCacheService) SaveInteraction(ctx context.Context, snapshot *domain.InteractionSnapshot, ttl time.Duration) error {
	if s == nil || s.cache == nil || snapshot == nil {
		return nil
	}
	raw, err := sonic.Marshal(snapshot)
	if err != nil {
		return err
	}
	return s.cache.SetString(ctx, s.snapshotKey(snapshot.LoginTransactionID), string(raw), ttl)
}

func (s *InteractionCacheService) RemoveInteraction(ctx context.Context, loginTransactionID string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, s.snapshotKey(loginTransactionID))
}

func (s *InteractionCacheService) IsCompleted(ctx context.Context, loginTransactionID string) (bool, error) {
	if s == nil || s.cache == nil {
		return false, nil
	}
	_, found, err := s.cache.GetString(ctx, s.completedKey(loginTransactionID))
	if err != nil {
		return false, err
	}
	return found, nil
}

func (s *InteractionCacheService) MarkCompleted(ctx context.Context, loginTransactionID string, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.SetString(ctx, s.completedKey(loginTransactionID), "1", ttl)
}

func (s *InteractionCacheService) AcquirePrimaryAuthenticationLock(ctx context.Context, loginTransactionID string) (bool, error) {
	return s.acquireLock(ctx, s.primaryLockKey(loginTransactionID), 15*time.Second)
}

func (s *InteractionCacheService) ReleasePrimaryAuthenticationLock(ctx context.Context, loginTransactionID string) error {
	return s.releaseLock(ctx, s.primaryLockKey(loginTransactionID))
}

func (s *InteractionCacheService) AcquireChallengeDispatchLock(ctx context.Context, loginTransactionID string) (bool, error) {
	return s.acquireLock(ctx, s.challengeLockKey(loginTransactionID), 15*time.Second)
}

func (s *InteractionCacheService) ReleaseChallengeDispatchLock(ctx context.Context, loginTransactionID string) error {
	return s.releaseLock(ctx, s.challengeLockKey(loginTransactionID))
}

func (s *InteractionCacheService) acquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if s == nil || s.cache == nil {
		return true, nil
	}
	return s.cache.SetNXString(ctx, key, "1", ttl)
}

func (s *InteractionCacheService) releaseLock(ctx context.Context, key string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, key)
}

func (s *InteractionCacheService) snapshotKey(loginTransactionID string) string {
	return s.cache.Builder().Build("login", "interaction", loginTransactionID)
}

func (s *InteractionCacheService) primaryLockKey(loginTransactionID string) string {
	return s.cache.Builder().Build("login", "primary-lock", loginTransactionID)
}

func (s *InteractionCacheService) challengeLockKey(loginTransactionID string) string {
	return s.cache.Builder().Build("login", "challenge-lock", loginTransactionID)
}

func (s *InteractionCacheService) completedKey(loginTransactionID string) string {
	return s.cache.Builder().Build("login", "completed", loginTransactionID)
}
