package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/mcpserver"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	openapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	resultfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
)

type failingDownloadReader struct {
	served bool
	closed bool
}

func (r *failingDownloadReader) Read(buffer []byte) (int, error) {
	if !r.served {
		r.served = true
		return copy(buffer, "partial-output"), nil
	}
	return 0, errors.New("private object key must not be logged")
}

func (r *failingDownloadReader) Close() error {
	r.closed = true
	return nil
}

func TestDecodeStrictOpenTaskRequest(t *testing.T) {
	valid := []byte(`{"name":"n","instruction":"i","tags":["bugfix"],"referenceAssetIds":["801"],"deliveryPreference":"PROJECT_DEFAULT","externalCorrelationId":"c"}`)
	var request domain.CreateOpenTaskRequest
	if err := decodeStrict(valid, &request); err != nil {
		t.Fatal(err)
	}
	if request.ProjectID != 0 || len(request.ReferenceAssetIDs) != 1 || request.ReferenceAssetIDs[0] != 801 {
		t.Fatalf("decimal-string identities were not decoded: %+v", request)
	}
	for _, invalid := range [][]byte{
		[]byte(`{"instruction":"i","externalCorrelationId":"c","runtimeReleaseId":"0901","modelSelection":{"credentialId":"801","model":"m"}}`),
		[]byte(`{"instruction":"i","externalCorrelationId":"c","runtimeReleaseId":"901","modelSelection":{"credentialId":"+801","model":"m"}}`),
		[]byte(`{"instruction":"i","externalCorrelationId":"c","runtimeReleaseId":901,"modelSelection":{"credentialId":"801","model":"m"}}`),
		[]byte(`{"instruction":"i","externalCorrelationId":"c","runtimeReleaseId":"9223372036854775808","modelSelection":{"credentialId":"801","model":"m"}}`),
		[]byte(`{"instruction":"i","externalCorrelationId":"c","runtimeReleaseId":"901","modelSelection":{"credentialId":"801","model":"m","apiKey":"fixture-secret"}}`),
		[]byte(`{"scenarioKey":"mvp-empty-workspace","name":"n","instruction":"i","externalCorrelationId":"c"}`),
		[]byte(`{"projectId":"701","name":"n","instruction":"i","externalCorrelationId":"c"}`),
		[]byte(`{"projectId":"701","name":"n","instruction":"i","externalCorrelationId":"c","providerAlias":"x"}`),
		[]byte(`{"projectId":"701","instruction":"i","externalCorrelationId":"c","harnessAlias":"x"}`),
		[]byte(`{"projectId":"701","instruction":"i","externalCorrelationId":"c"} {}`),
		[]byte(`[]`),
	} {
		var target domain.CreateOpenTaskRequest
		if err := decodeStrict(invalid, &target); err == nil {
			t.Fatalf("decodeStrict accepted %s", invalid)
		}
	}
}

func TestSafeDownloadNamePreventsHeaderInjectionAndPathTraversal(t *testing.T) {
	longName := strings.Repeat("界", 200)
	for _, test := range []struct {
		input string
		want  string
	}{
		{"../secret/result.md", "result.md"},
		{`..\secret\result.md`, "result.md"},
		{"bad\r\nname\x00\"'.md", "badname.md"},
		{"", "output"},
		{"..", "output"},
	} {
		if got := safeDownloadName(test.input); got != test.want {
			t.Fatalf("safeDownloadName(%q)=%q, want %q", test.input, got, test.want)
		}
	}
	if got := safeDownloadName(longName); len([]rune(got)) != 160 {
		t.Fatalf("long name rune count=%d, want 160", len([]rune(got)))
	}
}

func TestSafeMediaType(t *testing.T) {
	if got := safeMediaType("text/markdown; charset=utf-8"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("valid media type changed: %q", got)
	}
	for _, invalid := range []string{"", "not a media type", "text/plain\r\nX-Evil: yes"} {
		if got := safeMediaType(invalid); got != "application/octet-stream" {
			t.Fatalf("safeMediaType(%q)=%q", invalid, got)
		}
	}
}

