package domain

import (
	"encoding/json"
	"fmt"
	"strconv"

	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

const AssignmentSchema = "seven.task-assignment/v1"

type AssignmentMessage struct {
	SchemaVersion              int    `json:"schemaVersion"`
	Kind                       string `json:"kind"`
	MessageID                  int64  `json:"messageId,string"`
	AssignmentID               int64  `json:"assignmentId,string"`
	TaskID                     int64  `json:"taskId,string"`
	ExecutionNodeID            int64  `json:"executionNodeId,string"`
	WorkerID                   int64  `json:"workerId,string"`
	Generation                 int64  `json:"generation,string"`
	ExecutionRequirementDigest string `json:"executionRequirementDigest"`
	ExpiresAt                  string `json:"expiresAt"`
}

type MachineEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	WorkerID      int64  `json:"workerId,string"`
	InstanceID    string `json:"instanceId"`
	HostBootID    string `json:"hostBootId"`
	RequestID     string `json:"requestId"`
	Generation    int64  `json:"generation,string"`
	OwnerEpoch    int64  `json:"ownerEpoch,string,omitempty"`
}

type DeliveryDirective string

const (
	DirectiveStartNewRuntime         DeliveryDirective = "START_NEW_RUNTIME"
	DirectiveResumeAuthorizedRuntime DeliveryDirective = "RESUME_AUTHORIZED_RUNTIME"
	DirectiveAckAlreadyRunning       DeliveryDirective = "ACK_ALREADY_RUNNING"
	DirectiveAckTerminal             DeliveryDirective = "ACK_TERMINAL"
	DirectiveReconcileLiveRuntime    DeliveryDirective = "RECONCILE_LIVE_RUNTIME"
	DirectiveRejectStaleAssignment   DeliveryDirective = "REJECT_STALE_ASSIGNMENT"
)

type ExecutionContract struct {
	Directive                  DeliveryDirective   `json:"directive"`
	AssignmentID               int64               `json:"assignmentId,string"`
	ExecutionNodeID            int64               `json:"executionNodeId,string"`
	ExecutionRequirementDigest string              `json:"executionRequirementDigest"`
	Attempt                    taskdomain.Attempt  `json:"attempt"`
	WorkItem                   taskdomain.WorkItem `json:"workItem"`
	AttemptLeaseToken          string              `json:"attemptLeaseToken"`
	AttemptLeaseExpiresAt      string              `json:"attemptLeaseExpiresAt"`
	OwnerEpoch                 int64               `json:"ownerEpoch,string"`
}

// UnmarshalJSON accepts Framework's quoted int64 representation. Framework
// intentionally encodes int64 as a decimal string at HTTP boundaries to keep
// browser clients lossless; Worker machine clients share that response path.
func (c *ExecutionContract) UnmarshalJSON(body []byte) error {
	type plainContract ExecutionContract
	var wire struct {
		plainContract
		OwnerEpoch json.RawMessage `json:"ownerEpoch,string"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	ownerEpoch, err := parseWireInt64(wire.OwnerEpoch, false, "execution contract owner epoch")
	if err != nil {
		return err
	}
	*c = ExecutionContract(wire.plainContract)
	c.OwnerEpoch = ownerEpoch
	switch c.Directive {
	case DirectiveStartNewRuntime, DirectiveResumeAuthorizedRuntime, DirectiveAckAlreadyRunning, DirectiveReconcileLiveRuntime:
		if ownerEpoch < 1 {
			return fmt.Errorf("execution contract owner epoch must be positive for directive %s", c.Directive)
		}
	}
	return nil
}

type LaunchPermit struct {
	PermitID                int64             `json:"permitId,string"`
	PermitToken             string            `json:"permitToken"`
	ExpiresAt               string            `json:"expiresAt"`
	ExpectedContainerName   string            `json:"expectedContainerName"`
	ExecutionIdentityDigest string            `json:"executionIdentityDigest"`
	Labels                  map[string]string `json:"labels"`
}

type RunPermit struct {
	PermitID    int64  `json:"runPermitId,string"`
	PermitToken string `json:"runPermitToken"`
	RunNonce    string `json:"runNonce"`
	ExpiresAt   string `json:"expiresAt"`
	OwnerEpoch  int64  `json:"ownerEpoch,string"`
}

func (p *RunPermit) UnmarshalJSON(body []byte) error {
	type plainPermit RunPermit
	var wire struct {
		plainPermit
		OwnerEpoch json.RawMessage `json:"ownerEpoch,string"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	ownerEpoch, err := parseWireInt64(wire.OwnerEpoch, true, "run permit owner epoch")
	if err != nil {
		return err
	}
	*p = RunPermit(wire.plainPermit)
	p.OwnerEpoch = ownerEpoch
	return nil
}

func parseWireInt64(value json.RawMessage, positive bool, field string) (int64, error) {
	if len(value) == 0 || string(value) == "null" {
		return 0, fmt.Errorf("%s is required", field)
	}
	raw := string(value)
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return 0, fmt.Errorf("decode %s: %w", field, err)
		}
		raw = text
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || (positive && parsed < 1) || (!positive && parsed < 0) {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

type SupervisorReady struct {
	ContainerID       string `json:"containerId"`
	ControlProtocol   string `json:"controlProtocol"`
	ExecutionIdentity string `json:"executionIdentityDigest"`
}

// ProviderProxyReady is the immutable runtime identity of the trusted Provider
// sidecar. It is recorded before the Agent container is created and is required
// before a Provider-backed Runtime can receive a RunPermit.
type ProviderProxyReady struct {
	ContainerID             string `json:"containerId"`
	ContainerName           string `json:"containerName"`
	ImageDigest             string `json:"imageDigest"`
	ExecutionIdentityDigest string `json:"executionIdentityDigest"`
	ControlState            string `json:"controlState"`
}

type HarnessStarted struct {
	RunPermitID     int64  `json:"runPermitId,string"`
	RunPermitDigest string `json:"runPermitDigest"`
	RunNonceDigest  string `json:"runNonceDigest"`
	HarnessPID      int    `json:"harnessPid"`
	StartedAt       string `json:"startedAt"`
}

type Fact struct {
	FactID           string          `json:"factId"`
	ProducerSequence int64           `json:"producerSequence,string"`
	EventType        string          `json:"eventType"`
	Payload          json.RawMessage `json:"payload"`
	PayloadDigest    string          `json:"payloadDigest"`
}

type Outcome struct {
	Outcome          string
	ReasonCode       string
	Message          string
	ResultCommit     string
	ResultTree       string
	Summary          string
	WorkerID         int64
	WorkerInstanceID string
	HostBootID       string
	Generation       int64
}
