package dict

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	dictapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	dictfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/facade"
	dicthandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/handler"
	dictinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/infrastructure"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/bytedance/sonic"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	service *dictapp.Service
	handler *dicthandler.Handler
	oplog   adminfacade.OperationLogger
}

type Dependencies struct {
	CacheInvalidations cachegovernancefacade.InvalidationRegistrar
}

func Install(deps bootstrapruntime.ModuleDeps, options ...Dependencies) (*Module, dictfacade.DictFacade, error) {
	if deps.Infra.Datasource == nil {
		return nil, nil, fmt.Errorf("system dict module requires datasource provider")
	}
	repository, err := dictinfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, nil, err
	}
	dependencies := Dependencies{}
	if len(options) > 0 {
		dependencies = options[0]
	}
	cacheStore := dictinfra.NewCacheStore(deps.Infra.CacheMgr, dependencies.CacheInvalidations != nil && dependencies.CacheInvalidations.Enabled())
	service := dictapp.NewService(deps.Infra.Transactor, repository, cacheStore, domain.NewService(), dependencies.CacheInvalidations)
	module := &Module{
		service: service,
		handler: dicthandler.NewHandler(service, service),
	}
	return module, service, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "system-dict", Prefix: "/dict"}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.POST("/dict-type/add", m.wrapPermission("system:dict:add", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "新增字典类型", IncludeParams: true}, m.handler.AddDictType)))
	engine.POST("/dict-type/update", m.wrapPermission("system:dict:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "更新字典类型", IncludeParams: true}, m.handler.UpdateDictType)))
	engine.POST("/dict-type/delete", m.wrapPermission("system:dict:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "删除字典类型", IncludeParams: true}, m.handler.DeleteDictType)))
	engine.GET("/dict-type/:id", m.wrapPermission("system:dict:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询字典类型详情"}, m.handler.GetDictTypeByID)))
	engine.GET("/dict-type/types", m.wrapPermission("system:dict:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "分页查询字典类型", IncludeParams: true}, m.handler.GetDictTypePage)))
	engine.POST("/dict-type/status", m.wrapPermission("system:dict:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "修改字典类型状态", IncludeParams: true}, m.handler.ChangeDictTypeStatus)))
	engine.POST("/dict-type/:id/move", m.wrapPermission("system:dict:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "移动字典类型", IncludeParams: true}, m.handler.MoveDictType)))

	engine.POST("/dict/:typeId/items", m.wrapPermission("system:dict:add", m.wrapOperation(dictItemMutationSpec("新增字典项"), m.handler.AddDictItem)))
	engine.POST("/dict/items/update", m.wrapPermission("system:dict:edit", m.wrapOperation(dictItemMutationSpec("更新字典项"), m.handler.UpdateDictItem)))
	engine.POST("/dict/items/delete", m.wrapPermission("system:dict:delete", m.wrapOperation(dictItemMutationSpec("删除字典项"), m.handler.DeleteDictItem)))
	engine.POST("/dict/items/status", m.wrapPermission("system:dict:edit", m.wrapOperation(dictItemMutationSpec("修改字典项状态"), m.handler.ChangeDictItemStatus)))
	engine.GET("/dict/:typeId/items", m.wrapPermission("system:dict:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询字典项列表", IncludeParams: true}, m.handler.GetDictItemList)))
	engine.POST("/dict/items/sort", m.wrapPermission("system:dict:edit", m.wrapOperation(dictItemMutationSpec("批量排序字典项"), m.handler.BatchUpdateSort)))
	engine.POST("/dict/batch", m.wrapPermission("system:dict:query", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "批量查询字典", IncludeParams: true}, m.handler.BatchGetDict)))
	engine.POST("/dict/:typeId/items/:itemId/move", m.wrapPermission("system:dict:edit", m.wrapOperation(dictItemMutationSpec("移动字典项"), m.handler.MoveDictItem)))

	engine.GET("/dict-client/:dictCode", m.handler.GetDictByCodeForClient)
	engine.POST("/dict-client/batch", m.handler.BatchGetDictForClient)
}

func dictItemMutationSpec(description string) adminfacade.OperationLogSpec {
	return adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   description,
		IncludeParams: false,
		Enrichers:     []adminfacade.OperationLogEnricher{safeDictMutationAuditEnricher{}},
	}
}

type safeDictMutationAuditEnricher struct{}

func (safeDictMutationAuditEnricher) Enrich(_ context.Context, reqCtx *app.RequestContext, entry *adminfacade.OperationLogEntry) {
	if reqCtx == nil || entry == nil {
		return
	}
	body := reqCtx.Request.Body()
	sum := sha256.Sum256(body)
	payload := map[string]any{
		"kind":          "sensitive_dict_mutation",
		"contentLength": len(body),
		"bodySha256":    hex.EncodeToString(sum[:]),
		"requestMethod": strings.TrimSpace(string(reqCtx.Method())),
		"requestPath":   strings.TrimSpace(string(reqCtx.Path())),
	}
	encoded, err := sonic.Marshal(payload)
	if err == nil {
		entry.RequestParams = string(encoded)
	}
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
