package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	delivery "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	interactiondomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/interactionspool"
)

func TestInteractionRemoteErrorPreservesOnlyStableReasonCode(t *testing.T) {
	err := interactionRemoteError(&control.APIError{StatusCode: 400, ReasonCode: "EXECUTION_OPERATION_INVALID", Message: "raw detail"})
	if !errors.Is(err, errInteractionBinding) || !strings.Contains(err.Error(), "EXECUTION_OPERATION_INVALID") || strings.Contains(err.Error(), "raw detail") {
		t.Fatalf("unexpected safe remote error: %v", err)
	}
	err = interactionRemoteError(&control.APIError{StatusCode: 400, ReasonCode: "unsafe detail", Message: "raw detail"})
	if !errors.Is(err, errInteractionBinding) || strings.Contains(err.Error(), "unsafe") || strings.Contains(err.Error(), "raw detail") {
		t.Fatalf("unsafe remote detail escaped: %v", err)
	}
}

type pumpTestControl struct {
	publish  func(context.Context, json.RawMessage) (control.InteractionPublication, error)
	pull     func(context.Context) ([]interaction.Delivery, error)
	ack      func(context.Context, interaction.AckRequest) (interaction.Receipt, error)
	toolPull func(context.Context) ([]executionaccess.ToolResult, error)
	toolAck  func(context.Context, executionaccess.ResultAckRequest) (executionaccess.Receipt, error)
}

func (c pumpTestControl) PublishInteraction(ctx context.Context, a taskdomain.Attempt, g int64, f json.RawMessage) (control.InteractionPublication, error) {
	return c.publish(ctx, f)
}
func (c pumpTestControl) PullInteractions(ctx context.Context, a taskdomain.Attempt, g int64) ([]interaction.Delivery, error) {
	if c.pull == nil {
		return nil, nil
	}
	return c.pull(ctx)
}
func (c pumpTestControl) AcknowledgeInteraction(ctx context.Context, a taskdomain.Attempt, g int64, r interaction.AckRequest) (interaction.Receipt, error) {
	if c.ack == nil {
		return interaction.Receipt{}, errors.New("unexpected ack")
	}
	return c.ack(ctx, r)
}
func (c pumpTestControl) PullExecutionOperationResults(ctx context.Context, a taskdomain.Attempt, g int64, limit int) ([]executionaccess.ToolResult, error) {
	if c.toolPull == nil {
		return nil, nil
	}
	return c.toolPull(ctx)
}
func (c pumpTestControl) AckExecutionOperationResult(ctx context.Context, a taskdomain.Attempt, g int64, r executionaccess.ResultAckRequest) (executionaccess.Receipt, error) {
	if c.toolAck == nil {
		return executionaccess.Receipt{}, errors.New("unexpected tool ack")
	}
	return c.toolAck(ctx, r)
}

func (c pumpTestControl) AckColdExecutionOperationResult(ctx context.Context, a taskdomain.Attempt, sourceGeneration, operationID int64, r executionaccess.ResultAckRequest) (executionaccess.Receipt, error) {
	return c.AckExecutionOperationResult(ctx, a, sourceGeneration, r)
}

type pumpTestChannel struct {
	state         func(context.Context) (attemptruntime.InteractionExecutionState, error)
	notifications func(context.Context, int64) (attemptruntime.InteractionBatch, error)
	confirm       func(context.Context, int64) error
	send          func(context.Context, string, int64, string, json.RawMessage) error
}

func (c pumpTestChannel) State(ctx context.Context) (attemptruntime.InteractionExecutionState, error) {
	if c.state != nil {
		return c.state(ctx)
	}
	return attemptruntime.InteractionExecutionState{State: "RUNNING", RoundKey: "round:1", SessionGeneration: 1}, nil
}
func (c pumpTestChannel) Notifications(ctx context.Context, after int64) (attemptruntime.InteractionBatch, error) {
	if c.notifications == nil {
		return attemptruntime.InteractionBatch{Through: after}, nil
	}
	return c.notifications(ctx, after)
}
func (c pumpTestChannel) ConfirmSpooled(ctx context.Context, through int64) error {
	if c.confirm == nil {
		return nil
	}
	return c.confirm(ctx, through)
}
func (c pumpTestChannel) Send(ctx context.Context, round string, g int64, method string, body json.RawMessage) error {
	if c.send == nil {
		return errors.New("unexpected send")
	}
	return c.send(ctx, round, g, method, body)
}

