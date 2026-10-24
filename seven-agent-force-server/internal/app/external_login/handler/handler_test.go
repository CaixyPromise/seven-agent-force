package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestExternalLoginRoutesMountPublicProvidersStartAndCallback(t *testing.T) {
	service := &fakeExternalLoginService{
		methods: []facade.LoginMethodRecord{{ProviderCode: "github", DisplayName: "GitHub"}},
		start:   &facade.StartExternalLoginResult{RedirectURL: "https://github.example.com/oauth", StateID: "state-1"},
		result:  &facade.ExternalLoginResult{Authenticated: true, UserID: 1001},
	}
	engine := externalLoginTestEngine(NewHandler(service), false, "")

	assertBusinessCode(t, ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/providers", nil), apperrors.CodeSuccess)
	startResp := ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/github/start?loginTransactionId=ltx-1", nil)
	if startResp.Code != http.StatusFound {
		t.Fatalf("start status=%d body=%s", startResp.Code, startResp.Body.String())
	}
	if location := startResp.Header().Get("Location"); location != "https://github.example.com/oauth" {
		t.Fatalf("start redirect location=%q", location)
	}
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/github/callback?code=c&state=s", nil), apperrors.CodeSuccess)
	if service.startProvider != "github" || service.callbackProvider != "github" {
		t.Fatalf("provider path not forwarded to service: start=%q callback=%q", service.startProvider, service.callbackProvider)
	}
}

func TestAdminProviderMutationStartsStepUpWhenProofMissing(t *testing.T) {
	service := &fakeExternalLoginService{}
	auth := &fakeExternalLoginAuth{challenge: &authorizationfacade.StepUpChallengeVO{
		ChallengeIdentifier:        "challenge-1",
		ChallengeState:             "PENDING",
		EffectiveTimeToLiveSeconds: 300,
		RequiredAssuranceLevel:     "AAL2",
	}}
	handler := NewHandler(service)
	handler.BindAuthorization(auth)
	engine := externalLoginTestEngine(handler, true, "system:external-login-provider:add")

	resp := ut.PerformRequest(engine.Engine, http.MethodPost, "/external-login/admin/providers", jsonBody(t, map[string]any{"providerCode": "github"}),
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Test-Login", Value: "true"},
	)
	assertBusinessCode(t, resp, apperrors.CodeChallengeRequired)
	if service.createCalls != 0 {
		t.Fatalf("provider create should not execute before proof, got %d", service.createCalls)
	}
	if auth.lastChallenge.BusinessAction != "EXTERNAL_LOGIN_PROVIDER_CREATE" || auth.lastChallenge.OperationBinding != "external-login:provider:github|create" {
		t.Fatalf("unexpected step-up challenge request: %#v", auth.lastChallenge)
	}
}

func TestAdminProviderStatusConsumesMatchingStepUpProof(t *testing.T) {
	service := &fakeExternalLoginService{}
	auth := &fakeExternalLoginAuth{}
	handler := NewHandler(service)
	handler.BindAuthorization(auth)
	engine := externalLoginTestEngine(handler, true, "system:external-login-provider:status")

	resp := ut.PerformRequest(engine.Engine, http.MethodPut, "/external-login/admin/providers/github/status", jsonBody(t, map[string]any{"status": 1, "reason": "risk"}),
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "Proof-Token", Value: "proof-token"},
		ut.Header{Key: "Flow-Nonce", Value: "flow-1"},
	)
	assertBusinessCode(t, resp, apperrors.CodeSuccess)
	if auth.lastVerify.BusinessAction != "EXTERNAL_LOGIN_PROVIDER_STATUS_CHANGE" ||
		auth.lastVerify.OperationBinding != "external-login:provider:github|status:1" ||
		auth.lastVerify.FlowNonce != "flow-1" ||
		!auth.lastVerify.ConsumeOnce {
		t.Fatalf("unexpected proof verification request: %#v", auth.lastVerify)
	}
	if service.providerStatusProof.ProofIdentifier == "" || service.providerStatusProof.OperationBinding != "external-login:provider:github|status:1" {
		t.Fatalf("service did not receive consumed proof metadata: %#v", service.providerStatusProof)
	}
}

