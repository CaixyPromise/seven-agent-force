// Package controller exposes Result Processor authoring over the Framework
// authenticated operator API. It deliberately contains no execution state
// transitions; the application and Machine APIs own those transitions.
package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	processorapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/application"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Handler struct{ service *processorapp.Service }
type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func New(service *processorapp.Service) *Handler { return &Handler{service: service} }

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.GET("/workforce/result-processors", wrap("workforce:result-processor:list", h.listProcessors))
	router.POST("/workforce/result-processors", wrap("workforce:result-processor:author", h.createProcessor))
	router.GET("/workforce/result-processors/:id", wrap("workforce:result-processor:query", h.getProcessor))
	router.POST("/workforce/result-processors/:id/revisions", wrap("workforce:result-processor:author", h.createRevision))
	router.PUT("/workforce/result-processor-revisions/:id", wrap("workforce:result-processor:author", h.updateRevision))
	router.POST("/workforce/result-processor-revisions/:id/archive-import", wrap("workforce:result-processor:author", h.importArchive))
	router.POST("/workforce/result-processor-revisions/:id/validate", wrap("workforce:result-processor:author", h.validate))
	router.POST("/workforce/result-processor-revisions/:id/fixture-runs", wrap("workforce:result-processor:author", h.createFixture))
	router.GET("/workforce/result-processor-fixture-runs/:id", wrap("workforce:result-processor:audit", h.getFixture))
	router.POST("/workforce/result-processor-revisions/:id/publish", wrap("workforce:result-processor:publish", h.publish))
	router.PUT("/workforce/result-processor-bindings/default", wrap("workforce:result-processor:bind", h.setDefault))
	router.PUT("/workforce/result-processor-bindings/:id/environment", wrap("workforce:result-processor:secret:bind", h.setEnvironment))
}
func (h *Handler) listProcessors(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	items, err := h.service.ListProcessors(ctx, tenantID)
	write(c, items, err)
}

func (h *Handler) createProcessor(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	var request processorapp.CreateDraftCommand
	if !decode(c, &request, 8<<20) {
		return
	}
	item, err := h.service.CreateDraft(ctx, tenantID, actor(c), request)
	write(c, item, err)
}

func (h *Handler) getProcessor(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	item, err := h.service.GetProcessor(ctx, tenantID, pathID(c, "id"))
	write(c, item, err)
}

func (h *Handler) createRevision(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	item, err := h.service.CreateRevision(ctx, tenantID, pathID(c, "id"), actor(c))
	write(c, item, err)
}

func (h *Handler) updateRevision(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                  `json:"expectedRevision,string"`
		Manifest         processor.Manifest     `json:"manifest"`
		Files            []processor.SourceFile `json:"files"`
	}
	if !decode(c, &request, 8<<20) {
		return
	}
	item, err := h.service.UpdateDraft(ctx, tenantID, pathID(c, "id"), request.ExpectedRevision, request.Manifest, request.Files)
	write(c, item, err)
}

func (h *Handler) importArchive(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	expected, err := strconv.ParseInt(c.Query("expectedRevision"), 10, 64)
	if err != nil || expected < 1 {
		problem(c, http.StatusBadRequest, "EXPECTED_REVISION_REQUIRED", "expectedRevision must be a positive integer")
		return
	}
	body := c.Request.Body()
	if len(body) > 8<<20 {
		problem(c, http.StatusRequestEntityTooLarge, "RESULT_PROCESSOR_ARCHIVE_TOO_LARGE", "archive exceeds 8 MiB")
		return
	}
	item, err := h.service.ImportArchive(ctx, tenantID, pathID(c, "id"), expected, body)
	write(c, item, err)
}

func (h *Handler) validate(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	item, err := h.service.ValidateDraft(ctx, tenantID, pathID(c, "id"))
	write(c, item, err)
}

func (h *Handler) createFixture(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	var request processorapp.FixtureCommand
	if !decode(c, &request, 8<<20) {
		return
	}
	run, _, err := h.service.RequestFixtureRun(ctx, tenantID, pathID(c, "id"), actor(c), request)
	if err != nil {
		write(c, nil, err)
		return
	}
	c.Response.SetStatusCode(http.StatusAccepted)
	response.Success(c, map[string]any{"runId": strconv.FormatInt(run.ID, 10), "status": run.Status})
}

