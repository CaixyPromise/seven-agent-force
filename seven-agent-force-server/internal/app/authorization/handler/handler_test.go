package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestAuthHandlerGetCurrentUserAndPermissions(t *testing.T) {
	handler := NewAuthHandler(&fakeAuthFacade{
		user: &authorizationfacade.UserVO{
			UserID:      1001,
			Username:    "admin",
			Nickname:    "Admin",
			Permissions: []string{"system:role:list"},
		},
		perms: []string{"system:role:list"},
	}, &fakeMenuReader{
		items: []authorizationfacade.MenuTreeNodeVO{{MenuID: 1, Name: "工作台"}},
	})
	engine := server.Default()
	engine.GET("/auth/me", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID:           1001,
			IsAdmin:          true,
			PrimaryOrgID:     1,
			AuthVersion:      42,
			DataScopeType:    securitycontext.DataScopeCustom,
			DataScopeDeptIDs: []int64{10, 11},
			DataScopeOrgIDs:  []int64{1},
		})
		handler.GetCurrentUser(ctx, reqCtx, authorizationfacade.RequestScope{UserID: 1001, Username: "admin"})
	})
	engine.GET("/auth/permissions", func(ctx context.Context, reqCtx *app.RequestContext) {
		handler.GetUserPermissionsByModule(ctx, reqCtx, authorizationfacade.RequestScope{UserID: 1001, Username: "admin"})
	})

	meResponse := ut.PerformRequest(engine.Engine, "GET", "/auth/me", nil)
	assertAuthorizationSuccess(t, meResponse)
	var me struct {
		Data struct {
			IsAdmin      bool   `json:"isAdmin"`
			PrimaryOrgID string `json:"primaryOrgId"`
			AuthVersion  string `json:"authVersion"`
			DataScope    struct {
				UserID    string   `json:"userId"`
				DeptIDs   []string `json:"deptIds"`
				OrgIDs    []string `json:"orgIds"`
				ScopeType string   `json:"scopeType"`
			} `json:"dataScope"`
		} `json:"data"`
	}
	if err := json.Unmarshal(meResponse.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode /auth/me response: %v", err)
	}
	if !me.Data.IsAdmin {
		t.Fatal("expected /auth/me to expose authoritative isAdmin")
	}
	if me.Data.PrimaryOrgID != "1" || me.Data.AuthVersion != "42" {
		t.Fatalf("unexpected cache identity metadata: %#v", me.Data)
	}
	if me.Data.DataScope.UserID != "1001" || me.Data.DataScope.ScopeType != "CUSTOM" {
		t.Fatalf("unexpected data scope: %#v", me.Data.DataScope)
	}
	if len(me.Data.DataScope.DeptIDs) != 2 || len(me.Data.DataScope.OrgIDs) != 1 {
		t.Fatalf("unexpected data scope identifiers: %#v", me.Data.DataScope)
	}
	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "GET", "/auth/permissions?module=system", nil))
}

func TestAuthHandlerValidateStepUpPostBindsProofBody(t *testing.T) {
	auth := &fakeAuthFacade{}
	handler := NewAuthHandler(auth, &fakeMenuReader{})
	engine := server.Default()
	engine.POST("/auth/step-up/validate", func(ctx context.Context, reqCtx *app.RequestContext) {
		handler.ValidateStepUp(ctx, reqCtx, authorizationfacade.RequestScope{UserID: 1001, Username: "operator", SessionID: "sid-1"})
	})

	resp := ut.PerformRequest(engine.Engine, "POST", "/auth/step-up/validate", jsonBody(t, map[string]any{
		"proofToken":       "proof-token-live",
		"businessAction":   "RBAC_ASSIGN_USER_ROLES",
		"flowNonce":        "flow-live",
		"operationBinding": "user:1001|roles:1,2",
		"consumeOnce":      true,
	}), ut.Header{Key: "Content-Type", Value: "application/json"})

	assertAuthorizationSuccess(t, resp)
	if auth.lastValidate.ProofToken != "proof-token-live" {
		t.Fatalf("proof token must come from request body: %#v", auth.lastValidate)
	}
	if auth.lastValidate.BusinessAction != "RBAC_ASSIGN_USER_ROLES" {
		t.Fatalf("business action must come from request body: %#v", auth.lastValidate)
	}
	if auth.lastValidate.FlowNonce != "flow-live" || auth.lastValidate.OperationBinding != "user:1001|roles:1,2" || !auth.lastValidate.ConsumeOnce {
		t.Fatalf("step-up validation body was not preserved: %#v", auth.lastValidate)
	}
}