func pumpPath(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "interaction-spool")
}
func pumpHub(t *testing.T, path string, client pumpTestControl) *interactionTransportHub {
	t.Helper()
	h, err := newInteractionTransportHub(context.Background(), path, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

var pumpAttempt = taskdomain.Attempt{TaskID: 1, ID: 2, Generation: 3, OwnerEpoch: 4, LeaseToken: "test-lease"}
var pumpIdentity = interactionspool.Identity{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, CurrentRound: "round:1", SessionGeneration: 1}

func pumpFor(h *interactionTransportHub, c pumpTestChannel) *interactionPump {
	return &interactionPump{hub: h, key: interactionExecutionKey{1, 2, 3, 4}, attempt: pumpAttempt, channel: c, sent: make(map[interactionDeliveryKey]string), toolSent: make(map[toolResultDeliveryKey]string), toolResolved: make(map[int64]bool)}
}
func pumpFrame(t *testing.T, method string, payload any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: method, Key: "event:1", ReplyTo: "control:1", Payload: body})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func pumpQuestion(t *testing.T) json.RawMessage {
	return pumpFrame(t, "interaction.request", agentprotocol.QuestionRequest{RoundKey: "round:1", InteractionKey: "question:1", IsBlocking: true, Questions: []agentprotocol.Question{{Key: "q", Title: "Choose", Description: "", Kind: "TEXT", Required: true, Options: []agentprotocol.QuestionOption{}, AllowFreeText: true}}})
}
func pumpReceipt(raw json.RawMessage) control.InteractionPublication {
	sum := sha256.Sum256(raw)
	return control.InteractionPublication{FrameSHA256: hex.EncodeToString(sum[:]), Receipt: interaction.Receipt{Saved: true, InteractionID: 10, ResponseID: 11}}
}
func pumpBatch(frame json.RawMessage, seq int64) attemptruntime.InteractionBatch {
	return attemptruntime.InteractionBatch{Notifications: []attemptruntime.InteractionNotification{{Sequence: seq, RoundKey: "round:1", SessionGeneration: 1, Frame: frame}}, Through: seq}
}

func TestInteractionTransportToolLifecycleReachesAuthenticatedServerBoundary(t *testing.T) {
	args, err := json.Marshal(agentprotocol.NetworkCommandArguments{Purpose: "install dependencies", Command: "pnpm install --frozen-lockfile", CWD: ".", Targets: []agentprotocol.ToolTarget{{Host: "registry.npmjs.org", Protocol: "https", Port: 443}}})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := agentprotocol.DigestToolArguments(args)
	if err != nil {
		t.Fatal(err)
	}
	frames := []json.RawMessage{
		pumpFrame(t, "tool.request", agentprotocol.ToolRequestPayload{ToolName: agentprotocol.NetworkCommandToolName, LocalToolCallKey: "tool:1", NativeToolCallID: "native:1", RoundKey: "round:1", Arguments: args, ArgumentsDigest: digest}),
		pumpFrame(t, "tool.received", agentprotocol.ToolReceiptPayload{RoundKey: "round:1", LocalToolCallKey: "tool:1", OperationID: 91, Status: "RECEIVED"}),
		pumpFrame(t, "activity", map[string]any{"roundKey": "round:1", "text": "continuing after dependency installation"}),
		pumpFrame(t, "tool.closed", agentprotocol.ToolClosedPayload{RoundKey: "round:1", LocalToolCallKey: "tool:1", Reason: "COMPLETED"}),
	}
	methods := make([]string, 0, len(frames))
	requestSaved, resultSent := false, false
	acks := []executionaccess.ResultAckRequest{}
	var h *interactionTransportHub
	h = pumpHub(t, pumpPath(t), pumpTestControl{
		publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
			frame, decodeErr := agentprotocol.Decode(append(append([]byte(nil), raw...), '\n'), "up")
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			methods = append(methods, frame.Method)
			result := pumpReceipt(raw)
			switch frame.Method {
			case "tool.request":
				if pendingCount(t, h) != 1 {
					t.Fatal("tool request bypassed private spool persistence")
				}
				requestSaved = true
				result.Operation = executionaccess.Receipt{OperationID: 91, Version: 1, Status: "WAITING_APPROVAL"}
			case "tool.received", "tool.closed":
				result.Operation = executionaccess.Receipt{OperationID: 91, Version: 4, Status: "SUCCEEDED"}
			}
			return result, nil
		},
		toolPull: func(context.Context) ([]executionaccess.ToolResult, error) {
			if requestSaved && !resultSent {
				code := 0
				return []executionaccess.ToolResult{{OperationID: 91, RoundKey: "round:1", LocalToolCallKey: "tool:1", NativeToolCallID: "native:1", AdapterSessionGeneration: 1, Status: "SUCCEEDED", ExitCode: &code, Summary: "installed", Output: "ok", ResultDigest: strings.Repeat("a", 64)}}, nil
			}
			return nil, nil
		},
		toolAck: func(_ context.Context, ack executionaccess.ResultAckRequest) (executionaccess.Receipt, error) {
			acks = append(acks, ack)
			return executionaccess.Receipt{OperationID: 91, Version: int64(4 + len(acks)), Status: "SUCCEEDED"}, nil
		},
	})
	sequence := int64(0)
	c := pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		if after != sequence || sequence >= int64(len(frames)) {
			return attemptruntime.InteractionBatch{Through: after}, nil
		}
		// A native receipt can only exist after the result has crossed the
		// Supervisor downlink. Hold it back until that exact send occurred.
		if sequence == 1 && !resultSent {
			return attemptruntime.InteractionBatch{Through: after}, nil
		}
		frame := frames[sequence]
		sequence++
		return pumpBatch(frame, sequence), nil
	}, send: func(_ context.Context, round string, generation int64, method string, body json.RawMessage) error {
		if !requestSaved || round != "round:1" || generation != 1 || method != "tool.result" {
			t.Fatalf("result escaped the authenticated round: %s/%d/%s", round, generation, method)
		}
		var payload agentprotocol.ToolResultPayload
		if json.Unmarshal(body, &payload) != nil || payload.OperationID != 91 || payload.LocalToolCallKey != "tool:1" {
			t.Fatalf("invalid tool result %s", body)
		}
		resultSent = true
		return nil
	}}
	p := pumpFor(h, c)
	for step := 0; step < 5; step++ {
		if err = p.step(context.Background()); err != nil {
			t.Fatalf("step %d: %v", step+1, err)
		}
	}
	if strings.Join(methods, ",") != "tool.request,tool.received,activity,tool.closed" || len(acks) != 3 || acks[0].NativeResolved || acks[0].AgentContinued || !acks[1].NativeResolved || acks[1].AgentContinued || !acks[2].NativeResolved || !acks[2].AgentContinued || pendingCount(t, h) != 0 {
		t.Fatalf("tool lifecycle facts collapsed or did not cross transport: methods=%v acks=%+v pending=%d", methods, acks, pendingCount(t, h))
	}
}

