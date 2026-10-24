package docker

import (
	"context"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type PageResult[T any] struct {
	Current int64 `json:"current"`
	Size    int64 `json:"size"`
	Total   int64 `json:"total"`
	Records []T   `json:"records"`
}

type ContainerPortView struct {
	PrivatePort uint16 `json:"privatePort,omitempty"`
	PublicPort  uint16 `json:"publicPort,omitempty"`
	Type        string `json:"type,omitempty"`
	IP          string `json:"ip,omitempty"`
}

type ContainerView struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	Image              string              `json:"image"`
	ImageID            string              `json:"imageId"`
	State              string              `json:"state,omitempty"`
	Status             string              `json:"status,omitempty"`
	Created            int64               `json:"created,omitempty"`
	Ports              []ContainerPortView `json:"ports,omitempty"`
	Labels             map[string]string   `json:"labels,omitempty"`
	RestartCount       int                 `json:"restartCount"`
	ComposeManaged     bool                `json:"composeManaged"`
	ComposeProject     string              `json:"composeProject,omitempty"`
	ComposeService     string              `json:"composeService,omitempty"`
	ComposeConfigFiles string              `json:"composeConfigFiles,omitempty"`
	ComposeWorkingDir  string              `json:"composeWorkingDir,omitempty"`
	AvailableActions   []string            `json:"availableActions,omitempty"`
	ActiveOperation    *OperationVO        `json:"activeOperation,omitempty"`
}

type ContainerDetailView struct {
	Container   ContainerView  `json:"container"`
	Inspect     map[string]any `json:"inspect"`
	ComposeYaml string         `json:"composeYaml,omitempty"`
}

type ContainerLogQuery struct {
	Tail       int    `json:"tail,omitempty"`
	Since      string `json:"since,omitempty"`
	Until      string `json:"until,omitempty"`
	Timestamps bool   `json:"timestamps,omitempty"`
	Grep       string `json:"grep,omitempty"`
	Follow     bool   `json:"follow,omitempty"`
}

type ContainerTerminalRequest struct {
	Shell string `json:"shell,omitempty"`
	Rows  uint   `json:"rows,omitempty"`
	Cols  uint   `json:"cols,omitempty"`
}

type ContainerUsageView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image,omitempty"`
	State  string `json:"state,omitempty"`
	Status string `json:"status,omitempty"`
}

type ImageView struct {
	ImageID              string            `json:"imageId"`
	RepoTags             []string          `json:"repoTags"`
	RepoDigests          []string          `json:"repoDigests"`
	Size                 int64             `json:"size,omitempty"`
	Created              int64             `json:"created,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	UsedByContainerCount int               `json:"usedByContainerCount"`
}

type ImageDetailView struct {
	ImageID              string            `json:"imageId"`
	RepoTags             []string          `json:"repoTags"`
	RepoDigests          []string          `json:"repoDigests"`
	Size                 int64             `json:"size,omitempty"`
	Created              int64             `json:"created,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	UsedByContainerCount int               `json:"usedByContainerCount"`
	Inspect              map[string]any    `json:"inspect"`
}

type ImagePullCommand struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag,omitempty"`
	RegistryID int64  `json:"registryId,omitempty"`
}

type ImageTagCommand struct {
	SourceImage      string `json:"sourceImage"`
	TargetRepository string `json:"targetRepository"`
	TargetTag        string `json:"targetTag"`
}

type ImagePushCommand struct {
	SourceImage      string `json:"sourceImage"`
	TargetRepository string `json:"targetRepository,omitempty"`
	TargetTag        string `json:"targetTag,omitempty"`
	RegistryID       int64  `json:"registryId,omitempty"`
}

type RemoteRegistryView struct {
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Code                   string `json:"code"`
	RegistryType           string `json:"registryType"`
	Endpoint               string `json:"endpoint"`
	APIBaseURL             string `json:"apiBaseUrl,omitempty"`
	AuthType               string `json:"authType"`
	Username               string `json:"username,omitempty"`
	TokenRealm             string `json:"tokenRealm,omitempty"`
	TokenService           string `json:"tokenService,omitempty"`
	CredentialID           int64  `json:"credentialId,omitempty"`
	NamespaceWhitelistJSON string `json:"namespaceWhitelistJson,omitempty"`
	TLSEnabled             bool   `json:"tlsEnabled"`
	InsecureSkipVerify     bool   `json:"insecureSkipVerify"`
	DefaultRegistry        bool   `json:"defaultRegistry"`
	Status                 int    `json:"status"`
	Description            string `json:"description,omitempty"`
	Sort                   int    `json:"sort"`
	SecretConfigured       bool   `json:"secretConfigured"`
	SecretHint             string `json:"secretHint,omitempty"`
	CreateTime             string `json:"createTime,omitempty"`
	UpdateTime             string `json:"updateTime,omitempty"`
}

type RemoteRegistryCommand struct {
	Name                   string `json:"name"`
	Code                   string `json:"code"`
	RegistryType           string `json:"registryType"`
	Endpoint               string `json:"endpoint"`
	APIBaseURL             string `json:"apiBaseUrl,omitempty"`
	AuthType               string `json:"authType"`
	Username               string `json:"username,omitempty"`
	Password               string `json:"password,omitempty"`
	TokenRealm             string `json:"tokenRealm,omitempty"`
	TokenService           string `json:"tokenService,omitempty"`
	CredentialID           int64  `json:"credentialId,omitempty"`
	NamespaceWhitelistJSON string `json:"namespaceWhitelistJson,omitempty"`
	TLSEnabled             *bool  `json:"tlsEnabled,omitempty"`
	InsecureSkipVerify     *bool  `json:"insecureSkipVerify,omitempty"`
	DefaultRegistry        *bool  `json:"defaultRegistry,omitempty"`
	Status                 *int   `json:"status,omitempty"`
	Description            string `json:"description,omitempty"`
	Sort                   *int   `json:"sort,omitempty"`
}

type DockerDaemonConfigView struct {
	Supported       bool           `json:"supported"`
	SupportReason   string         `json:"supportReason,omitempty"`
	Platform        string         `json:"platform"`
	Rootless        bool           `json:"rootless"`
	ConfigPath      string         `json:"configPath"`
	Editable        map[string]any `json:"editable"`
	Readonly        map[string]any `json:"readonly"`
	Raw             map[string]any `json:"raw"`
	EditableKeys    []string       `json:"editableKeys"`
	RequiresRestart bool           `json:"requiresRestart"`
}

