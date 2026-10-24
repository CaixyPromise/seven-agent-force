package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	platformhandler "github.com/CaixyPromise/seven-agent/force/internal/app/platform/handler"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestPlatformAdminRoutesRequirePermission(t *testing.T) {
	module := testPlatformModule(&platformRouteService{})
	engine := platformControlPlaneTestEngine(module, &securitycontext.UserContext{
		UserID:      1001,
		Username:    "admin",
		Permissions: []string{"admin:platform:query"},
		SessionID:   "sid-1",
	})

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/admin/page", nil)
	assertPlatformBusinessCode(t, resp, apperrors.CodeForbidden)
}

func TestPlatformAdminPageRouteWired(t *testing.T) {
	module := testPlatformModule(&platformRouteService{})
	engine := platformControlPlaneTestEngine(module, &securitycontext.UserContext{
		UserID:      1001,
		Username:    "admin",
		Permissions: []string{"system:platform:list"},
		SessionID:   "sid-1",
	})

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/admin/page", nil)
	assertPlatformBusinessCode(t, resp, apperrors.CodeSuccess)
}

func TestPlatformAdminRoutesAreNotMountedOutsideHub(t *testing.T) {
	for _, mode := range []config.PlatformMode{config.PlatformModeLocal, config.PlatformModeNode} {
		t.Run(string(mode), func(t *testing.T) {
			service := &platformRouteService{}
			module := testPlatformModule(service)
			engine := platformModuleTestEngine(module, nil)

			public := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/public/login-options", nil)
			assertPlatformBusinessCode(t, public, apperrors.CodeSuccess)

			missing := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/route-not-registered", nil)
			admin := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/admin/page", nil)
			if admin.Code != missing.Code || admin.Body.String() != missing.Body.String() {
				t.Fatalf("admin route must be unregistered outside hub: admin=(%d, %q) missing=(%d, %q)", admin.Code, admin.Body.String(), missing.Code, missing.Body.String())
			}
		})
	}
}

func TestPlatformAdminSourceResolveRouteWired(t *testing.T) {
	service := &platformRouteService{}
	module := testPlatformModule(service)
	engine := platformControlPlaneTestEngine(module, &securitycontext.UserContext{
		UserID:      1001,
		Username:    "admin",
		Permissions: []string{"system:platform:query"},
		SessionID:   "sid-1",
	})

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/admin/source/resolve?clientId=authorization-console&redirect=http%3A%2F%2F127.0.0.1%3A5291%2Foidc%2Fcallback%2Fauthorization-console&host=127.0.0.1%3A5291&platformCode=seven-admin", nil)
	assertPlatformBusinessCode(t, resp, apperrors.CodeSuccess)
	if service.lastResolve.ClientID != "authorization-console" {
		t.Fatalf("clientId not forwarded: %+v", service.lastResolve)
	}
	if service.lastResolve.TrustedSource.Host != "127.0.0.1:5291" {
		t.Fatalf("host not forwarded: %+v", service.lastResolve.TrustedSource)
	}
	if service.lastResolve.ExplicitCode != "seven-admin" {
		t.Fatalf("explicit platform code not forwarded: %+v", service.lastResolve)
	}
}

func TestPublicLoginOptionsDoesNotTrustQueryClientOrRedirect(t *testing.T) {
	service := &platformRouteService{}
	module := testPlatformModule(service)
	engine := platformModuleTestEngine(module, nil)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/public/login-options?clientId=authorization-console&redirect=https%3A%2F%2Fevil.example%2Fcallback&platformCode=seven-admin", nil)
	assertPlatformBusinessCode(t, resp, apperrors.CodeSuccess)
	if service.lastResolve.ClientID != "authorization-console" || service.lastResolve.RedirectURL != "https://evil.example/callback" {
		t.Fatalf("query candidates not forwarded: %+v", service.lastResolve)
	}
	if service.lastResolve.TrustedSource.ClientID != "" || service.lastResolve.TrustedSource.RedirectURL != "" {
		t.Fatalf("query values leaked into trusted source: %+v", service.lastResolve.TrustedSource)
	}
}

func TestCompatibilityLoginOptionsRouteRemainsMounted(t *testing.T) {
	service := &platformRouteService{}
	module := testPlatformModule(service)
	engine := platformModuleTestEngine(module, nil)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/login-options", nil)
	assertPlatformBusinessCode(t, resp, apperrors.CodeSuccess)
	if !service.resolveCalled {
		t.Fatal("compatibility login options route should resolve the shared policy")
	}
}

