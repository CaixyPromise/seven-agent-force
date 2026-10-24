package build

import (
	"context"
	"fmt"

	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource"
	dsbootstrap "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/bootstrap"
	emailinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/email"
	jobregistry "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/registry"
	jobscheduler "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/scheduler"
	limiterinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/limiter"
	lockinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/lock"
	rabbitmqinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	obsinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/protocol/http"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"go.uber.org/zap"
)

func Infra(cfg config.Config, log *zap.Logger) (bootstrapruntime.InfraDeps, error) {
	provider, err := datasource.NewProvider(cfg.Datasource, log)
	if err != nil {
		return bootstrapruntime.InfraDeps{}, fmt.Errorf("build datasource provider: %w", err)
	}
	if cfg.Datasource.Bootstrap.StartupEnabled() {
		bootstrapper := dsbootstrap.NewService(log)
		if _, err := bootstrapper.Bootstrap(context.Background(), provider, cfg.Datasource.Bootstrap); err != nil {
			_ = provider.Close()
			return bootstrapruntime.InfraDeps{}, fmt.Errorf("bootstrap datasource schema: %w", err)
		}
	}

	cacheProvider := cacheinfra.NewProvider(cfg.Cache)
	cacheManager, err := cacheinfra.NewDefaultManager(cfg.Cache, cacheProvider)
	if err != nil {
		_ = provider.Close()
		return bootstrapruntime.InfraDeps{}, fmt.Errorf("build cache manager: %w", err)
	}
	cacheinfra.SetDefaultManager(cacheManager)
	lockService := lockinfra.NewRedisService(cacheProvider)
	limiterService := limiterinfra.New(cfg.Limiter, cacheManager)

	obsManager, err := obsinfra.New(cfg.Observability, log, provider.DB(), cacheManager)
	if err != nil {
		_ = provider.Close()
		_ = cacheProvider.Close()
		return bootstrapruntime.InfraDeps{}, fmt.Errorf("build observability: %w", err)
	}
	httpServer := http.NewServer(cfg, log, obsManager.Middlewares()...)
	obsManager.Attach(httpServer)

	jobRegistry := jobregistry.New()
	scheduler, err := jobscheduler.New(cfg.Scheduler, log, jobRegistry, lockService, obsManager)
	if err != nil {
		_ = provider.Close()
		_ = cacheProvider.Close()
		return bootstrapruntime.InfraDeps{}, fmt.Errorf("build scheduler: %w", err)
	}
	rabbitClient, err := rabbitmqinfra.New(cfg.RabbitMQ)
	if err != nil {
		_ = provider.Close()
		_ = cacheProvider.Close()
		return bootstrapruntime.InfraDeps{}, fmt.Errorf("build rabbitmq client: %w", err)
	}
	emailSender, err := emailinfra.New(cfg.Email, cacheManager, log)
	if err != nil {
		_ = provider.Close()
		_ = cacheProvider.Close()
		_ = rabbitClient.Close()
		return bootstrapruntime.InfraDeps{}, fmt.Errorf("build email sender: %w", err)
	}
	return bootstrapruntime.InfraDeps{
		Datasource: provider,
		Transactor: provider.Transactor(),
		Cache:      cacheProvider,
		CacheMgr:   cacheManager,
		Locker:     lockService,
		Limiter:    limiterService,
		Jobs:       scheduler,
		RabbitMQ:   rabbitClient,
		Email:      emailSender,
		Obs:        obsManager,
		HTTPServer: httpServer,
	}, nil
}
