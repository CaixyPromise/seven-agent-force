package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/google/uuid"
)

const challengeAudience = "challenge-app"

type MfaManagementService struct {
	credentials       credentialfacade.UserCredentialFacade
	subjects          *challengeinfra.SubjectResolver
	internal          facade.ChallengeInternalFacade
	verifier          facade.ProofTokenVerifier
	recoveryBatchSize int
	sessionTTLSeconds int
}

func NewMfaManagementService(
	credentials credentialfacade.UserCredentialFacade,
	subjects userfacade.SubjectFacade,
	internal facade.ChallengeInternalFacade,
	verifier facade.ProofTokenVerifier,
	recoveryBatchSize int,
	sessionTTLSeconds int,
) *MfaManagementService {
	return &MfaManagementService{
		credentials:       credentials,
		subjects:          challengeinfra.NewSubjectResolver(subjects),
		internal:          internal,
		verifier:          verifier,
		recoveryBatchSize: recoveryBatchSize,
		sessionTTLSeconds: sessionTTLSeconds,
	}
}

func (s *MfaManagementService) QueryMfaStatus(ctx context.Context, request facade.MfaStatusRequest) (*facade.MfaStatusResponse, error) {
	subject, err := s.resolveSubject(ctx, strings.TrimSpace(request.SubjectIdentifier))
	if err != nil {
		return nil, err
	}
	response := &facade.MfaStatusResponse{SubjectIdentifier: request.SubjectIdentifier}
	if subject == nil || subject.UserID <= 0 {
		return response, nil
	}
	if record, err := s.credentials.FindActiveTotpByUserID(ctx, subject.UserID); err != nil {
		return nil, err
	} else if record != nil {
		response.OTPBound = true
	}
	if items, err := s.credentials.ListActivePasskeys(ctx, subject.UserID); err != nil {
		return nil, err
	} else if len(items) > 0 {
		response.PasskeyBound = true
	}
	count, err := s.credentials.CountAvailableRecoveryCodes(ctx, subject.UserID)
	if err != nil {
		return nil, err
	}
	response.AvailableRecoveryCodeCount = count
	return response, nil
}

func (s *MfaManagementService) QueryMfaStatusByUserID(ctx context.Context, userID int64) (*facade.MfaStatusResponse, error) {
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	response := &facade.MfaStatusResponse{SubjectIdentifier: buildCurrentSubjectIdentifier(userID)}
	if record, err := s.credentials.FindActiveTotpByUserID(ctx, userID); err != nil {
		return nil, err
	} else if record != nil {
		response.OTPBound = true
	}
	if items, err := s.credentials.ListActivePasskeys(ctx, userID); err != nil {
		return nil, err
	} else if len(items) > 0 {
		response.PasskeyBound = true
	}
	count, err := s.credentials.CountAvailableRecoveryCodes(ctx, userID)
	if err != nil {
		return nil, err
	}
	response.AvailableRecoveryCodeCount = count
	return response, nil
}

func (s *MfaManagementService) RegenerateRecoveryCodes(ctx context.Context, request facade.RegenerateRecoveryCodeRequest) (*facade.RegenerateRecoveryCodeResponse, error) {
	subject, err := s.resolveRequiredSubject(ctx, strings.TrimSpace(request.SubjectIdentifier))
	if err != nil {
		return nil, err
	}
	result, err := s.credentials.RegenerateRecoveryCodes(ctx, subject.UserID, s.requiredRecoveryBatchSize())
	if err != nil {
		return nil, err
	}
	return &facade.RegenerateRecoveryCodeResponse{
		SubjectIdentifier: request.SubjectIdentifier,
		BatchIdentifier:   result.BatchIdentifier,
		RecoveryCodes:     result.PlainCodes,
		GeneratedAt:       result.GeneratedAt,
	}, nil
}

func (s *MfaManagementService) RegenerateRecoveryCodesByUserID(ctx context.Context, userID int64, proof stepup.ProofMetadata) (*facade.RegenerateRecoveryCodeResponse, error) {
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	if err := requireMfaStepUpProof(proof, domain.BusinessActionMFARecoveryCodesRegenerate, ""); err != nil {
		return nil, err
	}
	result, err := s.credentials.RegenerateRecoveryCodes(ctx, userID, s.requiredRecoveryBatchSize())
	if err != nil {
		return nil, err
	}
	return &facade.RegenerateRecoveryCodeResponse{
		SubjectIdentifier: buildCurrentSubjectIdentifier(userID),
		BatchIdentifier:   result.BatchIdentifier,
		RecoveryCodes:     result.PlainCodes,
		GeneratedAt:       result.GeneratedAt,
	}, nil
}

