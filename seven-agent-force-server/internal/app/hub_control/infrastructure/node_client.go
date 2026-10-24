package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	sharedlogger "github.com/CaixyPromise/seven-agent/force/internal/shared/logger"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type NodeClient struct {
	static, consul microservice.ServiceClient
	secrets        secretvalueinfra.Service
	logger         *zap.Logger
	options        microservice.HTTPClientOptions
	consulResolver microservice.ServiceResolver
	ownedClient    *microservice.HTTPServiceClient
}

// NodeTarget contains only transport routing and credential material.
type NodeTarget struct {
	NodeCode          string
	DiscoveryType     string
	ServiceName       string
	ManagementBaseURL string
	ManagementBearer  EncryptedValue
}

func NewNodeClient(staticClient, consulClient microservice.ServiceClient, secrets secretvalueinfra.Service, logger *zap.Logger) *NodeClient {
	return &NodeClient{static: staticClient, consul: consulClient, secrets: secrets, logger: logger}
}
func (c *NodeClient) SetHTTPOptions(options microservice.HTTPClientOptions) { c.options = options }

func (c *NodeClient) ConfigureRuntime(consulResolver microservice.ServiceResolver, options microservice.HTTPClientOptions) {
	c.options = options
	shared := microservice.NewHTTPServiceClient(nil, microservice.NewRoundRobin(), options)
	c.static = shared
	c.consul = shared
	c.consulResolver = consulResolver
	c.ownedClient = shared
}

func (c *NodeClient) CloseIdleConnections() {
	if c != nil && c.ownedClient != nil {
		c.ownedClient.CloseIdleConnections()
	}
}

type RemoteError struct {
	statusCode        int
	code              int
	message           string
	traceID           string
	retryAfterSeconds int
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("remote node returned HTTP %d code %d: %s", e.statusCode, e.code, e.message)
}
func (e *RemoteError) RemoteStatusCode() int { return e.statusCode }
func (e *RemoteError) RemoteCode() int       { return e.code }
func (e *RemoteError) RemoteMessage() string { return e.message }
func (e *RemoteError) RemoteRetryAfter() int { return e.retryAfterSeconds }
func (e *RemoteError) RemoteTraceID() string { return e.traceID }

type TransportError struct {
	Cause     error
	ambiguous bool
}

func (e *TransportError) Error() string   { return "remote node transport failed" }
func (e *TransportError) Unwrap() error   { return e.Cause }
func (e *TransportError) Ambiguous() bool { return e.ambiguous }

type envelope struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	TraceID string          `json:"traceId"`
}

var remoteTraceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type DecimalInt64 int64

func (v *DecimalInt64) UnmarshalJSON(data []byte) error {
	var text string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	} else {
		text = string(data)
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	*v = DecimalInt64(parsed)
	return nil
}