type DockerDaemonConfigUpdateRequest struct {
	Editable map[string]any `json:"editable"`
}

type DockerDaemonConfigValidateView struct {
	Valid   bool     `json:"valid"`
	Message string   `json:"message,omitempty"`
	Keys    []string `json:"keys,omitempty"`
}

type RegistryConnectionTestView struct {
	Success         bool   `json:"success"`
	Message         string `json:"message"`
	ServerHeader    string `json:"serverHeader,omitempty"`
	RegistryVersion string `json:"registryVersion,omitempty"`
	TokenRealm      string `json:"tokenRealm,omitempty"`
	TokenService    string `json:"tokenService,omitempty"`
}

type RemoteRepositoryView struct {
	Repository string `json:"repository"`
}

type RemoteTagsView struct {
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
}

type RemoteManifestView struct {
	Repository         string         `json:"repository"`
	Reference          string         `json:"reference"`
	Digest             string         `json:"digest,omitempty"`
	MediaType          string         `json:"mediaType,omitempty"`
	Size               int64          `json:"size,omitempty"`
	SchemaVersion      int            `json:"schemaVersion,omitempty"`
	OS                 string         `json:"os,omitempty"`
	Architecture       string         `json:"architecture,omitempty"`
	Variant            string         `json:"variant,omitempty"`
	Created            string         `json:"created,omitempty"`
	LayerCount         int            `json:"layerCount,omitempty"`
	ChildManifestCount int            `json:"childManifestCount,omitempty"`
	Payload            map[string]any `json:"payload,omitempty"`
}

type RemoteImagePullRequest struct {
	RegistryID int64  `json:"registryId"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

type KeyValueCommand struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type PortBindingCommand struct {
	HostIP        string `json:"hostIp,omitempty"`
	HostPort      uint16 `json:"hostPort,omitempty"`
	ContainerPort uint16 `json:"containerPort,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
}

type VolumeBindingCommand struct {
	Source   string `json:"source,omitempty"`
	Target   string `json:"target,omitempty"`
	Type     string `json:"type,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

type ResourceLimitCommand struct {
	CPUs         float64 `json:"cpus,omitempty"`
	MemoryMB     int64   `json:"memoryMb,omitempty"`
	MemorySwapMB int64   `json:"memorySwapMb,omitempty"`
	PidsLimit    int64   `json:"pidsLimit,omitempty"`
}

type ImageStartupPreview struct {
	ImageID              string                 `json:"imageId"`
	ImageReference       string                 `json:"imageReference,omitempty"`
	DefaultContainerName string                 `json:"defaultContainerName,omitempty"`
	DefaultServiceName   string                 `json:"defaultServiceName,omitempty"`
	DefaultProjectName   string                 `json:"defaultProjectName,omitempty"`
	OS                   string                 `json:"os,omitempty"`
	Architecture         string                 `json:"architecture,omitempty"`
	WorkingDir           string                 `json:"workingDir,omitempty"`
	User                 string                 `json:"user,omitempty"`
	Entrypoint           []string               `json:"entrypoint,omitempty"`
	Command              []string               `json:"command,omitempty"`
	Environment          []KeyValueCommand      `json:"environment,omitempty"`
	PortBindings         []PortBindingCommand   `json:"portBindings,omitempty"`
	VolumeBindings       []VolumeBindingCommand `json:"volumeBindings,omitempty"`
	Labels               []KeyValueCommand      `json:"labels,omitempty"`
	TTY                  bool                   `json:"tty,omitempty"`
	StdinOpen            bool                   `json:"stdinOpen,omitempty"`
	PublishAllPorts      bool                   `json:"publishAllPorts,omitempty"`
	SuggestedComposeYaml string                 `json:"suggestedComposeYaml,omitempty"`
}

type ContainerCreateRequest struct {
	ImageID              string                 `json:"imageId,omitempty"`
	ImageReference       string                 `json:"imageReference,omitempty"`
	ContainerName        string                 `json:"containerName,omitempty"`
	Entrypoint           []string               `json:"entrypoint,omitempty"`
	Command              []string               `json:"command,omitempty"`
	Environment          []KeyValueCommand      `json:"environment,omitempty"`
	PortBindings         []PortBindingCommand   `json:"portBindings,omitempty"`
	VolumeBindings       []VolumeBindingCommand `json:"volumeBindings,omitempty"`
	Labels               []KeyValueCommand      `json:"labels,omitempty"`
	WorkingDir           string                 `json:"workingDir,omitempty"`
	User                 string                 `json:"user,omitempty"`
	NetworkMode          string                 `json:"networkMode,omitempty"`
	Privileged           bool                   `json:"privileged,omitempty"`
	CapAdd               []string               `json:"capAdd,omitempty"`
	CapDrop              []string               `json:"capDrop,omitempty"`
	RestartPolicy        string                 `json:"restartPolicy,omitempty"`
	RestartMaxRetryCount int                    `json:"restartMaxRetryCount,omitempty"`
	TTY                  bool                   `json:"tty,omitempty"`
	StdinOpen            bool                   `json:"stdinOpen,omitempty"`
	PublishAllPorts      bool                   `json:"publishAllPorts,omitempty"`
	AutoRemove           bool                   `json:"autoRemove,omitempty"`
	ResourceLimits       *ResourceLimitCommand  `json:"resourceLimits,omitempty"`
}

type ComposeUpRequest struct {
	ProjectName     string `json:"projectName,omitempty"`
	ComposeYaml     string `json:"composeYaml"`
	WorkingDir      string `json:"workingDir,omitempty"`
	ComposeFilePath string `json:"composeFilePath,omitempty"`
}

type ComposeWorkspaceCheckRequest struct {
	WorkingDir               string `json:"workingDir"`
	CreateIfMissing          bool   `json:"createIfMissing,omitempty"`
	OverwriteExistingCompose bool   `json:"overwriteExistingCompose,omitempty"`
}

type ComposeWorkspaceCheckView struct {
	Valid             bool     `json:"valid"`
	Exists            bool     `json:"exists"`
	CanCreate         bool     `json:"canCreate"`
	CanWrite          bool     `json:"canWrite"`
	AllowedRoot       bool     `json:"allowedRoot"`
	ComposeFileExists bool     `json:"composeFileExists"`
	ResolvedPath      string   `json:"resolvedPath,omitempty"`
	Message           string   `json:"message,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
}

type ComposeProjectFileCommand struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding,omitempty"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

