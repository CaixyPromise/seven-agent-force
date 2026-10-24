package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	redisinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/redis"
	redisclient "github.com/redis/go-redis/v9"
)

type DistributedLock interface {
	TryLock(ctx context.Context, cacheKey string, ttl time.Duration) (string, bool, error)
	Unlock(ctx context.Context, cacheKey, token string) (bool, error)
	Refresh(ctx context.Context, cacheKey, token string, ttl time.Duration) (bool, error)
}

type SchedulerLockService interface {
	TryAcquire(ctx context.Context, cacheKey string, ttl time.Duration) (string, bool, error)
	Release(ctx context.Context, cacheKey, token string) (bool, error)
}

type ReplayProtectionService interface {
	CheckAndSetNonce(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error)
}

type Service interface {
	DistributedLock
	SchedulerLockService
	ReplayProtectionService
}

type RedisService struct {
	provider cacheinfra.Provider
}

func NewRedisService(provider cacheinfra.Provider) *RedisService {
	return &RedisService{provider: provider}
}

func (s *RedisService) TryLock(ctx context.Context, cacheKey string, ttl time.Duration) (string, bool, error) {
	client, err := s.client()
	if err != nil {
		return "", false, err
	}
	token, err := randomToken()
	if err != nil {
		return "", false, err
	}
	ok, err := client.SetNX(ctx, cacheKey, token, ttl).Result()
	if err != nil || !ok {
		return "", ok, err
	}
	return token, true, nil
}

func (s *RedisService) Unlock(ctx context.Context, cacheKey, token string) (bool, error) {
	client, err := s.client()
	if err != nil {
		return false, err
	}
	deleted, err := redisinfra.CompareDeleteScript.Run(ctx, client, []string{cacheKey}, token).Int64()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (s *RedisService) Refresh(ctx context.Context, cacheKey, token string, ttl time.Duration) (bool, error) {
	client, err := s.client()
	if err != nil {
		return false, err
	}
	updated, err := redisinfra.CompareExpireScript.Run(ctx, client, []string{cacheKey}, token, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return updated > 0, nil
}

func (s *RedisService) TryAcquire(ctx context.Context, cacheKey string, ttl time.Duration) (string, bool, error) {
	return s.TryLock(ctx, cacheKey, ttl)
}

func (s *RedisService) Release(ctx context.Context, cacheKey, token string) (bool, error) {
	return s.Unlock(ctx, cacheKey, token)
}

func (s *RedisService) CheckAndSetNonce(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	client, err := s.client()
	if err != nil {
		return false, err
	}
	return client.SetNX(ctx, cacheKey, "1", ttl).Result()
}

func (s *RedisService) client() (redisclient.UniversalClient, error) {
	if s == nil || s.provider == nil || !s.provider.Configured() || s.provider.Client() == nil {
		return nil, cacheinfra.ErrRedisUnavailable
	}
	return s.provider.Client(), nil
}

func randomToken() (string, error) {
	var buffer [16]byte
	var encoded [32]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", errors.New("generate lock token failed")
	}
	hex.Encode(encoded[:], buffer[:])
	return string(encoded[:]), nil
}