func (s *MfaManagementService) RegenerateRecoveryCodesWithChallenge(ctx context.Context, request facade.RegenerateRecoveryCodeRequest, context facade.MfaProtectedOperationContext) (*facade.RegenerateRecoveryCodeResponse, error) {
	if err := s.validateProtectedContext(context); err != nil {
		return nil, err
	}
	request.SubjectIdentifier = context.SubjectIdentifier
	flowNonce := chooseFlowNonce(context.FlowNonce)
	if ok, err := s.verifyProtectedOperation(ctx, context, flowNonce, "MFA_RECOVERY_CODES_REGENERATE", ""); err != nil {
		return nil, err
	} else if ok {
		return s.RegenerateRecoveryCodes(ctx, request)
	}
	challenge, err := s.startProtectedChallenge(ctx, context.SubjectIdentifier, flowNonce, "MFA_RECOVERY_CODES_REGENERATE", nil)
	if err != nil {
		return nil, err
	}
	return nil, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
	})
}

func (s *MfaManagementService) DeleteOtpBinding(ctx context.Context, request facade.MfaDeleteOtpBindingRequest) (bool, error) {
	subject, err := s.resolveRequiredSubject(ctx, strings.TrimSpace(request.SubjectIdentifier))
	if err != nil {
		return false, err
	}
	return s.credentials.DisableTotpCredential(ctx, subject.UserID)
}

func (s *MfaManagementService) DeleteOtpBindingByUserID(ctx context.Context, userID int64, proof stepup.ProofMetadata) (bool, error) {
	if userID <= 0 {
		return false, apperrors.Unauthorized("未登录或登录信息失效")
	}
	if err := requireMfaStepUpProof(proof, domain.BusinessActionMFAOTPDelete, ""); err != nil {
		return false, err
	}
	return s.credentials.DisableTotpCredential(ctx, userID)
}

func (s *MfaManagementService) DeleteOtpBindingWithChallenge(ctx context.Context, request facade.MfaDeleteOtpBindingRequest, context facade.MfaProtectedOperationContext) (bool, error) {
	if err := s.validateProtectedContext(context); err != nil {
		return false, err
	}
	request.SubjectIdentifier = context.SubjectIdentifier
	flowNonce := chooseFlowNonce(context.FlowNonce)
	if ok, err := s.verifyProtectedOperation(ctx, context, flowNonce, "MFA_OTP_DELETE", ""); err != nil {
		return false, err
	} else if ok {
		return s.DeleteOtpBinding(ctx, request)
	}
	challenge, err := s.startProtectedChallenge(ctx, context.SubjectIdentifier, flowNonce, "MFA_OTP_DELETE", nil)
	if err != nil {
		return false, err
	}
	return false, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
	})
}

func (s *MfaManagementService) ListPasskeys(ctx context.Context, request facade.MfaPasskeyListRequest) ([]facade.MfaPasskeyVO, error) {
	subject, err := s.resolveRequiredSubject(ctx, strings.TrimSpace(request.SubjectIdentifier))
	if err != nil {
		return nil, err
	}
	return s.listPasskeys(ctx, subject.UserID, request.SubjectIdentifier)
}

func (s *MfaManagementService) ListPasskeysByUserID(ctx context.Context, userID int64) ([]facade.MfaPasskeyVO, error) {
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return s.listPasskeys(ctx, userID, buildCurrentSubjectIdentifier(userID))
}

func (s *MfaManagementService) listPasskeys(ctx context.Context, userID int64, _ string) ([]facade.MfaPasskeyVO, error) {
	items, err := s.credentials.ListActivePasskeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]facade.MfaPasskeyVO, 0, len(items))
	for _, item := range items {
		result = append(result, facade.MfaPasskeyVO{
			CredentialIdentifier: item.CredentialKey,
			DisplayName:          item.DisplayName,
			AAGUID:               item.AAGUID,
			Transports:           item.Transports,
			CreatedAt:            item.CreateTime,
			LastUsedAt:           item.LastUsedAt,
		})
	}
	return result, nil
}

