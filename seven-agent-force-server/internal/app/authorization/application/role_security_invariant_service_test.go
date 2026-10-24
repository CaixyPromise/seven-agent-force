package application

import (
	"context"
	"sync"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestCreateRoleRejectsSystemRole(t *testing.T) {
	repo := newRoleSecurityRepository()
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)

	_, err := service.CreateRole(context.Background(), authorizationfacade.RoleCommand{
		Name: "内置角色", Code: "BUILT_IN", Type: "SYSTEM",
	})

	assertOperationError(t, err)
	if repo.createdRole {
		t.Fatal("ordinary role creation must not persist a SYSTEM role")
	}
}

func TestUpdateSystemRoleRejectsProtectedFieldChanges(t *testing.T) {
	statusEnabled := 0
	statusDisabled := 1
	customDataScope := 2
	tests := []struct {
		name    string
		command authorizationfacade.RoleCommand
	}{
		{
			name: "code",
			command: authorizationfacade.RoleCommand{
				ID: 1, Name: "超级管理员", Code: "RENAMED", Type: "SYSTEM", Status: &statusEnabled,
			},
		},
		{
			name: "type",
			command: authorizationfacade.RoleCommand{
				ID: 1, Name: "超级管理员", Code: "SUPER_ADMIN", Type: "CUSTOM", Status: &statusEnabled,
			},
		},
		{
			name: "disable",
			command: authorizationfacade.RoleCommand{
				ID: 1, Name: "超级管理员", Code: "SUPER_ADMIN", Type: "SYSTEM", Status: &statusDisabled,
			},
		},
		{
			name: "data scope",
			command: authorizationfacade.RoleCommand{
				ID: 1, Name: "超级管理员", Code: "SUPER_ADMIN", Type: "SYSTEM", Status: &statusEnabled, DataScope: &customDataScope,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRoleSecurityRepository()
			service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

			_, err := service.UpdateRole(context.Background(), tt.command)

			assertOperationError(t, err)
			if repo.updatedRole {
				t.Fatal("protected SYSTEM role fields must not be persisted")
			}
		})
	}
}

func TestDeleteRoleRejectsSystemRole(t *testing.T) {
	repo := newRoleSecurityRepository()
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.DeleteRole(context.Background(), 1, 9001)

	assertOperationError(t, err)
	if repo.deletedRole {
		t.Fatal("SYSTEM role must not be deleted")
	}
}

func TestUpdateSystemRoleAllowsDisplayFields(t *testing.T) {
	statusEnabled := 0
	sortOrder := 7
	repo := newRoleSecurityRepository()
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	updated, err := service.UpdateRole(context.Background(), authorizationfacade.RoleCommand{
		ID: 1, Name: "平台所有者", Code: "SUPER_ADMIN", Type: "SYSTEM", Status: &statusEnabled,
		DataScope: intPointer(1), SortOrder: &sortOrder, Remark: "允许维护的展示信息",
	})

	if err != nil {
		t.Fatalf("non-protected SYSTEM role fields should remain mutable: %v", err)
	}
	if updated == nil || updated.Name != "平台所有者" || updated.DataScope != 1 || updated.SortOrder != sortOrder {
		t.Fatalf("unexpected updated SYSTEM role: %#v", updated)
	}
}

func TestAuthorizationRootIdentityDoesNotDependOnRoleCode(t *testing.T) {
	repo := newRoleSecurityRepository()
	root := repo.rolesByID[1]
	root.Code = "PLATFORM_OWNER"
	root.SystemKey = domain.AuthorizationRootSystemKey
	repo.rolesByID[1] = root
	repo.directRolesByUser = map[int64][]int64{1001: {1}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	user, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test")
	if err != nil {
		t.Fatalf("build custom-code root context: %v", err)
	}
	if !user.IsAdmin {
		t.Fatal("AUTHORIZATION_ROOT must remain admin with a custom role code")
	}
}

func TestBuildUserContextFailsClosedWithoutSnapshotter(t *testing.T) {
	service := NewService(nilConfig(), nil, nil, newRoleSecurityRepository(), domain.NewService(), nil, nil, nil, nil, nil)
	if _, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test"); err == nil {
		t.Fatal("expected authorization context aggregation to fail without a consistent snapshot")
	}
}

