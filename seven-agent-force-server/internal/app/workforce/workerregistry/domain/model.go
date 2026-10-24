package domain

import (
	"encoding/json"
	"strconv"
	"time"
)

type MetricPoint struct {
	At                        time.Time `json:"at"`
	CPUUsagePercent           *float64  `json:"cpuUsagePercent"`
	MemoryUsagePercent        *float64  `json:"memoryUsagePercent"`
	WorkspaceDiskUsagePercent *float64  `json:"workspaceDiskUsagePercent"`
	ActiveAttempts            int       `json:"activeAttempts"`
	MaxActiveAttempts         int       `json:"maxActiveAttempts"`
}

type MetricSeries struct {
	From        time.Time     `json:"from"`
	To          time.Time     `json:"to"`
	StepSeconds int           `json:"stepSeconds"`
	Points      []MetricPoint `json:"points"`
}

const (
	DesiredEnabled  = "ENABLED"
	DesiredDraining = "DRAINING"
	DesiredDisabled = "DISABLED"

	StateProvisioned    = "PROVISIONED"
	StateReconciling    = "RECONCILING"
	StateOnline         = "ONLINE"
	StateDraining       = "DRAINING"
	StateDrained        = "DRAINED"
	StateStale          = "STALE"
	StateOffline        = "OFFLINE"
	StateDisabled       = "DISABLED"
	StateDecommissioned = "DECOMMISSIONED"
)

type QueueContract struct {
	SchemaVersion         int    `json:"schemaVersion"`
	Exchange              string `json:"exchange"`
	Queue                 string `json:"queue"`
	RoutingKey            string `json:"routingKey"`
	DeadQueue             string `json:"deadQueue"`
	MessageSchema         string `json:"messageSchema"`
	MaxPrefetch           int    `json:"maxPrefetch"`
	PostprocessExchange   string `json:"postprocessExchange"`
	PostprocessQueue      string `json:"postprocessQueue"`
	PostprocessRoutingKey string `json:"postprocessRoutingKey"`
	PostprocessDeadQueue  string `json:"postprocessDeadQueue"`
}

type Reconciliation struct {
	Required bool                 `json:"required"`
	Deadline string               `json:"deadline,omitempty"`
	Runtimes []RecoverableRuntime `json:"runtimes"`
}

type RecoverableRuntime struct {
	CancellationRequested       bool   `json:"cancellationRequested"`
	TaskID                      int64  `json:"taskId,string"`
	AttemptID                   int64  `json:"attemptId,string"`
	ExecutionNodeID             int64  `json:"executionNodeId,string"`
	AssignmentID                int64  `json:"assignmentId,string"`
	Generation                  int64  `json:"generation,string"`
	OwnerEpoch                  int64  `json:"ownerEpoch,string"`
	HostBootID                  string `json:"hostBootId"`
	ExpectedContainerName       string `json:"expectedContainerName"`
	ContainerID                 string `json:"containerId"`
	RuntimeStatus               string `json:"runtimeStatus"`
	ExecutionIdentityDigest     string `json:"executionIdentityDigest"`
	HarnessImageDigest          string `json:"harnessImageDigest"`
	HookConfigurationDigest     string `json:"hookConfigurationDigest"`
	ProviderBacked              bool   `json:"providerBacked"`
	ProviderProxyContainerID    string `json:"providerProxyContainerId,omitempty"`
	ProviderProxyContainerName  string `json:"providerProxyContainerName,omitempty"`
	ProviderProxyImageDigest    string `json:"providerProxyImageDigest,omitempty"`
	ProviderProxyIdentityDigest string `json:"providerProxyExecutionIdentityDigest,omitempty"`
	ReattachDeadline            string `json:"reattachDeadline"`
}