func TestInteractionTransportReusedDeliveryIntentResolvesEachNativeCall(t *testing.T) {
	requests := []agentprotocol.ToolRequestPayload{
		{ToolName: agentprotocol.GitDeliveryToolName, LocalToolCallKey: "git:1", NativeToolCallID: "native:1", RoundKey: "round:1"},
		{ToolName: agentprotocol.GitDeliveryToolName, LocalToolCallKey: "git:2", NativeToolCallID: "native:2", RoundKey: "round:2"},
	}
	for index := range requests {
		arguments, err := json.Marshal(agentprotocol.GitDeliveryArguments{Summary: "ready for delivery"})
		if err != nil {
			t.Fatal(err)
		}
		requests[index].Arguments = arguments
		requests[index].ArgumentsDigest, err = agentprotocol.DigestToolArgumentsFor(agentprotocol.GitDeliveryToolName, arguments)
		if err != nil {
			t.Fatal(err)
		}
	}
	sends := make([]agentprotocol.ToolResultPayload, 0, 2)
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		frame, err := agentprotocol.Decode(append(append([]byte(nil), raw...), '\n'), "up")
		if err != nil {
			t.Fatal(err)
		}
		var request agentprotocol.ToolRequestPayload
		if json.Unmarshal(frame.Payload, &request) != nil {
			t.Fatal("invalid request payload")
		}
		result := pumpReceipt(raw)
		result.DeliveryIntent = delivery.IntentReceipt{IntentID: 91, Version: 2, Status: "WAITING_FINALIZATION", DeliveryMode: "NEW_BRANCH", DeliveryAuthorization: "AUTO_AFTER_GATES", Replayed: request.RoundKey == "round:2"}
		result.ImmediateToolResult = delivery.ImmediateToolResult{RoundKey: request.RoundKey, LocalToolCallKey: request.LocalToolCallKey, NativeToolCallID: request.NativeToolCallID, IntentID: 91, Status: "SCHEDULED", Summary: "scheduled", ResultDigest: strings.Repeat("a", 64)}
		return result, nil
	}})
	p := pumpFor(h, pumpTestChannel{send: func(_ context.Context, round string, generation int64, method string, body json.RawMessage) error {
		var result agentprotocol.ToolResultPayload
		if generation != 1 || method != "tool.result" || json.Unmarshal(body, &result) != nil || result.RoundKey != round {
			t.Fatalf("invalid delivery intent result: %s/%d/%s/%s", round, generation, method, body)
		}
		sends = append(sends, result)
		return nil
	}})
	for index, request := range requests {
		frame := pumpFrame(t, "tool.request", request)
		identity := pumpIdentity
		identity.CurrentRound = request.RoundKey
		receipt, err := h.spool.Put(context.Background(), interactionspool.Key{Identity: identity, Occurrence: strconv.Itoa(index + 1)}, frame)
		if err != nil {
			t.Fatal(err)
		}
		if err = p.publish(context.Background(), interactionspool.Entry{Receipt: receipt, Frame: frame}); err != nil {
			t.Fatalf("native call %d: %v", index+1, err)
		}
	}
	if len(sends) != 2 || sends[0].OperationID != 91 || sends[1].OperationID != 91 || sends[0].NativeToolCallID == sends[1].NativeToolCallID || pendingCount(t, h) != 0 {
		t.Fatalf("reused intent did not resolve both native calls: sends=%+v pending=%d", sends, pendingCount(t, h))
	}
}

func TestInteractionTransportPersistsToolCatalogAsCriticalFact(t *testing.T) {
	names := []string{agentprotocol.GitDeliveryToolName, agentprotocol.NetworkCommandToolName}
	digest, err := agentprotocol.DigestToolCatalogNames(names)
	if err != nil {
		t.Fatal(err)
	}
	frame := pumpFrame(t, "tool.catalog", agentprotocol.ToolCatalogPayload{
		RoundKey:      "round:1",
		ToolNames:     names,
		CatalogDigest: digest,
	})
	published := 0
	var h *interactionTransportHub
	h = pumpHub(t, pumpPath(t), pumpTestControl{
		publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
			published++
			if string(raw) != string(frame) || pendingCount(t, h) != 1 {
				t.Fatal("tool catalog bypassed the private spool or changed bytes")
			}
			return pumpReceipt(raw), nil
		},
	})
	channel := pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		if after > 0 {
			return attemptruntime.InteractionBatch{Through: after}, nil
		}
		return pumpBatch(frame, 1), nil
	}}
	if err = pumpFor(h, channel).step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if published != 1 || pendingCount(t, h) != 0 {
		t.Fatalf("catalog publication=%d pending=%d", published, pendingCount(t, h))
	}
}

func TestInteractionStageErrorKeepsOnlyAllowlistedStage(t *testing.T) {
	err := interactionStageError("TOOL_RESULT_SEND", errors.New("private socket and payload detail"))
	if !errors.Is(err, errInteractionTransport) || err.Error() != "WORKER_INTERACTION_TRANSPORT_FAILED: TOOL_RESULT_SEND" {
		t.Fatalf("unsafe or unclassifiable stage error: %v", err)
	}
}

func TestInteractionTransportToolRequestKeepsRoundAndSessionFencing(t *testing.T) {
	args := json.RawMessage(`{"purpose":"install","command":"pnpm install","cwd":".","targets":[{"host":"registry.npmjs.org","protocol":"https","port":443}]}`)
	digest, err := agentprotocol.DigestToolArguments(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, round string
		session     int64
	}{{"wrong_round", "round:old", 1}, {"wrong_session", "round:1", 2}} {
		t.Run(test.name, func(t *testing.T) {
			published := false
			frame := pumpFrame(t, "tool.request", agentprotocol.ToolRequestPayload{ToolName: agentprotocol.NetworkCommandToolName, LocalToolCallKey: "tool:1", NativeToolCallID: "native:1", RoundKey: test.round, Arguments: args, ArgumentsDigest: digest})
			batch := pumpBatch(frame, 1)
			batch.Notifications[0].SessionGeneration = test.session
			h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(context.Context, json.RawMessage) (control.InteractionPublication, error) {
				published = true
				return control.InteractionPublication{}, nil
			}})
			err := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return batch, nil }}).step(context.Background())
			if !errors.Is(err, errInteractionBinding) {
				t.Fatalf("fenced tool request accepted: %v", err)
			}
			if published || pendingCount(t, h) != 0 {
				t.Fatal("fenced tool request reached spool or Server")
			}
		})
	}
}

