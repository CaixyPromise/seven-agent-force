package domain

import "testing"

func TestRuntimeLossReplacementRequiresProvenPreHarness(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    RuntimeLossExecutionEvidence
		want string
	}{
		{"authorized", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeAuthorized}, ""},
		{"container", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeContainerCreated}, ""},
		{"supervisor", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeSupervisorReady}, ""},
		{"pre-recovery", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeRecovering, PreviousRuntimeStatus: RuntimeSupervisorReady}, ""},
		{"started", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeRecovering, HarnessStarted: true}, ReasonRuntimeLostAfterHarnessStart},
		{"nonce-accepted", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeSupervisorReady, RunNonceAccepted: true}, ReasonRuntimeLostAfterHarnessStart},
		{"permit-no-ack", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeStarting, RunPermitIssued: true}, ReasonRuntimeStartOutcomeUnknown},
		{"cleared-permit-history", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeSupervisorReady, RunPermitIssued: true}, ReasonRuntimeStartOutcomeUnknown},
		{"previous-running", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeReattaching, PreviousRuntimeStatus: RuntimeRunning}, ReasonRuntimeLostAfterHarnessStart},
		{"task-running", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeSupervisorReady, TaskStatus: "RUNNING"}, ReasonRuntimeLostAfterHarnessStart},
		{"unknown-recovery", RuntimeLossExecutionEvidence{RuntimeStatus: RuntimeRecovering}, ReasonRuntimeStartOutcomeUnknown},
		{"unknown", RuntimeLossExecutionEvidence{}, ReasonRuntimeStartOutcomeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.e.ReplacementBlockedReason(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
