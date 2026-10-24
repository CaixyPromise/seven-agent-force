package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type Handler struct {
	profiles  userfacade.ProfileFacade
	accounts  userfacade.AccountFacade
	selectors userfacade.UserSelectorFacade
	auth      authorizationfacade.AuthFacade
	domain    *domain.Service
}

type UpdateProfileRequest struct {
	NickName    *string `json:"nickName,omitempty"`
	UserPhone   *string `json:"userPhone,omitempty"`
	UserProfile *string `json:"userProfile,omitempty"`
}

type UpdateEmailRequest struct {
	UserEmail            string `json:"userEmail" validate:"required"`
	ExpectedEmailVersion int64  `json:"expectedEmailVersion,string" validate:"required"`
}

type ChangePasswordRequest struct {
	OldPassword     string `json:"oldPassword" validate:"required"`
	NewPassword     string `json:"newPassword" validate:"required"`
	ConfirmPassword string `json:"confirmPassword" validate:"required"`
}

type CommitAvatarRequest struct {
	FileID int64 `json:"fileId" validate:"required"`
}

func NewHandler(profiles userfacade.ProfileFacade, accounts userfacade.AccountFacade, auth authorizationfacade.AuthFacade, selectors ...userfacade.UserSelectorFacade) *Handler {
	handler := &Handler{profiles: profiles, accounts: accounts, auth: auth, domain: domain.NewService()}
	if len(selectors) > 0 {
		handler.selectors = selectors[0]
	}
	return handler
}

func (c *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c == nil {
		return
	}
	c.auth = auth
}

