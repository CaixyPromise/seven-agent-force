package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/tracer/traceinfo"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLogsStartAndFinishWithMaskedPayload(t *testing.T) {
	const requestTraceID = "0123456789abcdef0123456789abcdef"
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.POST("/login", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := `{"username":"alice","password":"super-secret","secretCiphertext":"cipher","configKey":"payment.gateway.secret","configValue":"plain-secret-updated","isSensitive":1,"captchaCode":"captcha-live","otpCode":"otp-live","clientDataJSON":"client-data-live","authenticatorData":"auth-data-live","signature":"signature-live","credentialIdentifier":"credential-live"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/login?accessToken=query-token&id=1", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	},
		ut.Header{Key: xcontext.TraceIDHeader, Value: requestTraceID},
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "User-Agent", Value: "unit-test"},
		ut.Header{Key: "X-Forwarded-For", Value: "203.0.113.10, 10.0.0.2"},
	)

	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result.TraceID != requestTraceID {
		t.Fatalf("unexpected traceId: %s", result.TraceID)
	}

	started := findLogEntry(t, logs, "request_started")
	startedFields := started.ContextMap()
	if startedFields["trace_id"] != requestTraceID {
		t.Fatalf("unexpected trace_id: %#v", startedFields["trace_id"])
	}
	if startedFields["client_ip"] != "203.0.113.10" {
		t.Fatalf("unexpected client_ip: %#v", startedFields["client_ip"])
	}
	if startedFields["raw_query"] != "accessToken=******&id=1" {
		t.Fatalf("unexpected raw_query: %#v", startedFields["raw_query"])
	}

	payload := startedFields["payload"].(map[string]any)
	query := payload["query"].(map[string]any)
	if query["accessToken"] != "******" {
		t.Fatalf("query token not masked: %#v", query["accessToken"])
	}
	bodyPayload := payload["body"].(map[string]any)
	if bodyPayload["password"] != "******" {
		t.Fatalf("password not masked: %#v", bodyPayload["password"])
	}
	if bodyPayload["secretCiphertext"] != "******" {
		t.Fatalf("secretCiphertext not masked: %#v", bodyPayload["secretCiphertext"])
	}
	if bodyPayload["configKey"] != "payment.gateway.secret" {
		t.Fatalf("configKey should remain visible: %#v", bodyPayload["configKey"])
	}
	if bodyPayload["configValue"] != "******" {
		t.Fatalf("configValue not masked: %#v", bodyPayload["configValue"])
	}
	if bodyPayload["isSensitive"] != "******" {
		t.Fatalf("isSensitive flag not masked: %#v", bodyPayload["isSensitive"])
	}
	for _, key := range []string{"captchaCode", "otpCode", "clientDataJSON", "authenticatorData", "signature", "credentialIdentifier"} {
		if bodyPayload[key] != "******" {
			t.Fatalf("%s not masked: %#v", key, bodyPayload[key])
		}
	}

	rawStarted := started.ContextMap()
	for _, leaked := range []string{"query-token", "super-secret", "plain-secret-updated", "captcha-live", "otp-live", "client-data-live", "auth-data-live", "signature-live", "credential-live"} {
		if strings.Contains(rawStarted["raw_query"].(string), leaked) {
			t.Fatalf("raw_query leaked secret %q: %s", leaked, rawStarted["raw_query"])
		}
	}

	finished := findLogEntry(t, logs, "request_finished")
	finishedFields := finished.ContextMap()
	if finishedFields["trace_id"] != requestTraceID {
		t.Fatalf("unexpected finish trace_id: %#v", finishedFields["trace_id"])
	}
	if finishedFields["status"] != int64(200) {
		t.Fatalf("unexpected status: %#v", finishedFields["status"])
	}
	if finishedFields["response_size"] == nil {
		t.Fatal("expected response_size in request_finished log")
	}
	if finishedFields["latency_ms"] == nil {
		t.Fatal("expected latency_ms in request_finished log")
	}
}

func TestRequestLogsMaskCredentialBearingCloneURL(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))
	engine.POST("/workforce/projects", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	const secret = "glpat-must-not-reach-logs"
	body := `{"repository":{"cloneUrl":"https://seven:` + secret + `@gitlab.example.com/group/repo.git"}}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/workforce/projects", &ut.Body{
		Body: bytes.NewBufferString(body), Len: len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	started := findLogEntry(t, logs, "request_started").ContextMap()
	encoded, err := json.Marshal(started["payload"])
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "seven@") {
		t.Fatalf("credential-bearing clone URL reached request log: %s", encoded)
	}
	payload := started["payload"].(map[string]any)
	repository := payload["body"].(map[string]any)["repository"].(map[string]any)
	if repository["cloneUrl"] != "******" {
		t.Fatalf("cloneUrl was not fully masked: %#v", repository["cloneUrl"])
	}
}

