package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNodeClientStaticRoutingAndSecretHeaderInjection(t *testing.T) {
	serviceClient := &recordingServiceClient{response: successResponse(nodefacade.NodeDescriptor{NodeCode: "node-a", Health: "UP"})}
	client := NewNodeClient(serviceClient, serviceClient, testSecretService{}, nil)
	node := testNode("STATIC")

	err := describeNode(client, node)
	if err != nil {
		t.Fatalf("Describe() error=%v", err)
	}
	request := serviceClient.requests[0]
	if request.ServiceName != node.NodeCode || request.Header.Get("Authorization") != "Bearer management-token" {
		t.Fatalf("request routing/header=%+v", request)
	}
	if request.Header.Get("X-Trace-Id") == "" {
		t.Fatal("trace id not forwarded")
	}
	if !request.TracePropagation {
		t.Fatal("trusted Node call must explicitly enable trace propagation")
	}
}

func TestNodeClientTraceFailureOnlyWarnsAndKeepsBusinessCall(t *testing.T) {
	const traceID = "abababababababababababababababab"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotTraceID := request.Header.Get(xcontext.TraceIDHeader)
		if gotTraceID != traceID {
			t.Errorf("request trace id=%q want %q", gotTraceID, traceID)
		}
		writer.Header().Set(xcontext.TraceIDHeader, gotTraceID)
		_, _ = writer.Write([]byte(`{"code":0,"data":{"nodeCode":"node-a","health":"UP"},"message":"ok","traceId":"` + gotTraceID + `"}`))
	}))
	defer server.Close()
	core, logs := observer.New(zap.WarnLevel)
	client := NewNodeClient(nil, nil, testSecretService{}, zap.New(core))
	client.ConfigureRuntime(nil, microservice.HTTPClientOptions{HTTPClient: server.Client(), Tracer: panicStartTracer{}})
	node := testNode("STATIC")
	node.ManagementBaseURL = server.URL

	var descriptor nodefacade.NodeDescriptor
	if err := client.Do(xcontext.WithTraceID(context.Background(), traceID), node, http.MethodGet, "/internal/node/v1/descriptor", nil, nil, "", &descriptor); err != nil {
		t.Fatalf("trace degradation interrupted node business call: %v", err)
	}
	warnings := logs.FilterMessage("federation_trace_operation_failed").All()
	if len(warnings) != 1 {
		t.Fatalf("trace degradation warnings=%#v", warnings)
	}
	fields := warnings[0].ContextMap()
	if fields["trace_id"] != traceID || fields["operation"] != "client_span_start" {
		t.Fatalf("trace degradation fields=%#v", fields)
	}
}

type panicStartTracer struct{ embedded.Tracer }

func (panicStartTracer) Start(context.Context, string, ...trace.SpanStartOption) (context.Context, trace.Span) {
	panic("trace start must not interrupt business")
}

func TestNodeClientConsulRoutingUsesServiceNameAndEmptyIsAuthoritative(t *testing.T) {
	staticClient := &recordingServiceClient{response: successResponse(nodefacade.NodeDescriptor{Health: "UP"})}
	consulClient := &recordingServiceClient{err: microservice.ErrNoHealthyInstance}
	client := NewNodeClient(staticClient, consulClient, testSecretService{}, nil)
	node := testNode("CONSUL")
	node.ServiceName = "order-node-service"

	err := describeNode(client, node)
	if !errors.Is(err, microservice.ErrNoHealthyInstance) {
		t.Fatalf("Describe() error=%v", err)
	}
	if len(staticClient.requests) != 0 || len(consulClient.requests) != 1 || consulClient.requests[0].ServiceName != node.NodeCode || consulClient.requests[0].TrustScope != microservice.TrustScopeRegistry {
		t.Fatalf("unexpected fallback/routing static=%d consul=%+v", len(staticClient.requests), consulClient.requests)
	}
}

