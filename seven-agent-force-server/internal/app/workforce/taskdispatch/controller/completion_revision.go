package controller

import (
	"context"
	completion "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

func (h *MachineHandler) SetCompletionRevisions(service completion.RevisionMachineControl) {
	h.completionRevisions = service
}

func (h *MachineHandler) applyCompletionRevision(ctx context.Context, c *app.RequestContext) {
	var r struct {
		domain.MachineEnvelope
		AttemptLeaseToken string `json:"attemptLeaseToken"`
		CycleID           int64  `json:"cycleId,string"`
	}
	if !decode(c, &r, 16<<10) || !h.auth(ctx, c, r.MachineEnvelope) {
		return
	}
	if h.completionRevisions == nil {
		writeError(c, 503, "COMPLETION_CHECK_REVISION_UNAVAILABLE", "check revision is unavailable")
		return
	}
	// Application locks and authorizes the exact source, including read-only
	// replay after its successful transaction has superseded the old Attempt.
	result, err := h.completionRevisions.ApplyPendingAtSafePoint(ctx, completionAuthority(r.MachineEnvelope), pathID(c, "attemptId"), r.AttemptLeaseToken, r.CycleID)
	if err != nil {
		writeError(c, 409, "COMPLETION_CHECK_CONFLICT", "check revision could not be applied")
		return
	}
	response.Success(c, result)
}
