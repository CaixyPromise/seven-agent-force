package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/network/standard"
	"github.com/cloudwego/hertz/pkg/route"
	"go.uber.org/zap"
)

func TestAppRegistersKernelModuleAndRoutes(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: test-app
server:
  host: 127.0.0.1
  port: 9999
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
observability:
  prometheus:
    accessToken: test-ops-token
`)

	app, err := New(dir)
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}

	engine := app.Engine()
	if engine == nil {
		t.Fatal("expected engine")
	}

	modulesResp := ut.PerformRequest(engine, "GET", "/ops/modules", nil)
	if modulesResp.Code != 200 {
		t.Fatalf("unexpected modules status: %d", modulesResp.Code)
	}

	var result response.Result
	if err := json.Unmarshal(modulesResp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal modules response: %v", err)
	}
	if result.Code != 40100 {
		t.Fatalf("expected unauthenticated modules result code 40100, got %d", result.Code)
	}

	pingResp := ut.PerformRequest(engine, "GET", "/ping", nil)
	if pingResp.Code != 200 {
		t.Fatalf("unexpected ping status: %d", pingResp.Code)
	}

	healthResp := ut.PerformRequest(engine, "GET", "/healthz", nil)
	if healthResp.Code != 200 {
		t.Fatalf("unexpected health status: %d", healthResp.Code)
	}
	if err := json.Unmarshal(healthResp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal health response: %v", err)
	}
	healthBytes, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal health nested data: %v", err)
	}
	var health map[string]any
	if err := json.Unmarshal(healthBytes, &health); err != nil {
		t.Fatalf("decode health data: %v", err)
	}
	cacheData, ok := health["cache"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected cache health payload: %+v", health["cache"])
	}
	if cacheData["enabled"] != true {
		t.Fatalf("unexpected cache enabled value: %+v", cacheData["enabled"])
	}
	obsData, ok := health["observability"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected observability health payload: %+v", health["observability"])
	}
	promData, ok := obsData["prometheus"].(map[string]any)
	if !ok || promData["enabled"] != true {
		t.Fatalf("unexpected prometheus health payload: %+v", obsData)
	}
	promResp := ut.PerformRequest(engine, "GET", "/ops/prometheus", nil)
	if promResp.Code != 401 {
		t.Fatalf("expected unauthenticated prometheus status 401, got %d", promResp.Code)
	}
	promResp = ut.PerformRequest(engine, "GET", "/ops/prometheus", nil, ut.Header{Key: "Authorization", Value: "Bearer test-ops-token"})
	if promResp.Code != 200 {
		t.Fatalf("unexpected authenticated prometheus status: %d", promResp.Code)
	}
}

func TestAppModuleLifecycleInvokesShutdownHooks(t *testing.T) {
	module := &shutdownTrackingModule{}
	application := &App{modules: []core.Module{module}}
	application.shutdownModules(context.Background())
	if module.calls != 1 {
		t.Fatalf("shutdown calls=%d want 1", module.calls)
	}
}

type shutdownTrackingModule struct{ calls int }

func (*shutdownTrackingModule) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "shutdown-tracking"}
}
func (*shutdownTrackingModule) Mount(route.IRouter) {}
func (m *shutdownTrackingModule) Shutdown(context.Context) error {
	m.calls++
	return nil
}

var _ core.Module = (*shutdownTrackingModule)(nil)
var _ core.ShutdownHook = (*shutdownTrackingModule)(nil)

func TestAppMountsRoutesUnderContextPath(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: test-app
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
`)

	app, err := New(dir)
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}

	engine := app.Engine()
	if engine == nil {
		t.Fatal("expected engine")
	}

	rootResp := ut.PerformRequest(engine, "GET", "/ping", nil)
	if rootResp.Code == 200 {
		t.Fatal("expected root route to be unavailable when context path is configured")
	}

	pingResp := ut.PerformRequest(engine, "GET", "/api/ping", nil)
	if pingResp.Code != 200 {
		t.Fatalf("unexpected context path ping status: %d", pingResp.Code)
	}

	modulesResp := ut.PerformRequest(engine, "GET", "/api/ops/modules", nil)
	if modulesResp.Code != 200 {
		t.Fatalf("unexpected context path modules status: %d", modulesResp.Code)
	}
	var result response.Result
	if err := json.Unmarshal(modulesResp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal context path modules response: %v", err)
	}
	if result.Code != 40100 {
		t.Fatalf("expected unauthenticated context path modules result code 40100, got %d", result.Code)
	}
}

