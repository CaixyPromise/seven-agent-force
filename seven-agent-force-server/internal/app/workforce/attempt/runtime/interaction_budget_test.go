package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestInteractionBudgetClockBoundsWaitingAndCapture(t *testing.T) {
	now := time.Unix(1000, 0)
	b := &InteractionBudget{now: func() time.Time { return now }, started: now, last: now, absolute: now.Add(13 * time.Minute), limit: time.Minute}
	now = now.Add(10 * time.Second)
	b.questionRound, b.questionSession, b.questionUntil = "round:1", 1, now.Add(time.Hour)
	b.observe(InteractionExecutionState{Waiting: true, RoundKey: "round:1", SessionGeneration: 1})
	for n := 0; n < 600; n++ {
		now = now.Add(time.Second)
		b.observe(InteractionExecutionState{Waiting: true, RoundKey: "round:1", SessionGeneration: 1})
	}
	if b.active != 10*time.Second || b.waiting != 10*time.Minute {
		t.Fatalf("active=%v wait=%v", b.active, b.waiting)
	}
	now = now.Add(50 * time.Second)
	if !b.expired() {
		t.Fatal("wait extended active after cumulative cap")
	}
	b.observe(InteractionExecutionState{State: "PAUSING_PREPARE"})
	if b.expired() {
		t.Fatal("capture allowance absent")
	}
	now = now.Add(2 * time.Minute)
	if !b.expired() {
		t.Fatal("capture unbounded")
	}
}

func TestInteractionBudgetStaleOrUnboundQuestionDoesNotPause(t *testing.T) {
	now := time.Unix(1000, 0)
	b := &InteractionBudget{now: func() time.Time { return now }, started: now, last: now, absolute: now.Add(time.Hour), limit: time.Minute}
	b.observe(InteractionExecutionState{Waiting: true})
	now = now.Add(5 * time.Second)
	b.expired()
	if b.waiting != 0 {
		t.Fatal("unbound observation paused")
	}
	b.questionRound, b.questionSession, b.questionUntil = "round:1", 1, now.Add(time.Hour)
	b.observe(InteractionExecutionState{Waiting: true, RoundKey: "round:1", SessionGeneration: 1})
	now = now.Add(5 * time.Second)
	b.expired()
	if b.waiting != time.Second || b.active != 9*time.Second {
		t.Fatal("stale question gave unbounded credit")
	}
}

func TestBudgetTrustedRestorePreservesUsageAndDeadline(t *testing.T) {
	id := BudgetIdentity{1, 2, 3, 4}
	s := ExecutionBudgetSnapshot{Version: 1, BudgetIdentity: id, ActiveLimitMillis: 60000, ActiveElapsedMillis: 30000, InteractionWaitingMillis: 42000, AbsoluteDeadlineUnixMillis: time.Now().Add(time.Minute).UnixMilli()}
	if _, _, err := ExecutionBudgetContext(context.Background(), time.Minute, id, true); err == nil {
		t.Fatal("missing evidence accepted")
	}
	ctx := WithBudgetRestoreProvider(context.Background(), func(context.Context, BudgetIdentity) (ExecutionBudgetSnapshot, error) { return s, nil })
	ctx, cancel, err := ExecutionBudgetContext(ctx, time.Minute, id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	child, stop, err := ExecutionBudgetContext(ctx, time.Minute, id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	got, err := BudgetSnapshot(child)
	if err != nil || got.ActiveElapsedMillis < 30000 || got.InteractionWaitingMillis != 42000 || got.AbsoluteDeadlineUnixMillis != s.AbsoluteDeadlineUnixMillis {
		t.Fatalf("restored %#v %v", got, err)
	}
	body, _ := json.Marshal(got)
	var wire map[string]any
	_ = json.Unmarshal(body, &wire)
	if _, ok := wire["activeElapsedMillis"].(string); !ok {
		t.Fatal("counter not decimalstring")
	}
	id.OwnerEpoch++
	if _, _, err := ExecutionBudgetContext(ctx, time.Minute, id, true); err == nil {
		t.Fatal("identity rebound")
	}
}

func TestUnlimitedExecutionBudgetSurvivesRestore(t *testing.T) {
	id := BudgetIdentity{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4}
	ctx, cancel, err := ExecutionBudgetContext(context.Background(), 0, id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		t.Fatal("unlimited execution acquired a deadline")
	}
	snapshot, err := BudgetSnapshot(ctx)
	if err != nil || snapshot.ActiveLimitMillis != 0 || snapshot.AbsoluteDeadlineUnixMillis != 0 {
		t.Fatalf("unlimited snapshot = %+v, %v", snapshot, err)
	}
	parent := WithBudgetRestoreProvider(context.Background(), func(context.Context, BudgetIdentity) (ExecutionBudgetSnapshot, error) { return snapshot, nil })
	restored, stop, err := ExecutionBudgetContext(parent, 0, id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if _, hasDeadline := restored.Deadline(); hasDeadline {
		t.Fatal("unlimited restored execution acquired a deadline")
	}
}

func TestBudgetConcurrentSnapshotAndObservation(t *testing.T) {
	ctx, cancel, err := ExecutionBudgetContext(context.Background(), time.Minute, BudgetIdentity{1, 2, 3, 4}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	b := ctx.Value(interactionBudgetKey{}).(*InteractionBudget)
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.observe(InteractionExecutionState{Waiting: true, RoundKey: "round:1", SessionGeneration: 1})
				_, _ = BudgetSnapshot(ctx)
				b.expired()
			}
		}()
	}
	wg.Wait()
}