func pendingCount(t *testing.T, h *interactionTransportHub) int {
	t.Helper()
	entries, err := h.spool.Pending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func TestInteractionTransportPersistenceBeforeLocalReceiptAndServerDigest(t *testing.T) {
	ctx := context.Background()
	frame := pumpQuestion(t)
	confirmed := false
	var h *interactionTransportHub
	h = pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		if !confirmed || pendingCount(t, h) != 1 {
			t.Fatal("published before persistence/confirmation")
		}
		result := pumpReceipt(raw)
		result.Receipt.Saved = false // new question has no saved answer
		return result, nil
	}})
	c := pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }, confirm: func(_ context.Context, through int64) error {
		if through != 1 || pendingCount(t, h) != 1 {
			t.Fatal("local receipt before fsync")
		}
		confirmed = true
		return nil
	}}
	if err := pumpFor(h, c).step(ctx); err != nil {
		t.Fatal(err)
	}
	if pendingCount(t, h) != 0 {
		t.Fatal("authenticated exact digest did not delete")
	}
}

func TestInteractionTransportValidDisplayFrameBeyondOld64KiBBoundary(t *testing.T) {
	// The display cap includes payload JSON plus newline, but not the Frame
	// envelope. A payload at that cap exceeds the former 64KiB spool limit.
	frame := pumpFrame(t, "activity", map[string]any{"text": strings.Repeat("a", agentprotocol.MaxMessageBytes-len(`{"text":""}`)-1)})
	if len(frame) <= 64<<10 || len(frame) >= agentprotocol.MaxFrameBytes {
		t.Fatal("fixture does not cover wire-envelope expansion")
	}
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		return pumpReceipt(raw), nil
	}})
	p := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }})
	if err := p.step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pendingCount(t, h) != 0 || p.after != 1 {
		t.Fatal("valid frame was not acknowledged after Server persistence")
	}
}

func TestInteractionTransportWrongDigestOrForgedAckRetainsSpool(t *testing.T) {
	ctx := context.Background()
	frame := pumpQuestion(t)
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(context.Context, json.RawMessage) (control.InteractionPublication, error) {
		return control.InteractionPublication{FrameSHA256: strings.Repeat("0", 64)}, nil
	}})
	c := pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }}
	if err := pumpFor(h, c).step(ctx); !errors.Is(err, errInteractionBinding) {
		t.Fatal(err)
	}
	entries, err := h.spool.Pending(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatal("wrong digest dropped frame", err)
	}
	if err = h.spool.AcknowledgeServer(ctx, entries[0], []byte(entries[0].SHA256)); !errors.Is(err, interactionspool.ErrUnauthorized) {
		t.Fatal("container digest minted permit", err)
	}
}

func TestInteractionTransportRestartReplayAndStrictIdentity(t *testing.T) {
	ctx := context.Background()
	path := pumpPath(t)
	frame := pumpQuestion(t)
	publications := 0
	h := pumpHub(t, path, pumpTestControl{publish: func(context.Context, json.RawMessage) (control.InteractionPublication, error) {
		return control.InteractionPublication{}, io.EOF
	}})
	if err := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }}).step(ctx); !errors.Is(err, errInteractionRetry) {
		t.Fatal(err)
	}
	for _, field := range []string{"owner", "session", "round"} {
		id := pumpIdentity
		switch field {
		case "owner":
			id.OwnerEpoch--
		case "session":
			id.SessionGeneration++
		case "round":
			id.CurrentRound = "round:old"
		}
		if _, err := h.spool.Put(ctx, interactionspool.Key{Identity: id, Occurrence: "1"}, frame); err != nil {
			t.Fatal(err)
		}
	}
	_ = h.Close()
	h = pumpHub(t, path, pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		publications++
		return pumpReceipt(raw), nil
	}})
	if err := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) {
		return attemptruntime.InteractionBatch{Through: 1}, nil
	}}).step(ctx); err != nil {
		t.Fatal(err)
	}
	if publications != 1 || pendingCount(t, h) != 3 {
		t.Fatalf("implicit identity migration: publish=%d pending=%d", publications, pendingCount(t, h))
	}
}

func TestInteractionTransportDeliveryWaitsForNativeReceipt(t *testing.T) {
	ctx := context.Background()
	sends, acks := 0, 0
	hasReceipt := false
	delivery := interaction.Delivery{InteractionID: 10, ResponseID: 11, LocalRequestKey: "question:1", RoundKey: "round:1", AdapterSessionGeneration: 1, Kind: interactiondomain.QuestionKind, Answers: []interactiondomain.Answer{{QuestionKey: "q", Text: "yes"}}}
	frame := pumpFrame(t, "interaction.received", map[string]any{"roundKey": "round:1", "interactionKey": "question:1", "responseId": "11", "status": "RECEIVED"})
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		return pumpReceipt(raw), nil
	}, pull: func(context.Context) ([]interaction.Delivery, error) { return []interaction.Delivery{delivery}, nil }, ack: func(_ context.Context, ack interaction.AckRequest) (interaction.Receipt, error) {
		acks++
		if ack.InteractionID != 10 || ack.ResponseID != 11 {
			t.Fatal("wrong Server ack", ack)
		}
		return interaction.Receipt{InteractionID: 10, ResponseID: 11, Delivered: true}, nil
	}})
	c := pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		if hasReceipt && after == 0 {
			return pumpBatch(frame, 1), nil
		}
		return attemptruntime.InteractionBatch{Through: after}, nil
	}, send: func(_ context.Context, round string, g int64, method string, body json.RawMessage) error {
		sends++
		var answer agentprotocol.InteractionAnswer
		if json.Unmarshal(body, &answer) != nil || round != "round:1" || g != 1 || method != "interaction.respond" || answer.InteractionKey != "question:1" || answer.ResponseID != "11" || answer.Answers[0].SelectedOptionKeys == nil {
			t.Fatalf("wrong adapter binding %s", body)
		}
		return nil
	}}
	p := pumpFor(h, c)
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if sends != 1 || acks != 0 {
		t.Fatal("WRITTEN became receipt")
	}
	hasReceipt = true
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if sends != 1 || acks != 1 || pendingCount(t, h) != 0 {
		t.Fatalf("native receipt not correlated: sends=%d acks=%d", sends, acks)
	}
}

