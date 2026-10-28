package domain

import (
	"encoding/json"
	"fmt"
	"strconv"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
)

type Task struct {
	ID                          int64                `json:"id,string"`
	TenantID                    int64                `json:"tenantId,string"`
	ProjectID                   int64                `json:"projectId,string"`
	ProjectVersionID            int64                `json:"projectVersionId,string"`
	SelectedRuntimeReleaseID    int64                `json:"selectedRuntimeReleaseId,string,omitempty"`
	SelectedRuntime             *SelectedRuntimeView `json:"selectedRuntime,omitempty"`
	ExecutionRequirementsDigest string               `json:"executionRequirementsDigest,omitempty"`
	Name                        string               `json:"name"`
	Tags                        []string             `json:"tags"`
	CreatedBy                   int64                `json:"createdBy,string"`
	MetadataVersion             int64                `json:"metadataVersion,string"`
	InputKind                   string               `json:"inputKind,omitempty"`
	ReferenceCount              int                  `json:"referenceCount,omitempty"`
	RepositoryURL               string               `json:"repositoryUrl"`
	RepositoryRef               string               `json:"repositoryRef"`
	ResolvedBaseCommit          string               `json:"resolvedBaseCommit,omitempty"`
	Instruction                 string               `json:"instruction"`
	// ValidationCommands remains a read-only legacy field for terminal Tasks
	// created before Result Processing Closure. New Task creates do not accept
	// or execute it.
	ValidationCommands             []string            `json:"-"`
	ResultProcessorSelectionMode   string              `json:"-"`
	ResultProcessorReleaseID       int64               `json:"resultProcessorReleaseId,string,omitempty"`
	ResultProcessorParameters      json.RawMessage     `json:"-"`
	ResultProcessorConfigOverrides json.RawMessage     `json:"-"`
	TaskTurnID                     int64               `json:"taskTurnId,string,omitempty"`
	CurrentTurnNumber              int                 `json:"currentTurnNumber,omitempty"`
	HarnessAlias                   string              `json:"-"`
	HarnessReleaseID               int64               `json:"-"`
	ExecutionSnapshot              *ExecutionSnapshot  `json:"-"`
	ReviewEnabled                  bool                `json:"reviewEnabled"`
	ModelCredentialID              int64               `json:"-"`
	ModelCredentialName            string              `json:"-"`
	ModelProtocol                  string              `json:"-"`
	PrimaryModel                   string              `json:"-"`
	ModelSelection                 *ModelSelectionView `json:"modelSelection,omitempty"`
	Status                         TaskStatus          `json:"status"`
	Priority                       int                 `json:"priority"`
	HarnessTimeoutSeconds          int                 `json:"executionTimeoutSeconds"`
	ExecutionActiveMilliseconds    int64               `json:"executionActiveMilliseconds,string"`
	LastProgressAt                 string              `json:"lastProgressAt,omitempty"`
	ResultSummary                  string              `json:"resultSummary,omitempty"`
	LatestAttemptID                int64               `json:"latestAttemptId,string,omitempty"`
	CreatedAt                      string              `json:"createdAt"`
	StartedAt                      string              `json:"startedAt,omitempty"`
	FinishedAt                     string              `json:"finishedAt,omitempty"`
	Version                        int64               `json:"version,string"`
	Execution                      *ExecutionView      `json:"execution,omitempty"`
	CancelRequestID                int64               `json:"cancelRequestId,string,omitempty"`
	CancelGeneration               int64               `json:"cancelGeneration,string,omitempty"`
	CancelRequestedBy              int64               `json:"cancelRequestedBy,string,omitempty"`
	CancelRequestedAt              string              `json:"cancelRequestedAt,omitempty"`
	CancelReason                   string              `json:"cancelReason,omitempty"`
	// IndependentReviewStatus is a presentation-safe projection populated only
	// by ReviewRun's facade. It must never be inferred from TaskStatus, because
	// Processor/manual/delivery review use separate business semantics.
	IndependentReviewStatus string `json:"-"`
}

