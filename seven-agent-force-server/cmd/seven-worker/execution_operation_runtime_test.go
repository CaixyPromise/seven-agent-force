package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionaccessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
)

func TestResolveOperationDoHReturnsOnlyAddressRecords(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/dns-json" || r.URL.Query().Get("name") != "registry.example" {
			t.Fatal("trusted DoH request lost its fixed contract")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Status": 0, "Answer": []map[string]any{
			{"type": 5, "data": "ignored.example."},
			{"type": 1, "data": "203.0.113.9"},
			{"type": 1, "data": "8.8.8.8"},
		}})
	}))
	defer server.Close()

	addresses, err := resolveOperationDoH(context.Background(), server.Client(), server.URL, "registry.example")
	if err != nil || len(addresses) != 4 { // The fixture is returned for A and AAAA queries; filtering happens after parsing.
		t.Fatalf("addresses=%v err=%v", addresses, err)
	}
	if !hasPublicOperationIP(addresses) {
		t.Fatal("trusted DoH answer did not expose a public address to the existing pinning policy")
	}
}

func TestNetworkOperationRuntimeExecutesOnceAndReadsBack(t *testing.T) {
	workspace := t.TempDir()
	repository := filepath.Join(workspace, "repository")
	if err := os.MkdirAll(filepath.Join(repository, "packages", "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	journal, err := resourcejournal.Open(filepath.Join(workspace, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	var commands [][]string
	runtime := newNetworkOperationRuntime(workspace, journal)
	runtime.now = func() time.Time { return time.Date(2026, 9, 11, 2, 0, 0, 0, time.UTC) }
	runtime.resolve = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	runtime.run = func(_ context.Context, name string, args ...string) (string, error) {
		if name != "docker" {
			t.Fatalf("unexpected command %q", name)
		}
		commands = append(commands, append([]string(nil), args...))
		if len(args) >= 2 && args[0] == "network" && args[1] == "create" {
			return strings.Repeat("1", 64), nil
		}
		if len(args) >= 2 && args[0] == "container" && args[1] == "create" {
			return strings.Repeat("2", 64), nil
		}
		if len(args) >= 2 && args[0] == "network" && args[1] == "connect" {
			return "", nil
		}
		if len(args) >= 1 && args[0] == "inspect" {
			if strings.Contains(strings.Join(args, " "), "IPAddress") {
				return "172.30.0.2\n", nil
			}
			if strings.Contains(strings.Join(args, " "), "NetworkMode") {
				return "seven-op-9001-int\n", nil
			}
			return "true\n", nil
		}
		if len(args) >= 2 && args[0] == "network" && args[1] == "inspect" {
			return "seven-op-9001-runner\nseven-op-9001-gw-1\n", nil
		}
		if len(args) >= 2 && args[0] == "start" && args[1] == "-a" {
			runs++
			return "installed\n", nil
		}
		if len(args) >= 1 && args[0] == "start" {
			return "", nil
		}
		t.Fatalf("unexpected docker args: %v", args)
		return "", nil
	}
	claim := testNetworkOperationClaim(repository)
	claim, err = runtime.Prepare(context.Background(), claim)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Execute(context.Background(), claim)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "SUCCEEDED" || result.Output != "installed\n" || runs != 1 {
		t.Fatalf("unexpected first result=%+v runs=%d", result, runs)
	}
	replayedClaim, err := runtime.Prepare(context.Background(), testNetworkOperationClaim(repository))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := runtime.Execute(context.Background(), replayedClaim)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ResultDigest != result.ResultDigest || runs != 1 {
		t.Fatalf("readback re-executed command: first=%+v replay=%+v runs=%d", result, replayed, runs)
	}
	joined := ""
	for _, command := range commands {
		joined += strings.Join(command, " ") + "\n"
	}
	for _, forbidden := range []string{"docker.sock", "DOCKER_HOST", "WORKER_TOKEN", "PROVIDER_KEY", "--env", "/run/seven-operation", filepath.Join(workspace, ".seven", "execution-operations")} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("runner received forbidden capability %q: %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "--internal") || !strings.Contains(joined, "target-egress-gateway") || !strings.Contains(joined, "--network seven-op-9001-int") || !strings.Contains(joined, "--dns 127.0.0.1") || !strings.Contains(joined, "--add-host registry.npmjs.org:172.30.0.2") {
		t.Fatalf("fixed egress topology missing: %s", joined)
	}
}

func TestNetworkOperationRuntimeStartedWithoutResultIsUnknown(t *testing.T) {
	workspace := t.TempDir()
	repository := filepath.Join(workspace, "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	journal, err := resourcejournal.Open(filepath.Join(workspace, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := newNetworkOperationRuntime(workspace, journal)
	runtime.now = func() time.Time { return time.Date(2026, 9, 11, 2, 0, 0, 0, time.UTC) }
	claim := testNetworkOperationClaim(repository)
	claim.CWD = "."
	claim.RuntimeContainerID = "runner-id"
	claim.RelayIdentity = strings.Repeat("b", 64)
	root := filepath.Join(workspace, ".seven", "execution-operations", "9001")
	if err = os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = writeOperationJSON(filepath.Join(root, "started.json"), map[string]string{"descriptorDigest": claim.DescriptorDigest}); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Execute(context.Background(), claim)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "OUTCOME_UNKNOWN" || result.ReasonCode != "OPERATION_RESULT_NOT_RECOVERABLE" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestValidateNetworkOperationClaimRejectsEscapesAndInvalidTargets(t *testing.T) {
	workspace := t.TempDir()
	repository := filepath.Join(workspace, "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	claim := testNetworkOperationClaim(repository)
	claim.CWD = "../outside"
	if _, err := validateNetworkOperationClaim(claim, workspace); err == nil {
		t.Fatal("accepted escaping cwd")
	}
	claim = testNetworkOperationClaim(repository)
	claim.Targets[0].Host = "127.0.0.1"
	if _, err := validateNetworkOperationClaim(claim, workspace); err == nil {
		t.Fatal("accepted IP literal")
	}
	claim = testNetworkOperationClaim(repository)
	claim.Targets[0].Protocol = "ssh"
	if _, err := validateNetworkOperationClaim(claim, workspace); err == nil {
		t.Fatal("accepted unsupported protocol")
	}
	claim = testNetworkOperationClaim(repository)
	claim.Targets = append(claim.Targets, networkOperationTarget{Host: claim.Targets[0].Host, Protocol: "http", Port: 80})
	if _, err := validateNetworkOperationClaim(claim, workspace); err == nil {
		t.Fatal("accepted duplicate hostname with ambiguous relay routing")
	}
}

func TestPublicOperationIPRejectsControlPlaneRanges(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "192.0.2.2", "2001:db8::1"} {
		if publicOperationIP(net.ParseIP(value)) {
			t.Fatalf("accepted %s", value)
		}
	}
	if !publicOperationIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("rejected public address")
	}
}

func TestSafeOperationOutputBoundsAndRedacts(t *testing.T) {
	output, digest := safeOperationOutput("token=top-secret\n" + strings.Repeat("x", operationAgentOutputLimit+1))
	if strings.Contains(output, "top-secret") || len(output) > operationAgentOutputLimit || len(digest) != 64 {
		t.Fatalf("unsafe output len=%d digest=%q", len(output), digest)
	}
}

func testNetworkOperationClaim(repository string) networkOperationClaim {
	return networkOperationClaim{
		OperationID: 9001, TaskID: 8001, TaskTurnID: 8101, AttemptID: 8201, OwnerEpoch: 1,
		DescriptorDigest: strings.Repeat("a", 64), Purpose: "install locked dependencies",
		Command: "pnpm install --frozen-lockfile", CWD: "packages/app",
		Targets:      []networkOperationTarget{{Host: "registry.npmjs.org", Protocol: "https", Port: 443}},
		HarnessImage: "example.invalid/harness@sha256:" + strings.Repeat("b", 64), RepositoryRoot: repository,
		Deadline: time.Date(2026, 9, 11, 2, 5, 0, 0, time.UTC),
	}
}

type fakeOperationControl struct {
	claim       networkOperationClaim
	available   bool
	running     int
	submissions int
	submitErr   error
	confirmed   bool
}

func (f *fakeOperationControl) ConfirmNetworkOperationResult(context.Context, networkOperationClaim, networkOperationResult) (bool, error) {
	return f.confirmed, nil
}

func (f *fakeOperationControl) ClaimNetworkOperation(context.Context) (networkOperationClaim, bool, error) {
	return f.claim, f.available, nil
}
func (f *fakeOperationControl) MarkNetworkOperationRunning(context.Context, networkOperationClaim) error {
	f.running++
	return nil
}
func (f *fakeOperationControl) RenewNetworkOperation(context.Context, networkOperationClaim) error {
	return nil
}
func (f *fakeOperationControl) SubmitNetworkOperationResult(context.Context, networkOperationClaim, networkOperationResult) error {
	f.submissions++
	return f.submitErr
}

type fakeOperationExecutor struct {
	executions int
	cleanups   int
}

func (f *fakeOperationExecutor) Prepare(_ context.Context, claim networkOperationClaim) (networkOperationClaim, error) {
	claim.RuntimeContainerID = "runner-id"
	claim.RelayIdentity = strings.Repeat("d", 64)
	return claim, nil
}

func (f *fakeOperationExecutor) Execute(context.Context, networkOperationClaim) (networkOperationResult, error) {
	f.executions++
	return networkOperationResult{OperationID: 9001, Status: "SUCCEEDED", ResultDigest: strings.Repeat("c", 64)}, nil
}
func (f *fakeOperationExecutor) Cleanup(int64) error { f.cleanups++; return nil }

func TestExecutionOperationPumpSubmitsBeforeCleanup(t *testing.T) {
	control := &fakeOperationControl{available: true}
	executor := &fakeOperationExecutor{}
	worked, err := (executionOperationPump{control: control, executor: executor}).RunOnce(context.Background())
	if err != nil || !worked || control.running != 1 || control.submissions != 1 || executor.executions != 1 || executor.cleanups != 1 {
		t.Fatalf("unexpected pump state worked=%v err=%v control=%+v executor=%+v", worked, err, control, executor)
	}
}

func TestExecutionOperationPumpKeepsReadbackOnSubmitLoss(t *testing.T) {
	control := &fakeOperationControl{available: true, submitErr: errors.New("response lost")}
	executor := &fakeOperationExecutor{}
	worked, err := (executionOperationPump{control: control, executor: executor}).RunOnce(context.Background())
	if err == nil || !worked || executor.cleanups != 0 {
		t.Fatalf("submit loss was not retained: worked=%v err=%v cleanups=%d", worked, err, executor.cleanups)
	}
}

func TestExecutionOperationPumpCleansAfterCommittedResultResponseLoss(t *testing.T) {
	control := &fakeOperationControl{available: true, submitErr: errors.New("response lost"), confirmed: true}
	executor := &fakeOperationExecutor{}
	worked, err := (executionOperationPump{control: control, executor: executor}).RunOnce(context.Background())
	if err != nil || !worked || executor.executions != 1 || executor.cleanups != 1 {
		t.Fatalf("committed result was not confirmed and cleaned: worked=%v err=%v executor=%+v", worked, err, executor)
	}
}

func TestExecutionOperationPumpCleansUpAfterAuthoritativeFence(t *testing.T) {
	controlAPI := &fakeOperationControl{available: true, submitErr: &control.APIError{ReasonCode: executionaccessdomain.ErrGrantRevoked.Error()}}
	executor := &fakeOperationExecutor{}
	worked, err := (executionOperationPump{control: controlAPI, executor: executor}).RunOnce(context.Background())
	if err == nil || !worked || executor.cleanups != 1 {
		t.Fatalf("authoritative fence did not clean resources: worked=%v err=%v cleanups=%d", worked, err, executor.cleanups)
	}
}
