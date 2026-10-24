package mcpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type HandlerConfig struct {
	ProtocolVersion      string
	EndpointPath         string
	AllowedOrigins       []string
	AllowedHosts         []string
	MaxRequestBodyBytes  int64
	IngressConcurrency   int
	GlobalConcurrency    int
	PerApplicationLimit  int
	OutputTextLimitBytes int64
}

func (c HandlerConfig) normalized() (HandlerConfig, error) {
	if c.ProtocolVersion == "" {
		c.ProtocolVersion = ProtocolVersion
	}
	if c.ProtocolVersion != ProtocolVersion {
		return HandlerConfig{}, errors.New("only MCP protocol 2026-07-28 is supported")
	}
	if c.EndpointPath == "" {
		c.EndpointPath = DefaultEndpoint
	}
	if !strings.HasPrefix(c.EndpointPath, "/") || strings.ContainsAny(c.EndpointPath, "?#") {
		return HandlerConfig{}, errors.New("MCP endpoint path must be an absolute path")
	}
	if c.MaxRequestBodyBytes == 0 {
		c.MaxRequestBodyBytes = DefaultMaxBody
	}
	if c.OutputTextLimitBytes == 0 {
		c.OutputTextLimitBytes = DefaultMaxText
	}
	if c.GlobalConcurrency == 0 {
		c.GlobalConcurrency = 64
	}
	if c.IngressConcurrency == 0 {
		c.IngressConcurrency = c.GlobalConcurrency
	}
	if c.PerApplicationLimit == 0 {
		c.PerApplicationLimit = 8
	}
	if c.MaxRequestBodyBytes < 1 || c.OutputTextLimitBytes < 1 || c.IngressConcurrency < 1 || c.GlobalConcurrency < 1 || c.PerApplicationLimit < 1 {
		return HandlerConfig{}, errors.New("MCP body, output, and concurrency limits must be positive")
	}
	if c.MaxRequestBodyBytes > DefaultMaxBody || c.OutputTextLimitBytes > DefaultMaxText {
		return HandlerConfig{}, errors.New("MCP body and output limits cannot exceed 256 KiB")
	}
	if c.PerApplicationLimit > c.GlobalConcurrency {
		return HandlerConfig{}, errors.New("per-application concurrency cannot exceed global concurrency")
	}
	for _, origin := range c.AllowedOrigins {
		if _, err := normalizeOrigin(origin); err != nil {
			return HandlerConfig{}, errors.New("allowed origins must be explicit and cannot contain wildcards")
		}
	}
	if len(c.AllowedHosts) == 0 {
		c.AllowedHosts = []string{"127.0.0.1", "localhost", "::1"}
	}
	for _, host := range c.AllowedHosts {
		if _, _, err := normalizeAllowedHost(host); err != nil {
			return HandlerConfig{}, errors.New("allowed hosts must contain explicit hostnames or host:port values")
		}
	}
	return c, nil
}

type handler struct {
	config         HandlerConfig
	gateway        WorkforceGateway
	logger         *slog.Logger
	sdk            http.Handler
	ingress        chan struct{}
	global         chan struct{}
	applications   *applicationLimiter
	allowedOrigins map[string]struct{}
	allowedHosts   []allowedHost
}

type allowedHost struct {
	host string
	port string
}

