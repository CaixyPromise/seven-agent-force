package l1

import (
	"fmt"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestDeleteNamespaceEvictsOnlyRegisteredKeys(t *testing.T) {
	store, err := NewStore(config.CacheConfig{
		Enabled: true,
		L1: config.CacheL1Config{
			Enabled:     true,
			MaxCost:     1024,
			NumCounters: 100,
			BufferItems: 64,
			DefaultTTL:  time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(store.Close)

	store.SetInNamespace("config", "cache:config", []byte("config"), time.Minute)
	store.SetInNamespace("dict", "cache:dict", []byte("dict"), time.Minute)
	store.Set("cache:unmanaged", []byte("unmanaged"), time.Minute)

	store.DeleteNamespace("config")
	if _, ok := store.GetInNamespace("config", "cache:config"); ok {
		t.Fatal("target namespace key remained in L1")
	}
	if got, ok := store.GetInNamespace("dict", "cache:dict"); !ok || string(got) != "dict" {
		t.Fatalf("unrelated namespace was evicted: hit=%v payload=%q", ok, got)
	}
	if got, ok := store.Get("cache:unmanaged"); !ok || string(got) != "unmanaged" {
		t.Fatalf("unmanaged key was evicted: hit=%v payload=%q", ok, got)
	}
}

func TestNamespaceEpochDoesNotRetainPerKeyMetadata(t *testing.T) {
	store, err := NewStore(config.CacheConfig{
		Enabled: true,
		L1: config.CacheL1Config{
			Enabled:     true,
			MaxCost:     1024 * 1024,
			NumCounters: 1000,
			BufferItems: 64,
			DefaultTTL:  time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(store.Close)

	for index := 0; index < 1000; index++ {
		store.SetInNamespace("config", fmt.Sprintf("key-%d", index), []byte("value"), time.Second)
	}
	store.mu.Lock()
	if len(store.namespaceEpochs) != 1 {
		store.mu.Unlock()
		t.Fatalf("namespace metadata grew per key: %d namespaces", len(store.namespaceEpochs))
	}
	store.mu.Unlock()

	store.DeleteNamespace("config")
	if _, ok := store.GetInNamespace("config", "key-999"); ok {
		t.Fatal("previous namespace epoch remained readable")
	}
}
