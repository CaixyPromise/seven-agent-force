package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/google/uuid"
)

type ChallengeService struct {
	config      config.ChallengeConfig
	sessions    domain.ChallengeSessionRepository
	throttles   domain.ChallengeThrottleRepository
	steps       *StepService
	completion  *CompletionHandler
	proofTokens *challengeinfra.ProofTokenService
}

func NewChallengeService(
	cfg config.ChallengeConfig,
	sessions domain.ChallengeSessionRepository,
	throttles domain.ChallengeThrottleRepository,
	steps *StepService,
	completion *CompletionHandler,
	proofTokens *challengeinfra.ProofTokenService,
) *ChallengeService {
	return &ChallengeService{
		config:      cfg,
		sessions:    sessions,
		throttles:   throttles,
		steps:       steps,
		completion:  completion,
		proofTokens: proofTokens,
	}
}

func (s *ChallengeService) StartChallenge(ctx context.Context, request facade.StartChallengeRequest) (*facade.StartChallengeResponse, error) {
	if strings.TrimSpace(request.BusinessAction) == "" || strings.TrimSpace(request.FlowNonce) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return nil, apperrors.Params("businessAction/flowNonce/idempotencyKey不能为空")
	}
	if len(request.AudienceServiceNames) == 0 {
		return nil, apperrors.Params("audienceServiceNames不能为空")
	}
	policy, err := challengeActionPolicy(request.BusinessAction)
	if err != nil {
		return nil, err
	}
	if policy.RequiresOperationBinding() && operationBindingFromExtension(request.ExtensionContext) == "" {
		return nil, apperrors.Params("当前业务动作必须绑定具体操作对象")
	}
	if existingID, found, err := s.sessions.GetSessionByIdempotencyKey(ctx, request.IdempotencyKey); err != nil {
		return nil, err
	} else if found {
		session, err := s.getSession(ctx, existingID)
		if err != nil {
			return nil, err
		}
		response := toStartChallengeResponse(session, time.Now().UTC())
		return &response, nil
	}

	now := time.Now().UTC()
	session := s.buildSession(request, now, policy)
	if err := s.filterEligibleSteps(ctx, session, now); err != nil {
		return nil, err
	}
	if err := ensureRequiredProofStepsAvailable(session, policy); err != nil {
		return nil, err
	}
	for i := range session.Steps {
		if err := s.ensureStepTriggerAllowed(ctx, session, &session.Steps[i]); err != nil {
			return nil, err
		}
	}
	for i := range session.Steps {
		if err := s.steps.PrepareStep(ctx, session, &session.Steps[i]); err != nil {
			return nil, err
		}
	}
	for i := range session.Steps {
		if err := s.recordStepTrigger(ctx, session, &session.Steps[i]); err != nil {
			return nil, err
		}
	}
	session.RefreshRecommendedStepIdentifier(now)
	if err := s.sessions.SaveSession(ctx, session); err != nil {
		return nil, err
	}
	if err := s.sessions.BindIdempotencyKey(ctx, request.IdempotencyKey, session.ChallengeIdentifier, time.Duration(session.EffectiveTTLSeconds)*time.Second); err != nil {
		return nil, err
	}
	response := toStartChallengeResponse(session, now)
	return &response, nil
}

func (s *ChallengeService) filterEligibleSteps(ctx context.Context, session *domain.ChallengeSession, now time.Time) error {
	if session == nil {
		return apperrors.Params("challenge session不能为空")
	}
	steps := make([]domain.ChallengeStep, 0, len(session.Steps))
	for i := range session.Steps {
		step := session.Steps[i]
		eligible, err := s.steps.IsStepEligible(ctx, session, &step)
		if err != nil {
			return err
		}
		if eligible {
			steps = append(steps, step)
		}
	}
	session.Steps = steps
	session.RefreshRecommendedStepIdentifier(now)
	if session.CurrentStep(now) == nil {
		return apperrors.Operation("当前主体没有可用的验证方式")
	}
	return nil
}

func ensureRequiredProofStepsAvailable(session *domain.ChallengeSession, policy domain.ChallengeActionPolicy) error {
	if session == nil {
		return apperrors.Params("challenge session不能为空")
	}
	if len(policy.RequiredProofMethods) == 0 {
		return nil
	}
	available := make(map[domain.ChallengeType]struct{}, len(session.Steps))
	for _, step := range session.Steps {
		available[step.ChallengeType] = struct{}{}
	}
	for _, required := range policy.RequiredProofMethods {
		if _, ok := available[required]; !ok {
			return apperrors.Operation("当前主体没有可用的验证方式")
		}
	}
	return nil
}

func (s *ChallengeService) GetChallenge(ctx context.Context, challengeIdentifier string) (*facade.StartChallengeResponse, error) {
	session, err := s.getSession(ctx, challengeIdentifier)
	if err != nil {
		return nil, err
	}
	response := toStartChallengeResponse(session, time.Now().UTC())
	return &response, nil
}

