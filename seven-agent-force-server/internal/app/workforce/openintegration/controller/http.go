package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	openapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	resultfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

type Handler struct{ service *openapp.Service }

func New(service *openapp.Service) *Handler { return &Handler{service: service} }

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.GET("/workforce/open-integrations", wrap("workforce:open-integration:read", h.listApplications))
	router.POST("/workforce/open-integrations", wrap("workforce:open-integration:create", h.createApplication))
	router.POST("/workforce/open-integrations/:id/enable", wrap("workforce:open-integration:operate", h.enableApplication))
	router.POST("/workforce/open-integrations/:id/disable", wrap("workforce:open-integration:operate", h.disableApplication))
	router.POST("/workforce/open-integrations/:id/api-key/rotate", wrap("workforce:open-integration:operate", h.rotateAPIKey))
	router.POST("/workforce/open-integrations/:id/signing-secret/rotate", wrap("workforce:open-integration:operate", h.rotateSigningSecret))
	router.GET("/workforce/open-integrations/:id/deliveries", wrap("workforce:open-integration:read", h.listDeliveries))
	router.GET("/open/workforce/v1/capabilities", h.getOpenCapabilities)
	router.POST("/open/workforce/v1/projects/:projectId/tasks", h.createOpenTask)
	router.GET("/open/workforce/v1/projects/:projectId/model-options", h.listModelOptions)
	router.GET("/open/workforce/v1/projects/:projectId/runtime-options", h.listRuntimeOptions)
	router.GET("/open/workforce/v1/tasks/:id", h.getOpenTask)
	router.GET("/open/workforce/v1/tasks/:id/outputs", h.listOpenTaskOutputs)
	router.GET("/open/workforce/v1/tasks/:id/outputs/:outputId/download", h.downloadOpenTaskOutput)
}

func (h *Handler) getOpenCapabilities(ctx context.Context, c *app.RequestContext) {
	identity, err := h.service.Authenticate(ctx, string(c.Request.Header.Peek("Authorization")))
	if err != nil {
		writeOpenError(c, err)
		return
	}
	response.Success(c, h.service.GetOpenCapabilities(ctx, identity))
}

