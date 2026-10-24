package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type EmailOtpChallengeStepProvider struct {
	email *challengeinfra.EmailOTPService
	store SubjectCredentialStore
}

func NewEmailOtpChallengeStepProvider(email *challengeinfra.EmailOTPService, store SubjectCredentialStore) *EmailOtpChallengeStepProvider {
	return &EmailOtpChallengeStepProvider{email: email, store: store}
}

func (p *EmailOtpChallengeStepProvider) Type() domain.ChallengeType {
	return domain.ChallengeTypeEmailOneTimePassword
}

func (p *EmailOtpChallengeStepProvider) Eligible(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) (bool, error) {
	if p == nil || p.store == nil {
		return false, nil
	}
	email, err := p.resolveTargetEmail(ctx, session, step)
	if err != nil {
		return false, err
	}
	email = strings.TrimSpace(email)
	if email != "" {
		session.EnsureSessionContext()["email.target"] = email
	}
	return email != "", nil
}

func (p *EmailOtpChallengeStepProvider) Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	return p.Refresh(ctx, session, step)
}

func (p *EmailOtpChallengeStepProvider) Refresh(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if p == nil || p.email == nil || p.store == nil {
		return fmt.Errorf("email otp provider is not fully configured")
	}
	if session == nil || step == nil {
		return apperrors.Params("challenge session或step不能为空")
	}
	email, err := p.resolveTargetEmail(ctx, session, step)
	if err != nil {
		return err
	}
	if strings.TrimSpace(email) == "" {
		step.EnsureUserInterfaceHints()["emailMasked"] = ""
		return nil
	}
	code, err := p.email.Generate(ctx, email, emailScene(session), 5*time.Minute)
	if err != nil {
		return err
	}
	session.EnsureSessionContext()[emailOTPContextKey(step)] = code
	session.EnsureSessionContext()["email.target"] = email
	hints := step.EnsureUserInterfaceHints()
	hints["emailMasked"] = challengeinfra.MaskEmail(email)
	hints["biz"] = "MFA"
	hints["deliveryState"] = "SENT"
	hints["refreshable"] = true
	return nil
}

func emailScene(session *domain.ChallengeSession) string {
	if session == nil {
		return "LOGIN_UNLOCK"
	}
	switch strings.TrimSpace(session.BusinessAction) {
	case string(domain.BusinessActionProfileEmailUpdate):
		return "RESET_EMAIL"
	case string(domain.BusinessActionProfilePhoneUpdate), string(domain.BusinessActionMFAOTPBind), string(domain.BusinessActionMFAOTPSwitch), string(domain.BusinessActionMFAOTPDelete):
		return "LOGIN_UNLOCK"
	default:
		return "LOGIN_UNLOCK"
	}
}

func (p *EmailOtpChallengeStepProvider) Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	_ = ctx
	if p == nil || p.email == nil {
		return false, fmt.Errorf("email otp provider is not fully configured")
	}
	if session == nil || step == nil {
		return false, apperrors.Params("challenge session或step不能为空")
	}
	code := payloadString(payload, "oneTimePassword", "emailCode")
	expected := valueString(session.EnsureSessionContext()[emailOTPContextKey(step)])
	if p.email.Verify(expected, code) {
		delete(session.EnsureSessionContext(), emailOTPContextKey(step))
		return true, nil
	}
	return false, nil
}

func (p *EmailOtpChallengeStepProvider) resolveTargetEmail(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) (string, error) {
	if step != nil && step.PurposeOrDefault() == domain.ChallengeStepPurposeVerifyNew {
		if value := valueString(session.EnsureSessionContext()["targetEmail"]); value != "" {
			return value, nil
		}
		if extension, ok := session.EnsureSessionContext()["extensionContext"].(map[string]any); ok {
			if value := valueString(extension["targetEmail"]); value != "" {
				return value, nil
			}
		}
	}
	return p.store.ResolveTargetEmail(ctx, session)
}

func emailOTPContextKey(step *domain.ChallengeStep) string {
	return "email.otp.code." + step.StepIdentifier
}