func (s *ChallengeService) Respond(ctx context.Context, challengeIdentifier string, request facade.RespondChallengeRequest) (*facade.RespondChallengeResponse, error) {
	lockToken, locked, err := s.sessions.AcquireSubmitLock(ctx, challengeIdentifier, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, apperrors.Operation("挑战正在处理中，请勿重复提交")
	}
	defer func() {
		_ = s.sessions.ReleaseSubmitLock(ctx, challengeIdentifier, lockToken)
	}()

	session, err := s.getSession(ctx, challengeIdentifier)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if session.IsExpired(now) {
		session.ChallengeState = domain.ChallengeStateExpired
		session.FailureCode = "SESSION_EXPIRED"
		if saveErr := s.sessions.SaveSession(ctx, session); saveErr != nil {
			return nil, saveErr
		}
		return nil, apperrors.Operation("挑战已过期")
	}
	if session.ChallengeState == domain.ChallengeStatePassed {
		return s.buildPassedResponse(ctx, session)
	}
	if session.ChallengeState != domain.ChallengeStatePending {
		return nil, apperrors.Operation("挑战状态不可继续")
	}

	step, err := s.findStep(session, request.StepIdentifier, now)
	if err != nil {
		return nil, err
	}
	if !step.IsVerifiable() {
		return nil, apperrors.Operation("当前步骤不可提交")
	}
	if cooldown := step.RemainingCooldownSeconds(now); cooldown > 0 {
		return &facade.RespondChallengeResponse{
			ChallengeState:            string(domain.ChallengeStatePending),
			NextStepIdentifier:        session.RecommendedStepIdentifier,
			RemainingAttemptCount:     step.RemainingAttemptCount(),
			CooldownSeconds:           cooldown,
			CanSwitchMethod:           session.IsSwitchable(step),
			RecommendedStepIdentifier: session.RecommendedStepIdentifier,
			FailureReason:             "STEP_COOLDOWN_ACTIVE",
		}, nil
	}
	throttleKeys := challengeThrottleKeys(session, step)
	if decision, err := s.checkThrottle(ctx, throttleKeys); err != nil {
		return nil, err
	} else if decision != nil && decision.Locked {
		return throttledResponse(session, step, decision), nil
	}

	verified, err := s.steps.VerifyStep(ctx, session, step, request.Payload)
	if err != nil {
		return nil, err
	}
	if !verified {
		session.MarkStepFailure(step, "STEP_VERIFY_FAILED", now)
		decision, err := s.recordThrottleFailure(ctx, throttleKeys, step)
		if err != nil {
			return nil, err
		}
		if err := s.sessions.SaveSession(ctx, session); err != nil {
			return nil, err
		}
		if decision != nil && decision.Locked && session.ChallengeState != domain.ChallengeStateFailed {
			return throttledResponse(session, step, decision), nil
		}
		cooldown := step.RemainingCooldownSeconds(time.Now().UTC())
		failureReason := "STEP_VERIFY_FAILED"
		if session.ChallengeState == domain.ChallengeStateFailed {
			failureReason = "STEP_LOCKED"
		}
		return &facade.RespondChallengeResponse{
			ChallengeState:            string(session.ChallengeState),
			NextStepIdentifier:        session.RecommendedStepIdentifier,
			RemainingAttemptCount:     step.RemainingAttemptCount(),
			CooldownSeconds:           cooldown,
			CanSwitchMethod:           session.IsSwitchable(step),
			RecommendedStepIdentifier: session.RecommendedStepIdentifier,
			FailureReason:             failureReason,
		}, nil
	}

	session.MarkStepSuccess(step, now)
	if err := s.clearThrottleFailures(ctx, throttleKeys); err != nil {
		return nil, err
	}
	if session.ChallengeState == domain.ChallengeStatePassed {
		if err := s.completion.OnPassed(ctx, session); err != nil {
			return nil, err
		}
	}
	if err := s.sessions.SaveSession(ctx, session); err != nil {
		return nil, err
	}
	if session.ChallengeState == domain.ChallengeStatePassed {
		return s.buildPassedResponse(ctx, session)
	}
	return &facade.RespondChallengeResponse{
		ChallengeState:            string(session.ChallengeState),
		NextStepIdentifier:        session.RecommendedStepIdentifier,
		RemainingAttemptCount:     0,
		CooldownSeconds:           0,
		CanSwitchMethod:           false,
		RecommendedStepIdentifier: session.RecommendedStepIdentifier,
	}, nil
}

