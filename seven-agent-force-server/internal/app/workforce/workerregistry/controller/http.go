package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	machineidem "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/machineidempotency"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc
type Handler struct {
	service     *application.Service
	idempotency *machineidem.Middleware
}

func New(service *application.Service) *Handler                        { return &Handler{service: service} }
func (h *Handler) SetMachineIdempotency(value *machineidem.Middleware) { h.idempotency = value }

func (h *Handler) MountOperator(router route.IRouter, wrap PermissionWrapper) {
	router.POST("/workforce/workers", wrap("workforce:worker:create", h.create))
	router.GET("/workforce/workers", wrap("workforce:worker:list", h.list))
	router.GET("/workforce/workers/metrics", wrap("workforce:worker:query", h.metrics))
	router.GET("/workforce/workers/:id", wrap("workforce:worker:query", h.get))
	router.GET("/workforce/workers/:id/metrics", wrap("workforce:worker:query", h.workerMetrics))
	router.PUT("/workforce/workers/:id/desired-state", wrap("workforce:worker:operate", h.desiredState))
	router.PUT("/workforce/workers/:id/capacity", wrap("workforce:worker:operate", h.capacity))
	router.POST("/workforce/workers/:id/token-rotations", wrap("workforce:worker:token:rotate", h.rotate))
	router.POST("/workforce/workers/:id/image-inventory-refreshes", wrap("workforce:worker:operate", h.refreshInventory))
	router.POST("/workforce/workers/:id/decommission", wrap("workforce:worker:operate", h.decommission))
	router.GET("/workforce/workers/:id/assignments", wrap("workforce:worker:query", h.assignments))
}

func (h *Handler) MountMachine(router route.IRouter) {
	wrap := func(route string, ttl time.Duration, handler app.HandlerFunc) app.HandlerFunc {
		if h.idempotency == nil {
			return h.machine(handler)
		}
		return h.idempotency.Wrap(route, ttl, handler)
	}
	router.POST("/internal/workforce/v1/workers/:workerId/register", wrap("/workers/{workerId}/register", 24*time.Hour, h.register))
	router.POST("/internal/workforce/v1/workers/:workerId/heartbeat", wrap("/workers/{workerId}/heartbeat", 2*time.Minute, h.heartbeat))
	router.PUT("/internal/workforce/v1/workers/:workerId/image-inventory", wrap("/workers/{workerId}/image-inventory", time.Hour, h.inventory))
	router.POST("/internal/workforce/v1/workers/:workerId/deregister", wrap("/workers/{workerId}/deregister", 24*time.Hour, h.deregister))
	router.GET("/internal/workforce/v1/workers/:workerId/reconciliation", h.machine(h.reconciliation))
}

type createRequest struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	MaxActiveAttempts int    `json:"maxActiveAttempts"`
}

