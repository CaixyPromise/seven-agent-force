package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/domain"
	hubfacade "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/facade"
	hubinfra "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/infrastructure"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestHubHandlerAlwaysReturnsHubCanonicalTraceForRemoteErrors(t *testing.T) {
	const canonicalTraceID = "88888888888888888888888888888888"
	reqCtx := &app.RequestContext{}
	reqCtx.Set(xcontext.TraceIDKey, canonicalTraceID)
	(&Handler{}).write(reqCtx, nil, fakeRemoteHTTPError{traceID: "99999999999999999999999999999999"})
	var result responseTraceResult
	if err := json.Unmarshal(reqCtx.Response.Body(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TraceID != canonicalTraceID || reqCtx.Response.Header.Get(xcontext.TraceIDHeader) != canonicalTraceID {
		t.Fatalf("body/header trace=%q/%q", result.TraceID, reqCtx.Response.Header.Get(xcontext.TraceIDHeader))
	}
}

func TestHubHandlerPreservesCanonicalTraceAcrossFederationErrorStatuses(t *testing.T) {
	const canonicalTraceID = "abababababababababababababababab"
	for _, status := range []int{http.StatusUnauthorized, http.StatusConflict, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			reqCtx := &app.RequestContext{}
			reqCtx.Set(xcontext.TraceIDKey, canonicalTraceID)
			remote := fakeRemoteHTTPError{status: status, code: status*100 + 1}
			if status == http.StatusTooManyRequests {
				remote.retryAfter = 17
			}
			(&Handler{}).write(reqCtx, nil, remote)
			var result struct {
				Code    int    `json:"code"`
				TraceID string `json:"traceId"`
			}
			if err := json.Unmarshal(reqCtx.Response.Body(), &result); err != nil {
				t.Fatal(err)
			}
			if reqCtx.Response.StatusCode() != status || result.Code != remote.code || result.TraceID != canonicalTraceID || reqCtx.Response.Header.Get(xcontext.TraceIDHeader) != canonicalTraceID {
				t.Fatalf("status/code/body/header=%d/%d/%q/%q", reqCtx.Response.StatusCode(), result.Code, result.TraceID, reqCtx.Response.Header.Get(xcontext.TraceIDHeader))
			}
			if status == http.StatusTooManyRequests && reqCtx.Response.Header.Get("Retry-After") != "17" {
				t.Fatalf("Retry-After=%q", reqCtx.Response.Header.Get("Retry-After"))
			}
		})
	}
}

type responseTraceResult struct {
	TraceID string `json:"traceId"`
}

type fakeRemoteHTTPError struct {
	traceID    string
	status     int
	code       int
	retryAfter int
}

func (e fakeRemoteHTTPError) Error() string { return "remote" }
func (e fakeRemoteHTTPError) RemoteStatusCode() int {
	if e.status != 0 {
		return e.status
	}
	return http.StatusServiceUnavailable
}
func (e fakeRemoteHTTPError) RemoteCode() int {
	if e.code != 0 {
		return e.code
	}
	return apperrors.CodeServiceUnavailable
}
func (e fakeRemoteHTTPError) RemoteMessage() string { return "unavailable" }
func (e fakeRemoteHTTPError) RemoteTraceID() string { return e.traceID }
func (e fakeRemoteHTTPError) RemoteRetryAfter() int { return e.retryAfter }

func TestHubHandlerMountsCompleteAPIAndNeverReturnsSecrets(t *testing.T) {
	engine := server.Default()
	New(fakeFacade{}).Mount(engine)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/system/hub/nodes"}, {http.MethodPost, "/system/hub/nodes"},
		{http.MethodGet, "/system/hub/nodes/node-a"}, {http.MethodPut, "/system/hub/nodes/node-a"},
		{http.MethodPost, "/system/hub/nodes/node-a/copy"}, {http.MethodPut, "/system/hub/nodes/node-a/status"},
		{http.MethodPost, "/system/hub/nodes/node-a/connection-test"}, {http.MethodGet, "/system/hub/nodes/node-a/users"},
		{http.MethodGet, "/system/hub/nodes/node-a/users/42"}, {http.MethodPut, "/system/hub/nodes/node-a/users/42/status"},
		{http.MethodGet, "/system/hub/nodes/node-a/users/42/sessions"}, {http.MethodPost, "/system/hub/nodes/node-a/users/42/sessions/revoke"},
		{http.MethodGet, "/system/hub/nodes/node-a/login-policy"}, {http.MethodPost, "/system/hub/nodes/node-a/login-policy/apply"},
		{http.MethodGet, "/system/hub/nodes/node-a/federation"}, {http.MethodPost, "/system/hub/nodes/node-a/federation/provision"},
	}
	for _, route := range routes {
		var body *ut.Body
		if route.method == http.MethodPost || route.method == http.MethodPut {
			body = &ut.Body{Body: strings.NewReader(`{"nodeCode":"copy-a","nodeName":"Copy","status":1,"discoveryType":"STATIC","managementBaseUrl":"https://node.example.com:9443","hubIssuer":"https://hub.example.com","reason":"test","connectionVersion":"v1","redirectUri":"https://node.example.com/callback"}`), Len: -1}
		}
		response := ut.PerformRequest(engine.Engine, route.method, route.path, body, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Idempotency-Key", Value: "cmd-1"})
		if response.Code == http.StatusNotFound {
			t.Fatalf("route not mounted: %s %s", route.method, route.path)
		}
		payload := response.Body.String()
		for _, forbidden := range []string{"managementBearer", "oidcClientSecret", "ciphertext", "edek", "wrapKeyRef", "plain-secret"} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("%s %s leaked %q: %s", route.method, route.path, forbidden, payload)
			}
		}
	}
}

