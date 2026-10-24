package cache_governance

import (
	"context"
	"fmt"
	"strings"
	"time"

	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	cacheapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/application"
	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	cachehandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/handler"
	cacheinfraapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/infrastructure"
	cachejob "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/job"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	rabbitmqinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	cancel       context.CancelFunc
	brokerClient *rabbitmqinfra.Client
	targeted     cachefacade.TargetedInvalidationRegistrar
	refresh      cachefacade.RefreshFacade
	handler      *cachehandler.RefreshHandler
	oplog        adminfacade.OperationLogger
}

// Install composes DG5 only when all three durable dependencies are explicitly
// enabled. Returning a DisabledRegistrar otherwise keeps config/dict reads
// database-authoritative; it never creates a local-only invalidation path.
func Install(deps bootstrapruntime.ModuleDeps) (*Module, cachefacade.InvalidationRegistrar, error) {
	if !deps.Config.Cache.Governance.Enabled {
		return nil, cachefacade.DisabledRegistrar{}, nil
	}
	if deps.Infra.Jobs == nil {
		return nil, nil, fmt.Errorf("cache governance requires an outbox scheduler")
	}
	if deps.IDGen == nil {
		return nil, nil, fmt.Errorf("cache governance requires a distributed outbox id generator")
	}
	if !deps.Config.Cache.Enabled || deps.Infra.Datasource == nil || deps.Infra.Transactor == nil || !deps.Infra.Transactor.Enabled() {
		return nil, nil, fmt.Errorf("cache governance requires an enabled cache and datasource transaction")
	}
	if !deps.Config.RabbitMQ.Enabled || !deps.Config.RabbitMQ.Declare {
		return nil, nil, fmt.Errorf("cache governance requires enabled RabbitMQ topology declaration")
	}
	instanceID := strings.TrimSpace(deps.Config.Cache.Governance.InstanceID)
	if instanceID == "" {
		return nil, nil, fmt.Errorf("cache governance requires cache.governance.instanceId")
	}
	governed, ok := deps.Infra.CacheMgr.(cacheinfra.GovernedCache)
	if !ok {
		return nil, nil, fmt.Errorf("cache governance requires classified cache layer")
	}

	// DG5 owns its broker connection because its consumer retry loop can close
	// and reconnect after a broker fault. Reusing deps.Infra.RabbitMQ would let
	// a cache-only reconnect interrupt file/notification consumers on the
	// shared application connection.
	brokerClient, err := rabbitmqinfra.New(deps.Config.RabbitMQ)
	if err != nil {
		return nil, nil, fmt.Errorf("connect dedicated cache governance RabbitMQ client: %w", err)
	}
	if !brokerClient.Enabled() {
		_ = brokerClient.Close()
		return nil, nil, fmt.Errorf("cache governance dedicated RabbitMQ client is unavailable")
	}

	outbox := cacheinfraapp.NewOutboxAdapter(deps.Infra.Datasource.SQLX(), deps.IDGen.NextID)
	generation := cacheinfraapp.NewGenerationAdapter(governed)
	broker, err := cacheinfraapp.NewFanoutAdapter(brokerClient, generation, instanceID, true)
	if err != nil {
		_ = brokerClient.Close()
		return nil, nil, fmt.Errorf("initialize cache governance fanout: %w", err)
	}
	// A classified cache read is eligible only while this source-adjacent,
	// cross-instance freshness gate can establish that no committed invalidation
	// is pending. Installing it before the registrar becomes reachable keeps
	// the zero-stale contract fail-closed from the first application request.
	governed.SetFreshnessGate(outbox)
	targetedGoverned, ok := governed.(cacheinfra.TargetedGovernedCache)
	if !ok {
		_ = brokerClient.Close()
		return nil, nil, fmt.Errorf("cache governance requires targeted cache layer")
	}
	targetedGoverned.SetTargetFreshnessGate(outbox)
	// The lease owner is persisted in sys_outbox_event. Keep the raw runtime
	// instance identity out of that operational record just as cache targets
	// and keys are kept out of events and logs.
	instanceDigest := cachepolicy.EventDigest(instanceID)
	service := cacheapp.NewService(outbox, generation, broker, outbox, "cache-governance-relay:"+instanceDigest[:24])
	if !service.Enabled() {
		return nil, nil, fmt.Errorf("cache governance service is unavailable")
	}
	targeted := cacheapp.NewTargetedService(outbox, generation, broker, outbox, "cache-governance-session-v2:"+instanceDigest[:24])
	if !targeted.Enabled() {
		_ = brokerClient.Close()
		return nil, nil, fmt.Errorf("targeted cache governance service is unavailable")
	}
	refresh := cacheapp.NewRefreshService(deps.Infra.Transactor, outbox, generation, broker, outbox)
	if !refresh.Enabled() {
		_ = brokerClient.Close()
		return nil, nil, fmt.Errorf("global cache refresh service is unavailable")
	}
	// V3 consumption and recovery remain installed on every new instance. Only
	// request creation is rollout-gated so an old V1/V2 consumer cannot safely
	// coexist with a newly emitted global-refresh envelope.
	refresh.SetRequestEnabled(deps.Config.Cache.Governance.GlobalRefreshEnabled)
	service.BindTargeted(targeted)
	service.BindRefresh(refresh)
	if err := deps.Infra.Jobs.Register(cachejob.NewOutboxRelayJob(service, deps.Config.Cache.Governance.RelayInterval, deps.Config.Cache.Governance.RelayBatch)); err != nil {
		_ = brokerClient.Close()
		return nil, nil, err
	}
	consumerCtx, cancel := context.WithCancel(context.Background())
	startConsumer(consumerCtx, broker, service)
	return &Module{cancel: cancel, brokerClient: brokerClient, targeted: targeted, refresh: refresh, handler: cachehandler.NewRefreshHandler(refresh)}, service, nil
}

// TargetedInvalidations exposes only the DG6.2 constrained registration
// contract to SSO after composition; it never exposes cache keys or broker API.
func (m *Module) TargetedInvalidations() cachefacade.TargetedInvalidationRegistrar {
	if m == nil || m.targeted == nil {
		return cachefacade.DisabledTargetedRegistrar{}
	}
	return m.targeted
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "system-cache-governance", Prefix: ""}
}

func (m *Module) Mount(engine route.IRouter) {
	if m == nil || engine == nil || m.handler == nil {
		return
	}
	engine.POST("/system/cache/refresh", m.wrapPermission("system:cache:refresh", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeSystemCacheClear, Description: "刷新应用缓存", IncludeParams: false, IncludeResult: false, OmitQuery: true}, m.handler.Refresh)))
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m != nil {
		m.oplog = oplog
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

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}

func (m *Module) Shutdown(ctx context.Context) error {
	if m != nil && m.cancel != nil {
		m.cancel()
	}
	if m != nil && m.brokerClient != nil {
		_ = m.brokerClient.Close()
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

func startConsumer(ctx context.Context, broker *cacheinfraapp.FanoutAdapter, service *cacheapp.Service) {
	if ctx == nil || broker == nil || service == nil {
		return
	}
	go func() {
		for ctx.Err() == nil {
			_ = broker.ConsumeGoverned(ctx, service.HandleFanout, service.HandleTargetedFanout, service.HandleRefreshFanout)
			if ctx.Err() != nil {
				return
			}
			_ = broker.Reconnect(ctx)
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-timer.C:
			}
		}
	}()
}
