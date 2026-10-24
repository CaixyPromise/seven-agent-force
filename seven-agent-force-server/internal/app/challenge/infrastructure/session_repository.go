package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/google/uuid"
)

const (
	challengeSessionKeyPrefix     = "challenge:session:"
	challengeIdempotencyKeyPrefix = "challenge:idempotency:"
	challengeSubmitLockKeyPrefix  = "challenge:submit-lock:"
	challengeProofConsumedPrefix  = "challenge:proof-consumed:"
	defaultSubmitLockTTL          = 15 * time.Second
)

type SessionRepository struct {
	cache cacheinfra.Manager
}

func NewSessionRepository(cache cacheinfra.Manager) *SessionRepository {
	return &SessionRepository{cache: cache}
}

func (r *SessionRepository) SaveSession(ctx context.Context, session *domain.ChallengeSession) error {
	if r == nil || r.cache == nil || session == nil || strings.TrimSpace(session.ChallengeIdentifier) == "" {
		return fmt.Errorf("challenge session repository is not configured")
	}
	ttl := sessionTTL(session)
	return r.cache.Set(ctx, challengeSessionCacheKey(session.ChallengeIdentifier), session, ttl)
}

func (r *SessionRepository) GetSession(ctx context.Context, challengeIdentifier string) (*domain.ChallengeSession, error) {
	if r == nil || r.cache == nil || strings.TrimSpace(challengeIdentifier) == "" {
		return nil, nil
	}
	var result domain.ChallengeSession
	found, err := r.cache.Get(ctx, challengeSessionCacheKey(challengeIdentifier), &result)
	if err != nil || !found {
		return nil, err
	}
	return &result, nil
}

func (r *SessionRepository) BindIdempotencyKey(ctx context.Context, idempotencyKey, challengeIdentifier string, ttl time.Duration) error {
	if r == nil || r.cache == nil || strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(challengeIdentifier) == "" {
		return fmt.Errorf("challenge session repository is not configured")
	}
	return r.cache.SetString(ctx, challengeIdempotencyCacheKey(idempotencyKey), challengeIdentifier, ttl)
}

func (r *SessionRepository) GetSessionByIdempotencyKey(ctx context.Context, idempotencyKey string) (string, bool, error) {
	if r == nil || r.cache == nil || strings.TrimSpace(idempotencyKey) == "" {
		return "", false, nil
	}
	return r.cache.GetString(ctx, challengeIdempotencyCacheKey(idempotencyKey))
}

func (r *SessionRepository) AcquireSubmitLock(ctx context.Context, challengeIdentifier string, ttl time.Duration) (string, bool, error) {
	if r == nil || r.cache == nil || strings.TrimSpace(challengeIdentifier) == "" {
		return "", false, fmt.Errorf("challenge session repository is not configured")
	}
	if ttl <= 0 {
		ttl = defaultSubmitLockTTL
	}
	token := uuid.NewString()
	ok, err := r.cache.SetNXString(ctx, challengeSubmitLockCacheKey(challengeIdentifier), token, ttl)
	return token, ok, err
}

func (r *SessionRepository) ReleaseSubmitLock(ctx context.Context, challengeIdentifier, token string) error {
	if r == nil || r.cache == nil || strings.TrimSpace(challengeIdentifier) == "" || strings.TrimSpace(token) == "" {
		return nil
	}
	_, err := r.cache.CompareAndDeleteString(ctx, challengeSubmitLockCacheKey(challengeIdentifier), token)
	return err
}

func (r *SessionRepository) MarkProofConsumed(ctx context.Context, tokenUniqueIdentifier, audience string, ttl time.Duration) (bool, error) {
	if r == nil || r.cache == nil || strings.TrimSpace(tokenUniqueIdentifier) == "" || strings.TrimSpace(audience) == "" {
		return false, fmt.Errorf("challenge session repository is not configured")
	}
	return r.cache.SetNXString(ctx, challengeProofConsumedCacheKey(tokenUniqueIdentifier), audience, ttl)
}

func challengeSessionCacheKey(challengeIdentifier string) string {
	return challengeSessionKeyPrefix + strings.TrimSpace(challengeIdentifier)
}

func challengeIdempotencyCacheKey(idempotencyKey string) string {
	return challengeIdempotencyKeyPrefix + strings.TrimSpace(idempotencyKey)
}

func challengeSubmitLockCacheKey(challengeIdentifier string) string {
	return challengeSubmitLockKeyPrefix + strings.TrimSpace(challengeIdentifier)
}

func challengeProofConsumedCacheKey(tokenUniqueIdentifier string) string {
	return challengeProofConsumedPrefix + strings.TrimSpace(tokenUniqueIdentifier)
}

func sessionTTL(session *domain.ChallengeSession) time.Duration {
	if session == nil || session.ExpiresAt == nil {
		return time.Minute
	}
	ttl := time.Until(session.ExpiresAt.UTC())
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}
