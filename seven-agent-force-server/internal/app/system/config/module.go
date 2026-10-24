package config

import (
	"context"
	"fmt"
	"log"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	configapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	confighandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/handler"
	configinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/infrastructure"
	systemuser "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Dependencies struct {
	Profiles           systemuser.ProfileFacade
	CacheInvalidations cachegovernancefacade.InvalidationRegistrar
}

type Module struct {
	service *configapp.Service
	handler *confighandler.Handler
	oplog   adminfacade.OperationLogger
}

func Install(deps bootstrapruntime.ModuleDeps, wired Dependencies) (*Module, configfacade.ConfigFacade, error) {
	if deps.Infra.Datasource == nil {
		return nil, nil, fmt.Errorf("system config module requires datasource provider")
	}
	repository, err := configinfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, nil, err
	}
	cacheStore := configinfra.NewCacheStore(deps.Infra.CacheMgr, wired.CacheInvalidations != nil && wired.CacheInvalidations.Enabled())
	secretCipher := configinfra.NewSecretCipher(deps.Security.SecretValue)
	revealCipher := configinfra.NewSensitiveRevealCipher()
	userLookup := &profileLookup{profiles: wired.Profiles}
	service := configapp.NewService(
		deps.Infra.Transactor,
		repository,
		cacheStore,
		domain.NewService(),
		secretCipher,
		revealCipher,
		userLookup,
		wired.CacheInvalidations,
	)
	module := &Module{
		service: service,
		handler: confighandler.NewHandler(service, service),
	}
	if deps.Infra.Transactor != nil && deps.Infra.Transactor.Enabled() {
		if _, err := service.ApplyPendingConfigs(context.Background(), configapp.Actor{}, true); err != nil {
			log.Printf("system-config startup apply pending failed: %v", err)
		}
	}
	return module, service, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "system-config", Prefix: "/config"}
}

func (m *Module) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if m == nil || m.handler == nil {
		return
	}
	m.handler.BindAuthorization(auth)
}

func (m *Module) BindRoleSecurity(roles authorizationfacade.RoleFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindRoleSecurity(roles)
}

// BindConfigAssets completes the one-way module composition after the file
// module is installed. System-config only sees the narrow file facade; it
// cannot reach the file application's repositories or HTTP handlers.
func (m *Module) BindConfigAssets(assets filefacade.ConfigAssetFacade) {
	if m == nil || m.service == nil {
		return
	}
	m.service.BindConfigAssets(assets)
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.POST("/config-groups", m.wrapPermission("system:config:group:add", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigGroupCreate, Description: "新增配置分组", IncludeParams: true}, m.handler.AddConfigGroup)))
	engine.POST("/config-groups/update", m.wrapPermission("system:config:group:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigGroupUpdate, Description: "更新配置分组", IncludeParams: true}, m.handler.UpdateConfigGroup)))
	engine.POST("/config-groups/delete", m.wrapPermission("system:config:group:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigGroupDelete, Description: "删除配置分组", IncludeParams: true}, m.handler.DeleteConfigGroup)))
	engine.GET("/config-groups/page", m.wrapPermission("system:config:group:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分页查询配置分组", IncludeParams: true}, m.handler.GetConfigGroupPage)))
	engine.GET("/config-groups/:id", m.wrapPermission("system:config:group:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询配置分组详情"}, m.handler.GetConfigGroupByID)))
	engine.POST("/config-groups/:id/move", m.wrapPermission("system:config:group:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigGroupUpdate, Description: "移动配置分组", IncludeParams: true}, m.handler.MoveConfigGroup)))

	engine.POST("/config", m.wrapPermission("system:config:add", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigCreate, Description: "新增配置项", IncludeParams: true}, m.handler.AddConfig)))
	engine.POST("/config/update", m.wrapPermission("system:config:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "更新配置项", IncludeParams: true}, m.handler.UpdateConfig)))
	engine.POST("/config/delete", m.wrapPermission("system:config:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigDelete, Description: "删除配置项", IncludeParams: true}, m.handler.DeleteConfig)))
	engine.GET("/config/:id", m.wrapPermission("system:config:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询配置详情"}, m.handler.GetConfigByID)))
	engine.GET("/config", m.wrapPermission("system:config:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分页查询配置项", IncludeParams: true}, m.handler.GetConfigPage)))
	engine.POST("/config/enabled", m.wrapPermission("system:config:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigEnabledChange, Description: "修改配置启用状态", IncludeParams: true}, m.handler.ChangeEnabled)))
	engine.POST("/config/:id/sensitive/reveal", m.wrapPermission("system:config:sensitive", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "查看敏感配置明文", IncludeParams: true}, m.handler.RevealSensitiveValue)))
	engine.POST("/config/apply-pending", m.wrapPermission("system:config:apply", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigApply, Description: "应用待生效配置", IncludeParams: true}, m.handler.ApplyPendingConfigs)))
	engine.GET("/config/pending", m.wrapPermission("system:config:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询待生效配置", IncludeParams: true}, m.handler.GetPendingConfigs)))
	engine.GET("/config/:configId/history", m.wrapPermission("system:config:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询配置变更历史"}, m.handler.GetConfigChangeHistory)))
	engine.POST("/config/rollback", m.wrapPermission("system:config:rollback", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigRollback, Description: "回滚配置变更", IncludeParams: true}, m.handler.RollbackConfigChange)))
	engine.GET("/config/operation-chain/:logId", m.wrapPermission("system:config:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询配置操作链"}, m.handler.GetOperationChain)))
	engine.GET("/config/audit-logs", m.wrapPermission("system:config:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询配置审计日志", IncludeParams: true}, m.handler.GetAuditLogs)))
	engine.GET("/config-scopes/roles/:roleId", m.wrapPermission("system:config:scope:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询角色配置范围", IncludeParams: true}, m.handler.GetRoleConfigScopes)))
	engine.POST("/config-scopes/roles/:roleId", m.wrapPermission("system:config:scope:assign", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分配角色配置范围", IncludeParams: true}, m.handler.AssignRoleConfigScopes)))

	engine.GET("/config-client", m.handler.ListConfigsForClient)
	engine.GET("/config-client/:configKey", m.handler.GetConfigByKeyForClient)
	engine.POST("/config-client/batch", m.handler.GetConfigBatchForClient)
	engine.GET("/config-assets/:id", m.handler.OpenConfigAsset)
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

type profileLookup struct {
	profiles systemuser.ProfileFacade
}

func (l *profileLookup) FindNicknames(ctx context.Context, userIDs []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(userIDs))
	if l == nil || l.profiles == nil {
		return result, nil
	}
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		profile, err := l.profiles.GetProfileByUserID(ctx, userID)
		if err != nil {
			return result, err
		}
		if profile == nil {
			continue
		}
		result[userID] = profile.NickName
	}
	return result, nil
}