func TestAuthHandlerValidateStepUpGetRejectsProofTokenQuery(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{
			name: "non-empty token",
			path: "/auth/step-up/validate?token=proof-token-live&businessAction=RBAC_ASSIGN_USER_ROLES&flowNonce=flow-live&operationBinding=user%3A1001%7Croles%3A1%2C2&consumeOnce=true",
		},
		{
			name: "empty token",
			path: "/auth/step-up/validate?token=&businessAction=RBAC_ASSIGN_USER_ROLES",
		},
		{
			name: "blank token",
			path: "/auth/step-up/validate?token=%20%20&businessAction=RBAC_ASSIGN_USER_ROLES",
		},
		{
			name: "proofToken alias",
			path: "/auth/step-up/validate?proofToken=proof-token-live&businessAction=RBAC_ASSIGN_USER_ROLES",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			auth := &fakeAuthFacade{}
			handler := NewAuthHandler(auth, &fakeMenuReader{})
			engine := server.Default()
			engine.GET("/auth/step-up/validate", func(ctx context.Context, reqCtx *app.RequestContext) {
				handler.ValidateStepUp(ctx, reqCtx, authorizationfacade.RequestScope{UserID: 1001, Username: "operator", SessionID: "sid-1"})
			})

			resp := ut.PerformRequest(engine.Engine, "GET", tt.path, nil)

			if resp.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			var result response.Result
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode response: %v body=%s", err, resp.Body.String())
			}
			if result.Code == 0 {
				t.Fatalf("GET query proof token must be rejected, got success body=%s", resp.Body.String())
			}
			if auth.lastValidate.ProofToken != "" {
				t.Fatalf("GET query proof token reached facade: %#v", auth.lastValidate)
			}
		})
	}
}

func TestRoleAndInternalHandlers(t *testing.T) {
	roleCtrl := NewRoleHandler(&fakeRoleFacade{
		roles: []authorizationfacade.RoleVO{{RoleID: 1, Name: "管理员", Code: "ROLE_ADMIN"}},
	})
	internalCtrl := NewInternalHandler(&fakeAuthFacade{user: &authorizationfacade.UserVO{UserID: 1001, Username: "admin"}})

	engine := server.Default()
	engine.GET("/system/role/list", roleCtrl.ListRoles)
	engine.GET("/internal/auth/user/:userId", internalCtrl.GetUser)
	engine.POST("/internal/auth/user/:userId/permission-cache/refresh", internalCtrl.RefreshPermissionCache)

	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "GET", "/system/role/list", nil))
	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "GET", "/internal/auth/user/1001", nil))
	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "POST", "/internal/auth/user/1001/permission-cache/refresh", nil))
}

func TestRoleMutationHandlersPropagateOperatorID(t *testing.T) {
	facade := &fakeRoleFacade{}
	roleCtrl := NewRoleHandler(facade)
	roleCtrl.BindAuthorization(&fakeAuthFacade{})
	engine := server.Default()
	engine.GET("/system/role/:roleId/depts", roleCtrl.GetRoleDeptIDs)
	engine.POST("/system/role/depts/assign", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator"})
		roleCtrl.AssignRoleDepts(ctx, reqCtx)
	})
	engine.POST("/system/role/permissions/assign", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator"})
		roleCtrl.AssignRolePermissions(ctx, reqCtx)
	})
	engine.POST("/system/role/user-roles/assign", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator"})
		roleCtrl.AssignUserRoles(ctx, reqCtx)
	})

	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "POST", "/system/role/permissions/assign", jsonBody(t, map[string]any{
		"roleId":        1,
		"permissionIds": []int64{10},
	}), privilegedMutationHeaders()...))
	if facade.lastAssignRolePermissions.OperatorID != 9001 {
		t.Fatalf("expected operatorId to be propagated, got %d", facade.lastAssignRolePermissions.OperatorID)
	}

	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "GET", "/system/role/1/depts", nil))
	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "POST", "/system/role/depts/assign", jsonBody(t, map[string]any{
		"roleId":  1,
		"deptIds": []int64{20, 10},
	}), privilegedMutationHeaders()...))
	if facade.lastAssignRoleDepts.OperatorID != 9001 {
		t.Fatalf("expected operatorId to be propagated, got %d", facade.lastAssignRoleDepts.OperatorID)
	}
	if facade.lastAssignRoleDepts.RoleID != 1 {
		t.Fatalf("expected roleId to be bound, got %d", facade.lastAssignRoleDepts.RoleID)
	}

	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "POST", "/system/role/user-roles/assign", jsonBody(t, map[string]any{
		"userId":  1001,
		"roleIds": []int64{1},
	}), privilegedMutationHeaders()...))
	if facade.lastAssignUserRoles.OperatorID != 9001 {
		t.Fatalf("expected operatorId to be propagated, got %d", facade.lastAssignUserRoles.OperatorID)
	}
}

