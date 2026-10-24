package cache

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestNewProviderSupportsConfiguredModes(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.CacheConfig
		mode string
	}{
		{
			name: "single",
			cfg: config.CacheConfig{
				Enabled: true,
				Codec:   "sonic",
				Redis: config.RedisCacheConfig{
					Enabled: true,
					Mode:    config.RedisCacheModeSingle,
					Single: config.RedisSingleConfig{
						Addr: "127.0.0.1:6379",
					},
				},
			},
			mode: "single",
		},
		{
			name: "sentinel",
			cfg: config.CacheConfig{
				Enabled: true,
				Codec:   "sonic",
				Redis: config.RedisCacheConfig{
					Enabled: true,
					Mode:    config.RedisCacheModeSentinel,
					Sentinel: config.RedisSentinelConfig{
						MasterName: "seven-master",
						Addrs:      []string{"127.0.0.1:26379"},
					},
				},
			},
			mode: "sentinel",
		},
		{
			name: "cluster",
			cfg: config.CacheConfig{
				Enabled: true,
				Codec:   "sonic",
				Redis: config.RedisCacheConfig{
					Enabled: true,
					Mode:    config.RedisCacheModeCluster,
					Cluster: config.RedisClusterConfig{
						Addrs: []string{"127.0.0.1:7001", "127.0.0.1:7002"},
					},
				},
			},
			mode: "cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.cfg)
			if !provider.Configured() {
				t.Fatal("expected provider configured")
			}
			if provider.Mode() != tt.mode {
				t.Fatalf("unexpected provider mode: %s", provider.Mode())
			}
			if provider.Client() == nil {
				t.Fatal("expected raw client")
			}
			if err := provider.Close(); err != nil {
				t.Fatalf("close provider: %v", err)
			}
		})
	}
}

func TestNewProviderReturnsDisabledProviderWhenCacheDisabled(t *testing.T) {
	provider := NewProvider(config.CacheConfig{})
	if provider.Configured() {
		t.Fatal("expected disabled provider")
	}
	if provider.Client() != nil {
		t.Fatal("expected disabled provider to hide raw client")
	}
}