func TestNodeClientPreservesRemoteStatusCodeTraceAndRetryAfter(t *testing.T) {
	const canonicalTraceID = "44444444444444444444444444444444"
	const remoteHeaderTraceID = "55555555555555555555555555555555"
	const remoteBodyTraceID = "66666666666666666666666666666666"
	serviceClient := &recordingServiceClient{response: &microservice.ServiceResponse{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"17"}, xcontext.TraceIDHeader: []string{remoteHeaderTraceID}}, Body: []byte(`{"code":42917,"data":null,"message":"busy","traceId":"` + remoteBodyTraceID + `"}`)}}
	core, logs := observer.New(zap.DebugLevel)
	client := NewNodeClient(serviceClient, serviceClient, testSecretService{}, zap.New(core))

	ctx := xcontext.WithTraceID(context.Background(), canonicalTraceID)
	err := client.Do(ctx, testNode("STATIC"), http.MethodGet, "/internal/node/v1/descriptor", nil, nil, "", nil)
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("error=%T %v", err, err)
	}
	if remoteErr.RemoteStatusCode() != 429 || remoteErr.RemoteCode() != 42917 || remoteErr.RemoteTraceID() != canonicalTraceID || remoteErr.RemoteRetryAfter() != 17 {
		t.Fatalf("remote error=%+v", remoteErr)
	}
	mismatch := logs.FilterMessage("remote_trace_id_mismatch").All()
	if len(mismatch) != 1 {
		t.Fatalf("mismatch logs=%d, want 1", len(mismatch))
	}
	fields := mismatch[0].ContextMap()
	if fields["trace_id"] != canonicalTraceID || fields["remote_header_trace_id"] != remoteHeaderTraceID || fields["remote_body_trace_id"] != remoteBodyTraceID {
		t.Fatalf("mismatch fields=%#v", fields)
	}
}

