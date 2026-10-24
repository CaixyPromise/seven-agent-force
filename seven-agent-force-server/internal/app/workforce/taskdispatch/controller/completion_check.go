package controller

import (
	"context"
	completion "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
)

func (h *MachineHandler) SetCompletionChecks(control completion.MachineControl) {
	h.completionChecks = control
}
func completionAuthority(e domain.MachineEnvelope) completion.MachineAuthority {
	return completion.MachineAuthority{WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID, Generation: e.Generation, OwnerEpoch: e.OwnerEpoch}
}
func (h *MachineHandler) recordCompletionCheck(ctx context.Context, c *app.RequestContext) {
	var req struct {
		domain.MachineEnvelope
		AttemptLeaseToken string              `json:"attemptLeaseToken"`
		Evidence          completion.Evidence `json:"evidence"`
	}
	if !decode(c, &req, 128<<10) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	if h.completionChecks == nil {
		writeError(c, 503, "COMPLETION_CHECK_UNAVAILABLE", "completion check unavailable")
		return
	}
	result, err := h.completionChecks.RecordMachineEvidence(ctx, completionAuthority(req.MachineEnvelope), pathID(c, "attemptId"), req.AttemptLeaseToken, req.Evidence)
	if err != nil {
		writeError(c, 409, "COMPLETION_CHECK_CONFLICT", "completion check authority or evidence conflict")
		return
	}
	response.Success(c, result)
}
func (h *MachineHandler) readCompletionCheck(ctx context.Context, c *app.RequestContext) {
	var req struct {
		domain.MachineEnvelope
		AttemptLeaseToken string `json:"attemptLeaseToken"`
		Phase             string `json:"phase"`
		CycleID           int64  `json:"cycleId,string"`
		Digest            string `json:"digest"`
	}
	if !decode(c, &req, 16<<10) || !h.authAttempt(ctx, c, req.MachineEnvelope, req.AttemptLeaseToken) {
		return
	}
	if h.completionChecks == nil {
		writeError(c, 503, "COMPLETION_CHECK_UNAVAILABLE", "completion check unavailable")
		return
	}
	result, err := h.completionChecks.ReadMachineEvidence(ctx, completionAuthority(req.MachineEnvelope), pathID(c, "attemptId"), req.AttemptLeaseToken, req.Phase, req.CycleID, req.Digest)
	if err != nil {
		writeError(c, 409, "COMPLETION_CHECK_CONFLICT", "completion check authority conflict")
		return
	}
	response.Success(c, result)
}
