// Package facade is the interaction module's integration boundary.
package facade

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

type IDGenerator interface{ NextID() int64 }
type User struct {
	TenantID, TaskID, UserID int64
	CanContinue              bool
}

// Machine is supplied by authenticated Machine middleware, never browser JSON.
type Machine struct {
	TenantID, TaskID, TaskTurnID                                int64
	WorkerID                                                    int64
	InstanceID, HostBootID                                      string
	AttemptID, Generation, OwnerEpoch, AdapterSessionGeneration int64
	ExecutionLease                                              string
}
type Target struct {
	TenantID, TaskID, TaskTurnID, AttemptID, AdapterSessionGeneration, PrincipalID int64
	AllowedActions                                                                 []string
	Capabilities                                                                   []string
	AcceptsNativeInput                                                             bool
	// ClosedAttemptState comes only from the locked parent Task authority.
	// It reconciles residual input after terminal execution without granting input.
	ClosedAttemptState string
}

const (
	Read        = "READ"
	Answer      = "ANSWER"
	SendMessage = "SEND_MESSAGE"
	// DecideOperation is a conversation-surface action owned by executionaccess.
	// It does not turn an interaction answer into network authority.
	DecideOperation = "DECIDE_OPERATION"
	DecideDelivery  = "DECIDE_DELIVERY"
	RestartDelivery = "RESTART_DELIVERY"
	Publish         = "PUBLISH"
	Pull            = "PULL"
	Ack             = "ACK"
)

// Authority MUST inspect and lock current Task/Attempt authority in tx until
// commit: tenant and Task access for users; current Worker instance, host boot,
// Attempt generation, owner epoch, execution lease and cancel fence for machines.
// Machine also binds the admitted adapter session generation. PrincipalID is a
// resolved workforce_principals identity, not a Worker or User ID. No network
// calls or permissive fallback belong here. A user mutation must authorize the
// active continuation target, not merely organization membership. Read returns
// current server-derived AllowedActions. Implementations return domain safe errors.
type UserAuthority interface {
	AuthorizeUser(context.Context, pgx.Tx, User, string) (Target, error)
}
type MachineAuthority interface {
	Validate(context.Context, pgx.Tx, Machine) (Target, error)
}
type PublishRequest struct {
	RoundKey        string            `json:"roundKey,omitempty"`
	LocalRequestKey string            `json:"localRequestKey"`
	Questions       []domain.Question `json:"questions"`
	IsBlocking      bool              `json:"isBlocking"`
}
type ResponseRequest struct {
	Answers         []domain.Answer `json:"answers"`
	ExpectedVersion int64           `json:"expectedVersion,string"`
	IdempotencyKey  string          `json:"idempotencyKey"`
}
type MessageRequest struct {
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotencyKey"`
}
type AckRequest struct {
	InteractionID int64 `json:"interactionId,string"`
	ResponseID    int64 `json:"responseId,string"`
}
type Receipt struct {
	TaskTurnID    int64 `json:"taskTurnId,string,omitempty"`
	InteractionID int64 `json:"interactionId,string"`
	ResponseID    int64 `json:"responseId,string,omitempty"`
	Version       int64 `json:"version,string"`
	Saved         bool  `json:"saved"`
	Delivered     bool  `json:"delivered"`
	// An acknowledgement does not prove the Agent continued execution.
	ExecutionContinued bool `json:"executionContinued"`
	Replayed           bool `json:"replayed"`
}
type Delivery struct {
	ColdResume               bool            `json:"coldResume,omitempty"`
	LocalRequestKey          string          `json:"localRequestKey"`
	RoundKey                 string          `json:"roundKey,omitempty"`
	AdapterSessionGeneration int64           `json:"adapterSessionGeneration,string"`
	InteractionID            int64           `json:"interactionId,string"`
	ResponseID               int64           `json:"responseId,string,omitempty"`
	Kind                     string          `json:"kind"`
	Answers                  []domain.Answer `json:"answers,omitempty"`
	Text                     string          `json:"text,omitempty"`
}
type MachineControl interface {
	OpenQuestions(context.Context, Machine) ([]PendingQuestion, error)
	PublishReceipt(context.Context, Machine, PublishReceiptRequest) (Receipt, error)
	PublishEvent(context.Context, Machine, PublishEventRequest) (domain.Item, error)
	Publish(context.Context, Machine, PublishRequest) (Receipt, error)
	Close(context.Context, Machine, CloseRequest) (Receipt, error)
	Pull(context.Context, Machine, int) ([]Delivery, error)
	Ack(context.Context, Machine, AckRequest) (Receipt, error)
}
type CloseRequest struct {
	RoundKey        string `json:"roundKey"`
	LocalRequestKey string `json:"localRequestKey"`
	Reason          string `json:"reason"`
}
type PendingQuestion struct {
	InteractionID   int64     `json:"interactionId,string"`
	LocalRequestKey string    `json:"localRequestKey"`
	RoundKey        string    `json:"roundKey"`
	IsBlocking      bool      `json:"isBlocking"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// PublishReceipt resolves durable delivery identity after worker restart. It is
// an observation/readback, not the delivery ACK and never execution evidence.
type PublishReceiptRequest struct {
	LocalRequestKey string
	RoundKey        string
	ResponseID      int64
	Kind            string
	Status          string
}

// EventRecorder adapts the owning Task event facade. Sequence allocation is in
// tx; SSE publication occurs only after commit. Event contains safe metadata only.
type EventRecorder interface {
	RecordInteractionEvent(context.Context, pgx.Tx, Event) (int64, error)
}
type Event struct {
	TenantID, TaskTurnID               int64
	TaskID, AttemptID, ItemID, Version int64
	Type                               string
	// True only on first QUESTION occurrence creation, never replay/update.
	// Recorder appends the parent-owned attention occurrence in the same tx.
	AttentionRequired bool
}
type TaskSummary struct {
	PendingActionCount     int    `json:"pendingActionCount"`
	HasUnreadUpdates       bool   `json:"hasUnreadUpdates"`
	ActivitySummary        string `json:"activitySummary"`
	FirstPendingActionKind string `json:"firstPendingActionKind,omitempty"`
	FirstPendingActionID   int64  `json:"firstPendingActionId,string,omitempty"`
}

const (
	PendingConversationItem = "CONVERSATION_ITEM"
	PendingCompletionCheck  = "COMPLETION_CHECK"
	PendingDeliveryRequest  = "DELIVERY_REQUEST"
)

// SummaryAuthority performs one tenant-scoped batch authorization and principal
// lookup without per-Task locks. ActiveTaskIDs is the current Task lifecycle
// projection. PrincipalID may be zero when a user has never had a read-state.
type SummaryScope struct {
	PrincipalID            int64
	TaskIDs, ActiveTaskIDs []int64
}
type SummaryAuthority interface {
	AuthorizeSummary(context.Context, pgx.Tx, int64, int64, []int64) (SummaryScope, error)
}
type PublishEventRequest struct {
	LocalRequestKey string `json:"localRequestKey"`
	Kind            string `json:"kind"`
	Text            string `json:"text"`
}
