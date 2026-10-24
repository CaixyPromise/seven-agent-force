package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestMfaManagementHandlerQueryStatusForCurrentUser(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 1001, Username: "admin", SessionID: "sid-1"})
		reqCtx.Next(ctx)
	})
	handler := NewMfaManagementHandler(&fakeMfaManagementFacade{
		status: &challengefacade.MfaStatusResponse{
			SubjectIdentifier:          "user:1001",
			OTPBound:                   true,
			AvailableRecoveryCodeCount: 7,
		},
	}, &fakeChallengeAuthFacade{})
	engine.GET("/v1/mfa/status", handler.QueryStatusForCurrentUser)

	resp := ut.PerformRequest(engine.Engine, "GET", "/v1/mfa/status", nil, mfaAuthHeaders()...)
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"subjectIdentifier":"user:1001"`)) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"availableRecoveryCodeCount":7`)) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
}

func TestMfaManagementHandlerRegenerateReturnsChallengeRequiredPayload(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 1001, Username: "admin", SessionID: "sid-1"})
		reqCtx.Next(ctx)
	})
	handler := NewMfaManagementHandler(&fakeMfaManagementFacade{}, &fakeChallengeAuthFacade{
		challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-1", ChallengeState: "PENDING"},
	})
	engine.POST("/v1/mfa/recovery-codes/regenerate", handler.RegenerateRecoveryCodesForCurrentUser)

	body := marshalManagementBody(t, map[string]any{})
	resp := ut.PerformRequest(engine.Engine, "POST", "/v1/mfa/recovery-codes/regenerate", &ut.Body{
		Body: bytes.NewReader(body),
		Len:  len(body),
	}, append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, mfaAuthHeaders()...)...)
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":40120`)) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"challengeIdentifier":"challenge-1"`)) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
}

func TestMfaManagementHandlerPassesProofMetadataToCurrentUserMutations(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		path             string
		body             map[string]any
		register         func(*server.Hertz, *MfaManagementHandler)
		expectedAction   string
		expectedBinding  string
		expectedMutation func(*fakeMfaManagementFacade) int
	}{
		{
			name:   "regenerate_recovery_codes",
			method: "POST",
			path:   "/v1/mfa/recovery-codes/regenerate",
			body:   map[string]any{},
			register: func(engine *server.Hertz, handler *MfaManagementHandler) {
				engine.POST("/v1/mfa/recovery-codes/regenerate", handler.RegenerateRecoveryCodesForCurrentUser)
			},
			expectedAction:  string(challengedomain.BusinessActionMFARecoveryCodesRegenerate),
			expectedBinding: "",
			expectedMutation: func(f *fakeMfaManagementFacade) int {
				return f.regenerateByUserIDCalls
			},
		},
		{
			name:   "delete_otp_binding",
			method: "DELETE",
			path:   "/v1/mfa/otp-binding",
			body:   nil,
			register: func(engine *server.Hertz, handler *MfaManagementHandler) {
				engine.DELETE("/v1/mfa/otp-binding", handler.DeleteCurrentUserOtpBinding)
			},
			expectedAction:  string(challengedomain.BusinessActionMFAOTPDelete),
			expectedBinding: "",
			expectedMutation: func(f *fakeMfaManagementFacade) int {
				return f.deleteOtpByUserIDCalls
			},
		},
		{
			name:   "delete_passkey",
			method: "DELETE",
			path:   "/v1/mfa/passkeys/passkey-1",
			body:   nil,
			register: func(engine *server.Hertz, handler *MfaManagementHandler) {
				engine.DELETE("/v1/mfa/passkeys/:credentialIdentifier", handler.DeleteCurrentUserPasskey)
			},
			expectedAction:  string(challengedomain.BusinessActionMFAPasskeyDelete),
			expectedBinding: "passkey:passkey-1",
			expectedMutation: func(f *fakeMfaManagementFacade) int {
				return f.deletePasskeyByUserIDCalls
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := server.Default()
			engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
				securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 1001, Username: "admin", SessionID: "sid-1"})
				reqCtx.Next(ctx)
			})
			facade := &fakeMfaManagementFacade{}
			handler := NewMfaManagementHandler(facade, &fakeChallengeAuthFacade{})
			tt.register(engine, handler)

			var requestBody *ut.Body
			headers := []ut.Header{{Key: "Proof-Token", Value: "proof-token"}, {Key: "Flow-Nonce", Value: "flow-nonce"}}
			if tt.body != nil {
				body := marshalManagementBody(t, tt.body)
				requestBody = &ut.Body{Body: bytes.NewReader(body), Len: len(body)}
				headers = append(headers, ut.Header{Key: "Content-Type", Value: "application/json"})
			}
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, requestBody, headers...)
			if resp.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if calls := tt.expectedMutation(facade); calls != 1 {
				t.Fatalf("expected one proof-backed mutation call, got %d facade state: %+v", calls, facade)
			}
			if facade.lastProof.BusinessAction != tt.expectedAction ||
				facade.lastProof.OperationBinding != tt.expectedBinding ||
				facade.lastProof.ProofIdentifier != "proof-jti-verify" ||
				facade.lastProof.ChallengeIdentifier != "challenge-verify" {
				t.Fatalf("unexpected proof metadata: %#v", facade.lastProof)
			}
		})
	}
}

