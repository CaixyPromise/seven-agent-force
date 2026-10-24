package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

type Actor struct {
	UserID        int64
	ScopeID       string
	ScopeSource   string
	IsAdmin       bool
	Authenticated bool
	ClientIP      string
	Referer       string
}

func requireActorScope(actor Actor) (string, error) {
	if actor.UserID <= 0 || !actor.Authenticated {
		return "", apperrors.Unauthorized("未登录或登录信息失效")
	}
	scopeID := strings.TrimSpace(actor.ScopeID)
	if scopeID == "" {
		return "", apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	return scopeID, nil
}

func taskBelongsToActor(task *domain.UploadTask, actor Actor) bool {
	return task != nil &&
		task.UserID == actor.UserID &&
		strings.TrimSpace(task.ScopeID) != "" &&
		strings.TrimSpace(task.ScopeID) == strings.TrimSpace(actor.ScopeID)
}

func chunkUploadBelongsToActor(upload *domain.ChunkUpload, actor Actor) bool {
	return upload != nil &&
		upload.UserID == actor.UserID &&
		strings.TrimSpace(upload.ScopeID) != "" &&
		strings.TrimSpace(upload.ScopeID) == strings.TrimSpace(actor.ScopeID)
}

func uploadBindingChannel(channel, scopeSource string) string {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "unknown"
	}
	scopeSource = strings.TrimSpace(scopeSource)
	if scopeSource == "" {
		scopeSource = "unknown"
	}
	return channel + ";scope-source=" + scopeSource
}

type Service struct {
	transactor   TransactorPort
	repo         RepositoryPort
	outboxStore  OutboxPort
	storage      ObjectStorePort
	tokens       DownloadTokenPort
	rabbit       MessagePublisherPort
	idGen        *xid.Generator
	distribution config.FileDistributionConfig
	outbox       bool
}

type processTaskBatchRepository interface {
	ListProcessTasksByIDs(ctx context.Context, ids []int64) ([]domain.FileProcessTask, error)
	ResetProcessTasks(ctx context.Context, ids []int64) (int64, error)
}

type maintenanceBatchRepository interface {
	ResetPendingRetryProcessTasks(ctx context.Context, ids []int64) (int64, error)
	MarkBindingTasks(ctx context.Context, items []domain.FileBindingTask) (int64, error)
	ExpireUploadTasks(ctx context.Context, items []domain.UploadTask) (int64, error)
	ExpireChunkUploads(ctx context.Context, ids []int64) (int64, error)
	UpdateStrategyHealthBatch(ctx context.Context, updates []domain.StorageHealthUpdate) (int64, error)
}

type fileBatchReadRepository interface {
	ListFilesByIDs(ctx context.Context, ids []int64) ([]domain.FileInfo, error)
}

const (
	maintenanceMaxItems  = 100
	maintenanceChunkSize = 50
)

type UploadRequest struct {
	FileName     string
	ContentType  string
	Reader       io.Reader
	ExpectedSize int64
}

type UploadResult struct {
	FileID int64 `json:"fileId"`
}

type CheckFileResponse struct {
	Exists bool  `json:"exists"`
	FileID int64 `json:"fileId,omitempty"`
}

type DownloadResult struct {
	Object       domain.DownloadObject
	CacheControl string
	Public       bool
}

type UploadTaskInitRequest struct {
	FileName       string `json:"fileName"`
	ContentType    string `json:"contentType"`
	ExpectedSize   int64  `json:"expectedSize"`
	ExpectedSha256 string `json:"expectedSha256"`
}

type UploadTaskInitResponse struct {
	TaskID            string             `json:"taskId"`
	ObjectKeyStaging  string             `json:"objectKeyStaging,omitempty"`
	UploadMode        string             `json:"uploadMode"`
	MultipartUploadID string             `json:"multipartUploadId,omitempty"`
	PartSize          int                `json:"partSize,omitempty"`
	TotalParts        int                `json:"totalParts,omitempty"`
	ExpireAt          string             `json:"expireAt,omitempty"`
	InstantHit        bool               `json:"instantHit"`
	SingleUploadURL   *PresignedURLInfo  `json:"singleUploadUrl,omitempty"`
	PartUploadURLs    []PresignedURLInfo `json:"partUploadUrls,omitempty"`
	Challenge         *FileHMACInfo      `json:"challenge,omitempty"`
	// Backward-compatible Go convenience fields. Java-compatible clients should
	// consume singleUploadUrl.url and objectKeyStaging.
	UploadURL     string `json:"uploadUrl,omitempty"`
	CallbackToken string `json:"callbackToken,omitempty"`
}

type PresignedURLInfo struct {
	PartNumber int               `json:"partNumber,omitempty"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	ExpireAt   string            `json:"expireAt,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

type FileHMACInfo struct {
	Algorithm string `json:"algorithm,omitempty"`
	Nonce     string `json:"nonce,omitempty"`
	ExpireAt  string `json:"expireAt,omitempty"`
}

type UploadCallbackRequest struct {
	TaskID       string    `json:"taskId" form:"taskId"`
	BindingToken string    `json:"bindingToken" form:"bindingToken"`
	ETag         string    `json:"etag" form:"etag"`
	ActualSize   int64     `json:"actualSize" form:"actualSize"`
	FileSHA256   string    `json:"fileSha256" form:"fileSha256"`
	CallbackTime time.Time `json:"callbackTime,omitempty"`
}

type ChunkUploadInitRequest struct {
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	ChunkSize   int64  `json:"chunkSize"`
	FileSha256  string `json:"fileSha256"`
	ContentType string `json:"contentType"`
}

type UploadTaskStatusResponse struct {
	TaskID   string `json:"taskId"`
	Status   string `json:"status"`
	ExpireAt string `json:"expireAt,omitempty"`
	FileID   int64  `json:"fileId,omitempty"`
}

type ChunkUploadInitResponse struct {
	UploadID       string `json:"uploadId"`
	ChunkSize      int64  `json:"chunkSize"`
	TotalChunks    int    `json:"totalChunks"`
	ExpireAt       string `json:"expireAt"`
	StorageMode    string `json:"storageMode"`
	UploadedChunks []int  `json:"uploadedChunks"`
}

type ChunkPartRequest struct {
	UploadID     string
	PartNumber   int
	PartSHA256   string
	ContentType  string
	Reader       io.Reader
	ExpectedSize int64
	OriginalName string
}

type ChunkPartResponse struct {
	UploadID       string `json:"uploadId"`
	PartNumber     int    `json:"partNumber"`
	SHA256         string `json:"sha256"`
	Uploaded       bool   `json:"uploaded"`
	UploadedChunks []int  `json:"uploadedChunks"`
}

type ChunkUploadStatusResponse struct {
	UploadID       string `json:"uploadId"`
	Status         int    `json:"status"`
	StatusName     string `json:"statusName"`
	FileName       string `json:"fileName"`
	FileSize       int64  `json:"fileSize"`
	ChunkSize      int    `json:"chunkSize"`
	TotalChunks    int    `json:"totalChunks"`
	UploadedChunks []int  `json:"uploadedChunks"`
	ExpireAt       string `json:"expireAt,omitempty"`
}

type ProcessTaskStats struct {
	Pending int64 `json:"pending"`
	Running int64 `json:"running"`
	Done    int64 `json:"done"`
	Failed  int64 `json:"failed"`
}

type StorageStrategyCommand struct {
	ID                   int64   `json:"id,omitempty"`
	StrategyName         string  `json:"strategyName"`
	ProviderType         string  `json:"providerType"`
	IsDefault            bool    `json:"isDefault"`
	IsEnabled            bool    `json:"isEnabled"`
	RunState             string  `json:"runState"`
	Priority             int     `json:"priority"`
	ConfigJSON           string  `json:"configJson"`
	HealthCheckURL       string  `json:"healthCheckUrl"`
	FailureRateThreshold float64 `json:"failureRateThreshold"`
}

func NewService(transactor TransactorPort, repo RepositoryPort, outboxStore OutboxPort, storage ObjectStorePort, tokens DownloadTokenPort, rabbit MessagePublisherPort, idGen *xid.Generator, distribution config.FileDistributionConfig, outbox bool) *Service {
	return &Service{
		transactor:   transactor,
		repo:         repo,
		outboxStore:  outboxStore,
		storage:      storage,
		tokens:       tokens,
		rabbit:       rabbit,
		idGen:        idGen,
		distribution: distribution,
		outbox:       outbox,
	}
}

func (s *Service) CheckFile(ctx context.Context, actor Actor, sha256Value string, size int64) (*CheckFileResponse, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	sha256Value = strings.TrimSpace(strings.ToLower(sha256Value))
	if sha256Value == "" || size <= 0 {
		return nil, apperrors.Params("文件 SHA256 和大小不能为空")
	}
	file, err := s.repo.FindFileBySha256AndSize(ctx, sha256Value, size)
	if err != nil {
		return nil, err
	}
	if err := validateBindableFile(file, filefacade.DefaultFile); err != nil {
		return &CheckFileResponse{Exists: false}, nil
	}
	owned, err := s.hasFileAuthority(ctx, file.ID, actor.UserID, actor.ScopeID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return &CheckFileResponse{Exists: false}, nil
	}
	return &CheckFileResponse{Exists: true, FileID: file.ID}, nil
}

func (s *Service) Upload(ctx context.Context, actor Actor, request UploadRequest) (*UploadResult, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	biz, err := validateUploadRequest(request)
	if err != nil {
		return nil, err
	}
	strategy, err := s.requireWritableLocalStrategy(ctx)
	if err != nil {
		return nil, err
	}
	if request.ExpectedSize > 0 && request.ExpectedSize > biz.MaxSize {
		return nil, apperrors.Params("文件大小超出限制")
	}
	storagePath := buildStoragePath(biz, actor.UserID, request.FileName)
	reader := request.Reader
	if biz.MaxSize > 0 {
		reader = io.LimitReader(request.Reader, biz.MaxSize+1)
	}
	stored, err := s.storage.Save(ctx, *strategy, storagePath, reader, request.ContentType)
	if err != nil {
		return nil, apperrors.System("文件上传失败")
	}
	uploadStrategy := *strategy
	if stored.Size > biz.MaxSize {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
		return nil, apperrors.Params("文件大小超出限制")
	}
	if request.ExpectedSize > 0 && request.ExpectedSize != stored.Size {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
		return nil, apperrors.Params("文件大小校验失败")
	}
	file := &domain.FileInfo{
		ID:                s.nextID(),
		FileInnerName:     request.FileName,
		FileSize:          stored.Size,
		FileSha256:        stored.SHA256,
		HashAlgorithm:     "SHA-256",
		ContentType:       request.ContentType,
		StorageStrategyID: strategy.ID,
		StoragePath:       stored.StoragePath,
		Status:            domain.FileStatusAvailable,
		ScanStatus:        domain.ScanStatusClean,
		IntegrityStatus:   domain.IntegrityVerified,
	}
	insertFile := true
	existing, err := s.findReusableUploadedFile(ctx, file.FileSha256, file.FileSize, filefacade.DefaultFile)
	if err != nil {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
		return nil, err
	}
	if existing != nil {
		file = existing
		insertFile = false
	}
	var result *UploadResult
	var insertErr error
	persist := func(shouldInsert bool) error {
		return s.withTx(ctx, func(txCtx context.Context) error {
			if shouldInsert {
				fileID, err := s.repo.InsertFile(txCtx, file)
				if err != nil {
					insertErr = err
					return err
				}
				file.ID = fileID
			} else {
				lockedFile, err := s.lockReusableFileForCredential(txCtx, file.ID)
				if err != nil {
					return err
				}
				file = lockedFile
			}
			if err := s.repo.InsertUploadTask(txCtx, s.completedUploadCredential(actor, file, request.FileName, request.ContentType)); err != nil {
				return err
			}
			result = &UploadResult{FileID: file.ID}
			return nil
		})
	}
	err = persist(insertFile)
	reusedFile := !insertFile
	if err != nil && insertErr != nil {
		originalInsertErr := insertErr
		existing, lookupErr := s.findReusableUploadedFile(ctx, stored.SHA256, stored.Size, filefacade.DefaultFile)
		if lookupErr == nil && existing != nil {
			file = existing
			insertErr = nil
			result = nil
			err = persist(false)
			reusedFile = err == nil
		} else {
			err = originalInsertErr
		}
	}
	if err != nil {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
		return nil, err
	}
	if reusedFile && stored.StoragePath != file.StoragePath {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
	}
	return result, nil
}

func (s *Service) FasterUpload(ctx context.Context, actor Actor, sha256Value string, size int64, request UploadRequest) (*UploadResult, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	file, err := s.repo.FindFileBySha256AndSize(ctx, strings.ToLower(strings.TrimSpace(sha256Value)), size)
	if err != nil {
		return nil, err
	}
	if err := validateBindableFile(file, filefacade.DefaultFile); err != nil {
		return nil, err
	}
	owned, err := s.hasFileAuthority(ctx, file.ID, actor.UserID, actor.ScopeID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, apperrors.Forbidden("文件不存在或不可复用")
	}
	if _, err := validateUploadMetadata(request); err != nil {
		return nil, err
	}
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		lockedFile, err := s.lockReusableFileForCredential(txCtx, file.ID)
		if err != nil {
			return err
		}
		return s.repo.InsertUploadTask(txCtx, s.completedUploadCredential(actor, lockedFile, request.FileName, request.ContentType))
	}); err != nil {
		return nil, err
	}
	return &UploadResult{FileID: file.ID}, nil
}

func (s *Service) BuildDownloadURL(ctx context.Context, actor Actor, fileID int64) (string, error) {
	if _, err := requireActorScope(actor); err != nil {
		return "", err
	}
	if fileID <= 0 {
		return "", apperrors.Params("文件ID不能为空")
	}
	// A file that participates in CONFIG_ASSET has one presentation authority:
	// the owning configuration's stable route. Do this before issuing a token so
	// an old or generic fileId path can never become a second capability.
	if err := s.ensureNoConfigAssetReference(ctx, fileID); err != nil {
		return "", err
	}
	authorized, err := s.hasFileAuthority(ctx, fileID, actor.UserID, actor.ScopeID)
	if err != nil {
		return "", err
	}
	if !authorized {
		if err := s.checkDirectDownloadAccess(ctx, actor, fileID); err != nil {
			return "", err
		}
	}
	token, err := s.tokens.Issue(ctx, fileID, actor.UserID, actor.ScopeID, actor.ClientIP)
	if err != nil {
		return "", err
	}
	return withQueryParam(s.downloadGatewayPath(), "token", token), nil
}

func (s *Service) OpenDownload(ctx context.Context, actor Actor, fileID int64, token string) (*DownloadResult, error) {
	if token != "" {
		claims, err := s.tokens.Verify(ctx, token, actor.ClientIP)
		if err != nil {
			return nil, apperrors.Forbidden("下载令牌无效")
		}
		fileID = claims.FileID
		if claims.UserID > 0 && (!actor.Authenticated || actor.UserID != claims.UserID) {
			return nil, apperrors.Forbidden("下载令牌不属于当前用户")
		}
		if claims.UserID > 0 && strings.TrimSpace(claims.ScopeID) == "" {
			return nil, apperrors.Forbidden("下载令牌缺少组织范围")
		}
		if claims.ScopeID != "" && strings.TrimSpace(actor.ScopeID) != claims.ScopeID {
			return nil, apperrors.Forbidden("下载令牌不属于当前组织")
		}
	}
	if fileID <= 0 {
		return nil, apperrors.Params("文件ID不能为空")
	}
	// Re-check at stream time as well as token issuance. This makes a token
	// issued before a later CONFIG_ASSET bind unusable through the generic
	// endpoint, and protects any historical conflicting references.
	if err := s.ensureNoConfigAssetReference(ctx, fileID); err != nil {
		return nil, err
	}
	file, err := s.repo.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if file == nil || file.IsDeleted == 1 || file.Status != domain.FileStatusAvailable {
		return nil, apperrors.NotFound("文件不存在")
	}
	if token == "" {
		if err := s.checkDirectDownloadAccess(ctx, actor, file.ID); err != nil {
			return nil, err
		}
	}
	strategy, err := s.storageStrategyForFile(ctx, *file)
	if err != nil {
		return nil, err
	}
	object, err := s.storage.Open(ctx, strategy, *file)
	if err != nil {
		return nil, apperrors.NotFound("文件不存在")
	}
	public := false
	cacheControl := defaultString(s.distribution.CacheControlPrivate, "private,no-store,max-age=0")
	if ref, _ := s.repo.FindPublicReferenceByFile(ctx, file.ID); ref != nil && ref.AccessScope == string(filefacade.AccessPublic) {
		if err := s.checkHotlink(actor.Referer); err != nil {
			_ = object.File.Close()
			return nil, err
		}
		public = true
		cacheControl = defaultString(s.distribution.CacheControlPublic, "public,max-age=604800,immutable")
	}
	return &DownloadResult{Object: object, CacheControl: cacheControl, Public: public}, nil
}

func (s *Service) OpenReferenceDownload(ctx context.Context, actor Actor, referenceID int64) (*DownloadResult, error) {
	if referenceID <= 0 {
		return nil, apperrors.Params("引用ID不能为空")
	}
	ref, err := s.repo.GetReference(ctx, referenceID)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, apperrors.NotFound("文件引用不存在")
	}
	if ref.BizType == filefacade.ConfigAssetBizType {
		return nil, apperrors.Forbidden("配置资产只能通过配置稳定引用访问")
	}
	switch ref.AccessScope {
	case string(filefacade.AccessPublic):
	case string(filefacade.AccessLoginUsers):
		if !actor.Authenticated {
			return nil, apperrors.Unauthorized("请先登录")
		}
		if strings.TrimSpace(actor.ScopeID) == "" || actor.ScopeID != ref.ScopeID {
			return nil, apperrors.Forbidden("无文件访问权限")
		}
	default:
		if actor.UserID <= 0 || actor.UserID != ref.UserID || strings.TrimSpace(actor.ScopeID) == "" || actor.ScopeID != ref.ScopeID {
			return nil, apperrors.Forbidden("无文件访问权限")
		}
	}
	return s.OpenDownload(ctx, actor, ref.FileID, "")
}