type Worker struct {
	ID                         int64           `json:"id,string"`
	Name                       string          `json:"name"`
	Description                string          `json:"description"`
	DesiredState               string          `json:"desiredState"`
	State                      string          `json:"state"`
	MaxActiveAttempts          int             `json:"maxActiveAttempts"`
	ReportedMaxActiveAttempts  int             `json:"reportedMaxActiveAttempts"`
	CurrentInstanceID          string          `json:"currentInstanceId,omitempty"`
	HostBootID                 string          `json:"hostBootId,omitempty"`
	WorkerVersion              string          `json:"workerVersion,omitempty"`
	WorkerRevision             string          `json:"workerRevision,omitempty"`
	WorkerBuildDate            string          `json:"workerBuildDate,omitempty"`
	TokenHint                  string          `json:"tokenHint"`
	ObservedRemoteIP           string          `json:"observedRemoteIp,omitempty"`
	ReportedIP                 string          `json:"reportedIp,omitempty"`
	Hostname                   string          `json:"hostname,omitempty"`
	PrimaryInterface           string          `json:"primaryInterface,omitempty"`
	PrimaryMAC                 string          `json:"primaryMac,omitempty"`
	ConsumerState              string          `json:"consumerState,omitempty"`
	SessionState               string          `json:"sessionState,omitempty"`
	ReconciliationDeadline     string          `json:"reconciliationDeadline,omitempty"`
	DockerReady                bool            `json:"dockerReady"`
	ActiveAttempts             int             `json:"activeAttempts"`
	ReservedAttempts           int             `json:"reservedAttempts"`
	StartingAttempts           int             `json:"startingAttempts"`
	RunningAttempts            int             `json:"runningAttempts"`
	AvailableAttempts          int             `json:"availableAttempts"`
	InventoryRefreshGeneration int64           `json:"inventoryRefreshGeneration,string"`
	InventoryVersion           int64           `json:"inventoryVersion,string"`
	DecommissionedAt           string          `json:"decommissionedAt,omitempty"`
	LastHeartbeatAt            string          `json:"lastHeartbeatAt,omitempty"`
	ResourceSnapshot           json.RawMessage `json:"resourceSnapshot,omitempty"`
	Interfaces                 json.RawMessage `json:"interfaces,omitempty"`
	ImageCount                 int             `json:"imageCount"`
	PendingFinalizations       int             `json:"pendingFinalizations"`
	PendingArtifactUploads     int             `json:"pendingArtifactUploads"`
	Version                    int64           `json:"version,string"`
	CreatedAt                  string          `json:"createdAt"`
}

type Assignment struct {
	ID                         int64  `json:"id,string"`
	TaskID                     int64  `json:"taskId,string"`
	ExecutionNodeID            int64  `json:"executionNodeId,string"`
	WorkerInstanceID           string `json:"workerInstanceId"`
	Generation                 int64  `json:"generation,string"`
	ExecutionRequirementDigest string `json:"executionRequirementDigest"`
	Status                     string `json:"status"`
	MessageID                  int64  `json:"messageId,string"`
	AttemptID                  int64  `json:"attemptId,string,omitempty"`
	CreatedAt                  string `json:"createdAt"`
	ClaimedAt                  string `json:"claimedAt,omitempty"`
	CompletedAt                string `json:"completedAt,omitempty"`
}

type Image struct {
	ImageDigest             string          `json:"imageDigest"`
	ImageRef                string          `json:"imageRef"`
	Role                    string          `json:"role"`
	ContractVersion         string          `json:"contractVersion"`
	ManifestDigest          string          `json:"manifestDigest"`
	Manifest                json.RawMessage `json:"manifest"`
	Capabilities            json.RawMessage `json:"capabilities"`
	SchedulingScope         string          `json:"schedulingScope"`
	SchedulingPriority      int             `json:"schedulingPriority"`
	EnvironmentCapabilities json.RawMessage `json:"environmentCapabilities"`
	ExecutionKinds          json.RawMessage `json:"executionKinds"`
	SchedulingDigest        string          `json:"schedulingDigest"`
	AdmissionStatus         string          `json:"admissionStatus"`
	AdmissionEvidence       json.RawMessage `json:"admissionEvidence"`
	InventoryVersion        int64           `json:"inventoryVersion,string"`
	PresenceStatus          string          `json:"presenceStatus"`
	MissingSince            string          `json:"missingSince,omitempty"`
	DriftReason             string          `json:"driftReason,omitempty"`
}

func QueueFor(workerID int64, maximum ...int) QueueContract {
	prefetch := 1
	if len(maximum) > 0 && maximum[0] > 0 {
		prefetch = maximum[0]
	}
	if prefetch > 64 {
		prefetch = 64
	}
	id := strconv.FormatInt(workerID, 10)
	return QueueContract{SchemaVersion: 1, Exchange: "seven.workforce.tasks", Queue: "seven.workforce.worker." + id + ".tasks", RoutingKey: "worker." + id, DeadQueue: "seven.workforce.worker." + id + ".dead", MessageSchema: "seven.task-assignment/v1", MaxPrefetch: prefetch,
		PostprocessExchange: "seven.workforce.postprocess", PostprocessQueue: "seven.workforce.worker." + id + ".postprocess", PostprocessRoutingKey: "postprocess." + id, PostprocessDeadQueue: "seven.workforce.worker." + id + ".postprocess.dead"}
}