func (s *ChallengeService) Refresh(ctx context.Context, challengeIdentifier string, request facade.RefreshChallengeRequest) (*facade.StartChallengeResponse, error) {
	session, err := s.getSession(ctx, challengeIdentifier)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if session.IsExpired(now) {
		session.ChallengeState = domain.ChallengeStateExpired
		session.FailureCode = "SESSION_EXPIRED"
		if saveErr := s.sessions.SaveSession(ctx, session); saveErr != nil {
			return nil, saveErr
		}
		return nil, apperrors.Operation("挑战已过期")
	}
	if session.ChallengeState != domain.ChallengeStatePending {
		return nil, apperrors.Operation("挑战状态不可继续")
	}
	step, err := s.findStep(session, request.StepIdentifier, now)
	if err != nil {
		return nil, err
	}
	if err := s.ensureRefreshThrottleContext(ctx, session, step); err != nil {
		return nil, err
	}
	if cooldown := emailOTPResendCooldownSeconds(session, step, now); cooldown > 0 {
		return nil, apperrors.ChallengeThrottled("验证码已发送，请稍后再获取").WithDetails(map[string]any{
			"cooldownSeconds": cooldown,
			"dimension":       "email-otp-resend",
		})
	}
	if err := s.ensureStepTriggerAllowed(ctx, session, step); err != nil {
		return nil, err
	}
	if err := s.steps.RefreshStep(ctx, session, step); err != nil {
		return nil, err
	}
	// A newly issued code must be immediately verifiable. A previous invalid
	// submission may have put the step into verification cooldown; retaining
	// that cooldown after replacing the code makes the fresh code unusable.
	step.ClearCooldown()
	if err := s.recordStepTrigger(ctx, session, step); err != nil {
		return nil, err
	}
	if err := s.sessions.SaveSession(ctx, session); err != nil {
		return nil, err
	}
	response := toStartChallengeResponse(session, now)
	return &response, nil
}

func (s *ChallengeService) VerifyProofToken(ctx context.Context, request facade.ProofTokenVerifyRequest) (*facade.ProofTokenClaims, error) {
	policy, err := challengeActionPolicy(request.BusinessAction)
	if err != nil {
		return nil, err
	}
	if policy.RequiresOperationBinding() && strings.TrimSpace(request.OperationBinding) == "" {
		return nil, apperrors.Operation("操作绑定缺失")
	}
	claims, err := s.proofTokens.Verify(ctx, request)
	if err != nil {
		return nil, err
	}
	if !policy.ProofMethodsSatisfied(claims.AuthenticationMethodNames) {
		return nil, apperrors.Operation("proof token认证方式不满足当前业务动作要求")
	}
	return claims, nil
}

func (s *ChallengeService) buildPassedResponse(ctx context.Context, session *domain.ChallengeSession) (*facade.RespondChallengeResponse, error) {
	if session == nil {
		return nil, apperrors.Params("challenge session不能为空")
	}
	if cached := strings.TrimSpace(stringValue(session.EnsureSessionContext()["proofToken"])); cached != "" {
		return &facade.RespondChallengeResponse{
			ChallengeState: string(domain.ChallengeStatePassed),
			ProofToken:     cached,
		}, nil
	}
	_, raw, err := s.proofTokens.Issue(ctx, session)
	if err != nil {
		return nil, err
	}
	session.EnsureSessionContext()["proofToken"] = raw
	if err := s.sessions.SaveSession(ctx, session); err != nil {
		return nil, err
	}
	return &facade.RespondChallengeResponse{
		ChallengeState: string(domain.ChallengeStatePassed),
		ProofToken:     raw,
	}, nil
}

func (s *ChallengeService) getSession(ctx context.Context, challengeIdentifier string) (*domain.ChallengeSession, error) {
	session, err := s.sessions.GetSession(ctx, strings.TrimSpace(challengeIdentifier))
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, apperrors.NotFound("挑战会话不存在")
	}
	return session, nil
}

func (s *ChallengeService) findStep(session *domain.ChallengeSession, stepIdentifier string, now time.Time) (*domain.ChallengeStep, error) {
	if session == nil {
		return nil, apperrors.Params("challenge session不能为空")
	}
	value := strings.TrimSpace(stepIdentifier)
	if value == "" {
		step := session.CurrentStep(now)
		if step == nil {
			return nil, apperrors.Operation("当前步骤不存在")
		}
		return step, nil
	}
	for i := range session.Steps {
		step := &session.Steps[i]
		if step.StepIdentifier == value {
			return step, nil
		}
	}
	return nil, apperrors.NotFound("挑战步骤不存在")
}

