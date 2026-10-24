package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	bootstrapbuild "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/build"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	dockerinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	jobscheduler "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/scheduler"
	rabbitmqinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	obsinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability"
	protocolhttp "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/protocol/http"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/logger"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/route"
	"go.uber.org/zap"
)

type App struct {
	config     config.Config
	features   features.Set
	logger     *zap.Logger
	datasource store.Provider
	cache      cacheinfra.Provider
	docker     dockerinfra.Service
	jobs       jobscheduler.Scheduler
	rabbitmq   *rabbitmqinfra.Client
	obs        obsinfra.Manager
	httpServer *server.Hertz
	internal   *internalServer
	registry   *Registry
	modules    []core.Module
	obsStart   func(context.Context) error
	jobsStart  func(context.Context) error
}

// Options controls the filesystem resources used while constructing the server.
type Options struct {
	ConfigDir      string
	MigrationsRoot string
}

// New constructs the server with the historical configuration-directory API.
func New(configDir string) (*App, error) {
	return NewWithOptions(Options{ConfigDir: configDir})
}

// NewWithOptions constructs the server using explicit release filesystem paths.
func NewWithOptions(options Options) (*App, error) {
	configDir := strings.TrimSpace(options.ConfigDir)
	if configDir == "" {
		return nil, errors.New("configuration directory is required")
	}
	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if migrationsRoot := strings.TrimSpace(options.MigrationsRoot); migrationsRoot != "" {
		cfg.Datasource.Bootstrap.MigrationsDir = filepath.Join(migrationsRoot, cfg.Datasource.Driver)
	}

	log, err := logger.New(cfg.Logging, cfg.Profile)
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	log = logger.WithRuntimeIdentity(log, logger.RuntimeIdentity{
		ServiceName:       cfg.Observability.Tracing.ServiceName,
		PlatformMode:      string(cfg.Platform.Mode),
		NodeCode:          cfg.Platform.Node.Code,
		Profile:           cfg.Profile,
		ServiceInstanceID: cfg.Microservice.Service.InstanceID,
	})

	idGen, err := xid.New(cfg.ID.Node)
	if err != nil {
		return nil, fmt.Errorf("build snowflake generator: %w", err)
	}

	registry := NewRegistry()
	infraDeps, err := bootstrapbuild.Infra(cfg, log)
	if err != nil {
		return nil, err
	}
	cleanupInfra := func() {
		if infraDeps.Docker != nil {
			_ = infraDeps.Docker.Close()
		}
		if infraDeps.Datasource != nil {
			_ = infraDeps.Datasource.Close()
		}
		if infraDeps.Cache != nil {
			_ = infraDeps.Cache.Close()
		}
		if infraDeps.RabbitMQ != nil {
			_ = infraDeps.RabbitMQ.Close()
		}
		if infraDeps.Obs != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = infraDeps.Obs.Close(shutdownCtx)
		}
	}

	securityDeps, err := bootstrapbuild.Security(cfg)
	if err != nil {
		cleanupInfra()
		return nil, err
	}
	configuredFeatures := features.Resolve(cfg)
	featureSet, dockerService, err := activateDockerFeature(
		context.Background(),
		configuredFeatures,
		cfg.Docker,
		cfg.Security.OriginPatterns,
		idGen,
		infraDeps.Datasource,
		securityDeps.SecretValue,
		log,
		dockerinfra.New,
	)
	if err != nil {
		cleanupInfra()
		return nil, err
	}
	infraDeps.Docker = dockerService

	modules, err := bootstrapbuild.Modules(bootstrapruntime.ModuleDeps{
		Config:   cfg,
		Features: featureSet,
		Logger:   log,
		IDGen:    idGen,
		Registry: registry,
		Infra:    infraDeps,
		Security: securityDeps,
	})
	if err != nil {
		cleanupInfra()
		return nil, err
	}
	app := &App{
		config:     cfg,
		features:   featureSet,
		logger:     log,
		datasource: infraDeps.Datasource,
		cache:      infraDeps.Cache,
		docker:     infraDeps.Docker,
		jobs:       infraDeps.Jobs,
		rabbitmq:   infraDeps.RabbitMQ,
		obs:        infraDeps.Obs,
		httpServer: infraDeps.HTTPServer,
		registry:   registry,
		modules:    modules,
	}
	internal, err := configureInternalServerWithFeaturesAndMiddleware(
		cfg,
		featureSet,
		modules,
		protocolhttp.StandardMiddlewareChain(cfg, log, infraDeps.Obs.Middlewares()...),
	)
	if err != nil {
		cleanupInfra()
		return nil, err
	}
	app.internal = internal

	app.registerModules(infraDeps.HTTPServer.Engine, modules)
	return app, nil
}

