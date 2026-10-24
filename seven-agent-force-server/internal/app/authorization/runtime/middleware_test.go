package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestMiddlewareAllowsAnonymousPath(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"/public"},
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/public", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, map[string]any{
			"isLogin": securitycontext.IsLogin(reqCtx),
		})
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/public", nil)
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
	var body response.Result
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, _ := body.Data.(map[string]any)
	if data["isLogin"] != false {
		t.Fatalf("expected anonymous route to install anonymous context, got %+v", body.Data)
	}
}

func TestMiddlewareAllowsAnonymousWildcardPath(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"/dict-client/**"},
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/dict-client/:code", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, map[string]any{
			"isLogin": securitycontext.IsLogin(reqCtx),
			"code":    string(reqCtx.Param("code")),
		})
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/dict-client/demo", nil)
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
}

func TestMiddlewareAllowsOnlyGETForEnterpriseIdentityOAuthCallback(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"GET /external-identities/oauth/callback/*"},
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION", "/api").Handler())
	engine.GET("/api/external-identities/oauth/callback/:provider", func(_ context.Context, reqCtx *app.RequestContext) {
		if securitycontext.IsLogin(reqCtx) {
			t.Fatal("enterprise identity callback must use its frozen OAuth state rather than a browser session")
		}
		response.Success(reqCtx, map[string]any{"provider": string(reqCtx.Param("provider"))})
	})
	engine.POST("/api/external-identities/oauth/callback/:provider", func(_ context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, nil)
	})

	getResp := ut.PerformRequest(engine.Engine, "GET", "/api/external-identities/oauth/callback/wecom", nil)
	assertBusinessCode(t, getResp, 200, errors.CodeSuccess)
	postResp := ut.PerformRequest(engine.Engine, "POST", "/api/external-identities/oauth/callback/wecom", nil)
	assertBusinessCode(t, postResp, 200, errors.CodeNotLogin)
}

func TestMiddlewareDelegatesNestedOpenTaskGETRoutesToAPIKeyHandler(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"GET /open/workforce/v1/capabilities", "POST /open/workforce/v1/tasks", "GET /open/workforce/v1/tasks/**"},
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION", "/api").Handler())

	getCalls := 0
	for _, route := range []string{
		"/api/open/workforce/v1/capabilities",
		"/api/open/workforce/v1/tasks/:id",
		"/api/open/workforce/v1/tasks/:id/outputs",
		"/api/open/workforce/v1/tasks/:id/outputs/:outputId/download",
	} {
		engine.GET(route, func(ctx context.Context, reqCtx *app.RequestContext) {
			getCalls++
			if securitycontext.IsLogin(reqCtx) {
				t.Fatal("Open Task API Key route must delegate authentication to its owning handler")
			}
			reqCtx.Status(204)
		})
	}
	engine.POST("/api/open/workforce/v1/tasks/:id/outputs", func(_ context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(204)
	})

	for _, path := range []string{
		"/api/open/workforce/v1/capabilities",
		"/api/open/workforce/v1/tasks/9007199254740993",
		"/api/open/workforce/v1/tasks/9007199254740993/outputs",
		"/api/open/workforce/v1/tasks/9007199254740993/outputs/9007199254740995/download",
	} {
		resp := ut.PerformRequest(engine.Engine, "GET", path, nil)
		if resp.Code != 204 {
			t.Fatalf("nested Open Task GET route was not delegated: path=%s status=%d body=%s", path, resp.Code, resp.Body.String())
		}
	}
	if getCalls != 4 {
		t.Fatalf("delegated Open Task GET calls=%d, want 4", getCalls)
	}

	blocked := ut.PerformRequest(engine.Engine, "POST", "/api/open/workforce/v1/tasks/9007199254740993/outputs", nil)
	assertBusinessCode(t, blocked, 200, errors.CodeNotLogin)
}

