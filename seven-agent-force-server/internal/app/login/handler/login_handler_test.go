package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	loginfacade "github.com/CaixyPromise/seven-agent/force/internal/app/login/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestLoginHandlerWrapsPasswordAndPasskeyRoutes(t *testing.T) {
	engine := server.Default()
	handler := NewHandler(&fakePasswordFlowFacade{
		passwordState:  &loginfacade.PasswordState{CanPasswordLogin: true},
		passwordSubmit: &loginfacade.PasswordSubmitResult{Authenticated: true, CanPasswordLogin: true},
		passkeyStart:   &loginfacade.PasskeyStartResult{ChallengeIdentifier: "passkey-1", StepIdentifier: "step-passkey"},
		passkeyVerify:  &loginfacade.PasskeyVerifyResult{Authenticated: true},
		totpVerify:     &loginfacade.TotpVerifyResult{Authenticated: true},
	})
	engine.POST("/login/password/state", handler.PasswordState)
	engine.POST("/login/password", handler.Password)
	engine.POST("/login/passkey/start", handler.StartPasskey)
	engine.POST("/login/passkey/verify", handler.VerifyPasskey)
	engine.POST("/login/totp/verify", handler.VerifyTotp)

	assertLoginResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/login/password/state", bodyOf(t, map[string]any{
		"loginTransactionId": "txn-1",
		"userAccount":        "admin",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}))
	assertLoginResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/login/password", bodyOf(t, map[string]any{
		"loginTransactionId": "txn-1",
		"userAccount":        "admin",
		"password":           "secret123",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}))
	assertLoginResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/login/passkey/start", bodyOf(t, map[string]any{
		"loginTransactionId": "txn-1",
		"userAccount":        "admin",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}))
	assertLoginResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/login/passkey/verify", bodyOf(t, map[string]any{
		"loginTransactionId":   "txn-1",
		"userAccount":          "admin",
		"credentialIdentifier": "cred-1",
		"clientDataJSON":       "client-data",
		"authenticatorData":    "auth-data",
		"signature":            "sig",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}))
	assertLoginResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/login/totp/verify", bodyOf(t, map[string]any{
		"loginTransactionId": "txn-1",
		"userAccount":        "admin",
		"otpCode":            "123456",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}))
}

