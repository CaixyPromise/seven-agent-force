package application

import (
	"context"
	"errors"
	"testing"

	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

// RED before DG6.1: UpdateLockState wrote the user status, then ignored a
// failed best-effort session revocation and only deleted one local cache key.
// GREEN: the failure occurs inside the same transaction, before any durable
// invalidation registration can be committed.
func TestUpdateLockStateRollsBackWhenSessionRevocationFailsBeforeInvalidation(t *testing.T) {
	repo := &fakeRepository{}
	tx := &governedUserTestTransactor{}
	registrar := &governedUserTestRegistrar{}
	service := newTestService(repo, nil)
	service.transactor = tx
	service.BindCacheInvalidations(registrar)
	service.BindSessions(&fakeSessionFacade{revokeErr: errors.New("session revoke unavailable")})
	service.BindRoleAssignments(&fakeRoleAssignmentFacade{writer: repo})

	err := service.UpdateLockState(context.Background(), userfacade.UpdateLockStateCommand{
		UserID: 1001,
		Status: domain.UserStatusDisabled,
	})
	if err == nil {
		t.Fatal("lock state unexpectedly committed when session revocation failed")
	}
	if !tx.rolledBack || tx.committed {
		t.Fatalf("transaction state committed=%v rolledBack=%v, want false/true", tx.committed, tx.rolledBack)
	}
	if registrar.registered != 0 || registrar.afterCommitted != 0 {
		t.Fatalf("failed revoke emitted invalidation registered=%d afterCommit=%d", registrar.registered, registrar.afterCommitted)
	}
	if registrar.lease.releaseCount != 2 {
		t.Fatalf("rollback must release each of the two class freshness fences exactly once, got %d", registrar.lease.releaseCount)
	}
}

type governedUserTestTransactor struct {
	committed  bool
	rolledBack bool
	commit     []func()
	rollback   []func()
}

func (*governedUserTestTransactor) Enabled() bool { return true }

func (t *governedUserTestTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		t.rolledBack = true
		for _, callback := range t.rollback {
			callback()
		}
		return err
	}
	t.committed = true
	for _, callback := range t.commit {
		callback()
	}
	return nil
}

func (t *governedUserTestTransactor) RegisterAfterCommit(_ context.Context, callback func()) bool {
	t.commit = append(t.commit, callback)
	return true
}

func (t *governedUserTestTransactor) RegisterAfterRollback(_ context.Context, callback func()) bool {
	t.rollback = append(t.rollback, callback)
	return true
}

type governedUserTestRegistrar struct {
	registered     int
	afterCommitted int
	lease          governedUserTestLease
}

func (*governedUserTestRegistrar) Enabled() bool { return true }

func (r *governedUserTestRegistrar) Register(_ context.Context, class cachepolicy.DataClass) (cachegovernancefacade.Registration, error) {
	r.registered++
	return cachegovernancefacade.Registration{EventID: "test-event", DataClass: class}, nil
}

func (r *governedUserTestRegistrar) AfterCommit(_ context.Context, _ ...cachegovernancefacade.Registration) {
	r.afterCommitted++
}

func (r *governedUserTestRegistrar) AcquireMutationFence(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return &r.lease, nil
}

type governedUserTestLease struct{ releaseCount int }

func (*governedUserTestLease) Trusted() bool { return true }
func (l *governedUserTestLease) Release()    { l.releaseCount++ }

var _ cachegovernancefacade.CompletionCallbacks = (*governedUserTestTransactor)(nil)
var _ cachegovernancefacade.InvalidationRegistrar = (*governedUserTestRegistrar)(nil)
