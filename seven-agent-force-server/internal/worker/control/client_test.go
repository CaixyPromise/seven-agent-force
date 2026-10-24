package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func TestExecutionOperationResultReceiptUsesExactHotAndColdRoutes(t *testing.T) {
	t.Parallel()
	var paths []string
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		paths = append(paths, r.URL.Path)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"operationId":"9007199254740995","attemptId":"9007199254740996","version":"4","status":"SUCCEEDED","resultDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","replayed":true},"message":"ok"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 9007199254740993, "wki_1", "boot_1")
	if err != nil {
		t.Fatal(err)
	}
	attempt := taskdomain.Attempt{ID: 9007199254740994, Generation: 2, OwnerEpoch: 3, LeaseToken: "lease"}
	request := executionaccess.ResultReceiptRequest{OperationID: 9007199254740995, AttemptID: 9007199254740996, ResultDigest: strings.Repeat("a", 64)}
	hot, err := client.ReadExecutionOperationResultReceipt(context.Background(), attempt, 7, request)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := client.ReadColdExecutionOperationResultReceipt(context.Background(), attempt, 7, request.OperationID, request)
	if err != nil {
		t.Fatal(err)
	}
	if hot.ResultDigest != request.ResultDigest || cold.ResultDigest != request.ResultDigest || !hot.Replayed || !cold.Replayed {
		t.Fatalf("receipt decode mismatch: hot=%+v cold=%+v", hot, cold)
	}
	wantHot := "/api/internal/workforce/v1/attempts/9007199254740994/execution-operations/9007199254740995/result-receipt"
	wantCold := "/api/internal/workforce/v1/attempts/9007199254740994/execution-operations/9007199254740995/cold/result-receipt"
	if len(paths) != 2 || paths[0] != wantHot || paths[1] != wantCold {
		t.Fatalf("receipt routes=%v, want %q and %q", paths, wantHot, wantCold)
	}
	for _, body := range bodies {
		if !bytes.Contains(body, []byte(`"attemptId":"9007199254740996"`)) || !bytes.Contains(body, []byte(`"resultDigest":"`+request.ResultDigest+`"`)) || !bytes.Contains(body, []byte(`"adapterSessionGeneration":"7"`)) {
			t.Fatalf("receipt request lost exact authority: %s", body)
		}
	}
}

func TestCallUsesStableRequestIdentityAndAcceptsQuotedInt64(t *testing.T) {
	t.Parallel()
	var requests [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests = append(requests, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"generation":"9007199254740993"},"message":"ok"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 9007199254740993, "wki_1", "boot_1")
	if err != nil {
		t.Fatal(err)
	}
	var first, second struct {
		Generation int64 `json:"generation,string"`
	}
	for _, target := range []any{&first, &second} {
		body := client.envelope(1)
		if err := client.call(context.Background(), http.MethodPost, "/fixture", body, "stable-business-key", nil, target); err != nil {
			t.Fatal(err)
		}
	}
	if first.Generation != 9007199254740993 || second.Generation != first.Generation {
		t.Fatalf("quoted int64 decode=%d/%d", first.Generation, second.Generation)
	}
	if len(requests) != 2 || !bytes.Equal(requests[0], requests[1]) {
		t.Fatalf("same idempotency key produced different canonical request bodies: %q / %q", requests[0], requests[1])
	}
	if !bytes.Contains(requests[0], []byte(`"workerId":"9007199254740993"`)) || bytes.Contains(requests[0], []byte(`"workerId":9007199254740993`)) {
		t.Fatalf("Worker Snowflake crossed Machine API as a lossy JSON number: %s", requests[0])
	}
	if !bytes.Contains(requests[0], []byte(`"generation":"1"`)) || !bytes.Contains(requests[0], []byte(`"requestId":"req_`)) {
		t.Fatalf("Machine envelope did not preserve quoted generation and protocol request identity: %s", requests[0])
	}
}

