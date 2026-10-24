package observability

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	obsfacade "github.com/CaixyPromise/seven-agent/force/internal/app/observability/facade"
	obshandler "github.com/CaixyPromise/seven-agent/force/internal/app/observability/handler"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestModulePermissionGuards(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "true" {
			securitycontext.Set(reqCtx, &securitycontext.UserContext{
				UserID:      1001,
				Username:    "admin",
				Permissions: splitPermissions(string(reqCtx.Request.Header.Peek("X-Test-Permissions"))),
			})
		}
		reqCtx.Next(ctx)
	})
	module := &Module{handler: obshandler.NewHandler(fakeObservabilityService{})}
	module.Mount(engine.Engine)

	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/observability/overview", nil), apperrors.CodeNotLogin)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/observability/overview", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	), apperrors.CodeForbidden)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/observability/overview", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "admin:observability:view"},
	), apperrors.CodeSuccess)

	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/observability/logs/page", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "admin:observability:view"},
	), apperrors.CodeForbidden)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/observability/logs/page", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "admin:runtime-log:view"},
	), apperrors.CodeSuccess)
}

type fakeObservabilityService struct{}

func (fakeObservabilityService) GetOverview(context.Context, string, string) (*obsfacade.OverviewVO, error) {
	return &obsfacade.OverviewVO{GeneratedAt: time.Now().UTC(), SelectedPlatformKey: "sso", RangeKey: "24h"}, nil
}
func (fakeObservabilityService) PageLogs(context.Context, adminfacade.RuntimeLogQueryDTO) (*adminfacade.PageResult[adminfacade.RuntimeLogLineDTO], error) {
	return &adminfacade.PageResult[adminfacade.RuntimeLogLineDTO]{Current: 1, Size: 20, Records: []adminfacade.RuntimeLogLineDTO{}}, nil
}
func (fakeObservabilityService) StreamLogs(context.Context, adminfacade.RuntimeLogStreamRequestDTO, int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("event: heartbeat\ndata: {}\n\n")), nil
}

func assertBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	if result.Code != expected {
		t.Fatalf("expected code %d, got %d body=%s", expected, result.Code, recorder.Body.String())
	}
}

func splitPermissions(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