func TestMiddlewareDelegatesOnlyCurrentOpenProjectRoutesToAPIKeyHandler(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled: true, Mode: config.AuthorizationModeLocal,
		AnonymousURLs: []string{
			"POST /open/workforce/v1/projects/:projectId/tasks",
			"GET /open/workforce/v1/projects/:projectId/model-options",
			"GET /open/workforce/v1/projects/:projectId/runtime-options",
		},
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION", "/api").Handler())
	handler := func(_ context.Context, c *app.RequestContext) {
		if securitycontext.IsLogin(c) {
			t.Fatal("API Key handler must own authentication")
		}
		c.Status(204)
	}
	engine.POST("/api/open/workforce/v1/projects/:projectId/tasks", handler)
	engine.GET("/api/open/workforce/v1/projects/:projectId/model-options", handler)
	engine.GET("/api/open/workforce/v1/projects/:projectId/runtime-options", handler)
	engine.POST("/api/open/workforce/v1/projects/:projectId/model-options", handler)
	engine.POST("/api/open/workforce/v1/tasks", handler)
	engine.GET("/api/open/workforce/v1/projects/:projectId", handler)
	for _, request := range []struct {
		method, path string
		delegated    bool
	}{
		{"POST", "/api/open/workforce/v1/projects/9007199254740993/tasks", true},
		{"GET", "/api/open/workforce/v1/projects/9007199254740993/model-options", true},
		{"GET", "/api/open/workforce/v1/projects/9007199254740993/runtime-options", true},
		{"POST", "/api/open/workforce/v1/projects/9007199254740993/model-options", false},
		{"POST", "/api/open/workforce/v1/tasks", false},
		{"GET", "/api/open/workforce/v1/projects/9007199254740993", false},
		{"GET", "/api/open/workforce/v1/projects/01/model-options", false},
		{"GET", "/api/open/workforce/v1/projects/0/runtime-options", false},
		{"POST", "/api/open/workforce/v1/projects/9223372036854775808/tasks", false},
	} {
		response := ut.PerformRequest(engine.Engine, request.method, request.path, nil)
		if request.delegated {
			if response.Code != 204 {
				t.Fatalf("route not delegated: %s %s", request.method, request.path)
			}
		} else {
			assertBusinessCode(t, response, 200, errors.CodeNotLogin)
		}
	}
}

func TestMiddlewareDelegatesOnlyVersionedWorkforceMachinePathToOwningHandler(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeLocal,
		Internal: config.AuthorizationInternalConfig{
			Enabled:    true,
			HeaderName: "X-Internal-Token",
			Token:      "federation-token",
		},
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION", "/api").Handler())

	machineCalls := 0
	internalCalls := 0
	engine.POST("/api/internal/workforce/v1/workers/:workerId/register", func(ctx context.Context, reqCtx *app.RequestContext) {
		machineCalls++
		if securitycontext.IsLogin(reqCtx) {
			t.Fatal("machine handler must own authentication instead of receiving a Framework user")
		}
		reqCtx.Status(204)
	})
	engine.GET("/api/internal/auth/user/:userId", func(ctx context.Context, reqCtx *app.RequestContext) {
		internalCalls++
		reqCtx.Status(204)
	})

	machine := ut.PerformRequest(engine.Engine, "POST", "/api/internal/workforce/v1/workers/wrk_test/register", nil)
	if machine.Code != 204 || machineCalls != 1 {
		t.Fatalf("versioned Workforce Machine API was not delegated: status=%d calls=%d body=%s", machine.Code, machineCalls, machine.Body.String())
	}

	blocked := ut.PerformRequest(engine.Engine, "GET", "/api/internal/auth/user/1", nil)
	assertBusinessCode(t, blocked, 200, errors.CodeNotLogin)
	if internalCalls != 0 {
		t.Fatalf("Federation internal handler ran without its internal token")
	}
}

