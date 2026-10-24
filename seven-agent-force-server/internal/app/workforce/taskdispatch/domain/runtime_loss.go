package domain

const (
	ReasonRuntimeLostAfterHarnessStart = "RUNTIME_LOST_AFTER_HARNESS_START"
	ReasonRuntimeStartOutcomeUnknown   = "RUNTIME_START_OUTCOME_UNKNOWN"
)

// RuntimeLossExecutionEvidence comes from locked Server records, never a loss
// reporter's claimed reason. Missing an acknowledgement is not proof that an
// issued start permit was never executed.
type RuntimeLossExecutionEvidence struct {
	HarnessStarted        bool
	RunNonceAccepted      bool
	RunPermitIssued       bool
	RuntimeStatus         string
	PreviousRuntimeStatus string
	TaskStatus            string
}

func (e RuntimeLossExecutionEvidence) ReplacementBlockedReason() string {
	if e.HarnessStarted || e.RunNonceAccepted || e.RuntimeStatus == RuntimeRunning || e.PreviousRuntimeStatus == RuntimeRunning || e.TaskStatus == "RUNNING" || e.TaskStatus == "REVIEWING" {
		return ReasonRuntimeLostAfterHarnessStart
	}
	if e.RunPermitIssued {
		return ReasonRuntimeStartOutcomeUnknown
	}
	status := e.RuntimeStatus
	if status == RuntimeRecovering || status == RuntimeReattaching || status == RuntimeReconciliationRequired {
		status = e.PreviousRuntimeStatus
	}
	switch status {
	case RuntimeAuthorized, RuntimeContainerCreated, RuntimeSupervisorReady:
		return ""
	default:
		return ReasonRuntimeStartOutcomeUnknown
	}
}
