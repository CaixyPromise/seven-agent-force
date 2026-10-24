package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	delivery "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	executionaccessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	interactiondomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
	"strconv"
	"strings"
)

func (h *MachineHandler) SetInteractions(service interaction.MachineControl) {
	h.interactions = service
}

func (h *MachineHandler) mountInteractions(r route.IRouter) {
	r.POST("/internal/workforce/v1/attempts/:attemptId/interactions/pending", h.pendingInteractions)
	r.POST("/internal/workforce/v1/attempts/:attemptId/interactions/publish", h.publishInteraction)
	r.POST("/internal/workforce/v1/attempts/:attemptId/interactions/pull", h.pullInteractions)
	r.POST("/internal/workforce/v1/attempts/:attemptId/interactions/ack", h.ackInteraction)
}

type interactionEnvelope struct {
	domain.MachineEnvelope
	AttemptLeaseToken        string `json:"attemptLeaseToken"`
	AdapterSessionGeneration int64  `json:"adapterSessionGeneration,string"`
}

func (e interactionEnvelope) identity(attempt int64) interaction.Machine {
	return interaction.Machine{AttemptID: attempt, WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID, Generation: e.Generation, OwnerEpoch: e.OwnerEpoch, ExecutionLease: e.AttemptLeaseToken, AdapterSessionGeneration: e.AdapterSessionGeneration}
}

func (h *MachineHandler) interactionAuthorized(ctx context.Context, c *app.RequestContext, e interactionEnvelope) bool {
	if !h.authAttempt(ctx, c, e.MachineEnvelope, e.AttemptLeaseToken) {
		return false
	}
	if h.interactions == nil {
		writeError(c, 503, "INTERACTION_UNAVAILABLE", "interaction is unavailable")
		return false
	}
	return true
}
func writeInteractionError(c *app.RequestContext, err error) {
	code, status := "INTERACTION_UNAVAILABLE", 503
	for _, safe := range []error{interactiondomain.ErrInvalid, interactiondomain.ErrNotFound, interactiondomain.ErrStale, interactiondomain.ErrConflict, interactiondomain.ErrNotAllowed, interactiondomain.ErrFenced, interactiondomain.ErrBusy} {
		if errors.Is(err, safe) {
			code = safe.Error()
			status = 409
			if errors.Is(err, interactiondomain.ErrInvalid) {
				status = 400
			}
			if errors.Is(err, interactiondomain.ErrNotFound) {
				status = 404
			}
			break
		}
	}
	writeError(c, status, code, "interaction request could not be completed")
}

func writeDeliveryToolError(c *app.RequestContext, err error) {
	status := 409
	code := "DELIVERY_INTENT_UNAVAILABLE"
	if errors.Is(err, deliverydomain.ErrIntentInvalid) {
		status, code = 400, deliverydomain.ErrIntentInvalid.Error()
	} else if errors.Is(err, deliverydomain.ErrIntentNotAvailable) {
		code = err.Error()
	} else if errors.Is(err, deliverydomain.ErrDecisionConflict) {
		code = deliverydomain.ErrDecisionConflict.Error()
	}
	writeError(c, status, code, "Git delivery request could not be scheduled")
}

