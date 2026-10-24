package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestAdminHandlerAssignUserRolesRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	relations := &fakeUserRelationFacade{}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-user-role", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(users, relations, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/:id/roles", handler.AssignUserRoles)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/1001/roles", bodyOf(t, map[string]any{
		"roleIds": []int64{2, 1, 2},
	}), ut.Header{Key: "Content-Type", Value: "application/json"})

	assertAdminBusinessCode(t, resp, 40120)
	if relations.assignRolesCalled {
		t.Fatal("AssignUserRoles called relation facade without step-up proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionRBACAssignUserRoles) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "user:1001|roles:1,2" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestAdminHandlerEffectiveAccessBindsQuery(t *testing.T) {
	engine := adminTestEngine()
	access := &fakeAccessExplainFacade{effective: &authorizationfacade.EffectiveAccessVO{UserID: 1001, Username: "target"}}
	handler := NewAdminHandler(&fakeAdminUserFacade{}, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAccessExplain(access)
	engine.GET("/system/user/:id/effective-access", handler.GetEffectiveAccess)

	resp := ut.PerformRequest(engine.Engine, "GET", "/system/user/1001/effective-access?current=2&size=15&keyword=user&sourceType=POST&effective=false", nil)
	assertAdminBusinessCode(t, resp, 0)
	if access.effectiveUserID != 1001 || access.query.Current != 2 || access.query.Size != 15 || access.query.Keyword != "user" || access.query.SourceType != "POST" {
		t.Fatalf("unexpected effective access request: user=%d query=%#v", access.effectiveUserID, access.query)
	}
	if access.query.Effective == nil || *access.query.Effective {
		t.Fatalf("expected effective=false filter, got %#v", access.query.Effective)
	}
}

func TestAdminHandlerCreatePostAcceptsStringDepartmentID(t *testing.T) {
	engine := adminTestEngine()
	posts := &fakePostFacade{}
	depts := &fakeDeptFacade{dept: &userfacade.DeptVO{ID: 2078559343971979264, OrgID: 1}}
	handler := NewAdminHandler(&fakeAdminUserFacade{}, &fakeUserRelationFacade{}, nil, depts, posts)
	engine.POST("/system/post", handler.CreatePost)

	resp := ut.PerformRequest(engine.Engine, "POST", "/system/post", bodyOf(t, map[string]any{
		"code":   "AUDITOR",
		"name":   "审计岗位",
		"deptId": "2078559343971979264",
	}), ut.Header{Key: "Content-Type", Value: "application/json"})

	assertAdminBusinessCode(t, resp, 0)
	if !posts.createCalled || int64(posts.lastCreate.DeptID) != 2078559343971979264 {
		t.Fatalf("expected exact department id binding, got called=%v command=%#v", posts.createCalled, posts.lastCreate)
	}
}

func TestAdminHandlerExplainPermissionRejectsOutOfScopeTarget(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, DataScopeType: securitycontext.DataScopeNone})
		reqCtx.Next(ctx)
	})
	access := &fakeAccessExplainFacade{}
	handler := NewAdminHandler(&fakeAdminUserFacade{}, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAccessExplain(access)
	engine.GET("/system/user/:id/access-explain", handler.ExplainPermission)

	resp := ut.PerformRequest(engine.Engine, "GET", "/system/user/1001/access-explain?permissionCode=system:user:update", nil)
	assertAdminBusinessCode(t, resp, 40310)
	if access.explainCalled {
		t.Fatal("out-of-scope request reached the access explain facade")
	}
	if !strings.Contains(resp.Body.String(), `"reasonCode":"DATA_SCOPE_DENIED"`) {
		t.Fatalf("missing stable data scope denial reason: %s", resp.Body.String())
	}
}