type ModelSelectionView struct {
	CredentialID   int64  `json:"credentialId,string"`
	CredentialName string `json:"credentialName"`
	Protocol       string `json:"protocol"`
	PrimaryModel   string `json:"primaryModel"`
}

type SelectedRuntimeView struct {
	RuntimeReleaseID int64  `json:"runtimeReleaseId,string"`
	Name             string `json:"name"`
	ImageDigest      string `json:"imageDigest"`
}

type ExecutionView struct {
	ExecutionNodeID            int64                  `json:"executionNodeId,string"`
	NodeKey                    string                 `json:"nodeKey"`
	NodeStatus                 string                 `json:"nodeStatus"`
	QueueWaitReason            string                 `json:"queueWaitReason,omitempty"`
	ExecutionRequirementDigest string                 `json:"executionRequirementDigest"`
	AssignmentID               int64                  `json:"assignmentId,string,omitempty"`
	AssignmentStatus           string                 `json:"assignmentStatus,omitempty"`
	WorkerID                   int64                  `json:"workerId,string,omitempty"`
	WorkerInstanceID           string                 `json:"workerInstanceId,omitempty"`
	HostBootID                 string                 `json:"hostBootId,omitempty"`
	AttemptID                  int64                  `json:"attemptId,string,omitempty"`
	Generation                 int64                  `json:"generation,string"`
	ContainerName              string                 `json:"containerName,omitempty"`
	ContainerID                string                 `json:"containerId,omitempty"`
	ExecutionIdentityDigest    string                 `json:"executionIdentityDigest,omitempty"`
	RuntimeStatus              string                 `json:"runtimeStatus,omitempty"`
	OwnerEpoch                 int64                  `json:"ownerEpoch,string,omitempty"`
	ExecutionLeaseExpiresAt    string                 `json:"executionLeaseExpiresAt,omitempty"`
	ContainerCreatedAt         string                 `json:"containerCreatedAt,omitempty"`
	SupervisorReadyAt          string                 `json:"supervisorReadyAt,omitempty"`
	RunPermitIssuedAt          string                 `json:"runPermitIssuedAt,omitempty"`
	HarnessStartedAt           string                 `json:"harnessStartedAt,omitempty"`
	DeliveryAckedAt            string                 `json:"deliveryAckedAt,omitempty"`
	AttemptStatus              string                 `json:"attemptStatus,omitempty"`
	AttemptNumber              int                    `json:"attemptNumber,omitempty"`
	MaxAttempts                int                    `json:"maxAttempts"`
	RetryCount                 int                    `json:"retryCount"`
	NextRetryAt                string                 `json:"nextRetryAt,omitempty"`
	LastRetryReasonCode        string                 `json:"lastRetryReasonCode,omitempty"`
	RecoveryPreviousStatus     string                 `json:"recoveryPreviousStatus,omitempty"`
	RecoveryStartedAt          string                 `json:"recoveryStartedAt,omitempty"`
	ReattachDeadline           string                 `json:"reattachDeadline,omitempty"`
	PendingWorkerInstanceID    string                 `json:"pendingWorkerInstanceId,omitempty"`
	PendingOwnerEpoch          int64                  `json:"pendingOwnerEpoch,string,omitempty"`
	ReattachedAt               string                 `json:"reattachedAt,omitempty"`
	WatchdogDeadline           string                 `json:"watchdogDeadline,omitempty"`
	TerminalReasonCode         string                 `json:"terminalReasonCode,omitempty"`
	Resolved                   *ResolvedExecutionView `json:"resolved,omitempty"`
}

