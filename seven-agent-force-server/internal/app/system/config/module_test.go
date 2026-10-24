package config

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	authorizationruntime "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/runtime"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	configapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/application"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	confighandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/handler"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	sharedconfig "github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestModulePermissionGuardsAndClientRoutes(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "true" {
			securitycontext.Set(reqCtx, &securitycontext.UserContext{
				UserID:      1001,
				Username:    "admin",
				RoleIDs:     []int64{501},
				Permissions: splitPermissions(string(reqCtx.Request.Header.Peek("X-Test-Permissions"))),
			})
		}
		reqCtx.Next(ctx)
	})
	module := &Module{
		handler: confighandler.NewHandler(&fakeManagementService{}, &fakeClientService{}),
	}
	module.Mount(engine.Engine)

	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/config-groups/page", nil), apperrors.CodeNotLogin)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/config-groups/page", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	), apperrors.CodeForbidden)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/config-groups/page", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "system:config:group:query"},
	), apperrors.CodeSuccess)

	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/config-client/public.banner", nil), apperrors.CodeSuccess)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/config-client?groupCode=public", nil), apperrors.CodeSuccess)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/config-client/secure.banner", nil), apperrors.CodeNotLogin)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/config-client/secure.banner", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	), apperrors.CodeSuccess)
}

func TestConfigAssetRouteStreamsOnlyAProtectedStableRepresentation(t *testing.T) {
	tests := []struct {
		name        string
		assetType   filefacade.ConfigAssetType
		contentType string
		fileName    string
		wantPrefix  string
	}{
		{name: "image-inline", assetType: filefacade.ConfigAssetImage, contentType: "image/png", fileName: "brand.png", wantPrefix: "inline"},
		{name: "file-attachment", assetType: filefacade.ConfigAssetFile, contentType: "application/octet-stream", fileName: "notice.pdf", wantPrefix: "attachment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			management := &fakeManagementService{assetResult: &filefacade.ConfigAssetOpenResult{
				Reader: io.NopCloser(bytes.NewReader([]byte("asset-bytes"))), Size: int64(len("asset-bytes")), ContentType: tt.contentType,
				FileName: tt.fileName, AssetType: tt.assetType, AccessScope: filefacade.ConfigAssetPublic,
			}}
			module := &Module{handler: confighandler.NewHandler(management, &fakeClientService{})}
			engine := configTestEngine(module)
			resp := ut.PerformRequest(engine.Engine, "GET", "/config-assets/81", nil)
			if resp.Code != 200 || resp.Body.String() != "asset-bytes" {
				t.Fatalf("stable asset route response=%d body=%q", resp.Code, resp.Body.String())
			}
			assertHeader := func(key, want string) {
				t.Helper()
				if got := string(resp.Header().Peek(key)); got != want {
					t.Fatalf("%s=%q, want %q", key, got, want)
				}
			}
			assertHeader("Content-Type", tt.contentType)
			assertHeader("Cache-Control", "no-store, max-age=0")
			assertHeader("Pragma", "no-cache")
			assertHeader("X-Content-Type-Options", "nosniff")
			assertHeader("Cross-Origin-Resource-Policy", "same-origin")
			assertHeader("Referrer-Policy", "no-referrer")
			if got := string(resp.Header().Peek("Content-Security-Policy")); got == "" || !bytes.Contains([]byte(got), []byte("default-src 'none'")) {
				t.Fatalf("missing restrictive asset CSP: %q", got)
			}
			if got := string(resp.Header().Peek("Content-Disposition")); len(got) < len(tt.wantPrefix) || got[:len(tt.wantPrefix)] != tt.wantPrefix {
				t.Fatalf("Content-Disposition=%q, want %s prefix", got, tt.wantPrefix)
			}
			if management.assetCalls != 1 || management.assetID != 81 || management.assetActor.UserID != 0 || management.assetActor.Authenticated {
				t.Fatalf("stable route exposed unexpected authority or actor shape: calls=%d id=%d actor=%+v", management.assetCalls, management.assetID, management.assetActor)
			}
		})
	}
}

