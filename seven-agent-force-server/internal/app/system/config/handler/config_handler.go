package handler

import (
	"context"
	"fmt"
	"mime"
	"sort"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	configapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/application"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
)

type ManagementService interface {
	AddConfigGroup(ctx context.Context, actor configapp.Actor, request configfacade.ConfigGroupAddRequest) (int64, error)
	UpdateConfigGroup(ctx context.Context, actor configapp.Actor, request configfacade.ConfigGroupUpdateRequest) error
	DeleteConfigGroup(ctx context.Context, actor configapp.Actor, id int64) error
	GetConfigGroupPage(ctx context.Context, actor configapp.Actor, request configfacade.ConfigGroupQueryRequest) (*configfacade.PageResult[configfacade.ConfigGroupVO], error)
	GetConfigGroupByID(ctx context.Context, actor configapp.Actor, id int64) (*configfacade.ConfigGroupVO, error)
	MoveConfigGroup(ctx context.Context, actor configapp.Actor, id int64, beforeID, afterID *int64) error

	AddConfig(ctx context.Context, actor configapp.Actor, request configfacade.ConfigAddRequest) (int64, error)
	UpdateConfig(ctx context.Context, actor configapp.Actor, request configfacade.ConfigUpdateRequest) error
	DeleteConfig(ctx context.Context, actor configapp.Actor, id int64) error
	GetConfigByID(ctx context.Context, actor configapp.Actor, id int64) (*configfacade.ConfigVO, error)
	OpenConfigAsset(ctx context.Context, actor configapp.Actor, id int64) (*filefacade.ConfigAssetOpenResult, error)
	GetConfigPage(ctx context.Context, actor configapp.Actor, request configfacade.ConfigQueryRequest) (*configfacade.PageResult[configfacade.ConfigVO], error)
	ChangeEnabled(ctx context.Context, actor configapp.Actor, id int64, request configfacade.ConfigEnabledRequest) error
	RevealSensitiveValue(ctx context.Context, actor configapp.Actor, id int64, request configfacade.ConfigSensitiveRevealRequest) (*configfacade.ConfigSensitiveRevealResponse, error)
	ApplyPendingConfigs(ctx context.Context, actor configapp.Actor, isStartup bool) (int, error)
	GetPendingConfigs(ctx context.Context, actor configapp.Actor) ([]configfacade.PendingConfigVO, error)
	GetConfigChangeHistory(ctx context.Context, actor configapp.Actor, configID int64, limit int) ([]configfacade.ConfigChangeLogVO, error)
	RollbackConfigChange(ctx context.Context, actor configapp.Actor, logID int64, reason string) error
	GetOperationChain(ctx context.Context, actor configapp.Actor, logID int64) ([]configfacade.ConfigChangeLogVO, error)
	GetAuditLogs(ctx context.Context, actor configapp.Actor, request configfacade.AuditLogQueryRequest) ([]configfacade.ConfigChangeLogVO, error)
	GetRoleConfigScopes(ctx context.Context, actor configapp.Actor, roleID int64) ([]configfacade.ConfigScopeGrantVO, error)
	AssignRoleConfigScopes(ctx context.Context, actor configapp.Actor, roleID int64, request configfacade.AssignRoleConfigScopesRequest) error
}

type ClientService interface {
	ListConfigsForClient(ctx context.Context, actor configapp.Actor, request configfacade.ConfigClientListRequest) (map[string]configfacade.ConfigValueDTO, error)
	GetConfigByKeyForClient(ctx context.Context, actor configapp.Actor, configKey string) (*configfacade.ConfigValueDTO, error)
	GetConfigBatchForClient(ctx context.Context, actor configapp.Actor, request configfacade.ConfigBatchRequest) (map[string]configfacade.ConfigValueDTO, error)
}

type Handler struct {
	management ManagementService
	client     ClientService
	auth       authorizationfacade.AuthFacade
}

func NewHandler(management ManagementService, client ClientService) *Handler {
	return &Handler{management: management, client: client}
}

func (c *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c == nil {
		return
	}
	c.auth = auth
}

func (c *Handler) AddConfigGroup(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigGroupAddRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	id, err := c.management.AddConfigGroup(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, id)
}