func TestSensitiveDictMutationRequestLogUsesDigestOnly(t *testing.T) {
	const canaryValue = "secret-item-value-canary"
	const canaryLabel = "secret-item-label-canary"
	const canaryExt = "secret-ext-json-canary"
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))
	engine.POST("/api/dict/:typeId/items", func(_ context.Context, c *app.RequestContext) {
		response.Error(c, apperrors.Params("rejected "+canaryValue+" "+canaryLabel+" "+canaryExt))
	})
	body := `{"itemValue":"` + canaryValue + `","itemLabel":"` + canaryLabel + `","extJson":"` + canaryExt + `"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/api/dict/17/items", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected business error envelope over HTTP 200, got %d", recorder.Code)
	}
	started := findLogEntry(t, logs, "request_started")
	payload := started.ContextMap()["payload"].(map[string]any)
	digest := payload["body"].(map[string]any)
	if digest["kind"] != "sensitive_dict_mutation" || digest["sha256"] == "" {
		t.Fatalf("expected digest-only request summary, got %#v", digest)
	}
	finished := findLogEntry(t, logs, "request_finished")
	if got := finished.ContextMap()["error_message"]; got != "sensitive dictionary mutation rejected" {
		t.Fatalf("expected generic rejected mutation message, got %#v", got)
	}
	rendered, err := json.Marshal([]map[string]any{started.ContextMap(), finished.ContextMap()})
	if err != nil {
		t.Fatalf("marshal log: %v", err)
	}
	for _, canary := range []string{canaryValue, canaryLabel, canaryExt, "itemValue", "itemLabel", "extJson"} {
		if strings.Contains(string(rendered), canary) {
			t.Fatalf("sensitive dictionary request log leaked %q: %s", canary, rendered)
		}
	}
}

func TestProviderInvocationRequestLogUsesDigestOnly(t *testing.T) {
	const promptCanary = "provider-prompt-must-never-enter-request-logs"
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))
	engine.POST("/api/internal/workforce/v1/provider-invocations/responses", func(_ context.Context, c *app.RequestContext) {
		c.SetStatusCode(200)
		c.SetContentType("application/json")
		c.SetBodyString(`{"id":"resp_fake"}`)
	})
	body := `{"model":"gpt-fixture","input":"` + promptCanary + `","stream":false}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/api/internal/workforce/v1/provider-invocations/responses", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected HTTP 200, got %d", recorder.Code)
	}
	started := findLogEntry(t, logs, "request_started")
	payload := started.ContextMap()["payload"].(map[string]any)
	digest := payload["body"].(map[string]any)
	if digest["kind"] != "provider_invocation" || digest["sha256"] == "" || digest["content_length"] != len(body) {
		t.Fatalf("expected digest-only Provider request summary, got %#v", digest)
	}
	rendered, err := json.Marshal(started.ContextMap())
	if err != nil {
		t.Fatalf("marshal log: %v", err)
	}
	for _, forbidden := range []string{promptCanary, "gpt-fixture", `\"input\"`} {
		if strings.Contains(string(rendered), forbidden) {
			t.Fatalf("Provider Invocation request log leaked %q: %s", forbidden, rendered)
		}
	}
}

