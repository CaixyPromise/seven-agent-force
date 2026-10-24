package agentprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func interactionProcess(t *testing.T, ctx context.Context, source string) *Process {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(file), "../../../../deploy/workforce/runtime-sdk/src")
	p, err := OpenProcess(ctx, python, []string{"-c", source}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

const interactionFixture = `
import os,asyncio
from seven_agent.force.agent import AgentAdapter,RoundResult
from seven_agent.force.interaction import QuestionRequest,Question,QuestionOption,DeliveryStatus
agent=AgentAdapter(name="conformance",version="1",capabilities=("interaction.request",))
@agent.run
async def run(request,ctx):
    await ctx.started(native_pid=os.getpid())
    answer=await ctx.ask(QuestionRequest("question",(Question(key="scope",title="Scope?",kind="SINGLE_CHOICE",required=True,options=(QuestionOption("small","Small"),)),)))
    assert answer[0].selected_option_keys == ("small",)
    await asyncio.sleep(.15)
    return RoundResult.finished(summary="done",session_reference="session")
@agent.resume
async def resume(request,ctx): return await run(request,ctx)
@agent.message
async def message(message,ctx):
    await asyncio.sleep(.001)
    return DeliveryStatus.RECEIVED
agent.serve()
`

func testAnswer() InteractionAnswer {
	return InteractionAnswer{RoundKey: "r", InteractionKey: "question", ResponseID: "9223372036854775807", Answers: []Answer{{QuestionKey: "scope", SelectedOptionKeys: []string{"small"}, Text: ""}}}
}

func TestPythonConcurrentControlsAndDuplicateAnswersDoNotBlockRoundReader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := interactionProcess(t, ctx, interactionFixture)
	requested := make(chan struct{}, 1)
	var mu sync.Mutex
	receipts := map[string]Frame{}
	done := make(chan error, 1)
	go func() {
		result, err := p.Round(ctx, "r", map[string]any{"mode": "AGENT"}, "", nil, func(frame Frame) {
			if frame.Method == "interaction.request" {
				requested <- struct{}{}
			}
			if frame.Method == "interaction.received" || frame.Method == "message.received" {
				mu.Lock()
				receipts[frame.ReplyTo] = frame
				mu.Unlock()
			}
		})
		if err == nil && result.Outcome != "FINISHED" {
			err = fmt.Errorf("outcome %s", result.Outcome)
		}
		done <- err
	}()
	select {
	case <-requested:
	case <-ctx.Done():
		t.Fatal("question not emitted")
	}
	var workers sync.WaitGroup
	keys := make(chan string, 18)
	failures := make(chan error, 18)
	for n := 0; n < 16; n++ {
		workers.Add(1)
		go func(n int) {
			defer workers.Done()
			key, err := p.SendControl(ctx, "user.message", UserMessage{RoundKey: "r", MessageKey: fmt.Sprintf("m:%d", n), Text: "supplement"})
			if err != nil {
				failures <- err
			} else {
				keys <- key
			}
		}(n)
	}
	workers.Wait()
	for n := 0; n < 2; n++ {
		key, err := p.SendControl(ctx, "interaction.respond", testAnswer())
		if err != nil {
			t.Fatal(err)
		}
		keys <- key
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("controls blocked behind Round")
	}
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	close(keys)
	seen := map[string]bool{}
	mu.Lock()
	defer mu.Unlock()
	for key := range keys {
		if seen[key] {
			t.Fatal("duplicate wire key")
		}
		seen[key] = true
		if _, ok := receipts[key]; !ok {
			t.Errorf("missing correlated receipt %s", key)
		}
	}
	if len(receipts) != 18 {
		t.Fatalf("receipts=%d", len(receipts))
	}
}

