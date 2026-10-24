package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	workerruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	dispatch "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

// Exercise real CLI producers through real Worker readers. The fake Docker
// executable only supplies transport bytes; it does not recreate any decoder.
func TestRecoveryWireActualProducersToWorker(t *testing.T) {
	const big int64 = 9007199254740993
	root := t.TempDir()
	textID := strconv.FormatInt(big, 10)
	state := &supervisorState{TaskID: big, AttemptID: big, Generation: big, OwnerEpoch: big, State: "RUNNING", HarnessPID: 123, ReattachPermitID: big, ReattachPermitDigest: strings.Repeat("a", 64), RunNonceDigest: strings.Repeat("b", 64), LastJournalSequence: big, EvaluationCycleID: big, observationSequence: big}
	state.interaction.sessionGeneration = big
	state.interaction.currentRound = "round:1"
	state.interaction.active = true
	imageDigest, identityDigest := strings.Repeat("c", 64), strings.Repeat("d", 64)
	labels := map[string]string{"seven.managed": "true", "seven.task.id": textID, "seven.attempt.id": textID, "seven.execution.node.id": textID, "seven.assignment.id": textID, "seven.worker.id": textID, "seven.worker.host-boot-id": "test-boot", "seven.attempt.generation": textID, "seven.owner.epoch": textID, "seven.execution.identity.digest": identityDigest, "seven.harness.image.digest": imageDigest, "seven.hook.configuration.digest": ""}
	writeJSON := func(name string, value any) {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("inspect.json", []any{map[string]any{"Id": "test-container", "State": map[string]any{"Running": true}, "Config": map[string]any{"Labels": labels}}})
	writeJSON("state.json", map[string]any{"ok": true, "state": protocolStateSnapshot(state)})
	script := "#!/bin/sh\ncase \"$1\" in\ncontainer) /bin/cat \"$WIRE_FIXTURE/inspect.json\";;\nexec) input=$(/bin/cat); case \"$input\" in *adopt-owner*) /bin/cat \"$WIRE_FIXTURE/ack.json\";; *) /bin/cat \"$WIRE_FIXTURE/state.json\";; esac;;\n*) exit 2;;\nesac\n"
	if err := os.WriteFile(filepath.Join(root, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WIRE_FIXTURE", root)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	docker := workerruntime.NewDocker(config.WorkerConfig{WorkspaceRoot: root}, config.ProviderProxyConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe, err := docker.InspectRuntimeForRecovery(ctx, big, big, big, big, "test-boot", "test-name", "test-container", imageDigest, "", identityDigest, big, big)
	if err != nil {
		t.Fatalf("actual state decode: %v", err)
	}
	evidence := probe.Evidence
	evidence.InteractionReplayRequested = true
	// Audit every int64 field of the real evidence DTO, not a handpicked list.
	evidenceBody, _ := json.Marshal(evidence)
	var evidenceFields map[string]json.RawMessage
	if err = json.Unmarshal(evidenceBody, &evidenceFields); err != nil {
		t.Fatal(err)
	}
	shape := reflect.TypeOf(evidence)
	for i := 0; i < shape.NumField(); i++ {
		field := shape.Field(i)
		if field.Type.Kind() != reflect.Int64 {
			continue
		}
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if value, ok := evidenceFields[key]; ok {
			var decimal string
			if json.Unmarshal(value, &decimal) != nil {
				t.Fatalf("evidence %s is not decimal string", key)
			}
		}
	}
	snapshot := protocolStateSnapshot(state)
	for key, value := range snapshot {
		if reflect.TypeOf(value) != nil && reflect.TypeOf(value).Kind() == reflect.Int64 {
			t.Fatalf("state %s leaked int64 without wire encoding", key)
		}
	}
	body, err := json.Marshal(map[string]any{"evidence": evidence})
	if err != nil {
		t.Fatal(err)
	}
	var server struct {
		Evidence dispatch.RuntimeEvidence `json:"evidence"`
	}
	if err = json.Unmarshal(body, &server); err != nil {
		t.Fatalf("actual evidence decode: %v", err)
	}
	if server.Evidence.AdapterSessionGeneration != big || server.Evidence.LastJournalSequence != big || server.Evidence.RoundKey != "round:1" {
		t.Fatal("evidence lost counters")
	}
	if err = server.Evidence.ValidateFor(dispatch.RuntimeRecoveryIdentity{AttemptID: big, ExecutionNodeID: big, AssignmentID: big, WorkerID: big, HostBootID: "test-boot", Generation: big, OwnerEpoch: big, ContainerID: "test-container", ContainerName: "test-name", ExecutionIdentityDigest: identityDigest, HarnessImageDigest: imageDigest, ReattachDeadline: time.Now().Add(time.Minute)}, time.Now()); err != nil {
		t.Fatalf("Server evidence contract: %v", err)
	}
	state.OwnerEpoch = big + 1
	writeJSON("ack.json", map[string]any{"ok": true, "reattachAck": reattachAckSnapshot(state, big)})
	writeJSON("state.json", map[string]any{"ok": true, "state": protocolStateSnapshot(state)})
	ack, err := docker.AdoptOwner(ctx, "test-container", big, big, big, dispatch.ReattachPermit{PermitID: big, PermitToken: "test-only-not-a-real-permit", OldOwnerEpoch: big, NewOwnerEpoch: big + 1, ContainerID: "test-container", ExecutionIdentityDigest: identityDigest, AdapterSessionGeneration: big, RoundKey: "round:1", InteractionReplayRequested: true})
	if err != nil {
		t.Fatalf("actual ACK and post-state decode: %v", err)
	}
	if err = dispatch.ValidateReattachContinuity(evidence, ack); err != nil {
		t.Fatal(err)
	}
	// Run the actual journal producer, including a filtered machine transition.
	stateDir := filepath.Join(root, textID, "attempts", textID, "state")
	if err = os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(stateDir, "runtime-journal.jsonl")
	file, err := os.OpenFile(journalPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	control := &controlServer{spec: attemptSpec{AttemptID: big}, state: state, journal: file, journalSequence: big}
	if err = control.writeJournal("harness.started", map[string]any{"harnessPid": 123}); err != nil {
		t.Fatal(err)
	}
	if err = control.writeJournal("owner.adopted", reattachAckSnapshot(state, big)); err != nil {
		t.Fatal(err)
	}
	if err = control.writeJournal("hook.deny", map[string]any{"reasonCode": "TEST_DENIED"}); err != nil {
		t.Fatal(err)
	}
	facts, err := docker.ReadRuntimeJournalFacts(task.WorkItem{TaskID: big}, task.Attempt{ID: big})
	if err != nil {
		t.Fatalf("actual journal replay: %v", err)
	}
	if len(facts) != 2 || facts[0].EventType != "owner.adopted" || facts[0].ProducerSequence != big+2 || facts[1].EventType != "hook.deny" {
		t.Fatal("safe replay or sequence changed")
	}
	factBody, _ := json.Marshal(facts)
	var serverFacts []dispatch.Fact
	if err = json.Unmarshal(factBody, &serverFacts); err != nil || len(serverFacts) != 2 || serverFacts[0].ProducerSequence != big+2 {
		t.Fatal("Worker fact replay wire lost sequence")
	}
	// Wrong numeric journal remains rejected by the actual reader, no shim.
	journal, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	bad := bytes.Replace(journal, []byte(`"sequence":"9007199254740994"`), []byte(`"sequence":9007199254740994`), 1)
	if bytes.Equal(bad, journal) {
		t.Fatal("producer did not emit decimal sequence")
	}
	if err = os.WriteFile(journalPath, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = docker.ReadRuntimeJournalFacts(task.WorkItem{TaskID: big}, task.Attempt{ID: big}); err == nil {
		t.Fatal("wrong number journal accepted")
	}
}
