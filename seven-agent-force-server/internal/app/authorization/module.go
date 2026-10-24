package authorization

import (
	"context"
	"fmt"
	"strings"

	authorizationapp "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/application"
	authorizationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	authorizationhandler "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/handler"
	authorizationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/infrastructure"
	authorizationruntime "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/runtime"
	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Dependencies struct {
	SsoTokens   ssofacade.TokenFacade
	SsoSessions ssofacade.SessionFacade
	Challenges  challengefacade.ChallengeInternalFacade
	Proof       challengefacade.ProofTokenVerifier
	Users       userfacade.AuthorizationContextFacade
	// CacheInvalidations is bound after cache-governance composition. It is
	// deliberately optional so authorization remains source-authoritative when
	// the durable cache protocol is disabled or unavailable.
	CacheInvalidations cachegovernancefacade.InvalidationRegistrar
}

type Module struct {
	service      *authorizationapp.Service
	authCtrl     *authorizationhandler.AuthHandler
	roleCtrl     *authorizationhandler.RoleHandler
	tempCtrl     *authorizationhandler.TemporaryPermissionHandler
	internalCtrl *authorizationhandler.InternalHandler
	middleware   *authorizationruntime.Middleware
	oplog        adminfacade.OperationLogger
}

// BindRoleGrantConfigScopes binds config-scope validation and persistence into the atomic role grant policy.
func (m *Module) BindRoleGrantConfigScopes(port authorizationfacade.RoleGrantConfigScopePort) {
	if m != nil && m.service != nil {
		m.service.BindRoleGrantConfigScopes(port)
	}
}

// BindCacheInvalidations connects authorization-owned mutations to the
// durable cache-governance Facade after startup composition.
func (m *Module) BindCacheInvalidations(registrar cachegovernancefacade.InvalidationRegistrar) {
	if m != nil && m.service != nil {
		m.service.BindCacheInvalidations(registrar)
	}
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, authorizationfacade.AuthFacade, authorizationfacade.PermissionFacade, authorizationfacade.RoleFacade, authorizationfacade.AccessExplainFacade, error) {
	if deps.Infra.Datasource == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("authorization module requires datasource provider")
	}
	repository, err := authorizationinfra.NewRepository(deps.Infra.Datasource, refs.Users)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	service := authorizationapp.NewService(
		deps.Config.Authorization,
		deps.Infra.CacheMgr,
		deps.Infra.Transactor,
		repository,
		authorizationdomain.NewService(),
		deps.IDGen,
		refs.SsoTokens,
		refs.SsoSessions,
		refs.Challenges,
		refs.Proof,
		deps.Features,
	)
	service.BindCacheInvalidations(refs.CacheInvalidations)
	module := &Module{
		service:      service,
		authCtrl:     authorizationhandler.NewAuthHandler(service, service),
		roleCtrl:     authorizationhandler.NewRoleHandler(service),
		tempCtrl:     authorizationhandler.NewTemporaryPermissionHandler(service),
		internalCtrl: authorizationhandler.NewInternalHandler(service),
		middleware:   authorizationruntime.NewMiddleware(deps.Config.Authorization, deps.Infra.CacheMgr, service, deps.Config.SSO.SessionCookie.Name, deps.Config.ContextPath()),
	}
	module.roleCtrl.BindAuthorization(service)
	module.tempCtrl.BindAuthorization(service)
	snapshotter, _ := deps.Infra.Transactor.(store.Snapshotter)
	accessExplain := authorizationapp.NewAccessExplainService(repository, authorizationdomain.NewService(), deps.Features, snapshotter)
	return module, service, service, service, accessExplain, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "authorization", Prefix: "/auth"}
}