func TestExternalLoginCallbackWritesLoginCookies(t *testing.T) {
	service := &fakeExternalLoginService{result: &facade.ExternalLoginResult{
		Authenticated:            true,
		SessionCookieHeaderValue: "SEVEN_SSO_SESSION=session; Path=/",
		RefreshCookieHeaderValue: "__Host-seven_sso_rt=refresh; Path=/",
	}}
	engine := externalLoginTestEngine(NewHandler(service), false, "")

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/github/callback?code=c&state=s", nil)
	assertBusinessCode(t, resp, apperrors.CodeSuccess)
	cookies := string(resp.Header().Peek("Set-Cookie"))
	if !strings.Contains(cookies, "SEVEN_SSO_SESSION=session") || !strings.Contains(cookies, "__Host-seven_sso_rt=refresh") {
		t.Fatalf("expected login cookies, got %q", cookies)
	}
	if body := resp.Body.String(); strings.Contains(body, "SEVEN_SSO_SESSION=session") || strings.Contains(body, "__Host-seven_sso_rt=refresh") {
		t.Fatalf("callback response leaked cookie values: %s", body)
	}
}

func TestExternalBindingCallbackNeverWritesLoginCookies(t *testing.T) {
	service := &fakeExternalLoginService{result: &facade.ExternalLoginResult{
		Authenticated:            false,
		Outcome:                  "BOUND",
		Purpose:                  "IDENTITY_LINK",
		SessionCookieHeaderValue: "SEVEN_SSO_SESSION=must-not-be-written; Path=/",
	}}
	engine := externalLoginTestEngine(NewHandler(service), false, "")

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/github/callback?code=c&state=s", nil)
	assertBusinessCode(t, resp, apperrors.CodeSuccess)
	if cookies := string(resp.Header().Peek("Set-Cookie")); cookies != "" {
		t.Fatalf("binding callback wrote login cookie: %q", cookies)
	}
}

func TestExternalLoginCallbackRejectsProviderMismatch(t *testing.T) {
	service := &fakeExternalLoginService{rejectCallbackProvider: "google"}
	engine := externalLoginTestEngine(NewHandler(service), false, "")

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/login/external/github/callback?code=c&state=s&providerCode=google", nil)
	assertBusinessCode(t, resp, apperrors.CodeParamsError)
	if service.callbackCalls != 0 {
		t.Fatalf("callback service should not be called on provider mismatch, got %d", service.callbackCalls)
	}
}

func TestAdminIdentityListRouteReturnsServicePage(t *testing.T) {
	service := &fakeExternalLoginService{identityPage: &facade.IdentityPage{
		Records: []facade.ExternalIdentityRecord{{ID: 10, ProviderCode: "github", ExternalSubject: "sub-1", UserID: 1001}},
		Total:   1,
	}}
	handler := NewHandler(service)
	handler.BindAuthorization(&fakeExternalLoginAuth{})
	engine := externalLoginTestEngine(handler, true, "system:external-login-identity:list")

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/external-login/admin/identities?providerCode=github", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	)
	assertBusinessCode(t, resp, apperrors.CodeSuccess)
	if service.listIdentityCalls != 1 {
		t.Fatalf("identity list service calls=%d want 1", service.listIdentityCalls)
	}
}

func externalLoginTestEngine(handler *Handler, admin bool, permissions string) *server.Hertz {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "true" || !admin {
			securitycontext.Set(reqCtx, &securitycontext.UserContext{
				UserID:      1001,
				Username:    "admin",
				Permissions: splitPermissions(permissions),
				SessionID:   "sid-1",
			})
		}
		reqCtx.Next(ctx)
	})
	engine.GET("/login/external/providers", handler.ListLoginMethods)
	engine.GET("/login/external/:providerCode/start", handler.StartExternalLogin)
	engine.GET("/login/external/:providerCode/callback", handler.CompleteExternalCallback)
	engine.POST("/external-login/admin/providers", handler.CreateProvider)
	engine.PUT("/external-login/admin/providers/:providerCode/status", handler.UpdateProviderStatus)
	engine.GET("/external-login/admin/identities", handler.ListIdentities)
	return engine
}