type dockerServiceFactory func(config.DockerConfig, []string, *xid.Generator, store.Provider, secretvalueinfra.Service) (dockerinfra.Service, error)

const dockerStartupProbeTimeout = 5 * time.Second

func activateDockerFeature(
	ctx context.Context,
	featureSet features.Set,
	cfg config.DockerConfig,
	originPatterns []string,
	idGen *xid.Generator,
	provider store.Provider,
	secret secretvalueinfra.Service,
	log *zap.Logger,
	factory dockerServiceFactory,
) (features.Set, dockerinfra.Service, error) {
	if !featureSet.Enabled(features.DockerAdmin) {
		return featureSet, nil, nil
	}
	service, err := factory(cfg, originPatterns, idGen, provider, secret)
	if err != nil {
		return nil, nil, fmt.Errorf("build Docker administration feature: %w", err)
	}
	if service == nil {
		return nil, nil, errors.New("build Docker administration feature: constructor returned nil service")
	}
	probeCtx, cancel := context.WithTimeout(ctx, dockerStartupProbeTimeout)
	defer cancel()
	if err := service.Ping(probeCtx); err == nil {
		return featureSet, service, nil
	} else {
		_ = service.Close()
		action := "start Docker daemon and verify docker.engine.host, or set docker.enabled=false"
		if cfg.FailFast {
			return nil, nil, fmt.Errorf("required feature %s is unavailable: %w; %s", features.DockerAdmin, err, action)
		}
		if log != nil {
			log.Warn("optional feature unavailable; disabled for this process",
				zap.String("featureCode", string(features.DockerAdmin)),
				zap.Error(err),
				zap.String("action", action+"; set docker.failFast=true to stop startup instead"),
			)
		}
		return featureSet.Without(features.DockerAdmin), nil, nil
	}
}

func (a *App) Run() error {
	defer logger.Sync(a.logger)
	if a.internal != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = a.internal.Shutdown(shutdownCtx)
		}()
	}
	if a.datasource != nil {
		defer a.datasource.Close()
	}
	if a.cache != nil {
		defer a.cache.Close()
	}
	if a.docker != nil {
		defer func() { _ = a.docker.Close() }()
	}
	if a.jobs != nil {
		defer func() {
			_ = a.jobs.Stop(context.Background())
		}()
	}
	if a.rabbitmq != nil {
		defer func() {
			_ = a.rabbitmq.Close()
		}()
	}
	if len(a.modules) > 0 {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			a.shutdownModules(shutdownCtx)
		}()
	}
	if a.obs != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = a.obs.Close(shutdownCtx)
		}()
	}
	if err := a.startObservability(context.Background()); err != nil {
		return err
	}
	if err := a.startJobs(context.Background()); err != nil {
		return err
	}
	var internalFailure <-chan error
	if a.internal != nil {
		if err := a.internal.Start(); err != nil {
			return err
		}
		internalFailure = a.monitorInternalServer()
	}
	a.logger.Info("starting seven-framework-server",
		zap.String("address", a.config.Address()),
		zap.Strings("configFiles", a.config.LoadedFiles),
		zap.String("datasourceDriver", a.config.Datasource.Driver),
		zap.Bool("datasourceEnabled", a.config.Datasource.Enabled()),
		zap.Bool("datasourceConfigured", a.config.Datasource.Configured()),
		zap.Bool("cacheEnabled", a.config.Cache.Enabled),
		zap.Bool("cacheRedisEnabled", a.config.Cache.Redis.Enabled),
		zap.String("cacheRedisMode", string(a.config.Cache.Redis.Mode)),
		zap.Bool("cacheRedisConfigured", a.cache != nil && a.cache.Configured()),
		zap.Bool("cacheL1Enabled", a.config.Cache.L1Enabled()),
		zap.String("passwordAlgorithm", a.config.Security.Password.Algorithm),
		zap.String("keysProvider", a.config.Security.Keys.Provider),
		zap.Bool("schedulerEnabled", a.config.Scheduler.Enabled),
		zap.Bool("observabilityEnabled", a.config.Observability.Enabled),
		zap.Strings("runtimeFeatures", a.features.EnabledCodes()),
		zap.Int64("snowflakeNode", a.config.ID.Node),
	)
	a.httpServer.Spin()
	if internalFailure != nil {
		select {
		case err := <-internalFailure:
			return err
		default:
		}
	}
	return nil
}

