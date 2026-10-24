package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	adminhandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/handler"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	dockerinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestDockerRoutesReturnNotFoundWhenDockerUnavailable(t *testing.T) {
	module := &Module{handler: adminhandler.NewHandler(nil, nil, nil, nil, nil)}
	engine := server.Default()
	engine.NoRoute(func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Error(reqCtx, apperrors.NotFound("请求路径不存在"))
	})
	module.Mount(engine)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/admin/docker/operations", nil)

	assertAdminBusinessCode(t, resp, apperrors.CodeNotFound)
	assertAdminMessage(t, resp, "请求路径不存在")
	assertAdminNoErrorSemantics(t, resp)
}

func TestDockerRoutesMountWhenDockerAvailable(t *testing.T) {
	module := &Module{handler: adminhandler.NewHandler(nil, nil, nil, nil, enabledDockerService{})}
	engine := server.Default()
	engine.NoRoute(func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Error(reqCtx, apperrors.NotFound("请求路径不存在"))
	})
	module.Mount(engine)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/admin/docker/operations", nil)

	assertAdminBusinessCode(t, resp, apperrors.CodeNotLogin)
	assertAdminMessage(t, resp, "未登录")
}

type enabledDockerService struct {
	dockerinfra.Service
}

func (enabledDockerService) Enabled() bool { return true }

func assertAdminBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, want int) {
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

func assertAdminNoErrorSemantics(t *testing.T, recorder *ut.ResponseRecorder) {
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

func assertAdminMessage(t *testing.T, recorder *ut.ResponseRecorder, want string) {
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