func TestMiddlewareDefaultAnonymousIncludesOAuthHandlerValidatedRoutes(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeLocal,
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/sso/oauth2/userinfo", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.JSON(401, map[string]any{"error": "invalid_token"})
	})
	engine.POST("/sso/oauth2/revoke", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.JSON(401, map[string]any{"error": "invalid_client"})
	})
	engine.POST("/sso/oauth2/introspect", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.JSON(401, map[string]any{"error": "invalid_client"})
	})

	userInfoResp := ut.PerformRequest(engine.Engine, "GET", "/sso/oauth2/userinfo", nil)
	if userInfoResp.Code != 401 {
		t.Fatalf("expected userinfo handler to own OAuth invalid_token status, got %d body=%s", userInfoResp.Code, userInfoResp.Body.String())
	}

	revokeResp := ut.PerformRequest(engine.Engine, "POST", "/sso/oauth2/revoke", nil)
	if revokeResp.Code != 401 || !strings.Contains(revokeResp.Body.String(), "invalid_client") {
		t.Fatalf("expected revoke handler to own OAuth invalid_client response, got %d body=%s", revokeResp.Code, revokeResp.Body.String())
	}

	introspectResp := ut.PerformRequest(engine.Engine, "POST", "/sso/oauth2/introspect", nil)
	if introspectResp.Code != 401 || !strings.Contains(introspectResp.Body.String(), "invalid_client") {
		t.Fatalf("expected introspect handler to own OAuth invalid_client response, got %d body=%s", introspectResp.Code, introspectResp.Body.String())
	}
}

func TestMiddlewareDefaultAnonymousAllowsOnlyCanonicalGETConfigAssetRoute(t *testing.T) {
	withoutRule := server.Default()
	withoutRule.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"/ping"},
	}, nil, nil, "SEVEN_SSO_SESSION", "/api").Handler())
	blockedHandlerCalls := 0
	withoutRule.GET("/api/config-assets/:id", func(ctx context.Context, reqCtx *app.RequestContext) {
		blockedHandlerCalls++
		reqCtx.Status(204)
	})
	blocked := ut.PerformRequest(withoutRule.Engine, "GET", "/api/config-assets/81", nil)
	assertBusinessCode(t, blocked, 200, errors.CodeNotLogin)
	if blockedHandlerCalls != 0 {
		t.Fatalf("asset handler ran without the method-qualified anonymous rule: calls=%d", blockedHandlerCalls)
	}

	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeLocal,
	}, nil, nil, "SEVEN_SSO_SESSION", "/api").Handler())
	assetCalls := 0
	nestedCalls := 0
	genericFileCalls := 0
	engine.GET("/api/config-assets/:id", func(ctx context.Context, reqCtx *app.RequestContext) {
		assetCalls++
		reqCtx.Status(204)
	})
	engine.POST("/api/config-assets/:id", func(ctx context.Context, reqCtx *app.RequestContext) {
		assetCalls++
		reqCtx.Status(204)
	})
	engine.PUT("/api/config-assets/:id", func(ctx context.Context, reqCtx *app.RequestContext) {
		assetCalls++
		reqCtx.Status(204)
	})
	engine.DELETE("/api/config-assets/:id", func(ctx context.Context, reqCtx *app.RequestContext) {
		assetCalls++
		reqCtx.Status(204)
	})
	engine.HEAD("/api/config-assets/:id", func(ctx context.Context, reqCtx *app.RequestContext) {
		assetCalls++
		reqCtx.Status(204)
	})
	engine.GET("/api/config-assets/:id/extra", func(ctx context.Context, reqCtx *app.RequestContext) {
		nestedCalls++
		reqCtx.Status(204)
	})
	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/api/file/download"},
		{method: "POST", path: "/api/file/upload"},
		{method: "GET", path: "/api/uploads/:taskId/status"},
		{method: "GET", path: "/api/file-manage/:id/references"},
	} {
		route := route
		switch route.method {
		case "GET":
			engine.GET(route.path, func(ctx context.Context, reqCtx *app.RequestContext) {
				genericFileCalls++
				reqCtx.Status(204)
			})
		case "POST":
			engine.POST(route.path, func(ctx context.Context, reqCtx *app.RequestContext) {
				genericFileCalls++
				reqCtx.Status(204)
			})
		}
	}

	allowed := ut.PerformRequest(engine.Engine, "GET", "/api/config-assets/81", nil)
	if allowed.Code != 204 || assetCalls != 1 {
		t.Fatalf("canonical GET stable asset was not allowed: status=%d calls=%d body=%s", allowed.Code, assetCalls, allowed.Body.String())
	}

	for _, request := range []struct {
		name   string
		method string
		path   string
		body   bool
	}{
		{name: "post is not anonymous", method: "POST", path: "/api/config-assets/81", body: true},
		{name: "put is not anonymous", method: "PUT", path: "/api/config-assets/81", body: true},
		{name: "delete is not anonymous", method: "DELETE", path: "/api/config-assets/81", body: true},
		{name: "head is not anonymous", method: "HEAD", path: "/api/config-assets/81"},
		{name: "leading zero is not canonical", method: "GET", path: "/api/config-assets/081", body: true},
		{name: "zero is not a config id", method: "GET", path: "/api/config-assets/0", body: true},
		{name: "non numeric segment is not a config id", method: "GET", path: "/api/config-assets/file-81", body: true},
		{name: "encoded slash cannot normalize into an anonymous config id", method: "GET", path: "/api/config-assets/%2F81", body: true},
		// Hertz redirects this malformed trailing encoded slash before a handler
		// runs; it is still not an anonymous asset response.
		{name: "encoded suffix cannot normalize into an anonymous config id", method: "GET", path: "/api/config-assets/81%2F"},
		{name: "double encoded slash cannot normalize into an anonymous config id", method: "GET", path: "/api/config-assets/%252F81", body: true},
		{name: "query cannot supply a missing config id", method: "GET", path: "/api/config-assets?id=81", body: true},
		{name: "nested path is not the stable route", method: "GET", path: "/api/config-assets/81/extra", body: true},
		{name: "generic file download remains protected", method: "GET", path: "/api/file/download", body: true},
		{name: "generic file upload remains protected", method: "POST", path: "/api/file/upload", body: true},
		{name: "upload status remains protected", method: "GET", path: "/api/uploads/81/status", body: true},
		{name: "file reference management remains protected", method: "GET", path: "/api/file-manage/81/references", body: true},
	} {
		t.Run(request.name, func(t *testing.T) {
			resp := ut.PerformRequest(engine.Engine, request.method, request.path, nil)
			if resp.Code == 204 {
				t.Fatalf("anonymous request reached a protected handler: method=%s path=%s", request.method, request.path)
			}
			if request.body {
				assertBusinessCode(t, resp, 200, errors.CodeNotLogin)
			}
		})
	}
	if assetCalls != 1 || nestedCalls != 0 || genericFileCalls != 0 {
		t.Fatalf("non-stable request reached a handler: asset=%d nested=%d generic_file=%d", assetCalls, nestedCalls, genericFileCalls)
	}
}

