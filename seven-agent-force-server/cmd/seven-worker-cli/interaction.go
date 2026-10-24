package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

const criticalNotificationLimit = 128
const criticalNotificationBytes = 4 << 20
const notificationBatchBytes = 48 << 10
const unresolvedInteractionLimit = 8

// This memory window bridges to the trusted Worker's private durable spool.
// Reading never removes a frame; only notifications-received does that.
type agentNotification struct {
	Sequence          int64               `json:"sequence,string"`
	SessionGeneration int64               `json:"sessionGeneration,string"`
	RoundKey          string              `json:"roundKey"`
	Frame             agentprotocol.Frame `json:"frame"`
}
type pendingInteraction struct {
	request          agentprotocol.QuestionRequest
	identity         string
	local            bool
	answer           chan struct{}
	closed           bool
	accepted         *agentprotocol.InteractionAnswer
	resolved         bool
	restoredExpected *agentprotocol.InteractionAnswer
}
type pendingToolCall struct {
	request     agentprotocol.ToolRequestPayload
	identity    string
	operationID int64
	resolved    bool
}

func pendingToolAcceptsResult(pending *pendingToolCall, result agentprotocol.ToolResultPayload) bool {
	return pending != nil && !pending.resolved &&
		pending.request.RoundKey == result.RoundKey &&
		pending.request.LocalToolCallKey == result.LocalToolCallKey &&
		pending.request.NativeToolCallID == result.NativeToolCallID &&
		(pending.operationID == 0 || pending.operationID == result.OperationID)
}

type supervisorInteractions struct {
	currentRound           string
	sessionGeneration      int64
	active                 bool
	notifications          []agentNotification
	activityNotifications  []agentNotification
	activityDroppedThrough int64
	sequence               int64
	receivedThrough        int64
	bytes                  int
	overflow               bool
	pending                map[string]*pendingInteraction
	pendingTools           map[string]*pendingToolCall
	receipts               map[string]agentprotocol.Frame
	clock                  interactionClock
	hotWaitLimit           time.Duration
	changed                chan struct{}
	roundDone              chan struct{}
	roundClosed            bool
	accountedAt            time.Time
	activeElapsed          time.Duration
	waitingElapsed         time.Duration
	waitingSince           time.Time
	pausing                bool
	prepareSequence        int64
	suspendDecision        string
	waitAborted            bool
	adapterStopped         bool
	quiescenceHeld         bool
	suspendControlKey      string
	suspendReason          string
	captureReadyAt         string
	captureError           string
	admissionError         string
}

func (state *supervisorState) beginAgentRound(round string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	i := &state.interaction
	if i.clock == nil {
		i.clock = realInteractionClock{}
	}
	if i.hotWaitLimit == 0 {
		i.hotWaitLimit = 10 * time.Minute
	}
	state.accountInteractionTime(i.clock.Now())
	if i.sessionGeneration == 0 {
		i.sessionGeneration = 1
	}
	i.currentRound, i.active = round, true
	i.accountedAt = i.clock.Now()
	i.waitingSince = time.Time{}
	i.pausing, i.adapterStopped, i.quiescenceHeld = false, false, false
	i.suspendDecision, i.waitAborted = "", false
	i.suspendControlKey, i.suspendReason, i.captureReadyAt, i.captureError = "", "", "", ""
	i.changed = make(chan struct{}, 1)
	i.roundDone = make(chan struct{})
	i.roundClosed = false
	i.pending = make(map[string]*pendingInteraction)
	i.pendingTools = make(map[string]*pendingToolCall)
	i.receipts = make(map[string]agentprotocol.Frame)
}

func (state *supervisorState) endAgentRound(round string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.interaction.currentRound == round {
		i := &state.interaction
		state.accountInteractionTime(i.clock.Now())
		state.interaction.active = false
		if !i.roundClosed {
			close(i.roundDone)
			i.roundClosed = true
		}
		for _, pending := range state.interaction.pending {
			if pending.answer != nil && !pending.closed {
				close(pending.answer)
				pending.closed = true
			}
		}
		state.refreshInteractionWaiting()
	}
}

func canonicalPayload(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	body, _ := json.Marshal(value)
	return string(body)
}

