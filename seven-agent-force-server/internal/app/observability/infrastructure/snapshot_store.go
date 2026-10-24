package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/observability/domain"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	redisclient "github.com/redis/go-redis/v9"
)

const (
	snapshotKeyPrefix = "observability:snapshot:"
	snapshotRetention = 8 * 24 * time.Hour
	snapshotMaxPoints = 4096
)

type SnapshotStore struct {
	client redisclient.UniversalClient
	mu     sync.Mutex
	memory map[string][]domain.RuntimeSnapshot
}

func NewSnapshotStore(provider cacheinfra.Provider) *SnapshotStore {
	store := &SnapshotStore{memory: make(map[string][]domain.RuntimeSnapshot)}
	if provider != nil && provider.Configured() {
		store.client = provider.Client()
	}
	return store
}

func (s *SnapshotStore) Append(ctx context.Context, platformKey string, snapshot domain.RuntimeSnapshot) error {
	key := buildSnapshotKey(platformKey)
	if key == "" {
		return nil
	}
	if s != nil && s.client != nil {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		pipe := s.client.TxPipeline()
		pipe.LPush(ctx, key, payload)
		pipe.LTrim(ctx, key, 0, snapshotMaxPoints-1)
		pipe.Expire(ctx, key, snapshotRetention)
		_, err = pipe.Exec(ctx)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]domain.RuntimeSnapshot{snapshot}, s.memory[key]...)
	if len(items) > snapshotMaxPoints {
		items = items[:snapshotMaxPoints]
	}
	s.memory[key] = items
	return nil
}

func (s *SnapshotStore) ListBetween(ctx context.Context, platformKey string, startTime, endTime time.Time) ([]domain.RuntimeSnapshot, error) {
	key := buildSnapshotKey(platformKey)
	if key == "" {
		return []domain.RuntimeSnapshot{}, nil
	}
	var items []domain.RuntimeSnapshot
	if s != nil && s.client != nil {
		payloads, err := s.client.LRange(ctx, key, 0, snapshotMaxPoints-1).Result()
		if err != nil {
			return nil, fmt.Errorf("list observability snapshots: %w", err)
		}
		items = decodeSnapshots(payloads)
	} else {
		s.mu.Lock()
		items = append([]domain.RuntimeSnapshot(nil), s.memory[key]...)
		s.mu.Unlock()
	}
	result := make([]domain.RuntimeSnapshot, 0, len(items))
	for _, item := range items {
		if item.CapturedAt.Before(startTime) || item.CapturedAt.After(endTime) {
			continue
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CapturedAt.Before(result[j].CapturedAt)
	})
	return result, nil
}

func (s *SnapshotStore) Latest(ctx context.Context, platformKey string) (*domain.RuntimeSnapshot, error) {
	key := buildSnapshotKey(platformKey)
	if key == "" {
		return nil, nil
	}
	if s != nil && s.client != nil {
		payloads, err := s.client.LRange(ctx, key, 0, 0).Result()
		if err != nil {
			return nil, fmt.Errorf("latest observability snapshot: %w", err)
		}
		items := decodeSnapshots(payloads)
		if len(items) == 0 {
			return nil, nil
		}
		return &items[0], nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.memory[key]
	if len(items) == 0 {
		return nil, nil
	}
	item := items[0]
	return &item, nil
}

func buildSnapshotKey(platformKey string) string {
	value := strings.ToLower(strings.TrimSpace(platformKey))
	if value == "" {
		return ""
	}
	return snapshotKeyPrefix + value
}

func decodeSnapshots(payloads []string) []domain.RuntimeSnapshot {
	result := make([]domain.RuntimeSnapshot, 0, len(payloads))
	for _, payload := range payloads {
		var item domain.RuntimeSnapshot
		if err := json.Unmarshal([]byte(payload), &item); err == nil && !item.CapturedAt.IsZero() {
			result = append(result, item)
		}
	}
	return result
}