func (h *Handler) create(ctx context.Context, c *app.RequestContext) {
	var req createRequest
	if !decode(c, &req, 1<<20) {
		return
	}
	actorID, ok := securitycontext.CurrentUserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated actor is required")
		return
	}
	w, credential, err := h.service.Provision(ctx, req.Name, req.Description, actorID, req.MaxActiveAttempts)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_WORKER", err.Error())
		return
	}
	response.Success(c, map[string]any{"worker": w, "credential": credential})
}
func (h *Handler) list(ctx context.Context, c *app.RequestContext) {
	items, err := h.service.List(ctx, 100)
	if err != nil {
		writeError(c, 500, "WORKER_LIST_FAILED", "failed to list Workers")
		return
	}
	response.Success(c, items)
}
func (h *Handler) metrics(ctx context.Context, c *app.RequestContext) {
	h.writeMetrics(ctx, c, nil)
}
func (h *Handler) workerMetrics(ctx context.Context, c *app.RequestContext) {
	id := pathID(c, "id")
	if id <= 0 {
		writeError(c, http.StatusBadRequest, "WORKER_NOT_FOUND", "Worker 不存在")
		return
	}
	h.writeMetrics(ctx, c, &id)
}
func (h *Handler) writeMetrics(ctx context.Context, c *app.RequestContext, workerID *int64) {
	series, err := h.service.Metrics(ctx, workerID, string(c.Query("range")))
	if errors.Is(err, application.ErrMetricRangeInvalid) {
		writeError(c, http.StatusBadRequest, "WORKER_METRIC_RANGE_INVALID", "指标时间范围无效")
		return
	}
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, series)
}
func (h *Handler) get(ctx context.Context, c *app.RequestContext) {
	w, images, err := h.service.Get(ctx, pathID(c, "id"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	var reconciliation domain.Reconciliation
	if w.State == domain.StateReconciling && w.CurrentInstanceID != "" && w.HostBootID != "" {
		reconciliation, _ = h.service.Reconciliation(ctx, w.ID, w.CurrentInstanceID, w.HostBootID)
	}
	response.Success(c, map[string]any{"worker": w, "images": images, "queue": domain.QueueFor(w.ID, w.MaxActiveAttempts), "reconciliation": reconciliation})
}

type desiredRequest struct {
	DesiredState string `json:"desiredState"`
	Version      int64  `json:"version,string"`
}

func (h *Handler) desiredState(ctx context.Context, c *app.RequestContext) {
	var req desiredRequest
	if !decode(c, &req, 64<<10) {
		return
	}
	if err := h.service.SetDesiredState(ctx, pathID(c, "id"), req.DesiredState, req.Version); err != nil {
		writeServiceError(c, err)
		return
	}
	w, images, _ := h.service.Get(ctx, pathID(c, "id"))
	response.Success(c, map[string]any{"worker": w, "images": images})
}

type capacityRequest struct {
	MaxActiveAttempts int   `json:"maxActiveAttempts"`
	Version           int64 `json:"version,string"`
}

func (h *Handler) capacity(ctx context.Context, c *app.RequestContext) {
	var req capacityRequest
	if !decode(c, &req, 64<<10) {
		return
	}
	id := pathID(c, "id")
	if err := h.service.UpdateCapacity(ctx, id, req.MaxActiveAttempts, req.Version); err != nil {
		writeServiceError(c, err)
		return
	}
	w, images, err := h.service.Get(ctx, id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"worker": w, "images": images})
}
func (h *Handler) rotate(ctx context.Context, c *app.RequestContext) {
	credential, err := h.service.RotateToken(ctx, pathID(c, "id"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"credential": credential})
}

func (h *Handler) refreshInventory(ctx context.Context, c *app.RequestContext) {
	generation, err := h.service.RequestInventoryRefresh(ctx, pathID(c, "id"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"refreshInventoryGeneration": generation})
}

type decommissionRequest struct {
	Version int64 `json:"version,string"`
}

func (h *Handler) decommission(ctx context.Context, c *app.RequestContext) {
	var req decommissionRequest
	if !decode(c, &req, 64<<10) {
		return
	}
	count, err := h.service.Decommission(ctx, pathID(c, "id"), req.Version)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"decommissioned": true, "purgedMessages": count})
}
func (h *Handler) assignments(ctx context.Context, c *app.RequestContext) {
	items, err := h.service.ListAssignments(ctx, pathID(c, "id"), 100)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, items)
}

type machineEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	WorkerID      int64  `json:"workerId,string"`
	InstanceID    string `json:"instanceId"`
	HostBootID    string `json:"hostBootId"`
	RequestID     string `json:"requestId"`
	Generation    int64  `json:"generation,string,omitempty"`
}
type registerRequest struct {
	machineEnvelope
	StartedAt          string          `json:"startedAt"`
	Version            string          `json:"version"`
	Revision           string          `json:"revision"`
	BuildDate          string          `json:"buildDate"`
	MaxActiveAttempts  int             `json:"maxActiveAttempts"`
	MaxActiveReviews   int             `json:"maxActiveReviews"`
	MaxTotalContainers int             `json:"maxTotalContainers"`
	Capabilities       json.RawMessage `json:"capabilities"`
}