type ComposeBuildFileCommand struct {
	ServiceName       string                      `json:"serviceName"`
	Context           string                      `json:"context"`
	DockerfilePath    string                      `json:"dockerfilePath,omitempty"`
	DockerfileContent string                      `json:"dockerfileContent,omitempty"`
	ExtraFiles        []ComposeProjectFileCommand `json:"extraFiles,omitempty"`
	BuildArgs         []KeyValueCommand           `json:"buildArgs,omitempty"`
	ImageTag          string                      `json:"imageTag,omitempty"`
}

type ComposePreviewWithFilesRequest struct {
	ProjectName string                    `json:"projectName,omitempty"`
	WorkingDir  string                    `json:"workingDir,omitempty"`
	ComposeYaml string                    `json:"composeYaml"`
	BuildFiles  []ComposeBuildFileCommand `json:"buildFiles,omitempty"`
}

type DockerfileBuildPreviewRequest struct {
	ProjectName       string            `json:"projectName,omitempty"`
	WorkingDir        string            `json:"workingDir,omitempty"`
	ServiceName       string            `json:"serviceName"`
	Context           string            `json:"context"`
	DockerfilePath    string            `json:"dockerfilePath,omitempty"`
	DockerfileContent string            `json:"dockerfileContent,omitempty"`
	ImageTag          string            `json:"imageTag,omitempty"`
	BuildArgs         []KeyValueCommand `json:"buildArgs,omitempty"`
}

type DockerfileBuildPreviewView struct {
	Valid                  bool                `json:"valid"`
	Message                string              `json:"message,omitempty"`
	ResolvedContext        string              `json:"resolvedContext,omitempty"`
	ResolvedDockerfilePath string              `json:"resolvedDockerfilePath,omitempty"`
	ImageTag               string              `json:"imageTag,omitempty"`
	Warnings               []PolicyViolationVO `json:"warnings,omitempty"`
	Violations             []PolicyViolationVO `json:"violations,omitempty"`
}

type ComposeYamlValidateRequest struct {
	ProjectName string `json:"projectName,omitempty"`
	WorkingDir  string `json:"workingDir,omitempty"`
	ComposeYaml string `json:"composeYaml"`
}

type ComposeUnsupportedFieldVO struct {
	Path   string `json:"path"`
	Value  any    `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type ComposeYamlValidateView struct {
	Valid             bool                        `json:"valid"`
	Message           string                      `json:"message,omitempty"`
	NormalizedYaml    string                      `json:"normalizedYaml,omitempty"`
	Services          []ComposeServiceVO          `json:"services,omitempty"`
	Networks          []string                    `json:"networks,omitempty"`
	Volumes           []string                    `json:"volumes,omitempty"`
	UnsupportedFields []ComposeUnsupportedFieldVO `json:"unsupportedFields,omitempty"`
	Warnings          []string                    `json:"warnings,omitempty"`
	VisualDraft       *ComposeVisualDraftView     `json:"visualDraft,omitempty"`
}

type ComposeBuilderMetadataView struct {
	WorkspaceRoots             []string                         `json:"workspaceRoots"`
	DefaultWorkspaceRoot       string                           `json:"defaultWorkspaceRoot,omitempty"`
	DefaultFileName            string                           `json:"defaultFileName"`
	MaxComposeBytes            int                              `json:"maxComposeBytes"`
	MaxDockerfileBytes         int                              `json:"maxDockerfileBytes"`
	MaxExtraFilesBytes         int                              `json:"maxExtraFilesBytes"`
	AllowedProjectFileSuffixes []string                         `json:"allowedProjectFileSuffixes"`
	RestartPolicies            []string                         `json:"restartPolicies"`
	NetworkModes               []string                         `json:"networkModes"`
	SupportedServiceFields     []string                         `json:"supportedServiceFields"`
	DefaultService             ComposeBuilderDefaultServiceView `json:"defaultService"`
	HealthcheckDefaults        ComposeHealthcheckDefaultsView   `json:"healthcheckDefaults"`
	ResourceLimitHints         ComposeResourceLimitHintsView    `json:"resourceLimitHints"`
}

type ComposeBuilderDefaultServiceView struct {
	Restart     string `json:"restart,omitempty"`
	NetworkMode string `json:"networkMode,omitempty"`
}

type ComposeHealthcheckDefaultsView struct {
	Interval    string `json:"interval,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Retries     int    `json:"retries,omitempty"`
	StartPeriod string `json:"startPeriod,omitempty"`
}

type ComposeResourceLimitHintsView struct {
	CPUExamples    []string `json:"cpuExamples,omitempty"`
	MemoryExamples []string `json:"memoryExamples,omitempty"`
}

type ComposeVisualDraftView struct {
	Version  string                     `json:"version,omitempty"`
	Services []ComposeVisualServiceView `json:"services"`
	Networks []ComposeVisualNetworkView `json:"networks,omitempty"`
	Volumes  []ComposeVisualVolumeView  `json:"volumes,omitempty"`
}

type ComposeVisualServiceView struct {
	ServiceName       string                         `json:"serviceName"`
	Image             string                         `json:"image,omitempty"`
	Build             *ComposeVisualBuildView        `json:"build,omitempty"`
	ContainerName     string                         `json:"containerName,omitempty"`
	Ports             []ComposeVisualPortView        `json:"ports,omitempty"`
	Environment       []KeyValueCommand              `json:"environment,omitempty"`
	Volumes           []ComposeVisualVolumeMountView `json:"volumes,omitempty"`
	Networks          []string                       `json:"networks,omitempty"`
	DependsOn         []string                       `json:"dependsOn,omitempty"`
	Restart           string                         `json:"restart,omitempty"`
	Command           any                            `json:"command,omitempty"`
	WorkingDir        string                         `json:"workingDir,omitempty"`
	User              string                         `json:"user,omitempty"`
	Healthcheck       *ComposeVisualHealthcheckView  `json:"healthcheck,omitempty"`
	Resources         *ComposeVisualResourcesView    `json:"resources,omitempty"`
	Advanced          *ComposeVisualAdvancedView     `json:"advanced,omitempty"`
	UnsupportedFields []ComposeUnsupportedFieldVO    `json:"unsupportedFields,omitempty"`
}

type ComposeVisualBuildView struct {
	Context    string            `json:"context,omitempty"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
}

type ComposeVisualPortView struct {
	HostIP        string `json:"hostIp,omitempty"`
	HostPort      string `json:"hostPort,omitempty"`
	ContainerPort string `json:"containerPort,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
}

