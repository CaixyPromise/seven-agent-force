package handler

import (
	"context"
	"strconv"
	"strings"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

func (c *AdminHandler) GetEffectiveAccess(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if c.access == nil {
		response.Error(reqCtx, apperrors.System("权限解释服务未配置"))
		return
	}
	if !dataScopeAllowsUser(ctx, c.relations, reqCtx, userID) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足").WithDetails(map[string]string{"reasonCode": "DATA_SCOPE_DENIED"}))
		return
	}
	effective, err := queryOptionalBool(reqCtx, "effective")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.access.GetEffectiveAccess(ctx, userID, authorizationfacade.EffectiveAccessQuery{
		Current: queryInt64(reqCtx, "current", 1), Size: queryInt64(reqCtx, "size", 20),
		Keyword: queryString(reqCtx, "keyword"), SourceType: queryString(reqCtx, "sourceType"), Effective: effective,
	})
	write(reqCtx, result, err)
}

func (c *AdminHandler) ExplainPermission(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if c.access == nil {
		response.Error(reqCtx, apperrors.System("权限解释服务未配置"))
		return
	}
	if !dataScopeAllowsUser(ctx, c.relations, reqCtx, userID) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足").WithDetails(map[string]string{"reasonCode": "DATA_SCOPE_DENIED"}))
		return
	}
	result, err := c.access.ExplainPermission(ctx, userID, queryString(reqCtx, "permissionCode"))
	write(reqCtx, result, err)
}

func queryOptionalBool(reqCtx *app.RequestContext, key string) (*bool, error) {
	raw := strings.TrimSpace(string(reqCtx.QueryArgs().Peek(key)))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, apperrors.Params(key + "必须为布尔值")
	}
	return &value, nil
}