func (h *Handler) register(ctx context.Context, c *app.RequestContext) {
	var req registerRequest
	if !decode(c, &req, 1<<20) || !validateEnvelope(pathID(c, "workerId"), req.machineEnvelope, c) {
		return
	}
	started, err := time.Parse(time.RFC3339, req.StartedAt)
	if err != nil {
		writeError(c, 400, "INVALID_STARTED_AT", "startedAt must be RFC3339")
		return
	}
	result, err := h.service.Register(ctx, application.RegisterCommand{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, StartedAt: started.UTC().Format(time.RFC3339Nano), Version: req.Version, Revision: req.Revision, BuildDate: req.BuildDate, ObservedRemoteIP: remoteIP(c), MaxActiveAttempts: req.MaxActiveAttempts, MaxActiveReviews: req.MaxActiveReviews, MaxTotalContainers: req.MaxTotalContainers, Capabilities: req.Capabilities})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, result)
}

type heartbeatRequest struct {
	machineEnvelope
	Sequence          int64           `json:"sequence,string"`
	ConsumerState     string          `json:"consumerState"`
	DockerReady       bool            `json:"dockerReady"`
	ActiveAttempts    int             `json:"activeAttempts"`
	MaxActiveAttempts int             `json:"maxActiveAttempts"`
	Hostname          string          `json:"hostname"`
	ReportedIP        string          `json:"reportedIp"`
	PrimaryInterface  string          `json:"primaryInterface"`
	PrimaryMAC        string          `json:"primaryMac"`
	Interfaces        json.RawMessage `json:"interfaces"`
	ResourceSnapshot  json.RawMessage `json:"resourceSnapshot"`
}

func (h *Handler) heartbeat(ctx context.Context, c *app.RequestContext) {
	var req heartbeatRequest
	if !decode(c, &req, 1<<20) || !validateEnvelope(pathID(c, "workerId"), req.machineEnvelope, c) {
		return
	}
	result, err := h.service.Heartbeat(ctx, application.HeartbeatCommand{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, Sequence: req.Sequence, ConsumerState: req.ConsumerState, DockerReady: req.DockerReady, ActiveAttempts: req.ActiveAttempts, MaxActiveAttempts: req.MaxActiveAttempts, Hostname: req.Hostname, ReportedIP: req.ReportedIP, PrimaryInterface: req.PrimaryInterface, PrimaryMAC: strings.ToUpper(req.PrimaryMAC), Interfaces: req.Interfaces, ResourceSnapshot: req.ResourceSnapshot})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true, "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "desiredState": result.DesiredState, "derivedState": result.DerivedState, "consumerDirective": result.ConsumerDirective, "refreshInventoryGeneration": result.RefreshInventoryGeneration})
}

type inventoryRequest struct {
	machineEnvelope
	InventoryVersion int64          `json:"inventoryVersion,string"`
	InventoryDigest  string         `json:"inventoryDigest"`
	Images           []domain.Image `json:"images"`
}

func (h *Handler) inventory(ctx context.Context, c *app.RequestContext) {
	var req inventoryRequest
	if !decode(c, &req, 1<<20) || !validateEnvelope(pathID(c, "workerId"), req.machineEnvelope, c) {
		return
	}
	err := h.service.PublishInventory(ctx, application.InventoryCommand{WorkerID: req.WorkerID, InstanceID: req.InstanceID, HostBootID: req.HostBootID, InventoryVersion: req.InventoryVersion, InventoryDigest: req.InventoryDigest, Images: req.Images})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

type deregisterRequest struct {
	machineEnvelope
	Reason string `json:"reason"`
}

func (h *Handler) deregister(ctx context.Context, c *app.RequestContext) {
	var req deregisterRequest
	if !decode(c, &req, 64<<10) || !validateEnvelope(pathID(c, "workerId"), req.machineEnvelope, c) {
		return
	}
	if err := h.service.Deregister(ctx, req.WorkerID, req.InstanceID, req.Reason); err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, map[string]any{"ack": true})
}

