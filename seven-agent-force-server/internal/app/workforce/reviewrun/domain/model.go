// Package domain owns the durable state machine for an independent review of
// one trusted ResultCommit/ResultTree.  A ReviewRun is deliberately separate
// from Agent attempts, Result Processor evaluation, and Delivery review.
package domain

import (
	"encoding/json"
	"errors"
)

const (
	StatusPending          = "PENDING"
	StatusAssigned         = "ASSIGNED"
	StatusRunning          = "RUNNING"
	StatusValidatingReport = "VALIDATING_REPORT"
	StatusPassed           = "PASSED"
	StatusRevisionRequired = "REVISION_REQUIRED"
	StatusRejected         = "REJECTED"
	StatusRetryWait        = "RETRY_WAIT"
	StatusManualReview     = "MANUAL_REVIEW"
	StatusCancelled        = "CANCELLED"
)

var (
	ErrNotFound           = errors.New("REVIEW_RUN_NOT_FOUND")
	ErrIdentityConflict   = errors.New("REVIEW_RUN_IDENTITY_CONFLICT")
	ErrFenceRejected      = errors.New("REVIEW_RUN_FENCE_REJECTED")
	ErrInvalidTransition  = errors.New("REVIEW_RUN_INVALID_TRANSITION")
	ErrInvalidReviewInput = errors.New("REVIEW_RUN_INVALID_INPUT")
)

// FenceStage identifies which coarse, non-secret authority class rejected a
// machine mutation. It is intentionally narrower than the underlying
// predicate: callers may distinguish lease, permit, container and durable
// state failures without learning token values or comparison details.
type FenceStage string

const (
	FenceStageLease        FenceStage = "LEASE"
	FenceStagePermit       FenceStage = "PERMIT"
	FenceStageContainer    FenceStage = "CONTAINER"
	FenceStageRunState     FenceStage = "RUN_STATE"
	FenceStageAttemptState FenceStage = "ATTEMPT_STATE"
)

type FenceViolation struct{ Stage FenceStage }

func (e FenceViolation) Error() string { return ErrFenceRejected.Error() }
func (e FenceViolation) Unwrap() error { return ErrFenceRejected }

func NewFenceViolation(stage FenceStage) error { return FenceViolation{Stage: stage} }

func FenceViolationStage(err error) FenceStage {
	var violation FenceViolation
	if errors.As(err, &violation) {
		return violation.Stage
	}
	return ""
}

// IsActive fails closed: an unknown persisted state is treated as active by
// callers that make retention or capacity decisions.
func IsActive(status string) bool {
	switch status {
	case StatusPassed, StatusRevisionRequired, StatusRejected, StatusManualReview, StatusCancelled:
		return false
	default:
		return true
	}
}

func IsClaimable(status string) bool { return status == StatusAssigned }

func IsTerminal(status string) bool {
	switch status {
	case StatusPassed, StatusRevisionRequired, StatusRejected, StatusManualReview, StatusCancelled:
		return true
	default:
		return false
	}
}

type Summary struct {
	Required bool   `json:"required"`
	Status   string `json:"status,omitempty"`
}

// TaskReport is the task-authorized projection of a canonical review report.
// It is deliberately made of business evidence only: no worker identity,
// lease, permit, container, Provider identity, raw candidate report, or
// diagnostic payload can cross this boundary.
type TaskReport struct {
	Required                 bool         `json:"required"`
	Status                   string       `json:"status,omitempty"`
	Verdict                  string       `json:"verdict,omitempty"`
	Score                    *int         `json:"score,omitempty"`
	Summary                  string       `json:"summary,omitempty"`
	Findings                 []Finding    `json:"findings,omitempty"`
	ReviewTests              []ReviewTest `json:"reviewTests,omitempty"`
	Limitations              []Limitation `json:"limitations,omitempty"`
	ReportDigest             string       `json:"reportDigest,omitempty"`
	QualityOutcome           string       `json:"qualityOutcome,omitempty"`
	AutomaticRevisionCount   int          `json:"automaticRevisionCount"`
	AutomaticRevisionLimit   int          `json:"automaticRevisionLimit"`
	NoProgressStreak         int          `json:"noProgressStreak"`
	StopReason               string       `json:"stopReason,omitempty"`
	AvailableActions         []string     `json:"availableActions"`
	RevisionGuidanceRequired bool         `json:"revisionGuidanceRequired"`
}

type HumanDecisionSummary struct {
	ID                  int64  `json:"id,string"`
	Action              string `json:"action"`
	Reason              string `json:"reason"`
	RevisionGuidance    string `json:"revisionGuidance,omitempty"`
	AIVerdict           string `json:"aiVerdict,omitempty"`
	ReportDigest        string `json:"reportDigest,omitempty"`
	QualityOutcome      string `json:"qualityOutcome,omitempty"`
	ResultingTaskTurnID int64  `json:"resultingTaskTurnId,string"`
	DecidedAt           string `json:"decidedAt"`
}