func TestAppNewRejectsEnabledPlatformNodeInternalListenerWithoutMounters(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: node-internal-listener-test
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
platform:
  mode: node
  node:
    managementBearer: node-bearer
    internalListener:
      enabled: true
      listen: 127.0.0.1:0
`)

	app, err := New(dir)
	if err == nil || app != nil {
		t.Fatalf("enabled listener without Node mounters must fail App.New: app=%v err=%v", app, err)
	}
}

func TestAppRunReleasesBoundInternalListenerWhenEarlyStartupFails(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*App)
	}{
		{
			name: "observability",
			configure: func(app *App) {
				app.obsStart = func(context.Context) error { return errors.New("observability startup failed") }
			},
		},
		{
			name: "scheduler",
			configure: func(app *App) {
				app.jobsStart = func(context.Context) error { return errors.New("scheduler startup failed") }
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listener := listenLoopback(t)
			app := &App{logger: zap.NewNop(), internal: &internalServer{listener: listener}}
			tc.configure(app)
			if err := app.Run(); err == nil {
				t.Fatal("early startup failure must be returned")
			}
			waitForConnectionRefused(t, listener.Addr().String())
		})
	}
}

func TestAppRunReturnsPostReadinessInternalServerFailure(t *testing.T) {
	primaryListener := listenLoopback(t)
	primary := server.New(
		server.WithListener(primaryListener),
		server.WithTransport(standard.NewTransporter),
	)
	internalListener := listenLoopback(t)
	releaseInternal := make(chan struct{})
	internal := &internalServer{
		listener: internalListener,
		run: func() error {
			<-releaseInternal
			return errors.New("internal node server failed")
		},
		isRunning: func() bool { return true },
	}
	app := &App{
		logger:     zap.NewNop(),
		httpServer: primary,
		internal:   internal,
	}
	result := make(chan error, 1)
	go func() { result <- app.Run() }()
	waitForHertzRunning(t, primary)
	close(releaseInternal)
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "internal node server failed") {
			t.Fatalf("App.Run error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("App.Run did not return after post-readiness internal failure")
	}
	waitForConnectionRefused(t, primaryListener.Addr().String())
	waitForConnectionRefused(t, internalListener.Addr().String())
}

func TestInternalServerMonitorIgnoresCoordinatedShutdown(t *testing.T) {
	listener := listenLoopback(t)
	releaseInternal := make(chan struct{})
	internal := &internalServer{
		listener: listener,
		run: func() error {
			<-releaseInternal
			return nil
		},
		isRunning: func() bool { return true },
	}
	if err := internal.Start(); err != nil {
		t.Fatalf("start internal server: %v", err)
	}
	app := &App{internal: internal}
	failures := app.monitorInternalServer()
	if err := internal.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown internal server: %v", err)
	}
	close(releaseInternal)
	select {
	case err := <-failures:
		t.Fatalf("coordinated shutdown was misclassified as failure: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForHertzRunning(t *testing.T, instance *server.Hertz) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if instance.IsRunning() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Hertz server did not become ready")
}

func TestAppNewKeepsTask2PlatformAdminRoutesAbsentOutsideHub(t *testing.T) {
	for _, mode := range []string{"local", "node"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			platformNode := ""
			if mode == "node" {
				platformNode = "\n  node:\n    code: platform-node-fixture\n    managementBearer: node-bearer\n"
			}
			writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: platform-route-absence-test
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
platform:
  mode: `+mode+platformNode)

			app, err := New(dir)
			if err != nil {
				t.Fatalf("bootstrap %s app: %v", mode, err)
			}
			admin := ut.PerformRequest(app.Engine(), "GET", "/api/platform/admin/page", nil)
			if admin.Code != 404 {
				t.Fatalf("platform admin route status=%d want=404 body=%s", admin.Code, admin.Body.String())
			}
			var result response.Result
			if err := json.Unmarshal(admin.Body.Bytes(), &result); err != nil {
				t.Fatalf("unmarshal platform admin 404: %v", err)
			}
			if result.Code != 40400 || result.Message != "请求路径不存在" || result.Data != nil {
				t.Fatalf("platform admin must use the uniform unmatched-route envelope: %+v", result)
			}

		})
	}
}