func TestNodeClientWritesSafeStructuredCompletionLog(t *testing.T) {
	const canonicalTraceID = "77777777777777777777777777777777"
	response := successResponse(nil)
	response.Header.Set(xcontext.TraceIDHeader, canonicalTraceID)
	response.Body = []byte(`{"code":0,"data":null,"message":"ok","traceId":"` + canonicalTraceID + `"}`)
	response.InstanceID = "node-a-2"
	transport := &recordingServiceClient{response: response}
	core, logs := observer.New(zap.DebugLevel)
	client := NewNodeClient(transport, transport, testSecretService{}, zap.New(core))
	ctx := xcontext.WithTraceID(context.Background(), canonicalTraceID)

	err := client.Do(ctx, testNode("STATIC"), http.MethodPut, "/internal/node/v1/users/42/status", url.Values{"token": {"query-canary"}}, map[string]string{"password": "body-canary"}, "cmd-42", nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := logs.FilterMessage("federation_node_call_completed").All()
	if len(entries) != 1 {
		t.Fatalf("completion logs=%d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["trace_id"] != canonicalTraceID || fields["target_node_code"] != "node-a" || fields["target_instance_id"] != "node-a-2" || fields["request_route"] != "/internal/node/v1/users/42/status" || fields["result"] != "success" {
		t.Fatalf("completion fields=%#v", fields)
	}
	encoded, _ := json.Marshal(fields)
	for _, forbidden := range []string{"management-token", "query-canary", "body-canary", "Authorization"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("completion log leaked %q: %s", forbidden, encoded)
		}
	}
	if _, ok := fields["latency_ms"].(int64); !ok {
		t.Fatalf("latency_ms missing: %#v", fields)
	}
}

func TestNodeClientRejectsIncoherentHTTPEnvelopeMatrix(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "non-2xx success code", status: http.StatusInternalServerError, body: `{"code":0,"message":"not really success","traceId":"remote"}`},
		{name: "2xx business error", status: http.StatusOK, body: `{"code":40917,"message":"conflict","traceId":"remote"}`},
		{name: "redirect", status: http.StatusFound, body: `{"code":0,"message":"redirect","traceId":"remote"}`},
		{name: "empty no-content", status: http.StatusNoContent, body: ``},
		{name: "malformed", status: http.StatusBadGateway, body: `not-json`},
		{name: "invalid status class", status: 99, body: `{"code":0,"message":"invalid","traceId":"remote"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &recordingServiceClient{response: &microservice.ServiceResponse{StatusCode: test.status, Header: http.Header{}, Body: []byte(test.body)}}
			client := NewNodeClient(transport, transport, testSecretService{}, nil)
			err := describeNode(client, testNode("STATIC"))
			var remoteErr *RemoteError
			if !errors.As(err, &remoteErr) {
				t.Fatalf("error=%T %v", err, err)
			}
			if remoteErr.RemoteStatusCode() != http.StatusServiceUnavailable || remoteErr.RemoteCode() != apperrors.CodeServiceUnavailable {
				t.Fatalf("status/code=%d/%d want 503/%d", remoteErr.RemoteStatusCode(), remoteErr.RemoteCode(), apperrors.CodeServiceUnavailable)
			}
		})
	}
}

func TestNodeClientRemoteErrorCannotReflectOutboundSecrets(t *testing.T) {
	const oidcSecret = "oidc-client-secret-plaintext"
	const managementBearer = "management-token"
	serviceClient := &recordingServiceClient{response: &microservice.ServiceResponse{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{},
		Body:       []byte(`{"code":50201,"data":null,"message":"bad ` + oidcSecret + `","traceId":"trace-` + managementBearer + `"}`),
	}}
	client := NewNodeClient(serviceClient, serviceClient, testSecretService{}, nil)
	body := struct {
		ClientSecret string `json:"clientSecret"`
	}{ClientSecret: oidcSecret}

	err := client.Do(context.Background(), testNode("STATIC"), http.MethodPut, "/internal/node/v1/hub-connection", nil, body, "connect-v1", nil)
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("error=%T %v", err, err)
	}
	for _, exposed := range []string{err.Error(), remoteErr.RemoteMessage(), remoteErr.RemoteTraceID()} {
		if strings.Contains(exposed, oidcSecret) || strings.Contains(exposed, managementBearer) {
			t.Fatalf("remote error reflected outbound secret: %q", exposed)
		}
	}
}

func TestNodeClientSanitizesSuccessDataAndBoundsRemoteEnvelope(t *testing.T) {
	const oidcSecret = "oidc-client-secret-plaintext"
	serviceClient := &recordingServiceClient{response: &microservice.ServiceResponse{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       []byte(`{"code":0,"data":{"displayName":"` + oidcSecret + ` management-token"},"message":"` + strings.Repeat("m", 700) + `","traceId":"invalid trace management-token"}`),
	}}
	client := NewNodeClient(serviceClient, serviceClient, testSecretService{}, nil)
	var out map[string]string
	err := client.Do(context.Background(), testNode("STATIC"), http.MethodPut, "/internal/node/v1/hub-connection", nil, map[string]string{"clientSecret": oidcSecret}, "connect-v1", &out)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), oidcSecret) || strings.Contains(string(encoded), "management-token") {
		t.Fatalf("success payload reflected outbound secret: %s", encoded)
	}
	if got := sanitizeRemoteTraceID("invalid trace management-token", []string{"management-token"}); got != "" {
		t.Fatalf("invalid remote trace preserved: %q", got)
	}
	if got := sanitizeRemoteText(strings.Repeat("m", 700), nil, 512); len(got) != 512 {
		t.Fatalf("remote message length=%d want 512", len(got))
	}
}

func TestConcurrentRequestsBindEndpointCredentialAndBodyToOneTargetSnapshot(t *testing.T) {
	transport := &snapshotRecordingServiceClient{ready: make(chan struct{})}
	client := NewNodeClient(transport, transport, targetSecretService{}, nil)
	targets := []NodeTarget{
		{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: "http://127.0.0.1:18081", ManagementBearer: EncryptedValue{Ciphertext: "bearer-a"}},
		{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: "http://127.0.0.1:18082", ManagementBearer: EncryptedValue{Ciphertext: "bearer-b"}},
	}
	errs := make(chan error, len(targets))
	for index, target := range targets {
		go func(target NodeTarget, payload string) {
			errs <- client.Do(context.Background(), target, http.MethodPut, "/internal/node/v1/hub-connection", nil, map[string]string{"payload": payload}, "connect-"+payload, nil)
		}(target, []string{"a", "b"}[index])
	}
	for range targets {
		if err := <-errs; err != nil {
			t.Fatalf("Do() error=%v", err)
		}
	}

	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.requests) != 2 {
		t.Fatalf("requests=%d want 2", len(transport.requests))
	}
	for _, request := range transport.requests {
		if len(request.ResolvedInstances) != 1 {
			t.Fatalf("request has no immutable target snapshot: %+v", request)
		}
		port := request.ResolvedInstances[0].Port
		bearer := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		var body map[string]string
		if err := json.Unmarshal(request.Body, &body); err != nil {
			t.Fatal(err)
		}
		wantSuffix := "a"
		if port == 18082 {
			wantSuffix = "b"
		}
		if bearer != "bearer-"+wantSuffix || body["payload"] != wantSuffix {
			t.Fatalf("cross-routed request port=%d bearer=%q body=%v", port, bearer, body)
		}
	}
}

func TestConcurrentSameNodeSnapshotsReachOnlyTheirOwnEndpoints(t *testing.T) {
	type receivedRequest struct {
		endpoint string
		bearer   string
		payload  string
	}
	var mu sync.Mutex
	received := make([]receivedRequest, 0, 2)
	allArrived := make(chan struct{})
	newEndpoint := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]string
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			received = append(received, receivedRequest{endpoint: name, bearer: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), payload: payload["payload"]})
			if len(received) == 2 {
				close(allArrived)
			}
			mu.Unlock()
			<-allArrived
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":null,"message":"ok","traceId":"remote-trace"}`))
		}))
	}
	first := newEndpoint("a")
	defer first.Close()
	second := newEndpoint("b")
	defer second.Close()

	client := NewNodeClient(nil, nil, targetSecretService{}, nil)
	client.ConfigureRuntime(nil, microservice.HTTPClientOptions{HTTPClient: first.Client()})
	targets := []NodeTarget{
		{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: first.URL, ManagementBearer: EncryptedValue{Ciphertext: "bearer-a"}},
		{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: second.URL, ManagementBearer: EncryptedValue{Ciphertext: "bearer-b"}},
	}
	errs := make(chan error, 2)
	for index, target := range targets {
		go func(target NodeTarget, payload string) {
			errs <- client.Do(context.Background(), target, http.MethodPut, "/internal/node/v1/hub-connection", nil, map[string]string{"payload": payload}, "connect-"+payload, nil)
		}(target, []string{"a", "b"}[index])
	}
	for range targets {
		if err := <-errs; err != nil {
			t.Fatalf("Do() error=%v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("received=%d want 2", len(received))
	}
	for _, request := range received {
		if request.bearer != "bearer-"+request.endpoint || request.payload != request.endpoint {
			t.Fatalf("endpoint cross-route: %+v", request)
		}
	}
}

func TestNodeClientDecodesDecimalStringPagination(t *testing.T) {
	serviceClient := &recordingServiceClient{response: &microservice.ServiceResponse{StatusCode: 200, Header: http.Header{}, Body: []byte(`{"code":0,"message":"ok","traceId":"remote","data":{"current":"1","size":"20","total":"9007199254740993","records":[]}}`)}}
	client := NewNodeClient(serviceClient, serviceClient, testSecretService{}, nil)

	var page struct {
		Total DecimalInt64 `json:"total"`
	}
	err := client.Do(context.Background(), testNode("STATIC"), http.MethodGet, "/internal/node/v1/users", url.Values{"current": {"1"}, "size": {"20"}}, nil, "", &page)
	if err != nil {
		t.Fatalf("ListUsers() error=%v", err)
	}
	if int64(page.Total) != 9007199254740993 {
		t.Fatalf("total=%d", page.Total)
	}
}

func TestNodeClientReplaySafeWritePreservesBodyAndKey(t *testing.T) {
	serviceClient := &recordingServiceClient{response: successResponse(nodefacade.CommandResult{ChangedCount: 1})}
	client := NewNodeClient(serviceClient, serviceClient, testSecretService{}, nil)
	command := nodefacade.SetUserStatusCommand{UserID: "42", Status: 1, Reason: "security", IdempotencyKey: "cmd-42"}

	if err := setNodeUserStatus(client, testNode("STATIC"), command); err != nil {
		t.Fatalf("SetUserStatus() error=%v", err)
	}
	request := serviceClient.requests[0]
	if !request.ReplaySafe || request.Header.Get("Idempotency-Key") != "cmd-42" {
		t.Fatalf("request=%+v", request)
	}
	var body map[string]any
	if err := json.Unmarshal(request.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, exists := body["idempotencyKey"]; exists {
		t.Fatal("idempotency key leaked into body")
	}
	if want := map[string]any{"reason": "security", "status": float64(1)}; !reflect.DeepEqual(body, want) {
		t.Fatalf("body=%v want=%v", body, want)
	}
}

func TestNodeClientReplaySafeWriteUsesAtMostTwoInstancesAndExactRequest(t *testing.T) {
	type capturedRequest struct {
		method, path, key, body string
	}
	var mu sync.Mutex
	requests := make([]capturedRequest, 0, 2)
	servers := make([]*httptest.Server, 0, 2)
	instances := make([]microservice.ServiceInstance, 0, 2)
	for i := 0; i < 2; i++ {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			requests = append(requests, capturedRequest{method: r.Method, path: r.URL.Path, key: r.Header.Get("Idempotency-Key"), body: string(body)})
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":50301,"message":"unavailable","traceId":"remote"}`))
		}))
		servers = append(servers, server)
		t.Cleanup(server.Close)
		parsed, _ := url.Parse(server.URL)
		instances = append(instances, microservice.ServiceInstance{ID: string(rune('a' + i)), ServiceName: "node-a", Host: parsed.Hostname(), Port: mustPort(t, parsed.Port()), Scheme: parsed.Scheme, Healthy: true})
	}
	client := NewNodeClient(nil, nil, testSecretService{}, nil)
	client.ConfigureRuntime(fixedResolver{instances: instances}, microservice.HTTPClientOptions{HTTPClient: servers[0].Client()})
	command := nodefacade.SetUserStatusCommand{UserID: "42", Status: 1, Reason: "security", IdempotencyKey: "cmd-42"}
	node := testNode("CONSUL")
	node.ServiceName = "node-a-service"
	if err := setNodeUserStatus(client, node, command); err == nil {
		t.Fatal("expected final 503 error")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("sends=%d want 2", len(requests))
	}
	if !reflect.DeepEqual(requests[0], requests[1]) {
		t.Fatalf("retry changed request: %#v", requests)
	}
}

