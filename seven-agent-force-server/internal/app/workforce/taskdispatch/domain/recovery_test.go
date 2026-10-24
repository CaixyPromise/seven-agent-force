package domain

import (
	"testing"
	"time"
)

func TestRecoveryPolicyClassifiesOnlyInfrastructureFailuresForAgentRetry(t *testing.T) {
	policy := DefaultRecoveryPolicy()
	for _, code := range []string{
		ReasonHostLost,
		ReasonRuntimeMissing,
		ReasonWorkerLostWithoutRuntime,
		ReasonDockerTransientBeforeHarness,
		ReasonMachineControlTransientBeforeHarness,
		ReasonWorkspacePrepareInfrastructure,
		ReasonSupervisorStartupInfrastructure,
	} {
		if !policy.AgentRetryable(code) {
			t.Fatalf("reason %q must be retryable", code)
		}
	}
	for _, code := range []string{
		"USER_CANCELLED", "HARNESS_TIMEOUT", "HOOK_DENIED", "VALIDATION_FAILED",
		"PROVIDER_TIMEOUT", "SECURITY_IDENTITY_DRIFT", "ARTIFACT_DIGEST_MISMATCH",
	} {
		if policy.AgentRetryable(code) {
			t.Fatalf("reason %q must not rerun the agent", code)
		}
	}
}

func TestRecoveryPolicyAttemptBudgetIncludesInitialAttempt(t *testing.T) {
	policy := DefaultRecoveryPolicy()
	if policy.MaxAgentAttempts != 3 {
		t.Fatalf("max attempts=%d, want 3", policy.MaxAgentAttempts)
	}
	if !policy.CanCreateReplacement(2, ReasonHostLost) {
		t.Fatal("second completed attempt should leave one replacement")
	}
	if policy.CanCreateReplacement(3, ReasonHostLost) {
		t.Fatal("third attempt must exhaust the automatic budget")
	}
	if policy.CanCreateReplacement(1, "HARNESS_TIMEOUT") {
		t.Fatal("non-infrastructure failures must not consume a replacement")
	}
}

func TestRuntimeEvidenceRequiresExactIdentity(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	expected := RuntimeRecoveryIdentity{
		AttemptID: 3107414342, ExecutionNodeID: 400450328, AssignmentID: 984513750,
		WorkerID: 1688799477, HostBootID: "boot_1", Generation: 1, OwnerEpoch: 3,
		ContainerID: "container-1", ContainerName: "seven-attempt-att_1-g1",
		ExecutionIdentityDigest: digest64('a'), HarnessImageDigest: digest64('b'), HookConfigurationDigest: digest64('c'),
		ReattachDeadline: now.Add(2 * time.Minute),
	}
	evidence := RuntimeEvidence{
		AttemptID: expected.AttemptID, ExecutionNodeID: expected.ExecutionNodeID, AssignmentID: expected.AssignmentID,
		WorkerID: expected.WorkerID, HostBootID: expected.HostBootID, Generation: expected.Generation, OwnerEpoch: expected.OwnerEpoch,
		ContainerID: expected.ContainerID, ContainerName: expected.ContainerName,
		LabelsDigest:            digest64('d'),
		ExecutionIdentityDigest: expected.ExecutionIdentityDigest, HarnessImageDigest: expected.HarnessImageDigest, HookConfigurationDigest: expected.HookConfigurationDigest,
		SupervisorProtocol: SupervisorControlV2, SupervisorState: RuntimeRunning, HarnessPID: 42,
	}
	if err := evidence.ValidateFor(expected, now); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	evidence.ContainerID = "other"
	if err := evidence.ValidateFor(expected, now); err == nil {
		t.Fatal("container identity drift must be rejected")
	}
}

func TestProviderBackedRuntimeEvidenceRequiresExactProxy(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	expected := RuntimeRecoveryIdentity{
		AttemptID: 3107414342, ExecutionNodeID: 400450328, AssignmentID: 984513750, WorkerID: 1688799477, HostBootID: "boot_1",
		Generation: 1, OwnerEpoch: 3, ContainerID: "agent-1", ContainerName: "seven-attempt-att_1-g1",
		ExecutionIdentityDigest: digest64('a'), HarnessImageDigest: digest64('b'), HookConfigurationDigest: digest64('c'),
		ProviderBacked: true, ProviderProxyContainerID: "proxy-1", ProviderProxyContainerName: "seven-provider-proxy-att_1",
		ProviderProxyImageDigest: digest64('e'), ProviderProxyIdentityDigest: digest64('f'), ReattachDeadline: now.Add(time.Minute),
	}
	evidence := RuntimeEvidence{
		AttemptID: expected.AttemptID, ExecutionNodeID: expected.ExecutionNodeID, AssignmentID: expected.AssignmentID,
		WorkerID: expected.WorkerID, HostBootID: expected.HostBootID, Generation: 1, OwnerEpoch: 3,
		ContainerID: expected.ContainerID, ContainerName: expected.ContainerName, LabelsDigest: digest64('d'),
		ExecutionIdentityDigest: expected.ExecutionIdentityDigest, HarnessImageDigest: expected.HarnessImageDigest,
		HookConfigurationDigest: expected.HookConfigurationDigest, SupervisorProtocol: SupervisorControlV2, SupervisorState: RuntimeRunning, HarnessPID: 42,
	}
	if err := evidence.ValidateFor(expected, now); err == nil {
		t.Fatal("provider-backed Runtime without Proxy evidence must be rejected")
	}
	evidence.ProviderProxy = &ProviderProxyEvidence{ContainerID: "proxy-1", ContainerName: "seven-provider-proxy-att_1", LabelsDigest: digest64('1'), ImageDigest: digest64('e'), ExecutionIdentityDigest: digest64('f'), OwnerEpoch: 3, ControlState: "READY"}
	if err := evidence.ValidateFor(expected, now); err != nil {
		t.Fatalf("valid dual-component evidence rejected: %v", err)
	}
	evidence.ProviderProxy.ImageDigest = digest64('9')
	if err := evidence.ValidateFor(expected, now); err == nil {
		t.Fatal("Proxy digest drift must be rejected")
	}
}

func TestReattachPhaseNeverRestartsRunningHarness(t *testing.T) {
	for _, tt := range []struct {
		state       string
		harnessPID  int
		wantMode    ReattachMode
		startNeeded bool
	}{
		{RuntimeSupervisorReady, 0, ReattachResumeGated, true},
		{RuntimeStarting, 0, ReattachResumeGated, true},
		{RuntimeStarting, 77, ReattachFollowRunning, false},
		{RuntimeRunning, 77, ReattachFollowRunning, false},
	} {
		mode, start, err := ResolveReattachMode(tt.state, tt.harnessPID)
		if err != nil || mode != tt.wantMode || start != tt.startNeeded {
			t.Fatalf("state=%s pid=%d got mode=%s start=%t err=%v", tt.state, tt.harnessPID, mode, start, err)
		}
	}
}

func digest64(ch byte) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}