func (m *Module) Middlewares() []app.HandlerFunc {
	if m == nil || m.middleware == nil {
		return nil
	}
	return []app.HandlerFunc{m.middleware.Handler()}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil {
		return
	}
	engine.GET("/auth/me", m.wrapAuth(m.authCtrl.GetCurrentUser))
	engine.GET("/auth/menus", m.wrapAuth(m.authCtrl.GetCurrentUserMenus))
	engine.GET("/auth/permissions", m.wrapAuth(m.authCtrl.GetUserPermissionsByModule))
	engine.POST("/auth/step-up/challenge", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "创建 step-up 挑战",
		IncludeParams: true,
	}, m.wrapAuth(m.authCtrl.CreateStepUpChallenge)))
	engine.POST("/auth/step-up/verify", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "校验 step-up 挑战",
		IncludeParams: true,
	}, m.wrapAuth(m.authCtrl.VerifyStepUp)))
	engine.POST("/auth/step-up/validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "校验 step-up proof token",
		IncludeParams: true,
	}, m.wrapAuth(m.authCtrl.ValidateStepUp)))

	engine.GET("/system/role/list", m.wrapPermission("system:role:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询角色列表",
	}, m.roleCtrl.ListRoles)))
	engine.GET("/system/role/page", m.wrapPermission("system:role:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "分页查询角色",
	}, m.roleCtrl.PageRoles)))
	engine.GET("/system/role/security-status", m.wrapPermission("system:role:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询角色安全状态",
	}, m.roleCtrl.GetSecurityStatus)))
	engine.GET("/system/role/:roleId/grant-snapshot", m.wrapPermission("system:role:grant", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeOther, Description: "查询角色授权快照",
	}, m.roleCtrl.GetRoleGrantSnapshot)))
	engine.POST("/system/role/:roleId/grant-preview", m.wrapPermission("system:role:grant", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeOther, Description: "预览角色授权变更", IncludeParams: true,
	}, m.roleCtrl.PreviewRoleGrantBundle)))
	engine.PUT("/system/role/:roleId/grants", m.wrapPermission("system:role:grant", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeRoleAssignPermission, Description: "原子提交角色授权", IncludeParams: true,
	}, m.roleCtrl.CommitRoleGrantBundle)))
	engine.GET("/system/role/:roleId/menu-tree", m.wrapPermission("system:role:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询角色菜单树",
	}, m.roleCtrl.GetRoleMenuTree)))
	engine.GET("/system/role/:roleId/menus", m.wrapPermission("system:role:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询角色菜单",
	}, m.roleCtrl.GetRoleMenusCompat)))
	engine.POST("/system/role/:roleId/menus", m.wrapPermission("system:role:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleAssignPermission,
		Description:   "分配角色菜单",
		IncludeParams: true,
	}, m.roleCtrl.AssignRoleMenusCompat)))
	engine.GET("/system/role/:id", m.wrapPermission("system:role:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询角色详情",
	}, m.roleCtrl.GetRole)))
	engine.POST("/system/role", m.wrapPermission("system:role:add", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleCreate,
		Description:   "创建角色",
		IncludeParams: true,
	}, m.roleCtrl.CreateRole)))
	engine.PUT("/system/role", m.wrapPermission("system:role:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleUpdate,
		Description:   "更新角色",
		IncludeParams: true,
	}, m.roleCtrl.UpdateRole)))
	engine.DELETE("/system/role/:id", m.wrapPermission("system:role:remove", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleDelete,
		Description:   "删除角色",
		IncludeParams: true,
	}, m.roleCtrl.DeleteRole)))
	engine.GET("/system/role/:roleId/depts", m.wrapPermission("system:role:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询角色自定数据范围",
	}, m.roleCtrl.GetRoleDeptIDs)))
	engine.POST("/system/role/depts/assign", m.wrapPermission("system:role:grant", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleAssignPermission,
		Description:   "分配角色自定数据范围",
		IncludeParams: true,
	}, m.roleCtrl.AssignRoleDepts)))
	engine.POST("/system/role/permissions/assign", m.wrapPermission("system:role:grant", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleAssignPermission,
		Description:   "分配角色权限",
		IncludeParams: true,
	}, m.roleCtrl.AssignRolePermissions)))
	engine.POST("/system/role/user-roles/assign", m.wrapPermission("system:user-role:assign", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRoleUpdate,
		Description:   "分配用户角色",
		IncludeParams: true,
	}, m.roleCtrl.AssignUserRoles)))

	engine.GET("/system/menu/tree", m.wrapPermission("system:menu:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询菜单树",
	}, m.roleCtrl.GetMenuTree)))
	engine.GET("/system/menu/tree/enabled", m.wrapPermission("system:menu:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询启用菜单树",
	}, m.roleCtrl.GetEnabledMenuTree)))
	engine.GET("/system/menu/permissions", m.wrapPermission("system:permission:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询权限资源",
	}, m.roleCtrl.ListPermissions)))
	engine.POST("/system/menu/permissions", m.wrapPermission("system:permission:add", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "创建权限资源",
		IncludeParams: true,
	}, m.roleCtrl.CreatePermission)))
	engine.GET("/system/menu/permissions/:permissionId", m.wrapPermission("system:permission:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询权限资源详情",
	}, m.roleCtrl.GetPermission)))
	engine.PUT("/system/menu/permissions/:permissionId", m.wrapPermission("system:permission:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "更新权限资源",
		IncludeParams: true,
	}, m.roleCtrl.UpdatePermission)))
	engine.DELETE("/system/menu/permissions/:permissionId", m.wrapPermission("system:permission:remove", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "删除权限资源",
		IncludeParams: true,
	}, m.roleCtrl.DeletePermission)))
	engine.GET("/system/menu/:menuId/permissions", m.wrapPermission("system:menu:permission:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询菜单权限绑定",
	}, m.roleCtrl.GetMenuPermissionIDs)))
	engine.POST("/system/menu/:menuId/permissions", m.wrapPermission("system:menu:permission:assign", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "绑定菜单权限",
		IncludeParams: true,
	}, m.roleCtrl.BindMenuPermissions)))
	engine.GET("/system/menu/:id", m.wrapPermission("system:menu:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询菜单详情",
	}, m.roleCtrl.GetMenu)))
	engine.POST("/system/menu", m.wrapPermission("system:menu:add", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "创建菜单",
		IncludeParams: true,
	}, m.roleCtrl.CreateMenu)))
	engine.PUT("/system/menu", m.wrapPermission("system:menu:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "更新菜单",
		IncludeParams: true,
	}, m.roleCtrl.UpdateMenu)))
	engine.DELETE("/system/menu/:id", m.wrapPermission("system:menu:remove", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "删除菜单",
		IncludeParams: true,
	}, m.roleCtrl.DeleteMenu)))

	engine.POST("/admin/temp-permission/grant", m.wrapPermission("admin:temp-permission:grant", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "授予临时权限",
		IncludeParams: true,
	}, m.tempCtrl.Grant)))
	engine.DELETE("/admin/temp-permission/revoke", m.wrapPermission("admin:temp-permission:revoke", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "撤销临时权限",
		IncludeParams: true,
	}, m.tempCtrl.RevokeCompat)))
	engine.POST("/admin/temp-permission/revoke", m.wrapPermission("admin:temp-permission:revoke", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "撤销临时权限",
		IncludeParams: true,
	}, m.tempCtrl.Revoke)))
	engine.PUT("/admin/temp-permission/extend", m.wrapPermission("admin:temp-permission:extend", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "延长临时权限",
		IncludeParams: true,
	}, m.tempCtrl.ExtendCompat)))
	engine.POST("/admin/temp-permission/extend", m.wrapPermission("admin:temp-permission:extend", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "延长临时权限",
		IncludeParams: true,
	}, m.tempCtrl.Extend)))
	engine.POST("/admin/temp-permission/cleanup", m.wrapPermission("admin:temp-permission:cleanup", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "清理过期临时权限",
	}, m.tempCtrl.Cleanup)))
	engine.GET("/admin/temp-permission/statistics", m.wrapPermission("admin:temp-permission:stats", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询临时权限统计",
	}, m.tempCtrl.Stats)))
	engine.GET("/admin/temp-permission/stats", m.wrapPermission("admin:temp-permission:stats", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询临时权限统计",
	}, m.tempCtrl.Stats)))
	engine.GET("/admin/temp-permission/list", m.wrapPermission("admin:temp-permission:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询用户临时权限",
		IncludeParams: true,
	}, m.tempCtrl.ListByUserQuery)))
	engine.GET("/admin/temp-permission/user/:userId", m.wrapPermission("admin:temp-permission:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询用户临时权限",
	}, m.tempCtrl.ListByUser)))

	engine.GET("/internal/auth/user/:userId", m.wrapInternal(m.internalCtrl.GetUser))
	engine.GET("/internal/auth/user/:userId/vo", m.wrapInternal(m.internalCtrl.GetUser))
	engine.POST("/internal/auth/user/:userId/permission-cache/refresh", m.wrapInternal(m.internalCtrl.RefreshPermissionCache))
}

