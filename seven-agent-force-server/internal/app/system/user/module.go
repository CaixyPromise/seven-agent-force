package user

import (
	"context"
	"fmt"
	"strings"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	userhandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/handler"
	userinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/infrastructure"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Facades struct {
	Subjects               userfacade.SubjectFacade
	Profiles               userfacade.ProfileFacade
	Accounts               userfacade.AccountFacade
	Provisioning           userfacade.ProvisioningFacade
	AdminUsers             userfacade.AdminUserFacade
	UserSelectors          userfacade.UserSelectorFacade
	Relations              userfacade.UserRelationFacade
	NotificationAudience   userfacade.NotificationAudienceFacade
	NotificationRecipients userfacade.NotificationRecipientFacade
	Context                userfacade.AuthorizationContextFacade
	Orgs                   userfacade.OrgFacade
	Depts                  userfacade.DeptFacade
	Posts                  userfacade.PostFacade
}

type Module struct {
	service      *application.Service
	facades      Facades
	handler      *userhandler.Handler
	adminHandler *userhandler.AdminHandler
	oplog        adminfacade.OperationLogger
}

func Install(deps bootstrapruntime.ModuleDeps) (*Module, Facades, error) {
	if deps.Infra.Datasource == nil {
		return nil, Facades{}, fmt.Errorf("system user module requires datasource provider")
	}
	if deps.Security.Password == nil {
		return nil, Facades{}, fmt.Errorf("system user module requires password service")
	}
	repository, err := userinfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, Facades{}, err
	}
	service := application.NewService(
		repository,
		domain.NewService(),
		deps.Security.Password,
		nil,
		application.WithTransactor(deps.Infra.Transactor),
		application.WithIDGenerator(deps.IDGen),
	)
	module := &Module{
		service: service,
		facades: Facades{
			Subjects:               service,
			Profiles:               service,
			Accounts:               service,
			Provisioning:           service,
			AdminUsers:             service,
			UserSelectors:          service,
			Relations:              service,
			NotificationAudience:   service,
			NotificationRecipients: service,
			Context:                service,
			Orgs:                   service,
			Depts:                  service,
			Posts:                  service,
		},
		handler:      userhandler.NewHandler(service, service, nil, service),
		adminHandler: userhandler.NewAdminHandler(service, service, service, service, service),
	}
	return module, module.facades, nil
}

func (m *Module) BindCredentialFacade(credentials credentialfacade.UserCredentialFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindCredentials(credentials)
}

func (m *Module) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if m == nil || m.handler == nil {
		return
	}
	m.handler.BindAuthorization(auth)
	if m.adminHandler != nil {
		m.adminHandler.BindAuthorization(auth)
	}
}

func (m *Module) BindFileAssets(files filefacade.FileAssetBindingFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindFileAssets(files)
}

func (m *Module) BindPermissions(permissions authorizationfacade.PermissionFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindPermissions(permissions)
}

func (m *Module) BindRoleAssignments(assignments authorizationfacade.UserRoleAssignmentFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindRoleAssignments(assignments)
}

func (m *Module) BindAccessExplain(access authorizationfacade.AccessExplainFacade) {
	if m == nil || m.adminHandler == nil {
		return
	}
	m.adminHandler.BindAccessExplain(access)
}

func (m *Module) BindSessions(sessions ssofacade.SessionFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindSessions(sessions)
}

// BindManagedSessions binds cutoff-based session revocation for trusted Node commands.
func (m *Module) BindManagedSessions(sessions ssofacade.ManagedSessionFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindManagedSessions(sessions)
}