func TestPositiveOpenIDPreservesSnowflakeRange(t *testing.T) {
	const raw = "9007199254740993"
	if got, ok := positiveOpenID(raw); !ok || got != 9_007_199_254_740_993 {
		t.Fatalf("positiveOpenID(%q)=(%d,%v)", raw, got, ok)
	}
	for _, invalid := range []string{"", "0", "-1", "+1", "01", " 1 ", "1.5", "9223372036854775808"} {
		if _, ok := positiveOpenID(invalid); ok {
			t.Fatalf("positiveOpenID accepted %q", invalid)
		}
	}
}

func TestOpenTaskContentTypeIsStrictJSON(t *testing.T) {
	for _, valid := range []string{"application/json", "application/json; charset=utf-8", "Application/JSON"} {
		if !isJSONContentType(valid) {
			t.Fatalf("expected valid JSON content type: %q", valid)
		}
	}
	for _, invalid := range []string{"", "text/plain", "application/problem+json", "application/json; charset="} {
		if isJSONContentType(invalid) {
			t.Fatalf("expected invalid JSON content type: %q", invalid)
		}
	}
}

func TestClassifiedDownloadStreamReportsSafeCategoryWithoutAppendingJSON(t *testing.T) {
	reader := &failingDownloadReader{}
	var categories []string
	stream := newClassifiedDownloadStream(reader, 101, 202, func(category string, taskID, outputID int64) {
		if taskID != 101 || outputID != 202 {
			t.Fatalf("unexpected stream identities: task=%d output=%d", taskID, outputID)
		}
		categories = append(categories, category)
	})
	content, err := io.ReadAll(stream)
	if err == nil {
		t.Fatal("expected post-start reader error")
	}
	if string(content) != "partial-output" || strings.Contains(string(content), "open-error") || strings.Contains(string(content), "private object key") {
		t.Fatalf("stream appended an error body: %q", content)
	}
	if len(categories) != 1 || categories[0] != "object_read_error" {
		t.Fatalf("unsafe or duplicate categories: %#v", categories)
	}
	if closeErr := stream.Close(); closeErr != nil || !reader.closed {
		t.Fatalf("stream close err=%v closed=%v", closeErr, reader.closed)
	}
}

func TestOpenOutputErrorsKeepStableOpenErrorEnvelope(t *testing.T) {
	for _, test := range []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{resultfacade.ErrOutputNotFound, http.StatusNotFound, "OUTPUT_NOT_FOUND"},
		{resultfacade.ErrOutputUnavailable, http.StatusServiceUnavailable, "OUTPUT_UNAVAILABLE"},
	} {
		c := &app.RequestContext{}
		writeOpenError(c, test.err)
		if c.Response.StatusCode() != test.wantStatus {
			t.Fatalf("status=%d, want %d", c.Response.StatusCode(), test.wantStatus)
		}
		var body struct {
			Code int `json:"code"`
			Data struct {
				ReasonCode string `json:"reasonCode"`
			} `json:"data"`
		}
		if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code == 0 || body.Data.ReasonCode != test.wantCode {
			t.Fatalf("unexpected error envelope: %+v", body)
		}
	}
}

