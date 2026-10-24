package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This opt-in test creates only its own no-network container and temporary
// bind mounts. It does not use Worker registration, a database, or a Provider.
// It stops at EVALUATION_READY: Processor/Server ACK/business completion are
// deliberately outside the evidence supplied by this transport conformance.
func TestExecutionSessionDockerReconnectConformance(t *testing.T) {
	image := os.Getenv("BATCH2_ADAPTER_DOCKER_IMAGE")
	if image == "" {
		t.Skip("BATCH2_ADAPTER_DOCKER_IMAGE must name an exact local image digest")
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(image, "sha256:"))
	if !strings.HasPrefix(image, "sha256:") || len(digest) != 32 || err != nil || strings.ToLower(image) != image {
		t.Fatal("exact sha256 image ID required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := func(args ...string) []byte {
		t.Helper()
		out, err := exec.CommandContext(ctx, "docker", args...).Output()
		if err != nil {
			t.Fatalf("Docker %s failed: %v", args[0], err)
		}
		return out
	}
	if strings.TrimSpace(string(command("image", "inspect", image, "--format", "{{.Id}}"))) != image {
		t.Fatal("local image identity mismatch")
	}
	root := t.TempDir()
	inputRoot, attemptRoot, processorRoot := filepath.Join(root, "input"), filepath.Join(root, "attempt"), filepath.Join(root, "processor")
	for _, dir := range []string{inputRoot, filepath.Join(inputRoot, "attempts", "2"), attemptRoot, filepath.Join(attemptRoot, "repo"), processorRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The image's non-root uid creates its own private state/runtime directories.
	for _, dir := range []string{attemptRoot, filepath.Join(attemptRoot, "repo")} {
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	processor := []byte("def process(context):\n    raise RuntimeError('transport conformance must not invoke Processor')\n")
	processorHash := sha256.Sum256(processor)
	processorDigest := hex.EncodeToString(processorHash[:])
	if err := os.WriteFile(filepath.Join(processorRoot, "processor.py"), processor, 0o444); err != nil {
		t.Fatal(err)
	}
	containerRoot := "/workspace/1/attempts/2"
	id := strings.TrimSpace(string(command("create", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--cpus", "1", "--restart", "no",
		"--label", "seven.batch2.execution-session-conformance=true", "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m,mode=1777",
		"--mount", "type=bind,src="+inputRoot+",dst=/workspace/1/input,readonly",
		"--mount", "type=bind,src="+attemptRoot+",dst="+containerRoot,
		"--mount", "type=bind,src="+processorRoot+",dst=/opt/seven-worker/result-processors/"+processorDigest+",readonly",
		"--env", "SEVEN_CONTROL_SOCKET="+containerRoot+"/runtime/s", "--entrypoint", "/usr/local/bin/seven-worker-cli", image, "supervise", "--spec", "/workspace/1/input/spec.json")))
	if decoded, err := hex.DecodeString(id); err != nil || len(decoded) != 32 {
		t.Fatalf("Docker returned invalid container identity %q", id)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := exec.CommandContext(cleanupCtx, "docker", "rm", "--force", id).Run(); err != nil {
			t.Errorf("remove exact test container %s: %v", id, err)
			return
		}
		out, err := exec.CommandContext(cleanupCtx, "docker", "container", "inspect", id).CombinedOutput()
		if err == nil || !strings.Contains(strings.ToLower(string(out)), "no such") {
			t.Errorf("container absence not verified for %s", id)
			return
		}
		out, err = exec.CommandContext(cleanupCtx, "docker", "container", "ls", "--all", "--no-trunc", "--filter", "id="+id, "--format", "{{.ID}}").Output()
		if err != nil || len(bytes.TrimSpace(out)) != 0 {
			t.Errorf("container inventory still contains %s or is unavailable", id)
			return
		}
		t.Logf("cleanup verified: exact container %s inspect=absent, inventory=absent", id)
	})
	var manifest struct {
		ResultProcessor struct {
			Python    string `json:"pythonRuntimeIdentity"`
			Bootstrap string `json:"bootstrapSha256"`
			SDK       string `json:"sdkSha256"`
			Index     string `json:"moduleIndexSha256"`
		} `json:"resultProcessor"`
	}
	if err := json.Unmarshal(batch2ContainerFile(t, ctx, id, "/opt/seven-worker/contract/manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	completion := []byte(`{"mode":"COMMAND","command":"test $(cat value.txt) = good"}`)
	completionHash := sha256.Sum256(completion)
	completionDigest := hex.EncodeToString(completionHash[:])
	writeJSON := func(path string, value any) {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(filepath.Join(inputRoot, "attempts", "2", "completion-check.json"), map[string]any{"schema": "seven.completion-check-input/v1", "completionCheck": json.RawMessage(completion), "completionCheckDigest": completionDigest, "references": []any{}})
	writeJSON(filepath.Join(inputRoot, "spec.json"), map[string]any{
		"schemaVersion": 1, "taskId": "1", "attemptId": "2", "generation": "1", "ownerEpoch": "1",
		"instruction": "Transport conformance. Read /workspace/1/input/attempts/2/completion-check.json (digest " + completionDigest + ")",
		"workspace":   containerRoot + "/repo", "output": containerRoot + "/output", "state": containerRoot + "/state", "runtime": containerRoot + "/runtime", "temporary": containerRoot + "/tmp",
		"resultProcessor": map[string]any{"releaseId": "3", "digest": processorDigest, "root": "/opt/seven-worker/result-processors/" + processorDigest, "bootstrap": "/opt/seven-worker/result-runtime/bootstrap.py", "runtime": "/opt/seven-worker/result-runtime", "pythonRuntimeIdentity": manifest.ResultProcessor.Python, "bootstrapDigest": manifest.ResultProcessor.Bootstrap, "sdkDigest": manifest.ResultProcessor.SDK, "moduleIndexDigest": manifest.ResultProcessor.Index, "parameters": map[string]any{}, "maxEvaluationCycles": 3},
	})
	command("start", id)
	var transport *dockerExecutionTransport
	dials := 0
	session := newExecutionSession(ctx, 1, 2, 1, 1, func(callCtx context.Context, container string) (executionTransport, error) {
		p, err := dialDockerExecution(callCtx, container)
		if err == nil {
			transport = p.(*dockerExecutionTransport)
			dials++
		}
		return p, err
	})
	defer session.Close()
	call := func(request any) (supervisorResponse, error) {
		callCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		return session.Call(callCtx, id, request)
	}
	waitState := func(want string) supervisorStateSnapshot {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			response, err := call(map[string]any{"command": "state"})
			if err == nil && response.OK && response.State.State == want {
				return response.State
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
		// Only safe lifecycle fields are inspected on failure; no raw logs/spec.
		t.Fatalf("Supervisor did not reach %s; container state=%s", want, strings.TrimSpace(string(command("inspect", "--format", "{{.State.Status}} exit={{.State.ExitCode}}", id))))
		return supervisorStateSnapshot{}
	}
	waitState("READY")
	readyDials := dials
	request := map[string]any{"schemaVersion": 2, "command": "start-harness", "taskId": "1", "attemptId": "2", "generation": "1", "ownerEpoch": "1", "runPermitId": "4", "runPermitToken": strings.Repeat("p", 64), "runNonce": strings.Repeat("n", 64), "expiresAt": time.Now().Add(40 * time.Second).UTC().Format(time.RFC3339Nano)}
	response, err := call(request)
	if err != nil || !response.OK {
		t.Fatalf("start request: err=%v reason=%s", err, response.ReasonCode)
	}
	request["command"] = "confirm-running"
	response, err = call(request)
	if err != nil || !response.OK {
		t.Fatalf("confirm-running: err=%v reason=%s", err, response.ReasonCode)
	}
	before := waitState("EVALUATION_READY")
	if before.HarnessPID < 1 || before.SessionDigest == "" || before.EvaluationCycle != 1 {
		t.Fatalf("missing native execution evidence: %+v", before)
	}
	if dials != readyDials {
		t.Fatal("ordinary control calls did not reuse one Docker exec")
	}
	if string(batch2ContainerFile(t, ctx, id, containerRoot+"/repo/value.txt")) != "bad\n" {
		t.Fatal("Fake first round did not write bad fixture")
	}
	identityBefore := batch2ContainerIdentity(t, ctx, id, image)
	nativeBefore := strings.TrimSpace(string(command("exec", id, "python3", "-c", "import pathlib,sys; print(pathlib.Path('/proc/'+sys.argv[1]+'/stat').read_text().rsplit(')',1)[1].split()[19])", strconv.Itoa(before.HarnessPID))))
	journalBefore := batch2StartCount(t, batch2ContainerFile(t, ctx, id, containerRoot+"/state/runtime-journal.jsonl"))
	privateBefore := batch2ContainerFile(t, ctx, id, containerRoot+"/state/fake-session.json")
	var private struct {
		Round int `json:"round"`
	}
	if json.Unmarshal(privateBefore, &private) != nil || private.Round != 1 || journalBefore != 1 {
		t.Fatal("first-round/start-count evidence invalid")
	}
	// Break the actual docker exec process/pipes, leaving the session unaware.
	// The next call must report transport loss, not resend a mutating request.
	oldTransport := transport
	_ = oldTransport.Close()
	if oldTransport.cmd.ProcessState == nil {
		t.Fatal("old Docker exec process was not reaped")
	}
	if _, err := call(map[string]any{"command": "state"}); err == nil {
		t.Fatal("broken transport unexpectedly succeeded")
	}
	after := waitState("EVALUATION_READY")
	if dials != readyDials+1 || transport == oldTransport {
		t.Fatal("reconnect did not establish exactly one new exec")
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("state changed across reconnect: before=%+v after=%+v", before, after)
	}
	if got := batch2ContainerIdentity(t, ctx, id, image); got != identityBefore {
		t.Fatal("container restarted across control reconnect")
	}
	nativeAfter := strings.TrimSpace(string(command("exec", id, "python3", "-c", "import pathlib,sys; print(pathlib.Path('/proc/'+sys.argv[1]+'/stat').read_text().rsplit(')',1)[1].split()[19])", strconv.Itoa(after.HarnessPID))))
	if nativeBefore == "" || nativeAfter != nativeBefore {
		t.Fatal("Adapter native process identity changed")
	}
	if got := batch2StartCount(t, batch2ContainerFile(t, ctx, id, containerRoot+"/state/runtime-journal.jsonl")); got != journalBefore {
		t.Fatal("start count increased across reconnect")
	}
	if !bytes.Equal(privateBefore, batch2ContainerFile(t, ctx, id, containerRoot+"/state/fake-session.json")) {
		t.Fatal("private Fake session changed across reconnect")
	}
	_ = session.Close()
	// Closing the host exec must also terminate the in-container thin bridge.
	bridges := ""
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		bridges = strings.TrimSpace(string(command("exec", id, "python3", "-c", "import pathlib; print(sum(1 for p in pathlib.Path('/proc').glob('[0-9]*/cmdline') if b'\\x00control\\x00--stream\\x00' in p.read_bytes()))")))
		if bridges == "0" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if bridges != "0" {
		t.Fatal("in-container control bridge survived session close")
	}
	t.Logf("image=%s container=%s state=%s adapterPid=%d nativeStartTicks=%s harnessStarts=%d fakeRound=%d reconnects=1 value=bad; bridge cleanup=0 remaining", image, id, after.State, after.HarnessPID, nativeAfter, journalBefore, private.Round)
	t.Log("NOT EXERCISED: Processor execution, Server ACK/authority service, resume/correction, finalization, real Provider")
}

func batch2ContainerFile(t *testing.T, ctx context.Context, id, path string) []byte {
	t.Helper()
	raw, err := exec.CommandContext(ctx, "docker", "cp", id+":"+path, "-").Output()
	if err != nil {
		t.Fatalf("read isolated container file %s: %v", path, err)
	}
	r := tar.NewReader(bytes.NewReader(raw))
	h, err := r.Next()
	if err != nil || h.Size > 1<<20 || h.Typeflag != tar.TypeReg {
		t.Fatal("invalid bounded container file")
	}
	body, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		t.Fatal("container file exceeded bound")
	}
	return body
}

func batch2StartCount(t *testing.T, body []byte) int {
	t.Helper()
	count := 0
	decoder := json.NewDecoder(bytes.NewReader(body))
	for {
		var event struct {
			Type string `json:"type"`
		}
		err := decoder.Decode(&event)
		if err == io.EOF {
			return count
		}
		if err != nil {
			t.Fatal("invalid journal")
		}
		if event.Type == "harness.started" {
			count++
		}
	}
}

func batch2ContainerIdentity(t *testing.T, ctx context.Context, id, image string) string {
	t.Helper()
	body, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .}}", id).Output()
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		ID           string `json:"Id"`
		Image        string
		RestartCount int
		State        struct {
			Running   bool
			Pid       int
			StartedAt string
		}
		HostConfig struct {
			ReadonlyRootfs bool
			NetworkMode    string
		}
		Mounts []struct {
			Destination string
			RW          bool
		}
	}
	if json.Unmarshal(body, &state) != nil || state.ID != id || state.Image != image || !state.State.Running || state.RestartCount != 0 || !state.HostConfig.ReadonlyRootfs || state.HostConfig.NetworkMode != "none" {
		t.Fatal("isolated container identity/config invalid")
	}
	inputRO := false
	for _, mount := range state.Mounts {
		if mount.Destination == "/workspace/1/input" {
			inputRO = !mount.RW
		}
	}
	if !inputRO {
		t.Fatal("input is not read-only")
	}
	return fmt.Sprintf("%s/%d/%s/%d", state.ID, state.State.Pid, state.State.StartedAt, state.RestartCount)
}