func TestMfaManagementHandlerInternalQueryStatusRejectsPublicRequest(t *testing.T) {
	engine := server.Default()
	handler := NewMfaManagementHandler(&fakeMfaManagementFacade{
		status: &challengefacade.MfaStatusResponse{
			SubjectIdentifier:          "user:1001",
			OTPBound:                   true,
			AvailableRecoveryCodeCount: 7,
		},
	}, &fakeChallengeAuthFacade{})
	engine.POST("/internal/mfa/status", handler.QueryStatusInternal)

	body := marshalManagementBody(t, map[string]any{"subjectIdentifier": "user:1001"})
	resp := ut.PerformRequest(engine.Engine, "POST", "/internal/mfa/status", &ut.Body{
		Body: bytes.NewReader(body),
		Len:  len(body),
	}, append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, mfaAuthHeaders()...)...)
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":40100`)) {
		t.Fatalf("expected internal auth failure: %s", resp.Body.String())
	}
	if bytes.Contains(resp.Body.Bytes(), []byte(`"subjectIdentifier":"user:1001"`)) {
		t.Fatalf("public request leaked internal MFA status: %s", resp.Body.String())
	}
}

func TestMfaManagementHandlerInternalRegenerateRejectsPublicRequest(t *testing.T) {
	engine := server.Default()
	handler := NewMfaManagementHandler(&fakeMfaManagementFacade{}, &fakeChallengeAuthFacade{})
	engine.POST("/internal/mfa/recovery-codes/regenerate", handler.RegenerateRecoveryCodesInternal)

	body := marshalManagementBody(t, map[string]any{"subjectIdentifier": "user:1001"})
	resp := ut.PerformRequest(engine.Engine, "POST", "/internal/mfa/recovery-codes/regenerate", &ut.Body{
		Body: bytes.NewReader(body),
		Len:  len(body),
	}, append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, mfaAuthHeaders()...)...)
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":40100`)) {
		t.Fatalf("expected internal auth failure: %s", resp.Body.String())
	}
	if bytes.Contains(resp.Body.Bytes(), []byte(`"recoveryCodes"`)) {
		t.Fatalf("public request regenerated recovery codes: %s", resp.Body.String())
	}
}

func TestMfaManagementHandlerInternalRegenerateRejectsIncompleteInternalContext(t *testing.T) {
	tests := []struct {
		name       string
		middleware app.HandlerFunc
	}{
		{
			name: "marker_only",
			middleware: func(ctx context.Context, reqCtx *app.RequestContext) {
				reqCtx.Set("__seven_auth_internal__", true)
				reqCtx.Next(ctx)
			},
		},
		{
			name: "source_and_role_without_marker",
			middleware: func(ctx context.Context, reqCtx *app.RequestContext) {
				securitycontext.Set(reqCtx, &securitycontext.UserContext{
					Username: "internal-service",
					Roles:    []string{"ROLE_INTERNAL"},
					Source:   "internal",
				})
				reqCtx.Next(ctx)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := server.Default()
			engine.Use(tt.middleware)
			handler := NewMfaManagementHandler(&fakeMfaManagementFacade{}, &fakeChallengeAuthFacade{})
			engine.POST("/internal/mfa/recovery-codes/regenerate", handler.RegenerateRecoveryCodesInternal)

			body := marshalManagementBody(t, map[string]any{"subjectIdentifier": "user:1001"})
			resp := ut.PerformRequest(engine.Engine, "POST", "/internal/mfa/recovery-codes/regenerate", &ut.Body{
				Body: bytes.NewReader(body),
				Len:  len(body),
			}, append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, mfaAuthHeaders()...)...)
			if resp.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":40100`)) {
				t.Fatalf("expected internal auth failure: %s", resp.Body.String())
			}
			if bytes.Contains(resp.Body.Bytes(), []byte(`"recoveryCodes"`)) {
				t.Fatalf("incomplete internal context regenerated recovery codes: %s", resp.Body.String())
			}
		})
	}
}

