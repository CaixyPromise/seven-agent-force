package controller

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
	"strconv"
)

func (h *Handler) questions(ctx context.Context, c *app.RequestContext) {
	u, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	var cursor int64
	if raw := string(c.Query("cursor")); raw != "" {
		cursor, e = positive(raw)
		if e != nil {
			problem(c, e)
			return
		}
	}
	limit := 20
	if raw := string(c.Query("limit")); raw != "" {
		limit, e = strconv.Atoi(raw)
		if e != nil || limit < 1 || limit > 50 {
			problem(c, domain.ErrInvalid)
			return
		}
	}
	out, e := h.service.Questions(ctx, u, cursor, limit)
	if e != nil {
		problem(c, e)
		return
	}
	response.Success(c, out)
}
