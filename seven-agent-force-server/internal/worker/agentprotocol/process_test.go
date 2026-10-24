package agentprotocol

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPythonDuplicateRoundAcknowledgesWithoutRestartAndReplaysResult(t *testing.T) {
	executable, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	_, source, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(source), "../../../../deploy/workforce/runtime-sdk/src")
	fixture := strings.Replace(pythonFixture, "import os", "import os, asyncio\nstarts = 0", 1)
	fixture = strings.Replace(fixture, "    await context.started", "    global starts\n    starts += 1\n    assert starts == 1\n    await context.started", 1)
	fixture = strings.Replace(fixture, "    await context.emit_message", "    await asyncio.sleep(0.05)\n    await context.emit_message", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := OpenProcess(ctx, executable, []string{"-c", fixture}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	starts := 0
	started := func(pid int) error {
		starts++
		// Duplicate the actual active wire request, preserving its key. The
		// SDK must ack it, never spawn another native execution.
		body, _ := json.Marshal(map[string]any{"roundKey": "r1", "inputs": map[string]any{}})
		raw, _ := json.Marshal(Frame{Protocol: Version, Method: "execution.start", Key: "supervisor:2", Payload: body})
		_, err := p.input.Write(append(raw, '\n'))
		return err
	}
	first, err := p.Round(ctx, "r1", map[string]any{}, "", started, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Round(ctx, "r1", map[string]any{}, "", func(int) error { starts++; return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || starts != 1 {
		t.Fatalf("duplicate restarted or changed result: starts=%d", starts)
	}
	if _, err := p.Round(ctx, "r1", map[string]any{"changed": true}, "", nil, nil); err == nil {
		t.Fatal("conflicting round accepted")
	}
}

func TestJSONLineRejectsTruncatedFinalFrame(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(`{"method":"ready"}`))
	scanner.Split(jsonLine)
	if scanner.Scan() || scanner.Err() == nil {
		t.Fatal("unterminated stdout must not become a valid frame")
	}
}

func TestReplayRejectsChangedCachedResult(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	_, source, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(source), "../../../../deploy/workforce/runtime-sdk/src")
	fixture := `import sys,os
from seven_agent.force._protocol import decode,encode
def read(): return decode(sys.stdin.buffer.readline(),direction="down")
def emit(method,payload,reply):
    sys.stdout.buffer.write(encode(dict(protocol="seven.agent-adapter/v1",method=method,key="a:1",replyTo=reply,payload=payload)));sys.stdout.buffer.flush()
init=read()
emit("ready",dict(name="bad-cache",version="1",capabilities=[]),init["key"])
first=read()
emit("execution.started",dict(roundKey="r",nativePid=os.getpid()),first["key"])
emit("execution.finished",dict(roundKey="r",outcome="FINISHED",summary="original",sessionReference="s"),first["key"])
second=read()
emit("execution.finished",dict(roundKey="r",outcome="FINISHED",summary="changed",sessionReference="s"),second["key"])
`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := OpenProcess(ctx, python, []string{"-c", fixture}, []string{"PYTHONPATH=" + sdk})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Round(ctx, "r", map[string]any{}, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Round(ctx, "r", map[string]any{}, "", nil, nil); err == nil || err.Error() != "ADAPTER_REPLAY_MISMATCH" {
		t.Fatalf("changed result: %v", err)
	}
}

func TestPythonAckCannotInventStartupOrCorrelateAnotherRequest(t *testing.T) {
	for _, wrongReply := range []bool{false, true} {
		t.Run(strconv.FormatBool(wrongReply), func(t *testing.T) {
			python, err := exec.LookPath("python3")
			if err != nil {
				t.Fatal(err)
			}
			_, source, _, _ := runtime.Caller(0)
			sdk := filepath.Join(filepath.Dir(source), "../../../../deploy/workforce/runtime-sdk/src")
			fixture := `import sys
from seven_agent.force._protocol import decode,encode
init=decode(sys.stdin.buffer.readline(),direction="down")
def emit(method,payload,reply):
    sys.stdout.buffer.write(encode(dict(protocol="seven.agent-adapter/v1",method=method,key="a:1",replyTo=reply,payload=payload)));sys.stdout.buffer.flush()
emit("ready",dict(name="bad",version="1",capabilities=[]),init["key"])
request=decode(sys.stdin.buffer.readline(),direction="down")
reply=request["key"] if sys.argv[1]=="false" else "wrong"
emit("control.ack",dict(roundKey="r"),reply)
emit("execution.finished",dict(roundKey="r",outcome="FINISHED",summary="",sessionReference="s"),request["key"])
`
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p, err := OpenProcess(ctx, python, []string{"-c", fixture, strconv.FormatBool(wrongReply)}, []string{"PYTHONPATH=" + sdk})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			if _, err := p.Round(ctx, "r", map[string]any{}, "", nil, nil); err == nil {
				t.Fatal("ack accepted as execution authority")
			}
		})
	}
}

const pythonFixture = `
import os
from seven_agent.force.agent import AgentAdapter, RoundResult
agent = AgentAdapter(name="go-pipe-fixture",version="1")
@agent.run
async def run(request, context):
    await context.started(native_pid=os.getpid())
    await context.emit_message("safe activity")
    return RoundResult.finished(summary="done",session_reference="local-session")
@agent.resume
async def resume(request, context):
    return await run(request, context)
agent.serve()
`

func TestPythonProcessRoundAndResume(t *testing.T) {
	executable, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 required for Adapter conformance")
	}
	_, source, _, _ := runtime.Caller(0)
	sdk := filepath.Join(filepath.Dir(source), "../../../../deploy/workforce/runtime-sdk/src")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := OpenProcess(ctx, executable, []string{"-c", pythonFixture}, []string{"PYTHONPATH=" + sdk, "PYTHONDONTWRITEBYTECODE=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var pids []int
	activities := 0
	started := func(pid int) error { pids = append(pids, pid); return nil }
	observe := func(frame Frame) {
		if frame.Method == "message" {
			activities++
		}
	}
	first, err := p.Round(ctx, "round:1", map[string]any{}, "", started, observe)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Round(ctx, "round:2", map[string]any{}, first.SessionReference, started, observe)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != "FINISHED" || second.Outcome != "FINISHED" || len(pids) != 2 || pids[0] != pids[1] || activities != 2 {
		t.Fatal("session or activity contract mismatch")
	}
	p.Close() // Also prove repeat cleanup is safe.
	select {
	case <-p.processDone:
	default:
		t.Fatal("adapter process not reaped")
	}
	select {
	case <-p.readDone:
	default:
		t.Fatal("adapter reader not stopped")
	}
}