func TestMfaManagementHandlerInternalRegenerateAllowsInternalRequest(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			Username: "internal-service",
			Roles:    []string{"ROLE_INTERNAL"},
			Source:   "internal",
		})
		reqCtx.Set("__seven_auth_internal__", true)
		reqCtx.Next(ctx)
	})
	handler := NewMfaManagementHandler(&fakeMfaManagementFacade{}, &fakeChallengeAuthFacade{})
	engine.POST("/internal/mfa/recovery-codes/regenerate", handler.RegenerateRecoveryCodesInternal)

	body := marshalManagementBody(t, map[string]any{"subjectIdentifier": "user:1001"})
	resp := ut.PerformRequest(engine.Engine, "POST", "/internal/mfa/recovery-codes/regenerate", &ut.Body{
		Body: bytes.NewReader(body),
		Len:  len(body),
	}, append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, mfaAuthHeaders()...)...)
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"batchIdentifier":"batch-1"`)) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"recoveryCodes":["AAAA-BBBB"]`)) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
}

type fakeMfaManagementFacade struct {
	status                          *challengefacade.MfaStatusResponse
	challengeErr                    error
	regenerateByUserIDCalls         int
	regenerateWithChallengeCalls    int
	deleteOtpByUserIDCalls          int
	deleteOtpWithChallengeCalls     int
	deletePasskeyByUserIDCalls      int
	deletePasskeyWithChallengeCalls int
	lastPasskeyCredentialIdentifier string
	lastProof                       stepup.ProofMetadata
}

func (f *fakeMfaManagementFacade) QueryMfaStatus(context.Context, challengefacade.MfaStatusRequest) (*challengefacade.MfaStatusResponse, error) {
	return f.status, nil
}
func (f *fakeMfaManagementFacade) QueryMfaStatusByUserID(context.Context, int64) (*challengefacade.MfaStatusResponse, error) {
	return f.status, nil
}
func (f *fakeMfaManagementFacade) RegenerateRecoveryCodes(context.Context, challengefacade.RegenerateRecoveryCodeRequest) (*challengefacade.RegenerateRecoveryCodeResponse, error) {
	return &challengefacade.RegenerateRecoveryCodeResponse{SubjectIdentifier: "user:1001", BatchIdentifier: "batch-1", RecoveryCodes: []string{"AAAA-BBBB"}, GeneratedAt: pointerTime(now())}, nil
}
func (f *fakeMfaManagementFacade) RegenerateRecoveryCodesByUserID(_ context.Context, _ int64, proof stepup.ProofMetadata) (*challengefacade.RegenerateRecoveryCodeResponse, error) {
	f.regenerateByUserIDCalls++
	f.lastProof = proof
	if f.challengeErr != nil {
		return nil, f.challengeErr
	}
	return &challengefacade.RegenerateRecoveryCodeResponse{SubjectIdentifier: "user:1001", BatchIdentifier: "batch-1", RecoveryCodes: []string{"AAAA-BBBB"}, GeneratedAt: pointerTime(now())}, nil
}
func (f *fakeMfaManagementFacade) RegenerateRecoveryCodesWithChallenge(context.Context, challengefacade.RegenerateRecoveryCodeRequest, challengefacade.MfaProtectedOperationContext) (*challengefacade.RegenerateRecoveryCodeResponse, error) {
	f.regenerateWithChallengeCalls++
	if f.challengeErr != nil {
		return nil, f.challengeErr
	}
	return &challengefacade.RegenerateRecoveryCodeResponse{SubjectIdentifier: "user:1001", BatchIdentifier: "batch-1", RecoveryCodes: []string{"AAAA-BBBB"}, GeneratedAt: pointerTime(now())}, nil
}
func (f *fakeMfaManagementFacade) DeleteOtpBinding(context.Context, challengefacade.MfaDeleteOtpBindingRequest) (bool, error) {
	return true, nil
}
func (f *fakeMfaManagementFacade) DeleteOtpBindingByUserID(_ context.Context, _ int64, proof stepup.ProofMetadata) (bool, error) {
	f.deleteOtpByUserIDCalls++
	f.lastProof = proof
	return true, nil
}
func (f *fakeMfaManagementFacade) DeleteOtpBindingWithChallenge(context.Context, challengefacade.MfaDeleteOtpBindingRequest, challengefacade.MfaProtectedOperationContext) (bool, error) {
	f.deleteOtpWithChallengeCalls++
	return true, nil
}
func (f *fakeMfaManagementFacade) ListPasskeys(context.Context, challengefacade.MfaPasskeyListRequest) ([]challengefacade.MfaPasskeyVO, error) {
	return nil, nil
}
func (f *fakeMfaManagementFacade) ListPasskeysByUserID(context.Context, int64) ([]challengefacade.MfaPasskeyVO, error) {
	return nil, nil
}
func (f *fakeMfaManagementFacade) DeletePasskey(context.Context, challengefacade.MfaDeletePasskeyRequest) (bool, error) {
	return true, nil
}
func (f *fakeMfaManagementFacade) DeletePasskeyByUserID(_ context.Context, _ int64, credentialIdentifier string, proof stepup.ProofMetadata) (bool, error) {
	f.deletePasskeyByUserIDCalls++
	f.lastPasskeyCredentialIdentifier = credentialIdentifier
	f.lastProof = proof
	return true, nil
}
func (f *fakeMfaManagementFacade) DeletePasskeyWithChallenge(_ context.Context, request challengefacade.MfaDeletePasskeyRequest, _ challengefacade.MfaProtectedOperationContext) (bool, error) {
	f.deletePasskeyWithChallengeCalls++
	f.lastPasskeyCredentialIdentifier = request.CredentialIdentifier
	return true, nil
}
func (f *fakeMfaManagementFacade) StartMfaChallenge(context.Context, challengefacade.MfaChallengeStartRequest, challengefacade.MfaChallengeStartContext) (*challengefacade.StartChallengeResponse, error) {
	return &challengefacade.StartChallengeResponse{ChallengeIdentifier: "challenge-1", ChallengeState: "PENDING"}, nil
}
func (f *fakeMfaManagementFacade) StartMfaChallengeByUserID(context.Context, int64, challengefacade.MfaChallengeStartRequest, challengefacade.MfaChallengeStartContext) (*challengefacade.StartChallengeResponse, error) {
	return &challengefacade.StartChallengeResponse{ChallengeIdentifier: "challenge-1", ChallengeState: "PENDING"}, nil
}

