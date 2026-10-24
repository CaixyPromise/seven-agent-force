package infrastructure

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestSanitizeOperationPayloadMasksDockerKeyValueAndComposeSecrets(t *testing.T) {
	payload := map[string]any{
		"environment": []any{
			map[string]any{"key": "API_TOKEN", "value": "raw-token"},
			map[string]any{"key": "NORMAL", "value": "visible"},
		},
		"composeYaml": "services:\n  app:\n    environment:\n      DB_PASSWORD: raw-password\n",
	}
	masked := sanitizePayload(payload, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxFieldLength: 512,
	})
	encoded, err := json.Marshal(masked)
	if err != nil {
		t.Fatalf("marshal masked payload: %v", err)
	}
	value := string(encoded)
	for _, secret := range []string{"raw-token", "raw-password"} {
		if strings.Contains(value, secret) {
			t.Fatalf("expected secret %q to be masked, got %s", secret, value)
		}
	}
	if !strings.Contains(value, "visible") {
		t.Fatalf("non-sensitive value should remain visible, got %s", value)
	}
}

func TestSanitizeOperationPayloadMasksConfigValueField(t *testing.T) {
	payload := map[string]any{
		"configKey":   "smtpHost",
		"configValue": "plain-prod-credential",
		"configDesc":  "metadata stays visible",
		"isSensitive": 1,
	}
	masked := sanitizePayload(payload, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization", "key"},
		MaxFieldLength: 512,
	})
	encoded, err := json.Marshal(masked)
	if err != nil {
		t.Fatalf("marshal masked payload: %v", err)
	}
	value := string(encoded)
	if strings.Contains(value, "plain-prod-credential") {
		t.Fatalf("expected configValue to be masked, got %s", value)
	}
	if strings.Contains(value, `"isSensitive":1`) {
		t.Fatalf("expected isSensitive flag to be masked, got %s", value)
	}
	if !strings.Contains(value, "smtpHost") {
		t.Fatalf("configKey should remain useful for audit lookup, got %s", value)
	}
	if !strings.Contains(value, "metadata stays visible") {
		t.Fatalf("non-sensitive config metadata should remain visible, got %s", value)
	}
}

func TestOperationLoggerMasksManagementBearerInParsedAndMalformedJSON(t *testing.T) {
	const plaintext = "hub-management-bearer-plaintext"
	cfg := config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxFieldLength: 512,
	}
	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "parsed", body: `{"nodeCode":"node-a","managementBearer":"` + plaintext + `"}`},
		{name: "malformed fallback", body: `{"nodeCode":"node-a","managementBearer":"` + plaintext + `"`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeOperationLogFacade{}
			logger := NewOperationLogger(service, cfg)
			engine := server.Default()
			engine.POST("/hub-node", logger.Wrap(adminfacade.OperationLogSpec{IncludeParams: true}, func(_ context.Context, reqCtx *app.RequestContext) {
				reqCtx.JSON(http.StatusOK, map[string]any{"ok": true})
			}))

			response := ut.PerformRequest(engine.Engine, http.MethodPost, "/hub-node", &ut.Body{
				Body: strings.NewReader(testCase.body),
				Len:  len(testCase.body),
			}, ut.Header{Key: "Content-Type", Value: "application/json"})
			if response.Code != http.StatusOK || service.saved == nil {
				t.Fatalf("request status=%d saved=%v", response.Code, service.saved != nil)
			}
			if strings.Contains(service.saved.RequestParams, plaintext) {
				t.Fatalf("operation params leaked management bearer: %s", service.saved.RequestParams)
			}
		})
	}
}

func TestOperationLoggerFailsClosedForEscapedMalformedSensitiveJSON(t *testing.T) {
	plaintext := `secret\"suffix-that-must-not-survive`
	service := &fakeOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{Enabled: true, MaxBodyBytes: 4096})
	engine := server.Default(server.WithHostPorts("127.0.0.1:0"))
	engine.POST("/hub-node", logger.Wrap(adminfacade.OperationLogSpec{IncludeParams: true}, func(_ context.Context, reqCtx *app.RequestContext) {
		reqCtx.JSON(http.StatusOK, map[string]any{"ok": true})
	}))

	body := `{"nodeCode":"node-a","managementBearer":"` + plaintext + `"`
	ut.PerformRequest(engine.Engine, http.MethodPost, "/hub-node", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})

	if service.saved == nil {
		t.Fatal("operation log was not captured")
	}
	if strings.Contains(service.saved.RequestParams, "suffix-that-must-not-survive") || strings.Contains(service.saved.RequestParams, "secret") {
		t.Fatalf("malformed sensitive body leaked: %s", service.saved.RequestParams)
	}
}