// ResolvedExecutionView is the safe diagnostic projection of the immutable
// Assignment/Attempt selection. It is deliberately separate from Task's
// project-owned business snapshot and excludes secrets, credential versions,
// raw Provider configuration and private runtime locators.
type ResolvedExecutionView struct {
	RuntimeReleaseID        int64              `json:"runtimeReleaseId,string"`
	RuntimeAlias            string             `json:"runtimeAlias"`
	RuntimeImageDigest      string             `json:"runtimeImageDigest"`
	AdapterID               string             `json:"adapterId"`
	AdapterVersion          string             `json:"adapterVersion"`
	AdapterProtocol         string             `json:"adapterProtocol"`
	AdapterCapabilities     []string           `json:"adapterCapabilities"`
	HookConfigurationDigest string             `json:"hookConfigurationDigest,omitempty"`
	Hooks                   []ResolvedHookView `json:"hooks"`
	ModelCredentialID       int64              `json:"modelCredentialId,string"`
	ModelCredentialName     string             `json:"modelCredentialName"`
	ModelProtocol           string             `json:"modelProtocol"`
	PrimaryModel            string             `json:"primaryModel"`
	CredentialVersion       int64              `json:"credentialVersion,string"`
	WorkerImageContract     string             `json:"workerImageContract"`
	IsolationProfile        string             `json:"isolationProfile"`
	SelectionDigest         string             `json:"selectionDigest"`
	ExecutionDeadlineSecs   int                `json:"executionDeadlineSeconds"`
}

