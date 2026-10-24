package domain

import "errors"

// InteractionReplayBinding authorizes exact receipt replay across one committed
// owner transfer. It never authorizes moving a spool or starting an Agent.
type InteractionReplayBinding struct {
	ReconciliationID         int64  `json:"reconciliationId,string"`
	TaskID                   int64  `json:"taskId,string"`
	AttemptID                int64  `json:"attemptId,string"`
	Generation               int64  `json:"generation,string"`
	OldOwnerEpoch            int64  `json:"oldOwnerEpoch,string"`
	NewOwnerEpoch            int64  `json:"newOwnerEpoch,string"`
	ContainerID              string `json:"containerId"`
	ExecutionIdentityDigest  string `json:"executionIdentityDigest"`
	AdapterSessionGeneration int64  `json:"adapterSessionGeneration,string"`
	RoundKey                 string `json:"roundKey"`
}

// ValidateReattachContinuity compares authenticated Worker observations on both
// sides of adoption. Empty session fields preserve non-interaction runtimes but
// cannot yield interaction replay authority.
func ValidateReattachContinuity(before RuntimeEvidence, after ReattachAck) error {
	if before.ContainerID != after.ContainerID || before.ExecutionIdentityDigest != after.ExecutionIdentityDigest ||
		before.OwnerEpoch != after.OldOwnerEpoch || before.OwnerEpoch+1 != after.NewOwnerEpoch ||
		before.AdapterSessionGeneration != after.AdapterSessionGeneration || before.RoundKey != after.RoundKey {
		return errors.New("reattach session identity changed")
	}
	if before.AdapterSessionGeneration < 0 || (before.AdapterSessionGeneration == 0) != (before.RoundKey == "") || len(before.RoundKey) > 256 {
		return errors.New("reattach session identity incomplete")
	}
	_, beforeStart, err := ResolveReattachMode(before.SupervisorState, before.HarnessPID)
	if err != nil {
		return err
	}
	_, afterStart, err := ResolveReattachMode(after.SupervisorState, after.HarnessPID)
	if err != nil {
		return err
	}
	if beforeStart != afterStart || (!beforeStart && (before.HarnessPID != after.HarnessPID || before.RunNonceDigest != after.RunNonceDigest)) {
		return errors.New("reattach execution changed")
	}
	return nil
}