type fixedResolver struct {
	instances []microservice.ServiceInstance
}

func (r fixedResolver) Resolve(context.Context, string) ([]microservice.ServiceInstance, error) {
	return append([]microservice.ServiceInstance(nil), r.instances...), nil
}

func mustPort(t *testing.T, raw string) int {
	t.Helper()
	port, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func testNode(discovery string) NodeTarget {
	return NodeTarget{NodeCode: "node-a", DiscoveryType: discovery, ManagementBaseURL: "https://node.example.com:9443", ManagementBearer: EncryptedValue{Ciphertext: "cipher", EDEK: "edek", WrapKeyRef: "key"}}
}

func describeNode(client *NodeClient, node NodeTarget) error {
	var out nodefacade.NodeDescriptor
	return client.Do(context.Background(), node, http.MethodGet, "/internal/node/v1/descriptor", nil, nil, "", &out)
}

func setNodeUserStatus(client *NodeClient, node NodeTarget, command nodefacade.SetUserStatusCommand) error {
	body := struct {
		Status int    `json:"status"`
		Reason string `json:"reason"`
	}{command.Status, command.Reason}
	return client.Do(context.Background(), node, http.MethodPut, "/internal/node/v1/users/"+url.PathEscape(command.UserID)+"/status", nil, body, command.IdempotencyKey, nil)
}

func successResponse(data any) *microservice.ServiceResponse {
	payload, _ := json.Marshal(map[string]any{"code": 0, "message": "ok", "traceId": "remote-trace", "data": data})
	return &microservice.ServiceResponse{StatusCode: 200, Header: http.Header{}, Body: payload}
}

type recordingServiceClient struct {
	response *microservice.ServiceResponse
	err      error
	requests []microservice.ServiceRequest
}

type snapshotRecordingServiceClient struct {
	mu       sync.Mutex
	ready    chan struct{}
	requests []microservice.ServiceRequest
}

func (c *snapshotRecordingServiceClient) Do(_ context.Context, request microservice.ServiceRequest) (*microservice.ServiceResponse, error) {
	c.mu.Lock()
	c.requests = append(c.requests, request)
	if len(c.requests) == 2 {
		close(c.ready)
	}
	c.mu.Unlock()
	<-c.ready
	return successResponse(nil), nil
}

func (c *recordingServiceClient) Do(_ context.Context, request microservice.ServiceRequest) (*microservice.ServiceResponse, error) {
	c.requests = append(c.requests, request)
	return c.response, c.err
}

type testSecretService struct{}

func (testSecretService) EncryptString(context.Context, string) (secretvalueinfra.SecretValue, error) {
	panic("not used")
}
func (testSecretService) DecryptString(context.Context, secretvalueinfra.SecretValue) (string, error) {
	return "management-token", nil
}
func (testSecretService) EncryptBytes(context.Context, []byte) (secretvalueinfra.SecretValue, error) {
	panic("not used")
}
func (testSecretService) DecryptBytes(context.Context, secretvalueinfra.SecretValue) ([]byte, error) {
	panic("not used")
}

type targetSecretService struct{ testSecretService }

func (targetSecretService) DecryptString(_ context.Context, value secretvalueinfra.SecretValue) (string, error) {
	return value.CiphertextB64, nil
}