type ResolvedHookView struct {
	ReleaseID int64  `json:"releaseId,string"`
	HookID    int64  `json:"hookId,string"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

type BacklogReasonCount struct {
	ReasonCode string `json:"reasonCode"`
	Count      int    `json:"count"`
}

type BacklogSummary struct {
	QueuedTasks       int                  `json:"queuedTasks"`
	OldestWaitSeconds int64                `json:"oldestWaitSeconds"`
	Reasons           []BacklogReasonCount `json:"reasons"`
	CompatibleWorkers int                  `json:"compatibleWorkers"`
	AvailableWorkers  int                  `json:"availableWorkers"`
}

type HookPackageReleaseSnapshot struct {
	ID               int64  `json:"id,string"`
	PackageID        int64  `json:"packageId,string"`
	Name             string `json:"name"`
	Version          string `json:"version"`
	Digest           string `json:"digest"`
	RuntimeKind      string `json:"runtimeKind"`
	ArchiveObjectKey string `json:"archiveObjectKey,omitempty"`
}

func (s *HookPackageReleaseSnapshot) UnmarshalJSON(body []byte) error {
	type snapshotAlias HookPackageReleaseSnapshot
	var value struct {
		snapshotAlias
		LegacyID string `json:"packageReleaseId"`
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return err
	}
	*s = HookPackageReleaseSnapshot(value.snapshotAlias)
	if s.ID == 0 && value.LegacyID != "" {
		parsed, err := strconv.ParseInt(value.LegacyID, 10, 64)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("invalid legacy package release id")
		}
		s.ID = parsed
	}
	return nil
}

type ExecutionSnapshot struct {
	StagePurpose               string                       `json:"stagePurpose,omitempty"`
	ApprovedPlan               json.RawMessage              `json:"approvedPlan,omitempty"`
	CompletionCheckInputDigest string                       `json:"completionCheckInputDigest,omitempty"`
	CompletionCheck            *completioncheck.Input       `json:"completionCheck,omitempty"`
	CompletionCheckDigest      string                       `json:"completionCheckDigest,omitempty"`
	CompletionCheckReferences  []CompletionCheckReference   `json:"completionCheckReferences,omitempty"`
	HarnessReleaseID           int64                        `json:"harnessReleaseId,string"`
	HarnessImageDigest         string                       `json:"harnessImageDigest"`
	HookConfigurationDigest    string                       `json:"hookConfigurationDigest,omitempty"`
	PackageReleases            []HookPackageReleaseSnapshot `json:"packageReleases"`
	DispatcherVersion          string                       `json:"dispatcherVersion,omitempty"`
	DispatcherSHA256           string                       `json:"dispatcherSha256,omitempty"`
	AdapterVersion             string                       `json:"adapterVersion,omitempty"`
	ProviderProxyImageDigest   string                       `json:"providerProxyImageDigest,omitempty"`
	ProviderProxyVersion       string                       `json:"providerProxyVersion,omitempty"`
	ModelCredentialID          int64                        `json:"modelCredentialId,string,omitempty"`
	CredentialVersion          int64                        `json:"credentialVersion,string,omitempty"`
	ModelProtocol              string                       `json:"modelProtocol,omitempty"`
	PrimaryModel               string                       `json:"primaryModel,omitempty"`
	LocalProviderFacadeVersion string                       `json:"localProviderFacadeVersion,omitempty"`
	LocalProviderFacadeDigest  string                       `json:"localProviderFacadeDigest,omitempty"`
	ProviderGatewayIdentity    string                       `json:"providerGatewayIdentity,omitempty"`
	ProviderGatewayContract    string                       `json:"providerGatewayContractDigest,omitempty"`
	ProviderAdapterIdentity    string                       `json:"providerAdapterIdentity,omitempty"`
	ExecutionDeadlineSeconds   int                          `json:"executionDeadlineSeconds"`
	ResultProcessorReleaseID   int64                        `json:"resultProcessorReleaseId,string,omitempty"`
	ResultProcessorDigest      string                       `json:"resultProcessorDigest,omitempty"`
	PythonRuntimeIdentity      string                       `json:"pythonRuntimeIdentity,omitempty"`
	BootstrapDigest            string                       `json:"bootstrapDigest,omitempty"`
	SDKDigest                  string                       `json:"sdkDigest,omitempty"`
	ModuleIndexDigest          string                       `json:"moduleIndexDigest,omitempty"`
	EnvironmentConfigDigest    string                       `json:"environmentConfigDigest,omitempty"`
	ResultProcessorEnvironment json.RawMessage              `json:"resultProcessorEnvironment,omitempty"`
	SecretVersionIdentities    json.RawMessage              `json:"secretVersionIdentities,omitempty"`
	ResultProcessorParameters  json.RawMessage              `json:"resultProcessorParameters,omitempty"`
	MaxEvaluationCycles        int                          `json:"maxEvaluationCycles,omitempty"`
	ContinuationPolicy         string                       `json:"continuationPolicy,omitempty"`
	DeliveryPolicy             string                       `json:"deliveryPolicy,omitempty"`
	IsolationProfile           string                       `json:"isolationProfile"`
}

type Event struct {
	ID        int64           `json:"id,string"`
	TaskID    int64           `json:"taskId,string"`
	AttemptID int64           `json:"attemptId,string,omitempty"`
	Type      string          `json:"type"`
	Sequence  int64           `json:"sequence,string"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"createdAt"`
}

// UnmarshalJSON mirrors Attempt's safe-int boundary for event cursors. Event
// IDs and producer sequences are encoded as quoted decimals by Framework HTTP.
func (e *Event) UnmarshalJSON(body []byte) error {
	type plainEvent Event
	var wire struct {
		plainEvent
		ID       json.RawMessage `json:"id"`
		Sequence json.RawMessage `json:"sequence,string"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	id, err := parseWireInt64(wire.ID, false, "event id")
	if err != nil {
		return err
	}
	sequence, err := parseWireInt64(wire.Sequence, false, "event sequence")
	if err != nil {
		return err
	}
	*e = Event(wire.plainEvent)
	e.ID, e.Sequence = id, sequence
	return nil
}

