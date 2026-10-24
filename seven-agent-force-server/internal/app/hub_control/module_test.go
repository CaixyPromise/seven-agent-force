package hub_control

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	hubfacade "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/facade"
	hubhandler "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/handler"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestActualHubModuleOrdersAuthPermissionAuditBeforeHandler(t *testing.T) {
	service := &moduleTestFacade{}
	handler := hubhandler.New(service)
	audit := &countingOperationLogger{}
	module := &Module{handler: handler, oplog: audit}
	handler.BindRouteWrapper(module.wrapRoute)
	engine := server.Default()
	engine.Use(func(ctx context.Context, c *app.RequestContext) {
		switch string(c.Request.Header.Peek("X-Test-Role")) {
		case "no-permission":
			securitycontext.Set(c, &securitycontext.UserContext{UserID: 1})
		case "allowed":
			securitycontext.Set(c, &securitycontext.UserContext{UserID: 1, Permissions: []string{"system:hub-node:list"}})
		}
		c.Next(ctx)
	})
	module.MountHub(engine)

	assertCode := func(role string, want int) {
		headers := []ut.Header{}
		if role != "" {
			headers = append(headers, ut.Header{Key: "X-Test-Role", Value: role})
		}
		resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/system/hub/nodes", nil, headers...)
		var body struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != want {
			t.Fatalf("role=%s code=%d body=%s", role, body.Code, resp.Body.String())
		}
	}
	assertCode("", apperrors.CodeNotLogin)
	assertCode("no-permission", apperrors.CodeForbidden)
	if audit.calls != 0 || service.calls != 0 {
		t.Fatalf("denied request entered audit/handler: audit=%d handler=%d", audit.calls, service.calls)
	}
	assertCode("allowed", apperrors.CodeSuccess)
	if audit.calls != 1 || service.calls != 1 {
		t.Fatalf("allowed chain audit=%d handler=%d", audit.calls, service.calls)
	}
}

func TestHubRemoteRouteAuditSpecsAreDataMinimal(t *testing.T) {
	tests := []struct {
		permission    string
		includeParams bool
	}{
		{permission: "system:hub-node:user:list"},
		{permission: "system:hub-node:user:query"},
		{permission: "system:hub-node:session:list"},
		{permission: "system:hub-node:policy:query"},
		{permission: "system:hub-node:user:status", includeParams: true},
		{permission: "system:hub-node:session:revoke", includeParams: true},
		{permission: "system:hub-node:policy:apply"},
	}
	for _, test := range tests {
		spec := hubOperationLogSpec(test.permission, "remote operation")
		if spec.IncludeResult {
			t.Fatalf("%s must not persist remote response bodies", test.permission)
		}
		if !spec.OmitQuery {
			t.Fatalf("%s must omit remote query values", test.permission)
		}
		if spec.IncludeParams != test.includeParams {
			t.Fatalf("%s IncludeParams=%v want %v", test.permission, spec.IncludeParams, test.includeParams)
		}
		if len(spec.CompletionEnrichers) != 1 {
			t.Fatalf("%s must retain only bounded completion metadata", test.permission)
		}
	}
}

func TestHubModuleShutdownClosesOwnedHTTPResourcesOnce(t *testing.T) {
	var calls atomic.Int32
	module := &Module{closeIdleConnections: func() { calls.Add(1) }}
	var hook core.ShutdownHook = module
	if err := hook.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := hook.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("CloseIdleConnections calls=%d want 1", calls.Load())
	}
}

func TestHubAuditCompletionRetainsOnlyBoundedMetadata(t *testing.T) {
	const canonicalTraceID = "dddddddddddddddddddddddddddddddd"
	responses := []string{
		`{"code":0,"traceId":"remote-user-trace","data":{"total":"2","records":[{"username":"alice","email":"alice@example.test"}]}}`,
		`{"code":0,"traceId":"remote-session-trace","data":{"total":"1","records":[{"sessionRef":"opaque-session-ref","clientId":"console"}]}}`,
		`{"code":0,"traceId":"remote-policy-trace","data":{"passwordPolicy":{"minimumLength":12},"mfaPolicy":{"required":true}}}`,
		`{"code":0,"traceId":"remote-revoke-trace","data":{"changedCount":"3","sessionRefs":["opaque-session-ref"]}}`,
	}
	for _, responseBody := range responses {
		reqCtx := &app.RequestContext{}
		reqCtx.Set(xcontext.TraceIDKey, canonicalTraceID)
		reqCtx.Request.SetRequestURI("/system/hub/nodes/node-a/users/42/sessions")
		reqCtx.Response.SetBodyString(responseBody)
		entry := &adminfacade.OperationLogEntry{}
		hubAuditCompletionEnricher{}.Enrich(context.Background(), reqCtx, entry)
		for _, forbidden := range []string{"alice", "example.test", "opaque-session-ref", "passwordPolicy", "mfaPolicy", "sessionRefs"} {
			if strings.Contains(entry.ResponseResult, forbidden) {
				t.Fatalf("audit summary retained %q: %s", forbidden, entry.ResponseResult)
			}
		}
		if strings.Contains(entry.MethodName, "node-a") || strings.Contains(entry.MethodName, "42") || entry.MethodName != "/system/hub/nodes/:nodeCode/users/:userId/sessions" {
			t.Fatalf("audit route was not templated: %s", entry.MethodName)
		}
		for _, required := range []string{"node-a", "traceId", "code"} {
			if !strings.Contains(entry.ResponseResult, required) {
				t.Fatalf("audit summary missing %q: %s", required, entry.ResponseResult)
			}
		}
		if !strings.Contains(entry.ResponseResult, canonicalTraceID) || strings.Contains(entry.ResponseResult, "remote-") {
			t.Fatalf("audit did not retain only canonical trace: %s", entry.ResponseResult)
		}
	}
}

func TestHubControlDDDImportBoundaries(t *testing.T) {
	applicationSource, err := os.ReadFile(filepath.Join("application", "service.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"internal/infrastructure/microservice", "internal/infrastructure/security/crypto/secretvalue"} {
		if strings.Contains(string(applicationSource), forbidden) {
			t.Fatalf("hub application imports infrastructure package %q", forbidden)
		}
	}
	entries, err := os.ReadDir("infrastructure")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join("infrastructure", entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, forbidden := range []string{"internal/app/"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("hub infrastructure %s imports upper layer %q", entry.Name(), forbidden)
			}
		}
	}
}

type moduleTestFacade struct {
	hubfacade.NodeAdminFacade
	calls int
}

func (f *moduleTestFacade) PageNodes(context.Context, hubfacade.NodePageQuery) (*hubfacade.NodePage, error) {
	f.calls++
	return &hubfacade.NodePage{Current: 1, Size: 20}, nil
}

type countingOperationLogger struct{ calls int }

func (l *countingOperationLogger) Wrap(_ adminfacade.OperationLogSpec, next app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) { l.calls++; next(ctx, c) }
}