func TestMiddlewarePreservesAuthenticatedContextForStableConfigAssetRoute(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeLocal,
	}, nil, &fakeContextBuilder{
		bearer: map[string]*securitycontext.UserContext{
			"asset-token": {UserID: 1001, Username: "operator", Source: "bearer"},
		},
	}, "SEVEN_SSO_SESSION", "/api").Handler())
	engine.GET("/api/config-assets/:id", func(ctx context.Context, reqCtx *app.RequestContext) {
		actor := securitycontext.Require(reqCtx)
		response.Success(reqCtx, map[string]any{
			"userId":        actor.UserID,
			"authenticated": !actor.IsAnonymous,
			"source":        actor.Source,
		})
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/api/config-assets/81", nil, ut.Header{Key: "Authorization", Value: "Bearer asset-token"})
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
	var body response.Result
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := body.Data.(map[string]any)
	if int64Value(data["userId"]) != 1001 || data["authenticated"] != true || data["source"] != "bearer" {
		t.Fatalf("stable asset route lost authenticated context: %+v", data)
	}
}

func TestMiddlewareNormalizesConfiguredContextPath(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"/dict-client/**"},
	}, nil, &fakeContextBuilder{}, "SEVEN_SSO_SESSION", "/api").Handler())
	engine.GET("/api/dict-client/:code", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, map[string]any{
			"isLogin": securitycontext.IsLogin(reqCtx),
			"code":    string(reqCtx.Param("code")),
		})
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/api/dict-client/demo", nil)
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
}

func TestMiddlewareBuildsContextFromBearer(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"/public"},
	}, nil, &fakeContextBuilder{
		bearer: map[string]*securitycontext.UserContext{
			"token-1": {UserID: 1001, Username: "admin", Permissions: []string{"system:role:list"}, Source: "bearer"},
		},
	}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/secure", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, map[string]any{
			"userId":   securitycontext.Require(reqCtx).UserID,
			"username": securitycontext.Require(reqCtx).Username,
			"source":   securitycontext.Require(reqCtx).Source,
		})
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/secure", nil, ut.Header{Key: "Authorization", Value: "Bearer token-1"})
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
	var body response.Result
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := body.Data.(map[string]any)
	if int64Value(data["userId"]) != 1001 || data["username"] != "admin" || data["source"] != "bearer" {
		t.Fatalf("unexpected authenticated payload: %+v", data)
	}
}

