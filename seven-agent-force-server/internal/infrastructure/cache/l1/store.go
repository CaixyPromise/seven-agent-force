package l1

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/dgraph-io/ristretto"
)

type Store struct {
	cache      *ristretto.Cache
	enabled    bool
	defaultTTL time.Duration
	mu         sync.Mutex
	// namespaceEpochs is intentionally one small counter per registered
	// namespace, never a set of every cached key. An invalidation advances the
	// epoch, making all older entries unreachable; Ristretto's own TTL/cost
	// eviction then reclaims them. This avoids turning per-scope DG5 reads into
	// an unbounded metadata map.
	namespaceEpochs map[string]uint64
}

func NewStore(cfg config.CacheConfig) (*Store, error) {
	if !cfg.L1Enabled() {
		return &Store{}, nil
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: cfg.L1.NumCounters,
		MaxCost:     cfg.L1.MaxCost,
		BufferItems: cfg.L1.BufferItems,
	})
	if err != nil {
		return nil, err
	}
	return &Store{
		cache:           cache,
		enabled:         true,
		defaultTTL:      cfg.L1.DefaultTTL,
		namespaceEpochs: make(map[string]uint64),
	}, nil
}

func (s *Store) Enabled() bool {
	return s != nil && s.enabled && s.cache != nil
}

func (s *Store) DefaultTTL() time.Duration {
	if s == nil {
		return 0
	}
	return s.defaultTTL
}

func (s *Store) Get(key string) ([]byte, bool) {
	if !s.Enabled() {
		return nil, false
	}
	value, ok := s.cache.Get(key)
	if !ok {
		return nil, false
	}
	payload, ok := value.([]byte)
	if !ok {
		return nil, false
	}
	result := append([]byte(nil), payload...)
	return result, true
}

func (s *Store) Set(key string, payload []byte, ttl time.Duration) {
	if !s.Enabled() {
		return
	}
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	copied := append([]byte(nil), payload...)
	cost := int64(len(copied))
	if cost <= 0 {
		cost = 1
	}
	if ttl > 0 {
		s.cache.SetWithTTL(key, copied, cost, ttl)
	} else {
		s.cache.Set(key, copied, cost)
	}
	s.cache.Wait()
}

// SetInNamespace stores under the namespace's current local epoch. It is not
// a wildcard facility and does not record one metadata entry per key.
func (s *Store) SetInNamespace(namespace, key string, payload []byte, ttl time.Duration) {
	if !s.Enabled() {
		return
	}
	namespace = strings.TrimSpace(namespace)
	key = strings.TrimSpace(key)
	if namespace == "" || key == "" {
		return
	}
	s.mu.Lock()
	epoch, known := s.namespaceEpochs[namespace]
	if !known {
		s.namespaceEpochs[namespace] = epoch
	}
	s.mu.Unlock()
	s.Set(s.namespacedKey(namespace, epoch, key), payload, ttl)
}

// GetInNamespace reads only the current epoch. A key written before a class
// invalidation is therefore never returned even if the underlying Ristretto
// entry has not yet been reclaimed.
func (s *Store) GetInNamespace(namespace, key string) ([]byte, bool) {
	if !s.Enabled() {
		return nil, false
	}
	namespace = strings.TrimSpace(namespace)
	key = strings.TrimSpace(key)
	if namespace == "" || key == "" {
		return nil, false
	}
	s.mu.Lock()
	epoch := s.namespaceEpochs[namespace]
	s.mu.Unlock()
	return s.Get(s.namespacedKey(namespace, epoch, key))
}

func (s *Store) Delete(keys ...string) {
	if !s.Enabled() {
		return
	}
	for _, key := range keys {
		s.cache.Del(key)
	}
}

// DeleteNamespace advances only this namespace's epoch. It never scans
// Ristretto, so it cannot become an unbounded global cache-clearing backdoor.
func (s *Store) DeleteNamespace(namespace string) {
	if !s.Enabled() {
		return
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.namespaceEpochs[namespace]++
}

func (s *Store) Close() {
	if !s.Enabled() {
		return
	}
	s.cache.Close()
	s.mu.Lock()
	s.namespaceEpochs = nil
	s.mu.Unlock()
}

func (s *Store) namespacedKey(namespace string, epoch uint64, key string) string {
	return "l1ns:" + namespace + ":" + strconv.FormatUint(epoch, 10) + ":" + key
}