// TestProductionAuthorizationMiddlewareDefersOnlyTheStableConfigAssetRoute
// composes the real config module route with the same authorization runtime
// and /api group ordering used by bootstrap.App.registerModules.  The
// management stub represents the already-tested application exposure decision:
// PUBLIC streams, while AUTHENTICATED and INTERNAL return their policy error.
// This protects the boundary where a global middleware could otherwise reject
// a public login logo before the configuration application can decide it.
func TestProductionAuthorizationMiddlewareDefersOnlyTheStableConfigAssetRoute(t *testing.T) {
	t.Setenv("SEVEN_PROFILE", "dev")
	configDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "configs"))
	if err != nil {
		t.Fatalf("resolve production config directory: %v", err)
	}
	example, err := os.ReadFile(filepath.Join(configDir, "application.example.yaml"))
	if err != nil {
		t.Fatalf("read public configuration example: %v", err)
	}
	runtimeConfigDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeConfigDir, "application.yaml"), example, 0o600); err != nil {
		t.Fatalf("prepare runtime configuration: %v", err)
	}
	cfg, err := sharedconfig.Load(runtimeConfigDir)
	if err != nil {
		t.Fatalf("load production authorization configuration: %v", err)
	}
	if cfg.ContextPath() != "/api" {
		t.Fatalf("expected dev production router context path /api, got %q", cfg.ContextPath())
	}

	tests := []struct {
		name       string
		path       string
		result     *filefacade.ConfigAssetOpenResult
		resultErr  error
		wantStatus int
		wantCode   int
		wantCalls  int
	}{
		{
			name: "public reaches config handler and streams",
			path: "/api/config-assets/81",
			result: &filefacade.ConfigAssetOpenResult{
				Reader: io.NopCloser(bytes.NewReader([]byte("public-logo"))), Size: int64(len("public-logo")), ContentType: "image/png",
				FileName: "logo.png", AssetType: filefacade.ConfigAssetImage, AccessScope: filefacade.ConfigAssetPublic,
			},
			wantStatus: 200,
			wantCalls:  1,
		},
		{
			name:       "authenticated exposure remains application denied to anonymous",
			path:       "/api/config-assets/82",
			resultErr:  apperrors.Unauthorized("该配置需要登录后访问"),
			wantStatus: 200,
			wantCode:   apperrors.CodeNotLogin,
			wantCalls:  1,
		},
		{
			name:       "internal exposure remains application denied to anonymous",
			path:       "/api/config-assets/83",
			resultErr:  apperrors.Forbidden("配置资产读取策略不允许当前身份访问"),
			wantStatus: 200,
			wantCode:   apperrors.CodeForbidden,
			wantCalls:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			management := &fakeManagementService{assetResult: tt.result, assetErr: tt.resultErr}
			module := &Module{handler: confighandler.NewHandler(management, &fakeClientService{})}
			engine := server.Default()
			router := engine.Engine.Group(cfg.ContextPath())
			router.Use(authorizationruntime.NewMiddleware(
				cfg.Authorization,
				nil,
				nil,
				cfg.SSO.SessionCookie.Name,
				cfg.ContextPath(),
			).Handler())
			module.Mount(router)

			resp := ut.PerformRequest(engine.Engine, "GET", tt.path, nil)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status=%d body=%s, want %d", resp.Code, resp.Body.String(), tt.wantStatus)
			}
			if management.assetCalls != tt.wantCalls {
				t.Fatalf("handler calls=%d, want %d", management.assetCalls, tt.wantCalls)
			}
			if tt.wantCode != 0 {
				assertBusinessCode(t, resp, tt.wantCode)
				return
			}
			if got := resp.Body.String(); got != "public-logo" {
				t.Fatalf("public stable asset body=%q", got)
			}
			if management.assetActor.Authenticated {
				t.Fatalf("anonymous public request unexpectedly became authenticated: %+v", management.assetActor)
			}
		})
	}
}

