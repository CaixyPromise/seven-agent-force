package domain

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	FinalizationMessageKind = "seven.finalization-run/v1"
	UploadMessageKind       = "seven.artifact-upload-attempt/v1"
)

type WorkMessage struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	MessageID     int64  `json:"messageId,string"`
	WorkerID      int64  `json:"workerId,string"`
	ResourceID    int64  `json:"resourceId,string"`
	Generation    int64  `json:"generation,string"`
}

type FinalizationClaim struct {
	RunID           int64           `json:"runId,string"`
	TaskID          int64           `json:"taskId,string"`
	TaskTurnID      int64           `json:"taskTurnId,string"`
	AttemptID       int64           `json:"attemptId,string"`
	ExecutionNodeID int64           `json:"executionNodeId,string"`
	Generation      int64           `json:"generation,string"`
	RunNumber       int             `json:"runNumber"`
	LeaseToken      string          `json:"leaseToken"`
	LeaseExpiresAt  string          `json:"leaseExpiresAt"`
	WorkItem        json.RawMessage `json:"workItem"`
	Attempt         json.RawMessage `json:"attempt"`
	Completion      json.RawMessage `json:"completion"`
	// ResultContract is the final, Server-persisted PASS contract from the
	// immutable Result Processor. The Worker uses it only as a declaration for
	// trusted output materialization; it never trusts Processor self-reported
	// hashes, bytes, object keys or success state.
	EvaluationCycleID int64           `json:"evaluationCycleId,string"`
	ResultContract    json.RawMessage `json:"resultContract"`
	Checks            json.RawMessage `json:"checks"`
	Events            json.RawMessage `json:"events"`
}