func (c *Handler) GetCurrentUserProfile(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.profiles.GetProfileByUserID(ctx, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) UpdateCurrentUserProfile(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request UpdateProfileRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if bytes := reqCtx.Request.Body(); strings.Contains(string(bytes), "userAvatar") {
		response.Error(reqCtx, apperrors.Forbidden("头像必须通过文件绑定接口更新"))
		return
	}
	if requiresPhoneStepUp(request.UserPhone) {
		profile, profileErr := c.profiles.GetProfileByUserID(ctx, userID)
		if profileErr != nil {
			response.Error(reqCtx, profileErr)
			return
		}
		if stepUpErr := c.ensureSensitiveOperation(
			ctx,
			reqCtx,
			string(challengedomain.BusinessActionProfilePhoneUpdate),
			c.domain.BuildOperationBinding("phone", strings.TrimSpace(*request.UserPhone)),
			func(flowNonce string) (*authorizationfacade.StepUpChallengeVO, error) {
				scope, scopeErr := buildRequestScope(reqCtx)
				if scopeErr != nil {
					return nil, scopeErr
				}
				return c.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
					BusinessAction:   string(challengedomain.BusinessActionProfilePhoneUpdate),
					FlowNonce:        flowNonce,
					OperationBinding: c.domain.BuildOperationBinding("phone", strings.TrimSpace(*request.UserPhone)),
				})
			},
			func() bool {
				return profile != nil && strings.TrimSpace(*request.UserPhone) != "" && strings.TrimSpace(*request.UserPhone) != strings.TrimSpace(profile.Phone)
			},
		); stepUpErr != nil {
			response.Error(reqCtx, stepUpErr)
			return
		}
	}
	err = c.profiles.UpdateSelfProfile(ctx, userfacade.UpdateSelfProfileCommand{
		UserID:      userID,
		NickName:    request.NickName,
		UserPhone:   request.UserPhone,
		UserProfile: request.UserProfile,
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) UpdateCurrentUserEmail(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request UpdateEmailRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.ensureSensitiveOperation(
		ctx,
		reqCtx,
		string(challengedomain.BusinessActionProfileEmailUpdate),
		c.domain.BuildOperationBinding("email", strings.TrimSpace(request.UserEmail)),
		func(flowNonce string) (*authorizationfacade.StepUpChallengeVO, error) {
			scope, scopeErr := buildRequestScope(reqCtx)
			if scopeErr != nil {
				return nil, scopeErr
			}
			return c.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
				BusinessAction:   string(challengedomain.BusinessActionProfileEmailUpdate),
				FlowNonce:        flowNonce,
				OperationBinding: c.domain.BuildOperationBinding("email", strings.TrimSpace(request.UserEmail)),
				TargetEmail:      strings.TrimSpace(request.UserEmail),
			})
		},
		func() bool { return true },
	); err != nil {
		response.Error(reqCtx, err)
		return
	}
	err = c.profiles.UpdateSelfEmail(ctx, userfacade.UpdateSelfEmailCommand{
		UserID:               userID,
		UserEmail:            request.UserEmail,
		ExpectedEmailVersion: request.ExpectedEmailVersion,
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) CommitCurrentUserAvatar(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request CommitAvatarRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	ctx = securitycontext.WithUser(ctx, securitycontext.Get(reqCtx))
	avatar, err := c.profiles.CommitCurrentUserAvatar(ctx, userID, request.FileID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, map[string]string{"userAvatar": avatar})
}

func (c *Handler) ChangeCurrentUserPassword(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request ChangePasswordRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if request.NewPassword != request.ConfirmPassword {
		response.Error(reqCtx, userParams("新密码与确认密码不一致"))
		return
	}
	ok, err := c.accounts.VerifyPassword(ctx, userID, request.OldPassword)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !ok {
		response.Error(reqCtx, userParams("当前密码错误"))
		return
	}
	binding := currentUserPasswordChangeBinding(userID)
	if err := c.ensureSensitiveOperation(
		ctx,
		reqCtx,
		string(challengedomain.BusinessActionCurrentUserPasswordChange),
		binding,
		func(flowNonce string) (*authorizationfacade.StepUpChallengeVO, error) {
			scope, scopeErr := buildRequestScope(reqCtx)
			if scopeErr != nil {
				return nil, scopeErr
			}
			return c.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
				BusinessAction:   string(challengedomain.BusinessActionCurrentUserPasswordChange),
				FlowNonce:        flowNonce,
				OperationBinding: binding,
			})
		},
		func() bool { return true },
	); err != nil {
		response.Error(reqCtx, err)
		return
	}
	err = c.accounts.UpdatePassword(ctx, userfacade.UpdatePasswordCommand{
		UserID:      userID,
		RawPassword: request.NewPassword,
		OperatorID:  userID,
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func currentUserPasswordChangeBinding(userID int64) string {
	return "user:" + strconv.FormatInt(userID, 10) + "|change-password"
}

func requireCurrentUserID(reqCtx *app.RequestContext) (int64, error) {
	userID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok || userID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return userID, nil
}

func buildRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	userID, err := requireCurrentUserID(reqCtx)
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

func (c *Handler) ensureSensitiveOperation(
	ctx context.Context,
	reqCtx *app.RequestContext,
	businessAction string,
	operationBinding string,
	createChallenge func(flowNonce string) (*authorizationfacade.StepUpChallengeVO, error),
	requiresStepUp func() bool,
) error {
	if !requiresStepUp() {
		return nil
	}
	if c.auth == nil {
		return apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildRequestScope(reqCtx)
	if err != nil {
		return err
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
			return err
		}
		if token == nil {
			return apperrors.Forbidden("step-up proof验证失败")
		}
		securitycontext.SetStepUpProofAudit(reqCtx, stepUpProofAuditFromToken(token, businessAction, operationBinding))
		return nil
	}
	challenge, err := createChallenge(flowNonce)
	if err != nil {
		return err
	}
	return apperrors.ChallengeRequired("", map[string]any{
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

func requiresPhoneStepUp(phone *string) bool {
	return phone != nil && strings.TrimSpace(*phone) != ""
}

func chooseFlowNonce(flowNonce, businessAction string) string {
	value := strings.TrimSpace(flowNonce)
	if value == "" {
		return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return value
}

func userParams(message string) error {
	return apperrors.Params(message)
}
