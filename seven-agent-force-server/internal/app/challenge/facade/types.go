package facade

import "time"

type OtpBindingRecord struct {
	UserID          int64      `json:"userId"`
	SecretEncrypted string     `json:"secretEncrypted"`
	VerifiedAt      *time.Time `json:"verifiedAt,omitempty"`
	LastUsedAt      *time.Time `json:"lastUsedAt,omitempty"`
}

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

type StartChallengeRequest struct {
	IssuingServiceName         string         `json:"issuingServiceName" validate:"required"`
	AudienceServiceNames       []string       `json:"audienceServiceNames" validate:"required,min=1,dive,required"`
	BusinessAction             string         `json:"businessAction" validate:"required"`
	SubjectHint                *SubjectHint   `json:"subjectHint,omitempty"`
	SubjectIdentifier          string         `json:"subjectIdentifier,omitempty"`
	FlowNonce                  string         `json:"flowNonce" validate:"required"`
	RequestedTimeToLiveSeconds int            `json:"requestedTimeToLiveSeconds,omitempty"`
	IdempotencyKey             string         `json:"idempotencyKey" validate:"required"`
	RiskContext                *RiskContext   `json:"riskContext,omitempty"`
	RequiredAssuranceLevel     string         `json:"requiredAssuranceLevel,omitempty"`
	MinimumAssuranceLevel      string         `json:"minimumAssuranceLevel,omitempty"`
	ExpectedChallengeTypes     []string       `json:"expectedChallengeTypes,omitempty"`
	AllowDowngrade             *bool          `json:"allowDowngrade,omitempty"`
	PolicyIdentifier           string         `json:"policyIdentifier,omitempty"`
	ExtensionContext           map[string]any `json:"extensionContext,omitempty"`
}

type ChallengeStepVO struct {
	StepIdentifier        string         `json:"stepIdentifier"`
	ChallengeType         string         `json:"challengeType"`
	StepPurpose           string         `json:"stepPurpose,omitempty"`
	StepState             string         `json:"stepState"`
	RemainingAttemptCount int            `json:"remainingAttemptCount"`
	CooldownSeconds       int            `json:"cooldownSeconds"`
	Switchable            bool           `json:"switchable"`
	UserInterfaceHints    map[string]any `json:"userInterfaceHints,omitempty"`
}

type StartChallengeResponse struct {
	ChallengeIdentifier        string            `json:"challengeIdentifier"`
	ExpiresAt                  *time.Time        `json:"expiresAt,omitempty"`
	Steps                      []ChallengeStepVO `json:"steps,omitempty"`
	ChallengeState             string            `json:"challengeState"`
	EffectiveTimeToLiveSeconds int               `json:"effectiveTimeToLiveSeconds"`
	RequiredAssuranceLevel     string            `json:"requiredAssuranceLevel,omitempty"`
	ResolvedAssuranceLevel     string            `json:"resolvedAssuranceLevel,omitempty"`
	RecommendedStepIdentifier  string            `json:"recommendedStepIdentifier,omitempty"`
	ActualChallengeTypeNames   []string          `json:"actualChallengeTypeNames,omitempty"`
}

type RespondChallengeRequest struct {
	StepIdentifier string         `json:"stepIdentifier" validate:"required"`
	Payload        map[string]any `json:"payload,omitempty"`
}

type RespondChallengeResponse struct {
	ChallengeState            string `json:"challengeState"`
	ProofToken                string `json:"proofToken,omitempty"`
	NextStepIdentifier        string `json:"nextStepIdentifier,omitempty"`
	RemainingAttemptCount     int    `json:"remainingAttemptCount"`
	CooldownSeconds           int    `json:"cooldownSeconds"`
	CanSwitchMethod           bool   `json:"canSwitchMethod"`
	RecommendedStepIdentifier string `json:"recommendedStepIdentifier,omitempty"`
	FailureReason             string `json:"-"`
}

type RefreshChallengeRequest struct {
	StepIdentifier string `json:"stepIdentifier,omitempty"`
}