// All callers hold state.mu. Saturation fails closed instead of evicting a
// question or receipt that the Worker has not persisted. No Task success follows.
func (state *supervisorState) appendNotification(frame agentprotocol.Frame) error {
	i := &state.interaction
	if frame.Method == "interaction.request" {
		var request agentprotocol.QuestionRequest
		if json.Unmarshal(frame.Payload, &request) != nil {
			return agentprotocol.ErrInvalid
		}
		for n := range request.Questions {
			q := &request.Questions[n]
			q.Title = observationCredential.ReplaceAllString(q.Title, "[redacted]")
			q.Description = observationCredential.ReplaceAllString(q.Description, "[redacted]")
			for m := range q.Options {
				q.Options[m].Label = observationCredential.ReplaceAllString(q.Options[m].Label, "[redacted]")
			}
		}
		frame.Payload, _ = json.Marshal(request)
	}
	size := len(frame.Payload) + 512
	if size > notificationBatchBytes || len(i.notifications) >= criticalNotificationLimit || i.bytes+size > criticalNotificationBytes {
		i.overflow = true
		if state.cancel != nil {
			state.cancel()
		}
		return errors.New("SUPERVISOR_NOTIFICATION_BACKPRESSURE")
	}
	i.sequence++
	frame.Payload = append(json.RawMessage(nil), frame.Payload...)
	i.notifications = append(i.notifications, agentNotification{i.sequence, i.sessionGeneration, i.currentRound, frame})
	i.bytes += size
	return nil
}

// Display events share the replay cursor but never share critical capacity.
// Their projection is <=4 KiB and the complete frame stays below 64 KiB.
func (state *supervisorState) appendActivityNotification(frame agentprotocol.Frame) {
	i := &state.interaction
	var binding struct {
		RoundKey string `json:"roundKey"`
	}
	if !i.active || json.Unmarshal(frame.Payload, &binding) != nil || binding.RoundKey != i.currentRound {
		return
	}
	i.sequence++
	frame.Protocol = agentprotocol.Version
	if frame.Key == "" {
		frame.Key = "observation:" + strconv.FormatInt(i.sequence, 10)
	}
	frame.Payload = append(json.RawMessage(nil), frame.Payload...)
	i.activityNotifications = append(i.activityNotifications, agentNotification{i.sequence, i.sessionGeneration, i.currentRound, frame})
	if len(i.activityNotifications) > observationWindow {
		i.activityDroppedThrough = i.activityNotifications[0].Sequence
		i.activityNotifications = append([]agentNotification(nil), i.activityNotifications[1:]...)
	}
}