type Artifact struct {
	ID          int64           `json:"id,string"`
	TaskID      int64           `json:"taskId,string"`
	AttemptID   int64           `json:"attemptId,string"`
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	ObjectKey   string          `json:"-"`
	SHA256      string          `json:"sha256"`
	SizeBytes   int64           `json:"sizeBytes"`
	MediaType   string          `json:"mediaType"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   string          `json:"createdAt"`
	DownloadURL string          `json:"downloadUrl"`
}

type WorkItem struct {
	CompletionCheck            *completioncheck.Input      `json:"completionCheck,omitempty"`
	CompletionCheckDigest      string                      `json:"completionCheckDigest,omitempty"`
	CompletionCheckInputDigest string                      `json:"completionCheckInputDigest,omitempty"`
	CompletionCheckReferences  []completioncheck.Reference `json:"completionCheckReferences,omitempty"`
	TaskID                     int64                       `json:"taskId,string"`
	TenantID                   int64                       `json:"tenantId,string"`
	ProjectVersionID           int64                       `json:"projectVersionId,string"`
	// Legacy read-only fields are populated only for historical evidence paths.
	// New executions require TaskInput and never clone these locations.
	RepositoryURL                string             `json:"-"`
	RepositoryRef                string             `json:"-"`
	TaskInput                    *TaskInputContract `json:"taskInput"`
	Instruction                  string             `json:"instruction"`
	TaskTurnID                   int64              `json:"taskTurnId,string,omitempty"`
	ResultProcessorSelectionMode string             `json:"resultProcessorSelectionMode,omitempty"`
	ResultProcessorParameters    json.RawMessage    `json:"resultProcessorParameters,omitempty"`
	// ResultProcessorArchiveObjectKey is a transport locator resolved by the
	// Server from the immutable release id/digest. It is not an authoring
	// default and must never be re-resolved by a Worker from a mutable binding.
	ResultProcessorArchiveObjectKey string               `json:"resultProcessorArchiveObjectKey,omitempty"`
	ValidationCommands              []string             `json:"validationCommands"`
	HarnessAlias                    string               `json:"harnessAlias"`
	HarnessReleaseID                int64                `json:"harnessReleaseId,string"`
	HarnessImage                    string               `json:"harnessImage"`
	HarnessImageDigest              string               `json:"harnessImageDigest"`
	ExecutionSnapshot               ExecutionSnapshot    `json:"executionSnapshot"`
	TaskExecutionSnapshotDigest     string               `json:"taskExecutionSnapshotDigest"`
	ModelCredentialName             string               `json:"modelCredentialName,omitempty"`
	PrimaryModel                    string               `json:"primaryModel,omitempty"`
	ResolvedBaseCommit              string               `json:"resolvedBaseCommit,omitempty"`
	HarnessTimeoutSeconds           int                  `json:"harnessTimeoutSeconds"`
	ExecutionActiveMilliseconds     int64                `json:"executionActiveMilliseconds,string"`
	ContinuationRestore             *ContinuationRestore `json:"continuationRestore,omitempty"`
	InteractionBudgetRestored       bool                 `json:"-"`
	ReviewCorrection                *ReviewCorrection    `json:"reviewCorrection,omitempty"`
	// Set only by the Worker after authenticated private checkpoint readback.
	// It is not accepted from task creation or a public WorkItem JSON field.
	InteractionResponse            json.RawMessage `json:"-"`
	InteractionQuestionRequest     json.RawMessage `json:"-"`
	SourceExecutionOperationID     int64           `json:"-"`
	SourceAdapterSessionGeneration int64           `json:"-"`
	OperationToolResult            json.RawMessage `json:"-"`
	// AllowedAdapterTools is resolved by the Server from frozen Task policy.
	// It crosses only the authenticated Server-to-Worker WorkItem contract; task
	// creation DTOs and repository content have no field that can populate it.
	AllowedAdapterTools []string `json:"allowedAdapterTools,omitempty"`
}

// ReviewCorrection is safe canonical business evidence for an Agent repair
// turn. It contains no Reviewer runtime, Provider, lease, permit or host path.
type ReviewCorrection struct {
	SourceReviewRunID int64           `json:"sourceReviewRunId,string"`
	RevisionKind      string          `json:"revisionKind"`
	Verdict           string          `json:"verdict,omitempty"`
	ReportSchema      string          `json:"reportSchemaVersion,omitempty"`
	ReportDigest      string          `json:"reportDigest,omitempty"`
	Report            json.RawMessage `json:"report,omitempty"`
	ReasonCode        string          `json:"reasonCode,omitempty"`
}
type TaskInputContract struct {
	SourceKind                    string               `json:"sourceKind"`
	SnapshotDigest                string               `json:"snapshotDigest"`
	SourceRevisionID              int64                `json:"workspaceSourceRevisionId,string"`
	SourceCommit                  string               `json:"sourceCommit"`
	SourceTree                    string               `json:"sourceTree"`
	SourceIdentityDigest          string               `json:"sourceIdentityDigest"`
	RepositoryBundleObjectKey     string               `json:"repositoryBundleObjectKey"`
	RepositoryBundleContentSHA256 string               `json:"repositoryBundleContentSha256"`
	RepositoryBundleObjectSHA256  string               `json:"repositoryBundleObjectSha256"`
	RepositoryBundleSizeBytes     int64                `json:"repositoryBundleSizeBytes"`
	References                    []TaskInputReference `json:"references"`
}

// UnmarshalJSON accepts Framework's lossless quoted int64 representation at
// the Machine HTTP boundary. Repository bundles are currently bounded well
// below JavaScript's safe-integer limit, but using one transport rule for every
// int64 keeps browser and Worker clients on the same contract.
func (c *TaskInputContract) UnmarshalJSON(body []byte) error {
	type plainContract TaskInputContract
	var wire struct {
		plainContract
		RepositoryBundleSizeBytes json.RawMessage `json:"repositoryBundleSizeBytes"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	*c = TaskInputContract(wire.plainContract)
	if len(wire.RepositoryBundleSizeBytes) == 0 {
		return nil
	}
	size, err := parseWireInt64(wire.RepositoryBundleSizeBytes, false, "task input repository bundle size")
	if err != nil {
		return err
	}
	c.RepositoryBundleSizeBytes = size
	return nil
}