func TestAuthorizationRootHasRoleKeepsOrdinaryCodeMatching(t *testing.T) {
	repo := newRoleSecurityRepository()
	root := repo.rolesByID[1]
	root.Code = "PLATFORM_OWNER"
	root.SystemKey = domain.AuthorizationRootSystemKey
	repo.rolesByID[1] = root
	repo.directRolesByUser = map[int64][]int64{1001: {1}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	matched, err := service.HasRole(context.Background(), 1001, "UNASSIGNED_ROLE")
	if err != nil {
		t.Fatalf("check unassigned role: %v", err)
	}
	if matched {
		t.Fatal("authorization-root bypass must not make arbitrary HasRole checks true")
	}

	matched, err = service.HasRole(context.Background(), 1001, "PLATFORM_OWNER")
	if err != nil || !matched {
		t.Fatalf("assigned root code should retain ordinary role matching, matched=%v err=%v", matched, err)
	}
}

func TestLegacyAdminCodeDoesNotGrantGlobalBypass(t *testing.T) {
	repo := newRoleSecurityRepository()
	legacy := repo.rolesByID[2]
	legacy.Code = "ADMIN"
	repo.rolesByID[2] = legacy
	repo.directRolesByUser = map[int64][]int64{1001: {2}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	user, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test")
	if err != nil {
		t.Fatalf("build legacy ADMIN context: %v", err)
	}
	if user.IsAdmin {
		t.Fatal("legacy ADMIN code must not grant global admin bypass")
	}
}

func TestRootSecurityStatusReportsRedundancy(t *testing.T) {
	tests := []struct {
		name       string
		users      map[int64][]int64
		health     string
		warningLen int
	}{
		{name: "one root", users: map[int64][]int64{1001: {1}}, health: "LOW_REDUNDANCY", warningLen: 1},
		{name: "two roots", users: map[int64][]int64{1001: {1}, 1002: {1}}, health: "HEALTHY", warningLen: 0},
		{name: "zero roots", users: map[int64][]int64{}, health: "LOW_REDUNDANCY", warningLen: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRoleSecurityRepository()
			repo.directRolesByUser = tt.users
			service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
			status, err := service.GetRootSecurityStatus(context.Background())
			if err != nil {
				t.Fatalf("root status: %v", err)
			}
			if status.Health != tt.health || len(status.Warnings) != tt.warningLen {
				t.Fatalf("unexpected root status: %#v", status)
			}
		})
	}
}

func TestAuthorizationRootGrantSurfacesCannotBeWeakened(t *testing.T) {
	repo := newRoleSecurityRepository()
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "data scope", run: func() error {
			return service.AssignRoleDepts(context.Background(), authorizationfacade.AssignRoleDeptsCommand{RoleID: 1, StepUpProof: validRootGrantProof(stepUpActionRBACAssignRoleDepts, roleDeptAssignmentBinding(1, nil))})
		}},
		{name: "menus", run: func() error {
			return service.AssignRoleMenus(context.Background(), authorizationfacade.AssignRoleMenusCommand{RoleID: 1, StepUpProof: validRootGrantProof(stepUpActionRBACAssignRoleMenus, roleMenuAssignmentBinding(1, nil))})
		}},
		{name: "permissions", run: func() error {
			return service.AssignRolePermissions(context.Background(), authorizationfacade.AssignRolePermissionsCommand{RoleID: 1, StepUpProof: validRootGrantProof(stepUpActionRBACAssignRolePermissions, rolePermissionAssignmentBinding(1, nil, nil))})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assertOperationError(t, tt.run()) })
	}
}

func TestBootstrapAuthorizationRootRejectsInvalidCodeBeforePersistence(t *testing.T) {
	service := NewService(nilConfig(), nil, nil, newRoleSecurityRepository(), domain.NewService(), nil, nil, nil, nil, nil)
	_, err := service.BootstrapAuthorizationRoot(context.Background(), authorizationfacade.BootstrapAuthorizationRootCommand{Code: "bad-code", Name: "Owner"})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected invalid root code to be rejected, got %v", err)
	}
}

func validRootGrantProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{BusinessAction: action, OperationBinding: binding, ProofIdentifier: "proof", ChallengeIdentifier: "challenge", AssuranceLevel: "AAL2", AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"}}
}

