package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
)

// CommandClaim is the infrastructure representation of an owned command claim.
type CommandClaim struct {
	RequestDigest string `json:"requestDigest"`
	OwnerToken    string `json:"ownerToken"`
	Result        []byte `json:"result,omitempty"`
}

// PreparedCommand is the durable preparation captured when a command is accepted.
type PreparedCommand struct {
	RequestDigest string `json:"requestDigest"`
	Payload       []byte `json:"payload"`
}

// CommandStore exposes Redis claim and preparation primitives using infrastructure types.
type CommandStore interface {
	CreateClaim(ctx context.Context, key string, claim CommandClaim, ttl time.Duration) (bool, error)
	GetClaim(ctx context.Context, key string) (CommandClaim, bool, error)
	DeleteClaim(ctx context.Context, key string, expected CommandClaim) (bool, error)
	CompleteClaim(ctx context.Context, key, preparedKey string, expected, completed CommandClaim, ttl time.Duration) (bool, error)
	CreatePrepared(ctx context.Context, key string, prepared PreparedCommand, ttl time.Duration) (bool, error)
	GetPrepared(ctx context.Context, key string) (PreparedCommand, bool, error)
}

type redisCommandStore struct {
	cache cacheinfra.Manager
}

// NewCommandStore creates the Redis-backed command storage adapter.
func NewCommandStore(cache cacheinfra.Manager) CommandStore {
	return &redisCommandStore{cache: cache}
}

func (s *redisCommandStore) CreateClaim(ctx context.Context, key string, claim CommandClaim, ttl time.Duration) (bool, error) {
	encoded, err := encode(claim)
	if err != nil {
		return false, err
	}
	return s.cache.SetNXString(ctx, key, encoded, ttl)
}

func (s *redisCommandStore) GetClaim(ctx context.Context, key string) (CommandClaim, bool, error) {
	var claim CommandClaim
	hit, err := s.get(ctx, key, &claim)
	return claim, hit, err
}

func (s *redisCommandStore) DeleteClaim(ctx context.Context, key string, expected CommandClaim) (bool, error) {
	encoded, err := encode(expected)
	if err != nil {
		return false, err
	}
	return s.cache.CompareAndDeleteString(ctx, key, encoded)
}

func (s *redisCommandStore) CompleteClaim(ctx context.Context, key, preparedKey string, expected, completed CommandClaim, ttl time.Duration) (bool, error) {
	expectedValue, err := encode(expected)
	if err != nil {
		return false, err
	}
	completedValue, err := encode(completed)
	if err != nil {
		return false, err
	}
	return s.cache.CompareAndSetStringAndExpire(ctx, key, expectedValue, completedValue, preparedKey, ttl)
}

func (s *redisCommandStore) CreatePrepared(ctx context.Context, key string, prepared PreparedCommand, ttl time.Duration) (bool, error) {
	encoded, err := encode(prepared)
	if err != nil {
		return false, err
	}
	return s.cache.SetNXString(ctx, key, encoded, ttl)
}

func (s *redisCommandStore) GetPrepared(ctx context.Context, key string) (PreparedCommand, bool, error) {
	var prepared PreparedCommand
	hit, err := s.get(ctx, key, &prepared)
	return prepared, hit, err
}

func (s *redisCommandStore) get(ctx context.Context, key string, destination any) (bool, error) {
	if s == nil || s.cache == nil {
		return false, fmt.Errorf("command store is unavailable")
	}
	value, hit, err := s.cache.GetString(ctx, key)
	if err != nil || !hit {
		return hit, err
	}
	if err := json.Unmarshal([]byte(value), destination); err != nil {
		return false, fmt.Errorf("decode command store value: %w", err)
	}
	return true, nil
}

func encode(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode command store value: %w", err)
	}
	return string(encoded), nil
}
