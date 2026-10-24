package control

import (
	"context"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"net/http"
	"strconv"
)

func (c *Client) PendingInteractions(ctx context.Context, a task.Attempt, generation int64) ([]interaction.PendingQuestion, error) {
	var result []interaction.PendingQuestion
	body := merge(c.attemptEnvelope(a.Generation, a.OwnerEpoch), map[string]any{"attemptLeaseToken": a.LeaseToken, "adapterSessionGeneration": strconv.FormatInt(generation, 10)})
	err := c.call(ctx, http.MethodPost, c.attemptPath(a.ID, "/interactions/pending"), body, "", nil, &result)
	return result, err
}

func (c *Client) ReserveInteractionSuspend(ctx context.Context, a task.Attempt, generation, interactionID, currentRound int64, suspendControlKey string) (continuation.InteractionReservationResult, error) {
	return c.ReserveSuspend(ctx, a, generation, interactionID, 0, currentRound, suspendControlKey)
}

func (c *Client) ReserveOperationSuspend(ctx context.Context, a task.Attempt, generation, operationID, currentRound int64, suspendControlKey string) (continuation.InteractionReservationResult, error) {
	return c.ReserveSuspend(ctx, a, generation, 0, operationID, currentRound, suspendControlKey)
}

func (c *Client) ReserveSuspend(ctx context.Context, a task.Attempt, generation, interactionID, operationID, currentRound int64, suspendControlKey string) (continuation.InteractionReservationResult, error) {
	var result continuation.InteractionReservationResult
	body := merge(c.attemptEnvelope(a.Generation, a.OwnerEpoch), map[string]any{"attemptLeaseToken": a.LeaseToken, "adapterSessionGeneration": strconv.FormatInt(generation, 10), "interactionId": strconv.FormatInt(interactionID, 10), "operationId": strconv.FormatInt(operationID, 10), "checkpointId": "0", "checkpointIdentity": "", "currentRound": strconv.FormatInt(currentRound, 10), "suspendControlKey": suspendControlKey})
	err := c.call(ctx, http.MethodPost, c.attemptPath(a.ID, "/interactions/suspend/reserve"), body, "", nil, &result)
	return result, err
}

type InteractionBlob struct {
	ObjectKey string `json:"objectKey"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes,string"`
}
type InteractionCapturePublication struct {
	SessionGeneration, InteractionID, OperationID, CheckpointID int64
	Identity                                                    string
	Bundle, Overlay, Session, Manifest                          InteractionBlob
}

func (c *Client) PublishInteractionSuspend(ctx context.Context, a task.Attempt, p InteractionCapturePublication) error {
	body := merge(c.attemptEnvelope(a.Generation, a.OwnerEpoch), map[string]any{"attemptLeaseToken": a.LeaseToken, "adapterSessionGeneration": strconv.FormatInt(p.SessionGeneration, 10), "interactionId": strconv.FormatInt(p.InteractionID, 10), "operationId": strconv.FormatInt(p.OperationID, 10), "checkpointId": strconv.FormatInt(p.CheckpointID, 10), "checkpointIdentity": p.Identity, "bundle": p.Bundle, "overlay": p.Overlay, "session": p.Session, "manifest": p.Manifest})
	var result struct {
		Saved bool `json:"saved"`
	}
	if err := c.call(ctx, http.MethodPost, c.attemptPath(a.ID, "/interactions/suspend/publish"), body, "", nil, &result); err != nil {
		return err
	}
	if !result.Saved {
		return &APIError{ReasonCode: "INTERACTION_SUSPEND_UNAVAILABLE"}
	}
	return nil
}