func (s *ChallengeService) buildSession(request facade.StartChallengeRequest, now time.Time, policy domain.ChallengeActionPolicy) *domain.ChallengeSession {
	ttlSeconds := request.RequestedTimeToLiveSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = s.config.SessionTTLSeconds
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	session := &domain.ChallengeSession{
		ChallengeIdentifier:       "challenge_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		IssuingServiceName:        strings.TrimSpace(request.IssuingServiceName),
		AudienceServiceNames:      append([]string(nil), request.AudienceServiceNames...),
		SubjectHint:               toDomainSubjectHint(request.SubjectHint),
		SubjectIdentifier:         strings.TrimSpace(request.SubjectIdentifier),
		FlowNonce:                 strings.TrimSpace(request.FlowNonce),
		IdempotencyKey:            strings.TrimSpace(request.IdempotencyKey),
		CreatedAt:                 timePointer(now),
		ExpiresAt:                 timePointer(now.Add(time.Duration(ttlSeconds) * time.Second)),
		EffectiveTTLSeconds:       ttlSeconds,
		BusinessAction:            strings.TrimSpace(request.BusinessAction),
		ChallengeState:            domain.ChallengeStatePending,
		SessionContext:            make(map[string]any),
		AuthenticationMethodNames: make([]string, 0, 2),
	}
	if request.ExtensionContext != nil {
		session.SessionContext["extensionContext"] = request.ExtensionContext
	}
	if value := strings.TrimSpace(request.RequiredAssuranceLevel); value != "" {
		session.SessionContext["requiredAssuranceLevel"] = value
		session.SessionContext["resolvedAssuranceLevel"] = value
	} else if value := strings.TrimSpace(policy.MinimumAssuranceLevel); value != "" {
		session.SessionContext["requiredAssuranceLevel"] = value
		session.SessionContext["resolvedAssuranceLevel"] = value
	}
	if value := strings.TrimSpace(request.MinimumAssuranceLevel); value != "" {
		session.SessionContext["minimumAssuranceLevel"] = value
	}
	if len(request.ExpectedChallengeTypes) > 0 {
		session.SessionContext["expectedChallengeTypes"] = append([]string(nil), request.ExpectedChallengeTypes...)
	}
	if request.AllowDowngrade != nil {
		session.SessionContext["allowDowngrade"] = *request.AllowDowngrade
	}
	if value := strings.TrimSpace(request.PolicyIdentifier); value != "" {
		session.SessionContext["policyIdentifier"] = value
	}
	if request.RiskContext != nil {
		session.SessionContext["riskContext"] = map[string]any{
			"ipAddress":        request.RiskContext.IPAddress,
			"userAgent":        request.RiskContext.UserAgent,
			"deviceIdentifier": request.RiskContext.DeviceIdentifier,
			"tenantIdentifier": request.RiskContext.TenantIdentifier,
		}
	}
	session.Steps = s.buildSteps(request)
	session.RefreshRecommendedStepIdentifier(now)
	return session
}

