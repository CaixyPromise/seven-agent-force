package controller

import (
	"context"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

func (h *MachineHandler) SetInteractionSuspend(service *continuation.InteractionSuspendService) {
	h.interactionSuspend = service
}

func (h *MachineHandler) mountInteractionSuspend(r route.IRouter) {
	r.POST("/internal/workforce/v1/attempts/:attemptId/interactions/suspend/reserve", h.reserveInteractionSuspend)
	r.POST("/internal/workforce/v1/attempts/:attemptId/interactions/suspend/publish", h.publishInteractionSuspend)
}

type suspendEnvelope struct {
	interactionEnvelope
	InteractionID      int64  `json:"interactionId,string"`
	OperationID        int64  `json:"operationId,string"`
	CheckpointID       int64  `json:"checkpointId,string"`
	CheckpointIdentity string `json:"checkpointIdentity"`
}

func (h *MachineHandler) suspendIdentity(ctx context.Context, c *app.RequestContext, r suspendEnvelope) (continuation.InteractionCaptureRequest, bool) {
	if !h.interactionAuthorized(ctx, c, r.interactionEnvelope) {
		return continuation.InteractionCaptureRequest{}, false
	}
	if h.interactionSuspend == nil {
		writeError(c, 503, "INTERACTION_UNAVAILABLE", "suspension is unavailable")
		return continuation.InteractionCaptureRequest{}, false
	}
	attempt := pathID(c, "attemptId")
	target, err := h.store.AttemptContext(ctx, attempt)
	if err != nil {
		writeError(c, 409, "INTERACTION_FENCED", "execution is no longer current")
		return continuation.InteractionCaptureRequest{}, false
	}
	return continuation.InteractionCaptureRequest{
		MachineAuthority: continuation.MachineAuthority{WorkerID: r.WorkerID, InstanceID: r.InstanceID, HostBootID: r.HostBootID, AttemptID: attempt, LeaseToken: r.AttemptLeaseToken, Generation: r.Generation, OwnerEpoch: r.OwnerEpoch},
		TenantID:         target.TenantID, TaskID: target.TaskID, TaskTurnID: target.TaskTurnID, InteractionID: r.InteractionID, OperationID: r.OperationID, CheckpointID: r.CheckpointID, CheckpointIdentity: r.CheckpointIdentity,
	}, true
}

func (h *MachineHandler) reserveInteractionSuspend(ctx context.Context, c *app.RequestContext) {
	var r struct {
		suspendEnvelope
		SessionDigest     string `json:"sessionDigest"`
		CurrentRound      int64  `json:"currentRound,string"`
		SuspendControlKey string `json:"suspendControlKey"`
	}
	if !decode(c, &r, 32<<10) {
		return
	}
	identity, ok := h.suspendIdentity(ctx, c, r.suspendEnvelope)
	if !ok {
		return
	}
	result, err := h.interactionSuspend.ReserveInteractionSuspend(ctx, continuation.ReserveInteractionRequest{InteractionCaptureRequest: identity, AdapterSessionGeneration: r.AdapterSessionGeneration, SessionDigest: r.SessionDigest, CurrentRound: r.CurrentRound, SuspendControlKey: r.SuspendControlKey})
	if err != nil {
		writeError(c, 409, "INTERACTION_SUSPEND_UNAVAILABLE", "progress could not be saved")
		return
	}
	response.Success(c, result)
}

type suspendedBlob struct {
	ObjectKey string `json:"objectKey"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes,string"`
}

func (b suspendedBlob) value() continuation.InteractionEncryptedBlob {
	return continuation.InteractionEncryptedBlob{ObjectKey: b.ObjectKey, SHA256: b.SHA256, SizeBytes: b.SizeBytes}
}
func (h *MachineHandler) publishInteractionSuspend(ctx context.Context, c *app.RequestContext) {
	var r struct {
		suspendEnvelope
		Bundle   suspendedBlob `json:"bundle"`
		Overlay  suspendedBlob `json:"overlay"`
		Session  suspendedBlob `json:"session"`
		Manifest suspendedBlob `json:"manifest"`
	}
	if !decode(c, &r, 32<<10) {
		return
	}
	identity, ok := h.suspendIdentity(ctx, c, r.suspendEnvelope)
	if !ok {
		return
	}
	err := h.interactionSuspend.PublishInteractionSuspend(ctx, continuation.PublishInteractionRequest{InteractionCaptureRequest: identity, Bundle: r.Bundle.value(), Overlay: r.Overlay.value(), Session: r.Session.value(), Manifest: r.Manifest.value()})
	if err != nil {
		writeError(c, 409, "INTERACTION_SUSPEND_UNAVAILABLE", "progress could not be saved")
		return
	}
	response.Success(c, map[string]bool{"saved": true})
}