func TestMergePreservesNestedInt64WithoutFloatRounding(t *testing.T) {
	t.Parallel()
	const cycleID int64 = 9007199254740993
	body := merge(map[string]any{"schemaVersion": 1}, map[string]any{
		"facts": []map[string]any{{"payload": map[string]any{"cycleId": cycleID}}},
	})
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"cycleId":9007199254740993`)) {
		t.Fatalf("nested int64 was rounded while composing Machine API body: %s", encoded)
	}
}

func TestScopedIdempotencyKeyIsStableBoundedAndOwnerSensitive(t *testing.T) {
	t.Parallel()
	first := scopedIdempotencyKey("fn-claim", "wki_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "fnr_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "msg_cccccccccccccccccccccccccccccccc")
	if len(first) > 128 || first != scopedIdempotencyKey("fn-claim", "wki_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "fnr_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "msg_cccccccccccccccccccccccccccccccc") {
		t.Fatalf("unstable or oversized key %q", first)
	}
	if first == scopedIdempotencyKey("fn-claim", "wki_other", "fnr_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "msg_cccccccccccccccccccccccccccccccc") {
		t.Fatal("new owner must receive a distinct claim key")
	}
}

func TestClaimIdempotencyKeyIsStablePerInstanceAndGeneration(t *testing.T) {
	first := claimIdempotencyKey("wki_1", 9007199254740993, 3)
	if first != claimIdempotencyKey("wki_1", 9007199254740993, 3) {
		t.Fatal("same instance and Assignment must reuse the Claim key")
	}
	if first == claimIdempotencyKey("wki_2", 9007199254740993, 3) {
		t.Fatal("replacement Worker instance must not reuse the prior owner's Claim key")
	}
	if first == claimIdempotencyKey("wki_1", 9007199254740993, 4) {
		t.Fatal("a new execution generation must not reuse the previous Claim key")
	}
}

func TestPostprocessMutationKeysSeparateRetryMessages(t *testing.T) {
	first := scopedIdempotencyKey("fn-freeze", "wki_1", "fnr_1", "msg_1", "sha256:manifest")
	replay := scopedIdempotencyKey("fn-freeze", "wki_1", "fnr_1", "msg_1", "sha256:manifest")
	retry := scopedIdempotencyKey("fn-freeze", "wki_1", "fnr_1", "msg_2", "sha256:manifest")
	if first != replay {
		t.Fatalf("same post-process message must have a stable key: %q != %q", first, replay)
	}
	if first == retry {
		t.Fatalf("a new post-process retry message must not replay the previous Lease mutation: %q", first)
	}
}

func TestAttemptIdempotencyKeyReplaysWithinOwnerAndSeparatesReattach(t *testing.T) {
	client, err := New("http://example.invalid", "worker-token-with-at-least-thirty-two-bytes", 9007199254740993, "wki_old", "boot_1")
	if err != nil {
		t.Fatal(err)
	}
	old := taskdomain.Attempt{ID: 9007199254740994, Generation: 1, OwnerEpoch: 1}
	oldReplay := client.attemptIdempotencyKey("evaluation", old, "rec_1", "sha256:contract")
	if oldReplay != client.attemptIdempotencyKey("evaluation", old, "rec_1", "sha256:contract") {
		t.Fatal("same owner must replay the same Result Evaluation transport request")
	}

	// A new Worker process that has adopted the same Attempt is a fenced new
	// authority. It must not receive IDEMPOTENCY_CONFLICT merely because the
	// original process had already cached an encrypted response.
	replacement, err := New("http://example.invalid", "worker-token-with-at-least-thirty-two-bytes", 9007199254740993, "wki_new", "boot_1")
	if err != nil {
		t.Fatal(err)
	}
	adopted := old
	adopted.OwnerEpoch = 2
	if oldReplay == replacement.attemptIdempotencyKey("evaluation", adopted, "rec_1", "sha256:contract") {
		t.Fatal("reattached owner must receive a distinct transport replay key")
	}
}

func TestProcessorMaterializationSeparatesUnallocatedEvaluationRounds(t *testing.T) {
	var keys []string
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"cycleId":"9007199254740995","variables":{}},"message":"ok"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 9007199254740993, "wki_1", "boot_1")
	if err != nil {
		t.Fatal(err)
	}
	attempt := taskdomain.Attempt{ID: 9007199254740994, Generation: 1, OwnerEpoch: 1, LeaseToken: "lease"}
	for cycleNumber := 1; cycleNumber <= 2; cycleNumber++ {
		if _, err = client.ProcessorMaterialization(context.Background(), attempt, 0, cycleNumber); err != nil {
			t.Fatalf("materialize cycle %d: %v", cycleNumber, err)
		}
	}
	if len(keys) != 2 || keys[0] == keys[1] {
		t.Fatalf("unallocated evaluation rounds reused transport identity: %v", keys)
	}
	if !bytes.Contains(bodies[0], []byte(`"cycleNumber":1`)) || !bytes.Contains(bodies[1], []byte(`"cycleNumber":2`)) {
		t.Fatalf("cycle numbers missing from Machine requests: %s / %s", bodies[0], bodies[1])
	}
}

func TestContinuationRestoreResultUsesBoundedStableKey(t *testing.T) {
	t.Parallel()
	var got string
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Idempotency-Key")
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{},"message":"ok"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 9007199254740993, "wki_1", "boot_1")
	if err != nil {
		t.Fatal(err)
	}
	restoreRunID := int64(9007199254740995)
	attempt := taskdomain.Attempt{ID: 9007199254740994, Generation: 1, OwnerEpoch: 1, LeaseToken: "lease"}
	if err = client.CompleteContinuationRestore(context.Background(), attempt, restoreRunID, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "base", "result", "tree"); err != nil {
		t.Fatal(err)
	}
	if got != "continuation-restore-result:"+idText(restoreRunID) || len(got) > 128 {
		t.Fatalf("unexpected restore result key %q (%d bytes)", got, len(got))
	}
	for _, field := range []string{`"workerId":"9007199254740993"`, `"attemptId":"9007199254740994"`, `"restoreRunId":"9007199254740995"`, `"generation":"1"`, `"ownerEpoch":"1"`} {
		if !bytes.Contains(body, []byte(field)) {
			t.Fatalf("missing lossless int64 field %s in %s", field, body)
		}
	}
}

func TestWorkspaceGCEligibilityUsesTaskScopedLosslessMachineRequest(t *testing.T) {
	t.Parallel()
	const taskID int64 = 9007199254740993
	var method, path string
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"taskId":"9007199254740993","terminal":true,"terminalAt":"2026-01-01T00:00:00Z","activeReferences":false,"portableStateAvailable":true,"reasonCode":"RETENTION_ELIGIBLE"},"message":"ok"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 9007199254740994, "wki_1", "boot_1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.WorkspaceGCEligibility(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/api/internal/workforce/v1/tasks/9007199254740993/workspace-gc-eligibility" {
		t.Fatalf("workspace GC request = %s %s", method, path)
	}
	if !bytes.Contains(body, []byte(`"workerId":"9007199254740994"`)) || bytes.Contains(body, []byte(`"workerId":9007199254740994`)) {
		t.Fatalf("machine request did not preserve Worker int64: %s", body)
	}
	if got.TaskID != taskID || !got.Terminal || got.ActiveReferences || !got.PortableStateAvailable || got.ReasonCode != "RETENTION_ELIGIBLE" || got.TerminalAt.IsZero() {
		t.Fatalf("WorkspaceGCEligibility() = %#v", got)
	}
}
