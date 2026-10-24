package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RuntimeAuthorized             = "AUTHORIZED"
	RuntimeContainerCreated       = "CONTAINER_CREATED"
	RuntimeSupervisorReady        = "SUPERVISOR_READY"
	RuntimeStarting               = "STARTING"
	RuntimeRunning                = "RUNNING"
	RuntimeRecovering             = "RECOVERING"
	RuntimeReattaching            = "REATTACHING"
	RuntimeTerminating            = "TERMINATING"
	RuntimeReconciliationRequired = "RECONCILIATION_REQUIRED"
	RuntimeLost                   = "LOST"
	RuntimeFenced                 = "FENCED"
	RuntimeSucceeded              = "SUCCEEDED"
	RuntimeFailed                 = "FAILED"
	RuntimeCancelled              = "CANCELLED"
	RuntimeTimedOut               = "TIMED_OUT"

	SupervisorControlV2 = "seven.supervisor-control/v2"

	ReasonHostLost                             = "HOST_LOST"
	ReasonRuntimeMissing                       = "RUNTIME_MISSING"
	ReasonWorkerLostWithoutRuntime             = "WORKER_LOST_WITHOUT_REATTACHABLE_RUNTIME"
	ReasonDockerTransientBeforeHarness         = "DOCKER_DAEMON_TRANSIENT_BEFORE_HARNESS"
	ReasonMachineControlTransientBeforeHarness = "MACHINE_CONTROL_TRANSIENT_BEFORE_HARNESS"
	ReasonWorkspacePrepareInfrastructure       = "WORKSPACE_PREPARE_INFRASTRUCTURE_FAILURE"
	ReasonSupervisorStartupInfrastructure      = "SUPERVISOR_STARTUP_INFRASTRUCTURE_FAILURE"
)

type RecoveryPolicy struct {
	MaxAgentAttempts int
	LeaseTTL         time.Duration
	RenewInterval    time.Duration
	ReattachGrace    time.Duration
	TerminateGrace   time.Duration
	KillGrace        time.Duration
	retryable        map[string]struct{}
}

func DefaultRecoveryPolicy() RecoveryPolicy {
	return RecoveryPolicy{
		MaxAgentAttempts: 3,
		LeaseTTL:         30 * time.Second,
		RenewInterval:    10 * time.Second,
		ReattachGrace:    2 * time.Minute,
		TerminateGrace:   10 * time.Second,
		KillGrace:        5 * time.Second,
		retryable: map[string]struct{}{
			ReasonHostLost: {}, ReasonRuntimeMissing: {}, ReasonWorkerLostWithoutRuntime: {},
			ReasonDockerTransientBeforeHarness: {}, ReasonMachineControlTransientBeforeHarness: {},
			ReasonWorkspacePrepareInfrastructure: {}, ReasonSupervisorStartupInfrastructure: {},
		},
	}
}

func (p RecoveryPolicy) AgentRetryable(reason string) bool {
	_, ok := p.retryable[strings.TrimSpace(reason)]
	return ok
}

func (p RecoveryPolicy) CanCreateReplacement(attemptCount int, reason string) bool {
	return attemptCount >= 0 && attemptCount < p.MaxAgentAttempts && p.AgentRetryable(reason)
}

type RuntimeRecoveryIdentity struct {
	AttemptID                   int64
	ExecutionNodeID             int64
	AssignmentID                int64
	WorkerID                    int64
	HostBootID                  string
	Generation                  int64
	OwnerEpoch                  int64
	ContainerID                 string
	ContainerName               string
	ExecutionIdentityDigest     string
	HarnessImageDigest          string
	HookConfigurationDigest     string
	ProviderBacked              bool
	ProviderProxyContainerID    string
	ProviderProxyContainerName  string
	ProviderProxyImageDigest    string
	ProviderProxyIdentityDigest string
	ReattachDeadline            time.Time
}

type ProviderProxyEvidence struct {
	ContainerID             string `json:"containerId"`
	ContainerName           string `json:"containerName"`
	LabelsDigest            string `json:"labelsDigest"`
	ImageDigest             string `json:"imageDigest"`
	ExecutionIdentityDigest string `json:"executionIdentityDigest"`
	OwnerEpoch              int64  `json:"ownerEpoch,string"`
	ControlState            string `json:"controlState"`
}