func TestConfigAssetEndpointsCarryServerAuthenticatedContext(t *testing.T) {
	management := &fakeManagementService{assetResult: &filefacade.ConfigAssetOpenResult{
		Reader: io.NopCloser(bytes.NewReader([]byte("asset-bytes"))), Size: int64(len("asset-bytes")), ContentType: "image/png",
		FileName: "brand.png", AssetType: filefacade.ConfigAssetImage, AccessScope: filefacade.ConfigAssetAuthenticated,
	}}
	module := &Module{handler: confighandler.NewHandler(management, &fakeClientService{})}
	engine := configTestEngine(module)

	addResponse := ut.PerformRequest(engine.Engine, "POST", "/config", jsonBody(t, map[string]any{
		"groupId": 1, "configKey": "tenantLogo", "valueType": "IMAGE", "uiWidget": "IMAGE_UPLOAD", "assetFileId": 77,
	}),
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "system:config:add"},
	)
	assertBusinessCode(t, addResponse, apperrors.CodeSuccess)
	if management.addContextUser == nil || management.addContextUser.UserID != 1001 || management.addContextUser.PrimaryOrgID != 22 {
		t.Fatalf("config asset mutation lost server-authenticated user context: %+v", management.addContextUser)
	}
	updateResponse := ut.PerformRequest(engine.Engine, "POST", "/config/update", jsonBody(t, map[string]any{"id": 81, "assetFileId": 78, "version": 1}),
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "system:config:edit"},
	)
	assertBusinessCode(t, updateResponse, apperrors.CodeSuccess)
	if management.updateContextUser == nil || management.updateContextUser.UserID != 1001 || management.updateContextUser.PrimaryOrgID != 22 {
		t.Fatalf("config asset replacement lost server-authenticated user context: %+v", management.updateContextUser)
	}
	deleteResponse := ut.PerformRequest(engine.Engine, "POST", "/config/delete?id=81", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "system:config:delete"},
	)
	assertBusinessCode(t, deleteResponse, apperrors.CodeSuccess)
	if management.deleteContextUser == nil || management.deleteContextUser.UserID != 1001 || management.deleteContextUser.PrimaryOrgID != 22 {
		t.Fatalf("config asset clear lost server-authenticated user context: %+v", management.deleteContextUser)
	}

	assetResponse := ut.PerformRequest(engine.Engine, "GET", "/config-assets/81", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	)
	if assetResponse.Code != 200 || management.assetContextUser == nil || management.assetContextUser.UserID != 1001 || management.assetContextUser.PrimaryOrgID != 22 {
		t.Fatalf("config asset read lost server-authenticated user context: status=%d user=%+v", assetResponse.Code, management.assetContextUser)
	}
}

func TestProtectedConfigMutationsRequireStepUpProof(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		body        map[string]any
		permissions string
	}{
		{
			name:        "sensitive_reveal",
			method:      "POST",
			path:        "/config/204/sensitive/reveal",
			body:        map[string]any{"obfuscatedClientPublicKey": "public-key"},
			permissions: "system:config:sensitive",
		},
		{
			name:        "apply_pending",
			method:      "POST",
			path:        "/config/apply-pending",
			body:        map[string]any{},
			permissions: "system:config:apply",
		},
		{
			name:        "rollback",
			method:      "POST",
			path:        "/config/rollback?logId=701&reason=test",
			body:        map[string]any{},
			permissions: "system:config:rollback",
		},
		{
			name:        "assign_config_scope",
			method:      "POST",
			path:        "/config-scopes/roles/501",
			body:        map[string]any{"grants": []map[string]any{{"groupCode": "ops", "configKey": "title", "canRead": 1, "canWrite": 1}}},
			permissions: "system:config:scope:assign",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			management := &fakeManagementService{}
			module := &Module{handler: confighandler.NewHandler(management, &fakeClientService{})}
			module.BindAuthorization(&fakeConfigAuthFacade{challenge: &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-" + tt.name, ChallengeState: "PENDING"}})
			engine := configTestEngine(module)
			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, jsonBody(t, tt.body),
				ut.Header{Key: "Content-Type", Value: "application/json"},
				ut.Header{Key: "X-Test-Login", Value: "true"},
				ut.Header{Key: "X-Test-Permissions", Value: tt.permissions},
			)
			assertBusinessCode(t, resp, apperrors.CodeChallengeRequired)
			if management.mutationCalls != 0 {
				t.Fatalf("protected config mutation should not execute before proof, got %d calls", management.mutationCalls)
			}
		})
	}
}

