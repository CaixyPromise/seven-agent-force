package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
)

const (
	challengeThrottleCountPrefix = "challenge:throttle:count:"
	challengeThrottleLockPrefix  = "challenge:throttle:lock:"
)

type ThrottleRepository struct {
	cache cacheinfra.Manager
}

func NewThrottleRepository(cache cacheinfra.Manager) *ThrottleRepository {
	return &ThrottleRepository{cache: cache}
}

func (r *ThrottleRepository) CheckLocked(ctx context.Context, keys []domain.ChallengeThrottleKey) (*domain.ChallengeThrottleDecision, error) {
	if r == nil || r.cache == nil || len(keys) == 0 {
		return nil, nil
	}
	for _, key := range keys {
		normalized, ok := normalizeThrottleKey(key)
		if !ok {
			continue
		}
		if _, found, err := r.cache.GetString(ctx, challengeThrottleLockCacheKey(normalized)); err != nil {
			return nil, err
		} else if found {
			return &domain.ChallengeThrottleDecision{
				Locked:           true,
				Dimension:        key.Dimension,
				RemainingSeconds: 1,
			}, nil
		}
	}
	return nil, nil
}

func (r *ThrottleRepository) RecordFailure(ctx context.Context, keys []domain.ChallengeThrottleKey, maxFailures int, windowTTL, lockTTL time.Duration) (*domain.ChallengeThrottleDecision, error) {
	if r == nil || r.cache == nil || len(keys) == 0 || maxFailures <= 0 {
		return nil, nil
	}
	if windowTTL <= 0 {
		windowTTL = 5 * time.Minute
	}
	if lockTTL <= 0 {
		lockTTL = windowTTL
	}
	var locked *domain.ChallengeThrottleDecision
	for _, key := range keys {
		normalized, ok := normalizeThrottleKey(key)
		if !ok {
			continue
		}
		count, err := r.cache.Incr(ctx, challengeThrottleCountCacheKey(normalized), windowTTL)
		if err != nil {
			return nil, err
		}
		if int(count) >= maxFailures {
			if err := r.cache.SetString(ctx, challengeThrottleLockCacheKey(normalized), "1", lockTTL); err != nil {
				return nil, err
			}
			if locked == nil {
				locked = &domain.ChallengeThrottleDecision{
					Locked:           true,
					Dimension:        key.Dimension,
					FailureCount:     int(count),
					RemainingSeconds: int(lockTTL.Seconds()),
				}
			}
		}
	}
	return locked, nil
}

func (r *ThrottleRepository) ClearFailures(ctx context.Context, keys []domain.ChallengeThrottleKey) error {
	if r == nil || r.cache == nil || len(keys) == 0 {
		return nil
	}
	cacheKeys := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		normalized, ok := normalizeThrottleKey(key)
		if !ok {
			continue
		}
		cacheKeys = append(cacheKeys, challengeThrottleCountCacheKey(normalized), challengeThrottleLockCacheKey(normalized))
	}
	if len(cacheKeys) == 0 {
		return nil
	}
	return r.cache.DeleteMany(ctx, cacheKeys...)
}

func normalizeThrottleKey(key domain.ChallengeThrottleKey) (string, bool) {
	dimension := strings.ToLower(strings.TrimSpace(key.Dimension))
	value := strings.ToLower(strings.TrimSpace(key.Value))
	if dimension == "" || value == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(dimension + "|" + value))
	return fmt.Sprintf("%s:%s", dimension, hex.EncodeToString(sum[:])), true
}

func challengeThrottleCountCacheKey(normalized string) string {
	return challengeThrottleCountPrefix + normalized
}

func challengeThrottleLockCacheKey(normalized string) string {
	return challengeThrottleLockPrefix + normalized
}
