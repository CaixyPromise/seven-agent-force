package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHandlerUsesPure20260728DiscoverAndSixTools(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	var sessionHeaders []string
	gateway := testGateway()
	handler, err := NewHandler(HandlerConfig{}, gateway, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Header.Get("Mcp-Method"))
		mu.Unlock()
		handler.ServeHTTP(w, r)
		mu.Lock()
		sessionHeaders = append(sessionHeaders, w.Header().Get("Mcp-Session-Id"))
		mu.Unlock()
	}))
	defer server.Close()

	session := connectClient(t, server.URL, "Bearer swk_locator.secret")
	defer session.Close()
	if got := session.InitializeResult().ProtocolVersion; got != ProtocolVersion {
		t.Fatalf("protocol = %q, want %q", got, ProtocolVersion)
	}
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 6 {
		t.Fatalf("tools = %d, want 6", len(listed.Tools))
	}
	want := map[string]bool{"list_model_options": true, "list_runtime_options": true, "create_task": true, "get_task": true, "list_task_outputs": true, "read_task_output": true}
	for _, tool := range listed.Tools {
		delete(want, tool.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing tools: %v", want)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(methods) == 0 || methods[0] != "server/discover" {
		t.Fatalf("first method = %q, want server/discover", methods)
	}
	for _, method := range methods {
		if method == "initialize" {
			t.Fatal("client fell back to legacy initialize")
		}
	}
	for _, sessionID := range sessionHeaders {
		if sessionID != "" {
			t.Fatalf("stateless response set Mcp-Session-Id %q", sessionID)
		}
	}
}

