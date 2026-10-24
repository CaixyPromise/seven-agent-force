package domain

import (
	"time"
)

type ChallengeType string

const (
	ChallengeTypeImageCaptcha                ChallengeType = "IMAGE_CAPTCHA"
	ChallengeTypePasswordVerification        ChallengeType = "PASSWORD_VERIFICATION"
	ChallengeTypeEmailOneTimePassword        ChallengeType = "EMAIL_ONE_TIME_PASSWORD"
	ChallengeTypeTimeBasedOneTimePassword    ChallengeType = "TIME_BASED_ONE_TIME_PASSWORD"
	ChallengeTypeRecoveryCodeVerification    ChallengeType = "RECOVERY_CODE_VERIFICATION"
	ChallengeTypeWebAuthnPasskeyAssertion    ChallengeType = "WEBAUTHN_PASSKEY_ASSERTION"
	ChallengeTypeWebAuthnPasskeyRegistration ChallengeType = "WEBAUTHN_PASSKEY_REGISTRATION"
)

func AllChallengeTypes() []ChallengeType {
	return []ChallengeType{
		ChallengeTypeImageCaptcha,
		ChallengeTypePasswordVerification,
		ChallengeTypeEmailOneTimePassword,
		ChallengeTypeTimeBasedOneTimePassword,
		ChallengeTypeRecoveryCodeVerification,
		ChallengeTypeWebAuthnPasskeyAssertion,
		ChallengeTypeWebAuthnPasskeyRegistration,
	}
}

type ChallengeStepPurpose string

const (
	ChallengeStepPurposeDefault        ChallengeStepPurpose = "DEFAULT"
	ChallengeStepPurposeVerifyOld      ChallengeStepPurpose = "VERIFY_OLD"
	ChallengeStepPurposeVerifyNew      ChallengeStepPurpose = "VERIFY_NEW"
	ChallengeStepPurposeRegisterNew    ChallengeStepPurpose = "REGISTER_NEW"
	ChallengeStepPurposeRecoveryVerify ChallengeStepPurpose = "RECOVERY_VERIFY"
)

type ChallengeState string

const (
	ChallengeStatePending ChallengeState = "PENDING"
	ChallengeStatePassed  ChallengeState = "PASSED"
	ChallengeStateFailed  ChallengeState = "FAILED"
	ChallengeStateExpired ChallengeState = "EXPIRED"
)

type ChallengeStepState string

const (
	ChallengeStepStatePending    ChallengeStepState = "PENDING"
	ChallengeStepStateInProgress ChallengeStepState = "IN_PROGRESS"
	ChallengeStepStateLocked     ChallengeStepState = "LOCKED"
	ChallengeStepStateCompleted  ChallengeStepState = "COMPLETED"
)

type ChallengeStep struct {
	StepIdentifier     string
	ChallengeType      ChallengeType
	StepPurpose        string
	SlotNumber         int
	AttemptNumber      int
	MaxAttempts        int
	CooldownSeconds    int
	CooldownUntil      *time.Time
	StepState          ChallengeStepState
	UserInterfaceHints map[string]any
}

func (s *ChallengeStep) EnsureUserInterfaceHints() map[string]any {
	if s.UserInterfaceHints == nil {
		s.UserInterfaceHints = make(map[string]any)
	}
	return s.UserInterfaceHints
}

func (s ChallengeStep) PurposeOrDefault() ChallengeStepPurpose {
	if s.StepPurpose == "" {
		return ChallengeStepPurposeDefault
	}
	return ChallengeStepPurpose(s.StepPurpose)
}

func (s *ChallengeStep) RemainingAttemptCount() int {
	if s == nil {
		return 0
	}
	remaining := s.MaxAttempts - s.AttemptNumber
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *ChallengeStep) CanRetry() bool {
	if s == nil {
		return false
	}
	return s.AttemptNumber < s.MaxAttempts
}

func (s *ChallengeStep) IsCoolingDown(now time.Time) bool {
	if s == nil || s.CooldownUntil == nil {
		return false
	}
	return now.Before(*s.CooldownUntil)
}

func (s *ChallengeStep) RemainingCooldownSeconds(now time.Time) int {
	if !s.IsCoolingDown(now) {
		return 0
	}
	seconds := int(s.CooldownUntil.Sub(now).Seconds())
	if seconds <= 0 {
		return 1
	}
	return seconds
}

func (s *ChallengeStep) ActivateCooldown(now time.Time) {
	if s == nil || s.CooldownSeconds <= 0 {
		return
	}
	until := now.UTC().Add(time.Duration(s.CooldownSeconds) * time.Second)
	s.CooldownUntil = &until
}

func (s *ChallengeStep) ClearCooldown() {
	if s == nil {
		return
	}
	s.CooldownUntil = nil
}

func (s *ChallengeStep) IsVerifiable() bool {
	return s != nil && s.StepState == ChallengeStepStateInProgress
}
