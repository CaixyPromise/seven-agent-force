package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
)

func TestBindMenuPermissionsUsesBoundedSetsBatchLocksAndBatchCacheRetries(t *testing.T) {
	users := make([]int64, 450)
	for index := range users {
		users[index] = int64(index + 1)
	}
	repo := &menuPermissionBatchRepo{
		menu:          &domain.MenuRecord{MenuID: 9, Name: "Users"},
		roles:         []int64{30, 10, 20},
		affectedUsers: users,
	}
	cache := &menuPermissionBatchCache{failDeleteManyCalls: 1}
	service := NewService(nilConfig(), cache, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.BindMenuPermissions(context.Background(), authorizationfacade.MenuPermissionAssignCommand{
		MenuID:        9,
		PermissionIDs: []int64{101},
		OperatorID:    0,
		StepUpProof:   validStepUpProof(stepUpActionRBACAssignMenuPermissions, menuPermissionAssignmentBinding(9, []int64{101})),
	})
	if err != nil {
		t.Fatalf("BindMenuPermissions() error=%v", err)
	}
	if repo.unboundedUserListCalls != 0 {
		t.Fatalf("unbounded affected-user queries=%d, want zero", repo.unboundedUserListCalls)
	}
	if repo.userPageCalls != 0 {
		t.Fatalf("affected-user page queries=%d, want zero: class generation invalidation must not enumerate users", repo.userPageCalls)
	}
	if repo.singleRoleLockCalls != 0 || repo.batchRoleLockCalls != 1 {
		t.Fatalf("role locks single=%d batch=%d, want 0/1", repo.singleRoleLockCalls, repo.batchRoleLockCalls)
	}
	if repo.menuLockCalls != 1 {
		t.Fatalf("menu locks=%d, want one relationship guard", repo.menuLockCalls)
	}
	if cache.deleteCalls != 0 {
		t.Fatalf("cache Delete fallback calls=%d, want zero", cache.deleteCalls)
	}
	if len(cache.deleteManyCalls) != 0 {
		t.Fatalf("cache DeleteMany calls=%d, want zero: durable class invalidation owns correctness", len(cache.deleteManyCalls))
	}
	if !repo.replacedMenuPermissions || !repo.replacedDerivedPermissions {
		t.Fatal("menu and derived role permission writes did not complete")
	}
	if repo.replacedRolePermissions {
		t.Fatal("BindMenuPermissions rewrote role menus/direct permissions")
	}
}

func TestBindMenuPermissionsRereadsRoleMenusAfterRelationshipAndRoleLocks(t *testing.T) {
	repo := &menuPermissionBatchRepo{
		menu:                  &domain.MenuRecord{MenuID: 9, Name: "Users"},
		roles:                 []int64{10},
		roleMenus:             map[int64][]int64{10: {9}},
		mutateRoleMenusOnLock: true,
	}
	service := NewService(nilConfig(), &menuPermissionBatchCache{}, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.BindMenuPermissions(context.Background(), authorizationfacade.MenuPermissionAssignCommand{
		MenuID:        9,
		PermissionIDs: []int64{101},
		OperatorID:    0,
		StepUpProof:   validStepUpProof(stepUpActionRBACAssignMenuPermissions, menuPermissionAssignmentBinding(9, []int64{101})),
	})
	if err != nil {
		t.Fatalf("BindMenuPermissions() error=%v", err)
	}
	if len(repo.derivedAssignments) != 1 {
		t.Fatalf("derived assignments=%d, want one", len(repo.derivedAssignments))
	}
	got := repo.derivedAssignments[0].MenuPermissionIDs
	if fmt.Sprint(sortedIDs(got)) != fmt.Sprint([]int64{101, 202}) {
		t.Fatalf("derived permission ids=%v, want locked re-read [101 202]", got)
	}
}

func TestAuthorizationTransactionRetriesSerializationFailure(t *testing.T) {
	transactor := &serializationRetryTransactor{}
	service := NewService(nilConfig(), nil, transactor, &menuPermissionBatchRepo{}, domain.NewService(), nil, nil, nil, nil, nil)
	callbackCalls := 0
	if err := service.withTransaction(context.Background(), func(context.Context) error {
		callbackCalls++
		return nil
	}); err != nil {
		t.Fatalf("withTransaction() error=%v", err)
	}
	if transactor.calls != 2 || callbackCalls != 2 {
		t.Fatalf("retry calls transactor=%d callback=%d, want 2/2", transactor.calls, callbackCalls)
	}
}

type serializationRetryTransactor struct {
	calls int
}

func (t *serializationRetryTransactor) Enabled() bool { return true }

func (t *serializationRetryTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return t.WithinConsistentTransaction(ctx, fn)
}

func (t *serializationRetryTransactor) WithinConsistentTransaction(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	if err := fn(ctx); err != nil {
		return err
	}
	if t.calls == 1 {
		return serializationStateError{}
	}
	return nil
}

type serializationStateError struct{}

func (serializationStateError) Error() string    { return "serialization failure" }
func (serializationStateError) SQLState() string { return "40001" }

type menuPermissionBatchRepo struct {
	Repository
	menu                       *domain.MenuRecord
	roles                      []int64
	affectedUsers              []int64
	unboundedUserListCalls     int
	userPageCalls              int
	singleRoleLockCalls        int
	batchRoleLockCalls         int
	menuLockCalls              int
	replacedMenuPermissions    bool
	replacedRolePermissions    bool
	replacedDerivedPermissions bool
	roleMenus                  map[int64][]int64
	mutateRoleMenusOnLock      bool
	derivedAssignments         []domain.RolePermissionAssignment
}

func (r *menuPermissionBatchRepo) FindMenuByID(context.Context, int64) (*domain.MenuRecord, error) {
	copy := *r.menu
	return &copy, nil
}

func (r *menuPermissionBatchRepo) CountPermissionsByIDs(_ context.Context, ids []int64) (int, error) {
	return len(ids), nil
}

func (r *menuPermissionBatchRepo) LockPermissionGrants(_ context.Context, permissionIDs []int64) ([]domain.PermissionRecord, error) {
	result := make([]domain.PermissionRecord, 0, len(permissionIDs))
	for _, permissionID := range permissionIDs {
		result = append(result, domain.PermissionRecord{PermissionID: permissionID})
	}
	return result, nil
}

func (r *menuPermissionBatchRepo) TouchPermissionGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *menuPermissionBatchRepo) ListMenuPermissionIDs(context.Context, []int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *menuPermissionBatchRepo) ListRoleIDsByMenuID(context.Context, int64) ([]int64, error) {
	return append([]int64(nil), r.roles...), nil
}

func (r *menuPermissionBatchRepo) ListRoleMenuIDsByRoleIDs(_ context.Context, roleIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64, len(roleIDs))
	for _, roleID := range roleIDs {
		if ids, ok := r.roleMenus[roleID]; ok {
			result[roleID] = append([]int64(nil), ids...)
		} else {
			result[roleID] = []int64{9}
		}
	}
	return result, nil
}

