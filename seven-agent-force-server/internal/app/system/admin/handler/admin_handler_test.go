package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestValidateKickTarget(t *testing.T) {
	t.Run("allow other user", func(t *testing.T) {
		if err := validateKickTarget(1, 2); err != nil {
			t.Fatalf("expected different user to be kickable, got error: %v", err)
		}
	})

	t.Run("reject self kick", func(t *testing.T) {
		err := validateKickTarget(3, 3)
		if err == nil {
			t.Fatal("expected self kick to be rejected")
		}
		if got := err.Error(); got != "不能踢掉自己，请使用其他管理员账号操作" {
			t.Fatalf("unexpected error message: %s", got)
		}
	})
}

func TestGetOperationTypesReturnsServerOwnedLabels(t *testing.T) {
	engine := adminTestEngine()
	handler := NewHandler(nil, nil, &fakeOperationLogService{
		operationTypes: []adminfacade.OperationTypeOption{
			{Value: "CONFIG_UPDATE", Label: "更新配置"},
		},
	}, nil, nil)
	engine.GET("/admin/logs/operation/types", handler.GetOperationTypes)

	resp := ut.PerformRequest(engine.Engine, "GET", "/admin/logs/operation/types", nil)
	if resp.Code != 200 {
		t.Fatalf("unexpected http status: %d body=%s", resp.Code, resp.Body.String())
	}

	var result struct {
		Code int                               `json:"code"`
		Data []adminfacade.OperationTypeOption `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result.Code != 0 {
		t.Fatalf("unexpected business code: %d", result.Code)
	}
	if !reflect.DeepEqual(result.Data, []adminfacade.OperationTypeOption{{Value: "CONFIG_UPDATE", Label: "更新配置"}}) {
		t.Fatalf("unexpected type options: %#v", result.Data)
	}
}

func TestKickUserRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	online := &fakeOnlineUserService{}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-kick", ChallengeState: "PENDING"}}
	handler := NewHandler(nil, online, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/admin/kick/:userId", handler.KickUser)

	resp := ut.PerformRequest(engine.Engine, "POST", "/admin/kick/1002", nil)

	assertAdminBusinessCode(t, resp, 40120)
	if online.forceLogoutCalled {
		t.Fatal("ForceLogout called before step-up proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionAdminForceLogout) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "user:1002|force-logout" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestKickUserValidatesStepUpProofWithCanonicalBinding(t *testing.T) {
	engine := adminTestEngine()
	online := &fakeOnlineUserService{}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewHandler(nil, online, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/admin/kick/:userId", handler.KickUser)

	resp := ut.PerformRequest(engine.Engine, "POST", "/admin/kick/1002", nil, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-kick"})

	assertAdminBusinessCode(t, resp, 0)
	if !online.forceLogoutCalled {
		t.Fatal("ForceLogout was not called after valid step-up proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionAdminForceLogout) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-kick" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "user:1002|force-logout" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if online.lastForceLogout.StepUpProof.BusinessAction != string(challengedomain.BusinessActionAdminForceLogout) ||
		online.lastForceLogout.StepUpProof.OperationBinding != "user:1002|force-logout" ||
		online.lastForceLogout.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		online.lastForceLogout.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected force logout proof metadata: %#v", online.lastForceLogout.StepUpProof)
	}
}

func TestBatchKickUsersRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	online := &fakeOnlineUserService{}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-batch-kick", ChallengeState: "PENDING"}}
	handler := NewHandler(nil, online, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/admin/kick/batch", handler.BatchKickUsers)

	resp := ut.PerformRequest(engine.Engine, "POST", "/admin/kick/batch", jsonBody(t, map[string]any{"userIds": []int64{1003, 1002, 1002}}))

	assertAdminBusinessCode(t, resp, 40120)
	if online.batchForceLogoutCalled {
		t.Fatal("BatchForceLogout called before step-up proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionAdminForceLogout) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "users:1002,1003|force-logout" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestBatchKickUsersAcceptsStringUserIDArray(t *testing.T) {
	engine := adminTestEngine()
	online := &fakeOnlineUserService{}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-batch-kick", ChallengeState: "PENDING"}}
	handler := NewHandler(nil, online, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/admin/kick/batch", handler.BatchKickUsers)

	resp := ut.PerformRequest(engine.Engine, "POST", "/admin/kick/batch", jsonBody(t, []string{"2065424359060983808", "2065424246313897984"}))

	assertAdminBusinessCode(t, resp, 40120)
	if online.batchForceLogoutCalled {
		t.Fatal("BatchForceLogout called before step-up proof")
	}
	if auth.lastChallenge.OperationBinding != "users:2065424246313897984,2065424359060983808|force-logout" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestBatchKickUsersUsesCompactBindingForLargeStringUserIDArray(t *testing.T) {
	engine := adminTestEngine()
	online := &fakeOnlineUserService{}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-batch-kick", ChallengeState: "PENDING"}}
	handler := NewHandler(nil, online, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/admin/kick/batch", handler.BatchKickUsers)

	userIDs := make([]string, 0, 100)
	parsed := make([]int64, 0, 100)
	for i := int64(0); i < 100; i++ {
		id := 2065424246313897984 + i
		userIDs = append(userIDs, strconv.FormatInt(id, 10))
		parsed = append(parsed, id)
	}

	resp := ut.PerformRequest(engine.Engine, "POST", "/admin/kick/batch", jsonBody(t, userIDs))

	assertAdminBusinessCode(t, resp, 40120)
	if online.batchForceLogoutCalled {
		t.Fatal("BatchForceLogout called before step-up proof")
	}
	want := batchForceLogoutBinding(parsed)
	if auth.lastChallenge.OperationBinding != want {
		t.Fatalf("unexpected compact operation binding: got=%q want=%q", auth.lastChallenge.OperationBinding, want)
	}
	if len(auth.lastChallenge.OperationBinding) > 128 {
		t.Fatalf("expected compact operation binding, got len=%d binding=%q", len(auth.lastChallenge.OperationBinding), auth.lastChallenge.OperationBinding)
	}
	if !strings.HasPrefix(auth.lastChallenge.OperationBinding, "users:count=100,sha256=") {
		t.Fatalf("expected count+sha256 binding, got %q", auth.lastChallenge.OperationBinding)
	}
}

func TestBatchKickUsersValidatesStepUpProofWithCanonicalBinding(t *testing.T) {
	engine := adminTestEngine()
	online := &fakeOnlineUserService{}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewHandler(nil, online, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/admin/kick/batch", handler.BatchKickUsers)

	resp := ut.PerformRequest(engine.Engine, "POST", "/admin/kick/batch", jsonBody(t, map[string]any{"userIds": []int64{1003, 1002, 1002}}), ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-batch-kick"})

	assertAdminBusinessCode(t, resp, 0)
	if !online.batchForceLogoutCalled {
		t.Fatal("BatchForceLogout was not called after valid step-up proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionAdminForceLogout) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-batch-kick" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "users:1002,1003|force-logout" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if online.lastBatchForceLogout.StepUpProof.BusinessAction != string(challengedomain.BusinessActionAdminForceLogout) ||
		online.lastBatchForceLogout.StepUpProof.OperationBinding != "users:1002,1003|force-logout" ||
		online.lastBatchForceLogout.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		online.lastBatchForceLogout.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected batch force logout proof metadata: %#v", online.lastBatchForceLogout.StepUpProof)
	}
}

func TestBatchKickUsersAcceptsObjectStringUserIDsWithProof(t *testing.T) {
	engine := adminTestEngine()
	online := &fakeOnlineUserService{}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewHandler(nil, online, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/admin/kick/batch", handler.BatchKickUsers)

	resp := ut.PerformRequest(
		engine.Engine,
		"POST",
		"/admin/kick/batch",
		jsonBody(t, map[string]any{"userIds": []string{"2065424359060983808", "2065424246313897984"}}),
		ut.Header{Key: "Proof-Token", Value: "proof-token"},
		ut.Header{Key: "Flow-Nonce", Value: "flow-batch-kick"},
	)

	assertAdminBusinessCode(t, resp, 0)
	if !online.batchForceLogoutCalled {
		t.Fatal("BatchForceLogout was not called after valid proof")
	}
	want := []int64{2065424359060983808, 2065424246313897984}
	if !reflect.DeepEqual(online.lastBatchForceLogout.UserIDs, want) {
		t.Fatalf("unexpected parsed user ids: got=%v want=%v", online.lastBatchForceLogout.UserIDs, want)
	}
	if auth.lastValidate.OperationBinding != "users:2065424246313897984,2065424359060983808|force-logout" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
}

func adminTestEngine() *server.Hertz {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "admin", SessionID: "sid-admin", IsAdmin: true})
		reqCtx.Next(ctx)
	})
	return engine
}

func assertAdminBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expectedCode int) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected http status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != expectedCode {
		t.Fatalf("unexpected business code: got=%d want=%d body=%s", result.Code, expectedCode, recorder.Body.String())
	}
}

func jsonBody(t *testing.T, payload any) *ut.Body {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal json body: %v", err)
	}
	return &ut.Body{Body: bytes.NewReader(data), Len: len(data)}
}

type fakeOnlineUserService struct {
	forceLogoutCalled      bool
	lastForceLogout        adminfacade.ForceLogoutCommand
	batchForceLogoutCalled bool
	lastBatchForceLogout   adminfacade.BatchForceLogoutCommand
}

func (f *fakeOnlineUserService) GetOnlineUsers(context.Context, int64, int64, string, string, string, string, string) (*adminfacade.PageResult[adminfacade.OnlineUserVO], error) {
	return &adminfacade.PageResult[adminfacade.OnlineUserVO]{}, nil
}
func (f *fakeOnlineUserService) GetOnlineUserStats(context.Context) (*adminfacade.OnlineUserStatsVO, error) {
	return &adminfacade.OnlineUserStatsVO{}, nil
}
func (f *fakeOnlineUserService) GetUserSession(context.Context, int64, string) (*adminfacade.OnlineUserVO, error) {
	return &adminfacade.OnlineUserVO{}, nil
}
func (f *fakeOnlineUserService) ForceLogout(_ context.Context, command adminfacade.ForceLogoutCommand) (bool, error) {
	f.forceLogoutCalled = true
	f.lastForceLogout = command
	return true, nil
}
func (f *fakeOnlineUserService) BatchForceLogout(_ context.Context, command adminfacade.BatchForceLogoutCommand) (*adminfacade.BatchLogoutResultVO, error) {
	f.batchForceLogoutCalled = true
	f.lastBatchForceLogout = command
	return &adminfacade.BatchLogoutResultVO{SuccessIDs: command.UserIDs, TotalCount: len(command.UserIDs), SuccessCount: len(command.UserIDs)}, nil
}
func (f *fakeOnlineUserService) GetOnlineUserCount(context.Context) (int64, error) { return 0, nil }
func (f *fakeOnlineUserService) IsUserOnline(context.Context, int64) (bool, error) {
	return true, nil
}

type fakeOperationLogService struct {
	OperationLogService
	operationTypes []adminfacade.OperationTypeOption
}

func (f *fakeOperationLogService) GetOperationTypes(context.Context) []adminfacade.OperationTypeOption {
	return f.operationTypes
}

type fakeAuthFacade struct {
	authorizationfacade.AuthFacade
	challenge     *authorizationfacade.StepUpChallengeVO
	validate      *authorizationfacade.StepUpTokenVO
	lastChallenge authorizationfacade.StepUpChallengeRequest
	lastValidate  authorizationfacade.StepUpVerifyRequest
}

func (f *fakeAuthFacade) CreateStepUpChallenge(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	f.lastChallenge = request
	if f.challenge != nil {
		return f.challenge, nil
	}
	return &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-id", ChallengeState: "PENDING"}, nil
}

func (f *fakeAuthFacade) VerifyStepUp(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
	f.lastValidate = request
	if f.validate != nil {
		token := *f.validate
		token.BusinessAction = request.BusinessAction
		token.FlowNonce = request.FlowNonce
		token.OperationBinding = request.OperationBinding
		token.TokenUniqueIdentifier = "proof-jti-verify"
		token.ChallengeID = "challenge-verify"
		token.AuthenticationMethodNames = []string{"TIME_BASED_ONE_TIME_PASSWORD"}
		return &token, nil
	}
	return nil, nil
}
