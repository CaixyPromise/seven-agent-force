package controller

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

type evaluationReplayRequest struct {
	domain.MachineEnvelope
	AttemptLeaseToken string                          `json:"attemptLeaseToken"`
	Pending           facade.EvaluationReplayIdentity `json:"pending"`
}

func (h *MachineHandler) readEvaluation(ctx context.Context, c *app.RequestContext) {
	var req evaluationReplayRequest
	if !decode(c, &req, 64<<10) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	if h.processors == nil {
		writeError(c, 503, "RESULT_PROCESSOR_UNAVAILABLE", "result processor runtime is unavailable")
		return
	}
	result, err := h.processors.ReadEvaluation(ctx, processorIdentity(req.MachineEnvelope), pathID(c, "attemptId"), req.AttemptLeaseToken, req.Pending)
	if err != nil {
		writeProcessorError(c, err)
		return
	}
	response.Success(c, result)
}