type TaskInputReference struct {
	ID            int64  `json:"id,string"`
	DisplayName   string `json:"displayName"`
	ContentType   string `json:"contentType"`
	ContentSHA256 string `json:"contentSha256"`
	ObjectKey     string `json:"objectKey"`
	ObjectSHA256  string `json:"objectSha256"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// UnmarshalJSON mirrors TaskInputContract for nested reference assets. Without
// this boundary adapter a valid reference uploaded through Framework is
// rejected by the Worker before Claim can resume after a delivery retry.
func (r *TaskInputReference) UnmarshalJSON(body []byte) error {
	type plainReference TaskInputReference
	var wire struct {
		plainReference
		SizeBytes json.RawMessage `json:"sizeBytes"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	*r = TaskInputReference(wire.plainReference)
	if len(wire.SizeBytes) == 0 {
		return nil
	}
	size, err := parseWireInt64(wire.SizeBytes, false, "task input reference size")
	if err != nil {
		return err
	}
	r.SizeBytes = size
	return nil
}

type ContinuationRestore struct {
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
	ResumeNonce                   string `json:"resumeNonce"`
	ResumeNonceDigest             string `json:"resumeNonceDigest"`
	EncryptionAlgorithm           string `json:"encryptionAlgorithm,omitempty"`
	EncryptionKeyID               string `json:"encryptionKeyId,omitempty"`
	WrappedDataKey                string `json:"-"`
}

