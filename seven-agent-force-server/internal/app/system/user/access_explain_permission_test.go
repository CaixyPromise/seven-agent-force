package user

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestAccessExplainRoutesKeepLowPrivilegeDenialsDataMinimal(t *testing.T) {
	module := &Module{}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		permissions := strings.FieldsFunc(string(reqCtx.Request.Header.Peek("X-Test-Permissions")), func(r rune) bool {
			return r == ','
		})
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:      9001,
			Username:    "isolated-auditor",
			Permissions: permissions,
		})
		reqCtx.Next(ctx)
	})

	effectiveCalls := 0
	explainCalls := 0
	engine.GET("/system/user/:id/effective-access", module.wrapPermission("system:user:access:query", func(_ context.Context, reqCtx *app.RequestContext) {
		effectiveCalls++
		response.Success(reqCtx, map[string]any{"allowed": true})
	}))
	engine.GET("/system/user/:id/access-explain", module.wrapPermission("system:user:access:explain", func(_ context.Context, reqCtx *app.RequestContext) {
		explainCalls++
		response.Success(reqCtx, map[string]any{"allowed": true})
	}))

	basePermissions := "system:user:list,system:user:query"
	assertSafePermissionDenial(t, ut.PerformRequest(engine.Engine, http.MethodGet, "/system/user/1001/effective-access", nil,
		ut.Header{Key: "X-Test-Permissions", Value: basePermissions},
	), "system:user:access:query")
	assertSafePermissionDenial(t, ut.PerformRequest(engine.Engine, http.MethodGet, "/system/user/1001/access-explain?permissionCode=system:user:update", nil,
		ut.Header{Key: "X-Test-Permissions", Value: basePermissions},
	), "system:user:access:explain")
	if effectiveCalls != 0 || explainCalls != 0 {
		t.Fatalf("denied requests reached handlers: effective=%d explain=%d", effectiveCalls, explainCalls)
	}

	queryOnly := basePermissions + ",system:user:access:query"
	assertAccessBusinessCode(t, ut.PerformRequest(engine.Engine, http.MethodGet, "/system/user/1001/effective-access", nil,
		ut.Header{Key: "X-Test-Permissions", Value: queryOnly},
	), apperrors.CodeSuccess)
	assertSafePermissionDenial(t, ut.PerformRequest(engine.Engine, http.MethodGet, "/system/user/1001/access-explain?permissionCode=system:user:update", nil,
		ut.Header{Key: "X-Test-Permissions", Value: queryOnly},
	), "system:user:access:explain")
	if effectiveCalls != 1 || explainCalls != 0 {
		t.Fatalf("unexpected handler calls after query grant: effective=%d explain=%d", effectiveCalls, explainCalls)
	}
}

func assertAccessBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, want int) {
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

func assertSafePermissionDenial(t *testing.T, recorder *ut.ResponseRecorder, requiredPermission string) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("http status=%d want=%d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body struct {
		Code int               `json:"code"`
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, recorder.Body.String())
	}
	if body.Code != apperrors.CodeForbidden {
		t.Fatalf("business code=%d want=%d body=%s", body.Code, apperrors.CodeForbidden, recorder.Body.String())
	}
	want := map[string]string{
		"requiredPermission": requiredPermission,
		"reasonCode":         "PERMISSION_NOT_GRANTED",
	}
	if len(body.Data) != len(want) || body.Data["requiredPermission"] != want["requiredPermission"] || body.Data["reasonCode"] != want["reasonCode"] {
		t.Fatalf("unsafe or unstable denial details: %#v", body.Data)
	}
	lowerBody := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"role", "post", "menu", "grantor", "authorizationchain"} {
		if strings.Contains(lowerBody, forbidden) {
			t.Fatalf("denial leaked %q details: %s", forbidden, recorder.Body.String())
		}
	}
}