func (s *MfaManagementService) DeletePasskey(ctx context.Context, request facade.MfaDeletePasskeyRequest) (bool, error) {
	subject, err := s.resolveRequiredSubject(ctx, strings.TrimSpace(request.SubjectIdentifier))
	if err != nil {
		return false, err
	}
	return s.credentials.DisablePasskeyCredential(ctx, subject.UserID, request.CredentialIdentifier)
}

func (s *MfaManagementService) DeletePasskeyByUserID(ctx context.Context, userID int64, credentialIdentifier string, proof stepup.ProofMetadata) (bool, error) {
	if userID <= 0 {
		return false, apperrors.Unauthorized("未登录或登录信息失效")
	}
	operationBinding := buildPasskeyOperationBinding(strings.TrimSpace(credentialIdentifier))
	if err := requireMfaStepUpProof(proof, domain.BusinessActionMFAPasskeyDelete, operationBinding); err != nil {
		return false, err
	}
	return s.credentials.DisablePasskeyCredential(ctx, userID, credentialIdentifier)
}

func (s *MfaManagementService) DeletePasskeyWithChallenge(ctx context.Context, request facade.MfaDeletePasskeyRequest, context facade.MfaProtectedOperationContext) (bool, error) {
	if err := s.validateProtectedContext(context); err != nil {
		return false, err
	}
	request.SubjectIdentifier = context.SubjectIdentifier
	operationBinding := buildPasskeyOperationBinding(strings.TrimSpace(request.CredentialIdentifier))
	flowNonce := chooseFlowNonce(context.FlowNonce)
	if ok, err := s.verifyProtectedOperation(ctx, context, flowNonce, "MFA_PASSKEY_DELETE", operationBinding); err != nil {
		return false, err
	} else if ok {
		return s.DeletePasskey(ctx, request)
	}
	challenge, err := s.startProtectedChallenge(ctx, context.SubjectIdentifier, flowNonce, "MFA_PASSKEY_DELETE", map[string]any{
		"operationBinding": operationBinding,
	})
	if err != nil {
		return false, err
	}
	return false, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
		"operationBinding":           operationBinding,
	})
}

func (s *MfaManagementService) StartMfaChallenge(ctx context.Context, request facade.MfaChallengeStartRequest, context facade.MfaChallengeStartContext) (*facade.StartChallengeResponse, error) {
	if err := s.validateProtectedContext(facade.MfaProtectedOperationContext{
		SubjectIdentifier: context.SubjectIdentifier,
		IPAddress:         context.IPAddress,
		UserAgent:         context.UserAgent,
		DeviceIdentifier:  context.DeviceIdentifier,
		TenantIdentifier:  context.TenantIdentifier,
	}); err != nil {
		return nil, err
	}
	return s.startProtectedChallenge(ctx, context.SubjectIdentifier, chooseFlowNonce(request.FlowNonce), request.BusinessAction, request.ExtensionContext)
}

func (s *MfaManagementService) StartMfaChallengeByUserID(ctx context.Context, userID int64, request facade.MfaChallengeStartRequest, context facade.MfaChallengeStartContext) (*facade.StartChallengeResponse, error) {
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return s.StartMfaChallenge(ctx, request, facade.MfaChallengeStartContext{
		SubjectIdentifier: buildCurrentSubjectIdentifier(userID),
		IPAddress:         context.IPAddress,
		UserAgent:         context.UserAgent,
		DeviceIdentifier:  context.DeviceIdentifier,
		TenantIdentifier:  context.TenantIdentifier,
	})
}

func (s *MfaManagementService) startProtectedChallenge(ctx context.Context, subjectIdentifier, flowNonce, businessAction string, extensionContext map[string]any) (*facade.StartChallengeResponse, error) {
	if s.internal == nil {
		return nil, fmt.Errorf("challenge internal facade is not configured")
	}
	request := facade.StartChallengeRequest{
		IssuingServiceName:         challengeAudience,
		AudienceServiceNames:       []string{challengeAudience},
		BusinessAction:             strings.TrimSpace(businessAction),
		SubjectIdentifier:          strings.TrimSpace(subjectIdentifier),
		SubjectHint:                &facade.SubjectHint{SubjectType: "USER", SubjectValue: strings.TrimSpace(subjectIdentifier)},
		FlowNonce:                  flowNonce,
		RequestedTimeToLiveSeconds: s.sessionTTL(),
		IdempotencyKey:             buildChallengeIdempotencyKey(businessAction, subjectIdentifier, flowNonce),
		ExtensionContext:           cloneMap(extensionContext),
	}
	return s.internal.StartChallenge(ctx, request)
}