func TestAppNewMountsRealNodeManagementDescriptorOnlyInNodeMode(t *testing.T) {
	for _, mode := range []string{"local", "hub", "node"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			nodeConfig := ""
			if mode == "node" {
				nodeConfig = "\n  node:\n    code: real-node-fixture\n    managementBearer: real-node-bearer\n"
			}
			writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: real-node-module-test
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
platform:
  mode: `+mode+nodeConfig)

			app, err := New(dir)
			if err != nil {
				t.Fatalf("bootstrap %s app: %v", mode, err)
			}
			responseRecorder := ut.PerformRequest(app.Engine(), "GET", "/internal/node/v1/descriptor", nil,
				ut.Header{Key: "Authorization", Value: "Bearer real-node-bearer"})
			var result response.Result
			if err := json.Unmarshal(responseRecorder.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode %s descriptor response: %v body=%s", mode, err, responseRecorder.Body.String())
			}
			if mode != "node" {
				if responseRecorder.Code != 404 || result.Code != 40400 || result.Message != "请求路径不存在" || result.Data != nil {
					t.Fatalf("%s descriptor must be uniform 404: status=%d result=%+v", mode, responseRecorder.Code, result)
				}
				return
			}
			if responseRecorder.Code != 200 || result.Code != 0 {
				t.Fatalf("node descriptor failed: status=%d result=%+v", responseRecorder.Code, result)
			}
			data, err := json.Marshal(result.Data)
			if err != nil {
				t.Fatalf("marshal descriptor data: %v", err)
			}
			var descriptor struct {
				NodeCode string `json:"nodeCode"`
			}
			if err := json.Unmarshal(data, &descriptor); err != nil {
				t.Fatalf("decode descriptor data: %v", err)
			}
			if descriptor.NodeCode != "real-node-fixture" {
				t.Fatalf("descriptor nodeCode=%q want real-node-fixture", descriptor.NodeCode)
			}
		})
	}
}

func TestAppNewMountsConcreteHubControlModuleOnlyInHubMode(t *testing.T) {
	for _, mode := range []string{"local", "hub", "node"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			nodeConfig := ""
			if mode == "node" {
				nodeConfig = "\n  node:\n    code: real-node-fixture\n    managementBearer: real-node-bearer\n"
			}
			writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: real-hub-control-module-test
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: true
  redis:
    enabled: false
logging:
  level: debug
  format: json
id:
  node: 9
setup:
  enabled: false
platform:
  mode: `+mode+nodeConfig)

			app, err := New(dir)
			if err != nil {
				t.Fatalf("bootstrap %s app: %v", mode, err)
			}
			responseRecorder := ut.PerformRequest(app.Engine(), http.MethodGet, "/api/system/hub/nodes", nil)
			var result response.Result
			if err := json.Unmarshal(responseRecorder.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode %s Hub response: %v body=%s", mode, err, responseRecorder.Body.String())
			}
			registered := false
			for _, descriptor := range app.registry.ListModules() {
				if descriptor.Name == "hub-control" {
					registered = true
				}
			}
			if mode == "hub" {
				if responseRecorder.Code != http.StatusOK || result.Code != 40100 || !registered {
					t.Fatalf("hub concrete route/wiring status=%d result=%+v registered=%v", responseRecorder.Code, result, registered)
				}
				return
			}
			if responseRecorder.Code != http.StatusNotFound || result.Code != 40400 || result.Message != "请求路径不存在" || result.Data != nil || registered {
				t.Fatalf("%s Hub route must be pre-middleware 404: status=%d result=%+v registered=%v", mode, responseRecorder.Code, result, registered)
			}
		})
	}
}

func TestAppBuildsWithSecureProductionLikeAuthConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "application.yaml"), `
seven:
  name: prod-like-app
  env: prod
server:
  host: 127.0.0.1
  port: 9999
  contextPath: /api
datasource:
  driver: mysql
cache:
  enabled: false
  redis:
    enabled: false
logging:
  level: info
  format: json
id:
  node: 9
setup:
  enabled: false
  requireOriginHeader: true
  allowedOriginPatterns:
    - https://auth.example.com
observability:
  prometheus:
    accessToken: production-like-ops-token-123456
sso:
  issuer: https://auth.example.com/api/sso
  baseUrl: https://auth.example.com/api/sso
  frontendLoginUrl: https://auth.example.com/login
  rateLimit:
    failClosedOnError: true
  sessionCookie:
    secure: true
    sameSite: Lax
  refreshCookie:
    secure: true
    httpOnly: true
    sameSite: Lax
authorization:
  internal:
    enabled: true
    token: pA9qZ7mR4vL2nX8cT6bY3wK5
    signatureEnabled: true
    signatureSecret: aR8mV2qL9zX5cN7pT4wK6yB3dH1s
challenge:
  webauthnRpId: auth.example.com
  webauthnAllowedOrigins:
    - https://auth.example.com
`)
	t.Setenv("SEVEN_PROFILE", "prod")

	app, err := New(dir)
	if err != nil {
		t.Fatalf("bootstrap production-like app: %v", err)
	}

	engine := app.Engine()
	if engine == nil {
		t.Fatal("expected engine")
	}
	pingResp := ut.PerformRequest(engine, "GET", "/api/ping", nil)
	if pingResp.Code != 200 {
		t.Fatalf("unexpected production-like context path ping status: %d", pingResp.Code)
	}
	modulesResp := ut.PerformRequest(engine, "GET", "/api/ops/modules", nil)
	if modulesResp.Code != 200 {
		t.Fatalf("unexpected production-like modules status: %d", modulesResp.Code)
	}
	var result response.Result
	if err := json.Unmarshal(modulesResp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal production-like modules response: %v", err)
	}
	if result.Code != 40100 {
		t.Fatalf("expected unauthenticated production-like modules result code 40100, got %d", result.Code)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
