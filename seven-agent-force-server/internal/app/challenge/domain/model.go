package domain

import "time"

type SubjectHint struct {
	SubjectType  string `json:"subjectType,omitempty"`
	SubjectValue string `json:"subjectValue,omitempty"`
}

type RiskContext struct {
	IPAddress        string `json:"ipAddress,omitempty"`
	UserAgent        string `json:"userAgent,omitempty"`
	DeviceIdentifier string `json:"deviceIdentifier,omitempty"`
	TenantIdentifier string `json:"tenantIdentifier,omitempty"`
}

type ChallengeSession struct {
	ChallengeIdentifier       string
	IssuingServiceName        string
	AudienceServiceNames      []string
	SubjectHint               *SubjectHint
	SubjectIdentifier         string
	SubjectAccountName        string
	SubjectUserID             int64
	FlowNonce                 string
	IdempotencyKey            string
	CreatedAt                 *time.Time
	ExpiresAt                 *time.Time
	EffectiveTTLSeconds       int
	BusinessAction            string
	ChallengeState            ChallengeState
	RecommendedStepIdentifier string
	AuthenticationMethodNames []string
	Steps                     []ChallengeStep
	SessionContext            map[string]any
	FailureCode               string
}

type OtpBindingRecord struct {
	UserID          int64
	SecretEncrypted string
	VerifiedAt      *time.Time
	LastUsedAt      *time.Time
}

type PasskeyRegistration struct {
	CredentialIdentifier string
	PublicKeyCose        string
	SignCount            int64
	UserHandle           string
	AAGUID               string
	Transports           string
	AttestationFormat    string
	DisplayName          string
}

func (s *ChallengeSession) EnsureSessionContext() map[string]any {
	if s.SessionContext == nil {
		s.SessionContext = make(map[string]any)
	}
	return s.SessionContext
}

func (s *ChallengeSession) EnsureAuthenticationMethodNames() []string {
	if s.AuthenticationMethodNames == nil {
		s.AuthenticationMethodNames = make([]string, 0, 2)
	}
	return s.AuthenticationMethodNames
}

func (s *ChallengeSession) IsExpired(now time.Time) bool {
	if s == nil || s.ExpiresAt == nil {
		return true
	}
	return now.After(s.ExpiresAt.UTC())
}

func (s *ChallengeSession) CurrentStep(now time.Time) *ChallengeStep {
	if s == nil {
		return nil
	}
	if s.RecommendedStepIdentifier != "" {
		for i := range s.Steps {
			step := &s.Steps[i]
			if step.StepIdentifier == s.RecommendedStepIdentifier &&
				step.StepState == ChallengeStepStateInProgress &&
				!step.IsCoolingDown(now) {
				return step
			}
		}
	}
	for i := range s.Steps {
		step := &s.Steps[i]
		if step.StepState == ChallengeStepStateInProgress && !step.IsCoolingDown(now) {
			return step
		}
	}
	if s.RecommendedStepIdentifier != "" {
		for i := range s.Steps {
			step := &s.Steps[i]
			if step.StepIdentifier == s.RecommendedStepIdentifier &&
				step.StepState == ChallengeStepStateInProgress {
				return step
			}
		}
	}
	for i := range s.Steps {
		step := &s.Steps[i]
		if step.StepState == ChallengeStepStateInProgress {
			return step
		}
	}
	return nil
}

func (s *ChallengeSession) ActiveSlotSteps(now time.Time) []*ChallengeStep {
	current := s.CurrentStep(now)
	if current == nil {
		return nil
	}
	slot := current.SlotNumber
	result := make([]*ChallengeStep, 0, 2)
	for i := range s.Steps {
		step := &s.Steps[i]
		if step.SlotNumber == slot && step.StepState == ChallengeStepStateInProgress {
			result = append(result, step)
		}
	}
	return result
}

func (s *ChallengeSession) MarkStepSuccess(step *ChallengeStep, now time.Time) {
	if s == nil || step == nil {
		return
	}
	step.ClearCooldown()
	step.StepState = ChallengeStepStateCompleted
	s.AuthenticationMethodNames = append(s.EnsureAuthenticationMethodNames(), string(step.ChallengeType))
	for i := range s.Steps {
		item := &s.Steps[i]
		if item.StepIdentifier != step.StepIdentifier &&
			item.SlotNumber == step.SlotNumber &&
			item.StepState == ChallengeStepStateInProgress {
			item.StepState = ChallengeStepStateLocked
			item.ClearCooldown()
		}
	}
	if !s.ActivateSlot(step.SlotNumber + 1) {
		s.ChallengeState = ChallengeStatePassed
	}
	s.RefreshRecommendedStepIdentifier(now)
}

func (s *ChallengeSession) MarkStepFailure(step *ChallengeStep, code string, now time.Time) {
	if s == nil || step == nil {
		return
	}
	step.AttemptNumber++
	if !step.CanRetry() {
		step.ClearCooldown()
		step.StepState = ChallengeStepStateLocked
		if len(s.ActiveSlotSteps(now)) == 0 {
			s.ChallengeState = ChallengeStateFailed
			s.FailureCode = code
		}
	} else {
		step.ActivateCooldown(now)
	}
	s.RefreshRecommendedStepIdentifier(now)
}

func (s *ChallengeSession) ActivateSlot(slotNumber int) bool {
	if s == nil {
		return false
	}
	activated := false
	for i := range s.Steps {
		step := &s.Steps[i]
		if step.SlotNumber == slotNumber && step.StepState == ChallengeStepStatePending {
			step.StepState = ChallengeStepStateInProgress
			activated = true
		}
	}
	return activated
}

func (s *ChallengeSession) RefreshRecommendedStepIdentifier(now time.Time) {
	if s == nil {
		return
	}
	current := s.CurrentStep(now)
	if current == nil {
		s.RecommendedStepIdentifier = ""
		return
	}
	s.RecommendedStepIdentifier = current.StepIdentifier
}

func (s *ChallengeSession) IsSwitchable(step *ChallengeStep) bool {
	if s == nil || step == nil || step.StepState != ChallengeStepStateInProgress {
		return false
	}
	count := 0
	for i := range s.Steps {
		item := &s.Steps[i]
		if item.SlotNumber == step.SlotNumber && item.StepState == ChallengeStepStateInProgress {
			count++
		}
	}
	return count > 1
}