func TestRoleMutationHandlersRequireStepUpProof(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     map[string]any
		register func(*RoleHandler, *server.Hertz)
	}{
		{
			name:   "assign_user_roles",
			method: "POST",
			path:   "/system/role/user-roles/assign",
			body:   map[string]any{"userId": 1001, "roleIds": []int64{2, 1}},
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/user-roles/assign", roleCtrl.AssignUserRoles)
			},
		},
		{
			name:   "assign_role_permissions",
			method: "POST",
			path:   "/system/role/permissions/assign",
			body:   map[string]any{"roleId": 7, "permissionIds": []int64{12, 10}, "menuIds": []int64{5}},
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/permissions/assign", roleCtrl.AssignRolePermissions)
			},
		},
		{
			name:   "assign_role_depts",
			method: "POST",
			path:   "/system/role/depts/assign",
			body:   map[string]any{"roleId": 7, "deptIds": []int64{20, 10}},
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/depts/assign", roleCtrl.AssignRoleDepts)
			},
		},
		{
			name:   "assign_role_menus",
			method: "POST",
			path:   "/system/role/7/menus",
			body:   map[string]any{"menuIds": []int64{5, 2}},
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/:roleId/menus", roleCtrl.AssignRoleMenusCompat)
			},
		},
		{
			name:   "bind_menu_permissions",
			method: "POST",
			path:   "/system/menu/9/permissions",
			body:   map[string]any{"permissionIds": []int64{12, 10}},
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/menu/:menuId/permissions", roleCtrl.BindMenuPermissions)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facade := &fakeRoleFacade{}
			auth := &fakeAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-" + tt.name, ChallengeState: "PENDING"}}
			roleCtrl := NewRoleHandler(facade)
			roleCtrl.BindAuthorization(auth)
			engine := server.Default()
			engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
				securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator", SessionID: "sid-1"})
				reqCtx.Next(ctx)
			})
			tt.register(roleCtrl, engine)

			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, jsonBody(t, tt.body), ut.Header{Key: "Content-Type", Value: "application/json"})
			if resp.Code != 200 {
				t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
			}
			if !bytes.Contains(resp.Body.Bytes(), []byte(`"code":40120`)) {
				t.Fatalf("expected challenge_required response: %s", resp.Body.String())
			}
			if !bytes.Contains(resp.Body.Bytes(), []byte(`"challengeIdentifier":"challenge-`+tt.name+`"`)) {
				t.Fatalf("expected challenge payload: %s", resp.Body.String())
			}
			if facade.assignCalls != 0 {
				t.Fatalf("mutation facade should not be called before proof, got %d calls", facade.assignCalls)
			}
		})
	}
}

