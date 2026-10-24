package controller

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

func (h *MachineHandler) pendingInteractions(ctx context.Context, c *app.RequestContext) {
	var r interactionEnvelope
	if !decode(c, &r, 16<<10) || !h.interactionAuthorized(ctx, c, r) {
		return
	}
	result, err := h.interactions.OpenQuestions(ctx, r.identity(pathID(c, "attemptId")))
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	response.Success(c, result)
}
