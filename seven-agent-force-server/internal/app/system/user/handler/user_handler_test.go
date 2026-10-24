package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestUserHandlerRoutes(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 1001, Username: "admin", PrimaryOrgID: 22, OrgIDs: []int64{22}, SessionID: "sid-1"})
		reqCtx.Next(ctx)
	})
	handler := NewHandler(&fakeProfileFacade{
		profile: &userfacade.UserProfile{UserID: 1001, AccountName: "admin", Email: "admin@example.com", Enabled: true},
	}, &fakeAccountFacade{verifyOK: true}, &fakeAuthFacade{})

	engine.GET("/user/profile/me", handler.GetCurrentUserProfile)
	engine.POST("/user/profile/update", handler.UpdateCurrentUserProfile)
	engine.POST("/user/profile/email/update", handler.UpdateCurrentUserEmail)
	engine.POST("/user/profile/change-password", handler.ChangeCurrentUserPassword)

	assertUserResultOK(t, ut.PerformRequest(engine.Engine, "GET", "/user/profile/me", nil, authHeaders()...))
	assertUserResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/user/profile/update", bodyOf(t, map[string]any{
		"nickName": "Admin",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, authHeaders()...)...))
	assertUserResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/user/profile/email/update", bodyOf(t, map[string]any{
		"userEmail": "new@example.com", "expectedEmailVersion": "1",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}, {Key: "Proof-Token", Value: "proof-token"}, {Key: "Flow-Nonce", Value: "flow-1"}}, authHeaders()...)...))
	assertUserResultOK(t, ut.PerformRequest(engine.Engine, "POST", "/user/profile/change-password", bodyOf(t, map[string]any{
		"oldPassword":     "OldPass123",
		"newPassword":     "NewPass123",
		"confirmPassword": "NewPass123",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}, {Key: "Proof-Token", Value: "proof-token"}, {Key: "Flow-Nonce", Value: "flow-password"}}, authHeaders()...)...))
}

func TestUserHandlerReturnsChallengeRequiredPayload(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 1001, Username: "admin", SessionID: "sid-1"})
		reqCtx.Next(ctx)
	})
	handler := NewHandler(&fakeProfileFacade{
		profile: &userfacade.UserProfile{UserID: 1001, AccountName: "admin", Phone: "13800138000", Enabled: true},
	}, &fakeAccountFacade{verifyOK: true}, &fakeAuthFacade{
		challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-1", ChallengeState: "PENDING"},
	})
	engine.POST("/user/profile/update", handler.UpdateCurrentUserProfile)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/profile/update", bodyOf(t, map[string]any{
		"userPhone": "13900139000",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, authHeaders()...)...)
	if resp.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":40120`)) || !bytes.Contains(resp.Body.Bytes(), []byte(`"challengeIdentifier":"challenge-1"`)) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
}

func TestUserHandlerChangePasswordRequiresStepUpAfterOldPassword(t *testing.T) {
	engine := userTestEngine()
	accounts := &fakeAccountFacade{verifyOK: true}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-password", ChallengeState: "PENDING"}}
	handler := NewHandler(&fakeProfileFacade{}, accounts, auth)
	engine.POST("/user/profile/change-password", handler.ChangeCurrentUserPassword)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/profile/change-password", bodyOf(t, map[string]any{
		"oldPassword":     "OldPass123",
		"newPassword":     "NewPass123",
		"confirmPassword": "NewPass123",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, authHeaders()...)...)

	assertUserBusinessCode(t, resp, 40120)
	if accounts.updateCalled {
		t.Fatal("UpdatePassword called before step-up proof")
	}
	if auth.lastChallenge.BusinessAction != "CURRENT_USER_PASSWORD_CHANGE" {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "user:1001|change-password" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestUserHandlerChangePasswordValidatesProofWithCanonicalBinding(t *testing.T) {
	engine := userTestEngine()
	accounts := &fakeAccountFacade{verifyOK: true}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewHandler(&fakeProfileFacade{}, accounts, auth)
	engine.POST("/user/profile/change-password", handler.ChangeCurrentUserPassword)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/profile/change-password", bodyOf(t, map[string]any{
		"oldPassword":     "OldPass123",
		"newPassword":     "NewPass123",
		"confirmPassword": "NewPass123",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}, {Key: "Proof-Token", Value: "proof-token"}, {Key: "Flow-Nonce", Value: "flow-password"}}, authHeaders()...)...)

	assertUserBusinessCode(t, resp, 0)
	if !accounts.updateCalled {
		t.Fatal("UpdatePassword was not called after valid proof")
	}
	if auth.lastValidate.BusinessAction != "CURRENT_USER_PASSWORD_CHANGE" {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-password" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "user:1001|change-password" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
}