func TestMiddlewareRejectsInvalidBearerOnProtectedPath(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"/public"},
	}, nil, &fakeContextBuilder{
		bearerErr: errors.Unauthorized("token invalid"),
	}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/secure", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, true)
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/secure", nil, ut.Header{Key: "Authorization", Value: "Bearer broken"})
	assertBusinessCode(t, resp, 200, errors.CodeNotLogin)
}

func TestMiddlewareBuildsContextFromSessionCookie(t *testing.T) {
	engine := server.Default()
	engine.Use(NewMiddleware(config.AuthorizationConfig{
		Enabled:       true,
		Mode:          config.AuthorizationModeLocal,
		AnonymousURLs: []string{"/public"},
	}, nil, &fakeContextBuilder{
		sessions: map[string]*securitycontext.UserContext{
			"sid-1": {UserID: 1002, Username: "operator", Source: "local-session"},
		},
	}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/secure", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, map[string]any{
			"userId": securitycontext.Require(reqCtx).UserID,
			"source": securitycontext.Require(reqCtx).Source,
		})
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/secure", nil, ut.Header{Key: "Cookie", Value: "SEVEN_SSO_SESSION=sid-1"})
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
}

func TestMiddlewareAuthenticatesInternalRequestWithSignature(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	nonce := "nonce-01"
	internalToken := "internal-token"
	authHeader := "Bearer access-token"
	engine := server.Default()
	manager := cacheinfra.NewManager("test", nil, cacheinfra.WithPrimitiveLayer("primitive", newMemoryPrimitiveLayer()))
	cfg := config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeLocal,
		Internal: config.AuthorizationInternalConfig{
			Enabled:              true,
			HeaderName:           "X-Internal-Token",
			Token:                internalToken,
			SignatureEnabled:     true,
			SignatureSecret:      "internal-secret",
			NonceTTLSeconds:      300,
			TimestampToleranceMs: 300000,
		},
	}
	engine.Use(NewMiddleware(cfg, manager, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/internal/ping", func(ctx context.Context, reqCtx *app.RequestContext) {
		internal, _ := reqCtx.Get("__seven_auth_internal__")
		response.Success(reqCtx, map[string]any{
			"internal": internal,
			"source":   securitycontext.Require(reqCtx).Source,
		})
	})
	signature := signBase64(stringsJoinLines("GET", "/internal/ping", timestamp, nonce, hashHex(authHeader), internalToken), cfg.Internal.SignatureSecret)
	resp := ut.PerformRequest(engine.Engine, "GET", "/internal/ping", nil,
		ut.Header{Key: "Authorization", Value: authHeader},
		ut.Header{Key: "X-Internal-Token", Value: internalToken},
		ut.Header{Key: headerInternalTimestamp, Value: timestamp},
		ut.Header{Key: headerInternalNonce, Value: nonce},
		ut.Header{Key: headerInternalSignature, Value: signature},
	)
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
}

