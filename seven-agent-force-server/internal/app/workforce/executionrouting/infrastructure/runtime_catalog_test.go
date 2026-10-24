package infrastructure

import (
	"testing"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
)

func TestSortRuntimeOptionsUsesStableRecommendationOrder(t *testing.T) {
	items := []routingfacade.RuntimeOption{
		{RuntimeReleaseID: 40, SchedulingPriority: 20, AvailableSlots: 10},
		{RuntimeReleaseID: 30, SchedulingPriority: 10, AvailableSlots: 2, WaitingTaskCount: 4},
		{RuntimeReleaseID: 20, SchedulingPriority: 10, AvailableSlots: 2, WaitingTaskCount: 1},
		{RuntimeReleaseID: 10, SchedulingPriority: 10, AvailableSlots: 2, WaitingTaskCount: 1},
		{RuntimeReleaseID: 50, SchedulingPriority: 10, AvailableSlots: 1},
	}

	sortRuntimeOptions(items)

	want := []int64{20, 10, 30, 50, 40}
	for index := range want {
		if items[index].RuntimeReleaseID != want[index] {
			t.Fatalf("items[%d]=%d, want %d", index, items[index].RuntimeReleaseID, want[index])
		}
	}
}

func TestRuntimeRecommendationPrefersNewerReleaseWithoutRemovingOlderChoice(t *testing.T) {
	const oldRelease, fixedRelease int64 = 8000000000000000010, 8000000000000000011
	items := []routingfacade.RuntimeOption{
		{RuntimeReleaseID: oldRelease, SchedulingPriority: 100, AvailableSlots: 1},
		{RuntimeReleaseID: fixedRelease, SchedulingPriority: 100, AvailableSlots: 1},
	}
	sortRuntimeOptions(items)
	if len(items) != 2 || items[0].RuntimeReleaseID != fixedRelease || items[1].RuntimeReleaseID != oldRelease {
		t.Fatalf("fixed release should be the first option while preserving the old choice: %#v", items)
	}
}

func TestAttachWorkerInventoryKeepsEveryReleaseForSharedDigest(t *testing.T) {
	first := &routingfacade.RuntimeOption{RuntimeReleaseID: 10}
	second := &routingfacade.RuntimeOption{RuntimeReleaseID: 20}
	byDigest := map[string][]*routingfacade.RuntimeOption{
		"sha256:shared": {first, second},
	}

	attachWorkerInventory(byDigest, &runtimeWorkerInventory{
		id: 100, name: "worker-a", digest: "sha256:shared", maxAttempts: 4, running: 1,
	})

	for _, option := range []*routingfacade.RuntimeOption{first, second} {
		if option.AvailableSlots != 3 || len(option.Workers) != 1 || option.Workers[0].WorkerID != 100 {
			t.Fatalf("release %d worker aggregation=%#v, want shared worker with three slots", option.RuntimeReleaseID, option)
		}
	}
}
