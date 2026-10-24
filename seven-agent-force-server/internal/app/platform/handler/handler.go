package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	platformapp "github.com/CaixyPromise/seven-agent/force/internal/app/platform/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
)

type Service interface {
	facade.PublicFacade
	facade.AdminFacade
}

type Handler struct {
	service Service
	auth    authorizationfacade.AuthFacade
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if h == nil {
		return
	}
	h.auth = auth
}

func (h *Handler) ResolveLoginOptions(ctx context.Context, reqCtx *app.RequestContext) {
	redirectURL := strings.TrimSpace(string(reqCtx.Query("redirectUrl")))
	if redirectURL == "" {
		redirectURL = strings.TrimSpace(string(reqCtx.Query("redirect")))
	}
	request := facade.ResolvePlatformRequest{
		ClientID:           strings.TrimSpace(string(reqCtx.Query("clientId"))),
		LoginTransactionID: strings.TrimSpace(string(reqCtx.Query("loginTransactionId"))),
		LoginContextID:     strings.TrimSpace(string(reqCtx.Query("loginContextId"))),
		RedirectURL:        redirectURL,
		ExplicitCode:       strings.TrimSpace(string(reqCtx.Query("platformCode"))),
		TrustedSource: facade.TrustedSource{
			Host:    strings.TrimSpace(string(reqCtx.Host())),
			Origin:  strings.TrimSpace(string(reqCtx.GetHeader("Origin"))),
			Referer: strings.TrimSpace(string(reqCtx.GetHeader("Referer"))),
		},
	}
	result, err := h.service.ResolveLoginOptions(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (h *Handler) ListPlatforms(ctx context.Context, reqCtx *app.RequestContext) {
	var query facade.PlatformQuery
	if err := httpx.BindAndValidate(reqCtx, &query); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := h.service.ListPlatforms(ctx, query)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (h *Handler) ResolveSource(ctx context.Context, reqCtx *app.RequestContext) {
	redirectURL := strings.TrimSpace(string(reqCtx.Query("redirectUrl")))
	if redirectURL == "" {
		redirectURL = strings.TrimSpace(string(reqCtx.Query("redirect")))
	}
	trusted := facade.TrustedSource{
		Host:    firstNonBlank(strings.TrimSpace(string(reqCtx.Query("host"))), strings.TrimSpace(string(reqCtx.Host()))),
		Origin:  firstNonBlank(strings.TrimSpace(string(reqCtx.Query("origin"))), strings.TrimSpace(string(reqCtx.GetHeader("Origin")))),
		Referer: firstNonBlank(strings.TrimSpace(string(reqCtx.Query("referer"))), strings.TrimSpace(string(reqCtx.GetHeader("Referer")))),
	}
	request := facade.ResolvePlatformRequest{
		ClientID:           strings.TrimSpace(string(reqCtx.Query("clientId"))),
		LoginTransactionID: strings.TrimSpace(string(reqCtx.Query("loginTransactionId"))),
		RedirectURL:        redirectURL,
		ExplicitCode:       strings.TrimSpace(string(reqCtx.Query("platformCode"))),
		TrustedSource:      trusted,
	}
	platformCode, err := h.service.ResolvePlatformCode(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{
		"platformCode": platformCode,
		"source": map[string]string{
			"clientId":     request.ClientID,
			"redirectUrl":  request.RedirectURL,
			"host":         trusted.Host,
			"origin":       trusted.Origin,
			"referer":      trusted.Referer,
			"platformCode": request.ExplicitCode,
		},
	})
}

func (h *Handler) GetPlatform(ctx context.Context, reqCtx *app.RequestContext) {
	detail, err := h.service.GetPlatform(ctx, string(reqCtx.Param("platformCode")))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, detail)
}

func (h *Handler) CreatePlatform(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.PlatformSaveRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		response.Error(reqCtx, apperrors.Params("reason不能为空"))
		return
	}
	binding := platformapp.BuildPlatformCreateOperationBinding(request.PlatformCode)
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, platformapp.StepUpActionPlatformCreate, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	detail, err := h.service.CreatePlatform(ctx, currentUserID(reqCtx), request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, detail)
}

func (h *Handler) UpdatePlatform(ctx context.Context, reqCtx *app.RequestContext) {
	platformCode := string(reqCtx.Param("platformCode"))
	var request facade.PlatformSaveRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		response.Error(reqCtx, apperrors.Params("reason不能为空"))
		return
	}
	binding := platformapp.BuildPlatformUpdateOperationBinding(platformCode)
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, platformapp.StepUpActionPlatformUpdate, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	before, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	detail, err := h.service.UpdatePlatform(ctx, currentUserID(reqCtx), platformCode, request, proof)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true, "before": before, "after": detail})
}

