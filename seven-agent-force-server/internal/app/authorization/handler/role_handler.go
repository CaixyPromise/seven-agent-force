package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type RoleHandler struct {
	roles authorizationfacade.RoleFacade
	auth  authorizationfacade.AuthFacade
}

func NewRoleHandler(roles authorizationfacade.RoleFacade) *RoleHandler {
	return &RoleHandler{roles: roles}
}

func (c *RoleHandler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c == nil {
		return
	}
	c.auth = auth
}

func (c *RoleHandler) ListRoles(ctx context.Context, reqCtx *app.RequestContext) {
	items, err := c.roles.GetRoleList(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *RoleHandler) PageRoles(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.roles.PageRoles(ctx, authorizationfacade.RolePageQuery{
		Current: queryInt64(reqCtx, "current", 1),
		Size:    queryInt64(reqCtx, "size", queryInt64(reqCtx, "pageSize", 10)),
		Code:    queryString(reqCtx, "code"),
		Name:    queryString(reqCtx, "name"),
		Status:  queryOptionalInt(reqCtx, "status"),
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *RoleHandler) GetSecurityStatus(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.roles.GetRootSecurityStatus(ctx)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) GetRoleGrantSnapshot(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.roles.GetRoleGrantSnapshot(ctx, roleID)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) PreviewRoleGrantBundle(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request authorizationfacade.RoleGrantBundleRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.roles.PreviewRoleGrantBundle(ctx, authorizationfacade.PreviewRoleGrantBundleCommand{
		RoleID: roleID, OperatorID: currentUserID(reqCtx), RoleGrantBundleRequest: request,
	})
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) CommitRoleGrantBundle(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request authorizationfacade.RoleGrantBundleRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding, err := authorizationfacade.RoleGrantOperationBinding(roleID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACCommitRoleGrants), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.roles.CommitRoleGrantBundle(ctx, authorizationfacade.CommitRoleGrantBundleCommand{
		RoleID: roleID, OperatorID: currentUserID(reqCtx), StepUpProof: proof, RoleGrantBundleRequest: request,
	})
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) GetRole(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.roles.GetRole(ctx, roleID)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) CreateRole(ctx context.Context, reqCtx *app.RequestContext) {
	var command authorizationfacade.RoleCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.OperatorID = currentUserID(reqCtx)
	result, err := c.roles.CreateRole(ctx, command)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) UpdateRole(ctx context.Context, reqCtx *app.RequestContext) {
	var command authorizationfacade.RoleCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.OperatorID = currentUserID(reqCtx)
	result, err := c.roles.UpdateRole(ctx, command)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) DeleteRole(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeResult(reqCtx, true, c.roles.DeleteRole(ctx, roleID, currentUserID(reqCtx)))
}

func (c *RoleHandler) GetRoleDeptIDs(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.roles.GetRoleDeptIDs(ctx, roleID)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) AssignRoleDepts(ctx context.Context, reqCtx *app.RequestContext) {
	var request authorizationfacade.AssignRoleDeptsCommand
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := roleDeptAssignmentBinding(request.RoleID, []int64(request.DeptIDs))
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignRoleDepts), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.OperatorID = currentUserID(reqCtx)
	request.StepUpProof = proof
	if err := c.roles.AssignRoleDepts(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *RoleHandler) GetRoleMenuTree(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := c.roles.GetRoleMenuTree(ctx, roleID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *RoleHandler) GetRoleMenusCompat(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := c.roles.GetRoleMenuIDs(ctx, roleID)
	writeResult(reqCtx, items, err)
}

func (c *RoleHandler) AssignRoleMenusCompat(ctx context.Context, reqCtx *app.RequestContext) {
	roleID, err := parsePathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	menuIDs, err := bindInt64SliceBody(reqCtx, "menuIds")
	if err != nil {
		response.Error(reqCtx, apperrors.Params("菜单ID列表参数错误"))
		return
	}
	binding := roleMenuAssignmentBinding(roleID, menuIDs)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignRoleMenus), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeResult(reqCtx, true, c.roles.AssignRoleMenus(ctx, authorizationfacade.AssignRoleMenusCommand{
		RoleID:      int64(roleID),
		MenuIDs:     []int64(menuIDs),
		OperatorID:  currentUserID(reqCtx),
		StepUpProof: proof,
	}))
}

func (c *RoleHandler) AssignRolePermissions(ctx context.Context, reqCtx *app.RequestContext) {
	var request authorizationfacade.AssignRolePermissionsCommand
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := rolePermissionAssignmentBinding(request.RoleID, []int64(request.PermissionIDs), []int64(request.MenuIDs))
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignRolePermissions), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.OperatorID = currentUserID(reqCtx)
	request.StepUpProof = proof
	if err := c.roles.AssignRolePermissions(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *RoleHandler) AssignUserRoles(ctx context.Context, reqCtx *app.RequestContext) {
	var request authorizationfacade.AssignUserRolesCommand
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := userRoleAssignmentBinding(request.UserID, []int64(request.RoleIDs))
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignUserRoles), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.OperatorID = currentUserID(reqCtx)
	request.StepUpProof = proof
	if err := c.roles.AssignUserRoles(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *RoleHandler) GetMenuTree(ctx context.Context, reqCtx *app.RequestContext) {
	items, err := c.roles.GetMenuTree(ctx, false)
	writeResult(reqCtx, items, err)
}

func (c *RoleHandler) GetEnabledMenuTree(ctx context.Context, reqCtx *app.RequestContext) {
	items, err := c.roles.GetMenuTree(ctx, true)
	writeResult(reqCtx, items, err)
}

func (c *RoleHandler) GetMenu(ctx context.Context, reqCtx *app.RequestContext) {
	menuID, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.roles.GetMenu(ctx, menuID)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) CreateMenu(ctx context.Context, reqCtx *app.RequestContext) {
	var command authorizationfacade.MenuCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.OperatorID = currentUserID(reqCtx)
	result, err := c.roles.CreateMenu(ctx, command)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) UpdateMenu(ctx context.Context, reqCtx *app.RequestContext) {
	var command authorizationfacade.MenuCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.OperatorID = currentUserID(reqCtx)
	result, err := c.roles.UpdateMenu(ctx, command)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) DeleteMenu(ctx context.Context, reqCtx *app.RequestContext) {
	menuID, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeResult(reqCtx, true, c.roles.DeleteMenu(ctx, menuID, currentUserID(reqCtx)))
}

func (c *RoleHandler) ListPermissions(ctx context.Context, reqCtx *app.RequestContext) {
	if queryString(reqCtx, "current") != "" || queryString(reqCtx, "size") != "" || queryString(reqCtx, "pageSize") != "" {
		result, err := c.roles.PagePermissions(ctx, authorizationfacade.PermissionPageQuery{
			Current:      queryInt64(reqCtx, "current", 1),
			Size:         queryInt64(reqCtx, "size", queryInt64(reqCtx, "pageSize", 10)),
			Code:         queryString(reqCtx, "code"),
			Name:         queryString(reqCtx, "name"),
			ResourceType: queryString(reqCtx, "resourceType"),
			Method:       queryString(reqCtx, "method"),
			Path:         queryString(reqCtx, "path"),
			Status:       queryOptionalInt(reqCtx, "status"),
		})
		writeResult(reqCtx, result, err)
		return
	}
	items, err := c.roles.ListPermissions(ctx, authorizationfacade.PermissionQuery{
		Code:         queryString(reqCtx, "code"),
		Name:         queryString(reqCtx, "name"),
		ResourceType: queryString(reqCtx, "resourceType"),
		Method:       queryString(reqCtx, "method"),
		Path:         queryString(reqCtx, "path"),
		Status:       queryOptionalInt(reqCtx, "status"),
	})
	writeResult(reqCtx, items, err)
}

func (c *RoleHandler) GetPermission(ctx context.Context, reqCtx *app.RequestContext) {
	permissionID, err := parsePathInt64(reqCtx, "permissionId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.roles.GetPermission(ctx, permissionID)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) CreatePermission(ctx context.Context, reqCtx *app.RequestContext) {
	var command authorizationfacade.PermissionCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.OperatorID = currentUserID(reqCtx)
	result, err := c.roles.CreatePermission(ctx, command)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) UpdatePermission(ctx context.Context, reqCtx *app.RequestContext) {
	permissionID, err := parsePathInt64(reqCtx, "permissionId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var command authorizationfacade.PermissionCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.OperatorID = currentUserID(reqCtx)
	result, err := c.roles.UpdatePermission(ctx, permissionID, command)
	writeResult(reqCtx, result, err)
}

func (c *RoleHandler) DeletePermission(ctx context.Context, reqCtx *app.RequestContext) {
	permissionID, err := parsePathInt64(reqCtx, "permissionId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	writeResult(reqCtx, true, c.roles.DeletePermission(ctx, permissionID, currentUserID(reqCtx)))
}

func (c *RoleHandler) GetMenuPermissionIDs(ctx context.Context, reqCtx *app.RequestContext) {
	menuID, err := parsePathInt64(reqCtx, "menuId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items, err := c.roles.GetMenuPermissionIDs(ctx, menuID)
	writeResult(reqCtx, items, err)
}

func (c *RoleHandler) BindMenuPermissions(ctx context.Context, reqCtx *app.RequestContext) {
	menuID, err := parsePathInt64(reqCtx, "menuId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var command authorizationfacade.MenuPermissionAssignCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.MenuID = int64(menuID)
	command.OperatorID = currentUserID(reqCtx)
	binding := menuPermissionAssignmentBinding(menuID, []int64(command.PermissionIDs))
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignMenuPermissions), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	command.StepUpProof = proof
	writeResult(reqCtx, true, c.roles.BindMenuPermissions(ctx, command))
}

func (c *RoleHandler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, businessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if c.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildRoleRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseRoleFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
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

func buildRoleRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
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

func chooseRoleFlowNonce(flowNonce, businessAction string) string {
	value := strings.TrimSpace(flowNonce)
	if value == "" {
		return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return value
}

func userRoleAssignmentBinding(userID int64, roleIDs []int64) string {
	return fmt.Sprintf("user:%d|roles:%s", userID, joinSortedIDs(roleIDs))
}

func rolePermissionAssignmentBinding(roleID int64, permissionIDs, menuIDs []int64) string {
	return fmt.Sprintf("role:%d|permissions:%s|menus:%s", roleID, joinSortedIDs(permissionIDs), joinSortedIDs(menuIDs))
}

func roleMenuAssignmentBinding(roleID int64, menuIDs []int64) string {
	return fmt.Sprintf("role:%d|menus:%s", roleID, joinSortedIDs(menuIDs))
}

func roleDeptAssignmentBinding(roleID int64, deptIDs []int64) string {
	return fmt.Sprintf("role:%d|depts:%s", roleID, joinSortedIDs(deptIDs))
}

func menuPermissionAssignmentBinding(menuID int64, permissionIDs []int64) string {
	return fmt.Sprintf("menu:%d|permissions:%s", menuID, joinSortedIDs(permissionIDs))
}

func joinSortedIDs(ids []int64) string {
	normalized := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	parts := make([]string, 0, len(normalized))
	for _, id := range normalized {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func writeResult(reqCtx *app.RequestContext, data any, err error) {
	if err != nil {
		log.Printf("authorization handler failed traceId=%s path=%s err=%v", xcontext.TraceID(reqCtx), string(reqCtx.Path()), err)
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, data)
}

func currentUserID(reqCtx *app.RequestContext) int64 {
	userID, _ := securitycontext.CurrentUserID(reqCtx)
	return userID
}

func queryString(reqCtx *app.RequestContext, key string) string {
	return strings.TrimSpace(string(reqCtx.Query(key)))
}

func queryInt64(reqCtx *app.RequestContext, key string, fallback int64) int64 {
	raw := queryString(reqCtx, key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func queryOptionalInt(reqCtx *app.RequestContext, key string) *int {
	raw := queryString(reqCtx, key)
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &value
}

func bindInt64SliceBody(reqCtx *app.RequestContext, fallbackKey string) ([]int64, error) {
	body := reqCtx.Request.Body()
	var rawItems []json.RawMessage
	if err := json.Unmarshal(body, &rawItems); err == nil {
		return parseRawInt64Items(rawItems)
	}
	var object map[string][]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, err
	}
	return parseRawInt64Items(object[fallbackKey])
}

func parseRawInt64Items(rawItems []json.RawMessage) ([]int64, error) {
	values := make([]int64, 0, len(rawItems))
	seen := make(map[int64]struct{}, len(rawItems))
	for _, raw := range rawItems {
		text := strings.TrimSpace(string(raw))
		if text == "" || text == "null" {
			continue
		}
		if strings.HasPrefix(text, "\"") {
			var unquoted string
			if err := json.Unmarshal(raw, &unquoted); err != nil {
				return nil, err
			}
			text = strings.TrimSpace(unquoted)
		}
		if text == "" {
			continue
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, err
		}
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values, nil
}
