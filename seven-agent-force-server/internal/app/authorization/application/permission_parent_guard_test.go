package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
)

func TestDeletePermissionGuardsParentsDeletesChildrenAndInvalidatesUsersInPages(t *testing.T) {
	users := make([]int64, 450)
	for index := range users {
		users[index] = int64(index + 1)
	}
	repo := &permissionParentGuardRepo{users: users}
	cache := &menuPermissionBatchCache{}
	service := NewService(nilConfig(), cache, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	if err := service.DeletePermission(context.Background(), 101, 7); err != nil {
		t.Fatalf("DeletePermission() error=%v", err)
	}
	if !repo.permissionLocked || !repo.permissionTouched || !repo.menuLocked || !repo.menuTouched || !repo.roleLocked {
		t.Fatalf("missing parent guard permission=%v/%v menu=%v/%v role=%v",
			repo.permissionLocked, repo.permissionTouched, repo.menuLocked, repo.menuTouched, repo.roleLocked)
	}
	if !repo.userChildrenDeleted || !repo.permissionDeleted {
		t.Fatalf("child-first delete missing users=%v permission=%v", repo.userChildrenDeleted, repo.permissionDeleted)
	}
	if repo.userPageCalls != 0 || len(cache.deleteManyCalls) != 0 {
		t.Fatalf("user pagination/local cache chunks=%d/%d, want 0/0: class generation invalidation owns correctness", repo.userPageCalls, len(cache.deleteManyCalls))
	}
}

type permissionParentGuardRepo struct {
	Repository
	users               []int64
	permissionLocked    bool
	permissionTouched   bool
	menuLocked          bool
	menuTouched         bool
	roleLocked          bool
	userChildrenDeleted bool
	permissionDeleted   bool
	userPageCalls       int
}

func (r *permissionParentGuardRepo) LockPermissionGrants(context.Context, []int64) ([]domain.PermissionRecord, error) {
	r.permissionLocked = true
	return []domain.PermissionRecord{{PermissionID: 101, Code: "system:user:list"}}, nil
}

func (r *permissionParentGuardRepo) TouchPermissionGrantGuards(context.Context, []int64) error {
	r.permissionTouched = true
	return nil
}

func (r *permissionParentGuardRepo) ListMenuIDsByPermissionIDs(context.Context, []int64) ([]int64, error) {
	return []int64{9}, nil
}

func (r *permissionParentGuardRepo) LockMenuGrants(context.Context, []int64) ([]domain.MenuRecord, error) {
	r.menuLocked = true
	return []domain.MenuRecord{{MenuID: 9}}, nil
}

func (r *permissionParentGuardRepo) TouchMenuGrantGuards(context.Context, []int64) error {
	r.menuTouched = true
	return nil
}

func (r *permissionParentGuardRepo) ListRoleIDsByPermissionIDs(context.Context, []int64) ([]int64, error) {
	return []int64{10}, nil
}

func (r *permissionParentGuardRepo) LockRoleGrants(context.Context, []int64) ([]domain.RoleRecord, error) {
	r.roleLocked = true
	return []domain.RoleRecord{{RoleID: 10, GrantRevision: 3}}, nil
}

func (r *permissionParentGuardRepo) SoftDeleteUserPermissionsByPermissionID(context.Context, int64, int64) error {
	r.userChildrenDeleted = true
	return nil
}

func (r *permissionParentGuardRepo) DeletePermission(context.Context, int64, int64) error {
	if !r.userChildrenDeleted {
		return context.Canceled
	}
	r.permissionDeleted = true
	return nil
}

func (r *permissionParentGuardRepo) UpdateRoleGrantRevision(context.Context, int64, int64, int64, int64) error {
	return nil
}

func (r *permissionParentGuardRepo) UpdateRoleGrantRevisions(context.Context, []domain.RoleRecord, int64) error {
	return nil
}

func (r *permissionParentGuardRepo) ListUserIDsByRoleIDsPage(context.Context, []int64, int64, int) ([]int64, error) {
	return []int64{}, nil
}

func (r *permissionParentGuardRepo) ListUserIDsByPermissionIDPage(_ context.Context, _ int64, afterUserID int64, limit int) ([]int64, error) {
	r.userPageCalls++
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