func TestInteractionTransportSuspendPrepareObservesAndAbortResumes(t *testing.T) {
	ctx := context.Background()
	state := "RUNNING"
	sends, pulls, publications, acks := 0, 0, 0, 0
	delivery := interaction.Delivery{InteractionID: 10, ResponseID: 11, LocalRequestKey: "question:1", RoundKey: "round:1", AdapterSessionGeneration: 1, Kind: interactiondomain.QuestionKind, Answers: []interactiondomain.Answer{{QuestionKey: "q", Text: "yes"}}}
	received := pumpFrame(t, "interaction.received", map[string]any{"roundKey": "round:1", "interactionKey": "question:1", "responseId": "11", "status": "RECEIVED"})
	activity := pumpFrame(t, "activity", map[string]any{"text": "preparing"})
	h := pumpHub(t, pumpPath(t), pumpTestControl{
		publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
			publications++
			return pumpReceipt(raw), nil
		},
		pull: func(context.Context) ([]interaction.Delivery, error) {
			pulls++
			return []interaction.Delivery{delivery}, nil
		},
		ack: func(context.Context, interaction.AckRequest) (interaction.Receipt, error) {
			acks++
			return interaction.Receipt{InteractionID: 10, ResponseID: 11, Delivered: true}, nil
		},
	})
	p := pumpFor(h, pumpTestChannel{
		state: func(context.Context) (attemptruntime.InteractionExecutionState, error) {
			return attemptruntime.InteractionExecutionState{State: state, RoundKey: "round:1", SessionGeneration: 1}, nil
		},
		notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
			if state != "RUNNING" {
				if after == 0 {
					return pumpBatch(received, 1), nil
				}
				return pumpBatch(activity, after+1), nil
			}
			return attemptruntime.InteractionBatch{Through: after}, nil
		},
		send: func(context.Context, string, int64, string, json.RawMessage) error { sends++; return nil },
	})
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"PAUSING_PREPARE", "PAUSING", "INTERACTION_SUSPENDED"} {
		state = phase
		if err := p.step(ctx); err != nil {
			t.Fatalf("%s: %v", phase, err)
		}
		if pulls != 1 || sends != 1 || pendingCount(t, h) != 0 {
			t.Fatal("pause sent input or failed to drain observations")
		}
	}
	if publications != 3 || acks != 1 {
		t.Fatal("pause did not publish receipt/activity")
	}
	// ANSWER_WON abort returns native state to RUNNING; the same pump can
	// pick up another delivery without losing its existing receipt history.
	state = "RUNNING"
	delivery = interaction.Delivery{InteractionID: 12, LocalRequestKey: "message:2", AdapterSessionGeneration: 1, Kind: interactiondomain.MessageKind, Text: "continue"}
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if sends != 2 || pulls != 2 || h.Err() != nil {
		t.Fatal("abort did not resume next poll")
	}
}

func TestInteractionTransportPausingSendRaceDoesNotConsumeRetryBudget(t *testing.T) {
	ctx := context.Background()
	sends := 0
	paused := true
	delivery := interaction.Delivery{InteractionID: 10, LocalRequestKey: "message:1", AdapterSessionGeneration: 1, Kind: interactiondomain.MessageKind, Text: "continue"}
	h := pumpHub(t, pumpPath(t), pumpTestControl{pull: func(context.Context) ([]interaction.Delivery, error) {
		return []interaction.Delivery{delivery, delivery}, nil
	}})
	p := pumpFor(h, pumpTestChannel{send: func(context.Context, string, int64, string, json.RawMessage) error {
		sends++
		if paused {
			return attemptruntime.ErrInteractionPausing
		}
		return nil
	}})
	for i := 0; i < 10; i++ {
		if err := p.step(ctx); err != nil {
			t.Fatalf("pausing must be a successful poll, not retry/fatal: %v", err)
		}
	}
	if sends != 10 || len(p.sent) != 0 {
		t.Fatal("rejected write was retained or remaining sends were attempted")
	}
	paused = false
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if sends != 11 || len(p.sent) != 1 {
		t.Fatal("successful retry not recorded exactly once")
	}
}

func TestInteractionTransportLostServerAckReplaysAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := pumpPath(t)
	delivered := false
	acks := 0
	frame := pumpFrame(t, "interaction.received", map[string]any{"roundKey": "round:1", "interactionKey": "question:1", "responseId": "11", "status": "RECEIVED"})
	client := pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		r := pumpReceipt(raw)
		r.Receipt.Delivered = delivered
		return r, nil
	}, pull: func(context.Context) ([]interaction.Delivery, error) {
		if delivered {
			return nil, nil
		}
		return []interaction.Delivery{{InteractionID: 10, ResponseID: 11, LocalRequestKey: "question:1", RoundKey: "round:1", AdapterSessionGeneration: 1, Kind: interactiondomain.QuestionKind}}, nil
	}, ack: func(_ context.Context, ack interaction.AckRequest) (interaction.Receipt, error) {
		acks++
		if !delivered {
			delivered = true
			return interaction.Receipt{}, io.EOF
		}
		return interaction.Receipt{InteractionID: 10, ResponseID: 11, Delivered: true}, nil
	}}
	h := pumpHub(t, path, client)
	if err := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }}).step(ctx); !errors.Is(err, errInteractionRetry) {
		t.Fatal(err)
	}
	if pendingCount(t, h) != 1 {
		t.Fatal("lost ACK removed evidence")
	}
	_ = h.Close()
	h = pumpHub(t, path, client)
	if err := pumpFor(h, pumpTestChannel{}).step(ctx); err != nil {
		t.Fatal(err)
	}
	if acks != 2 || pendingCount(t, h) != 0 {
		t.Fatalf("ACK replay failed: %d", acks)
	}
}

