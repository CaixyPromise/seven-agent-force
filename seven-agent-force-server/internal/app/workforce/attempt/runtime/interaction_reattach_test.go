package runtime

import (
	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"testing"
)

func TestAdoptOwnerInteractionPostStateContinuity(t *testing.T) {
	permit := dispatchdomain.ReattachPermit{PermitID: 90, OldOwnerEpoch: 5, NewOwnerEpoch: 6, ContainerID: "container", ExecutionIdentityDigest: "identity", AdapterSessionGeneration: 1, RoundKey: "round:1", InteractionReplayRequested: true}
	ack := dispatchdomain.ReattachAck{PermitID: 90, OldOwnerEpoch: 5, NewOwnerEpoch: 6, HarnessPID: 12, RunNonceDigest: "nonce", SupervisorState: "RUNNING"}
	original := supervisorResponse{OK: true, State: supervisorStateSnapshot{TaskID: 20, AttemptID: 40, Generation: 3, OwnerEpoch: 6, SessionGeneration: 1, CurrentRound: "round:1", HarnessPID: 12, RunNonceDigest: "nonce", State: "RUNNING"}}
	got, err := bindAdoptedInteractionState("container", 20, 40, 3, permit, ack, original)
	if err != nil || got.AdapterSessionGeneration != 1 || got.RoundKey != "round:1" || got.ContainerID != "container" {
		t.Fatalf("valid transfer: %+v %v", got, err)
	}
	for name, mutate := range map[string]func(*supervisorResponse){
		"failedread": func(r *supervisorResponse) { r.OK = false }, "task": func(r *supervisorResponse) { r.State.TaskID++ }, "attempt": func(r *supervisorResponse) { r.State.AttemptID++ }, "generation": func(r *supervisorResponse) { r.State.Generation++ }, "owner": func(r *supervisorResponse) { r.State.OwnerEpoch-- }, "session": func(r *supervisorResponse) { r.State.SessionGeneration++ }, "round": func(r *supervisorResponse) { r.State.CurrentRound = "round:2" }, "pid": func(r *supervisorResponse) { r.State.HarnessPID++ }, "nonce": func(r *supervisorResponse) { r.State.RunNonceDigest = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			r := original
			mutate(&r)
			if _, err := bindAdoptedInteractionState("container", 20, 40, 3, permit, ack, r); err == nil {
				t.Fatal("drift accepted")
			}
		})
	}
}
