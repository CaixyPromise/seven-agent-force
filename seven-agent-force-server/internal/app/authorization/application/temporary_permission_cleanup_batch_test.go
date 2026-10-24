package application

import (
	"context"
	"testing"
)

func TestCleanupExpiredTemporaryPermissionsUsesDeterministicPages(t *testing.T) {
	users := make([]int64, 450)
	for index := range users {
		users[index] = int64(index + 1)
	}
	repo := &temporaryPermissionCleanupRepo{users: users}
	cache := &menuPermissionBatchCache{}
	service := NewService(nilConfig(), cache, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)

	if err := service.CleanupExpiredTemporaryPermissions(context.Background()); err != nil {
		t.Fatalf("CleanupExpiredTemporaryPermissions() error=%v", err)
	}
	if repo.unboundedListCalls != 0 || repo.unboundedCleanupCalls != 0 {
		t.Fatalf("unbounded cleanup calls list=%d update=%d, want zero", repo.unboundedListCalls, repo.unboundedCleanupCalls)
	}
	if repo.pageCalls != 3 || repo.pageCleanupCalls != 3 {
		t.Fatalf("paged cleanup calls list=%d update=%d, want 3/3", repo.pageCalls, repo.pageCleanupCalls)
	}
	if len(cache.deleteManyCalls) != 0 {
		t.Fatalf("local cache invalidation chunks=%d, want zero: cleanup emits bounded class invalidations", len(cache.deleteManyCalls))
	}
}

type temporaryPermissionCleanupRepo struct {
	Repository
	users                 []int64
	unboundedListCalls    int
	unboundedCleanupCalls int
	pageCalls             int
	pageCleanupCalls      int
}

func (r *temporaryPermissionCleanupRepo) ListExpiredTemporaryPermissionUserIDs(context.Context) ([]int64, error) {
	r.unboundedListCalls++
	return append([]int64(nil), r.users...), nil
}

func (r *temporaryPermissionCleanupRepo) CleanupExpiredTemporaryPermissions(context.Context) error {
	r.unboundedCleanupCalls++
	return nil
}

func (r *temporaryPermissionCleanupRepo) ListExpiredTemporaryPermissionUserIDsPage(_ context.Context, afterUserID int64, limit int) ([]int64, error) {
	r.pageCalls++
	result := make([]int64, 0, limit)
	for _, userID := range r.users {
		if userID <= afterUserID {
			continue
		}
		result = append(result, userID)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (r *temporaryPermissionCleanupRepo) CleanupExpiredTemporaryPermissionsByUserIDs(_ context.Context, userIDs []int64) error {
	r.pageCleanupCalls++
	if len(userIDs) == 0 || len(userIDs) > authorizationAffectedUserPageSize {
		return context.Canceled
	}
	return nil
}
