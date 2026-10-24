package cache_governance

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	cachehandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/handler"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestRefreshRouteRequiresPermissionAndRejectsBody(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "true" {
			permissions := []string(nil)
			if string(reqCtx.Request.Header.Peek("X-Test-Refresh")) == "true" {
				permissions = []string{"system:cache:refresh"}
			}
			securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 1, Permissions: permissions})
		}
		reqCtx.Next(ctx)
	})
	facade := &refreshRouteFacade{}
	module := &Module{refresh: facade, handler: cachehandler.NewRefreshHandler(facade)}
	module.Mount(engine.Engine)
	assertRefreshCode(t, ut.PerformRequest(engine.Engine, "POST", "/system/cache/refresh", nil), apperrors.CodeNotLogin)
	assertRefreshCode(t, ut.PerformRequest(engine.Engine, "POST", "/system/cache/refresh", nil, ut.Header{Key: "X-Test-Login", Value: "true"}), apperrors.CodeForbidden)
	assertRefreshCode(t, ut.PerformRequest(engine.Engine, "POST", "/system/cache/refresh", nil, ut.Header{Key: "X-Test-Login", Value: "true"}, ut.Header{Key: "X-Test-Refresh", Value: "true"}), apperrors.CodeSuccess)
	assertRefreshCode(t, ut.PerformRequest(engine.Engine, "POST", "/system/cache/refresh", &ut.Body{Body: strings.NewReader(`{}`), Len: 2}, ut.Header{Key: "X-Test-Login", Value: "true"}, ut.Header{Key: "X-Test-Refresh", Value: "true"}), apperrors.CodeParamsError)
	facade.state = "COOLDOWN"
	assertRefreshCode(t, ut.PerformRequest(engine.Engine, "POST", "/system/cache/refresh", nil, ut.Header{Key: "X-Test-Login", Value: "true"}, ut.Header{Key: "X-Test-Refresh", Value: "true"}), apperrors.CodeRateLimited)
	facade.state = "DISABLED"
	assertRefreshCode(t, ut.PerformRequest(engine.Engine, "POST", "/system/cache/refresh", nil, ut.Header{Key: "X-Test-Login", Value: "true"}, ut.Header{Key: "X-Test-Refresh", Value: "true"}), apperrors.CodeServiceUnavailable)
	if facade.calls != 3 {
		t.Fatalf("refresh facade calls=%d, request body must never be accepted", facade.calls)
	}
}

type refreshRouteFacade struct {
	calls int
	state string
}

func (*refreshRouteFacade) Enabled() bool { return true }
func (f *refreshRouteFacade) Refresh(context.Context) (cachefacade.RefreshResult, error) {
	f.calls++
	state := f.state
	if state == "" {
		state = "PENDING"
	}
	return cachefacade.RefreshResult{State: state}, nil
}

func assertRefreshCode(t *testing.T, recorder *ut.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != http.StatusOK && !(want == apperrors.CodeRateLimited && recorder.Code == http.StatusTooManyRequests) && !(want == apperrors.CodeServiceUnavailable && recorder.Code == http.StatusServiceUnavailable) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.Code != want {
		t.Fatalf("result=%+v err=%v body=%s want=%d", result, err, recorder.Body.String(), want)
	}
}
