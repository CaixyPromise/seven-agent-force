package challenge

import (
	"context"
	"strings"
	"testing"

	challengehandler "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/handler"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestModuleWrapsMfaStateMutationRoutesWithOperationLogger(t *testing.T) {
	logger := &recordingOperationLogger{}
	module := &Module{
		managementCtrl: challengehandler.NewMfaManagementHandler(nil, nil),
		oplog:          logger,
	}
	engine := server.Default()
	module.Mount(engine)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: "POST", path: "/internal/mfa/recovery-codes/regenerate", body: `{"userId":1001}`},
		{method: "POST", path: "/v1/mfa/recovery-codes/regenerate", body: `{}`},
		{method: "DELETE", path: "/v1/mfa/otp-binding", body: `{}`},
		{method: "DELETE", path: "/v1/mfa/passkeys/passkey-1", body: `{}`},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			logger.reset()
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, &ut.Body{
				Body: strings.NewReader(tt.body),
				Len:  len(tt.body),
			}, ut.Header{Key: "Content-Type", Value: "application/json"})
			if resp.Code != 204 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if logger.calls != 1 {
				t.Fatalf("expected route to be wrapped once, got %d", logger.calls)
			}
			if !logger.lastSpec.IncludeParams {
				t.Fatalf("expected operation log to include request params for %s", tt.path)
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