func now() time.Time {
	return time.Now().UTC()
}

func pointerTime(value time.Time) *time.Time {
	return &value
}

func marshalManagementBody(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return body
}

type fakeChallengeAuthFacade struct {
	challenge *authorizationfacade.StepUpChallengeVO
}

func (f *fakeChallengeAuthFacade) GetLoginUser(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeChallengeAuthFacade) GetLoginUserPermitNull(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeChallengeAuthFacade) GetLoginUserID(context.Context, authorizationfacade.RequestScope) (int64, error) {
	return 0, nil
}
func (f *fakeChallengeAuthFacade) GetLoginUsername(context.Context, authorizationfacade.RequestScope) (string, error) {
	return "", nil
}
func (f *fakeChallengeAuthFacade) IsLogin(context.Context, authorizationfacade.RequestScope) bool {
	return true
}
func (f *fakeChallengeAuthFacade) IsAdmin(context.Context, authorizationfacade.RequestScope) bool {
	return false
}
func (f *fakeChallengeAuthFacade) IsCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return false
}
func (f *fakeChallengeAuthFacade) IsAdminOrCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return false
}
func (f *fakeChallengeAuthFacade) GetUserVO(context.Context, int64) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeChallengeAuthFacade) RefreshUserPermissionCache(context.Context, int64) error {
	return nil
}
func (f *fakeChallengeAuthFacade) GetUserPermissionsByModule(context.Context, authorizationfacade.RequestScope, string) ([]string, error) {
	return nil, nil
}
func (f *fakeChallengeAuthFacade) CreateStepUpChallenge(context.Context, authorizationfacade.RequestScope, authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	if f.challenge != nil {
		return f.challenge, nil
	}
	return &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-default", ChallengeState: "PENDING"}, nil
}
func (f *fakeChallengeAuthFacade) VerifyStepUp(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
	return &authorizationfacade.StepUpTokenVO{
		ProofToken:                request.ProofToken,
		ChallengeID:               "challenge-verify",
		TokenUniqueIdentifier:     "proof-jti-verify",
		BusinessAction:            request.BusinessAction,
		FlowNonce:                 request.FlowNonce,
		OperationBinding:          request.OperationBinding,
		AuthenticationMethodNames: []string{"TOTP"},
	}, nil
}
func (f *fakeChallengeAuthFacade) ValidateStepUpToken(context.Context, authorizationfacade.RequestScope, authorizationfacade.StepUpValidateRequest) (bool, error) {
	return true, nil
}

func mfaAuthHeaders() []ut.Header {
	return nil
}