// UnmarshalJSON accepts Framework's lossless quoted int64 representation at
// the Machine HTTP boundary. Checkpoint byte sizes can exceed JavaScript's
// exact integer range even though the current product limits are smaller, so
// the transport deliberately applies the same safe-int contract everywhere.
func (r *ContinuationRestore) UnmarshalJSON(body []byte) error {
	type plainRestore ContinuationRestore
	var wire struct {
		plainRestore
		WorkspaceSizeBytes        json.RawMessage `json:"workspaceSizeBytes"`
		RepositoryBundleSizeBytes json.RawMessage `json:"repositoryBundleSizeBytes"`
		WorkspaceOverlaySizeBytes json.RawMessage `json:"workspaceOverlaySizeBytes"`
		SessionSizeBytes          json.RawMessage `json:"sessionSizeBytes"`
		ManifestSizeBytes         json.RawMessage `json:"manifestSizeBytes"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	*r = ContinuationRestore(wire.plainRestore)
	values := []struct {
		raw   json.RawMessage
		name  string
		apply func(int64)
	}{
		{wire.WorkspaceSizeBytes, "continuation workspace size", func(value int64) { r.WorkspaceSizeBytes = value }},
		{wire.RepositoryBundleSizeBytes, "continuation repository bundle size", func(value int64) { r.RepositoryBundleSizeBytes = value }},
		{wire.WorkspaceOverlaySizeBytes, "continuation workspace overlay size", func(value int64) { r.WorkspaceOverlaySizeBytes = value }},
		{wire.SessionSizeBytes, "continuation session size", func(value int64) { r.SessionSizeBytes = value }},
		{wire.ManifestSizeBytes, "continuation manifest size", func(value int64) { r.ManifestSizeBytes = value }},
	}
	for _, value := range values {
		if len(value.raw) == 0 {
			continue
		}
		parsed, err := parseWireInt64(value.raw, false, value.name)
		if err != nil {
			return err
		}
		value.apply(parsed)
	}
	return nil
}

type Attempt struct {
	ID              int64             `json:"id,string"`
	TaskID          int64             `json:"taskId,string"`
	Generation      int64             `json:"generation,string"`
	LeaseToken      string            `json:"leaseToken"`
	LeaseExpiresAt  string            `json:"leaseExpiresAt,omitempty"`
	WorkspacePath   string            `json:"workspacePath"`
	ExecutionNodeID int64             `json:"executionNodeId,string,omitempty"`
	AssignmentID    int64             `json:"assignmentId,string,omitempty"`
	WorkerID        int64             `json:"workerId,string,omitempty"`
	WorkerInstance  string            `json:"workerInstanceId,omitempty"`
	HostBootID      string            `json:"hostBootId,omitempty"`
	OwnerEpoch      int64             `json:"ownerEpoch,string,omitempty"`
	ContainerName   string            `json:"expectedContainerName,omitempty"`
	RuntimeLabels   map[string]string `json:"runtimeLabels,omitempty"`
}

// UnmarshalJSON accepts both a JSON number and a quoted decimal generation.
// Framework responses intentionally represent int64 values as strings so
// browser clients never lose precision. Machine clients use the same contract,
// so this boundary accepts either canonical representation.
func (a *Attempt) UnmarshalJSON(body []byte) error {
	type plainAttempt Attempt
	var wire struct {
		plainAttempt
		Generation json.RawMessage `json:"generation,string"`
		OwnerEpoch json.RawMessage `json:"ownerEpoch,string"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	// Delivery directives such as ACK_TERMINAL intentionally carry an empty
	// Attempt value. Framework's lossless JSON encoder still serializes the
	// zero int64 fields as "0"; accepting that representation only when the
	// Attempt identity itself is empty lets a Worker ACK a terminal redelivery
	// without weakening validation for executable Attempts.
	if wire.plainAttempt.ID == 0 {
		generation, err := parseWireInt64(wire.Generation, false, "attempt generation")
		if err != nil {
			return err
		}
		*a = Attempt(wire.plainAttempt)
		a.Generation = generation
		if len(wire.OwnerEpoch) != 0 && string(wire.OwnerEpoch) != "null" {
			ownerEpoch, parseErr := parseWireInt64(wire.OwnerEpoch, false, "attempt owner epoch")
			if parseErr != nil {
				return parseErr
			}
			a.OwnerEpoch = ownerEpoch
		}
		return nil
	}
	generation, err := parseWireInt64(wire.Generation, true, "attempt generation")
	if err != nil {
		return err
	}
	*a = Attempt(wire.plainAttempt)
	a.Generation = generation
	if len(wire.OwnerEpoch) != 0 && string(wire.OwnerEpoch) != "null" {
		ownerEpoch, parseErr := parseWireInt64(wire.OwnerEpoch, true, "attempt owner epoch")
		if parseErr != nil {
			return parseErr
		}
		a.OwnerEpoch = ownerEpoch
	}
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
