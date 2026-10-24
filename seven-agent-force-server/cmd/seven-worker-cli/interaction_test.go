package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func interactionServer(t *testing.T) *controlServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	state := &supervisorState{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, State: "RUNNING", ctx: ctx, cancel: cancel}
	state.beginAgentRound("round:1")
	return &controlServer{state: state}
}
func interactionRequest(command string) attemptControlRequest {
	return attemptControlRequest{SchemaVersion: 2, Command: command, TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, CurrentRound: "round:1", SessionGeneration: 1}
}
func questionFrame(key string) agentprotocol.Frame {
	body, _ := json.Marshal(agentprotocol.QuestionRequest{RoundKey: "round:1", InteractionKey: key, IsBlocking: true, Questions: []agentprotocol.Question{{Key: "scope", Title: "Scope", Description: "description", Kind: "SINGLE_CHOICE", Required: true, Options: []agentprotocol.QuestionOption{{Key: "small", Label: "Small"}}}}})
	return agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "interaction.request", Key: key, ReplyTo: "start:1", Payload: body}
}
func callInteraction(t *testing.T, s *controlServer, r attemptControlRequest) map[string]json.RawMessage {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	go func() { defer server.Close(); s.handleInteractionControl(server, r) }()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	var response map[string]json.RawMessage
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}
func okResponse(t *testing.T, response map[string]json.RawMessage) {
	t.Helper()
	if string(response["ok"]) != "true" {
		t.Fatalf("response %s", response)
	}
}

func TestInteractionNotificationsRetainFullSchemaUntilSpoolAck(t *testing.T) {
	s := interactionServer(t)
	s.state.observeAgent(questionFrame("q"))
	for n := 0; n < 40; n++ {
		body, _ := json.Marshal(map[string]any{"roundKey": "round:1", "text": "display"})
		s.state.observeAgent(agentprotocol.Frame{Method: "message", Payload: body})
	}
	request := interactionRequest("notifications")
	for n := 0; n < 2; n++ {
		response := callInteraction(t, s, request)
		okResponse(t, response)
		if string(response["through"]) != `"41"` || string(response["hasMore"]) != "false" || string(response["activityGap"]) != "true" || string(response["activityDroppedThrough"]) != `"33"` {
			t.Fatal("wrong cursor")
		}
		var batch []agentNotification
		_ = json.Unmarshal(response["notifications"], &batch)
		if len(batch) != 9 || batch[0].Sequence != 1 || batch[0].SessionGeneration != 1 || batch[0].RoundKey != "round:1" || batch[1].Sequence != 34 {
			t.Fatal("critical event lost to activity")
		}
		var question agentprotocol.QuestionRequest
		_ = json.Unmarshal(batch[0].Frame.Payload, &question)
		if question.Questions[0].Description != "description" || !question.Questions[0].Required || len(question.Questions[0].Options) != 1 {
			t.Fatal("question schema stripped")
		}
	}
	request.After = 41
	response := callInteraction(t, s, request)
	okResponse(t, response)
	if string(response["notifications"]) != "[]" {
		t.Fatal("after ignored")
	}
	s.state.mu.RLock()
	if len(s.state.interaction.notifications) != 1 {
		t.Error("read removed unpersisted frame")
	}
	s.state.mu.RUnlock()
	ack := interactionRequest("notifications-received")
	ack.Through = 41
	ack.OwnerEpoch = 3
	if string(callInteraction(t, s, ack)["ok"]) != "false" {
		t.Fatal("stale owner acked")
	}
	ack.OwnerEpoch = 4
	okResponse(t, callInteraction(t, s, ack))
	okResponse(t, callInteraction(t, s, ack))
	request.After = 0
	response = callInteraction(t, s, request)
	if string(response["notifications"]) != "[]" || string(response["through"]) != `"41"` || string(response["activityGap"]) != "false" {
		t.Fatal("ack not applied")
	}
	s.state.mu.RLock()
	snapshot := protocolStateSnapshot(s.state)
	s.state.mu.RUnlock()
	if snapshot["sessionGeneration"] != "1" || snapshot["currentRound"] != "round:1" || snapshot["pendingNotifications"] != "0" {
		t.Fatalf("binding snapshot %v", snapshot)
	}
}

