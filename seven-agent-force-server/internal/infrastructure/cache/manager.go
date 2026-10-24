package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/key"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

var ErrCacheLayerUnsupported = errors.New("cache layer is not configured for this manager")

type Layer interface {
	Name() string
}

type KVCache interface {
	Get(ctx context.Context, cacheKey string, dest any) (bool, error)
	GetString(ctx context.Context, cacheKey string) (string, bool, error)
	GetBytes(ctx context.Context, cacheKey string) ([]byte, bool, error)
	Set(ctx context.Context, cacheKey string, value any, ttl time.Duration) error
	SetString(ctx context.Context, cacheKey string, value string, ttl time.Duration) error
	SetBytes(ctx context.Context, cacheKey string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, cacheKey string) error
	Exists(ctx context.Context, cacheKey string) (bool, error)
	Expire(ctx context.Context, cacheKey string, ttl time.Duration) error
	GetDel(ctx context.Context, cacheKey string, dest any) (bool, error)
	GetDelString(ctx context.Context, cacheKey string) (string, bool, error)
	CompareAndDelete(ctx context.Context, cacheKey string, expected any) (bool, error)
	CompareAndDeleteString(ctx context.Context, cacheKey string, expected string) (bool, error)
}

type HashCache interface {
	HGetAll(ctx context.Context, cacheKey string) (map[string]string, error)
	HSet(ctx context.Context, cacheKey string, values map[string]string) error
	HGetAllDel(ctx context.Context, cacheKey string) (map[string]string, error)
	HDel(ctx context.Context, cacheKey string, fields ...string) error
}

type TwoLevelCache interface {
	GetOrLoad(ctx context.Context, cacheKey string, dest any, ttl time.Duration, loader func(context.Context) (any, error)) (bool, error)
}

type PrimitiveCache interface {
	SetNX(ctx context.Context, cacheKey string, value any, ttl time.Duration) (bool, error)
	SetNXString(ctx context.Context, cacheKey string, value string, ttl time.Duration) (bool, error)
	SetNXBytes(ctx context.Context, cacheKey string, value []byte, ttl time.Duration) (bool, error)
	Incr(ctx context.Context, cacheKey string, ttl time.Duration) (int64, error)
	DeleteMany(ctx context.Context, cacheKeys ...string) error
}

type Manager interface {
	Name() string
	Layers() []string
	Builder() *key.Builder
	Health(ctx context.Context) HealthSnapshot
	KVCache
	HashCache
	TwoLevelCache
	PrimitiveCache
	CompareAndSetString(ctx context.Context, cacheKey string, expected string, replacement string, ttl time.Duration) (bool, error)
	CompareAndSetStringAndExpire(ctx context.Context, cacheKey string, expected string, replacement string, expiryKey string, ttl time.Duration) (bool, error)
	SetMaxTimestamp(ctx context.Context, cacheKey string, value time.Time, ttl time.Duration) (bool, error)
}

type manager struct {
	name      string
	builder   *key.Builder
	layerList []string
	kv        KVCache
	hash      HashCache
	twoLevel  TwoLevelCache
	primitive PrimitiveCache
	governed  GovernedCache
	healthFn  func(context.Context) HealthSnapshot
}

type ManagerOption func(*manager)

func NewManager(name string, builder *key.Builder, options ...ManagerOption) Manager {
	mgr := &manager{
		name:    name,
		builder: builder,
		healthFn: func(context.Context) HealthSnapshot {
			return HealthSnapshot{}
		},
	}
	for _, option := range options {
		if option != nil {
			option(mgr)
		}
	}
	return mgr
}

func WithKVLayer(name string, layer KVCache) ManagerOption {
	return func(mgr *manager) {
		mgr.kv = layer
		mgr.addLayer(name)
	}
}

func WithHashLayer(name string, layer HashCache) ManagerOption {
	return func(mgr *manager) {
		mgr.hash = layer
		mgr.addLayer(name)
	}
}

func WithTwoLevelLayer(name string, layer TwoLevelCache) ManagerOption {
	return func(mgr *manager) {
		mgr.twoLevel = layer
		mgr.addLayer(name)
	}
}