func TestProtectedConfigMutationsValidateProofWithCanonicalBinding(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		body        map[string]any
		permissions string
		wantAction  string
		wantBinding string
		assertCall  func(*testing.T, *fakeManagementService)
	}{
		{
			name:        "sensitive_reveal",
			method:      "POST",
			path:        "/config/204/sensitive/reveal",
			body:        map[string]any{"obfuscatedClientPublicKey": "public-key"},
			permissions: "system:config:sensitive",
			wantAction:  "CONFIG_SENSITIVE_REVEAL",
			wantBinding: "config:204|reveal",
			assertCall: func(t *testing.T, management *fakeManagementService) {
				t.Helper()
				if management.revealCalls != 1 {
					t.Fatalf("expected reveal service call, got %d", management.revealCalls)
				}
			},
		},
		{
			name:        "apply_pending",
			method:      "POST",
			path:        "/config/apply-pending",
			body:        map[string]any{},
			permissions: "system:config:apply",
			wantAction:  "CONFIG_APPLY_PENDING",
			wantBinding: "config:apply-pending",
		},
		{
			name:        "rollback",
			method:      "POST",
			path:        "/config/rollback?logId=701&reason=test",
			body:        map[string]any{},
			permissions: "system:config:rollback",
			wantAction:  "CONFIG_ROLLBACK",
			wantBinding: "config:rollback:701",
		},
		{
			name:        "assign_config_scope",
			method:      "POST",
			path:        "/config-scopes/roles/501",
			body:        map[string]any{"grants": []map[string]any{{"groupCode": "ops", "configKey": "title", "canRead": 1, "canWrite": 1}, {"groupCode": "app", "canRead": 1}}},
			permissions: "system:config:scope:assign",
			wantAction:  "CONFIG_SCOPE_ASSIGN",
			wantBinding: "config-scope:role:501|scopes:app:r1w0d0,ops.title:r1w1d0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			management := &fakeManagementService{}
			auth := &fakeConfigAuthFacade{}
			module := &Module{handler: confighandler.NewHandler(management, &fakeClientService{})}
			module.BindAuthorization(auth)
			engine := configTestEngine(module)

			resp := ut.PerformRequest(engine.Engine, tt.method, tt.path, jsonBody(t, tt.body),
				ut.Header{Key: "Content-Type", Value: "application/json"},
				ut.Header{Key: "X-Test-Login", Value: "true"},
				ut.Header{Key: "X-Test-Permissions", Value: tt.permissions},
				ut.Header{Key: "Proof-Token", Value: "proof-token"},
				ut.Header{Key: "Flow-Nonce", Value: "flow-1"},
			)
			assertBusinessCode(t, resp, apperrors.CodeSuccess)
			if auth.lastValidate.BusinessAction != tt.wantAction {
				t.Fatalf("unexpected business action: %#v", auth.lastValidate)
			}
			if auth.lastValidate.OperationBinding != tt.wantBinding {
				t.Fatalf("unexpected operation binding: %#v", auth.lastValidate)
			}
			if auth.lastValidate.FlowNonce != "flow-1" || !auth.lastValidate.ConsumeOnce {
				t.Fatalf("proof validation must preserve flow nonce and consume once: %#v", auth.lastValidate)
			}
			if management.lastProof.BusinessAction != tt.wantAction || management.lastProof.OperationBinding != tt.wantBinding {
				t.Fatalf("protected config service did not receive proof metadata: %#v", management.lastProof)
			}
			if management.lastProof.ProofIdentifier == "" || management.lastProof.ChallengeIdentifier == "" {
				t.Fatalf("protected config service did not receive proof identifiers: %#v", management.lastProof)
			}
			if tt.assertCall != nil {
				tt.assertCall(t, management)
			}
		})
	}
}