func (h *Handler) createOpenTask(ctx context.Context, c *app.RequestContext) {
	identity, err := h.service.Authenticate(ctx, string(c.Request.Header.Peek("Authorization")))
	if err != nil {
		writeOpenError(c, err)
		return
	}
	if !isJSONContentType(string(c.Request.Header.ContentType())) {
		writeOpenError(c, domain.ErrInvalidRequest)
		return
	}
	projectID, ok := positiveOpenID(c.Param("projectId"))
	if !ok {
		writeOpenError(c, domain.ErrInvalidRequest)
		return
	}
	body := c.Request.Body()
	if len(body) > 32<<10 || !utf8.Valid(body) {
		writeOpenError(c, domain.ErrInvalidRequest)
		return
	}
	var request domain.CreateOpenTaskRequest
	if err := decodeStrict(body, &request); err != nil {
		writeOpenError(c, domain.ErrInvalidRequest)
		return
	}
	request.ProjectID = projectID
	view, err := h.service.SubmitOpenTask(ctx, identity, string(c.Request.Header.Peek("Idempotency-Key")), request)
	if err != nil {
		writeOpenError(c, err)
		return
	}
	response.SuccessWithStatus(c, http.StatusAccepted, view)
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func (h *Handler) getOpenTask(ctx context.Context, c *app.RequestContext) {
	identity, err := h.service.Authenticate(ctx, string(c.Request.Header.Peek("Authorization")))
	if err != nil {
		writeOpenError(c, err)
		return
	}
	taskID, ok := positiveOpenID(c.Param("id"))
	if !ok {
		writeOpenError(c, openinfra.ErrNotFound)
		return
	}
	view, err := h.service.GetOpenTask(ctx, identity, taskID)
	if err != nil {
		writeOpenError(c, err)
		return
	}
	response.Success(c, view)
}

func (h *Handler) listOpenTaskOutputs(ctx context.Context, c *app.RequestContext) {
	identity, err := h.service.Authenticate(ctx, string(c.Request.Header.Peek("Authorization")))
	if err != nil {
		writeOpenError(c, err)
		return
	}
	taskID, ok := positiveOpenID(c.Param("id"))
	if !ok {
		writeOpenError(c, resultfacade.ErrOutputNotFound)
		return
	}
	view, err := h.service.ListOpenTaskOutputs(ctx, identity, taskID)
	if err != nil {
		writeOpenError(c, err)
		return
	}
	response.Success(c, view)
}

func (h *Handler) downloadOpenTaskOutput(ctx context.Context, c *app.RequestContext) {
	identity, err := h.service.Authenticate(ctx, string(c.Request.Header.Peek("Authorization")))
	if err != nil {
		writeOpenError(c, err)
		return
	}
	taskID, taskOK := positiveOpenID(c.Param("id"))
	outputID, outputOK := positiveOpenID(c.Param("outputId"))
	if !taskOK || !outputOK {
		writeOpenError(c, resultfacade.ErrOutputNotFound)
		return
	}
	download, err := h.service.OpenTaskOutput(ctx, identity, taskID, outputID)
	if err != nil {
		writeOpenError(c, err)
		return
	}
	maxInt := int64(^uint(0) >> 1)
	if download.Output.SizeBytes < 0 || download.Output.SizeBytes > maxInt {
		_ = download.Content.Close()
		writeOpenError(c, resultfacade.ErrOutputUnavailable)
		return
	}

	fileName := safeDownloadName(download.Output.Name)
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": fileName})
	if disposition == "" {
		disposition = "attachment"
	}
	c.Response.Header.Set("Content-Type", safeMediaType(download.Output.MediaType))
	c.Response.Header.Set("Content-Disposition", disposition)
	c.Response.Header.Set("Cache-Control", "private, no-store, max-age=0")
	c.Response.Header.Set("Pragma", "no-cache")
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.SetStatusCode(http.StatusOK)
	c.Response.SetBodyStream(newClassifiedDownloadStream(download.Content, taskID, outputID, reportDownloadStreamFailure), int(download.Output.SizeBytes))
}

type downloadStreamFailureReporter func(category string, taskID, outputID int64)

type classifiedDownloadStream struct {
	content           io.ReadCloser
	taskID, outputID  int64
	report            downloadStreamFailureReporter
	reportFailureOnce sync.Once
}

func newClassifiedDownloadStream(content io.ReadCloser, taskID, outputID int64, report downloadStreamFailureReporter) io.ReadCloser {
	return &classifiedDownloadStream{content: content, taskID: taskID, outputID: outputID, report: report}
}

func (s *classifiedDownloadStream) Read(buffer []byte) (int, error) {
	read, err := s.content.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		s.reportFailure("object_read_error")
	}
	return read, err
}

func (s *classifiedDownloadStream) Close() error {
	err := s.content.Close()
	if err != nil {
		s.reportFailure("object_close_error")
	}
	return err
}

func (s *classifiedDownloadStream) reportFailure(category string) {
	if s.report == nil {
		return
	}
	s.reportFailureOnce.Do(func() { s.report(category, s.taskID, s.outputID) })
}

func reportDownloadStreamFailure(category string, taskID, outputID int64) {
	// The underlying reader error may contain a private object key or storage
	// endpoint. Record only a stable category and public entity identities.
	log.Printf("open output stream aborted: category=%s task_id=%d output_id=%d", category, taskID, outputID)
}

func positiveOpenID(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

func safeDownloadName(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	value = path.Base(value)
	runes := make([]rune, 0, min(utf8.RuneCountInString(value), 160))
	for _, current := range value {
		if unicode.IsControl(current) || current == '"' || current == '\'' || current == '/' || current == '\\' {
			continue
		}
		if len(runes) == 160 {
			break
		}
		runes = append(runes, current)
	}
	result := strings.TrimSpace(string(runes))
	if result == "" || result == "." || result == ".." {
		return "output"
	}
	return result
}

func safeMediaType(value string) string {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil || mediaType == "" {
		return "application/octet-stream"
	}
	formatted := mime.FormatMediaType(mediaType, params)
	if formatted == "" {
		return "application/octet-stream"
	}
	return formatted
}

func (h *Handler) listApplications(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusForbidden, apperrors.DataScopeDenied("an unambiguous organization scope is required"))
		return
	}
	items, err := h.service.ListApplications(ctx, tenantID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError, apperrors.System("failed to list integration applications"))
		return
	}
	response.Success(c, items)
}