func TestRoleLifecycleRejectsRemovingLastActiveSuperAdmin(t *testing.T) {
	statusEnabled := 0
	statusDisabled := 1
	tests := []struct {
		name   string
		mutate func(*Service) error
	}{
		{
			name: "change code",
			mutate: func(service *Service) error {
				_, err := service.UpdateRole(context.Background(), authorizationfacade.RoleCommand{
					ID: 1, Name: "超级管理员", Code: "RENAMED", Type: "CUSTOM", Status: &statusEnabled,
				})
				return err
			},
		},
		{
			name: "disable",
			mutate: func(service *Service) error {
				_, err := service.UpdateRole(context.Background(), authorizationfacade.RoleCommand{
					ID: 1, Name: "超级管理员", Code: "SUPER_ADMIN", Type: "CUSTOM", Status: &statusDisabled,
				})
				return err
			},
		},
		{
			name: "delete",
			mutate: func(service *Service) error {
				return service.DeleteRole(context.Background(), 1, 9001)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRoleSecurityRepository()
			repo.directRolesByUser = map[int64][]int64{1001: {1}}
			service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

			assertOperationError(t, tt.mutate(service))
			if repo.superAdminCount() != 1 {
				t.Fatal("role lifecycle mutation removed the final active SUPER_ADMIN")
			}
		})
	}
}

func TestGuardUserDeactivationProtectsOnlyLastActiveSuperAdmin(t *testing.T) {
	t.Run("last user rejected", func(t *testing.T) {
		repo := newRoleSecurityRepository()
		repo.directRolesByUser = map[int64][]int64{1001: {1}}
		service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)

		assertOperationError(t, service.GuardUserDeactivation(context.Background(), 1001))
	})

	t.Run("non-last user allowed", func(t *testing.T) {
		repo := newRoleSecurityRepository()
		repo.directRolesByUser = map[int64][]int64{1001: {1}, 1002: {1}}
		service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)

		if err := service.GuardUserDeactivation(context.Background(), 1001); err != nil {
			t.Fatalf("non-last SUPER_ADMIN deactivation should be allowed: %v", err)
		}
	})
}