type ComposeVisualVolumeMountView struct {
	Source   string `json:"source,omitempty"`
	Target   string `json:"target,omitempty"`
	Type     string `json:"type,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

type ComposeVisualHealthcheckView struct {
	Test        any    `json:"test,omitempty"`
	Interval    string `json:"interval,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Retries     int    `json:"retries,omitempty"`
	StartPeriod string `json:"startPeriod,omitempty"`
	Disable     bool   `json:"disable,omitempty"`
}

type ComposeVisualResourcesView struct {
	CPUs              string `json:"cpus,omitempty"`
	Memory            string `json:"memory,omitempty"`
	MemoryReservation string `json:"memoryReservation,omitempty"`
	PidsLimit         int64  `json:"pidsLimit,omitempty"`
}

type ComposeVisualAdvancedView struct {
	Privileged  bool     `json:"privileged,omitempty"`
	NetworkMode string   `json:"networkMode,omitempty"`
	PID         string   `json:"pid,omitempty"`
	IPC         string   `json:"ipc,omitempty"`
	CapAdd      []string `json:"capAdd,omitempty"`
	CapDrop     []string `json:"capDrop,omitempty"`
}

type ComposeVisualNetworkView struct {
	Name     string `json:"name"`
	Driver   string `json:"driver,omitempty"`
	External bool   `json:"external,omitempty"`
}

type ComposeVisualVolumeView struct {
	Name     string `json:"name"`
	Driver   string `json:"driver,omitempty"`
	External bool   `json:"external,omitempty"`
}

type ComposeProjectFileManifestView struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	ServiceName string `json:"serviceName,omitempty"`
	SizeBytes   int    `json:"sizeBytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
}

type ComposeProjectSource string

const (
	ComposeProjectSourceManaged    ComposeProjectSource = "MANAGED"
	ComposeProjectSourceDiscovered ComposeProjectSource = "DISCOVERED"
)

type ComposeProjectStatus string

const (
	ComposeProjectStatusRunning  ComposeProjectStatus = "running"
	ComposeProjectStatusDegraded ComposeProjectStatus = "degraded"
	ComposeProjectStatusStopped  ComposeProjectStatus = "stopped"
	ComposeProjectStatusUnknown  ComposeProjectStatus = "unknown"
)

type ComposeProjectSummaryVO struct {
	ProjectID             string               `json:"projectId"`
	ProjectName           string               `json:"projectName"`
	Source                ComposeProjectSource `json:"source"`
	WorkingDir            string               `json:"workingDir,omitempty"`
	ConfigFiles           []string             `json:"configFiles,omitempty"`
	ServiceCount          int                  `json:"serviceCount"`
	ContainerCount        int                  `json:"containerCount"`
	RunningCount          int                  `json:"runningCount"`
	ExitedCount           int                  `json:"exitedCount"`
	Status                ComposeProjectStatus `json:"status"`
	WarningCount          int                  `json:"warningCount,omitempty"`
	ViolationCount        int                  `json:"violationCount,omitempty"`
	Safe                  *bool                `json:"safe,omitempty"`
	LastOperationID       int64                `json:"lastOperationId,omitempty"`
	LastOperationType     string               `json:"lastOperationType,omitempty"`
	LastOperationStatus   OperationStatus      `json:"lastOperationStatus,omitempty"`
	LastOperationProgress int                  `json:"lastOperationProgress,omitempty"`
	LastOperationStage    string               `json:"lastOperationStage,omitempty"`
	AvailableActions      []string             `json:"availableActions,omitempty"`
	ActiveOperation       *OperationVO         `json:"activeOperation,omitempty"`
	CreatedAt             string               `json:"createdAt,omitempty"`
	UpdatedAt             string               `json:"updatedAt,omitempty"`
}

type ComposeProjectDetailVO struct {
	ProjectID        string                           `json:"projectId"`
	ProjectName      string                           `json:"projectName"`
	Source           ComposeProjectSource             `json:"source"`
	WorkingDir       string                           `json:"workingDir,omitempty"`
	ConfigFiles      []string                         `json:"configFiles,omitempty"`
	ComposeFilePath  string                           `json:"composeFilePath,omitempty"`
	FileManifest     []ComposeProjectFileManifestView `json:"fileManifest,omitempty"`
	ComposeYaml      string                           `json:"composeYaml,omitempty"`
	NormalizedYaml   string                           `json:"normalizedYaml,omitempty"`
	VisualDraft      *ComposeVisualDraftView          `json:"visualDraft,omitempty"`
	Services         []ComposeServiceVO               `json:"services,omitempty"`
	Containers       []ContainerView                  `json:"containers,omitempty"`
	Preview          *PreviewVO                       `json:"preview,omitempty"`
	Validation       *ComposeValidationView           `json:"validation,omitempty"`
	LastOperation    *OperationVO                     `json:"lastOperation,omitempty"`
	ActiveOperation  *OperationVO                     `json:"activeOperation,omitempty"`
	AvailableActions []string                         `json:"availableActions,omitempty"`
	RecentEvents     []OperationEventVO               `json:"recentEvents,omitempty"`
}

type ActionNotAllowedVO struct {
	TargetType       string   `json:"targetType"`
	TargetID         string   `json:"targetId,omitempty"`
	CurrentState     string   `json:"currentState,omitempty"`
	RequestedAction  string   `json:"requestedAction"`
	AvailableActions []string `json:"availableActions,omitempty"`
	Message          string   `json:"message"`
}

type ComposeServiceVO struct {
	ServiceName    string               `json:"serviceName"`
	Image          string               `json:"image,omitempty"`
	ContainerCount int                  `json:"containerCount"`
	RunningCount   int                  `json:"runningCount"`
	ExitedCount    int                  `json:"exitedCount"`
	Status         ComposeProjectStatus `json:"status"`
	Ports          []ContainerPortView  `json:"ports,omitempty"`
	Containers     []ContainerView      `json:"containers,omitempty"`
	WarningCount   int                  `json:"warningCount,omitempty"`
	ViolationCount int                  `json:"violationCount,omitempty"`
}

type ComposeProjectCreateRequest struct {
	ProjectName       string                    `json:"projectName"`
	WorkingDir        string                    `json:"workingDir,omitempty"`
	Description       string                    `json:"description,omitempty"`
	ComposeYaml       string                    `json:"composeYaml"`
	WriteFiles        bool                      `json:"writeFiles,omitempty"`
	OverwriteExisting bool                      `json:"overwriteExisting,omitempty"`
	AutoUp            bool                      `json:"autoUp,omitempty"`
	BuildFiles        []ComposeBuildFileCommand `json:"buildFiles,omitempty"`
}

type ComposeProjectCreateResult struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	OperationID int64  `json:"operationId,omitempty"`
}

type ComposeProjectUpdateRequest struct {
	ComposeYaml        string                    `json:"composeYaml"`
	ValidateBeforeSave bool                      `json:"validateBeforeSave,omitempty"`
	WriteFiles         bool                      `json:"writeFiles,omitempty"`
	BuildFiles         []ComposeBuildFileCommand `json:"buildFiles,omitempty"`
}

type DockerComposeImportDiscoveredRequest struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
}

type ComposeValidationView struct {
	Valid          bool   `json:"valid"`
	Message        string `json:"message,omitempty"`
	NormalizedYaml string `json:"normalizedYaml,omitempty"`
}

type ComposeExportView struct {
	ContainerID    string `json:"containerId"`
	ComposeManaged bool   `json:"composeManaged,omitempty"`
	ComposeProject string `json:"composeProject,omitempty"`
	ComposeService string `json:"composeService,omitempty"`
	ComposeYaml    string `json:"composeYaml,omitempty"`
	Source         string `json:"source,omitempty"`
}

type OperationStatus string

const (
	OperationStatusPending   OperationStatus = "PENDING"
	OperationStatusRunning   OperationStatus = "RUNNING"
	OperationStatusSucceeded OperationStatus = "SUCCEEDED"
	OperationStatusFailed    OperationStatus = "FAILED"
	OperationStatusCancelled OperationStatus = "CANCELLED"
	OperationStatusTimeout   OperationStatus = "TIMEOUT"
)

type OperationEventType string

const (
	OperationEventState    OperationEventType = "STATE"
	OperationEventProgress OperationEventType = "PROGRESS"
	OperationEventLog      OperationEventType = "LOG"
	OperationEventPolicy   OperationEventType = "POLICY"
	OperationEventResult   OperationEventType = "RESULT"
	OperationEventError    OperationEventType = "ERROR"

	OperationIntegrityDiagnosePermission = "admin:docker:operation-integrity:diagnose"
	OperationIntegrityCleanupPermission  = "admin:docker:operation-integrity:cleanup"
)

type OperationActor struct {
	UserID      int64    `json:"userId,omitempty"`
	Username    string   `json:"username,omitempty"`
	IsAdmin     bool     `json:"isAdmin,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

