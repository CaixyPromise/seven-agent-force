package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
)

func TestPlatformModeRouteMatrix(t *testing.T) {
	cases := []struct {
		mode       config.PlatformMode
		hubStatus  int
		nodeStatus int
	}{
		{mode: config.PlatformModeLocal, hubStatus: http.StatusNotFound, nodeStatus: http.StatusNotFound},
		{mode: config.PlatformModeHub, hubStatus: http.StatusUnauthorized, nodeStatus: http.StatusNotFound},
		{mode: config.PlatformModeNode, hubStatus: http.StatusNotFound, nodeStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			engine, probes := newModeServerWithFakeRoleMounters(tc.mode)

			for _, route := range hubRouteInventory {
				assertModeRouteStatus(t, engine, route.method, route.path, tc.hubStatus)
				if tc.hubStatus == http.StatusNotFound {
					probes.hub.assertNotInvoked(t, route)
				}
			}
			for _, route := range nodeManagementRouteInventory {
				assertModeRouteStatus(t, engine, route.method, route.path, tc.nodeStatus)
				if tc.nodeStatus == http.StatusNotFound {
					probes.node.assertNotInvoked(t, route)
				}
			}

			sso := ut.PerformRequest(engine.Engine, http.MethodGet, "/api/sso/.well-known/openid-configuration", nil)
			if sso.Code == http.StatusNotFound {
				t.Fatal("SSO provider discovery must remain available in every platform mode")
			}
			for _, route := range []modeRoute{
				{method: http.MethodPost, path: "/api/login/password/state"},
				{method: http.MethodGet, path: "/api/login/external/github/start"},
				{method: http.MethodGet, path: "/api/login/external/google/start"},
				{method: http.MethodGet, path: "/api/login/enterprise/options?org=example"},
				{method: http.MethodPost, path: "/api/login/enterprise/FEISHU/authorizations"},
				{method: http.MethodGet, path: "/api/login/enterprise/FEISHU/callback?code=invalid&state=invalid"},
			} {
				response := ut.PerformRequest(engine.Engine, route.method, route.path, nil)
				if response.Code == http.StatusNotFound {
					t.Fatalf("public login route must remain available: %s %s", route.method, route.path)
				}
			}

		})
	}
}

func TestNodeInternalListenerMountsNodeRoutesOnlyOnDedicatedServer(t *testing.T) {
	listener := listenLoopback(t)
	listenAddress := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release listener reservation: %v", err)
	}
	cfg := config.Config{
		Server: config.ServerConfig{ContextPath: "/api"},
		Platform: config.PlatformConfig{
			Mode: config.PlatformModeNode,
			Node: config.PlatformNodeConfig{
				ManagementBearer: "node-bearer",
				InternalListener: config.PlatformNodeInternalListenerConfig{
					Enabled: true,
					Listen:  listenAddress,
				},
			},
		},
	}
	probes := &modeRouteProbes{}
	modules := []core.Module{fakeNodeRoleMounter{probes: probes}}
	internal, err := configureInternalServer(cfg, modules)
	if err != nil {
		t.Fatalf("configure internal listener: %v", err)
	}
	if internal == nil {
		t.Fatal("enabled node internal listener must be configured")
	}

	primary := newModeTestEngine()
	app := &App{config: cfg, internal: internal, registry: NewRegistry()}
	app.registerModules(primary.Engine, modules)
	assertModeRouteStatus(t, primary, http.MethodGet, "/internal/node/v1/descriptor", http.StatusNotFound)

	if err := internal.Start(); err != nil {
		t.Fatalf("start internal listener: %v", err)
	}
	internalURL := "http://" + internal.engine.GetOptions().Addr
	waitForHTTPStatus(t, internalURL+"/internal/node/v1/descriptor", http.StatusUnauthorized)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := internal.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown internal listener: %v", err)
	}
	waitForConnectionRefused(t, internal.engine.GetOptions().Addr)
}

func TestNodeInternalListenerDisabledMountsNodeRoutesOnRawPrimaryEngine(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{ContextPath: "/api"},
		Platform: config.PlatformConfig{
			Mode: config.PlatformModeNode,
			Node: config.PlatformNodeConfig{ManagementBearer: "node-bearer"},
		},
	}
	probes := &modeRouteProbes{}
	modules := []core.Module{fakeNodeRoleMounter{probes: probes}}
	internal, err := configureInternalServer(cfg, modules)
	if err != nil {
		t.Fatalf("configure disabled internal listener: %v", err)
	}
	if internal != nil {
		t.Fatal("disabled node internal listener must not create a dedicated server")
	}

	primary := newModeTestEngine()
	app := &App{config: cfg, registry: NewRegistry()}
	app.registerModules(primary.Engine, modules)
	assertModeRouteStatus(t, primary, http.MethodGet, "/internal/node/v1/descriptor", http.StatusUnauthorized)
}