func (h *MachineHandler) publishInteraction(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Frame json.RawMessage `json:"frame"`
	}
	if !decode(c, &req, interactionPublishEnvelopeBytes) || !h.interactionAuthorized(ctx, c, req.interactionEnvelope) {
		return
	}
	frame, err := agentprotocol.Decode(append(append([]byte(nil), req.Frame...), '\n'), "up")
	if err != nil {
		writeInteractionError(c, interactiondomain.ErrInvalid)
		return
	}
	identity := req.identity(pathID(c, "attemptId"))
	var result interaction.Receipt
	var operation executionaccess.Receipt
	var intent delivery.IntentReceipt
	var immediate delivery.ImmediateToolResult
	switch frame.Method {
	case "interaction.request":
		var payload struct {
			RoundKey       string                       `json:"roundKey"`
			InteractionKey string                       `json:"interactionKey"`
			Questions      []interactiondomain.Question `json:"questions"`
			IsBlocking     bool                         `json:"isBlocking"`
		}
		if json.Unmarshal(frame.Payload, &payload) != nil || payload.RoundKey == "" {
			writeInteractionError(c, interactiondomain.ErrInvalid)
			return
		}
		result, err = h.interactions.Publish(ctx, identity, interaction.PublishRequest{RoundKey: payload.RoundKey, LocalRequestKey: payload.InteractionKey, Questions: payload.Questions, IsBlocking: payload.IsBlocking})
	case "interaction.closed":
		var payload struct {
			RoundKey       string `json:"roundKey"`
			InteractionKey string `json:"interactionKey"`
			Reason         string `json:"reason"`
		}
		if json.Unmarshal(frame.Payload, &payload) != nil {
			writeInteractionError(c, interactiondomain.ErrInvalid)
			return
		}
		result, err = h.interactions.Close(ctx, identity, interaction.CloseRequest{RoundKey: payload.RoundKey, LocalRequestKey: payload.InteractionKey, Reason: payload.Reason})
	case "interaction.received", "message.received":
		var payload struct {
			RoundKey       string `json:"roundKey"`
			InteractionKey string `json:"interactionKey"`
			MessageKey     string `json:"messageKey"`
			ResponseID     string `json:"responseId"`
			Status         string `json:"status"`
		}
		if json.Unmarshal(frame.Payload, &payload) != nil {
			writeInteractionError(c, interactiondomain.ErrInvalid)
			return
		}
		kind, key := interactiondomain.MessageKind, payload.MessageKey
		var responseID int64
		if frame.Method == "interaction.received" {
			kind, key = interactiondomain.QuestionKind, payload.InteractionKey
			responseID, err = strconv.ParseInt(payload.ResponseID, 10, 64)
			if err != nil || responseID <= 0 {
				writeInteractionError(c, interactiondomain.ErrInvalid)
				return
			}
		}
		result, err = h.interactions.PublishReceipt(ctx, identity, interaction.PublishReceiptRequest{LocalRequestKey: key, RoundKey: payload.RoundKey, ResponseID: responseID, Kind: kind, Status: payload.Status})
	case "message", "plan", "activity", "execution.suspended":
		kind, text, projectErr := interaction.ProjectAdapterEvent(frame.Method, frame.Payload)
		if projectErr != nil {
			writeInteractionError(c, projectErr)
			return
		}
		var item interactiondomain.Item
		item, err = h.interactions.PublishEvent(ctx, identity, interaction.PublishEventRequest{LocalRequestKey: frame.Key, Kind: kind, Text: text})
		result = interaction.Receipt{Version: item.Version, Saved: err == nil}
	case "tool.catalog":
		var payload agentprotocol.ToolCatalogPayload
		if json.Unmarshal(frame.Payload, &payload) != nil {
			err = interactiondomain.ErrInvalid
			break
		}
		var item interactiondomain.Item
		item, err = h.interactions.PublishEvent(ctx, identity, interaction.PublishEventRequest{LocalRequestKey: frame.Key, Kind: "ACTIVITY", Text: "Platform tools ready: " + strings.Join(payload.ToolNames, ", ")})
		result = interaction.Receipt{Version: item.Version, Saved: err == nil}
	case "tool.request":
		var payload agentprotocol.ToolRequestPayload
		if json.Unmarshal(frame.Payload, &payload) != nil {
			err = executionaccessdomain.ErrInvalid
			break
		}
		if payload.ToolName == agentprotocol.GitDeliveryToolName {
			if h.deliveryTools == nil {
				err = deliverydomain.ErrIntentNotAvailable
				break
			}
			intent, immediate, err = h.deliveryTools.PublishIntent(ctx, deliveryToolMachine(req.interactionEnvelope, pathID(c, "attemptId")), delivery.PublishIntentRequest{RoundKey: payload.RoundKey, LocalToolCallKey: payload.LocalToolCallKey, NativeToolCallID: payload.NativeToolCallID, ArgumentsDigest: payload.ArgumentsDigest, Arguments: payload.Arguments})
			break
		}
		if h.executionAccess == nil || payload.ToolName != agentprotocol.NetworkCommandToolName {
			err = executionaccessdomain.ErrUnavailable
			break
		}
		var arguments agentprotocol.NetworkCommandArguments
		if json.Unmarshal(payload.Arguments, &arguments) != nil {
			err = executionaccessdomain.ErrInvalid
			break
		}
		targets := make([]executionaccessdomain.Target, 0, len(arguments.Targets))
		for _, target := range arguments.Targets {
			targets = append(targets, executionaccessdomain.Target{Host: target.Host, Protocol: target.Protocol, Port: target.Port})
		}
		operation, err = h.executionAccess.Publish(ctx, executionAccessMachine(req.interactionEnvelope, pathID(c, "attemptId")), executionaccess.PublishRequest{RoundKey: payload.RoundKey, LocalToolCallKey: payload.LocalToolCallKey, NativeToolCallID: payload.NativeToolCallID, ToolName: payload.ToolName, ArgumentsDigest: payload.ArgumentsDigest, Arguments: executionaccessdomain.Descriptor{Purpose: arguments.Purpose, Command: arguments.Command, Cwd: arguments.CWD, Targets: targets}})
	case "tool.received":
		var payload agentprotocol.ToolReceiptPayload
		if json.Unmarshal(frame.Payload, &payload) != nil || payload.Status != "RECEIVED" {
			err = executionaccessdomain.ErrInvalid
			break
		}
		if h.deliveryTools != nil {
			intent, err = h.deliveryTools.AckIntent(ctx, deliveryToolMachine(req.interactionEnvelope, pathID(c, "attemptId")), delivery.AckIntentRequest{IntentID: payload.OperationID})
			if err == nil {
				break
			}
			if !errors.Is(err, deliverydomain.ErrIntentNotAvailable) {
				break
			}
		}
		if h.executionAccess == nil {
			err = executionaccessdomain.ErrUnavailable
			break
		}
		err = nil
		operation, err = h.executionAccess.AckResult(ctx, executionAccessMachine(req.interactionEnvelope, pathID(c, "attemptId")), executionaccess.ResultAckRequest{OperationID: payload.OperationID, NativeResolved: true})
	case "tool.closed":
		var payload agentprotocol.ToolClosedPayload
		if json.Unmarshal(frame.Payload, &payload) != nil {
			err = executionaccessdomain.ErrInvalid
			break
		}
		if h.deliveryTools != nil {
			intent, err = h.deliveryTools.CloseIntent(ctx, deliveryToolMachine(req.interactionEnvelope, pathID(c, "attemptId")), delivery.CloseIntentRequest{RoundKey: payload.RoundKey, LocalToolCallKey: payload.LocalToolCallKey, Reason: payload.Reason})
			if err == nil {
				break
			}
			if !errors.Is(err, deliverydomain.ErrIntentNotAvailable) {
				break
			}
		}
		if h.executionAccess == nil {
			err = executionaccessdomain.ErrUnavailable
			break
		}
		err = nil
		operation, err = h.executionAccess.CloseToolCall(ctx, executionAccessMachine(req.interactionEnvelope, pathID(c, "attemptId")), executionaccess.CloseRequest{RoundKey: payload.RoundKey, LocalToolCallKey: payload.LocalToolCallKey})
	default:
		err = interactiondomain.ErrInvalid
	}
	if err != nil {
		if frame.Method == "tool.request" || frame.Method == "tool.received" || frame.Method == "tool.closed" {
			if errors.Is(err, deliverydomain.ErrIntentInvalid) || errors.Is(err, deliverydomain.ErrIntentNotAvailable) || errors.Is(err, deliverydomain.ErrDecisionConflict) {
				writeDeliveryToolError(c, err)
			} else {
				writeExecutionAccessError(c, err)
			}
		} else {
			writeInteractionError(c, err)
		}
		return
	}
	sum := sha256.Sum256(req.Frame)
	response.Success(c, struct {
		Receipt             interaction.Receipt          `json:"receipt"`
		Operation           executionaccess.Receipt      `json:"operation,omitempty"`
		DeliveryIntent      delivery.IntentReceipt       `json:"deliveryIntent,omitempty"`
		ImmediateToolResult delivery.ImmediateToolResult `json:"immediateToolResult,omitempty"`
		FrameSHA256         string                       `json:"frameSha256"`
	}{result, operation, intent, immediate, hex.EncodeToString(sum[:])})
}

