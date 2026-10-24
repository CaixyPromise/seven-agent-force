package controller

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	invocationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/application"
	invocationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type WorkerAuthenticator interface {
	Authenticate(context.Context, int64, string) error
}

type Handler struct {
	service *invocationapp.Service
	auth    WorkerAuthenticator
}

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

func New(service *invocationapp.Service, auth WorkerAuthenticator) *Handler {
	return &Handler{service: service, auth: auth}
}

func (h *Handler) Mount(router route.IRouter) {
	if h == nil || h.service == nil || h.auth == nil || router == nil {
		return
	}
	router.POST("/internal/workforce/v1/model-invocations/responses", h.invoke(invocationdomain.OperationResponsesCreate))
	router.POST("/internal/workforce/v1/model-invocations/responses/compact", h.invoke(invocationdomain.OperationResponsesCompact))
	router.POST("/internal/workforce/v1/model-invocations/messages", h.invoke(invocationdomain.OperationMessagesCreate))
	router.POST("/internal/workforce/v1/model-invocations/messages/count_tokens", h.invoke(invocationdomain.OperationMessagesCountTokens))
}

func (h *Handler) MountOperator(router route.IRouter, wrap PermissionWrapper) {
	if h == nil || h.service == nil || router == nil || wrap == nil {
		return
	}
	router.GET("/workforce/model-credentials/analytics", wrap("workforce:model-credential:usage", h.analytics))
	router.GET("/workforce/model-credentials/:id/invocations", wrap("workforce:model-credential:usage", h.listCredential))
	router.GET("/workforce/tasks/:id/model-invocations", wrap("workforce:task:query", h.listTask))
	router.GET("/workforce/model-invocations/:id", wrap("workforce:model-credential:usage", h.getInvocation))
}

func (h *Handler) analytics(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := operatorTenantScope(c)
	if !ok {
		response.Error(c, apperrors.DataScopeDenied("organization scope is required"))
		return
	}
	days := 30
	if raw := strings.TrimSpace(string(c.Query("days"))); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			response.Error(c, apperrors.Params("days must be 7, 30, or 90"))
			return
		}
		days = parsed
	}
	result, err := h.service.GetAnalytics(ctx, tenantID, days)
	if err != nil {
		var domainErr *invocationdomain.Error
		if errors.As(err, &domainErr) && domainErr.HTTPStatus == http.StatusBadRequest {
			response.Error(c, apperrors.Params(domainErr.Message))
			return
		}
		response.Error(c, apperrors.System("failed to load model analytics"))
		return
	}
	response.Success(c, result)
}

func (h *Handler) listCredential(ctx context.Context, c *app.RequestContext) {
	h.list(ctx, c, func(tenantID int64, limit int) ([]invocationdomain.View, error) {
		return h.service.ListCredentialInvocations(ctx, tenantID, positivePathID(c, "id"), limit)
	})
}

func (h *Handler) listTask(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := operatorTenantScope(c)
	if !ok {
		response.Error(c, apperrors.DataScopeDenied("organization scope is required"))
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(string(c.Query("limit"))))
	items, err := h.service.ListTaskInvocations(ctx, tenantID, positivePathID(c, "id"), limit)
	if err != nil {
		response.Error(c, apperrors.System("failed to list model invocations"))
		return
	}
	response.Success(c, items)
}

func (h *Handler) list(ctx context.Context, c *app.RequestContext, load func(int64, int) ([]invocationdomain.View, error)) {
	tenantID, ok := operatorTenantScope(c)
	if !ok {
		response.Error(c, apperrors.DataScopeDenied("organization scope is required"))
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(string(c.Query("limit"))))
	items, err := load(tenantID, limit)
	if err != nil {
		response.Error(c, apperrors.System("failed to list model invocations"))
		return
	}
	response.Success(c, items)
}

func (h *Handler) getInvocation(ctx context.Context, c *app.RequestContext) {
	tenantID, ok := operatorTenantScope(c)
	if !ok {
		response.Error(c, apperrors.DataScopeDenied("organization scope is required"))
		return
	}
	item, err := h.service.ResolveInvocationView(ctx, tenantID, positivePathID(c, "id"))
	if err != nil {
		response.Error(c, apperrors.NotFound("model invocation not found"))
		return
	}
	response.Success(c, item)
}