func TestMiddlewareRejectsInternalSignatureNonceOutsideConfiguredLength(t *testing.T) {
	cfg := config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeLocal,
		Internal: config.AuthorizationInternalConfig{
			Enabled:              true,
			HeaderName:           "X-Internal-Token",
			Token:                "internal-token",
			SignatureEnabled:     true,
			SignatureSecret:      "internal-secret",
			NonceTTLSeconds:      300,
			NonceMinLength:       8,
			NonceMaxLength:       16,
			TimestampToleranceMs: 300000,
		},
	}
	for _, tt := range []struct {
		name  string
		nonce string
	}{
		{name: "too_short", nonce: "short"},
		{name: "too_long", nonce: "nonce-that-is-too-long"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
			engine := server.Default()
			manager := cacheinfra.NewManager("test", nil, cacheinfra.WithPrimitiveLayer("primitive", newMemoryPrimitiveLayer()))
			engine.Use(NewMiddleware(cfg, manager, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
			engine.GET("/internal/ping", func(ctx context.Context, reqCtx *app.RequestContext) {
				response.Success(reqCtx, true)
			})
			signature := signBase64(stringsJoinLines("GET", "/internal/ping", timestamp, tt.nonce, hashHex(""), cfg.Internal.Token), cfg.Internal.SignatureSecret)
			resp := ut.PerformRequest(engine.Engine, "GET", "/internal/ping", nil,
				ut.Header{Key: "X-Internal-Token", Value: cfg.Internal.Token},
				ut.Header{Key: headerInternalTimestamp, Value: timestamp},
				ut.Header{Key: headerInternalNonce, Value: tt.nonce},
				ut.Header{Key: headerInternalSignature, Value: signature},
			)
			assertBusinessCode(t, resp, 200, errors.CodeNotLogin)
		})
	}
}

func TestMiddlewareAuthenticatesGatewayHeadersInRemoteMode(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	nonce := "nonce-gateway-1"
	cfg := config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeRemote,
		Remote: config.AuthorizationRemoteConfig{
			AcceptGatewayHeaders: true,
		},
		Gateway: config.AuthorizationGatewayConfig{
			SignatureEnabled:          true,
			SignatureVersion:          "v1",
			AcceptedSignatureVersions: []string{"v1"},
			Secret:                    "gateway-secret",
			TimestampToleranceSeconds: 300,
		},
		Network: config.AuthorizationNetworkConfig{TrustedCIDRs: []string{"0.0.0.0/0", "::/0"}},
	}
	manager := cacheinfra.NewManager("test", nil, cacheinfra.WithPrimitiveLayer("primitive", newMemoryPrimitiveLayer()))
	engine := server.Default()
	engine.Use(NewMiddleware(cfg, manager, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/secure", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, map[string]any{
			"userId":   securitycontext.Require(reqCtx).UserID,
			"username": securitycontext.Require(reqCtx).Username,
			"isAdmin":  securitycontext.Require(reqCtx).IsAdmin,
		})
	})
	payload := canonicalGatewayPayload("/secure", timestamp, nonce, map[string]string{
		headerUserID:           "1003",
		headerUsername:         "gateway-user",
		headerNickname:         "Gateway User",
		headerRoles:            "ROLE_ADMIN",
		headerPermissions:      "system:role:list,admin:temp-permission:grant",
		headerOrgID:            "2001",
		headerOrgIDs:           "2001,2002",
		headerDeptID:           "3001",
		headerDeptIDs:          "3001,3002",
		headerPostIDs:          "4001",
		headerPostCodes:        "POST_ADMIN",
		headerDataScopeDeptIDs: "3001,3002",
		headerDataScopeOrgIDs:  "2001",
		headerDataScopeType:    string(securitycontext.DataScopeCustom),
		headerIsAdmin:          "true",
		headerSessionID:        "sid-gateway",
		headerAuthVersion:      "12",
		headerSessionVersion:   "3",
		headerIssuedAtEpoch:    "1710000000",
		headerExpireAtEpoch:    "1810000000",
	})
	signature := signBase64(payload, cfg.Gateway.Secret)
	resp := ut.PerformRequest(engine.Engine, "GET", "/secure", nil,
		ut.Header{Key: headerGatewaySignature, Value: signature},
		ut.Header{Key: headerGatewayTimestamp, Value: timestamp},
		ut.Header{Key: headerGatewayNonce, Value: nonce},
		ut.Header{Key: headerGatewayVersion, Value: "v1"},
		ut.Header{Key: headerUserID, Value: "1003"},
		ut.Header{Key: headerUsername, Value: "gateway-user"},
		ut.Header{Key: headerNickname, Value: "Gateway User"},
		ut.Header{Key: headerRoles, Value: "ROLE_ADMIN"},
		ut.Header{Key: headerPermissions, Value: "system:role:list,admin:temp-permission:grant"},
		ut.Header{Key: headerOrgID, Value: "2001"},
		ut.Header{Key: headerOrgIDs, Value: "2001,2002"},
		ut.Header{Key: headerDeptID, Value: "3001"},
		ut.Header{Key: headerDeptIDs, Value: "3001,3002"},
		ut.Header{Key: headerPostIDs, Value: "4001"},
		ut.Header{Key: headerPostCodes, Value: "POST_ADMIN"},
		ut.Header{Key: headerDataScopeDeptIDs, Value: "3001,3002"},
		ut.Header{Key: headerDataScopeOrgIDs, Value: "2001"},
		ut.Header{Key: headerDataScopeType, Value: string(securitycontext.DataScopeCustom)},
		ut.Header{Key: headerIsAdmin, Value: "true"},
		ut.Header{Key: headerSessionID, Value: "sid-gateway"},
		ut.Header{Key: headerAuthVersion, Value: "12"},
		ut.Header{Key: headerSessionVersion, Value: "3"},
		ut.Header{Key: headerIssuedAtEpoch, Value: "1710000000"},
		ut.Header{Key: headerExpireAtEpoch, Value: "1810000000"},
	)
	assertBusinessCode(t, resp, 200, errors.CodeSuccess)
}