func TestInteractionTransportColdReceiptUsesAuthenticatedTargetDelivery(t *testing.T) {
	for _, cold := range []bool{true, false} {
		t.Run(map[bool]string{true: "cold_target", false: "normal_missing_round"}[cold], func(t *testing.T) {
			acks := 0
			frame := pumpFrame(t, "interaction.received", map[string]any{"roundKey": "round:1", "interactionKey": "question:old", "responseId": "11", "status": "RECEIVED"})
			d := interaction.Delivery{InteractionID: 10, ResponseID: 11, LocalRequestKey: "question:old", AdapterSessionGeneration: 1, Kind: interactiondomain.QuestionKind, ColdResume: cold}
			h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
				return pumpReceipt(raw), nil
			}, pull: func(context.Context) ([]interaction.Delivery, error) { return []interaction.Delivery{d}, nil }, ack: func(context.Context, interaction.AckRequest) (interaction.Receipt, error) {
				acks++
				return interaction.Receipt{InteractionID: 10, ResponseID: 11, Delivered: true}, nil
			}})
			p := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }})
			err := p.step(context.Background())
			if cold {
				if err != nil || acks != 1 || len(p.sent) != 0 || pendingCount(t, h) != 0 {
					t.Fatalf("cold private-input receipt failed without ordinary Send: %v", err)
				}
			} else if !errors.Is(err, errInteractionBinding) || acks != 0 || pendingCount(t, h) != 1 {
				t.Fatal("normal missing-round delivery acquired cold authority")
			}
		})
	}
}

func TestInteractionTransportWrongNativeBindingNeverAcknowledges(t *testing.T) {
	ctx := context.Background()
	acks := 0
	frame := pumpFrame(t, "interaction.received", map[string]any{"roundKey": "round:1", "interactionKey": "forged:key", "responseId": "11", "status": "RECEIVED"})
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		return pumpReceipt(raw), nil
	}, pull: func(context.Context) ([]interaction.Delivery, error) {
		return []interaction.Delivery{{InteractionID: 10, ResponseID: 11, LocalRequestKey: "question:1", RoundKey: "round:1", AdapterSessionGeneration: 1, Kind: interactiondomain.QuestionKind}}, nil
	}, ack: func(context.Context, interaction.AckRequest) (interaction.Receipt, error) {
		acks++
		return interaction.Receipt{}, nil
	}})
	err := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }}).step(ctx)
	if !errors.Is(err, errInteractionBinding) || acks != 0 || pendingCount(t, h) != 1 {
		t.Fatalf("forged native binding accepted: %v", err)
	}
}

func TestInteractionTransportHubReportsFatalAndCloseJoins(t *testing.T) {
	h := pumpHub(t, pumpPath(t), pumpTestControl{})
	stop, err := h.Register(context.Background(), 1, pumpAttempt, pumpTestChannel{state: func(context.Context) (attemptruntime.InteractionExecutionState, error) {
		return attemptruntime.InteractionExecutionState{}, errors.New("private provider body")
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case failure := <-h.Errors():
		if failure.AttemptID != 2 || failure.OwnerEpoch != 4 || strings.Contains(failure.Err.Error(), "private") {
			t.Fatal("bad runtime failure", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("fatal failure was only logged")
	}
	stop()
	if _, err = h.Register(context.Background(), 1, pumpAttempt, pumpTestChannel{}); err == nil {
		t.Fatal("failed hub accepted registration")
	}
	second := pumpHub(t, pumpPath(t), pumpTestControl{})
	entered := make(chan struct{})
	exited := make(chan struct{})
	_, err = second.Register(context.Background(), 1, pumpAttempt, pumpTestChannel{state: func(ctx context.Context) (attemptruntime.InteractionExecutionState, error) {
		close(entered)
		<-ctx.Done()
		close(exited)
		return attemptruntime.InteractionExecutionState{}, ctx.Err()
	}})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("Close returned before reader exited")
	}
}

func TestInteractionTransportSpoolOverflowDoesNotConfirm(t *testing.T) {
	ctx := context.Background()
	path := pumpPath(t)
	h := pumpHub(t, path, pumpTestControl{})
	_ = h.spool.Close()
	var err error
	h.spool, err = interactionspool.Open(path, interactionspool.Limits{MaxEntries: 1, MaxBytes: 8192, MaxFrameBytes: 4096}, h.verifyServerAck)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.spool.Put(ctx, interactionspool.Key{Identity: pumpIdentity, Occurrence: "old"}, pumpQuestion(t)); err != nil {
		t.Fatal(err)
	}
	confirms := 0
	err = pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) {
		return pumpBatch(pumpQuestion(t), 1), nil
	}, confirm: func(context.Context, int64) error { confirms++; return nil }}).step(ctx)
	if !errors.Is(err, interactionspool.ErrFull) || confirms != 0 {
		t.Fatalf("overflow confirmed: %v", err)
	}
}

