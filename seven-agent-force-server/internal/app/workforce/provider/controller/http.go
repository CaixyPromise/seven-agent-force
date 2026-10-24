package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/application"
	providerinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Handler struct{ service *application.Service }
type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func New(service *application.Service) *Handler { return &Handler{service: service} }

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.GET("/workforce/model-credentials", wrap("workforce:model-credential:list", h.list))
	router.POST("/workforce/model-credentials", wrap("workforce:model-credential:create", h.create))
	router.PUT("/workforce/model-credentials/:id", wrap("workforce:model-credential:update", h.update))
	router.PUT("/workforce/model-credentials/:id/status", wrap("workforce:model-credential:status", h.status))
	router.DELETE("/workforce/model-credentials/:id", wrap("workforce:model-credential:delete", h.delete))
	router.POST("/workforce/model-credentials/models:discover", wrap("workforce:model-credential:update", h.discover))
	router.GET("/workforce/model-options", wrap("workforce:task:create", h.options))
}

type createRequest struct {
	Name     string   `json:"name"`
	Protocol string   `json:"protocol"`
	BaseURL  string   `json:"baseUrl"`
	APIKey   string   `json:"apiKey"`
	Models   []string `json:"models"`
}

func (h *Handler) create(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, 403, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return
	}
	var request createRequest
	if !decodeBody(c, &request) {
		return
	}
	item, err := h.service.Create(ctx, tenantID, application.CreateCommand{Name: request.Name, Protocol: request.Protocol, BaseURL: request.BaseURL, APIKey: request.APIKey, Models: request.Models})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) list(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, 403, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return
	}
	items, err := h.service.List(ctx, tenantID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, items)
}
func (h *Handler) options(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, 403, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return
	}
	items, err := h.service.Options(ctx, tenantID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, items)
}

type updateRequest struct {
	ExpectedVersion int64    `json:"expectedVersion,string"`
	Name            string   `json:"name"`
	Protocol        string   `json:"protocol"`
	BaseURL         string   `json:"baseUrl"`
	APIKey          string   `json:"apiKey"`
	Models          []string `json:"models"`
}

func (h *Handler) update(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, 403, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return
	}
	var request updateRequest
	if !decodeBody(c, &request) {
		return
	}
	item, err := h.service.Update(ctx, tenantID, pathID(c), application.UpdateCommand{ExpectedVersion: request.ExpectedVersion, Name: request.Name, Protocol: request.Protocol, BaseURL: request.BaseURL, APIKey: request.APIKey, Models: request.Models})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) status(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, 403, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return
	}
	var request struct {
		ExpectedVersion int64  `json:"expectedVersion,string"`
		Status          string `json:"status"`
	}
	if !decodeBody(c, &request) {
		return
	}
	item, err := h.service.SetStatus(ctx, tenantID, pathID(c), request.Status, request.ExpectedVersion)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) delete(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, 403, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return
	}
	var request struct {
		ExpectedVersion int64 `json:"expectedVersion,string"`
	}
	if !decodeBody(c, &request) {
		return
	}
	result, err := h.service.Delete(ctx, tenantID, pathID(c), request.ExpectedVersion)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) discover(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		writeError(c, 403, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return
	}
	var request struct {
		CredentialID int64  `json:"credentialId,string"`
		Protocol     string `json:"protocol"`
		BaseURL      string `json:"baseUrl"`
		APIKey       string `json:"apiKey"`
	}
	if !decodeBody(c, &request) {
		return
	}
	models, err := h.service.Discover(ctx, tenantID, application.DiscoverCommand{CredentialID: request.CredentialID, Protocol: request.Protocol, BaseURL: request.BaseURL, APIKey: request.APIKey})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"models": models})
}

func decodeBody(c *app.RequestContext, target any) bool {
	if len(c.Request.Body()) > 1<<20 {
		writeError(c, 413, "REQUEST_TOO_LARGE", "request body exceeds 1 MiB")
		return false
	}
	if json.Unmarshal(c.Request.Body(), target) != nil {
		writeError(c, 400, "INVALID_JSON", "request body is not valid JSON")
		return false
	}
	return true
}
func tenantScope(c *app.RequestContext) (int64, bool) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	return scope.OrgID, err == nil && scope.OrgID > 0
}
func pathID(c *app.RequestContext) int64 {
	value, err := strconv.ParseInt(string(c.Param("id")), 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func writeServiceError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, application.ErrNotFound):
		writeError(c, 404, "MODEL_CREDENTIAL_NOT_FOUND", "model credential not found")
	case errors.Is(err, application.ErrVersionConflict), errors.Is(err, application.ErrNameConflict):
		writeError(c, 409, err.Error(), "model credential changed")
	case errors.Is(err, application.ErrProtocolInUse):
		writeError(c, 409, "MODEL_CREDENTIAL_PROTOCOL_IN_USE", "model credential protocol is in use")
	case errors.Is(err, providerinfra.ErrDiscoveryAuthentication):
		writeError(c, 401, "MODEL_DISCOVERY_AUTHENTICATION_FAILED", "model discovery authentication failed")
	case errors.Is(err, providerinfra.ErrDiscoveryUnavailable):
		writeError(c, 503, "MODEL_DISCOVERY_UNAVAILABLE", "model discovery is unavailable; models can be entered manually")
	case application.IsRequestValidationError(err):
		writeError(c, 400, "MODEL_CREDENTIAL_INVALID", "model credential request is invalid")
	default:
		writeError(c, 500, "MODEL_CREDENTIAL_INTERNAL_ERROR", "model credential request could not be processed")
	}
}
func writeError(c *app.RequestContext, status int, reason, message string) {
	_ = status
	details := map[string]string{"reasonCode": reason}
	var err *apperrors.AppError
	switch status {
	case 401:
		err = apperrors.Unauthorized(message)
	case 403:
		err = apperrors.DataScopeDenied(message)
	case 404:
		err = apperrors.NotFound(message)
	case 409:
		err = apperrors.ObjectState(message)
	case 500:
		err = apperrors.System(message)
	case 503:
		err = apperrors.ServiceUnavailable(message)
	default:
		err = apperrors.Params(message)
	}
	response.Error(c, err.WithDetails(details))
}
