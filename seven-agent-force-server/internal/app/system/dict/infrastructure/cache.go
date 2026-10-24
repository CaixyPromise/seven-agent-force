package infrastructure

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

// CacheStore is the compatibility boundary for dictionary callers. Raw type,
// item, and batch keys are intentionally disabled; only a classified DG5
// public dictionary read may use the governed two-level protocol.
type CacheStore struct {
	governed cache.GovernedCache
	enabled  bool
}

func NewCacheStore(cacheManager cache.Manager, governanceEnabled ...bool) *CacheStore {
	governed, _ := cacheManager.(cache.GovernedCache)
	enabled := len(governanceEnabled) > 0 && governanceEnabled[0]
	return &CacheStore{governed: governed, enabled: enabled && governed != nil}
}

func (s *CacheStore) ClassifiedEnabled() bool {
	return s != nil && s.enabled && s.governed != nil
}

func (s *CacheStore) GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error) {
	if !s.ClassifiedEnabled() {
		return false, nil
	}
	return s.governed.GetOrLoadClassified(ctx, request, dest, cache.ClassifiedLoader(loader))
}

// GetOrLoadClassifiedWithPreflight retains the DG5 cache boundary while
// letting the application validate source-side eligibility before L1/L2 is
// considered. The infrastructure receives only the boolean result, never a
// dictionary, actor, or business authorization rule.
func (s *CacheStore) GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight func(context.Context) (bool, error), loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error) {
	if !s.ClassifiedEnabled() {
		return false, nil
	}
	return s.governed.GetOrLoadClassifiedWithPreflight(ctx, request, dest, cache.ClassifiedPreflight(preflight), cache.ClassifiedLoader(loader))
}

func (*CacheStore) GetTypeByID(context.Context, int64) (*domain.DictType, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetTypeByID(context.Context, *domain.DictType) error { return nil }
func (*CacheStore) GetTypeByCode(context.Context, string) (*domain.DictType, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetTypeByCode(context.Context, string, *domain.DictType) error { return nil }
func (*CacheStore) GetItemsByType(context.Context, int64) ([]domain.DictItemView, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetItemsByType(context.Context, int64, []domain.DictItemView) error { return nil }
func (*CacheStore) GetItemsByCode(context.Context, string) ([]domain.DictItemView, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetItemsByCode(context.Context, string, []domain.DictItemView) error { return nil }
func (*CacheStore) GetBatch(context.Context, string) (*domain.BatchResult, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetBatch(context.Context, string, *domain.BatchResult) error { return nil }
func (*CacheStore) CurrentBatchVersion(context.Context) (int64, error)          { return 0, nil }
func (*CacheStore) BumpBatchVersion(context.Context) error                      { return nil }
func (*CacheStore) InvalidateType(context.Context, int64, string) error         { return nil }
func (*CacheStore) InvalidateItems(context.Context, int64, string) error        { return nil }
