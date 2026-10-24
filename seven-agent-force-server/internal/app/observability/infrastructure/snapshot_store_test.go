package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/observability/domain"
)

func TestSnapshotStoreMemoryFallbackListLatestAndTrim(t *testing.T) {
	store := NewSnapshotStore(nil)
	base := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()

	for i := 0; i < snapshotMaxPoints+4; i++ {
		if err := store.Append(ctx, " SSO ", domain.RuntimeSnapshot{
			CapturedAt:   base.Add(time.Duration(i) * time.Minute),
			RequestCount: int64(i),
		}); err != nil {
			t.Fatalf("Append(%d) error = %v", i, err)
		}
	}

	latest, err := store.Latest(ctx, "sso")
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if latest == nil || latest.RequestCount != snapshotMaxPoints+3 {
		t.Fatalf("unexpected latest snapshot: %+v", latest)
	}

	items, err := store.ListBetween(ctx, "sso", base, base.Add(time.Duration(snapshotMaxPoints+4)*time.Minute))
	if err != nil {
		t.Fatalf("ListBetween() error = %v", err)
	}
	if len(items) != snapshotMaxPoints {
		t.Fatalf("expected trimmed snapshot count %d, got %d", snapshotMaxPoints, len(items))
	}
	if items[0].RequestCount != 4 || items[len(items)-1].RequestCount != snapshotMaxPoints+3 {
		t.Fatalf("unexpected sorted/trimmed snapshots first=%+v last=%+v", items[0], items[len(items)-1])
	}
}

func TestSnapshotStoreIgnoresBlankPlatform(t *testing.T) {
	store := NewSnapshotStore(nil)
	if err := store.Append(context.Background(), " ", domain.RuntimeSnapshot{CapturedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("Append(blank) error = %v", err)
	}
	latest, err := store.Latest(context.Background(), " ")
	if err != nil {
		t.Fatalf("Latest(blank) error = %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil latest for blank platform, got %+v", latest)
	}
}