func TestOneTimeCredentialResponseIsNoStoreAndPreservesInt64Wire(t *testing.T) {
	c := &app.RequestContext{}
	writeOneTimeSecret(c, openapp.APIKeyRotation{
		ApplicationID:        9_007_199_254_740_993,
		CredentialID:         9_007_199_254_740_995,
		CredentialVersion:    2,
		APIKey:               "swk_fixture.secret",
		CredentialHint:       "secret",
		PreviousCredentialID: 9_007_199_254_740_997,
		PreviousValidUntil:   time.Date(2026, 8, 30, 12, 15, 0, 0, time.UTC),
		CreatedAt:            time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	})
	if got := string(c.Response.Header.Peek("Cache-Control")); got != "no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
	if got := string(c.Response.Header.Peek("Pragma")); got != "no-cache" {
		t.Fatalf("Pragma=%q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["applicationId"] != "9007199254740993" || data["credentialId"] != "9007199254740995" || data["previousCredentialId"] != "9007199254740997" {
		t.Fatalf("credential response lost int64 wire contract: %#v", body)
	}
}

func TestMCPOnlyCreateResponseOmitsSigningSecretMaterial(t *testing.T) {
	c := &app.RequestContext{}
	writeOneTimeSecret(c, openapp.CreatedApplication{
		Application: domain.Application{
			ID: 9_007_199_254_740_993, Name: "Internal Codex", WebhookURL: "", Status: domain.ApplicationEnabled,
			APICredential: domain.CredentialSummary{ID: 9_007_199_254_740_995, Version: 1, Hint: "abcd1234"},
		},
		APIKey: "swk_fixture.secret", APICredentialID: 9_007_199_254_740_995, APICredentialVersion: 1,
	})
	var body map[string]any
	if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	data := body["data"].(map[string]any)
	for _, field := range []string{"signingSecret", "signingCredentialId", "signingCredentialVersion"} {
		if _, exists := data[field]; exists {
			t.Fatalf("MCP-only creation exposed %s: %#v", field, data)
		}
	}
	if data["apiKey"] != "swk_fixture.secret" || data["apiCredentialId"] != "9007199254740995" {
		t.Fatalf("MCP-only API credential missing: %#v", data)
	}
}

func TestCredentialRotationRejectsAnyRequestBodyBeforeMutation(t *testing.T) {
	for _, invoke := range []func(context.Context, *app.RequestContext){
		New(nil).rotateAPIKey,
		New(nil).rotateSigningSecret,
	} {
		c := &app.RequestContext{}
		c.Request.SetBody([]byte(`{}`))
		invoke(context.Background(), c)
		if c.Response.StatusCode() != http.StatusBadRequest {
			t.Fatalf("rotation body status=%d", c.Response.StatusCode())
		}
	}
}

func TestSigningRotationWithoutWebhookReturnsStableConflictReason(t *testing.T) {
	c := &app.RequestContext{}
	writeRotationError(c, openinfra.ErrWebhookNotConfigured)
	if c.Response.StatusCode() != http.StatusConflict || !strings.Contains(string(c.Response.Body()), "WEBHOOK_NOT_CONFIGURED") {
		t.Fatalf("unexpected webhook-not-configured response: status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
}

// The captured gateway bytes pass the same decoder and domain validator as
// createOpenTask; a synthetic success cannot hide a missing selection field.
func TestMCPGatewayBodyPassesRESTValidation(t *testing.T) {
	for _, tc := range []struct {
		name                string
		credential, runtime int64
		model               string
		valid               bool
	}{
		{"valid", 9007199254740993, 9007199254740994, "model", true},
		{"missing credential", 0, 99, "model", false}, {"missing runtime", 88, 0, "model", false},
		{"missing model", 88, 99, "", false}, {"negative credential", -1, 99, "model", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/open/workforce/v1/capabilities" {
					capability := openapp.NewService(nil, nil, nil, nil, nil, "").GetOpenCapabilities(r.Context(), domain.Identity{ApplicationID: 1})
					json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": capability})
					return
				}
				if r.URL.Path != "/api/open/workforce/v1/projects/77/tasks" || r.Header.Get("Authorization") != "Bearer local-fixture" {
					t.Fatalf("wrong REST identity/path: %s", r.URL)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				var request domain.CreateOpenTaskRequest
				if err = decodeStrict(body, &request); err != nil {
					if tc.valid {
						t.Fatalf("REST decode: %v", err)
					}
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, `{"code":40000,"data":{"reasonCode":"INVALID_REQUEST"}}`)
					return
				}
				request.ProjectID = 77
				normalized, err := request.NormalizeAndValidate()
				if (err == nil) != tc.valid {
					t.Fatalf("validation error=%v valid=%v", err, tc.valid)
				}
				if r.Header.Get("Idempotency-Key") != "idem" {
					t.Fatal("lost idempotency header")
				}
				if tc.valid && (normalized.ModelSelection.CredentialID != tc.credential || normalized.RuntimeReleaseID != tc.runtime || normalized.ReviewEnabled == nil || *normalized.ReviewEnabled || len(normalized.ReferenceAssetIDs) != 1) {
					t.Fatalf("lost selections/options: %+v", normalized)
				}
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, `{"code":40000,"data":{"reasonCode":"INVALID_REQUEST"}}`)
					return
				}
				io.WriteString(w, `{"code":0,"data":{"taskId":"1","taskTurnId":"2","projectId":"77","status":"QUEUED"}}`)
			}))
			defer server.Close()
			gateway, err := mcpserver.NewHTTPGateway(mcpserver.HTTPGatewayConfig{BaseURL: server.URL + "/api/"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.valid {
				handler, handlerErr := mcpserver.NewHandler(mcpserver.HandlerConfig{}, gateway, nil)
				if handlerErr != nil {
					t.Fatal(handlerErr)
				}
				mcpHTTP := httptest.NewServer(handler)
				defer mcpHTTP.Close()
				client := mcp.NewClient(&mcp.Implementation{Name: "REST-validator-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
				session, connectErr := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: mcpHTTP.URL + "/mcp", HTTPClient: &http.Client{Transport: fixtureBearerTransport{}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
				if connectErr != nil {
					t.Fatal(connectErr)
				}
				defer session.Close()
				result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_task", Arguments: map[string]any{
					"projectId": "77", "idempotencyKey": "idem", "instruction": "work", "externalCorrelationId": "external",
					"modelSelection": map[string]any{"credentialId": "9007199254740993", "model": "model"}, "runtimeReleaseId": "9007199254740994", "reviewEnabled": false, "referenceAssetIds": []string{"801"}, "deliveryPreference": "SAVE_ONLY",
				}})
				if callErr != nil || result.IsError {
					t.Fatalf("actual MCP-to-REST result=%+v error=%v", result, callErr)
				}
				return
			}
			review := false
			_, err = gateway.CreateTask(context.Background(), "Bearer local-fixture", mcpserver.CreateTaskRequest{ProjectID: 77, IdempotencyKey: "idem", Instruction: "work", ExternalCorrelationID: "external", ModelSelection: taskfacade.ModelSelection{CredentialID: tc.credential, Model: tc.model}, RuntimeReleaseID: tc.runtime, ReviewEnabled: &review, ReferenceAssetIDs: []int64{801}, DeliveryPreference: "SAVE_ONLY"})
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && mcpserver.ErrorCode(err) != "INVALID_REQUEST" {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

type fixtureBearerTransport struct{}

func (fixtureBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header.Set("Authorization", "Bearer local-fixture")
	return http.DefaultTransport.RoundTrip(cloned)
}

func TestSelectionEndpointsAuthenticateBeforeProjectOrCatalogAccess(t *testing.T) {
	handler := New(openapp.NewService(nil, nil, nil, nil, nil, ""))
	for _, invoke := range []func(context.Context, *app.RequestContext){handler.listModelOptions, handler.listRuntimeOptions} {
		c := &app.RequestContext{}
		invoke(context.Background(), c)
		if c.Response.StatusCode() != http.StatusUnauthorized || !strings.Contains(string(c.Response.Body()), "UNAUTHORIZED") {
			t.Fatalf("unauthenticated discovery status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
		}
	}
}

func TestPublicSelectionErrorClassificationUsesOnlySharedSentinels(t *testing.T) {
	for _, tc := range []struct {
		name          string
		err           error
		status        int
		code, message string
	}{
		{"invalid model", fmt.Errorf("private model detail: %w", providerfacade.ErrInvalidTaskModelSelection), 400, "INVALID_REQUEST", "The request is invalid."},
		{"invalid task", fmt.Errorf("private task detail: %w", taskfacade.ErrInvalidTaskSubmission), 400, "INVALID_REQUEST", "The request is invalid."},
		{"unknown database error", errors.New("TASK_MODEL_NOT_IN_CATALOG: private database outage"), 500, "INTERNAL_ERROR", "The request could not be completed."},
		{"matching sentinel text only", errors.New(providerfacade.ErrInvalidTaskModelSelection.Error()), 500, "INTERNAL_ERROR", "The request could not be completed."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &app.RequestContext{}
			writeOpenError(c, tc.err)
			var body struct {
				Message string `json:"message"`
				Data    struct {
					ReasonCode string `json:"reasonCode"`
				} `json:"data"`
			}
			if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if c.Response.StatusCode() != tc.status || body.Data.ReasonCode != tc.code || body.Message != tc.message {
				t.Fatalf("public error status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
			}
			if strings.Contains(string(c.Response.Body()), "private") || strings.Contains(string(c.Response.Body()), "TASK_MODEL_NOT_IN_CATALOG") {
				t.Fatal("internal failure text leaked")
			}
		})
	}
}