func operatorTenantScope(c *app.RequestContext) (int64, bool) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	return scope.OrgID, err == nil && scope.OrgID > 0
}

func (h *Handler) invoke(operation invocationdomain.Operation) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		identity, token, err := readIdentity(c)
		if err != nil {
			writeError(c, err)
			return
		}
		if err = h.auth.Authenticate(ctx, identity.WorkerID, token); err != nil {
			writeError(c, &invocationdomain.Error{ReasonCode: "WORKER_UNAUTHORIZED", HTTPStatus: http.StatusUnauthorized, Message: "worker credential is invalid"})
			return
		}
		body := append([]byte(nil), c.Request.Body()...)
		result, err := h.service.InvokeProvider(ctx, invocationapp.InvokeRequest{
			Identity: identity, Operation: operation, Body: body, Accept: string(c.Request.Header.Peek("Accept")), Header: http.Header{
				"Anthropic-Version": []string{string(c.Request.Header.Peek("anthropic-version"))},
				"Anthropic-Beta":    []string{string(c.Request.Header.Peek("anthropic-beta"))},
			},
		})
		if err != nil {
			writeError(c, err)
			return
		}
		for name, values := range result.Header {
			for _, value := range values {
				c.Response.Header.Add(name, value)
			}
		}
		c.Response.Header.Set("X-Seven-Invocation-ID", result.ProtocolInvocationKey)
		c.Response.Header.SetStatusCode(result.StatusCode)
		c.Response.SetBodyStream(result.Body, -1)
	}
}