func TestNodeInternalListenerBindFailureIsReturnedDuringConfiguration(t *testing.T) {
	listener := listenLoopback(t)
	cfg := config.Config{
		Platform: config.PlatformConfig{
			Mode: config.PlatformModeNode,
			Node: config.PlatformNodeConfig{
				ManagementBearer: "node-bearer",
				InternalListener: config.PlatformNodeInternalListenerConfig{
					Enabled: true,
					Listen:  listener.Addr().String(),
				},
			},
		},
	}
	internal, err := configureInternalServer(cfg, []core.Module{fakeNodeRoleMounter{probes: &modeRouteProbes{}}})
	if err == nil || internal != nil {
		t.Fatalf("bind failure must be returned during configuration: internal=%v err=%v", internal, err)
	}
}

func TestPlatformRoleMountersKeepPublicRoutesAndMiddleware(t *testing.T) {
	for _, mode := range []config.PlatformMode{
		config.PlatformModeLocal,
		config.PlatformModeHub,
		config.PlatformModeNode,
	} {
		t.Run(string(mode), func(t *testing.T) {
			engine := newModeTestEngine()
			composite := &fakeCompositePublicHubModule{}
			app := &App{
				config:   config.Config{Server: config.ServerConfig{ContextPath: "/api"}, Platform: config.PlatformConfig{Mode: mode}},
				registry: NewRegistry(),
			}
			app.registerModules(engine.Engine, []core.Module{composite})

			assertModeRouteStatus(t, engine, http.MethodGet, "/api/composite/public", http.StatusNoContent)
			if composite.middlewareCalls == 0 {
				t.Fatal("middleware from a public-plus-role module must be installed")
			}
			if mode == config.PlatformModeHub {
				assertModeRouteStatus(t, engine, http.MethodGet, "/api/composite/hub", http.StatusNoContent)
			} else {
				assertModeRouteStatus(t, engine, http.MethodGet, "/api/composite/hub", http.StatusNotFound)
			}
			if composite.mountCalls != 0 {
				t.Fatalf("role module must not fall back to Module.Mount: %d", composite.mountCalls)
			}
		})
	}
}

func TestRoleOnlyMounterDoesNotFallBackToModuleMount(t *testing.T) {
	for _, mode := range []config.PlatformMode{config.PlatformModeLocal, config.PlatformModeHub} {
		t.Run(string(mode), func(t *testing.T) {
			engine := newModeTestEngine()
			roleOnly := &fakeRoleOnlyHubModule{}
			app := &App{
				config:   config.Config{Server: config.ServerConfig{ContextPath: "/api"}, Platform: config.PlatformConfig{Mode: mode}},
				registry: NewRegistry(),
			}
			app.registerModules(engine.Engine, []core.Module{roleOnly})

			assertModeRouteStatus(t, engine, http.MethodGet, "/api/role-only/fallback", http.StatusNotFound)
			if roleOnly.mountCalls != 0 {
				t.Fatalf("role-only module fell back to Module.Mount: %d", roleOnly.mountCalls)
			}
			if mode == config.PlatformModeHub {
				assertModeRouteStatus(t, engine, http.MethodGet, "/api/role-only/hub", http.StatusNoContent)
			} else {
				assertModeRouteStatus(t, engine, http.MethodGet, "/api/role-only/hub", http.StatusNotFound)
			}
		})
	}
}

type modeRoute struct {
	method string
	path   string
}

var nodeManagementRouteInventory = []modeRoute{
	{method: http.MethodGet, path: "/internal/node/v1/descriptor"},
	{method: http.MethodGet, path: "/internal/node/v1/users"},
	{method: http.MethodGet, path: "/internal/node/v1/users/1001"},
	{method: http.MethodPut, path: "/internal/node/v1/users/1001/status"},
	{method: http.MethodGet, path: "/internal/node/v1/users/1001/sessions"},
	{method: http.MethodPost, path: "/internal/node/v1/users/1001/sessions/revoke"},
	{method: http.MethodGet, path: "/internal/node/v1/login-policy"},
	{method: http.MethodPost, path: "/internal/node/v1/login-policy/apply"},
	{method: http.MethodPut, path: "/internal/node/v1/hub-connection"},
}

