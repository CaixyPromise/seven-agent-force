package infrastructure

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/app/observability/domain"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	obsinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability"
)

type RuntimeSnapshotProvider struct {
	manager obsinfra.Manager
}

func NewRuntimeSnapshotProvider(manager obsinfra.Manager) *RuntimeSnapshotProvider {
	return &RuntimeSnapshotProvider{manager: manager}
}

func (p *RuntimeSnapshotProvider) Snapshot(ctx context.Context) domain.RuntimeSnapshot {
	if p == nil || p.manager == nil {
		return domain.RuntimeSnapshot{}
	}
	return domain.RuntimeSnapshot(p.manager.Snapshot(ctx))
}

type DependencyHealthProvider struct {
	cache cacheinfra.Manager
}

func NewDependencyHealthProvider(cache cacheinfra.Manager) *DependencyHealthProvider {
	return &DependencyHealthProvider{cache: cache}
}

func (p *DependencyHealthProvider) CacheHealth(ctx context.Context) domain.CacheHealth {
	if p == nil || p.cache == nil {
		return domain.CacheHealth{}
	}
	health := p.cache.Health(ctx)
	return domain.CacheHealth{
		RedisEnabled:    health.Redis.Enabled,
		RedisMode:       health.Redis.Mode,
		RedisConfigured: health.Redis.Configured,
		RedisPing:       health.Redis.Ping,
	}
}