func WithPrimitiveLayer(name string, layer PrimitiveCache) ManagerOption {
	return func(mgr *manager) {
		mgr.primitive = layer
		mgr.addLayer(name)
	}
}

// WithGovernedLayer installs the explicit DG5-only cache surface without
// widening the general Manager contract used by unrelated callers.
func WithGovernedLayer(name string, layer GovernedCache) ManagerOption {
	return func(mgr *manager) {
		mgr.governed = layer
		mgr.addLayer(name)
	}
}

func WithHealthFunc(fn func(context.Context) HealthSnapshot) ManagerOption {
	return func(mgr *manager) {
		if fn != nil {
			mgr.healthFn = fn
		}
	}
}

func (m *manager) Name() string {
	return m.name
}

func (m *manager) Layers() []string {
	return append([]string(nil), m.layerList...)
}

func (m *manager) Builder() *key.Builder {
	return m.builder
}

func (m *manager) Health(ctx context.Context) HealthSnapshot {
	return m.healthFn(ctx)
}

func (m *manager) GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader ClassifiedLoader) (bool, error) {
	if m.governed == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.governed.GetOrLoadClassified(ctx, request, dest, loader)
}

func (m *manager) GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight ClassifiedPreflight, loader ClassifiedLoader) (bool, error) {
	if m.governed == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.governed.GetOrLoadClassifiedWithPreflight(ctx, request, dest, preflight, loader)
}

func (m *manager) MarkLocalDirty(eventID string, classes ...cachepolicy.DataClass) {
	if m.governed != nil {
		m.governed.MarkLocalDirty(eventID, classes...)
	}
}

func (m *manager) EvictLocalAndResolve(eventID string, classes ...cachepolicy.DataClass) {
	if m.governed != nil {
		m.governed.EvictLocalAndResolve(eventID, classes...)
	}
}

func (m *manager) AdvanceGeneration(ctx context.Context, eventID string, class cachepolicy.DataClass) (bool, error) {
	if m.governed == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.governed.AdvanceGeneration(ctx, eventID, class)
}

func (m *manager) SetFanoutHealthy(healthy bool) {
	if m.governed != nil {
		m.governed.SetFanoutHealthy(healthy)
	}
}

func (m *manager) SetFreshnessGate(gate cachepolicy.FreshnessGate) {
	if m.governed != nil {
		m.governed.SetFreshnessGate(gate)
	}
}

func (m *manager) RecordRejectedFanout() {
	if m.governed != nil {
		m.governed.RecordRejectedFanout()
	}
}

func (m *manager) GetOrLoadTargeted(ctx context.Context, request cachepolicy.TargetedReadRequest, dest any, loader TargetedLoader) (bool, error) {
	targeted, ok := m.governed.(TargetedGovernedCache)
	if !ok || targeted == nil {
		return false, ErrCacheLayerUnsupported
	}
	return targeted.GetOrLoadTargeted(ctx, request, dest, loader)
}
func (m *manager) MarkTargetLocalDirty(eventID string, request cachepolicy.TargetedReadRequest) {
	if targeted, ok := m.governed.(TargetedGovernedCache); ok && targeted != nil {
		targeted.MarkTargetLocalDirty(eventID, request)
	}
}
func (m *manager) EvictTargetLocalAndResolve(eventID string, request cachepolicy.TargetedReadRequest) {
	if targeted, ok := m.governed.(TargetedGovernedCache); ok && targeted != nil {
		targeted.EvictTargetLocalAndResolve(eventID, request)
	}
}
func (m *manager) AdvanceTargetGeneration(ctx context.Context, eventID string, request cachepolicy.TargetedReadRequest) (bool, error) {
	targeted, ok := m.governed.(TargetedGovernedCache)
	if !ok || targeted == nil {
		return false, ErrCacheLayerUnsupported
	}
	return targeted.AdvanceTargetGeneration(ctx, eventID, request)
}
func (m *manager) SetTargetFreshnessGate(gate cachepolicy.TargetedFreshnessGate) {
	if targeted, ok := m.governed.(TargetedGovernedCache); ok && targeted != nil {
		targeted.SetTargetFreshnessGate(gate)
	}
}

