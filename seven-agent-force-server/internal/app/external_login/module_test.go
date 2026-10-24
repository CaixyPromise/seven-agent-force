package external_login

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/handler"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	microserviceinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestExternalLoginModuleMountPublicRouteAllowsAnonymous(t *testing.T) {
	module := testExternalLoginModule(&moduleRouteService{
		methods: []facade.LoginMethodRecord{{ProviderCode: "github", DisplayName: "GitHub"}},
	})
	engine := externalLoginModuleTestEngine(module, nil)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/providers", nil)
	assertModuleBusinessCode(t, resp, apperrors.CodeSuccess)
}

func TestExternalLoginModuleKeepsOrdinaryOAuthAvailable(t *testing.T) {
	service := &moduleRouteService{
		methods: []facade.LoginMethodRecord{{ProviderCode: "github", DisplayName: "GitHub"}},
	}
	module := testExternalLoginModule(service)
	engine := externalLoginModuleTestEngine(module, nil)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/providers", nil)
	assertModuleBusinessCode(t, resp, apperrors.CodeSuccess)
	if service.listMethodCalls != 1 {
		t.Fatalf("ordinary external login should remain available, got calls=%d", service.listMethodCalls)
	}
}

func TestExternalLoginModuleDoesNotPublishEnterpriseLoginRoutes(t *testing.T) {
	module := testExternalLoginModule(&moduleRouteService{})
	engine := externalLoginModuleTestEngine(module, &securitycontext.UserContext{
		UserID:      1001,
		Permissions: []string{"system:enterprise-identity:login-access"},
	})

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/login/enterprise/options?org=acme"},
		{http.MethodPost, "/login/enterprise/FEISHU/authorizations"},
		{http.MethodGet, "/external-identities/login/options?org=acme"},
		{http.MethodPost, "/external-identities/login/FEISHU/authorizations"},
		{http.MethodPut, "/system/enterprise-identities/1/login-access"},
	} {
		resp := ut.PerformRequest(engine.Engine, testCase.method, testCase.path, nil)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s %s should be unpublished, status=%d body=%s", testCase.method, testCase.path, resp.Code, resp.Body.String())
		}
	}
}

func TestOIDCOutboundRoundTripperRejectsPrivateResolution(t *testing.T) {
	policy, err := microserviceinfra.NewOutboundTrustPolicy(microserviceinfra.OutboundTrustConfig{}, staticIPResolver{ip: net.ParseIP("169.254.169.254")})
	if err != nil {
		t.Fatalf("build outbound policy: %v", err)
	}
	client := microserviceinfra.NewHTTPServiceClient(nil, microserviceinfra.NewRoundRobin(), microserviceinfra.HTTPClientOptions{OutboundPolicy: policy})
	request, err := http.NewRequest(http.MethodGet, "https://metadata.internal/.well-known/openid-configuration", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if _, err := (oidcOutboundRoundTripper{client: client}).RoundTrip(request); err == nil {
		t.Fatal("OIDC outbound transport accepted private metadata target")
	}
}

func TestOIDCOutboundRoundTripperDoesNotPropagateFederationTraceHeaders(t *testing.T) {
	var captured http.Header
	client := microserviceinfra.NewHTTPServiceClient(nil, microserviceinfra.NewRoundRobin(), microserviceinfra.HTTPClientOptions{
		HTTPClient: &http.Client{Transport: externalRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = request.Header.Clone()
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
		})},
	})
	ctx := xcontext.WithTraceID(context.Background(), "abababababababababababababababab")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://issuer.example/userinfo", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (oidcOutboundRoundTripper{client: client}).RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	for _, key := range []string{"traceparent", "tracestate", xcontext.TraceIDHeader, "baggage"} {
		if value := captured.Get(key); value != "" {
			t.Fatalf("external OIDC request leaked %s=%q", key, value)
		}
	}
}

type externalRoundTripFunc func(*http.Request) (*http.Response, error)

func (f externalRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type staticIPResolver struct{ ip net.IP }

func (r staticIPResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: r.ip}}, nil
}

func TestExternalLoginModuleMountAdminRouteRejectsAnonymous(t *testing.T) {
	module := testExternalLoginModule(&moduleRouteService{})
	engine := externalLoginModuleTestEngine(module, nil)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/external-login/admin/providers", nil)
	assertModuleBusinessCode(t, resp, apperrors.CodeNotLogin)
}

func TestExternalLoginModuleMountAdminRouteRejectsMissingPermission(t *testing.T) {
	module := testExternalLoginModule(&moduleRouteService{})
	engine := externalLoginModuleTestEngine(module, &securitycontext.UserContext{
		UserID:      1001,
		Username:    "admin",
		Permissions: []string{"system:external-login-provider:query"},
		SessionID:   "sid-1",
	})

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/external-login/admin/providers", nil)
	assertModuleBusinessCode(t, resp, apperrors.CodeForbidden)
}