func TestRawDiscoverAdvertisesOnly20260728(t *testing.T) {
	handler, err := NewHandler(HandlerConfig{}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"raw-test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}`
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	setMCPHeaders(request)
	request.Header.Set("Mcp-Method", "server/discover")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("discover status/body = %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Result struct {
			SupportedVersions []string `json:"supportedVersions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Result.SupportedVersions) != 1 || envelope.Result.SupportedVersions[0] != ProtocolVersion {
		t.Fatalf("supportedVersions = %v, want [%s]", envelope.Result.SupportedVersions, ProtocolVersion)
	}
}

func TestHandlerRejectsLegacyInitializeAndProtocolMismatch(t *testing.T) {
	handler, err := NewHandler(HandlerConfig{}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(legacy))
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer swk_x.y")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "-32022") {
		t.Fatalf("status/body = %d %s", response.Code, response.Body.String())
	}

	modernInitialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}`
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(modernInitialize))
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer swk_x.y")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", ProtocolVersion)
	req.Header.Set("Mcp-Method", "initialize")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "-32601") {
		t.Fatalf("2026 initialize status/body = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerAuthMethodOriginAndSessionlessTransport(t *testing.T) {
	handler, err := NewHandler(HandlerConfig{AllowedOrigins: []string{"https://client.example"}}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/mcp", nil)
		req.Host = "localhost"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "POST" {
			t.Fatalf("%s status/allow = %d %q", method, response.Code, response.Header().Get("Allow"))
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Host = "localhost"
	req.Header.Set("Mcp-Protocol-Version", ProtocolVersion)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthorized status/header = %d %q", response.Code, response.Header().Get("WWW-Authenticate"))
	}

	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Host = "evil.example"
	req.Header.Set("Origin", "https://evil.example")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross origin status = %d, want 403", response.Code)
	}

	trustedHandler, err := NewHandler(HandlerConfig{AllowedOrigins: []string{"HTTPS://CLIENT.EXAMPLE"}, AllowedHosts: []string{"workforce.example"}}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Host = "workforce.example"
	req.Header.Set("Origin", "https://client.example")
	req.Header.Set("Authorization", "Bearer swk_x.y")
	req.Header.Set("Mcp-Protocol-Version", ProtocolVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response = httptest.NewRecorder()
	trustedHandler.ServeHTTP(response, req)
	if response.Code == http.StatusForbidden {
		t.Fatalf("trusted origin and host were rejected: %s", response.Body.String())
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	session := connectClient(t, server.URL, "Bearer swk_x.y")
	defer session.Close()
	if session.InitializeResult().ProtocolVersion != ProtocolVersion {
		t.Fatal("discover did not negotiate 2026")
	}
}

func TestHandlerPreservesDisabledCodeAndEnforcesBodyLimit(t *testing.T) {
	gateway := testGateway()
	gateway.GetCapabilitiesFunc = func(context.Context, string) (Capabilities, error) {
		return Capabilities{}, &GatewayError{Code: "APPLICATION_DISABLED", HTTPStatus: http.StatusUnauthorized}
	}
	handler, err := NewHandler(HandlerConfig{MaxRequestBodyBytes: 64}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer swk_x.y")
	req.Header.Set("Mcp-Protocol-Version", ProtocolVersion)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "APPLICATION_DISABLED") {
		t.Fatalf("disabled response = %d %s", response.Code, response.Body.String())
	}

	handler, err = NewHandler(HandlerConfig{MaxRequestBodyBytes: 64}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(strings.Repeat("x", 65)))
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer swk_x.y")
	req.Header.Set("Mcp-Protocol-Version", ProtocolVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerRejectsHeaderBodyProtocolMismatch(t *testing.T) {
	handler, err := NewHandler(HandlerConfig{}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25","io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer swk_x.y")
	req.Header.Set("Mcp-Protocol-Version", ProtocolVersion)
	req.Header.Set("Mcp-Method", "tools/list")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "-32020") {
		t.Fatalf("mismatch response = %d %s", response.Code, response.Body.String())
	}
}

func TestApplicationLimiterRemovesIdleEntries(t *testing.T) {
	limiter := newApplicationLimiter(1)
	release, ok := limiter.acquire(101)
	if !ok {
		t.Fatal("first acquire failed")
	}
	if _, ok := limiter.acquire(101); ok {
		t.Fatal("second acquire unexpectedly succeeded")
	}
	if limiter.size() != 1 {
		t.Fatalf("size while active = %d", limiter.size())
	}
	release()
	if limiter.size() != 0 {
		t.Fatalf("idle limiter was retained: %d", limiter.size())
	}
}

func TestHandlerLogsDoNotContainAuthorizationOrToolPayload(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	gateway := testGateway()
	handler, err := NewHandler(HandlerConfig{}, gateway, logger)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	secret := "swk_locator.super-secret"
	session := connectClient(t, server.URL, "Bearer "+secret)
	defer session.Close()
	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: map[string]any{
		"idempotencyKey": "idem", "projectId": "77", "name": "task",
		"modelSelection": map[string]any{"credentialId": "88", "model": "model"}, "runtimeReleaseId": "99",
		"instruction": "private instruction", "externalCorrelationId": "external",
	}})
	if err != nil {
		t.Fatal(err)
	}
	logged := logs.String()
	for _, forbidden := range []string{secret, "private instruction", "Authorization", "swk_"} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("logs contain forbidden value %q: %s", forbidden, logged)
		}
	}
}

func TestCancelledToolRequestReleasesApplicationPermit(t *testing.T) {
	gateway := testGateway()
	var calls atomic.Int32
	firstExited := make(chan struct{})
	gateway.CreateTaskFunc = func(ctx context.Context, _ string, _ CreateTaskRequest) (Task, error) {
		if calls.Add(1) == 1 {
			<-ctx.Done()
			close(firstExited)
			return Task{}, ctx.Err()
		}
		return Task{TaskID: 11, TaskTurnID: 12, Status: "QUEUED"}, nil
	}
	handler, err := NewHandler(HandlerConfig{GlobalConcurrency: 1, PerApplicationLimit: 1}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := connectClient(t, server.URL, "Bearer swk_x.y")
	defer session.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "create_task", Arguments: validCreateArguments("first")})
	}()
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("first tool request did not reach gateway")
	}
	cancel()
	select {
	case <-firstExited:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled request did not release gateway")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled MCP call did not finish")
	}
	secondSession := connectClient(t, server.URL, "Bearer swk_x.y")
	defer secondSession.Close()
	result, err := secondSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: validCreateArguments("second")})
	if err != nil || result.IsError {
		t.Fatalf("second call did not acquire released permit: result=%+v err=%v", result, err)
	}
}

