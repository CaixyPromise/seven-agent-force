package admin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	adminhandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/handler"
	admininfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/infrastructure"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Dependencies struct {
	Subjects userfacade.SubjectFacade
	Accounts userfacade.AccountFacade
	Auth     authorizationfacade.AuthFacade
	Sessions ssofacade.SessionFacade
}

type Facades struct {
	LoginFailures adminfacade.LoginFailureFacade
	OnlineUsers   adminfacade.OnlineUserFacade
	OperationLogs adminfacade.OperationLogFacade
	RuntimeLogs   adminfacade.RuntimeLogFacade
	Operation     adminfacade.OperationLogger
}

type Module struct {
	service   *adminapp.Service
	handler   *adminhandler.Handler
	operation adminfacade.OperationLogger
	facades   Facades
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, Facades, error) {
	if deps.Infra.Datasource == nil {
		return nil, Facades{}, fmt.Errorf("system admin module requires datasource provider")
	}
	if deps.Infra.CacheMgr == nil {
		return nil, Facades{}, fmt.Errorf("system admin module requires cache manager")
	}
	if refs.Subjects == nil || refs.Accounts == nil || refs.Auth == nil || refs.Sessions == nil {
		return nil, Facades{}, fmt.Errorf("system admin module requires user, authorization, and sso facades")
	}

	repository, err := admininfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, Facades{}, err
	}
	onlineStore := admininfra.NewOnlineUserStateStore(deps.Infra.CacheMgr)
	loginFailureStore := admininfra.NewLoginFailureStateStore(deps.Infra.CacheMgr)
	runtimeLogConfig := deps.Config.Admin.RuntimeLog
	if strings.TrimSpace(runtimeLogConfig.ActiveFile) == "" {
		logPath := strings.TrimSpace(deps.Config.Logging.File.Path)
		if logPath != "" {
			runtimeLogConfig.BaseDir = filepath.Dir(logPath)
			runtimeLogConfig.ActiveFile = filepath.Base(logPath)
		}
	}
	runtimeLogs := admininfra.NewRuntimeLogProvider(
		runtimeLogConfig,
		deps.Config.Logging.Request.MaskedFields,
		deps.Config.Logging.Request.MaxFieldLength,
	)
	service := adminapp.NewService(
		adminapp.LoginSettings{
			CaptchaThreshold:     deps.Config.Login.CaptchaThreshold,
			LockThreshold:        deps.Config.Login.LockThreshold,
			ContextLockThreshold: deps.Config.Login.ContextLockThreshold,
			LockDurationHours:    deps.Config.Login.LockDurationHours,
		},
		refs.Subjects,
		refs.Accounts,
		refs.Auth,
		refs.Sessions,
		onlineStore,
		loginFailureStore,
		repository,
		admininfra.NewAsyncOperationLogWriter(repository),
		runtimeLogs,
		domain.NewService(),
	)
	operationLogger := admininfra.NewOperationLogger(service, deps.Config.Logging.Request)
	handler := adminhandler.NewHandler(service, service, service, service, deps.Infra.Docker)
	handler.BindAuthorization(refs.Auth)

	module := &Module{
		service:   service,
		handler:   handler,
		operation: operationLogger,
	}
	module.facades = Facades{
		LoginFailures: service,
		OnlineUsers:   service,
		OperationLogs: service,
		RuntimeLogs:   service,
		Operation:     operationLogger,
	}
	return module, module.facades, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "system-admin", Prefix: "/admin"}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/admin/online/users", m.wrapPermission("admin:online:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询在线用户列表",
		IncludeParams: true,
	}, m.handler.GetOnlineUsers)))
	engine.GET("/admin/online/count", m.wrapPermission("admin:online:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询在线用户数量",
	}, m.handler.GetOnlineUserCount)))
	engine.GET("/admin/online/stats", m.wrapPermission("admin:online:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询在线用户统计",
	}, m.handler.GetOnlineUserStats)))
	engine.GET("/admin/online/users/:userId", m.wrapPermission("admin:online:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询用户在线会话详情",
	}, m.handler.GetUserSession)))
	engine.POST("/admin/kick/:userId", m.wrapPermission("admin:online:kick", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeAdminForceLogout,
		Description:   "管理员强制下线用户",
		IncludeParams: true,
	}, m.handler.KickUser)))
	engine.POST("/admin/kick/batch", m.wrapPermission("admin:online:kick", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeAdminForceLogout,
		Description:   "管理员批量强制下线用户",
		IncludeParams: true,
	}, m.handler.BatchKickUsers)))
	engine.GET("/admin/online/check/:userId", m.wrapPermission("admin:online:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "检查用户在线状态",
	}, m.handler.CheckUserOnline)))

	engine.GET("/admin/logs/operation", m.wrapPermission("admin:log:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询操作日志列表",
		IncludeParams: true,
	}, m.handler.GetOperationLogs)))
	engine.GET("/admin/logs/operation/:id", m.wrapPermission("admin:log:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询操作日志详情",
	}, m.handler.GetOperationLogByID)))
	engine.POST("/admin/logs/operation/clean", m.wrapPermission("admin:log:clean", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeSystemCacheClear,
		Description:   "清理过期操作日志",
		IncludeParams: true,
	}, m.handler.CleanExpiredLogs)))
	engine.GET("/admin/logs/operation/types", m.wrapPermission("admin:log:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询操作日志类型",
	}, m.handler.GetOperationTypes)))
	engine.GET("/admin/logs/operation/export", m.wrapPermission("admin:log:export", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDataExport,
		Description:   "导出操作日志",
		IncludeParams: true,
	}, m.handler.ExportOperationLogs)))
	engine.POST("/admin/logs/operation/deleteByTimeRange", m.wrapPermission("admin:log:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeSystemLogClear,
		Description:   "按时间范围删除操作日志",
		IncludeParams: true,
	}, m.handler.DeleteLogsByTimeRange)))
	engine.GET("/admin/logs/operation/my", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询我的操作日志",
		IncludeParams: true,
	}, m.handler.GetMyOperationLogs))

	engine.GET("/admin/runtime-logs/page", m.wrapPermission("admin:runtime-log:view", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRuntimeLogQuery,
		Description:   "查询运行日志",
		IncludeParams: true,
	}, m.handler.RuntimeLogPage)))
	engine.GET("/admin/runtime-logs/stream", m.wrapPermission("admin:runtime-log:stream", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeRuntimeLogStreamSubscribe,
		Description:   "订阅运行日志",
		IncludeParams: true,
	}, m.handler.RuntimeLogStream)))

	if m.handler.DockerEnabled() {
		m.mountDocker(engine)
	}
}