func TestOperationLoggerFailsClosedForUnicodeEscapedMalformedSensitiveKey(t *testing.T) {
	service := &fakeOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{Enabled: true, MaxBodyBytes: 4096})
	engine := server.Default(server.WithHostPorts("127.0.0.1:0"))
	engine.POST("/hub-node", logger.Wrap(adminfacade.OperationLogSpec{IncludeParams: true}, func(_ context.Context, reqCtx *app.RequestContext) {
		reqCtx.JSON(http.StatusOK, map[string]any{"ok": true})
	}))

	body := `{"nodeCode":"node-a","management\u0042earer":"secret-suffix-that-must-not-survive"`
	ut.PerformRequest(engine.Engine, http.MethodPost, "/hub-node", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})

	if service.saved == nil {
		t.Fatal("operation log was not captured")
	}
	for _, leaked := range []string{"node-a", "management", "secret-suffix-that-must-not-survive"} {
		if strings.Contains(service.saved.RequestParams, leaked) {
			t.Fatalf("malformed body content %q leaked: %s", leaked, service.saved.RequestParams)
		}
	}
}

func TestOperationLoggerFailsClosedForMalformedNonSensitiveJSON(t *testing.T) {
	service := &fakeOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{Enabled: true, MaxBodyBytes: 4096})
	engine := server.Default(server.WithHostPorts("127.0.0.1:0"))
	engine.POST("/hub-node", logger.Wrap(adminfacade.OperationLogSpec{IncludeParams: true}, func(_ context.Context, reqCtx *app.RequestContext) {
		reqCtx.JSON(http.StatusOK, map[string]any{"ok": true})
	}))

	body := `{"nodeCode":"node-a","displayName":"broken`
	ut.PerformRequest(engine.Engine, http.MethodPost, "/hub-node", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})

	if service.saved == nil {
		t.Fatal("operation log was not captured")
	}
	for _, leaked := range []string{"node-a", "displayName", "broken"} {
		if strings.Contains(service.saved.RequestParams, leaked) {
			t.Fatalf("malformed non-sensitive body content %q leaked: %s", leaked, service.saved.RequestParams)
		}
	}
	if !strings.Contains(service.saved.RequestParams, "malformed_json_omitted") {
		t.Fatalf("malformed body omission marker missing: %s", service.saved.RequestParams)
	}
}

func TestSanitizeJSONRetainsStructuredFieldsAndMasksOnlySensitiveValues(t *testing.T) {
	cfg := config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxFieldLength: 512,
	}

	nonSensitive, err := json.Marshal(sanitizeJSONOrRaw([]byte(`{"displayName":"Node A","region":"Singapore"}`), cfg))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Node A", "Singapore"} {
		if !strings.Contains(string(nonSensitive), expected) {
			t.Fatalf("valid non-sensitive JSON lost %q: %s", expected, nonSensitive)
		}
	}

	sensitive, err := json.Marshal(sanitizeJSONOrRaw([]byte(`{"displayName":"Node A","managementBearer":"secret-value"}`), cfg))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sensitive), "secret-value") {
		t.Fatalf("valid sensitive JSON leaked value: %s", sensitive)
	}
	if !strings.Contains(string(sensitive), "Node A") || !strings.Contains(string(sensitive), "******") {
		t.Fatalf("valid sensitive JSON lost useful fields or mask: %s", sensitive)
	}
}

func TestOperationLoggerBoundsLargeValidJSONBeforePersistence(t *testing.T) {
	cfg := config.RequestLoggingConfig{MaxBodyBytes: 256, MaxFieldLength: 512}
	large := []byte(`{"records":[{"displayName":"` + strings.Repeat("x", 8192) + `"}]}`)
	encoded, err := json.Marshal(sanitizeJSONOrRaw(large, cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 512 || strings.Contains(string(encoded), strings.Repeat("x", 128)) {
		t.Fatalf("large valid JSON persisted beyond configured bound: bytes=%d", len(encoded))
	}
}

func TestOperationLoggerMasksOpaqueSessionReferences(t *testing.T) {
	cfg := config.RequestLoggingConfig{MaxBodyBytes: 4096, MaxFieldLength: 512}
	const reference = "opaque-replayable-session-reference"
	encoded, err := json.Marshal(sanitizeJSONOrRaw([]byte(`{"sessionRef":"`+reference+`","sessionRefs":["`+reference+`"]}`), cfg))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), reference) {
		t.Fatalf("opaque session reference persisted: %s", encoded)
	}
}