func (state *supervisorState) observeInteraction(frame agentprotocol.Frame) bool {
	switch frame.Method {
	case "interaction.request", "interaction.received", "interaction.closed", "message.received", "execution.suspended", "tool.catalog", "tool.request", "tool.received", "tool.closed":
	default:
		return false
	}
	if agentprotocol.ValidateInteractionPayload(frame.Method, frame.Payload) != nil || agentprotocol.ValidateToolPayload(frame.Method, frame.Payload) != nil {
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	i := &state.interaction
	var binding struct {
		RoundKey         string                       `json:"roundKey"`
		InteractionKey   string                       `json:"interactionKey"`
		Status           agentprotocol.DeliveryStatus `json:"status"`
		LocalToolCallKey string                       `json:"localToolCallKey"`
		OperationID      int64                        `json:"operationId,string"`
	}
	_ = json.Unmarshal(frame.Payload, &binding)
	if !i.active || binding.RoundKey != i.currentRound {
		return true
	}
	if frame.Method == "interaction.request" {
		var request agentprotocol.QuestionRequest
		_ = json.Unmarshal(frame.Payload, &request)
		if previous := i.pending[request.InteractionKey]; previous != nil {
			if previous.identity != canonicalPayload(frame.Payload) && state.cancel != nil {
				state.cancel()
			}
			return true
		}
		if state.unresolvedInteractionCount() >= unresolvedInteractionLimit || i.pausing {
			i.admissionError = "ADAPTER_INTERACTION_BUSY"
			return true
		}
		if len(i.pending) >= criticalNotificationLimit {
			i.overflow = true
			if state.cancel != nil {
				state.cancel()
			}
			return true
		}
		i.pending[request.InteractionKey] = &pendingInteraction{request: request, identity: canonicalPayload(frame.Payload)}
		// appendNotification retains every question field and redacts only text.
	} else if frame.Method == "tool.request" {
		var request agentprotocol.ToolRequestPayload
		_ = json.Unmarshal(frame.Payload, &request)
		if previous := i.pendingTools[request.LocalToolCallKey]; previous != nil {
			if previous.identity != canonicalPayload(frame.Payload) && state.cancel != nil {
				state.cancel()
			}
			return true
		}
		if state.unresolvedInteractionCount() >= unresolvedInteractionLimit || i.pausing {
			i.admissionError = "ADAPTER_TOOL_BUSY"
			return true
		}
		i.pendingTools[request.LocalToolCallKey] = &pendingToolCall{request: request, identity: canonicalPayload(frame.Payload)}
	} else if frame.Method == "interaction.closed" {
		pending := i.pending[binding.InteractionKey]
		if pending == nil {
			return true
		}
		pending.resolved = true
		if pending.answer != nil && !pending.closed {
			close(pending.answer)
			pending.closed = true
		}
	} else if frame.Method == "tool.closed" {
		if pending := i.pendingTools[binding.LocalToolCallKey]; pending != nil {
			pending.resolved = true
		}
	} else {
		if previous, ok := i.receipts[frame.ReplyTo]; ok {
			if previous.Method != frame.Method || canonicalPayload(previous.Payload) != canonicalPayload(frame.Payload) {
				if state.cancel != nil {
					state.cancel()
				}
			}
			return true
		}
		i.receipts[frame.ReplyTo] = frame
		if frame.Method == "execution.suspended" {
			if i.suspendControlKey == "" {
				i.suspendControlKey = frame.ReplyTo
			}
			if !i.pausing {
				state.enterInteractionPausing("NATIVE_SUSPENDED")
			}
			state.closeLocalQuestionWaits()
		}
		if frame.Method == "interaction.received" && binding.Status == agentprotocol.DeliveryReceived {
			if pending := i.pending[binding.InteractionKey]; pending != nil {
				pending.resolved = true
			}
		}
		if frame.Method == "tool.received" {
			if pending := i.pendingTools[binding.LocalToolCallKey]; pending != nil && pending.operationID == binding.OperationID {
				pending.resolved = true
			} else if state.cancel != nil {
				state.cancel()
			}
		}
	}
	_ = state.appendNotification(frame)
	state.refreshInteractionWaiting()
	return true
}

// Caller holds state.mu. Full payloads are fetched in bounded notification
// batches, keeping ordinary state/reattach snapshots below the transport limit.
func (state *supervisorState) addInteractionSnapshot(snapshot map[string]any) {
	i := &state.interaction
	active, waiting := state.interactionTimes()
	snapshot["activeElapsedMillis"] = strconv.FormatInt(active.Milliseconds(), 10)
	snapshot["interactionWaitingMillis"] = strconv.FormatInt(waiting.Milliseconds(), 10)
	snapshot["interactionHotWaitMillis"] = strconv.FormatInt(i.hotWaitLimit.Milliseconds(), 10)
	snapshot["interactionWaiting"] = !i.waitingSince.IsZero() && i.active && !i.pausing
	snapshot["adapterStopped"] = i.adapterStopped
	snapshot["quiescenceHeld"] = i.quiescenceHeld
	snapshot["suspendControlKey"] = i.suspendControlKey
	snapshot["suspendReason"] = i.suspendReason
	snapshot["captureReadyAt"] = i.captureReadyAt
	snapshot["captureError"] = i.captureError
	snapshot["interactionAdmissionError"] = i.admissionError
	snapshot["currentRound"] = i.currentRound
	snapshot["sessionGeneration"] = strconv.FormatInt(i.sessionGeneration, 10)
	snapshot["interactionActive"] = i.active
	snapshot["pendingNotifications"] = strconv.Itoa(len(i.notifications) + len(i.activityNotifications))
	snapshot["pendingCriticalNotifications"] = strconv.Itoa(len(i.notifications))
	snapshot["activityDroppedThrough"] = strconv.FormatInt(i.activityDroppedThrough, 10)
	snapshot["notificationSequence"] = strconv.FormatInt(i.sequence, 10)
	snapshot["notificationReceivedThrough"] = strconv.FormatInt(i.receivedThrough, 10)
	snapshot["notificationOverflow"] = i.overflow
	keys := make([]string, 0, len(i.pending))
	for key, pending := range i.pending {
		if pending.request.IsBlocking && pending.accepted == nil && !pending.resolved {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	snapshot["pendingInteractions"] = keys
	toolKeys := make([]string, 0, len(i.pendingTools))
	for key, pending := range i.pendingTools {
		if !pending.resolved {
			toolKeys = append(toolKeys, key)
		}
	}
	sort.Strings(toolKeys)
	snapshot["pendingTools"] = toolKeys
	snapshot["controlReceiptCount"] = strconv.Itoa(len(i.receipts))
}

func (s *controlServer) validInteractionOwner(r attemptControlRequest) bool {
	return r.SchemaVersion == 2 && r.TaskID > 0 && r.AttemptID > 0 && r.Generation > 0 && r.OwnerEpoch > 0 && r.TaskID == s.state.TaskID && r.AttemptID == s.state.AttemptID && r.Generation == s.state.Generation && r.OwnerEpoch == s.state.OwnerEpoch
}

func (s *controlServer) handleInteractionControl(conn net.Conn, r attemptControlRequest) bool {
	switch r.Command {
	case "notifications", "notifications-received", "agent-control", "interaction-ask", "interaction-capture", "interaction-suspend-commit", "interaction-suspend-abort":
	default:
		return false
	}
	respond := func(response any) { _ = json.NewEncoder(conn).Encode(response) }
	fail := func(code string) { respond(map[string]any{"ok": false, "reasonCode": code}) }
	if r.Command == "interaction-ask" {
		s.handleInteractionAsk(conn, r)
		return true
	}
	s.state.mu.Lock()
	if !s.validInteractionOwner(r) {
		s.state.mu.Unlock()
		fail("SUPERVISOR_IDENTITY_MISMATCH")
		return true
	}
	i := &s.state.interaction
	switch r.Command {
	case "interaction-suspend-commit", "interaction-suspend-abort":
		var payload struct {
			SuspendControlKey string `json:"suspendControlKey"`
		}
		decoder := json.NewDecoder(bytes.NewReader(r.Payload))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF || payload.SuspendControlKey == "" || payload.SuspendControlKey != i.suspendControlKey || r.CurrentRound != i.currentRound || r.SessionGeneration != i.sessionGeneration || r.SessionGeneration <= 0 {
			s.state.mu.Unlock()
			fail("SUPERVISOR_SUSPEND_BINDING_MISMATCH")
			return true
		}
		decision := r.Command
		if i.suspendDecision != "" {
			matched := i.suspendDecision == decision
			captureError := i.captureError
			s.state.mu.Unlock()
			if matched && captureError != "" {
				fail("ADAPTER_SUSPEND_SEND_FAILED")
			} else if matched {
				respond(map[string]any{"ok": true})
			} else {
				fail("SUPERVISOR_SUSPEND_DECISION_CONFLICT")
			}
			return true
		}
		if s.state.State != "PAUSING_PREPARE" || !i.active {
			s.state.mu.Unlock()
			fail("SUPERVISOR_SUSPEND_NOT_PREPARED")
			return true
		}
		process, ctx := s.state.adapterProcess, s.state.ctx
		if decision == "interaction-suspend-abort" {
			i.accountedAt = i.clock.Now()
			i.suspendDecision, i.pausing, i.waitAborted = decision, false, true
			s.state.State = "RUNNING"
			s.state.refreshInteractionWaiting()
			round := i.currentRound
			s.state.mu.Unlock()
			go s.state.monitorInteractionWait(ctx, round, process)
			respond(map[string]any{"ok": true})
			return true
		}
		if process == nil || !process.HasCapability("execution.suspend") {
			s.state.mu.Unlock()
			fail("ADAPTER_METHOD_UNSUPPORTED")
			return true
		}
		i.suspendDecision = decision
		s.state.enterInteractionPausing("WORKER_RESERVED")
		round := i.currentRound
		s.state.mu.Unlock()
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		key, err := process.SendControl(writeCtx, "execution.suspend", map[string]any{"roundKey": round})
		cancel()
		if err != nil {
			s.state.mu.Lock()
			s.state.interaction.captureError = "ADAPTER_SUSPEND_SEND_FAILED"
			s.state.mu.Unlock()
			fail("ADAPTER_SUSPEND_SEND_FAILED")
			return true
		}
		respond(map[string]any{"ok": true, "controlKey": key, "status": "WRITTEN"})
	case "interaction-capture":
		if r.CurrentRound != i.currentRound || r.SessionGeneration <= 0 || r.SessionGeneration != i.sessionGeneration || s.state.State != "INTERACTION_SUSPENDED" || !i.adapterStopped || !i.quiescenceHeld || i.captureError != "" {
			s.state.mu.Unlock()
			fail("SUPERVISOR_CAPTURE_NOT_READY")
			return true
		}
		evidence := s.state.interactionCaptureSnapshot()
		s.state.mu.Unlock()
		respond(map[string]any{"ok": true, "capture": evidence})
	case "notifications":
		if r.After < 0 || r.After > i.sequence {
			s.state.mu.Unlock()
			fail("SUPERVISOR_NOTIFICATION_CURSOR_INVALID")
			return true
		}
		batch := make([]agentNotification, 0)
		through := r.After
		if through < i.receivedThrough {
			through = i.receivedThrough
		}
		used := 0
		retained := append([]agentNotification(nil), i.notifications...)
		retained = append(retained, i.activityNotifications...)
		sort.Slice(retained, func(a, b int) bool { return retained[a].Sequence < retained[b].Sequence })
		activityGap := through < i.activityDroppedThrough
		droppedThrough := i.activityDroppedThrough
		for _, event := range retained {
			if event.Sequence <= through {
				continue
			}
			body, _ := json.Marshal(event)
			if used+len(body) > notificationBatchBytes {
				break
			}
			batch = append(batch, event)
			used += len(body)
			through = event.Sequence
		}
		hasMore := through < i.sequence
		s.state.mu.Unlock()
		respond(map[string]any{"ok": true, "notifications": batch, "through": strconv.FormatInt(through, 10), "hasMore": hasMore, "activityGap": activityGap, "activityDroppedThrough": strconv.FormatInt(droppedThrough, 10)})
	case "notifications-received":
		if r.Through < 0 || r.Through > i.sequence {
			s.state.mu.Unlock()
			fail("SUPERVISOR_NOTIFICATION_CURSOR_INVALID")
			return true
		}
		if r.Through > i.receivedThrough {
			remaining := make([]agentNotification, 0, len(i.notifications))
			for _, event := range i.notifications {
				if event.Sequence > r.Through {
					remaining = append(remaining, event)
				} else {
					i.bytes -= len(event.Frame.Payload) + 512
				}
			}
			i.notifications = remaining
			activity := make([]agentNotification, 0, len(i.activityNotifications))
			for _, event := range i.activityNotifications {
				if event.Sequence > r.Through {
					activity = append(activity, event)
				}
			}
			i.activityNotifications = activity
			i.receivedThrough = r.Through
		}
		through := i.receivedThrough
		s.state.mu.Unlock()
		respond(map[string]any{"ok": true, "through": strconv.FormatInt(through, 10)})
	case "agent-control":
		if i.pausing {
			s.state.mu.Unlock()
			fail("SUPERVISOR_PAUSING")
			return true
		}
		if !i.active || i.overflow || r.CurrentRound != i.currentRound || r.SessionGeneration <= 0 || r.SessionGeneration != i.sessionGeneration || (s.state.ctx != nil && s.state.ctx.Err() != nil) {
			s.state.mu.Unlock()
			fail("SUPERVISOR_SESSION_MISMATCH")
			return true
		}
		var binding struct {
			RoundKey string `json:"roundKey"`
		}
		if json.Unmarshal(r.Payload, &binding) != nil || binding.RoundKey != i.currentRound || agentprotocol.ValidateInteractionPayload(r.Method, r.Payload) != nil || agentprotocol.ValidateToolPayload(r.Method, r.Payload) != nil {
			s.state.mu.Unlock()
			fail("ADAPTER_PROTOCOL_INVALID")
			return true
		}
		if r.Method == "interaction.respond" {
			var answer agentprotocol.InteractionAnswer
			_ = json.Unmarshal(r.Payload, &answer)
			pending := i.pending[answer.InteractionKey]
			if pending == nil || agentprotocol.ValidateAnswer(pending.request, answer) != nil {
				s.state.mu.Unlock()
				fail("SUPERVISOR_INTERACTION_MISMATCH")
				return true
			}
			if pending.restoredExpected != nil {
				expected, _ := json.Marshal(pending.restoredExpected)
				actual, _ := json.Marshal(answer)
				if !bytes.Equal(expected, actual) {
					s.state.mu.Unlock()
					fail("SUPERVISOR_RESTORED_ANSWER_MISMATCH")
					return true
				}
			}
			if pending.local {
				key := "cli-answer:" + answer.ResponseID
				if pending.accepted != nil {
					old, _ := json.Marshal(pending.accepted)
					next, _ := json.Marshal(answer)
					if !bytes.Equal(old, next) {
						s.state.mu.Unlock()
						fail("ADAPTER_IDEMPOTENCY_CONFLICT")
						return true
					}
				} else {
					payload, _ := json.Marshal(map[string]any{"roundKey": i.currentRound, "interactionKey": answer.InteractionKey, "responseId": answer.ResponseID, "status": agentprotocol.DeliveryReceived})
					frame := agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "interaction.received", Key: key, ReplyTo: key, Payload: payload}
					if err := s.state.appendNotification(frame); err != nil {
						s.state.mu.Unlock()
						fail(err.Error())
						return true
					}
					pending.accepted = &answer
					i.receipts[key] = frame
					close(pending.answer)
					pending.closed = true
					s.state.refreshInteractionWaiting()
				}
				s.state.mu.Unlock()
				respond(map[string]any{"ok": true, "controlKey": key, "status": "RECEIVED"})
				return true
			}
		}
		if r.Method == "tool.result" {
			var result agentprotocol.ToolResultPayload
			_ = json.Unmarshal(r.Payload, &result)
			pending := i.pendingTools[result.LocalToolCallKey]
			if !pendingToolAcceptsResult(pending, result) {
				s.state.mu.Unlock()
				fail("SUPERVISOR_TOOL_MISMATCH")
				return true
			}
			pending.operationID = result.OperationID
		}
		process, ctx := s.state.adapterProcess, s.state.ctx
		if r.Method == "execution.suspend" {
			if process == nil || !process.HasCapability("execution.suspend") {
				s.state.mu.Unlock()
				fail("ADAPTER_METHOD_UNSUPPORTED")
				return true
			}
			s.state.prepareInteractionSuspension("WORKER_REQUESTED")
			key := i.suspendControlKey
			s.state.mu.Unlock()
			respond(map[string]any{"ok": true, "suspendControlKey": key, "status": "PREPARED"})
			return true
		}
		s.state.mu.Unlock()
		if process == nil {
			fail("ADAPTER_UNAVAILABLE")
			return true
		}
		if ctx == nil {
			ctx = context.Background()
		}
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		key, err := process.SendControl(writeCtx, r.Method, r.Payload)
		cancel()
		if err != nil {
			if r.Method == "execution.suspend" {
				s.state.mu.Lock()
				s.state.interaction.captureError = "ADAPTER_SUSPEND_SEND_FAILED"
				s.state.mu.Unlock()
			}
			fail(err.Error())
			return true
		}
		respond(map[string]any{"ok": true, "controlKey": key, "status": "WRITTEN"})
	}
	return true
}

// CLI questions use only the current runtime binding. The caller cannot choose
// another Task, round, session, or authority. This remains an untrusted proposal.
func (s *controlServer) handleInteractionAsk(conn net.Conn, r attemptControlRequest) {
	fail := func(code string) { _ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": code}) }
	if r.SchemaVersion != 2 || r.TaskID != 0 || r.AttemptID != 0 || r.Generation != 0 || r.OwnerEpoch != 0 || r.CurrentRound != "" || r.SessionGeneration != 0 {
		fail("SUPERVISOR_IDENTITY_NOT_CALLER_SELECTED")
		return
	}
	var raw map[string]json.RawMessage
	if agentprotocol.ValidateDocument(r.Payload) != nil || json.Unmarshal(r.Payload, &raw) != nil || len(raw) != 3 || raw["interactionKey"] == nil || raw["questions"] == nil || raw["isBlocking"] == nil {
		fail("ADAPTER_PROTOCOL_INVALID")
		return
	}
	s.state.mu.Lock()
	i := &s.state.interaction
	if !i.active || i.pausing || i.overflow || s.state.adapterProcess == nil || !s.state.adapterProcess.HasCapability("interaction.request") {
		s.state.mu.Unlock()
		fail("SUPERVISOR_INTERACTION_UNAVAILABLE")
		return
	}
	raw["roundKey"], _ = json.Marshal(i.currentRound)
	body, _ := json.Marshal(raw)
	frame := agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "interaction.request", Payload: body}
	if agentprotocol.ValidateInteractionPayload(frame.Method, body) != nil {
		s.state.mu.Unlock()
		fail("ADAPTER_PROTOCOL_INVALID")
		return
	}
	var request agentprotocol.QuestionRequest
	_ = json.Unmarshal(body, &request)
	frame.Key = "cli:" + sha256Text(request.InteractionKey)
	pending := i.pending[request.InteractionKey]
	if pending != nil {
		if !pending.local || pending.identity != canonicalPayload(body) {
			s.state.mu.Unlock()
			fail("ADAPTER_IDEMPOTENCY_CONFLICT")
			return
		}
	} else {
		if s.state.unresolvedInteractionCount() >= unresolvedInteractionLimit {
			s.state.mu.Unlock()
			fail("ADAPTER_INTERACTION_BUSY")
			return
		}
		if len(i.pending) >= criticalNotificationLimit {
			s.state.mu.Unlock()
			fail("SUPERVISOR_INTERACTION_LIMIT")
			return
		}
		pending = &pendingInteraction{request: request, identity: canonicalPayload(body), local: true, answer: make(chan struct{})}
		i.pending[request.InteractionKey] = pending
		if err := s.state.appendNotification(frame); err != nil {
			s.state.mu.Unlock()
			fail(err.Error())
			return
		}
		s.state.refreshInteractionWaiting()
	}
	if pending.accepted != nil {
		answer := *pending.accepted
		s.state.mu.Unlock()
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "answer": answer})
		return
	}
	answerCh, ctx := pending.answer, s.state.ctx
	s.state.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Minute))
	disconnected := make(chan struct{})
	go func() { var b [1]byte; _, _ = conn.Read(b[:]); close(disconnected) }()
	select {
	case <-answerCh:
		s.state.mu.RLock()
		accepted := pending.accepted
		s.state.mu.RUnlock()
		if accepted == nil {
			fail("SUPERVISOR_SESSION_ENDED")
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "answer": accepted})
	case <-disconnected:
		return
	case <-ctx.Done():
		fail("SUPERVISOR_SESSION_ENDED")
	case <-time.After(30 * time.Minute):
		fail("SUPERVISOR_INTERACTION_TIMEOUT")
	}
}

