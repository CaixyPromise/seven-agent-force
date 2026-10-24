// Package facade exposes the narrow Adapter-to-Delivery intent boundary.
package facade

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

const (
	GitDeliveryToolName = "request_git_delivery"
	IntentScheduled     = "SCHEDULED"
)

type Machine struct {
	TenantID, TaskID, TaskTurnID                                int64
	WorkerID                                                    int64
	InstanceID, HostBootID                                      string
	AttemptID, Generation, OwnerEpoch, AdapterSessionGeneration int64
	ExecutionLease                                              string
}

type Target struct {
	TenantID, TaskID, TaskTurnID, AttemptID, AdapterSessionGeneration int64
	WorkerID, OwnerEpoch                                              int64
	TaskTerminal, TaskCancelled                                       bool
	Capabilities                                                      []string
}

type MachineAuthority interface {
	ValidateDeliveryTool(context.Context, pgx.Tx, Machine) (Target, error)
}

type PublishIntentRequest struct {
	RoundKey, LocalToolCallKey, NativeToolCallID string
	ArgumentsDigest                              string
	Arguments                                    json.RawMessage
}

type AckIntentRequest struct {
	IntentID int64
}

type CloseIntentRequest struct {
	RoundKey, LocalToolCallKey, Reason string
}

type IntentReceipt struct {
	IntentID              int64  `json:"intentId,string"`
	Version               int64  `json:"version,string"`
	Status                string `json:"status"`
	DeliveryMode          string `json:"deliveryMode"`
	DeliveryAuthorization string `json:"deliveryAuthorization"`
	Replayed              bool   `json:"replayed"`
}

type ImmediateToolResult struct {
	RoundKey, LocalToolCallKey, NativeToolCallID string
	IntentID                                     int64
	Status, Summary, ResultDigest, ReasonCode    string
}

type MachineToolService interface {
	PublishIntent(context.Context, Machine, PublishIntentRequest) (IntentReceipt, ImmediateToolResult, error)
	AckIntent(context.Context, Machine, AckIntentRequest) (IntentReceipt, error)
	CloseIntent(context.Context, Machine, CloseIntentRequest) (IntentReceipt, error)
}