func TestUserHandlerChangePasswordRejectsFalseStepUpValidation(t *testing.T) {
	engine := userTestEngine()
	accounts := &fakeAccountFacade{verifyOK: true}
	auth := &fakeAuthFacade{validateResult: boolPtr(false)}
	handler := NewHandler(&fakeProfileFacade{}, accounts, auth)
	engine.POST("/user/profile/change-password", handler.ChangeCurrentUserPassword)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/profile/change-password", bodyOf(t, map[string]any{
		"oldPassword":     "OldPass123",
		"newPassword":     "NewPass123",
		"confirmPassword": "NewPass123",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}, {Key: "Proof-Token", Value: "invalid-proof"}, {Key: "Flow-Nonce", Value: "flow-password"}}, authHeaders()...)...)

	assertUserBusinessCode(t, resp, 40300)
	if accounts.updateCalled {
		t.Fatal("UpdatePassword called after false step-up validation")
	}
	if auth.lastValidate.BusinessAction != "CURRENT_USER_PASSWORD_CHANGE" {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
}

func TestUserHandlerChangePasswordDoesNotChallengeWhenOldPasswordInvalid(t *testing.T) {
	engine := userTestEngine()
	accounts := &fakeAccountFacade{verifyOK: false}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-password", ChallengeState: "PENDING"}}
	handler := NewHandler(&fakeProfileFacade{}, accounts, auth)
	engine.POST("/user/profile/change-password", handler.ChangeCurrentUserPassword)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/profile/change-password", bodyOf(t, map[string]any{
		"oldPassword":     "wrong",
		"newPassword":     "NewPass123",
		"confirmPassword": "NewPass123",
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, authHeaders()...)...)

	assertUserBusinessCode(t, resp, 40000)
	if auth.lastChallenge.BusinessAction != "" {
		t.Fatalf("old password failure should not start challenge: %+v", auth.lastChallenge)
	}
	if accounts.updateCalled {
		t.Fatal("UpdatePassword called after invalid old password")
	}
}

func TestCommitCurrentUserAvatarDelegatesAtomicApplicationUseCase(t *testing.T) {
	engine := userTestEngine()
	profiles := &fakeProfileFacade{
		profile:      &userfacade.UserProfile{UserID: 1001, AccountName: "admin", Enabled: true},
		avatarResult: "/api/file/download?referenceId=ref-1",
	}
	handler := NewHandler(profiles, &fakeAccountFacade{verifyOK: true}, &fakeAuthFacade{})
	engine.POST("/user/profile/avatar/commit", handler.CommitCurrentUserAvatar)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/profile/avatar/commit", bodyOf(t, map[string]any{
		"fileId": 9001,
	}), append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, authHeaders()...)...)
	assertUserResultOK(t, resp)
	if profiles.avatarUserID != 1001 || profiles.avatarFileID != 9001 {
		t.Fatalf("controller did not delegate one avatar application call: user=%d file=%d", profiles.avatarUserID, profiles.avatarFileID)
	}
}

func TestAvatarControllerDelegatesAtomicApplicationUseCase(t *testing.T) {
	source, err := os.ReadFile("user_handler.go")
	if err != nil {
		t.Fatalf("read user_handler.go: %v", err)
	}
	handlerSource := string(source)
	if !strings.Contains(handlerSource, "c.profiles.CommitCurrentUserAvatar(ctx, userID, request.FileID)") {
		t.Fatal("avatar controller must delegate one atomic application use case")
	}
	for _, forbidden := range []string{"filefacade", "BindFileReference", "ListFileByBiz"} {
		if strings.Contains(handlerSource, forbidden) {
			t.Fatalf("avatar controller must not orchestrate file binding directly: found %q", forbidden)
		}
	}
}

type fakeProfileFacade struct {
	profile      *userfacade.UserProfile
	updateErr    error
	emailErr     error
	lastAvatar   *string
	avatarResult string
	avatarUserID int64
	avatarFileID int64
}

func (f *fakeProfileFacade) GetProfileByUserID(context.Context, int64) (*userfacade.UserProfile, error) {
	return f.profile, nil
}
func (f *fakeProfileFacade) UpdateSelfProfile(_ context.Context, command userfacade.UpdateSelfProfileCommand) error {
	if command.UserAvatar != nil {
		f.lastAvatar = command.UserAvatar
	}
	return f.updateErr
}
func (f *fakeProfileFacade) CommitCurrentUserAvatar(_ context.Context, userID, fileID int64) (string, error) {
	f.avatarUserID = userID
	f.avatarFileID = fileID
	return f.avatarResult, f.updateErr
}
func (f *fakeProfileFacade) UpdateSelfEmail(context.Context, userfacade.UpdateSelfEmailCommand) error {
	return f.emailErr
}
func (f *fakeProfileFacade) SyncExternalProfile(context.Context, userfacade.SyncExternalProfileCommand) error {
	return nil
}