func TestMiddlewareRejectsGatewayReplay(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	nonce := "nonce-gateway-replay"
	cfg := config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeRemote,
		Remote:  config.AuthorizationRemoteConfig{AcceptGatewayHeaders: true},
		Gateway: config.AuthorizationGatewayConfig{
			SignatureEnabled:          true,
			SignatureVersion:          "v1",
			AcceptedSignatureVersions: []string{"v1"},
			Secret:                    "gateway-secret",
			TimestampToleranceSeconds: 300,
		},
		Network: config.AuthorizationNetworkConfig{TrustedCIDRs: []string{"0.0.0.0/0", "::/0"}},
	}
	manager := cacheinfra.NewManager("test", nil, cacheinfra.WithPrimitiveLayer("primitive", newMemoryPrimitiveLayer()))
	engine := server.Default()
	engine.Use(NewMiddleware(cfg, manager, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/secure", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, true)
	})
	payload := canonicalGatewayPayload("/secure", timestamp, nonce, map[string]string{
		headerUserID:         "1003",
		headerUsername:       "gateway-user",
		headerSessionID:      "sid-gateway",
		headerGatewayVersion: "v1",
	})
	signature := signBase64(payload, cfg.Gateway.Secret)
	headers := []ut.Header{
		{Key: headerGatewaySignature, Value: signature},
		{Key: headerGatewayTimestamp, Value: timestamp},
		{Key: headerGatewayNonce, Value: nonce},
		{Key: headerGatewayVersion, Value: "v1"},
		{Key: headerUserID, Value: "1003"},
		{Key: headerUsername, Value: "gateway-user"},
		{Key: headerSessionID, Value: "sid-gateway"},
	}
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/secure", nil, headers...), 200, errors.CodeSuccess)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/secure", nil, headers...), 200, errors.CodeNotLogin)
}

func TestMiddlewareRejectsUnknownGatewayVersionWhenVersionedSecretsConfigured(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	nonce := "nonce-gateway-unknown-version"
	cfg := config.AuthorizationConfig{
		Enabled: true,
		Mode:    config.AuthorizationModeRemote,
		Remote:  config.AuthorizationRemoteConfig{AcceptGatewayHeaders: true},
		Gateway: config.AuthorizationGatewayConfig{
			SignatureEnabled:          true,
			SignatureVersion:          "v1",
			AcceptedSignatureVersions: []string{"v1", "v2"},
			Secret:                    "gateway-secret-fallback",
			SecretsByVersion: map[string]string{
				"v1": "gateway-secret-v1",
			},
			TimestampToleranceSeconds: 300,
		},
		Network: config.AuthorizationNetworkConfig{TrustedCIDRs: []string{"0.0.0.0/0", "::/0"}},
	}
	manager := cacheinfra.NewManager("test", nil, cacheinfra.WithPrimitiveLayer("primitive", newMemoryPrimitiveLayer()))
	engine := server.Default()
	engine.Use(NewMiddleware(cfg, manager, &fakeContextBuilder{}, "SEVEN_SSO_SESSION").Handler())
	engine.GET("/secure", func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Success(reqCtx, true)
	})
	payload := canonicalGatewayPayload("/secure", timestamp, nonce, map[string]string{
		headerUserID:    "1003",
		headerUsername:  "gateway-user",
		headerSessionID: "sid-gateway",
	})
	signature := signBase64(payload, cfg.Gateway.Secret)
	resp := ut.PerformRequest(engine.Engine, "GET", "/secure", nil,
		ut.Header{Key: headerGatewaySignature, Value: signature},
		ut.Header{Key: headerGatewayTimestamp, Value: timestamp},
		ut.Header{Key: headerGatewayNonce, Value: nonce},
		ut.Header{Key: headerGatewayVersion, Value: "v2"},
		ut.Header{Key: headerUserID, Value: "1003"},
		ut.Header{Key: headerUsername, Value: "gateway-user"},
		ut.Header{Key: headerSessionID, Value: "sid-gateway"},
	)
	assertBusinessCode(t, resp, 200, errors.CodeNotLogin)
}

