package limiter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

var ErrRateLimited = errors.New("rate limit exceeded")

type Decision struct {
	Allowed    bool
	Key        string
	Limit      int64
	Current    int64
	Remaining  int64
	RetryAfter time.Duration
	ResetAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (Decision, error)
	AllowDefault(ctx context.Context, key string) (Decision, error)
}

type FailOpenOverrideLimiter interface {
	AllowWithFailOpen(ctx context.Context, key string, limit int64, window time.Duration, failOpen bool) (Decision, error)
}

type Service struct {
	cfg config.LimiterConfig
	mgr cacheinfra.Manager
}

func New(cfg config.LimiterConfig, mgr cacheinfra.Manager) *Service {
	cfg.KeyPrefix = strings.TrimSpace(cfg.KeyPrefix)
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "seven:limit"
	}
	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = 60
	}
	if cfg.DefaultWindow <= 0 {
		cfg.DefaultWindow = time.Minute
	}
	return &Service{cfg: cfg, mgr: mgr}
}

func (s *Service) AllowDefault(ctx context.Context, key string) (Decision, error) {
	if s == nil {
		return Decision{Allowed: true}, nil
	}
	return s.Allow(ctx, key, s.cfg.DefaultLimit, s.cfg.DefaultWindow)
}

func (s *Service) Allow(ctx context.Context, key string, limit int64, window time.Duration) (Decision, error) {
	if s == nil {
		return Decision{Allowed: true, Limit: limit, Remaining: max(limit, 0), ResetAfter: window}, nil
	}
	return s.allow(ctx, key, limit, window, s.cfg.FailOpen)
}

func (s *Service) AllowWithFailOpen(ctx context.Context, key string, limit int64, window time.Duration, failOpen bool) (Decision, error) {
	if s == nil {
		return Decision{Allowed: true, Limit: limit, Remaining: max(limit, 0), ResetAfter: window}, nil
	}
	return s.allow(ctx, key, limit, window, failOpen)
}

func (s *Service) allow(ctx context.Context, key string, limit int64, window time.Duration, failOpen bool) (Decision, error) {
	normalizedKey := s.cacheKey(key)
	if limit <= 0 || window <= 0 || !s.cfg.Enabled {
		return Decision{Allowed: true, Key: normalizedKey, Limit: limit, Remaining: max(limit, 0), ResetAfter: window}, nil
	}
	if s.mgr == nil {
		if failOpen {
			return Decision{Allowed: true, Key: normalizedKey, Limit: limit, Remaining: limit, ResetAfter: window}, nil
		}
		return Decision{Allowed: false, Key: normalizedKey, Limit: limit, RetryAfter: window, ResetAfter: window}, cacheinfra.ErrCacheLayerUnsupported
	}
	current, err := s.mgr.Incr(ctx, normalizedKey, window)
	if err != nil {
		if failOpen {
			return Decision{Allowed: true, Key: normalizedKey, Limit: limit, Remaining: limit, ResetAfter: window}, nil
		}
		return Decision{Allowed: false, Key: normalizedKey, Limit: limit, RetryAfter: window, ResetAfter: window}, err
	}
	remaining := limit - current
	if remaining < 0 {
		remaining = 0
	}
	allowed := current <= limit
	decision := Decision{
		Allowed:    allowed,
		Key:        normalizedKey,
		Limit:      limit,
		Current:    current,
		Remaining:  remaining,
		ResetAfter: window,
	}
	if !allowed {
		decision.RetryAfter = window
		return decision, ErrRateLimited
	}
	return decision, nil
}

func (s *Service) cacheKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "anonymous"
	}
	prefix := "seven:limit"
	if s != nil && strings.TrimSpace(s.cfg.KeyPrefix) != "" {
		prefix = strings.TrimSpace(s.cfg.KeyPrefix)
	}
	return fmt.Sprintf("%s:%s", strings.TrimRight(prefix, ":"), key)
}
