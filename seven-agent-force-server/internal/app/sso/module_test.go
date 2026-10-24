package sso

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	ssoapp "github.com/CaixyPromise/seven-agent/force/internal/app/sso/application"
	ssohandler "github.com/CaixyPromise/seven-agent/force/internal/app/sso/handler"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestModuleMountsDiscoveryForExactRootIssuer(t *testing.T) {
	const issuer = "https://hub.example.com/"
	service := ssoapp.NewService(config.SSOConfig{Issuer: issuer, BaseURL: "https://hub.example.com"}, nil, nil, nil, nil, nil, nil, nil)
	module := &Module{handler: ssohandler.NewHandler(service, ssohandler.ConfigView{Issuer: issuer, BaseURL: "https://hub.example.com"})}
	engine := server.Default()
	module.Mount(engine)

	for _, path := range []string{"/.well-known/openid-configuration", "/sso/.well-known/openid-configuration"} {
		response := ut.PerformRequest(engine.Engine, http.MethodGet, path, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
		var document map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
			t.Fatalf("decode discovery: %v", err)
		}
		if document["issuer"] != issuer {
			t.Fatalf("GET %s issuer=%v want exact %q", path, document["issuer"], issuer)
		}
	}
}

func TestModuleWrapsOAuthSecurityRoutesWithOperationLogger(t *testing.T) {
	logger := &recordingOperationLogger{}
	module := &Module{
		handler: ssohandler.NewHandler(nil, ssohandler.ConfigView{}),
		oplog:   logger,
	}
	engine := server.Default()
	module.Mount(engine)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{
			method: "POST",
			path:   "/sso/oauth2/revoke",
			body:   "client_id=client-a&client_secret=plain-secret&token=access-token-a",
		},
		{
			method: "POST",
			path:   "/sso/oauth2/introspect",
			body:   "client_id=client-a&client_secret=plain-secret&token=access-token-a",
		},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			logger.reset()
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, &ut.Body{
				Body: strings.NewReader(tt.body),
				Len:  len(tt.body),
			}, ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"})
			if resp.Code != 204 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if logger.calls != 1 {
				t.Fatalf("expected route to be wrapped once, got %d", logger.calls)
			}
			if !logger.lastSpec.IncludeParams {
				t.Fatalf("expected operation log to include request params for %s", tt.path)
			}
			if logger.lastSpec.Operation != adminfacade.OperationTypeOther {
				t.Fatalf("operation=%s, want %s", logger.lastSpec.Operation, adminfacade.OperationTypeOther)
			}
		})
	}
}

func TestModuleMountsUserInfoGetAndPost(t *testing.T) {
	module := &Module{handler: ssohandler.NewHandler(&ssoapp.Service{}, ssohandler.ConfigView{})}
	engine := server.Default()
	module.Mount(engine)

	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			resp := ut.PerformRequest(engine.Engine, method, "/sso/oauth2/userinfo", nil)
			if resp.Code != 401 {
				t.Fatalf("unexpected status for %s userinfo: %d body=%s", method, resp.Code, resp.Body.String())
			}
		})
	}
}

func TestModuleWrapsSessionLifecycleRoutesWithResultLogging(t *testing.T) {
	logger := &recordingOperationLogger{}
	module := &Module{
		handler: ssohandler.NewHandler(nil, ssohandler.ConfigView{}),
		oplog:   logger,
	}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:      9001,
			Username:    "admin",
			SessionID:   "admin-session",
			IsAdmin:     true,
			Permissions: []string{"admin:sso:session:kick", "admin:sso:device:kick"},
		})
	})
	module.Mount(engine)

	cases := []struct {
		method string
		path   string
	}{
		{method: "POST", path: "/sso/logout"},
		{method: "DELETE", path: "/sso/sessions/session-a"},
		{method: "DELETE", path: "/sso/devices/device-a"},
		{method: "POST", path: "/sso/logout-all"},
		{method: "POST", path: "/sso/admin/users/1001/sessions/session-a/kick"},
		{method: "POST", path: "/sso/admin/users/1001/logout-all"},
		{method: "POST", path: "/sso/admin/users/1001/devices/device-a/kick"},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			logger.reset()
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil)
			if resp.Code != 204 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if logger.calls != 1 {
				t.Fatalf("expected route to be wrapped once, got %d", logger.calls)
			}
			if !logger.lastSpec.IncludeResult {
				t.Fatalf("expected operation log to include response result for %s", tt.path)
			}
		})
	}
}

func TestModuleWrapsSessionReadRoutesWithResultLogging(t *testing.T) {
	logger := &recordingOperationLogger{}
	module := &Module{
		handler: ssohandler.NewHandler(nil, ssohandler.ConfigView{}),
		oplog:   logger,
	}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:      9001,
			Username:    "admin",
			SessionID:   "admin-session",
			IsAdmin:     true,
			Permissions: []string{"admin:sso:session:list", "admin:sso:device:list"},
		})
	})
	module.Mount(engine)

	cases := []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/sso/sessions"},
		{method: "GET", path: "/sso/devices"},
		{method: "GET", path: "/sso/admin/users/1001/sessions"},
		{method: "GET", path: "/sso/admin/users/1001/devices"},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			logger.reset()
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil)
			if resp.Code != 204 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if logger.calls != 1 {
				t.Fatalf("expected route to be wrapped once, got %d", logger.calls)
			}
			if !logger.lastSpec.IncludeResult {
				t.Fatalf("expected operation log to include response result for %s", tt.path)
			}
		})
	}
}

