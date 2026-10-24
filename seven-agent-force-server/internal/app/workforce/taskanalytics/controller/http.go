package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	analytics "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskanalytics/application"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

type Handler struct{ service *analytics.Service }

func New(service *analytics.Service) *Handler { return &Handler{service: service} }

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	// Mount the static analytics paths before Project and Task parameter routes.
	router.GET("/workforce/projects/analytics", wrap("workforce:task:list", h.projects))
	router.GET("/workforce/projects/:projectId/analytics", wrap("workforce:task:query", h.project))
	router.GET("/workforce/tasks/analytics", wrap("workforce:task:list", h.tasks))
	router.GET("/workforce/workbench", wrap("workforce:task:list", h.workbench))
}

func (h *Handler) workbench(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	value := strings.TrimSpace(c.Query("range"))
	if value == "" {
		value = "14d"
	}
	result, err := h.service.Workbench(ctx, tenantID, value, securitycontext.HasPermission(c, "workforce:model-credential:usage"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) projects(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	value := strings.TrimSpace(c.Query("range"))
	if value == "" {
		value = "30d"
	}
	result, err := h.service.ProjectList(ctx, tenantID, value)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) project(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	projectID, err := strconv.ParseInt(strings.TrimSpace(c.Param("projectId")), 10, 64)
	if err != nil || projectID < 1 {
		response.Error(c, apperrors.Params("项目编号无效").WithDetails(map[string]string{"reasonCode": "INVALID_ID"}))
		return
	}
	value := strings.TrimSpace(c.Query("range"))
	if value == "" {
		value = "30d"
	}
	result, err := h.service.Project(ctx, tenantID, projectID, value)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) tasks(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	value := strings.TrimSpace(c.Query("range"))
	if value == "" {
		value = "14d"
	}
	result, err := h.service.Tasks(ctx, tenantID, value)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func tenant(c *app.RequestContext) (int64, bool) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil || scope.OrgID < 1 {
		response.Error(c, apperrors.DataScopeDenied("需要明确的组织范围"))
		return 0, false
	}
	return scope.OrgID, true
}

func writeError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, analytics.ErrRangeInvalid):
		response.Error(c, apperrors.Params("分析范围无效").WithDetails(map[string]string{"reasonCode": "ANALYTICS_RANGE_INVALID"}))
	case errors.Is(err, analytics.ErrProjectNotFound):
		response.ErrorWithStatus(c, http.StatusNotFound, apperrors.NotFound("项目不存在").WithDetails(map[string]string{"reasonCode": "PROJECT_NOT_FOUND"}))
	default:
		response.Error(c, apperrors.System("分析数据暂时无法加载"))
	}
}
