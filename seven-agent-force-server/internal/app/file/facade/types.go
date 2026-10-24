package facade

import (
	"io"
	"strconv"
	"time"
)

type FileActionBiz int

const (
	UserAvatar  FileActionBiz = 0
	DefaultFile FileActionBiz = 1
)

type FileAssetSlot string

const (
	FileAssetSlotUserAvatar FileAssetSlot = "USER_AVATAR"
	// FileAssetSlotConfigAsset is intentionally not accepted by the generic
	// BindUploadedFile command. Configuration assets use the dedicated facade
	// below so a caller can never supply reference ownership or access policy.
	FileAssetSlotConfigAsset FileAssetSlot = "CONFIG_ASSET"
)

// ConfigAssetBizType is a server-owned sys_file_reference bizType. It is a
// literal rather than an integer FileActionBiz because the configuration
// application owns the configId target and its policy mapping.
const ConfigAssetBizType = "CONFIG_ASSET"
const WorkforceInputBizType = "WORKFORCE_INPUT"

type WorkforceInputKind string

const (
	WorkforceInputWorkspaceArchive WorkforceInputKind = "WORKSPACE_ARCHIVE"
	WorkforceInputReference        WorkforceInputKind = "REFERENCE"
)

type ClaimWorkforceInputCommand struct {
	FileID      int64
	Kind        WorkforceInputKind
	DisplayName string
}

type WorkforceInputClaim struct {
	Handle        string
	ScopeID       string
	FileName      string
	ContentType   string
	ContentSHA256 string
	Size          int64
}

type OpenWorkforceInputCommand struct {
	Handle          string
	ExpectedScopeID string
}

type WorkforceInputOpenResult struct {
	Reader        io.ReadCloser
	Size          int64
	ContentType   string
	FileName      string
	ContentSHA256 string
}

type ReleaseWorkforceInputCommand struct {
	Handle          string
	ExpectedScopeID string
}

type ConfigAssetType string

const (
	ConfigAssetImage ConfigAssetType = "IMAGE"
	ConfigAssetFile  ConfigAssetType = "FILE"
)

type ConfigAssetExposure string

const (
	ConfigAssetInternal      ConfigAssetExposure = "INTERNAL"
	ConfigAssetAuthenticated ConfigAssetExposure = "AUTHENTICATED"
	ConfigAssetPublic        ConfigAssetExposure = "PUBLIC"
)

type BindUploadedFileCommand struct {
	FileID      int64
	Slot        FileAssetSlot
	DisplayName string
}

// BindConfigAssetCommand is application-to-application only. ConfigID,
// AssetType, and Exposure are derived by the system-config application after
// its own authorization and validation; no HTTP request field maps to a file
// reference id, owner, scope, visit URL, or access policy.
type BindConfigAssetCommand struct {
	FileID    int64
	ConfigID  int64
	AssetType ConfigAssetType
	Exposure  ConfigAssetExposure
}

// UpdateConfigAssetPolicyCommand updates the derived reference policy when a
// configuration asset's exposure changes without changing its file.
type UpdateConfigAssetPolicyCommand struct {
	ConfigID  int64
	AssetType ConfigAssetType
	Exposure  ConfigAssetExposure
}

// ConfigAssetBindingState is a private, server-to-server description of the
// single CONFIG_ASSET slot owned by a configuration. It intentionally has no
// JSON representation: file IDs, scope IDs, and policy state must never enter
// management/history response objects or become request authority.
//
// It is captured from sys_file_reference by the file application and is only
// accepted back by RestoreConfigAssetBinding when supplied from the
// configuration application's private audit snapshot.
type ConfigAssetBindingState struct {
	ConfigID  int64                  `json:"-"`
	State     ConfigAssetBindingKind `json:"-"`
	FileID    int64                  `json:"-"`
	ScopeID   string                 `json:"-"`
	AssetType ConfigAssetType        `json:"-"`
	Exposure  ConfigAssetExposure    `json:"-"`
}

type ConfigAssetBindingKind string

const (
	ConfigAssetBindingEmpty ConfigAssetBindingKind = "EMPTY"
	ConfigAssetBindingBound ConfigAssetBindingKind = "BOUND"
)

// CaptureConfigAssetBindingCommand contains only metadata already normalized
// by system-config. It never accepts a file ID, reference ID, scope, user, or
// access policy from a HTTP request.
type CaptureConfigAssetBindingCommand struct {
	ConfigID  int64
	AssetType ConfigAssetType
	Exposure  ConfigAssetExposure
}

// RestoreConfigAssetBindingCommand is deliberately narrower than the normal
// upload binder. AssetType and Exposure describe the expected *current* state;
// Restore carries the prior server-derived policy and may differ only in
// exposure for a policy-history rollback. Both states must originate from the
// private configuration audit record; this operation does not accept an upload
// credential and is not exposed as a HTTP/file-management command.
type RestoreConfigAssetBindingCommand struct {
	ConfigID  int64
	AssetType ConfigAssetType
	Exposure  ConfigAssetExposure
	Expected  ConfigAssetBindingState
	Restore   ConfigAssetBindingState
}

// ConfigAssetOpenResult is intentionally a stream-only cross-module result.
// It never contains a storage path, a reference ID, a download token, or a
// URL that callers could persist or expose as authority.
type ConfigAssetOpenResult struct {
	Reader      io.ReadCloser
	Size        int64
	ContentType string
	FileName    string
	AssetType   ConfigAssetType
	AccessScope ConfigAssetExposure
}