func (a *App) shutdownModules(ctx context.Context) {
	if a == nil {
		return
	}
	for _, module := range a.modules {
		if hook, ok := module.(core.ShutdownHook); ok {
			_ = hook.Shutdown(ctx)
		}
	}
}

func (a *App) startObservability(ctx context.Context) error {
	if a.obsStart != nil {
		return a.obsStart(ctx)
	}
	if a.obs != nil {
		return a.obs.Start(ctx)
	}
	return nil
}

func (a *App) startJobs(ctx context.Context) error {
	if a.jobsStart != nil {
		return a.jobsStart(ctx)
	}
	if a.jobs != nil {
		return a.jobs.Start(ctx)
	}
	return nil
}

func (a *App) monitorInternalServer() <-chan error {
	failures := make(chan error, 1)
	internal := a.internal
	go func() {
		err := <-internal.Completion()
		if internal.Stopping() {
			return
		}
		if err == nil {
			err = errors.New("internal server stopped unexpectedly")
		}
		failures <- fmt.Errorf("internal server exited: %w", err)
		if a.httpServer != nil {
			_ = a.httpServer.Close()
		}
	}()
	return failures
}

func (a *App) Engine() *route.Engine {
	return a.httpServer.Engine
}

func (a *App) registerModules(engine *route.Engine, modules []core.Module) {
	rawRouter := route.IRouter(engine)
	router := rawRouter
	if contextPath := a.config.ContextPath(); contextPath != "" {
		router = engine.Group(contextPath)
	}
	for _, module := range modules {
		if provider, ok := module.(core.MiddlewareProvider); ok {
			middlewares := provider.Middlewares()
			if len(middlewares) > 0 {
				router.Use(middlewares...)
			}
		}
	}
	a.mountPlatformRoleRoutes(rawRouter, router, modules)
	for _, module := range modules {
		mountPublicModule(module, router)
		a.registry.Register(module)
	}
}

func (a *App) mountPlatformRoleRoutes(rawRouter, publicRouter route.IRouter, modules []core.Module) {
	featureSet := features.OrResolve(a.features, a.config)
	for _, module := range modules {
		if featureSet.Enabled(features.FederationHub) {
			if mounter, ok := module.(bootstrapruntime.HubRouteMounter); ok {
				mounter.MountHub(publicRouter)
			}
		}
		if featureSet.Enabled(features.FederationNode) {
			if a.internal == nil {
				if mounter, ok := module.(bootstrapruntime.InternalRouteMounter); ok {
					mounter.MountInternal(rawRouter)
				}
			}
		}
	}
}

func configureInternalServer(cfg config.Config, modules []core.Module) (*internalServer, error) {
	return configureInternalServerWithFeatures(cfg, features.Resolve(cfg), modules)
}

func configureInternalServerWithFeatures(cfg config.Config, featureSet features.Set, modules []core.Module) (*internalServer, error) {
	return configureInternalServerWithFeaturesAndMiddleware(cfg, featureSet, modules, protocolhttp.StandardMiddlewareChain(cfg, zap.NewNop()))
}

func configureInternalServerWithFeaturesAndMiddleware(cfg config.Config, featureSet features.Set, modules []core.Module, middlewares []app.HandlerFunc) (*internalServer, error) {
	if !featureSet.Enabled(features.FederationNode) || !cfg.Platform.Node.InternalListener.Enabled {
		return nil, nil
	}
	listener, err := net.Listen("tcp", cfg.Platform.Node.InternalListener.Listen)
	if err != nil {
		return nil, fmt.Errorf("bind platform node internal listener: %w", err)
	}
	mounters := make([]bootstrapruntime.InternalRouteMounter, 0)
	for _, module := range modules {
		if mounter, ok := module.(bootstrapruntime.InternalRouteMounter); ok {
			mounters = append(mounters, mounter)
		}
	}
	internal, err := newInternalServerWithMiddleware(listener, middlewares, mounters...)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	return internal, nil
}

func isPlatformRoleMounter(module core.Module) bool {
	if module == nil {
		return false
	}
	if _, ok := module.(bootstrapruntime.HubRouteMounter); ok {
		return true
	}
	_, ok := module.(bootstrapruntime.InternalRouteMounter)
	return ok
}

func mountPublicModule(module core.Module, router route.IRouter) {
	if mounter, ok := module.(bootstrapruntime.PublicRouteMounter); ok {
		mounter.MountPublic(router)
		return
	}
	if isPlatformRoleMounter(module) {
		return
	}
	module.Mount(router)
}