func (s *ChallengeService) buildSteps(request facade.StartChallengeRequest) []domain.ChallengeStep {
	action := domain.BusinessAction(strings.TrimSpace(request.BusinessAction))
	switch action {
	case domain.BusinessActionLogin:
		return []domain.ChallengeStep{s.buildLoginStep(request.ExpectedChallengeTypes)}
	case domain.BusinessActionRegisterAccount:
		return []domain.ChallengeStep{s.buildRegisterStep(request.ExpectedChallengeTypes)}
	case domain.BusinessActionProfilePhoneUpdate:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyAssertion, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeRecoveryCodeVerification, domain.ChallengeStepPurposeRecoveryVerify, 1),
			s.buildStep(domain.ChallengeTypeEmailOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
		}
	case domain.BusinessActionProfileEmailUpdate:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyAssertion, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeRecoveryCodeVerification, domain.ChallengeStepPurposeRecoveryVerify, 1),
			s.buildStep(domain.ChallengeTypeEmailOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeEmailOneTimePassword, domain.ChallengeStepPurposeVerifyNew, 2),
		}
	case domain.BusinessActionMFAOTPBind:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypePasswordVerification, domain.ChallengeStepPurposeDefault, 1),
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeRegisterNew, 2),
		}
	case domain.BusinessActionMFAOTPSwitch:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypePasswordVerification, domain.ChallengeStepPurposeDefault, 2),
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeRegisterNew, 3),
		}
	case domain.BusinessActionMFAPasskeyBind:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypePasswordVerification, domain.ChallengeStepPurposeDefault, 1),
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyRegistration, domain.ChallengeStepPurposeRegisterNew, 2),
		}
	case domain.BusinessActionMFAPasskeySwitch:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyAssertion, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyRegistration, domain.ChallengeStepPurposeRegisterNew, 2),
		}
	case domain.BusinessActionMFARecoveryVerify, domain.BusinessActionMFARecoveryCodesRegenerate:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypePasswordVerification, domain.ChallengeStepPurposeDefault, 1),
			s.buildStep(domain.ChallengeTypeRecoveryCodeVerification, domain.ChallengeStepPurposeRecoveryVerify, 2),
		}
	case domain.BusinessActionMFAOTPDelete:
		// Deleting a bound security factor must not downgrade to password/email fallback.
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
		}
	case domain.BusinessActionMFAPasskeyDelete:
		// Deleting a bound security factor must not downgrade to password/email fallback.
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyAssertion, domain.ChallengeStepPurposeVerifyOld, 1),
		}
	case domain.BusinessActionNotificationDeliveryContentView:
		// Diagnostic content may include sensitive data or an ephemeral secret.
		// Do not make email possession sufficient to reveal it.
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyAssertion, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
		}
	case domain.BusinessActionRBACAssignUserRoles,
		domain.BusinessActionRBACAssignRolePermissions,
		domain.BusinessActionRBACAssignRoleMenus,
		domain.BusinessActionRBACAssignRoleDepts,
		domain.BusinessActionRBACAssignMenuPermissions,
		domain.BusinessActionRBACAssignPostRoles,
		domain.BusinessActionRBACCommitRoleGrants,
		domain.BusinessActionRBACGrantTempPermission,
		domain.BusinessActionRBACRevokeTempPermission,
		domain.BusinessActionRBACExtendTempPermission,
		domain.BusinessActionConfigSensitiveReveal,
		domain.BusinessActionConfigApplyPending,
		domain.BusinessActionConfigRollback,
		domain.BusinessActionConfigScopeAssign,
		domain.BusinessActionNotificationGroupRouteSave,
		domain.BusinessActionNotificationGroupRouteEnable,
		domain.BusinessActionNotificationGroupRouteDisable,
		domain.BusinessActionNotificationGroupRouteRetire,
		domain.BusinessActionNotificationGroupRouteDelete,
		domain.BusinessActionAdminResetPassword,
		domain.BusinessActionCurrentUserPasswordChange,
		domain.BusinessActionAdminForceLogout,
		domain.BusinessActionAdminDeleteUser,
		domain.BusinessActionAdminChangeUserStatus,
		domain.BusinessActionSSOClientCreate,
		domain.BusinessActionSSOClientUpdate,
		domain.BusinessActionSSOClientStatusChange,
		domain.BusinessActionSSOClientRedirectEdit,
		domain.BusinessActionSSOClientSecretGenerate,
		domain.BusinessActionSSOClientSecretDisable,
		domain.BusinessActionExternalLoginProviderCreate,
		domain.BusinessActionExternalLoginProviderUpdate,
		domain.BusinessActionExternalLoginProviderStatusChange,
		domain.BusinessActionExternalLoginProviderSecretRotate,
		domain.BusinessActionExternalLoginIdentityStatusChange,
		domain.BusinessActionExternalOAuthTokenRevoke,
		domain.BusinessActionEnterpriseConnectionSave,
		domain.BusinessActionEnterpriseConnectionStatus,
		domain.BusinessActionEnterpriseIdentityLoginAccess,
		domain.BusinessActionEnterpriseIdentityUnbind,
		domain.BusinessActionWorkforceIndependentReviewDecision,
		domain.BusinessActionPlatformCreate,
		domain.BusinessActionPlatformUpdate,
		domain.BusinessActionPlatformStatusChange,
		domain.BusinessActionPlatformLoginMethodsReplace,
		domain.BusinessActionPlatformSourceRulesReplace,
		domain.BusinessActionPlatformDefaultRolesReplace:
		return []domain.ChallengeStep{
			s.buildStep(domain.ChallengeTypeWebAuthnPasskeyAssertion, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
			s.buildStep(domain.ChallengeTypeEmailOneTimePassword, domain.ChallengeStepPurposeVerifyOld, 1),
		}
	default:
		return nil
	}
}

func (s *ChallengeService) buildLoginStep(expected []string) domain.ChallengeStep {
	for _, item := range expected {
		switch domain.ChallengeType(strings.TrimSpace(item)) {
		case domain.ChallengeTypeTimeBasedOneTimePassword:
			return s.buildStep(domain.ChallengeTypeTimeBasedOneTimePassword, domain.ChallengeStepPurposeDefault, 1)
		case domain.ChallengeTypeWebAuthnPasskeyAssertion:
			return s.buildStep(domain.ChallengeTypeWebAuthnPasskeyAssertion, domain.ChallengeStepPurposeDefault, 1)
		case domain.ChallengeTypeImageCaptcha:
			return s.buildStep(domain.ChallengeTypeImageCaptcha, domain.ChallengeStepPurposeDefault, 1)
		}
	}
	return s.buildStep(domain.ChallengeTypeImageCaptcha, domain.ChallengeStepPurposeDefault, 1)
}