func TestAdminHandlerAssignUserRolesValidatesProofWithCanonicalBinding(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	relations := &fakeUserRelationFacade{}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(users, relations, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/:id/roles", handler.AssignUserRoles)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/1001/roles", bodyOf(t, map[string]any{
		"roleIds": []int64{2, 1, 2},
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-1"})

	assertAdminBusinessCode(t, resp, 0)
	if !relations.assignRolesCalled {
		t.Fatal("AssignUserRoles did not call relation facade after valid proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionRBACAssignUserRoles) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-1" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "user:1001|roles:1,2" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if relations.lastAssignRoles.StepUpProof.BusinessAction != string(challengedomain.BusinessActionRBACAssignUserRoles) ||
		relations.lastAssignRoles.StepUpProof.OperationBinding != "user:1001|roles:1,2" ||
		relations.lastAssignRoles.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		relations.lastAssignRoles.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected step-up proof metadata: %#v", relations.lastAssignRoles.StepUpProof)
	}
}

func TestAdminHandlerCreateUserWithRolesRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-create-role", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/create", handler.CreateUser)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/create", bodyOf(t, map[string]any{
		"username": "created",
		"nickname": "Created User",
		"password": "Secret123",
		"roleIds":  []int64{5, 3, 3},
	}), ut.Header{Key: "Content-Type", Value: "application/json"})

	assertAdminBusinessCode(t, resp, 40120)
	if users.createCalled {
		t.Fatal("CreateAdminUser called before role assignment proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionRBACAssignUserRoles) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "user:create:created|roles:3,5" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestAdminHandlerCreateUserWithoutRolesDoesNotRequireStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{createdID: 2002}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	engine.POST("/user/create", handler.CreateUser)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/create", bodyOf(t, map[string]any{
		"username": "plain",
		"nickname": "Plain User",
		"password": "Secret123",
	}), ut.Header{Key: "Content-Type", Value: "application/json"})

	assertAdminBusinessCode(t, resp, 0)
	if !users.createCalled {
		t.Fatal("CreateAdminUser was not called for role-free user creation")
	}
}

