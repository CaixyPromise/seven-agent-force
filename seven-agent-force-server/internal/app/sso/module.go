package sso

import (
	"context"
	"fmt"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	ssoapp "github.com/CaixyPromise/seven-agent/force/internal/app/sso/application"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	ssohandler "github.com/CaixyPromise/seven-agent/force/internal/app/sso/handler"
	ssoinfra "github.com/CaixyPromise/seven-agent/force/internal/app/sso/infrastructure"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Facades struct {
	AuthorizationSessions    ssofacade.AuthorizationSessionFacade
	AuthenticationCompletion ssofacade.AuthenticationCompletionFacade
	Bootstrap                ssofacade.BootstrapSessionFacade
	Sessions                 ssofacade.SessionFacade
	Tokens                   ssofacade.TokenFacade
	AuditEvents              ssofacade.AuditEventQueryFacade
	Clients                  ssofacade.ClientQueryFacade
	ManagedClients           ssofacade.ManagedClientFacade
}

type Dependencies struct {
	Profiles userfacade.ProfileFacade
	Subjects userfacade.SubjectFacade
}

type Module struct {
	service *ssoapp.Service
	handler *ssohandler.Handler
	facades Facades
	oplog   adminfacade.OperationLogger
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, Facades, error) {
	if !deps.Config.SSO.Enabled {
		return nil, Facades{}, nil
	}
	if deps.Infra.Datasource == nil {
		return nil, Facades{}, fmt.Errorf("sso module requires datasource provider")
	}
	if deps.Infra.CacheMgr == nil {
		return nil, Facades{}, fmt.Errorf("sso module requires cache manager")
	}
	if deps.Security.Password == nil {
		return nil, Facades{}, fmt.Errorf("sso module requires password service")
	}
	if refs.Profiles == nil || refs.Subjects == nil {
		return nil, Facades{}, fmt.Errorf("sso module requires system user facades")
	}
	repository, err := ssoinfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, Facades{}, err
	}
	jwtService, err := ssoinfra.BuildJWTService(deps.Config.SSO)
	if err != nil {
		return nil, Facades{}, err
	}
	cacheService := ssoinfra.NewAuthSessionCache(deps.Infra.CacheMgr)
	service := ssoapp.NewService(
		deps.Config.SSO,
		deps.IDGen,
		repository,
		cacheService,
		jwtService,
		deps.Security.Password,
		refs.Profiles,
		refs.Subjects,
	)
	service.BindTransactor(deps.Infra.Transactor)
	module := &Module{
		service: service,
		handler: ssohandler.NewHandler(service, ssohandler.ConfigView{
			Enabled:                 deps.Config.SSO.Enabled,
			FrontendPrimaryEnabled:  deps.Config.SSO.FrontendPrimaryEnabled,
			ResourceServerEnabled:   deps.Config.SSO.ResourceServerEnabled,
			Issuer:                  deps.Config.SSO.Issuer,
			BaseURL:                 deps.Config.SSO.BaseURL,
			FrontendLoginURL:        deps.Config.SSO.FrontendLoginURL,
			DefaultFirstPartyClient: deps.Config.SSO.DefaultFirstPartyClientID,
			SessionCookieName:       deps.Config.SSO.SessionCookie.Name,
			RefreshCookieName:       deps.Config.SSO.RefreshCookie.Name,
			RefreshCookieSecure:     deps.Config.SSO.RefreshCookie.Secure,
			TokenRateLimit:          deps.Config.SSO.RateLimit.TokenLimit,
			TokenRateLimitWindow:    deps.Config.SSO.RateLimit.TokenWindow,
			UserInfoRateLimit:       deps.Config.SSO.RateLimit.UserInfoLimit,
			UserInfoRateLimitWindow: deps.Config.SSO.RateLimit.UserInfoWindow,
			RateLimitFailClosed:     deps.Config.SSO.RateLimit.FailClosedOnError,
		}),
	}
	module.handler.BindLimiter(deps.Infra.Limiter)
	module.facades = Facades{
		AuthorizationSessions:    service,
		AuthenticationCompletion: service,
		Bootstrap:                service,
		Sessions:                 service,
		Tokens:                   service,
		AuditEvents:              service,
		Clients:                  service,
		ManagedClients:           service,
	}
	return module, module.facades, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "sso", Prefix: "/sso"}
}

func (m *Module) Mount(engine route.IRouter) {
	m.MountPublic(engine)
}

