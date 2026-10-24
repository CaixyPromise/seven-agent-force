package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPGatewayForwardsAuthorizationAndIdempotency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/open/workforce/v1/projects/77/tasks" || r.Header.Get("Authorization") != "Bearer swk_x.y" || r.Header.Get("Idempotency-Key") != "idem-1" {
			t.Fatalf("request = %s auth=%q idem=%q", r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"code":0,"data":{"schema":"seven.workforce.open-task/v1","taskId":"9007199254740993","taskTurnId":"9007199254740994","projectId":"77","status":"QUEUED"},"message":"ok"}`)
	}))
	defer server.Close()
	gateway, err := NewHTTPGateway(HTTPGatewayConfig{BaseURL: server.URL + "/api/"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := gateway.CreateTask(context.Background(), "Bearer swk_x.y", CreateTaskRequest{IdempotencyKey: "idem-1", ProjectID: 77, Name: "task", Instruction: "work", ExternalCorrelationID: "external"})
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID != 9_007_199_254_740_993 {
		t.Fatalf("task ID = %d", task.TaskID)
	}
}

func TestHTTPGatewayValidatesCapabilitiesAndOutputBounds(t *testing.T) {
	responses := map[string]any{
		"/api/open/workforce/v1/capabilities":    map[string]any{"schema": "wrong", "applicationId": "1", "taskSubmission": map[string]any{"requiresProjectId": true}},
		"/api/open/workforce/v1/tasks/1/outputs": map[string]any{"taskId": "1", "outputs": []any{map[string]any{"outputId": "2", "name": "x", "status": "AVAILABLE", "sizeBytes": "-1"}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": responses[r.URL.Path], "message": "ok"})
	}))
	defer server.Close()
	gateway, _ := NewHTTPGateway(HTTPGatewayConfig{BaseURL: server.URL + "/api/"})
	if _, err := gateway.GetCapabilities(context.Background(), "Bearer swk_x.y"); ErrorCode(err) != "INVALID_OPEN_RESPONSE" {
		t.Fatalf("capabilities error = %v", err)
	}
	if _, err := gateway.ListOutputs(context.Background(), "Bearer swk_x.y", 1); ErrorCode(err) != "INVALID_OPEN_RESPONSE" {
		t.Fatalf("outputs error = %v", err)
	}
}

func TestHTTPGatewayDoesNotFollowRedirectOrExposeRawErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			t.Fatal("redirect was followed")
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()
	gateway, _ := NewHTTPGateway(HTTPGatewayConfig{BaseURL: server.URL + "/api/"})
	_, err := gateway.GetTask(context.Background(), "Bearer secret-that-must-not-leak", 1)
	if err == nil || strings.Contains(err.Error(), "secret-that-must-not-leak") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPGatewayAllowsOnlyStablePublicErrorCodes(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		upstream  string
		want      string
		retryable bool
	}{
		{name: "unknown client error", status: http.StatusBadRequest, upstream: "DATABASE_PASSWORD_EXPOSED", want: "INTERNAL_ERROR"},
		{name: "unknown server error", status: http.StatusServiceUnavailable, upstream: "PRIVATE_BACKEND_FAILURE", want: "OPEN_API_UNAVAILABLE", retryable: true},
		{name: "project not found", status: http.StatusNotFound, upstream: "PROJECT_NOT_FOUND", want: "PROJECT_NOT_FOUND"},
		{name: "project archived", status: http.StatusConflict, upstream: "PROJECT_ARCHIVED", want: "PROJECT_ARCHIVED"},
		{name: "known conflict", status: http.StatusConflict, upstream: "IDEMPOTENCY_CONFLICT", want: "IDEMPOTENCY_CONFLICT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 40900, "data": map[string]any{"reasonCode": test.upstream}, "message": "sensitive backend detail"})
			}))
			defer server.Close()
			gateway, _ := NewHTTPGateway(HTTPGatewayConfig{BaseURL: server.URL + "/api/"})
			_, err := gateway.CreateTask(context.Background(), "Bearer swk_x.y", CreateTaskRequest{IdempotencyKey: "idem"})
			var gatewayError *GatewayError
			if !errors.As(err, &gatewayError) {
				t.Fatalf("error = %v", err)
			}
			if gatewayError.Code != test.want || gatewayError.Retryable != test.retryable {
				t.Fatalf("error = %+v, want code=%s retryable=%v", gatewayError, test.want, test.retryable)
			}
			if (test.upstream != test.want && strings.Contains(err.Error(), test.upstream)) || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("upstream detail leaked: %v", err)
			}
		})
	}
}

func TestHTTPGatewaySelectionQueriesReturnOnlyPublicFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-fixture" {
			t.Fatal("authorization lost")
		}
		switch r.URL.Path {
		case "/api/open/workforce/v1/projects/77/model-options":
			io.WriteString(w, `{"code":0,"data":{"schema":"seven.workforce.open-model-options/v1","projectId":"77","items":[{"credentialId":"9007199254740993","model":"configured-model","protocol":"OPENAI","label":"configured","secret":"must-not-leak"}]}}`)
		case "/api/open/workforce/v1/projects/77/runtime-options":
			if r.URL.Query().Get("modelCredentialId") != "9007199254740993" {
				t.Fatal("credential query lost")
			}
			io.WriteString(w, `{"code":0,"data":{"schema":"seven.workforce.open-runtime-options/v1","projectVersionId":"9007199254740994","items":[{"runtimeReleaseId":"9007199254740995","name":"runtime","recommended":true,"workers":[{"id":"1"}],"imageRef":"must-not-leak","availableSlots":4}]}}`)
		default:
			t.Fatalf("unexpected query %s", r.URL)
		}
	}))
	defer server.Close()
	gateway, err := NewHTTPGateway(HTTPGatewayConfig{BaseURL: server.URL + "/api/"})
	if err != nil {
		t.Fatal(err)
	}
	models, err := gateway.ListModelOptions(context.Background(), "Bearer local-fixture", 77)
	if err != nil {
		t.Fatal(err)
	}
	if len(models.Items) != 1 || models.Items[0].CredentialID != 9007199254740993 {
		t.Fatalf("models=%+v", models)
	}
	runtimes, err := gateway.ListRuntimeOptions(context.Background(), "Bearer local-fixture", 77, 9007199254740993)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimes.Items) != 1 || runtimes.Items[0].RuntimeReleaseID != 9007199254740995 || !runtimes.Items[0].Recommended {
		t.Fatalf("runtimes=%+v", runtimes)
	}
	raw, _ := json.Marshal([]any{models, runtimes})
	if strings.Contains(string(raw), "must-not-leak") || strings.Contains(string(raw), "workers") || strings.Contains(string(raw), "availableSlots") {
		t.Fatalf("private fields leaked: %s", raw)
	}
}