func (s *MfaManagementService) verifyProtectedOperation(ctx context.Context, context facade.MfaProtectedOperationContext, flowNonce, businessAction, operationBinding string) (bool, error) {
	if strings.TrimSpace(context.ProofToken) == "" {
		return false, nil
	}
	if s.verifier == nil {
		return false, fmt.Errorf("challenge proof token verifier is not configured")
	}
	_, err := s.verifier.VerifyProofToken(ctx, facade.ProofTokenVerifyRequest{
		ProofToken:          context.ProofToken,
		AudienceServiceName: challengeAudience,
		BusinessAction:      businessAction,
		FlowNonce:           flowNonce,
		SubjectIdentifier:   context.SubjectIdentifier,
		OperationBinding:    operationBinding,
		ConsumeOnce:         true,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *MfaManagementService) resolveSubject(ctx context.Context, subjectIdentifier string) (*challengeinfra.ResolvedSubject, error) {
	if strings.TrimSpace(subjectIdentifier) == "" || s.subjects == nil {
		return nil, nil
	}
	return s.subjects.Resolve(ctx, &domain.ChallengeSession{SubjectIdentifier: subjectIdentifier})
}

func (s *MfaManagementService) resolveRequiredSubject(ctx context.Context, subjectIdentifier string) (*challengeinfra.ResolvedSubject, error) {
	subject, err := s.resolveSubject(ctx, subjectIdentifier)
	if err != nil {
		return nil, err
	}
	if subject == nil || subject.UserID <= 0 {
		return nil, apperrors.Params("subjectIdentifier不能为空或主体不存在")
	}
	return subject, nil
}

func (s *MfaManagementService) validateProtectedContext(context facade.MfaProtectedOperationContext) error {
	if strings.TrimSpace(context.SubjectIdentifier) == "" {
		return apperrors.Params("subjectIdentifier不能为空")
	}
	return nil
}

func (s *MfaManagementService) requiredRecoveryBatchSize() int {
	if s.recoveryBatchSize <= 0 {
		return 10
	}
	return s.recoveryBatchSize
}

func (s *MfaManagementService) sessionTTL() int {
	if s.sessionTTLSeconds <= 0 {
		return 300
	}
	return s.sessionTTLSeconds
}

func requireMfaStepUpProof(proof stepup.ProofMetadata, action domain.BusinessAction, operationBinding string) error {
	policy, ok := domain.LookupChallengeActionPolicy(string(action))
	if !ok {
		return apperrors.System("MFA step-up策略未配置")
	}
	normalized := proof.Normalized()
	if normalized.ProofIdentifier == "" {
		return apperrors.Forbidden("缺少step-up proof元数据")
	}
	if normalized.ChallengeIdentifier == "" {
		return apperrors.Forbidden("缺少step-up challenge元数据")
	}
	if strings.TrimSpace(policy.MinimumAssuranceLevel) != "" && normalized.AssuranceLevel != strings.ToUpper(strings.TrimSpace(policy.MinimumAssuranceLevel)) {
		return apperrors.Forbidden("step-up proof保证等级不足")
	}
	if normalized.BusinessAction != string(action) {
		return apperrors.Forbidden("step-up proof业务动作不匹配")
	}
	expectedBinding := strings.TrimSpace(operationBinding)
	if policy.RequiresOperationBinding() && expectedBinding == "" {
		return apperrors.System("MFA step-up操作绑定未配置")
	}
	if normalized.OperationBinding != expectedBinding {
		return apperrors.Forbidden("step-up proof操作绑定不匹配")
	}
	if !policy.ProofMethodsSatisfied(normalized.AuthenticationMethods) {
		return apperrors.Forbidden("step-up proof认证方式不足")
	}
	return nil
}

func chooseFlowNonce(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return uuid.NewString()
}

func buildChallengeIdempotencyKey(action, subjectIdentifier, flowNonce string) string {
	return strings.TrimSpace(action) + "|" + strings.TrimSpace(subjectIdentifier) + "|" + strings.TrimSpace(flowNonce)
}

func buildPasskeyOperationBinding(credentialIdentifier string) string {
	return "passkey:" + credentialIdentifier
}

func buildCurrentSubjectIdentifier(userID int64) string {
	return fmt.Sprintf("user:%d", userID)
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
