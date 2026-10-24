package infrastructure

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

// CacheStore intentionally exposes the legacy method shape so callers can be
// migrated safely, but those methods no longer write generic Redis entries.
// DG5 permits only the catalogued classified-read entry point below.
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

// The following legacy methods are deliberately authoritative-cache no-ops.
// They preserve source compatibility while preventing raw/secret/configured
// values from entering an unclassified cache namespace.
func (*CacheStore) GetConfigByKey(context.Context, string) (*domain.Config, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetConfigByKey(context.Context, string, *domain.Config) error { return nil }
func (*CacheStore) GetGroupByCode(context.Context, string) (*domain.ConfigGroup, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetGroupByCode(context.Context, string, *domain.ConfigGroup) error { return nil }
func (*CacheStore) GetListByGroup(context.Context, int64) ([]domain.Config, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetListByGroup(context.Context, int64, []domain.Config) error { return nil }
func (*CacheStore) GetBatch(context.Context, string) (map[string]domain.Config, bool, error) {
	return nil, false, nil
}
func (*CacheStore) SetBatch(context.Context, string, map[string]domain.Config) error { return nil }
func (*CacheStore) CurrentBatchVersion(context.Context) (int64, error)               { return 0, nil }
func (*CacheStore) BumpBatchVersion(context.Context) error                           { return nil }
func (*CacheStore) InvalidateConfig(context.Context, string) error                   { return nil }
func (*CacheStore) InvalidateGroup(context.Context, string) error                    { return nil }
func (*CacheStore) InvalidateGroupList(context.Context, int64) error                 { return nil }
func (*CacheStore) InvalidateConfigBatch(context.Context, []domain.Config) error     { return nil }
