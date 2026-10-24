package handler

import (
	"context"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

type InternalHandler struct {
	auth authorizationfacade.AuthFacade
}

func NewInternalHandler(auth authorizationfacade.AuthFacade) *InternalHandler {
	return &InternalHandler{auth: auth}
}

func (c *InternalHandler) GetUser(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parsePathInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.auth.GetUserVO(ctx, userID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *InternalHandler) RefreshPermissionCache(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parsePathInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.auth.RefreshUserPermissionCache(ctx, userID); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}