var hubRouteInventory = []modeRoute{
	{method: http.MethodGet, path: "/api/platform/admin/page"},
	{method: http.MethodGet, path: "/api/platform/admin/test-platform"},
	{method: http.MethodPost, path: "/api/platform/admin"},
	{method: http.MethodPut, path: "/api/platform/admin/test-platform"},
	{method: http.MethodPut, path: "/api/platform/admin/test-platform/status"},
	{method: http.MethodPut, path: "/api/platform/admin/test-platform/login-methods"},
	{method: http.MethodPut, path: "/api/platform/admin/test-platform/source-rules"},
	{method: http.MethodPut, path: "/api/platform/admin/test-platform/default-roles"},
	{method: http.MethodGet, path: "/api/platform/admin/source/resolve"},
	{method: http.MethodGet, path: "/api/platform/admin/platforms"},
	{method: http.MethodGet, path: "/api/platform/admin/platforms/test-platform"},
	{method: http.MethodPost, path: "/api/platform/admin/platforms"},
	{method: http.MethodPut, path: "/api/platform/admin/platforms/test-platform"},
	{method: http.MethodPut, path: "/api/platform/admin/platforms/test-platform/status"},
	{method: http.MethodPut, path: "/api/platform/admin/platforms/test-platform/login-methods"},
	{method: http.MethodPut, path: "/api/platform/admin/platforms/test-platform/source-rules"},
	{method: http.MethodPut, path: "/api/platform/admin/platforms/test-platform/default-roles"},
	{method: http.MethodGet, path: "/api/platform/admin/platforms/source/resolve"},
	{method: http.MethodGet, path: "/api/system/hub/nodes"},
	{method: http.MethodPost, path: "/api/system/hub/nodes"},
	{method: http.MethodGet, path: "/api/system/hub/nodes/test-node"},
	{method: http.MethodPut, path: "/api/system/hub/nodes/test-node"},
	{method: http.MethodPost, path: "/api/system/hub/nodes/test-node/copy"},
	{method: http.MethodPut, path: "/api/system/hub/nodes/test-node/status"},
	{method: http.MethodPost, path: "/api/system/hub/nodes/test-node/connection-test"},
	{method: http.MethodGet, path: "/api/system/hub/nodes/test-node/users"},
	{method: http.MethodGet, path: "/api/system/hub/nodes/test-node/users/test-user"},
	{method: http.MethodPut, path: "/api/system/hub/nodes/test-node/users/test-user/status"},
	{method: http.MethodGet, path: "/api/system/hub/nodes/test-node/users/test-user/sessions"},
	{method: http.MethodPost, path: "/api/system/hub/nodes/test-node/users/test-user/sessions/revoke"},
	{method: http.MethodGet, path: "/api/system/hub/nodes/test-node/login-policy"},
	{method: http.MethodPost, path: "/api/system/hub/nodes/test-node/login-policy/apply"},
	{method: http.MethodGet, path: "/api/system/hub/nodes/test-node/federation"},
	{method: http.MethodPost, path: "/api/system/hub/nodes/test-node/federation/provision"},
}

type modeRouteProbes struct {
	hub  routeProbe
	node routeProbe
}

type routeProbe struct {
	auth       int
	permission int
	audit      int
	terminal   int
}

func (p routeProbe) assertNotInvoked(t *testing.T, route modeRoute) {
	t.Helper()
	if p.auth != 0 || p.permission != 0 || p.audit != 0 || p.terminal != 0 {
		t.Fatalf(
			"unregistered role route entered its auth/permission/audit/terminal chain: %s %s auth=%d permission=%d audit=%d terminal=%d",
			route.method,
			route.path,
			p.auth,
			p.permission,
			p.audit,
			p.terminal,
		)
	}
}

func newModeServerWithFakeRoleMounters(mode config.PlatformMode) (*server.Hertz, *modeRouteProbes) {
	probes := &modeRouteProbes{}
	engine := newModeTestEngine()

	app := &App{
		config: config.Config{
			Server:   config.ServerConfig{ContextPath: "/api"},
			Platform: config.PlatformConfig{Mode: mode},
		},
		registry: NewRegistry(),
	}
	app.registerModules(engine.Engine, []core.Module{
		fakePublicModule{},
		fakeHubRoleMounter{probes: probes},
		fakeNodeRoleMounter{probes: probes},
	})
	return engine, probes
}

func newModeTestEngine() *server.Hertz {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		xcontext.EnsureTraceID(reqCtx)
		reqCtx.Next(ctx)
	})
	engine.NoRoute(func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Error(reqCtx, apperrors.NotFound("请求路径不存在"))
	})
	return engine
}

func assertModeRouteStatus(t *testing.T, engine *server.Hertz, method, path string, want int) {
	t.Helper()
	resp := ut.PerformRequest(engine.Engine, method, path, nil)
	if resp.Code != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, resp.Code, want, resp.Body.String())
	}
	if want == http.StatusNotFound {
		assertUniformNotFound(t, resp)
	}
}