func TestInteractionTransportUserMessageUsesCurrentNativeRound(t *testing.T) {
	ctx := context.Background()
	sent := false
	h := pumpHub(t, pumpPath(t), pumpTestControl{})
	p := pumpFor(h, pumpTestChannel{send: func(_ context.Context, round string, g int64, method string, raw json.RawMessage) error {
		var message agentprotocol.UserMessage
		_ = json.Unmarshal(raw, &message)
		if round != "round:1" || g != 1 || method != "user.message" || message.RoundKey != round || message.MessageKey != "message:1" {
			t.Fatalf("wrong message %s", raw)
		}
		sent = true
		return nil
	}})
	if err := p.deliver(ctx, attemptruntime.InteractionExecutionState{State: "RUNNING", RoundKey: "round:1", SessionGeneration: 1}, interaction.Delivery{InteractionID: 10, AdapterSessionGeneration: 1, Kind: interactiondomain.MessageKind, LocalRequestKey: "message:1", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	if !sent {
		t.Fatal("message not sent")
	}
}

func TestInteractionTransportLostLocalReceiptRetriesDurablePrefix(t *testing.T) {
	ctx := context.Background()
	confirms, polls := 0, 0
	frame := pumpQuestion(t)
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		return pumpReceipt(raw), nil
	}})
	c := pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		polls++
		if polls == 1 {
			return pumpBatch(frame, 1), nil
		}
		if after != 1 {
			t.Fatalf("lost local ACK repolled unconfirmed prefix %d", after)
		}
		return attemptruntime.InteractionBatch{Through: 1}, nil
	}, confirm: func(_ context.Context, through int64) error {
		confirms++
		if pendingCount(t, h) != 1 || through != 1 {
			t.Fatal("retry lacked durable prefix")
		}
		if confirms == 1 {
			return io.EOF
		}
		return nil
	}}
	p := pumpFor(h, c)
	if err := p.step(ctx); !errors.Is(err, errInteractionRetry) {
		t.Fatal(err)
	}
	if pendingCount(t, h) != 1 {
		t.Fatal("uncertain local receipt lost frame")
	}
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if confirms != 2 || pendingCount(t, h) != 0 {
		t.Fatalf("local confirmation retry: %d", confirms)
	}
}

func TestInteractionTransportSequenceGapAndFutureOwnerFailClosed(t *testing.T) {
	ctx := context.Background()
	frame := pumpQuestion(t)
	h := pumpHub(t, pumpPath(t), pumpTestControl{})
	p := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 3), nil }})
	p.after = 1
	p.seen = true
	if err := p.step(ctx); !errors.Is(err, errInteractionBinding) || pendingCount(t, h) != 0 {
		t.Fatalf("gap confirmed: %v", err)
	}
	old := pumpIdentity
	old.OwnerEpoch++
	if _, err := h.spool.Put(ctx, interactionspool.Key{Identity: old, Occurrence: "1"}, frame); err != nil {
		t.Fatal(err)
	}
	p = pumpFor(h, pumpTestChannel{})
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if pendingCount(t, h) != 1 {
		t.Fatal("future owner inherited authority")
	}
}

func TestInteractionTransportActivityGapIsPersistedWithoutChangingACKWatermark(t *testing.T) {
	ctx := context.Background()
	frame := pumpQuestion(t)
	keys := []string{}
	confirms := 0
	var h *interactionTransportHub
	h = pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		var f agentprotocol.Frame
		_ = json.Unmarshal(raw, &f)
		keys = append(keys, f.Key)
		if f.Key == "gap:3" {
			var body struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(f.Payload, &body)
			if f.Method != "activity" || body.Text != "部分早期活动已合并" {
				t.Fatalf("gap was misrepresented %s", raw)
			}
		}
		return pumpReceipt(raw), nil
	}})
	c := pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) {
		b := pumpBatch(frame, 4)
		b.ActivityGap = true
		b.ActivityDroppedThrough = 3
		return b, nil
	}, confirm: func(_ context.Context, through int64) error {
		confirms++
		if through != 4 || pendingCount(t, h) != 2 {
			t.Fatal("gap altered ACK or was not durable")
		}
		return nil
	}}
	p := pumpFor(h, c)
	p.after = 1
	p.seen = true
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if confirms != 1 || len(keys) != 2 || keys[0] != "gap:3" || p.after != 4 {
		t.Fatalf("gap missing: %v, after=%d", keys, p.after)
	}
	// A gap-only page must not acknowledge the dropped-through counter itself.
	keys = nil
	confirms = 0
	p.channel = pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) {
		return attemptruntime.InteractionBatch{Through: 4, ActivityGap: true, ActivityDroppedThrough: 9}, nil
	}, confirm: func(context.Context, int64) error { confirms++; return nil }}
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if confirms != 0 || p.after != 4 || len(keys) != 1 || keys[0] != "gap:9" {
		t.Fatalf("gap-only ACK/dedup: %v after=%d", keys, p.after)
	}
}

func TestInteractionTransportActivityGapCannotHideLaterCriticalGap(t *testing.T) {
	ctx := context.Background()
	h := pumpHub(t, pumpPath(t), pumpTestControl{})
	p := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) {
		b := pumpBatch(pumpQuestion(t), 5)
		b.ActivityGap = true
		b.ActivityDroppedThrough = 3
		return b, nil
	}})
	p.after = 1
	p.seen = true
	if err := p.step(ctx); !errors.Is(err, errInteractionBinding) {
		t.Fatal("critical seq 4 hidden by activity drop3", err)
	}
	if pendingCount(t, h) != 0 {
		t.Fatal("invalid gap persisted")
	}
}

func TestInteractionTransportNormalStopDrainsFinalNotification(t *testing.T) {
	ctx := context.Background()
	warm := make(chan struct{})
	var polls, published atomic.Int64
	frame := pumpFrame(t, "activity", map[string]any{"text": "last activity"})
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		published.Add(1)
		return pumpReceipt(raw), nil
	}})
	h.interval = time.Hour
	stop, err := h.Register(ctx, 1, pumpAttempt, pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		if polls.Add(1) == 1 {
			close(warm)
			return attemptruntime.InteractionBatch{Through: 0}, nil
		}
		if after == 0 {
			return pumpBatch(frame, 1), nil
		}
		return attemptruntime.InteractionBatch{Through: after}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	<-warm
	stop()
	if published.Load() != 1 || pendingCount(t, h) != 0 {
		t.Fatal("normal cleanup discarded final notification")
	}
}

