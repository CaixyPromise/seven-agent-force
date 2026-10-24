package kernel

import (
	"context"
	"fmt"

	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	jobinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/scheduler"
	obsinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xtime"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	config    config.Config
	catalog   core.ModuleCatalog
	cache     cacheinfra.Manager
	scheduler jobinfra.Scheduler
	obs       obsinfra.Manager
	jwt       *jwtinfra.Service
	features  features.Set
	startedAt string
}

func Install(deps bootstrapruntime.ModuleDeps) (*Module, error) {
	if deps.Registry == nil {
		return nil, fmt.Errorf("kernel module requires module catalog")
	}
	return &Module{
		config:    deps.Config,
		catalog:   deps.Registry,
		cache:     deps.Infra.CacheMgr,
		scheduler: deps.Infra.Jobs,
		obs:       deps.Infra.Obs,
		jwt:       deps.Security.JWT,
		features:  features.OrResolve(deps.Features, deps.Config),
		startedAt: xtime.Now().Format("2006-01-02T15:04:05.000Z07:00"),
	}, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{
		Name:   "kernel",
		Prefix: "/",
	}
}

func (m *Module) Mount(engine route.IRouter) {
	engine.GET("/ping", m.ping)
	engine.GET("/healthz", m.healthz)
	engine.GET("/system/features/runtime", m.runtimeFeatures)
	engine.GET("/ops/modules", m.wrapPermission("admin:ops:module:list", m.modules))
}

func (m *Module) ping(ctx context.Context, c *app.RequestContext) {
	response.Success(c, map[string]any{
		"pong": true,
	})
}

func (m *Module) healthz(ctx context.Context, c *app.RequestContext) {
	cacheHealth := cacheinfra.HealthSnapshot{}
	if m.cache != nil {
		cacheHealth = m.cache.Health(ctx)
	}
	observabilityHealth := obsinfra.HealthSnapshot{}
	if m.obs != nil {
		observabilityHealth = m.obs.Health()
	}
	jwtRotation := any(nil)
	if m.jwt != nil {
		if snapshot, err := m.jwt.Snapshot(ctx); err == nil {
			jwtRotation = snapshot
		}
	}
	response.Success(c, map[string]any{
		"status":      "ok",
		"appName":     m.config.Seven.Name,
		"profile":     m.config.Profile,
		"startedAt":   m.startedAt,
		"configFiles": m.config.LoadedFiles,
		"datasource": map[string]any{
			"driver":     m.config.Datasource.Driver,
			"enabled":    m.config.Datasource.Enabled(),
			"configured": m.config.Datasource.Configured(),
		},
		"cache": cacheHealth,
		"security": map[string]any{
			"passwordAlgorithm": m.config.Security.Password.Algorithm,
			"keysProvider":      m.config.Security.Keys.Provider,
			"masterConfigured":  m.config.Security.HasMasterKey(),
			"jwtConfigured":     m.config.Security.Keys.JWT.Configured(),
			"jwtKeyRotation":    jwtRotation,
		},
		"scheduler": map[string]any{
			"enabled":  m.config.Scheduler.Enabled,
			"running":  m.scheduler != nil && m.scheduler.Running(),
			"timezone": m.config.Scheduler.Timezone,
			"lock": map[string]any{
				"enabled": m.config.Scheduler.Lock.Enabled,
				"ttl":     m.config.Scheduler.Lock.TTL.String(),
			},
		},
		"observability": observabilityHealth,
	})
}

func (m *Module) modules(ctx context.Context, c *app.RequestContext) {
	response.Success(c, m.catalog.ListModules())
}

func (m *Module) runtimeFeatures(ctx context.Context, c *app.RequestContext) {
	response.Success(c, RuntimeFeaturesFromSet(m.config, features.OrResolve(m.features, m.config)))
}

type RuntimeFeatures struct {
	Features     RuntimeFeatureSet      `json:"features"`
	Platform     RuntimePlatformFeature `json:"platform"`
	Docker       RuntimeDockerFeature   `json:"docker"`
	Notification RuntimeManagedFeature  `json:"notification"`
	RuntimeLog   RuntimeManagedFeature  `json:"runtimeLog"`
}

type RuntimeFeatureSet struct {
	Enabled []string `json:"enabled"`
}

type RuntimePlatformFeature struct {
	Mode         string                      `json:"mode"`
	Capabilities config.PlatformCapabilities `json:"capabilities"`
}

type RuntimeDockerFeature struct {
	Enabled bool `json:"enabled"`
}

type RuntimeManagedFeature struct {
	ManagedByPlatform bool `json:"managedByPlatform"`
}

func RuntimeFeaturesFromConfig(cfg config.Config) RuntimeFeatures {
	return RuntimeFeaturesFromSet(cfg, features.Resolve(cfg))
}

func RuntimeFeaturesFromSet(cfg config.Config, featureSet features.Set) RuntimeFeatures {
	return RuntimeFeatures{
		Features: RuntimeFeatureSet{Enabled: featureSet.EnabledCodes()},
		Platform: RuntimePlatformFeature{
			Mode:         string(cfg.Platform.Mode),
			Capabilities: cfg.Platform.Capabilities(),
		},
		Docker: RuntimeDockerFeature{
			Enabled: featureSet.Enabled(features.DockerAdmin),
		},
		Notification: RuntimeManagedFeature{ManagedByPlatform: false},
		RuntimeLog:   RuntimeManagedFeature{ManagedByPlatform: false},
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
