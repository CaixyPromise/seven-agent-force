// Package facade is the controlled execution-access integration boundary.
package facade

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
)

type IDGenerator interface{ NextID() int64 }

const NetworkCommandToolName = "run_command_with_network_access"

const NetworkCommandToolDescription = "Run a shell command that requires outbound network access. Call this tool whenever the task container cannot reach a required external host; do not retry the command through the normal shell while this call is pending. Provide the purpose, exact shell command, repository-relative working directory, and every required host, protocol, and port. The platform may ask the user to allow this operation once or for the rest of the task."

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func NetworkCommandToolDefinition() ToolDefinition {
	return ToolDefinition{Name: NetworkCommandToolName, Description: NetworkCommandToolDescription, InputSchema: map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"purpose", "command", "cwd", "targets"},
		"properties": map[string]any{
			"purpose": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048},
			"command": map[string]any{"type": "string", "minLength": 1, "maxLength": 16384},
			"cwd":     map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
			"targets": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "uniqueItems": true, "items": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"host", "protocol", "port"},
				"properties": map[string]any{"host": map[string]any{"type": "string"}, "protocol": map[string]any{"type": "string", "enum": []string{"http", "https"}}, "port": map[string]any{"type": "integer", "minimum": 1, "maximum": 65535}},
			}},
		},
	}}
}

type User struct {
	TenantID, TaskID, UserID int64
	CanContinue              bool
}

// Machine is populated from authenticated Worker middleware, not request JSON.
type Machine struct {
	TenantID, TaskID, TaskTurnID                                int64
	WorkerID                                                    int64
	InstanceID, HostBootID                                      string
	AttemptID, Generation, OwnerEpoch, AdapterSessionGeneration int64
	ExecutionLease                                              string
	// ColdResumeOperationID is set only by the machine-only cold restore
	// endpoint. It authorizes the new Attempt to finish the exact Operation
	// bound to the consumed INTERACTION_SUSPEND checkpoint; it never broadens
	// normal hot-session authority.
	ColdResumeOperationID int64
}

type Target struct {
	TenantID, TaskID, TaskTurnID, AttemptID, AdapterSessionGeneration int64
	PrincipalID, WorkerID, OwnerEpoch                                 int64
	Capabilities                                                      []string
	TaskTerminal, TaskCancelled                                       bool
	SourceOperationID                                                 int64
}

const (
	ActionRead    = "READ"
	ActionDecide  = "DECIDE"
	ActionRevoke  = "REVOKE"
	ActionPublish = "PUBLISH"
	ActionClaim   = "CLAIM"
	ActionStart   = "START"
	ActionRenew   = "RENEW"
	ActionResult  = "RESULT"
	ActionReceipt = "RESULT_RECEIPT"
	ActionPull    = "PULL"
	ActionAck     = "ACK"
	ActionClose   = "CLOSE"
)

// UserAuthority must authorize and lock the tenant Task for mutations. A
// principal ID is the resolved structured Workforce principal, never a user
// name or a composed identity string.
type UserAuthority interface {
	AuthorizeUser(context.Context, pgx.Tx, User, string) (Target, error)
}

// MachineAuthority validates current Worker instance, host boot, Attempt
// generation, owner epoch, lease and adapter generation in the caller's tx.
type MachineAuthority interface {
	ValidateMachine(context.Context, pgx.Tx, Machine, string) (Target, error)
}

type Cipher interface {
	Encrypt(context.Context, []byte) (domain.EncryptedDescriptor, error)
	Decrypt(context.Context, domain.EncryptedDescriptor) ([]byte, error)
}

// Projector receives only a safe projection: never descriptor ciphertext,
// command text, raw output, Worker identity, lease or platform credentials.
type Projector interface {
	ProjectExecutionOperation(context.Context, pgx.Tx, Projection) error
}

