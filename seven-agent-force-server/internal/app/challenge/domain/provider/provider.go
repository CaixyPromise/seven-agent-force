package provider

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
)

type ChallengeStepProvider interface {
	Type() domain.ChallengeType
	Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error
	Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error)
}

type ChallengeStepEligibilityProvider interface {
	Eligible(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) (bool, error)
}

type RefreshableChallengeStepProvider interface {
	Refresh(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error
}

type SubjectCredentialStore interface {
	FindEnabledOtpBinding(ctx context.Context, session *domain.ChallengeSession) (*domain.OtpBindingRecord, error)
	FindEnabledOtpSecret(ctx context.Context, session *domain.ChallengeSession) (string, error)
	FindPasswordCredential(ctx context.Context, session *domain.ChallengeSession) (string, error)
	ListPasskeys(ctx context.Context, session *domain.ChallengeSession) ([]domain.PasskeyRegistration, error)
	FindPasskey(ctx context.Context, credentialKey string) (*domain.PasskeyRegistration, error)
	UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error
	ConsumeRecoveryCode(ctx context.Context, session *domain.ChallengeSession, recoveryCode string, usedAt time.Time) (bool, error)
	CompleteTotpBinding(ctx context.Context, session *domain.ChallengeSession, plainSecret string, verifiedAt time.Time, recoveryBatchSize int) error
	CompletePasskeyBinding(ctx context.Context, session *domain.ChallengeSession, registration domain.PasskeyRegistration, disableExisting bool, verifiedAt time.Time, recoveryBatchSize int) error
	ResolveAccountName(ctx context.Context, session *domain.ChallengeSession) (string, error)
	ResolveTargetEmail(ctx context.Context, session *domain.ChallengeSession) (string, error)
}