func configTestEngine(module *Module) *server.Hertz {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "true" {
			securitycontext.Set(reqCtx, &securitycontext.UserContext{
				UserID:       1001,
				Username:     "admin",
				RoleIDs:      []int64{501},
				Permissions:  splitPermissions(string(reqCtx.Request.Header.Peek("X-Test-Permissions"))),
				PrimaryOrgID: 22,
				OrgIDs:       []int64{22},
				SessionID:    "sid-1",
			})
		}
		reqCtx.Next(ctx)
	})
	module.Mount(engine.Engine)
	return engine
}

type fakeManagementService struct {
	mutationCalls     int
	revealCalls       int
	lastProof         stepup.ProofMetadata
	assetResult       *filefacade.ConfigAssetOpenResult
	assetErr          error
	assetCalls        int
	assetID           int64
	assetActor        configapp.Actor
	addContextUser    *securitycontext.UserContext
	updateContextUser *securitycontext.UserContext
	deleteContextUser *securitycontext.UserContext
	assetContextUser  *securitycontext.UserContext
}

func (f *fakeManagementService) AddConfigGroup(context.Context, configapp.Actor, configfacade.ConfigGroupAddRequest) (int64, error) {
	return 1, nil
}
func (f *fakeManagementService) UpdateConfigGroup(context.Context, configapp.Actor, configfacade.ConfigGroupUpdateRequest) error {
	return nil
}
func (f *fakeManagementService) DeleteConfigGroup(context.Context, configapp.Actor, int64) error {
	return nil
}
func (f *fakeManagementService) GetConfigGroupPage(context.Context, configapp.Actor, configfacade.ConfigGroupQueryRequest) (*configfacade.PageResult[configfacade.ConfigGroupVO], error) {
	return &configfacade.PageResult[configfacade.ConfigGroupVO]{
		Current: 1,
		Size:    10,
		Total:   1,
		Records: []configfacade.ConfigGroupVO{{ID: 1, GroupCode: "basic", GroupName: "Basic", Status: 1}},
	}, nil
}
func (f *fakeManagementService) GetConfigGroupByID(context.Context, configapp.Actor, int64) (*configfacade.ConfigGroupVO, error) {
	return &configfacade.ConfigGroupVO{ID: 1, GroupCode: "basic", GroupName: "Basic", Status: 1}, nil
}
func (f *fakeManagementService) MoveConfigGroup(context.Context, configapp.Actor, int64, *int64, *int64) error {
	return nil
}
func (f *fakeManagementService) AddConfig(ctx context.Context, _ configapp.Actor, _ configfacade.ConfigAddRequest) (int64, error) {
	f.addContextUser = securitycontext.FromContext(ctx)
	return 1, nil
}
func (f *fakeManagementService) UpdateConfig(ctx context.Context, _ configapp.Actor, _ configfacade.ConfigUpdateRequest) error {
	f.updateContextUser = securitycontext.FromContext(ctx)
	return nil
}
func (f *fakeManagementService) DeleteConfig(ctx context.Context, _ configapp.Actor, _ int64) error {
	f.deleteContextUser = securitycontext.FromContext(ctx)
	return nil
}
func (f *fakeManagementService) GetConfigByID(context.Context, configapp.Actor, int64) (*configfacade.ConfigVO, error) {
	return &configfacade.ConfigVO{ID: 1, ConfigKey: "banner", ConfigValue: "hello", ValueType: "string"}, nil
}
func (f *fakeManagementService) OpenConfigAsset(ctx context.Context, actor configapp.Actor, id int64) (*filefacade.ConfigAssetOpenResult, error) {
	f.assetCalls++
	f.assetID = id
	f.assetActor = actor
	f.assetContextUser = securitycontext.FromContext(ctx)
	if f.assetErr != nil {
		return nil, f.assetErr
	}
	if f.assetResult == nil {
		return nil, apperrors.NotFound("配置资产不存在")
	}
	return f.assetResult, nil
}
func (f *fakeManagementService) GetConfigPage(context.Context, configapp.Actor, configfacade.ConfigQueryRequest) (*configfacade.PageResult[configfacade.ConfigVO], error) {
	return &configfacade.PageResult[configfacade.ConfigVO]{
		Current: 1,
		Size:    10,
		Total:   1,
		Records: []configfacade.ConfigVO{{ID: 1, ConfigKey: "banner", ConfigValue: "hello", ValueType: "string"}},
	}, nil
}
func (f *fakeManagementService) ChangeEnabled(context.Context, configapp.Actor, int64, configfacade.ConfigEnabledRequest) error {
	return nil
}
func (f *fakeManagementService) RevealSensitiveValue(_ context.Context, actor configapp.Actor, _ int64, _ configfacade.ConfigSensitiveRevealRequest) (*configfacade.ConfigSensitiveRevealResponse, error) {
	f.mutationCalls++
	f.revealCalls++
	f.lastProof = actor.StepUpProof
	return &configfacade.ConfigSensitiveRevealResponse{EncryptedValue: "ciphertext"}, nil
}
func (f *fakeManagementService) ApplyPendingConfigs(_ context.Context, actor configapp.Actor, _ bool) (int, error) {
	f.mutationCalls++
	f.lastProof = actor.StepUpProof
	return 1, nil
}
func (f *fakeManagementService) GetPendingConfigs(context.Context, configapp.Actor) ([]configfacade.PendingConfigVO, error) {
	return []configfacade.PendingConfigVO{{LogID: 1, ConfigID: 1, ConfigKey: "banner"}}, nil
}
func (f *fakeManagementService) GetConfigChangeHistory(context.Context, configapp.Actor, int64, int) ([]configfacade.ConfigChangeLogVO, error) {
	return []configfacade.ConfigChangeLogVO{{ID: 1, ConfigID: 1, ConfigKey: "banner"}}, nil
}
func (f *fakeManagementService) RollbackConfigChange(_ context.Context, actor configapp.Actor, _ int64, _ string) error {
	f.mutationCalls++
	f.lastProof = actor.StepUpProof
	return nil
}
func (f *fakeManagementService) GetOperationChain(context.Context, configapp.Actor, int64) ([]configfacade.ConfigChangeLogVO, error) {
	return []configfacade.ConfigChangeLogVO{{ID: 1, ConfigID: 1, ConfigKey: "banner"}}, nil
}
func (f *fakeManagementService) GetAuditLogs(context.Context, configapp.Actor, configfacade.AuditLogQueryRequest) ([]configfacade.ConfigChangeLogVO, error) {
	return []configfacade.ConfigChangeLogVO{{ID: 1, ConfigID: 1, ConfigKey: "banner"}}, nil
}
func (f *fakeManagementService) GetRoleConfigScopes(context.Context, configapp.Actor, int64) ([]configfacade.ConfigScopeGrantVO, error) {
	return []configfacade.ConfigScopeGrantVO{{GroupCode: "public", CanRead: 1}}, nil
}
func (f *fakeManagementService) AssignRoleConfigScopes(_ context.Context, actor configapp.Actor, _ int64, _ configfacade.AssignRoleConfigScopesRequest) error {
	f.mutationCalls++
	f.lastProof = actor.StepUpProof
	return nil
}