func TestAssignUserRolesRejectsRemovingLastActiveSuperAdmin(t *testing.T) {
	repo := newRoleSecurityRepository()
	repo.directRolesByUser = map[int64][]int64{1001: {1}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.AssignUserRoles(context.Background(), authorizationfacade.AssignUserRolesCommand{
		UserID:      1001,
		RoleIDs:     []int64{2},
		OperatorID:  9001,
		StepUpProof: validAuthUserRoleStepUpProof("user:1001|roles:2"),
	})

	assertOperationError(t, err)
	if repo.superAdminCount() != 1 {
		t.Fatalf("last active SUPER_ADMIN relation was removed, remaining=%d", repo.superAdminCount())
	}
}

func TestAssignUserRolesRejectsSelfRemovalWhenOperatorIsLastSuperAdmin(t *testing.T) {
	repo := newRoleSecurityRepository()
	repo.directRolesByUser = map[int64][]int64{9001: {1}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.AssignUserRoles(context.Background(), authorizationfacade.AssignUserRolesCommand{
		UserID:      9001,
		RoleIDs:     []int64{2},
		OperatorID:  9001,
		StepUpProof: validAuthUserRoleStepUpProof("user:9001|roles:2"),
	})

	assertOperationError(t, err)
	if repo.superAdminCount() != 1 {
		t.Fatal("last SUPER_ADMIN removed their own authority")
	}
}

func TestAssignUserRolesAllowsRemovingNonLastSuperAdmin(t *testing.T) {
	repo := newRoleSecurityRepository()
	repo.directRolesByUser = map[int64][]int64{1001: {1}, 1002: {1}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.AssignUserRoles(context.Background(), authorizationfacade.AssignUserRolesCommand{
		UserID:      1001,
		RoleIDs:     []int64{2},
		OperatorID:  9001,
		StepUpProof: validAuthUserRoleStepUpProof("user:1001|roles:2"),
	})

	if err != nil {
		t.Fatalf("non-last SUPER_ADMIN should be removable: %v", err)
	}
	if repo.superAdminCount() != 1 {
		t.Fatalf("unexpected remaining SUPER_ADMIN count: %d", repo.superAdminCount())
	}
}

func TestConcurrentSuperAdminRemovalsAllowAtMostOneSuccess(t *testing.T) {
	repo := newRoleSecurityRepository()
	repo.directRolesByUser = map[int64][]int64{1001: {1}, 1002: {1}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, userID := range []int64{1001, 1002} {
		wg.Add(1)
		go func(targetID int64) {
			defer wg.Done()
			<-start
			results <- service.AssignUserRoles(context.Background(), authorizationfacade.AssignUserRolesCommand{
				UserID:      int64(targetID),
				RoleIDs:     []int64{2},
				OperatorID:  9001,
				StepUpProof: validAuthUserRoleStepUpProof("user:" + int64String(targetID) + "|roles:2"),
			})
		}(userID)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		assertOperationError(t, err)
	}
	if successes > 1 {
		t.Fatalf("concurrent removals succeeded %d times; want at most one", successes)
	}
	if repo.superAdminCount() != 1 {
		t.Fatalf("concurrent removals left %d active SUPER_ADMIN users; want 1", repo.superAdminCount())
	}
}

func assertOperationError(t *testing.T, err error) {
	t.Helper()
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeOperateError {
		t.Fatalf("expected operation error, got %v", err)
	}
}

type serialTestTransactor struct {
	mu sync.Mutex
}

func (*serialTestTransactor) Enabled() bool { return true }

func (t *serialTestTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fn(ctx)
}

func (t *serialTestTransactor) WithinReadOnlySnapshot(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (t *serialTestTransactor) WithinConsistentTransaction(ctx context.Context, fn func(context.Context) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fn(ctx)
}

type roleSecurityRepository struct {
	Repository
	mu                sync.Mutex
	rolesByID         map[int64]domain.RoleRecord
	directRolesByUser map[int64][]int64
	createdRole       bool
	updatedRole       bool
	deletedRole       bool
}

func (r *roleSecurityRepository) CountAuthorizationRootRolesByIDs(_ context.Context, roleIDs []int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, id := range roleIDs {
		if role, ok := r.rolesByID[id]; ok && role.IsAuthorizationRoot() {
			count++
		}
	}
	return count, nil
}

func newRoleSecurityRepository() *roleSecurityRepository {
	return &roleSecurityRepository{
		rolesByID: map[int64]domain.RoleRecord{
			1: {RoleID: 1, Name: "超级管理员", Code: "SUPER_ADMIN", SystemKey: domain.AuthorizationRootSystemKey, Type: 1, Status: 0, DataScope: 1},
			2: {RoleID: 2, Name: "普通管理员", Code: "NORMAL_ADMIN", Type: 3, Status: 0, DataScope: 5},
		},
		directRolesByUser: map[int64][]int64{},
	}
}

func (r *roleSecurityRepository) CreateRole(_ context.Context, record domain.RoleRecord, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createdRole = true
	r.rolesByID[record.RoleID] = record
	return nil
}

func (r *roleSecurityRepository) UpdateRole(_ context.Context, record domain.RoleRecord, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updatedRole = true
	r.rolesByID[record.RoleID] = record
	return nil
}

func (r *roleSecurityRepository) DeleteRole(_ context.Context, roleID int64, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletedRole = true
	delete(r.rolesByID, roleID)
	return nil
}

func (r *roleSecurityRepository) FindRoleByID(_ context.Context, roleID int64) (*domain.RoleRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	role, ok := r.rolesByID[roleID]
	if !ok {
		return nil, nil
	}
	copy := role
	return &copy, nil
}

func (r *roleSecurityRepository) LockRoleGrant(ctx context.Context, roleID int64) (*domain.RoleRecord, error) {
	return r.FindRoleByID(ctx, roleID)
}

func (r *roleSecurityRepository) LockRoleGrants(_ context.Context, roleIDs []int64) ([]domain.RoleRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]domain.RoleRecord, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if role, ok := r.rolesByID[roleID]; ok {
			result = append(result, role)
		}
	}
	return result, nil
}

func (r *roleSecurityRepository) TouchRoleGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *roleSecurityRepository) ListRoleMenuIDs(context.Context, int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *roleSecurityRepository) LockMenuGrants(context.Context, []int64) ([]domain.MenuRecord, error) {
	return []domain.MenuRecord{}, nil
}

func (r *roleSecurityRepository) TouchMenuGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *roleSecurityRepository) LockAuthorizationCreationGuard(context.Context) error {
	return nil
}

func (r *roleSecurityRepository) CountRoleCodeExcludingID(_ context.Context, roleID int64, code string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, role := range r.rolesByID {
		if id != roleID && role.Code == code {
			return 1, nil
		}
	}
	return 0, nil
}

func (r *roleSecurityRepository) CountRolesByIDs(_ context.Context, roleIDs []int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, roleID := range roleIDs {
		if role, ok := r.rolesByID[roleID]; ok && role.Status == 0 {
			count++
		}
	}
	return count, nil
}

func (r *roleSecurityRepository) LockSuperAdminInvariant(_ context.Context, targetUserID int64) (domain.SuperAdminInvariantSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := domain.SuperAdminInvariantSnapshot{}
	for userID, roleIDs := range r.directRolesByUser {
		for _, roleID := range roleIDs {
			role, ok := r.rolesByID[roleID]
			if !ok || !role.IsActiveSuperAdmin() {
				continue
			}
			snapshot.ActiveUserCount++
			if userID == targetUserID {
				snapshot.TargetUserActive = true
			}
			break
		}
	}
	return snapshot, nil
}

func (r *roleSecurityRepository) GetAuthorizationRootSecuritySnapshot(_ context.Context) (*domain.AuthorizationRootSecuritySnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var root domain.RoleRecord
	for _, role := range r.rolesByID {
		if role.IsAuthorizationRoot() {
			root = role
			break
		}
	}
	count := 0
	for _, roleIDs := range r.directRolesByUser {
		for _, roleID := range roleIDs {
			if role, ok := r.rolesByID[roleID]; ok && role.IsActiveSuperAdmin() {
				count++
				break
			}
		}
	}
	return &domain.AuthorizationRootSecuritySnapshot{Role: root, ActiveUserCount: count}, nil
}

func (r *roleSecurityRepository) CountUserRoleReferences(context.Context, int64) (int, error) {
	return 0, nil
}

func (r *roleSecurityRepository) CountPostRoleReferences(context.Context, int64) (int, error) {
	return 0, nil
}

func (r *roleSecurityRepository) ListUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *roleSecurityRepository) ListPermissionCodesByRoleIDs(context.Context, []int64) ([]string, error) {
	return []string{}, nil
}