func (h *Handler) createApplication(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusForbidden, apperrors.DataScopeDenied("an unambiguous organization scope is required"))
		return
	}
	if len(c.Request.Body()) > 16<<10 {
		response.ErrorWithStatus(c, http.StatusBadRequest, apperrors.Params("invalid request"))
		return
	}
	var request openapp.CreateApplicationRequest
	if err := decodeStrict(c.Request.Body(), &request); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, apperrors.Params("invalid request"))
		return
	}
	creatorUserID, ok := securitycontext.CurrentUserID(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized, apperrors.Unauthorized("an authenticated user identity is required"))
		return
	}
	created, err := h.service.CreateApplication(ctx, tenantID, creatorUserID, request)
	if err != nil {
		status, _, message := openapp.StableError(err)
		response.ErrorWithStatus(c, status, apperrors.Params(message))
		return
	}
	writeOneTimeSecret(c, created)
}

func (h *Handler) enableApplication(ctx context.Context, c *app.RequestContext) {
	h.setApplicationStatus(ctx, c, domain.ApplicationEnabled)
}

func (h *Handler) disableApplication(ctx context.Context, c *app.RequestContext) {
	h.setApplicationStatus(ctx, c, domain.ApplicationDisabled)
}

func (h *Handler) rotateAPIKey(ctx context.Context, c *app.RequestContext) {
	if len(c.Request.Body()) != 0 {
		response.ErrorWithStatus(c, http.StatusBadRequest, apperrors.Params("credential rotation does not accept a request body"))
		return
	}
	tenantID, applicationID, actorUserID, ok := rotationScope(c)
	if !ok {
		return
	}
	rotated, err := h.service.RotateAPIKey(ctx, tenantID, applicationID, actorUserID)
	if err != nil {
		writeRotationError(c, err)
		return
	}
	writeOneTimeSecret(c, rotated)
}

func (h *Handler) rotateSigningSecret(ctx context.Context, c *app.RequestContext) {
	if len(c.Request.Body()) != 0 {
		response.ErrorWithStatus(c, http.StatusBadRequest, apperrors.Params("credential rotation does not accept a request body"))
		return
	}
	tenantID, applicationID, actorUserID, ok := rotationScope(c)
	if !ok {
		return
	}
	rotated, err := h.service.RotateSigningSecret(ctx, tenantID, applicationID, actorUserID)
	if err != nil {
		writeRotationError(c, err)
		return
	}
	writeOneTimeSecret(c, rotated)
}

func rotationScope(c *app.RequestContext) (tenantID, applicationID, actorUserID int64, ok bool) {
	tenantID, ok = tenantScope(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusForbidden, apperrors.DataScopeDenied("an unambiguous organization scope is required"))
		return 0, 0, 0, false
	}
	applicationID, ok = positiveOpenID(c.Param("id"))
	if !ok {
		response.ErrorWithStatus(c, http.StatusNotFound, apperrors.NotFound("integration application not found"))
		return 0, 0, 0, false
	}
	actorUserID, ok = securitycontext.CurrentUserID(c)
	if !ok || actorUserID <= 0 {
		response.ErrorWithStatus(c, http.StatusUnauthorized, apperrors.Unauthorized("an authenticated user identity is required"))
		return 0, 0, 0, false
	}
	return tenantID, applicationID, actorUserID, true
}

func writeRotationError(c *app.RequestContext, err error) {
	if errors.Is(err, openinfra.ErrNotFound) {
		response.ErrorWithStatus(c, http.StatusNotFound, apperrors.NotFound("integration application not found"))
		return
	}
	if errors.Is(err, domain.ErrInvalidRequest) {
		response.ErrorWithStatus(c, http.StatusBadRequest, apperrors.Params("invalid rotation request"))
		return
	}
	if errors.Is(err, openinfra.ErrWebhookNotConfigured) {
		response.ErrorWithStatus(c, http.StatusConflict, apperrors.ObjectState("Webhook is not configured for this integration application.").WithDetails(map[string]string{"reasonCode": "WEBHOOK_NOT_CONFIGURED"}))
		return
	}
	response.ErrorWithStatus(c, http.StatusInternalServerError, apperrors.System("failed to rotate integration credential"))
}

func writeOneTimeSecret(c *app.RequestContext, value any) {
	c.Response.Header.Set("Cache-Control", "no-store")
	c.Response.Header.Set("Pragma", "no-cache")
	response.Success(c, value)
}

