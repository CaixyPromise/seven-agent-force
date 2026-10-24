package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

type testInteractionClock struct {
	mu        sync.Mutex
	now       time.Time
	timers    []*testInteractionTimer
	scheduled chan struct{}
}
type testInteractionTimer struct {
	clock    *testInteractionClock
	deadline time.Time
	ch       chan time.Time
	stopped  bool
}

func newTestInteractionClock() *testInteractionClock {
	return &testInteractionClock{now: time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC), scheduled: make(chan struct{}, 16)}
}
func (c *testInteractionClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *testInteractionClock) NewTimer(d time.Duration) interactionTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &testInteractionTimer{clock: c, deadline: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, timer)
	c.scheduled <- struct{}{}
	return timer
}
func (t *testInteractionTimer) C() <-chan time.Time { return t.ch }
func (t *testInteractionTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := !t.stopped
	t.stopped = true
	return was
}
func (c *testInteractionClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, timer := range c.timers {
		if !timer.stopped && !timer.deadline.After(c.now) {
			timer.stopped = true
			timer.ch <- c.now
		}
	}
}

func TestInteractionBudgetPausesDuringQuestionAndAccumulatesAcrossRounds(t *testing.T) {
	s := interactionServer(t)
	clock := newTestInteractionClock()
	s.state.interaction = supervisorInteractions{clock: clock}
	s.state.beginAgentRound("round:1")
	clock.Advance(10 * time.Second)
	s.state.observeAgent(questionFrame("q"))
	clock.Advance(9 * time.Minute)
	s.state.mu.RLock()
	snapshot := protocolStateSnapshot(s.state)
	s.state.mu.RUnlock()
	if snapshot["activeElapsedMillis"] != "10000" || snapshot["interactionWaitingMillis"] != "540000" {
		t.Fatalf("wait charged active budget: %v", snapshot)
	}
	body, _ := json.Marshal(map[string]any{"roundKey": "round:1", "interactionKey": "q", "responseId": "1", "status": "RECEIVED"})
	s.state.observeAgent(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "interaction.received", Key: "receipt", ReplyTo: "answer", Payload: body})
	clock.Advance(20 * time.Second)
	s.state.endAgentRound("round:1")
	clock.Advance(time.Minute)
	s.state.beginAgentRound("round:2")
	clock.Advance(5 * time.Second)
	s.state.mu.RLock()
	snapshot = protocolStateSnapshot(s.state)
	s.state.mu.RUnlock()
	if snapshot["activeElapsedMillis"] != "35000" || snapshot["interactionWaitingMillis"] != "540000" {
		t.Fatalf("cumulative budget: %v", snapshot)
	}
}

func TestNonBlockingQuestionDoesNotPauseActiveBudget(t *testing.T) {
	s := interactionServer(t)
	clock := newTestInteractionClock()
	s.state.interaction = supervisorInteractions{clock: clock}
	s.state.beginAgentRound("round:1")
	frame := questionFrame("nonblocking")
	var request agentprotocol.QuestionRequest
	_ = json.Unmarshal(frame.Payload, &request)
	request.IsBlocking = false
	frame.Payload, _ = json.Marshal(request)
	s.state.observeAgent(frame)
	clock.Advance(time.Minute)
	s.state.mu.RLock()
	snapshot := protocolStateSnapshot(s.state)
	s.state.mu.RUnlock()
	if snapshot["activeElapsedMillis"] != "60000" || snapshot["interactionWaitingMillis"] != "0" || snapshot["interactionWaiting"] != false {
		t.Fatalf("nonblocking question paused execution: %v", snapshot)
	}
}