func (r *menuPermissionBatchRepo) ListDirectRolePermissionIDsByRoleIDs(_ context.Context, roleIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64, len(roleIDs))
	for _, roleID := range roleIDs {
		result[roleID] = []int64{}
	}
	return result, nil
}

func (r *menuPermissionBatchRepo) ListUserIDsByRoleIDs(context.Context, []int64) ([]int64, error) {
	r.unboundedUserListCalls++
	return append([]int64(nil), r.affectedUsers...), nil
}

func (r *menuPermissionBatchRepo) ListUserIDsByRoleIDsPage(_ context.Context, _ []int64, afterUserID int64, limit int) ([]int64, error) {
	r.userPageCalls++
	result := make([]int64, 0, limit)
	for _, userID := range r.affectedUsers {
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

func (r *menuPermissionBatchRepo) ListMenuPermissionIDsByMenuIDs(_ context.Context, menuIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64, len(menuIDs))
	for _, menuID := range menuIDs {
		if menuID == 11 {
			result[menuID] = []int64{202}
		} else {
			result[menuID] = []int64{}
		}
	}
	return result, nil
}

func (r *menuPermissionBatchRepo) LockRoleGrant(_ context.Context, roleID int64) (*domain.RoleRecord, error) {
	r.singleRoleLockCalls++
	return &domain.RoleRecord{RoleID: roleID, GrantRevision: roleID}, nil
}

func (r *menuPermissionBatchRepo) LockRoleGrants(_ context.Context, roleIDs []int64) ([]domain.RoleRecord, error) {
	r.batchRoleLockCalls++
	if r.mutateRoleMenusOnLock {
		r.roleMenus = map[int64][]int64{10: {9, 11}}
		r.mutateRoleMenusOnLock = false
	}
	result := make([]domain.RoleRecord, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		result = append(result, domain.RoleRecord{RoleID: roleID, GrantRevision: roleID})
	}
	return result, nil
}

func (r *menuPermissionBatchRepo) LockMenuGrants(_ context.Context, menuIDs []int64) ([]domain.MenuRecord, error) {
	r.menuLockCalls++
	result := make([]domain.MenuRecord, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		result = append(result, domain.MenuRecord{MenuID: menuID})
	}
	return result, nil
}

func (r *menuPermissionBatchRepo) TouchMenuGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *menuPermissionBatchRepo) ReplaceMenuPermissions(context.Context, int64, []int64, int64, func() int64) error {
	r.replacedMenuPermissions = true
	return nil
}

func (r *menuPermissionBatchRepo) ReplaceRolePermissionsBatch(context.Context, []domain.RolePermissionAssignment, int64, func() int64) error {
	r.replacedRolePermissions = true
	return nil
}

func (r *menuPermissionBatchRepo) ReplaceDerivedRolePermissionsBatch(_ context.Context, assignments []domain.RolePermissionAssignment, _ int64, _ func() int64) error {
	r.replacedDerivedPermissions = true
	r.derivedAssignments = append([]domain.RolePermissionAssignment(nil), assignments...)
	return nil
}

func (r *menuPermissionBatchRepo) UpdateRoleGrantRevision(context.Context, int64, int64, int64, int64) error {
	return nil
}

func (r *menuPermissionBatchRepo) UpdateRoleGrantRevisions(context.Context, []domain.RoleRecord, int64) error {
	return nil
}

type menuPermissionBatchCache struct {
	cacheinfra.Manager
	failDeleteManyCalls int
	deleteManyCalls     [][]string
	deleteCalls         int
}

func (c *menuPermissionBatchCache) DeleteMany(_ context.Context, keys ...string) error {
	c.deleteManyCalls = append(c.deleteManyCalls, append([]string(nil), keys...))
	if c.failDeleteManyCalls > 0 {
		c.failDeleteManyCalls--
		return fmt.Errorf("transient cache failure")
	}
	return nil
}

func (c *menuPermissionBatchCache) Delete(context.Context, string) error {
	c.deleteCalls++
	return nil
}