func (m *Module) MountPublic(engine route.IRouter) {
	if engine == nil || m.handler == nil {
		return
	}
	engine.GET("/sso/runtime/config", m.handler.RuntimeConfig)
	engine.GET("/.well-known/openid-configuration", m.handler.Discovery)
	engine.GET("/.well-known/jwks.json", m.handler.JWKS)
	engine.GET("/sso/.well-known/openid-configuration", m.handler.Discovery)
	engine.GET("/sso/.well-known/jwks.json", m.handler.JWKS)
	engine.GET("/sso/oauth2/authorize", m.handler.Authorize)
	engine.GET("/sso/oauth2/authorize/login", m.handler.AuthorizeLogin)
	engine.POST("/sso/oauth2/token", m.handler.Token)
	engine.GET("/sso/oauth2/userinfo", m.handler.UserInfo)
	engine.POST("/sso/oauth2/userinfo", m.handler.UserInfo)
	engine.POST("/sso/oauth2/revoke", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "OAuth 令牌撤销",
		IncludeParams: true,
	}, m.handler.Revoke))
	engine.POST("/sso/oauth2/introspect", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "OAuth 令牌自省",
		IncludeParams: true,
	}, m.handler.Introspect))
	engine.POST("/sso/logout", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogout,
		Description:   "用户登出",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.Logout))
	engine.GET("/internal/sso/access-tokens/validate", m.handler.InternalValidate)
	engine.GET("/sso/sessions", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询当前用户会话列表",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListSessions))
	engine.DELETE("/sso/sessions/:sessionId", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogout,
		Description:   "删除会话",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.DeleteSession))
	engine.GET("/sso/devices", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询当前用户设备列表",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListDevices))
	engine.DELETE("/sso/devices/:deviceId", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogout,
		Description:   "删除设备会话",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.DeleteDevice))
	engine.POST("/sso/logout-all", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogout,
		Description:   "全部登出",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.LogoutAll))
	engine.GET("/sso/admin/users/:userId/sessions", m.wrapPermission("admin:sso:session:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "管理员查询用户会话列表",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.AdminListUserSessions)))
	engine.POST("/sso/admin/users/:userId/sessions/:sessionId/kick", m.wrapPermission("admin:sso:session:kick", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogout,
		Description:   "管理员踢出用户会话",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.AdminKickUserSession)))
	engine.POST("/sso/admin/users/:userId/logout-all", m.wrapPermission("admin:sso:session:kick", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogout,
		Description:   "管理员踢出用户全部会话",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.AdminLogoutAllUserSessions)))
	engine.GET("/sso/admin/users/:userId/devices", m.wrapPermission("admin:sso:device:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "管理员查询用户设备列表",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.AdminListUserDevices)))
	engine.POST("/sso/admin/users/:userId/devices/:deviceId/kick", m.wrapPermission("admin:sso:device:kick", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogout,
		Description:   "管理员踢出用户设备",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.AdminKickUserDevice)))
	engine.GET("/sso/admin/client-capabilities", m.wrapPermission("system:sso-client:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询SSO客户端能力",
		IncludeResult: true,
	}, m.handler.ClientCapabilities)))
	engine.GET("/sso/admin/clients", m.wrapPermission("system:sso-client:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询SSO客户端列表",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListClients)))
	engine.POST("/sso/admin/clients", m.wrapPermission("system:sso-client:add", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "新增SSO客户端",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.CreateClient)))
	engine.GET("/sso/admin/clients/:clientId", m.wrapPermission("system:sso-client:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询SSO客户端详情",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.GetClient)))
	engine.PUT("/sso/admin/clients/:clientId", m.wrapPermission("system:sso-client:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "更新SSO客户端",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdateClient)))
	engine.PUT("/sso/admin/clients/:clientId/status", m.wrapPermission("system:sso-client:status", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "修改SSO客户端状态",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdateClientStatus)))
	engine.GET("/sso/admin/clients/:clientId/redirect-uris", m.wrapPermission("system:sso-client:redirect:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询SSO客户端回调地址",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListClientRedirectURIs)))
	engine.PUT("/sso/admin/clients/:clientId/redirect-uris", m.wrapPermission("system:sso-client:redirect:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "更新SSO客户端回调地址",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdateClientRedirectURIs)))
	engine.GET("/sso/admin/clients/:clientId/secrets", m.wrapPermission("system:sso-client:secret:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询SSO客户端密钥",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListClientSecrets)))
	engine.POST("/sso/admin/clients/:clientId/secrets", m.wrapPermission("system:sso-client:secret:generate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "生成SSO客户端密钥",
		IncludeParams: true,
		IncludeResult: false,
	}, m.handler.GenerateClientSecret)))
	engine.PUT("/sso/admin/clients/:clientId/secrets/:secretId/status", m.wrapPermission("system:sso-client:secret:disable", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "禁用SSO客户端密钥",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.DisableClientSecret)))
}

func (m *Module) Facades() Facades {
	return m.facades
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m == nil {
		return
	}
	m.oplog = oplog
}

func (m *Module) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if m == nil || m.handler == nil {
		return
	}
	m.handler.BindAuthorization(auth)
}

// BindActiveSessionValidityCache is deliberately post-composition: no
// governance protocol means SSO remains authority-only.
func (m *Module) BindActiveSessionValidityCache(governed cacheinfra.TargetedGovernedCache) {
	if m == nil || m.service == nil || governed == nil {
		return
	}
	m.service.BindActiveSessionValidityCache(ssoinfra.NewActiveSessionValidityCache(governed))
}

func (m *Module) BindActiveSessionValidityInvalidations(registrar cachegovernancefacade.TargetedInvalidationRegistrar) {
	if m != nil && m.service != nil {
		m.service.BindActiveSessionValidityInvalidations(registrar)
	}
}

// BindActiveSessionValidityGovernance turns on candidate reads only when the
// matching durable registration protocol is present.
func (m *Module) BindActiveSessionValidityGovernance(registrar cachegovernancefacade.TargetedInvalidationRegistrar, governed cacheinfra.TargetedGovernedCache) {
	if m == nil || m.service == nil || registrar == nil || !registrar.Enabled() || governed == nil {
		return
	}
	m.service.BindActiveSessionValidityInvalidations(registrar)
	m.service.BindActiveSessionValidityCache(ssoinfra.NewActiveSessionValidityCache(governed))
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

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}
