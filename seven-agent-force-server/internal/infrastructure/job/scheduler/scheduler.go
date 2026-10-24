package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	jobregistry "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/registry"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/runner"
	lockinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/lock"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type Job = jobregistry.Job
type Registry = jobregistry.Registry

type Scheduler interface {
	Register(job Job) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Running() bool
}

type Service struct {
	cfg      config.SchedulerConfig
	logger   *zap.Logger
	registry Registry
	cron     *cron.Cron
	locker   lockinfra.DistributedLock
	metrics  runner.Metrics
	mu       sync.RWMutex
	running  bool
}

func New(cfg config.SchedulerConfig, log *zap.Logger, reg Registry, locker lockinfra.DistributedLock, metrics runner.Metrics) (*Service, error) {
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load scheduler timezone: %w", err)
	}
	service := &Service{
		cfg:      cfg,
		logger:   log,
		registry: reg,
		locker:   locker,
		metrics:  metrics,
	}
	service.cron = cron.New(
		cron.WithSeconds(),
		cron.WithLocation(location),
		cron.WithLogger(cron.PrintfLogger(zap.NewStdLog(log.Named("scheduler")))),
		cron.WithChain(
			cron.SkipIfStillRunning(cron.PrintfLogger(zap.NewStdLog(log.Named("scheduler")))),
			cron.Recover(cron.PrintfLogger(zap.NewStdLog(log.Named("scheduler")))),
		),
	)
	return service, nil
}

func (s *Service) Register(job Job) error {
	if s.registry != nil {
		if err := s.registry.Register(job); err != nil {
			return err
		}
	}
	if !s.cfg.Enabled {
		return nil
	}
	_, err := s.cron.AddJob(job.Spec(), cron.FuncJob(func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Lock.TTL)
		defer cancel()
		if execErr := runner.Execute(ctx, job, s.cfg.Lock.Enabled, s.cfg.Lock.TTL, s.locker, s.metrics); execErr != nil {
			s.logger.Error("scheduler_job_failed", zap.String("job", job.Name()), zap.Error(execErr))
		}
	}))
	return err
}

func (s *Service) Start(ctx context.Context) error {
	_ = ctx
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	s.cron.Start()
	s.running = true
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil
	}
	stopCtx := s.cron.Stop()
	if ctx != nil {
		select {
		case <-stopCtx.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		<-stopCtx.Done()
	}
	s.running = false
	return nil
}

func (s *Service) Running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}