func (m *manager) GovernedStatus() GovernedStatus {
	if m.governed == nil {
		return GovernedStatus{}
	}
	return m.governed.GovernedStatus()
}

func (m *manager) AdvanceGlobalRefresh(ctx context.Context, eventID string) (bool, error) {
	refresh, ok := m.governed.(GlobalRefreshGovernedCache)
	if !ok || refresh == nil {
		return false, ErrCacheLayerUnsupported
	}
	return refresh.AdvanceGlobalRefresh(ctx, eventID)
}

func (m *manager) MarkGlobalRefreshDirty(eventID string) {
	if refresh, ok := m.governed.(GlobalRefreshGovernedCache); ok && refresh != nil {
		refresh.MarkGlobalRefreshDirty(eventID)
	}
}

func (m *manager) EvictAllGovernedLocal(eventID string) {
	if refresh, ok := m.governed.(GlobalRefreshGovernedCache); ok && refresh != nil {
		refresh.EvictAllGovernedLocal(eventID)
	}
}

func (m *manager) Get(ctx context.Context, cacheKey string, dest any) (bool, error) {
	if m.kv == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.kv.Get(ctx, cacheKey, dest)
}

func (m *manager) Set(ctx context.Context, cacheKey string, value any, ttl time.Duration) error {
	if m.kv == nil {
		return ErrCacheLayerUnsupported
	}
	return m.kv.Set(ctx, cacheKey, value, ttl)
}

func (m *manager) GetString(ctx context.Context, cacheKey string) (string, bool, error) {
	if m.kv == nil {
		return "", false, ErrCacheLayerUnsupported
	}
	return m.kv.GetString(ctx, cacheKey)
}

func (m *manager) GetBytes(ctx context.Context, cacheKey string) ([]byte, bool, error) {
	if m.kv == nil {
		return nil, false, ErrCacheLayerUnsupported
	}
	return m.kv.GetBytes(ctx, cacheKey)
}

func (m *manager) SetString(ctx context.Context, cacheKey string, value string, ttl time.Duration) error {
	if m.kv == nil {
		return ErrCacheLayerUnsupported
	}
	return m.kv.SetString(ctx, cacheKey, value, ttl)
}

func (m *manager) SetBytes(ctx context.Context, cacheKey string, value []byte, ttl time.Duration) error {
	if m.kv == nil {
		return ErrCacheLayerUnsupported
	}
	return m.kv.SetBytes(ctx, cacheKey, value, ttl)
}

func (m *manager) Delete(ctx context.Context, cacheKey string) error {
	if m.kv == nil {
		return ErrCacheLayerUnsupported
	}
	return m.kv.Delete(ctx, cacheKey)
}

func (m *manager) Exists(ctx context.Context, cacheKey string) (bool, error) {
	if m.kv == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.kv.Exists(ctx, cacheKey)
}

func (m *manager) Expire(ctx context.Context, cacheKey string, ttl time.Duration) error {
	if m.kv == nil {
		return ErrCacheLayerUnsupported
	}
	return m.kv.Expire(ctx, cacheKey, ttl)
}

func (m *manager) GetDel(ctx context.Context, cacheKey string, dest any) (bool, error) {
	if m.kv == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.kv.GetDel(ctx, cacheKey, dest)
}

func (m *manager) GetDelString(ctx context.Context, cacheKey string) (string, bool, error) {
	if m.kv == nil {
		return "", false, ErrCacheLayerUnsupported
	}
	return m.kv.GetDelString(ctx, cacheKey)
}

func (m *manager) CompareAndDelete(ctx context.Context, cacheKey string, expected any) (bool, error) {
	if m.kv == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.kv.CompareAndDelete(ctx, cacheKey, expected)
}

func (m *manager) CompareAndDeleteString(ctx context.Context, cacheKey string, expected string) (bool, error) {
	if m.kv == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.kv.CompareAndDeleteString(ctx, cacheKey, expected)
}