func TestSanitizeOperationPayloadMasksLoginFactorMaterial(t *testing.T) {
	payload := map[string]any{
		"userAccount":          "alice@example.test",
		"captchaCode":          "raw-captcha",
		"otpCode":              "123456",
		"credentialIdentifier": "credential-1",
		"clientDataJSON":       "raw-client-data-json",
		"authenticatorData":    "raw-authenticator-data",
		"signature":            "raw-webauthn-signature",
	}
	masked := sanitizePayload(payload, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxFieldLength: 512,
	})
	encoded, err := json.Marshal(masked)
	if err != nil {
		t.Fatalf("marshal masked payload: %v", err)
	}
	value := string(encoded)
	for _, secret := range []string{
		"raw-captcha",
		"123456",
		"credential-1",
		"raw-client-data-json",
		"raw-authenticator-data",
		"raw-webauthn-signature",
	} {
		if strings.Contains(value, secret) {
			t.Fatalf("expected login factor material %q to be masked, got %s", secret, value)
		}
	}
	if !strings.Contains(value, "alice@example.test") {
		t.Fatalf("userAccount should remain visible for submitted-login audit lookup, got %s", value)
	}
}

func TestOperationLoggerAppendsStepUpProofAuditMetadata(t *testing.T) {
	service := &fakeOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxFieldLength: 512,
	})
	engine := server.Default()
	engine.POST("/protected", logger.Wrap(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleAssignPermission,
		Description:   "protected mutation",
		IncludeParams: true,
	}, func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.SetStepUpProofAudit(reqCtx, securitycontext.StepUpProofAudit{
			BusinessAction:        "RBAC_ASSIGN_USER_ROLES",
			OperationBinding:      "user:1001|roles:1,2",
			ProofIdentifier:       "proof-jti-1",
			ChallengeIdentifier:   "challenge-1",
			AssuranceLevel:        "AAL2",
			AuthenticationMethods: []string{"TOTP"},
			ProofToken:            "raw-proof-token",
		})
		reqCtx.JSON(200, map[string]any{"ok": true})
	}))

	resp := ut.PerformRequest(engine.Engine, "POST", "/protected", &ut.Body{
		Body: strings.NewReader(`{"roleIds":[2,1],"password":"raw-password"}`),
		Len:  len(`{"roleIds":[2,1],"password":"raw-password"}`),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if service.saved == nil {
		t.Fatal("expected operation log entry")
	}
	params := service.saved.RequestParams
	for _, secret := range []string{"raw-proof-token", "raw-password"} {
		if strings.Contains(params, secret) {
			t.Fatalf("operation params leaked secret %q: %s", secret, params)
		}
	}
	for _, expected := range []string{
		`"stepUpProof"`,
		`"businessAction":"RBAC_ASSIGN_USER_ROLES"`,
		`"operationBinding":"user:1001|roles:1,2"`,
		`"proofIdentifier":"proof-jti-1"`,
		`"challengeIdentifier":"challenge-1"`,
		`"assuranceLevel":"AAL2"`,
		`"authenticationMethods":["TOTP"]`,
	} {
		if !strings.Contains(params, expected) {
			t.Fatalf("operation params missing %s: %s", expected, params)
		}
	}
}

func TestOperationLoggerAppendsLoginPunishmentAuditMetadata(t *testing.T) {
	service := &fakeOperationLogFacade{}
	logger := NewOperationLogger(service, config.RequestLoggingConfig{
		MaskedFields:   []string{"password", "token", "secret", "authorization"},
		MaxFieldLength: 512,
	})
	expiresAt := int64(1893456000000)
	engine := server.Default()
	engine.POST("/login/password", logger.Wrap(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogin,
		Description:   "password login",
		IncludeParams: true,
	}, func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.SetLoginPunishmentAudit(reqCtx, securitycontext.LoginPunishmentAudit{
			LoginTransactionID: "txn-login-1",
			AccountFingerprint: "sha256:account-fingerprint",
			Outcome:            "locked",
			Locked:             true,
			LockExpiresAt:      &expiresAt,
		})
		reqCtx.JSON(200, map[string]any{"ok": true})
	}))

	resp := ut.PerformRequest(engine.Engine, "POST", "/login/password", &ut.Body{
		Body: strings.NewReader(`{"loginTransactionId":"txn-login-1","userAccount":"alice@example.test","password":"raw-password"}`),
		Len:  len(`{"loginTransactionId":"txn-login-1","userAccount":"alice@example.test","password":"raw-password"}`),
	}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if service.saved == nil {
		t.Fatal("expected operation log entry")
	}
	if service.saved.Status != 0 || !strings.Contains(service.saved.ErrorMsg, "locked") {
		t.Fatalf("expected locked login to be audited as unsuccessful, got status=%d error=%q", service.saved.Status, service.saved.ErrorMsg)
	}
	params := service.saved.RequestParams
	if strings.Contains(params, "raw-password") {
		t.Fatalf("operation params leaked password: %s", params)
	}
	for _, expected := range []string{
		`"loginPunishment"`,
		`"loginTransactionId":"txn-login-1"`,
		`"accountFingerprint":"sha256:account-fingerprint"`,
		`"outcome":"locked"`,
		`"locked":true`,
		`"lockExpiresAt":"1893456000000"`,
	} {
		if !strings.Contains(params, expected) {
			t.Fatalf("operation params missing %s: %s", expected, params)
		}
	}
}