func TestModuleDoesNotLogDeniedAdminSessionReadRoutes(t *testing.T) {
	logger := &recordingOperationLogger{}
	module := &Module{
		handler: ssohandler.NewHandler(nil, ssohandler.ConfigView{}),
		oplog:   logger,
	}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:    9001,
			Username:  "operator",
			SessionID: "operator-session",
		})
	})
	module.Mount(engine)

	cases := []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/sso/admin/users/1001/sessions"},
		{method: "GET", path: "/sso/admin/users/1001/devices"},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			logger.reset()
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil)
			if resp.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if logger.calls != 0 {
				t.Fatalf("denied admin read route should not be operation-logged, got %d calls", logger.calls)
			}
		})
	}
}

func TestModuleWrapsClientAdminReadRoutesWithPermissionAndResultLogging(t *testing.T) {
	logger := &recordingOperationLogger{}
	module := &Module{
		handler: ssohandler.NewHandler(nil, ssohandler.ConfigView{}),
		oplog:   logger,
	}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:    9001,
			Username:  "admin",
			SessionID: "admin-session",
			IsAdmin:   true,
			Permissions: []string{
				"system:sso-client:list",
				"system:sso-client:query",
				"system:sso-client:redirect:list",
				"system:sso-client:redirect:edit",
				"system:sso-client:secret:list",
				"system:sso-client:secret:generate",
				"system:sso-client:secret:disable",
			},
		})
	})
	module.Mount(engine)

	cases := []struct {
		method            string
		path              string
		wantIncludeResult bool
	}{
		{method: "GET", path: "/sso/admin/client-capabilities", wantIncludeResult: true},
		{method: "GET", path: "/sso/admin/clients", wantIncludeResult: true},
		{method: "GET", path: "/sso/admin/clients/demo-client", wantIncludeResult: true},
		{method: "GET", path: "/sso/admin/clients/demo-client/redirect-uris", wantIncludeResult: true},
		{method: "PUT", path: "/sso/admin/clients/demo-client/redirect-uris", wantIncludeResult: true},
		{method: "GET", path: "/sso/admin/clients/demo-client/secrets", wantIncludeResult: true},
		{method: "POST", path: "/sso/admin/clients/demo-client/secrets", wantIncludeResult: false},
		{method: "PUT", path: "/sso/admin/clients/demo-client/secrets/99/status", wantIncludeResult: true},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			logger.reset()
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil)
			if resp.Code != 204 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if logger.calls != 1 {
				t.Fatalf("expected route to be wrapped once, got %d", logger.calls)
			}
			if logger.lastSpec.IncludeResult != tt.wantIncludeResult {
				t.Fatalf("operation IncludeResult=%v, want %v for %s", logger.lastSpec.IncludeResult, tt.wantIncludeResult, tt.path)
			}
		})
	}
}

func TestModuleDoesNotLogDeniedClientAdminReadRoutes(t *testing.T) {
	logger := &recordingOperationLogger{}
	module := &Module{
		handler: ssohandler.NewHandler(nil, ssohandler.ConfigView{}),
		oplog:   logger,
	}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:    9001,
			Username:  "operator",
			SessionID: "operator-session",
		})
	})
	module.Mount(engine)

	cases := []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/sso/admin/client-capabilities"},
		{method: "GET", path: "/sso/admin/clients"},
		{method: "GET", path: "/sso/admin/clients/demo-client"},
		{method: "GET", path: "/sso/admin/clients/demo-client/redirect-uris"},
		{method: "PUT", path: "/sso/admin/clients/demo-client/redirect-uris"},
		{method: "GET", path: "/sso/admin/clients/demo-client/secrets"},
		{method: "POST", path: "/sso/admin/clients/demo-client/secrets"},
		{method: "PUT", path: "/sso/admin/clients/demo-client/secrets/99/status"},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			logger.reset()
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, nil)
			if resp.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if logger.calls != 0 {
				t.Fatalf("denied client admin read route should not be operation-logged, got %d calls", logger.calls)
			}
		})
	}
}

type recordingOperationLogger struct {
	calls    int
	lastSpec adminfacade.OperationLogSpec
}

func (l *recordingOperationLogger) reset() {
	l.calls = 0
	l.lastSpec = adminfacade.OperationLogSpec{}
}

func (l *recordingOperationLogger) Wrap(spec adminfacade.OperationLogSpec, _ app.HandlerFunc) app.HandlerFunc {
	return func(_ context.Context, reqCtx *app.RequestContext) {
		l.calls++
		l.lastSpec = spec
		reqCtx.SetStatusCode(204)
	}
}