func (s *Service) InitUploadTask(ctx context.Context, actor Actor, request UploadTaskInitRequest) (*UploadTaskInitResponse, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	biz := domain.DefaultFileBiz
	if _, err := validateUploadMetadata(UploadRequest{FileName: request.FileName, ContentType: request.ContentType}); err != nil {
		return nil, err
	}
	strategy, err := s.requireWritableLocalStrategy(ctx)
	if err != nil {
		return nil, err
	}
	taskID := uuid.NewString()
	token := randomToken()
	expireAt := time.Now().UTC().Add(24 * time.Hour)
	uploadURL := "/uploads/complete"
	uploadMode := "single"
	objectKey := buildStoragePath(biz, actor.UserID, request.FileName)
	var singleUploadURL *PresignedURLInfo
	if strings.ToUpper(strings.TrimSpace(strategy.ProviderType)) != domain.ProviderLocal {
		signedURL, err := s.storage.PresignPut(ctx, *strategy, objectKey, request.ContentType, time.Until(expireAt))
		if err != nil {
			return nil, apperrors.System("文件上传失败")
		}
		uploadURL = signedURL
		singleUploadURL = &PresignedURLInfo{
			Method:   "PUT",
			URL:      signedURL,
			ExpireAt: expireAt.Format(time.RFC3339),
			Headers:  map[string]string{"Content-Type": request.ContentType},
		}
	}
	task := &domain.UploadTask{
		ID:                 taskID,
		UserID:             actor.UserID,
		ScopeID:            actor.ScopeID,
		CredentialID:       uuid.NewString(),
		CredentialVersion:  domain.UploadCredentialVersion1,
		FileName:           request.FileName,
		ContentType:        request.ContentType,
		StorageStrategyID:  strategy.ID,
		ObjectKeyStaging:   objectKey,
		ObjectKeyClean:     objectKey,
		Status:             domain.UploadTaskInit,
		UploadMode:         uploadMode,
		ExpectedSize:       request.ExpectedSize,
		ExpectedSha256:     strings.ToLower(strings.TrimSpace(request.ExpectedSha256)),
		BindingToken:       token,
		BindingChannel:     uploadBindingChannel("direct", actor.ScopeSource),
		ExpireAt:           &expireAt,
		ProtectedUntil:     &expireAt,
		CredentialExpireAt: &expireAt,
		UserIP:             actor.ClientIP,
	}
	if err := s.repo.InsertUploadTask(ctx, task); err != nil {
		return nil, err
	}
	return &UploadTaskInitResponse{
		TaskID:           taskID,
		ObjectKeyStaging: objectKey,
		UploadURL:        uploadURL,
		UploadMode:       uploadMode,
		CallbackToken:    token,
		ExpireAt:         expireAt.Format(time.RFC3339),
		InstantHit:       false,
		SingleUploadURL:  singleUploadURL,
	}, nil
}

func (s *Service) ConfirmInstantUpload(ctx context.Context, actor Actor, taskID string) (*UploadResult, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	task, err := s.repo.GetUploadTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !taskBelongsToActor(task, actor) {
		return nil, apperrors.NotFound("上传任务不存在")
	}
	file, err := s.repo.FindFileBySha256AndSize(ctx, task.ExpectedSha256, task.ExpectedSize)
	if err != nil {
		return nil, err
	}
	if err := validateBindableFile(file, filefacade.DefaultFile); err != nil {
		return nil, err
	}
	owned, err := s.hasFileAuthority(ctx, file.ID, actor.UserID, actor.ScopeID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, apperrors.Forbidden("文件不存在或不可复用")
	}
	task.FileID = file.ID
	task.ActualSize = file.FileSize
	task.Status = domain.UploadTaskClean
	task.FailureCategory = domain.FailureNone
	task.FailureReason = ""
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		lockedFile, err := s.lockReusableFileForCredential(txCtx, file.ID)
		if err != nil {
			return err
		}
		task.FileID = lockedFile.ID
		task.ActualSize = lockedFile.FileSize
		return s.repo.UpdateUploadTask(txCtx, task)
	}); err != nil {
		return nil, err
	}
	return &UploadResult{FileID: file.ID}, nil
}

func (s *Service) CompleteUploadTask(ctx context.Context, actor Actor, taskID string) (*UploadResult, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	task, err := s.repo.GetUploadTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !taskBelongsToActor(task, actor) {
		return nil, apperrors.NotFound("上传任务不存在")
	}
	if task.ExpireAt != nil && time.Now().After(*task.ExpireAt) {
		task.Status = domain.UploadTaskExpired
		_ = s.repo.UpdateUploadTask(ctx, task)
		return nil, apperrors.Operation("上传任务已过期")
	}
	if task.Status == domain.UploadTaskClean {
		return &UploadResult{FileID: task.FileID}, nil
	}
	if task.Status == domain.UploadTaskUploaded || task.Status == domain.UploadTaskProcessing {
		return s.finishUploadTask(ctx, task.ID, "direct-complete:"+task.ID)
	}
	if task.Status != domain.UploadTaskInit && task.Status != domain.UploadTaskUploading {
		return nil, apperrors.Operation("上传任务状态不允许完成")
	}
	strategy, err := s.storageStrategyByID(ctx, task.StorageStrategyID)
	if err != nil {
		return nil, err
	}
	actualSize, actualSHA, err := s.verifyStagedUploadObject(ctx, strategy, task, 0, "", "")
	if err != nil {
		return nil, err
	}
	task.ActualSize = actualSize
	task.ETag = actualSHA
	task.ExpectedSha256 = defaultString(task.ExpectedSha256, actualSHA)
	task.Status = domain.UploadTaskUploaded
	task.FailureCategory = domain.FailureNone
	task.FailureReason = ""
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.UpdateUploadTask(txCtx, task); err != nil {
			return err
		}
		return s.appendOutbox(txCtx, "UPLOAD_TASK_READY", "UPLOAD_TASK", task.ID, map[string]any{"taskId": task.ID})
	}); err != nil {
		return nil, err
	}
	return s.finishUploadTask(ctx, task.ID, "direct-complete:"+task.ID)
}

func (s *Service) CompleteUploadTaskWithReader(ctx context.Context, actor Actor, taskID string, reader io.Reader, size int64, contentType string) (*UploadResult, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	task, err := s.repo.GetUploadTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !taskBelongsToActor(task, actor) {
		return nil, apperrors.NotFound("上传任务不存在")
	}
	if task.ExpireAt != nil && time.Now().After(*task.ExpireAt) {
		task.Status = domain.UploadTaskExpired
		_ = s.repo.UpdateUploadTask(ctx, task)
		return nil, apperrors.Operation("上传任务已过期")
	}
	if reader == nil {
		return nil, apperrors.Params("文件内容不能为空")
	}
	if task.Status == domain.UploadTaskClean {
		return &UploadResult{FileID: task.FileID}, nil
	}
	if task.Status == domain.UploadTaskUploaded || task.Status == domain.UploadTaskProcessing {
		return s.finishUploadTask(ctx, task.ID, "direct-complete:"+task.ID)
	}
	if task.Status != domain.UploadTaskInit && task.Status != domain.UploadTaskUploading && task.Status != domain.UploadTaskUploaded {
		return nil, apperrors.Operation("上传任务状态不允许完成")
	}
	if task.ExpectedSize > 0 && size > 0 && task.ExpectedSize != size {
		return nil, apperrors.Params("文件大小校验失败")
	}
	if strings.TrimSpace(contentType) != "" {
		task.ContentType = contentType
	}
	strategy, err := s.storageStrategyByID(ctx, task.StorageStrategyID)
	if err != nil {
		return nil, err
	}
	stored, err := s.storage.Save(ctx, strategy, task.ObjectKeyStaging, reader, contentTypeByName(task.FileName, task.ContentType))
	if err != nil {
		return nil, apperrors.System("文件上传失败")
	}
	if task.ExpectedSize > 0 && task.ExpectedSize != stored.Size {
		_ = s.storage.Delete(ctx, strategy, stored.StoragePath)
		return nil, apperrors.Params("文件大小校验失败")
	}
	if task.ExpectedSha256 != "" && task.ExpectedSha256 != stored.SHA256 {
		_ = s.storage.Delete(ctx, strategy, stored.StoragePath)
		return nil, apperrors.Params("文件 SHA256 校验失败")
	}
	task.ActualSize = stored.Size
	task.ETag = stored.SHA256
	task.Status = domain.UploadTaskUploaded
	task.FailureCategory = domain.FailureNone
	task.FailureReason = ""
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.UpdateUploadTask(txCtx, task); err != nil {
			return err
		}
		return s.appendOutbox(txCtx, "UPLOAD_TASK_READY", "UPLOAD_TASK", task.ID, map[string]any{"taskId": task.ID})
	}); err != nil {
		return nil, err
	}
	return s.finishUploadTask(ctx, task.ID, "direct-complete:"+task.ID)
}

func (s *Service) CompleteUploadCallback(ctx context.Context, request UploadCallbackRequest) (*UploadTaskStatusResponse, error) {
	task, err := s.repo.GetUploadTask(ctx, strings.TrimSpace(request.TaskID))
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, apperrors.NotFound("上传任务不存在")
	}
	if strings.TrimSpace(task.BindingToken) == "" || strings.TrimSpace(request.BindingToken) != task.BindingToken {
		return nil, apperrors.Forbidden("上传回调签名无效")
	}
	if task.ExpireAt != nil && time.Now().After(*task.ExpireAt) {
		task.Status = domain.UploadTaskExpired
		_ = s.repo.UpdateUploadTask(ctx, task)
		return nil, apperrors.Operation("上传任务已过期")
	}
	if task.Status == domain.UploadTaskClean {
		return uploadTaskStatusResponse(task), nil
	}
	if task.Status == domain.UploadTaskUploaded || task.Status == domain.UploadTaskProcessing {
		_, _ = s.finishUploadTask(ctx, task.ID, "callback-complete:"+task.ID)
		current, getErr := s.repo.GetUploadTask(ctx, task.ID)
		if getErr != nil {
			return nil, getErr
		}
		return uploadTaskStatusResponse(current), nil
	}
	if task.Status != domain.UploadTaskInit && task.Status != domain.UploadTaskUploading && task.Status != domain.UploadTaskUploaded {
		return nil, apperrors.Operation("上传任务状态不允许完成")
	}
	strategy, err := s.storageStrategyByID(ctx, task.StorageStrategyID)
	if err != nil {
		return nil, err
	}
	actualSize, actualSHA, err := s.verifyStagedUploadObject(ctx, strategy, task, request.ActualSize, request.FileSHA256, request.ETag)
	if err != nil {
		return nil, err
	}
	task.ActualSize = actualSize
	task.ETag = strings.TrimSpace(request.ETag)
	if task.ETag == "" {
		task.ETag = actualSHA
	}
	task.ExpectedSha256 = defaultString(task.ExpectedSha256, actualSHA)
	task.Status = domain.UploadTaskUploaded
	task.FailureCategory = domain.FailureNone
	task.FailureReason = ""
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.UpdateUploadTask(txCtx, task); err != nil {
			return err
		}
		return s.appendOutbox(txCtx, "UPLOAD_TASK_READY", "UPLOAD_TASK", task.ID, map[string]any{"taskId": task.ID})
	}); err != nil {
		return nil, err
	}
	_, _ = s.finishUploadTask(ctx, task.ID, "callback-complete:"+task.ID)
	current, err := s.repo.GetUploadTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	return uploadTaskStatusResponse(current), nil
}

func (s *Service) GetUploadTaskStatus(ctx context.Context, actor Actor, taskID string) (*UploadTaskStatusResponse, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	task, err := s.repo.GetUploadTask(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	if !taskBelongsToActor(task, actor) {
		return nil, apperrors.NotFound("上传任务不存在")
	}
	return uploadTaskStatusResponse(task), nil
}

func (s *Service) finishUploadTask(ctx context.Context, taskID, messageID string) (*UploadResult, error) {
	if err := s.HandleUploadTaskMessage(ctx, domain.UploadTaskMessage{MessageID: messageID, TaskID: taskID}); err != nil {
		return nil, err
	}
	task, err := s.repo.GetUploadTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, apperrors.NotFound("上传任务不存在")
	}
	switch task.Status {
	case domain.UploadTaskClean:
		if task.FileID <= 0 {
			return nil, apperrors.System("上传任务缺少文件结果")
		}
		return &UploadResult{FileID: task.FileID}, nil
	case domain.UploadTaskFailed, domain.UploadTaskRejected, domain.UploadTaskExpired:
		return nil, apperrors.Operation("上传任务未成功完成")
	default:
		return nil, apperrors.Operation("上传任务处理中，请通过任务状态查询结果")
	}
}

func uploadTaskStatusResponse(task *domain.UploadTask) *UploadTaskStatusResponse {
	if task == nil {
		return nil
	}
	result := &UploadTaskStatusResponse{
		TaskID: task.ID,
		Status: task.Status,
	}
	if task.ExpireAt != nil {
		result.ExpireAt = task.ExpireAt.UTC().Format(time.RFC3339)
	}
	if task.Status == domain.UploadTaskClean {
		result.FileID = task.FileID
	}
	return result
}

func (s *Service) InitChunkUpload(ctx context.Context, actor Actor, request ChunkUploadInitRequest) (*ChunkUploadInitResponse, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	request.FileName = strings.TrimSpace(request.FileName)
	if request.FileName == "" {
		return nil, apperrors.Params("文件名不能为空")
	}
	biz := domain.DefaultFileBiz
	if request.FileSize <= 0 {
		return nil, apperrors.Params("文件大小不能为空")
	}
	if request.FileSize > biz.MaxSize {
		return nil, apperrors.Params("文件大小超出限制")
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(request.FileName)), ".")
	if ext != "" && !biz.Suffixes[ext] {
		return nil, apperrors.Params("不支持的文件类型")
	}
	chunkSize := request.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 5 * 1024 * 1024
	}
	if chunkSize < 256*1024 {
		return nil, apperrors.Params("分块大小不能小于256KB")
	}
	totalChunks := int((request.FileSize + chunkSize - 1) / chunkSize)
	if totalChunks <= 0 {
		return nil, apperrors.Params("分块数量错误")
	}
	strategy, err := s.requireWritableLocalStrategy(ctx)
	if err != nil {
		return nil, err
	}
	uploadID := uuid.NewString()
	taskID := uuid.NewString()
	expireAt := time.Now().UTC().Add(24 * time.Hour)
	upload := &domain.ChunkUpload{
		ID:                s.nextID(),
		UploadID:          uploadID,
		UploadTaskID:      taskID,
		UserID:            actor.UserID,
		ScopeID:           actor.ScopeID,
		FileName:          request.FileName,
		FileSize:          request.FileSize,
		ChunkSize:         int(chunkSize),
		TotalChunks:       totalChunks,
		UploadedChunks:    []int{},
		ChunkSha256Map:    map[int]string{},
		PartETagsMap:      map[int]string{},
		FileSha256:        strings.ToLower(strings.TrimSpace(request.FileSha256)),
		StorageStrategyID: strategy.ID,
		TempStoragePath:   filepath.Join("tmp", "chunk-upload", uploadID),
		ContentType:       contentTypeByName(request.FileName, request.ContentType),
		Status:            domain.ChunkStatusInit,
		ExpireTime:        expireAt,
	}
	task := &domain.UploadTask{
		ID:                 taskID,
		UserID:             actor.UserID,
		ScopeID:            actor.ScopeID,
		CredentialID:       uuid.NewString(),
		CredentialVersion:  domain.UploadCredentialVersion1,
		FileName:           request.FileName,
		ContentType:        upload.ContentType,
		StorageStrategyID:  strategy.ID,
		ObjectKeyStaging:   upload.TempStoragePath,
		ObjectKeyClean:     upload.TempStoragePath,
		Status:             domain.UploadTaskInit,
		UploadMode:         "chunk",
		ExpectedSize:       request.FileSize,
		ExpectedSha256:     upload.FileSha256,
		BindingToken:       randomToken(),
		BindingChannel:     uploadBindingChannel("chunk", actor.ScopeSource),
		ExpireAt:           &expireAt,
		ProtectedUntil:     &expireAt,
		CredentialExpireAt: &expireAt,
		UserIP:             actor.ClientIP,
	}
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.InsertUploadTask(txCtx, task); err != nil {
			return err
		}
		return s.repo.InsertChunkUpload(txCtx, upload)
	}); err != nil {
		return nil, err
	}
	return &ChunkUploadInitResponse{
		UploadID:       uploadID,
		ChunkSize:      chunkSize,
		TotalChunks:    totalChunks,
		ExpireAt:       expireAt.Format(time.RFC3339),
		StorageMode:    "LOCAL",
		UploadedChunks: []int{},
	}, nil
}