func (h *Handler) UpdatePlatformStatus(ctx context.Context, reqCtx *app.RequestContext) {
	platformCode := string(reqCtx.Param("platformCode"))
	var request facade.PlatformStatusRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := platformapp.BuildPlatformStatusOperationBinding(platformCode, request.Status)
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, platformapp.StepUpActionPlatformStatusChange, binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	before, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.UpdatePlatformStatus(ctx, currentUserID(reqCtx), platformCode, request, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	after, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true, "before": before, "after": after})
}

func (h *Handler) ReplaceLoginMethods(ctx context.Context, reqCtx *app.RequestContext) {
	platformCode := string(reqCtx.Param("platformCode"))
	var request struct {
		Methods []facade.LoginMethodSaveRequest `json:"methods"`
		Reason  string                          `json:"reason"`
	}
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		response.Error(reqCtx, apperrors.Params("reason不能为空"))
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, platformapp.StepUpActionPlatformLoginMethodsReplace, platformapp.BuildPlatformLoginMethodsOperationBinding(platformCode))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	before, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.ReplaceLoginMethods(ctx, currentUserID(reqCtx), platformCode, request.Methods, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	after, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true, "before": before, "after": after})
}

func (h *Handler) ReplaceSourceRules(ctx context.Context, reqCtx *app.RequestContext) {
	platformCode := string(reqCtx.Param("platformCode"))
	var request struct {
		Rules  []facade.SourceRuleSaveRequest `json:"rules"`
		Reason string                         `json:"reason"`
	}
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		response.Error(reqCtx, apperrors.Params("reason不能为空"))
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, platformapp.StepUpActionPlatformSourceRulesReplace, platformapp.BuildPlatformSourceRulesOperationBinding(platformCode))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	before, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.ReplaceSourceRules(ctx, currentUserID(reqCtx), platformCode, request.Rules, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	after, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true, "before": before, "after": after})
}

func (h *Handler) ReplaceDefaultRoles(ctx context.Context, reqCtx *app.RequestContext) {
	platformCode := string(reqCtx.Param("platformCode"))
	var request struct {
		Roles  []facade.DefaultRoleSaveRequest `json:"roles"`
		Reason string                          `json:"reason"`
	}
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		response.Error(reqCtx, apperrors.Params("reason不能为空"))
		return
	}
	proof, err := h.ensureProtectedMutation(ctx, reqCtx, platformapp.StepUpActionPlatformDefaultRolesReplace, platformapp.BuildPlatformDefaultRolesOperationBinding(platformCode))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	before, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := h.service.ReplaceDefaultRoles(ctx, currentUserID(reqCtx), platformCode, request.Roles, proof); err != nil {
		response.Error(reqCtx, err)
		return
	}
	after, err := h.service.GetPlatform(ctx, platformCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]any{"updated": true, "before": before, "after": after})
}

func (h *Handler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, businessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if h.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
	if proofToken != "" {
		token, err := h.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{
			ProofToken:       proofToken,
			BusinessAction:   businessAction,
			FlowNonce:        flowNonce,
			OperationBinding: operationBinding,
			ConsumeOnce:      true,
		})
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		if token == nil || !stepUpTokenMatches(token, businessAction, operationBinding) {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof验证失败")
		}
		securitycontext.SetStepUpProofAudit(reqCtx, stepUpProofAuditFromToken(token, businessAction, operationBinding))
		return stepUpProofMetadataFromToken(token, businessAction, operationBinding), nil
	}
	challenge, err := h.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
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
		"businessAction":             businessAction,
		"operationBinding":           operationBinding,
	})
}

func currentUserID(reqCtx *app.RequestContext) int64 {
	return securitycontext.Require(reqCtx).UserID
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