func TestInteractionTransportDrainReturnsUnconfirmedFailure(t *testing.T) {
	ctx := context.Background()
	warm := make(chan struct{})
	var polls atomic.Int64
	frame := pumpQuestion(t)
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(context.Context, json.RawMessage) (control.InteractionPublication, error) {
		return control.InteractionPublication{}, io.EOF
	}})
	h.interval = time.Hour
	stop, err := h.Register(ctx, 1, pumpAttempt, pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		if polls.Add(1) == 1 {
			close(warm)
			return attemptruntime.InteractionBatch{}, nil
		}
		return pumpBatch(frame, 1), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	<-warm
	if err := h.Drain(ctx, pumpAttempt); err == nil {
		t.Fatal("unconfirmed critical frame allowed Finalizer")
	}
	stop()
	if h.Err() == nil || pendingCount(t, h) != 1 {
		t.Fatal("normal cleanup swallowed drain failure")
	}
}

func TestInteractionTransportDrainRejectsFencedResidual(t *testing.T) {
	ctx := context.Background()
	warm := make(chan struct{})
	var polls atomic.Int64
	h := pumpHub(t, pumpPath(t), pumpTestControl{})
	h.interval = time.Hour
	old := pumpIdentity
	old.OwnerEpoch--
	if _, err := h.spool.Put(ctx, interactionspool.Key{Identity: old, Occurrence: "1"}, pumpQuestion(t)); err != nil {
		t.Fatal(err)
	}
	stop, err := h.Register(ctx, 1, pumpAttempt, pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		if polls.Add(1) == 1 {
			close(warm)
		}
		return attemptruntime.InteractionBatch{Through: after}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	<-warm
	if err = h.Drain(ctx, pumpAttempt); err == nil {
		t.Fatal("fenced owner frame skipped during final success gate")
	}
	stop()
	if pendingCount(t, h) != 1 {
		t.Fatal("fenced frame implicitly replayed/deleted")
	}
}

func TestInteractionTransportDrainHonorsCallerDeadline(t *testing.T) {
	ctx := context.Background()
	warm := make(chan struct{})
	var calls atomic.Int64
	h := pumpHub(t, pumpPath(t), pumpTestControl{})
	h.interval = time.Hour
	stop, err := h.Register(ctx, 1, pumpAttempt, pumpTestChannel{state: func(ctx context.Context) (attemptruntime.InteractionExecutionState, error) {
		if calls.Add(1) == 1 {
			close(warm)
			return attemptruntime.InteractionExecutionState{RoundKey: "round:1", SessionGeneration: 1}, nil
		}
		<-ctx.Done()
		return attemptruntime.InteractionExecutionState{}, ctx.Err()
	}})
	if err != nil {
		t.Fatal(err)
	}
	<-warm
	drainCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if err = h.Drain(drainCtx, pumpAttempt); err == nil {
		t.Fatal("deadline drain reported success")
	}
	stop()
	if h.Err() == nil {
		t.Fatal("drain error not retained")
	}
}

func TestInteractionTransportDrainDoesNotPullNewWorkInEvaluation(t *testing.T) {
	ctx := context.Background()
	frame := pumpFrame(t, "activity", map[string]any{"text": "last observation"})
	h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		return pumpReceipt(raw), nil
	}, pull: func(context.Context) ([]interaction.Delivery, error) {
		t.Fatal("drain tried to authorize a new delivery in EVALUATING")
		return nil, nil
	}})
	p := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }})
	p.draining = true
	if err := p.step(ctx); err != nil {
		t.Fatal(err)
	}
	if pendingCount(t, h) != 0 {
		t.Fatal("final observation not confirmed")
	}
}

func TestInteractionTransportPhaseDenialDrainsButFencingIsFatal(t *testing.T) {
	for _, reason := range []string{interactiondomain.ErrNotAllowed.Error(), interactiondomain.ErrFenced.Error()} {
		t.Run(reason, func(t *testing.T) {
			frame := pumpFrame(t, "activity", map[string]any{"text": "last observation"})
			published := false
			h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
				published = true
				return pumpReceipt(raw), nil
			}, pull: func(context.Context) ([]interaction.Delivery, error) {
				return nil, &control.APIError{StatusCode: 409, ReasonCode: reason}
			}})
			err := pumpFor(h, pumpTestChannel{notifications: func(context.Context, int64) (attemptruntime.InteractionBatch, error) { return pumpBatch(frame, 1), nil }}).step(context.Background())
			if reason == interactiondomain.ErrNotAllowed.Error() {
				if err != nil || !published || pendingCount(t, h) != 0 {
					t.Fatal("phase transition stopped last activity", err)
				}
			} else {
				if err == nil || published || pendingCount(t, h) != 1 {
					t.Fatal("formal fencing ignored", err)
				}
			}
		})
	}
}

func TestInteractionTransportDrainIsRepeatableUntilCleanup(t *testing.T) {
	ctx := context.Background()
	warm := make(chan struct{})
	var polls atomic.Int64
	h := pumpHub(t, pumpPath(t), pumpTestControl{})
	h.interval = time.Hour
	stop, err := h.Register(ctx, 1, pumpAttempt, pumpTestChannel{notifications: func(_ context.Context, after int64) (attemptruntime.InteractionBatch, error) {
		if polls.Add(1) == 1 {
			close(warm)
		}
		return attemptruntime.InteractionBatch{Through: after}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	<-warm
	if err = h.Drain(ctx, pumpAttempt); err != nil {
		t.Fatal(err)
	}
	count := polls.Load()
	if err = h.Drain(ctx, pumpAttempt); err != nil {
		t.Fatal("suspension then final gate repeated drain", err)
	}
	if polls.Load() != count {
		t.Fatal("completed drain restarted pump")
	}
	stop()
}