func NewHandler(config HandlerConfig, gateway WorkforceGateway, logger *slog.Logger) (http.Handler, error) {
	if gateway == nil {
		return nil, errors.New("Workforce gateway is required")
	}
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "seven-agent-force", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Create and inspect asynchronous Seven Agent Force tasks. Use string IDs exactly as returned.",
	})
	registerTools(server, gateway, normalized.OutputTextLimitBytes)
	sdkHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		MaxRequestBodyBytes:          normalized.MaxRequestBodyBytes,
		PropagateRequestCancellation: true,
		EventStore:                   nil,
	})
	h := &handler{
		config: normalized, gateway: gateway, logger: logger, sdk: sdkHandler,
		ingress:        make(chan struct{}, normalized.IngressConcurrency),
		global:         make(chan struct{}, normalized.GlobalConcurrency),
		applications:   newApplicationLimiter(normalized.PerApplicationLimit),
		allowedOrigins: make(map[string]struct{}, len(normalized.AllowedOrigins)),
	}
	for _, origin := range normalized.AllowedOrigins {
		normalizedOrigin, _ := normalizeOrigin(origin)
		h.allowedOrigins[normalizedOrigin] = struct{}{}
	}
	for _, host := range normalized.AllowedHosts {
		normalizedHost, port, _ := normalizeAllowedHost(host)
		h.allowedHosts = append(h.allowedHosts, allowedHost{host: normalizedHost, port: port})
	}
	var result http.Handler = http.HandlerFunc(h.serveHTTP)
	protection := http.NewCrossOriginProtection()
	for _, origin := range normalized.AllowedOrigins {
		normalizedOrigin, _ := normalizeOrigin(origin)
		if err := protection.AddTrustedOrigin(normalizedOrigin); err != nil {
			return nil, errors.New("invalid trusted MCP origin")
		}
	}
	result = protection.Handler(result)
	return result, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func (h *handler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	recorder := &responseRecorder{ResponseWriter: w}
	if r.URL.Path != h.config.EndpointPath {
		http.NotFound(recorder, r)
		return
	}
	if !h.hostAllowed(r.Host) {
		http.Error(recorder, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		normalizedOrigin, err := normalizeOrigin(origin)
		if err != nil {
			http.Error(recorder, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		if _, ok := h.allowedOrigins[normalizedOrigin]; !ok {
			http.Error(recorder, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
	}
	if r.Method != http.MethodPost {
		recorder.Header().Set("Allow", http.MethodPost)
		http.Error(recorder, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	select {
	case h.ingress <- struct{}{}:
	default:
		writeRateLimited(recorder)
		return
	}
	// Read the bounded raw body before acquiring execution permits or calling
	// the authentication gateway. The HTTP server's ReadTimeout therefore only
	// bounds client upload time; downstream calls cannot consume that budget.
	// A separate ingress permit bounds unauthenticated slow uploads without
	// consuming a business execution permit.
	if r.ContentLength > h.config.MaxRequestBodyBytes {
		<-h.ingress
		recorder.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.config.MaxRequestBodyBytes+1))
	_ = r.Body.Close()
	<-h.ingress
	if err != nil {
		writeJSONRPCError(recorder, http.StatusBadRequest, -32700, "invalid request body", nil)
		return
	}
	if int64(len(body)) > h.config.MaxRequestBodyBytes {
		recorder.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	select {
	case h.global <- struct{}{}:
		defer func() { <-h.global }()
	default:
		writeRateLimited(recorder)
		return
	}
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authorization, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")) == "" {
		writeUnauthorized(recorder, "UNAUTHORIZED")
		return
	}
	capabilities, err := h.gateway.GetCapabilities(r.Context(), authorization)
	if err != nil {
		var gatewayError *GatewayError
		if errors.As(err, &gatewayError) && (gatewayError.Code == "UNAUTHORIZED" || gatewayError.Code == "APPLICATION_DISABLED" || gatewayError.HTTPStatus == http.StatusUnauthorized) {
			code := gatewayError.Code
			if code != "APPLICATION_DISABLED" {
				code = "UNAUTHORIZED"
			}
			writeUnauthorized(recorder, code)
			return
		}
		writeJSONRPCError(recorder, http.StatusServiceUnavailable, -32002, "authentication service unavailable", map[string]any{"code": "OPEN_API_UNAVAILABLE", "retryable": true})
		return
	}
	if capabilities.ApplicationID <= 0 {
		writeJSONRPCError(recorder, http.StatusBadGateway, -32003, "invalid authentication response", map[string]any{"code": "INVALID_OPEN_RESPONSE", "retryable": false})
		return
	}
	release, ok := h.applications.acquire(capabilities.ApplicationID)
	if !ok {
		writeJSONRPCError(recorder, http.StatusTooManyRequests, -32001, "rate limited", map[string]any{"code": "RATE_LIMITED", "retryable": true})
		return
	}
	defer release()
	if r.Header.Get("Mcp-Protocol-Version") != h.config.ProtocolVersion {
		writeJSONRPCError(recorder, http.StatusBadRequest, -32022, "unsupported protocol version", map[string]any{
			"requested": r.Header.Get("Mcp-Protocol-Version"), "supported": []string{h.config.ProtocolVersion},
		})
		return
	}
	r = r.WithContext(withIdentity(r.Context(), requestIdentity{authorization: authorization, capabilities: capabilities}))
	if rpcMethod(body) == "server/discover" {
		filtered := &discoverResponseWriter{target: recorder}
		h.sdk.ServeHTTP(filtered, r)
		filtered.flush(h.config.ProtocolVersion)
	} else {
		h.sdk.ServeHTTP(recorder, r)
	}
	h.logger.Info("mcp request",
		"application_id", strconv.FormatInt(capabilities.ApplicationID, 10),
		"mcp_method", safeLogValue(r.Header.Get("Mcp-Method")),
		"tool_name", safeToolName(r.Header.Get("Mcp-Name")),
		"http_status", recorder.statusCode(),
		"duration_ms", time.Since(started).Milliseconds(),
		"response_size", recorder.bytes,
	)
}

func (h *handler) hostAllowed(requestHost string) bool {
	host, port, err := splitRequestHost(requestHost)
	if err != nil {
		return false
	}
	for _, allowed := range h.allowedHosts {
		if host == allowed.host && (allowed.port == "" || port == allowed.port) {
			return true
		}
	}
	return false
}

func normalizeOrigin(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "*") {
		return "", errors.New("origin is empty or contains a wildcard")
	}
	parsed, err := url.Parse(value)
	scheme := strings.ToLower(parsed.Scheme)
	if err != nil || (scheme != "http" && scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("origin must be an HTTP(S) origin")
	}
	return scheme + "://" + strings.ToLower(parsed.Host), nil
}

func normalizeAllowedHost(value string) (string, string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || strings.ContainsAny(value, "*/?#@") {
		return "", "", errors.New("invalid host")
	}
	return splitRequestHost(value)
}

func splitRequestHost(value string) (string, string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "", "", errors.New("empty host")
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		return strings.Trim(strings.ToLower(host), "[]"), port, nil
	}
	if strings.Count(value, ":") > 1 {
		return strings.Trim(value, "[]"), "", nil
	}
	if strings.Contains(value, ":") {
		return "", "", errors.New("invalid host port")
	}
	return value, "", nil
}

func safeLogValue(value string) string {
	switch value {
	case "server/discover", "tools/list", "tools/call", "ping":
		return value
	default:
		return "unknown"
	}
}

func safeToolName(value string) string {
	switch value {
	case "create_task", "get_task", "list_task_outputs", "read_task_output":
		return value
	default:
		return ""
	}
}

func writeUnauthorized(w http.ResponseWriter, code string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="seven-workforce-mcp"`)
	writeJSONRPCError(w, http.StatusUnauthorized, -32000, "unauthorized", map[string]any{"code": code, "retryable": false})
}

func writeRateLimited(w http.ResponseWriter) {
	writeJSONRPCError(w, http.StatusTooManyRequests, -32001, "rate limited", map[string]any{"code": "RATE_LIMITED", "retryable": true})
}

func rpcMethod(body []byte) string {
	var request struct {
		Method string `json:"method"`
	}
	if json.Unmarshal(body, &request) != nil {
		return ""
	}
	return request.Method
}

// discoverResponseWriter buffers only the small, trusted SDK-generated
// server/discover response so the advertised version set matches this
// endpoint's strict protocol gate. Other MCP responses remain unbuffered.
type discoverResponseWriter struct {
	target http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *discoverResponseWriter) Header() http.Header { return w.target.Header() }

func (w *discoverResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *discoverResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func (w *discoverResponseWriter) flush(protocolVersion string) {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	body := w.body.Bytes()
	if status >= 200 && status < 300 {
		var envelope map[string]json.RawMessage
		var result map[string]json.RawMessage
		if json.Unmarshal(body, &envelope) == nil && json.Unmarshal(envelope["result"], &result) == nil {
			versions, _ := json.Marshal([]string{protocolVersion})
			result["supportedVersions"] = versions
			if encodedResult, err := json.Marshal(result); err == nil {
				envelope["result"] = encodedResult
				if encodedEnvelope, err := json.Marshal(envelope); err == nil {
					body = append(encodedEnvelope, '\n')
				}
			}
		}
	}
	w.target.Header().Del("Content-Length")
	w.target.WriteHeader(status)
	_, _ = w.target.Write(body)
}

func writeJSONRPCError(w http.ResponseWriter, status, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": nil,
		"error": map[string]any{"code": code, "message": message, "data": data},
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += int64(n)
	return n, err
}

func (w *responseRecorder) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }
