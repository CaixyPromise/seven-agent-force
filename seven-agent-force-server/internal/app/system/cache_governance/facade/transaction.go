package facade

import (
	"context"
	"errors"
	"sort"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

// TransactionBoundary lets a domain application service retain ownership of
// its normal or consistent transaction policy while sharing the DG5 durable
// registration/completion protocol with other application services.
type TransactionBoundary func(context.Context, func(context.Context) error) error

// CompletionCallbacks is implemented by store-managed transactions. Both
// outcomes are needed because a mutation fence acquired before an outer
// transaction must be released on rollback as well as after commit.
type CompletionCallbacks interface {
	RegisterAfterCommit(ctx context.Context, callback func()) bool
	RegisterAfterRollback(ctx context.Context, callback func()) bool
}

// TransactionResourceRegistry lets a store-managed transaction share one
// non-database resource between sibling application mutations. The facade
// owns only this narrow contract; datasource infrastructure chooses how the
// resource is scoped to the actual transaction and never imports application
// or domain code.
type TransactionResourceRegistry interface {
	GetOrCreateTransactionResource(ctx context.Context, key string, factory func() (any, error)) (value any, created bool, available bool, err error)
	DeleteTransactionResource(ctx context.Context, key string, expected any)
}

type mutationFenceContextKey struct{}

type mutationFenceResource struct {
	lease cachepolicy.FreshnessLease
}

type mutationFenceHandle struct {
	ctx      context.Context
	lease    cachepolicy.FreshnessLease
	owned    bool
	registry TransactionResourceRegistry
	key      string
	resource any
}

func (h mutationFenceHandle) release() {
	if !h.owned || h.lease == nil {
		return
	}
	h.lease.Release()
	if h.registry != nil && h.resource != nil {
		h.registry.DeleteTransactionResource(h.ctx, h.key, h.resource)
	}
}

// RunInvalidatedMutation is the one shared application-level protocol for a
// config/dict mutation: acquire the cross-instance freshness fence, execute
// business work plus outbox registration in the same transaction, dirty the
// local writer only after real commit, and release the fence on either final
// transaction outcome. It contains no config/dict business rule.
func RunInvalidatedMutation(
	ctx context.Context,
	boundary TransactionBoundary,
	callbacks any,
	registrar InvalidationRegistrar,
	dataClass cachepolicy.DataClass,
	operation func(context.Context) (bool, error),
) error {
	if boundary == nil || operation == nil {
		return errors.New("cache invalidation transaction boundary and operation are required")
	}
	if registrar == nil || !registrar.Enabled() {
		return boundary(ctx, func(txCtx context.Context) error {
			_, err := operation(txCtx)
			return err
		})
	}
	hooks, ok := callbacks.(CompletionCallbacks)
	if !ok || hooks == nil {
		// A successful return from an externally-owned/nested transaction is
		// not proof of a commit. Reject before acquiring a fence or executing
		// business work so DG5 cannot persist a source write without a paired
		// durable invalidation and final-outcome release hook.
		return errors.New("cache invalidation completion callbacks are required when cache governance is enabled")
	}

	fenceHandle, err := acquireMutationFence(ctx, callbacks, registrar, dataClass)
	if err != nil {
		return err
	}
	releaseFence := fenceHandle.owned
	defer func() {
		if releaseFence {
			fenceHandle.release()
		}
	}()

	var registration Registration
	registered := false
	deferredCompletion := false
	err = boundary(fenceHandle.ctx, func(txCtx context.Context) error {
		changed, operationErr := operation(txCtx)
		if operationErr != nil || !changed {
			return operationErr
		}
		registration, operationErr = registrar.Register(txCtx, dataClass)
		if operationErr != nil {
			return operationErr
		}
		registered = true
		// Install rollback first. If registering the commit callback then fails,
		// returning this operation error makes the owning transaction roll back
		// and releases the cross-instance fence. A plain return from a nested or
		// externally-owned transaction is not evidence of a real commit.
		rollback := func() {
			if fenceHandle.owned {
				fenceHandle.release()
			}
		}
		if !hooks.RegisterAfterRollback(txCtx, rollback) {
			return errors.New("cache invalidation rollback completion hook is unavailable")
		}
		// The rollback callback now owns the first acquired lease if a later
		// completion-hook registration fails. This prevents a second sibling
		// from seeing a released transaction-scoped resource.
		if fenceHandle.owned {
			releaseFence = false
		}
		commit := func() { registrar.AfterCommit(context.Background(), registration) }
		if fenceHandle.owned {
			commit = func() {
				registrar.AfterCommit(context.Background(), registration)
				fenceHandle.release()
			}
		}
		if !hooks.RegisterAfterCommit(txCtx, commit) {
			return errors.New("cache invalidation commit completion hook is unavailable")
		}
		deferredCompletion = true
		// Once paired real transaction-outcome callbacks own the lease, the
		// outer defer must not release it again on a commit error or rollback.
		// FreshnessLease is idempotent defensively, but this keeps completion
		// ownership exact and testable.
		return nil
	})
	if err != nil {
		return err
	}
	if registered && !deferredCompletion {
		registrar.AfterCommit(context.Background(), registration)
	}
	return nil
}

// RunInvalidatedMutationClasses applies the same durable, zero-stale mutation
// protocol to a bounded set of catalogued data classes. It executes operation
// exactly once and appends every class invalidation in that operation's
// transaction. Callers use it where one authoritative write affects multiple
// independently-versioned cache projections (for example, authorization
// context and menu snapshots); they must not nest RunInvalidatedMutation.
func RunInvalidatedMutationClasses(
	ctx context.Context,
	boundary TransactionBoundary,
	callbacks any,
	registrar InvalidationRegistrar,
	classes []cachepolicy.DataClass,
	operation func(context.Context) (bool, error),
) error {
	if boundary == nil || operation == nil {
		return errors.New("cache invalidation transaction boundary and operation are required")
	}
	classes, err := normalizedDataClasses(classes)
	if err != nil {
		return err
	}
	if registrar == nil || !registrar.Enabled() {
		return boundary(ctx, func(txCtx context.Context) error {
			_, operationErr := operation(txCtx)
			return operationErr
		})
	}
	hooks, ok := callbacks.(CompletionCallbacks)
	if !ok || hooks == nil {
		return errors.New("cache invalidation completion callbacks are required when cache governance is enabled")
	}

	// Acquire every class fence before the source mutation. This makes a
	// multi-projection write one freshness boundary rather than a sequence in
	// which a peer can observe only part of the committed authorization state.
	fenceCtx := ctx
	handles := make([]mutationFenceHandle, 0, len(classes))
	for _, dataClass := range classes {
		handle, acquireErr := acquireMutationFence(fenceCtx, callbacks, registrar, dataClass)
		if acquireErr != nil {
			for index := len(handles) - 1; index >= 0; index-- {
				handles[index].release()
			}
			return acquireErr
		}
		handles = append(handles, handle)
		fenceCtx = handle.ctx
	}
	releaseFences := true
	releaseAll := func() {
		for index := len(handles) - 1; index >= 0; index-- {
			handles[index].release()
		}
	}
	defer func() {
		if releaseFences {
			releaseAll()
		}
	}()

	var registrations []Registration
	deferredCompletion := false
	err = boundary(fenceCtx, func(txCtx context.Context) error {
		changed, operationErr := operation(txCtx)
		if operationErr != nil || !changed {
			return operationErr
		}
		registrations = make([]Registration, 0, len(classes))
		for _, dataClass := range classes {
			registration, registerErr := registrar.Register(txCtx, dataClass)
			if registerErr != nil {
				return registerErr
			}
			registrations = append(registrations, registration)
		}
		if !hooks.RegisterAfterRollback(txCtx, releaseAll) {
			return errors.New("cache invalidation rollback completion hook is unavailable")
		}
		// Paired final-outcome hooks now own every acquired fence. A nested
		// sibling may share an outer transaction resource, but only the handle
		// that created that resource releases it.
		releaseFences = false
		if !hooks.RegisterAfterCommit(txCtx, func() {
			registrar.AfterCommit(context.Background(), registrations...)
			releaseAll()
		}) {
			return errors.New("cache invalidation commit completion hook is unavailable")
		}
		deferredCompletion = true
		return nil
	})
	if err != nil {
		return err
	}
	if len(registrations) > 0 && !deferredCompletion {
		registrar.AfterCommit(context.Background(), registrations...)
	}
	return nil
}

func normalizedDataClasses(classes []cachepolicy.DataClass) ([]cachepolicy.DataClass, error) {
	if len(classes) == 0 {
		return nil, errors.New("cache invalidation data classes are required")
	}
	unique := make(map[cachepolicy.DataClass]struct{}, len(classes))
	for _, dataClass := range classes {
		if dataClass == "" {
			return nil, errors.New("cache invalidation data class is required")
		}
		unique[dataClass] = struct{}{}
	}
	result := make([]cachepolicy.DataClass, 0, len(unique))
	for dataClass := range unique {
		result = append(result, dataClass)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func acquireMutationFence(ctx context.Context, callbacks any, registrar InvalidationRegistrar, dataClass cachepolicy.DataClass) (mutationFenceHandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	active, _ := ctx.Value(mutationFenceContextKey{}).(map[cachepolicy.DataClass]struct{})
	if active != nil {
		if _, alreadyHeld := active[dataClass]; alreadyHeld {
			return mutationFenceHandle{ctx: ctx}, nil
		}
	}
	if registry, ok := callbacks.(TransactionResourceRegistry); ok && registry != nil {
		key := "cache-governance:mutation-fence:" + string(dataClass)
		resource, created, available, err := registry.GetOrCreateTransactionResource(ctx, key, func() (any, error) {
			fence, fenceErr := registrar.AcquireMutationFence(ctx, dataClass)
			if fenceErr != nil {
				return nil, fenceErr
			}
			if fence == nil {
				return nil, errors.New("cache invalidation mutation fence is unavailable")
			}
			return &mutationFenceResource{lease: fence}, nil
		})
		if err != nil {
			return mutationFenceHandle{ctx: ctx}, err
		}
		if available {
			fenceResource, valid := resource.(*mutationFenceResource)
			if !valid || fenceResource == nil || fenceResource.lease == nil {
				return mutationFenceHandle{ctx: ctx}, errors.New("cache invalidation transaction fence resource is invalid")
			}
			next := copyMutationFenceClasses(active, dataClass)
			return mutationFenceHandle{
				ctx:      context.WithValue(ctx, mutationFenceContextKey{}, next),
				lease:    fenceResource.lease,
				owned:    created,
				registry: registry,
				key:      key,
				resource: resource,
			}, nil
		}
	}
	fence, err := registrar.AcquireMutationFence(ctx, dataClass)
	if err != nil {
		return mutationFenceHandle{ctx: ctx}, err
	}
	if fence == nil {
		return mutationFenceHandle{ctx: ctx}, errors.New("cache invalidation mutation fence is unavailable")
	}
	next := copyMutationFenceClasses(active, dataClass)
	return mutationFenceHandle{ctx: context.WithValue(ctx, mutationFenceContextKey{}, next), lease: fence, owned: true}, nil
}

func copyMutationFenceClasses(active map[cachepolicy.DataClass]struct{}, dataClass cachepolicy.DataClass) map[cachepolicy.DataClass]struct{} {
	next := make(map[cachepolicy.DataClass]struct{}, len(active)+1)
	for class := range active {
		next[class] = struct{}{}
	}
	next[dataClass] = struct{}{}
	return next
}
