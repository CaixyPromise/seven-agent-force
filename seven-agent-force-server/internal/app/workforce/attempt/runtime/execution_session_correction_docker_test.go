package runtime

import (
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
	"strconv"
	"strings"
	"testing"
	"time"
)

// Real image-owned SDK and protected bootstrap evaluate the actual repository.
// The test Worker supplies directives ONLY after reading/verifying the candidate.
// No ServerDB, Server authorization/ACK transaction, Finalizer, Review or Provider
// participates. A completion proposal is the terminal evidence, not trusted success.
const batch2CorrectionProcessor = `from seven_agent.force.processor import ask_agent, pass_result

def process(context):
    result = context.process.run(["sh", "-c", "test $(cat value.txt) = good"], timeout_seconds=5)
    message = "cycle=%s;exit=%s;sdk=seven_agent.force.processor" % (context.cycle_number, result.returncode)
    if result.returncode != 0:
        return ask_agent(reason_code="BATCH2_VALUE_BAD", message=message)
    return pass_result(reason_code="BATCH2_VALUE_GOOD", message=message)
`

func TestExecutionSessionDockerProcessorCorrection(t *testing.T) {
	image := os.Getenv("BATCH2_ADAPTER_DOCKER_IMAGE")
	if image == "" {
		t.Skip("BATCH2_ADAPTER_DOCKER_IMAGE must name an exact local image digest")
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(image, "sha256:"))
	if !strings.HasPrefix(image, "sha256:") || err != nil || len(digest) != 32 || strings.ToLower(image) != image {
		t.Fatal("exact image digest required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	docker := func(args ...string) []byte {
		t.Helper()
		body, err := exec.CommandContext(ctx, "docker", args...).Output()
		if err != nil {
			t.Fatalf("Docker %s failed: %v", args[0], err)
		}
		return body
	}
	if strings.TrimSpace(string(docker("image", "inspect", image, "--format", "{{.Id}}"))) != image {
		t.Fatal("image identity mismatch")
	}
	root := t.TempDir()
	inputRoot := filepath.Join(root, "input")
	attemptRoot := filepath.Join(root, "attempt")
	processorRoot := filepath.Join(root, "processor")
	for _, dir := range []string{filepath.Join(inputRoot, "attempts", "2"), filepath.Join(attemptRoot, "repo"), processorRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{attemptRoot, filepath.Join(attemptRoot, "repo")} {
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	processorHash := sha256.Sum256([]byte(batch2CorrectionProcessor))
	processorDigest := hex.EncodeToString(processorHash[:])
	if err := os.WriteFile(filepath.Join(processorRoot, "processor.py"), []byte(batch2CorrectionProcessor), 0o444); err != nil {
		t.Fatal(err)
	}
	const containerRoot = "/workspace/1/attempts/2"
	id := strings.TrimSpace(string(docker("create", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--cpus", "1", "--restart", "no",
		"--label", "seven.batch2.processor-correction-conformance=true", "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m,mode=1777",
		"--mount", "type=bind,src="+inputRoot+",dst=/workspace/1/input,readonly",
		"--mount", "type=bind,src="+attemptRoot+",dst="+containerRoot,
		"--mount", "type=bind,src="+processorRoot+",dst=/opt/seven-worker/result-processors/"+processorDigest+",readonly",
		"--env", "SEVEN_CONTROL_SOCKET="+containerRoot+"/runtime/s", "--entrypoint", "/usr/local/bin/seven-worker-cli", image, "supervise", "--spec", "/workspace/1/input/spec.json")))
	if raw, err := hex.DecodeString(id); err != nil || len(raw) != 32 {
		t.Fatal("invalid created container ID")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := exec.CommandContext(cleanup, "docker", "rm", "--force", id).Run(); err != nil {
			t.Errorf("exact cleanup %s failed: %v", id, err)
			return
		}
		body, err := exec.CommandContext(cleanup, "docker", "container", "inspect", id).CombinedOutput()
		if err == nil || !strings.Contains(strings.ToLower(string(body)), "no such") {
			t.Errorf("exact container %s absence not proven", id)
			return
		}
		body, err = exec.CommandContext(cleanup, "docker", "container", "ls", "--all", "--no-trunc", "--filter", "id="+id, "--format", "{{.ID}}").Output()
		if err != nil || len(bytes.TrimSpace(body)) != 0 {
			t.Errorf("exact container %s still listed or inventory unavailable", id)
			return
		}
		t.Logf("cleanup verified: exact container=%s inspect=absent inventory=absent; no named volumes/networks created", id)
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
	sum := sha256.Sum256(completion)
	completionDigest := hex.EncodeToString(sum[:])
	write := func(path string, value any) {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(inputRoot, "attempts", "2", "completion-check.json"), map[string]any{"schema": "seven.completion-check-input/v1", "completionCheck": json.RawMessage(completion), "completionCheckDigest": completionDigest, "references": []any{}})
	write(filepath.Join(inputRoot, "spec.json"), map[string]any{
		"schemaVersion": 1, "taskId": "1", "attemptId": "2", "generation": "1", "ownerEpoch": "1",
		"instruction": "SDK correction conformance. Read /workspace/1/input/attempts/2/completion-check.json (digest " + completionDigest + ")",
		"workspace":   containerRoot + "/repo", "output": containerRoot + "/output", "state": containerRoot + "/state", "runtime": containerRoot + "/runtime", "temporary": containerRoot + "/tmp",
		"resultProcessor": map[string]any{"releaseId": "3", "digest": processorDigest, "root": "/opt/seven-worker/result-processors/" + processorDigest, "bootstrap": "/opt/seven-worker/result-runtime/bootstrap.py", "runtime": "/opt/seven-worker/result-runtime", "pythonRuntimeIdentity": manifest.ResultProcessor.Python, "bootstrapDigest": manifest.ResultProcessor.Bootstrap, "sdkDigest": manifest.ResultProcessor.SDK, "moduleIndexDigest": manifest.ResultProcessor.Index, "parameters": map[string]any{}, "maxEvaluationCycles": 3},
	})
	docker("start", id)
	session := newExecutionSession(ctx, 1, 2, 1, 1, dialDockerExecution)
	defer session.Close()
	call := func(request map[string]any) (supervisorResponse, error) {
		callCtx, stop := context.WithTimeout(ctx, 35*time.Second)
		defer stop()
		return session.Call(callCtx, id, request)
	}
	authorized := func(command string) map[string]any {
		return map[string]any{"schemaVersion": 2, "command": command, "taskId": "1", "attemptId": "2", "generation": "1", "ownerEpoch": "1"}
	}
	wait := func(state string, cycle int) supervisorStateSnapshot {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
			r, err := call(map[string]any{"command": "state"})
			if err == nil && r.OK && r.State.State == state && (cycle == 0 || r.State.EvaluationCycle == cycle) {
				return r.State
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
		t.Fatalf("did not reach %s cycle=%d", state, cycle)
		return supervisorStateSnapshot{}
	}
	wait("READY", 0)
	request := authorized("start-harness")
	request["runPermitId"] = "4"
	request["runPermitToken"] = strings.Repeat("p", 64)
	request["runNonce"] = strings.Repeat("n", 64)
	request["expiresAt"] = time.Now().Add(45 * time.Second).UTC().Format(time.RFC3339Nano)
	r, err := call(request)
	if err != nil || !r.OK {
		t.Fatalf("start: %v %s", err, r.ReasonCode)
	}
	request["command"] = "confirm-running"
	r, err = call(request)
	if err != nil || !r.OK {
		t.Fatalf("confirm: %v %s", err, r.ReasonCode)
	}
	first := wait("EVALUATION_READY", 1)
	if first.SessionDigest == "" || first.HarnessPID < 1 {
		t.Fatal("missing first-round identity")
	}
	containerIdentity := batch2ContainerIdentity(t, ctx, id, image)
	nativeIdentity := func(pid int) string {
		return strings.TrimSpace(string(docker("exec", id, "python3", "-c", "import pathlib,sys; print(pathlib.Path('/proc/'+sys.argv[1]+'/stat').read_text().rsplit(')',1)[1].split()[19])", strconv.Itoa(pid))))
	}
	nativeStart := nativeIdentity(first.HarnessPID)
	readPrivate := func() struct {
		Round         int
		SessionID     string
		SessionDigest string
	} {
		t.Helper()
		var value struct {
			Round         int
			SessionID     string
			SessionDigest string
		}
		if json.Unmarshal(batch2ContainerFile(t, ctx, id, containerRoot+"/state/fake-session.json"), &value) != nil {
			t.Fatal("invalid private Fake session")
		}
		return value
	}
	privateFirst := readPrivate()
	if privateFirst.Round != 1 || privateFirst.SessionID == "" {
		t.Fatal("invalid first private session")
	}
	if string(batch2ContainerFile(t, ctx, id, containerRoot+"/repo/value.txt")) != "bad\n" {
		t.Fatal("first round not bad")
	}
	// Fail closed if a proposal exists before the real Processor evaluates.
	noProposal := func() {
		t.Helper()
		docker("exec", id, "python3", "-c", "import pathlib; assert not pathlib.Path('/workspace/1/attempts/2/output/completion.json').exists()")
	}
	noProposal()
	type candidate struct {
		Decision   string
		ReasonCode string
		Message    string
	}
	evaluate := func(cycle int, wantDecision, wantReason string) candidate {
		t.Helper()
		cycleID := strconv.Itoa(100 + cycle)
		request := authorized("run-evaluation")
		request["cycleId"] = cycleID
		request["cycleNumber"] = cycle
		request["materialization"] = map[string]any{"cycleId": cycleID, "variables": map[string]string{}, "expiresAt": time.Now().Add(40 * time.Second).UTC().Format(time.RFC3339Nano)}
		response, err := call(request)
		if err != nil || !response.OK {
			t.Fatalf("evaluation %d: %v %s", cycle, err, response.ReasonCode)
		}
		if response.Candidate.CycleID != int64(100+cycle) || response.Candidate.CycleNumber != cycle || response.Candidate.CandidateFile != cycleID+".result.json" || response.Candidate.SessionDigest != first.SessionDigest {
			t.Fatal("candidate association mismatch")
		}
		body := batch2ContainerFile(t, ctx, id, containerRoot+"/runtime/evaluations/"+response.Candidate.CandidateFile)
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != response.Candidate.ContractSHA {
			t.Fatal("candidate digest mismatch")
		}
		var result candidate
		if json.Unmarshal(body, &result) != nil || result.Decision != wantDecision || result.ReasonCode != wantReason {
			t.Fatalf("real Processor %d returned decision=%s reason=%s", cycle, result.Decision, result.ReasonCode)
		}
		exit := 1
		if cycle == 2 {
			exit = 0
		}
		if result.Message != fmt.Sprintf("cycle=%d;exit=%d;sdk=seven_agent.force.processor", cycle, exit) {
			t.Fatal("missing real SDK command execution evidence")
		}
		pending := wait("EVALUATION_PENDING_ACK", cycle)
		if pending.EvaluationCycleID != int64(100+cycle) {
			t.Fatal("pending cycle mismatch")
		}
		noProposal()
		return result
	}
	firstCandidate := evaluate(1, "REVISE", "BATCH2_VALUE_BAD")
	// Controlled Worker-side stand-in, not a Server ACK or authority transaction.
	request = authorized("continue-evaluation")
	request["cycleId"] = "101"
	request["directive"] = "RESUME_SAME_SESSION"
	request["decision"] = firstCandidate.Decision
	request["reasonCode"] = firstCandidate.ReasonCode
	request["message"] = firstCandidate.Message
	request["managedFeedback"] = "Restore value.txt to good and produce the fixed Batch1 result."
	request["resumeNonce"] = strings.Repeat("r", 64)
	r, err = call(request)
	if err != nil || !r.OK {
		t.Fatalf("controlled REVISE directive: %v %s", err, r.ReasonCode)
	}
	second := wait("EVALUATION_READY", 2)
	privateSecond := readPrivate()
	if second.HarnessPID != first.HarnessPID || second.SessionDigest != first.SessionDigest || nativeIdentity(second.HarnessPID) != nativeStart || batch2ContainerIdentity(t, ctx, id, image) != containerIdentity {
		t.Fatal("same-session/native/container identity changed")
	}
	if privateSecond.Round != 2 || privateSecond.SessionID != privateFirst.SessionID || privateSecond.SessionDigest != privateFirst.SessionDigest {
		t.Fatal("private session did not resume same identity at round 2")
	}
	if string(batch2ContainerFile(t, ctx, id, containerRoot+"/repo/value.txt")) != "good\n" {
		t.Fatal("second round did not fix value")
	}
	if string(batch2ContainerFile(t, ctx, id, containerRoot+"/repo/batch1-result.txt")) != "Batch 1 result: restored value.txt to good and produced this result record.\n" {
		t.Fatal("fixed result record missing")
	}
	secondCandidate := evaluate(2, "PASS", "BATCH2_VALUE_GOOD")
	request = authorized("continue-evaluation")
	request["cycleId"] = "102"
	request["directive"] = "PROPOSE_COMPLETION"
	request["decision"] = secondCandidate.Decision
	request["reasonCode"] = secondCandidate.ReasonCode
	request["message"] = secondCandidate.Message
	r, err = call(request)
	if err != nil || !r.OK {
		t.Fatalf("controlled PASS directive: %v %s", err, r.ReasonCode)
	}
	if strings.TrimSpace(string(docker("wait", id))) != "0" {
		t.Fatal("Supervisor did not finish cleanly")
	}
	_ = session.Close()
	var proposal struct {
		SchemaVersion     int
		TaskID            string
		AttemptID         string
		EvaluationCycleID string
		SessionDigest     string
	}
	if json.Unmarshal(batch2ContainerFile(t, ctx, id, containerRoot+"/output/completion.json"), &proposal) != nil || proposal.SchemaVersion != 1 || proposal.TaskID != "1" || proposal.AttemptID != "2" || proposal.EvaluationCycleID != "102" || proposal.SessionDigest != first.SessionDigest {
		t.Fatal("completion proposal identity mismatch")
	}
	journal := batch2ContainerFile(t, ctx, id, containerRoot+"/state/runtime-journal.jsonl")
	if batch2StartCount(t, journal) != 1 {
		t.Fatal("Harness start count increased")
	}
	counts := map[string]int{}
	decoder := json.NewDecoder(bytes.NewReader(journal))
	for {
		var event struct{ Type string }
		err := decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("invalid journal")
		}
		counts[event.Type]++
	}
	if counts["evaluation.ready"] != 2 || counts["evaluation.candidate"] != 2 || counts["evaluation.acknowledged"] != 2 {
		t.Fatalf("unexpected lifecycle counts: %v", counts)
	}
	t.Logf("image=%s container=%s sdk=%s processor=%s: bad -> real SDK REVISE -> controlled RESUME_SAME_SESSION -> good -> real SDK PASS -> controlled PROPOSE_COMPLETION -> proposal cycle=102; rounds=2 starts=1 nativePid=%d nativeStart=%s", image, id, manifest.ResultProcessor.SDK, processorDigest, first.HarnessPID, nativeStart)
	t.Log("BOUNDARY: no ServerDB, real Server permit/ACK/authority, Finalizer, Review, Provider, trusted ResultCommit or Task success")
}
