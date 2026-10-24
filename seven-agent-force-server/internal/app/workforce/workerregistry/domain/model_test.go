package domain

import "testing"

func TestQueueForUsesBoundedWorkerCapacity(t *testing.T) {
	if got := QueueFor(1001, 7); got.MaxPrefetch != 7 || got.Queue != "seven.workforce.worker.1001.tasks" {
		t.Fatalf("unexpected queue contract: %+v", got)
	}
	if got := QueueFor(1001, 100).MaxPrefetch; got != 64 {
		t.Fatalf("prefetch must be capped at 64, got %d", got)
	}
}
