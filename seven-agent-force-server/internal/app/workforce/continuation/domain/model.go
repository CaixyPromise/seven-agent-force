package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	DecisionPass         = "PASS"
	DecisionWaitUser     = "WAIT_USER"
	DecisionManualReview = "MANUAL_REVIEW"

	TurnActive       = "ACTIVE"
	TurnSucceeded    = "SUCCEEDED"
	TurnReviewing    = "REVIEWING"
	TurnManualReview = "MANUAL_REVIEW"

	MaterializationDeclaredOutput = "DECLARED_OUTPUT"
	MaterializationZIP            = "ZIP"
	MaterializationTarGZ          = "TAR_GZ"
	MaterializationPatch          = "PATCH"
	MaterializationGitBundle      = "GIT_BUNDLE"
	// MaterializationBundle remains an input alias for historical Operator
	// clients. New records and views use GIT_BUNDLE.
	MaterializationBundle      = "BUNDLE"
	MaterializationMessageKind = "seven.result-materialization-run/v1"
)

var (
	ErrTaskTurnConflict        = errors.New("TASK_TURN_CONFLICT")
	ErrTaskTurnAlreadyActive   = errors.New("TASK_TURN_ALREADY_ACTIVE")
	ErrContinuationNotAllowed  = errors.New("CONTINUATION_NOT_ALLOWED")
	ErrCheckpointUnavailable   = errors.New("CONTINUATION_CHECKPOINT_MISSING")
	ErrCheckpointExpired       = errors.New("CONTINUATION_CHECKPOINT_EXPIRED")
	ErrCheckpointFormat        = errors.New("CONTINUATION_CHECKPOINT_FORMAT_UNSUPPORTED")
	ErrManualApprovalRequired  = errors.New("CONTINUATION_APPROVAL_REQUIRED")
	ErrIdempotencyConflict     = errors.New("IDEMPOTENCY_CONFLICT")
	ErrContinuationInstruction = errors.New("CONTINUATION_INSTRUCTION_INVALID")
	ErrContinuationIdentity    = errors.New("CONTINUATION_IDENTITY_INCOMPATIBLE")
	ErrMaterializationInvalid  = errors.New("RESULT_MATERIALIZATION_INVALID")
	ErrMaterializationExpired  = errors.New("RESULT_MATERIALIZATION_SOURCE_EXPIRED")
)

type Source struct {
	TaskID             int64
	CurrentTurnID      int64
	CurrentTurnStatus  string
	CheckpointID       int64
	CheckpointDecision string
	CheckpointStatus   string
	CheckpointExpires  time.Time
	ConsumedByTurnID   int64
	ApprovedAt         *time.Time
}

func (s Source) ValidateCreate(expectedTurnID int64, now time.Time) error {
	if expectedTurnID <= 0 || expectedTurnID != s.CurrentTurnID {
		return ErrTaskTurnConflict
	}
	if s.CheckpointStatus != "AVAILABLE" {
		return ErrCheckpointUnavailable
	}
	if !s.CheckpointExpires.After(now) {
		return ErrCheckpointExpired
	}
	if s.ConsumedByTurnID != 0 {
		return ErrTaskTurnAlreadyActive
	}
	switch s.CheckpointDecision {
	case DecisionPass:
		if s.CurrentTurnStatus != TurnSucceeded {
			return ErrContinuationNotAllowed
		}
	case DecisionWaitUser:
		if s.CurrentTurnStatus != TurnReviewing {
			return ErrContinuationNotAllowed
		}
	case DecisionManualReview:
		if s.CurrentTurnStatus != TurnManualReview {
			return ErrContinuationNotAllowed
		}
		if s.ApprovedAt == nil {
			return ErrManualApprovalRequired
		}
	default:
		return ErrContinuationNotAllowed
	}
	return nil
}

func ValidateInstruction(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 16*1024 {
		return ErrContinuationInstruction
	}
	return nil
}

