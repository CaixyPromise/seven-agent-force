package main

import (
	"context"
	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrivateBudgetAtomicRestoreAndTamper(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// macOS temporary roots may themselves contain a system symlink.
	dir, _ = filepath.EvalSymlinks(dir)
	store, err := openPrivateInteractionBudgets(dir, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	now := time.Now()
	store.now = func() time.Time { return now }
	s := attemptruntime.ExecutionBudgetSnapshot{Version: 1, BudgetIdentity: attemptruntime.BudgetIdentity{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4}, ActiveLimitMillis: 60000, ActiveElapsedMillis: 10000, InteractionWaitingMillis: 20000, AbsoluteDeadlineUnixMillis: now.Add(time.Minute).UnixMilli()}
	if err := store.save(s); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Second)
	restored, err := store.Restore(context.Background(), s.BudgetIdentity)
	if err != nil || restored.ActiveElapsedMillis != 15000 || restored.InteractionWaitingMillis != 20000 {
		t.Fatalf("restore %v %v", restored, err)
	}
	file := filepath.Join(dir, budgetRecordName(s.BudgetIdentity))
	if err := os.WriteFile(file, []byte(`{"snapshot":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(context.Background(), s.BudgetIdentity); err == nil {
		t.Fatal("tampered record accepted")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", file); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(context.Background(), s.BudgetIdentity); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestColdBudgetUsesServerTargetDeadlineWithoutResettingUsage(t *testing.T) {
	directory, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(directory, 0700)
	store, err := initPrivateInteractionBudgets(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	w := &worker{interactionBudgets: store}
	item := task.WorkItem{TaskID: 1, HarnessTimeoutSeconds: 60, TaskExecutionSnapshotDigest: "snapshot", ContinuationRestore: &task.ContinuationRestore{CheckpointIdentity: "checkpoint", SourceTurnID: 2, SessionLineageDigest: "session"}}
	a := task.Attempt{ID: 7, Generation: 8, OwnerEpoch: 9}
	source := wcp.ExecutionBudget{Version: 1, TaskID: 1, AttemptID: 3, Generation: 4, OwnerEpoch: 5, ActiveLimitMillis: 60000, ActiveElapsedMillis: 30000, InteractionWaitingMillis: 540000, AbsoluteDeadlineUnixMillis: time.Now().Add(-time.Hour).UnixMilli()}
	target := source
	target.AttemptID, target.Generation, target.OwnerEpoch = 7, 8, 9
	target.AbsoluteDeadlineUnixMillis = time.Now().Add(200 * time.Second).UnixMilli()
	input := wcp.RestoreInput{Manifest: wcp.Manifest{TaskID: 1, TaskTurnID: 2, AttemptID: 3, Generation: 4, CheckpointIdentity: "checkpoint", SnapshotDigest: "snapshot", SessionDigest: "session", ExecutionBudget: source}, ExecutionBudget: target}
	if err := w.acceptVerifiedInteractionBudget(context.Background(), item, a, input); err != nil {
		t.Fatal(err)
	}
	parent := attemptruntime.WithBudgetRestoreProvider(context.Background(), store.Restore)
	id := attemptruntime.BudgetIdentity{TaskID: 1, AttemptID: 7, Generation: 8, OwnerEpoch: 9}
	ctx, cancel, err := withExecutionBudget(parent, time.Minute, id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	snapshot, _ := attemptruntime.BudgetSnapshot(ctx)
	if snapshot.ActiveElapsedMillis < 30000 || snapshot.InteractionWaitingMillis != 540000 || snapshot.AbsoluteDeadlineUnixMillis != target.AbsoluteDeadlineUnixMillis {
		t.Fatalf("reset budget %#v", snapshot)
	}
	fresh, stop, err := withExecutionBudget(context.Background(), time.Minute, id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	snapshot, _ = attemptruntime.BudgetSnapshot(fresh)
	if snapshot.ActiveElapsedMillis >= 30000 || snapshot.InteractionWaitingMillis != 0 {
		t.Fatal("regular new Turn inherited interaction usage")
	}
	input.ExecutionBudget.ActiveElapsedMillis--
	if err := w.acceptVerifiedInteractionBudget(context.Background(), item, a, input); err == nil {
		t.Fatal("decreasing trusted source usage accepted")
	}
}

func TestReplacementAttemptInheritsTaskActiveExecution(t *testing.T) {
	id := attemptruntime.BudgetIdentity{TaskID: 11, AttemptID: 22, Generation: 2, OwnerEpoch: 3}
	ctx, cancel, err := withExecutionBudget(context.Background(), time.Minute, id, false, 41000)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	snapshot, err := attemptruntime.BudgetSnapshot(ctx)
	if err != nil || snapshot.ActiveElapsedMillis < 41000 || snapshot.ActiveLimitMillis != 60000 {
		t.Fatalf("replacement lost Task budget: %#v", snapshot)
	}
	if _, stop, err := withExecutionBudget(context.Background(), time.Minute, id, false, 61000); err == nil {
		stop()
		t.Fatal("exhausted Task budget accepted for replacement")
	}
}

func TestPrivateBudgetReattachRequiresExactAuthorizedOldOwner(t *testing.T) {
	directory, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(directory, 0700)
	store, err := initPrivateInteractionBudgets(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	now := time.Now()
	store.now = func() time.Time { return now }
	id := attemptruntime.BudgetIdentity{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4}
	s := attemptruntime.ExecutionBudgetSnapshot{Version: 1, BudgetIdentity: id, ActiveLimitMillis: 60000, ActiveElapsedMillis: 10000, AbsoluteDeadlineUnixMillis: now.Add(time.Minute).UnixMilli()}
	if err := store.save(s); err != nil {
		t.Fatal(err)
	}
	target := id
	target.OwnerEpoch = 5
	now = now.Add(time.Second)
	got, err := store.reattachProvider(id, target)(context.Background(), target)
	if err != nil || got.ActiveElapsedMillis != 11000 || got.AbsoluteDeadlineUnixMillis != s.AbsoluteDeadlineUnixMillis {
		t.Fatalf("reattach %v %v", got, err)
	}
	target.AttemptID++
	if _, err := store.reattachProvider(id, target)(context.Background(), target); err == nil {
		t.Fatal("different Attempt rebound")
	}
}
