package handler

import (
	"context"

	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

// RefreshHandler owns the HTTP boundary only. It receives a narrow facade and
// has no access to Redis, RabbitMQ, outbox rows, or generic cache managers.
type RefreshHandler struct {
	refresh cachefacade.RefreshFacade
}

func NewRefreshHandler(refresh cachefacade.RefreshFacade) *RefreshHandler {
	return &RefreshHandler{refresh: refresh}
}

// Refresh accepts no body; the single protected operation is global by
// design. Its response is deliberately a safe state rather than an event ID.
func (h *RefreshHandler) Refresh(ctx context.Context, reqCtx *app.RequestContext) {
	if reqCtx == nil || len(reqCtx.Request.Body()) != 0 {
		response.Error(reqCtx, apperrors.Params("缓存刷新不接受请求体"))
		return
	}
	if h == nil || h.refresh == nil || !h.refresh.Enabled() {
		response.Error(reqCtx, apperrors.ServiceUnavailable("缓存治理不可用"))
		return
	}
	result, err := h.refresh.Refresh(ctx)
	if err != nil {
		response.Error(reqCtx, apperrors.Operation("缓存刷新提交失败"))
		return
	}
	if result.State == "DISABLED" {
		response.Error(reqCtx, apperrors.ServiceUnavailable("缓存刷新尚未启用"))
		return
	}
	if result.State == "COOLDOWN" {
		response.Error(reqCtx, apperrors.RateLimited("操作过于频繁，请稍后再试"))
		return
	}
	response.Success(reqCtx, map[string]string{"state": result.State})
}
