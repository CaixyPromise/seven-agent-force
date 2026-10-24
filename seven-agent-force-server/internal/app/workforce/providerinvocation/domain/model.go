package domain

import (
	"encoding/json"
	"errors"
	"time"
)

type Operation string

const (
	OperationResponsesCreate     Operation = "responses.create"
	OperationResponsesCompact    Operation = "responses.compact"
	OperationMessagesCreate      Operation = "messages.create"
	OperationMessagesCountTokens Operation = "messages.count_tokens"
)

func (o Operation) Valid() bool {
	return o == OperationResponsesCreate || o == OperationResponsesCompact || o == OperationMessagesCreate || o == OperationMessagesCountTokens
}

type ExecutionKind string

const (
	ExecutionKindAgentAttempt ExecutionKind = "AGENT_ATTEMPT"
	ExecutionKindReviewRun    ExecutionKind = "REVIEW_RUN"
)

func (k ExecutionKind) Normalized() ExecutionKind {
	if k == "" {
		return ExecutionKindAgentAttempt
	}
	return k
}
func (k ExecutionKind) Valid() bool {
	return k.Normalized() == ExecutionKindAgentAttempt || k == ExecutionKindReviewRun
}

const (
	StatusAuthorized     = "AUTHORIZED"
	StatusStreaming      = "STREAMING"
	StatusSucceeded      = "SUCCEEDED"
	StatusFailed         = "FAILED"
	StatusCancelled      = "CANCELLED"
	StatusOutcomeUnknown = "OUTCOME_UNKNOWN"
)

type RequestIdentity struct {
	ExecutionKind            ExecutionKind
	WorkerID                 int64
	InstanceID               string
	HostBootID               string
	TaskID                   int64
	AttemptID                int64
	ReviewRunID              int64
	ReviewRunAttemptID       int64
	Generation               int64
	OwnerEpoch               int64
	StructuredOutputContract string
	LeaseToken               string
	ProtocolInvocationKey    string
	Nonce                    string
	BodySHA256               string
	Timestamp                time.Time
}

func (i RequestIdentity) Validate() error {
	kind := i.ExecutionKind.Normalized()
	if !i.ExecutionKind.Valid() {
		return invalidIdentity("MODEL_EXECUTION_KIND_INVALID", "execution kind is not supported")
	}
	if i.WorkerID < 1 || i.TaskID < 1 || i.OwnerEpoch < 1 || i.InstanceID == "" || i.HostBootID == "" || i.LeaseToken == "" || i.ProtocolInvocationKey == "" || i.Nonce == "" || i.BodySHA256 == "" || i.Timestamp.IsZero() {
		return invalidIdentity("MODEL_INVOCATION_IDENTITY_INVALID", "model invocation identity is incomplete")
	}
	if kind == ExecutionKindAgentAttempt && (i.AttemptID < 1 || i.Generation < 1 || i.ReviewRunID != 0 || i.ReviewRunAttemptID != 0) {
		return invalidIdentity("MODEL_AGENT_ATTEMPT_IDENTITY_MIXED", "Agent Attempt identity is invalid")
	}
	if kind == ExecutionKindReviewRun && (i.ReviewRunID < 1 || i.ReviewRunAttemptID < 1 || i.AttemptID != 0 || i.Generation != 0) {
		return invalidIdentity("MODEL_REVIEW_RUN_IDENTITY_MIXED", "ReviewRun identity is invalid")
	}
	return nil
}

type FrozenModel struct {
	TenantID          int64
	CredentialID      int64
	CredentialVersion int64
	Protocol          string
	PrimaryModel      string
	ExecutionDeadline time.Time
}
type Reservation struct {
	ID                    int64
	ProtocolInvocationKey string
	ExecutionKind         ExecutionKind
	TaskID                int64
	AttemptID             int64
	ReviewRunID           int64
	ReviewRunAttemptID    int64
	OwnerEpoch            int64
	Frozen                FrozenModel
	RequestedModel        string
	StartedAt             time.Time
}

type Usage struct {
	InputTokens              *int64          `json:"inputTokens,omitempty"`
	CachedInputTokens        *int64          `json:"cachedInputTokens,omitempty"`
	CacheCreationInputTokens *int64          `json:"cacheCreationInputTokens,omitempty"`
	OutputTokens             *int64          `json:"outputTokens,omitempty"`
	ReasoningOutputTokens    *int64          `json:"reasoningOutputTokens,omitempty"`
	TotalTokens              *int64          `json:"totalTokens,omitempty"`
	Raw                      json.RawMessage `json:"-"`
}
type Finish struct {
	ID                int64
	Status            string
	StatusCode        *int
	UpstreamRequestID string
	HeadersAt         *time.Time
	FirstByteAt       *time.Time
	FinishedAt        time.Time
	ResponseModel     string
	Usage             Usage
	ErrorCode         string
}

