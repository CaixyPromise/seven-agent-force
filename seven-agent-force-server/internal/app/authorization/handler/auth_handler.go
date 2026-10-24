package handler

import (
	"context"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type AuthHandler struct {
	auth    authorizationfacade.AuthFacade
	service menuReader
}

type menuReader interface {
	GetCurrentUserMenus(ctx context.Context, userID int64) ([]authorizationfacade.MenuTreeNodeVO, error)
}

func NewAuthHandler(auth authorizationfacade.AuthFacade, service menuReader) *AuthHandler {
	return &AuthHandler{auth: auth, service: service}
}

func (c *AuthHandler) GetCurrentUser(ctx context.Context, reqCtx *app.RequestContext, scope authorizationfacade.RequestScope) {
	result, err := c.auth.GetLoginUser(ctx, scope)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	current := securitycontext.Require(reqCtx)
	deptIDs := append([]int64{}, current.DataScopeDeptIDs...)
	orgIDs := append([]int64{}, current.DataScopeOrgIDs...)
	response.Success(reqCtx, authorizationfacade.CurrentUserResponse{
		ID:            result.UserID,
		Username:      result.Username,
		Nickname:      result.Nickname,
		UserAvatar:    result.Avatar,
		UserRole:      result.RoleNames,
		UserPosition:  result.PostNames,
		Organizations: result.OrgNames,
		Departments:   result.DeptNames,
		Permissions:   result.Permissions,
		RoleCodes:     result.RoleCodes,
		PostCodes:     result.PostCodes,
		OrgCodes:      result.OrgCodes,
		DeptCodes:     result.DeptCodes,
		IsAdmin:       current.IsAdmin,
		PrimaryOrgID:  current.PrimaryOrgID,
		AuthVersion:   current.AuthVersion,
		DataScope: authorizationfacade.UserDataScopeVO{
			UserID:    current.UserID,
			DeptIDs:   deptIDs,
			OrgIDs:    orgIDs,
			ScopeType: string(current.DataScopeType),
		},
	})
}

func (c *AuthHandler) GetCurrentUserMenus(ctx context.Context, reqCtx *app.RequestContext, scope authorizationfacade.RequestScope) {
	items, err := c.service.GetCurrentUserMenus(ctx, scope.UserID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *AuthHandler) GetUserPermissionsByModule(ctx context.Context, reqCtx *app.RequestContext, scope authorizationfacade.RequestScope) {
	module := string(reqCtx.Query("module"))
	items, err := c.auth.GetUserPermissionsByModule(ctx, scope, module)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *AuthHandler) CreateStepUpChallenge(ctx context.Context, reqCtx *app.RequestContext, scope authorizationfacade.RequestScope) {
	var request authorizationfacade.StepUpChallengeRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.auth.CreateStepUpChallenge(ctx, scope, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *AuthHandler) VerifyStepUp(ctx context.Context, reqCtx *app.RequestContext, scope authorizationfacade.RequestScope) {
	var request authorizationfacade.StepUpVerifyRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.auth.VerifyStepUp(ctx, scope, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *AuthHandler) ValidateStepUp(ctx context.Context, reqCtx *app.RequestContext, scope authorizationfacade.RequestScope) {
	request, err := bindStepUpValidateRequest(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.auth.ValidateStepUpToken(ctx, scope, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func bindStepUpValidateRequest(reqCtx *app.RequestContext) (authorizationfacade.StepUpValidateRequest, error) {
	if string(reqCtx.Method()) == "POST" {
		var request authorizationfacade.StepUpValidateRequest
		if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
			return authorizationfacade.StepUpValidateRequest{}, err
		}
		return request, nil
	}
	if reqCtx.QueryArgs().Has("token") || reqCtx.QueryArgs().Has("proofToken") {
		return authorizationfacade.StepUpValidateRequest{}, apperrors.Params("step-up proof token不允许通过URL查询参数传递，请使用POST请求体")
	}
	return authorizationfacade.StepUpValidateRequest{
		ProofToken:       string(reqCtx.Query("token")),
		BusinessAction:   string(reqCtx.Query("businessAction")),
		FlowNonce:        string(reqCtx.Query("flowNonce")),
		OperationBinding: string(reqCtx.Query("operationBinding")),
		ConsumeOnce:      string(reqCtx.Query("consumeOnce")) == "true",
	}, nil
}
