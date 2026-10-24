package control

import (
	"context"
	"encoding/json"
	delivery "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"net/http"
	"strconv"
)

type InteractionPublication struct {
	Receipt             interaction.Receipt          `json:"receipt"`
	Operation           executionaccess.Receipt      `json:"operation,omitempty"`
	DeliveryIntent      delivery.IntentReceipt       `json:"deliveryIntent,omitempty"`
	ImmediateToolResult delivery.ImmediateToolResult `json:"immediateToolResult,omitempty"`
	FrameSHA256         string                       `json:"frameSha256"`
}

func (c *Client) PublishInteraction(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, frame json.RawMessage) (InteractionPublication, error) {
	var result InteractionPublication
	// merge decodes nested RawMessage into maps and reorders its object keys.
	// The hub already marshaled these immutable bytes before private spool Put;
	// preserve them so the Server hashes the exact same frame for its receipt.
	body := c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch)
	body["attemptLeaseToken"] = attempt.LeaseToken
	body["adapterSessionGeneration"] = strconv.FormatInt(sessionGeneration, 10)
	body["frame"] = frame
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/interactions/publish"), body, "", nil, &result)
	return result, err
}
func (c *Client) PullInteractions(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64) ([]interaction.Delivery, error) {
	var result []interaction.Delivery
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "adapterSessionGeneration": strconv.FormatInt(sessionGeneration, 10)})
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/interactions/pull"), body, "", nil, &result)
	return result, err
}
func (c *Client) AcknowledgeInteraction(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, ack interaction.AckRequest) (interaction.Receipt, error) {
	var result interaction.Receipt
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "adapterSessionGeneration": strconv.FormatInt(sessionGeneration, 10), "ack": ack})
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/interactions/ack"), body, "", nil, &result)
	return result, err
}