type fakeAccountFacade struct {
	verifyOK     bool
	updateErr    error
	updateCalled bool
}

func (f *fakeAccountFacade) VerifyPassword(context.Context, int64, string) (bool, error) {
	return f.verifyOK, nil
}
func (f *fakeAccountFacade) UpdatePassword(context.Context, userfacade.UpdatePasswordCommand) error {
	f.updateCalled = true
	return f.updateErr
}
func (f *fakeAccountFacade) UpdateLockState(context.Context, userfacade.UpdateLockStateCommand) error {
	return nil
}

type fakeAuthFacade struct {
	challenge      *authorizationfacade.StepUpChallengeVO
	validate       *authorizationfacade.StepUpTokenVO
	validateResult *bool
	lastChallenge  authorizationfacade.StepUpChallengeRequest
	lastValidate   authorizationfacade.StepUpValidateRequest
}

func (f *fakeAuthFacade) GetLoginUser(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeAuthFacade) GetLoginUserPermitNull(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeAuthFacade) GetLoginUserID(context.Context, authorizationfacade.RequestScope) (int64, error) {
	return 0, nil
}
func (f *fakeAuthFacade) GetLoginUsername(context.Context, authorizationfacade.RequestScope) (string, error) {
	return "", nil
}
func (f *fakeAuthFacade) IsLogin(context.Context, authorizationfacade.RequestScope) bool {
	return true
}
func (f *fakeAuthFacade) IsAdmin(context.Context, authorizationfacade.RequestScope) bool {
	return false
}
func (f *fakeAuthFacade) IsCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return false
}
func (f *fakeAuthFacade) IsAdminOrCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return false
}
func (f *fakeAuthFacade) GetUserVO(context.Context, int64) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeAuthFacade) RefreshUserPermissionCache(context.Context, int64) error {
	return nil
}
func (f *fakeAuthFacade) GetUserPermissionsByModule(context.Context, authorizationfacade.RequestScope, string) ([]string, error) {
	return nil, nil
}
func (f *fakeAuthFacade) CreateStepUpChallenge(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	f.lastChallenge = request
	if f.challenge != nil {
		return f.challenge, nil
	}
	return &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-default", ChallengeState: "PENDING"}, nil
}
func (f *fakeAuthFacade) VerifyStepUp(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
	f.lastValidate = authorizationfacade.StepUpValidateRequest(request)
	if f.validateResult != nil && !*f.validateResult {
		return nil, nil
	}
	if f.validate != nil {
		copy := *f.validate
		if copy.ProofToken == "" {
			copy.ProofToken = request.ProofToken
		}
		if copy.ChallengeID == "" {
			copy.ChallengeID = "challenge-verify"
		}
		if copy.TokenUniqueIdentifier == "" {
			copy.TokenUniqueIdentifier = "proof-jti-verify"
		}
		if copy.BusinessAction == "" {
			copy.BusinessAction = request.BusinessAction
		}
		if copy.FlowNonce == "" {
			copy.FlowNonce = request.FlowNonce
		}
		if copy.OperationBinding == "" {
			copy.OperationBinding = request.OperationBinding
		}
		if len(copy.AuthenticationMethodNames) == 0 {
			copy.AuthenticationMethodNames = []string{"TOTP"}
		}
		return &copy, nil
	}
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
func (f *fakeAuthFacade) ValidateStepUpToken(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpValidateRequest) (bool, error) {
	f.lastValidate = request
	if f.validateResult != nil {
		return *f.validateResult, nil
	}
	return true, nil
}

func userTestEngine() *server.Hertz {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 1001, Username: "admin", SessionID: "sid-1"})
		reqCtx.Next(ctx)
	})
	return engine
}

func bodyOf(t *testing.T, payload any) *ut.Body {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return &ut.Body{Body: bytes.NewReader(body), Len: len(body)}
}

func assertUserResultOK(t *testing.T, recorder *ut.ResponseRecorder) {
	t.Helper()
	assertUserBusinessCode(t, recorder, 0)
}

func assertUserBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expectedCode int) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != expectedCode {
		t.Fatalf("unexpected business code: got=%d want=%d body=%s", result.Code, expectedCode, recorder.Body.String())
	}
}

type fakeUserSessionFacade struct{}

func authHeaders() []ut.Header {
	return nil
}
