package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
)

type MfaManagementHandler struct {
	facade challengefacade.MfaManagementFacade
	auth   authorizationfacade.AuthFacade
}

func NewMfaManagementHandler(facade challengefacade.MfaManagementFacade, auth authorizationfacade.AuthFacade) *MfaManagementHandler {
	return &MfaManagementHandler{facade: facade, auth: auth}
}

func (c *MfaManagementHandler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c == nil {
		return
	}
	c.auth = auth
}

func (c *MfaManagementHandler) QueryStatusInternal(ctx context.Context, reqCtx *app.RequestContext) {
	if err := requireInternalRequest(reqCtx); err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request challengefacade.MfaStatusRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.QueryMfaStatus(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *MfaManagementHandler) QueryStatusForCurrentUser(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := currentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.QueryMfaStatusByUserID(ctx, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *MfaManagementHandler) RegenerateRecoveryCodesInternal(ctx context.Context, reqCtx *app.RequestContext) {
	if err := requireInternalRequest(reqCtx); err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request challengefacade.RegenerateRecoveryCodeRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.RegenerateRecoveryCodes(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *MfaManagementHandler) RegenerateRecoveryCodesForCurrentUser(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := currentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedOperation(ctx, reqCtx, string(challengedomain.BusinessActionMFARecoveryCodesRegenerate), "")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.RegenerateRecoveryCodesByUserID(ctx, userID, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *MfaManagementHandler) DeleteCurrentUserOtpBinding(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := currentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedOperation(ctx, reqCtx, string(challengedomain.BusinessActionMFAOTPDelete), "")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.DeleteOtpBindingByUserID(ctx, userID, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *MfaManagementHandler) ListCurrentUserPasskeys(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := currentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.ListPasskeysByUserID(ctx, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *MfaManagementHandler) DeleteCurrentUserPasskey(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := currentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	credentialIdentifier := strings.TrimSpace(string(reqCtx.Param("credentialIdentifier")))
	proof, err := c.ensureProtectedOperation(ctx, reqCtx, string(challengedomain.BusinessActionMFAPasskeyDelete), "passkey:"+credentialIdentifier)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.DeletePasskeyByUserID(ctx, userID, credentialIdentifier, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func requireInternalRequest(reqCtx *app.RequestContext) error {
	internal, _ := reqCtx.Get("__seven_auth_internal__")
	if ok, _ := internal.(bool); !ok {
		return apperrors.Unauthorized("内部服务鉴权失败")
	}
	user := securitycontext.Get(reqCtx)
	if user == nil || !strings.EqualFold(strings.TrimSpace(user.Source), "internal") || !hasRole(user.Roles, "ROLE_INTERNAL") {
		return apperrors.Unauthorized("内部服务鉴权失败")
	}
	return nil
}

func hasRole(roles []string, expected string) bool {
	for _, role := range roles {
		if strings.TrimSpace(role) == expected {
			return true
		}
	}
	return false
}

func (c *MfaManagementHandler) StartMfaChallenge(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := currentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request challengefacade.MfaChallengeStartRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.StartMfaChallengeByUserID(ctx, userID, request, challengefacade.MfaChallengeStartContext{
		SubjectIdentifier: "",
		IPAddress:         reqCtx.ClientIP(),
		UserAgent:         string(reqCtx.UserAgent()),
		DeviceIdentifier:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantIdentifier:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func currentUserID(reqCtx *app.RequestContext) (int64, error) {
	userID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok || userID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return userID, nil
}

func buildRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	userID, err := currentUserID(reqCtx)
	if err != nil {
		return authorizationfacade.RequestScope{}, err
	}
	user := securitycontext.Require(reqCtx)
	return authorizationfacade.RequestScope{
		UserID:    userID,
		Username:  user.Username,
		IPAddress: reqCtx.ClientIP(),
		UserAgent: string(reqCtx.UserAgent()),
		DeviceID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
		SessionID: user.SessionID,
		Source:    user.Source,
	}, nil
}

func (c *MfaManagementHandler) ensureProtectedOperation(ctx context.Context, reqCtx *app.RequestContext, businessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if c.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
	if proofToken != "" {
		token, err := c.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{
			ProofToken:       proofToken,
			BusinessAction:   businessAction,
			FlowNonce:        flowNonce,
			OperationBinding: operationBinding,
			ConsumeOnce:      true,
		})
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		if token == nil {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof验证失败")
		}
		securitycontext.SetStepUpProofAudit(reqCtx, stepUpProofAuditFromToken(token, businessAction, operationBinding))
		return stepUpProofMetadataFromToken(token, businessAction, operationBinding), nil
	}
	challenge, err := c.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
		BusinessAction:   businessAction,
		FlowNonce:        flowNonce,
		OperationBinding: operationBinding,
	})
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	return stepup.ProofMetadata{}, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"requiredAssuranceLevel":     challenge.RequiredAssuranceLevel,
		"resolvedAssuranceLevel":     challenge.ResolvedAssuranceLevel,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
		"actualChallengeTypeNames":   challenge.ActualChallengeTypeNames,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"operationBinding":           operationBinding,
	})
}

func stepUpProofAuditFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) securitycontext.StepUpProofAudit {
	if token == nil {
		return securitycontext.StepUpProofAudit{}
	}
	return securitycontext.StepUpProofAudit{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func stepUpProofMetadataFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) stepup.ProofMetadata {
	if token == nil {
		return stepup.ProofMetadata{}
	}
	return stepup.ProofMetadata{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func firstNonBlank(values ...string) string {
	for _, item := range values {
		if value := strings.TrimSpace(item); value != "" {
			return value
		}
	}
	return ""
}

func chooseFlowNonce(flowNonce, businessAction string) string {
	value := strings.TrimSpace(flowNonce)
	if value == "" {
		return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return value
}
