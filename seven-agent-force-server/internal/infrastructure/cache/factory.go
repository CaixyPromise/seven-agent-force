package cache

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/key"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/l1"
	redisinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/redis"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	redisclient "github.com/redis/go-redis/v9"
)

type disabledProvider struct {
	mode string
}

func NewProvider(cfg config.CacheConfig) Provider {
	if !cfg.Enabled || !cfg.Redis.Enabled {
		return disabledProvider{mode: string(cfg.Redis.Mode)}
	}
	return redisinfra.NewProvider(cfg.Redis)
}

func NewDefaultManager(cfg config.CacheConfig, provider Provider) (Manager, error) {
	codec, err := NewCodec(cfg.Codec)
	if err != nil {
		return nil, err
	}
	builder := key.NewBuilder(cfg.Redis.KeyPrefix)
	l1Store, err := l1.NewStore(cfg)
	if err != nil {
		return nil, err
	}
	redisLayer := NewRedisLayer(provider, codec, l1Store)
	twoLevelLayer := NewTwoLevelLayer(l1Store, redisLayer, codec)
	governedLayer := NewGovernedLayer(builder, l1Store, provider, codec)

	options := []ManagerOption{
		WithHealthFunc(func(ctx context.Context) HealthSnapshot {
			snapshot := BuildHealthSnapshot(ctx, cfg, provider, l1Store, codec)
			if cfg.Governance.Enabled {
				status := governedLayer.GovernedStatus()
				snapshot.Governance.Enabled = status.Enabled
				snapshot.Governance.FanoutHealthy = status.FanoutHealthy
				snapshot.Governance.RedisHealthy = status.RedisHealthy
				snapshot.Governance.FreshnessHealthy = status.FreshnessHealthy
				snapshot.Governance.DirtyClasses = status.DirtyClasses
				snapshot.Governance.DirtyOverflowClasses = status.DirtyOverflowClasses
				snapshot.Governance.TransitioningClasses = status.TransitioningClasses
				snapshot.Governance.UnsafeClasses = status.UnsafeClasses
				snapshot.Governance.RejectedFanoutMessages = status.RejectedFanoutMessages
				snapshot.Governance.ReadTrusted = status.ReadTrusted
			}
			return snapshot
		}),
	}
	if provider != nil && provider.Configured() {
		options = append(options,
			WithKVLayer(redisLayer.Name(), redisLayer),
			WithHashLayer(redisLayer.Name(), redisLayer),
			WithPrimitiveLayer(redisLayer.Name(), redisLayer),
		)
	}
	if twoLevelLayer.Enabled() {
		options = append(options, WithTwoLevelLayer(twoLevelLayer.Name(), twoLevelLayer))
	}
	options = append(options, WithGovernedLayer(governedLayer.Name(), governedLayer))

	return NewManager("default", builder, options...), nil
}

func BuildHealthSnapshot(ctx context.Context, cfg config.CacheConfig, provider Provider, l1Store *l1.Store, codec Codec) HealthSnapshot {
	snapshot := HealthSnapshot{
		Enabled: cfg.Enabled,
	}
	if codec != nil {
		snapshot.Codec = codec.Name()
	}
	snapshot.Redis.Enabled = cfg.Enabled && cfg.Redis.Enabled
	if provider != nil {
		snapshot.Redis.Mode = provider.Mode()
		snapshot.Redis.Configured = provider.Configured()
		snapshot.Redis.Ping = provider.Configured() && provider.Ping(ctx) == nil
	}
	snapshot.L1.Enabled = l1Store != nil && l1Store.Enabled()
	return snapshot
}

func (p disabledProvider) Mode() string {
	if p.mode != "" {
		return p.mode
	}
	return "disabled"
}

func (disabledProvider) Client() redisclient.UniversalClient {
	return nil
}

func (disabledProvider) Ping(ctx context.Context) error {
	return ErrRedisUnavailable
}

func (disabledProvider) Configured() bool {
	return false
}

func (disabledProvider) Close() error {
	return nil
}