func TestRoleMutationHandlersValidateProofWithCanonicalBinding(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		body        map[string]any
		register    func(*RoleHandler, *server.Hertz)
		wantAction  string
		wantBinding string
		assertCall  func(*testing.T, *fakeRoleFacade)
	}{
		{
			name:        "assign_user_roles",
			path:        "/system/role/user-roles/assign",
			body:        map[string]any{"userId": 1001, "roleIds": []int64{2, 1, 2}},
			wantAction:  "RBAC_ASSIGN_USER_ROLES",
			wantBinding: "user:1001|roles:1,2",
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/user-roles/assign", roleCtrl.AssignUserRoles)
			},
			assertCall: func(t *testing.T, facade *fakeRoleFacade) {
				t.Helper()
				if facade.lastAssignUserRoles.OperatorID != 9001 {
					t.Fatalf("expected operatorId to be propagated, got %d", facade.lastAssignUserRoles.OperatorID)
				}
				assertStepUpProof(t, facade.lastAssignUserRoles.StepUpProof, "RBAC_ASSIGN_USER_ROLES", "user:1001|roles:1,2")
			},
		},
		{
			name:        "assign_role_permissions",
			path:        "/system/role/permissions/assign",
			body:        map[string]any{"roleId": 7, "permissionIds": []int64{12, 10, 10}, "menuIds": []int64{5, 2}},
			wantAction:  "RBAC_ASSIGN_ROLE_PERMISSIONS",
			wantBinding: "role:7|permissions:10,12|menus:2,5",
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/permissions/assign", roleCtrl.AssignRolePermissions)
			},
			assertCall: func(t *testing.T, facade *fakeRoleFacade) {
				t.Helper()
				if facade.lastAssignRolePermissions.OperatorID != 9001 {
					t.Fatalf("expected operatorId to be propagated, got %d", facade.lastAssignRolePermissions.OperatorID)
				}
				assertStepUpProof(t, facade.lastAssignRolePermissions.StepUpProof, "RBAC_ASSIGN_ROLE_PERMISSIONS", "role:7|permissions:10,12|menus:2,5")
			},
		},
		{
			name:        "assign_role_depts",
			path:        "/system/role/depts/assign",
			body:        map[string]any{"roleId": 7, "deptIds": []int64{20, 10, 20}},
			wantAction:  "RBAC_ASSIGN_ROLE_DEPTS",
			wantBinding: "role:7|depts:10,20",
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/depts/assign", roleCtrl.AssignRoleDepts)
			},
			assertCall: func(t *testing.T, facade *fakeRoleFacade) {
				t.Helper()
				if facade.lastAssignRoleDepts.OperatorID != 9001 {
					t.Fatalf("expected operatorId to be propagated, got %d", facade.lastAssignRoleDepts.OperatorID)
				}
				assertStepUpProof(t, facade.lastAssignRoleDepts.StepUpProof, "RBAC_ASSIGN_ROLE_DEPTS", "role:7|depts:10,20")
			},
		},
		{
			name:        "assign_role_menus",
			path:        "/system/role/7/menus",
			body:        map[string]any{"menuIds": []int64{5, 2, 5}},
			wantAction:  "RBAC_ASSIGN_ROLE_MENUS",
			wantBinding: "role:7|menus:2,5",
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/role/:roleId/menus", roleCtrl.AssignRoleMenusCompat)
			},
			assertCall: func(t *testing.T, facade *fakeRoleFacade) {
				t.Helper()
				if facade.assignCalls != 1 {
					t.Fatalf("expected one mutation call, got %d", facade.assignCalls)
				}
				assertStepUpProof(t, facade.lastAssignRoleMenus.StepUpProof, "RBAC_ASSIGN_ROLE_MENUS", "role:7|menus:2,5")
			},
		},
		{
			name:        "bind_menu_permissions",
			path:        "/system/menu/9/permissions",
			body:        map[string]any{"permissionIds": []int64{12, 10, 12}},
			wantAction:  "RBAC_ASSIGN_MENU_PERMISSIONS",
			wantBinding: "menu:9|permissions:10,12",
			register: func(roleCtrl *RoleHandler, engine *server.Hertz) {
				engine.POST("/system/menu/:menuId/permissions", roleCtrl.BindMenuPermissions)
			},
			assertCall: func(t *testing.T, facade *fakeRoleFacade) {
				t.Helper()
				if facade.lastBindMenuPermissions.OperatorID != 9001 {
					t.Fatalf("expected operatorId to be propagated, got %d", facade.lastBindMenuPermissions.OperatorID)
				}
				assertStepUpProof(t, facade.lastBindMenuPermissions.StepUpProof, "RBAC_ASSIGN_MENU_PERMISSIONS", "menu:9|permissions:10,12")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facade := &fakeRoleFacade{}
			auth := &fakeAuthFacade{}
			roleCtrl := NewRoleHandler(facade)
			roleCtrl.BindAuthorization(auth)
			engine := server.Default()
			engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
				securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator", SessionID: "sid-1"})
				reqCtx.Next(ctx)
			})
			tt.register(roleCtrl, engine)

			resp := ut.PerformRequest(engine.Engine, "POST", tt.path, jsonBody(t, tt.body), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-1"})
			assertAuthorizationSuccess(t, resp)
			if auth.lastValidate.BusinessAction != tt.wantAction {
				t.Fatalf("unexpected business action: %#v", auth.lastValidate)
			}
			if auth.lastValidate.OperationBinding != tt.wantBinding {
				t.Fatalf("unexpected operation binding: %#v", auth.lastValidate)
			}
			if auth.lastValidate.FlowNonce != "flow-1" || !auth.lastValidate.ConsumeOnce {
				t.Fatalf("proof validation must preserve flow nonce and consume once: %#v", auth.lastValidate)
			}
			tt.assertCall(t, facade)
		})
	}
}

