package provider

import (
	"context"
	"fmt"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xtime"
)

type RecoveryCodeChallengeStepProvider struct {
	store SubjectCredentialStore
}

func NewRecoveryCodeChallengeStepProvider(store SubjectCredentialStore) *RecoveryCodeChallengeStepProvider {
	return &RecoveryCodeChallengeStepProvider{store: store}
}

func (p *RecoveryCodeChallengeStepProvider) Type() domain.ChallengeType {
	return domain.ChallengeTypeRecoveryCodeVerification
}

func (p *RecoveryCodeChallengeStepProvider) Eligible(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) (bool, error) {
	_ = step
	if p == nil || p.store == nil {
		return false, nil
	}
	counter, ok := p.store.(interface {
		CountAvailableRecoveryCodes(context.Context, *domain.ChallengeSession) (int, error)
	})
	if !ok {
		return false, nil
	}
	count, err := counter.CountAvailableRecoveryCodes(ctx, session)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *RecoveryCodeChallengeStepProvider) Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	_ = ctx
	if session == nil {
		return apperrors.Params("challenge session不能为空")
	}
	if step == nil {
		return apperrors.Params("challenge step不能为空")
	}
	hints := step.EnsureUserInterfaceHints()
	hints["format"] = "XXXX-XXXX-XXXX"
	hints["singleUse"] = true
	return nil
}

func (p *RecoveryCodeChallengeStepProvider) Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	_ = step
	if p == nil || p.store == nil {
		return false, fmt.Errorf("recovery code provider is not fully configured")
	}
	if session == nil {
		return false, apperrors.Params("challenge session不能为空")
	}
	recoveryCode := payloadString(payload, "recoveryCode")
	if recoveryCode == "" {
		return false, nil
	}
	return p.store.ConsumeRecoveryCode(ctx, session, recoveryCode, xtime.Now())
}