// Do sends one Node request; HTTPServiceClient owns the only alternate-instance retry.
func (c *NodeClient) Do(ctx context.Context, node NodeTarget, method, path string, query url.Values, body any, key string, out any) (resultErr error) {
	ctx, traceID := canonicalNodeCallContext(ctx)
	startedAt := time.Now()
	targetInstanceID := ""
	requestRoute := path
	if parsedRoute, parseErr := url.Parse(path); parseErr == nil {
		requestRoute = parsedRoute.Path
	}
	defer func() {
		fields := []zap.Field{
			zap.String("target_node_code", node.NodeCode),
			zap.String("target_service_name", targetServiceName(node)),
			zap.String("target_instance_id", targetInstanceID),
			zap.String("discovery_type", node.DiscoveryType),
			zap.String("request_method", method),
			zap.String("request_route", requestRoute),
			zap.Int64("latency_ms", time.Since(startedAt).Milliseconds()),
			zap.String("result", nodeCallResult(resultErr)),
		}
		sharedlogger.WithContext(ctx, c.logger).Info("federation_node_call_completed", fields...)
	}()
	client, serviceName, instances, err := c.clientFor(ctx, node)
	if err != nil {
		return err
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode node request: %w", err)
		}
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	plain, err := c.secrets.DecryptString(ctx, secretvalueinfra.SecretValue{CiphertextB64: node.ManagementBearer.Ciphertext, EDEKB64: node.ManagementBearer.EDEK, WrapKeyRef: node.ManagementBearer.WrapKeyRef})
	if err != nil {
		return fmt.Errorf("decrypt node management credential: %w", err)
	}
	header := make(http.Header)
	header.Set("Accept", "application/json")
	header.Set("Authorization", "Bearer "+plain)
	header.Set("X-Trace-Id", traceID)
	if body != nil {
		header.Set("Content-Type", "application/json")
	}
	if key != "" {
		header.Set("Idempotency-Key", key)
	}
	trustScope := microservice.TrustScopeDefault
	if node.DiscoveryType == "CONSUL" {
		trustScope = microservice.TrustScopeRegistry
	}
	response, err := client.Do(ctx, microservice.ServiceRequest{
		ServiceName:       serviceName,
		ResolvedInstances: instances,
		Method:            method,
		Path:              path,
		Header:            header,
		Body:              payload,
		ReplaySafe:        key != "",
		TracePropagation:  true,
		TraceWarning: func(operation string) {
			sharedlogger.WithContext(ctx, c.logger).Warn("federation_trace_operation_failed",
				zap.String("operation", operation),
			)
		},
		MaxResponseBytes: 4 << 20,
		TrustScope:       trustScope,
	})
	if err != nil {
		plain = ""
		return &TransportError{Cause: err, ambiguous: key != "" && !errors.Is(err, microservice.ErrNoHealthyInstance)}
	}
	targetInstanceID = response.InstanceID
	outboundSecrets := collectOutboundSecrets(payload, plain)
	remoteHeaderTraceID := sanitizeRemoteTraceID(response.Header.Get(xcontext.TraceIDHeader), outboundSecrets)
	var env envelope
	if decodeErr := json.Unmarshal(response.Body, &env); decodeErr != nil {
		plain = ""
		logRemoteTraceMismatch(ctx, c.logger, traceID, remoteHeaderTraceID, "")
		return unusableRemoteProtocol("Node响应格式错误", traceID)
	}
	plain = ""
	env.Message = sanitizeRemoteText(env.Message, outboundSecrets, 512)
	env.TraceID = sanitizeRemoteTraceID(env.TraceID, outboundSecrets)
	if remoteHeaderTraceID != traceID || env.TraceID != traceID || remoteHeaderTraceID != env.TraceID {
		logRemoteTraceMismatch(ctx, c.logger, traceID, remoteHeaderTraceID, env.TraceID)
	}
	if response.StatusCode < 100 || response.StatusCode > 599 || (response.StatusCode >= 300 && response.StatusCode < 400) {
		return unusableRemoteProtocol("Node响应协议不可用", traceID)
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && env.Code != 0 {
		return unusableRemoteProtocol("Node响应状态与业务码不一致", traceID)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if env.Code == 0 {
			return unusableRemoteProtocol("Node响应状态与业务码不一致", traceID)
		}
		return &RemoteError{statusCode: response.StatusCode, code: env.Code, message: env.Message, traceID: traceID, retryAfterSeconds: boundedRetryAfter(response.Header.Get("Retry-After"))}
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	safeData, err := sanitizeRemoteJSON(env.Data, outboundSecrets)
	if err != nil {
		return &RemoteError{statusCode: response.StatusCode, code: apperrors.CodeSystemError, message: "Node响应数据格式错误", traceID: traceID}
	}
	if err := json.Unmarshal(safeData, out); err != nil {
		return &RemoteError{statusCode: response.StatusCode, code: apperrors.CodeSystemError, message: "Node响应数据格式错误", traceID: traceID}
	}
	return nil
}

func unusableRemoteProtocol(message, traceID string) *RemoteError {
	return &RemoteError{statusCode: http.StatusServiceUnavailable, code: apperrors.CodeServiceUnavailable, message: message, traceID: traceID}
}

func collectOutboundSecrets(payload []byte, managementBearer string) []string {
	values := make([]string, 0, 4)
	if managementBearer != "" {
		values = append(values, managementBearer)
	}
	if len(payload) == 0 {
		return values
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return values
	}
	collectSensitiveValues(decoded, "", &values)
	return values
}

func collectSensitiveValues(value any, field string, values *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			collectSensitiveValues(child, key, values)
		}
	case []any:
		for _, child := range typed {
			collectSensitiveValues(child, field, values)
		}
	case string:
		if typed != "" && isOutboundSecretField(field) {
			*values = append(*values, typed)
		}
	}
}

