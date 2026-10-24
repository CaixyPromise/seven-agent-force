package handler

import (
	"context"
	"strings"

	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

type ClientHandler struct {
	facade challengefacade.ChallengeClientFacade
}

func NewClientHandler(facade challengefacade.ChallengeClientFacade) *ClientHandler {
	return &ClientHandler{facade: facade}
}

func (c *ClientHandler) Get(ctx context.Context, reqCtx *app.RequestContext) {
	challengeIdentifier := string(reqCtx.Param("challengeIdentifier"))
	result, err := c.facade.GetChallenge(ctx, challengeIdentifier)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *ClientHandler) Respond(ctx context.Context, reqCtx *app.RequestContext) {
	challengeIdentifier := string(reqCtx.Param("challengeIdentifier"))
	var request challengefacade.RespondChallengeRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.Respond(ctx, challengeIdentifier, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if challengeErr := challengeRespondError(result); challengeErr != nil {
		response.Error(reqCtx, challengeErr)
		return
	}
	response.Success(reqCtx, result)
}

func challengeRespondError(result *challengefacade.RespondChallengeResponse) error {
	if result == nil {
		return nil
	}
	if strings.TrimSpace(result.ChallengeState) == "PASSED" {
		return nil
	}
	reason := strings.TrimSpace(result.FailureReason)
	switch reason {
	case "STEP_COOLDOWN_ACTIVE", "CHALLENGE_THROTTLED":
		return apperrors.RateLimited("验证请求过于频繁，请稍后重试").WithDetails(result)
	case "STEP_LOCKED":
		return apperrors.ObjectState("当前验证方式已暂时锁定，请稍后再试").WithDetails(result)
	case "STEP_VERIFY_FAILED":
		return apperrors.Params("验证码错误，请重新输入").WithDetails(result)
	}
	if strings.TrimSpace(result.ChallengeState) == "FAILED" {
		return apperrors.ObjectState("挑战已失效，请重新发起").WithDetails(result)
	}
	return apperrors.Params("验证码错误，请重新输入").WithDetails(result)
}

func (c *ClientHandler) Refresh(ctx context.Context, reqCtx *app.RequestContext) {
	challengeIdentifier := string(reqCtx.Param("challengeIdentifier"))
	request := challengefacade.RefreshChallengeRequest{}
	if len(reqCtx.Request.Body()) > 0 {
		if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
			response.Error(reqCtx, err)
			return
		}
	}
	result, err := c.facade.Refresh(ctx, challengeIdentifier, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}