func TestToolFailureRejectsArbitraryGatewayCode(t *testing.T) {
	result := gatewayToolFailure(&GatewayError{Code: "PRIVATE_SQL_FAILURE", Message: "raw database detail", Retryable: true})
	raw, _ := json.Marshal(result.StructuredContent)
	if !result.IsError || !strings.Contains(string(raw), "INTERNAL_ERROR") || strings.Contains(string(raw), "PRIVATE_SQL_FAILURE") || strings.Contains(string(raw), "database") {
		t.Fatalf("unsafe tool error: %s", raw)
	}
}

func TestSlowBodyDoesNotOccupyExecutionPermit(t *testing.T) {
	handler, err := NewHandler(HandlerConfig{IngressConcurrency: 2, GlobalConcurrency: 1, PerApplicationLimit: 1}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	releaseBody := make(chan struct{})
	slowRequest := httptest.NewRequest(http.MethodPost, "/mcp", &gatedReader{release: releaseBody, content: []byte(`{}`)})
	setMCPHeaders(slowRequest)
	slowResponse := httptest.NewRecorder()
	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		handler.ServeHTTP(slowResponse, slowRequest)
	}()
	time.Sleep(20 * time.Millisecond)
	fastRequest := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	setMCPHeaders(fastRequest)
	fastResponse := httptest.NewRecorder()
	handler.ServeHTTP(fastResponse, fastRequest)
	if fastResponse.Code == http.StatusTooManyRequests {
		t.Fatal("slow request body occupied the global execution permit")
	}
	close(releaseBody)
	select {
	case <-slowDone:
	case <-time.After(2 * time.Second):
		t.Fatal("slow request did not finish after body release")
	}
}

func TestIngressLimiterBoundsSlowBodiesAndReleases(t *testing.T) {
	handler, err := NewHandler(HandlerConfig{IngressConcurrency: 1, GlobalConcurrency: 2, PerApplicationLimit: 1}, testGateway(), nil)
	if err != nil {
		t.Fatal(err)
	}
	releaseBody := make(chan struct{})
	started := make(chan struct{})
	firstRequest := httptest.NewRequest(http.MethodPost, "/mcp", &gatedReader{release: releaseBody, content: []byte(`{}`), started: started})
	setMCPHeaders(firstRequest)
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.ServeHTTP(httptest.NewRecorder(), firstRequest)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not enter body read")
	}

	var secondReads atomic.Int32
	secondRequest := httptest.NewRequest(http.MethodPost, "/mcp", &countingReader{reads: &secondReads, content: []byte(`{}`)})
	setMCPHeaders(secondRequest)
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusTooManyRequests || !strings.Contains(secondResponse.Body.String(), "RATE_LIMITED") {
		t.Fatalf("second status/body = %d %s", secondResponse.Code, secondResponse.Body.String())
	}
	if secondReads.Load() != 0 {
		t.Fatalf("rejected request body was read %d times", secondReads.Load())
	}

	close(releaseBody)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not release ingress permit")
	}
	thirdRequest := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	setMCPHeaders(thirdRequest)
	thirdResponse := httptest.NewRecorder()
	handler.ServeHTTP(thirdResponse, thirdRequest)
	if thirdResponse.Code == http.StatusTooManyRequests {
		t.Fatal("completed body read did not release ingress permit")
	}
}

