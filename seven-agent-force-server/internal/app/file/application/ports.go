package application

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
)

type permanentConsumeError struct {
	err error
}

func (e permanentConsumeError) Error() string {
	if e.err == nil {
		return "permanent file message failure"
	}
	return e.err.Error()
}

func (e permanentConsumeError) Unwrap() error { return e.err }

func (e permanentConsumeError) Permanent() bool { return true }

// PermanentConsumeError marks an exhausted or malformed file message so the
// generic broker consumer can dead-letter it instead of requeueing forever.
func PermanentConsumeError(err error) error {
	if err == nil {
		err = errors.New("permanent file message failure")
	}
	return permanentConsumeError{err: err}
}

type RepositoryPort interface {
	GetDefaultStrategy(ctx context.Context) (*domain.StorageStrategy, error)
	GetStrategy(ctx context.Context, id int64) (*domain.StorageStrategy, error)
	ListStrategies(ctx context.Context) ([]domain.StorageStrategy, error)
	InsertStrategy(ctx context.Context, item *domain.StorageStrategy) (int64, error)
	UpdateStrategy(ctx context.Context, item *domain.StorageStrategy) error
	SetOnlyDefaultStrategy(ctx context.Context, id int64) error
	EnableStrategy(ctx context.Context, id int64, enabled bool) error
	DeleteStrategy(ctx context.Context, id int64) error
	UpdateStrategyHealth(ctx context.Context, id int64, health int, healthy bool) error

	FindFileBySha256AndSize(ctx context.Context, sha256Value string, size int64) (*domain.FileInfo, error)
	GetFile(ctx context.Context, id int64) (*domain.FileInfo, error)
	GetFileForUpdate(ctx context.Context, id int64) (*domain.FileInfo, error)
	InsertFile(ctx context.Context, item *domain.FileInfo) (int64, error)
	UpdateFile(ctx context.Context, item *domain.FileInfo) error
	SoftDeleteFile(ctx context.Context, id int64) error
	ListCleanupCandidates(ctx context.Context, now time.Time, limit int) ([]domain.FileInfo, error)
	ClaimFileForCleanup(ctx context.Context, fileID int64, now time.Time) (bool, error)
	HasActiveReferences(ctx context.Context, fileID int64) (bool, error)
	HasProtectedCredential(ctx context.Context, fileID int64, now time.Time) (bool, error)
	RestoreFileAvailableIfCleaning(ctx context.Context, fileID int64) error
	MarkFileDeletedIfCleaning(ctx context.Context, fileID int64) (bool, error)
	QueryFiles(ctx context.Context, current, size int64, fileName, fileType string, bizType *int, startTime, endTime string) (*domain.Page[domain.FileInfo], error)
	FileStats(ctx context.Context) (map[string]any, error)

	InsertReference(ctx context.Context, item *domain.FileReference) (int64, error)
	GetReference(ctx context.Context, id int64) (*domain.FileReference, error)
	ListReferencesByBiz(ctx context.Context, userID int64, bizType string, bizID int64) ([]domain.FileReference, error)
	FindConfigAssetReference(ctx context.Context, configID int64) (*domain.FileReference, error)
	ListReferencesByFile(ctx context.Context, fileID int64) ([]domain.FileReference, error)
	FindPublicReferenceByFile(ctx context.Context, fileID int64) (*domain.FileReference, error)
	UpdateReferenceAccess(ctx context.Context, id int64, accessScope, visitStrategy string, accessLevel int, visitURL string) (*domain.FileReference, error)
	UpdateConfigAssetReference(ctx context.Context, item *domain.FileReference) error
	DeleteReferencesByBiz(ctx context.Context, userID int64, bizType string, bizID int64) error
	SoftDeleteReference(ctx context.Context, fileID, userID int64, bizType string, bizID int64) error
	SoftDeleteReferenceInScope(ctx context.Context, fileID, userID int64, scopeID, bizType string, bizID int64) error
	SoftDeleteConfigAssetReference(ctx context.Context, configID int64, scopeID string) error

	InsertUploadTask(ctx context.Context, task *domain.UploadTask) error
	GetUploadTask(ctx context.Context, id string) (*domain.UploadTask, error)
	FindUploadCredential(ctx context.Context, userID int64, scopeID string, fileID int64) (*domain.UploadTask, error)
	ListExpiredUploadTasks(ctx context.Context, now time.Time, limit int) ([]domain.UploadTask, error)
	UpdateUploadTask(ctx context.Context, task *domain.UploadTask) error
	UpdateUploadTaskStatusIfMatch(ctx context.Context, id, from, to string) (bool, error)

	InsertChunkUpload(ctx context.Context, upload *domain.ChunkUpload) error
	GetChunkUpload(ctx context.Context, uploadID string) (*domain.ChunkUpload, error)
	GetChunkUploadForUpdate(ctx context.Context, uploadID string) (*domain.ChunkUpload, error)
	UpdateChunkUpload(ctx context.Context, upload *domain.ChunkUpload) error
	ListActiveChunkUploads(ctx context.Context, userID int64, scopeID string) ([]domain.ChunkUpload, error)
	ListExpiredChunkUploads(ctx context.Context, now time.Time, limit int) ([]domain.ChunkUpload, error)

	InsertProcessTask(ctx context.Context, task *domain.FileProcessTask) (int64, error)
	GetProcessTask(ctx context.Context, id int64) (*domain.FileProcessTask, error)
	QueryProcessTasks(ctx context.Context, current, size int64, status *int, taskType string) (*domain.Page[domain.FileProcessTask], error)
	ClaimProcessTask(ctx context.Context, id int64) (bool, error)
	UpdateProcessTaskStatus(ctx context.Context, id int64, status int, errMsg, result string) error
	InsertProcessRun(ctx context.Context, run *domain.FileProcessRun) error
	FindCompletedProcessTask(ctx context.Context, dedupKey, taskType string) (*domain.FileProcessTask, error)
	ListPendingRetryProcessTasks(ctx context.Context, now time.Time, limit int) ([]domain.FileProcessTask, error)

	InsertBindingTask(ctx context.Context, task *domain.FileBindingTask) (int64, error)
	MarkBindingTask(ctx context.Context, fileID int64, bindingToken, status, displayName, visitStrategy, accessScope, lastError string) error
	ListRetryBindingTasks(ctx context.Context, now time.Time, limit int) ([]domain.FileBindingTask, error)
}

