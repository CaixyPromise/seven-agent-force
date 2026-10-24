package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
)

type PasswordChallengeStepProvider struct {
	password *passwordinfra.Service
	store    SubjectCredentialStore
}

func NewPasswordChallengeStepProvider(password *passwordinfra.Service, store SubjectCredentialStore) *PasswordChallengeStepProvider {
	return &PasswordChallengeStepProvider{password: password, store: store}
}

func (p *PasswordChallengeStepProvider) Type() domain.ChallengeType {
	return domain.ChallengeTypePasswordVerification
}

func (p *PasswordChallengeStepProvider) Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	_ = ctx
	if session == nil || step == nil {
		return apperrors.Params("challenge session或step不能为空")
	}
	step.EnsureUserInterfaceHints()["required"] = true
	return nil
}

func (p *PasswordChallengeStepProvider) Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	_ = step
	if p == nil || p.password == nil || p.store == nil {
		return false, fmt.Errorf("password provider is not fully configured")
	}
	if session == nil {
		return false, apperrors.Params("challenge session不能为空")
	}
	password := payloadString(payload, "password")
	if password == "" {
		return false, nil
	}
	hash, err := p.store.FindPasswordCredential(ctx, session)
	if err != nil || strings.TrimSpace(hash) == "" {
		return false, err
	}
	if err := p.password.Verify(ctx, password, hash); err != nil {
		return false, nil
	}
	return true, nil
}