func TestPythonToolResultReceiptAndContinuationCrossRealPipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fixture := `
import os
from seven_agent.force.agent import AgentAdapter,RoundResult,ActivityStatus
from seven_agent.force.tools import ToolCallRequest
agent=AgentAdapter(name="tool-conformance",version="1",capabilities=("tool.request",))
@agent.run
async def run(request,ctx):
    await ctx.started(native_pid=os.getpid())
    result=await ctx.request_tool(ToolCallRequest("native-call","network:1","run_command_with_network_access",dict(purpose="install",command="pnpm install",cwd=".",targets=[dict(host="registry.npmjs.org",protocol="https",port=443)])))
    assert result.operation_id == "91" and result.status == "SUCCEEDED"
    await ctx.tool_received(result)
    await ctx.emit_activity(key="continued",title="Continued after network operation",status=ActivityStatus.SUCCEEDED)
    return RoundResult.finished(summary="done",session_reference="session")
agent.serve()
`
	p := interactionProcess(t, ctx, fixture)
	methods := make([]string, 0, 3)
	result, err := p.Round(ctx, "r", map[string]any{"mode": "AGENT", "availableTools": []ToolDefinition{NetworkCommandToolDefinition()}}, "", nil, func(frame Frame) {
		methods = append(methods, frame.Method)
		if frame.Method != "tool.request" {
			return
		}
		var request ToolRequestPayload
		if json.Unmarshal(frame.Payload, &request) != nil || request.LocalToolCallKey != "network:1" || request.NativeToolCallID != "native-call" {
			t.Errorf("invalid tool request: %s", frame.Payload)
			return
		}
		exitCode := 0
		_, sendErr := p.SendControl(ctx, "tool.result", ToolResultPayload{RoundKey: "r", NativeToolCallID: "native-call", LocalToolCallKey: "network:1", OperationID: 91, Status: "SUCCEEDED", ExitCode: &exitCode, Summary: "installed", Output: "ok", ResultDigest: strings.Repeat("a", 64)})
		if sendErr != nil {
			t.Errorf("send tool result: %v", sendErr)
		}
	})
	if err != nil || result.Outcome != "FINISHED" {
		t.Fatalf("tool round: %+v %v", result, err)
	}
	if strings.Join(methods, ",") != "tool.request,tool.received,activity" {
		t.Fatalf("tool lifecycle order=%v", methods)
	}
}

func TestAdapterCannotRequestToolOutsideFrozenRoundAllowlist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fixture := `
import os
from seven_agent.force.agent import AgentAdapter,RoundResult
from seven_agent.force.tools import ToolCallRequest
agent=AgentAdapter(name="tool-conformance",version="1",capabilities=("tool.request",))
@agent.run
async def run(request,ctx):
    await ctx.started(native_pid=os.getpid())
    await ctx.request_tool(ToolCallRequest("native-git","git:1","request_git_delivery",dict(summary="Ready")))
    return RoundResult.finished(summary="must not finish",session_reference="session")
agent.serve()
`
	p := interactionProcess(t, ctx, fixture)
	defer p.Close()
	if _, err := p.Round(ctx, "r", map[string]any{"mode": "AGENT", "availableTools": []ToolDefinition{}}, "", nil, func(Frame) {}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unlisted tool request error = %v, want %v", err, ErrUnsupported)
	}
}

func TestAdapterToolCatalogMustMatchFrozenRoundAllowlist(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprint(mismatch), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			catalog := `("request_git_delivery", "run_command_with_network_access")`
			if mismatch {
				catalog = `("request_git_delivery",)`
			}
			fixture := `
import os
from seven_agent.force.agent import AgentAdapter,RoundResult
agent=AgentAdapter(name="tool-catalog",version="1",capabilities=("tool.request",))
@agent.run
async def run(request,ctx):
    await ctx.started(native_pid=os.getpid())
    await ctx.tool_catalog_ready(` + catalog + `)
    return RoundResult.finished(summary="done",session_reference="session")
agent.serve()
`
			p := interactionProcess(t, ctx, fixture)
			defer p.Close()
			tools := []ToolDefinition{GitDeliveryToolDefinition(), NetworkCommandToolDefinition()}
			observed := false
			result, err := p.Round(ctx, "r", map[string]any{"mode": "AGENT", "availableTools": tools}, "", nil, func(frame Frame) {
				if frame.Method == "tool.catalog" {
					observed = true
				}
			})
			if mismatch {
				if !errors.Is(err, ErrInvalid) || observed {
					t.Fatalf("mismatched catalog result=%+v err=%v observed=%v", result, err, observed)
				}
				return
			}
			if err != nil || result.Outcome != "FINISHED" || !observed {
				t.Fatalf("matching catalog result=%+v err=%v observed=%v", result, err, observed)
			}
		})
	}
}

