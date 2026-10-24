package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	reviewfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func TestRuntimeStartsDetachedThenFollowsLogs(t *testing.T) {
	t.Parallel()
	if got, want := runtimeStartCommand("ctr_1"), []string{"start", "ctr_1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("start command=%v want=%v", got, want)
	}
	if got, want := runtimeAttachCommand("ctr_1"), []string{"logs", "-f", "ctr_1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("logs command=%v want=%v", got, want)
	}
}

func TestAttemptSpecQuotesInt64ProtocolValues(t *testing.T) {
	t.Parallel()
	payload := newAttemptSpecPayload(
		taskdomain.WorkItem{TaskID: 9_100_000_000_000_000_001, Instruction: "test"},
		taskdomain.Attempt{ID: 9_100_000_000_000_000_002, Generation: 9_100_000_000_000_000_003, OwnerEpoch: 9_100_000_000_000_000_004},
		"/workspace/attempt",
	)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"taskId":"9100000000000000001"`,
		`"attemptId":"9100000000000000002"`,
		`"generation":"9100000000000000003"`,
		`"ownerEpoch":"9100000000000000004"`,
	} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("AttemptSpec %s does not contain %s", body, expected)
		}
	}
}

func TestReviewCorrectionMaterializesCanonicalReadOnlyInput(t *testing.T) {
	t.Parallel()
	storage, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const taskID int64 = 9_100_000_000_000_000_001
	const attemptID int64 = 9_100_000_000_000_000_002
	workspace, err := storage.PrepareAttempt(taskID, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	docker := &Docker{workspace: storage}
	report := json.RawMessage(`{"summary":"fix required","verdict":"REVISE","schemaVersion":"review-report/v1"}`)
	canonical, digest, err := reviewfacade.CanonicalizeCorrection(report)
	if err != nil {
		t.Fatal(err)
	}
	item := taskdomain.WorkItem{TaskID: taskID, Instruction: "Read " + continuationfacade.ReviewReportPathPlaceholder + " and apply the current review.", ReviewCorrection: &taskdomain.ReviewCorrection{
		SourceReviewRunID: 9_100_000_000_000_000_003, RevisionKind: "AUTOMATIC", Verdict: "REVISE",
		ReportSchema: "review-report/v1", ReportDigest: digest, Report: report,
	}}
	entry, err := docker.materializeReviewCorrection(item, taskdomain.Attempt{ID: attemptID}, workspace, filepath.Join("/workspace", strconv.FormatInt(taskID, 10)))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace.InputRoot, "attempts", strconv.FormatInt(attemptID, 10), "review", "review-report.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, canonical) || info.Mode().Perm() != 0o444 || entry["sourceReviewRunId"] != "9100000000000000003" || entry["reportDigest"] != digest {
		t.Fatalf("materialized correction mismatch: mode=%o entry=%v body=%s", info.Mode().Perm(), entry, body)
	}
	bound, err := bindReviewCorrectionInstruction(item.Instruction, entry["reportPath"].(string))
	if err != nil || strings.Contains(bound, continuationfacade.ReviewReportPathPlaceholder) || strings.Count(bound, entry["reportPath"].(string)) != 1 {
		t.Fatalf("container report path was not bound into managed feedback: bound=%q err=%v", bound, err)
	}
	if _, err = bindReviewCorrectionInstruction("missing placeholder", entry["reportPath"].(string)); err == nil {
		t.Fatal("instruction without the report path placeholder was accepted")
	}
	item.ReviewCorrection.ReportDigest = strings.Repeat("0", 64)
	if _, err = docker.materializeReviewCorrection(item, taskdomain.Attempt{ID: attemptID}, workspace, filepath.Join("/workspace", strconv.FormatInt(taskID, 10))); err == nil {
		t.Fatal("mismatched canonical report digest was accepted")
	}
}

func TestHarnessTimeoutFollowsTaskSelection(t *testing.T) {
	t.Parallel()
	docker := &Docker{cfg: config.WorkerConfig{}}
	if got := docker.harnessTimeout(taskdomain.WorkItem{HarnessTimeoutSeconds: 2}); got != 2*time.Second {
		t.Fatalf("shortened timeout=%s", got)
	}
	if got := docker.harnessTimeout(taskdomain.WorkItem{HarnessTimeoutSeconds: 900}); got != 15*time.Minute {
		t.Fatalf("task-selected timeout was shortened: %s", got)
	}
	if got := docker.harnessTimeout(taskdomain.WorkItem{}); got != 0 {
		t.Fatalf("unlimited task acquired a timeout: %s", got)
	}
}

func TestReadEvaluationTerminalWaitsForBindMountPublication(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "evaluation-terminal.json")
	go func() {
		time.Sleep(75 * time.Millisecond)
		_ = os.WriteFile(path, []byte(`{"directive":"WAIT_FOR_USER","decision":"WAIT_USER","reasonCode":"MORE_INPUT_REQUIRED","message":"请补充信息","evaluationCycleId":"1001","sessionDigest":"sha256:session"}`), 0o600)
	}()
	terminal, err := readEvaluationTerminal(path)
	if err != nil {
		t.Fatalf("delayed terminal was not observed: %v", err)
	}
	if terminal.Directive != processorfacade.DirectiveWaitForUser || terminal.EvaluationCycleID != 1001 {
		t.Fatalf("unexpected delayed terminal: %#v", terminal)
	}
}

func TestReadRuntimeJournalFactsUsesStableIdentityAndFiltersMachineTransitions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docker := &Docker{cfg: config.WorkerConfig{WorkspaceRoot: root}}
	item := taskdomain.WorkItem{TaskID: 2610240832}
	attempt := taskdomain.Attempt{ID: 3107414342}
	state := filepath.Join(root, strconv.FormatInt(item.TaskID, 10), "attempts", strconv.FormatInt(attempt.ID, 10), "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	line := func(sequence int64, eventType string, payload map[string]any) []byte {
		t.Helper()
		body, _ := json.Marshal(payload)
		sum := sha256.Sum256(body)
		envelope, _ := json.Marshal(map[string]any{
			"schemaVersion": 1, "sequence": strconv.FormatInt(sequence, 10), "factId": "jrn_3107414342_" + strconv.FormatInt(sequence, 10),
			"type": eventType, "time": "2026-08-26T00:00:00Z", "payloadDigest": hex.EncodeToString(sum[:]), "payload": json.RawMessage(body),
		})
		return append(envelope, '\n')
	}
	journal := append(line(1, "harness.started", map[string]any{"pid": 7}), line(2, "hook.deny", map[string]any{"reasonCode": "DENIED"})...)
	if err := os.WriteFile(filepath.Join(state, "runtime-journal.jsonl"), journal, 0o600); err != nil {
		t.Fatal(err)
	}
	facts, err := docker.ReadRuntimeJournalFacts(item, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].FactID != "jrn_3107414342_2" || facts[0].EventType != "hook.deny" || facts[0].ProducerSequence != 2 {
		t.Fatalf("facts=%+v", facts)
	}
}

func TestProcessorSecretExposureChecksContractAndWorkspaceWithoutFollowingGit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	exposed, err := processorSecretExposure([]byte(`{"decision":"PASS"}`), root, []string{"secret-value"})
	if err != nil || exposed {
		t.Fatalf("git metadata must not be treated as a user result: exposed=%t err=%v", exposed, err)
	}
	if err := os.WriteFile(filepath.Join(root, "report.txt"), []byte("contains secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	exposed, err = processorSecretExposure([]byte(`{"decision":"PASS"}`), root, []string{"secret-value"})
	if err != nil || !exposed {
		t.Fatalf("workspace secret exposure=%t err=%v", exposed, err)
	}
	exposed, err = processorSecretExposure([]byte(`{"message":"secret-value"}`), root, []string{"secret-value"})
	if err != nil || !exposed {
		t.Fatalf("contract secret exposure=%t err=%v", exposed, err)
	}
}

func TestReadProcessorCandidateReplacesSecretLeakWithSafeReject(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	workspace := Workspace{Repo: filepath.Join(root, "repo"), Runtime: filepath.Join(root, "runtime")}
	if err := os.MkdirAll(filepath.Join(workspace.Runtime, "evaluations"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace.Repo, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"decision":"PASS","reasonCode":"OK","message":"secret-value"}`)
	path := filepath.Join(workspace.Runtime, "evaluations", "1001.result.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	candidate, err := readProcessorCandidate(workspace, taskdomain.WorkItem{ExecutionSnapshot: taskdomain.ExecutionSnapshot{ResultProcessorReleaseID: 362175473, ResultProcessorDigest: "digest"}}, taskdomain.Attempt{ID: 3107414342}, supervisorStateSnapshot{EvaluationCycleID: 1001, EvaluationCycle: 1}, supervisorEvaluationCandidate{CycleID: 1001, CycleNumber: 1, CandidateFile: "1001.result.json", ContractSHA: hex.EncodeToString(sum[:])}, processorfacade.Materialization{RedactionHint: []string{"secret-value"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(candidate.Contract) == string(body) || candidate.ContractDigest == hex.EncodeToString(sum[:]) {
		t.Fatal("unredacted processor contract was retained")
	}
	if !bytes.Contains(candidate.Contract, []byte(`"PROCESSOR_SECRET_EXPOSURE"`)) || bytes.Contains(candidate.Contract, []byte("secret-value")) {
		t.Fatalf("safe contract=%s", candidate.Contract)
	}
	persisted, err := os.ReadFile(path)
	if err != nil || bytes.Contains(persisted, []byte("secret-value")) {
		t.Fatalf("candidate file was not safely replaced: %s err=%v", persisted, err)
	}
}