type OutboxPort interface {
	AppendOutbox(ctx context.Context, event *domain.OutboxEvent) error
	AppendOutboxBatch(ctx context.Context, events []domain.OutboxEvent) error
	ListReadyOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	ListUnknownOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	TryClaimOutbox(ctx context.Context, id int64, eventType, worker string) (*domain.OutboxLease, bool, error)
	MarkOutbox(ctx context.Context, id int64, eventType, leaseToken, status, lastError string, retryCount int, nextRetryAt *time.Time) (bool, error)
	BeginConsume(ctx context.Context, messageID, consumer, worker, detail string) (*domain.ConsumeLease, bool, error)
	MarkConsumed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error)
	MarkConsumeFailed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error)
}

type TransactorPort interface {
	Enabled() bool
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}

type ObjectStorePort interface {
	Save(ctx context.Context, strategy domain.StorageStrategy, storagePath string, reader io.Reader, contentType string) (domain.StoredObject, error)
	Open(ctx context.Context, strategy domain.StorageStrategy, file domain.FileInfo) (domain.DownloadObject, error)
	Delete(ctx context.Context, strategy domain.StorageStrategy, storagePath string) error
	PublicURL(strategy domain.StorageStrategy, storagePath string) string
	PresignPut(ctx context.Context, strategy domain.StorageStrategy, storagePath, contentType string, ttl time.Duration) (string, error)
	Health(ctx context.Context, strategy domain.StorageStrategy) error
}

type DownloadTokenPort interface {
	Issue(ctx context.Context, fileID, userID int64, scopeID, ip string) (string, error)
	Verify(ctx context.Context, token, ip string) (*domain.DownloadTokenClaims, error)
}

type MessagePublisherPort interface {
	Enabled() bool
	PublishUploadTask(ctx context.Context, message domain.UploadTaskMessage) error
	PublishUploadTaskRetry(ctx context.Context, message domain.UploadTaskMessage, delay time.Duration) error
	PublishFileProcessTask(ctx context.Context, message domain.FileProcessMessage) error
	ConsumeUploadTasks(ctx context.Context, consumer string, handler func(context.Context, domain.UploadTaskMessage) error) error
	ConsumeFileProcessTasks(ctx context.Context, consumer string, handler func(context.Context, domain.FileProcessMessage) error) error
	Reconnect(ctx context.Context) error
}