func TestPythonControlFromObserveCallbackAndReviewerRejectsInteraction(t *testing.T) {
	for _, review := range []bool{false, true} {
		t.Run(fmt.Sprint(review), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			p := interactionProcess(t, ctx, interactionFixture)
			mode := "AGENT"
			if review {
				mode = "REVIEW"
			}
			observed := false
			_, err := p.Round(ctx, "r", map[string]any{"mode": mode}, "", nil, func(f Frame) {
				if f.Method == "interaction.request" {
					observed = true
					if _, err := p.SendControl(ctx, "interaction.respond", testAnswer()); err != nil {
						t.Error(err)
					}
				}
			})
			if review {
				if err == nil || observed {
					t.Fatal("Reviewer interaction accepted")
				}
			} else if err != nil || !observed {
				t.Fatalf("callback control: %v", err)
			}
		})
	}
}

func TestControlRejectsStaleRoundAndUnsupportedCapability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p := interactionProcess(t, ctx, interactionFixture)
	_, err := p.Round(ctx, "r", map[string]any{}, "", nil, func(f Frame) {
		if f.Method == "interaction.request" {
			if _, e := p.SendControl(ctx, "execution.suspend", map[string]any{"roundKey": "r"}); e != ErrUnsupported {
				t.Errorf("missing capability: %v", e)
			}
			bad := testAnswer()
			bad.RoundKey = "old"
			if _, e := p.SendControl(ctx, "interaction.respond", bad); e == nil {
				t.Error("stale round accepted")
			}
			_, _ = p.SendControl(ctx, "interaction.respond", testAnswer())
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.SendControl(ctx, "interaction.respond", testAnswer()); err == nil {
		t.Fatal("control accepted after round")
	}
}

func TestPythonSuspendReturnsDistinctOutcomeAndSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	fixture := `import os,asyncio
from seven_agent.force.agent import AgentAdapter,RoundResult
agent=AgentAdapter(name="suspend",version="1")
@agent.run
async def run(request,ctx):
    await ctx.started(native_pid=os.getpid())
    await asyncio.Event().wait()
@agent.resume
async def resume(request,ctx): return await run(request,ctx)
@agent.suspend
async def suspend(request,ctx): return "native-session"
agent.serve()
`
	p := interactionProcess(t, ctx, fixture)
	observed := false
	result, err := p.Round(ctx, "r", map[string]any{}, "", func(int) error {
		_, err := p.SendControl(ctx, "execution.suspend", map[string]any{"roundKey": "r"})
		return err
	}, func(f Frame) { observed = f.Method == "execution.suspended" })
	if err != nil || result.Outcome != "SUSPENDED" || result.SessionReference != "native-session" || !observed {
		t.Fatalf("suspension: %+v %v", result, err)
	}
	replayed, err := p.Round(ctx, "r", map[string]any{}, "", func(int) error { t.Error("suspended replay restarted native execution"); return nil }, nil)
	if err != nil || replayed != result {
		t.Fatalf("suspended replay: %+v %v", replayed, err)
	}
}

func TestPythonSpoofedReceiptCannotCorrelateResponseOrStart(t *testing.T) {
	for _, fault := range []string{"response", "reply", "method"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			fixture := `import sys,os,json
def read(): return json.loads(sys.stdin.readline())
def emit(method,payload,reply):
 print(json.dumps(dict(protocol="seven.agent-adapter/v1",method=method,key="adapter:1",replyTo=reply,payload=payload)),flush=True)
init=read()
emit("ready",dict(name="spoof",version="1",capabilities=["execution.start","execution.interrupt","interaction.request","interaction.respond","interaction.received","interaction.closed"]),init["key"])
start=read()
emit("execution.started",dict(roundKey="r",nativePid=os.getpid()),start["key"])
control=read()
payload=dict(roundKey="r",interactionKey="question",responseId="9223372036854775807",status="RECEIVED")
reply=control["key"]
method="interaction.received"
FAULT
emit(method,payload,reply)
`
			faultCode := map[string]string{"response": `payload["responseId"]="1"`, "reply": `reply=start["key"]`, "method": `method="execution.started";payload=dict(roundKey="r",nativePid=os.getpid())`}[fault]
			p := interactionProcess(t, ctx, strings.Replace(fixture, "FAULT", faultCode, 1))
			_, err := p.Round(ctx, "r", map[string]any{}, "", func(int) error { _, e := p.SendControl(ctx, "interaction.respond", testAnswer()); return e }, func(Frame) { t.Error("spoofed receipt observed") })
			if err == nil {
				t.Fatal("spoofed receipt accepted")
			}
		})
	}
}