func TestBoundedSlowGatewayCompletesAfterBodyIsBuffered(t *testing.T) {
	gateway := testGateway()
	gateway.GetCapabilitiesFunc = func(ctx context.Context, _ string) (Capabilities, error) {
		select {
		case <-time.After(20 * time.Millisecond):
			return validCapabilities(101), nil
		case <-ctx.Done():
			return Capabilities{}, ctx.Err()
		}
	}
	gateway.CreateTaskFunc = func(ctx context.Context, _ string, _ CreateTaskRequest) (Task, error) {
		select {
		case <-time.After(20 * time.Millisecond):
			return Task{TaskID: 11, TaskTurnID: 12, Status: "QUEUED"}, nil
		case <-ctx.Done():
			return Task{}, ctx.Err()
		}
	}
	handler, err := NewHandler(HandlerConfig{}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := connectClient(t, server.URL, "Bearer swk_x.y")
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_task", Arguments: validCreateArguments("slow")})
	if err != nil || result.IsError {
		t.Fatalf("bounded slow gateway request failed: result=%+v err=%v", result, err)
	}
}

func TestConcurrentApplicationsDoNotCrossBearerContexts(t *testing.T) {
	gateway := testGateway()
	gateway.GetCapabilitiesFunc = func(_ context.Context, authorization string) (Capabilities, error) {
		applicationID := int64(101)
		if strings.Contains(authorization, "key-b") {
			applicationID = 202
		}
		return validCapabilities(applicationID), nil
	}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	type pair struct{ authorization, name string }
	pairs := make(chan pair, 2)
	gateway.CreateTaskFunc = func(_ context.Context, authorization string, request CreateTaskRequest) (Task, error) {
		pairs <- pair{authorization: authorization, name: request.Name}
		started <- struct{}{}
		<-release
		return Task{TaskID: 11, TaskTurnID: 12, Status: "QUEUED"}, nil
	}
	handler, err := NewHandler(HandlerConfig{GlobalConcurrency: 2, PerApplicationLimit: 1}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	sessionA := connectClient(t, server.URL, "Bearer key-a")
	defer sessionA.Close()
	sessionB := connectClient(t, server.URL, "Bearer key-b")
	defer sessionB.Close()
	results := make(chan error, 2)
	go func() {
		result, err := sessionA.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: namedCreateArguments("app-a")})
		if err == nil && result.IsError {
			err = errors.New("application A tool error")
		}
		results <- err
	}()
	go func() {
		result, err := sessionB.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: namedCreateArguments("app-b")})
		if err == nil && result.IsError {
			err = errors.New("application B tool error")
		}
		results <- err
	}()
	<-started
	<-started
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	got := map[pair]bool{<-pairs: true, <-pairs: true}
	if !got[pair{"Bearer key-a", "app-a"}] || !got[pair{"Bearer key-b", "app-b"}] {
		t.Fatalf("bearer/request contexts crossed: %#v", got)
	}
}

func TestCurrentAndGraceKeysShareApplicationLimit(t *testing.T) {
	gateway := testGateway()
	gateway.GetCapabilitiesFunc = func(context.Context, string) (Capabilities, error) {
		return validCapabilities(101), nil
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	gateway.CreateTaskFunc = func(_ context.Context, _ string, _ CreateTaskRequest) (Task, error) {
		if calls.Add(1) > 1 {
			return Task{}, errors.New("second key bypassed application limiter")
		}
		close(started)
		<-release
		return Task{TaskID: 11, TaskTurnID: 12, Status: "QUEUED"}, nil
	}
	handler, err := NewHandler(HandlerConfig{GlobalConcurrency: 2, PerApplicationLimit: 1}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	current := connectClient(t, server.URL, "Bearer current-key")
	defer current.Close()
	grace := connectClient(t, server.URL, "Bearer grace-key")
	defer grace.Close()
	firstDone := make(chan error, 1)
	go func() {
		_, err := current.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: validCreateArguments("current")})
		firstDone <- err
	}()
	<-started
	_, secondErr := grace.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: validCreateArguments("grace")})
	close(release)
	firstErr := <-firstDone
	if secondErr == nil || (!strings.Contains(strings.ToLower(secondErr.Error()), "rate limited") && !strings.Contains(strings.ToLower(secondErr.Error()), "too many requests")) {
		t.Fatalf("grace key did not share current key limiter: %v", secondErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("gateway calls = %d, want 1", calls.Load())
	}
	if firstErr != nil {
		t.Fatal(firstErr)
	}
}