func (s *ChallengeService) buildRegisterStep(expected []string) domain.ChallengeStep {
	for _, item := range expected {
		switch domain.ChallengeType(strings.TrimSpace(item)) {
		case domain.ChallengeTypeEmailOneTimePassword:
			return s.buildStep(domain.ChallengeTypeEmailOneTimePassword, domain.ChallengeStepPurposeVerifyNew, 1)
		case domain.ChallengeTypeImageCaptcha:
			return s.buildStep(domain.ChallengeTypeImageCaptcha, domain.ChallengeStepPurposeDefault, 1)
		}
	}
	return s.buildStep(domain.ChallengeTypeImageCaptcha, domain.ChallengeStepPurposeDefault, 1)
}

func (s *ChallengeService) buildStep(challengeType domain.ChallengeType, purpose domain.ChallengeStepPurpose, slotNumber int) domain.ChallengeStep {
	maxAttempts, cooldown := s.challengeLimits(challengeType)
	state := domain.ChallengeStepStatePending
	if slotNumber == 1 {
		state = domain.ChallengeStepStateInProgress
	}
	return domain.ChallengeStep{
		StepIdentifier:     fmt.Sprintf("step-%s-%s", strings.ToLower(string(challengeType)), uuid.NewString()[:8]),
		ChallengeType:      challengeType,
		StepPurpose:        string(purpose),
		SlotNumber:         slotNumber,
		AttemptNumber:      0,
		MaxAttempts:        maxAttempts,
		CooldownSeconds:    cooldown,
		StepState:          state,
		UserInterfaceHints: make(map[string]any),
	}
}

func (s *ChallengeService) challengeLimits(challengeType domain.ChallengeType) (int, int) {
	switch challengeType {
	case domain.ChallengeTypeImageCaptcha:
		return defaultPositive(s.config.ImageMaxAttempts, 5), defaultPositive(s.config.ImageCooldownSeconds, 10)
	case domain.ChallengeTypePasswordVerification:
		return defaultPositive(s.config.PasswordMaxAttempts, 5), defaultPositive(s.config.PasswordCooldownSeconds, 10)
	case domain.ChallengeTypeEmailOneTimePassword:
		return defaultPositive(s.config.EmailMaxAttempts, 5), defaultPositive(s.config.EmailCooldownSeconds, 60)
	case domain.ChallengeTypeTimeBasedOneTimePassword:
		return defaultPositive(s.config.OTPMaxAttempts, 5), defaultPositive(s.config.OTPCooldownSeconds, 30)
	case domain.ChallengeTypeRecoveryCodeVerification:
		return defaultPositive(s.config.RecoveryMaxAttempts, 5), defaultPositive(s.config.RecoveryCooldownSeconds, 30)
	default:
		return defaultPositive(s.config.PasswordMaxAttempts, 5), 0
	}
}

func (s *ChallengeService) checkThrottle(ctx context.Context, keys []domain.ChallengeThrottleKey) (*domain.ChallengeThrottleDecision, error) {
	if s == nil || s.throttles == nil || len(keys) == 0 {
		return nil, nil
	}
	return s.throttles.CheckLocked(ctx, keys)
}

func (s *ChallengeService) recordThrottleFailure(ctx context.Context, keys []domain.ChallengeThrottleKey, step *domain.ChallengeStep) (*domain.ChallengeThrottleDecision, error) {
	if s == nil || s.throttles == nil || step == nil || len(keys) == 0 {
		return nil, nil
	}
	maxFailures := defaultPositive(s.config.ThrottleMaxFailures, step.MaxAttempts)
	windowTTL := time.Duration(defaultPositive(s.config.ThrottleWindowSeconds, s.config.SessionTTLSeconds)) * time.Second
	lockTTL := time.Duration(defaultPositive(s.config.ThrottleLockSeconds, s.config.SessionTTLSeconds)) * time.Second
	return s.throttles.RecordFailure(ctx, keys, maxFailures, windowTTL, lockTTL)
}

func (s *ChallengeService) clearThrottleFailures(ctx context.Context, keys []domain.ChallengeThrottleKey) error {
	if s == nil || s.throttles == nil || len(keys) == 0 {
		return nil
	}
	return s.throttles.ClearFailures(ctx, keys)
}

func (s *ChallengeService) ensureStepTriggerAllowed(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if s == nil || s.config.TriggerMaxAttempts <= 0 {
		return nil
	}
	keys := challengeTriggerThrottleKeys(session, step)
	decision, err := s.checkThrottle(ctx, keys)
	if err != nil {
		return err
	}
	if decision != nil && decision.Locked {
		return challengeThrottledError(decision)
	}
	return nil
}

