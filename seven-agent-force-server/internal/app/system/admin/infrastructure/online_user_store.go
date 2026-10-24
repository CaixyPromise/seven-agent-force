package infrastructure

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
)

const (
	onlineUserKeyPrefix = "auth:online:detail:"
	onlineUserTTL       = 24 * time.Hour
)

type OnlineUserStateStore struct {
	cache cache.Manager
}

func NewOnlineUserStateStore(cacheManager cache.Manager) *OnlineUserStateStore {
	return &OnlineUserStateStore{cache: cacheManager}
}

func (s *OnlineUserStateStore) GetByUserID(ctx context.Context, userID int64) (*domain.OnlineUser, bool, error) {
	if s == nil || s.cache == nil || userID <= 0 {
		return nil, false, nil
	}
	var item domain.OnlineUser
	hit, err := s.cache.Get(ctx, s.key(userID), &item)
	if !hit || err != nil {
		return nil, hit, err
	}
	return &item, true, nil
}

func (s *OnlineUserStateStore) Save(ctx context.Context, userID int64, item *domain.OnlineUser) error {
	if s == nil || s.cache == nil || userID <= 0 || item == nil {
		return nil
	}
	return s.cache.Set(ctx, s.key(userID), item, onlineUserTTL)
}

func (s *OnlineUserStateStore) Delete(ctx context.Context, userID int64) error {
	if s == nil || s.cache == nil || userID <= 0 {
		return nil
	}
	return s.cache.Delete(ctx, s.key(userID))
}

func (s *OnlineUserStateStore) key(userID int64) string {
	return onlineUserKeyPrefix + strings.TrimSpace(strconv.FormatInt(userID, 10))
}
