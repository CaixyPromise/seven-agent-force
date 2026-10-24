package infrastructure

import (
	"context"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

// ActiveSessionValidityCache is a narrow SSO adapter over the targeted cache
// protocol. It cannot cache a complete session aggregate or token material.
type ActiveSessionValidityCache struct {
	governed cacheinfra.TargetedGovernedCache
}

func NewActiveSessionValidityCache(governed cacheinfra.TargetedGovernedCache) *ActiveSessionValidityCache {
	return &ActiveSessionValidityCache{governed: governed}
}

// Resolve does not interpret session facts. The application-owned loader
// supplies the narrow projection, cache admission decision, and hard expiry;
// this adapter only delegates the strictly targeted cache protocol.
func (c *ActiveSessionValidityCache) Resolve(ctx context.Context, sessionID string, loader func(context.Context) (*cachepolicy.ActiveSessionValiditySnapshot, bool, error)) (*cachepolicy.ActiveSessionValiditySnapshot, error) {
	if c == nil || c.governed == nil || loader == nil {
		return nil, nil
	}
	request, ok := cachepolicy.ActiveSessionValidityReadRequest(sessionID)
	if !ok {
		return nil, nil
	}
	var snapshot cachepolicy.ActiveSessionValiditySnapshot
	found, err := c.governed.GetOrLoadTargeted(ctx, request, &snapshot, func(loadCtx context.Context) (cachepolicy.TargetedCacheableValue, error) {
		value, cacheable, err := loader(loadCtx)
		if err != nil || value == nil {
			return cachepolicy.TargetedCacheableValue{}, err
		}
		return cachepolicy.TargetedCacheableValue{Value: *value, Cacheable: cacheable, ExpiresAt: value.ExpiresAt}, nil
	})
	if err != nil || !found {
		return nil, err
	}
	snapshot.AMR = append([]string(nil), snapshot.AMR...)
	return &snapshot, nil
}