func TestAppendStepUpProofAuditMetadataMasksInvalidExistingParams(t *testing.T) {
	reqCtx := &app.RequestContext{}
	securitycontext.SetStepUpProofAudit(reqCtx, securitycontext.StepUpProofAudit{
		BusinessAction:   "CONFIG_REVEAL_SENSITIVE",
		OperationBinding: "config:10|reveal",
		ProofIdentifier:  "challenge-2",
		ProofToken:       "raw-proof-token",
	})
	params := appendStepUpProofAuditMetadata(
		"password=raw-password token=raw-token",
		reqCtx,
		config.RequestLoggingConfig{
			MaskedFields:   []string{"password", "token", "secret", "authorization"},
			MaxFieldLength: 512,
		},
	)
	for _, secret := range []string{"raw-password", "raw-token", "raw-proof-token"} {
		if strings.Contains(params, secret) {
			t.Fatalf("operation params leaked secret %q: %s", secret, params)
		}
	}
	for _, expected := range []string{`"raw":"password=****** token=******"`, `"stepUpProof"`} {
		if !strings.Contains(params, expected) {
			t.Fatalf("operation params missing %s: %s", expected, params)
		}
	}
}

type fakeOperationLogFacade struct {
	saved *adminfacade.OperationLogEntry
}

func (f *fakeOperationLogFacade) SaveLogAsync(_ context.Context, entry adminfacade.OperationLogEntry) {
	copy := entry
	f.saved = &copy
}

func (f *fakeOperationLogFacade) SaveLog(_ context.Context, entry adminfacade.OperationLogEntry) error {
	copy := entry
	f.saved = &copy
	return nil
}

func (f *fakeOperationLogFacade) GetOperationLogs(context.Context, adminfacade.OperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error) {
	return nil, nil
}

func (f *fakeOperationLogFacade) GetOperationLogByID(context.Context, int64) (*adminfacade.OperationLogVO, error) {
	return nil, nil
}

func (f *fakeOperationLogFacade) CleanExpiredLogs(context.Context, int) (int64, error) {
	return 0, nil
}

func (f *fakeOperationLogFacade) ExportOperationLogs(context.Context, adminfacade.OperationLogExportRequest, int64) ([]adminfacade.OperationLogExportDTO, error) {
	return nil, nil
}

func (f *fakeOperationLogFacade) DeleteLogsByTimeRange(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (f *fakeOperationLogFacade) GetMyOperationLogs(context.Context, int64, adminfacade.MyOperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error) {
	return nil, nil
}

func (f *fakeOperationLogFacade) GetOperationTypes(context.Context) []adminfacade.OperationTypeOption {
	return nil
}