func jsonBody(t *testing.T, value any) *ut.Body {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json body: %v", err)
	}
	return &ut.Body{Body: bytes.NewReader(raw), Len: len(raw)}
}

func assertBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, want int) {
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

func splitPermissions(raw string) []string {
	if raw == "" {
		return nil
	}
	return []string{raw}
}

type fakeExternalLoginService struct {
	methods                []facade.LoginMethodRecord
	start                  *facade.StartExternalLoginResult
	result                 *facade.ExternalLoginResult
	rejectCallbackProvider string
	startProvider          string
	callbackProvider       string
	callbackCalls          int
	createCalls            int
	providerStatusProof    stepup.ProofMetadata
	identityPage           *facade.IdentityPage
	listIdentityCalls      int
}

func (f *fakeExternalLoginService) ListLoginMethods(context.Context, facade.ListLoginMethodsRequest) ([]facade.LoginMethodRecord, error) {
	return f.methods, nil
}

func (f *fakeExternalLoginService) StartExternalLogin(_ context.Context, req facade.StartExternalLoginRequest) (*facade.StartExternalLoginResult, error) {
	f.startProvider = req.ProviderCode
	if f.start != nil {
		return f.start, nil
	}
	return &facade.StartExternalLoginResult{RedirectURL: "https://provider.example.com/auth", StateID: "state-1"}, nil
}

func (f *fakeExternalLoginService) CompleteExternalCallback(_ context.Context, req facade.CompleteExternalCallbackRequest) (*facade.ExternalLoginResult, error) {
	f.callbackCalls++
	f.callbackProvider = req.ProviderCode
	if f.result != nil {
		return f.result, nil
	}
	return &facade.ExternalLoginResult{Authenticated: true}, nil
}

func (f *fakeExternalLoginService) ProviderCapabilities(context.Context) facade.ProviderCapabilityCatalog {
	return nil
}

func (f *fakeExternalLoginService) ListProviderCapabilities(context.Context) facade.ProviderCapabilityCatalog {
	return nil
}

func (f *fakeExternalLoginService) ListProviderMethods(context.Context, string) ([]facade.ProviderMethodDescriptor, error) {
	return nil, nil
}

func (f *fakeExternalLoginService) ListProviders(context.Context, facade.ProviderQuery) (*facade.ProviderPage, error) {
	return nil, nil
}

func (f *fakeExternalLoginService) GetProvider(context.Context, string) (*facade.ProviderDetail, error) {
	return nil, nil
}

func (f *fakeExternalLoginService) CreateProvider(context.Context, int64, facade.ProviderSaveRequest, stepup.ProofMetadata) (*facade.ProviderDetail, error) {
	f.createCalls++
	return &facade.ProviderDetail{}, nil
}

func (f *fakeExternalLoginService) UpdateProvider(context.Context, int64, string, facade.ProviderUpdateRequest, stepup.ProofMetadata) (*facade.ProviderDetail, error) {
	return &facade.ProviderDetail{}, nil
}

func (f *fakeExternalLoginService) UpdateProviderStatus(_ context.Context, _ int64, _ string, _ facade.ProviderStatusRequest, proof stepup.ProofMetadata) error {
	f.providerStatusProof = proof
	return nil
}

func (f *fakeExternalLoginService) RotateClientSecret(context.Context, int64, string, facade.RotateClientSecretRequest, stepup.ProofMetadata) error {
	return nil
}

func (f *fakeExternalLoginService) ListIdentities(context.Context, facade.IdentityQuery) (*facade.IdentityPage, error) {
	f.listIdentityCalls++
	if f.identityPage != nil {
		return f.identityPage, nil
	}
	return &facade.IdentityPage{}, nil
}

func (f *fakeExternalLoginService) ListCurrentUserBindings(context.Context, int64) ([]facade.CurrentUserBinding, error) {
	return nil, nil
}

func (f *fakeExternalLoginService) UpdateIdentityStatus(context.Context, int64, int64, facade.IdentityStatusRequest, stepup.ProofMetadata) error {
	return nil
}

func (f *fakeExternalLoginService) ResolveIdentity(context.Context, string, string) (*facade.ExternalIdentityRecord, error) {
	return nil, nil
}

func (f *fakeExternalLoginService) ListTokens(context.Context, facade.TokenQuery) (*facade.TokenPage, error) {
	return nil, nil
}

func (f *fakeExternalLoginService) AcquireAccessToken(context.Context, facade.AcquireAccessTokenRequest) (*facade.AccessTokenLease, error) {
	return nil, nil
}

func (f *fakeExternalLoginService) RefreshToken(context.Context, int64) error {
	return nil
}

func (f *fakeExternalLoginService) RevokeToken(context.Context, int64, int64, string, stepup.ProofMetadata) error {
	return nil
}

func (f *fakeExternalLoginService) RevokeTokensByProvider(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (f *fakeExternalLoginService) RevokeTokensByIdentity(context.Context, int64, string) (int64, error) {
	return 0, nil
}

type fakeExternalLoginAuth struct {
	challenge     *authorizationfacade.StepUpChallengeVO
	lastChallenge authorizationfacade.StepUpChallengeRequest
	lastVerify    authorizationfacade.StepUpVerifyRequest
}

func (f *fakeExternalLoginAuth) CreateStepUpChallenge(_ context.Context, _ authorizationfacade.RequestScope, req authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	f.lastChallenge = req
	if f.challenge != nil {
		return f.challenge, nil
	}
	return &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-1", ChallengeState: "PENDING"}, nil
}

func (f *fakeExternalLoginAuth) VerifyStepUp(_ context.Context, _ authorizationfacade.RequestScope, req authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
	f.lastVerify = req
	return &authorizationfacade.StepUpTokenVO{
		ProofToken:                req.ProofToken,
		ChallengeID:               "challenge-1",
		TokenUniqueIdentifier:     "proof-jti",
		BusinessAction:            req.BusinessAction,
		OperationBinding:          req.OperationBinding,
		AuthenticationMethodNames: []string{"TOTP"},
	}, nil
}

func (f *fakeExternalLoginAuth) GetLoginUser(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeExternalLoginAuth) GetLoginUserPermitNull(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeExternalLoginAuth) GetLoginUserID(context.Context, authorizationfacade.RequestScope) (int64, error) {
	return 0, nil
}
func (f *fakeExternalLoginAuth) GetLoginUsername(context.Context, authorizationfacade.RequestScope) (string, error) {
	return "", nil
}
func (f *fakeExternalLoginAuth) IsLogin(context.Context, authorizationfacade.RequestScope) bool {
	return true
}
func (f *fakeExternalLoginAuth) IsAdmin(context.Context, authorizationfacade.RequestScope) bool {
	return true
}
func (f *fakeExternalLoginAuth) IsCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return true
}
func (f *fakeExternalLoginAuth) IsAdminOrCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return true
}
func (f *fakeExternalLoginAuth) GetUserVO(context.Context, int64) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeExternalLoginAuth) RefreshUserPermissionCache(context.Context, int64) error {
	return nil
}
func (f *fakeExternalLoginAuth) GetUserPermissionsByModule(context.Context, authorizationfacade.RequestScope, string) ([]string, error) {
	return nil, nil
}
func (f *fakeExternalLoginAuth) ValidateStepUpToken(context.Context, authorizationfacade.RequestScope, authorizationfacade.StepUpValidateRequest) (bool, error) {
	return true, nil
}