func TestToolCatalogIsCriticalAndSurvivesActivityWindowEviction(t *testing.T) {
	s := interactionServer(t)
	names := []string{agentprotocol.GitDeliveryToolName, agentprotocol.NetworkCommandToolName}
	digest, err := agentprotocol.DigestToolCatalogNames(names)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(agentprotocol.ToolCatalogPayload{
		RoundKey:      "round:1",
		ToolNames:     names,
		CatalogDigest: digest,
	})
	s.state.observeAgent(agentprotocol.Frame{
		Protocol: agentprotocol.Version,
		Method:   "tool.catalog",
		Key:      "catalog:1",
		ReplyTo:  "start:1",
		Payload:  body,
	})
	for n := 0; n < observationWindow+5; n++ {
		activity, _ := json.Marshal(map[string]any{"roundKey": "round:1", "text": "display"})
		s.state.observeAgent(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "message", Payload: activity})
	}

	response := callInteraction(t, s, interactionRequest("notifications"))
	okResponse(t, response)
	var batch []agentNotification
	_ = json.Unmarshal(response["notifications"], &batch)
	if len(batch) != observationWindow+1 || batch[0].Frame.Method != "tool.catalog" || batch[0].Sequence != 1 {
		t.Fatalf("tool catalog was evicted or reordered: %#v", batch)
	}
	if string(response["activityGap"]) != "true" {
		t.Fatal("activity eviction was not reported")
	}
}

func TestInteractionBackpressureNeverEvictsAndCancelWins(t *testing.T) {
	s := interactionServer(t)
	for n := 0; n < criticalNotificationLimit+1; n++ {
		body, _ := json.Marshal(map[string]any{"roundKey": "round:1", "messageKey": fmt.Sprintf("m:%d", n), "status": "RECEIVED"})
		s.state.observeAgent(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "message.received", Key: fmt.Sprintf("r:%d", n), ReplyTo: fmt.Sprintf("c:%d", n), Payload: body})
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	if len(s.state.interaction.notifications) != criticalNotificationLimit || !s.state.interaction.overflow || s.state.ctx.Err() == nil {
		t.Fatal("overflow must cancel without eviction")
	}
	if s.state.interaction.notifications[0].Sequence != 1 || s.state.State != "RUNNING" {
		t.Fatal("overflow discarded evidence or invented Task state")
	}
}

func TestInteractionQuestionDuplicateAndBindingFences(t *testing.T) {
	s := interactionServer(t)
	frame := questionFrame("q")
	s.state.observeAgent(frame)
	s.state.observeAgent(frame)
	if len(s.state.interaction.notifications) != 1 {
		t.Fatal("duplicate question emitted twice")
	}
	for _, field := range []string{"round", "session", "owner"} {
		r := interactionRequest("agent-control")
		r.Method = "interaction.respond"
		r.Payload = json.RawMessage(`{"roundKey":"round:1","interactionKey":"q","responseId":"1","answers":[{"questionKey":"scope","selectedOptionKeys":["small"],"text":""}]}`)
		switch field {
		case "round":
			r.CurrentRound = "round:2"
		case "session":
			r.SessionGeneration = 2
		case "owner":
			r.OwnerEpoch = 3
		}
		if string(callInteraction(t, s, r)["ok"]) != "false" {
			t.Fatalf("%s fence missing", field)
		}
	}
	frame.Payload = json.RawMessage(strings.Replace(string(frame.Payload), "Scope", "changed", 1))
	s.state.observeAgent(frame)
	if s.state.ctx.Err() == nil {
		t.Fatal("same-key changed question accepted")
	}
}

func TestInteractionAskUsesSameUDSRuntimeAndReplaysLocalAnswer(t *testing.T) {
	s := interactionServer(t)
	_, file, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(file), "../../../deploy/workforce/runtime-sdk/src")
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	fixture := `from seven_agent.force.agent import AgentAdapter
agent=AgentAdapter(name="uds",version="1",capabilities=("interaction.request",))
@agent.run
async def run(request,ctx): pass
agent.serve()
`
	p, err := agentprotocol.OpenProcess(s.state.ctx, python, []string{"-c", fixture}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s.state.adapterProcess = p
	frame := questionFrame("cli-question")
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(frame.Payload, &raw)
	delete(raw, "roundKey")
	payload, _ := json.Marshal(raw)
	ask := attemptControlRequest{SchemaVersion: 2, Command: "interaction-ask", Payload: payload}
	forged := ask
	forged.TaskID = 999
	if string(callInteraction(t, s, forged)["ok"]) != "false" {
		t.Fatal("caller chose Task")
	}
	client, server := net.Pipe()
	defer client.Close()
	go func() { defer server.Close(); s.handleInteractionControl(server, ask) }()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(time.Second)
	for {
		s.state.mu.RLock()
		ready := len(s.state.interaction.notifications) == 1
		s.state.mu.RUnlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("CLI question not queued")
		}
		time.Sleep(time.Millisecond)
	}
	answer := interactionRequest("agent-control")
	concurrent := ask
	concurrent.Payload = json.RawMessage(strings.Replace(string(ask.Payload), "Scope", "Changed", 1))
	if string(callInteraction(t, s, concurrent)["reasonCode"]) != `"ADAPTER_IDEMPOTENCY_CONFLICT"` {
		t.Fatal("same-key changed local ask was not rejected")
	}
	answer.Method = "interaction.respond"
	answer.Payload = json.RawMessage(`{"roundKey":"round:1","interactionKey":"cli-question","responseId":"9007199254740993","answers":[{"questionKey":"scope","selectedOptionKeys":["small"],"text":""}]}`)
	okResponse(t, callInteraction(t, s, answer))
	var answered map[string]json.RawMessage
	if err := json.NewDecoder(client).Decode(&answered); err != nil {
		t.Fatal(err)
	}
	okResponse(t, answered)
	if !strings.Contains(string(answered["answer"]), `"responseId":"9007199254740993"`) {
		t.Fatal("response identity lost")
	}
	okResponse(t, callInteraction(t, s, answer))
	okResponse(t, callInteraction(t, s, ask))
	response := callInteraction(t, s, interactionRequest("notifications"))
	var batch []agentNotification
	_ = json.Unmarshal(response["notifications"], &batch)
	if len(batch) != 2 || batch[1].Frame.Method != "interaction.received" {
		t.Fatal("local delivery did not retain exactly one receipt")
	}
	if !strings.Contains(string(batch[1].Frame.Payload), `"responseId":"9007199254740993"`) {
		t.Fatal("receipt missing responseId")
	}
	answer.Payload = json.RawMessage(strings.Replace(string(answer.Payload), "9007199254740993", "9007199254740994", 1))
	if string(callInteraction(t, s, answer)["ok"]) != "false" {
		t.Fatal("different responseId replayed")
	}
	if s.state.State != "RUNNING" {
		t.Fatal("question answer changed Task lifecycle")
	}
}