func TestInteractionHotWaitExpiresIntoPausingWithoutFalseCapture(t *testing.T) {
	s := interactionServer(t)
	clock := newTestInteractionClock()
	s.state.interaction = supervisorInteractions{clock: clock, hotWaitLimit: 2 * time.Second}
	s.state.beginAgentRound("round:1")
	s.state.observeAgent(questionFrame("q"))
	done := make(chan struct{})
	go func() { s.state.monitorInteractionWait(s.state.ctx, "round:1", nil); close(done) }()
	select {
	case <-clock.scheduled:
	case <-time.After(time.Second):
		t.Fatal("timer not installed")
	}
	clock.Advance(2 * time.Second)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fakeclock timeout not handled")
	}
	s.state.mu.RLock()
	snapshot := protocolStateSnapshot(s.state)
	s.state.mu.RUnlock()
	if snapshot["state"] != "PAUSING_PREPARE" || snapshot["quiescenceHeld"] != false || snapshot["interactionWaitingMillis"] != "2000" || s.state.ctx.Err() != nil {
		t.Fatalf("hotwait state: %v", snapshot)
	}
	r := interactionRequest("agent-control")
	r.Method = "user.message"
	r.Payload = json.RawMessage(`{"roundKey":"round:1","messageKey":"m","text":"supplement"}`)
	response := callInteraction(t, s, r)
	if string(response["reasonCode"]) != `"SUPERVISOR_PAUSING"` {
		t.Fatal("new message not explicitly rejected while pausing")
	}
	if string(callInteraction(t, s, interactionRequest("interaction-capture"))["ok"]) != "false" {
		t.Fatal("unsupported suspend invented quiescence")
	}
	clock.Advance(time.Minute)
	s.state.mu.RLock()
	snapshot = protocolStateSnapshot(s.state)
	s.state.mu.RUnlock()
	if snapshot["interactionWaitingMillis"] != "2000" || snapshot["activeElapsedMillis"] != "0" {
		t.Fatal("pausing charged active/wait budget")
	}
}