func TestLoginHandlerAnnotatesPasswordPunishmentAudit(t *testing.T) {
	expiresAt := int64(1893456000000)
	cases := []struct {
		name           string
		submit         *loginfacade.PasswordSubmitResult
		submitErr      error
		expectedStatus int
		expectedCode   int
		body           map[string]any
		expected       map[string]any
	}{
		{
			name: "captcha required",
			submit: &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				CaptchaRequired:  true,
				Captcha:          &loginfacade.Captcha{ChallengeIdentifier: "captcha-challenge-1", StepIdentifier: "step-1"},
			},
			expectedStatus: 200,
			expectedCode:   0,
			body: map[string]any{
				"loginTransactionId": "txn-punish",
				"userAccount":        "alice@example.test",
				"password":           "raw-password",
			},
			expected: map[string]any{
				"outcome":             "captcha_required",
				"captchaRequired":     true,
				"challengeIdentifier": "captcha-challenge-1",
			},
		},
		{
			name: "captcha failed",
			submit: &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				CaptchaRequired:  true,
				CaptchaRejected:  true,
				Captcha:          &loginfacade.Captcha{ChallengeIdentifier: "captcha-challenge-2", StepIdentifier: "step-2"},
			},
			expectedStatus: 200,
			expectedCode:   0,
			body: map[string]any{
				"loginTransactionId": "txn-punish",
				"userAccount":        "alice@example.test",
				"password":           "raw-password",
				"captchaCode":        "wrong",
			},
			expected: map[string]any{
				"outcome":             "captcha_failed",
				"captchaRequired":     true,
				"challengeIdentifier": "captcha-challenge-2",
			},
		},
		{
			name: "password rejected after captcha passed",
			submit: &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				CaptchaRequired:  true,
				Captcha:          &loginfacade.Captcha{ChallengeIdentifier: "captcha-challenge-3", StepIdentifier: "step-3"},
			},
			expectedStatus: 200,
			expectedCode:   0,
			body: map[string]any{
				"loginTransactionId": "txn-punish",
				"userAccount":        "alice@example.test",
				"password":           "raw-password",
				"captchaCode":        "accepted-but-password-wrong",
			},
			expected: map[string]any{
				"outcome":             "rejected",
				"captchaRequired":     true,
				"challengeIdentifier": "captcha-challenge-3",
			},
		},
		{
			name: "locked",
			submit: &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				Locked:           true,
				LockExpiresAt:    &expiresAt,
			},
			expectedStatus: 200,
			expectedCode:   0,
			body: map[string]any{
				"loginTransactionId": "txn-punish",
				"userAccount":        "alice@example.test",
				"password":           "raw-password",
			},
			expected: map[string]any{
				"outcome":       "locked",
				"locked":        true,
				"lockExpiresAt": expiresAt,
			},
		},
		{
			name:           "rejected",
			submitErr:      apperrors.Unauthorized("账号或密码错误"),
			expectedStatus: 200,
			expectedCode:   apperrors.CodeNotLogin,
			body: map[string]any{
				"loginTransactionId": "txn-punish",
				"userAccount":        "alice@example.test",
				"password":           "raw-password",
			},
			expected: map[string]any{
				"outcome": "rejected",
				"code":    apperrors.CodeNotLogin,
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			engine := server.Default()
			handler := NewHandler(&fakePasswordFlowFacade{
				passwordSubmit:    tt.submit,
				passwordSubmitErr: tt.submitErr,
			})
			var metadata map[string]any
			engine.POST("/login/password", func(ctx context.Context, reqCtx *app.RequestContext) {
				handler.Password(ctx, reqCtx)
				var ok bool
				metadata, ok = securitycontext.LoginPunishmentAuditMetadata(reqCtx)
				if !ok {
					t.Fatalf("expected login punishment audit metadata")
				}
			})
			recorder := ut.PerformRequest(engine.Engine, "POST", "/login/password", bodyOf(t, tt.body), ut.Header{Key: "Content-Type", Value: "application/json"})
			if recorder.Code != tt.expectedStatus {
				t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
			}
			var result response.Result
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if result.Code != tt.expectedCode {
				t.Fatalf("unexpected business code: %+v", result)
			}
			for key, expected := range tt.expected {
				if got := metadata[key]; got != expected {
					t.Fatalf("metadata[%s] expected %#v, got %#v in %#v", key, expected, got, metadata)
				}
			}
			fingerprint, ok := metadata["accountFingerprint"].(string)
			if !ok || fingerprint == "" || fingerprint == "alice@example.test" {
				t.Fatalf("expected non-raw account fingerprint, got %#v in %#v", metadata["accountFingerprint"], metadata)
			}
			if got := metadata["loginTransactionId"]; got != "txn-punish" {
				t.Fatalf("unexpected loginTransactionId metadata: %#v", metadata)
			}
		})
	}
}