func TestOpenTaskRequestLogUsesDigestOnly(t *testing.T) {
	const instructionCanary = "external-instruction-must-never-enter-request-logs"
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))
	engine.POST("/api/open/workforce/v1/tasks", func(_ context.Context, c *app.RequestContext) {
		c.JSON(http.StatusAccepted, map[string]string{"taskId": "tsk_fixture"})
	})
	body := `{"scenarioKey":"mvp-empty-workspace","name":"Task","instruction":"` + instructionCanary + `","externalCorrelationId":"issue-1"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/api/open/workforce/v1/tasks", &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected HTTP 202, got %d", recorder.Code)
	}
	started := findLogEntry(t, logs, "request_started")
	payload := started.ContextMap()["payload"].(map[string]any)
	digest := payload["body"].(map[string]any)
	if digest["kind"] != "open_task_submission" || digest["sha256"] == "" || digest["content_length"] != len(body) {
		t.Fatalf("expected digest-only Open Task summary, got %#v", digest)
	}
	rendered, _ := json.Marshal(started.ContextMap())
	if strings.Contains(string(rendered), instructionCanary) || strings.Contains(string(rendered), "issue-1") {
		t.Fatalf("Open Task request log leaked business data: %s", rendered)
	}
}

func TestNewServerUsesConfiguredRequestBodyLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Server.MaxRequestBodyBytes = 6 * 1024 * 1024
	if got := NewServer(cfg, zap.NewNop()).GetOptions().MaxRequestBodySize; got != cfg.Server.MaxRequestBodyBytes {
		t.Fatalf("MaxRequestBodySize = %d, want %d", got, cfg.Server.MaxRequestBodyBytes)
	}

	cfg.Server.MaxRequestBodyBytes = 0
	if got := NewServer(cfg, zap.NewNop()).GetOptions().MaxRequestBodySize; got != defaultMaxRequestBodyBytes {
		t.Fatalf("default MaxRequestBodySize = %d, want %d", got, defaultMaxRequestBodyBytes)
	}
}

func TestInternalRequestUsesCanonicalGoContextAndInternalEventNames(t *testing.T) {
	const traceID = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))
	engine.GET("/internal/node/v1/descriptor", func(ctx context.Context, c *app.RequestContext) {
		if got := xcontext.TraceIDFromContext(ctx); got != traceID {
			t.Errorf("Go context trace=%q, want %q", got, traceID)
		}
		response.Success(c, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/internal/node/v1/descriptor", nil,
		ut.Header{Key: xcontext.TraceIDHeader, Value: traceID})
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if logs.FilterMessage("internal_request_started").Len() != 1 || logs.FilterMessage("internal_request_finished").Len() != 1 {
		t.Fatalf("internal events missing: %#v", logs.All())
	}
	if logs.FilterMessage("request_started").Len() != 0 || logs.FilterMessage("request_finished").Len() != 0 {
		t.Fatalf("internal request used public event names: %#v", logs.All())
	}
}

func TestServerBindsQuotedInt64AndReturnsBusinessInt64AsString(t *testing.T) {
	type requestBody struct {
		ID      int64   `json:"id"`
		RoleIDs []int64 `json:"roleIds"`
	}

	engine := NewServer(testConfig(), zap.NewNop())
	engine.POST("/int64-contract", func(ctx context.Context, c *app.RequestContext) {
		var request requestBody
		if err := httpx.Bind(c, &request); err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, request)
	})

	body := `{"id":"9007199254740993","roleIds":["9007199254740995",7]}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/int64-contract", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected HTTP 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var actual struct {
		Code int `json:"code"`
		Data struct {
			ID      string   `json:"id"`
			RoleIDs []string `json:"roleIds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &actual); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if actual.Code != apperrors.CodeSuccess {
		t.Fatalf("expected success response, got %d: %s", actual.Code, recorder.Body.String())
	}
	if actual.Data.ID != "9007199254740993" || len(actual.Data.RoleIDs) != 2 || actual.Data.RoleIDs[0] != "9007199254740995" || actual.Data.RoleIDs[1] != "7" {
		t.Fatalf("unexpected int64 wire values: %#v", actual.Data)
	}
}

func TestDirectProtocolJSONKeepsNumericFields(t *testing.T) {
	engine := NewServer(testConfig(), zap.NewNop())
	engine.GET("/oauth-token-shape", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, map[string]any{"expires_in": int64(3600)})
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/oauth-token-shape", nil)
	var actual map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &actual); err != nil {
		t.Fatalf("unmarshal protocol response: %v", err)
	}
	if actual["expires_in"] != float64(3600) {
		t.Fatalf("protocol number must remain numeric: %#v", actual)
	}
}

func TestRequestLogsAlwaysMaskSessionRef(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	cfg := testConfig()
	cfg.Logging.Request.MaskedFields = nil
	engine := NewServer(cfg, zap.New(core))
	engine.POST("/internal/node/v1/users/2001/sessions/revoke", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := `{"sessionRefs":["opaque-session-live"],"reason":"incident"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/internal/node/v1/users/2001/sessions/revoke", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "internal_request_started").ContextMap()
	raw := fmt.Sprint(started["payload"])
	if strings.Contains(raw, "opaque-session-live") {
		t.Fatalf("request log leaked sessionRef: %s", raw)
	}
	if !strings.Contains(raw, "******") {
		t.Fatalf("request log did not mask sessionRef: %s", raw)
	}
}

func TestRequestLogsAlwaysMaskClientSecretWithCustomMaskedFields(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	cfg := testConfig()
	cfg.Logging.Request.MaskedFields = []string{"password"}
	engine := NewServer(cfg, zap.New(core))
	engine.POST("/sso/clients", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := `{"clientSecret":"raw-client-secret-live","name":"console"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/sso/clients", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "request_started").ContextMap()
	raw := fmt.Sprint(started["payload"])
	if strings.Contains(raw, "raw-client-secret-live") {
		t.Fatalf("request log leaked clientSecret: %s", raw)
	}
	if !strings.Contains(raw, "******") {
		t.Fatalf("request log did not mask clientSecret: %s", raw)
	}
}

func TestRequestLogSensitiveBaselineIncludesClientSecretAndKeyword(t *testing.T) {
	cfg := withRequestLogSensitiveBaseline(config.RequestLoggingConfig{MaskedFields: []string{"password"}})
	fields := make(map[string]bool, len(cfg.MaskedFields))
	for _, field := range cfg.MaskedFields {
		fields[strings.ToLower(field)] = true
	}
	for _, field := range []string{"clientsecret", "managementbearer", "keyword"} {
		if !fields[field] {
			t.Fatalf("mandatory request-log mask %q missing from %v", field, cfg.MaskedFields)
		}
	}
}

func TestRequestLogsAlwaysMaskManagementBearer(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	cfg := testConfig()
	cfg.Logging.Request.MaskedFields = []string{"password"}
	engine := NewServer(cfg, zap.New(core))
	engine.POST("/system/hub/nodes", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := `{"nodeCode":"node-a","managementBearer":"node-management-bearer"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/system/hub/nodes", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	raw := fmt.Sprint(findLogEntry(t, logs, "request_started").ContextMap()["payload"])
	if strings.Contains(raw, "node-management-bearer") || !strings.Contains(raw, "******") {
		t.Fatalf("request log did not mask managementBearer: %s", raw)
	}
}

func TestRequestLogsAlwaysMaskNodeUserKeywordWithCustomMaskedFields(t *testing.T) {
	for _, keyword := range []string{"alice@example.com", "+6591234567"} {
		t.Run(keyword, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			cfg := testConfig()
			cfg.Logging.Request.MaskedFields = []string{"password"}
			engine := NewServer(cfg, zap.New(core))
			engine.GET("/internal/node/v1/users", func(ctx context.Context, c *app.RequestContext) {
				response.Success(c, map[string]any{"ok": true})
			})

			recorder := ut.PerformRequest(Routes(engine), "GET", "/internal/node/v1/users?keyword="+url.QueryEscape(keyword)+"&current=1", nil)
			if recorder.Code != 200 {
				t.Fatalf("expected 200, got %d", recorder.Code)
			}

			started := findLogEntry(t, logs, "internal_request_started").ContextMap()
			raw := fmt.Sprint(started)
			if strings.Contains(raw, keyword) || strings.Contains(raw, url.QueryEscape(keyword)) {
				t.Fatalf("request log leaked keyword: %s", raw)
			}
			if !strings.Contains(raw, "******") {
				t.Fatalf("request log did not mask keyword: %s", raw)
			}
		})
	}
}

func TestRequestLogRawQueryMasksStepUpProofToken(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.GET("/auth/step-up/validate", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(
		Routes(engine),
		"GET",
		"/auth/step-up/validate?token=proof-token-live&businessAction=RBAC_ASSIGN_USER_ROLES&flowNonce=flow-live&operationBinding=user%3A1001%7Croles%3A1%2C2&consumeOnce=true",
		nil,
	)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	rawQuery, ok := findLogEntry(t, logs, "request_started").ContextMap()["raw_query"].(string)
	if !ok {
		t.Fatal("expected raw_query string field")
	}
	if strings.Contains(rawQuery, "proof-token-live") {
		t.Fatalf("raw_query leaked proof token: %s", rawQuery)
	}
	if strings.Contains(rawQuery, "flow-live") || strings.Contains(rawQuery, "user%3A1001") {
		t.Fatalf("raw_query leaked proof binding context: %s", rawQuery)
	}
	if rawQuery != "token=******&businessAction=RBAC_ASSIGN_USER_ROLES&flowNonce=******&operationBinding=******&consumeOnce=true" {
		t.Fatalf("unexpected masked raw_query: %s", rawQuery)
	}
}

func TestRequestLogsMaskStepUpBindingContextInJSONPayload(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.POST("/auth/step-up/challenge", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := `{"businessAction":"CONFIG_SENSITIVE_REVEAL","flowNonce":"flow-live","operationBinding":"config:1001|reveal","proofToken":"proof-live"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/auth/step-up/challenge", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	payload := findLogEntry(t, logs, "request_started").ContextMap()["payload"].(map[string]any)
	bodyPayload := payload["body"].(map[string]any)
	for _, key := range []string{"flowNonce", "operationBinding", "proofToken"} {
		if bodyPayload[key] != "******" {
			t.Fatalf("%s not masked: %#v", key, bodyPayload[key])
		}
	}
	encoded, _ := json.Marshal(payload)
	for _, leaked := range []string{"flow-live", "config:1001|reveal", "proof-live"} {
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("payload leaked step-up context %q: %s", leaked, encoded)
		}
	}
}

func TestRequestLogsMaskOAuthAuthorizationCodeAndVerifier(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.POST("/sso/oauth2/token", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := "grant_type=authorization_code&client_id=authorization-console&code=oauth-code-live&code_verifier=pkce-verifier-live&statusCode=200&errorCode=invalid_request&redirect_uri=http%3A%2F%2F127.0.0.1%2Fcallback"
	recorder := ut.PerformRequest(Routes(engine), "POST", "/sso/oauth2/token?code=query-oauth-code&codeVerifier=query-pkce-verifier&statusCode=200", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "request_started").ContextMap()
	rawQuery := started["raw_query"].(string)
	for _, leaked := range []string{"query-oauth-code", "query-pkce-verifier"} {
		if strings.Contains(rawQuery, leaked) {
			t.Fatalf("raw_query leaked OAuth credential %q: %s", leaked, rawQuery)
		}
	}
	if !strings.Contains(rawQuery, "statusCode=200") {
		t.Fatalf("statusCode should remain visible, got %s", rawQuery)
	}

	payload := started["payload"].(map[string]any)
	bodyPayload := payload["body"].(map[string]any)
	if bodyPayload["code"] != "******" {
		t.Fatalf("authorization code not masked: %#v", bodyPayload["code"])
	}
	if bodyPayload["code_verifier"] != "******" {
		t.Fatalf("PKCE verifier not masked: %#v", bodyPayload["code_verifier"])
	}
	if bodyPayload["statusCode"] != "200" {
		t.Fatalf("statusCode should remain visible in parsed payload: %#v", bodyPayload["statusCode"])
	}
	if bodyPayload["errorCode"] != "invalid_request" {
		t.Fatalf("errorCode should remain visible in parsed payload: %#v", bodyPayload["errorCode"])
	}
	for _, leaked := range []string{"oauth-code-live", "pkce-verifier-live"} {
		encoded, _ := json.Marshal(payload)
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("payload leaked OAuth credential %q: %s", leaked, encoded)
		}
	}
}

func TestRequestLogsMaskOAuthAuthorizationCodeInJSONAndRawFallback(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.POST("/sso/oauth2/token", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	jsonBody := `{"code":"json-oauth-code","codeVerifier":"json-pkce-verifier","statusCode":200,"errorCode":"invalid_request"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/sso/oauth2/token", &ut.Body{
		Body: bytes.NewBufferString(jsonBody),
		Len:  len(jsonBody),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	jsonPayload := findLogEntry(t, logs, "request_started").ContextMap()["payload"].(map[string]any)["body"].(map[string]any)
	if jsonPayload["code"] != "******" {
		t.Fatalf("JSON authorization code not masked: %#v", jsonPayload["code"])
	}
	if jsonPayload["codeVerifier"] != "******" {
		t.Fatalf("JSON PKCE verifier not masked: %#v", jsonPayload["codeVerifier"])
	}
	if fmt.Sprint(jsonPayload["statusCode"]) != "200" || jsonPayload["errorCode"] != "invalid_request" {
		t.Fatalf("statusCode/errorCode should remain visible: %#v", jsonPayload)
	}

	core2, logs2 := observer.New(zap.DebugLevel)
	engine2 := NewServer(testConfig(), zap.New(core2))
	engine2.POST("/raw", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	rawBody := `{"code":"raw-oauth-code","code_verifier":"raw-pkce-verifier","statusCode":200`
	recorder = ut.PerformRequest(Routes(engine2), "POST", "/raw", &ut.Body{
		Body: bytes.NewBufferString(rawBody),
		Len:  len(rawBody),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	rawPayload := findLogEntry(t, logs2, "request_started").ContextMap()["payload"].(map[string]any)["body"].(map[string]any)
	raw := rawPayload["raw"].(string)
	for _, leaked := range []string{"raw-oauth-code", "raw-pkce-verifier"} {
		if strings.Contains(raw, leaked) {
			t.Fatalf("raw fallback leaked OAuth credential %q: %s", leaked, raw)
		}
	}
	if !strings.Contains(raw, `"statusCode":200`) {
		t.Fatalf("statusCode should remain visible in raw fallback: %s", raw)
	}
}

func TestRequestLogsMaskSecurityFieldsEvenWithLegacyMaskedConfig(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	cfg := testConfig()
	cfg.Logging.Request.MaskedFields = []string{"password"}
	engine := NewServer(cfg, zap.New(core))

	engine.POST("/login/passkey/verify", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := `{"password":"secret-password","captchaCode":"captcha-live","clientDataJSON":"client-data-live","authenticatorData":"auth-data-live","signature":"signature-live","credentialIdentifier":"credential-live"}`
	recorder := ut.PerformRequest(Routes(engine), "POST", "/login/passkey/verify?signature=query-signature-live", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "request_started").ContextMap()
	if rawQuery, _ := started["raw_query"].(string); strings.Contains(rawQuery, "query-signature-live") {
		t.Fatalf("raw_query leaked signature: %s", rawQuery)
	}
	payload := started["payload"].(map[string]any)
	bodyPayload := payload["body"].(map[string]any)
	for _, key := range []string{"password", "captchaCode", "clientDataJSON", "authenticatorData", "signature", "credentialIdentifier"} {
		if bodyPayload[key] != "******" {
			t.Fatalf("%s not masked with legacy config: %#v", key, bodyPayload[key])
		}
	}
}

func TestMiddlewareHandlesPanicAndLogsTraceID(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	log := zap.New(core)
	engine := NewServer(testConfig(), log)

	engine.GET("/panic", func(ctx context.Context, c *app.RequestContext) {
		panic("boom")
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/panic", nil)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var body response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != apperrors.CodeSystemError {
		t.Fatalf("unexpected business code: %d", body.Code)
	}
	if body.TraceID == "" {
		t.Fatal("expected traceId in response")
	}

	findLogEntry(t, logs, "request_started")
	panicEntry := findLogEntry(t, logs, "request_panic_recovered")
	if panicEntry.ContextMap()["trace_id"] != body.TraceID {
		t.Fatalf("unexpected panic trace_id: %#v", panicEntry.ContextMap()["trace_id"])
	}

	finished := findLogEntry(t, logs, "request_finished").ContextMap()
	if finished["status"] != int64(200) {
		t.Fatalf("unexpected panic status: %#v", finished["status"])
	}
	if finished["error_code"] != int64(apperrors.CodeSystemError) {
		t.Fatalf("unexpected panic error_code: %#v", finished["error_code"])
	}
}

func TestRequestLogsInternalErrorCauseWithoutReturningItToClient(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.GET("/internal-error", func(ctx context.Context, c *app.RequestContext) {
		response.Error(c, fmt.Errorf("insert sso session: duplicate test failure"))
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/internal-error", nil)
	var body response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != apperrors.CodeSystemError || body.Message != "系统内部异常" {
		t.Fatalf("unexpected public error body: %#v", body)
	}
	if strings.Contains(recorder.Body.String(), "duplicate test failure") {
		t.Fatalf("internal cause leaked to client: %s", recorder.Body.String())
	}

	finished := findLogEntry(t, logs, "request_finished").ContextMap()
	if finished["error_cause"] != "insert sso session: duplicate test failure" {
		t.Fatalf("unexpected internal error cause: %#v", finished["error_cause"])
	}
}

func TestNoRouteReturns404WithUnifiedBody(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	recorder := ut.PerformRequest(Routes(engine), "GET", "/missing", nil)
	if recorder.Code != 404 {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}

	var body response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != apperrors.CodeNotFound {
		t.Fatalf("unexpected business code: %d", body.Code)
	}
	if body.TraceID == "" {
		t.Fatal("expected traceId in 404 response")
	}

	finished := findLogEntry(t, logs, "request_finished").ContextMap()
	if finished["status"] != int64(404) {
		t.Fatalf("unexpected 404 status: %#v", finished["status"])
	}
	if finished["error_code"] != int64(apperrors.CodeNotFound) {
		t.Fatalf("unexpected 404 error_code: %#v", finished["error_code"])
	}
}

func TestBindAndValidateReturns200WithParamsCode(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	type request struct {
		Name string `query:"name" validate:"required"`
	}

	engine.GET("/bind", func(ctx context.Context, c *app.RequestContext) {
		var req request
		if err := httpx.BindAndValidate(c, &req); err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, req)
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/bind", nil)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var body response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != apperrors.CodeParamsError {
		t.Fatalf("unexpected business code: %d", body.Code)
	}
	if body.TraceID == "" {
		t.Fatal("expected traceId in validation response")
	}

	finished := findLogEntry(t, logs, "request_finished").ContextMap()
	if finished["status"] != int64(200) {
		t.Fatalf("unexpected validation status: %#v", finished["status"])
	}
	if finished["error_code"] != int64(apperrors.CodeParamsError) {
		t.Fatalf("unexpected validation error_code: %#v", finished["error_code"])
	}
}

func TestNoMethodReturns405WithUnifiedBody(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.GET("/method", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(Routes(engine), "POST", "/method", nil)
	if recorder.Code != 405 {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}

	var body response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != apperrors.CodeForbidden {
		t.Fatalf("unexpected business code: %d", body.Code)
	}

	finished := findLogEntry(t, logs, "request_finished").ContextMap()
	if finished["status"] != int64(405) {
		t.Fatalf("unexpected method-not-allowed status: %#v", finished["status"])
	}
}

func TestErrorResponseCarriesStableRejectionSemantics(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "permission denied",
			err:      apperrors.Forbidden("无权限"),
			wantCode: apperrors.CodeForbidden,
		},
		{
			name:     "data scope denied",
			err:      apperrors.DataScopeDenied("数据范围不足"),
			wantCode: apperrors.CodeDataScopeDenied,
		},
		{
			name:     "invalid object state",
			err:      apperrors.ObjectState("当前状态不允许操作"),
			wantCode: apperrors.CodeObjectStateInvalid,
		},
		{
			name:     "not found",
			err:      apperrors.NotFound("资源不存在"),
			wantCode: apperrors.CodeNotFound,
		},
		{
			name:     "params error",
			err:      apperrors.Params("参数错误"),
			wantCode: apperrors.CodeParamsError,
		},
		{
			name:     "operation failed",
			err:      apperrors.Operation("操作失败"),
			wantCode: apperrors.CodeOperateError,
		},
		{
			name:     "challenge throttled",
			err:      apperrors.ChallengeThrottled("挑战触发过于频繁"),
			wantCode: apperrors.CodeRateLimited,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := NewServer(testConfig(), zap.NewNop())
			engine.GET("/semantic-error", func(ctx context.Context, c *app.RequestContext) {
				response.Error(c, tc.err)
			})

			recorder := ut.PerformRequest(Routes(engine), "GET", "/semantic-error", nil)
			if tc.wantCode == apperrors.CodeNotFound {
				if recorder.Code != 404 {
					t.Fatalf("expected not-found HTTP 404, got %d", recorder.Code)
				}
			} else if tc.wantCode == apperrors.CodeRateLimited {
				if recorder.Code != 429 {
					t.Fatalf("expected rate-limited HTTP 429, got %d", recorder.Code)
				}
			} else if recorder.Code != 200 {
				t.Fatalf("expected HTTP 200 for business rejection, got %d", recorder.Code)
			}

			var body response.Result
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if body.Code != tc.wantCode {
				t.Fatalf("unexpected business code: got %d want %d", body.Code, tc.wantCode)
			}
			if body.TraceID == "" {
				t.Fatal("expected traceId")
			}
			var raw map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
				t.Fatalf("unmarshal raw response: %v", err)
			}
			if _, exists := raw["errorType"]; exists {
				t.Fatalf("error response must omit errorType: %#v", raw)
			}
			if _, exists := raw["errorCode"]; exists {
				t.Fatalf("error response must omit errorCode: %#v", raw)
			}
		})
	}
}

func TestSuccessResponseOmitsErrorSemantics(t *testing.T) {
	engine := NewServer(testConfig(), zap.NewNop())
	engine.GET("/success", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/success", nil)
	if recorder.Code != 200 {
		t.Fatalf("expected HTTP 200, got %d", recorder.Code)
	}

	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if raw["code"] != float64(apperrors.CodeSuccess) {
		t.Fatalf("unexpected business code: %#v", raw["code"])
	}
	if _, exists := raw["errorType"]; exists {
		t.Fatalf("success response must omit errorType: %#v", raw)
	}
	if _, exists := raw["errorCode"]; exists {
		t.Fatalf("success response must omit errorCode: %#v", raw)
	}
	if raw["traceId"] == "" {
		t.Fatal("expected traceId")
	}
}

func TestSuccessWithStatusPreservesFrameworkEnvelopeAndInt64Wire(t *testing.T) {
	engine := NewServer(testConfig(), zap.NewNop())
	engine.POST("/accepted", func(ctx context.Context, c *app.RequestContext) {
		response.SuccessWithStatus(c, http.StatusAccepted, struct {
			TaskID int64 `json:"taskId"`
		}{TaskID: 9_007_199_254_740_993})
	})

	recorder := ut.PerformRequest(Routes(engine), "POST", "/accepted", nil)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d, want 202", recorder.Code)
	}
	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	data, ok := raw["data"].(map[string]any)
	if !ok || data["taskId"] != "9007199254740993" || raw["message"] != "ok" || raw["code"] != float64(apperrors.CodeSuccess) || raw["traceId"] == "" {
		t.Fatalf("unexpected envelope: %#v", raw)
	}
}

type readTrackingReader struct{ reads int }

func (r *readTrackingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}

func TestResponseSizeDoesNotMaterializeBodyStream(t *testing.T) {
	c := &app.RequestContext{}
	reader := &readTrackingReader{}
	c.Response.SetBodyStream(reader, 123)
	if got := responseSize(c); got != 123 {
		t.Fatalf("responseSize=%d, want 123", got)
	}
	if reader.reads != 0 {
		t.Fatalf("response size logging consumed stream %d times", reader.reads)
	}
	_ = c.Response.CloseBodyStream()
}

func TestResponseWriteFailureTracerClassifiesStreamErrorsWithoutRawDetails(t *testing.T) {
	for _, test := range []struct {
		name, errorText, wantCategory string
	}{
		{"client disconnect", "write tcp: broken pipe", "client_disconnect"},
		{"stream failure", "private/object/key storage failure", "stream_write_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			c := &app.RequestContext{}
			c.SetTraceInfo(traceinfo.NewTraceInfo())
			c.GetTraceInfo().Stats().SetError(errors.New(test.errorText))
			c.Request.SetMethod("GET")
			c.Request.SetRequestURI("/api/open/workforce/v1/tasks/1/outputs/2/download")
			c.Response.SetBodyStream(bytes.NewBufferString("output"), 6)
			c.Response.Header.Set("Content-Disposition", `attachment; filename="output"`)
			// Hertz closes and clears BodyStream before tracer Finish. The response
			// metadata must retain the transport classification boundary.
			_ = c.Response.CloseBodyStream()

			responseWriteFailureTracer{log: zap.New(core)}.Finish(context.Background(), c)
			entry := findLogEntry(t, logs, "response_stream_write_aborted")
			fields := entry.ContextMap()
			if fields["category"] != test.wantCategory {
				t.Fatalf("category=%#v, want %q", fields["category"], test.wantCategory)
			}
			encoded, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), test.errorText) || strings.Contains(string(encoded), "private/object/key") {
				t.Fatalf("write tracer leaked raw error: %s", encoded)
			}
		})
	}
}

func TestRecoverySkipsJsonWriteForCommittedEventStream(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.GET("/stream", func(ctx context.Context, c *app.RequestContext) {
		c.Response.Header.SetContentType("text/event-stream")
		c.Response.SetBodyStream(bytes.NewBufferString("data: ping\n\n"), -1)
		panic("stream-boom")
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/stream", nil)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	if recorder.Body.String() != "data: ping\n\n" {
		t.Fatalf("unexpected stream body: %q", recorder.Body.String())
	}

	findLogEntry(t, logs, "request_stream_write_aborted")
	finished := findLogEntry(t, logs, "request_finished").ContextMap()
	if _, exists := finished["error_code"]; exists {
		t.Fatalf("expected no error_code for committed stream: %#v", finished["error_code"])
	}
}

func TestIsClientDisconnectClassification(t *testing.T) {
	if !isClientDisconnect(errors.New("write tcp 127.0.0.1: broken pipe")) {
		t.Fatal("expected broken pipe to be classified as client disconnect")
	}
	if !isClientDisconnect(errors.New("connection reset by peer")) {
		t.Fatal("expected connection reset to be classified as client disconnect")
	}
	if isClientDisconnect(errors.New("ordinary failure")) {
		t.Fatal("did not expect ordinary failure to be classified as disconnect")
	}
}

func TestRequestLogsMultipartSummaryWithoutRawBody(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.POST("/upload", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"uploaded": true})
	})

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.WriteField("password", "multipart-secret"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := writer.WriteField("code", "multipart-oauth-code"); err != nil {
		t.Fatalf("write oauth code field: %v", err)
	}
	if err := writer.WriteField("codeVerifier", "multipart-pkce-verifier"); err != nil {
		t.Fatalf("write pkce verifier field: %v", err)
	}
	if err := writer.WriteField("statusCode", "200"); err != nil {
		t.Fatalf("write status field: %v", err)
	}
	part, err := writer.CreateFormFile("avatar", "avatar.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	recorder := ut.PerformRequest(Routes(engine), "POST", "/upload", &ut.Body{
		Body: bytes.NewReader(buffer.Bytes()),
		Len:  buffer.Len(),
	}, ut.Header{Key: "Content-Type", Value: writer.FormDataContentType()})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "request_started").ContextMap()
	payload := started["payload"].(map[string]any)
	bodyPayload := payload["body"].(map[string]any)
	if bodyPayload["kind"] != "multipart" {
		t.Fatalf("unexpected body kind: %#v", bodyPayload["kind"])
	}
	fields := bodyPayload["fields"].(map[string]any)
	if fields["password"] != "******" {
		t.Fatalf("multipart password not masked: %#v", fields["password"])
	}
	if fields["code"] != "******" {
		t.Fatalf("multipart OAuth code not masked: %#v", fields["code"])
	}
	if fields["codeVerifier"] != "******" {
		t.Fatalf("multipart PKCE verifier not masked: %#v", fields["codeVerifier"])
	}
	if fields["statusCode"] != "200" {
		t.Fatalf("multipart statusCode should remain visible: %#v", fields["statusCode"])
	}
	if _, exists := bodyPayload["raw"]; exists {
		t.Fatal("multipart payload should not include raw body")
	}
	files := bodyPayload["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("unexpected files payload: %#v", files)
	}
	file := files[0].(map[string]any)
	if file["size"] != "5" {
		t.Fatalf("unexpected file size serialization: %#v", file["size"])
	}
}

func TestRequestLogMasksOAuthCallbackStateAndNonce(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.GET("/login/external/:providerCode/callback", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(
		Routes(engine),
		"GET",
		"/login/external/github/callback?code=leak-code-secret&state=leak-state-secret&providerCode=google&oidcNonce=leak-nonce-secret&codeVerifier=leak-verifier-secret",
		nil,
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "request_started")
	fields := started.ContextMap()
	rawQuery := fields["raw_query"].(string)
	for _, leaked := range []string{"leak-code-secret", "leak-state-secret", "leak-nonce-secret", "leak-verifier-secret"} {
		if strings.Contains(rawQuery, leaked) {
			t.Fatalf("raw_query leaked OAuth secret %q: %s", leaked, rawQuery)
		}
	}
	payload := fields["payload"].(map[string]any)
	query := payload["query"].(map[string]any)
	for _, key := range []string{"code", "state", "oidcNonce", "codeVerifier"} {
		if query[key] != "******" {
			t.Fatalf("%s not masked in payload query: %#v", key, query[key])
		}
	}
}

func TestRequestLogsRawBodyMasksSensitiveConfigFields(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.POST("/raw", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	body := "configKey=payment.gateway configValue=plain-secret-updated isSensitive=1"
	recorder := ut.PerformRequest(Routes(engine), "POST", "/raw", &ut.Body{
		Body: bytes.NewBufferString(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "text/plain"})
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "request_started").ContextMap()
	payload := started["payload"].(map[string]any)
	bodyPayload := payload["body"].(map[string]any)
	raw := bodyPayload["raw"].(string)
	for _, leaked := range []string{"plain-secret-updated", "isSensitive=1"} {
		if strings.Contains(raw, leaked) {
			t.Fatalf("raw request log leaked %q in %s", leaked, raw)
		}
	}
	if !strings.Contains(raw, "configKey=payment.gateway") {
		t.Fatalf("configKey should remain visible in raw request log: %s", raw)
	}
}

func TestRequestLogsLoopbackAndRealIP(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	engine := NewServer(testConfig(), zap.New(core))

	engine.GET("/ip", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})

	recorder := ut.PerformRequest(Routes(engine), "GET", "/ip", nil,
		ut.Header{Key: "X-Forwarded-For", Value: "::1, 10.0.0.1"},
		ut.Header{Key: "X-Real-IP", Value: "198.51.100.9"},
	)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	started := findLogEntry(t, logs, "request_started").ContextMap()
	if started["client_ip"] != "127.0.0.1" {
		t.Fatalf("unexpected client_ip: %#v", started["client_ip"])
	}

	core2, logs2 := observer.New(zap.DebugLevel)
	engine2 := NewServer(testConfig(), zap.New(core2))
	engine2.GET("/ip", func(ctx context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{"ok": true})
	})
	recorder = ut.PerformRequest(Routes(engine2), "GET", "/ip", nil,
		ut.Header{Key: "X-Real-IP", Value: "198.51.100.9"},
	)
	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	started = findLogEntry(t, logs2, "request_started").ContextMap()
	if started["client_ip"] != "198.51.100.9" {
		t.Fatalf("unexpected real-ip client_ip: %#v", started["client_ip"])
	}
}

func findLogEntry(t *testing.T, logs *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	for _, entry := range logs.All() {
		if entry.Message == message {
			return entry
		}
	}
	t.Fatalf("log entry %s not found", message)
	return observer.LoggedEntry{}
}

func testConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         8888,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			IdleTimeout:  time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "json",
			Request: config.RequestLoggingConfig{
				Enabled:          true,
				MaxBodyBytes:     4096,
				MaxFieldLength:   512,
				IncludeQuery:     true,
				MaskedFields:     []string{"password", "pwd", "token", "secret", "authorization", "cookie", "set-cookie", "accessToken", "refreshToken", "clientSecret", "secretCiphertext", "passwordHash", "apiKey", "accessKey", "privateKey", "configValue", "isSensitive", "captchaCode", "otpCode", "totpCode", "oneTimePassword", "emailCode", "recoveryCode", "credentialIdentifier", "clientDataJSON", "authenticatorData", "signature", "userHandle"},
				SkipContentTypes: []string{"multipart/form-data", "application/octet-stream"},
			},
		},
	}
}