func (m *manager) CompareAndSetString(ctx context.Context, cacheKey string, expected string, replacement string, ttl time.Duration) (bool, error) {
	layer, ok := m.kv.(interface {
		CompareAndSetString(context.Context, string, string, string, time.Duration) (bool, error)
	})
	if !ok {
		return false, ErrCacheLayerUnsupported
	}
	return layer.CompareAndSetString(ctx, cacheKey, expected, replacement, ttl)
}

func (m *manager) CompareAndSetStringAndExpire(ctx context.Context, cacheKey string, expected string, replacement string, expiryKey string, ttl time.Duration) (bool, error) {
	layer, ok := m.kv.(interface {
		CompareAndSetStringAndExpire(context.Context, string, string, string, string, time.Duration) (bool, error)
	})
	if !ok {
		return false, ErrCacheLayerUnsupported
	}
	return layer.CompareAndSetStringAndExpire(ctx, cacheKey, expected, replacement, expiryKey, ttl)
}

func (m *manager) SetMaxTimestamp(ctx context.Context, cacheKey string, value time.Time, ttl time.Duration) (bool, error) {
	layer, ok := m.kv.(interface {
		SetMaxTimestamp(context.Context, string, time.Time, time.Duration) (bool, error)
	})
	if !ok {
		return false, ErrCacheLayerUnsupported
	}
	return layer.SetMaxTimestamp(ctx, cacheKey, value, ttl)
}

func (m *manager) HGetAll(ctx context.Context, cacheKey string) (map[string]string, error) {
	if m.hash == nil {
		return nil, ErrCacheLayerUnsupported
	}
	return m.hash.HGetAll(ctx, cacheKey)
}

func (m *manager) HSet(ctx context.Context, cacheKey string, values map[string]string) error {
	if m.hash == nil {
		return ErrCacheLayerUnsupported
	}
	return m.hash.HSet(ctx, cacheKey, values)
}

func (m *manager) HGetAllDel(ctx context.Context, cacheKey string) (map[string]string, error) {
	if m.hash == nil {
		return nil, ErrCacheLayerUnsupported
	}
	return m.hash.HGetAllDel(ctx, cacheKey)
}

func (m *manager) HDel(ctx context.Context, cacheKey string, fields ...string) error {
	if m.hash == nil {
		return ErrCacheLayerUnsupported
	}
	return m.hash.HDel(ctx, cacheKey, fields...)
}

func (m *manager) GetOrLoad(ctx context.Context, cacheKey string, dest any, ttl time.Duration, loader func(context.Context) (any, error)) (bool, error) {
	if m.twoLevel == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.twoLevel.GetOrLoad(ctx, cacheKey, dest, ttl, loader)
}

func (m *manager) SetNX(ctx context.Context, cacheKey string, value any, ttl time.Duration) (bool, error) {
	if m.primitive == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.primitive.SetNX(ctx, cacheKey, value, ttl)
}

func (m *manager) Incr(ctx context.Context, cacheKey string, ttl time.Duration) (int64, error) {
	if m.primitive == nil {
		return 0, ErrCacheLayerUnsupported
	}
	return m.primitive.Incr(ctx, cacheKey, ttl)
}

func (m *manager) SetNXString(ctx context.Context, cacheKey string, value string, ttl time.Duration) (bool, error) {
	if m.primitive == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.primitive.SetNXString(ctx, cacheKey, value, ttl)
}

func (m *manager) SetNXBytes(ctx context.Context, cacheKey string, value []byte, ttl time.Duration) (bool, error) {
	if m.primitive == nil {
		return false, ErrCacheLayerUnsupported
	}
	return m.primitive.SetNXBytes(ctx, cacheKey, value, ttl)
}

func (m *manager) DeleteMany(ctx context.Context, cacheKeys ...string) error {
	if m.primitive == nil {
		return ErrCacheLayerUnsupported
	}
	return m.primitive.DeleteMany(ctx, cacheKeys...)
}

func (m *manager) addLayer(name string) {
	if name == "" {
		return
	}
	for _, current := range m.layerList {
		if current == name {
			return
		}
	}
	m.layerList = append(m.layerList, name)
}

var (
	defaultManager Manager
	defaultMu      sync.RWMutex
)

func SetDefaultManager(manager Manager) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultManager = manager
}

func DefaultManager() Manager {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultManager
}
