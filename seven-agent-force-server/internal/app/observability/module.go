package observability

import (
	"context"
	"fmt"

	obsapp "github.com/CaixyPromise/seven-agent/force/internal/app/observability/application"
	obshandler "github.com/CaixyPromise/seven-agent/force/internal/app/observability/handler"
	obsinfra "github.com/CaixyPromise/seven-agent/force/internal/app/observability/infrastructure"
	obsjob "github.com/CaixyPromise/seven-agent/force/internal/app/observability/job"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	dockerinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Dependencies struct {
	AuditEvents ssofacade.AuditEventQueryFacade
	Clients     ssofacade.ClientQueryFacade
	Sessions    ssofacade.SessionFacade
	RuntimeLogs adminfacade.RuntimeLogFacade
}

type Module struct {
	service *obsapp.Service
	handler *obshandler.Handler
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, error) {
	if !deps.Config.Observability.Enabled {
		return nil, nil
	}
	if deps.Infra.Obs == nil {
		return nil, fmt.Errorf("observability module requires observability manager")
	}
	if refs.AuditEvents == nil || refs.Clients == nil || refs.Sessions == nil {
		return nil, fmt.Errorf("observability module requires sso read facades")
	}
	store := obsinfra.NewSnapshotStore(deps.Infra.Cache)
	runtimeProvider := obsinfra.NewRuntimeSnapshotProvider(deps.Infra.Obs)
	dependencyHealth := obsinfra.NewDependencyHealthProvider(deps.Infra.CacheMgr)
	service := obsapp.NewService(
		deps.Config.Observability,
		store,
		dependencyHealth,
		runtimeProvider,
		refs.AuditEvents,
		refs.Clients,
		refs.Sessions,
		refs.RuntimeLogs,
		newDockerMetricsAdapter(deps.Infra.Docker),
	)
	if deps.Infra.Jobs != nil {
		if err := deps.Infra.Jobs.Register(obsjob.NewSnapshotJob(service, deps.Config.Observability.SnapshotIntervalMs)); err != nil {
			return nil, err
		}
	}
	return &Module{
		service: service,
		handler: obshandler.NewHandler(service),
	}, nil
}

type dockerMetricsAdapter struct {
	source dockerinfra.Service
}

func newDockerMetricsAdapter(source dockerinfra.Service) obsapp.DockerMetricsProvider {
	if source == nil {
		return nil
	}
	return dockerMetricsAdapter{source: source}
}

func (a dockerMetricsAdapter) MetricsSnapshot(ctx context.Context) (*obsapp.DockerMetricsSnapshot, error) {
	snapshot, err := a.source.MetricsSnapshot(ctx)
	if err != nil || snapshot == nil {
		return nil, err
	}
	return &obsapp.DockerMetricsSnapshot{
		Enabled:               snapshot.Enabled,
		DaemonHealthy:         snapshot.DaemonHealthy,
		RegistryHealthy:       snapshot.RegistryHealthy,
		ContainerCountByState: snapshot.ContainerCountByState,
		ImageCount:            snapshot.ImageCount,
		ImageSizeBytes:        snapshot.ImageSizeBytes,
		OperationTotal:        snapshot.OperationTotal,
		OperationSucceeded:    snapshot.OperationSucceeded,
		OperationFailed:       snapshot.OperationFailed,
		PolicyViolationTotal:  snapshot.PolicyViolationTotal,
	}, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "observability", Prefix: "/observability"}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/observability/overview", m.wrapPermission("admin:observability:view", m.handler.Overview))
	engine.GET("/observability/logs/page", m.wrapPermission("admin:runtime-log:view", m.handler.LogPage))
	engine.GET("/observability/logs/stream", m.wrapPermission("admin:runtime-log:stream", m.handler.LogStream))
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
