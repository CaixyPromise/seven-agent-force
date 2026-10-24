package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInteractionReattachContinuity(t *testing.T) {
	before := RuntimeEvidence{ContainerID: "container", ExecutionIdentityDigest: "identity", OwnerEpoch: 5, AdapterSessionGeneration: 2, RoundKey: "round:1", SupervisorState: RuntimeRunning, HarnessPID: 12, RunNonceDigest: "nonce"}
	original := ReattachAck{ContainerID: "container", ExecutionIdentityDigest: "identity", OldOwnerEpoch: 5, NewOwnerEpoch: 6, AdapterSessionGeneration: 2, RoundKey: "round:1", SupervisorState: RuntimeRunning, HarnessPID: 12, RunNonceDigest: "nonce"}
	if err := ValidateReattachContinuity(before, original); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ReattachAck){
		"container": func(a *ReattachAck) { a.ContainerID = "other" }, "identity": func(a *ReattachAck) { a.ExecutionIdentityDigest = "other" },
		"oldOwner": func(a *ReattachAck) { a.OldOwnerEpoch++ }, "newOwner": func(a *ReattachAck) { a.NewOwnerEpoch++ },
		"session": func(a *ReattachAck) { a.AdapterSessionGeneration++ }, "round": func(a *ReattachAck) { a.RoundKey = "round:2" },
		"pid": func(a *ReattachAck) { a.HarnessPID++ }, "nonce": func(a *ReattachAck) { a.RunNonceDigest = "other" },
		"start": func(a *ReattachAck) { a.SupervisorState = RuntimeSupervisorReady; a.HarnessPID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			a := original
			mutate(&a)
			if ValidateReattachContinuity(before, a) == nil {
				t.Fatal("changed identity accepted")
			}
		})
	}
}

func TestInteractionReplayBindingDecimalJSON(t *testing.T) {
	b, _ := json.Marshal(InteractionReplayBinding{ReconciliationID: 9007199254740993, TaskID: 20, AttemptID: 40, Generation: 3, OldOwnerEpoch: 5, NewOwnerEpoch: 6, AdapterSessionGeneration: 1})
	for _, field := range []string{"reconciliationId", "taskId", "attemptId", "generation", "oldOwnerEpoch", "newOwnerEpoch", "adapterSessionGeneration"} {
		if !strings.Contains(string(b), `"`+field+`":"`) {
			t.Fatalf("numeric identity: %s", field)
		}
	}
	empty, _ := json.Marshal(ReattachResult{})
	if strings.Contains(string(empty), "interactionReplayBinding") {
		t.Fatal("ordinary runtime forced into interaction binding")
	}
}