func TestInteractionHotWaitIsOperatorConfigOnly(t *testing.T) {
	state := &supervisorState{}
	t.Setenv("SEVEN_INTERACTION_HOT_WAIT", "")
	if err := state.configureInteractionWait(); err != nil || state.interaction.hotWaitLimit != 10*time.Minute {
		t.Fatal("default hotwait")
	}
	t.Setenv("SEVEN_INTERACTION_HOT_WAIT", "50ms")
	if err := state.configureInteractionWait(); err != nil || state.interaction.hotWaitLimit != 50*time.Millisecond {
		t.Fatal("operator override")
	}
	for _, bad := range []string{"0s", "-1s", "25h", "garbage"} {
		t.Setenv("SEVEN_INTERACTION_HOT_WAIT", bad)
		if state.configureInteractionWait() == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestSuspendPrepareAbortKeepsQuestionAndRejectsLateCommit(t *testing.T) {
	s := interactionServer(t)
	s.state.observeAgent(questionFrame("q"))
	s.state.mu.Lock()
	s.state.prepareInteractionSuspension("HOT_WAIT_EXPIRED")
	key := s.state.interaction.suspendControlKey
	pending := s.state.interaction.pending["q"]
	pending.local = true
	pending.answer = make(chan struct{})
	s.state.mu.Unlock()
	r := interactionRequest("interaction-suspend-abort")
	r.Payload, _ = json.Marshal(map[string]string{"suspendControlKey": key})
	okResponse(t, callInteraction(t, s, r))
	okResponse(t, callInteraction(t, s, r))
	r.Command = "interaction-suspend-commit"
	if string(callInteraction(t, s, r)["ok"]) != "false" {
		t.Fatal("commit after ANSWER_WON accepted")
	}
	s.state.mu.RLock()
	if s.state.State != "RUNNING" || s.state.interaction.pending["q"] != pending || pending.closed || s.state.interaction.adapterStopped {
		t.Error("abort lost live question")
	}
	s.state.mu.RUnlock()
	r = interactionRequest("agent-control")
	r.Method = "interaction.respond"
	r.Payload = json.RawMessage(`{"roundKey":"round:1","interactionKey":"q","responseId":"1","answers":[{"questionKey":"scope","selectedOptionKeys":["small"],"text":""}]}`)
	okResponse(t, callInteraction(t, s, r))
	select {
	case <-pending.answer:
	default:
		t.Fatal("answer not delivered after abort")
	}
}

func TestNativeSuspensionHoldsUntilParentInterruptAndCaptureIsFenced(t *testing.T) {
	s := interactionServer(t)
	s.state.sessionArchivePath = filepath.Join(t.TempDir(), "session.json")
	_, file, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(file), "../../../deploy/workforce/runtime-sdk/src")
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	fixture := `import os,asyncio
from seven_agent.force.agent import AgentAdapter
agent=AgentAdapter(name="hold",version="1")
@agent.run
async def run(request,ctx):
 await ctx.started(native_pid=os.getpid())
 await asyncio.Event().wait()
@agent.resume
async def resume(request,ctx): return await run(request,ctx)
@agent.suspend
async def suspend(request,ctx): return "private-native-reference"
agent.serve()
`
	p, err := agentprotocol.OpenProcess(s.state.ctx, python, []string{"-c", fixture}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s.state.adapterProcess = p
	done := make(chan error, 1)
	startedRound := make(chan struct{})
	go func() {
		result, err := p.Round(s.state.ctx, "round:1", map[string]any{}, "", func(int) error {
			close(startedRound)
			return nil
		}, s.state.observeAgent)
		if err == nil {
			err = s.state.holdInteractionSuspension(s.state.ctx, p, result)
		}
		done <- err
	}()
	select {
	case <-startedRound:
	case <-time.After(time.Second):
		t.Fatal("native did not start")
	}
	s.state.mu.Lock()
	s.state.prepareInteractionSuspension("HOT_WAIT_EXPIRED")
	prepareKey := s.state.interaction.suspendControlKey
	if s.state.interaction.adapterStopped {
		t.Error("prepare stopped native")
	}
	s.state.mu.Unlock()
	if string(callInteraction(t, s, interactionRequest("interaction-capture"))["ok"]) != "false" {
		t.Fatal("capture before reserve/commit")
	}
	commit := interactionRequest("interaction-suspend-commit")
	commit.Payload = json.RawMessage(`{"suspendControlKey":"wrong"}`)
	if string(callInteraction(t, s, commit)["ok"]) != "false" {
		t.Fatal("wrong reservation key accepted")
	}
	commit.Payload, _ = json.Marshal(map[string]string{"suspendControlKey": prepareKey})
	okResponse(t, callInteraction(t, s, commit))
	okResponse(t, callInteraction(t, s, commit))
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.state.mu.RLock()
		ready := s.state.State == "INTERACTION_SUSPENDED"
		s.state.mu.RUnlock()
		if ready {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("exited before checkpoint: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("capture never ready")
		}
		time.Sleep(time.Millisecond)
	}
	response := callInteraction(t, s, interactionRequest("interaction-capture"))
	okResponse(t, response)
	var capture map[string]any
	_ = json.Unmarshal(response["capture"], &capture)
	if capture["purpose"] != "INTERACTION_SUSPEND" || capture["adapterStopped"] != true || capture["quiescenceHeld"] != true || capture["sessionDigest"] != sha256Text("private-native-reference") {
		t.Fatalf("capture: %v", capture)
	}
	if capture["sessionReference"] != nil {
		t.Fatal("raw session reference exposed")
	}
	for _, command := range []string{"interaction-capture", "agent-control"} {
		r := interactionRequest(command)
		r.SessionGeneration = 2
		if string(callInteraction(t, s, r)["ok"]) != "false" {
			t.Fatal("stale capture/control accepted")
		}
	}
	if _, err := os.Stat(s.state.sessionArchivePath); err != nil {
		t.Fatal("private session archive absent")
	}
	select {
	case err := <-done:
		t.Fatalf("suspended became failure/completion: %v", err)
	default:
	}
	s.state.mu.RLock()
	if s.state.EvaluationCandidate != "" || s.state.EvaluationCycle != 0 {
		t.Error("suspend entered evaluation")
	}
	s.state.mu.RUnlock()
	okResponse(t, callInteraction(t, s, interactionRequest("interaction-capture")))
	s.state.cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("parent cleanup: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("held quiescence ignored parent cancellation")
	}
}

func TestPausingRacingNativeFinishCannotEnterEvaluation(t *testing.T) {
	s := interactionServer(t)
	_, file, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(file), "../../../deploy/workforce/runtime-sdk/src")
	fixture := `from seven_agent.force.agent import AgentAdapter,RoundResult
agent=AgentAdapter(name="finish-race",version="1")
@agent.run
async def run(request,ctx):
 await ctx.started()
 return RoundResult.finished(summary="done",session_reference="session")
agent.serve()
`
	p, err := agentprotocol.OpenProcess(s.state.ctx, "python3", []string{"-c", fixture}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s.state.adapterProcess = p
	done := make(chan error, 1)
	go func() {
		_, err := runAgentAdapterRound(s.state.ctx, attemptSpec{TaskID: 1, AttemptID: 2, Generation: 3}, s.state, 1, func(int) error {
			s.state.mu.Lock()
			s.state.enterInteractionPausing("WORKER_REQUESTED")
			s.state.mu.Unlock()
			return nil
		})
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.state.mu.RLock()
		ready := s.state.interaction.captureError == "ADAPTER_SUSPEND_NOT_CONFIRMED"
		s.state.mu.RUnlock()
		if ready {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("normal finish escaped PAUSING: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("failed safe point not held")
		}
		time.Sleep(time.Millisecond)
	}
	s.state.mu.RLock()
	if s.state.State != "PAUSING" || s.state.interaction.quiescenceHeld || s.state.EvaluationCycle != 0 {
		t.Error("unconfirmed suspension became capture/evaluation")
	}
	s.state.mu.RUnlock()
	s.state.cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("held failure ignored cancellation")
	}
}