func TestHubHandlerRejectsUnknownWriteFields(t *testing.T) {
	engine := server.Default()
	New(fakeFacade{}).Mount(engine)
	response := ut.PerformRequest(engine.Engine, http.MethodPost, "/system/hub/nodes", &ut.Body{Body: strings.NewReader(`{"nodeCode":"node-a","unknownSecret":"value"}`), Len: -1}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHubHandlerRejectsSecondTopLevelJSONValue(t *testing.T) {
	engine := server.Default()
	New(fakeFacade{}).Mount(engine)
	// A stray closing delimiter makes Decoder.More report false even though a
	// second top-level value follows; only a second Decode requiring io.EOF is safe.
	body := `{"nodeCode":"node-a"}] {"nodeCode":"node-b"}`
	response := ut.PerformRequest(engine.Engine, http.MethodPost, "/system/hub/nodes", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHubHandlerDoesNotReturnSecretsReflectedByRemoteNode(t *testing.T) {
	const oidcSecret = "oidc-client-secret-plaintext"
	const managementBearer = "management-token"
	transport := reflectedEnvelopeTransport{response: &microservice.ServiceResponse{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{},
		Body:       []byte(`{"code":50201,"data":null,"message":"bad ` + oidcSecret + `","traceId":"trace-` + managementBearer + `"}`),
	}}
	client := hubinfra.NewNodeClient(transport, transport, handlerSecretService{}, nil)
	err := client.Do(context.Background(), hubinfra.NodeTarget{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: "https://node.example.com:9443", ManagementBearer: hubinfra.EncryptedValue{Ciphertext: "cipher"}}, http.MethodPut, "/internal/node/v1/hub-connection", nil, map[string]string{"clientSecret": oidcSecret}, "connect-v1", nil)
	reqCtx := &app.RequestContext{}
	(&Handler{}).write(reqCtx, nil, err)

	body := string(reqCtx.Response.Body())
	if strings.Contains(body, oidcSecret) || strings.Contains(body, managementBearer) {
		t.Fatalf("handler returned reflected outbound secret: %s", body)
	}
}

func TestHubHandlerNormalizesIncoherentRemoteSuccessCode(t *testing.T) {
	transport := reflectedEnvelopeTransport{response: &microservice.ServiceResponse{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{},
		Body:       []byte(`{"code":0,"data":null,"message":"upstream failed","traceId":"remote"}`),
	}}
	client := hubinfra.NewNodeClient(transport, transport, handlerSecretService{}, nil)
	err := client.Do(context.Background(), hubinfra.NodeTarget{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: "https://node.example.com:9443", ManagementBearer: hubinfra.EncryptedValue{Ciphertext: "cipher"}}, http.MethodGet, "/internal/node/v1/descriptor", nil, nil, "", nil)
	reqCtx := &app.RequestContext{}
	(&Handler{}).write(reqCtx, nil, err)
	var result struct {
		Code int `json:"code"`
	}
	if decodeErr := json.Unmarshal(reqCtx.Response.Body(), &result); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if reqCtx.Response.StatusCode() != http.StatusServiceUnavailable || result.Code != apperrors.CodeServiceUnavailable {
		t.Fatalf("status/code=%d/%d body=%s", reqCtx.Response.StatusCode(), result.Code, reqCtx.Response.Body())
	}
}

func TestHubHandlerKeepsTransportFailureBaselineAndCanonicalTrace(t *testing.T) {
	const canonicalTraceID = "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"
	transport := reflectedEnvelopeTransport{err: microservice.ErrNoHealthyInstance}
	client := hubinfra.NewNodeClient(transport, transport, handlerSecretService{}, nil)
	err := client.Do(
		xcontext.WithTraceID(context.Background(), canonicalTraceID),
		hubinfra.NodeTarget{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: "https://node.example.com:9443", ManagementBearer: hubinfra.EncryptedValue{Ciphertext: "cipher"}},
		http.MethodGet,
		"/internal/node/v1/descriptor",
		nil,
		nil,
		"",
		nil,
	)
	reqCtx := &app.RequestContext{}
	reqCtx.Set(xcontext.TraceIDKey, canonicalTraceID)
	(&Handler{}).write(reqCtx, nil, err)
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		TraceID string `json:"traceId"`
	}
	if decodeErr := json.Unmarshal(reqCtx.Response.Body(), &result); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if reqCtx.Response.StatusCode() != http.StatusOK || result.Code != apperrors.CodeSystemError || result.Message != "系统内部异常" || result.TraceID != canonicalTraceID {
		t.Fatalf("status/result=%d/%+v", reqCtx.Response.StatusCode(), result)
	}
}

type reflectedEnvelopeTransport struct {
	response *microservice.ServiceResponse
	err      error
}

func (t reflectedEnvelopeTransport) Do(context.Context, microservice.ServiceRequest) (*microservice.ServiceResponse, error) {
	return t.response, t.err
}

type handlerSecretService struct{}

func (handlerSecretService) EncryptString(context.Context, string) (secretvalueinfra.SecretValue, error) {
	panic("not used")
}
func (handlerSecretService) DecryptString(context.Context, secretvalueinfra.SecretValue) (string, error) {
	return "management-token", nil
}
func (handlerSecretService) EncryptBytes(context.Context, []byte) (secretvalueinfra.SecretValue, error) {
	panic("not used")
}
func (handlerSecretService) DecryptBytes(context.Context, secretvalueinfra.SecretValue) ([]byte, error) {
	panic("not used")
}

type fakeFacade struct{}

func (fakeFacade) PageNodes(context.Context, hubfacade.NodePageQuery) (*hubfacade.NodePage, error) {
	return &hubfacade.NodePage{Current: 1, Size: 20}, nil
}
func (fakeFacade) GetNode(context.Context, string) (*hubfacade.NodeDetail, error) {
	return &hubfacade.NodeDetail{NodeCode: "node-a", ConnectionStatus: domain.ConnectionPending}, nil
}
func (fakeFacade) SaveNode(context.Context, hubfacade.SaveNodeCommand) (*hubfacade.NodeDetail, error) {
	return &hubfacade.NodeDetail{NodeCode: "node-a"}, nil
}
func (fakeFacade) CopyNode(context.Context, string, hubfacade.CopyNodeCommand) (*hubfacade.NodeDetail, error) {
	return &hubfacade.NodeDetail{NodeCode: "copy-a"}, nil
}
func (fakeFacade) SetNodeStatus(context.Context, hubfacade.SetNodeStatusCommand) error { return nil }
func (fakeFacade) TestConnection(context.Context, string) (*hubfacade.NodeHealth, error) {
	return &hubfacade.NodeHealth{Health: "UP"}, nil
}
func (fakeFacade) ListNodeUsers(context.Context, string, nodefacade.UserPageQuery) (*nodefacade.UserPage, error) {
	return &nodefacade.UserPage{}, nil
}
func (fakeFacade) GetNodeUser(context.Context, string, string) (*nodefacade.UserDetail, error) {
	return &nodefacade.UserDetail{UserID: "42"}, nil
}
func (fakeFacade) SetNodeUserStatus(context.Context, hubfacade.NodeUserStatusCommand) error {
	return nil
}
func (fakeFacade) ListNodeUserSessions(context.Context, string, string, nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error) {
	return &nodefacade.SessionPage{}, nil
}
func (fakeFacade) RevokeNodeUserSessions(context.Context, hubfacade.RevokeNodeSessionsCommand) error {
	return nil
}
func (fakeFacade) GetNodeLoginPolicy(context.Context, string) (*nodefacade.ManagedLoginPolicy, error) {
	return &nodefacade.ManagedLoginPolicy{}, nil
}
func (fakeFacade) ApplyNodeLoginPolicy(context.Context, string, nodefacade.ApplyLoginPolicyCommand) error {
	return nil
}
func (fakeFacade) GetFederationStatus(context.Context, string) (*hubfacade.FederationStatus, error) {
	return &hubfacade.FederationStatus{NodeCode: "node-a", ConnectionStatus: domain.ConnectionPending}, nil
}
func (fakeFacade) ProvisionNodeConnection(context.Context, hubfacade.ProvisionConnectionCommand) error {
	return nil
}