func TestAdminHandlerCreateUserWithRolesValidatesProofWithCreateBinding(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{createdID: 2001}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/create", handler.CreateUser)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/create", bodyOf(t, map[string]any{
		"username": "created",
		"nickname": "Created User",
		"password": "Secret123",
		"roleIds":  []int64{5, 3, 3},
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-create"})

	assertAdminBusinessCode(t, resp, 0)
	if !users.createCalled {
		t.Fatal("CreateAdminUser was not called after valid proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionRBACAssignUserRoles) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-create" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "user:create:created|roles:3,5" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if users.lastCreate.StepUpProof.BusinessAction != string(challengedomain.BusinessActionRBACAssignUserRoles) ||
		users.lastCreate.StepUpProof.OperationBinding != "user:create:created|roles:3,5" ||
		users.lastCreate.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		users.lastCreate.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected create step-up proof metadata: %#v", users.lastCreate.StepUpProof)
	}
}

func TestAdminHandlerAssignPostRolesRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	posts := &fakePostFacade{post: &userfacade.PostVO{ID: 2001, OrgID: 1, DeptID: 2}}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-post-role", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(&fakeAdminUserFacade{}, &fakeUserRelationFacade{}, nil, nil, posts)
	handler.BindAuthorization(auth)
	engine.POST("/system/post/:postId/roles", handler.AssignPostRoles)

	resp := ut.PerformRequest(engine.Engine, "POST", "/system/post/2001/roles", bodyOf(t, map[string]any{
		"roleIds": []int64{2, 1, 2},
	}), ut.Header{Key: "Content-Type", Value: "application/json"})

	assertAdminBusinessCode(t, resp, 40120)
	if posts.assignRolesCalled {
		t.Fatal("AssignPostRoles called post facade without step-up proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionRBACAssignPostRoles) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "post:2001|roles:1,2" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestAdminHandlerAssignPostRolesValidatesProofWithCanonicalBinding(t *testing.T) {
	engine := adminTestEngine()
	posts := &fakePostFacade{post: &userfacade.PostVO{ID: 2001, OrgID: 1, DeptID: 2}}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(&fakeAdminUserFacade{}, &fakeUserRelationFacade{}, nil, nil, posts)
	handler.BindAuthorization(auth)
	engine.POST("/system/post/:postId/roles", handler.AssignPostRoles)

	resp := ut.PerformRequest(engine.Engine, "POST", "/system/post/2001/roles", bodyOf(t, map[string]any{
		"roleIds": []int64{2, 1, 2},
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-post"})

	assertAdminBusinessCode(t, resp, 0)
	if !posts.assignRolesCalled {
		t.Fatal("AssignPostRoles was not called after valid proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionRBACAssignPostRoles) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-post" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "post:2001|roles:1,2" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if posts.lastAssignRoles.StepUpProof.BusinessAction != string(challengedomain.BusinessActionRBACAssignPostRoles) ||
		posts.lastAssignRoles.StepUpProof.OperationBinding != "post:2001|roles:1,2" ||
		posts.lastAssignRoles.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		posts.lastAssignRoles.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected post role step-up proof metadata: %#v", posts.lastAssignRoles.StepUpProof)
	}
}

func TestAdminHandlerRemovePostRolesRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	posts := &fakePostFacade{post: &userfacade.PostVO{ID: 2001, OrgID: 1, DeptID: 2}}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-post-role-clear", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(&fakeAdminUserFacade{}, &fakeUserRelationFacade{}, nil, nil, posts)
	handler.BindAuthorization(auth)
	engine.DELETE("/system/post/:postId/roles", handler.RemovePostRoles)

	resp := ut.PerformRequest(engine.Engine, "DELETE", "/system/post/2001/roles", nil)

	assertAdminBusinessCode(t, resp, 40120)
	if posts.assignRolesCalled {
		t.Fatal("RemovePostRoles called post facade without step-up proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionRBACAssignPostRoles) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "post:2001|roles:" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestAdminHandlerRemovePostRolesValidatesProofWithClearBinding(t *testing.T) {
	engine := adminTestEngine()
	posts := &fakePostFacade{post: &userfacade.PostVO{ID: 2001, OrgID: 1, DeptID: 2}}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(&fakeAdminUserFacade{}, &fakeUserRelationFacade{}, nil, nil, posts)
	handler.BindAuthorization(auth)
	engine.DELETE("/system/post/:postId/roles", handler.RemovePostRoles)

	resp := ut.PerformRequest(engine.Engine, "DELETE", "/system/post/2001/roles", nil, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-post-clear"})

	assertAdminBusinessCode(t, resp, 0)
	if !posts.assignRolesCalled {
		t.Fatal("RemovePostRoles was not called after valid proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionRBACAssignPostRoles) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-post-clear" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "post:2001|roles:" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if len(posts.lastAssignRoles.RoleIDs) != 0 {
		t.Fatalf("expected clear post roles command, got %v", posts.lastAssignRoles.RoleIDs)
	}
	if posts.lastAssignRoles.StepUpProof.BusinessAction != string(challengedomain.BusinessActionRBACAssignPostRoles) ||
		posts.lastAssignRoles.StepUpProof.OperationBinding != "post:2001|roles:" ||
		posts.lastAssignRoles.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		posts.lastAssignRoles.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected clear post role step-up proof metadata: %#v", posts.lastAssignRoles.StepUpProof)
	}
}

func TestAdminHandlerResetPasswordRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-reset-password", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/reset-password/:id", handler.ResetPassword)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/reset-password/1001", bodyOf(t, map[string]any{
		"password": "NewSecret123",
	}), ut.Header{Key: "Content-Type", Value: "application/json"})

	assertAdminBusinessCode(t, resp, 40120)
	if users.resetPasswordCalled {
		t.Fatal("ResetAdminUserPassword called before step-up proof")
	}
	if auth.lastChallenge.BusinessAction != "ADMIN_RESET_PASSWORD" {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "user:1001|reset-password" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestAdminHandlerResetPasswordValidatesProofWithCanonicalBinding(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/reset-password/:id", handler.ResetPassword)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/reset-password/1001", bodyOf(t, map[string]any{
		"password": "NewSecret123",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-reset"})

	assertAdminBusinessCode(t, resp, 0)
	if !users.resetPasswordCalled {
		t.Fatal("ResetAdminUserPassword was not called after valid proof")
	}
	if auth.lastValidate.BusinessAction != "ADMIN_RESET_PASSWORD" {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-reset" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "user:1001|reset-password" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
}

func TestAdminHandlerResetPasswordRejectsFalseStepUpValidation(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{validateResult: boolPtr(false)}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/reset-password/:id", handler.ResetPassword)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/reset-password/1001", bodyOf(t, map[string]any{
		"password": "NewSecret123",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "invalid-proof"}, ut.Header{Key: "Flow-Nonce", Value: "flow-reset"})

	assertAdminBusinessCode(t, resp, 40300)
	if users.resetPasswordCalled {
		t.Fatal("ResetAdminUserPassword called after false step-up validation")
	}
	if auth.lastValidate.BusinessAction != "ADMIN_RESET_PASSWORD" {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
}

func TestAdminHandlerDeleteUserRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-delete-user", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/delete/:id", handler.DeleteUser)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/delete/1001", nil)

	assertAdminBusinessCode(t, resp, 40120)
	if users.deleteCalled {
		t.Fatal("DeleteAdminUser called before step-up proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionAdminDeleteUser) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "user:1001|delete" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestAdminHandlerDeleteUserValidatesProofWithCanonicalBinding(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/delete/:id", handler.DeleteUser)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/delete/1001", nil, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-delete"})

	assertAdminBusinessCode(t, resp, 0)
	if !users.deleteCalled {
		t.Fatal("DeleteAdminUser was not called after valid proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionAdminDeleteUser) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-delete" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "user:1001|delete" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if users.lastDelete.StepUpProof.BusinessAction != string(challengedomain.BusinessActionAdminDeleteUser) ||
		users.lastDelete.StepUpProof.OperationBinding != "user:1001|delete" ||
		users.lastDelete.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		users.lastDelete.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected delete step-up proof metadata: %#v", users.lastDelete.StepUpProof)
	}
}

func TestAdminHandlerChangeUserStatusRequiresStepUpProof(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-status-user", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/status/:id", handler.ChangeUserStatus)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/status/1001?status=1", nil)

	assertAdminBusinessCode(t, resp, 40120)
	if users.statusCalled {
		t.Fatal("UpdateAdminUserStatus called before step-up proof")
	}
	if auth.lastChallenge.BusinessAction != string(challengedomain.BusinessActionAdminChangeUserStatus) {
		t.Fatalf("unexpected challenge action: %+v", auth.lastChallenge)
	}
	if auth.lastChallenge.OperationBinding != "user:1001|status:1" {
		t.Fatalf("unexpected operation binding: %q", auth.lastChallenge.OperationBinding)
	}
}

func TestAdminHandlerChangeUserStatusRejectsInvalidStatusBeforeStepUp(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-status-user", ChallengeState: "PENDING"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/status/:id", handler.ChangeUserStatus)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/status/1001?status=-1", nil, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-status"})

	assertAdminBusinessCode(t, resp, 40000)
	if users.statusCalled {
		t.Fatal("UpdateAdminUserStatus called for invalid status")
	}
	if auth.lastChallenge.BusinessAction != "" || auth.lastValidate.BusinessAction != "" {
		t.Fatalf("invalid status must not start or validate step-up, challenge=%+v validate=%+v", auth.lastChallenge, auth.lastValidate)
	}
}

func TestAdminHandlerChangeUserStatusValidatesProofWithCanonicalBinding(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/status/:id", handler.ChangeUserStatus)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/status/1001?status=1", nil, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-status"})

	assertAdminBusinessCode(t, resp, 0)
	if !users.statusCalled {
		t.Fatal("UpdateAdminUserStatus was not called after valid proof")
	}
	if auth.lastValidate.BusinessAction != string(challengedomain.BusinessActionAdminChangeUserStatus) {
		t.Fatalf("unexpected validate action: %+v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-status" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("expected consume-once validation with flow nonce, got %+v", auth.lastValidate)
	}
	if auth.lastValidate.OperationBinding != "user:1001|status:1" {
		t.Fatalf("unexpected operation binding: %q", auth.lastValidate.OperationBinding)
	}
	if users.lastStatus.StepUpProof.BusinessAction != string(challengedomain.BusinessActionAdminChangeUserStatus) ||
		users.lastStatus.StepUpProof.OperationBinding != "user:1001|status:1" ||
		users.lastStatus.StepUpProof.ProofIdentifier != "proof-jti-verify" ||
		users.lastStatus.StepUpProof.ChallengeIdentifier != "challenge-verify" {
		t.Fatalf("unexpected status step-up proof metadata: %#v", users.lastStatus.StepUpProof)
	}
}

func TestAdminHandlerChangeUserStatusAllowsPendingReview(t *testing.T) {
	engine := adminTestEngine()
	users := &fakeAdminUserFacade{user: &userfacade.AdminUserVO{ID: 1001, Username: "target"}}
	auth := &fakeAuthFacade{validate: &authorizationfacade.StepUpTokenVO{ProofToken: "proof-token"}}
	handler := NewAdminHandler(users, &fakeUserRelationFacade{}, nil, nil, nil)
	handler.BindAuthorization(auth)
	engine.POST("/user/status/:id", handler.ChangeUserStatus)

	resp := ut.PerformRequest(engine.Engine, "POST", "/user/status/1001?status=2", nil, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-status"})

	assertAdminBusinessCode(t, resp, 0)
	if !users.statusCalled || users.lastStatus.Status != 2 {
		t.Fatalf("expected pending review status update, got called=%v command=%#v", users.statusCalled, users.lastStatus)
	}
	if auth.lastValidate.OperationBinding != "user:1001|status:2" {
		t.Fatalf("unexpected pending review operation binding: %q", auth.lastValidate.OperationBinding)
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

func boolPtr(value bool) *bool {
	return &value
}

type fakeAdminUserFacade struct {
	user                *userfacade.AdminUserVO
	createdID           int64
	createCalled        bool
	lastCreate          userfacade.AdminUserCreateCommand
	resetPasswordCalled bool
	deleteCalled        bool
	lastDelete          userfacade.AdminUserDeleteCommand
	statusCalled        bool
	lastStatus          userfacade.AdminUserStatusCommand
}

type fakeAccessExplainFacade struct {
	effective       *authorizationfacade.EffectiveAccessVO
	effectiveUserID int64
	query           authorizationfacade.EffectiveAccessQuery
	explainCalled   bool
}

func (f *fakeAccessExplainFacade) GetEffectiveAccess(_ context.Context, userID int64, query authorizationfacade.EffectiveAccessQuery) (*authorizationfacade.EffectiveAccessVO, error) {
	f.effectiveUserID = userID
	f.query = query
	return f.effective, nil
}

func (f *fakeAccessExplainFacade) ExplainPermission(context.Context, int64, string) (*authorizationfacade.PermissionExplainVO, error) {
	f.explainCalled = true
	return &authorizationfacade.PermissionExplainVO{}, nil
}

func (f *fakeAdminUserFacade) QueryUsers(context.Context, userfacade.AdminUserQuery) (*userfacade.PageResult[userfacade.AdminUserVO], error) {
	return &userfacade.PageResult[userfacade.AdminUserVO]{}, nil
}
func (f *fakeAdminUserFacade) GetAdminUser(_ context.Context, userID int64) (*userfacade.AdminUserVO, error) {
	if f.user != nil {
		return f.user, nil
	}
	return &userfacade.AdminUserVO{ID: userID, Username: "target"}, nil
}
func (f *fakeAdminUserFacade) CreateAdminUser(_ context.Context, command userfacade.AdminUserCreateCommand) (int64, error) {
	f.createCalled = true
	f.lastCreate = command
	if f.createdID > 0 {
		return f.createdID, nil
	}
	return 10001, nil
}
func (f *fakeAdminUserFacade) UpdateAdminUser(context.Context, userfacade.AdminUserUpdateCommand) error {
	return nil
}
func (f *fakeAdminUserFacade) DeleteAdminUser(_ context.Context, command userfacade.AdminUserDeleteCommand) error {
	f.deleteCalled = true
	f.lastDelete = command
	return nil
}
func (f *fakeAdminUserFacade) UpdateAdminUserStatus(_ context.Context, command userfacade.AdminUserStatusCommand) error {
	f.statusCalled = true
	f.lastStatus = command
	return nil
}
func (f *fakeAdminUserFacade) ResetAdminUserPassword(context.Context, userfacade.AdminPasswordResetCommand) error {
	f.resetPasswordCalled = true
	return nil
}

type fakeUserRelationFacade struct {
	assignRolesCalled bool
	lastAssignRoles   userfacade.RelationAssignCommand
}

func (f *fakeUserRelationFacade) ListUserRoleIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (f *fakeUserRelationFacade) AssignUserRoles(_ context.Context, command userfacade.RelationAssignCommand) error {
	f.assignRolesCalled = true
	f.lastAssignRoles = command
	return nil
}
func (f *fakeUserRelationFacade) ListUserOrgIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (f *fakeUserRelationFacade) AssignUserOrgs(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}
func (f *fakeUserRelationFacade) ListUserDeptIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (f *fakeUserRelationFacade) AssignUserDepts(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}
func (f *fakeUserRelationFacade) ListUserPostIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (f *fakeUserRelationFacade) AssignUserPosts(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}
func (f *fakeUserRelationFacade) ListActiveUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	return nil, nil
}

type fakePostFacade struct {
	userfacade.PostFacade
	post              *userfacade.PostVO
	createCalled      bool
	lastCreate        userfacade.PostCommand
	assignRolesCalled bool
	lastAssignRoles   userfacade.PostRoleAssignCommand
}

type fakeDeptFacade struct {
	userfacade.DeptFacade
	dept *userfacade.DeptVO
}

func (f *fakeDeptFacade) GetDeptByID(context.Context, int64) (*userfacade.DeptVO, error) {
	return f.dept, nil
}

func (f *fakePostFacade) CreatePost(_ context.Context, command userfacade.PostCommand) error {
	f.createCalled = true
	f.lastCreate = command
	return nil
}

func (f *fakePostFacade) GetPostByID(_ context.Context, postID int64) (*userfacade.PostVO, error) {
	if f.post != nil {
		return f.post, nil
	}
	return &userfacade.PostVO{ID: postID, OrgID: 1, DeptID: 2}, nil
}

func (f *fakePostFacade) ListPostsByIDs(_ context.Context, postIDs []int64) ([]userfacade.PostVO, error) {
	result := make([]userfacade.PostVO, 0, len(postIDs))
	for _, postID := range postIDs {
		result = append(result, userfacade.PostVO{ID: postID, OrgID: 1, DeptID: 2})
	}
	return result, nil
}

func (f *fakePostFacade) AssignPostRoles(_ context.Context, command userfacade.PostRoleAssignCommand) error {
	f.assignRolesCalled = true
	f.lastAssignRoles = command
	return nil
}
