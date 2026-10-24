package application

import (
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"testing"
	"time"
)

func TestColdBudgetPreservesAllowanceAfterSourceContainerDeadline(t *testing.T) {
	now := time.Now().UTC()
	source := wcp.ExecutionBudget{Version: 1, TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, ActiveLimitMillis: 600000, ActiveElapsedMillis: 100000, InteractionWaitingMillis: 200000, AbsoluteDeadlineUnixMillis: now.Add(-24 * time.Hour).UnixMilli()}
	target := MachineAuthority{AttemptID: 5, Generation: 6, OwnerEpoch: 7}
	got, err := ColdInteractionBudget(source, target, now)
	if err != nil || got.ActiveLimitMillis != source.ActiveLimitMillis || got.ActiveElapsedMillis != source.ActiveElapsedMillis || got.InteractionWaitingMillis != source.InteractionWaitingMillis || got.AbsoluteDeadlineUnixMillis != now.UnixMilli()+500000+400000+120000 || got.AttemptID != 5 || got.Generation != 6 || got.OwnerEpoch != 7 {
		t.Fatalf("cold allowance changed: %+v %v", got, err)
	}
	source.InteractionWaitingMillis = 700000
	got, err = ColdInteractionBudget(source, target, now)
	if err != nil || got.AbsoluteDeadlineUnixMillis != now.UnixMilli()+500000+120000 {
		t.Fatal("exhausted waiting allowance regenerated")
	}
	source.ActiveElapsedMillis = source.ActiveLimitMillis
	if _, err = ColdInteractionBudget(source, target, now); err == nil {
		t.Fatal("exhausted active budget renewed")
	}
}

func TestColdBudgetPreservesUnlimitedExecution(t *testing.T) {
	source := wcp.ExecutionBudget{Version: 1, TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, ActiveElapsedMillis: 900000}
	got, err := ColdInteractionBudget(source, MachineAuthority{AttemptID: 5, Generation: 6, OwnerEpoch: 7}, time.Now())
	if err != nil || got.ActiveLimitMillis != 0 || got.AbsoluteDeadlineUnixMillis != 0 || got.ActiveElapsedMillis != source.ActiveElapsedMillis || got.AttemptID != 5 {
		t.Fatalf("unlimited execution acquired a deadline: %+v %v", got, err)
	}
}
