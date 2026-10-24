package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

func TestCleanupFencesCapacityUntilVerifiedRetry(t *testing.T) {
	j, err := resourcejournal.Open(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	w := &worker{cleanupJournal: j}
	w.consumerState.Store("CONSUMING")
	w.cleanupReconciling.Store(true)
	if w.currentConsumerState() != "STOPPED" {
		t.Fatal("startup reconciliation exposed capacity")
	}
	w.cleanupReconciling.Store(false)
	r, err := j.Begin("container", "owned", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = j.Cleanup(r.Key, func(context.Context, resourcejournal.Resource) error { return errors.New("daemon offline") })
	if w.currentConsumerState() != "STOPPED" {
		t.Fatal("failed cleanup released capacity")
	}
	// A server directive cannot override the local resource reservation.
	w.consumerState.Store("CONSUMING")
	if w.currentConsumerState() != "STOPPED" {
		t.Fatal("directive bypassed cleanup")
	}
	if err = j.Cleanup(r.Key, func(context.Context, resourcejournal.Resource) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if w.currentConsumerState() != "CONSUMING" {
		t.Fatal("verified cleanup did not restore capacity")
	}
}