func TestExternalLoginModuleMountClientSecretRotateUsesContractPath(t *testing.T) {
	service := &moduleRouteService{}
	module := testExternalLoginModule(service)
	engine := externalLoginModuleTestEngine(module, &securitycontext.UserContext{
		UserID:      1001,
		Username:    "admin",
		Permissions: []string{"system:external-login-provider:secret:rotate"},
		SessionID:   "sid-1",
	})

	resp := ut.PerformRequest(engine.Engine, http.MethodPost, "/external-login/admin/providers/github/client-secret/rotate", nil)
	if resp.Code == http.StatusNotFound {
		t.Fatalf("contract client-secret rotate path returned 404 body=%s", resp.Body.String())
	}

	legacy := ut.PerformRequest(engine.Engine, http.MethodPost, "/external-login/admin/providers/github/secret/rotate", nil)
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("legacy secret rotate path status=%d want 404 body=%s", legacy.Code, legacy.Body.String())
	}
}

func testExternalLoginModule(service handler.Service) *Module {
	return &Module{handler: handler.NewHandler(service)}
}

func externalLoginModuleTestEngine(module *Module, user *securitycontext.UserContext) *server.Hertz {
	engine := server.Default()
	if user != nil {
		engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
			securitycontext.Set(reqCtx, user)
			reqCtx.Next(ctx)
		})
	}
	module.Mount(engine)
	return engine
}

func assertModuleBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, want int) {
	t.Helper()
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, recorder.Body.String())
	}
	if body.Code != want {
		t.Fatalf("business code=%d want=%d body=%s", body.Code, want, recorder.Body.String())
	}
}

func assertModuleNoErrorSemantics(t *testing.T, recorder *ut.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, recorder.Body.String())
	}
	if _, ok := body["errorType"]; ok {
		t.Fatalf("response must omit errorType body=%s", recorder.Body.String())
	}
	if _, ok := body["errorCode"]; ok {
		t.Fatalf("response must omit errorCode body=%s", recorder.Body.String())
	}
}

type moduleRouteService struct {
	methods         []facade.LoginMethodRecord
	listMethodCalls int
}

func (s *moduleRouteService) ListLoginMethods(context.Context, facade.ListLoginMethodsRequest) ([]facade.LoginMethodRecord, error) {
	s.listMethodCalls++
	return s.methods, nil
}

func (s *moduleRouteService) StartExternalLogin(context.Context, facade.StartExternalLoginRequest) (*facade.StartExternalLoginResult, error) {
	return &facade.StartExternalLoginResult{}, nil
}

func (s *moduleRouteService) CompleteExternalCallback(context.Context, facade.CompleteExternalCallbackRequest) (*facade.ExternalLoginResult, error) {
	return &facade.ExternalLoginResult{}, nil
}

func (s *moduleRouteService) ProviderCapabilities(context.Context) facade.ProviderCapabilityCatalog {
	return nil
}

func (s *moduleRouteService) ListProviders(context.Context, facade.ProviderQuery) (*facade.ProviderPage, error) {
	return &facade.ProviderPage{}, nil
}

func (s *moduleRouteService) GetProvider(context.Context, string) (*facade.ProviderDetail, error) {
	return &facade.ProviderDetail{}, nil
}

func (s *moduleRouteService) CreateProvider(context.Context, int64, facade.ProviderSaveRequest, stepup.ProofMetadata) (*facade.ProviderDetail, error) {
	return &facade.ProviderDetail{}, nil
}

func (s *moduleRouteService) UpdateProvider(context.Context, int64, string, facade.ProviderUpdateRequest, stepup.ProofMetadata) (*facade.ProviderDetail, error) {
	return &facade.ProviderDetail{}, nil
}

func (s *moduleRouteService) UpdateProviderStatus(context.Context, int64, string, facade.ProviderStatusRequest, stepup.ProofMetadata) error {
	return nil
}

func (s *moduleRouteService) RotateClientSecret(context.Context, int64, string, facade.RotateClientSecretRequest, stepup.ProofMetadata) error {
	return nil
}

func (s *moduleRouteService) ListIdentities(context.Context, facade.IdentityQuery) (*facade.IdentityPage, error) {
	return &facade.IdentityPage{}, nil
}

func (s *moduleRouteService) ListCurrentUserBindings(context.Context, int64) ([]facade.CurrentUserBinding, error) {
	return nil, nil
}

func (s *moduleRouteService) UpdateIdentityStatus(context.Context, int64, int64, facade.IdentityStatusRequest, stepup.ProofMetadata) error {
	return nil
}

func (s *moduleRouteService) ResolveIdentity(context.Context, string, string) (*facade.ExternalIdentityRecord, error) {
	return &facade.ExternalIdentityRecord{}, nil
}

func (s *moduleRouteService) ListTokens(context.Context, facade.TokenQuery) (*facade.TokenPage, error) {
	return &facade.TokenPage{}, nil
}

func (s *moduleRouteService) AcquireAccessToken(context.Context, facade.AcquireAccessTokenRequest) (*facade.AccessTokenLease, error) {
	return &facade.AccessTokenLease{}, nil
}

func (s *moduleRouteService) RefreshToken(context.Context, int64) error {
	return nil
}

func (s *moduleRouteService) RevokeToken(context.Context, int64, int64, string, stepup.ProofMetadata) error {
	return nil
}

func (s *moduleRouteService) RevokeTokensByProvider(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (s *moduleRouteService) RevokeTokensByIdentity(context.Context, int64, string) (int64, error) {
	return 0, nil
}

func (s *moduleRouteService) ListProviderCapabilities(context.Context) facade.ProviderCapabilityCatalog {
	return nil
}

func (s *moduleRouteService) ListProviderMethods(context.Context, string) ([]facade.ProviderMethodDescriptor, error) {
	return nil, nil
}
