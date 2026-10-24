package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestNodeHandlerUsesUpstreamGoTraceContextWithoutManualOverride(t *testing.T) {
	const traceID = "12121212121212121212121212121212"
	service := &fakeNodeService{}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Next(xcontext.WithTraceID(ctx, traceID))
	})
	New(service, service, "node-bearer").Mount(engine)

	response := ut.PerformRequest(engine.Engine, http.MethodGet, "/internal/node/v1/descriptor", nil,
		ut.Header{Key: "Authorization", Value: "Bearer node-bearer"})
	assertEnvelopeCode(t, response, apperrors.CodeSuccess)
	if service.lastTraceID != traceID {
		t.Fatalf("service trace=%q, want upstream %q", service.lastTraceID, traceID)
	}
}

func TestNodeHandlerMountsAllNineBearerProtectedRoutes(t *testing.T) {
	service := &fakeNodeService{}
	handler := New(service, service, "node-bearer")
	engine := server.Default()
	handler.Mount(engine)

	cases := []struct {
		method string
		path   string
		body   string
		write  bool
	}{
		{http.MethodGet, "/internal/node/v1/descriptor", "", false},
		{http.MethodGet, "/internal/node/v1/users", "", false},
		{http.MethodGet, "/internal/node/v1/users/2001", "", false},
		{http.MethodPut, "/internal/node/v1/users/2001/status", `{"status":1,"reason":"incident"}`, true},
		{http.MethodGet, "/internal/node/v1/users/2001/sessions", "", false},
		{http.MethodPost, "/internal/node/v1/users/2001/sessions/revoke", `{"all":true,"reason":"incident"}`, true},
		{http.MethodGet, "/internal/node/v1/login-policy", "", false},
		{http.MethodPost, "/internal/node/v1/login-policy/apply", `{"platformCode":"seven-admin","status":0,"allowAutoRegister":false,"allowFormRegister":true,"loginMethods":[],"sourceRules":[],"reason":"policy"}`, true},
		{http.MethodPut, "/internal/node/v1/hub-connection", `{"connectionVersion":"v1","enabled":true,"issuer":"https://hub.example.com","clientId":"node-a","clientSecret":"secret","redirectUri":"https://node.example.com/callback","reason":"provision"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			headers := []ut.Header{{Key: "Authorization", Value: "Bearer node-bearer"}, {Key: "Content-Type", Value: "application/json"}}
			if tc.write {
				headers = append(headers, ut.Header{Key: "Idempotency-Key", Value: "cmd-1"})
			}
			var body *ut.Body
			if tc.body != "" {
				body = &ut.Body{Body: strings.NewReader(tc.body), Len: len(tc.body)}
			}
			resp := ut.PerformRequest(engine.Engine, tc.method, tc.path, body, headers...)
			assertEnvelopeCode(t, resp, apperrors.CodeSuccess)
		})
	}
	if service.calls != 9 {
		t.Fatalf("service calls=%d want 9", service.calls)
	}
}

func TestNodeBearerRejectsMissingDuplicateWrongSchemeAndOversizedHeaders(t *testing.T) {
	service := &fakeNodeService{}
	handler := New(service, service, "node-bearer")
	engine := server.Default()
	handler.Mount(engine)

	cases := []struct {
		name    string
		headers []ut.Header
	}{
		{name: "missing"},
		{name: "wrong scheme", headers: []ut.Header{{Key: "Authorization", Value: "Basic node-bearer"}}},
		{name: "wrong token", headers: []ut.Header{{Key: "Authorization", Value: "Bearer wrong"}}},
		{name: "duplicate", headers: []ut.Header{{Key: "Authorization", Value: "Bearer node-bearer"}, {Key: "Authorization", Value: "Bearer node-bearer"}}},
		{name: "oversized", headers: []ut.Header{{Key: "Authorization", Value: "Bearer " + strings.Repeat("x", 8193)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/internal/node/v1/descriptor", nil, tc.headers...)
			assertEnvelopeCode(t, resp, apperrors.CodeNotLogin)
		})
	}
	if service.calls != 0 {
		t.Fatalf("unauthorized requests reached service: %d", service.calls)
	}
}

func TestNodeBearerSchemeIsCaseInsensitive(t *testing.T) {
	engine := server.New(server.WithHostPorts("127.0.0.1:0"))
	service := &fakeNodeService{}
	handler := New(service, service, "node-bearer")
	handler.Mount(engine)

	for _, scheme := range []string{"bearer", "BEARER", "BeArEr"} {
		response := ut.PerformRequest(engine.Engine, http.MethodGet, "/internal/node/v1/descriptor", nil,
			ut.Header{Key: "Authorization", Value: scheme + " node-bearer"})
		if response.Code != http.StatusOK {
			t.Fatalf("scheme %q status=%d body=%s", scheme, response.Code, response.Body.String())
		}
	}
}

func TestNodeWritesRequireIdempotencyKeyAndBoundedReason(t *testing.T) {
	service := &fakeNodeService{}
	handler := New(service, service, "node-bearer")
	engine := server.Default()
	handler.Mount(engine)
	headers := []ut.Header{{Key: "Authorization", Value: "Bearer node-bearer"}, {Key: "Content-Type", Value: "application/json"}}

	missingKey := ut.PerformRequest(engine.Engine, http.MethodPut, "/internal/node/v1/users/2001/status", &ut.Body{Body: strings.NewReader(`{"status":1,"reason":"incident"}`)}, headers...)
	assertEnvelopeCode(t, missingKey, apperrors.CodeParamsError)
	headers = append(headers, ut.Header{Key: "Idempotency-Key", Value: "cmd-1"})
	missingReason := ut.PerformRequest(engine.Engine, http.MethodPut, "/internal/node/v1/users/2001/status", &ut.Body{Body: strings.NewReader(`{"status":1}`)}, headers...)
	assertEnvelopeCode(t, missingReason, apperrors.CodeParamsError)
	longReason := `{"status":1,"reason":"` + strings.Repeat("x", 513) + `"}`
	tooLong := ut.PerformRequest(engine.Engine, http.MethodPut, "/internal/node/v1/users/2001/status", &ut.Body{Body: strings.NewReader(longReason)}, headers...)
	assertEnvelopeCode(t, tooLong, apperrors.CodeParamsError)
	if service.calls != 0 {
		t.Fatalf("invalid writes reached service: %d", service.calls)
	}
}

func TestNodeStatusRequiresPresentStatusAndAcceptsExplicitZero(t *testing.T) {
	service := &fakeNodeService{}
	handler := New(service, service, "node-bearer")
	engine := server.Default()
	handler.Mount(engine)
	headers := []ut.Header{
		{Key: "Authorization", Value: "Bearer node-bearer"},
		{Key: "Content-Type", Value: "application/json"},
		{Key: "Idempotency-Key", Value: "cmd-status"},
	}

	for _, body := range []string{`{"reason":"incident"}`, `{"status":null,"reason":"incident"}`} {
		response := ut.PerformRequest(engine.Engine, http.MethodPut, "/internal/node/v1/users/2001/status", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, headers...)
		assertEnvelopeCode(t, response, apperrors.CodeParamsError)
		if service.calls != 0 {
			t.Fatalf("invalid status request reached command facade: %d", service.calls)
		}
	}

	body := `{"status":0,"reason":"incident"}`
	response := ut.PerformRequest(engine.Engine, http.MethodPut, "/internal/node/v1/users/2001/status", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, headers...)
	assertEnvelopeCode(t, response, apperrors.CodeSuccess)
	if service.calls != 1 {
		t.Fatalf("explicit status zero did not reach command facade exactly once: %d", service.calls)
	}
}

type fakeNodeService struct {
	calls       int
	lastTraceID string
}

func (f *fakeNodeService) Describe(ctx context.Context) (*nodefacade.NodeDescriptor, error) {
	f.calls++
	f.lastTraceID = xcontext.TraceIDFromContext(ctx)
	return &nodefacade.NodeDescriptor{NodeCode: "order-admin"}, nil
}
func (f *fakeNodeService) ListUsers(context.Context, nodefacade.UserPageQuery) (*nodefacade.UserPage, error) {
	f.calls++
	return &nodefacade.UserPage{}, nil
}
func (f *fakeNodeService) GetUser(context.Context, int64) (*nodefacade.UserDetail, error) {
	f.calls++
	return &nodefacade.UserDetail{}, nil
}
func (f *fakeNodeService) ListUserSessions(context.Context, int64, nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error) {
	f.calls++
	return &nodefacade.SessionPage{}, nil
}
func (f *fakeNodeService) GetLoginPolicy(context.Context) (*nodefacade.ManagedLoginPolicy, error) {
	f.calls++
	return &nodefacade.ManagedLoginPolicy{}, nil
}
func (f *fakeNodeService) SetUserStatus(context.Context, nodefacade.SetUserStatusCommand) (*nodefacade.CommandResult, error) {
	f.calls++
	return &nodefacade.CommandResult{}, nil
}
func (f *fakeNodeService) RevokeUserSessions(context.Context, nodefacade.RevokeUserSessionsCommand) (*nodefacade.RevokeResult, error) {
	f.calls++
	return &nodefacade.RevokeResult{}, nil
}
func (f *fakeNodeService) ApplyLoginPolicy(context.Context, nodefacade.ApplyLoginPolicyCommand) (*nodefacade.CommandResult, error) {
	f.calls++
	return &nodefacade.CommandResult{}, nil
}
func (f *fakeNodeService) ApplyHubConnection(context.Context, nodefacade.ApplyHubConnectionCommand) (*nodefacade.CommandResult, error) {
	f.calls++
	return &nodefacade.CommandResult{}, nil
}

func assertEnvelopeCode(t *testing.T, recorder *ut.ResponseRecorder, want int) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, recorder.Body.String())
	}
	if got := int(body["code"].(float64)); got != want {
		t.Fatalf("business code=%d want %d body=%s", got, want, recorder.Body.String())
	}
	for _, forbidden := range []string{"errorType", "errorCode"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("unexpected %s in envelope: %s", forbidden, recorder.Body.String())
		}
	}
	for _, required := range []string{"code", "data", "message", "traceId"} {
		if _, ok := body[required]; !ok {
			t.Fatalf("missing %s in envelope: %s", required, recorder.Body.String())
		}
	}
}