func (h *Handler) setApplicationStatus(ctx context.Context, c *app.RequestContext, status string) {
	tenantID, ok := tenantScope(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusForbidden, apperrors.DataScopeDenied("an unambiguous organization scope is required"))
		return
	}
	applicationID, parseErr := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if parseErr != nil || applicationID <= 0 {
		response.ErrorWithStatus(c, http.StatusNotFound, apperrors.NotFound("integration application not found"))
		return
	}
	if err := h.service.SetApplicationStatus(ctx, tenantID, applicationID, status); err != nil {
		if errors.Is(err, openinfra.ErrNotFound) {
			response.ErrorWithStatus(c, http.StatusNotFound, apperrors.NotFound("integration application not found"))
			return
		}
		response.ErrorWithStatus(c, http.StatusInternalServerError, apperrors.System("failed to update integration application"))
		return
	}
	response.Success(c, map[string]string{"status": status})
}

func (h *Handler) listDeliveries(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenantScope(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusForbidden, apperrors.DataScopeDenied("an unambiguous organization scope is required"))
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	applicationID, parseErr := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if parseErr != nil || applicationID <= 0 {
		response.ErrorWithStatus(c, http.StatusNotFound, apperrors.NotFound("integration application not found"))
		return
	}
	items, err := h.service.ListDeliveries(ctx, tenantID, applicationID, limit)
	if err != nil {
		if errors.Is(err, openinfra.ErrNotFound) {
			response.ErrorWithStatus(c, http.StatusNotFound, apperrors.NotFound("integration application not found"))
			return
		}
		response.ErrorWithStatus(c, http.StatusInternalServerError, apperrors.System("failed to list webhook deliveries"))
		return
	}
	response.Success(c, items)
}

func decodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func tenantScope(c *app.RequestContext) (int64, bool) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	return scope.OrgID, err == nil && scope.OrgID > 0
}

func writeOpenError(c *app.RequestContext, err error) {
	status, code, message := openapp.StableError(err)
	if code == "INTERNAL_ERROR" {
		// Open clients receive only the stable public envelope; the local Server
		// log retains the typed internal cause needed to operate the integration.
		// Submission request bodies are hash-only in request logging and are not
		// included here.
		log.Printf("open integration internal error: %T: %v", err, err)
	}
	var publicErr error
	switch status {
	case http.StatusBadRequest:
		publicErr = apperrors.Params(message)
	case http.StatusUnauthorized:
		publicErr = apperrors.Unauthorized(message)
	case http.StatusForbidden:
		publicErr = apperrors.Forbidden(message)
	case http.StatusNotFound:
		publicErr = apperrors.NotFound(message)
	case http.StatusConflict:
		publicErr = apperrors.ObjectState(message)
	case http.StatusTooManyRequests:
		publicErr = apperrors.RateLimited(message)
	case http.StatusServiceUnavailable:
		publicErr = apperrors.ServiceUnavailable(message)
	default:
		publicErr = apperrors.System(message)
	}
	response.ErrorWithReason(c, status, publicErr, code, nil)
}

func (h *Handler) listModelOptions(ctx context.Context, c *app.RequestContext) {
	identity, err := h.service.Authenticate(ctx, string(c.Request.Header.Peek("Authorization")))
	if err != nil {
		writeOpenError(c, err)
		return
	}
	projectID, ok := positiveOpenID(c.Param("projectId"))
	if !ok {
		writeOpenError(c, domain.ErrInvalidRequest)
		return
	}
	result, err := h.service.ListModelOptions(ctx, identity, projectID)
	if err != nil {
		writeOpenError(c, err)
		return
	}
	response.Success(c, result)
}
func (h *Handler) listRuntimeOptions(ctx context.Context, c *app.RequestContext) {
	identity, err := h.service.Authenticate(ctx, string(c.Request.Header.Peek("Authorization")))
	if err != nil {
		writeOpenError(c, err)
		return
	}
	projectID, ok := positiveOpenID(c.Param("projectId"))
	credentialID, credentialOK := positiveOpenID(c.Query("modelCredentialId"))
	if !ok || !credentialOK {
		writeOpenError(c, domain.ErrInvalidRequest)
		return
	}
	result, err := h.service.ListRuntimeOptions(ctx, identity, projectID, credentialID)
	if err != nil {
		writeOpenError(c, err)
		return
	}
	response.Success(c, result)
}