func (s *Service) UploadChunkPart(ctx context.Context, actor Actor, request ChunkPartRequest) (*ChunkPartResponse, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	upload, err := s.repo.GetChunkUpload(ctx, strings.TrimSpace(request.UploadID))
	if err != nil {
		return nil, err
	}
	if !chunkUploadBelongsToActor(upload, actor) {
		return nil, apperrors.NotFound("分块上传任务不存在")
	}
	if upload.Status == domain.ChunkStatusCompleted {
		return nil, apperrors.Operation("分块上传已完成")
	}
	if upload.Status == domain.ChunkStatusAborted || upload.Status == domain.ChunkStatusExpired || time.Now().After(upload.ExpireTime) {
		upload.Status = domain.ChunkStatusExpired
		_ = s.repo.UpdateChunkUpload(ctx, upload)
		return nil, apperrors.Operation("分块上传任务已失效")
	}
	if request.PartNumber <= 0 || request.PartNumber > upload.TotalChunks {
		return nil, apperrors.Params("分块序号错误")
	}
	if request.Reader == nil {
		return nil, apperrors.Params("分块内容不能为空")
	}
	if existing := strings.TrimSpace(upload.ChunkSha256Map[request.PartNumber]); existing != "" {
		expected := strings.ToLower(strings.TrimSpace(request.PartSHA256))
		if expected == "" || expected == existing {
			return &ChunkPartResponse{UploadID: upload.UploadID, PartNumber: request.PartNumber, SHA256: existing, Uploaded: true, UploadedChunks: normalizeUploadedChunks(upload.UploadedChunks)}, nil
		}
		return nil, apperrors.Params("重复分块的 SHA256 不一致")
	}
	partPath := chunkPartPath(upload.UploadID, request.PartNumber)
	strategy, err := s.storageStrategyByID(ctx, upload.StorageStrategyID)
	if err != nil {
		return nil, err
	}
	var stored domain.StoredObject
	expected := strings.ToLower(strings.TrimSpace(request.PartSHA256))
	var uploadedChunks []int
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		locked, err := s.repo.GetChunkUploadForUpdate(txCtx, upload.UploadID)
		if err != nil {
			return err
		}
		if !chunkUploadBelongsToActor(locked, actor) {
			return apperrors.NotFound("分块上传任务不存在")
		}
		if locked.Status == domain.ChunkStatusCompleted {
			return apperrors.Operation("分块上传已完成")
		}
		if locked.Status == domain.ChunkStatusAborted || locked.Status == domain.ChunkStatusExpired || time.Now().After(locked.ExpireTime) {
			locked.Status = domain.ChunkStatusExpired
			_ = s.repo.UpdateChunkUpload(txCtx, locked)
			return apperrors.Operation("分块上传任务已失效")
		}
		if existing := strings.TrimSpace(locked.ChunkSha256Map[request.PartNumber]); existing != "" {
			if expected != "" && expected != existing {
				return apperrors.Params("重复分块的 SHA256 不一致")
			}
			uploadedChunks = normalizeUploadedChunks(locked.UploadedChunks)
			stored = domain.StoredObject{StoragePath: partPath, SHA256: existing}
			return nil
		}
		stored, err = s.storage.Save(txCtx, strategy, partPath, request.Reader, request.ContentType)
		if err != nil {
			return apperrors.System("文件上传失败")
		}
		if expected != "" && expected != stored.SHA256 {
			return apperrors.Params("分块 SHA256 校验失败")
		}
		if request.ExpectedSize > 0 && request.ExpectedSize != stored.Size {
			return apperrors.Params("分块大小校验失败")
		}
		locked.Status = domain.ChunkStatusUploading
		locked.ChunkSha256Map[request.PartNumber] = stored.SHA256
		locked.PartETagsMap[request.PartNumber] = stored.SHA256
		locked.UploadedChunks = appendUniqueInt(locked.UploadedChunks, request.PartNumber)
		uploadedChunks = normalizeUploadedChunks(locked.UploadedChunks)
		return s.repo.UpdateChunkUpload(txCtx, locked)
	}); err != nil {
		if strings.TrimSpace(stored.StoragePath) != "" {
			_ = s.storage.Delete(ctx, strategy, stored.StoragePath)
		}
		return nil, err
	}
	return &ChunkPartResponse{UploadID: upload.UploadID, PartNumber: request.PartNumber, SHA256: stored.SHA256, Uploaded: true, UploadedChunks: uploadedChunks}, nil
}

func (s *Service) CompleteChunkUpload(ctx context.Context, actor Actor, uploadID string) (*UploadResult, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	upload, err := s.repo.GetChunkUpload(ctx, strings.TrimSpace(uploadID))
	if err != nil {
		return nil, err
	}
	if !chunkUploadBelongsToActor(upload, actor) {
		return nil, apperrors.NotFound("分块上传任务不存在")
	}
	if upload.Status == domain.ChunkStatusCompleted {
		task, taskErr := s.repo.GetUploadTask(ctx, upload.UploadTaskID)
		if taskErr != nil {
			return nil, taskErr
		}
		if task != nil && task.Authorizes(actor.UserID, actor.ScopeID, task.FileID, time.Now().UTC()) {
			return &UploadResult{FileID: task.FileID}, nil
		}
		return nil, apperrors.Operation("分块上传完成状态缺少有效文件结果")
	}
	if len(upload.UploadedChunks) != upload.TotalChunks {
		return nil, apperrors.Operation("分块未上传完成")
	}
	for part := 1; part <= upload.TotalChunks; part++ {
		if strings.TrimSpace(upload.ChunkSha256Map[part]) == "" {
			return nil, apperrors.Operation("分块未上传完成")
		}
	}
	biz := domain.DefaultFileBiz
	strategy, err := s.storageStrategyByID(ctx, upload.StorageStrategyID)
	if err != nil {
		return nil, err
	}
	storagePath := buildStoragePath(biz, actor.UserID, upload.FileName)
	chunkReader := &chunkSequenceReader{ctx: ctx, storage: s.storage, strategy: strategy, uploadID: upload.UploadID, totalParts: upload.TotalChunks, contentType: upload.ContentType, fileName: upload.FileName}
	defer chunkReader.Close()
	stored, err := s.storage.Save(ctx, strategy, storagePath, chunkReader, upload.ContentType)
	if err != nil {
		return nil, apperrors.System("文件上传失败")
	}
	if stored.Size != upload.FileSize {
		_ = s.storage.Delete(ctx, strategy, stored.StoragePath)
		return nil, apperrors.Params("文件大小校验失败")
	}
	if upload.FileSha256 != "" && upload.FileSha256 != stored.SHA256 {
		_ = s.storage.Delete(ctx, strategy, stored.StoragePath)
		return nil, apperrors.Params("文件 SHA256 校验失败")
	}
	uploadStrategy := strategy
	file := &domain.FileInfo{
		ID:                s.nextID(),
		FileInnerName:     upload.FileName,
		FileSize:          stored.Size,
		FileSha256:        stored.SHA256,
		HashAlgorithm:     "SHA-256",
		ContentType:       upload.ContentType,
		StorageStrategyID: upload.StorageStrategyID,
		StoragePath:       stored.StoragePath,
		Status:            domain.FileStatusAvailable,
		ScanStatus:        domain.ScanStatusClean,
		IntegrityStatus:   domain.IntegrityVerified,
	}
	insertFile := true
	existing, err := s.findReusableUploadedFile(ctx, file.FileSha256, file.FileSize, filefacade.DefaultFile)
	if err != nil {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
		return nil, err
	}
	if existing != nil {
		file = existing
		insertFile = false
	}
	var result *UploadResult
	var insertErr error
	persist := func(shouldInsert bool) error {
		return s.withTx(ctx, func(txCtx context.Context) error {
			if shouldInsert {
				fileID, err := s.repo.InsertFile(txCtx, file)
				if err != nil {
					insertErr = err
					return err
				}
				file.ID = fileID
			} else {
				lockedFile, err := s.lockReusableFileForCredential(txCtx, file.ID)
				if err != nil {
					return err
				}
				file = lockedFile
			}
			task, err := s.repo.GetUploadTask(txCtx, upload.UploadTaskID)
			if err != nil {
				return err
			}
			if task == nil {
				task = s.completedUploadCredential(actor, file, upload.FileName, upload.ContentType)
				upload.UploadTaskID = task.ID
				if err := s.repo.InsertUploadTask(txCtx, task); err != nil {
					return err
				}
			} else {
				task.FileID = file.ID
				task.ActualSize = file.FileSize
				task.ETag = file.FileSha256
				task.ExpectedSha256 = defaultString(task.ExpectedSha256, file.FileSha256)
				task.Status = domain.UploadTaskClean
				task.FailureCategory = domain.FailureNone
				task.FailureReason = ""
				if err := s.repo.UpdateUploadTask(txCtx, task); err != nil {
					return err
				}
			}
			upload.Status = domain.ChunkStatusCompleted
			upload.FileSha256 = stored.SHA256
			if err := s.repo.UpdateChunkUpload(txCtx, upload); err != nil {
				return err
			}
			result = &UploadResult{FileID: file.ID}
			return nil
		})
	}
	err = persist(insertFile)
	reusedFile := !insertFile
	if err != nil && insertErr != nil {
		originalInsertErr := insertErr
		existing, lookupErr := s.findReusableUploadedFile(ctx, stored.SHA256, stored.Size, filefacade.DefaultFile)
		if lookupErr == nil && existing != nil {
			file = existing
			insertErr = nil
			result = nil
			err = persist(false)
			reusedFile = err == nil
		} else {
			err = originalInsertErr
		}
	}
	if err != nil {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
		return nil, err
	}
	if reusedFile && stored.StoragePath != file.StoragePath {
		_ = s.storage.Delete(ctx, uploadStrategy, stored.StoragePath)
	}
	for part := 1; part <= upload.TotalChunks; part++ {
		_ = s.storage.Delete(ctx, uploadStrategy, chunkPartPath(upload.UploadID, part))
	}
	return result, nil
}

func (s *Service) AbortChunkUpload(ctx context.Context, actor Actor, uploadID string) error {
	if _, err := requireActorScope(actor); err != nil {
		return err
	}
	upload, err := s.repo.GetChunkUpload(ctx, strings.TrimSpace(uploadID))
	if err != nil {
		return err
	}
	if !chunkUploadBelongsToActor(upload, actor) {
		return apperrors.NotFound("分块上传任务不存在")
	}
	if upload.Status == domain.ChunkStatusCompleted {
		return apperrors.Operation("分块上传已完成")
	}
	upload.Status = domain.ChunkStatusAborted
	if err := s.repo.UpdateChunkUpload(ctx, upload); err != nil {
		return err
	}
	strategy, err := s.storageStrategyByID(ctx, upload.StorageStrategyID)
	if err != nil {
		return err
	}
	for part := 1; part <= upload.TotalChunks; part++ {
		_ = s.storage.Delete(ctx, strategy, chunkPartPath(upload.UploadID, part))
	}
	return nil
}

func (s *Service) ChunkUploadStatus(ctx context.Context, actor Actor, uploadID string) (*ChunkUploadStatusResponse, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	upload, err := s.repo.GetChunkUpload(ctx, strings.TrimSpace(uploadID))
	if err != nil {
		return nil, err
	}
	if !chunkUploadBelongsToActor(upload, actor) {
		return nil, apperrors.NotFound("分块上传任务不存在")
	}
	return chunkStatusResponse(*upload), nil
}

func (s *Service) ActiveChunkUploads(ctx context.Context, actor Actor) ([]ChunkUploadStatusResponse, error) {
	if _, err := requireActorScope(actor); err != nil {
		return nil, err
	}
	items, err := s.repo.ListActiveChunkUploads(ctx, actor.UserID, actor.ScopeID)
	if err != nil {
		return nil, err
	}
	result := make([]ChunkUploadStatusResponse, 0, len(items))
	for _, item := range items {
		result = append(result, *chunkStatusResponse(item))
	}
	return result, nil
}

func (s *Service) CleanupExpiredChunks(ctx context.Context, limit int) error {
	batchRepo, ok := s.repo.(maintenanceBatchRepository)
	if !ok {
		return apperrors.System("文件维护批量仓储能力未配置")
	}
	items, err := s.repo.ListExpiredChunkUploads(ctx, time.Now(), maintenanceLimit(limit))
	if err != nil {
		return err
	}
	strategies, err := s.repo.ListStrategies(ctx)
	if err != nil {
		return err
	}
	strategyByID := make(map[int64]domain.StorageStrategy, len(strategies))
	for _, strategy := range strategies {
		if strategy.Readable() {
			strategyByID[strategy.ID] = strategy
		}
	}
	candidates := make([]domain.ChunkUpload, 0, len(items))
	for _, item := range items {
		if _, exists := strategyByID[item.StorageStrategyID]; !exists {
			continue
		}
		candidates = append(candidates, item)
	}
	for start := 0; start < len(candidates); start += maintenanceChunkSize {
		end := min(start+maintenanceChunkSize, len(candidates))
		ids := make([]int64, 0, end-start)
		for _, item := range candidates[start:end] {
			ids = append(ids, item.ID)
		}
		if s.transactor == nil || !s.transactor.Enabled() {
			return apperrors.System("分块清理事务能力未配置")
		}
		if err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			matched, expireErr := batchRepo.ExpireChunkUploads(txCtx, ids)
			if expireErr != nil {
				return expireErr
			}
			if matched != int64(len(ids)) {
				return fmt.Errorf("分块上传状态发生并发变化")
			}
			return nil
		}); err != nil {
			return err
		}
		for _, item := range candidates[start:end] {
			strategy := strategyByID[item.StorageStrategyID]
			for part := 1; part <= item.TotalChunks; part++ {
				_ = s.storage.Delete(ctx, strategy, chunkPartPath(item.UploadID, part))
			}
		}
	}
	return nil
}