func (h *Handler) getFixture(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	item, err := h.service.FixtureRun(ctx, tenantID, pathID(c, "id"))
	write(c, item, err)
}

func (h *Handler) publish(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	var request struct {
		Version string `json:"version"`
	}
	if !decode(c, &request, 4096) {
		return
	}
	item, err := h.service.PublishRelease(ctx, tenantID, pathID(c, "id"), actor(c), request.Version)
	write(c, item, err)
}

func (h *Handler) setDefault(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	var request struct {
		ProcessorReleaseID int64 `json:"processorReleaseId,string"`
	}
	if !decode(c, &request, 4096) {
		return
	}
	err := h.service.SetDefaultBinding(ctx, tenantID, actor(c), request.ProcessorReleaseID)
	write(c, map[string]any{"name": "default", "processorReleaseId": strconv.FormatInt(request.ProcessorReleaseID, 10)}, err)
}

func (h *Handler) setEnvironment(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := tenant(c)
	if !ok {
		return
	}
	var request infrastructure.EnvironmentBinding
	if !decode(c, &request, 64<<10) {
		return
	}
	request.ProcessorReleaseID = pathID(c, "id")
	item, err := h.service.SetEnvironmentBinding(ctx, tenantID, actor(c), request)
	write(c, item, err)
}

func tenant(c *app.RequestContext) (int64, bool) {
	value, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if err != nil || value.OrgID < 1 {
		problem(c, http.StatusForbidden, "TENANT_SCOPE_REQUIRED", "organization scope is required")
		return 0, false
	}
	return value.OrgID, true
}

func actor(c *app.RequestContext) int64 {
	if id, ok := securitycontext.CurrentUserID(c); ok {
		return id
	}
	return 0
}

func pathID(c *app.RequestContext, name string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(string(c.Param(name))), 10, 64)
	if err != nil || value < 1 {
		return 0
	}
	return value
}

func decode(c *app.RequestContext, target any, max int) bool {
	if len(c.Request.Body()) > max {
		problem(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds the endpoint limit")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		problem(c, http.StatusBadRequest, "INVALID_JSON", "request body is not valid for this operation")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		problem(c, http.StatusBadRequest, "INVALID_JSON", "request body contains trailing JSON values")
		return false
	}
	return true
}

func write(c *app.RequestContext, value any, err error) {
	if err == nil {
		response.Success(c, value)
		return
	}
	switch {
	case errors.Is(err, infrastructure.ErrNotFound):
		problem(c, http.StatusNotFound, "RESULT_PROCESSOR_NOT_FOUND", "Result Processor resource was not found")
	case errors.Is(err, infrastructure.ErrConflict):
		problem(c, http.StatusConflict, "RESULT_PROCESSOR_REVISION_CONFLICT", err.Error())
	default:
		problem(c, http.StatusBadRequest, resultCode(err), err.Error())
	}
}

func resultCode(err error) string {
	message := err.Error()
	if strings.Contains(message, "RESULT_PROCESSOR_DEFAULT_NOT_CONFIGURED") {
		return "RESULT_PROCESSOR_DEFAULT_NOT_CONFIGURED"
	}
	if strings.Contains(message, "RESULT_PROCESSOR_FIXTURE_WORKER_UNAVAILABLE") {
		return "RESULT_PROCESSOR_FIXTURE_WORKER_UNAVAILABLE"
	}
	if strings.Contains(message, "RESULT_PROCESSOR_VALIDATION_REQUIRED") {
		return "RESULT_PROCESSOR_VALIDATION_REQUIRED"
	}
	return "RESULT_PROCESSOR_REQUEST_INVALID"
}

func problem(c *app.RequestContext, status int, code, message string) {
	details := map[string]string{"reasonCode": code}
	var err *apperrors.AppError
	switch status {
	case http.StatusForbidden:
		err = apperrors.DataScopeDenied(message)
	case http.StatusNotFound:
		err = apperrors.NotFound(message)
	case http.StatusConflict:
		err = apperrors.ObjectState(message)
	case http.StatusInternalServerError:
		err = apperrors.System(message)
	default:
		err = apperrors.Params(message)
	}
	response.Error(c, err.WithDetails(details))
}