func isOutboundSecretField(field string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(field), "_", ""), "-", ""))
	for _, marker := range []string{"secret", "token", "bearer", "password", "credential", "authorization"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func sanitizeRemoteText(value string, secrets []string, limit int) string {
	safe := value
	for _, secret := range secrets {
		if secret != "" {
			safe = strings.ReplaceAll(safe, secret, "[REDACTED]")
		}
	}
	if limit > 0 && len(safe) > limit {
		safe = safe[:limit]
	}
	return safe
}

func sanitizeRemoteTraceID(value string, secrets []string) string {
	safe := strings.TrimSpace(sanitizeRemoteText(value, secrets, 128))
	if !remoteTraceIDPattern.MatchString(safe) || !xcontext.IsCanonicalTraceID(safe) {
		return ""
	}
	return safe
}

func canonicalNodeCallContext(ctx context.Context) (context.Context, string) {
	if spanContext := oteltrace.SpanContextFromContext(ctx); spanContext.IsValid() {
		traceID := spanContext.TraceID().String()
		return xcontext.WithTraceID(ctx, traceID), traceID
	}
	return xcontext.EnsureContextTraceID(ctx)
}

func targetServiceName(node NodeTarget) string {
	if node.DiscoveryType == "CONSUL" && strings.TrimSpace(node.ServiceName) != "" {
		return strings.TrimSpace(node.ServiceName)
	}
	return strings.TrimSpace(node.NodeCode)
}

func nodeCallResult(err error) string {
	if err == nil {
		return "success"
	}
	var transport *TransportError
	if errors.As(err, &transport) {
		return "transport_error"
	}
	var remote *RemoteError
	if errors.As(err, &remote) {
		return "remote_error"
	}
	return "local_error"
}

func logRemoteTraceMismatch(ctx context.Context, log *zap.Logger, expected, headerTraceID, bodyTraceID string) {
	sharedlogger.WithContext(ctx, log).Warn("remote_trace_id_mismatch",
		zap.String("remote_header_trace_id", headerTraceID),
		zap.String("remote_body_trace_id", bodyTraceID),
	)
}

func sanitizeRemoteJSON(raw []byte, secrets []string) ([]byte, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	sanitized := sanitizeRemoteValue(decoded, secrets)
	return json.Marshal(sanitized)
}

func sanitizeRemoteValue(value any, secrets []string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = sanitizeRemoteValue(child, secrets)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = sanitizeRemoteValue(child, secrets)
		}
		return result
	case string:
		return sanitizeRemoteText(typed, secrets, 0)
	default:
		return value
	}
}

func (c *NodeClient) clientFor(ctx context.Context, node NodeTarget) (microservice.ServiceClient, string, []microservice.ServiceInstance, error) {
	switch node.DiscoveryType {
	case "STATIC":
		instance, err := microservice.ParseServiceURL(node.NodeCode, node.ManagementBaseURL)
		if err != nil {
			return nil, "", nil, err
		}
		if c.static != nil {
			return c.static, node.NodeCode, []microservice.ServiceInstance{instance}, nil
		}
		return microservice.NewHTTPServiceClient(nil, microservice.NewRoundRobin(), c.options), node.NodeCode, []microservice.ServiceInstance{instance}, nil
	case "CONSUL":
		if c.consul == nil {
			return nil, "", nil, microservice.ErrInvalidDependency
		}
		if c.consulResolver != nil {
			instances, err := c.consulResolver.Resolve(ctx, node.ServiceName)
			if err != nil {
				return nil, "", nil, err
			}
			return c.consul, node.NodeCode, append([]microservice.ServiceInstance(nil), instances...), nil
		}
		return c.consul, node.NodeCode, nil, nil
	default:
		return nil, "", nil, microservice.ErrInvalidRequest
	}
}
func boundedRetryAfter(value string) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 0 {
		return 0
	}
	if seconds > 3600 {
		return 3600
	}
	return seconds
}
