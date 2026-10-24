package main

import (
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func restoredInteractionSpec(t *testing.T) attemptSpec {
	t.Helper()
	var question map[string]json.RawMessage
	_ = json.Unmarshal(questionFrame("restored-question").Payload, &question)
	delete(question, "roundKey")
	body, _ := json.Marshal(question)
	return attemptSpec{Continuation: &continuationSpec{CheckpointID: 17}, InteractionQuestionRequest: body, InteractionResponse: json.RawMessage(`{"interactionKey":"restored-question","responseId":"9007199254740993","answers":[{"questionKey":"scope","selectedOptionKeys":["small"],"text":""}]}`)}
}

func TestRestoredPendingRegistrationIsNotRequestOrReceipt(t *testing.T) {
	s := interactionServer(t)
	spec := restoredInteractionSpec(t)
	if err := s.state.registerRestoredInteraction(spec, 1); err != nil {
		t.Fatal(err)
	}
	s.state.mu.RLock()
	p := s.state.interaction.pending["restored-question"]
	if p == nil || p.accepted != nil || p.resolved || p.local || p.request.RoundKey != "round:1" || len(s.state.interaction.notifications) != 0 || len(s.state.interaction.receipts) != 0 {
		t.Error("private input fabricated request or receipt")
	}
	s.state.mu.RUnlock()
	s.state.endAgentRound("round:1")
	s.state.beginAgentRound("round:2")
	if err := s.state.registerRestoredInteraction(spec, 2); err != nil {
		t.Fatal(err)
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	if len(s.state.interaction.pending) != 0 {
		t.Fatal("old cold answer registered again in correction round")
	}
}

func TestOrdinaryStartAcceptsAbsentOrNullRestorePair(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"interactionQuestionRequest":null,"interactionResponse":null}`,
		`{"interactionQuestionRequest":null}`,
		`{"interactionResponse":null}`,
		`{"interactionQuestionRequest": null ,"interactionResponse": null }`,
	} {
		t.Run(body, func(t *testing.T) {
			s := interactionServer(t)
			var spec attemptSpec
			if err := json.Unmarshal([]byte(body), &spec); err != nil {
				t.Fatal(err)
			}
			if hasRestoredInteractionInput(spec) {
				t.Fatal("absent/null would be forwarded as restore input")
			}
			if err := s.state.registerRestoredInteraction(spec, 1); err != nil {
				t.Fatal(err)
			}
			if len(s.state.interaction.pending) != 0 || len(s.state.interaction.notifications) != 0 || len(s.state.interaction.receipts) != 0 {
				t.Fatal("ordinary start created restore state")
			}
		})
	}
}

func TestRestoredPendingRejectsUntrustedOrMismatchedPair(t *testing.T) {
	for _, kind := range []string{"no-continuation", "missing-question", "missing-answer", "null-question", "null-answer", "empty-object-pair", "empty-string-pair", "malformed-pair", "source-round", "question-key", "option-key", "unknown-field"} {
		t.Run(kind, func(t *testing.T) {
			s := interactionServer(t)
			spec := restoredInteractionSpec(t)
			switch kind {
			case "no-continuation":
				spec.Continuation = nil
			case "missing-question":
				spec.InteractionQuestionRequest = nil
			case "missing-answer":
				spec.InteractionResponse = nil
			case "null-question":
				spec.InteractionQuestionRequest = json.RawMessage(`null`)
			case "null-answer":
				spec.InteractionResponse = json.RawMessage(`null`)
			case "empty-object-pair":
				spec.InteractionQuestionRequest, spec.InteractionResponse = json.RawMessage(`{}`), json.RawMessage(`{}`)
			case "empty-string-pair":
				spec.InteractionQuestionRequest, spec.InteractionResponse = json.RawMessage(`""`), json.RawMessage(`""`)
			case "malformed-pair":
				spec.InteractionQuestionRequest, spec.InteractionResponse = json.RawMessage(`nul`), json.RawMessage(`nul`)
			case "source-round":
				spec.InteractionQuestionRequest = json.RawMessage(strings.Replace(string(spec.InteractionQuestionRequest), "{", `{"roundKey":"round:99",`, 1))
			case "question-key":
				spec.InteractionResponse = json.RawMessage(strings.Replace(string(spec.InteractionResponse), "scope", "other", 1))
			case "option-key":
				spec.InteractionResponse = json.RawMessage(strings.Replace(string(spec.InteractionResponse), "small", "other", 1))
			case "unknown-field":
				spec.InteractionQuestionRequest = json.RawMessage(strings.Replace(string(spec.InteractionQuestionRequest), "{", `{"taskId":"999",`, 1))
			}
			if err := s.state.registerRestoredInteraction(spec, 1); err == nil {
				t.Fatal("invalid pair admitted")
			}
			if len(s.state.interaction.pending) != 0 || len(s.state.interaction.notifications) != 0 {
				t.Fatal("failure mutated pending/notifications")
			}
		})
	}
}

func TestColdRestoredAnswerUsesNormalSDKReceiptWithoutNewQuestion(t *testing.T) {
	s := interactionServer(t)
	spec := restoredInteractionSpec(t)
	if err := s.state.registerRestoredInteraction(spec, 1); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(file), "../../../deploy/workforce/runtime-sdk/src")
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	fixture := `import asyncio
from seven_agent.force.agent import AgentAdapter,RoundResult,QuestionRequest
from seven_agent.force.interaction import InteractionResponse,answers_from_wire
agent=AgentAdapter(name="cold-receipt",version="1",capabilities=("interaction.request",))
@agent.run
async def run(request,ctx):
 await ctx.started()
 q=QuestionRequest.from_wire(request.inputs["interactionQuestionRequest"])
 a=request.inputs["interactionResponse"]
 expected=InteractionResponse(a["interactionKey"],a["responseId"],answers_from_wire(a["answers"]))
 waiter=asyncio.create_task(ctx.receive_restored_answer(q,expected))
 await asyncio.sleep(0)
 await ctx.emit_message("registered-only")
 await waiter
 return RoundResult.finished(session_reference="fixture-session")
@agent.resume
async def resume(request,ctx): return await run(request,ctx)
agent.serve()
`
	p, err := agentprotocol.OpenProcess(s.state.ctx, python, []string{"-c", fixture}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s.state.adapterProcess = p
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := p.Round(s.state.ctx, "round:1", map[string]any{"mode": "AGENT", "interactionQuestionRequest": spec.InteractionQuestionRequest, "interactionResponse": spec.InteractionResponse}, "fixture-session", nil, func(frame agentprotocol.Frame) {
			s.state.observeAgent(frame)
			if frame.Method == "message" {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		})
		done <- err
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("native ended early: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("SDK registration stalled")
	}
	s.state.mu.RLock()
	if s.state.interaction.pending["restored-question"].resolved || len(s.state.interaction.receipts) != 0 {
		t.Error("private expected answer produced receipt")
	}
	s.state.mu.RUnlock()
	answer := interactionRequest("agent-control")
	answer.Method = "interaction.respond"
	_, bound, err := agentprotocol.ValidateRestoredInteraction(spec.InteractionQuestionRequest, spec.InteractionResponse, "round:1")
	if err != nil {
		t.Fatal(err)
	}
	bound.ResponseID = "9007199254740994"
	answer.Payload, _ = json.Marshal(bound)
	if string(callInteraction(t, s, answer)["reasonCode"]) != `"SUPERVISOR_RESTORED_ANSWER_MISMATCH"` {
		t.Fatal("wrong saved response accepted")
	}
	bound.ResponseID = "9007199254740993"
	answer.Payload, _ = json.Marshal(bound)
	okResponse(t, callInteraction(t, s, answer))
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("normal answer did not unblock SDK")
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	requests, receipts := 0, 0
	for _, n := range s.state.interaction.notifications {
		if n.Frame.Method == "interaction.request" {
			requests++
		}
		if n.Frame.Method == "interaction.received" {
			receipts++
			if !strings.Contains(string(n.Frame.Payload), `"responseId":"9007199254740993"`) {
				t.Error("receipt identity lost")
			}
		}
	}
	if requests != 0 || receipts != 1 || !s.state.interaction.pending["restored-question"].resolved {
		t.Fatalf("requests=%d receipts=%d", requests, receipts)
	}
}
