package handler

import (
	"context"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
)

type tempPermissionService interface {
	GrantTemporaryPermission(ctx context.Context, command authorizationfacade.TemporaryPermissionGrantCommand) error
	RevokeTemporaryPermission(ctx context.Context, command authorizationfacade.TemporaryPermissionUpdateCommand) error
	ExtendTemporaryPermission(ctx context.Context, command authorizationfacade.TemporaryPermissionUpdateCommand) error
	CleanupExpiredTemporaryPermissions(ctx context.Context) error
	ListUserTemporaryPermissions(ctx context.Context, userID int64) ([]authorizationfacade.TemporaryPermissionVO, error)
	TemporaryPermissionStats(ctx context.Context) (*authorizationfacade.TemporaryPermissionStatsVO, error)
	ResolvePermissionCode(ctx context.Context, permissionID int64) (string, error)
}

type TemporaryPermissionHandler struct {
	service tempPermissionService
	auth    authorizationfacade.AuthFacade
}

// temporaryPermissionGrantRequest keeps JSON entity identifiers precision-safe.
// The UI intentionally serializes 64-bit IDs as decimal strings.
type temporaryPermissionGrantRequest struct {
	UserID         int64      `json:"userId"`
	PermissionCode string     `json:"permissionCode"`
	ExpireAt       *time.Time `json:"expireAt,omitempty"`
	Source         string     `json:"source,omitempty"`
	Reason         string     `json:"reason"`
}

// temporaryPermissionUpdateRequest is shared by revoke and extend operations.
type temporaryPermissionUpdateRequest struct {
	UserID         int64      `json:"userId"`
	PermissionCode string     `json:"permissionCode"`
	ExpireAt       *time.Time `json:"expireAt,omitempty"`
	Reason         string     `json:"reason"`
}

func (c *TemporaryPermissionHandler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c != nil {
		c.auth = auth
	}
}

func NewTemporaryPermissionHandler(service tempPermissionService) *TemporaryPermissionHandler {
	return &TemporaryPermissionHandler{service: service}
}

