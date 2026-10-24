package facade

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type MfaCredentialFacade interface {
	FindEnabledOtpBinding(ctx context.Context, userID int64) (*OtpBindingRecord, error)
	ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error)
	MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error
}

type MfaManagementFacade interface {
	QueryMfaStatus(ctx context.Context, request MfaStatusRequest) (*MfaStatusResponse, error)
	QueryMfaStatusByUserID(ctx context.Context, userID int64) (*MfaStatusResponse, error)
	RegenerateRecoveryCodes(ctx context.Context, request RegenerateRecoveryCodeRequest) (*RegenerateRecoveryCodeResponse, error)
	RegenerateRecoveryCodesByUserID(ctx context.Context, userID int64, proof stepup.ProofMetadata) (*RegenerateRecoveryCodeResponse, error)
	RegenerateRecoveryCodesWithChallenge(ctx context.Context, request RegenerateRecoveryCodeRequest, context MfaProtectedOperationContext) (*RegenerateRecoveryCodeResponse, error)
	DeleteOtpBindingByUserID(ctx context.Context, userID int64, proof stepup.ProofMetadata) (bool, error)
	DeleteOtpBindingWithChallenge(ctx context.Context, request MfaDeleteOtpBindingRequest, context MfaProtectedOperationContext) (bool, error)
	ListPasskeys(ctx context.Context, request MfaPasskeyListRequest) ([]MfaPasskeyVO, error)
	ListPasskeysByUserID(ctx context.Context, userID int64) ([]MfaPasskeyVO, error)
	DeletePasskeyByUserID(ctx context.Context, userID int64, credentialIdentifier string, proof stepup.ProofMetadata) (bool, error)
	DeletePasskeyWithChallenge(ctx context.Context, request MfaDeletePasskeyRequest, context MfaProtectedOperationContext) (bool, error)
	StartMfaChallenge(ctx context.Context, request MfaChallengeStartRequest, context MfaChallengeStartContext) (*StartChallengeResponse, error)
	StartMfaChallengeByUserID(ctx context.Context, userID int64, request MfaChallengeStartRequest, context MfaChallengeStartContext) (*StartChallengeResponse, error)
}

type ChallengeStepFacade interface {
	PrepareStep(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error
	VerifyStep(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error)
	OnPassed(ctx context.Context, session *domain.ChallengeSession) error
}

type ChallengeInternalFacade interface {
	StartChallenge(ctx context.Context, request StartChallengeRequest) (*StartChallengeResponse, error)
}

type ChallengeClientFacade interface {
	GetChallenge(ctx context.Context, challengeIdentifier string) (*StartChallengeResponse, error)
	Respond(ctx context.Context, challengeIdentifier string, request RespondChallengeRequest) (*RespondChallengeResponse, error)
	Refresh(ctx context.Context, challengeIdentifier string, request RefreshChallengeRequest) (*StartChallengeResponse, error)
}

type ProofTokenVerifier interface {
	VerifyProofToken(ctx context.Context, request ProofTokenVerifyRequest) (*ProofTokenClaims, error)
}