func readIdentity(c *app.RequestContext) (invocationdomain.RequestIdentity, string, error) {
	authorization := strings.TrimSpace(string(c.Request.Header.Peek("Authorization")))
	if !strings.HasPrefix(authorization, "Bearer ") {
		return invocationdomain.RequestIdentity{}, "", invalidIdentity("WORKER_UNAUTHORIZED", http.StatusUnauthorized, "worker bearer credential is required")
	}
	parseInt := func(name string) (int64, error) {
		value, err := strconv.ParseInt(strings.TrimSpace(string(c.Request.Header.Peek(name))), 10, 64)
		if err != nil || value < 1 {
			return 0, invalidIdentity("PROVIDER_INVOCATION_IDENTITY_INVALID", http.StatusBadRequest, name+" is invalid")
		}
		return value, nil
	}
	kind := invocationdomain.ExecutionKind(strings.TrimSpace(string(c.Request.Header.Peek("X-Seven-Execution-Kind"))))
	if kind == "" {
		kind = invocationdomain.ExecutionKindAgentAttempt
	}
	if !kind.Valid() {
		return invocationdomain.RequestIdentity{}, "", invalidIdentity("PROVIDER_EXECUTION_KIND_INVALID", http.StatusBadRequest, "X-Seven-Execution-Kind is invalid")
	}
	ownerEpoch, err := parseInt("X-Seven-Owner-Epoch")
	if err != nil {
		return invocationdomain.RequestIdentity{}, "", err
	}
	timestamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(c.Request.Header.Peek("X-Seven-Timestamp"))))
	if err != nil {
		return invocationdomain.RequestIdentity{}, "", invalidIdentity("PROVIDER_TIMESTAMP_INVALID", http.StatusBadRequest, "X-Seven-Timestamp is invalid")
	}
	workerID, err := parseInt("X-Seven-Worker-ID")
	if err != nil {
		return invocationdomain.RequestIdentity{}, "", err
	}
	taskID, err := parseInt("X-Seven-Task-ID")
	if err != nil {
		return invocationdomain.RequestIdentity{}, "", err
	}
	identity := invocationdomain.RequestIdentity{
		ExecutionKind: kind,
		WorkerID:      workerID, InstanceID: header(c, "X-Seven-Worker-Instance"),
		HostBootID: header(c, "X-Seven-Host-Boot-ID"), TaskID: taskID,
		OwnerEpoch:            ownerEpoch,
		ProtocolInvocationKey: header(c, "X-Seven-Invocation-ID"),
		Nonce:                 header(c, "X-Seven-Nonce"), BodySHA256: strings.TrimPrefix(strings.ToLower(header(c, "X-Seven-Body-SHA256")), "sha256:"), Timestamp: timestamp,
	}
	if kind == invocationdomain.ExecutionKindAgentAttempt {
		generation, parseErr := parseInt("X-Seven-Generation")
		if parseErr != nil {
			return invocationdomain.RequestIdentity{}, "", parseErr
		}
		attemptID, parseErr := parseInt("X-Seven-Attempt-ID")
		if parseErr != nil {
			return invocationdomain.RequestIdentity{}, "", parseErr
		}
		identity.Generation = generation
		identity.AttemptID = attemptID
		identity.LeaseToken = header(c, "X-Seven-Attempt-Lease")
	} else {
		if header(c, "X-Seven-Attempt-ID") != "" || header(c, "X-Seven-Generation") != "" || header(c, "X-Seven-Attempt-Lease") != "" {
			return invocationdomain.RequestIdentity{}, "", invalidIdentity("PROVIDER_REVIEW_RUN_IDENTITY_MIXED", http.StatusBadRequest, "ReviewRun identity cannot contain ordinary Attempt headers")
		}
		identity.ReviewRunID, err = parseInt("X-Seven-Review-Run-ID")
		if err != nil {
			return invocationdomain.RequestIdentity{}, "", err
		}
		identity.ReviewRunAttemptID, err = parseInt("X-Seven-Review-Run-Attempt-ID")
		if err != nil {
			return invocationdomain.RequestIdentity{}, "", err
		}
		identity.LeaseToken = header(c, "X-Seven-Review-Run-Lease")
		identity.StructuredOutputContract = header(c, "X-Seven-Review-Contract")
	}
	for name, value := range map[string]string{
		"instanceId": identity.InstanceID, "hostBootId": identity.HostBootID, "executionLease": identity.LeaseToken,
		"protocolInvocationKey": identity.ProtocolInvocationKey, "nonce": identity.Nonce, "bodySha256": identity.BodySHA256,
	} {
		if value == "" || len(value) > 512 {
			return invocationdomain.RequestIdentity{}, "", invalidIdentity("PROVIDER_INVOCATION_IDENTITY_INVALID", http.StatusBadRequest, name+" is missing or too long")
		}
	}
	if len(identity.ProtocolInvocationKey) > 64 {
		return invocationdomain.RequestIdentity{}, "", invalidIdentity("PROVIDER_INVOCATION_IDENTITY_INVALID", http.StatusBadRequest, "protocolInvocationKey is too long")
	}
	decodedDigest, digestErr := hex.DecodeString(identity.BodySHA256)
	if len(identity.BodySHA256) != 64 || digestErr != nil || len(decodedDigest) != 32 {
		return invocationdomain.RequestIdentity{}, "", invalidIdentity("PROVIDER_BODY_DIGEST_MISMATCH", http.StatusBadRequest, "body SHA-256 is invalid")
	}
	if err := identity.Validate(); err != nil {
		return invocationdomain.RequestIdentity{}, "", err
	}
	return identity, strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), nil
}

func positivePathID(c *app.RequestContext, name string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(string(c.Param(name))), 10, 64)
	if err != nil || value < 1 {
		return 0
	}
	return value
}

func header(c *app.RequestContext, name string) string {
	return strings.TrimSpace(string(c.Request.Header.Peek(name)))
}

func invalidIdentity(code string, status int, message string) error {
	return &invocationdomain.Error{ReasonCode: code, HTTPStatus: status, Message: message}
}

func writeError(c *app.RequestContext, err error) {
	status := http.StatusInternalServerError
	reason := invocationdomain.Reason(err)
	message := "Provider invocation failed"
	var invocationError *invocationdomain.Error
	if errors.As(err, &invocationError) {
		if invocationError.HTTPStatus > 0 {
			status = invocationError.HTTPStatus
		}
		if invocationError.Message != "" {
			message = invocationError.Message
		}
	}
	c.JSON(status, map[string]any{"error": map[string]any{"reasonCode": reason, "message": message}})
}
