package facade

// Runtime contracts deliberately use only bounded, serializable values. The
// Supervisor and Worker receive these values through the Machine API, while
// authoring records and mutable defaults stay inside the Result Processor
// module.

import (
	"encoding/json"

	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

// ResultContract is the stable contract accepted by cross-module gates.
type ResultContract = processor.ResultContract

// MachineIdentity is the fenced Worker identity attached to every processor
// materialization and evaluation write. It mirrors the existing Machine API
// envelope without importing the Task Dispatcher internals.
type MachineIdentity struct {
	WorkerID   int64  `json:"workerId,string"`
	InstanceID string `json:"instanceId"`
	HostBootID string `json:"hostBootId"`
	Generation int64  `json:"generation,string"`
	OwnerEpoch int64  `json:"ownerEpoch,string"`
}

// Materialization is a one-cycle, in-memory-only processor environment. The
// Values may contain sensitive CONFIG/SECRET material and must never be
// persisted, logged, put in an Attempt spec, or shown by the Web API.
type Materialization struct {
	CycleID       int64             `json:"cycleId,string"`
	Variables     map[string]string `json:"variables"`
	ExpiresAt     string            `json:"expiresAt"`
	RedactionHint []string          `json:"-"`
}

// EvaluationCandidate is produced by the protected bootstrap after a Harness
// round has ended. Contract is the original canonical ResultContract JSON;
// the Server independently validates and persists it before allowing the
// Supervisor to resume or propose completion.
type EvaluationCandidate struct {
	CycleID             int64           `json:"cycleId,string"`
	CycleNumber         int             `json:"cycleNumber"`
	SessionDigest       string          `json:"sessionDigest,omitempty"`
	CandidateTreeDigest string          `json:"candidateTreeDigest,omitempty"`
	FindingsDigest      string          `json:"findingsDigest,omitempty"`
	Contract            json.RawMessage `json:"contract"`
	ContractDigest      string          `json:"contractDigest"`
	ProcessorReleaseID  int64           `json:"processorReleaseId,string"`
	ProcessorDigest     string          `json:"processorDigest"`
	EvaluationStartedAt string          `json:"evaluationStartedAt,omitempty"`
}

const (
	DirectiveResumeSameSession = "RESUME_SAME_SESSION"
	DirectiveProposeCompletion = "PROPOSE_COMPLETION"
	DirectiveWaitForUser       = "WAIT_FOR_USER"
	DirectiveStopRejected      = "STOP_REJECTED"
	DirectiveStopManualReview  = "STOP_MANUAL_REVIEW"
	DirectiveAckReplay         = "ACK_REPLAY"
)

// EvaluationDirective is the only Server-authoritative continuation command
// understood by the Supervisor after an EvaluationCycle is durably recorded.
// ResumeNonce is intentionally excluded from ordinary persistence; the
// Machine idempotency middleware keeps an encrypted replay response for the
// short period in which a transport retry might need it.
type EvaluationDirective struct {
	Contract        json.RawMessage `json:"contract"`
	ContractDigest  string          `json:"contractDigest"`
	Directive       string          `json:"directive"`
	CycleID         int64           `json:"cycleId,string"`
	Decision        string          `json:"decision"`
	ReasonCode      string          `json:"reasonCode"`
	Message         string          `json:"message,omitempty"`
	ManagedFeedback string          `json:"managedFeedback,omitempty"`
	ResumeNonce     string          `json:"resumeNonce,omitempty"`
	RemainingCycles int             `json:"remainingCycles,omitempty"`
	CheckpointID    int64           `json:"checkpointId,string,omitempty"`
}

// EvaluationView is safe for the task detail/audit API. It intentionally
// excludes secret values, raw session IDs, and any unredacted Processor logs.
type EvaluationView struct {
	ID                  int64           `json:"id,string"`
	TaskID              int64           `json:"taskId,string"`
	TaskTurnID          int64           `json:"taskTurnId,string"`
	ExecutionNodeID     int64           `json:"executionNodeId,string"`
	AttemptID           int64           `json:"attemptId,string"`
	CycleNumber         int             `json:"cycleNumber"`
	SessionDigest       string          `json:"sessionDigest,omitempty"`
	CandidateTreeDigest string          `json:"candidateTreeDigest,omitempty"`
	FindingsDigest      string          `json:"findingsDigest,omitempty"`
	ContractDigest      string          `json:"contractDigest"`
	Decision            string          `json:"decision"`
	ReasonCode          string          `json:"reasonCode"`
	Message             string          `json:"message,omitempty"`
	Contract            json.RawMessage `json:"contract"`
	Status              string          `json:"status"`
	StartedAt           string          `json:"startedAt"`
	PersistedAt         string          `json:"persistedAt,omitempty"`
	CompletedAt         string          `json:"completedAt,omitempty"`
}

// ContinuationCheckpointCandidate is the metadata for three Worker-produced,
// byte-for-byte verified checkpoint objects.  The Server never accepts file
// paths from the Worker: object keys are deterministic from the frozen Task
// identity and all digests are verified again on every later inspection.
type ContinuationCheckpointCandidate struct {
	FormatVersion                 int    `json:"formatVersion"`
	CheckpointIdentity            string `json:"checkpointIdentity"`
	EvaluationCycleID             int64  `json:"evaluationCycleId,string"`
	Decision                      string `json:"decision"`
	WorkspaceObjectKey            string `json:"workspaceObjectKey"`
	WorkspaceSHA256               string `json:"workspaceSha256"`
	WorkspaceContentSHA256        string `json:"workspaceContentSha256"`
	WorkspaceObjectSHA256         string `json:"workspaceObjectSha256"`
	WorkspaceSizeBytes            int64  `json:"workspaceSizeBytes"`
	RepositoryBundleObjectKey     string `json:"repositoryBundleObjectKey,omitempty"`
	RepositoryBundleContentSHA256 string `json:"repositoryBundleContentSha256,omitempty"`
	RepositoryBundleObjectSHA256  string `json:"repositoryBundleObjectSha256,omitempty"`
	RepositoryBundleSizeBytes     int64  `json:"repositoryBundleSizeBytes,omitempty"`
	WorkspaceOverlayObjectKey     string `json:"workspaceOverlayObjectKey,omitempty"`
	WorkspaceOverlayContentSHA256 string `json:"workspaceOverlayContentSha256,omitempty"`
	WorkspaceOverlayObjectSHA256  string `json:"workspaceOverlayObjectSha256,omitempty"`
	WorkspaceOverlaySizeBytes     int64  `json:"workspaceOverlaySizeBytes,omitempty"`
	SelectionDigest               string `json:"selectionDigest,omitempty"`
	GitignoreDigest               string `json:"gitignoreDigest,omitempty"`
	SelectionPolicyDigest         string `json:"selectionPolicyDigest,omitempty"`
	SessionObjectKey              string `json:"sessionObjectKey"`
	SessionSHA256                 string `json:"sessionSha256"`
	SessionContentSHA256          string `json:"sessionContentSha256"`
	SessionObjectSHA256           string `json:"sessionObjectSha256"`
	SessionSizeBytes              int64  `json:"sessionSizeBytes"`
	ManifestObjectKey             string `json:"manifestObjectKey"`
	ManifestSHA256                string `json:"manifestSha256"`
	ManifestContentSHA256         string `json:"manifestContentSha256"`
	ManifestObjectSHA256          string `json:"manifestObjectSha256"`
	ManifestSizeBytes             int64  `json:"manifestSizeBytes"`
	TaskExecutionSnapshotDigest   string `json:"taskExecutionSnapshotDigest"`
	BaseCommit                    string `json:"baseCommit,omitempty"`
	ResultCommit                  string `json:"resultCommit,omitempty"`
	ResultTree                    string `json:"resultTree,omitempty"`
	SessionLineageDigest          string `json:"sessionLineageDigest"`
	EncryptionAlgorithm           string `json:"encryptionAlgorithm"`
	EncryptionKeyID               string `json:"encryptionKeyId"`
	WrappedDataKey                string `json:"wrappedDataKey"`
	ExpiresAt                     string `json:"expiresAt"`
}

// ContinuationCheckpointView is safe for operator/task audit views. It does
// not expose object bytes, raw Session IDs, or any materialized secret value.
type ContinuationCheckpointView struct {
	ID                 int64  `json:"id,string"`
	TaskID             int64  `json:"taskId,string"`
	TaskTurnID         int64  `json:"taskTurnId,string"`
	AttemptID          int64  `json:"attemptId,string"`
	CheckpointIdentity string `json:"checkpointIdentity"`
	EvaluationCycleID  int64  `json:"evaluationCycleId,string"`
	Decision           string `json:"decision"`
	ManifestSHA256     string `json:"manifestSha256"`
	Status             string `json:"status"`
	ExpiresAt          string `json:"expiresAt"`
	CreatedAt          string `json:"createdAt"`
	AvailableAt        string `json:"availableAt,omitempty"`
}

type ResultOutputView struct {
	ID                int64  `json:"id,string"`
	TaskID            int64  `json:"taskId,string"`
	TaskTurnID        int64  `json:"taskTurnId,string"`
	EvaluationCycleID int64  `json:"evaluationCycleId,string"`
	Name              string `json:"name"`
	OutputKind        string `json:"outputKind"`
	ArchiveFormat     string `json:"archiveFormat,omitempty"`
	Storage           string `json:"storage"`
	Status            string `json:"status"`
	ArtifactID        int64  `json:"artifactId,string,omitempty"`
	SHA256            string `json:"sha256,omitempty"`
	SizeBytes         int64  `json:"sizeBytes,omitempty"`
	CreatedAt         string `json:"createdAt"`
}

// FixtureExecution is the bounded, exact-image test contract returned only to
// the assigned Worker.  Draft files are included because a Fixture validates
// an unpublished Revision and therefore cannot rely on a published object.
type FixtureExecution struct {
	RunID              int64                  `json:"runId,string"`
	Generation         int64                  `json:"generation,string"`
	HarnessImageDigest string                 `json:"harnessImageDigest"`
	RevisionID         int64                  `json:"revisionId,string"`
	RevisionDigest     string                 `json:"revisionDigest"`
	Manifest           processor.Manifest     `json:"manifest"`
	Files              []processor.SourceFile `json:"files"`
	EvaluationContext  json.RawMessage        `json:"evaluationContext"`
	WorkspaceFiles     json.RawMessage        `json:"workspaceFiles"`
	ConfigOverrides    json.RawMessage        `json:"configOverrides"`
	ExpectedDecision   string                 `json:"expectedDecision,omitempty"`
}

// FixtureResult is a bounded Result Contract produced by the protected
// bootstrap in an exact Harness image. Diagnostics are deliberately safe and
// bounded; secret values and raw interpreter output never cross this seam.
type FixtureResult struct {
	Contract       json.RawMessage `json:"contract,omitempty"`
	ContractDigest string          `json:"contractDigest,omitempty"`
	ReasonCode     string          `json:"reasonCode,omitempty"`
	Diagnostics    string          `json:"diagnostics,omitempty"`
}