func (m *Module) wrapAuth(handler func(context.Context, *app.RequestContext, authorizationfacade.RequestScope)) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		scope, err := buildRequestScope(reqCtx)
		if err != nil {
			responseError(reqCtx, err)
			return
		}
		handler(ctx, reqCtx, scope)
	}
}

func (m *Module) wrapPermission(permission string, handler app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			responseError(reqCtx, fmt.Errorf("未登录"))
			return
		}
		if !securitycontext.HasPermission(reqCtx, permission) {
			response.Error(reqCtx, apperrors.PermissionDenied(permission))
			return
		}
		handler(ctx, reqCtx)
	}
}

func (m *Module) wrapInternal(handler app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		internal, _ := reqCtx.Get("__seven_auth_internal__")
		if ok, _ := internal.(bool); !ok {
			responseError(reqCtx, fmt.Errorf("内部服务鉴权失败"))
			return
		}
		handler(ctx, reqCtx)
	}
}

func buildRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	userID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok {
		return authorizationfacade.RequestScope{}, fmt.Errorf("未登录")
	}
	user := securitycontext.Require(reqCtx)
	return authorizationfacade.RequestScope{
		UserID:    userID,
		Username:  user.Username,
		IPAddress: reqCtx.ClientIP(),
		UserAgent: string(reqCtx.UserAgent()),
		DeviceID:  string(reqCtx.Request.Header.Peek("X-Device-Id")),
		TenantID:  string(reqCtx.Request.Header.Peek("X-Tenant-Id")),
		SessionID: user.SessionID,
		Source:    user.Source,
	}, nil
}

func responseError(reqCtx *app.RequestContext, err error) {
	if strings.Contains(err.Error(), "无权限") {
		response.Error(reqCtx, apperrors.Forbidden(err.Error()))
		return
	}
	response.Error(reqCtx, apperrors.Unauthorized(err.Error()))
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m == nil {
		return
	}
	m.oplog = oplog
}

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}
