package file

import (
	"context"
	"fmt"
	"strings"

	fileapp "github.com/CaixyPromise/seven-agent/force/internal/app/file/application"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	filehandler "github.com/CaixyPromise/seven-agent/force/internal/app/file/handler"
	fileinfra "github.com/CaixyPromise/seven-agent/force/internal/app/file/infrastructure"
	filejob "github.com/CaixyPromise/seven-agent/force/internal/app/file/job"
	filelistener "github.com/CaixyPromise/seven-agent/force/internal/app/file/listener"
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
	service *fileapp.Service
	handler *filehandler.Handler
	oplog   adminfacade.OperationLogger
	facades filefacade.Facades
	cancel  context.CancelFunc
}

func Install(deps bootstrapruntime.ModuleDeps) (*Module, filefacade.Facades, error) {
	if !deps.Config.File.Enabled {
		return nil, filefacade.Facades{}, nil
	}
	if deps.Infra.Datasource == nil {
		return nil, filefacade.Facades{}, fmt.Errorf("file module requires datasource provider")
	}
	repository, err := fileinfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, filefacade.Facades{}, err
	}
	storage, err := fileinfra.NewStorageRouter(deps.Config.Storage)
	if err != nil {
		return nil, filefacade.Facades{}, err
	}
	distribution := deps.Config.File.Distribution
	distribution.GatewayPath = withContextPath(deps.Config.ContextPath(), distribution.GatewayPath)
	tokenService, err := fileinfra.NewDownloadTokenService(distribution, deps.Infra.CacheMgr)
	if err != nil {
		return nil, filefacade.Facades{}, err
	}
	outboxStore := fileinfra.NewOutboxStore(deps.Infra.Datasource.SQLX())
	var rabbit *fileinfra.RabbitMQ
	if deps.Config.RabbitMQ.Enabled && deps.Config.File.Rabbit.Enabled {
		rabbit, err = fileinfra.NewRabbitMQ(deps.Infra.RabbitMQ, deps.Config.RabbitMQ.Declare)
		if err != nil {
			return nil, filefacade.Facades{}, fmt.Errorf("file module rabbitmq bootstrap failed: %w", err)
		}
	} else {
		rabbit = fileinfra.NewDisabledRabbitMQ()
	}
	service := fileapp.NewService(deps.Infra.Transactor, repository, outboxStore, storage, tokenService, rabbit, deps.IDGen, distribution, deps.Config.File.Outbox.Enabled)
	if deps.Infra.Jobs != nil && deps.Config.File.Outbox.Enabled {
		if err := deps.Infra.Jobs.Register(filejob.NewOutboxRelayJob(service, deps.Config.File.Outbox.RelayIntervalMS, deps.Config.File.Outbox.BatchSize)); err != nil {
			return nil, filefacade.Facades{}, err
		}
	}
	if deps.Infra.Jobs != nil {
		if err := deps.Infra.Jobs.Register(filejob.NewStorageHealthJob(service, deps.Config.File.HealthCheck.IntervalMS)); err != nil {
			return nil, filefacade.Facades{}, err
		}
		if err := deps.Infra.Jobs.Register(filejob.NewProcessRetryJob(service, deps.Config.File.ProcessTask.RetryIntervalMS, deps.Config.File.ProcessTask.RetryBatchSize)); err != nil {
			return nil, filefacade.Facades{}, err
		}
		if err := deps.Infra.Jobs.Register(filejob.NewBindingRetryJob(service, deps.Config.File.Binding.RetryDelaySeconds*1000, deps.Config.File.Binding.RetryBatchSize)); err != nil {
			return nil, filefacade.Facades{}, err
		}
		if err := deps.Infra.Jobs.Register(filejob.NewChunkCleanupJob(service, deps.Config.File.Cleanup.ChunkCleanupIntervalMS, deps.Config.File.Cleanup.BatchSize)); err != nil {
			return nil, filefacade.Facades{}, err
		}
		if err := deps.Infra.Jobs.Register(filejob.NewUploadTaskCleanupJob(service, deps.Config.File.Cleanup.ChunkCleanupIntervalMS, deps.Config.File.Cleanup.BatchSize)); err != nil {
			return nil, filefacade.Facades{}, err
		}
		if err := deps.Infra.Jobs.Register(filejob.NewFileCleanupJob(service, deps.Config.File.Cleanup.ChunkCleanupIntervalMS, deps.Config.File.Cleanup.BatchSize)); err != nil {
			return nil, filefacade.Facades{}, err
		}
		if err := deps.Infra.Jobs.Register(filejob.NewStorageStrategyDrainJob(service, deps.Config.File.HealthCheck.IntervalMS, 100)); err != nil {
			return nil, filefacade.Facades{}, err
		}
	}
	consumerCtx, cancelConsumers := context.WithCancel(context.Background())
	filelistener.StartRabbitConsumers(consumerCtx, rabbit, service)
	module := &Module{
		service: service,
		handler: filehandler.NewHandler(service),
		cancel:  cancelConsumers,
	}
	module.facades = filefacade.Facades{
		Assets:          service,
		ConfigAssets:    service,
		WorkforceInputs: service,
	}
	return module, module.facades, nil
}