type FrozenArtifact struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	SourcePath string `json:"sourcePath,omitempty"`
	MediaType  string `json:"mediaType"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
}

type FreezeResult struct {
	ResultCommit   string           `json:"resultCommit"`
	ResultTree     string           `json:"resultTree"`
	ManifestDigest string           `json:"manifestDigest"`
	SpoolIdentity  string           `json:"spoolIdentity"`
	Artifacts      []FrozenArtifact `json:"artifacts"`
	ReviewEvidence []ReviewEvidence `json:"reviewEvidence,omitempty"`
}

// ReviewEvidence is an immutable, Finalizer-derived ResultCommit index. The
// Server consumes it only to validate canonical ReviewRun evidence; it is not
// Worker-provided review output.
type ReviewEvidence struct {
	Path      string `json:"path"`
	LineCount int    `json:"lineCount"`
}

type UploadClaim struct {
	ID                   int64  `json:"id,string"`
	FinalizationRunID    int64  `json:"finalizationRunId,string"`
	MaterializationRunID int64  `json:"materializationRunId,string,omitempty"`
	TaskID               int64  `json:"taskId,string"`
	TaskTurnID           int64  `json:"taskTurnId,string,omitempty"`
	AttemptID            int64  `json:"attemptId,string"`
	ArtifactType         string `json:"artifactType"`
	Name                 string `json:"name"`
	ObjectKey            string `json:"objectKey"`
	SHA256               string `json:"sha256"`
	SizeBytes            int64  `json:"sizeBytes"`
	MediaType            string `json:"mediaType"`
	AttemptNumber        int    `json:"attemptNumber"`
	LeaseToken           string `json:"leaseToken"`
	LeaseExpiresAt       string `json:"leaseExpiresAt"`
	SpoolIdentity        string `json:"spoolIdentity"`
	SpoolRelativePath    string `json:"spoolRelativePath"`
	SpoolKind            string `json:"spoolKind"`
}

type FinalizationView struct {
	ID                     int64        `json:"id,string"`
	TaskID                 int64        `json:"taskId,string"`
	AttemptID              int64        `json:"attemptId,string"`
	ExecutionNodeID        int64        `json:"executionNodeId,string"`
	Generation             int64        `json:"generation,string"`
	Status                 string       `json:"status"`
	RunNumber              int          `json:"runNumber"`
	MaxRuns                int          `json:"maxRuns"`
	WorkerID               int64        `json:"workerId,string"`
	WorkerInstanceID       string       `json:"workerInstanceId,omitempty"`
	HostBootID             string       `json:"hostBootId"`
	ManifestDigest         string       `json:"manifestDigest,omitempty"`
	SpoolIdentity          string       `json:"spoolIdentity,omitempty"`
	NextRetryAt            string       `json:"nextRetryAt,omitempty"`
	ErrorCode              string       `json:"errorCode,omitempty"`
	CreatedAt              string       `json:"createdAt"`
	StartedAt              string       `json:"startedAt,omitempty"`
	FrozenAt               string       `json:"frozenAt,omitempty"`
	FinishedAt             string       `json:"finishedAt,omitempty"`
	ArtifactUploadAttempts []UploadView `json:"artifactUploadAttempts"`
}

type UploadView struct {
	ID                   int64  `json:"id,string"`
	FinalizationRunID    int64  `json:"finalizationRunId,string"`
	MaterializationRunID int64  `json:"materializationRunId,string,omitempty"`
	TaskID               int64  `json:"taskId,string"`
	AttemptID            int64  `json:"attemptId,string"`
	ArtifactType         string `json:"artifactType"`
	Name                 string `json:"name"`
	ObjectKey            string `json:"objectKey"`
	SHA256               string `json:"sha256"`
	SizeBytes            int64  `json:"sizeBytes"`
	MediaType            string `json:"mediaType"`
	AttemptNumber        int    `json:"attemptNumber"`
	MaxAttempts          int    `json:"maxAttempts"`
	Status               string `json:"status"`
	NextRetryAt          string `json:"nextRetryAt,omitempty"`
	ErrorCode            string `json:"errorCode,omitempty"`
	StartedAt            string `json:"startedAt,omitempty"`
	UploadedAt           string `json:"uploadedAt,omitempty"`
	VerifiedAt           string `json:"verifiedAt,omitempty"`
}

const (
	FinalizationPending   = "PENDING"
	FinalizationRunning   = "RUNNING"
	FinalizationRetryWait = "RETRY_WAIT"
	FinalizationFrozen    = "FROZEN"
	FinalizationFailed    = "FAILED"
	FinalizationCancelled = "CANCELLED"

	UploadPending   = "PENDING"
	UploadUploading = "UPLOADING"
	UploadVerifying = "VERIFYING"
	UploadRetryWait = "RETRY_WAIT"
	UploadVerified  = "VERIFIED"
	UploadFailed    = "FAILED"
	UploadCancelled = "CANCELLED"

	SourceLossAgentRetry   = "AGENT_RETRY"
	SourceLossManualReview = "MANUAL_REVIEW"
)

type FinalizationRun struct {
	Status         string
	RunNumber      int
	MaxRuns        int
	ManifestDigest string
	ErrorCode      string
}

func (r *FinalizationRun) Start() error {
	if r.Status != FinalizationPending && r.Status != FinalizationRetryWait {
		return errors.New("finalization run is not startable")
	}
	if r.RunNumber < 1 || r.MaxRuns < 1 || r.RunNumber > r.MaxRuns {
		return errors.New("finalization retry budget is invalid")
	}
	r.Status = FinalizationRunning
	return nil
}

func (r *FinalizationRun) Retry(reason string) error {
	if r.Status != FinalizationRunning {
		return errors.New("only a running finalization may be retried")
	}
	if r.RunNumber >= r.MaxRuns {
		r.Status = FinalizationFailed
		r.ErrorCode = strings.TrimSpace(reason)
		return errors.New("finalization retry budget exhausted")
	}
	r.RunNumber++
	r.Status = FinalizationRetryWait
	r.ErrorCode = strings.TrimSpace(reason)
	return nil
}

func (r *FinalizationRun) Freeze(manifestDigest string) error {
	if r.Status != FinalizationRunning || len(manifestDigest) != 64 {
		return errors.New("finalization cannot be frozen")
	}
	r.Status = FinalizationFrozen
	r.ManifestDigest = manifestDigest
	r.ErrorCode = ""
	return nil
}

type ArtifactUploadAttempt struct {
	Status        string
	AttemptNumber int
	MaxAttempts   int
	ErrorCode     string
}

func (a *ArtifactUploadAttempt) Start() error {
	if a.Status != UploadPending && a.Status != UploadRetryWait {
		return errors.New("artifact upload is not startable")
	}
	if a.AttemptNumber < 1 || a.MaxAttempts < 1 || a.AttemptNumber > a.MaxAttempts {
		return errors.New("artifact upload retry budget is invalid")
	}
	a.Status = UploadUploading
	return nil
}

func (a *ArtifactUploadAttempt) Retry(reason string) error {
	if a.Status != UploadUploading && a.Status != UploadVerifying {
		return errors.New("artifact upload is not retryable from current state")
	}
	if a.AttemptNumber >= a.MaxAttempts {
		a.Status = UploadFailed
		a.ErrorCode = strings.TrimSpace(reason)
		return errors.New("artifact upload retry budget exhausted")
	}
	a.AttemptNumber++
	a.Status = UploadRetryWait
	a.ErrorCode = strings.TrimSpace(reason)
	return nil
}

func (a *ArtifactUploadAttempt) Verify() error {
	if a.Status != UploadUploading && a.Status != UploadVerifying {
		return errors.New("artifact upload is not verifiable")
	}
	a.Status = UploadVerified
	a.ErrorCode = ""
	return nil
}

func ResolveSourceLoss(frozen bool) string {
	if frozen {
		return SourceLossManualReview
	}
	return SourceLossAgentRetry
}
