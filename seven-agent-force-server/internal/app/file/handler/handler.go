package handler

import (
	"context"
	"fmt"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"

	fileapp "github.com/CaixyPromise/seven-agent/force/internal/app/file/application"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type Handler struct {
	service *fileapp.Service
}

func NewHandler(service *fileapp.Service) *Handler {
	return &Handler{service: service}
}

func (c *Handler) Check(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.service.CheckFile(ctx, actor(reqCtx), strings.TrimSpace(string(reqCtx.Query("sha256"))), queryInt64(reqCtx, "fileSize", queryInt64(reqCtx, "size", 0)))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) Upload(ctx context.Context, reqCtx *app.RequestContext) {
	header, err := formFile(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	file, err := header.Open()
	if err != nil {
		response.Error(reqCtx, apperrors.Params("文件读取失败"))
		return
	}
	defer file.Close()
	request := fileapp.UploadRequest{
		FileName:     header.Filename,
		ContentType:  contentType(header),
		Reader:       file,
		ExpectedSize: header.Size,
	}
	result, err := c.service.Upload(ctx, actor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) FasterUpload(ctx context.Context, reqCtx *app.RequestContext) {
	var request struct {
		FileName    string `json:"fileName" form:"fileName"`
		ContentType string `json:"contentType" form:"contentType"`
		SHA256      string `json:"sha256" form:"sha256"`
		FileSize    int64  `json:"fileSize" form:"fileSize"`
	}
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.FasterUpload(ctx, actor(reqCtx), request.SHA256, request.FileSize, fileapp.UploadRequest{
		FileName:    request.FileName,
		ContentType: request.ContentType,
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) Download(ctx context.Context, reqCtx *app.RequestContext) {
	referenceID := queryInt64(reqCtx, "referenceId", 0)
	var (
		result *fileapp.DownloadResult
		err    error
	)
	if referenceID > 0 {
		result, err = c.service.OpenReferenceDownload(ctx, actor(reqCtx), referenceID)
	} else {
		result, err = c.service.OpenDownload(ctx, actor(reqCtx), queryInt64(reqCtx, "fileId", 0), strings.TrimSpace(string(reqCtx.Query("token"))))
	}
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	reqCtx.Response.Header.Set("Content-Type", result.Object.ContentType)
	reqCtx.Response.Header.Set("Cache-Control", result.CacheControl)
	reqCtx.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", result.Object.Name))
	reqCtx.Response.SetBodyStream(result.Object.File, int(result.Object.Size))
}

func (c *Handler) InitUpload(ctx context.Context, reqCtx *app.RequestContext) {
	var request fileapp.UploadTaskInitRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.InitUploadTask(ctx, actor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) InstantConfirm(ctx context.Context, reqCtx *app.RequestContext) {
	var request struct {
		TaskID string `json:"taskId" form:"taskId"`
	}
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.ConfirmInstantUpload(ctx, actor(reqCtx), strings.TrimSpace(request.TaskID))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) CompleteUpload(ctx context.Context, reqCtx *app.RequestContext) {
	if header, err := formFile(reqCtx); err == nil && header != nil {
		file, err := header.Open()
		if err != nil {
			response.Error(reqCtx, apperrors.Params("文件读取失败"))
			return
		}
		defer file.Close()
		taskID := firstNonBlank(string(reqCtx.FormValue("taskId")), string(reqCtx.Query("taskId")))
		result, err := c.service.CompleteUploadTaskWithReader(ctx, actor(reqCtx), strings.TrimSpace(taskID), file, header.Size, contentType(header))
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
		response.Success(reqCtx, result)
		return
	}
	var request struct {
		TaskID string `json:"taskId" form:"taskId"`
	}
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.CompleteUploadTask(ctx, actor(reqCtx), strings.TrimSpace(request.TaskID))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) Callback(ctx context.Context, reqCtx *app.RequestContext) {
	var request fileapp.UploadCallbackRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if strings.TrimSpace(request.BindingToken) == "" {
		request.BindingToken = strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Upload-Callback-Token")))
	}
	result, err := c.service.CompleteUploadCallback(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) UploadStatus(ctx context.Context, reqCtx *app.RequestContext) {
	taskID := strings.TrimSpace(string(reqCtx.Param("taskId")))
	result, err := c.service.GetUploadTaskStatus(ctx, actor(reqCtx), taskID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) DownloadURL(ctx context.Context, reqCtx *app.RequestContext) {
	fileID, err := parsePathInt64(reqCtx, "fileId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	url, err := c.service.BuildDownloadURL(ctx, actor(reqCtx), fileID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, downloadURLPayload(url))
}

func downloadURLPayload(url string) map[string]any {
	return map[string]any{
		"url":         url,
		"downloadUrl": url,
	}
}

func (c *Handler) ChunkInit(ctx context.Context, reqCtx *app.RequestContext) {
	var request fileapp.ChunkUploadInitRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.InitChunkUpload(ctx, actor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) ChunkPart(ctx context.Context, reqCtx *app.RequestContext) {
	header, err := chunkFormFile(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	file, err := header.Open()
	if err != nil {
		response.Error(reqCtx, apperrors.Params("文件读取失败"))
		return
	}
	defer file.Close()
	partNumber := int(formOrQueryInt64(reqCtx, "partNumber", formOrQueryInt64(reqCtx, "chunkIndex", formOrQueryInt64(reqCtx, "chunkNumber", 0))))
	result, err := c.service.UploadChunkPart(ctx, actor(reqCtx), fileapp.ChunkPartRequest{
		UploadID:     firstNonBlank(string(reqCtx.FormValue("uploadId")), string(reqCtx.Query("uploadId"))),
		PartNumber:   partNumber,
		PartSHA256:   firstNonBlank(string(reqCtx.FormValue("sha256")), string(reqCtx.FormValue("chunkSha256")), string(reqCtx.Query("sha256"))),
		ContentType:  contentType(header),
		Reader:       file,
		ExpectedSize: header.Size,
		OriginalName: header.Filename,
	})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) ChunkComplete(ctx context.Context, reqCtx *app.RequestContext) {
	var request struct {
		UploadID string `json:"uploadId" form:"uploadId"`
	}
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.CompleteChunkUpload(ctx, actor(reqCtx), strings.TrimSpace(request.UploadID))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) ChunkAbort(ctx context.Context, reqCtx *app.RequestContext) {
	uploadID := firstNonBlank(string(reqCtx.Query("uploadId")), string(reqCtx.FormValue("uploadId")))
	if strings.TrimSpace(uploadID) == "" {
		var request struct {
			UploadID string `json:"uploadId" form:"uploadId"`
		}
		if err := httpx.Bind(reqCtx, &request); err == nil {
			uploadID = request.UploadID
		}
	}
	if err := c.service.AbortChunkUpload(ctx, actor(reqCtx), strings.TrimSpace(uploadID)); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) ChunkStatus(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.service.ChunkUploadStatus(ctx, actor(reqCtx), strings.TrimSpace(string(reqCtx.Query("uploadId"))))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) ChunkActive(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.service.ActiveChunkUploads(ctx, actor(reqCtx))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) ChunkResumeInfo(ctx context.Context, reqCtx *app.RequestContext) {
	c.ChunkStatus(ctx, reqCtx)
}

func (c *Handler) FileManageList(ctx context.Context, reqCtx *app.RequestContext) {
	page, err := c.service.QueryFiles(ctx, queryInt64(reqCtx, "current", 1), pageSize(reqCtx, 10), string(reqCtx.Query("fileName")), string(reqCtx.Query("fileType")), queryOptionalInt(reqCtx, "bizType"), string(reqCtx.Query("startTime")), string(reqCtx.Query("endTime")))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) FileDetail(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.GetFile(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) FileReferences(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	refs, err := c.service.ListReferences(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, refs)
}

func (c *Handler) UpdateReferenceAccess(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request struct {
		AccessScope   string `json:"accessScope" form:"accessScope"`
		VisitStrategy string `json:"visitStrategy" form:"visitStrategy"`
		AccessLevel   *int   `json:"accessLevel" form:"accessLevel"`
	}
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.UpdateReferenceAccess(ctx, id, request.AccessScope, request.VisitStrategy, request.AccessLevel)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) FileStats(ctx context.Context, reqCtx *app.RequestContext) {
	stats, err := c.service.FileStats(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, stats)
}

func (c *Handler) DeleteFile(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if _, err := c.service.RemoveFile(ctx, id, 0, "", 0); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, fileBatchDeleteResponse([]int64{id}, []int64{id}, nil))
}

func (c *Handler) BatchDeleteFiles(ctx context.Context, reqCtx *app.RequestContext) {
	var request struct {
		IDs []int64 `json:"ids"`
	}
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if _, err := c.service.BatchRemoveByIDs(ctx, request.IDs, 0, "", 0); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, fileBatchDeleteResponse(request.IDs, request.IDs, nil))
}

func (c *Handler) StorageStrategies(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.service.StorageStrategies(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) CreateStorageStrategy(ctx context.Context, reqCtx *app.RequestContext) {
	var request fileapp.StorageStrategyCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	id, err := c.service.CreateStorageStrategy(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, id)
}

func (c *Handler) UpdateStorageStrategy(ctx context.Context, reqCtx *app.RequestContext) {
	var request fileapp.StorageStrategyCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.service.UpdateStorageStrategy(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) UpdateStorageStrategyByPath(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request fileapp.StorageStrategyCommand
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.ID = id
	if err := c.service.UpdateStorageStrategy(ctx, request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) DeleteStorageStrategy(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.service.DeleteStorageStrategy(ctx, id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) StorageStrategy(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.StorageStrategy(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) SetDefaultStrategy(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.service.SetDefaultStrategy(ctx, id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) EnableStrategy(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	enabled := true
	value := strings.TrimSpace(string(reqCtx.Query("enabled")))
	if value == "" {
		value = strings.TrimSpace(string(reqCtx.Query("enable")))
	}
	if value == "false" || value == "0" {
		enabled = false
	}
	if err := c.service.EnableStrategy(ctx, id, enabled); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) StorageHealth(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.CheckStorageHealth(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) ProcessTaskPage(ctx context.Context, reqCtx *app.RequestContext) {
	page, err := c.service.QueryProcessTasks(ctx, queryInt64(reqCtx, "current", 1), pageSize(reqCtx, 10), queryOptionalInt(reqCtx, "status"), string(reqCtx.Query("taskType")))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) ProcessTaskDetail(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.service.GetProcessTask(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) RetryProcessTask(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.service.RetryProcessTask(ctx, id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) BatchRetryProcessTasks(ctx context.Context, reqCtx *app.RequestContext) {
	var request struct {
		IDs []int64 `json:"ids"`
	}
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.service.BatchRetryProcessTasks(ctx, request.IDs); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) ProcessTaskStats(ctx context.Context, reqCtx *app.RequestContext) {
	stats, err := c.service.ProcessTaskStats(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, stats)
}

func actor(reqCtx *app.RequestContext) fileapp.Actor {
	user := securitycontext.Get(reqCtx)
	if user == nil {
		return fileapp.Actor{ClientIP: reqCtx.ClientIP(), Referer: string(reqCtx.Request.Header.Peek("Referer"))}
	}
	orgScope, _ := securitycontext.ResolveOrganizationScope(user)
	return fileapp.Actor{
		UserID:        user.UserID,
		ScopeID:       orgScope.ScopeID,
		ScopeSource:   orgScope.Source,
		IsAdmin:       user.IsAdmin,
		Authenticated: !user.IsAnonymous && user.UserID > 0,
		ClientIP:      reqCtx.ClientIP(),
		Referer:       string(reqCtx.Request.Header.Peek("Referer")),
	}
}

func formFile(reqCtx *app.RequestContext) (*multipart.FileHeader, error) {
	for _, name := range []string{"file", "uploadFile"} {
		header, err := reqCtx.FormFile(name)
		if err == nil && header != nil {
			return header, nil
		}
	}
	return nil, apperrors.Params("文件不能为空")
}

func chunkFormFile(reqCtx *app.RequestContext) (*multipart.FileHeader, error) {
	for _, name := range []string{"file", "chunk", "part", "uploadFile"} {
		header, err := reqCtx.FormFile(name)
		if err == nil && header != nil {
			return header, nil
		}
	}
	return nil, apperrors.Params("分块文件不能为空")
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func contentType(header *multipart.FileHeader) string {
	if header == nil {
		return "application/octet-stream"
	}
	if values := header.Header.Values("Content-Type"); len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return strings.TrimSpace(values[0])
	}
	if detected := mime.TypeByExtension(filepath.Ext(header.Filename)); detected != "" {
		return detected
	}
	return "application/octet-stream"
}

func parsePathInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(reqCtx.Param(key)), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, apperrors.Params("参数错误")
	}
	return parsed, nil
}

func queryInt64(reqCtx *app.RequestContext, key string, fallback int64) int64 {
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func formOrQueryInt64(reqCtx *app.RequestContext, key string, fallback int64) int64 {
	value := firstNonBlank(string(reqCtx.FormValue(key)), string(reqCtx.Query(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func pageSize(reqCtx *app.RequestContext, fallback int64) int64 {
	if value := queryInt64(reqCtx, "pageSize", 0); value > 0 {
		return value
	}
	return queryInt64(reqCtx, "size", fallback)
}

type batchDeleteResponse struct {
	Success        bool                     `json:"success"`
	Outcome        string                   `json:"outcome"`
	RequestedCount int                      `json:"requestedCount"`
	DeletedCount   int                      `json:"deletedCount"`
	SkippedCount   int                      `json:"skippedCount"`
	DeletedIDs     []int64                  `json:"deletedIds"`
	SkippedItems   []batchDeleteSkippedItem `json:"skippedItems"`
}

type batchDeleteSkippedItem struct {
	FileID  int64  `json:"fileId"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func fileBatchDeleteResponse(requested []int64, deleted []int64, skipped []batchDeleteSkippedItem) batchDeleteResponse {
	outcome := "FULL_SUCCESS"
	if len(deleted) == 0 && len(requested) > 0 {
		outcome = "FULL_FAILED"
	} else if len(skipped) > 0 {
		outcome = "PARTIAL_SUCCESS"
	}
	if deleted == nil {
		deleted = []int64{}
	}
	if skipped == nil {
		skipped = []batchDeleteSkippedItem{}
	}
	return batchDeleteResponse{
		Success:        len(skipped) == 0,
		Outcome:        outcome,
		RequestedCount: len(requested),
		DeletedCount:   len(deleted),
		SkippedCount:   len(skipped),
		DeletedIDs:     deleted,
		SkippedItems:   skipped,
	}
}

func queryOptionalInt(reqCtx *app.RequestContext, key string) *int {
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}