func TestToolsForwardIdentityAndVerifyTextOutput(t *testing.T) {
	content := []byte("verified output\n")
	sum := sha256.Sum256(content)
	var gotAuthorization, gotIdempotency string
	gateway := testGateway()
	gateway.CreateTaskFunc = func(_ context.Context, authorization string, request CreateTaskRequest) (Task, error) {
		gotAuthorization, gotIdempotency = authorization, request.IdempotencyKey
		replayed := false
		return Task{TaskID: 9_007_199_254_740_993, TaskTurnID: 9_007_199_254_740_994, Status: "QUEUED", Replayed: &replayed}, nil
	}
	gateway.ListOutputsFunc = func(context.Context, string, int64) ([]Output, error) {
		return []Output{{OutputID: 9_007_199_254_740_995, Name: "result", Status: "AVAILABLE", MediaType: "text/markdown", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))}}, nil
	}
	gateway.OpenOutputFunc = func(context.Context, string, int64, int64) (OutputStream, error) {
		return OutputStream{Content: io.NopCloser(bytes.NewReader(content))}, nil
	}
	handler, err := NewHandler(HandlerConfig{}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := connectClient(t, server.URL, "Bearer swk_locator.secret")
	defer session.Close()

	created, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: map[string]any{
		"idempotencyKey": "issue-1", "projectId": "77", "name": "task",
		"modelSelection": map[string]any{"credentialId": "88", "model": "model"}, "runtimeReleaseId": "99",
		"instruction": "do work", "externalCorrelationId": "external-1",
	}})
	if err != nil || created.IsError {
		t.Fatalf("create result/error = %+v %v", created, err)
	}
	if gotAuthorization != "Bearer swk_locator.secret" || gotIdempotency != "issue-1" {
		t.Fatalf("forwarded auth/idempotency = %q %q", gotAuthorization, gotIdempotency)
	}
	raw, _ := json.Marshal(created.StructuredContent)
	if !strings.Contains(string(raw), `"taskId":"9007199254740993"`) {
		t.Fatalf("task ID was not a JSON string: %s", raw)
	}

	read, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "read_task_output", Arguments: map[string]any{
		"taskId": "9007199254740993", "outputId": "9007199254740995",
	}})
	if err != nil || read.IsError {
		t.Fatalf("read result/error = %+v %v", read, err)
	}
	if len(read.Content) != 1 || read.Content[0].(*mcp.TextContent).Text != string(content) {
		t.Fatalf("read content = %#v", read.Content)
	}
	raw, _ = json.Marshal(read.StructuredContent)
	if strings.Contains(string(raw), string(content)) || !strings.Contains(string(raw), `"verified":true`) {
		t.Fatalf("structured output duplicated or missed verification: %s", raw)
	}
}

func TestReadOutputFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		output   Output
		content  []byte
		wantCode string
	}{
		{name: "binary", output: Output{OutputID: 2, Status: "AVAILABLE", MediaType: "application/zip", SizeBytes: 1, SHA256: strings.Repeat("0", 64)}, content: []byte("x"), wantCode: "OUTPUT_MEDIA_TYPE_UNSUPPORTED"},
		{name: "too large", output: Output{OutputID: 2, Status: "AVAILABLE", MediaType: "text/plain", SizeBytes: 5, SHA256: strings.Repeat("0", 64)}, content: []byte("12345"), wantCode: "OUTPUT_TOO_LARGE"},
		{name: "bad hash", output: Output{OutputID: 2, Status: "AVAILABLE", MediaType: "text/plain", SizeBytes: 1, SHA256: strings.Repeat("0", 64)}, content: []byte("x"), wantCode: "OUTPUT_INTEGRITY_FAILED"},
		{name: "invalid utf8", output: outputFor([]byte{0xff}), content: []byte{0xff}, wantCode: "OUTPUT_INVALID_TEXT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gateway := testGateway()
			gateway.ListOutputsFunc = func(context.Context, string, int64) ([]Output, error) { return []Output{test.output}, nil }
			gateway.OpenOutputFunc = func(context.Context, string, int64, int64) (OutputStream, error) {
				return OutputStream{Content: io.NopCloser(bytes.NewReader(test.content))}, nil
			}
			handler, err := NewHandler(HandlerConfig{OutputTextLimitBytes: 4}, gateway, nil)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			session := connectClient(t, server.URL, "Bearer swk_x.y")
			defer session.Close()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "read_task_output", Arguments: map[string]any{"taskId": "1", "outputId": "2"}})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(result.StructuredContent)
			if !result.IsError || !strings.Contains(string(raw), test.wantCode) {
				t.Fatalf("result = %+v structured=%s, want %s", result, raw, test.wantCode)
			}
		})
	}
}

func testGateway() *FakeGateway {
	return &FakeGateway{
		GetCapabilitiesFunc: func(context.Context, string) (Capabilities, error) {
			return validCapabilities(101), nil
		},
		CreateTaskFunc:  func(context.Context, string, CreateTaskRequest) (Task, error) { return Task{}, nil },
		GetTaskFunc:     func(context.Context, string, int64) (Task, error) { return Task{}, nil },
		ListOutputsFunc: func(context.Context, string, int64) ([]Output, error) { return []Output{}, nil },
		OpenOutputFunc:  func(context.Context, string, int64, int64) (OutputStream, error) { return OutputStream{}, nil },
	}
}