func TestInteractionSchemaAlignsPythonCodec(t *testing.T) {
	request := QuestionRequest{RoundKey: "r", InteractionKey: "question", IsBlocking: true, Questions: []Question{{Key: "范围", Title: "Scope", Description: "details", Kind: "SINGLE_CHOICE", Required: true, AllowFreeText: false, Options: []QuestionOption{{Key: "小", Label: "Small"}}}, {Key: "optional", Title: "Optional", Kind: "TEXT", Options: []QuestionOption{}}}}
	answer := testAnswer()
	answer.Answers = []Answer{{QuestionKey: "范围", SelectedOptionKeys: []string{"小"}, Text: ""}}
	if err := ValidateAnswer(request, answer); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(request)
	cases := []struct {
		method  string
		payload json.RawMessage
		valid   bool
	}{{"interaction.request", body, true}}
	for _, value := range []any{answer, InteractionAnswer{RoundKey: "r", InteractionKey: "question", ResponseID: "1", Answers: []Answer{}}, map[string]any{"roundKey": "r", "interactionKey": "question", "responseId": "9223372036854775808", "answers": []any{}}, map[string]any{"roundKey": "r", "interactionKey": "question", "responseId": 1, "answers": []any{}}} {
		b, _ := json.Marshal(value)
		cases = append(cases, struct {
			method  string
			payload json.RawMessage
			valid   bool
		}{"interaction.respond", b, len(cases) < 3})
	}
	cases = append(cases, struct {
		method  string
		payload json.RawMessage
		valid   bool
	}{"interaction.request", json.RawMessage(strings.Replace(string(body), `"required":true,`, "", 1)), false})
	for _, c := range cases {
		raw, _ := json.Marshal(Frame{Protocol: Version, Method: c.method, Key: "frame", Payload: c.payload})
		raw = append(raw, '\n')
		direction := "up"
		if c.method == "interaction.respond" {
			direction = "down"
		}
		_, err := Decode(raw, direction)
		if (err == nil) != c.valid {
			t.Fatalf("Go schema %s: %v", c.payload, err)
		}
		_, file, _, _ := runtime.Caller(0)
		sdk := filepath.Join(filepath.Dir(file), "../../../../deploy/workforce/runtime-sdk/src")
		cmd := exec.Command("python3", "-c", `import sys
from seven_agent.force._protocol import decode
decode(sys.stdin.buffer.readline(),direction=sys.argv[1])`, direction)
		cmd.Env = []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"}
		cmd.Stdin = strings.NewReader(string(raw))
		err = cmd.Run()
		if (err == nil) != c.valid {
			t.Fatalf("Python schema disagrees for %s", c.payload)
		}
	}
}

func TestTerminalFrameDrainsAlreadyWrittenControlReceipt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	fixture := `import sys,os,json,time
def read(): return json.loads(sys.stdin.readline())
def emit(method,payload,reply): print(json.dumps(dict(protocol="seven.agent-adapter/v1",method=method,key="a:1",replyTo=reply,payload=payload)),flush=True)
init=read()
emit("ready",dict(name="late",version="1",capabilities=["execution.start","execution.interrupt","user.message","message.received"]),init["key"])
start=read()
emit("execution.started",dict(roundKey="r",nativePid=os.getpid()),start["key"])
control=read()
emit("execution.finished",dict(roundKey="r",outcome="FINISHED",summary="done",sessionReference="s"),start["key"])
time.sleep(.01)
emit("message.received",dict(roundKey="r",messageKey="m",status="UNKNOWN"),control["key"])
`
	p := interactionProcess(t, ctx, fixture)
	observed := false
	result, err := p.Round(ctx, "r", map[string]any{}, "", func(int) error {
		_, err := p.SendControl(ctx, "user.message", UserMessage{RoundKey: "r", MessageKey: "m", Text: "queued"})
		return err
	}, func(f Frame) {
		if f.Method == "message.received" {
			observed = true
		}
	})
	if err != nil || result.Outcome != "FINISHED" || !observed {
		t.Fatalf("late delivery lost: %+v %v", result, err)
	}
}