func assertUniformNotFound(t *testing.T, resp *ut.ResponseRecorder) {
	t.Helper()
	var body struct {
		Code    int    `json:"code"`
		Data    any    `json:"data"`
		Message string `json:"message"`
		TraceID string `json:"traceId"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal 404 envelope: %v body=%s", err, resp.Body.String())
	}
	if body.Code != apperrors.CodeNotFound || body.Data != nil || body.Message != "请求路径不存在" || body.TraceID == "" {
		t.Fatalf("unexpected 404 envelope: %+v", body)
	}
}

type fakePublicModule struct{}

func (fakePublicModule) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "fake-sso-provider", Prefix: "/sso"}
}

func (fakePublicModule) Mount(router route.IRouter) {
	fakePublicModule{}.MountPublic(router)
}

func (fakePublicModule) MountPublic(router route.IRouter) {
	router.GET("/sso/.well-known/openid-configuration", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
	router.POST("/login/password/state", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
	router.GET("/login/external/:providerCode/start", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
	router.GET("/login/enterprise/options", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
	router.POST("/login/enterprise/:provider/authorizations", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
	router.GET("/login/enterprise/:provider/callback", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
}

type fakeHubRoleMounter struct {
	probes *modeRouteProbes
}

func (m fakeHubRoleMounter) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "fake-hub-control", Prefix: "/system/hub"}
}

func (m fakeHubRoleMounter) Mount(router route.IRouter) {
	m.MountHub(router)
}

func (m fakeHubRoleMounter) MountHub(router route.IRouter) {
	protected := router.Group("")
	protected.Use(roleChainMiddleware(&m.probes.hub.auth))
	protected.Use(roleChainMiddleware(&m.probes.hub.permission))
	protected.Use(roleChainMiddleware(&m.probes.hub.audit))
	for _, item := range hubRouteInventory {
		route := item
		protected.Handle(route.method, strings.TrimPrefix(route.path, "/api"), func(ctx context.Context, reqCtx *app.RequestContext) {
			m.probes.hub.terminal++
			reqCtx.Status(http.StatusUnauthorized)
		})
	}
}

type fakeNodeRoleMounter struct {
	probes *modeRouteProbes
}

func (m fakeNodeRoleMounter) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "fake-node-management", Prefix: "/internal/node/v1"}
}

func (m fakeNodeRoleMounter) Mount(router route.IRouter) {
	m.MountInternal(router)
}

func (m fakeNodeRoleMounter) MountInternal(router route.IRouter) {
	protected := router.Group("")
	protected.Use(roleChainMiddleware(&m.probes.node.auth))
	protected.Use(roleChainMiddleware(&m.probes.node.permission))
	protected.Use(roleChainMiddleware(&m.probes.node.audit))
	for _, item := range nodeManagementRouteInventory {
		route := item
		protected.Handle(route.method, route.path, func(ctx context.Context, reqCtx *app.RequestContext) {
			m.probes.node.terminal++
			reqCtx.Status(http.StatusUnauthorized)
		})
	}
}

func roleChainMiddleware(counter *int) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		(*counter)++
		reqCtx.Next(ctx)
	}
}

type fakeCompositePublicHubModule struct {
	middlewareCalls int
	mountCalls      int
}

type fakeRoleOnlyHubModule struct {
	mountCalls int
}

func (*fakeRoleOnlyHubModule) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "fake-role-only-hub", Prefix: "/role-only"}
}

func (m *fakeRoleOnlyHubModule) Mount(router route.IRouter) {
	m.mountCalls++
	router.GET("/role-only/fallback", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
}

func (*fakeRoleOnlyHubModule) MountHub(router route.IRouter) {
	router.GET("/role-only/hub", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
}

func (*fakeCompositePublicHubModule) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "fake-composite-public-hub", Prefix: "/composite"}
}

func (m *fakeCompositePublicHubModule) Mount(router route.IRouter) {
	m.mountCalls++
	router.GET("/composite/fallback", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
}

func (m *fakeCompositePublicHubModule) MountPublic(router route.IRouter) {
	router.GET("/composite/public", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
}

func (m *fakeCompositePublicHubModule) MountHub(router route.IRouter) {
	router.GET("/composite/hub", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
}

func (m *fakeCompositePublicHubModule) Middlewares() []app.HandlerFunc {
	return []app.HandlerFunc{func(ctx context.Context, reqCtx *app.RequestContext) {
		m.middlewareCalls++
		reqCtx.Next(ctx)
	}}
}