func (r *roleSecurityRepository) ReplaceUserRoles(_ context.Context, userID int64, roleIDs []int64, _ int64, _ func() int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.directRolesByUser[userID] = append([]int64(nil), roleIDs...)
	return nil
}

func (r *roleSecurityRepository) FindUserAggregate(_ context.Context, userID int64) (*domain.UserAggregate, error) {
	return &domain.UserAggregate{UserID: userID, Username: "operator", Enabled: true}, nil
}

func (r *roleSecurityRepository) ListUserRoles(_ context.Context, userID int64) ([]domain.RoleRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	roleIDs := r.directRolesByUser[userID]
	result := make([]domain.RoleRecord, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if role, ok := r.rolesByID[roleID]; ok {
			result = append(result, role)
		}
	}
	return result, nil
}

func (r *roleSecurityRepository) ListUserPermissions(_ context.Context, userID int64) ([]domain.PermissionRecord, error) {
	if userID == 9001 {
		return []domain.PermissionRecord{{Code: "system:user-role:assign"}}, nil
	}
	return []domain.PermissionRecord{}, nil
}

func (r *roleSecurityRepository) ListUserOrganizations(context.Context, int64) ([]domain.OrgRecord, error) {
	return []domain.OrgRecord{}, nil
}

func (r *roleSecurityRepository) ListUserDepartments(context.Context, int64) ([]domain.DeptRecord, error) {
	return []domain.DeptRecord{}, nil
}

func (r *roleSecurityRepository) ListUserPosts(context.Context, int64) ([]domain.PostRecord, error) {
	return []domain.PostRecord{}, nil
}

func (r *roleSecurityRepository) ListRoleDeptIDs(context.Context, []int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *roleSecurityRepository) ListDeptHierarchyMap(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{}, nil
}

func (r *roleSecurityRepository) ListDeptIDsByHierarchies(context.Context, []string) (map[string][]int64, error) {
	return map[string][]int64{}, nil
}

func (r *roleSecurityRepository) superAdminCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, roleIDs := range r.directRolesByUser {
		for _, roleID := range roleIDs {
			role, ok := r.rolesByID[roleID]
			if ok && role.SystemKey == domain.AuthorizationRootSystemKey && role.Status == 0 {
				count++
				break
			}
		}
	}
	return count
}

func intPointer(value int) *int { return &value }

func int64String(value int64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