type OperationSubmitCommand struct {
	OperationType string
	TargetType    string
	TargetID      string
	TargetName    string
	Payload       any
	Actor         OperationActor
	RetryOf       int64
	Timeout       time.Duration
}

type OperationAcceptedVO struct {
	OperationID   int64           `json:"operationId"`
	OperationType string          `json:"operationType"`
	TargetType    string          `json:"targetType"`
	TargetID      string          `json:"targetId,omitempty"`
	TargetName    string          `json:"targetName,omitempty"`
	Status        OperationStatus `json:"status"`
}

type OperationVO struct {
	OperationID   int64           `json:"operationId"`
	OperationType string          `json:"operationType"`
	TargetType    string          `json:"targetType"`
	TargetID      string          `json:"targetId,omitempty"`
	TargetName    string          `json:"targetName,omitempty"`
	Status        OperationStatus `json:"status"`
	Progress      int             `json:"progress"`
	CurrentStage  string          `json:"currentStage,omitempty"`
	StartedAt     *time.Time      `json:"startedAt,omitempty"`
	FinishedAt    *time.Time      `json:"finishedAt,omitempty"`
	TimeoutAt     *time.Time      `json:"timeoutAt,omitempty"`
	Actor         OperationActor  `json:"actor"`
	RetryOf       int64           `json:"retryOf,omitempty"`
	ErrorSummary  string          `json:"errorSummary,omitempty"`
	Result        map[string]any  `json:"result,omitempty"`
	CreateTime    *time.Time      `json:"createTime,omitempty"`
	UpdateTime    *time.Time      `json:"updateTime,omitempty"`
}

type OperationEventVO struct {
	EventID     int64              `json:"eventId"`
	OperationID int64              `json:"operationId"`
	Sequence    int64              `json:"sequence"`
	Type        OperationEventType `json:"type"`
	Stage       string             `json:"stage,omitempty"`
	Percent     *int               `json:"percent,omitempty"`
	Message     string             `json:"message,omitempty"`
	Payload     map[string]any     `json:"payload,omitempty"`
	OccurredAt  time.Time          `json:"occurredAt"`
}

type OperationEventOrphanDiagnosticVO struct {
	DiagnosticID             string    `json:"diagnosticId"`
	EventID                  int64     `json:"eventId"`
	OperationID              int64     `json:"operationId"`
	Sequence                 int64     `json:"sequence"`
	ExpectedIntegrityVersion int64     `json:"expectedIntegrityVersion"`
	Scope                    string    `json:"scope"`
	RelationshipType         string    `json:"relationshipType"`
	Reason                   string    `json:"reason"`
	OccurredAt               time.Time `json:"occurredAt"`
}

type OperationEventOrphanCleanupRequest struct {
	AuditID                  int64  `json:"auditId"`
	DiagnosticID             string `json:"diagnosticId"`
	EventID                  int64  `json:"eventId"`
	OperationID              int64  `json:"operationId"`
	Sequence                 int64  `json:"sequence"`
	ExpectedIntegrityVersion int64  `json:"expectedIntegrityVersion"`
	Reason                   string `json:"reason"`
}

type PolicyAction string

const (
	PolicyActionWarn  PolicyAction = "WARN"
	PolicyActionDeny  PolicyAction = "DENY"
	PolicyActionAllow PolicyAction = "ALLOW"
)

type PolicyViolationVO struct {
	Code        string       `json:"code"`
	Severity    string       `json:"severity"`
	Action      PolicyAction `json:"action"`
	Message     string       `json:"message"`
	Field       string       `json:"field,omitempty"`
	Value       string       `json:"value,omitempty"`
	Remediation string       `json:"remediation,omitempty"`
}

type PreviewVO struct {
	Safe              bool                `json:"safe"`
	Violations        []PolicyViolationVO `json:"violations,omitempty"`
	Warnings          []PolicyViolationVO `json:"warnings,omitempty"`
	AffectedResources []string            `json:"affectedResources,omitempty"`
	NormalizedSpec    any                 `json:"normalizedSpec,omitempty"`
}

