package cache

import (
	"context"
	"errors"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/l1"
	"golang.org/x/sync/singleflight"
)

type TwoLevelLayer struct {
	l1        *l1.Store
	redis     *RedisLayer
	codec     Codec
	loadGroup singleflight.Group
}

type loadResult struct {
	found   bool
	payload []byte
	ttl     time.Duration
}

func NewTwoLevelLayer(l1Store *l1.Store, redisLayer *RedisLayer, codec Codec) *TwoLevelLayer {
	return &TwoLevelLayer{
		l1:    l1Store,
		redis: redisLayer,
		codec: codec,
	}
}

func (l *TwoLevelLayer) Name() string {
	return "two_level"
}

func (l *TwoLevelLayer) Enabled() bool {
	return l != nil && ((l.l1 != nil && l.l1.Enabled()) || (l.redis != nil && l.redis.Enabled()))
}

func (l *TwoLevelLayer) GetOrLoad(ctx context.Context, cacheKey string, dest any, ttl time.Duration, loader func(context.Context) (any, error)) (bool, error) {
	if l == nil || !l.Enabled() {
		return false, ErrCacheLayerUnsupported
	}
	if payload, ok := l.getFromL1(cacheKey); ok {
		if err := l.codec.Unmarshal(payload, dest); err != nil {
			return false, err
		}
		return true, nil
	}

	result, err, _ := l.loadGroup.Do(cacheKey, func() (any, error) {
		if l.redis != nil && l.redis.Enabled() {
			payload, ok, err := l.redis.GetRaw(ctx, cacheKey)
			if err != nil {
				return nil, err
			}
			if ok {
				ttlValue, ttlErr := l.redis.TTL(ctx, cacheKey)
				if ttlErr != nil {
					return nil, ttlErr
				}
				return loadResult{found: true, payload: payload, ttl: ttlValue}, nil
			}
		}
		if loader == nil {
			return loadResult{}, nil
		}

		value, err := loader(ctx)
		if err != nil || value == nil {
			return loadResult{}, err
		}

		payload, err := l.codec.Marshal(value)
		if err != nil {
			return nil, err
		}

		if l.redis != nil && l.redis.Enabled() {
			if err := l.redis.SetRaw(ctx, cacheKey, payload, ttl); err != nil {
				return nil, err
			}
		}
		return loadResult{found: true, payload: payload, ttl: ttl}, nil
	})
	if err != nil {
		return false, err
	}

	typed, ok := result.(loadResult)
	if !ok || !typed.found {
		return false, nil
	}
	if len(typed.payload) == 0 {
		return false, errors.New("two-level cache loader returned empty payload")
	}

	l.setL1(cacheKey, typed.payload, typed.ttl)
	if err := l.codec.Unmarshal(typed.payload, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (l *TwoLevelLayer) getFromL1(cacheKey string) ([]byte, bool) {
	if l == nil || l.l1 == nil {
		return nil, false
	}
	return l.l1.Get(cacheKey)
}

func (l *TwoLevelLayer) setL1(cacheKey string, payload []byte, ttl time.Duration) {
	if l == nil || l.l1 == nil {
		return
	}
	l.l1.Set(cacheKey, payload, ttl)
}
