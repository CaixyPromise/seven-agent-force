package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateSpecAcceptsAttemptPrivatePaths(t *testing.T) {
	t.Parallel()
	spec := attemptSpec{
		SchemaVersion: 1, TaskID: 42465665, AttemptID: 4057650848, Generation: 1, OwnerEpoch: 1,
		Workspace: "/workspace/42465665/attempts/4057650848/repo",
		Output:    "/workspace/42465665/attempts/4057650848/output",
		State:     "/workspace/42465665/attempts/4057650848/state",
		Runtime:   "/workspace/42465665/attempts/4057650848/runtime",
		Temporary: "/workspace/42465665/attempts/4057650848/tmp",
		ResultProcessor: &resultProcessorSpec{
			ReleaseID: 1656557231, Digest: strings.Repeat("a", 64),
			Root:      "/opt/seven-worker/result-processors/" + strings.Repeat("a", 64),
			Bootstrap: "/opt/seven-worker/result-runtime/bootstrap.py", Runtime: "/opt/seven-worker/result-runtime",
			PythonRuntimeIdentity: strings.Repeat("b", 64), BootstrapDigest: strings.Repeat("c", 64),
			SDKDigest: strings.Repeat("d", 64), ModuleIndexDigest: strings.Repeat("e", 64),
			Parameters: json.RawMessage(`{}`), MaxEvaluationCycles: 3,
		},
	}
	if err := validateSpec(spec); err != nil {
		t.Fatalf("validateSpec() error = %v", err)
	}
}

func TestValidateReviewCorrectionFencesDigestPathVerdictAndReason(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "review-report.json")
	body := []byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"fix"}`)
	if err := os.WriteFile(path, body, 0o444); err != nil {
		t.Fatal(err)
	}
	digest := sha256Text(string(body))
	base := reviewCorrectionSpec{SourceReviewRunID: 9_100_000_000_000_000_003, RevisionKind: "AUTOMATIC", Verdict: "REVISE", ReportPath: path, ReportDigest: digest}
	if err := validateReviewCorrection(base, root, os.ReadFile); err != nil {
		t.Fatalf("valid automatic correction rejected: %v", err)
	}
	rejected := base
	rejected.Verdict = "REJECT"
	if err := validateReviewCorrection(rejected, root, os.ReadFile); err == nil {
		t.Fatal("automatic correction accepted a REJECT report")
	}
	rejected.RevisionKind = "HUMAN_AUTHORIZED"
	if err := validateReviewCorrection(rejected, root, os.ReadFile); err != nil {
		t.Fatalf("human-authorized correction rejected canonical REJECT: %v", err)
	}
	escaped := base
	escaped.ReportPath = filepath.Join(root, "..", "review-report.json")
	if err := validateReviewCorrection(escaped, root, func(string) ([]byte, error) { return body, nil }); err == nil {
		t.Fatal("correction report outside the attempt input path was accepted")
	}
	reasonOnly := reviewCorrectionSpec{SourceReviewRunID: base.SourceReviewRunID, RevisionKind: "HUMAN_AUTHORIZED", ReasonCode: "INDEPENDENT_REVIEW_RETRY_EXHAUSTED"}
	if err := validateReviewCorrection(reasonOnly, root, os.ReadFile); err != nil {
		t.Fatalf("allowlisted reportless human correction rejected: %v", err)
	}
	reasonOnly.ReasonCode = "RAW_PROVIDER_FAILURE: secret"
	if err := validateReviewCorrection(reasonOnly, root, os.ReadFile); err == nil {
		t.Fatal("untrusted reportless reason was accepted")
	}
}

func TestProtocolStateSnapshotQuotesInt64BoundaryValues(t *testing.T) {
	t.Parallel()
	state := &supervisorState{
		TaskID:              910000000000000001,
		AttemptID:           910000000000000002,
		Generation:          910000000000000003,
		OwnerEpoch:          910000000000000004,
		RunPermitID:         910000000000000005,
		LastJournalSequence: 910000000000000006,
		EvaluationCycleID:   910000000000000007,
	}
	body, err := json.Marshal(protocolStateSnapshot(state))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"taskId":"910000000000000001"`,
		`"attemptId":"910000000000000002"`,
		`"generation":"910000000000000003"`,
		`"ownerEpoch":"910000000000000004"`,
		`"runPermitId":"910000000000000005"`,
		`"lastJournalSequence":"910000000000000006"`,
		`"evaluationCycleId":"910000000000000007"`,
	} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("protocol state %s does not contain %s", body, expected)
		}
	}
}