type RuntimeEvidence struct {
	InteractionReplayRequested bool                   `json:"interactionReplayRequested,omitempty"`
	AdapterSessionGeneration   int64                  `json:"adapterSessionGeneration,string,omitempty"`
	RoundKey                   string                 `json:"roundKey,omitempty"`
	AttemptID                  int64                  `json:"attemptId,string"`
	ExecutionNodeID            int64                  `json:"executionNodeId,string"`
	AssignmentID               int64                  `json:"assignmentId,string"`
	WorkerID                   int64                  `json:"workerId,string"`
	HostBootID                 string                 `json:"hostBootId"`
	Generation                 int64                  `json:"generation,string"`
	OwnerEpoch                 int64                  `json:"ownerEpoch,string"`
	ContainerID                string                 `json:"containerId"`
	ContainerName              string                 `json:"containerName"`
	LabelsDigest               string                 `json:"labelsDigest"`
	ExecutionIdentityDigest    string                 `json:"executionIdentityDigest"`
	HarnessImageDigest         string                 `json:"harnessImageDigest"`
	HookConfigurationDigest    string                 `json:"hookConfigurationDigest"`
	SupervisorProtocol         string                 `json:"supervisorProtocol"`
	SupervisorState            string                 `json:"supervisorState"`
	HarnessPID                 int                    `json:"harnessPid"`
	RunNonceDigest             string                 `json:"runNonceDigest,omitempty"`
	LastJournalSequence        int64                  `json:"lastJournalSequence,string"`
	ProviderProxy              *ProviderProxyEvidence `json:"providerProxy,omitempty"`
}

func (e RuntimeEvidence) ValidateFor(expected RuntimeRecoveryIdentity, now time.Time) error {
	if e.InteractionReplayRequested && (e.AdapterSessionGeneration < 1 || e.RoundKey == "" || len(e.RoundKey) > 256) {
		return errors.New("interaction replay session identity required")
	}
	if now.After(expected.ReattachDeadline) {
		return errors.New("reattach deadline expired")
	}
	if e.AttemptID != expected.AttemptID || e.ExecutionNodeID != expected.ExecutionNodeID || e.AssignmentID != expected.AssignmentID ||
		e.WorkerID != expected.WorkerID || e.HostBootID != expected.HostBootID || e.Generation != expected.Generation || e.OwnerEpoch != expected.OwnerEpoch {
		return errors.New("runtime authority identity mismatch")
	}
	if e.ContainerID != expected.ContainerID || e.ContainerName != expected.ContainerName ||
		e.ExecutionIdentityDigest != expected.ExecutionIdentityDigest || e.HarnessImageDigest != expected.HarnessImageDigest || e.HookConfigurationDigest != expected.HookConfigurationDigest {
		return errors.New("runtime container identity mismatch")
	}
	if len(e.LabelsDigest) != 64 {
		return errors.New("runtime labels digest is required")
	}
	if e.SupervisorProtocol != SupervisorControlV2 {
		return errors.New("unsupported supervisor control protocol")
	}
	if _, _, err := ResolveReattachMode(e.SupervisorState, e.HarnessPID); err != nil {
		return err
	}
	if expected.ProviderBacked {
		if e.ProviderProxy == nil {
			return errors.New("provider proxy evidence is required")
		}
		proxy := e.ProviderProxy
		if proxy.ContainerID != expected.ProviderProxyContainerID || proxy.ContainerName != expected.ProviderProxyContainerName ||
			proxy.ImageDigest != expected.ProviderProxyImageDigest || proxy.ExecutionIdentityDigest != expected.ProviderProxyIdentityDigest ||
			proxy.OwnerEpoch != expected.OwnerEpoch || proxy.ControlState != "READY" || len(proxy.LabelsDigest) != 64 {
			return errors.New("provider proxy identity mismatch")
		}
	} else if e.ProviderProxy != nil {
		return errors.New("unexpected provider proxy evidence")
	}
	return nil
}

type ReattachMode string

const (
	ReattachResumeGated   ReattachMode = "RESUME_GATED_RUNTIME"
	ReattachFollowRunning ReattachMode = "FOLLOW_RUNNING_RUNTIME"
)

func ResolveReattachMode(runtimeState string, harnessPID int) (ReattachMode, bool, error) {
	switch runtimeState {
	case RuntimeSupervisorReady:
		if harnessPID != 0 {
			return "", false, errors.New("READY runtime must not report a harness pid")
		}
		return ReattachResumeGated, true, nil
	case RuntimeStarting:
		if harnessPID > 0 {
			return ReattachFollowRunning, false, nil
		}
		return ReattachResumeGated, true, nil
	case RuntimeRunning:
		if harnessPID < 1 {
			return "", false, errors.New("RUNNING runtime must report a harness pid")
		}
		return ReattachFollowRunning, false, nil
	default:
		return "", false, fmt.Errorf("runtime state %q is not reattachable", runtimeState)
	}
}