func (m *Module) mountDocker(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/admin/docker/operations", m.wrapPermission("admin:docker:operation:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker 操作列表",
		IncludeParams: true,
	}, m.handler.GetDockerOperations)))
	engine.GET("/admin/docker/operations/latest", m.wrapPermission("admin:docker:operation:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询最近 Docker 操作",
		IncludeParams: true,
	}, m.handler.GetLatestDockerOperation)))
	engine.GET("/admin/docker/operations/:operationId/events", m.wrapPermission("admin:docker:operation:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker 操作事件",
		IncludeParams: true,
	}, m.handler.GetDockerOperationEvents)))
	engine.GET("/admin/docker/operations/:operationId/stream", m.wrapPermission("admin:docker:operation:stream", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "订阅 Docker 操作事件",
		IncludeParams: true,
	}, m.handler.StreamDockerOperation)))
	engine.POST("/admin/docker/operations/:operationId/cancel", m.wrapPermission("admin:docker:operation:cancel", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "取消 Docker 操作",
		IncludeParams: true,
	}, m.handler.CancelDockerOperation)))
	engine.POST("/admin/docker/operations/:operationId/retry", m.wrapPermission("admin:docker:operation:retry", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "重试 Docker 操作",
		IncludeParams: true,
	}, m.handler.RetryDockerOperation)))
	engine.GET("/admin/docker/operations/:operationId", m.wrapPermission("admin:docker:operation:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker 操作详情",
	}, m.handler.GetDockerOperation)))
	engine.POST("/admin/docker/operation-integrity/orphans/diagnose", m.wrapPermission("admin:docker:operation-integrity:diagnose", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "诊断 Docker 操作事件孤儿",
		IncludeParams: true,
	}, m.handler.DiagnoseDockerOperationEventOrphans)))
	engine.POST("/admin/docker/operation-integrity/orphans/:eventId/cleanup", m.wrapPermission("admin:docker:operation-integrity:cleanup", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "清理已复核的 Docker 操作事件孤儿",
		IncludeParams: true,
	}, m.handler.CleanupDockerOperationEventOrphan)))
	engine.GET("/admin/docker/containers", m.wrapPermission("admin:docker:container:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker 容器列表",
		IncludeParams: true,
	}, m.handler.GetDockerContainers)))
	engine.POST("/admin/docker/containers/create-from-image", m.wrapPermission("admin:docker:container:create", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerContainerCreate,
		Description:   "从镜像创建 Docker 容器",
		IncludeParams: true,
	}, m.handler.CreateDockerContainerFromImage)))
	engine.GET("/admin/docker/containers/:id/logs", m.wrapPermission("admin:docker:container:logs", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "读取 Docker 容器日志",
		IncludeParams: true,
	}, m.handler.GetDockerContainerLogs)))
	engine.GET("/admin/docker/containers/:id/logs/stream", m.wrapPermission("admin:docker:container:logs", m.handler.StreamDockerContainerLogs))
	engine.GET("/admin/docker/containers/:id/stats", m.wrapPermission("admin:docker:container:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker 容器资源统计",
		IncludeParams: true,
	}, m.handler.GetDockerContainerStats)))
	engine.GET("/admin/docker/containers/:id/stats/stream", m.wrapPermission("admin:docker:container:query", m.handler.StreamDockerContainerStats))
	engine.GET("/admin/docker/containers/:id/terminal", m.wrapPermission("admin:docker:container:terminal", m.handler.OpenDockerContainerTerminal))
	engine.POST("/admin/docker/containers/cleanup/preview", m.wrapPermission("admin:docker:container:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerContainerDelete,
		Description:   "预览清理停止的 Docker 容器",
		IncludeParams: true,
	}, m.handler.PreviewDockerContainerCleanup)))
	engine.POST("/admin/docker/containers/cleanup/apply", m.wrapPermission("admin:docker:container:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerContainerDelete,
		Description:   "清理停止的 Docker 容器",
		IncludeParams: true,
	}, m.handler.ApplyDockerContainerCleanup)))
	engine.POST("/admin/docker/containers/:id/start", m.wrapPermission("admin:docker:container:start", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerContainerStart,
		Description:   "启动 Docker 容器",
		IncludeParams: true,
	}, m.handler.StartDockerContainer)))
	engine.POST("/admin/docker/containers/:id/stop", m.wrapPermission("admin:docker:container:stop", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerContainerStop,
		Description:   "停止 Docker 容器",
		IncludeParams: true,
	}, m.handler.StopDockerContainer)))
	engine.POST("/admin/docker/containers/:id/restart", m.wrapPermission("admin:docker:container:restart", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerContainerRestart,
		Description:   "重启 Docker 容器",
		IncludeParams: true,
	}, m.handler.RestartDockerContainer)))
	engine.GET("/admin/docker/containers/:id/compose-export", m.wrapPermission("admin:docker:container:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "导出 Docker Compose",
		IncludeParams: true,
	}, m.handler.ExportDockerContainerCompose)))
	engine.GET("/admin/docker/containers/:id", m.wrapPermission("admin:docker:container:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker 容器详情",
	}, m.handler.GetDockerContainer)))
	engine.DELETE("/admin/docker/containers/:id", m.wrapPermission("admin:docker:container:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerContainerDelete,
		Description:   "删除 Docker 容器",
		IncludeParams: true,
	}, m.handler.DeleteDockerContainer)))

	engine.GET("/admin/docker/images/local", m.wrapPermission("admin:docker:image:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker 本地镜像列表",
		IncludeParams: true,
	}, m.handler.GetLocalDockerImages)))
	engine.POST("/admin/docker/images/pull", m.wrapPermission("admin:docker:image:pull", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerImagePull,
		Description:   "拉取 Docker 镜像",
		IncludeParams: true,
	}, m.handler.PullDockerImage)))
	engine.POST("/admin/docker/images/tag", m.wrapPermission("admin:docker:image:tag", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerImageTag,
		Description:   "标记 Docker 镜像",
		IncludeParams: true,
	}, m.handler.TagDockerImage)))
	engine.POST("/admin/docker/images/push", m.wrapPermission("admin:docker:image:push", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerImagePush,
		Description:   "推送 Docker 镜像",
		IncludeParams: true,
	}, m.handler.PushDockerImage)))
	engine.POST("/admin/docker/images/remote/pull", m.wrapPermission("admin:docker:image:pull", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerImagePull,
		Description:   "拉取远程 Docker 镜像",
		IncludeParams: true,
	}, m.handler.PullRemoteDockerImage)))
	engine.POST("/admin/docker/images/cleanup/preview", m.wrapPermission("admin:docker:image:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerImageDelete,
		Description:   "预览清理 Docker dangling 镜像",
		IncludeParams: true,
	}, m.handler.PreviewDockerImageCleanup)))
	engine.POST("/admin/docker/images/cleanup/apply", m.wrapPermission("admin:docker:image:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerImageDelete,
		Description:   "清理 Docker dangling 镜像",
		IncludeParams: true,
	}, m.handler.ApplyDockerImageCleanup)))
	engine.GET("/admin/docker/images/local/:id/containers", m.wrapPermission("admin:docker:image:containers", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker 镜像关联容器",
	}, m.handler.GetLocalDockerImageContainers)))
	engine.GET("/admin/docker/images/local/:id/export", m.wrapPermission("admin:docker:image:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "导出 Docker 镜像",
		IncludeParams: true,
	}, m.handler.ExportLocalDockerImage)))
	engine.POST("/admin/docker/images/local/:id/startup-preview", m.wrapPermission("admin:docker:image:startup-preview", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "预览 Docker 镜像启动配置",
		IncludeParams: true,
	}, m.handler.GetDockerImageStartupPreview)))
	engine.GET("/admin/docker/images/local/:id", m.wrapPermission("admin:docker:image:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker 镜像详情",
	}, m.handler.GetLocalDockerImage)))
	engine.DELETE("/admin/docker/images/local/:id", m.wrapPermission("admin:docker:image:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerImageDelete,
		Description:   "删除 Docker 镜像",
		IncludeParams: true,
	}, m.handler.DeleteLocalDockerImage)))

	engine.GET("/admin/docker/compose/projects", m.wrapPermission("admin:docker:compose:project:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Compose 项目",
		IncludeParams: true,
	}, m.handler.GetDockerComposeProjects)))
	engine.POST("/admin/docker/compose/projects", m.wrapPermission("admin:docker:compose:project:create", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "创建 Docker Compose 项目",
		IncludeParams: true,
	}, m.handler.CreateDockerComposeProject)))
	engine.POST("/admin/docker/compose/projects/import-discovered", m.wrapPermission("admin:docker:compose:project:create", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "导入已发现 Docker Compose 项目",
		IncludeParams: true,
	}, m.handler.ImportDiscoveredDockerComposeProject)))
	engine.GET("/admin/docker/compose/projects/:projectId", m.wrapPermission("admin:docker:compose:project:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker Compose 项目详情",
	}, m.handler.GetDockerComposeProject)))
	engine.PUT("/admin/docker/compose/projects/:projectId/compose", m.wrapPermission("admin:docker:compose:project:update", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "更新 Docker Compose 项目配置",
		IncludeParams: true,
	}, m.handler.UpdateDockerComposeProject)))
	engine.POST("/admin/docker/compose/projects/:projectId/preview", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "预览 Docker Compose 项目",
		IncludeParams: true,
	}, m.handler.PreviewDockerComposeProject)))
	engine.POST("/admin/docker/compose/projects/:projectId/validate", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "校验 Docker Compose 项目",
		IncludeParams: true,
	}, m.handler.ValidateDockerComposeProject)))
	engine.POST("/admin/docker/compose/projects/:projectId/up", m.wrapPermission("admin:docker:compose:up", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeUp,
		Description:   "执行 Docker Compose 项目 Up",
		IncludeParams: true,
	}, m.handler.UpDockerComposeProject)))
	engine.POST("/admin/docker/compose/projects/:projectId/down", m.wrapPermission("admin:docker:compose:up", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeUp,
		Description:   "执行 Docker Compose 项目 Down",
		IncludeParams: true,
	}, m.handler.DownDockerComposeProject)))
	engine.POST("/admin/docker/compose/projects/:projectId/restart", m.wrapPermission("admin:docker:compose:up", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeUp,
		Description:   "执行 Docker Compose 项目 Restart",
		IncludeParams: true,
	}, m.handler.RestartDockerComposeProject)))
	engine.POST("/admin/docker/compose/projects/:projectId/ps", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Compose 项目 PS",
		IncludeParams: true,
	}, m.handler.DockerComposeProjectPS)))
	engine.POST("/admin/docker/compose/projects/:projectId/logs", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "读取 Docker Compose 项目 Logs",
		IncludeParams: true,
	}, m.handler.DockerComposeProjectLogs)))
	engine.POST("/admin/docker/compose/workspace/check", m.wrapPermission("admin:docker:compose:workspace:check", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "检查 Docker Compose 工作目录",
		IncludeParams: true,
	}, m.handler.CheckDockerComposeWorkspace)))
	engine.GET("/admin/docker/compose/builder/metadata", m.wrapPermission("admin:docker:compose:yaml:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker Compose Builder 元数据",
	}, m.handler.GetDockerComposeBuilderMetadata)))
	engine.POST("/admin/docker/compose/yaml/validate", m.wrapPermission("admin:docker:compose:yaml:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "校验 Docker Compose YAML",
		IncludeParams: true,
	}, m.handler.ValidateDockerComposeYaml)))
	engine.POST("/admin/docker/compose/dockerfile/preview", m.wrapPermission("admin:docker:compose:dockerfile:preview", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "预览 Dockerfile 构建配置",
		IncludeParams: true,
	}, m.handler.PreviewDockerfileBuild)))
	engine.POST("/admin/docker/compose/preview-with-files", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "带文件预览 Docker Compose",
		IncludeParams: true,
	}, m.handler.PreviewDockerComposeWithFiles)))

	engine.POST("/admin/docker/compose/validate", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "校验 Docker Compose",
		IncludeParams: true,
	}, m.handler.ValidateDockerCompose)))
	engine.POST("/admin/docker/compose/preview", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeValidate,
		Description:   "预览 Docker Compose",
		IncludeParams: true,
	}, m.handler.PreviewDockerCompose)))
	engine.POST("/admin/docker/compose/up", m.wrapPermission("admin:docker:compose:up", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeUp,
		Description:   "执行 Docker Compose Up",
		IncludeParams: true,
	}, m.handler.UpDockerCompose)))
	engine.POST("/admin/docker/compose/down", m.wrapPermission("admin:docker:compose:up", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeUp,
		Description:   "执行 Docker Compose Down",
		IncludeParams: true,
	}, m.handler.DownDockerCompose)))
	engine.POST("/admin/docker/compose/restart", m.wrapPermission("admin:docker:compose:up", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerComposeUp,
		Description:   "执行 Docker Compose Restart",
		IncludeParams: true,
	}, m.handler.RestartDockerCompose)))
	engine.POST("/admin/docker/compose/ps", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Compose PS",
		IncludeParams: true,
	}, m.handler.DockerComposePS)))
	engine.POST("/admin/docker/compose/logs", m.wrapPermission("admin:docker:compose:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "读取 Docker Compose Logs",
		IncludeParams: true,
	}, m.handler.DockerComposeLogs)))
	engine.GET("/admin/docker/volumes", m.wrapPermission("admin:docker:volume:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Volume",
		IncludeParams: true,
	}, m.handler.GetDockerVolumes)))
	engine.POST("/admin/docker/volumes", m.wrapPermission("admin:docker:volume:create", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "创建 Docker Volume",
		IncludeParams: true,
	}, m.handler.CreateDockerVolume)))
	engine.POST("/admin/docker/volumes/prune/preview", m.wrapPermission("admin:docker:volume:prune", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "预览清理 Docker Volume",
		IncludeParams: true,
	}, m.handler.PreviewDockerVolumePrune)))
	engine.POST("/admin/docker/volumes/prune/apply", m.wrapPermission("admin:docker:volume:prune", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "清理 Docker Volume",
		IncludeParams: true,
	}, m.handler.ApplyDockerVolumePrune)))
	engine.GET("/admin/docker/volumes/:name", m.wrapPermission("admin:docker:volume:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker Volume 详情",
	}, m.handler.GetDockerVolume)))
	engine.DELETE("/admin/docker/volumes/:name", m.wrapPermission("admin:docker:volume:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "删除 Docker Volume",
		IncludeParams: true,
	}, m.handler.DeleteDockerVolume)))
	engine.GET("/admin/docker/networks", m.wrapPermission("admin:docker:network:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Network",
		IncludeParams: true,
	}, m.handler.GetDockerNetworks)))
	engine.POST("/admin/docker/networks", m.wrapPermission("admin:docker:network:create", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "创建 Docker Network",
		IncludeParams: true,
	}, m.handler.CreateDockerNetwork)))
	engine.POST("/admin/docker/networks/prune/preview", m.wrapPermission("admin:docker:network:prune", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "预览清理 Docker Network",
		IncludeParams: true,
	}, m.handler.PreviewDockerNetworkPrune)))
	engine.POST("/admin/docker/networks/prune/apply", m.wrapPermission("admin:docker:network:prune", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "清理 Docker Network",
		IncludeParams: true,
	}, m.handler.ApplyDockerNetworkPrune)))
	engine.GET("/admin/docker/networks/:id", m.wrapPermission("admin:docker:network:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker Network 详情",
	}, m.handler.GetDockerNetwork)))
	engine.DELETE("/admin/docker/networks/:id", m.wrapPermission("admin:docker:network:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "删除 Docker Network",
		IncludeParams: true,
	}, m.handler.DeleteDockerNetwork)))
	engine.POST("/admin/docker/networks/:id/connect", m.wrapPermission("admin:docker:network:connect", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "连接 Docker Network",
		IncludeParams: true,
	}, m.handler.ConnectDockerNetwork)))
	engine.POST("/admin/docker/networks/:id/disconnect", m.wrapPermission("admin:docker:network:disconnect", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "断开 Docker Network",
		IncludeParams: true,
	}, m.handler.DisconnectDockerNetwork)))

	engine.GET("/admin/docker/daemon/config", m.wrapPermission("admin:docker:config:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker daemon 配置",
	}, m.handler.GetDockerDaemonConfig)))
	engine.POST("/admin/docker/daemon/config/validate", m.wrapPermission("admin:docker:config:validate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "校验 Docker daemon 配置",
		IncludeParams: true,
	}, m.handler.ValidateDockerDaemonConfig)))
	engine.PUT("/admin/docker/daemon/config", m.wrapPermission("admin:docker:config:update", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "保存 Docker daemon 配置",
		IncludeParams: true,
	}, m.handler.SaveDockerDaemonConfig)))
	engine.POST("/admin/docker/daemon/restart", m.wrapPermission("admin:docker:config:restart", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "重启 Docker daemon",
		IncludeParams: true,
	}, m.handler.RestartDockerDaemon)))

	engine.GET("/admin/docker/registries", m.wrapPermission("admin:docker:registry:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Registry 列表",
		IncludeParams: true,
	}, m.handler.GetDockerRegistries)))
	engine.POST("/admin/docker/registries", m.wrapPermission("admin:docker:registry:create", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerRegistryCreate,
		Description:   "创建 Docker Registry",
		IncludeParams: true,
	}, m.handler.CreateDockerRegistry)))
	engine.GET("/admin/docker/registries/:id/repositories", m.wrapPermission("admin:docker:registry:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Registry 仓库列表",
		IncludeParams: true,
	}, m.handler.GetDockerRepositories)))
	engine.GET("/admin/docker/registries/:id/repositories/*repository", m.wrapPermission("admin:docker:registry:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询 Docker Registry 仓库资源",
		IncludeParams: true,
	}, m.handler.DispatchDockerRepositoryResource)))
	engine.POST("/admin/docker/registries/:id/sync", m.wrapPermission("admin:docker:registry:sync", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "同步 Docker Registry",
		IncludeParams: true,
	}, m.handler.SyncDockerRegistry)))
	engine.POST("/admin/docker/registries/:id/test", m.wrapPermission("admin:docker:registry:test", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerRegistryTest,
		Description:   "测试 Docker Registry",
		IncludeParams: true,
	}, m.handler.TestDockerRegistry)))
	engine.GET("/admin/docker/registries/:id", m.wrapPermission("admin:docker:registry:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:   adminfacade.OperationTypeOther,
		Description: "查询 Docker Registry 详情",
	}, m.handler.GetDockerRegistry)))
	engine.PUT("/admin/docker/registries/:id", m.wrapPermission("admin:docker:registry:update", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerRegistryUpdate,
		Description:   "更新 Docker Registry",
		IncludeParams: true,
	}, m.handler.UpdateDockerRegistry)))
	engine.DELETE("/admin/docker/registries/:id", m.wrapPermission("admin:docker:registry:delete", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeDockerRegistryUpdate,
		Description:   "删除本地 Docker Registry 配置",
		IncludeParams: true,
	}, m.handler.DeleteDockerRegistry)))
}

func (m *Module) Facades() Facades {
	return m.facades
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
	if m == nil || m.operation == nil {
		return handler
	}
	return m.operation.Wrap(spec, handler)
}
