package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
)

func TestDeleteRoleUsesMenuGuardThenRoleLockBeforeReferenceChecks(t *testing.T) {
	repo := &relationshipDeleteGuardRepo{
		role:      &domain.RoleRecord{RoleID: 10, Code: "OPS"},
		roleMenus: []int64{9},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	if err := service.DeleteRole(context.Background(), 10, 7); err != nil {
		t.Fatalf("DeleteRole() error=%v", err)
	}
	want := "[list-role-menus lock-menus touch-menus lock-role list-role-menus count-user-refs count-post-refs delete-role]"
	if got := fmt.Sprint(repo.calls); got != want {
		t.Fatalf("DeleteRole() calls=%s, want %s", got, want)
	}
}

func TestDeleteMenuLocksAndTouchesGuardBeforeRelationshipRecheck(t *testing.T) {
	repo := &relationshipDeleteGuardRepo{menu: &domain.MenuRecord{MenuID: 9, Name: "Users"}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	if err := service.DeleteMenu(context.Background(), 9, 7); err != nil {
		t.Fatalf("DeleteMenu() error=%v", err)
	}
	want := "[lock-menus touch-menus count-menu-children count-role-menu-refs delete-menu-permissions delete-menu]"
	if got := fmt.Sprint(repo.calls); got != want {
		t.Fatalf("DeleteMenu() calls=%s, want %s", got, want)
	}
}

func TestUpdateMenuLocksGuardAndAffectedRolesBeforeMutation(t *testing.T) {
	repo := &relationshipDeleteGuardRepo{
		menu:  &domain.MenuRecord{MenuID: 9, Name: "Users"},
		roles: []int64{20, 10},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	if _, err := service.UpdateMenu(context.Background(), authorizationfacade.MenuCommand{
		ID: 9, Name: "Users", Type: "M", Permission: "system:user:list",
	}); err != nil {
		t.Fatalf("UpdateMenu() error=%v", err)
	}
	want := "[find-menu count-menu-permission lock-menus touch-menus count-menu-permission list-menu-roles lock-roles update-menu update-role-revision update-role-revision find-menu]"
	if got := fmt.Sprint(repo.calls); got != want {
		t.Fatalf("UpdateMenu() calls=%s, want %s", got, want)
	}
}

type relationshipDeleteGuardRepo struct {
	Repository
	role      *domain.RoleRecord
	menu      *domain.MenuRecord
	roleMenus []int64
	roles     []int64
	calls     []string
}

func (r *relationshipDeleteGuardRepo) FindMenuByID(context.Context, int64) (*domain.MenuRecord, error) {
	r.calls = append(r.calls, "find-menu")
	return r.menu, nil
}

func (r *relationshipDeleteGuardRepo) LockAuthorizationCreationGuard(context.Context) error {
	return nil
}

func (r *relationshipDeleteGuardRepo) CountMenuPermissionExcludingID(context.Context, int64, string) (int, error) {
	r.calls = append(r.calls, "count-menu-permission")
	return 0, nil
}

func (r *relationshipDeleteGuardRepo) ListRoleIDsByMenuID(context.Context, int64) ([]int64, error) {
	r.calls = append(r.calls, "list-menu-roles")
	return append([]int64(nil), r.roles...), nil
}

func (r *relationshipDeleteGuardRepo) LockRoleGrants(_ context.Context, roleIDs []int64) ([]domain.RoleRecord, error) {
	r.calls = append(r.calls, "lock-roles")
	result := make([]domain.RoleRecord, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		result = append(result, domain.RoleRecord{RoleID: roleID, GrantRevision: roleID})
	}
	return result, nil
}

func (r *relationshipDeleteGuardRepo) UpdateMenu(context.Context, domain.MenuRecord, int64) error {
	r.calls = append(r.calls, "update-menu")
	return nil
}

func (r *relationshipDeleteGuardRepo) UpdateRoleGrantRevision(context.Context, int64, int64, int64, int64) error {
	r.calls = append(r.calls, "update-role-revision")
	return nil
}

func (r *relationshipDeleteGuardRepo) UpdateRoleGrantRevisions(_ context.Context, roles []domain.RoleRecord, _ int64) error {
	for range roles {
		r.calls = append(r.calls, "update-role-revision")
	}
	return nil
}

func (r *relationshipDeleteGuardRepo) FindRoleByID(context.Context, int64) (*domain.RoleRecord, error) {
	r.calls = append(r.calls, "find-role")
	return r.role, nil
}

func (r *relationshipDeleteGuardRepo) ListRoleMenuIDs(context.Context, int64) ([]int64, error) {
	r.calls = append(r.calls, "list-role-menus")
	return append([]int64(nil), r.roleMenus...), nil
}

func (r *relationshipDeleteGuardRepo) LockMenuGrants(_ context.Context, menuIDs []int64) ([]domain.MenuRecord, error) {
	r.calls = append(r.calls, "lock-menus")
	result := make([]domain.MenuRecord, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		result = append(result, domain.MenuRecord{MenuID: menuID})
	}
	return result, nil
}

func (r *relationshipDeleteGuardRepo) TouchMenuGrantGuards(context.Context, []int64) error {
	r.calls = append(r.calls, "touch-menus")
	return nil
}

func (r *relationshipDeleteGuardRepo) LockRoleGrant(context.Context, int64) (*domain.RoleRecord, error) {
	r.calls = append(r.calls, "lock-role")
	return r.role, nil
}

func (r *relationshipDeleteGuardRepo) CountUserRoleReferences(context.Context, int64) (int, error) {
	r.calls = append(r.calls, "count-user-refs")
	return 0, nil
}

func (r *relationshipDeleteGuardRepo) CountPostRoleReferences(context.Context, int64) (int, error) {
	r.calls = append(r.calls, "count-post-refs")
	return 0, nil
}

func (r *relationshipDeleteGuardRepo) DeleteRole(context.Context, int64, int64) error {
	r.calls = append(r.calls, "delete-role")
	return nil
}

func (r *relationshipDeleteGuardRepo) LockMenuGrant(context.Context, int64) (*domain.MenuRecord, error) {
	r.calls = append(r.calls, "lock-menu")
	return r.menu, nil
}

func (r *relationshipDeleteGuardRepo) CountMenuChildren(context.Context, int64) (int, error) {
	r.calls = append(r.calls, "count-menu-children")
	return 0, nil
}

func (r *relationshipDeleteGuardRepo) CountRoleMenuReferences(context.Context, int64) (int, error) {
	r.calls = append(r.calls, "count-role-menu-refs")
	return 0, nil
}

func (r *relationshipDeleteGuardRepo) DeleteMenu(context.Context, int64, int64) error {
	r.calls = append(r.calls, "delete-menu")
	return nil
}

func (r *relationshipDeleteGuardRepo) DeleteMenuPermissionsByMenuID(context.Context, int64) error {
	r.calls = append(r.calls, "delete-menu-permissions")
	return nil
}