func (c *TemporaryPermissionHandler) Grant(ctx context.Context, reqCtx *app.RequestContext) {
	if strings.TrimSpace(string(reqCtx.Query("userId"))) != "" {
		c.GrantCompat(ctx, reqCtx)
		return
	}
	var request temporaryPermissionGrantRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	userID := request.UserID
	if !c.ensureUserInScope(ctx, reqCtx, userID) {
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, "RBAC_GRANT_TEMP_PERMISSION", authorizationfacade.TemporaryPermissionOperationBinding("RBAC_GRANT_TEMP_PERMISSION", userID, request.PermissionCode, request.ExpireAt, request.Reason))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	command := authorizationfacade.TemporaryPermissionGrantCommand{
		UserID: userID, PermissionCode: request.PermissionCode, ExpireAt: request.ExpireAt,
		Source: request.Source, Reason: request.Reason, GrantedBy: currentUserID(reqCtx), StepUpProof: proof,
	}
	if err := c.service.GrantTemporaryPermission(ctx, command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *TemporaryPermissionHandler) Revoke(ctx context.Context, reqCtx *app.RequestContext) {
	var request temporaryPermissionUpdateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	userID := request.UserID
	if !c.ensureUserInScope(ctx, reqCtx, userID) {
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, "RBAC_REVOKE_TEMP_PERMISSION", authorizationfacade.TemporaryPermissionOperationBinding("RBAC_REVOKE_TEMP_PERMISSION", userID, request.PermissionCode, nil, request.Reason))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	command := authorizationfacade.TemporaryPermissionUpdateCommand{
		UserID: userID, PermissionCode: request.PermissionCode, Reason: request.Reason,
		OperatorID: currentUserID(reqCtx), StepUpProof: proof,
	}
	if err := c.service.RevokeTemporaryPermission(ctx, command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *TemporaryPermissionHandler) Extend(ctx context.Context, reqCtx *app.RequestContext) {
	var request temporaryPermissionUpdateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	userID := request.UserID
	if !c.ensureUserInScope(ctx, reqCtx, userID) {
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, "RBAC_EXTEND_TEMP_PERMISSION", authorizationfacade.TemporaryPermissionOperationBinding("RBAC_EXTEND_TEMP_PERMISSION", userID, request.PermissionCode, request.ExpireAt, request.Reason))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	command := authorizationfacade.TemporaryPermissionUpdateCommand{
		UserID: userID, PermissionCode: request.PermissionCode, ExpireAt: request.ExpireAt,
		Reason: request.Reason, OperatorID: currentUserID(reqCtx), StepUpProof: proof,
	}
	if err := c.service.ExtendTemporaryPermission(ctx, command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *TemporaryPermissionHandler) Cleanup(ctx context.Context, reqCtx *app.RequestContext) {
	if err := c.service.CleanupExpiredTemporaryPermissions(ctx); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *TemporaryPermissionHandler) Stats(ctx context.Context, reqCtx *app.RequestContext) {
	stats, err := c.service.TemporaryPermissionStats(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, stats)
}

func (c *TemporaryPermissionHandler) ListByUser(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parsePathInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, userID) {
		return
	}
	items, err := c.service.ListUserTemporaryPermissions(ctx, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *TemporaryPermissionHandler) ListByUserQuery(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parseQueryInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, userID) {
		return
	}
	items, err := c.service.ListUserTemporaryPermissions(ctx, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *TemporaryPermissionHandler) GrantCompat(ctx context.Context, reqCtx *app.RequestContext) {
	expireAt, err := parseQueryTime(reqCtx, "expireTime")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	userID, err := parseQueryInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	permissionCode := strings.TrimSpace(string(reqCtx.Query("permissionCode")))
	if permissionCode == "" {
		permissionID, idErr := parseQueryInt64(reqCtx, "permissionId")
		if idErr == nil {
			permissionCode, err = c.service.ResolvePermissionCode(ctx, permissionID)
			if err != nil {
				response.Error(reqCtx, err)
				return
			}
		}
	}
	if permissionCode == "" {
		response.Error(reqCtx, contextParamError("permissionCode或permissionId不能为空"))
		return
	}
	request := authorizationfacade.TemporaryPermissionGrantCommand{
		UserID:         userID,
		PermissionCode: permissionCode,
		ExpireAt:       expireAt,
		Source:         strings.TrimSpace(string(reqCtx.Query("source"))),
		Reason:         strings.TrimSpace(string(reqCtx.Query("reason"))),
		GrantedBy:      currentUserID(reqCtx),
	}
	if !c.ensureUserInScope(ctx, reqCtx, request.UserID) {
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, "RBAC_GRANT_TEMP_PERMISSION", authorizationfacade.TemporaryPermissionOperationBinding("RBAC_GRANT_TEMP_PERMISSION", request.UserID, request.PermissionCode, request.ExpireAt, request.Reason))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.StepUpProof = proof
	if err := c.service.GrantTemporaryPermission(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *TemporaryPermissionHandler) RevokeCompat(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parseQueryInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	permissionCode := strings.TrimSpace(string(reqCtx.Query("permissionCode")))
	if permissionCode == "" {
		permissionID, idErr := parseQueryInt64(reqCtx, "permissionId")
		if idErr == nil {
			permissionCode, err = c.service.ResolvePermissionCode(ctx, permissionID)
			if err != nil {
				response.Error(reqCtx, err)
				return
			}
		}
	}
	if permissionCode == "" {
		response.Error(reqCtx, contextParamError("permissionCode或permissionId不能为空"))
		return
	}
	request := authorizationfacade.TemporaryPermissionUpdateCommand{
		UserID:         userID,
		PermissionCode: permissionCode,
		OperatorID:     currentUserID(reqCtx),
		Reason:         strings.TrimSpace(string(reqCtx.Query("reason"))),
	}
	if !c.ensureUserInScope(ctx, reqCtx, request.UserID) {
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, "RBAC_REVOKE_TEMP_PERMISSION", authorizationfacade.TemporaryPermissionOperationBinding("RBAC_REVOKE_TEMP_PERMISSION", request.UserID, request.PermissionCode, nil, request.Reason))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.StepUpProof = proof
	if err := c.service.RevokeTemporaryPermission(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *TemporaryPermissionHandler) ExtendCompat(ctx context.Context, reqCtx *app.RequestContext) {
	expireAt, err := parseQueryTime(reqCtx, "newExpireTime")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	userID, err := parseQueryInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	permissionCode := strings.TrimSpace(string(reqCtx.Query("permissionCode")))
	if permissionCode == "" {
		permissionID, idErr := parseQueryInt64(reqCtx, "permissionId")
		if idErr == nil {
			permissionCode, err = c.service.ResolvePermissionCode(ctx, permissionID)
			if err != nil {
				response.Error(reqCtx, err)
				return
			}
		}
	}
	if permissionCode == "" {
		response.Error(reqCtx, contextParamError("permissionCode或permissionId不能为空"))
		return
	}
	request := authorizationfacade.TemporaryPermissionUpdateCommand{
		UserID:         userID,
		PermissionCode: permissionCode,
		ExpireAt:       expireAt,
		OperatorID:     currentUserID(reqCtx),
		Reason:         strings.TrimSpace(string(reqCtx.Query("reason"))),
	}
	if !c.ensureUserInScope(ctx, reqCtx, request.UserID) {
		return
	}
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, "RBAC_EXTEND_TEMP_PERMISSION", authorizationfacade.TemporaryPermissionOperationBinding("RBAC_EXTEND_TEMP_PERMISSION", request.UserID, request.PermissionCode, request.ExpireAt, request.Reason))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.StepUpProof = proof
	if err := c.service.ExtendTemporaryPermission(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *TemporaryPermissionHandler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, action, binding string) (stepup.ProofMetadata, error) {
	return (&RoleHandler{auth: c.auth}).ensureProtectedMutation(ctx, reqCtx, action, binding)
}

func (c *TemporaryPermissionHandler) ensureUserInScope(ctx context.Context, reqCtx *app.RequestContext, userID int64) bool {
	if userID <= 0 {
		response.Error(reqCtx, apperrors.Params("userId不能为空"))
		return false
	}
	if c.auth == nil {
		response.Error(reqCtx, apperrors.System("临时权限用户范围校验未配置"))
		return false
	}
	target, err := c.auth.GetUserVO(ctx, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return false
	}
	if target == nil {
		response.Error(reqCtx, apperrors.NotFound("用户不存在"))
		return false
	}
	actor := securitycontext.Require(reqCtx)
	if actor.IsAdmin || actor.DataScopeType == securitycontext.DataScopeAll {
		return true
	}
	if actor.DataScopeType == securitycontext.DataScopeSelf {
		if actor.UserID == userID {
			return true
		}
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return false
	}
	if actor.DataScopeType == securitycontext.DataScopeCustom || actor.DataScopeType == securitycontext.DataScopeDept || actor.DataScopeType == securitycontext.DataScopeDeptAndChild {
		for _, allowedDeptID := range actor.DataScopeDeptIDs {
			for _, targetDeptID := range target.DeptIDs {
				if allowedDeptID == targetDeptID {
					return true
				}
			}
		}
	}
	response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
	return false
}
