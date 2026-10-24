package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	externalapp "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
)

type Service interface {
	facade.LoginMethodFacade
	facade.ExternalLoginFlowFacade
	facade.ProviderAdminFacade
	facade.IdentityBindingFacade
	facade.ExternalOAuthTokenFacade
	facade.CapabilityIndexFacade
}

type Handler struct {
	service    Service
	enterprise facade.EnterpriseIdentityFacade
	auth       authorizationfacade.AuthFacade
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) BindEnterpriseIdentity(service facade.EnterpriseIdentityFacade) {
	if h != nil {
		h.enterprise = service
	}
}

func (h *Handler) enterpriseScope(reqCtx *app.RequestContext) (int64, int64, error) {
	if !securitycontext.IsLogin(reqCtx) {
		return 0, 0, apperrors.Unauthorized("未登录")
	}
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(reqCtx))
	if err != nil {
		return 0, 0, apperrors.Forbidden("当前账号缺少明确的组织范围")
	}
	return scope.OrgID, currentUserID(reqCtx), nil
}

func (h *Handler) ListEnterpriseConnections(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, _, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := h.enterprise.ListEnterpriseConnections(ctx, orgID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (h *Handler) GetEnterpriseConnectionSetup(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, _, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	setup, err := h.enterprise.GetEnterpriseConnectionSetup(ctx, orgID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, setup)
}

func (h *Handler) ListEnterpriseIdentities(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, _, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := h.enterprise.ListEnterpriseIdentities(ctx, orgID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (h *Handler) SaveEnterpriseConnection(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, actorID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.SaveEnterpriseConnectionRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := "enterprise-connection:" + strconv.FormatInt(orgID, 10) + ":" + strings.ToUpper(strings.TrimSpace(request.Provider)) + "|version:" + strconv.FormatInt(request.ExpectedVersion, 10)
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionEnterpriseConnectionSave, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := h.enterprise.SaveEnterpriseConnection(ctx, orgID, actorID, request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (h *Handler) SetEnterpriseConnectionStatus(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, actorID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	connectionID, err := int64Path(reqCtx, "connectionId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.EnterpriseConnectionStatusRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := "enterprise-connection:" + strconv.FormatInt(connectionID, 10) + "|enabled:" + strconv.FormatBool(request.Enabled) + "|version:" + strconv.FormatInt(request.ExpectedVersion, 10)
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionEnterpriseConnectionStatus, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.enterprise.SetEnterpriseConnectionStatus(ctx, orgID, actorID, connectionID, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true})
}

func (h *Handler) ListMyEnterpriseIdentities(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, userID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := h.enterprise.ListMyEnterpriseIdentities(ctx, orgID, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (h *Handler) StartMyEnterpriseBinding(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, userID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.enterprise.StartEnterpriseBinding(ctx, orgID, userID, facade.EnterpriseAuthorizationStartRequest{Provider: string(reqCtx.Param("provider")), RedirectAfter: strings.TrimSpace(string(reqCtx.Query("redirectAfter")))})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (h *Handler) StartEnterpriseLogin(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.EnterpriseLoginStartRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.Provider = string(reqCtx.Param("provider"))
	result, err := h.enterprise.StartEnterpriseLogin(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (h *Handler) EnterpriseLoginOptions(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.enterprise.ResolveEnterpriseLoginOptions(ctx, strings.TrimSpace(string(reqCtx.Query("org"))))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (h *Handler) CompleteEnterpriseAuthorization(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.enterprise.CompleteEnterpriseAuthorization(ctx, facade.EnterpriseAuthorizationCallbackRequest{Provider: string(reqCtx.Param("provider")), Code: strings.TrimSpace(string(reqCtx.Query("code"))), State: strings.TrimSpace(string(reqCtx.Query("state"))), Error: strings.TrimSpace(string(reqCtx.Query("error")))})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if result.Authenticated {
		if cookie := strings.TrimSpace(result.SessionCookieHeaderValue); cookie != "" {
			reqCtx.Response.Header.Add("Set-Cookie", cookie)
		}
		if cookie := strings.TrimSpace(result.RefreshCookieHeaderValue); cookie != "" {
			reqCtx.Response.Header.Add("Set-Cookie", cookie)
		}
	}
	response.Success(reqCtx, result)
}

func (h *Handler) UpdateMyEnterpriseNotificationSettings(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, userID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	identityID, err := int64Path(reqCtx, "identityId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.EnterpriseNotificationSettingsRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.enterprise.UpdateMyEnterpriseNotificationSettings(ctx, orgID, userID, identityID, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true})
}

func (h *Handler) SendMyEnterpriseTestMessage(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, userID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	identityID, err := int64Path(reqCtx, "identityId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.enterprise.SendMyEnterpriseTestMessage(ctx, orgID, userID, identityID); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"sent": true})
}

func (h *Handler) UnbindMyEnterpriseIdentity(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, userID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	identityID, err := int64Path(reqCtx, "identityId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.EnterpriseUnbindRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := "enterprise-identity:" + strconv.FormatInt(identityID, 10) + "|unbind|version:" + strconv.FormatInt(request.ExpectedIdentityVersion, 10)
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionEnterpriseIdentityUnbind, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.enterprise.UnbindMyEnterpriseIdentity(ctx, orgID, userID, identityID, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"unbound": true})
}

func (h *Handler) SetEnterpriseLoginAccess(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, actorID, err := h.enterpriseScope(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	identityID, err := int64Path(reqCtx, "identityId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.EnterpriseLoginAccessRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := "enterprise-identity:" + strconv.FormatInt(identityID, 10) + "|login:" + strconv.FormatBool(request.Enabled) + "|version:" + strconv.FormatInt(request.ExpectedIdentityVersion, 10)
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionEnterpriseIdentityLoginAccess, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.enterprise.SetEnterpriseLoginAccess(ctx, orgID, actorID, identityID, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true})
}

func (h *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if h == nil {
		return
	}
	h.auth = auth
}

func (h *Handler) ListLoginMethods(ctx context.Context, reqCtx *app.RequestContext) {
	request := facade.ListLoginMethodsRequest{
		LoginTransactionID: strings.TrimSpace(string(reqCtx.Query("loginTransactionId"))),
		RequestContext:     requestContext(reqCtx),
	}
	items, err := h.service.ListLoginMethods(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (h *Handler) StartExternalLogin(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.StartExternalLogin(ctx, facade.StartExternalLoginRequest{
		ProviderCode:       string(reqCtx.Param("providerCode")),
		LoginTransactionID: strings.TrimSpace(string(reqCtx.Query("loginTransactionId"))),
		LoginContextID:     strings.TrimSpace(string(reqCtx.Query("loginContextId"))),
		PlatformCode:       strings.TrimSpace(string(reqCtx.Query("platformCode"))),
		RedirectAfterLogin: strings.TrimSpace(string(reqCtx.Query("redirectAfterLogin"))),
		RequestContext:     requestContext(reqCtx),
		TrustedSource:      trustedSource(reqCtx),
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	reqCtx.Redirect(302, []byte(result.RedirectURL))
}

func (h *Handler) ListCurrentUserBindings(ctx context.Context, reqCtx *app.RequestContext) {
	if !securitycontext.IsLogin(reqCtx) {
		response.Error(reqCtx, apperrors.Unauthorized("未登录"))
		return
	}
	items, err := h.service.ListCurrentUserBindings(ctx, currentUserID(reqCtx))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (h *Handler) StartCurrentUserBinding(ctx context.Context, reqCtx *app.RequestContext) {
	if !securitycontext.IsLogin(reqCtx) {
		response.Error(reqCtx, apperrors.Unauthorized("未登录"))
		return
	}
	result, err := h.service.StartExternalLogin(ctx, facade.StartExternalLoginRequest{
		ProviderCode:       string(reqCtx.Param("providerCode")),
		RedirectAfterLogin: firstNonBlank(strings.TrimSpace(string(reqCtx.Query("redirectAfterLogin"))), "/account/settings"),
		BindUserID:         currentUserID(reqCtx),
		RequestContext:     requestContext(reqCtx),
		TrustedSource:      trustedSource(reqCtx),
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	reqCtx.Redirect(302, []byte(result.RedirectURL))
}

func (h *Handler) CompleteExternalCallback(ctx context.Context, reqCtx *app.RequestContext) {
	providerCode := strings.TrimSpace(string(reqCtx.Param("providerCode")))
	if queryProvider := strings.TrimSpace(string(reqCtx.Query("providerCode"))); queryProvider != "" && !strings.EqualFold(queryProvider, providerCode) {
		response.Error(reqCtx, apperrors.Params("外部登录provider不匹配"))
		return
	}
	result, err := h.service.CompleteExternalCallback(ctx, facade.CompleteExternalCallbackRequest{
		ProviderCode:   providerCode,
		Code:           strings.TrimSpace(string(reqCtx.Query("code"))),
		State:          strings.TrimSpace(string(reqCtx.Query("state"))),
		Issuer:         strings.TrimSpace(string(reqCtx.Query("iss"))),
		RequestContext: requestContext(reqCtx),
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeLoginCookies(reqCtx, result)
	response.Success(reqCtx, publicLoginResult(result))
}

func (h *Handler) Capabilities(ctx context.Context, reqCtx *app.RequestContext) {
	response.Success(reqCtx, h.service.ProviderCapabilities(ctx))
}

func (h *Handler) ListProviderCapabilities(ctx context.Context, reqCtx *app.RequestContext) {
	response.Success(reqCtx, h.service.ListProviderCapabilities(ctx))
}

func (h *Handler) ListProviderMethods(ctx context.Context, reqCtx *app.RequestContext) {
	items, err := h.service.ListProviderMethods(ctx, string(reqCtx.Param("providerCode")))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (h *Handler) ListProviders(ctx context.Context, reqCtx *app.RequestContext) {
	var query facade.ProviderQuery
	if err := httpx.BindAndValidate(reqCtx, &query); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := h.service.ListProviders(ctx, query)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (h *Handler) GetProvider(ctx context.Context, reqCtx *app.RequestContext) {
	detail, err := h.service.GetProvider(ctx, string(reqCtx.Param("providerCode")))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, detail)
}

func (h *Handler) CreateProvider(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.ProviderSaveRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionExternalLoginProviderCreate, externalapp.BuildProviderCreateOperationBinding(request.ProviderCode))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	detail, err := h.service.CreateProvider(ctx, currentUserID(reqCtx), request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, detail)
}

func (h *Handler) UpdateProvider(ctx context.Context, reqCtx *app.RequestContext) {
	providerCode := string(reqCtx.Param("providerCode"))
	var request facade.ProviderUpdateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionExternalLoginProviderUpdate, externalapp.BuildProviderUpdateOperationBinding(providerCode))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	detail, err := h.service.UpdateProvider(ctx, currentUserID(reqCtx), providerCode, request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, detail)
}

func (h *Handler) UpdateProviderStatus(ctx context.Context, reqCtx *app.RequestContext) {
	providerCode := string(reqCtx.Param("providerCode"))
	var request facade.ProviderStatusRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionExternalLoginProviderStatusChange, externalapp.BuildProviderStatusOperationBinding(providerCode, request.Status))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.UpdateProviderStatus(ctx, currentUserID(reqCtx), providerCode, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true})
}

func (h *Handler) RotateClientSecret(ctx context.Context, reqCtx *app.RequestContext) {
	providerCode := string(reqCtx.Param("providerCode"))
	var request facade.RotateClientSecretRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionExternalLoginProviderSecretRotate, externalapp.BuildProviderSecretRotateOperationBinding(providerCode))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.RotateClientSecret(ctx, currentUserID(reqCtx), providerCode, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"rotated": true})
}

func (h *Handler) ListIdentities(ctx context.Context, reqCtx *app.RequestContext) {
	var query facade.IdentityQuery
	if err := httpx.BindAndValidate(reqCtx, &query); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := h.service.ListIdentities(ctx, query)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (h *Handler) UpdateIdentityStatus(ctx context.Context, reqCtx *app.RequestContext) {
	identityID, err := int64Path(reqCtx, "identityId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.IdentityStatusRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionExternalLoginIdentityStatusChange, externalapp.BuildIdentityStatusOperationBinding(identityID, request.Status))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.UpdateIdentityStatus(ctx, currentUserID(reqCtx), identityID, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true})
}

func (h *Handler) ResolveIdentity(ctx context.Context, reqCtx *app.RequestContext) {
	item, err := h.service.ResolveIdentity(ctx, string(reqCtx.Param("providerCode")), string(reqCtx.Param("externalSubject")))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (h *Handler) ListTokens(ctx context.Context, reqCtx *app.RequestContext) {
	var query facade.TokenQuery
	if err := httpx.BindAndValidate(reqCtx, &query); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := h.service.ListTokens(ctx, query)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (h *Handler) RevokeToken(ctx context.Context, reqCtx *app.RequestContext) {
	tokenID, err := int64Path(reqCtx, "tokenId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request struct {
		Reason string `json:"reason,omitempty"`
	}
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, challengedomain.BusinessActionExternalOAuthTokenRevoke, externalapp.BuildTokenRevokeOperationBinding(tokenID))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.RevokeToken(ctx, currentUserID(reqCtx), tokenID, request.Reason, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"revoked": true})
}

func (h *Handler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, businessAction challengedomain.BusinessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if h.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	businessActionValue := string(businessAction)
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessActionValue)
	if proofToken != "" {
		token, err := h.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{
			ProofToken:       proofToken,
			BusinessAction:   businessActionValue,
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
		if !stepUpTokenMatches(token, businessActionValue, operationBinding) {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof与当前操作不匹配")
		}
		securitycontext.SetStepUpProofAudit(reqCtx, stepUpProofAuditFromToken(token, businessActionValue, operationBinding))
		return stepUpProofMetadataFromToken(token, businessActionValue, operationBinding), nil
	}
	challenge, err := h.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
		BusinessAction:   businessActionValue,
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
		"businessAction":             businessActionValue,
		"operationBinding":           operationBinding,
	})
}

func requestContext(reqCtx *app.RequestContext) *facade.RequestContext {
	return &facade.RequestContext{
		DeviceID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
		LoginIP:   reqCtx.ClientIP(),
		UserAgent: string(reqCtx.UserAgent()),
		TraceID:   strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Trace-Id"))),
	}
}

func trustedSource(reqCtx *app.RequestContext) facade.TrustedSource {
	return facade.TrustedSource{
		ClientID:    strings.TrimSpace(string(reqCtx.Query("clientId"))),
		RedirectURL: firstNonBlankQuery(reqCtx, "redirectAfterLogin", "redirectUrl", "redirect"),
		Host:        strings.TrimSpace(string(reqCtx.Host())),
		Origin:      strings.TrimSpace(string(reqCtx.GetHeader("Origin"))),
		Referer:     strings.TrimSpace(string(reqCtx.GetHeader("Referer"))),
	}
}

func firstNonBlankQuery(reqCtx *app.RequestContext, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(string(reqCtx.Query(name))); value != "" {
			return value
		}
	}
	return ""
}

func writeLoginCookies(reqCtx *app.RequestContext, result *facade.ExternalLoginResult) {
	if result == nil || !result.Authenticated {
		return
	}
	if cookie := strings.TrimSpace(result.SessionCookieHeaderValue); cookie != "" {
		reqCtx.Response.Header.Add("Set-Cookie", cookie)
	}
	if cookie := strings.TrimSpace(result.RefreshCookieHeaderValue); cookie != "" {
		reqCtx.Response.Header.Add("Set-Cookie", cookie)
	}
}

func publicLoginResult(result *facade.ExternalLoginResult) *facade.ExternalLoginResult {
	if result == nil {
		return nil
	}
	copy := *result
	copy.SessionCookieHeaderValue = ""
	copy.RefreshCookieHeaderValue = ""
	return &copy
}

func currentUserID(reqCtx *app.RequestContext) int64 {
	return securitycontext.Require(reqCtx).UserID
}

func int64Path(reqCtx *app.RequestContext, name string) (int64, error) {
	raw := strings.TrimSpace(string(reqCtx.Param(name)))
	if raw == "" {
		return 0, apperrors.Params("路径参数不能为空")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, apperrors.Params("路径参数格式错误")
	}
	return value, nil
}

func buildRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	user := securitycontext.Require(reqCtx)
	if user.UserID <= 0 {
		return authorizationfacade.RequestScope{}, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return authorizationfacade.RequestScope{
		UserID:    user.UserID,
		Username:  user.Username,
		IPAddress: reqCtx.ClientIP(),
		UserAgent: string(reqCtx.UserAgent()),
		DeviceID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
		SessionID: user.SessionID,
		Source:    user.Source,
	}, nil
}

func chooseFlowNonce(flowNonce, businessAction string) string {
	if value := strings.TrimSpace(flowNonce); value != "" {
		return value
	}
	return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func stepUpTokenMatches(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) bool {
	if token == nil {
		return false
	}
	if strings.TrimSpace(token.BusinessAction) != "" && strings.TrimSpace(token.BusinessAction) != strings.TrimSpace(businessAction) {
		return false
	}
	if strings.TrimSpace(token.OperationBinding) != "" && strings.TrimSpace(token.OperationBinding) != strings.TrimSpace(operationBinding) {
		return false
	}
	return true
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
