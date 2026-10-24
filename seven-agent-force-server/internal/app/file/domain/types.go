package domain

import (
	"io"
	"strings"
	"time"
)

const (
	FileStatusPendingBind = "PENDING_BIND"
	FileStatusBindFailed  = "BIND_FAILED"
	FileStatusAvailable   = "AVAILABLE"
	FileStatusCleaning    = "CLEANING"
	FileStatusQuarantined = "QUARANTINED"
	FileStatusDeleted     = "DELETED"
	FileStatusBroken      = "BROKEN"

	ScanStatusPending      = "PENDING"
	ScanStatusClean        = "CLEAN"
	ScanStatusError        = "ERROR"
	ScanStatusInfected     = "INFECTED"
	ScanStatusMimeRejected = "MIME_REJECTED"
	ScanStatusDLPRejected  = "DLP_REJECTED"
	ScanStatusTimeout      = "SCAN_TIMEOUT"

	IntegrityPending      = "PENDING"
	IntegrityVerified     = "VERIFIED"
	IntegrityMismatch     = "MISMATCH"
	IntegrityHashMismatch = "HASH_MISMATCH"
	IntegrityCRCMismatch  = "CRC_MISMATCH"
	IntegrityError        = "ERROR"

	UploadTaskInit        = "INIT"
	UploadTaskUploading   = "UPLOADING"
	UploadTaskUploaded    = "UPLOADED"
	UploadTaskProcessing  = "PROCESSING"
	UploadTaskPendingBind = "PENDING_BIND"
	UploadTaskClean       = "CLEAN"
	UploadTaskRejected    = "REJECTED"
	UploadTaskFailed      = "FAILED"
	UploadTaskExpired     = "EXPIRED"

	UploadCredentialVersion1 = 1

	BindingPending   = "PENDING"
	BindingBound     = "BOUND"
	BindingFailed    = "FAILED"
	BindingCancelled = "CANCELLED"

	ProviderLocal      = "LOCAL"
	ProviderTencentCOS = "TENCENT_COS"
	ProviderAliyunOSS  = "ALIYUN_OSS"
	ProviderAWSS3      = "AWS_S3"

	RunStateActive   = "ACTIVE"
	RunStateDraining = "DRAINING"
	RunStateDisabled = "DISABLED"

	HealthUnhealthy = 0
	HealthHealthy   = 1
	HealthDegraded  = 2

	ProcessTaskPending      = 0
	ProcessTaskProcessing   = 1
	ProcessTaskCompleted    = 2
	ProcessTaskFailed       = 3
	ProcessTaskPendingRetry = 4

	ChunkStatusInit      = 0
	ChunkStatusUploading = 1
	ChunkStatusCompleted = 2
	ChunkStatusAborted   = 3
	ChunkStatusExpired   = 4

	FailureNone      = "NONE"
	FailureMime      = "MIME"
	FailureDLP       = "DLP"
	FailureMalware   = "MALWARE"
	FailureBinding   = "BINDING"
	FailureIntegrity = "INTEGRITY"
	FailureStorage   = "STORAGE"
	FailureSystem    = "SYSTEM"
)

type FileBiz struct {
	Code      int
	Name      string
	Label     string
	RoutePath string
	MaxSize   int64
	Suffixes  map[string]bool
}