func TestLoginHandlerAnnotatesSecondFactorPunishmentAudit(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		body    map[string]any
		handler func(*Handler) func(context.Context, *app.RequestContext)
		facade  *fakePasswordFlowFacade
	}{
		{
			name: "totp rejected",
			path: "/login/totp/verify",
			body: map[string]any{
				"loginTransactionId": "txn-totp",
				"userAccount":        "alice@example.test",
				"otpCode":            "123456",
			},
			handler: func(h *Handler) func(context.Context, *app.RequestContext) { return h.VerifyTotp },
			facade:  &fakePasswordFlowFacade{totpVerify: &loginfacade.TotpVerifyResult{Authenticated: false}},
		},
		{
			name: "passkey rejected",
			path: "/login/passkey/verify",
			body: map[string]any{
				"loginTransactionId":   "txn-passkey",
				"userAccount":          "alice@example.test",
				"credentialIdentifier": "credential-1",
				"clientDataJSON":       "client-data",
				"authenticatorData":    "auth-data",
				"signature":            "signature",
			},
			handler: func(h *Handler) func(context.Context, *app.RequestContext) { return h.VerifyPasskey },
			facade:  &fakePasswordFlowFacade{passkeyVerify: &loginfacade.PasskeyVerifyResult{Authenticated: false}},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			engine := server.Default()
			handler := NewHandler(tt.facade)
			var metadata map[string]any
			engine.POST(tt.path, func(ctx context.Context, reqCtx *app.RequestContext) {
				tt.handler(handler)(ctx, reqCtx)
				var ok bool
				metadata, ok = securitycontext.LoginPunishmentAuditMetadata(reqCtx)
				if !ok {
					t.Fatalf("expected login punishment audit metadata")
				}
			})
			recorder := ut.PerformRequest(engine.Engine, "POST", tt.path, bodyOf(t, tt.body), ut.Header{Key: "Content-Type", Value: "application/json"})
			if recorder.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
			}
			if got := metadata["outcome"]; got != "rejected" {
				t.Fatalf("expected rejected outcome, got %#v in %#v", got, metadata)
			}
			if fingerprint := metadata["accountFingerprint"]; fingerprint == "" || fingerprint == "alice@example.test" {
				t.Fatalf("expected non-raw account fingerprint, got %#v in %#v", fingerprint, metadata)
			}
		})
	}
}

type fakePasswordFlowFacade struct {
	passwordState     *loginfacade.PasswordState
	passwordSubmit    *loginfacade.PasswordSubmitResult
	passwordSubmitErr error
	registerState     *loginfacade.RegisterState
	registerEmailCode *loginfacade.RegisterEmailCodeResult
	registerSubmit    *loginfacade.RegisterSubmitResult
	passkeyStart      *loginfacade.PasskeyStartResult
	passkeyVerify     *loginfacade.PasskeyVerifyResult
	totpVerify        *loginfacade.TotpVerifyResult
}

func (f *fakePasswordFlowFacade) GetPasswordState(context.Context, loginfacade.PasswordStateRequest) (*loginfacade.PasswordState, error) {
	return f.passwordState, nil
}
func (f *fakePasswordFlowFacade) GetRegisterState(context.Context, loginfacade.RegisterStateRequest) (*loginfacade.RegisterState, error) {
	return f.registerState, nil
}
func (f *fakePasswordFlowFacade) SendRegisterEmailCode(context.Context, loginfacade.RegisterEmailCodeRequest) (*loginfacade.RegisterEmailCodeResult, error) {
	return f.registerEmailCode, nil
}
func (f *fakePasswordFlowFacade) SubmitPassword(context.Context, loginfacade.PasswordSubmitRequest) (*loginfacade.PasswordSubmitResult, error) {
	if f.passwordSubmitErr != nil {
		return nil, f.passwordSubmitErr
	}
	return f.passwordSubmit, nil
}
func (f *fakePasswordFlowFacade) SubmitRegister(context.Context, loginfacade.RegisterSubmitRequest) (*loginfacade.RegisterSubmitResult, error) {
	return f.registerSubmit, nil
}
func (f *fakePasswordFlowFacade) StartPasskey(context.Context, loginfacade.PasskeyStartRequest) (*loginfacade.PasskeyStartResult, error) {
	return f.passkeyStart, nil
}
func (f *fakePasswordFlowFacade) VerifyPasskey(context.Context, loginfacade.PasskeyVerifyRequest) (*loginfacade.PasskeyVerifyResult, error) {
	return f.passkeyVerify, nil
}
func (f *fakePasswordFlowFacade) VerifyTotp(context.Context, loginfacade.TotpVerifyRequest) (*loginfacade.TotpVerifyResult, error) {
	return f.totpVerify, nil
}

func bodyOf(t *testing.T, payload any) *ut.Body {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return &ut.Body{Body: bytes.NewReader(body), Len: len(body)}
}

func assertLoginResultOK(t *testing.T, recorder *ut.ResponseRecorder) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != 0 {
		t.Fatalf("unexpected business result: %+v", result)
	}
}