func TestContinuationSessionMustMatchFrozenLineage(t *testing.T) {
	stateRoot := t.TempDir()
	raw := "session-private-123"
	digest := sha256Text(raw)
	body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "kind": "agent-session", "sessionId": raw, "sessionDigest": digest})
	if err := os.WriteFile(filepath.Join(stateRoot, "session.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	nonce := strings.Repeat("a", 64)
	spec := attemptSpec{Instruction: "continue safely", Continuation: &continuationSpec{
		SourceTurnID: 637649102, TargetTurnID: 3171578228, CheckpointID: 910000000000000001,
		SessionLineageDigest: digest, ResumeNonce: nonce, ResumeNonceDigest: sha256Text(nonce),
	}}
	state := &supervisorState{sessionArchivePath: filepath.Join(stateRoot, "session.json")}
	if err := loadContinuationSession(spec, state); err != nil {
		t.Fatalf("loadContinuationSession() error = %v", err)
	}
	if state.sessionID != raw || state.SessionDigest != digest || state.managedFeedback != spec.Instruction {
		t.Fatalf("restored session state = %#v", state)
	}
	spec.Continuation.SessionLineageDigest = strings.Repeat("f", 64)
	if err := loadContinuationSession(spec, &supervisorState{sessionArchivePath: state.sessionArchivePath}); err == nil {
		t.Fatal("mismatched session lineage accepted")
	}
}

func TestValidateHookControlEventRequiresExactAttemptAndPackageIdentity(t *testing.T) {
	t.Parallel()
	spec := attemptSpec{TaskID: 2610240832, AttemptID: 3107414342, Generation: 2, Hooks: &hookSpec{ConfigurationDigest: "configuration-digest"}}
	configuration := hookConfigurationSpec{Hooks: []hookPackageSpec{{ReleaseID: 910000000000000002, Version: "1.0.0", Digest: "package-digest"}}}
	request := attemptControlRequest{
		SchemaVersion: 1, Command: "hook-event", TaskID: 2610240832, AttemptID: 3107414342, Generation: 2,
		EventType: "hook.deny", HookEvent: "codex.PreToolUse", InvocationID: "hinv_12345678",
		ConfigurationDigest: "configuration-digest", PackageReleaseID: 910000000000000002, PackageVersion: "1.0.0",
		PackageDigest: "package-digest", ReasonCode: "SENTINEL_COMMAND_DENIED", DurationMS: 7,
	}
	if err := validateHookControlEvent(spec, configuration, request); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	request.PackageDigest = "forged"
	if err := validateHookControlEvent(spec, configuration, request); err == nil {
		t.Fatal("forged package identity accepted")
	}
}

func TestEventWriterSerializesConcurrentJSONLines(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writer := &eventWriter{writer: &output}
	var group sync.WaitGroup
	for index := 0; index < 64; index++ {
		group.Add(1)
		go func(value int) {
			defer group.Done()
			if err := writer.Emit("fixture", map[string]any{"index": value}); err != nil {
				t.Errorf("emit: %v", err)
			}
		}(index)
	}
	group.Wait()
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	count := 0
	for decoder.More() {
		var line map[string]any
		if err := decoder.Decode(&line); err != nil {
			t.Fatalf("corrupt concurrent JSONL: %v\n%s", err, output.String())
		}
		count++
	}
	if count != 64 {
		t.Fatalf("event count=%d want=64", count)
	}
}

func TestValidateSpecRejectsTaskRootWrite(t *testing.T) {
	t.Parallel()
	spec := attemptSpec{
		SchemaVersion: 1, TaskID: 42465665, AttemptID: 4057650848, Generation: 1, OwnerEpoch: 1,
		Workspace: "/workspace/tsk_123/repo",
		Output:    "/workspace/tsk_123/attempts/att_456/output",
		State:     "/workspace/tsk_123/attempts/att_456/state",
		Runtime:   "/workspace/tsk_123/attempts/att_456/runtime",
		Temporary: "/workspace/tsk_123/attempts/att_456/tmp",
	}
	if err := validateSpec(spec); err == nil {
		t.Fatal("validateSpec() error = nil, want task-root write rejection")
	}
}

func TestSupervisorControlV2GatesHarnessAndMakesRunNonceIdempotent(t *testing.T) {
	root := shortSocketRoot(t)
	spec := attemptSpec{SchemaVersion: 1, TaskID: 3518409982, AttemptID: 3538623857, Generation: 1, OwnerEpoch: 3, Runtime: root}
	state := &supervisorState{State: "READY", TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: spec.Generation, OwnerEpoch: spec.OwnerEpoch, done: make(chan error, 1), ctx: context.Background()}
	var calls atomic.Int64
	runner := func(_ context.Context, _ attemptSpec, _ *supervisorState, started harnessStarted) error {
		calls.Add(1)
		if err := started(4242); err != nil {
			return err
		}
		return nil
	}
	socket := filepath.Join(root, "s")
	listener, err := listenControlWithRunner(socket, spec, state, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if calls.Load() != 0 {
		t.Fatal("Harness ran before start-harness")
	}

	request := attemptControlRequest{
		SchemaVersion: 2, Command: "start-harness", TaskID: spec.TaskID, AttemptID: spec.AttemptID,
		Generation: spec.Generation, OwnerEpoch: spec.OwnerEpoch, RunPermitID: 910000000000000021,
		RunPermitToken: "permit-token-with-at-least-thirty-two-bytes", RunNonce: "run-nonce-with-at-least-thirty-two-bytes",
		ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano),
	}
	first := sendControlFixture(t, socket, request)
	if ok, _ := first["ok"].(bool); !ok {
		t.Fatalf("first start response=%v", first)
	}
	started, _ := first["harnessStarted"].(map[string]any)
	if started["harnessPid"] != float64(4242) {
		t.Fatalf("harness PID is not the runner-reported identity: %v", first)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d want=1", calls.Load())
	}
	select {
	case <-state.done:
		t.Fatal("Harness business execution completed before Server confirmed RUNNING")
	default:
	}
	second := sendControlFixture(t, socket, request)
	if ok, _ := second["ok"].(bool); !ok || calls.Load() != 1 {
		t.Fatalf("idempotent start response=%v calls=%d", second, calls.Load())
	}
	request.RunNonce = "different-run-nonce-with-at-least-thirty-two-bytes"
	conflict := sendControlFixture(t, socket, request)
	if conflict["reasonCode"] != "RUN_NONCE_CONFLICT" || calls.Load() != 1 {
		t.Fatalf("conflict response=%v calls=%d", conflict, calls.Load())
	}
	wrongConfirm := request
	wrongConfirm.Command = "confirm-running"
	wrongConfirm.RunNonce = "wrong-confirm-nonce-with-at-least-thirty-two-bytes"
	if response := sendControlFixture(t, socket, wrongConfirm); response["reasonCode"] != "RUN_CONFIRMATION_IDENTITY_MISMATCH" {
		t.Fatalf("wrong confirm response=%v", response)
	}
	confirm := request
	confirm.Command = "confirm-running"
	confirm.RunNonce = "run-nonce-with-at-least-thirty-two-bytes"
	if response := sendControlFixture(t, socket, confirm); response["ok"] != true {
		t.Fatalf("confirm response=%v", response)
	}
	if response := sendControlFixture(t, socket, confirm); response["ok"] != true {
		t.Fatalf("replayed confirm response=%v", response)
	}
	select {
	case <-state.done:
	case <-time.After(time.Second):
		t.Fatal("Harness did not continue after Server confirmed RUNNING")
	}
}

func TestSupervisorControlV2ConcurrentSameNonceStartsOnce(t *testing.T) {
	root := shortSocketRoot(t)
	spec := attemptSpec{SchemaVersion: 1, TaskID: 2856541965, AttemptID: 1273592921, Generation: 1, OwnerEpoch: 1, Runtime: root}
	state := &supervisorState{State: "READY", TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: 1, OwnerEpoch: 1, done: make(chan error, 1), ctx: context.Background()}
	var calls atomic.Int64
	listener, err := listenControlWithRunner(filepath.Join(root, "s"), spec, state, func(_ context.Context, _ attemptSpec, _ *supervisorState, started harnessStarted) error {
		calls.Add(1)
		return started(5151)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	request := attemptControlRequest{SchemaVersion: 2, Command: "start-harness", TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: 1, OwnerEpoch: 1, RunPermitID: 910000000000000022, RunPermitToken: "permit-token-with-at-least-thirty-two-bytes", RunNonce: "same-run-nonce-with-at-least-thirty-two-bytes", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}
	responses := make(chan map[string]any, 2)
	var group sync.WaitGroup
	for index := 0; index < 2; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			responses <- sendControlFixture(t, filepath.Join(root, "s"), request)
		}()
	}
	group.Wait()
	close(responses)
	for response := range responses {
		if ok, _ := response["ok"].(bool); !ok {
			t.Fatalf("same nonce was not idempotent: %v", response)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("Harness runner calls=%d want=1", calls.Load())
	}
	confirm := request
	confirm.Command = "confirm-running"
	if response := sendControlFixture(t, filepath.Join(root, "s"), confirm); response["ok"] != true {
		t.Fatalf("confirm response=%v", response)
	}
}

func TestSupervisorControlV2StrictRequestBoundaries(t *testing.T) {
	root := shortSocketRoot(t)
	spec := attemptSpec{SchemaVersion: 1, TaskID: 2475032046, AttemptID: 2468020620, Generation: 1, OwnerEpoch: 1, Runtime: root}
	state := &supervisorState{State: "READY", TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: 1, OwnerEpoch: 1, done: make(chan error, 1)}
	socket := filepath.Join(root, "s")
	listener, err := listenControlWithRunner(socket, spec, state, func(context.Context, attemptSpec, *supervisorState, harnessStarted) error {
		return errors.New("must not run")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for name, body := range map[string][]byte{
		"unknown field":   []byte(`{"schemaVersion":2,"command":"state","unknown":true}` + "\n"),
		"trailing object": []byte(`{"schemaVersion":2,"command":"state"} {}` + "\n"),
		"oversized":       append([]byte(`{"schemaVersion":2,"command":"state","taskId":"`+strings.Repeat("x", 17<<10)+`"}`), '\n'),
	} {
		t.Run(name, func(t *testing.T) {
			response := sendRawControlFixture(t, socket, body)
			if ok, _ := response["ok"].(bool); ok {
				t.Fatalf("strict boundary accepted request: %v", response)
			}
		})
	}
}

func TestSupervisorControlV2RejectsExpiredPermitAndAdoptsOwnerIdempotently(t *testing.T) {
	root := shortSocketRoot(t)
	spec := attemptSpec{SchemaVersion: 1, TaskID: 3518409982, AttemptID: 3538623857, Generation: 1, OwnerEpoch: 1, Runtime: root}
	state := &supervisorState{State: "READY", TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: 1, OwnerEpoch: 1, done: make(chan error, 1)}
	socket := filepath.Join(root, "s")
	listener, err := listenControlWithRunner(socket, spec, state, func(context.Context, attemptSpec, *supervisorState, harnessStarted) error {
		return errors.New("must not run")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	expired := sendControlFixture(t, socket, attemptControlRequest{SchemaVersion: 2, Command: "start-harness", TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: 1, OwnerEpoch: 1, RunPermitID: 910000000000000023, RunPermitToken: "permit-token-with-at-least-thirty-two-bytes", RunNonce: "run-nonce-with-at-least-thirty-two-bytes", ExpiresAt: time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)})
	if expired["reasonCode"] != "RUN_PERMIT_EXPIRED" {
		t.Fatalf("expired response=%v", expired)
	}
	permitToken := "reattach-permit-token-with-at-least-thirty-two-bytes"
	adoptRequest := attemptControlRequest{
		SchemaVersion: 2, Command: "adopt-owner", TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: 1,
		PreviousOwnerEpoch: 1, OwnerEpoch: 2, ReattachPermitID: 910000000000000024, ReattachPermitToken: permitToken,
		ExpiresAt:      time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano),
		LeaseExpiresAt: time.Now().Add(30 * time.Second).UTC().Format(time.RFC3339Nano), LeaseReceiptDigest: strings.Repeat("a", 64),
	}
	adopt := sendControlFixture(t, socket, adoptRequest)
	if ok, _ := adopt["ok"].(bool); !ok {
		t.Fatalf("adopt response=%v", adopt)
	}
	replayed := sendControlFixture(t, socket, adoptRequest)
	if ok, _ := replayed["ok"].(bool); !ok {
		t.Fatalf("replayed adopt response=%v", replayed)
	}
	conflict := adoptRequest
	conflict.ReattachPermitToken = "different-reattach-permit-token-with-thirty-two-bytes"
	response := sendControlFixture(t, socket, conflict)
	if response["reasonCode"] != "REATTACH_PAYLOAD_CONFLICT" {
		t.Fatalf("conflicting adopt response=%v", response)
	}
}

func TestPendingEvaluationReplaysCandidateAfterOwnerReattach(t *testing.T) {
	t.Parallel()
	contractDigest := strings.Repeat("f", 64)
	server := &controlServer{
		spec: attemptSpec{TaskID: 48611430, AttemptID: 1289863862, Generation: 2, OwnerEpoch: 4, ResultProcessor: &resultProcessorSpec{MaxEvaluationCycles: 3}},
		state: &supervisorState{
			State:                     "EVALUATION_PENDING_ACK",
			TaskID:                    48611430,
			AttemptID:                 1289863862,
			Generation:                2,
			OwnerEpoch:                4,
			EvaluationCycleID:         703651839,
			EvaluationCycle:           2,
			EvaluationCandidate:       "/runtime/evaluations/cyc_recover.result.json",
			EvaluationCandidateDigest: contractDigest,
			SessionDigest:             strings.Repeat("a", 64),
		},
	}
	materialization, err := json.Marshal(evaluationMaterialization{CycleID: 703651839, Variables: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	result, reason, err := server.runEvaluation(attemptControlRequest{
		SchemaVersion: 2, Command: "run-evaluation", TaskID: 48611430, AttemptID: 1289863862,
		Generation: 2, OwnerEpoch: 4, CycleID: 703651839, CycleNumber: 2, Materialization: materialization,
	})
	if err != nil || reason != "" {
		t.Fatalf("replay pending evaluation = (%+v, %q, %v)", result, reason, err)
	}
	if result.CandidateFile != "cyc_recover.result.json" || result.ContractSHA256 != contractDigest || result.SessionDigest != strings.Repeat("a", 64) {
		t.Fatalf("unexpected replayed candidate: %+v", result)
	}
	server.state.mu.RLock()
	defer server.state.mu.RUnlock()
	if server.state.State != "EVALUATION_PENDING_ACK" || server.state.EvaluationCandidateDigest != contractDigest {
		t.Fatalf("candidate state mutated during replay: %+v", server.state)
	}
}

func TestSupervisorWatchdogKeepsHarnessDuringGraceThenTerminates(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	state := &supervisorState{
		State:           "RUNNING",
		authorityExpiry: time.Now().Add(-20 * time.Millisecond),
		watchdogGrace:   80 * time.Millisecond,
		cancel:          cancel,
	}
	done := make(chan struct{})
	go func() {
		monitorAuthority(ctx, state)
		close(done)
	}()

	select {
	case <-ctx.Done():
		t.Fatal("watchdog terminated Harness before orphan grace elapsed")
	case <-time.After(30 * time.Millisecond):
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("watchdog did not terminate Harness after orphan grace")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchdog loop did not stop after termination")
	}
	state.mu.RLock()
	orphanDeadline := state.OrphanDeadline
	state.mu.RUnlock()
	if orphanDeadline == "" {
		t.Fatal("watchdog did not expose the authoritative orphan deadline")
	}
}

func shortSocketRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "swc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func TestRecordEvaluationContinuationClearsAcceptedCycleIdentity(t *testing.T) {
	state := &supervisorState{
		State:             "EVALUATION_PENDING_ACK",
		EvaluationCycleID: 9007199254740993,
		journalWrite:      func(string, any) error { return nil },
	}
	continuation := evaluationContinuation{
		Directive:  "RESUME_SAME_SESSION",
		CycleID:    9007199254740993,
		Decision:   "REVISE",
		ReasonCode: "REVISION_REQUIRED",
	}
	if err := recordEvaluationContinuation(state, continuation); err != nil {
		t.Fatalf("record continuation: %v", err)
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.State != "RUNNING" || state.EvaluationCycleID != 0 {
		t.Fatalf("state=%s cycle=%d, want RUNNING with no active cycle", state.State, state.EvaluationCycleID)
	}
}

func sendControlFixture(t *testing.T, socket string, request attemptControlRequest) map[string]any {
	t.Helper()
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("decode response %q: %v", line, err)
	}
	return response
}

func sendRawControlFixture(t *testing.T, socket string, body []byte) map[string]any {
	t.Helper()
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(body); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("decode response %q: %v", line, err)
	}
	return response
}