func TestPublicLoginOptionsRemainAvailableForAllModes(t *testing.T) {
	for _, mode := range []config.PlatformMode{
		config.PlatformMode("local"),
		config.PlatformMode("hub"),
		config.PlatformMode("node"),
	} {
		t.Run(string(mode), func(t *testing.T) {
			service := &platformRouteService{}
			module := testPlatformModule(service)
			engine := platformModuleTestEngine(module, nil)

			resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/platform/public/login-options", nil)
			assertPlatformBusinessCode(t, resp, apperrors.CodeSuccess)
			if !service.resolveCalled {
				t.Fatalf("platform login policy should resolve local login options in %s mode", mode)
			}
		})
	}
}

func testPlatformModule(service platformhandler.Service) *Module {
	return &Module{handler: platformhandler.NewHandler(service)}
}

func platformModuleTestEngine(module *Module, user *securitycontext.UserContext) *server.Hertz {
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

func platformControlPlaneTestEngine(module *Module, user *securitycontext.UserContext) *server.Hertz {
	engine := platformModuleTestEngine(module, user)
	MountControlPlane(module).Mount(engine)
	return engine
}

func assertPlatformBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, want int) {
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

func assertPlatformMessage(t *testing.T, recorder *ut.ResponseRecorder, want string) {
	t.Helper()
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, recorder.Body.String())
	}
	if body.Message != want {
		t.Fatalf("message=%q want %q body=%s", body.Message, want, recorder.Body.String())
	}
}

func assertPlatformNoErrorSemantics(t *testing.T, recorder *ut.ResponseRecorder) {
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

type platformRouteService struct {
	lastResolve   facade.ResolvePlatformRequest
	listCalled    bool
	resolveCalled bool
}

func (s *platformRouteService) ResolveLoginOptions(_ context.Context, request facade.ResolvePlatformRequest) (*facade.LoginOptionResult, error) {
	s.lastResolve = request
	s.resolveCalled = true
	return &facade.LoginOptionResult{}, nil
}

func (s *platformRouteService) ResolvePlatformCode(_ context.Context, request facade.ResolvePlatformRequest) (string, error) {
	s.lastResolve = request
	return "seven-admin", nil
}

func (s *platformRouteService) ValidateLoginContext(context.Context, string, facade.ResolvePlatformRequest) (*facade.LoginContextValidation, error) {
	return &facade.LoginContextValidation{}, nil
}

func (s *platformRouteService) IssueProvisioningAuthority(context.Context, string, facade.ResolvePlatformRequest) (*facade.ProvisioningAuthority, error) {
	return &facade.ProvisioningAuthority{}, nil
}

func (s *platformRouteService) GetProvisioningPolicy(context.Context, facade.ProvisioningAuthority) (*facade.ProvisioningPolicy, error) {
	return &facade.ProvisioningPolicy{}, nil
}

func (s *platformRouteService) GetFormRegistrationPolicy(context.Context, string) (*facade.ProvisioningPolicy, error) {
	return &facade.ProvisioningPolicy{}, nil
}

func (s *platformRouteService) RequireLoginMethod(context.Context, string, string, string) error {
	return nil
}

func (s *platformRouteService) ListPlatforms(context.Context, facade.PlatformQuery) (*facade.PlatformPage, error) {
	s.listCalled = true
	return &facade.PlatformPage{Records: []facade.PlatformDetail{}, Total: 0, Current: 1, PageSize: 20}, nil
}

func (s *platformRouteService) GetPlatform(context.Context, string) (*facade.PlatformDetail, error) {
	return &facade.PlatformDetail{}, nil
}

func (s *platformRouteService) CreatePlatform(context.Context, int64, facade.PlatformSaveRequest, stepup.ProofMetadata) (*facade.PlatformDetail, error) {
	return &facade.PlatformDetail{}, nil
}

func (s *platformRouteService) UpdatePlatform(context.Context, int64, string, facade.PlatformSaveRequest, stepup.ProofMetadata) (*facade.PlatformDetail, error) {
	return &facade.PlatformDetail{}, nil
}

func (s *platformRouteService) UpdatePlatformStatus(context.Context, int64, string, facade.PlatformStatusRequest, stepup.ProofMetadata) error {
	return nil
}

func (s *platformRouteService) ReplaceLoginMethods(context.Context, int64, string, []facade.LoginMethodSaveRequest, stepup.ProofMetadata) error {
	return nil
}

func (s *platformRouteService) ReplaceSourceRules(context.Context, int64, string, []facade.SourceRuleSaveRequest, stepup.ProofMetadata) error {
	return nil
}

func (s *platformRouteService) ReplaceDefaultRoles(context.Context, int64, string, []facade.DefaultRoleSaveRequest, stepup.ProofMetadata) error {
	return nil
}