func (h *Handler) reconciliation(ctx context.Context, c *app.RequestContext) {
	instanceID := strings.TrimSpace(string(c.Request.Header.Peek("X-Seven-Worker-Instance")))
	hostBootID := strings.TrimSpace(string(c.Request.Header.Peek("X-Seven-Host-Boot-ID")))
	requestID := strings.TrimSpace(string(c.Request.Header.Peek("X-Seven-Request-ID")))
	if instanceID == "" || hostBootID == "" || requestID == "" {
		writeError(c, 400, "INVALID_MACHINE_ENVELOPE", "machine headers are invalid")
		return
	}
	result, err := h.service.Reconciliation(ctx, pathID(c, "workerId"), instanceID, hostBootID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) machine(next app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		workerID := pathID(c, "workerId")
		auth := strings.TrimSpace(string(c.Request.Header.Peek("Authorization")))
		if !strings.HasPrefix(auth, "Bearer ") || h.service.Authenticate(ctx, workerID, strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))) != nil {
			writeError(c, http.StatusUnauthorized, "WORKER_UNAUTHORIZED", "worker credential is invalid")
			return
		}
		next(ctx, c)
	}
}
func pathID(c *app.RequestContext, name string) int64 {
	value, err := strconv.ParseInt(string(c.Param(name)), 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func validateEnvelope(pathWorker int64, e machineEnvelope, c *app.RequestContext) bool {
	if e.SchemaVersion != 1 || e.WorkerID != pathWorker || e.InstanceID == "" || e.HostBootID == "" || e.RequestID == "" || len(e.RequestID) > 128 {
		writeError(c, 400, "INVALID_MACHINE_ENVELOPE", "machine envelope is invalid")
		return false
	}
	return true
}
func decode(c *app.RequestContext, dst any, limit int) bool {
	body := c.Request.Body()
	if len(body) == 0 || len(body) > limit {
		writeError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds limit")
		return false
	}
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(body), int64(limit)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(c, 400, "INVALID_JSON", "request body is invalid")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(c, 400, "INVALID_JSON", "request body has trailing data")
		return false
	}
	return true
}
func remoteIP(c *app.RequestContext) string {
	if c == nil || c.RemoteAddr() == nil {
		return ""
	}
	raw := strings.TrimSpace(c.RemoteAddr().String())
	host, _, err := net.SplitHostPort(raw)
	if err == nil {
		return host
	}
	return raw
}
func writeServiceError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, application.ErrUnauthorized):
		writeError(c, 401, "WORKER_UNAUTHORIZED", err.Error())
	case errors.Is(err, application.ErrNotFound):
		writeError(c, 404, "WORKER_NOT_FOUND", err.Error())
	case errors.Is(err, application.ErrStaleInstance):
		writeError(c, 409, "STALE_WORKER_INSTANCE", err.Error())
	case errors.Is(err, application.ErrNotDrained):
		writeError(c, 409, "WORKER_NOT_DRAINED", err.Error())
	case errors.Is(err, application.ErrProviderRuntimeActive):
		writeError(c, 409, "WORKER_HAS_ACTIVE_PROVIDER_RUNTIME", err.Error())
	case errors.Is(err, application.ErrCapacityExceedsWorker):
		writeError(c, 409, "WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT", "maximum concurrency exceeds this Worker capability")
	case errors.Is(err, application.ErrInvalidCapacity):
		writeError(c, 400, "WORKER_CAPACITY_INVALID", "maximum concurrency must be between 1 and 64")
	case errors.Is(err, application.ErrConflict):
		writeError(c, 409, "WORKER_STATE_CONFLICT", err.Error())
	default:
		writeError(c, 400, "WORKER_REQUEST_REJECTED", err.Error())
	}
}
func writeError(c *app.RequestContext, status int, code, message string) {
	var err *apperrors.AppError
	switch status {
	case 401:
		err = apperrors.Unauthorized(message)
	case 404:
		err = apperrors.NotFound(message)
	case 409:
		err = apperrors.ObjectState(message)
	case 500:
		err = apperrors.System(message)
	default:
		err = apperrors.Params(message)
	}
	response.Error(c, err.WithDetails(map[string]any{"reasonCode": code}))
	c.Response.SetStatusCode(status)
}