// Caller holds state.mu. One request may contain up to three typed questions,
// while an App Server turn may have several independent native requests.
func (state *supervisorState) unresolvedInteractionCount() int {
	count := 0
	for _, pending := range state.interaction.pending {
		if pending.accepted == nil && !pending.resolved {
			count++
		}
	}
	for _, pending := range state.interaction.pendingTools {
		if !pending.resolved {
			count++
		}
	}
	return count
}

func interactionMain(args []string) error {
	if len(args) != 3 || args[0] != "ask" || args[1] != "--request" {
		return errors.New("usage: interaction ask --request FILE")
	}
	file, err := os.Open(args[2])
	if err != nil {
		return errors.New("INTERACTION_REQUEST_UNAVAILABLE")
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, controlRequestLimit+1))
	if err != nil || len(body) > controlRequestLimit {
		return agentprotocol.ErrInvalid
	}
	// Validation occurs against the runtime-bound round inside the Supervisor.
	request, err := json.Marshal(attemptControlRequest{SchemaVersion: 2, Command: "interaction-ask", Payload: body})
	if err != nil || len(request) > controlRequestLimit {
		return agentprotocol.ErrInvalid
	}
	socket := os.Getenv("SEVEN_CONTROL_SOCKET")
	if socket == "" {
		return errors.New("SEVEN_CONTROL_SOCKET is required")
	}
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return errors.New("SUPERVISOR_UNAVAILABLE")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Minute))
	if _, err = conn.Write(append(request, '\n')); err != nil {
		return errors.New("SUPERVISOR_WRITE_FAILED")
	}
	var reply struct {
		OK         bool            `json:"ok"`
		Answer     json.RawMessage `json:"answer"`
		ReasonCode string          `json:"reasonCode"`
	}
	if json.NewDecoder(io.LimitReader(conn, controlStreamResponseLimit)).Decode(&reply) != nil {
		return errors.New("SUPERVISOR_RESPONSE_INVALID")
	}
	if !reply.OK {
		return errors.New(reply.ReasonCode)
	}
	_, err = os.Stdout.Write(append(reply.Answer, '\n'))
	return err
}
