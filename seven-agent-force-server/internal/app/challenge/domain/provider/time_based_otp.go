package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	totpinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/totp"
)

type TimeBasedOtpSettings struct {
	IssuerName          string
	AllowedDriftWindows int
}

type TimeBasedOtpChallengeStepProvider struct {
	totp     *totpinfra.Service
	store    SubjectCredentialStore
	settings TimeBasedOtpSettings
}

func NewTimeBasedOtpChallengeStepProvider(
	totp *totpinfra.Service,
	store SubjectCredentialStore,
	settings TimeBasedOtpSettings,
) *TimeBasedOtpChallengeStepProvider {
	return &TimeBasedOtpChallengeStepProvider{
		totp:     totp,
		store:    store,
		settings: settings,
	}
}

func (p *TimeBasedOtpChallengeStepProvider) Type() domain.ChallengeType {
	return domain.ChallengeTypeTimeBasedOneTimePassword
}

func (p *TimeBasedOtpChallengeStepProvider) Eligible(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) (bool, error) {
	if p == nil || p.store == nil {
		return false, nil
	}
	if step != nil && step.PurposeOrDefault() == domain.ChallengeStepPurposeRegisterNew {
		return true, nil
	}
	secret, err := p.store.FindEnabledOtpSecret(ctx, session)
	if err != nil {
		if appErr := apperrors.From(err); appErr != nil && appErr.Kind() == apperrors.KindObjectState {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(secret) != "", nil
}

func (p *TimeBasedOtpChallengeStepProvider) Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if p == nil || p.totp == nil {
		return fmt.Errorf("time based otp provider is not fully configured")
	}
	if session == nil {
		return apperrors.Params("challenge session不能为空")
	}
	if step == nil {
		return apperrors.Params("challenge step不能为空")
	}
	hints := step.EnsureUserInterfaceHints()
	purpose := step.PurposeOrDefault()
	if purpose == domain.ChallengeStepPurposeRegisterNew {
		issuer := p.resolveIssuerName()
		accountName := "user"
		if p.store != nil {
			if resolved, err := p.store.ResolveAccountName(ctx, session); err != nil {
				return err
			} else if strings.TrimSpace(resolved) != "" {
				accountName = resolved
			}
		}
		enrollment, err := p.totp.GenerateSecret(ctx, issuer, accountName)
		if err != nil {
			return err
		}
		sessionCtx := session.EnsureSessionContext()
		sessionCtx["otp.pendingSecretPlain"] = enrollment.Secret
		sessionCtx["otp.newSecretEncrypted"] = enrollment.Secret

		otpauthURL, err := p.totp.BuildKeyURI(enrollment.Secret, issuer, accountName)
		if err != nil {
			return err
		}
		hints["secret"] = enrollment.Secret
		hints["issuer"] = issuer
		hints["accountName"] = accountName
		hints["algorithm"] = "SHA1"
		hints["digits"] = 6
		hints["period"] = totpinfra.DefaultPeriod
		hints["otpauthUrl"] = otpauthURL
		return nil
	}

	hints["purpose"] = string(purpose)
	hints["digits"] = 6
	hints["period"] = totpinfra.DefaultPeriod
	return nil
}

func (p *TimeBasedOtpChallengeStepProvider) Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	if p == nil || p.totp == nil || p.store == nil {
		return false, fmt.Errorf("time based otp provider is not fully configured")
	}
	if session == nil {
		return false, apperrors.Params("challenge session不能为空")
	}
	if step == nil {
		return false, apperrors.Params("challenge step不能为空")
	}
	otpCode := extractOTP(payload)
	if otpCode == "" {
		return false, nil
	}

	purpose := step.PurposeOrDefault()
	var secret string
	if purpose == domain.ChallengeStepPurposeRegisterNew || purpose == domain.ChallengeStepPurposeVerifyNew {
		if session == nil || session.SessionContext == nil {
			return false, nil
		}
		secret, _ = session.SessionContext["otp.pendingSecretPlain"].(string)
		secret = strings.TrimSpace(secret)
	} else {
		resolved, err := p.store.FindEnabledOtpSecret(ctx, session)
		if err != nil {
			return false, err
		}
		secret = strings.TrimSpace(resolved)
	}
	if secret == "" {
		return false, nil
	}

	valid := p.totp.VerifyCode(secret, otpCode, time.Now(), p.allowedDriftWindows())
	if !valid {
		return false, nil
	}
	return true, nil
}

func (p *TimeBasedOtpChallengeStepProvider) resolveIssuerName() string {
	if value := strings.TrimSpace(p.settings.IssuerName); value != "" {
		return value
	}
	return "SevenFramework"
}

func (p *TimeBasedOtpChallengeStepProvider) allowedDriftWindows() int {
	if p.settings.AllowedDriftWindows < 0 {
		return totpinfra.DefaultSkew
	}
	return p.settings.AllowedDriftWindows
}

func extractOTP(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{"oneTimePassword", "otpCode"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		raw := strings.TrimSpace(fmt.Sprintf("%v", value))
		if len(raw) == 6 {
			return raw
		}
	}
	return ""
}
