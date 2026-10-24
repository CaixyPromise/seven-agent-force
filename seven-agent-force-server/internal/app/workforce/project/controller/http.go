package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Service interface {
	Create(context.Context, int64, int64, projectapp.UpsertRequest) (domain.Project, error)
	Update(context.Context, int64, int64, int64, projectapp.UpsertRequest) (domain.Project, error)
	Get(context.Context, int64, int64) (domain.Project, error)
	List(context.Context, int64, projectapp.ListRequest) (projectapp.ProjectPage, error)
	Archive(context.Context, int64, int64, int64, int64) (domain.Project, error)
	RetryRepository(context.Context, int64, int64, int64) (domain.Project, error)
}

type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.GET("/workforce/projects", wrap("workforce:task:list", h.list))
	router.POST("/workforce/projects", wrap("workforce:task:create", h.create))
	router.GET("/workforce/projects/:projectId", wrap("workforce:task:query", h.get))
	router.PUT("/workforce/projects/:projectId", wrap("workforce:task:metadata:update", h.update))
	router.POST("/workforce/projects/:projectId/archive", wrap("workforce:task:metadata:update", h.archive))
	router.POST("/workforce/projects/:projectId/repository/retry", wrap("workforce:source:create", h.retryRepository))
}
func (h *Handler) retryRepository(ctx context.Context, c *app.RequestContext) {
	tenant, actor, ok := identity(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	item, err := h.service.RetryRepository(ctx, tenant, actor, id)
	write(c, projectView(item), err)
}
func (h *Handler) create(ctx context.Context, c *app.RequestContext) {
	tenant, actor, ok := identity(c)
	if !ok {
		return
	}
	var request projectapp.UpsertRequest
	if !decode(c, &request) {
		return
	}
	request.DeliveryApproveAuthorized = securitycontext.HasPermission(c, "workforce:delivery:approve")
	request.IdempotencyKey = strings.TrimSpace(string(c.GetHeader("Idempotency-Key")))
	item, err := h.service.Create(ctx, tenant, actor, request)
	write(c, projectView(item), err)
}
func (h *Handler) update(ctx context.Context, c *app.RequestContext) {
	tenant, actor, ok := identity(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request projectapp.UpsertRequest
	if !decode(c, &request) {
		return
	}
	request.DeliveryApproveAuthorized = securitycontext.HasPermission(c, "workforce:delivery:approve")
	item, err := h.service.Update(ctx, tenant, actor, id, request)
	write(c, projectView(item), err)
}
func (h *Handler) get(ctx context.Context, c *app.RequestContext) {
	tenant, _, ok := identity(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(ctx, tenant, id)
	write(c, projectView(item), err)
}
func (h *Handler) list(ctx context.Context, c *app.RequestContext) {
	tenant, _, ok := identity(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	cursor, valid := optionalQueryID(c, "cursor")
	if !valid {
		return
	}
	page, err := h.service.List(ctx, tenant, projectapp.ListRequest{
		Query: c.Query("query"), Status: c.Query("status"), Cursor: cursor, Limit: limit,
	})
	if err != nil {
		write(c, nil, err)
		return
	}
	views := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		views = append(views, projectView(item))
	}
	var nextCursor any
	if page.NextCursor > 0 {
		nextCursor = page.NextCursor
	}
	write(c, map[string]any{"items": views, "nextCursor": nextCursor}, nil)
}
func (h *Handler) archive(ctx context.Context, c *app.RequestContext) {
	tenant, actor, ok := identity(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request struct {
		ExpectedVersionID int64 `json:"expectedVersionId,string"`
	}
	if !decode(c, &request) {
		return
	}
	item, err := h.service.Archive(ctx, tenant, actor, id, request.ExpectedVersionID)
	write(c, projectView(item), err)
}

func projectView(item domain.Project) map[string]any {
	if item.HookReleaseIDs == nil {
		item.HookReleaseIDs = sharedtransport.Int64Slice{}
	}
	if item.Hooks == nil {
		item.Hooks = []domain.HookRelease{}
	}
	var completion any = map[string]any{"mode": "NONE"}
	if len(item.CompletionCheck) > 0 {
		_ = json.Unmarshal(item.CompletionCheck, &completion)
	}
	repository := any(nil)
	if item.RepositoryDisplayURL != "" {
		repository = map[string]any{
			"displayUrl": item.RepositoryDisplayURL, "webUrl": item.RepositoryWebURL, "provider": item.RepositoryProviderType,
			"defaultRefName": item.DefaultRefName, "credentialConfigured": item.RepositoryCredentialVersion > 0,
			"sourceState": item.RepositorySourceState, "capabilities": item.RepositoryCapabilities,
			"lastCheckedAt": item.RepositoryLastCheckedAt, "reasonCode": item.RepositoryLastCheckReasonCode,
		}
	}
	departmentID := any(nil)
	if item.DepartmentID > 0 {
		departmentID = item.DepartmentID
	}
	groupRouteID := any(nil)
	if item.NotificationGroupRouteID > 0 {
		groupRouteID = item.NotificationGroupRouteID
	}
	delivery := map[string]any{"mode": item.DeliveryMode}
	if item.DeliveryMode != "NONE" {
		delivery["authorization"] = item.DeliveryAuthorization
		delivery["resultBranchPattern"] = item.ResultBranchPattern
	}
	return map[string]any{
		"id": item.ID, "tenantId": item.TenantID, "projectKey": item.ProjectKey, "name": item.Name,
		"description": item.Description, "departmentId": departmentID, "departmentName": item.DepartmentName,
		"kind": item.Kind, "status": item.Status, "versionId": item.VersionID, "repository": repository,
		"techStack": item.TechStack, "agentGuidance": item.AgentGuidance,
		"completionCheck": completion, "resultProcessingSummary": resultProcessingSummary(item),
		"reviewPrompt": item.ReviewPrompt, "hookReleaseIds": item.HookReleaseIDs, "hooks": item.Hooks,
		"delivery":     delivery,
		"notification": map[string]any{"enabled": item.NotificationEnabled, "events": item.NotificationEvents, "groupRouteId": groupRouteID, "groupRouteName": item.NotificationGroupRouteName},
		"taskCount":    item.TaskCount, "activeTaskCount": item.ActiveTaskCount, "latestTaskAt": item.LatestTaskAt,
		"createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt, "archivedAt": item.ArchivedAt,
	}
}

func resultProcessingSummary(item domain.Project) string {
	if item.Kind == "GENERAL" {
		return "生成可读结果"
	}
	return "保存代码与检查结果"
}

func decode(c *app.RequestContext, target any) bool {
	if len(c.Request.Body()) > 128<<10 {
		wireError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		wireError(c, http.StatusBadRequest, "INVALID_JSON")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		wireError(c, http.StatusBadRequest, "INVALID_JSON")
		return false
	}
	return true
}
func identity(c *app.RequestContext) (int64, int64, bool) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil || scope.OrgID < 1 {
		wireError(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED")
		return 0, 0, false
	}
	actor, ok := securitycontext.CurrentUserID(c)
	if !ok || actor < 1 {
		wireError(c, http.StatusUnauthorized, "PROJECT_ACTOR_REQUIRED")
		return 0, 0, false
	}
	return scope.OrgID, actor, true
}
func pathID(c *app.RequestContext) (int64, bool) {
	value := c.Param("projectId")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != value {
		wireError(c, http.StatusBadRequest, "INVALID_ID")
		return 0, false
	}
	return id, true
}

func optionalQueryID(c *app.RequestContext, name string) (int64, bool) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != value {
		wireError(c, http.StatusBadRequest, "INVALID_ID")
		return 0, false
	}
	return id, true
}
func write(c *app.RequestContext, value any, err error) {
	if err == nil {
		response.Success(c, value)
		return
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		wireError(c, http.StatusNotFound, "PROJECT_NOT_FOUND")
	case errors.Is(err, domain.ErrConflict):
		wireError(c, http.StatusConflict, "PROJECT_VERSION_CONFLICT")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		wireError(c, http.StatusConflict, "PROJECT_IDEMPOTENCY_CONFLICT")
	case errors.Is(err, domain.ErrArchived):
		wireError(c, http.StatusConflict, "PROJECT_ARCHIVED")
	case errors.Is(err, domain.ErrDeliveryApprovalRequired):
		wireError(c, http.StatusForbidden, "PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED")
	case errors.Is(err, domain.ErrRepositoryURLInvalid):
		wireError(c, http.StatusBadRequest, "PROJECT_REPOSITORY_URL_INVALID")
	case errors.Is(err, domain.ErrRepositoryCredentialRequired):
		wireError(c, http.StatusBadRequest, "PROJECT_REPOSITORY_CREDENTIAL_REQUIRED")
	case errors.Is(err, domain.ErrRepositoryAuthentication):
		wireError(c, http.StatusBadRequest, "PROJECT_REPOSITORY_AUTHENTICATION_FAILED")
	case errors.Is(err, domain.ErrRepositoryNotFound):
		wireError(c, http.StatusBadRequest, "PROJECT_REPOSITORY_NOT_FOUND")
	case errors.Is(err, domain.ErrRepositoryRefNotFound):
		wireError(c, http.StatusBadRequest, "PROJECT_REPOSITORY_REF_NOT_FOUND")
	case errors.Is(err, domain.ErrRepositoryPushUnavailable):
		wireError(c, http.StatusBadRequest, "PROJECT_REPOSITORY_PUSH_UNAVAILABLE")
	case errors.Is(err, domain.ErrRepositoryReviewUnavailable):
		wireError(c, http.StatusBadRequest, "PROJECT_REPOSITORY_REVIEW_UNAVAILABLE")
	case errors.Is(err, domain.ErrSourceUnavailable):
		wireError(c, http.StatusConflict, "PROJECT_REPOSITORY_NOT_READY")
	case errors.Is(err, domain.ErrRepositoryPreparationFailed):
		wireError(c, http.StatusConflict, "PROJECT_REPOSITORY_PREPARATION_FAILED")
	default:
		wireError(c, http.StatusBadRequest, "PROJECT_CONFIGURATION_INVALID")
	}
}
func wireError(c *app.RequestContext, status int, code string) {
	var publicErr error
	switch status {
	case http.StatusBadRequest:
		publicErr = apperrors.Params("项目配置无效")
	case http.StatusNotFound:
		publicErr = apperrors.NotFound("项目不存在")
	case http.StatusConflict:
		publicErr = apperrors.ObjectState("项目状态已变化")
	case http.StatusForbidden:
		publicErr = apperrors.Forbidden("无权限执行该项目操作")
	case http.StatusServiceUnavailable:
		publicErr = apperrors.ServiceUnavailable("项目服务暂时不可用")
	default:
		publicErr = apperrors.System("项目操作暂时不可用")
	}
	response.ErrorWithReason(c, status, publicErr, code, nil)
}