type View struct {
	ID                 int64         `json:"id,string"`
	ExecutionKind      ExecutionKind `json:"executionKind"`
	TaskID             int64         `json:"taskId,string"`
	TaskName           string        `json:"taskName"`
	AttemptID          int64         `json:"attemptId,string,omitempty"`
	ReviewRunID        int64         `json:"reviewRunId,string,omitempty"`
	ReviewRunAttemptID int64         `json:"reviewRunAttemptId,string,omitempty"`
	Generation         int64         `json:"generation,string"`
	OwnerEpoch         int64         `json:"ownerEpoch,string"`
	WorkerID           int64         `json:"workerId,string"`
	WorkerInstanceID   string        `json:"workerInstanceId"`
	HostBootID         string        `json:"hostBootId"`
	ModelCredentialID  int64         `json:"modelCredentialId,string"`
	CredentialVersion  int64         `json:"credentialVersion,string"`
	Protocol           string        `json:"protocol"`
	PrimaryModel       string        `json:"primaryModel"`
	RequestedModel     string        `json:"requestedModel"`
	ResponseModel      string        `json:"responseModel,omitempty"`
	IsPrimaryModel     bool          `json:"isPrimaryModel"`
	Operation          string        `json:"operation"`
	Status             string        `json:"status"`
	StatusCode         *int          `json:"statusCode,omitempty"`
	DurationMillis     int64         `json:"durationMs"`
	FirstByteMillis    *int64        `json:"firstByteMs,omitempty"`
	UpstreamRequestID  string        `json:"upstreamRequestId,omitempty"`
	Usage              Usage         `json:"usage"`
	ErrorCode          string        `json:"errorCode,omitempty"`
	StartedAt          string        `json:"startedAt"`
	FinishedAt         string        `json:"finishedAt,omitempty"`
}
type TaskView struct {
	ID              int64  `json:"id,string"`
	TaskID          int64  `json:"taskId,string"`
	TaskName        string `json:"taskName"`
	AttemptID       int64  `json:"attemptId,string,omitempty"`
	RequestedModel  string `json:"requestedModel"`
	ResponseModel   string `json:"responseModel,omitempty"`
	IsPrimaryModel  bool   `json:"isPrimaryModel"`
	Operation       string `json:"operation"`
	Status          string `json:"status"`
	StatusCode      *int   `json:"statusCode,omitempty"`
	DurationMillis  int64  `json:"durationMs"`
	FirstByteMillis *int64 `json:"firstByteMs,omitempty"`
	Usage           Usage  `json:"usage"`
	ErrorCode       string `json:"errorCode,omitempty"`
	StartedAt       string `json:"startedAt"`
	FinishedAt      string `json:"finishedAt,omitempty"`
}

type AnalyticsSummary struct {
	ActiveCredentialCount int64 `json:"activeCredentialCount"`
	ModelCount            int64 `json:"modelCount"`
	InvocationCount       int64 `json:"invocationCount"`
	TotalTokens           int64 `json:"totalTokens"`
}

type AnalyticsPoint struct {
	Date            string `json:"date"`
	InvocationCount int64  `json:"invocationCount"`
	TotalTokens     int64  `json:"totalTokens"`
}

type ModelDistribution struct {
	Model           string `json:"model"`
	InvocationCount int64  `json:"invocationCount"`
}

type AnalyticsComparison struct {
	InvocationPercent *float64 `json:"invocationPercent"`
	TokenPercent      *float64 `json:"tokenPercent"`
}

type Analytics struct {
	Days              int                 `json:"days"`
	Summary           AnalyticsSummary    `json:"summary"`
	Timeline          []AnalyticsPoint    `json:"timeline"`
	ModelDistribution []ModelDistribution `json:"modelDistribution"`
	CacheHitRate      *float64            `json:"cacheHitRate"`
	P95DurationMillis *int64              `json:"p95DurationMs"`
	Comparison        AnalyticsComparison `json:"comparison"`
}

func (v View) TaskProjection() TaskView {
	return TaskView{ID: v.ID, TaskID: v.TaskID, TaskName: v.TaskName, AttemptID: v.AttemptID, RequestedModel: v.RequestedModel, ResponseModel: v.ResponseModel, IsPrimaryModel: v.IsPrimaryModel, Operation: v.Operation, Status: v.Status, StatusCode: v.StatusCode, DurationMillis: v.DurationMillis, FirstByteMillis: v.FirstByteMillis, Usage: v.Usage, ErrorCode: v.ErrorCode, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt}
}

type Error struct {
	ReasonCode string
	HTTPStatus int
	Message    string
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return "model invocation failed"
	}
	if e.Message != "" {
		return e.Message
	}
	return e.ReasonCode
}
func (e *Error) Unwrap() error { return e.Cause }
func Reason(err error) string {
	var target *Error
	if errors.As(err, &target) && target.ReasonCode != "" {
		return target.ReasonCode
	}
	return "MODEL_INVOCATION_FAILED"
}
func invalidIdentity(reason, message string) error {
	return &Error{ReasonCode: reason, HTTPStatus: 400, Message: message}
}
