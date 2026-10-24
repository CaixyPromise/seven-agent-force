package platform

import (
	"context"
	"fmt"
	"strings"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	platformapp "github.com/CaixyPromise/seven-agent/force/internal/app/platform/application"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	platformhandler "github.com/CaixyPromise/seven-agent/force/internal/app/platform/handler"
	platforminfra "github.com/CaixyPromise/seven-agent/force/internal/app/platform/infrastructure"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	service *platformapp.Service
	handler *platformhandler.Handler
}

type ControlPlane struct {
	handler *platformhandler.Handler
	oplog   adminfacade.OperationLogger
}

type Dependencies struct {
	AuthorizationSessions ssofacade.AuthorizationSessionFacade
	Sessions              ssofacade.SessionFacade
}

func InstallPolicyCore(deps bootstrapruntime.ModuleDeps, moduleDeps ...Dependencies) (*Module, platformfacade.PublicFacade, error) {
	if deps.Infra.Datasource == nil {
		return nil, nil, fmt.Errorf("platform module requires datasource provider")
	}
	repository, err := platforminfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, nil, err
	}
	var dependencies Dependencies
	if len(moduleDeps) > 0 {
		dependencies = moduleDeps[0]
	}
	service := platformapp.NewService(repository, deps.Infra.CacheMgr, deps.IDGen, deps.Infra.Transactor)
	service.BindAuthorizationSessions(dependencies.AuthorizationSessions)
	service.BindSessions(dependencies.Sessions)
	module := &Module{
		service: service,
		handler: platformhandler.NewHandler(service),
	}
	return module, service, nil
}

func MountControlPlane(policyCore *Module) *ControlPlane {
	if policyCore == nil || policyCore.handler == nil {
		return nil
	}
	return &ControlPlane{handler: policyCore.handler}
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "platform", Prefix: "/platform"}
}

func (m *Module) Mount(engine route.IRouter) {
	m.MountPublic(engine)
}

func (m *Module) MountPublic(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/platform/public/login-options", m.handler.ResolveLoginOptions)
	engine.GET("/platform/login-options", m.handler.ResolveLoginOptions)
}

func (m *Module) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if m == nil || m.handler == nil {
		return
	}
	m.handler.BindAuthorization(auth)
}

func (m *ControlPlane) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "platform-control-plane", Prefix: "/platform/admin"}
}

func (m *ControlPlane) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	m.mountAdminRoutes(engine, "/platform/admin")
	m.mountAdminRoutes(engine, "/platform/admin/platforms")
}

func (m *ControlPlane) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m == nil {
		return
	}
	m.oplog = oplog
}

func (m *ControlPlane) mountAdminRoutes(engine route.IRouter, base string) {
	listPath := base + "/page"
	if strings.HasSuffix(base, "/platforms") {
		listPath = base
	}
	engine.GET(listPath, m.wrapPermission("system:platform:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询平台列表",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListPlatforms)))
	engine.GET(base+"/:platformCode", m.wrapPermission("system:platform:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询平台详情",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.GetPlatform)))
	engine.POST(base, m.wrapPermission("system:platform:add", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "创建平台",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.CreatePlatform)))
	engine.PUT(base+"/:platformCode", m.wrapPermission("system:platform:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "编辑平台",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdatePlatform)))
	engine.PUT(base+"/:platformCode/status", m.wrapPermission("system:platform:status", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "启停平台",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdatePlatformStatus)))
	engine.PUT(base+"/:platformCode/login-methods", m.wrapPermission("system:platform:login-method:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "编辑平台登录方式",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ReplaceLoginMethods)))
	engine.PUT(base+"/:platformCode/source-rules", m.wrapPermission("system:platform:source-rule:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "编辑平台来源规则",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ReplaceSourceRules)))
	engine.PUT(base+"/:platformCode/default-roles", m.wrapPermission("system:platform:default-role:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "编辑平台默认角色",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ReplaceDefaultRoles)))
	engine.GET(base+"/source/resolve", m.wrapPermission("system:platform:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "预览平台来源解析",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ResolveSource)))
}

func (m *ControlPlane) wrapPermission(permission string, handler app.HandlerFunc) app.HandlerFunc {
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

func (m *ControlPlane) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}