func connectClient(t *testing.T, endpoint, authorization string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	httpClient := &http.Client{Transport: authRoundTripper{authorization: authorization, base: http.DefaultTransport}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: strings.TrimRight(endpoint, "/") + "/mcp", HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

type authRoundTripper struct {
	authorization string
	base          http.RoundTripper
}

func (r authRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", r.authorization)
	return r.base.RoundTrip(clone)
}

func outputFor(content []byte) Output {
	sum := sha256.Sum256(content)
	return Output{OutputID: 2, Status: "AVAILABLE", MediaType: "text/plain", SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}
}

func validCreateArguments(suffix string) map[string]any {
	return map[string]any{
		"idempotencyKey": "idem-" + suffix, "projectId": "77", "name": "task",
		"modelSelection": map[string]any{"credentialId": "88", "model": "model"}, "runtimeReleaseId": "99",
		"instruction": "do work", "externalCorrelationId": "external-" + suffix,
	}
}

func validCapabilities(applicationID int64) Capabilities {
	return Capabilities{
		Schema: "seven.workforce.open-capabilities/v1", ApplicationID: applicationID,
		TaskSubmission: TaskSubmissionCapability{RequiresProjectID: true, SupportsOutputs: true},
	}
}

func namedCreateArguments(name string) map[string]any {
	arguments := validCreateArguments(name)
	arguments["name"] = name
	return arguments
}

func setMCPHeaders(request *http.Request) {
	request.Host = "localhost"
	request.Header.Set("Authorization", "Bearer swk_x.y")
	request.Header.Set("Mcp-Protocol-Version", ProtocolVersion)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
}

type gatedReader struct {
	release <-chan struct{}
	content []byte
	started chan struct{}
	once    sync.Once
	done    bool
}

func (r *gatedReader) Read(buffer []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	if r.started != nil {
		r.once.Do(func() { close(r.started) })
	}
	<-r.release
	r.done = true
	return copy(buffer, r.content), io.EOF
}

type countingReader struct {
	reads   *atomic.Int32
	content []byte
	done    bool
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	r.reads.Add(1)
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(buffer, r.content), io.EOF
}

func TestCreateTaskRejectsMissingSelectionsIllegalIDsAndSecrets(t *testing.T) {
	gateway := testGateway()
	gateway.CreateTaskFunc = func(context.Context, string, CreateTaskRequest) (Task, error) {
		t.Fatal("invalid input reached gateway")
		return Task{}, nil
	}
	handler, err := NewHandler(HandlerConfig{}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := connectClient(t, server.URL, "Bearer local-fixture")
	defer session.Close()
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing model selection", func(a map[string]any) { delete(a, "modelSelection") }},
		{"missing runtime", func(a map[string]any) { delete(a, "runtimeReleaseId") }},
		{"missing idempotency", func(a map[string]any) { delete(a, "idempotencyKey") }},
		{"numeric credential", func(a map[string]any) { a["modelSelection"] = map[string]any{"credentialId": 88, "model": "model"} }},
		{"credential overflow", func(a map[string]any) {
			a["modelSelection"] = map[string]any{"credentialId": "9223372036854775808", "model": "model"}
		}},
		{"leading zero credential", func(a map[string]any) { a["modelSelection"] = map[string]any{"credentialId": "088", "model": "model"} }},
		{"runtime overflow", func(a map[string]any) { a["runtimeReleaseId"] = "9223372036854775808" }},
		{"zero runtime", func(a map[string]any) { a["runtimeReleaseId"] = "0" }},
		{"raw provider secret", func(a map[string]any) {
			a["modelSelection"] = map[string]any{"credentialId": "88", "model": "model", "apiKey": "fixture-secret"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := validCreateArguments("invalid")
			tc.change(a)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: a})
			if err == nil && !result.IsError {
				t.Fatalf("accepted invalid input: %+v", result)
			}
		})
	}
}

func TestSelectionToolsForwardIdentityAndRejectInvalidIDs(t *testing.T) {
	gateway := testGateway()
	modelCalls, runtimeCalls := 0, 0
	gateway.ListModelOptionsFunc = func(_ context.Context, key string, id int64) (ModelOptions, error) {
		modelCalls++
		if key != "Bearer local-fixture" || id != 77 {
			t.Fatal("wrong identity/project")
		}
		return ModelOptions{ProjectID: id, Items: []ModelOption{{CredentialID: 9007199254740993, Model: "configured-model"}}}, nil
	}
	gateway.ListRuntimeOptionsFunc = func(_ context.Context, key string, id, credential int64) (RuntimeOptions, error) {
		runtimeCalls++
		if key != "Bearer local-fixture" || id != 77 || credential != 9007199254740993 {
			t.Fatal("wrong runtime query")
		}
		return RuntimeOptions{ProjectVersionID: 88, Items: []RuntimeOption{{RuntimeReleaseID: 99}}}, nil
	}
	handler, err := NewHandler(HandlerConfig{}, gateway, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := connectClient(t, server.URL, "Bearer local-fixture")
	defer session.Close()
	for _, input := range []struct {
		tool string
		args map[string]any
	}{
		{"list_model_options", map[string]any{"projectId": "77"}},
		{"list_runtime_options", map[string]any{"projectId": "77", "modelCredentialId": "9007199254740993"}},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: input.tool, Arguments: input.args})
		if err != nil || result.IsError {
			t.Fatalf("query: %+v %v", result, err)
		}
	}
	for _, id := range []string{"0", "077", "9223372036854775808"} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_runtime_options", Arguments: map[string]any{"projectId": "77", "modelCredentialId": id}})
		if err == nil && !result.IsError {
			t.Fatal("invalid credential accepted")
		}
	}
	if modelCalls != 1 || runtimeCalls != 1 {
		t.Fatalf("calls=%d/%d", modelCalls, runtimeCalls)
	}
}