// ConfigAssetStablePath is the only persisted presentation value for an
// IMAGE/FILE system configuration. It is same-origin, stable across file
// replacement, and contains neither a file ID nor a physical storage path.
func ConfigAssetStablePath(configID int64) string {
	if configID <= 0 {
		return ""
	}
	return "/api/config-assets/" + strconv.FormatInt(configID, 10)
}

type FileAccessScope string

const (
	AccessOwnerOnly  FileAccessScope = "OWNER_ONLY"
	AccessDelegated  FileAccessScope = "DELEGATED"
	AccessLoginUsers FileAccessScope = "LOGIN_USERS"
	AccessPublic     FileAccessScope = "PUBLIC"
)

type FileVisitStrategy string

const (
	VisitPublicStatic    FileVisitStrategy = "PUBLIC_STATIC"
	VisitPrivatePreview  FileVisitStrategy = "PRIVATE_PREVIEW"
	VisitPrivateDownload FileVisitStrategy = "PRIVATE_DOWNLOAD"
	VisitForbidden       FileVisitStrategy = "FORBIDDEN"
)

type FileInfoDTO struct {
	ID                int64  `json:"id"`
	FileInnerName     string `json:"fileInnerName"`
	FileSize          int64  `json:"fileSize"`
	FileSha256        string `json:"fileSha256"`
	ContentType       string `json:"contentType"`
	StorageStrategyID int64  `json:"storageStrategyId"`
	StoragePath       string `json:"storagePath"`
	Status            string `json:"status"`
	ScanStatus        string `json:"scanStatus"`
}

type FileReferenceDTO struct {
	ID            int64  `json:"id,omitempty"`
	FileID        int64  `json:"fileId"`
	UserID        int64  `json:"userId"`
	DisplayName   string `json:"displayName"`
	VisitURL      string `json:"visitUrl,omitempty"`
	BizType       string `json:"bizType"`
	BizID         int64  `json:"bizId"`
	VisitStrategy string `json:"visitStrategy,omitempty"`
	AccessScope   string `json:"accessScope,omitempty"`
}

type PageResult[T any] struct {
	Current int64 `json:"current"`
	Size    int64 `json:"size"`
	Total   int64 `json:"total"`
	Records []T   `json:"records"`
}

type FileInfoVO struct {
	ID                int64      `json:"id"`
	FileInnerName     string     `json:"fileInnerName"`
	FileSize          int64      `json:"fileSize"`
	FileSha256        string     `json:"fileSha256"`
	ContentType       string     `json:"contentType"`
	StorageType       int        `json:"storageType"`
	StorageStrategyID int64      `json:"storageStrategyId"`
	StoragePath       string     `json:"storagePath"`
	Status            string     `json:"status"`
	ScanStatus        string     `json:"scanStatus"`
	CreateTime        *time.Time `json:"createTime,omitempty"`
	UpdateTime        *time.Time `json:"updateTime,omitempty"`
	IsDeleted         int        `json:"isDeleted"`
}

type FileReferenceVO struct {
	ID            int64      `json:"id"`
	FileID        int64      `json:"fileId"`
	UserID        int64      `json:"userId"`
	DisplayName   string     `json:"displayName"`
	BizType       string     `json:"bizType"`
	BizID         int64      `json:"bizId"`
	VisitURL      string     `json:"visitUrl,omitempty"`
	AccessLevel   int        `json:"accessLevel"`
	VisitStrategy string     `json:"visitStrategy,omitempty"`
	AccessScope   string     `json:"accessScope,omitempty"`
	CreateTime    *time.Time `json:"createTime,omitempty"`
}

type StorageStrategyVO struct {
	ID                   int64      `json:"id"`
	StrategyName         string     `json:"strategyName"`
	ProviderType         string     `json:"providerType"`
	IsDefault            bool       `json:"isDefault"`
	IsEnabled            bool       `json:"isEnabled"`
	RunState             string     `json:"runState"`
	Priority             int        `json:"priority"`
	ConfigJSON           string     `json:"configJson,omitempty"`
	HealthCheckURL       string     `json:"healthCheckUrl,omitempty"`
	HealthStatus         int        `json:"healthStatus"`
	LastHealthCheck      *time.Time `json:"lastHealthCheck,omitempty"`
	FailureCount         int        `json:"failureCount"`
	TotalRequests        int        `json:"totalRequests"`
	FailureRateThreshold float64    `json:"failureRateThreshold"`
}

type StorageStrategyHealthVO struct {
	StrategyID      int64      `json:"strategyId"`
	HealthStatus    int        `json:"healthStatus"`
	Healthy         bool       `json:"healthy"`
	Message         string     `json:"message"`
	LastHealthCheck *time.Time `json:"lastHealthCheck,omitempty"`
}

type FileProcessTaskVO struct {
	ID            int64      `json:"id"`
	FileID        int64      `json:"fileId"`
	TaskType      string     `json:"taskType"`
	TaskParams    string     `json:"taskParams,omitempty"`
	Status        int        `json:"status"`
	RetryCount    int        `json:"retryCount"`
	MaxRetry      int        `json:"maxRetry"`
	ErrorMsg      string     `json:"errorMsg,omitempty"`
	ResultData    string     `json:"resultData,omitempty"`
	Priority      int        `json:"priority"`
	MQMessageID   string     `json:"mqMessageId,omitempty"`
	NextRetryTime *time.Time `json:"nextRetryTime,omitempty"`
	CreateTime    *time.Time `json:"createTime,omitempty"`
	UpdateTime    *time.Time `json:"updateTime,omitempty"`
	StartTime     *time.Time `json:"startTime,omitempty"`
	FinishTime    *time.Time `json:"finishTime,omitempty"`
}