type ComposePreviewView struct {
	Preview        PreviewVO             `json:"preview"`
	Validation     ComposeValidationView `json:"validation"`
	Services       []string              `json:"services,omitempty"`
	NormalizedYaml string                `json:"normalizedYaml,omitempty"`
}

type ComposePSView struct {
	ProjectName string             `json:"projectName,omitempty"`
	ProjectID   string             `json:"projectId,omitempty"`
	Containers  []ContainerView    `json:"containers,omitempty"`
	Services    []ComposeServiceVO `json:"services,omitempty"`
}

type LatestOperationQuery struct {
	TargetType    string `json:"targetType,omitempty"`
	TargetName    string `json:"targetName,omitempty"`
	TargetID      string `json:"targetId,omitempty"`
	OperationType string `json:"operationType,omitempty"`
}

type LatestOperationView struct {
	Operation *OperationVO       `json:"operation,omitempty"`
	Events    []OperationEventVO `json:"events,omitempty"`
}

type DockerResourceView struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Driver      string            `json:"driver,omitempty"`
	Scope       string            `json:"scope,omitempty"`
	CreatedAt   string            `json:"createdAt,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Dangling    bool              `json:"dangling,omitempty"`
	SizeBytes   int64             `json:"sizeBytes,omitempty"`
	Description string            `json:"description,omitempty"`
	Internal    bool              `json:"internal,omitempty"`
	Attachable  bool              `json:"attachable,omitempty"`
	Ingress     bool              `json:"ingress,omitempty"`
	IPv6        bool              `json:"ipv6,omitempty"`
	Mountpoint  string            `json:"mountpoint,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	Containers  map[string]any    `json:"containers,omitempty"`
}

type DockerNetworkIPAMConfigRequest struct {
	Subnet     string            `json:"subnet,omitempty"`
	IPRange    string            `json:"ipRange,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	AuxAddress map[string]string `json:"auxAddress,omitempty"`
}

type DockerNetworkCreateRequest struct {
	Name        string                           `json:"name"`
	Driver      string                           `json:"driver,omitempty"`
	Scope       string                           `json:"scope,omitempty"`
	EnableIPv4  *bool                            `json:"enableIpv4,omitempty"`
	EnableIPv6  *bool                            `json:"enableIpv6,omitempty"`
	Internal    bool                             `json:"internal,omitempty"`
	Attachable  bool                             `json:"attachable,omitempty"`
	Ingress     bool                             `json:"ingress,omitempty"`
	Options     map[string]string                `json:"options,omitempty"`
	Labels      map[string]string                `json:"labels,omitempty"`
	IPAMDriver  string                           `json:"ipamDriver,omitempty"`
	IPAMOptions map[string]string                `json:"ipamOptions,omitempty"`
	IPAMConfigs []DockerNetworkIPAMConfigRequest `json:"ipamConfigs,omitempty"`
}

type DockerNetworkConnectRequest struct {
	ContainerID  string            `json:"containerId"`
	Aliases      []string          `json:"aliases,omitempty"`
	Links        []string          `json:"links,omitempty"`
	IPv4Address  string            `json:"ipv4Address,omitempty"`
	IPv6Address  string            `json:"ipv6Address,omitempty"`
	LinkLocalIPs []string          `json:"linkLocalIps,omitempty"`
	MacAddress   string            `json:"macAddress,omitempty"`
	DriverOpts   map[string]string `json:"driverOpts,omitempty"`
}

type DockerNetworkDisconnectRequest struct {
	ContainerID string `json:"containerId"`
	Force       bool   `json:"force,omitempty"`
}

type DockerNetworkDetailView struct {
	Resource   DockerResourceView `json:"resource"`
	Inspect    map[string]any     `json:"inspect"`
	Containers map[string]any     `json:"containers,omitempty"`
	Options    map[string]string  `json:"options,omitempty"`
}

type DockerVolumeCreateRequest struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	DriverOpts map[string]string `json:"driverOpts,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type DockerVolumeDeleteRequest struct {
	Force bool `json:"force,omitempty"`
}

type DockerVolumeDetailView struct {
	Resource   DockerResourceView `json:"resource"`
	Inspect    map[string]any     `json:"inspect"`
	Mountpoint string             `json:"mountpoint,omitempty"`
	Options    map[string]string  `json:"options,omitempty"`
	Status     map[string]any     `json:"status,omitempty"`
}

type DockerResourcePrunePreview struct {
	PreviewToken string   `json:"previewToken,omitempty"`
	Count        int      `json:"count"`
	ResourceIDs  []string `json:"resourceIds"`
	ReclaimBytes int64    `json:"reclaimBytes"`
}

func (r DockerNetworkCreateRequest) Normalize() (DockerNetworkCreateRequest, error) {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return r, apperrors.Params("network name 不能为空")
	}
	r.Driver = strings.TrimSpace(r.Driver)
	r.Scope = strings.TrimSpace(r.Scope)
	r.IPAMDriver = strings.TrimSpace(r.IPAMDriver)
	r.Options = cleanDockerStringMap(r.Options)
	r.Labels = cleanDockerStringMap(r.Labels)
	r.IPAMOptions = cleanDockerStringMap(r.IPAMOptions)
	for idx := range r.IPAMConfigs {
		r.IPAMConfigs[idx].Subnet = strings.TrimSpace(r.IPAMConfigs[idx].Subnet)
		r.IPAMConfigs[idx].IPRange = strings.TrimSpace(r.IPAMConfigs[idx].IPRange)
		r.IPAMConfigs[idx].Gateway = strings.TrimSpace(r.IPAMConfigs[idx].Gateway)
		r.IPAMConfigs[idx].AuxAddress = cleanDockerStringMap(r.IPAMConfigs[idx].AuxAddress)
	}
	return r, nil
}

