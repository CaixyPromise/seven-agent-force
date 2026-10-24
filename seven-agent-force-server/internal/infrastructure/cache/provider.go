package cache

import (
	"context"
	"errors"

	redisclient "github.com/redis/go-redis/v9"
)

var ErrRedisUnavailable = errors.New("redis cache provider is not configured")

type Provider interface {
	Mode() string
	Client() redisclient.UniversalClient
	Ping(ctx context.Context) error
	Configured() bool
	Close() error
}

type HealthSnapshot struct {
	Enabled bool   `json:"enabled"`
	Codec   string `json:"codec"`
	Redis   struct {
		Enabled    bool   `json:"enabled"`
		Mode       string `json:"mode"`
		Configured bool   `json:"configured"`
		Ping       bool   `json:"ping"`
	} `json:"redis"`
	L1 struct {
		Enabled bool `json:"enabled"`
	} `json:"l1"`
	// Governance contains only aggregate DG5 freshness state. It deliberately
	// exposes no cached class target, raw Redis key, scope, event identifier,
	// or business value, while still making a RabbitMQ/Redis degradation
	// visible to the ordinary health surface.
	Governance struct {
		Enabled                bool   `json:"enabled"`
		FanoutHealthy          bool   `json:"fanoutHealthy"`
		RedisHealthy           bool   `json:"redisHealthy"`
		FreshnessHealthy       bool   `json:"freshnessHealthy"`
		DirtyClasses           int    `json:"dirtyClasses"`
		DirtyOverflowClasses   int    `json:"dirtyOverflowClasses"`
		TransitioningClasses   int    `json:"transitioningClasses"`
		UnsafeClasses          int    `json:"unsafeClasses"`
		RejectedFanoutMessages uint64 `json:"rejectedFanoutMessages"`
		ReadTrusted            bool   `json:"readTrusted"`
	} `json:"governance"`
}
