package infrastructure

import (
	"context"
	"strings"
	"testing"

	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestOperationLoggerMasksOAuthFormSecrets(t *testing.T) {
	service := &recordingOAuthOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxBodyBytes:   4096,
		MaxFieldLength: 128,
	})
	engine := server.Default()
	engine.POST("/sso/oauth2/introspect", logger.Wrap(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "OAuth introspection",
		IncludeParams: true,
	}, func(_ context.Context, reqCtx *app.RequestContext) {
		reqCtx.SetStatusCode(200)
	}))

	body := "client_id=client-a&client_secret=plain-secret&token=access-token-a&code=raw-code&code_verifier=raw-verifier&token_type_hint=access_token"
	resp := ut.PerformRequest(engine.Engine, "POST", "/sso/oauth2/introspect", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"})
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
	if strings.Contains(service.last.RequestParams, "plain-secret") ||
		strings.Contains(service.last.RequestParams, "access-token-a") ||
		strings.Contains(service.last.RequestParams, "raw-code") ||
		strings.Contains(service.last.RequestParams, "raw-verifier") {
		t.Fatalf("operation request params leaked OAuth secret material: %s", service.last.RequestParams)
	}
	if !strings.Contains(service.last.RequestParams, `"client_secret":"******"`) ||
		!strings.Contains(service.last.RequestParams, `"token":"******"`) ||
		!strings.Contains(service.last.RequestParams, `"code":"******"`) ||
		!strings.Contains(service.last.RequestParams, `"code_verifier":"******"`) {
		t.Fatalf("operation request params did not mask OAuth form secrets: %s", service.last.RequestParams)
	}
	if !strings.Contains(service.last.RequestParams, `"client_id":"client-a"`) {
		t.Fatalf("operation request params should keep non-secret client_id visible: %s", service.last.RequestParams)
	}
}

func TestOperationLoggerMasksOAuthRawFallbackSecrets(t *testing.T) {
	service := &recordingOAuthOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxBodyBytes:   4096,
		MaxFieldLength: 256,
	})
	engine := server.Default()
	engine.POST("/sso/oauth2/token", logger.Wrap(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "OAuth token",
		IncludeParams: true,
	}, func(_ context.Context, reqCtx *app.RequestContext) {
		reqCtx.SetStatusCode(200)
	}))

	body := `{"client_id":"client-a","code":"raw-code","code_verifier":"raw-verifier","authorization_code":"raw-authorization-code","statusCode":200`
	resp := ut.PerformRequest(engine.Engine, "POST", "/sso/oauth2/token", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
	for _, leaked := range []string{"raw-code", "raw-verifier", "raw-authorization-code"} {
		if strings.Contains(service.last.RequestParams, leaked) {
			t.Fatalf("operation raw fallback leaked OAuth secret %q: %s", leaked, service.last.RequestParams)
		}
	}
	if !strings.Contains(service.last.RequestParams, `"raw":"malformed_json_omitted"`) {
		t.Fatalf("sensitive malformed OAuth body must fail closed: %s", service.last.RequestParams)
	}
}

func TestOperationLoggerMasksGeneratedClientSecret(t *testing.T) {
	service := &recordingOAuthOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization", "clientSecret", "secretHash", "codeVerifier", "idToken", "accessToken", "refreshToken"},
		MaxBodyBytes:   4096,
		MaxFieldLength: 256,
	})
	engine := server.Default()
	engine.POST("/sso/admin/clients/demo-client/secrets", logger.Wrap(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "generate secret",
		IncludeParams: true,
		IncludeResult: true,
	}, func(_ context.Context, reqCtx *app.RequestContext) {
		reqCtx.JSON(200, map[string]any{
			"clientSecret": "sec_live_plain_generated_secret",
			"secretHash":   "$2a$04$plainhash",
			"idToken":      "id-token-value",
			"accessToken":  "access-token-value",
			"refreshToken": "refresh-token-value",
			"codeVerifier": "code-verifier-value",
			"secretHint":   "sec_****abcd",
		})
	}))

	body := `{"reason":"rotate","clientSecret":"sec_live_request_secret","secretHash":"raw-secret-hash","codeVerifier":"raw-code-verifier"}`
	resp := ut.PerformRequest(engine.Engine, "POST", "/sso/admin/clients/demo-client/secrets", &ut.Body{
		Body: strings.NewReader(body),
		Len:  len(body),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
	combined := service.last.RequestParams + "\n" + service.last.ResponseResult
	for _, leaked := range []string{
		"sec_live_plain_generated_secret",
		"sec_live_request_secret",
		"raw-secret-hash",
		"raw-code-verifier",
		"id-token-value",
		"access-token-value",
		"refresh-token-value",
		"code-verifier-value",
		"$2a$04$plainhash",
	} {
		if strings.Contains(combined, leaked) {
			t.Fatalf("operation log leaked %q: %s", leaked, combined)
		}
	}
	if strings.Contains(combined, "sec_live_") {
		t.Fatalf("operation log leaked generated secret prefix: %s", combined)
	}
}

type recordingOAuthOperationLogFacade struct {
	last adminfacade.OperationLogEntry
}

func (f *recordingOAuthOperationLogFacade) SaveLogAsync(_ context.Context, entry adminfacade.OperationLogEntry) {
	f.last = entry
}

func (f *recordingOAuthOperationLogFacade) SaveLog(context.Context, adminfacade.OperationLogEntry) error {
	return nil
}

func (f *recordingOAuthOperationLogFacade) GetOperationLogs(context.Context, adminfacade.OperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error) {
	return nil, nil
}

func (f *recordingOAuthOperationLogFacade) GetOperationLogByID(context.Context, int64) (*adminfacade.OperationLogVO, error) {
	return nil, nil
}

func (f *recordingOAuthOperationLogFacade) CleanExpiredLogs(context.Context, int) (int64, error) {
	return 0, nil
}

func (f *recordingOAuthOperationLogFacade) ExportOperationLogs(context.Context, adminfacade.OperationLogExportRequest, int64) ([]adminfacade.OperationLogExportDTO, error) {
	return nil, nil
}

func (f *recordingOAuthOperationLogFacade) DeleteLogsByTimeRange(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (f *recordingOAuthOperationLogFacade) GetMyOperationLogs(context.Context, int64, adminfacade.MyOperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error) {
	return nil, nil
}

func (f *recordingOAuthOperationLogFacade) GetOperationTypes(context.Context) []adminfacade.OperationTypeOption {
	return nil
}