func (r DockerNetworkConnectRequest) Normalize(networkID string) (DockerNetworkConnectRequest, string, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return r, networkID, apperrors.Params("network id 不能为空")
	}
	r.ContainerID = strings.TrimSpace(r.ContainerID)
	if r.ContainerID == "" {
		return r, networkID, apperrors.Params("containerId 不能为空")
	}
	r.Aliases = cleanDockerStrings(r.Aliases)
	r.Links = cleanDockerStrings(r.Links)
	r.LinkLocalIPs = cleanDockerStrings(r.LinkLocalIPs)
	r.IPv4Address = strings.TrimSpace(r.IPv4Address)
	r.IPv6Address = strings.TrimSpace(r.IPv6Address)
	r.MacAddress = strings.TrimSpace(r.MacAddress)
	r.DriverOpts = cleanDockerStringMap(r.DriverOpts)
	if r.IPv4Address != "" {
		if ip := net.ParseIP(r.IPv4Address); ip == nil || ip.To4() == nil || ip.IsUnspecified() {
			return r, networkID, apperrors.Params("ipv4Address 非法")
		}
	}
	if r.IPv6Address != "" {
		if ip := net.ParseIP(r.IPv6Address); ip == nil || ip.To4() != nil || ip.IsUnspecified() {
			return r, networkID, apperrors.Params("ipv6Address 非法")
		}
	}
	for _, value := range r.LinkLocalIPs {
		if ip := net.ParseIP(value); ip == nil || ip.IsUnspecified() {
			return r, networkID, apperrors.Params("linkLocalIps 包含非法 IP")
		}
	}
	return r, networkID, nil
}

func (r DockerNetworkDisconnectRequest) Normalize(networkID string) (DockerNetworkDisconnectRequest, string, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return r, networkID, apperrors.Params("network id 不能为空")
	}
	r.ContainerID = strings.TrimSpace(r.ContainerID)
	if r.ContainerID == "" {
		return r, networkID, apperrors.Params("containerId 不能为空")
	}
	return r, networkID, nil
}

func (r DockerVolumeCreateRequest) Normalize() (DockerVolumeCreateRequest, error) {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return r, apperrors.Params("volume name 不能为空")
	}
	r.Driver = strings.TrimSpace(r.Driver)
	r.DriverOpts = cleanDockerStringMap(r.DriverOpts)
	r.Labels = cleanDockerStringMap(r.Labels)
	return r, nil
}

func normalizeDockerResourceID(id, resourceType string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", apperrors.Params(resourceType + " id 不能为空")
	}
	return id, nil
}

func newDockerResourcePrunePreview(resourceType string, resourceIDs []string, reclaimBytes int64) DockerResourcePrunePreview {
	copied := append([]string{}, resourceIDs...)
	sort.Strings(copied)
	return DockerResourcePrunePreview{
		PreviewToken: cleanupToken(resourceType, copied),
		Count:        len(copied),
		ResourceIDs:  copied,
		ReclaimBytes: reclaimBytes,
	}
}

func cleanDockerStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cleaned := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		cleaned[key] = strings.TrimSpace(value)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func cleanDockerStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

type CleanupPreviewRequest struct {
	UntilHours int64 `json:"untilHours,omitempty"`
}

type CleanupApplyRequest struct {
	PreviewToken string `json:"previewToken,omitempty"`
	UntilHours   int64  `json:"untilHours,omitempty"`
}

type CleanupPreviewVO struct {
	PreviewToken      string   `json:"previewToken"`
	ResourceType      string   `json:"resourceType"`
	AffectedResources []string `json:"affectedResources"`
	EstimatedBytes    int64    `json:"estimatedBytes,omitempty"`
	Warning           string   `json:"warning,omitempty"`
}

type DockerMetricsSnapshot struct {
	Enabled               bool             `json:"enabled"`
	DaemonHealthy         bool             `json:"daemonHealthy"`
	RegistryHealthy       bool             `json:"registryHealthy"`
	ContainerCountByState map[string]int64 `json:"containerCountByState,omitempty"`
	ImageCount            int64            `json:"imageCount"`
	ImageSizeBytes        int64            `json:"imageSizeBytes"`
	OperationTotal        int64            `json:"operationTotal"`
	OperationSucceeded    int64            `json:"operationSucceeded"`
	OperationFailed       int64            `json:"operationFailed"`
	PolicyViolationTotal  int64            `json:"policyViolationTotal"`
	LatestFailed          []OperationVO    `json:"latestFailed,omitempty"`
	TopSlowOperations     []OperationVO    `json:"topSlowOperations,omitempty"`
}

