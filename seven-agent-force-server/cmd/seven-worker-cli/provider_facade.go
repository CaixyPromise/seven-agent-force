package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const providerFacadeMaxRequestBytes = 128 << 20

type providerFacade struct {
	server   *http.Server
	listener net.Listener
}

func (f *providerFacade) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return f.server.Shutdown(ctx)
}

func startProviderFacade(ctx context.Context, spec *providerFacadeSpec, runningConfirmed <-chan struct{}) (*providerFacade, error) {
	if spec == nil {
		return nil, nil
	}
	if err := probeProviderProxy(ctx, spec.ProxyBaseURL); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", spec.Listen)
	if err != nil {
		return nil, err
	}
	server := &http.Server{
		Handler:           newProviderFacadeHandler(*spec, nil, runningConfirmed),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	facade := &providerFacade{server: server, listener: listener}
	go func() {
		<-ctx.Done()
		_ = facade.Close()
	}()
	go func() { _ = server.Serve(listener) }()
	return facade, nil
}

func probeProviderProxy(ctx context.Context, baseURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return errors.New("Provider Proxy endpoint is invalid")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(probeCtx, "tcp", parsed.Host)
	if err != nil {
		return errors.New("Provider Proxy is unavailable")
	}
	return connection.Close()
}

type providerFacadeHandler struct {
	spec             providerFacadeSpec
	client           *http.Client
	runningConfirmed <-chan struct{}
}

func newProviderFacadeHandler(spec providerFacadeSpec, client *http.Client, runningConfirmed ...<-chan struct{}) http.Handler {
	if client == nil {
		client = &http.Client{}
	}
	var gate <-chan struct{}
	if len(runningConfirmed) > 0 {
		gate = runningConfirmed[0]
	}
	return &providerFacadeHandler{spec: spec, client: client, runningConfirmed: gate}
}

func (h *providerFacadeHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeProviderFacadeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	suffix, ok := providerFacadeSuffix(h.spec.Protocol, request.URL.Path)
	if !ok {
		writeProviderFacadeError(response, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if h.runningConfirmed != nil {
		select {
		case <-h.runningConfirmed:
		case <-request.Context().Done():
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, providerFacadeMaxRequestBytes+1))
	if err != nil || len(body) > providerFacadeMaxRequestBytes {
		writeProviderFacadeError(response, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return
	}
	var envelope struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		writeProviderFacadeError(response, http.StatusBadRequest, "INVALID_JSON")
		return
	}
	if strings.TrimSpace(envelope.Model) == "" || len(strings.TrimSpace(envelope.Model)) > 256 {
		writeProviderFacadeError(response, http.StatusBadRequest, "MODEL_REQUEST_MODEL_INVALID")
		return
	}
	proxyRequest, err := http.NewRequestWithContext(request.Context(), http.MethodPost, strings.TrimRight(h.spec.ProxyBaseURL, "/")+suffix, bytes.NewReader(body))
	if err != nil {
		writeProviderFacadeError(response, http.StatusBadGateway, "PROXY_REQUEST_FAILED")
		return
	}
	proxyRequest.Header.Set("Content-Type", "application/json")
	proxyRequest.Header.Set("Accept", request.Header.Get("Accept"))
	proxyRequest.Header.Set("anthropic-version", request.Header.Get("anthropic-version"))
	proxyRequest.Header.Set("anthropic-beta", request.Header.Get("anthropic-beta"))
	proxyResponse, err := h.client.Do(proxyRequest)
	if err != nil {
		writeProviderFacadeError(response, http.StatusBadGateway, "PROVIDER_PROXY_UNAVAILABLE")
		return
	}
	defer proxyResponse.Body.Close()
	copyProviderFacadeHeaders(response.Header(), proxyResponse.Header)
	response.WriteHeader(proxyResponse.StatusCode)
	_, _ = io.Copy(response, proxyResponse.Body)
}

func providerFacadeSuffix(protocol, path string) (string, bool) {
	switch strings.TrimRight(path, "/") {
	case "/v1/responses":
		return "/v1/responses", protocol == "OPENAI"
	case "/v1/responses/compact":
		return "/v1/responses/compact", protocol == "OPENAI"
	case "/v1/messages":
		return "/v1/messages", protocol == "ANTHROPIC"
	case "/v1/messages/count_tokens":
		return "/v1/messages/count_tokens", protocol == "ANTHROPIC"
	default:
		return "", false
	}
}

func copyProviderFacadeHeaders(target, source http.Header) {
	for key, values := range source {
		lower := strings.ToLower(key)
		if lower != "content-type" && lower != "x-request-id" && lower != "retry-after" && lower != "x-seven-invocation-id" && !strings.HasPrefix(lower, "x-ratelimit-") && !strings.HasPrefix(lower, "openai-") && !strings.HasPrefix(lower, "anthropic-") {
			continue
		}
		for _, value := range values {
			target.Add(key, value)
		}
	}
	target.Set("Cache-Control", "no-store")
}

func writeProviderFacadeError(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]any{"error": map[string]any{"type": "seven_provider_facade_error", "code": code}})
}