type RunView struct {
	CompletionCheckDigest   string                  `json:"completionCheckDigest,omitempty"`
	CompletionCheckSummary  *CompletionCheckSummary `json:"completionCheckSummary,omitempty"`
	ID                      int64                   `json:"id,string"`
	TaskID                  int64                   `json:"taskId,string"`
	TaskTurnID              int64                   `json:"taskTurnId,string"`
	Status                  string                  `json:"status"`
	Verdict                 string                  `json:"verdict,omitempty"`
	QualityOutcome          string                  `json:"qualityOutcome,omitempty"`
	ResultTree              string                  `json:"resultTree"`
	Report                  *TaskReport             `json:"report,omitempty"`
	AutomaticRevisionNumber int                     `json:"automaticRevisionNumber"`
	NoProgressStreak        int                     `json:"noProgressStreak"`
	StopReason              string                  `json:"stopReason,omitempty"`
	RevisionKind            string                  `json:"revisionKind,omitempty"`
	SuccessorTaskTurnID     int64                   `json:"successorTaskTurnId,string"`
	HumanDecision           *HumanDecisionSummary   `json:"humanDecision,omitempty"`
	CreatedAt               string                  `json:"createdAt"`
	FinishedAt              string                  `json:"finishedAt,omitempty"`
}

// CompletionCheckSummary is task-authorized historical evidence. It contains
// only declarative review criteria, never executable content or machine facts.
type CompletionCheckSummary struct {
	Instructions string `json:"instructions,omitempty"`
	ReviewPrompt string `json:"reviewPrompt,omitempty"`
}

type RunPage struct {
	Items      []RunView `json:"items"`
	NextCursor int64     `json:"nextCursor,string"`
}

// RunIdentity is the immutable, replay-safe identity defined by the P1.0
// contract. It contains no lease, Worker, credential, or Provider secret.
type RunIdentity struct {
	CompletionCheckDigest  string
	TenantID               int64
	TaskID                 int64
	TaskTurnID             int64
	FinalizationRunID      int64
	AttemptID              int64
	ResultCommit           string
	ResultTree             string
	HardTimeoutSeconds     int
	TransientRetryLimit    int
	AutomaticRevisionLimit int
}

// ResolvedSelection is the immutable infrastructure identity chosen when a
// pending ReviewRun is assigned. It is copied into both the ReviewRun and its
// concrete attempt before an assignment message is published.
type ResolvedSelection struct {
	RuntimeReleaseID    int64
	ReviewerImageDigest string
	AdapterIdentity     string
	AdapterVersion      string
	AdapterProtocol     string
	AdapterCapabilities json.RawMessage
	ModelCredentialID   int64
	CredentialVersion   int64
	ModelProtocol       string
	PrimaryModel        string
	ProviderProxyImage  string
	WorkerID            int64
	WorkerInstanceID    string
	SelectionDigest     string
}

type AssignmentMessage struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	MessageID     int64  `json:"messageId,string"`
	ReviewRunID   int64  `json:"reviewRunId,string"`
	WorkerID      int64  `json:"workerId,string"`
	OwnerEpoch    int64  `json:"ownerEpoch,string"`
	ExpiresAt     string `json:"expiresAt"`
}

const AssignmentMessageKind = "seven.review-run-assignment/v1"

type Claim struct {
	CompletionCheckDigest      string          `json:"completionCheckDigest,omitempty"`
	ReviewRunID                int64           `json:"reviewRunId,string"`
	ReviewRunAttemptID         int64           `json:"reviewRunAttemptId,string"`
	TenantID                   int64           `json:"tenantId,string"`
	TaskID                     int64           `json:"taskId,string"`
	TaskTurnID                 int64           `json:"taskTurnId,string"`
	FinalizationRunID          int64           `json:"finalizationRunId,string"`
	ResultCommit               string          `json:"resultCommit"`
	ResultTree                 string          `json:"resultTree"`
	ReviewerImage              string          `json:"reviewerImageDigest"`
	ModelCredentialID          int64           `json:"modelCredentialId,string"`
	CredentialVersion          int64           `json:"credentialVersion,string"`
	ModelProtocol              string          `json:"modelProtocol"`
	PrimaryModel               string          `json:"primaryModel"`
	ProviderStructuredContract string          `json:"providerStructuredOutputContract"`
	ProviderProxyImageDigest   string          `json:"providerProxyImageDigest,omitempty"`
	SystemContract             string          `json:"systemContract"`
	ReportSchemaVersion        string          `json:"reportSchemaVersion"`
	HardTimeoutSeconds         int             `json:"hardTimeoutSeconds"`
	ReviewContract             json.RawMessage `json:"reviewContract"`
	Requirements               json.RawMessage `json:"requirements"`
	OwnerEpoch                 int64           `json:"ownerEpoch,string"`
	LeaseToken                 string          `json:"reviewLeaseToken"`
	LeaseExpiresAt             string          `json:"reviewLeaseExpiresAt"`
	ExecutionDeadlineAt        string          `json:"executionDeadlineAt"`
}

type LaunchPermit struct {
	PermitID    int64  `json:"permitId,string"`
	PermitToken string `json:"permitToken"`
	ExpiresAt   string `json:"expiresAt"`
	OwnerEpoch  int64  `json:"ownerEpoch,string"`
}