func TestNativePendingRequestsAreBoundedAtEight(t *testing.T) {
	s := interactionServer(t)
	for index := 0; index < unresolvedInteractionLimit; index++ {
		s.state.observeAgent(questionFrame(fmt.Sprintf("question-%d", index)))
	}
	s.state.observeAgent(questionFrame("ninth"))
	s.state.mu.RLock()
	if len(s.state.interaction.pending) != unresolvedInteractionLimit || len(s.state.interaction.notifications) != unresolvedInteractionLimit || s.state.ctx.Err() != nil || s.state.interaction.admissionError != "ADAPTER_INTERACTION_BUSY" {
		t.Error("ninth request admitted or existing requests were damaged")
	}
	s.state.mu.RUnlock()
	body, _ := json.Marshal(map[string]any{"roundKey": "round:1", "interactionKey": "question-0", "responseId": "1", "status": "RECEIVED"})
	s.state.observeAgent(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "interaction.received", Key: "receipt", ReplyTo: "answer", Payload: body})
	s.state.observeAgent(questionFrame("ninth"))
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	if s.state.interaction.pending["ninth"] == nil {
		t.Fatal("next request after resolution rejected")
	}
}

func TestPythonChatFramesReachNotificationPollBelowProtocolLimit(t *testing.T) {
	s := interactionServer(t)
	_, file, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(file), "../../../deploy/workforce/runtime-sdk/src")
	fixture := `from seven_agent.force.agent import AgentAdapter,RoundResult,ActivityStatus
agent=AgentAdapter(name="chat",version="1")
@agent.run
async def run(request,ctx):
 await ctx.started()
 await ctx.emit_message("api_key=sk-privatefixture012345") # gitleaks:allow -- synthetic redaction fixture
 await ctx.emit_plan(["Inspect","Implement"])
 await ctx.emit_activity(key="edit",title="Editing",status=ActivityStatus.STARTED)
 return RoundResult.finished(summary="done",session_reference="s")
agent.serve()
`
	p, err := agentprotocol.OpenProcess(s.state.ctx, "python3", []string{"-c", fixture}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err = p.Round(s.state.ctx, "round:1", map[string]any{}, "", nil, s.state.observeAgent); err != nil {
		t.Fatal(err)
	}
	s.state.endAgentRound("round:1")
	response := callInteraction(t, s, interactionRequest("notifications"))
	okResponse(t, response)
	var events []agentNotification
	_ = json.Unmarshal(response["notifications"], &events)
	if len(events) != 3 || string(response["through"]) != `"3"` || string(response["activityGap"]) != "false" {
		t.Fatal("chat frames missing")
	}
	for n, method := range []string{"message", "plan", "activity"} {
		event := events[n]
		body, _ := json.Marshal(event.Frame)
		if event.Frame.Method != method || event.Sequence != int64(n+1) || len(body)+1 >= 64<<10 {
			t.Fatal("chat order or bound")
		}
		if _, err := agentprotocol.Decode(append(body, '\n'), "up"); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "sk-privatefixture") {
			t.Fatal("credential escaped projection")
		}
	}
	ack := interactionRequest("notifications-received")
	ack.Through = 3
	okResponse(t, callInteraction(t, s, ack))
	if string(callInteraction(t, s, interactionRequest("notifications"))["notifications"]) != "[]" {
		t.Fatal("chat spool ack not applied")
	}
}
