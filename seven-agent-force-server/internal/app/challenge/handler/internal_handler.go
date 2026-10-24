package handler

import (
	"context"

	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

type InternalHandler struct {
	facade challengefacade.ChallengeInternalFacade
}

func NewInternalHandler(facade challengefacade.ChallengeInternalFacade) *InternalHandler {
	return &InternalHandler{facade: facade}
}

func (c *InternalHandler) Start(ctx context.Context, reqCtx *app.RequestContext) {
	var request challengefacade.StartChallengeRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.facade.StartChallenge(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}
