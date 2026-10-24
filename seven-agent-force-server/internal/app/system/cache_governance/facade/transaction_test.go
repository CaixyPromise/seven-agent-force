package facade

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

func TestRunInvalidatedMutationCommitsWriterDirtyAndFenceTogether(t *testing.T) {
	hooks := &fakeCompletionHooks{}
	registrar := &fakeInvalidationRegistrar{enabled: true}
	boundary := hooks.boundary(false)
	err := RunInvalidatedMutation(context.Background(), boundary, hooks, registrar, cachepolicy.DataClassConfigPublicScalar, func(context.Context) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("run invalidated mutation: %v", err)
	}
	if registrar.registered != 1 || registrar.afterCommitted != 1 {
		t.Fatalf("registration/after-commit mismatch: %+v", registrar)
	}
	if registrar.lease.releaseCount != 1 {
		t.Fatalf("mutation fence release count=%d, want 1", registrar.lease.releaseCount)
	}
}

func TestRunInvalidatedMutationReleasesFenceOnOuterRollbackWithoutDirtyingWriter(t *testing.T) {
	hooks := &fakeCompletionHooks{}
	registrar := &fakeInvalidationRegistrar{enabled: true}
	commitErr := errors.New("outer commit failed")
	err := RunInvalidatedMutation(context.Background(), hooks.boundaryWithFailure(commitErr), hooks, registrar, cachepolicy.DataClassDictPublicItems, func(context.Context) (bool, error) {
		return true, nil
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("run invalidated mutation err=%v, want outer commit failure", err)
	}
	if registrar.registered != 1 || registrar.afterCommitted != 0 {
		t.Fatalf("rollback retained writer-dirty action: %+v", registrar)
	}
	if registrar.lease.releaseCount != 1 {
		t.Fatalf("rollback did not release mutation fence exactly once: %d", registrar.lease.releaseCount)
	}
}

// A successful callback return is not proof that an already-open outer
// transaction committed. DG5 must reject that boundary when it cannot install
// paired final-outcome hooks; otherwise a second instance can read an old L1
// value after the real outer commit.
func TestRunInvalidatedMutationRejectsOuterTransactionWithoutCompletionHooks(t *testing.T) {
	registrar := &fakeInvalidationRegistrar{enabled: true}
	rolledBack := false
	called := false
	boundary := TransactionBoundary(func(ctx context.Context, operation func(context.Context) error) error {
		if err := operation(ctx); err != nil {
			rolledBack = true
			return err
		}
		return nil
	})

	err := RunInvalidatedMutation(context.Background(), boundary, nil, registrar, cachepolicy.DataClassConfigPublicScalar, func(context.Context) (bool, error) {
		called = true
		return true, nil
	})
	if err == nil || !strings.Contains(err.Error(), "completion callbacks") {
		t.Fatalf("unmanaged outer transaction err=%v, want completion-hook rejection", err)
	}
	if rolledBack {
		t.Fatal("unmanaged outer transaction entered its boundary before DG5 rejected missing completion hooks")
	}
	if called {
		t.Fatal("unmanaged outer transaction executed a business write before DG5 rejected its missing completion hooks")
	}
	if registrar.afterCommitted != 0 {
		t.Fatalf("unmanaged outer transaction dirtied writer cache: %+v", registrar)
	}
	if registrar.lease.releaseCount != 0 {
		t.Fatalf("unmanaged outer transaction acquired/released a fence before preflight rejection: %d", registrar.lease.releaseCount)
	}
}

// Two sibling application mutations receive the same store-managed outer
// transaction context, not the derived context returned from the first
// RunInvalidatedMutation call. The freshness lease therefore has to be held
// in transaction-scoped state rather than only in a child context value.
func TestRunInvalidatedMutationCoalescesSiblingFencesInOneOuterTransaction(t *testing.T) {
	outer := newFakeOuterTransaction()
	registrar := &fakeInvalidationRegistrar{enabled: true, rejectReentrantFence: true}
	operation := func(context.Context) (bool, error) { return true, nil }

	if err := RunInvalidatedMutation(outer.ctx, outer.boundary, outer, registrar, cachepolicy.DataClassConfigPublicScalar, operation); err != nil {
		t.Fatalf("first sibling mutation: %v", err)
	}
	if err := RunInvalidatedMutation(outer.ctx, outer.boundary, outer, registrar, cachepolicy.DataClassConfigPublicScalar, operation); err != nil {
		t.Fatalf("second sibling mutation: %v", err)
	}
	if registrar.acquireCalls != 1 || registrar.registered != 2 || registrar.afterCommitted != 0 || registrar.lease.releaseCount != 0 {
		t.Fatalf("sibling mutations did not share an uncommitted transaction fence: acquire=%d registered=%d afterCommit=%d released=%d", registrar.acquireCalls, registrar.registered, registrar.afterCommitted, registrar.lease.releaseCount)
	}
	outer.runCommit()
	if registrar.afterCommitted != 2 || registrar.lease.releaseCount != 1 {
		t.Fatalf("outer commit did not complete both registrations and release once: afterCommit=%d released=%d", registrar.afterCommitted, registrar.lease.releaseCount)
	}
}

func TestRunInvalidatedMutationClassesExecutesOnceAndRegistersEachClass(t *testing.T) {
	hooks := &fakeCompletionHooks{}
	registrar := &fakeInvalidationRegistrar{enabled: true}
	calls := 0
	err := RunInvalidatedMutationClasses(context.Background(), hooks.boundary(false), hooks, registrar, []cachepolicy.DataClass{
		cachepolicy.DataClassDictPublicItems,
		cachepolicy.DataClassConfigPublicScalar,
		cachepolicy.DataClassDictPublicItems,
	}, func(context.Context) (bool, error) {
		calls++
		return true, nil
	})
	if err != nil {
		t.Fatalf("run invalidated mutation classes: %v", err)
	}
	if calls != 1 || registrar.registered != 2 || registrar.afterCommitted != 1 {
		t.Fatalf("operation/registration/after-commit mismatch: calls=%d registrar=%+v", calls, registrar)
	}
	if registrar.acquireCalls != 2 || registrar.lease.releaseCount != 2 {
		t.Fatalf("multi-class fence lifecycle mismatch: acquire=%d release=%d", registrar.acquireCalls, registrar.lease.releaseCount)
	}
}

func TestRunInvalidatedMutationClassesRollsBackEveryFence(t *testing.T) {
	hooks := &fakeCompletionHooks{}
	registrar := &fakeInvalidationRegistrar{enabled: true}
	commitErr := errors.New("outer commit failed")
	err := RunInvalidatedMutationClasses(context.Background(), hooks.boundaryWithFailure(commitErr), hooks, registrar, []cachepolicy.DataClass{
		cachepolicy.DataClassConfigPublicScalar,
		cachepolicy.DataClassDictPublicItems,
	}, func(context.Context) (bool, error) { return true, nil })
	if !errors.Is(err, commitErr) {
		t.Fatalf("run invalidated mutation classes err=%v, want commit failure", err)
	}
	if registrar.registered != 2 || registrar.afterCommitted != 0 || registrar.lease.releaseCount != 2 {
		t.Fatalf("rollback did not retain exact multi-class lifecycle: %+v releases=%d", registrar, registrar.lease.releaseCount)
	}
}

type fakeCompletionHooks struct {
	commit   []func()
	rollback []func()
}

type fakeOuterTransaction struct {
	fakeCompletionHooks
	ctx       context.Context
	resources map[string]any
}

func newFakeOuterTransaction() *fakeOuterTransaction {
	outer := &fakeOuterTransaction{resources: make(map[string]any)}
	outer.ctx = context.WithValue(context.Background(), fakeOuterTransactionContextKey{}, outer)
	return outer
}

type fakeOuterTransactionContextKey struct{}

func (f *fakeOuterTransaction) boundary(_ context.Context, operation func(context.Context) error) error {
	if f == nil || operation == nil {
		return errors.New("fake outer transaction boundary is invalid")
	}
	return operation(f.ctx)
}

func (f *fakeOuterTransaction) runCommit() {
	if f == nil {
		return
	}
	callbacks := append([]func(){}, f.commit...)
	f.commit = nil
	f.rollback = nil
	f.resources = make(map[string]any)
	for _, callback := range callbacks {
		callback()
	}
}

func (f *fakeOuterTransaction) GetOrCreateTransactionResource(_ context.Context, key string, factory func() (any, error)) (any, bool, bool, error) {
	if f == nil || factory == nil {
		return nil, false, false, errors.New("fake outer transaction resource factory is invalid")
	}
	if existing, ok := f.resources[key]; ok {
		return existing, false, true, nil
	}
	resource, err := factory()
	if err != nil {
		return nil, false, true, err
	}
	f.resources[key] = resource
	return resource, true, true, nil
}

func (f *fakeOuterTransaction) DeleteTransactionResource(_ context.Context, key string, expected any) {
	if f == nil {
		return
	}
	if current, ok := f.resources[key]; ok && current == expected {
		delete(f.resources, key)
	}
}

func (f *fakeCompletionHooks) RegisterAfterCommit(_ context.Context, callback func()) bool {
	f.commit = append(f.commit, callback)
	return true
}

func (f *fakeCompletionHooks) RegisterAfterRollback(_ context.Context, callback func()) bool {
	f.rollback = append(f.rollback, callback)
	return true
}

func (f *fakeCompletionHooks) boundary(fail bool) TransactionBoundary {
	return func(ctx context.Context, operation func(context.Context) error) error {
		if err := operation(ctx); err != nil {
			for _, callback := range f.rollback {
				callback()
			}
			return err
		}
		if fail {
			for _, callback := range f.rollback {
				callback()
			}
			return errors.New("forced transaction failure")
		}
		for _, callback := range f.commit {
			callback()
		}
		return nil
	}
}

func (f *fakeCompletionHooks) boundaryWithFailure(commitErr error) TransactionBoundary {
	return func(ctx context.Context, operation func(context.Context) error) error {
		if err := operation(ctx); err != nil {
			return err
		}
		for _, callback := range f.rollback {
			callback()
		}
		return commitErr
	}
}

type fakeInvalidationRegistrar struct {
	enabled              bool
	registered           int
	afterCommitted       int
	acquireCalls         int
	rejectReentrantFence bool
	lease                fakeFreshnessLease
}

func (f *fakeInvalidationRegistrar) Enabled() bool { return f.enabled }

func (f *fakeInvalidationRegistrar) Register(_ context.Context, dataClass cachepolicy.DataClass) (Registration, error) {
	f.registered++
	return Registration{EventID: "event", DataClass: dataClass}, nil
}

func (f *fakeInvalidationRegistrar) AfterCommit(_ context.Context, _ ...Registration) {
	f.afterCommitted++
}

func (f *fakeInvalidationRegistrar) AcquireMutationFence(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	f.acquireCalls++
	if f.rejectReentrantFence && f.acquireCalls > 1 {
		return nil, errors.New("non-reentrant mutation fence acquired twice")
	}
	return &f.lease, nil
}

type fakeFreshnessLease struct{ releaseCount int }

func (*fakeFreshnessLease) Trusted() bool { return true }
func (f *fakeFreshnessLease) Release()    { f.releaseCount++ }

var _ InvalidationRegistrar = (*fakeInvalidationRegistrar)(nil)
var _ CompletionCallbacks = (*fakeCompletionHooks)(nil)
var _ CompletionCallbacks = (*fakeOuterTransaction)(nil)
var _ TransactionResourceRegistry = (*fakeOuterTransaction)(nil)