type fakeConfigAuthFacade struct {
	challenge    *authorizationfacade.StepUpChallengeVO
	lastValidate authorizationfacade.StepUpValidateRequest
}

func (f *fakeConfigAuthFacade) GetLoginUser(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeConfigAuthFacade) GetLoginUserPermitNull(context.Context, authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeConfigAuthFacade) GetLoginUserID(context.Context, authorizationfacade.RequestScope) (int64, error) {
	return 0, nil
}
func (f *fakeConfigAuthFacade) GetLoginUsername(context.Context, authorizationfacade.RequestScope) (string, error) {
	return "", nil
}
func (f *fakeConfigAuthFacade) IsLogin(context.Context, authorizationfacade.RequestScope) bool {
	return true
}
func (f *fakeConfigAuthFacade) IsAdmin(context.Context, authorizationfacade.RequestScope) bool {
	return false
}
func (f *fakeConfigAuthFacade) IsCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return false
}
func (f *fakeConfigAuthFacade) IsAdminOrCurrentUser(context.Context, authorizationfacade.RequestScope, int64) bool {
	return false
}
func (f *fakeConfigAuthFacade) GetUserVO(context.Context, int64) (*authorizationfacade.UserVO, error) {
	return nil, nil
}
func (f *fakeConfigAuthFacade) RefreshUserPermissionCache(context.Context, int64) error {
	return nil
}
func (f *fakeConfigAuthFacade) GetUserPermissionsByModule(context.Context, authorizationfacade.RequestScope, string) ([]string, error) {
	return nil, nil
}
func (f *fakeConfigAuthFacade) CreateStepUpChallenge(context.Context, authorizationfacade.RequestScope, authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	if f.challenge != nil {
		return f.challenge, nil
	}
	return &authorizationfacade.StepUpChallengeVO{ChallengeIdentifier: "challenge-1", ChallengeState: "PENDING"}, nil
}
func (f *fakeConfigAuthFacade) VerifyStepUp(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
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
func (f *fakeConfigAuthFacade) ValidateStepUpToken(_ context.Context, _ authorizationfacade.RequestScope, request authorizationfacade.StepUpValidateRequest) (bool, error) {
	f.lastValidate = request
	return true, nil
}

type fakeClientService struct{}

func (f *fakeClientService) ListConfigsForClient(_ context.Context, _ configapp.Actor, request configfacade.ConfigClientListRequest) (map[string]configfacade.ConfigValueDTO, error) {
	return map[string]configfacade.ConfigValueDTO{
		request.GroupCode + ".banner": {Key: "banner", GroupCode: request.GroupCode, Type: "STRING", Value: "ok"},
	}, nil
}

func (f *fakeClientService) GetConfigByKeyForClient(_ context.Context, actor configapp.Actor, configKey string) (*configfacade.ConfigValueDTO, error) {
	if configKey == "secure.banner" && !actor.Authenticated {
		return nil, apperrors.Unauthorized("该配置需要登录后访问")
	}
	return &configfacade.ConfigValueDTO{Key: configKey, Type: "string", Value: "ok"}, nil
}

func (f *fakeClientService) GetConfigBatchForClient(_ context.Context, actor configapp.Actor, request configfacade.ConfigBatchRequest) (map[string]configfacade.ConfigValueDTO, error) {
	for _, key := range request.ConfigKeys {
		if key == "secure.banner" && !actor.Authenticated {
			return nil, apperrors.Unauthorized("该配置需要登录后访问")
		}
	}
	result := make(map[string]configfacade.ConfigValueDTO, len(request.ConfigKeys))
	for _, key := range request.ConfigKeys {
		result[key] = configfacade.ConfigValueDTO{Key: key, Type: "string", Value: "ok"}
	}
	return result, nil
}

func assertBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != expected {
		t.Fatalf("unexpected business code: got=%d want=%d body=%s", result.Code, expected, recorder.Body.String())
	}
}

func jsonBody(t *testing.T, value any) *ut.Body {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return &ut.Body{Body: bytes.NewReader(body), Len: len(body)}
}

func splitPermissions(value string) []string {
	if value == "" {
		return nil
	}
	result := make([]string, 0, 2)
	for _, item := range bytes.Split([]byte(value), []byte(",")) {
		trimmed := bytes.TrimSpace(item)
		if len(trimmed) == 0 {
			continue
		}
		result = append(result, string(trimmed))
	}
	return result
}
