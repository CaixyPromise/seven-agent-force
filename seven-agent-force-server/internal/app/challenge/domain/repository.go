package domain

import (
	"context"
	"time"
)

type MfaCredentialRepository interface {
	FindEnabledOtpBinding(ctx context.Context, userID int64) (*OtpBindingRecord, error)
	ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error)
	MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error
}

type ChallengeSessionRepository interface {
	SaveSession(ctx context.Context, session *ChallengeSession) error
	GetSession(ctx context.Context, challengeIdentifier string) (*ChallengeSession, error)
	BindIdempotencyKey(ctx context.Context, idempotencyKey, challengeIdentifier string, ttl time.Duration) error
	GetSessionByIdempotencyKey(ctx context.Context, idempotencyKey string) (string, bool, error)
	AcquireSubmitLock(ctx context.Context, challengeIdentifier string, ttl time.Duration) (string, bool, error)
	ReleaseSubmitLock(ctx context.Context, challengeIdentifier, token string) error
	MarkProofConsumed(ctx context.Context, tokenUniqueIdentifier, audience string, ttl time.Duration) (bool, error)
}

type ChallengeThrottleKey struct {
	Dimension string
	Value     string
}

type ChallengeThrottleDecision struct {
	Locked           bool
	Dimension        string
	FailureCount     int
	RemainingSeconds int
}

type ChallengeThrottleRepository interface {
	CheckLocked(ctx context.Context, keys []ChallengeThrottleKey) (*ChallengeThrottleDecision, error)
	RecordFailure(ctx context.Context, keys []ChallengeThrottleKey, maxFailures int, windowTTL, lockTTL time.Duration) (*ChallengeThrottleDecision, error)
	ClearFailures(ctx context.Context, keys []ChallengeThrottleKey) error
}