func (c *Handler) UpdateConfigGroup(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigGroupUpdateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.UpdateConfigGroup(ctx, currentActor(reqCtx), request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) DeleteConfigGroup(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parseQueryInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.DeleteConfigGroup(ctx, currentActor(reqCtx), id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) GetConfigGroupPage(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigGroupQueryRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.management.GetConfigGroupPage(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) GetConfigGroupByID(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := c.management.GetConfigGroupByID(ctx, currentActor(reqCtx), id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) MoveConfigGroup(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request configfacade.MoveRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.MoveConfigGroup(ctx, currentActor(reqCtx), id, request.BeforeID, request.AfterID); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) AddConfig(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigAddRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	ctx = trustedRequestContext(ctx, reqCtx)
	id, err := c.management.AddConfig(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, id)
}

func (c *Handler) UpdateConfig(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigUpdateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	ctx = trustedRequestContext(ctx, reqCtx)
	if err := c.management.UpdateConfig(ctx, currentActor(reqCtx), request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) DeleteConfig(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parseQueryInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	ctx = trustedRequestContext(ctx, reqCtx)
	if err := c.management.DeleteConfig(ctx, currentActor(reqCtx), id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) GetConfigByID(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := c.management.GetConfigByID(ctx, currentActor(reqCtx), id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

// OpenConfigAsset streams the server-owned stable config path. It does not
// expose a file ID, reference ID, storage path, signed URL, or any mutable
// access policy. Keep it outside operation logging so no file bytes or asset
// metadata become an administrative request/response log payload.
func (c *Handler) OpenConfigAsset(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	ctx = trustedRequestContext(ctx, reqCtx)
	result, err := c.management.OpenConfigAsset(ctx, currentActor(reqCtx), id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	disposition := "attachment"
	if result.AssetType == filefacade.ConfigAssetImage {
		disposition = "inline"
	}
	fileName := strings.TrimSpace(result.FileName)
	if fileName == "" {
		fileName = "config-asset"
	}
	contentDisposition := mime.FormatMediaType(disposition, map[string]string{"filename": fileName})
	if contentDisposition == "" {
		contentDisposition = disposition
	}
	reqCtx.Response.Header.Set("Content-Type", result.ContentType)
	reqCtx.Response.Header.Set("Content-Disposition", contentDisposition)
	reqCtx.Response.Header.Set("Cache-Control", "no-store, max-age=0")
	reqCtx.Response.Header.Set("Pragma", "no-cache")
	reqCtx.Response.Header.Set("X-Content-Type-Options", "nosniff")
	reqCtx.Response.Header.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; sandbox")
	reqCtx.Response.Header.Set("Cross-Origin-Resource-Policy", "same-origin")
	reqCtx.Response.Header.Set("Referrer-Policy", "no-referrer")
	reqCtx.Response.SetBodyStream(result.Reader, int(result.Size))
}

// trustedRequestContext carries only the already authenticated request user
// into application/facade calls. CONFIG_ASSET uses this server context for its
// scoped credential, binding and read checks; client payload fields never
// participate in that authority.
func trustedRequestContext(ctx context.Context, reqCtx *app.RequestContext) context.Context {
	return securitycontext.WithUser(ctx, securitycontext.Get(reqCtx))
}

func (c *Handler) GetConfigPage(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigQueryRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.management.GetConfigPage(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) ChangeEnabled(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parseQueryInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request configfacade.ConfigEnabledRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.ChangeEnabled(ctx, currentActor(reqCtx), id, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) RevealSensitiveValue(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request configfacade.ConfigSensitiveRevealRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionConfigSensitiveReveal), sensitiveRevealBinding(id))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := c.management.RevealSensitiveValue(ctx, actorWithStepUpProof(reqCtx, proof), id, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) ApplyPendingConfigs(ctx context.Context, reqCtx *app.RequestContext) {
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionConfigApplyPending), "config:apply-pending")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	count, err := c.management.ApplyPendingConfigs(ctx, actorWithStepUpProof(reqCtx, proof), false)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, count)
}

func (c *Handler) GetPendingConfigs(ctx context.Context, reqCtx *app.RequestContext) {
	items, err := c.management.GetPendingConfigs(ctx, currentActor(reqCtx))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) GetConfigChangeHistory(ctx context.Context, reqCtx *app.RequestContext) {
	configID, err := parsePathInt64(reqCtx, "configId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	limit := parseOptionalQueryInt(reqCtx, "limit")
	items, err := c.management.GetConfigChangeHistory(ctx, currentActor(reqCtx), configID, limit)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) RollbackConfigChange(ctx context.Context, reqCtx *app.RequestContext) {
	logID, err := parseQueryInt64(reqCtx, "logId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	reason := strings.TrimSpace(string(reqCtx.Query("reason")))
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionConfigRollback), configRollbackBinding(logID))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.RollbackConfigChange(ctx, actorWithStepUpProof(reqCtx, proof), logID, reason); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) GetOperationChain(ctx context.Context, reqCtx *app.RequestContext) {
	logID, err := parsePathInt64(reqCtx, "logId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := c.management.GetOperationChain(ctx, currentActor(reqCtx), logID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) GetAuditLogs(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.AuditLogQueryRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := c.management.GetAuditLogs(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) GetRoleConfigScopes(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := c.management.GetRoleConfigScopes(ctx, currentActor(reqCtx), roleID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) AssignRoleConfigScopes(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request configfacade.AssignRoleConfigScopesRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionConfigScopeAssign), configScopeAssignmentBinding(roleID, request.Grants))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.AssignRoleConfigScopes(ctx, actorWithStepUpProof(reqCtx, proof), roleID, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) GetConfigByKeyForClient(ctx context.Context, reqCtx *app.RequestContext) {
	configKey := strings.TrimSpace(string(reqCtx.Param("configKey")))
	item, err := c.client.GetConfigByKeyForClient(ctx, currentActor(reqCtx), configKey)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) ListConfigsForClient(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigClientListRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.client.ListConfigsForClient(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) GetConfigBatchForClient(ctx context.Context, reqCtx *app.RequestContext) {
	var request configfacade.ConfigBatchRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.client.GetConfigBatchForClient(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, businessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if c.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildConfigRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseConfigFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
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

func buildConfigRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	userID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok || userID <= 0 {
		return authorizationfacade.RequestScope{}, apperrors.Unauthorized("未登录或登录信息失效")
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

func chooseConfigFlowNonce(flowNonce, businessAction string) string {
	value := strings.TrimSpace(flowNonce)
	if value == "" {
		return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return value
}

func sensitiveRevealBinding(configID int64) string {
	return fmt.Sprintf("config:%d|reveal", configID)
}

func configRollbackBinding(logID int64) string {
	return fmt.Sprintf("config:rollback:%d", logID)
}

func configScopeAssignmentBinding(roleID int64, grants []configfacade.ConfigScopeGrantVO) string {
	items := make([]string, 0, len(grants))
	for _, grant := range grants {
		groupCode := strings.ToLower(strings.TrimSpace(grant.GroupCode))
		if groupCode == "" {
			continue
		}
		configKey := strings.ToLower(strings.TrimSpace(grant.ConfigKey))
		scope := groupCode
		if configKey != "" {
			scope += "." + configKey
		}
		scope += fmt.Sprintf(":r%dw%dd%d", normalizeConfigAccessFlag(grant.CanRead), normalizeConfigAccessFlag(grant.CanWrite), normalizeConfigAccessFlag(grant.CanDelete))
		items = append(items, scope)
	}
	sort.Strings(items)
	return fmt.Sprintf("config-scope:role:%d|scopes:%s", roleID, strings.Join(items, ","))
}

func normalizeConfigAccessFlag(value int) int {
	if value != 0 {
		return 1
	}
	return 0
}

func currentActor(reqCtx *app.RequestContext) configapp.Actor {
	user := securitycontext.Require(reqCtx)
	return configapp.Actor{
		UserID:        user.UserID,
		Username:      user.Username,
		Nickname:      user.Nickname,
		IsAdmin:       user.IsAdmin,
		Authenticated: securitycontext.IsLogin(reqCtx),
		AccountID:     user.UserID,
		ScopeID:       actorScopeID(user.PrimaryOrgID),
		AuthzVersion:  user.AuthVersion,
		RoleIDs:       append([]int64(nil), user.RoleIDs...),
		Permissions:   append([]string(nil), user.Permissions...),
	}
}

func actorScopeID(primaryOrgID int64) string {
	if primaryOrgID > 0 {
		return fmt.Sprintf("org:%d", primaryOrgID)
	}
	return "server:local"
}

func actorWithStepUpProof(reqCtx *app.RequestContext, proof stepup.ProofMetadata) configapp.Actor {
	actor := currentActor(reqCtx)
	actor.StepUpProof = proof
	return actor
}

func parsePathInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	if reqCtx == nil {
		return 0, apperrors.Params("路径参数错误")
	}
	return parseStringInt64(string(reqCtx.Param(key)))
}

func parseQueryInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	if reqCtx == nil {
		return 0, apperrors.Params("查询参数错误")
	}
	return parseStringInt64(string(reqCtx.Query(key)))
}

func parseOptionalQueryInt(reqCtx *app.RequestContext, key string) int {
	if reqCtx == nil {
		return 0
	}
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func parseStringInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, apperrors.Params("路径参数错误")
	}
	return parsed, nil
}