type RecoverableRuntime struct {
	TaskID                      int64  `json:"taskId,string"`
	AttemptID                   int64  `json:"attemptId,string"`
	ExecutionNodeID             int64  `json:"executionNodeId,string"`
	AssignmentID                int64  `json:"assignmentId,string"`
	Generation                  int64  `json:"generation,string"`
	OwnerEpoch                  int64  `json:"ownerEpoch,string"`
	HostBootID                  string `json:"hostBootId"`
	ExpectedContainerName       string `json:"expectedContainerName"`
	ContainerID                 string `json:"containerId"`
	RuntimeStatus               string `json:"runtimeStatus"`
	ExecutionIdentityDigest     string `json:"executionIdentityDigest"`
	HarnessImageDigest          string `json:"harnessImageDigest"`
	HookConfigurationDigest     string `json:"hookConfigurationDigest"`
	ProviderBacked              bool   `json:"providerBacked"`
	ProviderProxyContainerID    string `json:"providerProxyContainerId,omitempty"`
	ProviderProxyContainerName  string `json:"providerProxyContainerName,omitempty"`
	ProviderProxyImageDigest    string `json:"providerProxyImageDigest,omitempty"`
	ProviderProxyIdentityDigest string `json:"providerProxyExecutionIdentityDigest,omitempty"`
	ReattachDeadline            string `json:"reattachDeadline"`
}

type ReattachPermit struct {
	InteractionReplayRequested bool         `json:"interactionReplayRequested,omitempty"`
	ContainerID                string       `json:"containerId,omitempty"`
	ExecutionIdentityDigest    string       `json:"executionIdentityDigest,omitempty"`
	AdapterSessionGeneration   int64        `json:"adapterSessionGeneration,string,omitempty"`
	RoundKey                   string       `json:"roundKey,omitempty"`
	PermitID                   int64        `json:"reattachPermitId,string"`
	PermitToken                string       `json:"reattachPermitToken"`
	ExpiresAt                  string       `json:"expiresAt"`
	OldOwnerEpoch              int64        `json:"oldOwnerEpoch,string"`
	NewOwnerEpoch              int64        `json:"newOwnerEpoch,string"`
	LeaseToken                 string       `json:"executionLeaseToken"`
	LeaseExpiresAt             string       `json:"executionLeaseExpiresAt"`
	Mode                       ReattachMode `json:"mode"`
	StartNeeded                bool         `json:"startNeeded"`
}

type ReattachAck struct {
	ContainerID              string                    `json:"containerId,omitempty"`
	ExecutionIdentityDigest  string                    `json:"executionIdentityDigest,omitempty"`
	AdapterSessionGeneration int64                     `json:"adapterSessionGeneration,string,omitempty"`
	RoundKey                 string                    `json:"roundKey,omitempty"`
	PermitID                 int64                     `json:"reattachPermitId,string"`
	PermitDigest             string                    `json:"reattachPermitDigest"`
	OldOwnerEpoch            int64                     `json:"oldOwnerEpoch,string"`
	NewOwnerEpoch            int64                     `json:"newOwnerEpoch,string"`
	SupervisorState          string                    `json:"supervisorState"`
	HarnessPID               int                       `json:"harnessPid"`
	RunNonceDigest           string                    `json:"runNonceDigest,omitempty"`
	LastJournalSequence      int64                     `json:"lastJournalSequence,string"`
	ProviderProxy            *ProviderProxyReattachAck `json:"providerProxy,omitempty"`
}

type ProviderProxyReattachAck struct {
	PermitID      int64  `json:"reattachPermitId,string"`
	PermitDigest  string `json:"reattachPermitDigest"`
	OldOwnerEpoch int64  `json:"oldOwnerEpoch,string"`
	NewOwnerEpoch int64  `json:"newOwnerEpoch,string"`
	ControlState  string `json:"controlState"`
}

type ReattachResult struct {
	InteractionReplayBinding *InteractionReplayBinding `json:"interactionReplayBinding,omitempty"`
	Contract                 ExecutionContract         `json:"contract"`
	RuntimeState             string                    `json:"runtimeState"`
	Mode                     ReattachMode              `json:"mode"`
	StartNeeded              bool                      `json:"startNeeded"`
}

type RuntimeLossResult struct {
	Disposition    string `json:"disposition"`
	AttemptCount   int    `json:"attemptCount"`
	MaxAttempts    int    `json:"maxAttempts"`
	NextGeneration int64  `json:"nextGeneration,string,omitempty"`
	ReasonCode     string `json:"reasonCode"`
}