type Projection struct {
	TenantID, TaskID, TaskTurnID, AttemptID, OperationID int64
	Version                                              int64
	Status, Purpose, ResultDeliveryStatus, ResultSummary string
	Targets                                              []domain.Target
	ExitCode                                             *int
	AgentContinued                                       bool
	AttentionRequired                                    bool
	OccurredAt                                           time.Time
}

type PublishRequest struct {
	RoundKey         string            `json:"roundKey"`
	LocalToolCallKey string            `json:"localToolCallKey"`
	NativeToolCallID string            `json:"nativeToolCallId"`
	ToolName         string            `json:"toolName"`
	ArgumentsDigest  string            `json:"argumentsDigest"`
	Arguments        domain.Descriptor `json:"arguments"`
}

type DecisionRequest struct {
	ExpectedVersion int64  `json:"expectedVersion,string"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

type RevokeRequest struct {
	ExpectedVersion int64  `json:"expectedVersion,string"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

type ClaimRequest struct {
	OperationID int64         `json:"operationId,string"`
	LeaseTTL    time.Duration `json:"-"`
}

type StartRequest struct {
	OperationID   int64  `json:"operationId,string"`
	AttemptID     int64  `json:"attemptId,string"`
	ContainerID   string `json:"containerId"`
	RelayIdentity string `json:"relayIdentity"`
}

type RenewRequest struct {
	OperationID int64 `json:"operationId,string"`
	AttemptID   int64 `json:"attemptId,string"`
}

type ResultRequest struct {
	OperationID  int64  `json:"operationId,string"`
	AttemptID    int64  `json:"attemptId,string"`
	Status       string `json:"status"`
	ResultDigest string `json:"resultDigest"`
	ExitCode     *int   `json:"exitCode,omitempty"`
	Summary      string `json:"summary"`
	Output       string `json:"output"`
}

// ResultReceiptRequest confirms whether the Server committed one exact
// terminal result after the submit response became uncertain. It is a
// readback contract only: it never claims an Operation or authorizes another
// execution.
type ResultReceiptRequest struct {
	OperationID  int64  `json:"operationId,string"`
	AttemptID    int64  `json:"attemptId,string"`
	ResultDigest string `json:"resultDigest"`
}

type CloseRequest struct {
	RoundKey         string `json:"roundKey"`
	LocalToolCallKey string `json:"localToolCallKey"`
}

type ResultAckRequest struct {
	OperationID    int64 `json:"operationId,string"`
	NativeResolved bool  `json:"nativeResolved"`
	AgentContinued bool  `json:"agentContinued"`
}

type Receipt struct {
	TaskTurnID   int64  `json:"taskTurnId,string,omitempty"`
	OperationID  int64  `json:"operationId,string"`
	AttemptID    int64  `json:"attemptId,string,omitempty"`
	GrantID      int64  `json:"grantId,string,omitempty"`
	Version      int64  `json:"version,string"`
	Status       string `json:"status"`
	ResultDigest string `json:"resultDigest,omitempty"`
	Replayed     bool   `json:"replayed"`
}

type OperationView struct {
	ID                   int64           `json:"id,string"`
	TaskID               int64           `json:"taskId,string"`
	TaskTurnID           int64           `json:"taskTurnId,string"`
	AttemptID            int64           `json:"attemptId,string"`
	Version              int64           `json:"version,string"`
	Kind                 string          `json:"kind"`
	Status               string          `json:"status"`
	Purpose              string          `json:"purpose"`
	Targets              []domain.Target `json:"targets"`
	Cwd                  string          `json:"cwd"`
	Command              string          `json:"command"`
	ResultDeliveryStatus string          `json:"resultDeliveryStatus"`
	AgentContinued       bool            `json:"agentContinued"`
	DescriptorDigest     string          `json:"descriptorDigest,omitempty"`
	ResultDigest         string          `json:"resultDigest,omitempty"`
	ResultSummary        string          `json:"resultSummary,omitempty"`
	ExitCode             *int            `json:"exitCode,omitempty"`
	ExpiresAt            time.Time       `json:"expiresAt"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}

type GrantView struct {
	ID               int64               `json:"id,string"`
	TaskID           int64               `json:"taskId,string"`
	SourceDecisionID int64               `json:"sourceDecisionId,string"`
	Version          int64               `json:"version,string"`
	Targets          []domain.Target     `json:"targets"`
	Status           string              `json:"status"`
	ActorUserID      int64               `json:"actorUserId,string"`
	Purpose          string              `json:"purpose"`
	LastOperation    *GrantOperationView `json:"lastOperation,omitempty"`
	ExpiresAt        time.Time           `json:"expiresAt"`
	CreatedAt        time.Time           `json:"createdAt"`
	UpdatedAt        time.Time           `json:"updatedAt"`
}

type GrantOperationView struct {
	Status               string     `json:"status"`
	ResultSummary        string     `json:"resultSummary,omitempty"`
	ResultDeliveryStatus string     `json:"resultDeliveryStatus"`
	ExitCode             *int       `json:"exitCode,omitempty"`
	AgentContinued       bool       `json:"agentContinued"`
	FinishedAt           *time.Time `json:"finishedAt,omitempty"`
}

type Claim struct {
	Operation      OperationView `json:"operation"`
	AttemptID      int64         `json:"attemptId,string"`
	LeaseExpiresAt time.Time     `json:"leaseExpiresAt"`
}

type PendingSuspendOperation struct {
	OperationID      int64  `json:"operationId,string"`
	RoundKey         string `json:"roundKey"`
	LocalToolCallKey string `json:"localToolCallKey"`
	NativeToolCallID string `json:"nativeToolCallId"`
}

type ToolResult struct {
	OperationID              int64  `json:"operationId,string"`
	RoundKey                 string `json:"roundKey"`
	LocalToolCallKey         string `json:"localToolCallKey"`
	NativeToolCallID         string `json:"nativeToolCallId"`
	AdapterSessionGeneration int64  `json:"adapterSessionGeneration,string"`
	Status                   string `json:"status"`
	ExitCode                 *int   `json:"exitCode,omitempty"`
	Summary                  string `json:"summary,omitempty"`
	Output                   string `json:"output,omitempty"`
	ResultDigest             string `json:"resultDigest,omitempty"`
	ReasonCode               string `json:"reasonCode,omitempty"`
}

type PublicService interface {
	Get(context.Context, User, int64) (OperationView, error)
	Decide(context.Context, User, int64, DecisionRequest) (Receipt, error)
	ListGrants(context.Context, User) ([]GrantView, error)
	RevokeGrant(context.Context, User, int64, RevokeRequest) (Receipt, error)
}

type MachineService interface {
	Publish(context.Context, Machine, PublishRequest) (Receipt, error)
	Claim(context.Context, Machine, ClaimRequest) (Claim, error)
	ClaimNext(context.Context, Machine) (Claim, error)
	Start(context.Context, Machine, StartRequest) (Receipt, error)
	Renew(context.Context, Machine, RenewRequest) (Receipt, error)
	SubmitResult(context.Context, Machine, ResultRequest) (Receipt, error)
	ReadResultReceipt(context.Context, Machine, ResultReceiptRequest) (Receipt, error)
	PullResults(context.Context, Machine, int) ([]ToolResult, error)
	AckResult(context.Context, Machine, ResultAckRequest) (Receipt, error)
	CloseToolCall(context.Context, Machine, CloseRequest) (Receipt, error)
	PendingSuspendOperations(context.Context, Machine) ([]PendingSuspendOperation, error)
	ReadResult(context.Context, Machine, int64) (ToolResult, error)
}

// Lifecycle is called by the Task owner inside its existing cancellation
// transaction. It does not commit and does not decide Task state.
type Lifecycle interface {
	CancelTaskTx(context.Context, pgx.Tx, int64, int64, time.Time) error
}