type ProofTokenClaims struct {
	IssuerServiceName         string     `json:"issuerServiceName"`
	AudienceServiceNames      []string   `json:"audienceServiceNames,omitempty"`
	SubjectIdentifier         string     `json:"subjectIdentifier"`
	BusinessAction            string     `json:"businessAction"`
	ChallengeIdentifier       string     `json:"challengeIdentifier"`
	FlowNonce                 string     `json:"flowNonce"`
	OperationBinding          string     `json:"operationBinding,omitempty"`
	AuthenticationMethodNames []string   `json:"authenticationMethodNames,omitempty"`
	TokenUniqueIdentifier     string     `json:"tokenUniqueIdentifier"`
	IssuedAt                  *time.Time `json:"issuedAt,omitempty"`
	ExpiresAt                 *time.Time `json:"expiresAt,omitempty"`
}

type ProofTokenVerifyRequest struct {
	ProofToken          string `json:"proofToken" validate:"required"`
	AudienceServiceName string `json:"audienceServiceName" validate:"required"`
	BusinessAction      string `json:"businessAction" validate:"required"`
	FlowNonce           string `json:"flowNonce" validate:"required"`
	SubjectIdentifier   string `json:"subjectIdentifier,omitempty"`
	OperationBinding    string `json:"operationBinding,omitempty"`
	ConsumeOnce         bool   `json:"consumeOnce"`
}

type MfaStatusRequest struct {
	SubjectIdentifier string `json:"subjectIdentifier"`
}

type MfaStatusResponse struct {
	SubjectIdentifier          string `json:"subjectIdentifier"`
	OTPBound                   bool   `json:"otpBound"`
	PasskeyBound               bool   `json:"passkeyBound"`
	AvailableRecoveryCodeCount int    `json:"availableRecoveryCodeCount"`
}

type RegenerateRecoveryCodeRequest struct {
	SubjectIdentifier string `json:"subjectIdentifier,omitempty"`
}

type RegenerateRecoveryCodeResponse struct {
	SubjectIdentifier string     `json:"subjectIdentifier"`
	BatchIdentifier   string     `json:"batchIdentifier"`
	RecoveryCodes     []string   `json:"recoveryCodes"`
	GeneratedAt       *time.Time `json:"generatedAt,omitempty"`
}

type MfaDeleteOtpBindingRequest struct {
	SubjectIdentifier string `json:"subjectIdentifier,omitempty"`
}

type MfaPasskeyListRequest struct {
	SubjectIdentifier string `json:"subjectIdentifier,omitempty"`
}

type MfaDeletePasskeyRequest struct {
	SubjectIdentifier    string `json:"subjectIdentifier,omitempty"`
	CredentialIdentifier string `json:"credentialIdentifier" validate:"required"`
}

type MfaPasskeyVO struct {
	CredentialIdentifier string     `json:"credentialIdentifier"`
	DisplayName          string     `json:"displayName,omitempty"`
	AAGUID               string     `json:"aaguid,omitempty"`
	Transports           string     `json:"transports,omitempty"`
	CreatedAt            *time.Time `json:"createdAt,omitempty"`
	LastUsedAt           *time.Time `json:"lastUsedAt,omitempty"`
}

type MfaChallengeStartRequest struct {
	BusinessAction             string         `json:"businessAction" validate:"required"`
	FlowNonce                  string         `json:"flowNonce,omitempty"`
	RequestedTimeToLiveSeconds int            `json:"requestedTimeToLiveSeconds,omitempty"`
	ExtensionContext           map[string]any `json:"extensionContext,omitempty"`
}

type MfaChallengeStartContext struct {
	SubjectIdentifier string `json:"subjectIdentifier"`
	IPAddress         string `json:"ipAddress,omitempty"`
	UserAgent         string `json:"userAgent,omitempty"`
	DeviceIdentifier  string `json:"deviceIdentifier,omitempty"`
	TenantIdentifier  string `json:"tenantIdentifier,omitempty"`
}

type MfaProtectedOperationContext struct {
	SubjectIdentifier string `json:"subjectIdentifier"`
	ProofToken        string `json:"proofToken,omitempty"`
	FlowNonce         string `json:"flowNonce,omitempty"`
	IPAddress         string `json:"ipAddress,omitempty"`
	UserAgent         string `json:"userAgent,omitempty"`
	DeviceIdentifier  string `json:"deviceIdentifier,omitempty"`
	TenantIdentifier  string `json:"tenantIdentifier,omitempty"`
}