func withContextPath(contextPath, routePath string) string {
	route := strings.TrimSpace(routePath)
	if route == "" {
		route = "/file/download"
	}
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	ctx := strings.TrimRight(strings.TrimSpace(contextPath), "/")
	if ctx == "" || ctx == "/" || strings.HasPrefix(route, ctx+"/") || route == ctx {
		return route
	}
	if !strings.HasPrefix(ctx, "/") {
		ctx = "/" + ctx
	}
	return ctx + route
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "file", Prefix: "/file"}
}

func (m *Module) Shutdown(ctx context.Context) error {
	if m != nil && m.cancel != nil {
		m.cancel()
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/file/check", m.wrapLogin(m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "检查文件秒传", IncludeParams: true}, m.handler.Check)))
	engine.POST("/file/upload/faster", m.wrapLogin(m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeFileUpload, Description: "文件秒传", IncludeParams: true}, m.handler.FasterUpload)))
	engine.POST("/file/upload", m.wrapLogin(m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeFileUpload, Description: "文件上传", IncludeParams: false}, m.handler.Upload)))
	engine.GET("/file/download", m.handler.Download)

	engine.POST("/uploads/init", m.wrapLogin(m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeFileUpload, Description: "初始化直传任务", IncludeParams: true}, m.handler.InitUpload)))
	engine.POST("/uploads/instant/confirm", m.wrapLogin(m.handler.InstantConfirm))
	engine.POST("/uploads/complete", m.wrapLogin(m.handler.CompleteUpload))
	engine.POST("/uploads/callback", m.handler.Callback)
	engine.GET("/uploads/:taskId/status", m.wrapLogin(m.handler.UploadStatus))
	engine.GET("/uploads/files/:fileId/download-url", m.wrapLogin(m.handler.DownloadURL))

	engine.POST("/chunk-upload/init", m.wrapLogin(m.handler.ChunkInit))
	engine.POST("/chunk-upload/part", m.wrapLogin(m.handler.ChunkPart))
	engine.POST("/chunk-upload/complete", m.wrapLogin(m.handler.ChunkComplete))
	engine.POST("/chunk-upload/abort", m.wrapLogin(m.handler.ChunkAbort))
	engine.GET("/chunk-upload/status", m.wrapLogin(m.handler.ChunkStatus))
	engine.GET("/chunk-upload/active", m.wrapLogin(m.handler.ChunkActive))
	engine.GET("/chunk-upload/resume-info", m.wrapLogin(m.handler.ChunkResumeInfo))

	engine.GET("/file-manage/list", m.wrapPermission("system:file:list", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "查询文件列表", IncludeParams: true}, m.handler.FileManageList)))
	engine.GET("/file-manage/:id", m.wrapPermission("system:file:query", m.handler.FileDetail))
	engine.GET("/file-manage/:id/references", m.wrapPermission("system:file:query", m.handler.FileReferences))
	engine.POST("/file-manage/references/:id/access-level", m.wrapPermission("system:file:edit", m.handler.UpdateReferenceAccess))
	engine.PUT("/file-manage/references/:id/access-level", m.wrapPermission("system:file:edit", m.handler.UpdateReferenceAccess))
	engine.GET("/file-manage/stats", m.wrapPermission("system:file:list", m.handler.FileStats))
	engine.POST("/file-manage/:id/delete", m.wrapPermission("system:file:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeFileDelete, Description: "删除文件", IncludeParams: true}, m.handler.DeleteFile)))
	engine.DELETE("/file-manage/:id", m.wrapPermission("system:file:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeFileDelete, Description: "删除文件", IncludeParams: true}, m.handler.DeleteFile)))
	engine.POST("/file-manage/batch-delete", m.wrapPermission("system:file:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeFileDelete, Description: "批量删除文件", IncludeParams: true}, m.handler.BatchDeleteFiles)))
	engine.DELETE("/file-manage/batch", m.wrapPermission("system:file:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeFileDelete, Description: "批量删除文件", IncludeParams: true}, m.handler.BatchDeleteFiles)))

	engine.GET("/storage-strategy", m.wrapPermission("system:storage:list", m.handler.StorageStrategies))
	engine.POST("/storage-strategy", m.wrapPermission("system:storage:add", m.handler.CreateStorageStrategy))
	engine.POST("/storage-strategy/update", m.wrapPermission("system:storage:edit", m.handler.UpdateStorageStrategy))
	engine.GET("/storage-strategy/:id", m.wrapPermission("system:storage:list", m.handler.StorageStrategy))
	engine.PUT("/storage-strategy/:id", m.wrapPermission("system:storage:edit", m.handler.UpdateStorageStrategyByPath))
	engine.POST("/storage-strategy/:id/delete", m.wrapPermission("system:storage:delete", m.handler.DeleteStorageStrategy))
	engine.DELETE("/storage-strategy/:id", m.wrapPermission("system:storage:delete", m.handler.DeleteStorageStrategy))
	engine.POST("/storage-strategy/:id/default", m.wrapPermission("system:storage:edit", m.handler.SetDefaultStrategy))
	engine.PUT("/storage-strategy/:id/default", m.wrapPermission("system:storage:edit", m.handler.SetDefaultStrategy))
	engine.POST("/storage-strategy/:id/enable", m.wrapPermission("system:storage:edit", m.handler.EnableStrategy))
	engine.PUT("/storage-strategy/:id/enable", m.wrapPermission("system:storage:edit", m.handler.EnableStrategy))
	engine.GET("/storage-strategy/:id/health", m.wrapPermission("system:storage:list", m.handler.StorageHealth))

	engine.GET("/file-process-task", m.wrapPermission("system:file-task:list", m.handler.ProcessTaskPage))
	engine.GET("/file-process-task/:id", m.wrapPermission("system:file-task:list", m.handler.ProcessTaskDetail))
	engine.POST("/file-process-task/:id/retry", m.wrapPermission("system:file-task:retry", m.handler.RetryProcessTask))
	engine.POST("/file-process-task/:id/replay", m.wrapPermission("system:file-task:retry", m.handler.RetryProcessTask))
	engine.POST("/file-process-task/batch-retry", m.wrapPermission("system:file-task:retry", m.handler.BatchRetryProcessTasks))
	engine.GET("/file-process-task/stats", m.wrapPermission("system:file-task:list", m.handler.ProcessTaskStats))
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m != nil {
		m.oplog = oplog
	}
}

func (m *Module) Facades() filefacade.Facades {
	if m == nil {
		return filefacade.Facades{}
	}
	return m.facades
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

func (m *Module) wrapPermission(permission string, handler app.HandlerFunc) app.HandlerFunc {
	return m.wrapPermissionAny([]string{permission}, handler)
}

func (m *Module) wrapPermissionAny(permissions []string, handler app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			response.Error(reqCtx, apperrors.Unauthorized("未登录"))
			return
		}
		allowed := false
		for _, permission := range permissions {
			if securitycontext.HasPermission(reqCtx, permission) {
				allowed = true
				break
			}
		}
		if !allowed {
			response.Error(reqCtx, apperrors.PermissionDenied(strings.Join(permissions, "|")))
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
