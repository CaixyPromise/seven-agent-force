// Package facade exposes ReviewRun's intentionally small cross-module
// contracts. Artifact and Task code depend only on this package, never on the
// ReviewRun persistence implementation.
package facade

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

var (
	ErrNotFound         = domain.ErrNotFound
	ErrIdentityConflict = domain.ErrIdentityConflict
	ErrFenceRejected    = domain.ErrFenceRejected
	ErrInvalidReport    = domain.ErrInvalidReport
)

type FinalizationInput struct {
	TenantID          int64
	TaskID            int64
	TaskTurnID        int64
	AttemptID         int64
	FinalizationRunID int64
	ResultCommit      string
	ResultTree        string
}

type FinalizationDecision struct {
	ReviewRequired bool
	ReviewRunID    int64
	Status         string
}

// FinalizationGate runs inside the trusted Artifact Finalizer transaction,
// after result recomputation and all object-store readback verification.
type FinalizationGate interface {
	GateVerifiedFinalization(context.Context, pgx.Tx, FinalizationInput) (FinalizationDecision, error)
}

type TaskLifecycle interface {
	CancelForTask(context.Context, pgx.Tx, int64, string) error
}

type TaskSummaryProvider interface {
	SummaryForTask(context.Context, int64, int64) (domain.Summary, error)
}

// TaskReportProvider exposes only the canonical, task-authorized business
// evidence. It is intentionally distinct from MachineControl and does not
// permit a task user to discover execution/control-plane facts.
type TaskReportProvider interface {
	ReportForTask(context.Context, int64, int64) (domain.TaskReport, error)
}

type TaskReviewReader interface {
	TaskReportProvider
	ListRunsForTask(context.Context, int64, int64, int64, int) (domain.RunPage, error)
	RunDetailForTask(context.Context, int64, int64) (domain.RunView, error)
}

type RetentionDecision struct {
	ActiveReferences       bool
	PortableStateAvailable bool
	ReasonCode             string
}

type WorkspaceRetention interface {
	ReviewRunRetention(context.Context, int64) (RetentionDecision, error)
}

type MachineEnvelope struct {
	SchemaVersion int
	WorkerID      int64
	InstanceID    string
	HostBootID    string
	RequestID     string
	Generation    int64
	OwnerEpoch    int64
}

type Claim = domain.Claim
type LaunchPermit = domain.LaunchPermit
type UsageFacts = domain.UsageFacts
type TaskReport = domain.TaskReport

// CanonicalizeCorrection is the stable cross-module verifier used before an
// Agent correction report is written into its read-only input mount.
func CanonicalizeCorrection(raw []byte) ([]byte, string, error) {
	report, digest, err := domain.CanonicalizeReport(raw)
	if err != nil {
		return nil, "", err
	}
	canonical, err := json.Marshal(report)
	if err != nil {
		return nil, "", domain.ErrInvalidReport
	}
	return canonical, digest, nil
}

// ContainerRegistration is durable recovery evidence. It deliberately stores
// no Docker socket address, lease token, permit token or provider authority.
type ContainerRegistration struct {
	ContainerID             string
	ContainerName           string
	ImageDigest             string
	ExecutionIdentityDigest string
}

// ReportSubmission is a bounded candidate document. The Server owns
// canonicalization and never persists an invalid raw report.
type ReportSubmission struct {
	LeaseToken  string
	PermitID    int64
	PermitToken string
	Report      []byte
	Usage       UsageFacts
}

type ReportResult struct {
	Status       string
	Verdict      string
	ReportDigest string
}

type MachineControl interface {
	Claim(context.Context, MachineEnvelope, int64) (Claim, error)
	Recover(context.Context, MachineEnvelope, int64, ContainerRegistration) (Claim, error)
	RenewLease(context.Context, MachineEnvelope, int64, string) (string, error)
	AcquireLaunchPermit(context.Context, MachineEnvelope, int64, string) (LaunchPermit, error)
	RecordContainer(context.Context, MachineEnvelope, int64, ContainerRegistration, string, int64, string) error
	SubmitReport(context.Context, MachineEnvelope, int64, ReportSubmission) (ReportResult, error)
	RecordTransientFailure(context.Context, MachineEnvelope, int64, string, string) error
}

type Scheduler interface {
	AssignAvailable(context.Context, int) (int, error)
	ReconcileExpired(context.Context, int) (int, error)
}

type HumanDecisionRequest struct {
	TenantID         int64
	ReviewRunID      int64
	ActorUserID      int64
	Action           string
	Reason           string
	RevisionGuidance string
	IdempotencyKey   string
	Proof            stepup.ProofMetadata
}

type HumanDecisionResult struct {
	ID                  int64  `json:"id,string"`
	TaskID              int64  `json:"taskId,string"`
	TaskTurnID          int64  `json:"taskTurnId,string"`
	ReviewRunID         int64  `json:"reviewRunId,string"`
	Action              string `json:"action"`
	AIVerdict           string `json:"aiVerdict,omitempty"`
	ReportDigest        string `json:"reportDigest,omitempty"`
	QualityOutcome      string `json:"qualityOutcome,omitempty"`
	ResultingTaskTurnID int64  `json:"resultingTaskTurnId,string"`
	DecidedAt           string `json:"decidedAt"`
	RevisionGuidance    string `json:"revisionGuidance,omitempty"`
}

type HumanClosure interface {
	Decide(context.Context, HumanDecisionRequest) (HumanDecisionResult, bool, error)
}

func IsConflict(err error) bool {
	return errors.Is(err, domain.ErrIdentityConflict) || errors.Is(err, domain.ErrFenceRejected) || errors.Is(err, domain.ErrInvalidTransition)
}

func FenceStage(err error) string { return string(domain.FenceViolationStage(err)) }