var (
	DefaultFileBiz = FileBiz{
		Code:      1,
		Name:      "默认文件",
		Label:     "default_file",
		RoutePath: "file",
		MaxSize:   1024 * 1024 * 1024,
		Suffixes:  suffixSet("jpeg", "jpg", "png", "webp", "gif", "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "zip", "rar", "7z", "mp4", "mov"),
	}
	UserAvatarBiz = FileBiz{
		Code:      0,
		Name:      "用户头像",
		Label:     "user_avatar",
		RoutePath: "avatar",
		MaxSize:   2 * 1024 * 1024,
		Suffixes:  suffixSet("jpeg", "jpg", "svg", "png", "webp"),
	}
)

func suffixSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func ResolveBiz(code int) (FileBiz, bool) {
	switch code {
	case UserAvatarBiz.Code:
		return UserAvatarBiz, true
	case DefaultFileBiz.Code:
		return DefaultFileBiz, true
	default:
		return FileBiz{}, false
	}
}

type StorageStrategy struct {
	ID                   int64
	StrategyName         string
	ProviderType         string
	IsDefault            bool
	IsEnabled            bool
	RunState             string
	Priority             int
	ConfigCiphertext     string
	ConfigEDEK           string
	WrapKeyRef           string
	HealthCheckURL       string
	HealthStatus         int
	LastHealthCheck      *time.Time
	FailureCount         int
	TotalRequests        int
	FailureRateThreshold float64
	CreateTime           time.Time
	UpdateTime           time.Time
	IsDeleted            int
}

type StorageHealthUpdate struct {
	StrategyID   int64
	HealthStatus int
	Healthy      bool
}

func (s StorageStrategy) Writable() bool {
	return s.ID > 0 && s.IsEnabled && s.IsDeleted == 0 && s.RunState == RunStateActive
}

func (s StorageStrategy) Readable() bool {
	return s.ID > 0 && s.IsDeleted == 0
}

type FileInfo struct {
	ID                int64
	FileInnerName     string
	FileSize          int64
	FileSha256        string
	FileCrc32c        string
	HashAlgorithm     string
	ContentType       string
	StorageType       int
	StorageStrategyID int64
	StoragePath       string
	Status            string
	ScanStatus        string
	IntegrityStatus   string
	CreateTime        time.Time
	UpdateTime        time.Time
	IsDeleted         int
	DeletedTime       *time.Time
}

type Page[T any] struct {
	Current int64
	Size    int64
	Total   int64
	Records []T
}

type StoredObject struct {
	StoragePath string
	Size        int64
	SHA256      string
	ContentType string
}

type DownloadObject struct {
	File        io.ReadCloser
	Size        int64
	ModTime     time.Time
	ContentType string
	Name        string
}

type FileReference struct {
	ID            int64
	FileID        int64
	UserID        int64
	ScopeID       string
	DisplayName   string
	BizType       string
	BizID         int64
	VisitURL      string
	AccessLevel   int
	VisitStrategy string
	AccessScope   string
	CreateTime    time.Time
	UpdateTime    time.Time
	IsDeleted     int
}

type UploadTask struct {
	ID                 string
	UserID             int64
	ScopeID            string
	CredentialID       string
	CredentialVersion  int
	BizType            int
	BizID              int64
	FileName           string
	ContentType        string
	StorageStrategyID  int64
	ObjectKeyStaging   string
	ObjectKeyClean     string
	Status             string
	UploadMode         string
	MultipartUploadID  string
	PartSize           int64
	TotalParts         int
	ExpectedSize       int64
	ExpectedSha256     string
	ExpectedCrc32c     string
	ActualSize         int64
	ETag               string
	ServerCrc32c       string
	FailureCategory    string
	FailureReason      string
	FileID             int64
	BindingToken       string
	BindingChannel     string
	ExpireAt           *time.Time
	ProtectedUntil     *time.Time
	CredentialExpireAt *time.Time
	RevokedAt          *time.Time
	UserIP             string
	CreateTime         time.Time
	UpdateTime         time.Time
}

type ChunkUpload struct {
	ID                int64
	UploadID          string
	UserID            int64
	ScopeID           string
	UploadTaskID      string
	FileName          string
	FileSize          int64
	ChunkSize         int
	TotalChunks       int
	UploadedChunks    []int
	ChunkSha256Map    map[int]string
	PartETagsMap      map[int]string
	FileSha256        string
	ExpectedCrc32c    string
	ServerCrc32c      string
	StorageStrategyID int64
	TempStoragePath   string
	CloudUploadID     string
	BizType           string
	BizID             int64
	ContentType       string
	Status            int
	ExpireTime        time.Time
	CreateTime        time.Time
	UpdateTime        time.Time
}

func (t UploadTask) Authorizes(userID int64, scopeID string, fileID int64, now time.Time) bool {
	if userID <= 0 || fileID <= 0 || t.UserID != userID || t.FileID != fileID {
		return false
	}
	if strings.TrimSpace(scopeID) == "" || strings.TrimSpace(t.ScopeID) != strings.TrimSpace(scopeID) {
		return false
	}
	if t.Status != UploadTaskClean || t.CredentialVersion < UploadCredentialVersion1 || strings.TrimSpace(t.CredentialID) == "" {
		return false
	}
	if t.RevokedAt != nil && !t.RevokedAt.After(now) {
		return false
	}
	return t.CredentialExpireAt != nil && t.CredentialExpireAt.After(now)
}

func (t UploadTask) Protects(fileID int64, now time.Time) bool {
	if fileID <= 0 || t.FileID != fileID || t.Status != UploadTaskClean {
		return false
	}
	if t.RevokedAt != nil && !t.RevokedAt.After(now) {
		return false
	}
	return t.ProtectedUntil != nil && t.ProtectedUntil.After(now)
}

type FileProcessTask struct {
	ID             int64
	FileID         int64
	TaskType       string
	TaskParams     string
	PipelineID     string
	NodeID         string
	IdempotencyKey string
	DedupKey       string
	ReplayToken    string
	DependsOn      string
	Status         int
	RetryCount     int
	MaxRetry       int
	ErrorMsg       string
	ResultData     string
	Priority       int
	MQMessageID    string
	NextRetryTime  *time.Time
	CreateTime     time.Time
	UpdateTime     time.Time
	StartTime      *time.Time
	FinishTime     *time.Time
}

type FileProcessRun struct {
	ID         int64
	TaskID     int64
	FileID     int64
	TaskType   string
	Status     int
	Attempt    int
	ErrorMsg   string
	ResultData string
	StartedAt  time.Time
	FinishedAt *time.Time
	CreateTime time.Time
}

type FileBindingTask struct {
	ID            int64
	FileID        int64
	UserID        int64
	BizType       int
	BizID         int64
	BindingToken  string
	Channel       string
	Status        string
	AttemptCount  int
	NextRetryTime *time.Time
	LastError     string
	FileName      string
	DisplayName   string
	VisitStrategy string
	AccessScope   string
	CreateTime    time.Time
	UpdateTime    time.Time
}

type DownloadTokenClaims struct {
	FileID  int64  `json:"fid"`
	UserID  int64  `json:"uid,omitempty"`
	ScopeID string `json:"scope,omitempty"`
	IP      string `json:"ip,omitempty"`
	JTI     string `json:"jti,omitempty"`
	Exp     int64  `json:"exp"`
}

type UploadTaskMessage struct {
	MessageID string `json:"messageId"`
	TaskID    string `json:"taskId"`
	TraceID   string `json:"traceId,omitempty"`
	Retry     int    `json:"retry,omitempty"`
}

type FileProcessMessage struct {
	MessageID  string `json:"messageId"`
	TaskID     int64  `json:"taskId"`
	FileID     int64  `json:"fileId"`
	TaskType   string `json:"taskType"`
	TaskParams string `json:"taskParams,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
	Retry      int    `json:"retry,omitempty"`
}

type OutboxEvent struct {
	ID            int64
	EventID       string
	EventOwner    string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       string
	Status        string
	RetryCount    int
	NextRetryAt   time.Time
	LastError     string
	CreateTime    time.Time
	UpdateTime    time.Time
}

// OutboxLease is the fencing capability returned after an outbox event is
// claimed. Its token is required when the relay completes or retries the event.
type OutboxLease struct {
	Token string
	Until time.Time
}

// ConsumeLease is the fencing capability returned for a consumed message.
// It prevents a stale consumer invocation from overwriting a newer retry.
type ConsumeLease struct {
	Token string
	Until time.Time
}
