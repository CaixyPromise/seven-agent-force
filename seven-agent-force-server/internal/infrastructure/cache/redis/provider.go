package redis

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	redisclient "github.com/redis/go-redis/v9"
)

type Provider struct {
	mode       string
	configured bool
	client     redisclient.UniversalClient
}

func NewProvider(cfg config.RedisCacheConfig) *Provider {
	if !cfg.Configured() {
		return &Provider{
			mode:       string(cfg.Mode),
			configured: false,
		}
	}

	var client redisclient.UniversalClient
	switch cfg.Mode {
	case config.RedisCacheModeSentinel:
		client = redisclient.NewFailoverClient(&redisclient.FailoverOptions{
			MasterName:    cfg.Sentinel.MasterName,
			SentinelAddrs: cfg.Sentinel.Addrs,
			DB:            cfg.Database,
			Username:      cfg.Username,
			Password:      cfg.Password,
			ClientName:    cfg.ClientName,
			DialTimeout:   cfg.DialTimeout,
			ReadTimeout:   cfg.ReadTimeout,
			WriteTimeout:  cfg.WriteTimeout,
			PoolSize:      cfg.PoolSize,
			MinIdleConns:  cfg.MinIdleConns,
		})
	case config.RedisCacheModeCluster:
		client = redisclient.NewClusterClient(&redisclient.ClusterOptions{
			Addrs:        cfg.Cluster.Addrs,
			Username:     cfg.Username,
			Password:     cfg.Password,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			DialTimeout:  cfg.DialTimeout,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
		})
	default:
		client = redisclient.NewClient(&redisclient.Options{
			Addr:         cfg.Single.Addr,
			DB:           cfg.Database,
			Username:     cfg.Username,
			Password:     cfg.Password,
			ClientName:   cfg.ClientName,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
		})
	}

	return &Provider{
		mode:       string(cfg.Mode),
		configured: true,
		client:     client,
	}
}

func (p *Provider) Mode() string {
	if p == nil {
		return ""
	}
	return p.mode
}

func (p *Provider) Client() redisclient.UniversalClient {
	if p == nil {
		return nil
	}
	return p.client
}

func (p *Provider) Ping(ctx context.Context) error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Ping(ctx).Err()
}

func (p *Provider) Configured() bool {
	return p != nil && p.configured && p.client != nil
}

func (p *Provider) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}