type Service interface {
	Enabled() bool
	Ping(ctx context.Context) error
	Close() error
	ListContainers(ctx context.Context, current, size int64, keyword, state string) (*PageResult[ContainerView], error)
	GetContainer(ctx context.Context, id string) (*ContainerDetailView, error)
	GetContainerLogs(ctx context.Context, id string, tail int) (string, error)
	GetContainerLogsQuery(ctx context.Context, id string, query ContainerLogQuery) (string, error)
	StreamContainerLogs(ctx context.Context, id string, query ContainerLogQuery) (io.ReadCloser, error)
	GetContainerStats(ctx context.Context, id string) (map[string]any, error)
	StreamContainerStats(ctx context.Context, id string) (io.ReadCloser, error)
	ServeContainerTerminal(ctx context.Context, writer http.ResponseWriter, request *http.Request, id string, command ContainerTerminalRequest) error
	ValidateContainerOperation(ctx context.Context, id, operationType string) error
	StartContainer(ctx context.Context, id string) (bool, error)
	StopContainer(ctx context.Context, id string) (bool, error)
	RestartContainer(ctx context.Context, id string) (bool, error)
	DeleteContainer(ctx context.Context, id string) (bool, error)
	ListImages(ctx context.Context, current, size int64, keyword string) (*PageResult[ImageView], error)
	GetImage(ctx context.Context, id string) (*ImageDetailView, error)
	ListImageContainers(ctx context.Context, imageID string) ([]ContainerUsageView, error)
	PullImage(ctx context.Context, command ImagePullCommand) (bool, error)
	TagImage(ctx context.Context, command ImageTagCommand) (bool, error)
	PushImage(ctx context.Context, command ImagePushCommand) (bool, error)
	DeleteImage(ctx context.Context, id string) (bool, error)
	ExportImage(ctx context.Context, id string) (map[string]any, error)
	PullRemoteImage(ctx context.Context, command RemoteImagePullRequest) (bool, error)
	StartupPreview(ctx context.Context, imageID string) (*ImageStartupPreview, error)
	CreateContainerFromImage(ctx context.Context, command ContainerCreateRequest) (string, error)
	ValidateCompose(ctx context.Context, command ComposeUpRequest) (*ComposeValidationView, error)
	UpCompose(ctx context.Context, command ComposeUpRequest) (bool, error)
	ExportCompose(ctx context.Context, containerID string) (*ComposeExportView, error)
	PreviewCompose(ctx context.Context, actor OperationActor, command ComposeUpRequest) (*ComposePreviewView, error)
	DownCompose(ctx context.Context, command ComposeUpRequest) (bool, error)
	RestartCompose(ctx context.Context, command ComposeUpRequest) (bool, error)
	ComposePS(ctx context.Context, command ComposeUpRequest) (*ComposePSView, error)
	ComposeLogs(ctx context.Context, command ComposeUpRequest, tail int) (string, error)
	ListComposeProjects(ctx context.Context, current, size int64, keyword, status string) (*PageResult[ComposeProjectSummaryVO], error)
	GetComposeProject(ctx context.Context, projectID string) (*ComposeProjectDetailVO, error)
	CreateComposeProject(ctx context.Context, actor OperationActor, request ComposeProjectCreateRequest) (*ComposeProjectCreateResult, error)
	ImportDiscoveredComposeProject(ctx context.Context, actor OperationActor, request DockerComposeImportDiscoveredRequest) (*ComposeProjectDetailVO, error)
	UpdateComposeProjectCompose(ctx context.Context, actor OperationActor, projectID string, request ComposeProjectUpdateRequest) (*ComposeProjectDetailVO, error)
	CheckComposeWorkspace(ctx context.Context, request ComposeWorkspaceCheckRequest) (*ComposeWorkspaceCheckView, error)
	ValidateComposeYaml(ctx context.Context, request ComposeYamlValidateRequest) (*ComposeYamlValidateView, error)
	GetComposeBuilderMetadata(ctx context.Context) (*ComposeBuilderMetadataView, error)
	PreviewDockerfileBuild(ctx context.Context, request DockerfileBuildPreviewRequest) (*DockerfileBuildPreviewView, error)
	PreviewComposeWithFiles(ctx context.Context, actor OperationActor, request ComposePreviewWithFilesRequest) (*ComposePreviewView, error)
	PreviewComposeProject(ctx context.Context, actor OperationActor, projectID string) (*ComposePreviewView, error)
	ValidateComposeProject(ctx context.Context, actor OperationActor, projectID string) (*ComposePreviewView, error)
	ComposeProjectPS(ctx context.Context, projectID string) (*ComposePSView, error)
	ValidateComposeProjectOperation(ctx context.Context, projectID, operationType string) error
	SubmitComposeProjectOperation(ctx context.Context, actor OperationActor, projectID, operationType string, tail int) (*OperationAcceptedVO, error)
	ListRegistries(ctx context.Context) ([]RemoteRegistryView, error)
	GetRegistry(ctx context.Context, id int64) (*RemoteRegistryView, error)
	CreateRegistry(ctx context.Context, command RemoteRegistryCommand) (int64, error)
	UpdateRegistry(ctx context.Context, id int64, command RemoteRegistryCommand) (bool, error)
	DeleteRegistry(ctx context.Context, id int64) (bool, error)
	TestRegistry(ctx context.Context, id int64) (*RegistryConnectionTestView, error)
	ListRepositories(ctx context.Context, id, current, size int64, keyword string) (*PageResult[RemoteRepositoryView], error)
	ListTags(ctx context.Context, id int64, repository string) (*RemoteTagsView, error)
	GetManifest(ctx context.Context, id int64, repository, reference string) (*RemoteManifestView, error)
	ResolveDigest(ctx context.Context, id int64, repository, reference string) (string, error)
	ListVolumes(ctx context.Context, current, size int64, keyword string) (*PageResult[DockerResourceView], error)
	ListNetworks(ctx context.Context, current, size int64, keyword string) (*PageResult[DockerResourceView], error)
	GetNetwork(ctx context.Context, id string) (*DockerNetworkDetailView, error)
	CreateNetwork(ctx context.Context, request DockerNetworkCreateRequest) (*DockerResourceView, error)
	DeleteNetwork(ctx context.Context, id string) (bool, error)
	ConnectNetwork(ctx context.Context, id string, request DockerNetworkConnectRequest) (bool, error)
	DisconnectNetwork(ctx context.Context, id string, request DockerNetworkDisconnectRequest) (bool, error)
	PreviewNetworkPrune(ctx context.Context, request CleanupPreviewRequest) (*DockerResourcePrunePreview, error)
	ApplyNetworkPrune(ctx context.Context, request CleanupApplyRequest) (map[string]any, error)
	GetVolume(ctx context.Context, name string) (*DockerVolumeDetailView, error)
	CreateVolume(ctx context.Context, request DockerVolumeCreateRequest) (*DockerResourceView, error)
	DeleteVolume(ctx context.Context, name string, request DockerVolumeDeleteRequest) (bool, error)
	PreviewVolumePrune(ctx context.Context, request CleanupPreviewRequest) (*DockerResourcePrunePreview, error)
	ApplyVolumePrune(ctx context.Context, request CleanupApplyRequest) (map[string]any, error)
	PreviewImageCleanup(ctx context.Context, request CleanupPreviewRequest) (*CleanupPreviewVO, error)
	PreviewContainerCleanup(ctx context.Context, request CleanupPreviewRequest) (*CleanupPreviewVO, error)
	SubmitOperation(ctx context.Context, command OperationSubmitCommand) (*OperationAcceptedVO, error)
	ListOperations(ctx context.Context, current, size int64, status, operationType string) (*PageResult[OperationVO], error)
	LatestOperation(ctx context.Context, query LatestOperationQuery) (*LatestOperationView, error)
	GetOperation(ctx context.Context, operationID int64) (*OperationVO, error)
	ListOperationEvents(ctx context.Context, operationID, afterSequence int64, limit int) ([]OperationEventVO, error)
	DiagnoseOperationEventOrphans(ctx context.Context, afterEventID int64, limit int, actor OperationActor) ([]OperationEventOrphanDiagnosticVO, error)
	CleanupOperationEventOrphan(ctx context.Context, request OperationEventOrphanCleanupRequest, actor OperationActor) (OperationEventOrphanCleanupResult, error)
	StreamOperation(ctx context.Context, operationID, afterSequence int64) (io.ReadCloser, error)
	CancelOperation(ctx context.Context, operationID int64, actor OperationActor) (bool, error)
	RetryOperation(ctx context.Context, operationID int64, actor OperationActor) (*OperationAcceptedVO, error)
	MetricsSnapshot(ctx context.Context) (*DockerMetricsSnapshot, error)
	GetDaemonConfig(ctx context.Context) (*DockerDaemonConfigView, error)
	ValidateDaemonConfig(ctx context.Context, request DockerDaemonConfigUpdateRequest) (*DockerDaemonConfigValidateView, error)
	SaveDaemonConfig(ctx context.Context, request DockerDaemonConfigUpdateRequest) (*DockerDaemonConfigView, error)
	RestartDaemon(ctx context.Context) (bool, error)
}