type Turn struct {
	ID                      int64  `json:"id,string"`
	TaskID                  int64  `json:"taskId,string"`
	TenantID                int64  `json:"tenantId,string"`
	TurnNumber              int    `json:"turnNumber"`
	Instruction             string `json:"instruction"`
	Status                  string `json:"status"`
	SourceTurnID            int64  `json:"sourceTurnId,string,omitempty"`
	SourceCheckpointID      int64  `json:"sourceCheckpointId,string,omitempty"`
	ExecutionSnapshotDigest string `json:"executionSnapshotDigest"`
	ContinuationLineageID   int64  `json:"continuationLineageId,string"`
	CreatedBy               int64  `json:"createdBy,string"`
	ResultCommit            string `json:"resultCommit,omitempty"`
	ResultTree              string `json:"resultTree,omitempty"`
	ResultSummary           string `json:"resultSummary,omitempty"`
	CreatedAt               string `json:"createdAt"`
	StartedAt               string `json:"startedAt,omitempty"`
	FinishedAt              string `json:"finishedAt,omitempty"`
}

type Checkpoint struct {
	ID                 int64  `json:"id,string"`
	TaskID             int64  `json:"taskId,string"`
	TaskTurnID         int64  `json:"taskTurnId,string"`
	AttemptID          int64  `json:"attemptId,string"`
	Decision           string `json:"decision"`
	Status             string `json:"status"`
	CheckpointIdentity string `json:"checkpointIdentity"`
	ManifestSHA256     string `json:"manifestSha256"`
	ExpiresAt          string `json:"expiresAt"`
	ApprovedBy         int64  `json:"approvedBy,string,omitempty"`
	ApprovedAt         string `json:"approvedAt,omitempty"`
	ApprovalReason     string `json:"approvalReason,omitempty"`
	ConsumedByTurnID   int64  `json:"consumedByTurnId,string,omitempty"`
	ConsumedAt         string `json:"consumedAt,omitempty"`
}

type RestoreContract struct {
	FormatVersion                 int    `json:"formatVersion"`
	RestoreRunID                  int64  `json:"restoreRunId,string"`
	CheckpointID                  int64  `json:"checkpointId,string"`
	CheckpointIdentity            string `json:"checkpointIdentity"`
	SourceTurnID                  int64  `json:"sourceTurnId,string"`
	TargetTurnID                  int64  `json:"targetTurnId,string"`
	WorkspaceObjectKey            string `json:"workspaceObjectKey"`
	WorkspaceContentSHA256        string `json:"workspaceContentSha256"`
	WorkspaceObjectSHA256         string `json:"workspaceObjectSha256"`
	WorkspaceSizeBytes            int64  `json:"workspaceSizeBytes"`
	RepositoryBundleObjectKey     string `json:"repositoryBundleObjectKey"`
	RepositoryBundleContentSHA256 string `json:"repositoryBundleContentSha256"`
	RepositoryBundleObjectSHA256  string `json:"repositoryBundleObjectSha256"`
	RepositoryBundleSizeBytes     int64  `json:"repositoryBundleSizeBytes"`
	WorkspaceOverlayObjectKey     string `json:"workspaceOverlayObjectKey"`
	WorkspaceOverlayContentSHA256 string `json:"workspaceOverlayContentSha256"`
	WorkspaceOverlayObjectSHA256  string `json:"workspaceOverlayObjectSha256"`
	WorkspaceOverlaySizeBytes     int64  `json:"workspaceOverlaySizeBytes"`
	SelectionDigest               string `json:"selectionDigest"`
	SessionObjectKey              string `json:"sessionObjectKey"`
	SessionContentSHA256          string `json:"sessionContentSha256"`
	SessionObjectSHA256           string `json:"sessionObjectSha256"`
	SessionSizeBytes              int64  `json:"sessionSizeBytes"`
	ManifestObjectKey             string `json:"manifestObjectKey"`
	ManifestContentSHA256         string `json:"manifestContentSha256"`
	ManifestObjectSHA256          string `json:"manifestObjectSha256"`
	ManifestSizeBytes             int64  `json:"manifestSizeBytes"`
	TaskExecutionSnapshotDigest   string `json:"taskExecutionSnapshotDigest"`
	ExpectedBaseCommit            string `json:"expectedBaseCommit"`
	ExpectedResultCommit          string `json:"expectedResultCommit"`
	ExpectedResultTree            string `json:"expectedResultTree"`
	SessionLineageDigest          string `json:"sessionLineageDigest"`
	EncryptionAlgorithm           string `json:"encryptionAlgorithm"`
	EncryptionKeyID               string `json:"encryptionKeyId"`
	WrappedDataKey                string `json:"wrappedDataKey"`
}

