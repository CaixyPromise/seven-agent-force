package runtime

import (
	"errors"
	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

func bindAdoptedInteractionState(containerID string, taskID, attemptID, generation int64, permit dispatchdomain.ReattachPermit, ack dispatchdomain.ReattachAck, response supervisorResponse) (dispatchdomain.ReattachAck, error) {
	state := response.State
	if !response.OK || containerID == "" || containerID != permit.ContainerID || permit.ExecutionIdentityDigest == "" ||
		state.TaskID != taskID || state.AttemptID != attemptID || state.Generation != generation || state.OwnerEpoch != permit.NewOwnerEpoch ||
		state.SessionGeneration != permit.AdapterSessionGeneration || state.CurrentRound != permit.RoundKey ||
		ack.PermitID != permit.PermitID || ack.OldOwnerEpoch != permit.OldOwnerEpoch || ack.NewOwnerEpoch != permit.NewOwnerEpoch ||
		state.HarnessPID != ack.HarnessPID || state.RunNonceDigest != ack.RunNonceDigest {
		return dispatchdomain.ReattachAck{}, errors.New("REATTACH_INTERACTION_CONTINUITY_MISMATCH")
	}
	_, startNeeded, err := dispatchdomain.ResolveReattachMode(state.State, state.HarnessPID)
	if err != nil || startNeeded != permit.StartNeeded {
		return dispatchdomain.ReattachAck{}, errors.New("REATTACH_INTERACTION_CONTINUITY_MISMATCH")
	}
	ack.ContainerID = containerID
	ack.ExecutionIdentityDigest = permit.ExecutionIdentityDigest
	ack.AdapterSessionGeneration = state.SessionGeneration
	ack.RoundKey = state.CurrentRound
	return ack, nil
}