func (s *Service) CleanupExpiredUploadTasks(ctx context.Context, limit int) error {
	batchRepo, ok := s.repo.(maintenanceBatchRepository)
	if !ok {
		return apperrors.System("文件维护批量仓储能力未配置")
	}
	items, err := s.repo.ListExpiredUploadTasks(ctx, time.Now().UTC(), maintenanceLimit(limit))
	if err != nil {
		return err
	}
	items = uniqueUploadTasks(items)
	for start := 0; start < len(items); start += maintenanceChunkSize {
		end := min(start+maintenanceChunkSize, len(items))
		chunk := items[start:end]
		if s.transactor == nil || !s.transactor.Enabled() {
			return apperrors.System("上传任务清理事务能力未配置")
		}
		if err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			matched, expireErr := batchRepo.ExpireUploadTasks(txCtx, chunk)
			if expireErr != nil {
				return expireErr
			}
			if matched != int64(len(chunk)) {
				return fmt.Errorf("上传任务状态发生并发变化")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CleanupUnreferencedFiles(ctx context.Context, limit int) error {
	now := time.Now().UTC()
	items, err := s.repo.ListCleanupCandidates(ctx, now, maintenanceLimit(limit))
	if err != nil {
		return err
	}
	for _, item := range items {
		if _, err := s.claimAndDeleteUnreferencedFile(ctx, item, now, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DrainStorageStrategies(ctx context.Context, limit int) error {
	_ = ctx
	_ = limit
	return nil
}

func (s *Service) FindFileBySha256AndSize(ctx context.Context, sha256Value string, size int64) (*filefacade.FileInfoDTO, error) {
	file, err := s.repo.FindFileBySha256AndSize(ctx, sha256Value, size)
	if err != nil || file == nil {
		return nil, err
	}
	dto := toFileInfoDTO(*file)
	return &dto, nil
}

// BindUploadedFile validates an upload credential and applies the server-owned
// policy for a finite business asset slot.
func (s *Service) BindUploadedFile(ctx context.Context, command filefacade.BindUploadedFileCommand) (*filefacade.FileReferenceDTO, error) {
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	orgScope, scopeErr := securitycontext.ResolveOrganizationScope(currentUser)
	if scopeErr != nil {
		return nil, apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	userID := currentUser.UserID
	if command.FileID <= 0 {
		return nil, apperrors.Params("文件ID不能为空")
	}
	if command.Slot != filefacade.FileAssetSlotUserAvatar {
		return nil, apperrors.Params("不支持的文件业务槽位")
	}
	bizID := userID
	credential, err := s.repo.FindUploadCredential(ctx, userID, orgScope.ScopeID, command.FileID)
	if err != nil {
		return nil, err
	}
	if credential == nil || !credential.Authorizes(userID, orgScope.ScopeID, command.FileID, time.Now().UTC()) {
		return nil, apperrors.Forbidden("上传凭据无效或已过期")
	}
	file, err := s.repo.GetFile(ctx, command.FileID)
	if err != nil {
		return nil, err
	}
	if err := validateBindableFile(file, filefacade.UserAvatar); err != nil {
		return nil, err
	}
	if err := s.validateImageAsset(ctx, *file); err != nil {
		return nil, err
	}
	strategy, err := s.storageStrategyForFile(ctx, *file)
	if err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(command.DisplayName)
	if displayName == "" {
		displayName = file.FileInnerName
	}
	ref := &domain.FileReference{
		ID:            s.nextID(),
		FileID:        file.ID,
		UserID:        userID,
		ScopeID:       orgScope.ScopeID,
		DisplayName:   displayName,
		BizType:       strconv.Itoa(int(filefacade.UserAvatar)),
		BizID:         bizID,
		VisitURL:      buildVisitURL(s.storage, strategy, file.StoragePath, filefacade.VisitPublicStatic),
		AccessLevel:   accessLevelFromScope(string(filefacade.AccessPublic)),
		VisitStrategy: string(filefacade.VisitPublicStatic),
		AccessScope:   string(filefacade.AccessPublic),
	}
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		lockedFile, err := s.repo.GetFileForUpdate(txCtx, file.ID)
		if err != nil {
			return err
		}
		// Locking the file serializes this with CONFIG_ASSET binding. A generic
		// public reference for the same physical object would otherwise create
		// an alias that bypasses configuration exposure and its stable route.
		if err := s.ensureNoConfigAssetReference(txCtx, lockedFile.ID); err != nil {
			return err
		}
		if err := validateBindableFile(lockedFile, filefacade.UserAvatar); err != nil {
			return err
		}
		refs, err := s.repo.ListReferencesByBiz(txCtx, userID, ref.BizType, bizID)
		if err != nil {
			return err
		}
		for _, existing := range refs {
			if existing.IsDeleted != 0 || strings.TrimSpace(existing.ScopeID) != orgScope.ScopeID {
				continue
			}
			if existing.FileID == file.ID {
				ref = &existing
				return nil
			}
			if err := s.repo.SoftDeleteReferenceInScope(txCtx, existing.FileID, userID, orgScope.ScopeID, ref.BizType, bizID); err != nil {
				return err
			}
		}
		id, err := s.repo.InsertReference(txCtx, ref)
		if err != nil {
			return err
		}
		ref.ID = id
		return nil
	}); err != nil {
		return nil, err
	}
	dto := toReferenceDTO(*ref)
	return &dto, nil
}

// ClaimWorkforceInput converts an authenticated upload credential into a
// private, non-presentational reference. The returned handle is an internal
// lookup identity, not a bearer credential and not a storage address.
func (s *Service) ClaimWorkforceInput(ctx context.Context, command filefacade.ClaimWorkforceInputCommand) (*filefacade.WorkforceInputClaim, error) {
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	orgScope, err := securitycontext.ResolveOrganizationScope(currentUser)
	if err != nil {
		return nil, apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	if command.FileID <= 0 || (command.Kind != filefacade.WorkforceInputWorkspaceArchive && command.Kind != filefacade.WorkforceInputReference) {
		return nil, apperrors.Params("Workforce 输入文件或类型无效")
	}
	credential, err := s.repo.FindUploadCredential(ctx, currentUser.UserID, orgScope.ScopeID, command.FileID)
	if err != nil {
		return nil, err
	}
	if credential == nil || !credential.Authorizes(currentUser.UserID, orgScope.ScopeID, command.FileID, time.Now().UTC()) {
		return nil, apperrors.Forbidden("上传凭据无效或已过期")
	}
	file, err := s.repo.GetFile(ctx, command.FileID)
	if err != nil {
		return nil, err
	}
	if file == nil || file.IsDeleted != 0 || file.Status != domain.FileStatusAvailable || file.FileSize < 0 || strings.TrimSpace(file.FileSha256) == "" {
		return nil, apperrors.Operation("上传文件当前不可作为 Workforce 输入")
	}
	displayName := filepath.Base(strings.TrimSpace(command.DisplayName))
	if displayName == "." || displayName == "" {
		displayName = filepath.Base(file.FileInnerName)
	}
	ref := &domain.FileReference{ID: s.nextID(), FileID: file.ID, UserID: currentUser.UserID, ScopeID: orgScope.ScopeID, DisplayName: displayName, BizType: filefacade.WorkforceInputBizType, BizID: s.nextID(), AccessLevel: accessLevelFromScope(string(filefacade.AccessOwnerOnly)), VisitStrategy: string(filefacade.VisitForbidden), AccessScope: string(filefacade.AccessOwnerOnly)}
	if err = s.withTx(ctx, func(txCtx context.Context) error {
		locked, lockErr := s.repo.GetFileForUpdate(txCtx, file.ID)
		if lockErr != nil {
			return lockErr
		}
		if locked == nil || locked.IsDeleted != 0 || locked.Status != domain.FileStatusAvailable {
			return apperrors.Operation("上传文件当前不可作为 Workforce 输入")
		}
		id, insertErr := s.repo.InsertReference(txCtx, ref)
		ref.ID = id
		return insertErr
	}); err != nil {
		return nil, err
	}
	return &filefacade.WorkforceInputClaim{Handle: "wfi_" + strconv.FormatInt(ref.ID, 10), ScopeID: orgScope.ScopeID, FileName: displayName, ContentType: file.ContentType, ContentSHA256: strings.ToLower(file.FileSha256), Size: file.FileSize}, nil
}

func (s *Service) OpenWorkforceInput(ctx context.Context, command filefacade.OpenWorkforceInputCommand) (*filefacade.WorkforceInputOpenResult, error) {
	refID, err := parseWorkforceInputHandle(command.Handle)
	if err != nil || strings.TrimSpace(command.ExpectedScopeID) == "" {
		return nil, apperrors.Params("Workforce 输入句柄无效")
	}
	ref, err := s.repo.GetReference(ctx, refID)
	if err != nil {
		return nil, err
	}
	if ref == nil || ref.BizType != filefacade.WorkforceInputBizType || strings.TrimSpace(ref.ScopeID) != strings.TrimSpace(command.ExpectedScopeID) || ref.AccessScope != string(filefacade.AccessOwnerOnly) || ref.VisitStrategy != string(filefacade.VisitForbidden) {
		return nil, apperrors.Forbidden("Workforce 输入句柄无效或不属于当前组织")
	}
	file, err := s.repo.GetFile(ctx, ref.FileID)
	if err != nil {
		return nil, err
	}
	if file == nil || file.IsDeleted != 0 || file.Status != domain.FileStatusAvailable {
		return nil, apperrors.NotFound("Workforce 输入文件不存在")
	}
	strategy, err := s.storageStrategyForFile(ctx, *file)
	if err != nil {
		return nil, err
	}
	object, err := s.storage.Open(ctx, strategy, *file)
	if err != nil {
		return nil, apperrors.NotFound("Workforce 输入文件不存在")
	}
	return &filefacade.WorkforceInputOpenResult{Reader: object.File, Size: object.Size, ContentType: file.ContentType, FileName: filepath.Base(ref.DisplayName), ContentSHA256: strings.ToLower(file.FileSha256)}, nil
}

func (s *Service) ReleaseWorkforceInput(ctx context.Context, command filefacade.ReleaseWorkforceInputCommand) error {
	refID, err := parseWorkforceInputHandle(command.Handle)
	if err != nil || strings.TrimSpace(command.ExpectedScopeID) == "" {
		return apperrors.Params("Workforce 输入句柄无效")
	}
	ref, err := s.repo.GetReference(ctx, refID)
	if err != nil {
		return err
	}
	if ref == nil {
		return nil
	}
	if ref.BizType != filefacade.WorkforceInputBizType || strings.TrimSpace(ref.ScopeID) != strings.TrimSpace(command.ExpectedScopeID) {
		return apperrors.Forbidden("Workforce 输入句柄不属于当前组织")
	}
	return s.repo.SoftDeleteReferenceInScope(ctx, ref.FileID, ref.UserID, ref.ScopeID, ref.BizType, ref.BizID)
}

func parseWorkforceInputHandle(handle string) (int64, error) {
	handle = strings.TrimSpace(handle)
	if !strings.HasPrefix(handle, "wfi_") {
		return 0, fmt.Errorf("invalid Workforce input handle")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(handle, "wfi_"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid Workforce input handle")
	}
	return id, nil
}

// BindConfigAsset turns an uploaded file into the one active asset for a
// server-derived system configuration. The only caller is system-config; the
// HTTP request never supplies reference ownership, scope, access policy, or a
// visit URL. userId remains the binding operator for audit/credential checks,
// while bizId is always the configuration ID.
func (s *Service) BindConfigAsset(ctx context.Context, command filefacade.BindConfigAssetCommand) error {
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	orgScope, scopeErr := securitycontext.ResolveOrganizationScope(currentUser)
	if scopeErr != nil {
		return apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	if command.FileID <= 0 || command.ConfigID <= 0 {
		return apperrors.Params("配置资产文件和配置ID不能为空")
	}
	policy, err := configAssetBindingPolicy(command.AssetType, command.Exposure)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	userID := currentUser.UserID
	credential, err := s.repo.FindUploadCredential(ctx, userID, orgScope.ScopeID, command.FileID)
	if err != nil {
		return err
	}
	if credential == nil || !credential.Authorizes(userID, orgScope.ScopeID, command.FileID, time.Now().UTC()) {
		return apperrors.Forbidden("上传凭据无效或已过期")
	}
	file, err := s.repo.GetFile(ctx, command.FileID)
	if err != nil {
		return err
	}
	if err := s.validateConfigAssetFile(ctx, file, command.AssetType); err != nil {
		return err
	}

	return s.withTx(ctx, func(txCtx context.Context) error {
		lockedFile, lockErr := s.repo.GetFileForUpdate(txCtx, command.FileID)
		if lockErr != nil {
			return lockErr
		}
		// CONFIG_ASSET bytes are deliberately served only through the owning
		// configuration route. Hold the file lock while rejecting every active
		// non-config reference (and a different config slot) so a later public
		// avatar/generic binding cannot create a second presentation authority.
		if err := s.ensureConfigAssetFileExclusive(txCtx, lockedFile.ID, command.ConfigID); err != nil {
			return err
		}
		if err := s.validateConfigAssetFile(txCtx, lockedFile, command.AssetType); err != nil {
			return err
		}
		existing, findErr := s.repo.FindConfigAssetReference(txCtx, command.ConfigID)
		if findErr != nil {
			return findErr
		}
		if existing != nil {
			if strings.TrimSpace(existing.ScopeID) != orgScope.ScopeID {
				return apperrors.Forbidden("配置资产属于其他组织范围，不能替换")
			}
			if existing.FileID == lockedFile.ID {
				applyConfigAssetReferencePolicy(existing, *lockedFile, command.ConfigID, policy)
				return s.repo.UpdateConfigAssetReference(txCtx, existing)
			}
			if err := s.repo.SoftDeleteConfigAssetReference(txCtx, command.ConfigID, orgScope.ScopeID); err != nil {
				return err
			}
		}
		ref := &domain.FileReference{
			ID:      s.nextID(),
			FileID:  lockedFile.ID,
			UserID:  userID,
			ScopeID: orgScope.ScopeID,
			BizType: filefacade.ConfigAssetBizType,
			BizID:   command.ConfigID,
		}
		applyConfigAssetReferencePolicy(ref, *lockedFile, command.ConfigID, policy)
		_, err = s.repo.InsertReference(txCtx, ref)
		return err
	})
}

// UpdateConfigAssetPolicy keeps the stored reference aligned with the owning
// configuration's normalized exposure. It refuses cross-scope mutation and
// cannot be reached through file-management access-level APIs.
func (s *Service) UpdateConfigAssetPolicy(ctx context.Context, command filefacade.UpdateConfigAssetPolicyCommand) error {
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	orgScope, scopeErr := securitycontext.ResolveOrganizationScope(currentUser)
	if scopeErr != nil {
		return apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	if command.ConfigID <= 0 {
		return apperrors.Params("配置ID不能为空")
	}
	policy, err := configAssetBindingPolicy(command.AssetType, command.Exposure)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	return s.withTx(ctx, func(txCtx context.Context) error {
		ref, findErr := s.repo.FindConfigAssetReference(txCtx, command.ConfigID)
		if findErr != nil || ref == nil {
			return findErr
		}
		if strings.TrimSpace(ref.ScopeID) != orgScope.ScopeID {
			return apperrors.Forbidden("配置资产属于其他组织范围，不能修改")
		}
		file, getErr := s.repo.GetFileForUpdate(txCtx, ref.FileID)
		if getErr != nil {
			return getErr
		}
		if err := s.validateConfigAssetFile(txCtx, file, command.AssetType); err != nil {
			return err
		}
		applyConfigAssetReferencePolicy(ref, *file, command.ConfigID, policy)
		return s.repo.UpdateConfigAssetReference(txCtx, ref)
	})
}

// ClearConfigAsset removes the active reference in the same outer
// configuration transaction. It deliberately leaves the uploaded object to
// DC1 lifecycle cleanup rather than deleting bytes directly.
func (s *Service) ClearConfigAsset(ctx context.Context, configID int64) error {
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	orgScope, scopeErr := securitycontext.ResolveOrganizationScope(currentUser)
	if scopeErr != nil {
		return apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	if configID <= 0 {
		return apperrors.Params("配置ID不能为空")
	}
	return s.withTx(ctx, func(txCtx context.Context) error {
		ref, err := s.repo.FindConfigAssetReference(txCtx, configID)
		if err != nil || ref == nil {
			return err
		}
		if strings.TrimSpace(ref.ScopeID) != orgScope.ScopeID {
			return apperrors.Forbidden("配置资产属于其他组织范围，不能清除")
		}
		return s.repo.SoftDeleteConfigAssetReference(txCtx, configID, orgScope.ScopeID)
	})
}

// CaptureConfigAssetBinding returns a private description of the active
// CONFIG_ASSET slot. The system-config application calls it inside its outer
// mutation transaction before and after a bind/clear operation, then stores
// the result only in its non-API audit payload. A client never supplies or
// receives this state.
func (s *Service) CaptureConfigAssetBinding(ctx context.Context, command filefacade.CaptureConfigAssetBindingCommand) (filefacade.ConfigAssetBindingState, error) {
	currentUser, orgScope, err := configAssetOperatorScope(ctx)
	if err != nil {
		return filefacade.ConfigAssetBindingState{}, err
	}
	_ = currentUser // scope resolution also rejects anonymous/non-user contexts.
	if command.ConfigID <= 0 {
		return filefacade.ConfigAssetBindingState{}, apperrors.Params("配置ID不能为空")
	}
	policy, err := configAssetBindingPolicy(command.AssetType, command.Exposure)
	if err != nil {
		return filefacade.ConfigAssetBindingState{}, apperrors.Params(err.Error())
	}
	state := emptyConfigAssetBindingState(command.ConfigID, orgScope.ScopeID, command.AssetType, command.Exposure)
	err = s.withTx(ctx, func(txCtx context.Context) error {
		ref, findErr := s.repo.FindConfigAssetReference(txCtx, command.ConfigID)
		if findErr != nil {
			return findErr
		}
		if ref == nil {
			return nil
		}
		captured, captureErr := configAssetBindingStateFromReference(ref, orgScope.ScopeID, command, policy)
		if captureErr != nil {
			return captureErr
		}
		state = captured
		return nil
	})
	if err != nil {
		return filefacade.ConfigAssetBindingState{}, err
	}
	return state, nil
}

// RestoreConfigAssetBinding is a server-derived history recovery primitive.
// It is deliberately not a normal bind: it accepts no upload credential and
// is only reachable from the configuration rollback use case after that use
// case has verified current write access and a log-bound step-up proof. This
// method still independently validates the server-authenticated organization,
// expected active slot, file integrity/content, and the CONFIG_ASSET-only
// reference boundary before changing sys_file_reference.
func (s *Service) RestoreConfigAssetBinding(ctx context.Context, command filefacade.RestoreConfigAssetBindingCommand) error {
	currentUser, orgScope, err := configAssetOperatorScope(ctx)
	if err != nil {
		return err
	}
	if command.ConfigID <= 0 {
		return apperrors.Params("配置ID不能为空")
	}
	policy, err := configAssetBindingPolicy(command.AssetType, command.Exposure)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	if command.Expected.AssetType != command.AssetType || command.Expected.Exposure != command.Exposure ||
		command.Restore.AssetType != command.AssetType {
		return apperrors.Operation("配置资产历史类型或当前策略不一致")
	}
	restorePolicy, err := configAssetBindingPolicy(command.Restore.AssetType, command.Restore.Exposure)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	if err := validateConfigAssetBindingState(command.Expected, command.ConfigID, orgScope.ScopeID, command.AssetType, command.Exposure); err != nil {
		return err
	}
	if err := validateConfigAssetBindingState(command.Restore, command.ConfigID, orgScope.ScopeID, command.Restore.AssetType, command.Restore.Exposure); err != nil {
		return err
	}
	if configAssetBindingStatesEqual(command.Expected, command.Restore) {
		return apperrors.Params("配置资产历史状态没有可恢复的变化")
	}

	return s.withTx(ctx, func(txCtx context.Context) error {
		lockedFiles, lockErr := s.lockConfigAssetRestoreFiles(txCtx, command.Expected, command.Restore)
		if lockErr != nil {
			return lockErr
		}
		actual, findErr := s.repo.FindConfigAssetReference(txCtx, command.ConfigID)
		if findErr != nil {
			return findErr
		}
		actualState, stateErr := configAssetBindingStateFromReference(actual, orgScope.ScopeID, filefacade.CaptureConfigAssetBindingCommand{
			ConfigID: command.ConfigID, AssetType: command.AssetType, Exposure: command.Exposure,
		}, policy)
		if stateErr != nil {
			return stateErr
		}
		if !configAssetBindingStatesEqual(actualState, command.Expected) {
			return apperrors.ObjectState("配置资产已被其他变更修改，不能回滚历史记录")
		}
		if command.Expected.State == filefacade.ConfigAssetBindingBound {
			if err := s.ensureConfigAssetFileExclusive(txCtx, command.Expected.FileID, command.ConfigID); err != nil {
				return err
			}
		}
		if command.Restore.State == filefacade.ConfigAssetBindingBound {
			if err := s.ensureConfigAssetFileExclusive(txCtx, command.Restore.FileID, command.ConfigID); err != nil {
				return err
			}
			target := lockedFiles[command.Restore.FileID]
			if err := s.validateConfigAssetFile(txCtx, target, command.Restore.AssetType); err != nil {
				return err
			}
		}
		if actual != nil {
			if err := s.repo.SoftDeleteConfigAssetReference(txCtx, command.ConfigID, orgScope.ScopeID); err != nil {
				return err
			}
		}
		if command.Restore.State == filefacade.ConfigAssetBindingEmpty {
			return nil
		}
		target := lockedFiles[command.Restore.FileID]
		ref := &domain.FileReference{
			ID:      s.nextID(),
			FileID:  target.ID,
			UserID:  currentUser.UserID,
			ScopeID: orgScope.ScopeID,
			BizType: filefacade.ConfigAssetBizType,
			BizID:   command.ConfigID,
		}
		applyConfigAssetReferencePolicy(ref, *target, command.ConfigID, restorePolicy)
		_, insertErr := s.repo.InsertReference(txCtx, ref)
		return insertErr
	})
}

// OpenConfigAsset opens only a CONFIG_ASSET reference. Authorization is owned
// by system-config because only it can evaluate the configuration exposure;
// this facade never offers a public fileId or referenceId route for it.
func (s *Service) OpenConfigAsset(ctx context.Context, configID int64) (*filefacade.ConfigAssetOpenResult, error) {
	if configID <= 0 {
		return nil, apperrors.Params("配置ID不能为空")
	}
	ref, err := s.repo.FindConfigAssetReference(ctx, configID)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, apperrors.NotFound("配置资产不存在")
	}
	// The system-config caller evaluates typed exposure, but the reference is
	// also an organization-owned object. Enforce its server-derived scope here
	// before opening storage so an AUTHENTICATED or INTERNAL stable path cannot
	// become a cross-organization capability when a caller knows configID.
	if err := authorizeConfigAssetReadScope(ctx, ref); err != nil {
		return nil, err
	}
	file, err := s.repo.GetFile(ctx, ref.FileID)
	if err != nil {
		return nil, err
	}
	assetType, err := inferConfigAssetType(file)
	if err != nil {
		return nil, err
	}
	if err := s.validateConfigAssetFile(ctx, file, assetType); err != nil {
		return nil, err
	}
	strategy, err := s.storageStrategyForFile(ctx, *file)
	if err != nil {
		return nil, err
	}
	object, err := s.storage.Open(ctx, strategy, *file)
	if err != nil {
		return nil, apperrors.NotFound("配置资产不存在")
	}
	exposure, err := configAssetExposureFromAccessScope(ref.AccessScope)
	if err != nil {
		_ = object.File.Close()
		return nil, apperrors.Operation("配置资产访问策略无效")
	}
	// Re-emit only the normalized allowlisted media type. The stored declared
	// value participated in validation, but it is still not a response-header
	// authority (for example, it cannot carry arbitrary parameters).
	contentType := normalizeImageMIME(file.ContentType)
	if assetType == filefacade.ConfigAssetFile {
		// File assets are download-only. Do not let user-controlled declared MIME
		// select an executable browser renderer even after validation.
		contentType = "application/octet-stream"
	}
	return &filefacade.ConfigAssetOpenResult{
		Reader:      object.File,
		Size:        object.Size,
		ContentType: contentType,
		FileName:    filepath.Base(file.FileInnerName),
		AssetType:   assetType,
		AccessScope: exposure,
	}, nil
}

// ensureNoConfigAssetReference prevents generic file APIs from issuing or
// accepting a second capability for bytes owned by a CONFIG_ASSET. It is used
// at bind time and at generic download/token boundaries as defense in depth
// for historical data that could predate the binding isolation rule.
func (s *Service) ensureNoConfigAssetReference(ctx context.Context, fileID int64) error {
	refs, err := s.repo.ListReferencesByFile(ctx, fileID)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.IsDeleted == 0 && ref.BizType == filefacade.ConfigAssetBizType {
			return apperrors.Forbidden("配置资产只能通过配置稳定引用访问")
		}
	}
	return nil
}

// ensureConfigAssetFileExclusive establishes the inverse side of the
// CONFIG_ASSET boundary while the file row is locked. The same file may be
// retried for its current configuration slot, but it cannot become an alias
// for another configuration or any generic business reference whose download
// policy could differ from the configuration's exposure.
func (s *Service) ensureConfigAssetFileExclusive(ctx context.Context, fileID, configID int64) error {
	refs, err := s.repo.ListReferencesByFile(ctx, fileID)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.IsDeleted != 0 {
			continue
		}
		if ref.BizType != filefacade.ConfigAssetBizType || ref.BizID != configID {
			return apperrors.Forbidden("上传文件已绑定其他业务或配置资产，不能复用")
		}
	}
	return nil
}

// authorizeConfigAssetReadScope keeps non-public CONFIG_ASSET reads bound to
// the scope that the server recorded at bind time. PUBLIC assets deliberately
// remain readable without an authenticated context so login branding can be
// served before a session exists. AUTHENTICATED and INTERNAL assets always
// require the current server-resolved organization to match the reference.
func authorizeConfigAssetReadScope(ctx context.Context, ref *domain.FileReference) error {
	if ref == nil {
		return apperrors.NotFound("配置资产不存在")
	}
	exposure, err := configAssetExposureFromAccessScope(ref.AccessScope)
	if err != nil {
		return apperrors.Operation("配置资产访问策略无效")
	}
	if exposure == filefacade.ConfigAssetPublic {
		return nil
	}
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID <= 0 {
		return apperrors.Unauthorized("该配置资产需要登录后访问")
	}
	orgScope, scopeErr := securitycontext.ResolveOrganizationScope(currentUser)
	if scopeErr != nil {
		return apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	if strings.TrimSpace(ref.ScopeID) == "" || strings.TrimSpace(ref.ScopeID) != orgScope.ScopeID {
		return apperrors.Forbidden("配置资产不属于当前组织范围")
	}
	return nil
}

func configAssetOperatorScope(ctx context.Context) (*securitycontext.UserContext, securitycontext.OrganizationScope, error) {
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID <= 0 {
		return nil, securitycontext.OrganizationScope{}, apperrors.Unauthorized("未登录或登录信息失效")
	}
	orgScope, scopeErr := securitycontext.ResolveOrganizationScope(currentUser)
	if scopeErr != nil {
		return nil, securitycontext.OrganizationScope{}, apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	return currentUser, orgScope, nil
}

func emptyConfigAssetBindingState(configID int64, scopeID string, assetType filefacade.ConfigAssetType, exposure filefacade.ConfigAssetExposure) filefacade.ConfigAssetBindingState {
	return filefacade.ConfigAssetBindingState{
		ConfigID: configID, State: filefacade.ConfigAssetBindingEmpty, ScopeID: strings.TrimSpace(scopeID),
		AssetType: assetType, Exposure: exposure,
	}
}

func configAssetBindingStateFromReference(ref *domain.FileReference, scopeID string, command filefacade.CaptureConfigAssetBindingCommand, policy configAssetReferencePolicy) (filefacade.ConfigAssetBindingState, error) {
	if ref == nil {
		return emptyConfigAssetBindingState(command.ConfigID, scopeID, command.AssetType, command.Exposure), nil
	}
	if ref.IsDeleted != 0 || ref.BizType != filefacade.ConfigAssetBizType || ref.BizID != command.ConfigID {
		return filefacade.ConfigAssetBindingState{}, apperrors.Operation("配置资产引用状态无效")
	}
	if strings.TrimSpace(ref.ScopeID) == "" || strings.TrimSpace(ref.ScopeID) != strings.TrimSpace(scopeID) {
		return filefacade.ConfigAssetBindingState{}, apperrors.Forbidden("配置资产不属于当前组织范围")
	}
	if ref.FileID <= 0 || ref.VisitURL != filefacade.ConfigAssetStablePath(command.ConfigID) ||
		ref.AccessScope != string(policy.accessScope) || ref.VisitStrategy != string(policy.visitStrategy) ||
		ref.AccessLevel != accessLevelFromScope(ref.AccessScope) {
		return filefacade.ConfigAssetBindingState{}, apperrors.Operation("配置资产引用策略无效")
	}
	return filefacade.ConfigAssetBindingState{
		ConfigID: command.ConfigID, State: filefacade.ConfigAssetBindingBound, FileID: ref.FileID,
		ScopeID: strings.TrimSpace(ref.ScopeID), AssetType: command.AssetType, Exposure: command.Exposure,
	}, nil
}

func validateConfigAssetBindingState(state filefacade.ConfigAssetBindingState, configID int64, scopeID string, assetType filefacade.ConfigAssetType, exposure filefacade.ConfigAssetExposure) error {
	if state.ConfigID != configID || strings.TrimSpace(state.ScopeID) == "" || strings.TrimSpace(state.ScopeID) != strings.TrimSpace(scopeID) ||
		state.AssetType != assetType || state.Exposure != exposure {
		return apperrors.Forbidden("配置资产历史快照不属于当前配置范围")
	}
	switch state.State {
	case filefacade.ConfigAssetBindingEmpty:
		if state.FileID != 0 {
			return apperrors.Operation("配置资产空快照包含文件")
		}
	case filefacade.ConfigAssetBindingBound:
		if state.FileID <= 0 {
			return apperrors.Operation("配置资产绑定快照缺少文件")
		}
	default:
		return apperrors.Operation("配置资产历史快照状态无效")
	}
	return nil
}

func configAssetBindingStatesEqual(left, right filefacade.ConfigAssetBindingState) bool {
	return left.ConfigID == right.ConfigID && left.State == right.State && left.FileID == right.FileID &&
		strings.TrimSpace(left.ScopeID) == strings.TrimSpace(right.ScopeID) && left.AssetType == right.AssetType && left.Exposure == right.Exposure
}

func (s *Service) lockConfigAssetRestoreFiles(ctx context.Context, states ...filefacade.ConfigAssetBindingState) (map[int64]*domain.FileInfo, error) {
	ids := make([]int64, 0, len(states))
	seen := make(map[int64]struct{}, len(states))
	for _, state := range states {
		if state.State != filefacade.ConfigAssetBindingBound || state.FileID <= 0 {
			continue
		}
		if _, exists := seen[state.FileID]; exists {
			continue
		}
		seen[state.FileID] = struct{}{}
		ids = append(ids, state.FileID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	locked := make(map[int64]*domain.FileInfo, len(ids))
	for _, fileID := range ids {
		file, err := s.repo.GetFileForUpdate(ctx, fileID)
		if err != nil {
			return nil, err
		}
		if file == nil {
			return nil, apperrors.NotFound("配置资产历史文件不存在")
		}
		locked[fileID] = file
	}
	return locked, nil
}

type configAssetReferencePolicy struct {
	accessScope   filefacade.FileAccessScope
	visitStrategy filefacade.FileVisitStrategy
}

func configAssetBindingPolicy(assetType filefacade.ConfigAssetType, exposure filefacade.ConfigAssetExposure) (configAssetReferencePolicy, error) {
	if _, err := normalizeConfigAssetType(assetType); err != nil {
		return configAssetReferencePolicy{}, err
	}
	switch exposure {
	case filefacade.ConfigAssetPublic:
		return configAssetReferencePolicy{accessScope: filefacade.AccessPublic, visitStrategy: filefacade.VisitPublicStatic}, nil
	case filefacade.ConfigAssetAuthenticated:
		return configAssetReferencePolicy{accessScope: filefacade.AccessLoginUsers, visitStrategy: filefacade.VisitPrivatePreview}, nil
	case filefacade.ConfigAssetInternal:
		return configAssetReferencePolicy{accessScope: filefacade.AccessOwnerOnly, visitStrategy: filefacade.VisitPrivateDownload}, nil
	default:
		return configAssetReferencePolicy{}, fmt.Errorf("unsupported config asset exposure %q", exposure)
	}
}

func applyConfigAssetReferencePolicy(ref *domain.FileReference, file domain.FileInfo, configID int64, policy configAssetReferencePolicy) {
	if ref == nil {
		return
	}
	ref.DisplayName = filepath.Base(file.FileInnerName)
	ref.BizType = filefacade.ConfigAssetBizType
	ref.BizID = configID
	ref.VisitURL = filefacade.ConfigAssetStablePath(configID)
	ref.AccessScope = string(policy.accessScope)
	ref.VisitStrategy = string(policy.visitStrategy)
	ref.AccessLevel = accessLevelFromScope(ref.AccessScope)
}

func normalizeConfigAssetType(value filefacade.ConfigAssetType) (filefacade.ConfigAssetType, error) {
	switch filefacade.ConfigAssetType(strings.ToUpper(strings.TrimSpace(string(value)))) {
	case filefacade.ConfigAssetImage:
		return filefacade.ConfigAssetImage, nil
	case filefacade.ConfigAssetFile:
		return filefacade.ConfigAssetFile, nil
	default:
		return "", fmt.Errorf("unsupported config asset type %q", value)
	}
}

func configAssetExposureFromAccessScope(value string) (filefacade.ConfigAssetExposure, error) {
	switch strings.TrimSpace(value) {
	case string(filefacade.AccessPublic):
		return filefacade.ConfigAssetPublic, nil
	case string(filefacade.AccessLoginUsers):
		return filefacade.ConfigAssetAuthenticated, nil
	case string(filefacade.AccessOwnerOnly):
		return filefacade.ConfigAssetInternal, nil
	default:
		return "", fmt.Errorf("unsupported config asset access scope %q", value)
	}
}

func (s *Service) validateConfigAssetFile(ctx context.Context, file *domain.FileInfo, assetType filefacade.ConfigAssetType) error {
	normalized, err := normalizeConfigAssetType(assetType)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	if err := validateBindableFile(file, filefacade.DefaultFile); err != nil {
		return err
	}
	switch normalized {
	case filefacade.ConfigAssetImage:
		return s.validateImageAsset(ctx, *file)
	case filefacade.ConfigAssetFile:
		return s.validateOrdinaryConfigFile(ctx, *file)
	default:
		return apperrors.Params("不支持的配置资产类型")
	}
}

func inferConfigAssetType(file *domain.FileInfo) (filefacade.ConfigAssetType, error) {
	if file == nil {
		return "", apperrors.NotFound("配置资产不存在")
	}
	if normalizeImageMIME(file.ContentType) != "" {
		return filefacade.ConfigAssetImage, nil
	}
	if normalizedConfigFileMIME(file.ContentType) != "" {
		return filefacade.ConfigAssetFile, nil
	}
	return "", apperrors.Forbidden("配置资产类型不受支持")
}

func (s *Service) validateOrdinaryConfigFile(ctx context.Context, file domain.FileInfo) error {
	const maxConfigFileBytes = int64(10 * 1024 * 1024)
	if file.FileSize <= 0 || file.FileSize > maxConfigFileBytes {
		return apperrors.Forbidden("配置文件大小超出限制")
	}
	declaredMIME := normalizedConfigFileMIME(file.ContentType)
	if declaredMIME == "" || !configFileExtensionMatches(file.FileInnerName, declaredMIME) {
		return apperrors.Forbidden("配置文件扩展名或类型不受支持")
	}
	strategy, err := s.storageStrategyForFile(ctx, file)
	if err != nil {
		return err
	}
	object, err := s.storage.Open(ctx, strategy, file)
	if err != nil {
		return apperrors.NotFound("文件内容不存在")
	}
	defer object.File.Close()
	payload, err := io.ReadAll(io.LimitReader(object.File, maxConfigFileBytes+1))
	if err != nil {
		return apperrors.System("读取配置文件内容失败")
	}
	if int64(len(payload)) != file.FileSize || int64(len(payload)) > maxConfigFileBytes {
		return apperrors.Forbidden("配置文件大小或存储元数据不一致")
	}
	detectedMIME := normalizedConfigFileMIME(http.DetectContentType(payload))
	if detectedMIME == "" || detectedMIME != declaredMIME {
		return apperrors.Forbidden("配置文件 MIME 与实际内容不一致")
	}
	switch declaredMIME {
	case "application/pdf":
		if !bytes.HasPrefix(payload, []byte("%PDF-")) {
			return apperrors.Forbidden("PDF 文件内容无效")
		}
	case "text/plain":
		if !utf8.Valid(payload) || bytes.IndexByte(payload, 0) >= 0 {
			return apperrors.Forbidden("文本文件内容无效")
		}
		lower := strings.ToLower(string(payload))
		if strings.Contains(lower, "<script") || strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
			return apperrors.Forbidden("文本文件不得包含活动网页内容")
		}
	}
	return nil
}

func normalizedConfigFileMIME(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	switch strings.ToLower(mediaType) {
	case "application/pdf":
		return "application/pdf"
	case "text/plain":
		return "text/plain"
	default:
		return ""
	}
}

func configFileExtensionMatches(fileName, contentType string) bool {
	switch contentType {
	case "application/pdf":
		return strings.EqualFold(filepath.Ext(fileName), ".pdf")
	case "text/plain":
		return strings.EqualFold(filepath.Ext(fileName), ".txt")
	default:
		return false
	}
}

func (s *Service) validateImageAsset(ctx context.Context, file domain.FileInfo) error {
	const (
		maxImageBytes  = int64(2 * 1024 * 1024)
		maxImageSide   = 4096
		maxImagePixels = int64(16 * 1024 * 1024)
	)
	if file.FileSize <= 0 || file.FileSize > maxImageBytes {
		return apperrors.Forbidden("图片大小超出限制")
	}
	strategy, err := s.storageStrategyForFile(ctx, file)
	if err != nil {
		return err
	}
	object, err := s.storage.Open(ctx, strategy, file)
	if err != nil {
		return apperrors.NotFound("文件内容不存在")
	}
	defer object.File.Close()
	payload, err := io.ReadAll(io.LimitReader(object.File, maxImageBytes+1))
	if err != nil {
		return apperrors.System("读取文件内容失败")
	}
	if int64(len(payload)) != file.FileSize || int64(len(payload)) > maxImageBytes {
		return apperrors.Forbidden("图片大小或存储元数据不一致")
	}
	declaredMIME := normalizeImageMIME(file.ContentType)
	detectedMIME := normalizeImageMIME(http.DetectContentType(payload))
	if declaredMIME == "" || detectedMIME == "" || declaredMIME != detectedMIME {
		return apperrors.Forbidden("图片 MIME 与实际内容不一致")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		return apperrors.Forbidden("图片内容无法解析")
	}
	formatMIME := imageFormatMIME(format)
	if formatMIME == "" || formatMIME != declaredMIME || !imageExtensionMatches(file.FileInnerName, format) {
		return apperrors.Forbidden("图片扩展名、MIME 与内容不一致")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageSide || config.Height > maxImageSide ||
		int64(config.Width)*int64(config.Height) > maxImagePixels {
		return apperrors.Forbidden("图片尺寸超出限制")
	}
	if !imageHasExactContainerLength(payload, format) {
		return apperrors.Forbidden("图片包含额外或畸形载荷")
	}
	if _, decodedFormat, err := image.Decode(bytes.NewReader(payload)); err != nil || decodedFormat != format {
		return apperrors.Forbidden("图片内容无法完整解码")
	}
	return nil
}

func (s *Service) userOwnsFileReference(ctx context.Context, fileID, userID int64, scopeID string) (bool, error) {
	if fileID <= 0 || userID <= 0 || strings.TrimSpace(scopeID) == "" {
		return false, nil
	}
	refs, err := s.repo.ListReferencesByFile(ctx, fileID)
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		if ref.IsDeleted == 0 && ref.UserID == userID && strings.TrimSpace(ref.ScopeID) == scopeID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) hasFileAuthority(ctx context.Context, fileID, userID int64, scopeID string) (bool, error) {
	if fileID <= 0 || userID <= 0 || strings.TrimSpace(scopeID) == "" {
		return false, nil
	}
	credential, err := s.repo.FindUploadCredential(ctx, userID, scopeID, fileID)
	if err != nil {
		return false, err
	}
	if credential != nil && credential.Authorizes(userID, scopeID, fileID, time.Now().UTC()) {
		return true, nil
	}
	return s.userOwnsFileReference(ctx, fileID, userID, scopeID)
}

func (s *Service) SaveFileInfo(ctx context.Context, file filefacade.FileInfoDTO) (bool, error) {
	item := &domain.FileInfo{
		ID:                file.ID,
		FileInnerName:     file.FileInnerName,
		FileSize:          file.FileSize,
		FileSha256:        file.FileSha256,
		ContentType:       file.ContentType,
		StorageStrategyID: file.StorageStrategyID,
		StoragePath:       file.StoragePath,
		Status:            file.Status,
		ScanStatus:        file.ScanStatus,
	}
	_, err := s.repo.InsertFile(ctx, item)
	return err == nil, err
}

func (s *Service) RemoveFile(ctx context.Context, fileID, userID int64, bizType string, bizID int64) (bool, error) {
	if strings.TrimSpace(bizType) != "" && userID > 0 {
		if err := s.repo.SoftDeleteReference(ctx, fileID, userID, bizType, bizID); err != nil {
			return false, err
		}
		return true, nil
	}
	file, err := s.repo.GetFile(ctx, fileID)
	if err != nil {
		return false, err
	}
	if file == nil {
		return false, apperrors.NotFound("文件不存在")
	}
	return s.claimAndDeleteUnreferencedFile(ctx, *file, time.Now().UTC(), true)
}

func (s *Service) claimAndDeleteUnreferencedFile(ctx context.Context, file domain.FileInfo, now time.Time, strict bool) (bool, error) {
	if file.ID <= 0 || file.IsDeleted != 0 || file.Status == domain.FileStatusDeleted {
		return false, nil
	}
	claimed := file.Status == domain.FileStatusCleaning
	if !claimed {
		if file.Status != domain.FileStatusAvailable {
			return false, apperrors.Operation("文件当前状态不可删除")
		}
		if err := s.withTx(ctx, func(txCtx context.Context) error {
			lockedFile, err := s.repo.GetFileForUpdate(txCtx, file.ID)
			if err != nil {
				return err
			}
			if lockedFile == nil || lockedFile.Status != domain.FileStatusAvailable {
				return nil
			}
			hasReferences, err := s.repo.HasActiveReferences(txCtx, file.ID)
			if err != nil {
				return err
			}
			hasProtectedCredential, err := s.repo.HasProtectedCredential(txCtx, file.ID, now)
			if err != nil {
				return err
			}
			if hasReferences || hasProtectedCredential {
				return nil
			}
			claimed, err = s.repo.ClaimFileForCleanup(txCtx, file.ID, now)
			return err
		}); err != nil {
			return false, err
		}
	}
	if !claimed {
		if !strict {
			return false, nil
		}
		return false, apperrors.Operation("文件存在有效引用或保护凭据")
	}
	hasReferences, err := s.repo.HasActiveReferences(ctx, file.ID)
	if err != nil {
		_ = s.repo.RestoreFileAvailableIfCleaning(ctx, file.ID)
		return false, err
	}
	hasProtectedCredential, err := s.repo.HasProtectedCredential(ctx, file.ID, now)
	if err != nil {
		_ = s.repo.RestoreFileAvailableIfCleaning(ctx, file.ID)
		return false, err
	}
	if hasReferences || hasProtectedCredential {
		if err := s.repo.RestoreFileAvailableIfCleaning(ctx, file.ID); err != nil {
			return false, err
		}
		if !strict {
			return false, nil
		}
		return false, apperrors.Operation("文件存在有效引用或保护凭据")
	}
	strategy, err := s.storageStrategyForFile(ctx, file)
	if err != nil {
		_ = s.repo.RestoreFileAvailableIfCleaning(ctx, file.ID)
		return false, err
	}
	if err := s.storage.Delete(ctx, strategy, file.StoragePath); err != nil {
		_ = s.repo.RestoreFileAvailableIfCleaning(ctx, file.ID)
		return false, fmt.Errorf("delete unreferenced file object: %w", err)
	}
	deleted, err := s.repo.MarkFileDeletedIfCleaning(ctx, file.ID)
	if err != nil {
		return false, err
	}
	if !deleted {
		return false, apperrors.Operation("文件清理状态已变化")
	}
	return true, nil
}

func (s *Service) BatchRemoveByIDs(ctx context.Context, fileIDs []int64, userID int64, bizType string, bizID int64) (bool, error) {
	ids := uniquePositiveInt64s(fileIDs)
	if len(ids) > maintenanceMaxItems {
		return false, apperrors.Params("文件数量超过单次批量上限")
	}
	if len(ids) == 0 {
		return true, nil
	}
	if strings.TrimSpace(bizType) != "" && userID > 0 {
		for _, id := range ids {
			if _, err := s.RemoveFile(ctx, id, userID, bizType, bizID); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	batchRepo, ok := s.repo.(fileBatchReadRepository)
	if !ok {
		return false, apperrors.System("文件批量读取仓储能力未配置")
	}
	files, err := batchRepo.ListFilesByIDs(ctx, ids)
	if err != nil {
		return false, err
	}
	fileByID := make(map[int64]domain.FileInfo, len(files))
	for _, file := range files {
		fileByID[file.ID] = file
	}
	if len(fileByID) != len(ids) {
		return false, apperrors.NotFound("部分文件不存在")
	}
	now := time.Now().UTC()
	for _, id := range ids {
		file := fileByID[id]
		// Each file deliberately retains its own claim/recheck/object-delete
		// lifecycle; these external side effects cannot share one transaction.
		if _, err := s.claimAndDeleteUnreferencedFile(ctx, file, now, true); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Service) ListFileByBiz(ctx context.Context, userID int64, bizType filefacade.FileActionBiz, bizID int64) ([]filefacade.FileReferenceDTO, error) {
	refs, err := s.repo.ListReferencesByBiz(ctx, userID, strconv.Itoa(int(bizType)), bizID)
	if err != nil {
		return nil, err
	}
	result := make([]filefacade.FileReferenceDTO, 0, len(refs))
	for _, ref := range refs {
		result = append(result, toReferenceDTO(ref))
	}
	return result, nil
}

func (s *Service) QueryFiles(ctx context.Context, current, size int64, fileName, fileType string, bizType *int, startTime, endTime string) (*filefacade.PageResult[filefacade.FileInfoVO], error) {
	page, err := s.repo.QueryFiles(ctx, current, size, fileName, fileType, bizType, startTime, endTime)
	if err != nil {
		return nil, err
	}
	records := make([]filefacade.FileInfoVO, 0, len(page.Records))
	for _, item := range page.Records {
		records = append(records, toFileInfoVO(item))
	}
	return &filefacade.PageResult[filefacade.FileInfoVO]{Current: page.Current, Size: page.Size, Total: page.Total, Records: records}, nil
}

func (s *Service) GetFile(ctx context.Context, id int64) (*filefacade.FileInfoVO, error) {
	item, err := s.repo.GetFile(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("文件不存在")
	}
	vo := toFileInfoVO(*item)
	return &vo, nil
}

func (s *Service) ListReferences(ctx context.Context, fileID int64) ([]filefacade.FileReferenceVO, error) {
	refs, err := s.repo.ListReferencesByFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	result := make([]filefacade.FileReferenceVO, 0, len(refs))
	for _, ref := range refs {
		// CONFIG_ASSET reference IDs and policy are deliberately not a generic
		// file-management surface. Their only presentation route is the owning
		// config's stable path, whose exposure is evaluated by system-config.
		if ref.BizType == filefacade.ConfigAssetBizType {
			continue
		}
		result = append(result, toReferenceVO(ref))
	}
	return result, nil
}

func (s *Service) UpdateReferenceAccess(ctx context.Context, id int64, accessScope, visitStrategy string, accessLevel *int) (*filefacade.FileReferenceVO, error) {
	current, err := s.repo.GetReference(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, apperrors.NotFound("文件引用不存在")
	}
	if current.BizType == filefacade.ConfigAssetBizType {
		return nil, apperrors.Forbidden("配置资产访问策略由配置暴露级别派生，不能直接修改")
	}
	normalizedScope := strings.TrimSpace(accessScope)
	normalizedVisit := strings.TrimSpace(visitStrategy)
	if accessLevel != nil {
		if *accessLevel == 1 {
			normalizedScope = string(filefacade.AccessPublic)
			normalizedVisit = string(filefacade.VisitPublicStatic)
		} else {
			normalizedScope = string(filefacade.AccessOwnerOnly)
			normalizedVisit = string(filefacade.VisitPrivatePreview)
		}
	}
	level := accessLevelFromScope(normalizedScope)
	visitURL := ""
	if normalizedVisit == string(filefacade.VisitPublicStatic) {
		visitURL = withQueryParam(s.downloadGatewayPath(), "referenceId", strconv.FormatInt(id, 10))
	}
	ref, err := s.repo.UpdateReferenceAccess(ctx, id, normalizedScope, normalizedVisit, level, visitURL)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, apperrors.NotFound("文件引用不存在")
	}
	vo := toReferenceVO(*ref)
	return &vo, nil
}

func (s *Service) FileStats(ctx context.Context) (map[string]any, error) {
	return s.repo.FileStats(ctx)
}

func (s *Service) StorageStrategies(ctx context.Context) ([]filefacade.StorageStrategyVO, error) {
	items, err := s.repo.ListStrategies(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]filefacade.StorageStrategyVO, 0, len(items))
	for _, item := range items {
		result = append(result, toStrategyVO(item))
	}
	return result, nil
}

func (s *Service) CreateStorageStrategy(ctx context.Context, command StorageStrategyCommand) (int64, error) {
	item, err := storageStrategyFromCommand(command)
	if err != nil {
		return 0, err
	}
	item.ID = s.nextID()
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		if _, err := s.repo.InsertStrategy(txCtx, item); err != nil {
			return err
		}
		if item.IsDefault {
			return s.repo.SetOnlyDefaultStrategy(txCtx, item.ID)
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return item.ID, nil
}

func (s *Service) UpdateStorageStrategy(ctx context.Context, command StorageStrategyCommand) error {
	if command.ID <= 0 {
		return apperrors.Params("存储策略ID不能为空")
	}
	existing, err := s.repo.GetStrategy(ctx, command.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return apperrors.NotFound("存储策略不存在")
	}
	item, err := storageStrategyFromCommand(command)
	if err != nil {
		return err
	}
	item.ID = command.ID
	return s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.UpdateStrategy(txCtx, item); err != nil {
			return err
		}
		if item.IsDefault {
			return s.repo.SetOnlyDefaultStrategy(txCtx, item.ID)
		}
		return nil
	})
}

func (s *Service) DeleteStorageStrategy(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperrors.Params("存储策略ID不能为空")
	}
	return s.repo.DeleteStrategy(ctx, id)
}

func (s *Service) StorageStrategy(ctx context.Context, id int64) (*filefacade.StorageStrategyVO, error) {
	item, err := s.repo.GetStrategy(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("存储策略不存在")
	}
	vo := toStrategyVO(*item)
	return &vo, nil
}

func (s *Service) SetDefaultStrategy(ctx context.Context, id int64) error {
	return s.repo.SetOnlyDefaultStrategy(ctx, id)
}

func (s *Service) EnableStrategy(ctx context.Context, id int64, enabled bool) error {
	return s.repo.EnableStrategy(ctx, id, enabled)
}

func (s *Service) CheckStorageHealth(ctx context.Context, id int64) (*filefacade.StorageStrategyHealthVO, error) {
	strategy, err := s.repo.GetStrategy(ctx, id)
	if err != nil {
		return nil, err
	}
	if strategy == nil {
		return nil, apperrors.NotFound("存储策略不存在")
	}
	healthErr := s.storage.Health(ctx, *strategy)
	healthy := strategy.Writable() && healthErr == nil
	status := domain.HealthHealthy
	message := "ok"
	if !healthy {
		status = domain.HealthUnhealthy
		message = "strategy is not writable"
		if healthErr != nil {
			message = healthErr.Error()
		}
	}
	if err := s.repo.UpdateStrategyHealth(ctx, id, status, healthy); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &filefacade.StorageStrategyHealthVO{StrategyID: id, HealthStatus: status, Healthy: healthy, Message: message, LastHealthCheck: &now}, nil
}

func (s *Service) CheckAllStorageHealth(ctx context.Context, limit int) error {
	batchRepo, ok := s.repo.(maintenanceBatchRepository)
	if !ok {
		return apperrors.System("文件维护批量仓储能力未配置")
	}
	items, err := s.repo.ListStrategies(ctx)
	if err != nil {
		return err
	}
	maxItems := maintenanceLimit(limit)
	if len(items) > maxItems {
		items = items[:maxItems]
	}
	updates := make([]domain.StorageHealthUpdate, 0, len(items))
	for _, item := range items {
		healthy := false
		if item.Writable() {
			healthy = s.storage.Health(ctx, item) == nil
		}
		status := domain.HealthHealthy
		if !healthy {
			status = domain.HealthUnhealthy
		}
		updates = append(updates, domain.StorageHealthUpdate{StrategyID: item.ID, HealthStatus: status, Healthy: healthy})
	}
	for start := 0; start < len(updates); start += maintenanceChunkSize {
		end := min(start+maintenanceChunkSize, len(updates))
		chunk := updates[start:end]
		if s.transactor == nil || !s.transactor.Enabled() {
			return apperrors.System("存储健康检查事务能力未配置")
		}
		if err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			matched, updateErr := batchRepo.UpdateStrategyHealthBatch(txCtx, chunk)
			if updateErr != nil {
				return updateErr
			}
			if matched != int64(len(chunk)) {
				return fmt.Errorf("存储策略状态发生并发变化")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RetryPendingProcessTasks(ctx context.Context, limit int) error {
	batchRepo, ok := s.repo.(maintenanceBatchRepository)
	if !ok {
		return apperrors.System("文件维护批量仓储能力未配置")
	}
	items, err := s.repo.ListPendingRetryProcessTasks(ctx, time.Now(), maintenanceLimit(limit))
	if err != nil {
		return err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if len(items) == 0 {
		return nil
	}
	if !s.outbox || s.outboxStore == nil {
		return apperrors.System("文件处理重试 outbox 能力未配置")
	}
	if s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("文件处理重试事务能力未配置")
	}
	for start := 0; start < len(items); start += maintenanceChunkSize {
		end := min(start+maintenanceChunkSize, len(items))
		ids := make([]int64, 0, end-start)
		for _, item := range items[start:end] {
			ids = append(ids, item.ID)
		}
		if err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			matched, resetErr := batchRepo.ResetPendingRetryProcessTasks(txCtx, ids)
			if resetErr != nil {
				return resetErr
			}
			if matched != int64(len(ids)) {
				return fmt.Errorf("文件处理任务状态发生并发变化")
			}
			return s.appendFileProcessOutboxBatch(txCtx, items[start:end])
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RetryPendingBindingTasks(ctx context.Context, limit int) error {
	batchRepo, ok := s.repo.(maintenanceBatchRepository)
	if !ok {
		return apperrors.System("文件维护批量仓储能力未配置")
	}
	items, err := s.repo.ListRetryBindingTasks(ctx, time.Now(), maintenanceLimit(limit))
	if err != nil {
		return err
	}
	items = uniqueBindingTasks(items)
	for index := range items {
		item := &items[index]
		biz, ok := domain.ResolveBiz(item.BizType)
		if !ok {
			biz = domain.DefaultFileBiz
		}
		binding := defaultBindingResult(biz, item.FileName, filefacade.FileActionBiz(item.BizType))
		item.Status = domain.BindingBound
		item.DisplayName = binding.DisplayName
		item.VisitStrategy = string(binding.VisitStrategy)
		item.AccessScope = string(binding.AccessScope)
		item.LastError = ""
	}
	for start := 0; start < len(items); start += maintenanceChunkSize {
		end := min(start+maintenanceChunkSize, len(items))
		chunk := items[start:end]
		if s.transactor == nil || !s.transactor.Enabled() {
			return apperrors.System("文件绑定重试事务能力未配置")
		}
		if err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			matched, markErr := batchRepo.MarkBindingTasks(txCtx, chunk)
			if markErr != nil {
				return markErr
			}
			if matched != int64(len(chunk)) {
				return fmt.Errorf("文件绑定任务状态发生并发变化")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func uniqueBindingTasks(items []domain.FileBindingTask) []domain.FileBindingTask {
	seen := make(map[int64]struct{}, len(items))
	result := make([]domain.FileBindingTask, 0, len(items))
	for _, item := range items {
		if item.ID <= 0 {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func uniqueUploadTasks(items []domain.UploadTask) []domain.UploadTask {
	seen := make(map[string]struct{}, len(items))
	result := make([]domain.UploadTask, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		item.ID = id
		result = append(result, item)
	}
	return result
}

func maintenanceLimit(limit int) int {
	if limit <= 0 || limit > maintenanceMaxItems {
		return maintenanceMaxItems
	}
	return limit
}

func (s *Service) QueryProcessTasks(ctx context.Context, current, size int64, status *int, taskType string) (*filefacade.PageResult[filefacade.FileProcessTaskVO], error) {
	page, err := s.repo.QueryProcessTasks(ctx, current, size, status, taskType)
	if err != nil {
		return nil, err
	}
	records := make([]filefacade.FileProcessTaskVO, 0, len(page.Records))
	for _, item := range page.Records {
		records = append(records, toProcessTaskVO(item))
	}
	return &filefacade.PageResult[filefacade.FileProcessTaskVO]{Current: page.Current, Size: page.Size, Total: page.Total, Records: records}, nil
}

func (s *Service) ProcessTaskStats(ctx context.Context) (map[string]any, error) {
	var pending, running, done, failed int64
	for _, item := range []struct {
		status int
		target *int64
	}{
		{0, &pending},
		{1, &running},
		{2, &done},
		{3, &failed},
	} {
		page, err := s.repo.QueryProcessTasks(ctx, 1, 1, &item.status, "")
		if err != nil {
			return nil, err
		}
		*item.target = page.Total
	}
	return map[string]any{"pending": pending, "running": running, "done": done, "failed": failed}, nil
}

func (s *Service) GetProcessTask(ctx context.Context, id int64) (*filefacade.FileProcessTaskVO, error) {
	task, err := s.repo.GetProcessTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, apperrors.NotFound("文件处理任务不存在")
	}
	vo := toProcessTaskVO(*task)
	return &vo, nil
}

func (s *Service) RetryProcessTask(ctx context.Context, id int64) error {
	task, err := s.repo.GetProcessTask(ctx, id)
	if err != nil {
		return err
	}
	if task == nil {
		return apperrors.NotFound("文件处理任务不存在")
	}
	if task.Status != domain.ProcessTaskFailed && task.Status != domain.ProcessTaskPendingRetry {
		return apperrors.Operation("文件处理任务状态不允许重试")
	}
	batchRepo, ok := s.repo.(processTaskBatchRepository)
	if !ok || !s.outbox || s.outboxStore == nil || s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("文件处理任务可靠重试能力未配置")
	}
	return s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		matched, err := batchRepo.ResetProcessTasks(txCtx, []int64{id})
		if err != nil {
			return err
		}
		if matched != 1 {
			return fmt.Errorf("文件处理任务状态发生并发变化")
		}
		return s.appendFileProcessOutboxBatch(txCtx, []domain.FileProcessTask{*task})
	})
}

func (s *Service) BatchRetryProcessTasks(ctx context.Context, ids []int64) error {
	ids = uniquePositiveInt64s(ids)
	if len(ids) == 0 {
		return apperrors.Params("文件处理任务ID不能为空")
	}
	if len(ids) > 100 {
		return apperrors.Params("文件处理任务数量超过单次批量上限")
	}
	batchRepo, ok := s.repo.(processTaskBatchRepository)
	if !ok {
		return apperrors.System("文件处理任务批量仓储能力未配置")
	}
	tasks, err := batchRepo.ListProcessTasksByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if len(tasks) != len(ids) {
		return apperrors.NotFound("部分文件处理任务不存在")
	}
	for _, task := range tasks {
		if task.Status != domain.ProcessTaskFailed && task.Status != domain.ProcessTaskPendingRetry {
			return apperrors.Operation("部分文件处理任务状态不允许重试")
		}
	}
	if !s.outbox || s.outboxStore == nil || s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("文件处理任务可靠重试能力未配置")
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		matched, err := batchRepo.ResetProcessTasks(txCtx, ids)
		if err != nil {
			return err
		}
		if matched != int64(len(ids)) {
			return fmt.Errorf("文件处理任务状态发生并发变化")
		}
		return s.appendFileProcessOutboxBatch(txCtx, tasks)
	})
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func (s *Service) RelayOutbox(ctx context.Context, limit int) error {
	if !s.outbox {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	if s.outboxStore == nil {
		return fmt.Errorf("outbox store is not configured")
	}
	unknownEvents, err := s.outboxStore.ListUnknownOutbox(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range unknownEvents {
		lease, claimed, claimErr := s.outboxStore.TryClaimOutbox(ctx, event.ID, event.EventType, "file-outbox-relay")
		if claimErr != nil {
			return claimErr
		}
		if !claimed {
			continue
		}
		applied, markErr := s.outboxStore.MarkOutbox(ctx, event.ID, event.EventType, lease.Token, "DEAD", fmt.Sprintf("unsupported file outbox event type %q", event.EventType), event.RetryCount+1, nil)
		if markErr != nil {
			return markErr
		}
		if !applied {
			continue
		}
	}
	if s.rabbit == nil || !s.rabbit.Enabled() {
		return nil
	}
	events, err := s.outboxStore.ListReadyOutbox(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range events {
		lease, claimed, err := s.outboxStore.TryClaimOutbox(ctx, event.ID, event.EventType, "file-outbox-relay")
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		var publishErr error
		switch event.EventType {
		case "UPLOAD_TASK_READY", "UPLOAD_TASK_UPLOADED":
			if strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.AggregateID) == "" {
				if err := s.markPermanentOutboxFailure(ctx, event, lease.Token, "invalid upload task outbox payload"); err != nil {
					return err
				}
				continue
			}
			publishErr = s.rabbit.PublishUploadTask(ctx, domain.UploadTaskMessage{MessageID: event.EventID, TaskID: event.AggregateID})
		case "FILE_PROCESS_TASK":
			var payload struct {
				TaskID   int64  `json:"taskId"`
				FileID   int64  `json:"fileId"`
				TaskType string `json:"taskType"`
			}
			if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil ||
				strings.TrimSpace(event.EventID) == "" ||
				payload.TaskID <= 0 ||
				payload.FileID <= 0 ||
				strings.TrimSpace(payload.TaskType) == "" {
				if err := s.markPermanentOutboxFailure(ctx, event, lease.Token, "invalid file process outbox payload"); err != nil {
					return err
				}
				continue
			}
			publishErr = s.rabbit.PublishFileProcessTask(ctx, domain.FileProcessMessage{MessageID: event.EventID, TaskID: payload.TaskID, FileID: payload.FileID, TaskType: payload.TaskType})
		default:
			publishErr = fmt.Errorf("unsupported file outbox event type %q", event.EventType)
		}
		if publishErr != nil {
			next := time.Now().UTC().Add(backoff(event.RetryCount))
			if _, markErr := s.outboxStore.MarkOutbox(ctx, event.ID, event.EventType, lease.Token, "FAILED", publishErr.Error(), event.RetryCount+1, &next); markErr != nil {
				return markErr
			}
			continue
		}
		if _, err := s.outboxStore.MarkOutbox(ctx, event.ID, event.EventType, lease.Token, "SENT", "", event.RetryCount, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) markPermanentOutboxFailure(ctx context.Context, event domain.OutboxEvent, leaseToken, detail string) error {
	applied, err := s.outboxStore.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "DEAD", detail, event.RetryCount+1, nil)
	if err != nil {
		return err
	}
	if !applied {
		return nil
	}
	return nil
}

func (s *Service) BeginConsume(ctx context.Context, messageID, consumer, worker, detail string) (*domain.ConsumeLease, bool, error) {
	if s.outboxStore == nil {
		return nil, false, fmt.Errorf("outbox consume guard is not configured")
	}
	return s.outboxStore.BeginConsume(ctx, messageID, consumer, worker, detail)
}

func (s *Service) MarkConsumed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	if s.outboxStore == nil {
		return false, fmt.Errorf("outbox consume guard is not configured")
	}
	return s.outboxStore.MarkConsumed(ctx, messageID, consumer, leaseToken, detail)
}

func (s *Service) MarkConsumeFailed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	if s.outboxStore == nil {
		return false, fmt.Errorf("outbox consume guard is not configured")
	}
	return s.outboxStore.MarkConsumeFailed(ctx, messageID, consumer, leaseToken, detail)
}

func (s *Service) HandleUploadTaskMessage(ctx context.Context, message domain.UploadTaskMessage) error {
	task, err := s.repo.GetUploadTask(ctx, message.TaskID)
	if err != nil || task == nil {
		return err
	}
	if task.Status == domain.UploadTaskClean {
		return nil
	}
	if strings.TrimSpace(task.ScopeID) == "" {
		task.Status = domain.UploadTaskRejected
		task.FailureCategory = domain.FailureSystem
		task.FailureReason = "missing authenticated organization scope"
		return s.repo.UpdateUploadTask(ctx, task)
	}
	if task.Status == domain.UploadTaskUploaded && task.ExpireAt != nil && !task.ExpireAt.After(time.Now().UTC()) {
		_, err := s.repo.UpdateUploadTaskStatusIfMatch(ctx, task.ID, domain.UploadTaskUploaded, domain.UploadTaskExpired)
		return err
	}
	if task.Status != domain.UploadTaskUploaded && task.Status != domain.UploadTaskProcessing {
		return nil
	}
	if task.Status == domain.UploadTaskUploaded {
		claimed, err := s.repo.UpdateUploadTaskStatusIfMatch(ctx, task.ID, domain.UploadTaskUploaded, domain.UploadTaskProcessing)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		task.Status = domain.UploadTaskProcessing
	}
	strategy, err := s.storageStrategyByID(ctx, task.StorageStrategyID)
	if err != nil {
		return err
	}
	uploadStrategy := strategy
	object, err := s.storage.Open(ctx, strategy, domain.FileInfo{StoragePath: task.ObjectKeyStaging, ContentType: task.ContentType, FileInnerName: task.FileName})
	if err != nil {
		task.Status = domain.UploadTaskFailed
		task.FailureCategory = domain.FailureSystem
		task.FailureReason = "staging object missing"
		return s.repo.UpdateUploadTask(ctx, task)
	}
	defer object.File.Close()
	if task.ExpectedSize > 0 && task.ExpectedSize != object.Size {
		task.Status = domain.UploadTaskRejected
		task.FailureCategory = domain.FailureSystem
		task.FailureReason = "size mismatch"
		return s.repo.UpdateUploadTask(ctx, task)
	}
	fileSHA := strings.TrimSpace(task.ExpectedSha256)
	if fileSHA == "" {
		fileSHA = strings.TrimSpace(task.ETag)
	}
	file := &domain.FileInfo{
		ID:                s.nextID(),
		FileInnerName:     task.FileName,
		FileSize:          object.Size,
		FileSha256:        fileSHA,
		HashAlgorithm:     "SHA-256",
		ContentType:       contentTypeByName(task.FileName, task.ContentType),
		StorageStrategyID: task.StorageStrategyID,
		StoragePath:       task.ObjectKeyStaging,
		Status:            domain.FileStatusAvailable,
		ScanStatus:        domain.ScanStatusClean,
		IntegrityStatus:   domain.IntegrityVerified,
	}
	if file.FileSha256 == "" {
		file.FileSha256 = task.ETag
	}
	existing, err := s.repo.FindFileBySha256AndSize(ctx, file.FileSha256, object.Size)
	if err != nil {
		return err
	}
	reusedExisting := existing != nil && existing.Status == domain.FileStatusAvailable
	if reusedExisting {
		file = existing
		if resolved, err := s.storageStrategyForFile(ctx, *file); err == nil {
			strategy = resolved
		}
	}
	err = s.withTx(ctx, func(txCtx context.Context) error {
		if !reusedExisting {
			fileID, err := s.repo.InsertFile(txCtx, file)
			if err != nil {
				task.Status = domain.UploadTaskFailed
				task.FailureCategory = domain.FailureSystem
				task.FailureReason = err.Error()
				_ = s.repo.UpdateUploadTask(txCtx, task)
				return err
			}
			file.ID = fileID
		} else {
			lockedFile, err := s.lockReusableFileForCredential(txCtx, file.ID)
			if err != nil {
				return err
			}
			file = lockedFile
		}
		task.FileID = file.ID
		task.ActualSize = object.Size
		task.Status = domain.UploadTaskClean
		task.FailureCategory = domain.FailureNone
		task.FailureReason = ""
		if err := s.repo.UpdateUploadTask(txCtx, task); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		task.Status = domain.UploadTaskFailed
		task.FailureCategory = domain.FailureSystem
		task.FailureReason = truncateString(err.Error(), 512)
		_ = s.repo.UpdateUploadTask(ctx, task)
		return err
	}
	if err == nil && reusedExisting && task.ObjectKeyStaging != "" && task.ObjectKeyStaging != file.StoragePath {
		_ = s.storage.Delete(ctx, uploadStrategy, task.ObjectKeyStaging)
	}
	return nil
}

func (s *Service) verifyStagedUploadObject(ctx context.Context, strategy domain.StorageStrategy, task *domain.UploadTask, declaredSize int64, declaredSHA string, etag string) (int64, string, error) {
	object := domain.FileInfo{FileInnerName: task.FileName, ContentType: task.ContentType, StoragePath: task.ObjectKeyStaging}
	download, err := s.storage.Open(ctx, strategy, object)
	if err != nil {
		task.Status = domain.UploadTaskFailed
		task.FailureCategory = domain.FailureStorage
		task.FailureReason = "staging object inaccessible"
		_ = s.repo.UpdateUploadTask(ctx, task)
		return 0, "", apperrors.System("获取对象信息失败")
	}
	hash := sha256.New()
	actualSize, copyErr := io.Copy(hash, download.File)
	closeErr := download.File.Close()
	if copyErr != nil {
		task.Status = domain.UploadTaskFailed
		task.FailureCategory = domain.FailureStorage
		task.FailureReason = "staging object read failed"
		_ = s.repo.UpdateUploadTask(ctx, task)
		return 0, "", apperrors.System("文件上传失败")
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	actualSHA := hex.EncodeToString(hash.Sum(nil))
	if declaredSize > 0 && declaredSize != actualSize {
		task.Status = domain.UploadTaskRejected
		task.FailureCategory = domain.FailureIntegrity
		task.FailureReason = "declared size mismatch"
		_ = s.repo.UpdateUploadTask(ctx, task)
		return 0, "", apperrors.Params("文件大小校验失败")
	}
	if task.ExpectedSize > 0 && task.ExpectedSize != actualSize {
		task.Status = domain.UploadTaskRejected
		task.FailureCategory = domain.FailureIntegrity
		task.FailureReason = "expected size mismatch"
		_ = s.repo.UpdateUploadTask(ctx, task)
		return 0, "", apperrors.Params("文件大小校验失败")
	}
	expectedSHA := strings.ToLower(strings.TrimSpace(firstNonBlank(declaredSHA, task.ExpectedSha256, etag)))
	if expectedSHA != "" && expectedSHA != actualSHA {
		task.Status = domain.UploadTaskRejected
		task.FailureCategory = domain.FailureIntegrity
		task.FailureReason = "sha256 mismatch"
		_ = s.repo.UpdateUploadTask(ctx, task)
		return 0, "", apperrors.Params("文件 SHA256 校验失败")
	}
	return actualSize, actualSHA, nil
}

func (s *Service) HandleFileProcessMessage(ctx context.Context, message domain.FileProcessMessage) error {
	taskID := message.TaskID
	if taskID <= 0 {
		return nil
	}
	task, err := s.repo.GetProcessTask(ctx, taskID)
	if err != nil || task == nil {
		return err
	}
	if task.Status == domain.ProcessTaskCompleted {
		return nil
	}
	if task.Status == domain.ProcessTaskProcessing {
		return nil
	}
	if dedup := strings.TrimSpace(task.DedupKey); dedup != "" {
		completed, err := s.repo.FindCompletedProcessTask(ctx, dedup, task.TaskType)
		if err != nil {
			return err
		}
		if completed != nil && completed.ID != task.ID {
			result := defaultString(completed.ResultData, "{}")
			_ = s.repo.InsertProcessRun(ctx, &domain.FileProcessRun{ID: s.nextID(), TaskID: task.ID, FileID: task.FileID, TaskType: task.TaskType, Status: domain.ProcessTaskCompleted, Attempt: task.RetryCount + 1, ResultData: result, StartedAt: time.Now(), FinishedAt: timePtrNow()})
			return s.repo.UpdateProcessTaskStatus(ctx, task.ID, domain.ProcessTaskCompleted, "", result)
		}
	}
	claimed, err := s.repo.ClaimProcessTask(ctx, task.ID)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	started := time.Now()
	result, processErr := s.runFileProcessor(ctx, task)
	run := &domain.FileProcessRun{ID: s.nextID(), TaskID: task.ID, FileID: task.FileID, TaskType: task.TaskType, Attempt: task.RetryCount + 1, StartedAt: started}
	if processErr != nil {
		run.Status = domain.ProcessTaskFailed
		run.ErrorMsg = processErr.Error()
		finished := time.Now()
		run.FinishedAt = &finished
		_ = s.repo.InsertProcessRun(ctx, run)
		return s.repo.UpdateProcessTaskStatus(ctx, task.ID, domain.ProcessTaskFailed, processErr.Error(), "")
	}
	run.Status = domain.ProcessTaskCompleted
	run.ResultData = result
	finished := time.Now()
	run.FinishedAt = &finished
	if err := s.repo.InsertProcessRun(ctx, run); err != nil {
		return err
	}
	return s.repo.UpdateProcessTaskStatus(ctx, task.ID, domain.ProcessTaskCompleted, "", result)
}

func (s *Service) requireWritableLocalStrategy(ctx context.Context) (*domain.StorageStrategy, error) {
	strategy, err := s.repo.GetDefaultStrategy(ctx)
	if err != nil {
		return nil, err
	}
	if strategy == nil || !strategy.Writable() {
		return nil, apperrors.System("文件上传失败")
	}
	return strategy, nil
}

func (s *Service) storageStrategyForFile(ctx context.Context, file domain.FileInfo) (domain.StorageStrategy, error) {
	return s.storageStrategyByID(ctx, file.StorageStrategyID)
}

func (s *Service) storageStrategyByID(ctx context.Context, id int64) (domain.StorageStrategy, error) {
	var strategy *domain.StorageStrategy
	var err error
	if id > 0 {
		strategy, err = s.repo.GetStrategy(ctx, id)
	} else {
		strategy, err = s.repo.GetDefaultStrategy(ctx)
	}
	if err != nil {
		return domain.StorageStrategy{}, err
	}
	if strategy == nil || !strategy.Readable() {
		return domain.StorageStrategy{}, apperrors.System("文件存储策略不可用")
	}
	return *strategy, nil
}

func (s *Service) checkDirectDownloadAccess(ctx context.Context, actor Actor, fileID int64) error {
	ref, err := s.repo.FindPublicReferenceByFile(ctx, fileID)
	if err != nil {
		return err
	}
	if ref != nil && ref.AccessScope == string(filefacade.AccessPublic) && ref.VisitStrategy == string(filefacade.VisitPublicStatic) {
		return nil
	}
	refs, err := s.repo.ListReferencesByFile(ctx, fileID)
	if err != nil {
		return err
	}
	for _, item := range refs {
		if item.BizType == filefacade.ConfigAssetBizType {
			// CONFIG_ASSET authorization is tied to the owning configuration's
			// exposure and is intentionally never a generic fileId capability.
			continue
		}
		switch item.AccessScope {
		case string(filefacade.AccessPublic):
			return nil
		case string(filefacade.AccessLoginUsers):
			if actor.Authenticated && strings.TrimSpace(actor.ScopeID) != "" && actor.ScopeID == item.ScopeID {
				return nil
			}
		case string(filefacade.AccessOwnerOnly):
			if actor.UserID > 0 && actor.UserID == item.UserID && strings.TrimSpace(actor.ScopeID) != "" && actor.ScopeID == item.ScopeID {
				return nil
			}
		}
	}
	return apperrors.Forbidden("无文件访问权限")
}

func (s *Service) checkHotlink(referer string) error {
	if !s.distribution.HotlinkProtectionEnabled {
		return nil
	}
	referer = strings.TrimSpace(referer)
	if referer == "" {
		if s.distribution.AllowEmptyReferer {
			return nil
		}
		return apperrors.Forbidden("文件访问来源不被允许")
	}
	parsed, err := url.Parse(referer)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return apperrors.Forbidden("文件访问来源不被允许")
	}
	host := strings.ToLower(parsed.Hostname())
	for _, item := range strings.FieldsFunc(s.distribution.HotlinkAllowedDomains, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == ' '
	}) {
		allowed := strings.ToLower(strings.TrimSpace(item))
		if allowed == "" {
			continue
		}
		if host == allowed || (strings.HasPrefix(allowed, "*.") && strings.HasSuffix(host, strings.TrimPrefix(allowed, "*"))) {
			return nil
		}
	}
	return apperrors.Forbidden("文件访问来源不被允许")
}

func (s *Service) appendOutbox(ctx context.Context, eventType, aggregateType, aggregateID string, payload any) error {
	if !s.outbox {
		return nil
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	event := &domain.OutboxEvent{
		ID:            s.nextID(),
		EventID:       uuid.NewString(),
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       string(payloadBytes),
		Status:        "PENDING",
		NextRetryAt:   time.Now().UTC(),
	}
	if s.outboxStore == nil {
		return fmt.Errorf("outbox store is not configured")
	}
	return s.outboxStore.AppendOutbox(ctx, event)
}

func (s *Service) appendFileProcessOutboxBatch(ctx context.Context, tasks []domain.FileProcessTask) error {
	if !s.outbox || s.outboxStore == nil {
		return fmt.Errorf("file process outbox is not configured")
	}
	if len(tasks) > maintenanceChunkSize {
		return fmt.Errorf("file process outbox batch exceeds %d", maintenanceChunkSize)
	}
	events := make([]domain.OutboxEvent, 0, len(tasks))
	now := time.Now().UTC()
	for _, task := range tasks {
		payload, err := json.Marshal(map[string]any{
			"taskId": task.ID, "fileId": task.FileID, "taskType": task.TaskType,
		})
		if err != nil {
			return err
		}
		events = append(events, domain.OutboxEvent{
			ID: s.nextID(), EventID: uuid.NewString(), EventType: "FILE_PROCESS_TASK",
			AggregateType: "FILE_PROCESS_TASK", AggregateID: strconv.FormatInt(task.ID, 10),
			Payload: string(payload), Status: "PENDING", NextRetryAt: now,
		})
	}
	return s.outboxStore.AppendOutboxBatch(ctx, events)
}

func (s *Service) withTx(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor == nil || !s.transactor.Enabled() {
		return fn(ctx)
	}
	return s.transactor.WithinTransaction(ctx, fn)
}

func (s *Service) nextID() int64 {
	if s.idGen != nil {
		return s.idGen.NextID()
	}
	return time.Now().UnixNano()
}

func validateUploadRequest(request UploadRequest) (domain.FileBiz, error) {
	if request.Reader == nil {
		return domain.FileBiz{}, apperrors.Params("文件内容不能为空")
	}
	return validateUploadMetadata(request)
}

func validateUploadMetadata(request UploadRequest) (domain.FileBiz, error) {
	biz := domain.DefaultFileBiz
	request.FileName = strings.TrimSpace(request.FileName)
	if request.FileName == "" {
		return domain.FileBiz{}, apperrors.Params("文件名不能为空")
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(request.FileName)), ".")
	if ext != "" && !biz.Suffixes[ext] {
		return domain.FileBiz{}, apperrors.Params("不支持的文件类型")
	}
	return biz, nil
}

func (s *Service) completedUploadCredential(actor Actor, file *domain.FileInfo, fileName, contentType string) *domain.UploadTask {
	expireAt := time.Now().UTC().Add(24 * time.Hour)
	return &domain.UploadTask{
		ID:                 uuid.NewString(),
		UserID:             actor.UserID,
		ScopeID:            actor.ScopeID,
		CredentialID:       uuid.NewString(),
		CredentialVersion:  domain.UploadCredentialVersion1,
		FileName:           strings.TrimSpace(fileName),
		ContentType:        contentTypeByName(fileName, contentType),
		StorageStrategyID:  file.StorageStrategyID,
		ObjectKeyStaging:   file.StoragePath,
		ObjectKeyClean:     file.StoragePath,
		Status:             domain.UploadTaskClean,
		UploadMode:         "completed",
		ExpectedSize:       file.FileSize,
		ExpectedSha256:     file.FileSha256,
		ActualSize:         file.FileSize,
		ETag:               file.FileSha256,
		FileID:             file.ID,
		BindingToken:       randomToken(),
		BindingChannel:     uploadBindingChannel("upload-only", actor.ScopeSource),
		ExpireAt:           &expireAt,
		ProtectedUntil:     &expireAt,
		CredentialExpireAt: &expireAt,
		UserIP:             actor.ClientIP,
	}
}

func (s *Service) lockReusableFileForCredential(ctx context.Context, fileID int64) (*domain.FileInfo, error) {
	file, err := s.repo.GetFileForUpdate(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if err := validateBindableFile(file, filefacade.DefaultFile); err != nil {
		return nil, err
	}
	return file, nil
}

type bindingDefaults struct {
	DisplayName   string
	VisitStrategy filefacade.FileVisitStrategy
	AccessScope   filefacade.FileAccessScope
}

func defaultBindingResult(biz domain.FileBiz, fileName string, bizType filefacade.FileActionBiz) bindingDefaults {
	visit := filefacade.VisitPrivatePreview
	scope := filefacade.AccessOwnerOnly
	if bizType == filefacade.UserAvatar {
		visit = filefacade.VisitPublicStatic
		scope = filefacade.AccessPublic
	}
	displayName := strings.TrimSpace(fileName)
	if displayName == "" {
		displayName = biz.Name
	}
	return bindingDefaults{
		DisplayName:   displayName,
		VisitStrategy: visit,
		AccessScope:   scope,
	}
}

func (s *Service) findReusableUploadedFile(ctx context.Context, sha256Value string, size int64, biz filefacade.FileActionBiz) (*domain.FileInfo, error) {
	file, err := s.repo.FindFileBySha256AndSize(ctx, sha256Value, size)
	if err != nil || file == nil {
		return file, err
	}
	if err := validateBindableFile(file, biz); err != nil {
		return nil, err
	}
	return file, nil
}

func validateBindableFile(file *domain.FileInfo, biz filefacade.FileActionBiz) error {
	if file == nil || file.IsDeleted != 0 || file.Status == domain.FileStatusDeleted {
		return apperrors.NotFound("文件不存在")
	}
	if file.Status != domain.FileStatusAvailable {
		return apperrors.Forbidden("文件当前状态不可绑定")
	}
	if file.ScanStatus != domain.ScanStatusClean {
		return apperrors.Forbidden("文件安全扫描未通过，不能绑定")
	}
	if file.IntegrityStatus != domain.IntegrityVerified {
		return apperrors.Forbidden("文件完整性未通过，不能绑定")
	}
	if biz == filefacade.UserAvatar && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(file.ContentType)), "image/") {
		return apperrors.Forbidden("头像文件必须是图片")
	}
	return nil
}

func normalizeImageMIME(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	switch strings.ToLower(mediaType) {
	case "image/png":
		return "image/png"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	default:
		return ""
	}
}

func imageFormatMIME(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func imageExtensionMatches(fileName, format string) bool {
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.TrimSpace(fileName))), ".")
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return extension == "png"
	case "jpeg":
		return extension == "jpg" || extension == "jpeg"
	case "webp":
		return extension == "webp"
	default:
		return false
	}
}

func imageHasExactContainerLength(payload []byte, format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return len(payload) >= 12 && bytes.Equal(payload[len(payload)-12:], []byte{0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82})
	case "jpeg":
		return len(payload) >= 2 && payload[len(payload)-2] == 0xff && payload[len(payload)-1] == 0xd9
	case "webp":
		return len(payload) >= 12 && bytes.Equal(payload[:4], []byte("RIFF")) &&
			bytes.Equal(payload[8:12], []byte("WEBP")) && int(binary.LittleEndian.Uint32(payload[4:8]))+8 == len(payload)
	default:
		return false
	}
}

func buildStoragePath(biz domain.FileBiz, userID int64, fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	return filepath.Join(biz.RoutePath, strconv.FormatInt(userID, 10), uuid.NewString()+ext)
}

func chunkPartPath(uploadID string, partNumber int) string {
	return filepath.Join("tmp", "chunk-upload", strings.TrimSpace(uploadID), fmt.Sprintf("%06d.part", partNumber))
}

func appendUniqueInt(values []int, value int) []int {
	for _, item := range values {
		if item == value {
			return normalizeUploadedChunks(values)
		}
	}
	values = append(values, value)
	return normalizeUploadedChunks(values)
}

func normalizeUploadedChunks(values []int) []int {
	result := make([]int, 0, len(values))
	seen := map[int]struct{}{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func parseBizType(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return domain.DefaultFileBiz.Code
	}
	return parsed
}

func chunkStatusResponse(upload domain.ChunkUpload) *ChunkUploadStatusResponse {
	return &ChunkUploadStatusResponse{
		UploadID:       upload.UploadID,
		Status:         upload.Status,
		StatusName:     chunkStatusName(upload.Status),
		FileName:       upload.FileName,
		FileSize:       upload.FileSize,
		ChunkSize:      upload.ChunkSize,
		TotalChunks:    upload.TotalChunks,
		UploadedChunks: normalizeUploadedChunks(upload.UploadedChunks),
		ExpireAt:       upload.ExpireTime.Format(time.RFC3339),
	}
}

func chunkStatusName(status int) string {
	switch status {
	case domain.ChunkStatusInit:
		return "INIT"
	case domain.ChunkStatusUploading:
		return "UPLOADING"
	case domain.ChunkStatusCompleted:
		return "COMPLETED"
	case domain.ChunkStatusAborted:
		return "ABORTED"
	case domain.ChunkStatusExpired:
		return "EXPIRED"
	default:
		return "UNKNOWN"
	}
}

func timePtrNow() *time.Time {
	now := time.Now()
	return &now
}

type chunkSequenceReader struct {
	ctx         context.Context
	storage     ObjectStorePort
	strategy    domain.StorageStrategy
	uploadID    string
	totalParts  int
	contentType string
	fileName    string
	current     io.ReadCloser
	nextPart    int
}

func (r *chunkSequenceReader) Read(p []byte) (int, error) {
	for {
		if r.current == nil {
			if r.nextPart >= r.totalParts {
				return 0, io.EOF
			}
			r.nextPart++
			object, err := r.storage.Open(r.ctx, r.strategy, domain.FileInfo{StoragePath: chunkPartPath(r.uploadID, r.nextPart), ContentType: r.contentType, FileInnerName: r.fileName})
			if err != nil {
				return 0, err
			}
			r.current = object.File
		}
		n, err := r.current.Read(p)
		if err == io.EOF {
			_ = r.current.Close()
			r.current = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (r *chunkSequenceReader) Close() error {
	if r.current != nil {
		err := r.current.Close()
		r.current = nil
		return err
	}
	return nil
}

func (s *Service) runFileProcessor(ctx context.Context, task *domain.FileProcessTask) (string, error) {
	file, err := s.repo.GetFile(ctx, task.FileID)
	if err != nil {
		return "", err
	}
	if file == nil || file.IsDeleted == 1 {
		return "", apperrors.NotFound("文件不存在")
	}
	taskType := strings.ToUpper(strings.TrimSpace(task.TaskType))
	if taskType == "" {
		return "", apperrors.Params("处理任务类型不能为空")
	}
	switch taskType {
	case "THUMBNAIL", "COMPRESS":
		result := map[string]any{
			"fileId":      file.ID,
			"taskType":    taskType,
			"status":      "SKIPPED_NOOP_PROVIDER",
			"processedAt": time.Now().UTC().Format(time.RFC3339),
		}
		bytes, _ := json.Marshal(result)
		return string(bytes), nil
	default:
		return "", apperrors.Params("不支持的文件处理任务类型")
	}
}

func buildVisitURL(storage ObjectStorePort, storageStrategy domain.StorageStrategy, storagePath string, strategy filefacade.FileVisitStrategy) string {
	if strategy == filefacade.VisitPublicStatic && storage != nil {
		return storage.PublicURL(storageStrategy, storagePath)
	}
	return ""
}

func accessLevelFromScope(scope string) int {
	switch scope {
	case string(filefacade.AccessPublic):
		return 2
	case string(filefacade.AccessLoginUsers):
		return 1
	default:
		return 0
	}
}

func (s *Service) downloadGatewayPath() string {
	return defaultString(s.distribution.GatewayPath, "/file/download")
}

func withQueryParam(pathValue, key, value string) string {
	parsed, err := url.Parse(defaultString(pathValue, "/file/download"))
	if err != nil || parsed.Path == "" {
		parsed = &url.URL{Path: "/file/download"}
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func randomToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return uuid.NewString()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func contentTypeByName(fileName, provided string) string {
	if strings.TrimSpace(provided) != "" {
		return strings.TrimSpace(provided)
	}
	if detected := mime.TypeByExtension(filepath.Ext(fileName)); detected != "" {
		return detected
	}
	return "application/octet-stream"
}

func backoff(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount > 6 {
		retryCount = 6
	}
	return time.Duration(1<<retryCount) * time.Minute
}

func toFileInfoDTO(file domain.FileInfo) filefacade.FileInfoDTO {
	return filefacade.FileInfoDTO{
		ID:                file.ID,
		FileInnerName:     file.FileInnerName,
		FileSize:          file.FileSize,
		FileSha256:        file.FileSha256,
		ContentType:       file.ContentType,
		StorageStrategyID: file.StorageStrategyID,
		StoragePath:       file.StoragePath,
		Status:            file.Status,
		ScanStatus:        file.ScanStatus,
	}
}

func toFileInfoVO(file domain.FileInfo) filefacade.FileInfoVO {
	createTime := file.CreateTime
	updateTime := file.UpdateTime
	return filefacade.FileInfoVO{
		ID:                file.ID,
		FileInnerName:     file.FileInnerName,
		FileSize:          file.FileSize,
		FileSha256:        file.FileSha256,
		ContentType:       file.ContentType,
		StorageType:       file.StorageType,
		StorageStrategyID: file.StorageStrategyID,
		StoragePath:       file.StoragePath,
		Status:            file.Status,
		ScanStatus:        file.ScanStatus,
		CreateTime:        &createTime,
		UpdateTime:        &updateTime,
		IsDeleted:         file.IsDeleted,
	}
}

func toReferenceDTO(ref domain.FileReference) filefacade.FileReferenceDTO {
	return filefacade.FileReferenceDTO{
		ID:            ref.ID,
		FileID:        ref.FileID,
		UserID:        ref.UserID,
		DisplayName:   ref.DisplayName,
		VisitURL:      ref.VisitURL,
		BizType:       ref.BizType,
		BizID:         ref.BizID,
		VisitStrategy: ref.VisitStrategy,
		AccessScope:   ref.AccessScope,
	}
}

func toReferenceVO(ref domain.FileReference) filefacade.FileReferenceVO {
	createTime := ref.CreateTime
	return filefacade.FileReferenceVO{
		ID:            ref.ID,
		FileID:        ref.FileID,
		UserID:        ref.UserID,
		DisplayName:   ref.DisplayName,
		BizType:       ref.BizType,
		BizID:         ref.BizID,
		VisitURL:      ref.VisitURL,
		AccessLevel:   ref.AccessLevel,
		VisitStrategy: ref.VisitStrategy,
		AccessScope:   ref.AccessScope,
		CreateTime:    &createTime,
	}
}

func toStrategyVO(strategy domain.StorageStrategy) filefacade.StorageStrategyVO {
	return filefacade.StorageStrategyVO{
		ID:                   strategy.ID,
		StrategyName:         strategy.StrategyName,
		ProviderType:         strategy.ProviderType,
		IsDefault:            strategy.IsDefault,
		IsEnabled:            strategy.IsEnabled,
		RunState:             strategy.RunState,
		Priority:             strategy.Priority,
		ConfigJSON:           strategy.ConfigCiphertext,
		HealthCheckURL:       strategy.HealthCheckURL,
		HealthStatus:         strategy.HealthStatus,
		LastHealthCheck:      strategy.LastHealthCheck,
		FailureCount:         strategy.FailureCount,
		TotalRequests:        strategy.TotalRequests,
		FailureRateThreshold: strategy.FailureRateThreshold,
	}
}

func toProcessTaskVO(task domain.FileProcessTask) filefacade.FileProcessTaskVO {
	createTime := task.CreateTime
	updateTime := task.UpdateTime
	return filefacade.FileProcessTaskVO{
		ID:            task.ID,
		FileID:        task.FileID,
		TaskType:      task.TaskType,
		TaskParams:    task.TaskParams,
		Status:        task.Status,
		RetryCount:    task.RetryCount,
		MaxRetry:      task.MaxRetry,
		ErrorMsg:      task.ErrorMsg,
		ResultData:    task.ResultData,
		Priority:      task.Priority,
		MQMessageID:   task.MQMessageID,
		NextRetryTime: task.NextRetryTime,
		CreateTime:    &createTime,
		UpdateTime:    &updateTime,
		StartTime:     task.StartTime,
		FinishTime:    task.FinishTime,
	}
}

func storageStrategyFromCommand(command StorageStrategyCommand) (*domain.StorageStrategy, error) {
	name := strings.TrimSpace(command.StrategyName)
	if name == "" {
		return nil, apperrors.Params("存储策略名称不能为空")
	}
	provider := strings.TrimSpace(strings.ToUpper(command.ProviderType))
	if provider == "" {
		provider = domain.ProviderLocal
	}
	switch provider {
	case domain.ProviderLocal, domain.ProviderAWSS3, domain.ProviderAliyunOSS, domain.ProviderTencentCOS:
	default:
		return nil, apperrors.Params("不支持的存储提供商")
	}
	runState := strings.TrimSpace(strings.ToUpper(command.RunState))
	if runState == "" {
		runState = domain.RunStateActive
	}
	return &domain.StorageStrategy{
		StrategyName:         name,
		ProviderType:         provider,
		IsDefault:            command.IsDefault,
		IsEnabled:            command.IsEnabled,
		RunState:             runState,
		Priority:             command.Priority,
		ConfigCiphertext:     strings.TrimSpace(command.ConfigJSON),
		HealthCheckURL:       strings.TrimSpace(command.HealthCheckURL),
		HealthStatus:         domain.HealthHealthy,
		FailureRateThreshold: command.FailureRateThreshold,
	}, nil
}

func truncateString(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