type fakeContextBuilder struct {
	bearer     map[string]*securitycontext.UserContext
	sessions   map[string]*securitycontext.UserContext
	bearerErr  error
	sessionErr error
}

func (f *fakeContextBuilder) BuildContextFromAccessToken(context.Context, string, string) (*securitycontext.UserContext, error) {
	if f.bearerErr != nil {
		return nil, f.bearerErr
	}
	for _, item := range f.bearer {
		return item, nil
	}
	return nil, errors.Unauthorized("invalid bearer")
}

func (f *fakeContextBuilder) BuildContextFromSession(context.Context, string, string) (*securitycontext.UserContext, error) {
	if f.sessionErr != nil {
		return nil, f.sessionErr
	}
	for _, item := range f.sessions {
		return item, nil
	}
	return nil, errors.Unauthorized("invalid session")
}

type memoryPrimitiveLayer struct {
	values map[string]string
}

func newMemoryPrimitiveLayer() *memoryPrimitiveLayer {
	return &memoryPrimitiveLayer{values: map[string]string{}}
}

func (m *memoryPrimitiveLayer) SetNX(context.Context, string, any, time.Duration) (bool, error) {
	return false, nil
}

func (m *memoryPrimitiveLayer) SetNXString(_ context.Context, cacheKey string, value string, _ time.Duration) (bool, error) {
	if _, exists := m.values[cacheKey]; exists {
		return false, nil
	}
	m.values[cacheKey] = value
	return true, nil
}

func (m *memoryPrimitiveLayer) SetNXBytes(context.Context, string, []byte, time.Duration) (bool, error) {
	return false, nil
}

func (m *memoryPrimitiveLayer) Incr(context.Context, string, time.Duration) (int64, error) {
	return 0, nil
}

func (m *memoryPrimitiveLayer) DeleteMany(context.Context, ...string) error {
	return nil
}

func assertBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expectedStatus int, expectedCode int) {
	t.Helper()
	if recorder.Code != expectedStatus {
		t.Fatalf("unexpected http status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != expectedCode {
		t.Fatalf("unexpected business code: got=%d want=%d body=%s", result.Code, expectedCode, recorder.Body.String())
	}
}

func canonicalGatewayPayload(path, timestamp, nonce string, headers map[string]string) string {
	return stringsJoin(
		"GET",
		canonicalPathWithQuery(path, ""),
		"v1",
		headers[headerUserID],
		headers[headerUsername],
		headers[headerNickname],
		headers[headerRoles],
		headers[headerPermissions],
		headers[headerOrgID],
		headers[headerOrgIDs],
		headers[headerDeptID],
		headers[headerDeptIDs],
		headers[headerPostIDs],
		headers[headerPostCodes],
		headers[headerDataScopeDeptIDs],
		headers[headerDataScopeOrgIDs],
		headers[headerDataScopeType],
		headers[headerIsAdmin],
		headers[headerSessionID],
		headers[headerAuthVersion],
		headers[headerSessionVersion],
		headers[headerIssuedAtEpoch],
		headers[headerExpireAtEpoch],
		timestamp,
		nonce,
	)
}

func stringsJoin(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "|" + parts[i]
	}
	return result
}

func stringsJoinLines(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "\n" + parts[i]
	}
	return result
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	default:
		return 0
	}
}
