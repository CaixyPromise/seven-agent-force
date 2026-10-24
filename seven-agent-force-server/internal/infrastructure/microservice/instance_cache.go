package microservice

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type cacheEntry struct {
	instances []ServiceInstance
	err       error
	expiresAt time.Time
}

type CachedResolver struct {
	resolver       ServiceResolver
	ttl            time.Duration
	emptyTTL       time.Duration
	refreshTimeout time.Duration
	now            func() time.Time

	mu          sync.RWMutex
	entries     map[string]cacheEntry
	generations map[string]uint64
	refresh     singleflight.Group
}

type CachedResolverOptions struct {
	TTL            time.Duration
	EmptyTTL       time.Duration
	ResolveTimeout time.Duration
}

func NewCachedResolver(resolver ServiceResolver, ttl, emptyTTL time.Duration) *CachedResolver {
	return NewCachedResolverWithOptions(resolver, CachedResolverOptions{TTL: ttl, EmptyTTL: emptyTTL})
}

func NewCachedResolverWithOptions(resolver ServiceResolver, options CachedResolverOptions) *CachedResolver {
	if options.TTL <= 0 {
		options.TTL = 10 * time.Second
	}
	if options.EmptyTTL <= 0 {
		options.EmptyTTL = time.Second
	}
	if options.ResolveTimeout <= 0 {
		options.ResolveTimeout = 2 * time.Second
	}
	return &CachedResolver{
		resolver: resolver, ttl: options.TTL, emptyTTL: options.EmptyTTL, refreshTimeout: options.ResolveTimeout,
		now: time.Now, entries: make(map[string]cacheEntry), generations: make(map[string]uint64),
	}
}

func (r *CachedResolver) Resolve(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	if r == nil || isNilDependency(r.resolver) {
		return nil, ErrInvalidDependency
	}
	if ctx == nil {
		return nil, ErrInvalidContext
	}
	if serviceName == "" {
		return nil, ErrInvalidRequest
	}
	if instances, err, ok := r.cached(serviceName); ok {
		return instances, err
	}
	resultChannel := r.refresh.DoChan(serviceName, func() (any, error) {
		if instances, cachedErr, ok := r.cached(serviceName); ok {
			return cacheEntry{instances: instances, err: cachedErr}, nil
		}
		r.mu.RLock()
		generation := r.generations[serviceName]
		r.mu.RUnlock()
		refreshCtx, cancel := context.WithTimeout(context.Background(), r.refreshTimeout)
		instances, resolveErr := r.resolver.Resolve(refreshCtx, serviceName)
		cancel()
		if resolveErr != nil && !errors.Is(resolveErr, ErrNoHealthyInstance) {
			return nil, resolveErr
		}
		entry := cacheEntry{instances: cloneInstances(instances), err: resolveErr, expiresAt: r.now().Add(r.ttl)}
		if errors.Is(resolveErr, ErrNoHealthyInstance) || len(instances) == 0 {
			entry.err = ErrNoHealthyInstance
			entry.expiresAt = r.now().Add(r.emptyTTL)
		}
		r.mu.Lock()
		if r.generations[serviceName] == generation {
			r.entries[serviceName] = entry
		}
		r.mu.Unlock()
		return entry, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChannel:
		if result.Err != nil {
			return nil, result.Err
		}
		entry, ok := result.Val.(cacheEntry)
		if !ok {
			return nil, errors.New("invalid cache refresh result")
		}
		return cloneInstances(entry.instances), entry.err
	}
}

func (r *CachedResolver) Invalidate(serviceName string) {
	if r == nil || serviceName == "" {
		return
	}
	r.mu.Lock()
	r.generations[serviceName]++
	delete(r.entries, serviceName)
	r.refresh.Forget(serviceName)
	r.mu.Unlock()
}

func (r *CachedResolver) cached(serviceName string) ([]ServiceInstance, error, bool) {
	r.mu.RLock()
	entry, ok := r.entries[serviceName]
	r.mu.RUnlock()
	if !ok || !r.now().Before(entry.expiresAt) {
		return nil, nil, false
	}
	return cloneInstances(entry.instances), entry.err, true
}

func cloneInstances(instances []ServiceInstance) []ServiceInstance {
	if instances == nil {
		return nil
	}
	cloned := make([]ServiceInstance, len(instances))
	for index, instance := range instances {
		cloned[index] = instance
		cloned[index].Tags = append([]string(nil), instance.Tags...)
		if instance.Metadata != nil {
			cloned[index].Metadata = make(map[string]string, len(instance.Metadata))
			for key, value := range instance.Metadata {
				cloned[index].Metadata[key] = value
			}
		}
	}
	return cloned
}