func deliveryToolMachine(e interactionEnvelope, attemptID int64) delivery.Machine {
	return delivery.Machine{WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID, AttemptID: attemptID, Generation: e.Generation, OwnerEpoch: e.OwnerEpoch, ExecutionLease: e.AttemptLeaseToken, AdapterSessionGeneration: e.AdapterSessionGeneration}
}

// JSON transport escaping can expand one source byte to six bytes. The nested
// protocol decoder independently enforces MaxFrameBytes including its newline.
const interactionPublishEnvelopeBytes = 6*agentprotocol.MaxFrameBytes + (16 << 10)

func (h *MachineHandler) pullInteractions(ctx context.Context, c *app.RequestContext) {
	var req interactionEnvelope
	if !decode(c, &req, 16<<10) || !h.interactionAuthorized(ctx, c, req) {
		return
	}
	items, err := h.interactions.Pull(ctx, req.identity(pathID(c, "attemptId")), 50)
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	response.Success(c, items)
}
func (h *MachineHandler) ackInteraction(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Ack interaction.AckRequest `json:"ack"`
	}
	if !decode(c, &req, 32<<10) || !h.interactionAuthorized(ctx, c, req.interactionEnvelope) {
		return
	}
	result, err := h.interactions.Ack(ctx, req.identity(pathID(c, "attemptId")), req.Ack)
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	response.Success(c, result)
}