type MaterializationView struct {
	ID                 int64  `json:"id,string"`
	TaskID             int64  `json:"taskId,string"`
	TaskTurnID         int64  `json:"taskTurnId,string"`
	SourceCheckpointID int64  `json:"sourceCheckpointId,string"`
	OutputID           int64  `json:"outputId,string,omitempty"`
	Kind               string `json:"kind"`
	Status             string `json:"status"`
	RunNumber          int    `json:"runNumber"`
	MaxRuns            int    `json:"maxRuns"`
	ArtifactID         int64  `json:"artifactId,string,omitempty"`
	ArtifactSHA256     string `json:"artifactSha256,omitempty"`
	ArtifactSizeBytes  int64  `json:"artifactSizeBytes,omitempty"`
	ReasonCode         string `json:"reasonCode,omitempty"`
	CreatedAt          string `json:"createdAt"`
	StartedAt          string `json:"startedAt,omitempty"`
	FinishedAt         string `json:"finishedAt,omitempty"`
	NextRetryAt        string `json:"nextRetryAt,omitempty"`
}

type MaterializationMessage struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	MessageID     int64  `json:"messageId,string"`
	WorkerID      int64  `json:"workerId,string"`
	ResourceID    int64  `json:"resourceId,string"`
	Generation    int64  `json:"generation,string"`
}

type MaterializationClaim struct {
	RunID              int64           `json:"runId,string"`
	TaskID             int64           `json:"taskId,string"`
	TaskTurnID         int64           `json:"taskTurnId,string"`
	AttemptID          int64           `json:"attemptId,string"`
	Generation         int64           `json:"generation,string"`
	RunNumber          int             `json:"runNumber"`
	Kind               string          `json:"kind"`
	OutputID           int64           `json:"outputId,string,omitempty"`
	OutputName         string          `json:"outputName,omitempty"`
	OutputSourcePath   string          `json:"outputSourcePath,omitempty"`
	OutputKind         string          `json:"outputKind,omitempty"`
	ArchiveFormat      string          `json:"archiveFormat,omitempty"`
	SelectionSpec      string          `json:"selectionSpec,omitempty"`
	ExpectedBaseCommit string          `json:"expectedBaseCommit,omitempty"`
	ExpectedCommit     string          `json:"expectedResultCommit,omitempty"`
	ExpectedTree       string          `json:"expectedResultTree,omitempty"`
	Restore            RestoreContract `json:"restore"`
	Key                string          `json:"plainKeyBase64"`
	LeaseToken         string          `json:"leaseToken"`
	LeaseExpiresAt     string          `json:"leaseExpiresAt"`
}

type MaterializationFreeze struct {
	Name                    string          `json:"name"`
	MediaType               string          `json:"mediaType"`
	Path                    string          `json:"path"`
	SHA256                  string          `json:"sha256"`
	SizeBytes               int64           `json:"sizeBytes"`
	SpoolIdentity           string          `json:"spoolIdentity"`
	SelectionManifest       json.RawMessage `json:"selectionManifest,omitempty"`
	SelectionManifestDigest string          `json:"selectionManifestDigest,omitempty"`
	SelectedFileCount       int             `json:"selectedFileCount,omitempty"`
}