// BindCacheInvalidations connects user-owned authorization-affecting writes
// to the narrow cache-governance Facade. It deliberately does not reach into
// authorization application internals.
func (m *Module) BindCacheInvalidations(registrar cachegovernancefacade.InvalidationRegistrar) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindCacheInvalidations(registrar)
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m == nil {
		return
	}
	m.oplog = oplog
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "system-user", Prefix: "/user"}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m.handler == nil {
		return
	}
	engine.GET("/user/profile/me", m.handler.GetCurrentUserProfile)
	engine.POST("/user/profile/update", m.handler.UpdateCurrentUserProfile)
	engine.POST("/user/profile/email/update", m.handler.UpdateCurrentUserEmail)
	engine.POST("/user/profile/avatar/commit", m.handler.CommitCurrentUserAvatar)
	engine.POST("/user/profile/change-password", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeUserResetPassword, Description: "修改当前用户密码", IncludeParams: true}, m.handler.ChangeCurrentUserPassword))
	engine.GET("/user/options", m.wrapLogin(m.handler.ListUserOptions))
	engine.GET("/user/search", m.wrapLogin(m.handler.SearchUsers))
	engine.GET("/user/simple/:id", m.wrapLogin(m.handler.GetSimpleUser))

	admin := m.adminHandler
	engine.GET("/user/list/page", m.wrapPermission("system:user:list", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分页查询用户", IncludeParams: true}, admin.QueryUsers)))
	engine.POST("/user/list/page", m.wrapPermission("system:user:list", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分页查询用户", IncludeParams: true}, admin.QueryUsers)))
	engine.GET("/user/get/:id", m.wrapPermission("system:user:query", admin.GetUser))
	engine.POST("/user/create", m.wrapPermission("system:user:create", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "创建用户", IncludeParams: true}, admin.CreateUser)))
	engine.POST("/user/update", m.wrapPermission("system:user:update", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "更新用户", IncludeParams: true}, admin.UpdateUser)))
	engine.POST("/user/delete/:id", m.wrapPermission("system:user:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "删除用户"}, admin.DeleteUser)))
	engine.POST("/user/status/:id", m.wrapPermission("system:user:status", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeUserUpdateStatus, Description: "修改用户状态", IncludeParams: true, Enrichers: []adminfacade.OperationLogEnricher{userStatusOperationEnricher{}}}, admin.ChangeUserStatus)))
	engine.POST("/user/reset-password/:id", m.wrapPermission("system:user:reset-password", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "重置用户密码", IncludeParams: true}, admin.ResetPassword)))
	engine.GET("/user/:id/roles", m.wrapPermission("system:user:query", admin.ListUserRoles))
	engine.POST("/user/:id/roles", m.wrapPermission("system:user:update", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分配用户角色", IncludeParams: true}, admin.AssignUserRoles)))
	engine.GET("/user/:id/orgs", m.wrapPermission("system:user:query", admin.ListUserOrgs))
	engine.POST("/user/:id/orgs", m.wrapPermission("system:user:update", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分配用户组织", IncludeParams: true}, admin.AssignUserOrgs)))
	engine.GET("/user/:id/depts", m.wrapPermission("system:user:query", admin.ListUserDepts))
	engine.POST("/user/:id/depts", m.wrapPermission("system:user:update", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分配用户部门", IncludeParams: true}, admin.AssignUserDepts)))
	engine.GET("/user/:id/posts", m.wrapPermission("system:user:query", admin.ListUserPosts))
	engine.POST("/user/:id/posts", m.wrapPermission("system:user:update", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分配用户岗位", IncludeParams: true}, admin.AssignUserPosts)))
	engine.GET("/system/user/:id/effective-access", m.wrapPermission("system:user:access:query", admin.GetEffectiveAccess))
	engine.GET("/system/user/:id/access-explain", m.wrapPermission("system:user:access:explain", admin.ExplainPermission))

	engine.POST("/org/create", m.wrapPermission("system:org:create", admin.CreateOrg))
	engine.POST("/org/update", m.wrapPermission("system:org:update", admin.UpdateOrg))
	engine.POST("/org/delete/:id", m.wrapPermission("system:org:delete", admin.DeleteOrg))
	engine.GET("/org/get/:id", m.wrapPermission("system:org:list", admin.GetOrg))
	engine.GET("/org/getByCode/:code", m.wrapPermission("system:org:list", admin.GetOrgByCode))
	engine.GET("/org/getByUserId/:userId", m.wrapPermission("system:org:list", admin.GetOrgByUserID))
	engine.GET("/org/tree", m.wrapPermission("system:org:list", admin.OrgTree))
	engine.GET("/org/children/:parentId", m.wrapPermission("system:org:list", admin.OrgChildren))
	engine.GET("/org/active", m.wrapPermission("system:org:list", admin.ActiveOrgs))
	engine.GET("/org/checkCode", m.wrapPermission("system:org:list", admin.CheckOrgCode))
	engine.POST("/org/changeStatus", m.wrapPermission("system:org:update", admin.ChangeOrgStatus))
	engine.POST("/org/move", m.wrapPermission("system:org:update", admin.MoveOrg))

	engine.GET("/system/dept/tree", m.wrapPermission("system:dept:list", admin.DeptTree))
	engine.GET("/system/dept/tree/enabled", m.wrapPermission("system:dept:view", admin.EnabledDeptTree))
	engine.GET("/system/dept/options", m.wrapPermission("system:dept:list", admin.DeptOptions))
	engine.GET("/system/dept/:deptId/children", m.wrapPermission("system:dept:query", admin.ChildDeptIDs))
	engine.GET("/system/dept/:id", m.wrapPermission("system:dept:query", admin.GetDept))
	engine.POST("/system/dept", m.wrapPermission("system:dept:add", admin.CreateDept))
	engine.PUT("/system/dept", m.wrapPermission("system:dept:edit", admin.UpdateDept))
	engine.DELETE("/system/dept/:id", m.wrapPermission("system:dept:remove", admin.DeleteDept))

	engine.GET("/system/post/page", m.wrapPermission("system:post:list", admin.QueryPosts))
	engine.GET("/system/post/list", m.wrapPermission("system:post:list", admin.ListPosts))
	engine.GET("/system/post/role/:roleId/posts", m.wrapPermission("system:role:query", admin.GetPostsByRole))
	engine.GET("/system/post/:id", m.wrapPermission("system:post:list", admin.GetPost))
	engine.POST("/system/post", m.wrapPermission("system:post:create", admin.CreatePost))
	engine.PUT("/system/post", m.wrapPermission("system:post:update", admin.UpdatePost))
	engine.DELETE("/system/post/batch", m.wrapPermission("system:post:delete", admin.BatchDeletePosts))
	engine.DELETE("/system/post/:id", m.wrapPermission("system:post:delete", admin.DeletePost))
	engine.PUT("/system/post/:id/status", m.wrapPermission("system:post:update", admin.ChangePostStatus))
	engine.GET("/system/post/:postId/roles", m.wrapPermission("system:post:role", admin.GetPostRoles))
	engine.POST("/system/post/:postId/roles", m.wrapPermission("system:post:role", admin.AssignPostRoles))
	engine.DELETE("/system/post/:postId/roles", m.wrapPermission("system:post:role", admin.RemovePostRoles))
}

func (m *Module) wrapPermission(permission string, handler app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			response.Error(reqCtx, apperrors.Unauthorized("未登录"))
			return
		}
		if !securitycontext.HasPermission(reqCtx, permission) {
			response.Error(reqCtx, apperrors.PermissionDenied(permission))
			return
		}
		handler(ctx, reqCtx)
	}
}

func (m *Module) wrapLogin(handler app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			response.Error(reqCtx, apperrors.Unauthorized("未登录"))
			return
		}
		handler(ctx, reqCtx)
	}
}

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}

type userStatusOperationEnricher struct{}

func (userStatusOperationEnricher) Enrich(_ context.Context, reqCtx *app.RequestContext, entry *adminfacade.OperationLogEntry) {
	if entry == nil || reqCtx == nil {
		return
	}
	switch strings.TrimSpace(string(reqCtx.Query("status"))) {
	case "0":
		entry.OperationType = adminfacade.OperationTypeAdminUnlockAccount
	case "1":
		entry.OperationType = adminfacade.OperationTypeAdminBanUser
	default:
		entry.OperationType = adminfacade.OperationTypeUserUpdateStatus
	}
	entry.OperationDesc = entry.OperationType.Description()
}

func (m *Module) SubjectFacade() userfacade.SubjectFacade {
	return m.facades.Subjects
}

func (m *Module) ProfileFacade() userfacade.ProfileFacade {
	return m.facades.Profiles
}

func (m *Module) AccountFacade() userfacade.AccountFacade {
	return m.facades.Accounts
}

func (m *Module) ProvisioningFacade() userfacade.ProvisioningFacade {
	return m.facades.Provisioning
}

func (m *Module) AdminUserFacade() userfacade.AdminUserFacade { return m.facades.AdminUsers }
func (m *Module) OrgFacade() userfacade.OrgFacade             { return m.facades.Orgs }
func (m *Module) DeptFacade() userfacade.DeptFacade           { return m.facades.Depts }
func (m *Module) PostFacade() userfacade.PostFacade           { return m.facades.Posts }
