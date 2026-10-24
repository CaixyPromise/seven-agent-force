package handler

import (
	"context"

	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

// ListUserOptions returns the initial bounded option set for user selectors.
func (c *Handler) ListUserOptions(ctx context.Context, reqCtx *app.RequestContext) {
	c.listUserOptions(ctx, reqCtx)
}

// SearchUsers searches the bounded option set for user selectors.
func (c *Handler) SearchUsers(ctx context.Context, reqCtx *app.RequestContext) {
	c.listUserOptions(ctx, reqCtx)
}

// GetSimpleUser returns a minimum user projection visible to the caller.
func (c *Handler) GetSimpleUser(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.selectors.GetSimpleUser(ctx, userID, dataScopeFilter(reqCtx))
	write(reqCtx, result, err)
}

func (c *Handler) listUserOptions(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.selectors.ListUserOptions(ctx, userfacade.UserSelectorQuery{
		Keyword: queryString(reqCtx, "keyword"),
		Limit:   int(queryInt64(reqCtx, "limit", defaultUserSelectorLimit)),
		DeptID:  queryInt64(reqCtx, "deptId", 0),
		Scope:   dataScopeFilter(reqCtx),
	})
	write(reqCtx, result, err)
}

const defaultUserSelectorLimit = 20