func (s *ChallengeService) recordStepTrigger(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if session != nil && step != nil && step.ChallengeType == domain.ChallengeTypeEmailOneTimePassword {
		session.EnsureSessionContext()[emailOTPTriggerContextKey(step)] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if s == nil || s.config.TriggerMaxAttempts <= 0 {
		return nil
	}
	keys := challengeTriggerThrottleKeys(session, step)
	if len(keys) == 0 {
		return nil
	}
	_, err := s.recordThrottleEvent(ctx, keys)
	return err
}

func (s *ChallengeService) ensureRefreshThrottleContext(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if session == nil || step == nil || step.ChallengeType != domain.ChallengeTypeEmailOneTimePassword {
		return nil
	}
	if strings.TrimSpace(stringValue(session.EnsureSessionContext()["email.target"])) != "" {
		return nil
	}
	eligible, err := s.steps.IsStepEligible(ctx, session, step)
	if err != nil {
		return err
	}
	if !eligible || strings.TrimSpace(stringValue(session.EnsureSessionContext()["email.target"])) == "" {
		return apperrors.Operation("当前步骤不可刷新")
	}
	return nil
}

func (s *ChallengeService) recordThrottleEvent(ctx context.Context, keys []domain.ChallengeThrottleKey) (*domain.ChallengeThrottleDecision, error) {
	if s == nil || s.throttles == nil || len(keys) == 0 {
		return nil, nil
	}
	maxAttempts := s.config.TriggerMaxAttempts
	if maxAttempts <= 0 {
		return nil, nil
	}
	windowTTL := time.Duration(defaultPositive(s.config.ThrottleWindowSeconds, s.config.SessionTTLSeconds)) * time.Second
	lockTTL := time.Duration(defaultPositive(s.config.ThrottleLockSeconds, s.config.SessionTTLSeconds)) * time.Second
	return s.throttles.RecordFailure(ctx, keys, maxAttempts, windowTTL, lockTTL)
}

func challengeThrottleKeys(session *domain.ChallengeSession, step *domain.ChallengeStep) []domain.ChallengeThrottleKey {
	if session == nil || step == nil {
		return nil
	}
	action := strings.TrimSpace(session.BusinessAction)
	factor := strings.TrimSpace(string(step.ChallengeType))
	if action == "" || factor == "" {
		return nil
	}
	keys := make([]domain.ChallengeThrottleKey, 0, 5)
	if subject := strings.TrimSpace(session.SubjectIdentifier); subject != "" {
		keys = append(keys, domain.ChallengeThrottleKey{
			Dimension: "subject-action-factor",
			Value:     subject + "|" + action + "|" + factor,
		})
	}
	ipAddress, deviceIdentifier := riskIdentity(session)
	if ipAddress != "" {
		keys = append(keys, domain.ChallengeThrottleKey{
			Dimension: "ip-action-factor",
			Value:     ipAddress + "|" + action + "|" + factor,
		})
	}
	if deviceIdentifier != "" {
		keys = append(keys, domain.ChallengeThrottleKey{
			Dimension: "device-action-factor",
			Value:     deviceIdentifier + "|" + action + "|" + factor,
		})
	}
	if ipAddress != "" && deviceIdentifier != "" {
		keys = append(keys, domain.ChallengeThrottleKey{
			Dimension: "ip-device-action-factor",
			Value:     ipAddress + "|" + deviceIdentifier + "|" + action + "|" + factor,
		})
	}
	if step.ChallengeType == domain.ChallengeTypeEmailOneTimePassword {
		if target := emailThrottleTarget(session); target != "" {
			keys = append(keys, domain.ChallengeThrottleKey{
				Dimension: "email-target-action-factor",
				Value:     target + "|" + action + "|" + factor,
			})
		}
	}
	return keys
}

func challengeTriggerThrottleKeys(session *domain.ChallengeSession, step *domain.ChallengeStep) []domain.ChallengeThrottleKey {
	keys := challengeThrottleKeys(session, step)
	if len(keys) == 0 {
		return nil
	}
	for i := range keys {
		keys[i].Dimension = "trigger-" + keys[i].Dimension
	}
	return keys
}

func riskIdentity(session *domain.ChallengeSession) (string, string) {
	if session == nil || session.SessionContext == nil {
		return "", ""
	}
	raw, ok := session.SessionContext["riskContext"].(map[string]any)
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(stringValue(raw["ipAddress"])), strings.TrimSpace(stringValue(raw["deviceIdentifier"]))
}

func emailThrottleTarget(session *domain.ChallengeSession) string {
	if session == nil {
		return ""
	}
	if value := strings.TrimSpace(stringValue(session.EnsureSessionContext()["email.target"])); value != "" {
		return value
	}
	return strings.TrimSpace(session.SubjectIdentifier)
}

func throttledResponse(session *domain.ChallengeSession, step *domain.ChallengeStep, decision *domain.ChallengeThrottleDecision) *facade.RespondChallengeResponse {
	cooldown := 1
	if decision != nil && decision.RemainingSeconds > 0 {
		cooldown = decision.RemainingSeconds
	}
	return &facade.RespondChallengeResponse{
		ChallengeState:            string(session.ChallengeState),
		NextStepIdentifier:        session.RecommendedStepIdentifier,
		RemainingAttemptCount:     step.RemainingAttemptCount(),
		CooldownSeconds:           cooldown,
		CanSwitchMethod:           session.IsSwitchable(step),
		RecommendedStepIdentifier: session.RecommendedStepIdentifier,
		FailureReason:             "CHALLENGE_THROTTLED",
	}
}

func challengeThrottledError(decision *domain.ChallengeThrottleDecision) error {
	cooldown := 1
	dimension := ""
	if decision != nil {
		if decision.RemainingSeconds > 0 {
			cooldown = decision.RemainingSeconds
		}
		dimension = strings.TrimSpace(decision.Dimension)
	}
	return apperrors.ChallengeThrottled("挑战触发过于频繁，请稍后再试").WithDetails(map[string]any{
		"cooldownSeconds": cooldown,
		"dimension":       dimension,
	})
}

func toStartChallengeResponse(session *domain.ChallengeSession, now time.Time) facade.StartChallengeResponse {
	steps := make([]facade.ChallengeStepVO, 0, len(session.Steps))
	challengeTypes := make([]string, 0, len(session.Steps))
	for i := range session.Steps {
		step := &session.Steps[i]
		hints := cloneStringAnyMap(step.UserInterfaceHints)
		if cooldown := emailOTPResendCooldownSeconds(session, step, now); cooldown > 0 {
			hints["resendCooldownSeconds"] = cooldown
		}
		steps = append(steps, facade.ChallengeStepVO{
			StepIdentifier:        step.StepIdentifier,
			ChallengeType:         string(step.ChallengeType),
			StepPurpose:           step.StepPurpose,
			StepState:             string(step.StepState),
			RemainingAttemptCount: step.RemainingAttemptCount(),
			CooldownSeconds:       step.RemainingCooldownSeconds(now),
			Switchable:            session.IsSwitchable(step),
			UserInterfaceHints:    hints,
		})
		challengeTypes = appendIfMissing(challengeTypes, string(step.ChallengeType))
	}
	return facade.StartChallengeResponse{
		ChallengeIdentifier:        session.ChallengeIdentifier,
		ExpiresAt:                  session.ExpiresAt,
		Steps:                      steps,
		ChallengeState:             string(session.ChallengeState),
		EffectiveTimeToLiveSeconds: session.EffectiveTTLSeconds,
		RequiredAssuranceLevel:     strings.TrimSpace(stringValue(session.EnsureSessionContext()["requiredAssuranceLevel"])),
		ResolvedAssuranceLevel:     strings.TrimSpace(stringValue(session.EnsureSessionContext()["resolvedAssuranceLevel"])),
		RecommendedStepIdentifier:  session.RecommendedStepIdentifier,
		ActualChallengeTypeNames:   challengeTypes,
	}
}

func emailOTPTriggerContextKey(step *domain.ChallengeStep) string {
	if step == nil {
		return ""
	}
	return "email.otp.lastTriggeredAt." + strings.TrimSpace(step.StepIdentifier)
}

func emailOTPResendCooldownSeconds(session *domain.ChallengeSession, step *domain.ChallengeStep, now time.Time) int {
	if session == nil || step == nil || step.ChallengeType != domain.ChallengeTypeEmailOneTimePassword || step.CooldownSeconds <= 0 {
		return 0
	}
	raw := strings.TrimSpace(stringValue(session.EnsureSessionContext()[emailOTPTriggerContextKey(step)]))
	triggeredAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return 0
	}
	remaining := triggeredAt.Add(time.Duration(step.CooldownSeconds) * time.Second).Sub(now.UTC())
	if remaining <= 0 {
		return 0
	}
	seconds := int(remaining.Seconds())
	if seconds <= 0 {
		return 1
	}
	return seconds
}

func cloneStringAnyMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func appendIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func defaultPositive(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func toDomainSubjectHint(input *facade.SubjectHint) *domain.SubjectHint {
	if input == nil {
		return nil
	}
	return &domain.SubjectHint{
		SubjectType:  strings.TrimSpace(input.SubjectType),
		SubjectValue: strings.TrimSpace(input.SubjectValue),
	}
}

func timePointer(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}

func challengeActionPolicy(action string) (domain.ChallengeActionPolicy, error) {
	policy, ok := domain.LookupChallengeActionPolicy(action)
	if !ok {
		return domain.ChallengeActionPolicy{}, apperrors.Params("未知或不支持的业务动作")
	}
	return policy, nil
}

func operationBindingFromExtension(extensionContext map[string]any) string {
	if extensionContext == nil {
		return ""
	}
	return strings.TrimSpace(stringValue(extensionContext["operationBinding"]))
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}