func TestTemporaryPermissionGrantPropagatesGrantedBy(t *testing.T) {
	service := &fakeTempPermissionService{}
	ctrl := NewTemporaryPermissionHandler(service)
	ctrl.BindAuthorization(&fakeAuthFacade{user: &authorizationfacade.UserVO{UserID: 1001}})
	engine := server.Default()
	engine.POST("/admin/temp-permission/grant", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator", IsAdmin: true})
		ctrl.Grant(ctx, reqCtx)
	})

	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "POST", "/admin/temp-permission/grant", jsonBody(t, map[string]any{
		"userId":         "2078559343971979264",
		"permissionCode": "system:user:list",
		"reason":         "incident response",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-1"}))
	if service.lastGrant.GrantedBy != 9001 {
		t.Fatalf("expected grantedBy to be propagated, got %d", service.lastGrant.GrantedBy)
	}
	if service.lastGrant.UserID != 2078559343971979264 {
		t.Fatalf("expected decimal-string userId to retain int64 precision, got %d", service.lastGrant.UserID)
	}
}

func TestTemporaryPermissionExtendPropagatesOperatorID(t *testing.T) {
	service := &fakeTempPermissionService{}
	ctrl := NewTemporaryPermissionHandler(service)
	ctrl.BindAuthorization(&fakeAuthFacade{user: &authorizationfacade.UserVO{UserID: 1001}})
	engine := server.Default()
	engine.POST("/admin/temp-permission/extend", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator", IsAdmin: true})
		ctrl.Extend(ctx, reqCtx)
	})

	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "POST", "/admin/temp-permission/extend", jsonBody(t, map[string]any{
		"userId":         1001,
		"permissionCode": "system:user:list",
		"reason":         "extend incident window",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-1"}))
	if service.lastExtend.OperatorID != 9001 {
		t.Fatalf("expected operatorId to be propagated, got %d", service.lastExtend.OperatorID)
	}
}

func TestTemporaryPermissionRevokePropagatesOperatorID(t *testing.T) {
	service := &fakeTempPermissionService{}
	ctrl := NewTemporaryPermissionHandler(service)
	ctrl.BindAuthorization(&fakeAuthFacade{user: &authorizationfacade.UserVO{UserID: 1001}})
	engine := server.Default()
	engine.POST("/admin/temp-permission/revoke", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{UserID: 9001, Username: "operator", IsAdmin: true})
		ctrl.Revoke(ctx, reqCtx)
	})

	assertAuthorizationSuccess(t, ut.PerformRequest(engine.Engine, "POST", "/admin/temp-permission/revoke", jsonBody(t, map[string]any{
		"userId":         1001,
		"permissionCode": "system:user:list",
		"reason":         "incident resolved",
	}), ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Proof-Token", Value: "proof-token"}, ut.Header{Key: "Flow-Nonce", Value: "flow-1"}))
	if service.lastRevoke.OperatorID != 9001 {
		t.Fatalf("expected operatorId to be propagated, got %d", service.lastRevoke.OperatorID)
	}
}

func TestTemporaryPermissionRejectsTargetOutsideDataScope(t *testing.T) {
	service := &fakeTempPermissionService{}
	ctrl := NewTemporaryPermissionHandler(service)
	ctrl.BindAuthorization(&fakeAuthFacade{user: &authorizationfacade.UserVO{UserID: 1001, DeptIDs: []int64{20}}})
	engine := server.Default()
	engine.GET("/admin/temp-permission/user/:userId", func(ctx context.Context, reqCtx *app.RequestContext) {
		securitycontext.Set(reqCtx, &securitycontext.UserContext{
			UserID: 9001, DataScopeType: securitycontext.DataScopeCustom, DataScopeDeptIDs: []int64{10},
		})
		ctrl.ListByUser(ctx, reqCtx)
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/admin/temp-permission/user/1001", nil)
	if resp.Code != 200 {
		t.Fatalf("unexpected HTTP status: %d body=%s", resp.Code, resp.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != 40310 {
		t.Fatalf("expected stable data scope denial, got %#v", result)
	}
}

type fakeAuthFacade struct {
	user          *authorizationfacade.UserVO
	perms         []string
	challenge     *authorizationfacade.StepUpChallengeVO
	lastChallenge authorizationfacade.StepUpChallengeRequest
	lastValidate  authorizationfacade.StepUpValidateRequest
}

func (f *fakeAuthFacade) GetLoginUser(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return f.user, nil
}
func (f *fakeAuthFacade) GetLoginUserPermitNull(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return f.user, nil
}
func (f *fakeAuthFacade) GetLoginUserID(context.Context, authorizationfacade.RequestScope) (int64, error) {
	if f.user == nil {
		return 0, nil
	}
	return f.user.UserID, nil
}
func (f *fakeAuthFacade) GetLoginUsername(context.Context, authorizationfacade.RequestScope) (string, error) {
	if f.user == nil {
		return "", nil
	}
	return f.user.Username, nil
}
func (f *fakeAuthFacade) IsLogin(context.Context, authorizationfacade.RequestScope) bool { return true }
func (f *fakeAuthFacade) IsAdmin(context.Context, authorizationfacade.RequestScope) bool {
	return false
}
func (f *fakeAuthFacade) IsCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return true
}
func (f *fakeAuthFacade) IsAdminOrCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return true
}
func (f *fakeAuthFacade) GetUserVO(context.Context, int64) (*authorizationfacade.UserVO, error) {
	return f.user, nil
}
func (f *fakeAuthFacade) RefreshUserPermissionCache(context.Context, int64) error { return nil }
func (f *fakeAuthFacade) GetUserPermissionsByModule(context.Context, authorizationfacade.RequestScope, string) ([]string, error) {
	return f.perms, nil
}
func (f *fakeAuthFacade) CreateStepUpChallenge(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	f.lastChallenge = request
	if f.challenge != nil {
		return f.challenge, nil
	}
	return &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-1"}, nil
}
func (f *fakeAuthFacade) VerifyStepUp(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
	f.lastValidate = authorizationfacade.StepUpValidateRequest(request)
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
	return true, nil
}

type fakeMenuReader struct {
	items []authorizationfacade.MenuTreeNodeVO
}

func (f *fakeMenuReader) GetCurrentUserMenus(context.Context, int64) ([]authorizationfacade.MenuTreeNodeVO, error) {
	return f.items, nil
}

type fakeRoleFacade struct {
	authorizationfacade.UserRoleAssignmentFacade
	roles                     []authorizationfacade.RoleVO
	assignCalls               int
	lastAssignRolePermissions authorizationfacade.AssignRolePermissionsCommand
	lastAssignRoleDepts       authorizationfacade.AssignRoleDeptsCommand
	lastAssignRoleMenus       authorizationfacade.AssignRoleMenusCommand
	lastAssignUserRoles       authorizationfacade.AssignUserRolesCommand
	lastBindMenuPermissions   authorizationfacade.MenuPermissionAssignCommand
}

func (f *fakeRoleFacade) GetRoleList(context.Context) ([]authorizationfacade.RoleVO, error) {
	return f.roles, nil
}
func (f *fakeRoleFacade) PageRoles(context.Context, authorizationfacade.RolePageQuery) (*authorizationfacade.RolePageVO, error) {
	return &authorizationfacade.RolePageVO{Records: f.roles, Total: int64(len(f.roles)), Current: 1, Size: 10}, nil
}
func (f *fakeRoleFacade) GetRole(context.Context, int64) (*authorizationfacade.RoleVO, error) {
	if len(f.roles) == 0 {
		return nil, nil
	}
	return &f.roles[0], nil
}
func (f *fakeRoleFacade) GetRootSecurityStatus(context.Context) (*authorizationfacade.RoleSecurityStatusVO, error) {
	return &authorizationfacade.RoleSecurityStatusVO{Health: "HEALTHY"}, nil
}
func (f *fakeRoleFacade) BootstrapAuthorizationRoot(context.Context, authorizationfacade.BootstrapAuthorizationRootCommand) (*authorizationfacade.BootstrapAuthorizationRootResult, error) {
	return &authorizationfacade.BootstrapAuthorizationRootResult{}, nil
}
func (f *fakeRoleFacade) CreateRole(context.Context, authorizationfacade.RoleCommand) (*authorizationfacade.RoleVO, error) {
	return &authorizationfacade.RoleVO{RoleID: 1, ID: 1}, nil
}
func (f *fakeRoleFacade) UpdateRole(context.Context, authorizationfacade.RoleCommand) (*authorizationfacade.RoleVO, error) {
	return &authorizationfacade.RoleVO{RoleID: 1, ID: 1}, nil
}
func (f *fakeRoleFacade) DeleteRole(context.Context, int64, int64) error { return nil }
func (f *fakeRoleFacade) GetRoleDeptIDs(context.Context, int64) (*authorizationfacade.RoleDeptIDsVO, error) {
	return &authorizationfacade.RoleDeptIDsVO{RoleID: 1, DeptIDs: []int64{10, 20}}, nil
}
func (f *fakeRoleFacade) AssignRoleDepts(_ context.Context, command authorizationfacade.AssignRoleDeptsCommand) error {
	f.assignCalls++
	f.lastAssignRoleDepts = command
	return nil
}
func (f *fakeRoleFacade) GetRoleMenuTree(context.Context, int64) ([]authorizationfacade.MenuTreeNodeVO, error) {
	return []authorizationfacade.MenuTreeNodeVO{{MenuID: 1, Name: "角色菜单"}}, nil
}
func (f *fakeRoleFacade) GetRoleMenuIDs(context.Context, int64) ([]int64, error) {
	return []int64{1}, nil
}
func (f *fakeRoleFacade) AssignRoleMenus(_ context.Context, command authorizationfacade.AssignRoleMenusCommand) error {
	f.assignCalls++
	f.lastAssignRoleMenus = command
	return nil
}
func (f *fakeRoleFacade) AssignRolePermissions(_ context.Context, command authorizationfacade.AssignRolePermissionsCommand) error {
	f.assignCalls++
	f.lastAssignRolePermissions = command
	return nil
}
func (f *fakeRoleFacade) GetRoleGrantSnapshot(context.Context, int64) (*authorizationfacade.RoleGrantSnapshotVO, error) {
	return &authorizationfacade.RoleGrantSnapshotVO{}, nil
}
func (f *fakeRoleFacade) PreviewRoleGrantBundle(context.Context, authorizationfacade.PreviewRoleGrantBundleCommand) (*authorizationfacade.RoleGrantPreviewVO, error) {
	return &authorizationfacade.RoleGrantPreviewVO{}, nil
}
func (f *fakeRoleFacade) CommitRoleGrantBundle(context.Context, authorizationfacade.CommitRoleGrantBundleCommand) (*authorizationfacade.RoleGrantCommitVO, error) {
	return &authorizationfacade.RoleGrantCommitVO{}, nil
}
func (f *fakeRoleFacade) AdvanceRoleGrantRevision(context.Context, int64, int64) error { return nil }
func (f *fakeRoleFacade) AssignUserRoles(_ context.Context, command authorizationfacade.AssignUserRolesCommand) error {
	f.assignCalls++
	f.lastAssignUserRoles = command
	return nil
}
func (f *fakeRoleFacade) BootstrapOwnerRoles(context.Context, authorizationfacade.BootstrapOwnerRolesCommand) error {
	return nil
}
func (f *fakeRoleFacade) GetMenuTree(context.Context, bool) ([]authorizationfacade.MenuTreeNodeVO, error) {
	return []authorizationfacade.MenuTreeNodeVO{{MenuID: 1, Name: "角色菜单"}}, nil
}
func (f *fakeRoleFacade) GetMenu(context.Context, int64) (*authorizationfacade.MenuTreeNodeVO, error) {
	return &authorizationfacade.MenuTreeNodeVO{MenuID: 1, ID: 1, Name: "角色菜单"}, nil
}
func (f *fakeRoleFacade) CreateMenu(context.Context, authorizationfacade.MenuCommand) (*authorizationfacade.MenuTreeNodeVO, error) {
	return &authorizationfacade.MenuTreeNodeVO{MenuID: 1, ID: 1}, nil
}
func (f *fakeRoleFacade) UpdateMenu(context.Context, authorizationfacade.MenuCommand) (*authorizationfacade.MenuTreeNodeVO, error) {
	return &authorizationfacade.MenuTreeNodeVO{MenuID: 1, ID: 1}, nil
}
func (f *fakeRoleFacade) DeleteMenu(context.Context, int64, int64) error { return nil }
func (f *fakeRoleFacade) ListPermissions(context.Context, authorizationfacade.PermissionQuery) ([]authorizationfacade.PermissionVO, error) {
	return nil, nil
}
func (f *fakeRoleFacade) PagePermissions(context.Context, authorizationfacade.PermissionPageQuery) (*authorizationfacade.PermissionPageVO, error) {
	return &authorizationfacade.PermissionPageVO{Records: nil, Total: 0, Current: 1, Size: 10}, nil
}
func (f *fakeRoleFacade) GetPermission(context.Context, int64) (*authorizationfacade.PermissionVO, error) {
	return &authorizationfacade.PermissionVO{ID: 1, Code: "system:test"}, nil
}
func (f *fakeRoleFacade) CreatePermission(context.Context, authorizationfacade.PermissionCommand) (*authorizationfacade.PermissionVO, error) {
	return &authorizationfacade.PermissionVO{ID: 1}, nil
}
func (f *fakeRoleFacade) UpdatePermission(context.Context, int64, authorizationfacade.PermissionCommand) (*authorizationfacade.PermissionVO, error) {
	return &authorizationfacade.PermissionVO{ID: 1}, nil
}
func (f *fakeRoleFacade) DeletePermission(context.Context, int64, int64) error { return nil }
func (f *fakeRoleFacade) GetMenuPermissionIDs(context.Context, int64) ([]int64, error) {
	return []int64{1}, nil
}
func (f *fakeRoleFacade) BindMenuPermissions(_ context.Context, command authorizationfacade.MenuPermissionAssignCommand) error {
	f.assignCalls++
	f.lastBindMenuPermissions = command
	return nil
}

type fakeTempPermissionService struct {
	lastGrant  authorizationfacade.TemporaryPermissionGrantCommand
	lastRevoke authorizationfacade.TemporaryPermissionUpdateCommand
	lastExtend authorizationfacade.TemporaryPermissionUpdateCommand
}

func (f *fakeTempPermissionService) GrantTemporaryPermission(_ context.Context, command authorizationfacade.TemporaryPermissionGrantCommand) error {
	f.lastGrant = command
	return nil
}
func (f *fakeTempPermissionService) RevokeTemporaryPermission(_ context.Context, command authorizationfacade.TemporaryPermissionUpdateCommand) error {
	f.lastRevoke = command
	return nil
}
func (f *fakeTempPermissionService) ExtendTemporaryPermission(_ context.Context, command authorizationfacade.TemporaryPermissionUpdateCommand) error {
	f.lastExtend = command
	return nil
}
func (f *fakeTempPermissionService) CleanupExpiredTemporaryPermissions(context.Context) error {
	return nil
}
func (f *fakeTempPermissionService) ListUserTemporaryPermissions(context.Context, int64) ([]authorizationfacade.TemporaryPermissionVO, error) {
	return nil, nil
}
func (f *fakeTempPermissionService) TemporaryPermissionStats(context.Context) (*authorizationfacade.TemporaryPermissionStatsVO, error) {
	return &authorizationfacade.TemporaryPermissionStatsVO{}, nil
}
func (f *fakeTempPermissionService) ResolvePermissionCode(context.Context, int64) (string, error) {
	return "system:user:list", nil
}

func jsonBody(t *testing.T, payload any) *ut.Body {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return &ut.Body{Body: bytes.NewReader(body), Len: len(body)}
}

func privilegedMutationHeaders() []ut.Header {
	return []ut.Header{
		{Key: "Content-Type", Value: "application/json"},
		{Key: "Proof-Token", Value: "proof-token"},
		{Key: "Flow-Nonce", Value: "flow-1"},
	}
}

func assertAuthorizationSuccess(t *testing.T, recorder *ut.ResponseRecorder) {
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

func assertStepUpProof(t *testing.T, proof stepup.ProofMetadata, action, binding string) {
	t.Helper()
	if proof.BusinessAction != action || proof.OperationBinding != binding {
		t.Fatalf("unexpected step-up proof metadata: %#v", proof)
	}
	if proof.ProofIdentifier == "" || proof.ChallengeIdentifier == "" {
		t.Fatalf("expected proof identifiers to be propagated: %#v", proof)
	}
}
