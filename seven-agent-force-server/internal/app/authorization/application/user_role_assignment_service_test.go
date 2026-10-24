package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestAssignUserRolesRejectsSuperAdminRoleForNonSuperAdminOperator(t *testing.T) {
	repo := &userRoleAssignmentRepo{
		rolesByID: map[int64]domain.RoleRecord{
			1: {RoleID: 1, Code: "PLATFORM_OWNER", SystemKey: domain.AuthorizationRootSystemKey},
		},
		operatorPermissions: []string{"system:user-role:assign"},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.AssignUserRoles(context.Background(), authorizationfacade.AssignUserRolesCommand{
		UserID:      int64(1001),
		RoleIDs:     []int64{1},
		OperatorID:  9001,
		StepUpProof: validAuthUserRoleStepUpProof("user:1001|roles:1"),
	})

	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden when non-super-admin assigns SUPER_ADMIN, got %v", err)
	}
	if repo.replaceUserRolesCalled {
		t.Fatalf("SUPER_ADMIN assignment must not replace user roles after rejection")
	}
}

func TestAssignUserRolesAllowsSuperAdminRoleForSuperAdminOperator(t *testing.T) {
	repo := &userRoleAssignmentRepo{
		rolesByID: map[int64]domain.RoleRecord{
			1: {RoleID: 1, Code: "PLATFORM_OWNER", SystemKey: domain.AuthorizationRootSystemKey},
		},
		operatorRoles:       []domain.RoleRecord{{RoleID: 99, Code: "ROOT_OPERATOR", SystemKey: domain.AuthorizationRootSystemKey}},
		operatorPermissions: []string{"system:user-role:assign"},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.AssignUserRoles(context.Background(), authorizationfacade.AssignUserRolesCommand{
		UserID:      int64(1001),
		RoleIDs:     []int64{1},
		OperatorID:  9001,
		StepUpProof: validAuthUserRoleStepUpProof("user:1001|roles:1"),
	})

	if err != nil {
		t.Fatalf("expected SUPER_ADMIN operator to assign SUPER_ADMIN role: %v", err)
	}
	if !repo.replaceUserRolesCalled {
		t.Fatalf("expected valid SUPER_ADMIN assignment to replace user roles")
	}
}

func TestAssignUserRolesFailsClosedWithoutConsistentTransactor(t *testing.T) {
	repo := &userRoleAssignmentRepo{
		rolesByID: map[int64]domain.RoleRecord{
			2: {RoleID: 2, Code: "NORMAL_USER"},
		},
		operatorRoles:       []domain.RoleRecord{{RoleID: 99, Code: "ROOT_OPERATOR", SystemKey: domain.AuthorizationRootSystemKey}},
		operatorPermissions: []string{"system:user-role:assign"},
	}
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.AssignUserRoles(context.Background(), authorizationfacade.AssignUserRolesCommand{
		UserID:      int64(1001),
		RoleIDs:     []int64{2},
		OperatorID:  9001,
		StepUpProof: validAuthUserRoleStepUpProof("user:1001|roles:2"),
	})

	if err == nil {
		t.Fatal("expected assignment to fail closed without a consistent transactor")
	}
	if repo.replaceUserRolesCalled {
		t.Fatal("assignment must not write without a consistent transactor")
	}
}

func TestBootstrapOwnerRolesAllowsSuperAdminWithoutStepUpProof(t *testing.T) {
	repo := &userRoleAssignmentRepo{
		rolesByID: map[int64]domain.RoleRecord{
			1: {RoleID: 1, Code: "PLATFORM_OWNER", SystemKey: domain.AuthorizationRootSystemKey},
		},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.BootstrapOwnerRoles(context.Background(), authorizationfacade.BootstrapOwnerRolesCommand{
		UserID:     int64(1001),
		RoleIDs:    []int64{1},
		OperatorID: 1001,
	})

	if err != nil {
		t.Fatalf("expected owner bootstrap to assign SUPER_ADMIN without step-up proof: %v", err)
	}
	if !repo.replaceUserRolesCalled {
		t.Fatalf("expected owner bootstrap to replace user roles")
	}
}

func TestBootstrapOwnerRolesRejectsNonSuperAdminRole(t *testing.T) {
	repo := &userRoleAssignmentRepo{
		rolesByID: map[int64]domain.RoleRecord{
			2: {RoleID: 2, Code: "NORMAL_USER"},
		},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.BootstrapOwnerRoles(context.Background(), authorizationfacade.BootstrapOwnerRolesCommand{
		UserID:     int64(1001),
		RoleIDs:    []int64{2},
		OperatorID: 1001,
	})

	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params error when owner bootstrap misses SUPER_ADMIN, got %v", err)
	}
	if repo.replaceUserRolesCalled {
		t.Fatalf("non-SUPER_ADMIN bootstrap must not replace user roles")
	}
}

func TestValidatePostRoleAssignmentRejectsAuthorizationRootEvenForRootOperator(t *testing.T) {
	repo := &userRoleAssignmentRepo{
		rolesByID:     map[int64]domain.RoleRecord{1: {RoleID: 1, Code: "PLATFORM_OWNER", SystemKey: domain.AuthorizationRootSystemKey}},
		operatorRoles: []domain.RoleRecord{{RoleID: 1, Code: "PLATFORM_OWNER", SystemKey: domain.AuthorizationRootSystemKey}},
	}
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	ok, err := service.ValidatePostRoleAssignment(context.Background(), 9001, 100, 1)
	if err != nil {
		t.Fatalf("validate root post assignment: %v", err)
	}
	if ok {
		t.Fatal("authorization root must never be assignable through a post")
	}
}

func validAuthUserRoleStepUpProof(binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        stepUpActionRBACAssignUserRoles,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
}

type userRoleAssignmentRepo struct {
	Repository
	rolesByID              map[int64]domain.RoleRecord
	operatorRoles          []domain.RoleRecord
	operatorPermissions    []string
	replaceUserRolesCalled bool
}

func (r *userRoleAssignmentRepo) CountRolesByIDs(_ context.Context, roleIDs []int64) (int, error) {
	count := 0
	for _, id := range roleIDs {
		if _, ok := r.rolesByID[id]; ok {
			count++
		}
	}
	return count, nil
}

func (r *userRoleAssignmentRepo) CountAuthorizationRootRolesByIDs(_ context.Context, roleIDs []int64) (int, error) {
	count := 0
	for _, id := range roleIDs {
		if role, ok := r.rolesByID[id]; ok && role.IsAuthorizationRoot() {
			count++
		}
	}
	return count, nil
}

func (r *userRoleAssignmentRepo) LockSuperAdminInvariant(context.Context, int64) (domain.SuperAdminInvariantSnapshot, error) {
	return domain.SuperAdminInvariantSnapshot{}, nil
}

func (r *userRoleAssignmentRepo) FindRoleByID(_ context.Context, roleID int64) (*domain.RoleRecord, error) {
	role, ok := r.rolesByID[roleID]
	if !ok {
		return nil, nil
	}
	copy := role
	return &copy, nil
}

func (r *userRoleAssignmentRepo) LockRoleGrants(_ context.Context, roleIDs []int64) ([]domain.RoleRecord, error) {
	result := make([]domain.RoleRecord, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if role, ok := r.rolesByID[roleID]; ok {
			result = append(result, role)
		}
	}
	return result, nil
}

func (r *userRoleAssignmentRepo) TouchRoleGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *userRoleAssignmentRepo) ListPermissionCodesByRoleIDs(context.Context, []int64) ([]string, error) {
	return []string{}, nil
}

func (r *userRoleAssignmentRepo) ReplaceUserRoles(context.Context, int64, []int64, int64, func() int64) error {
	r.replaceUserRolesCalled = true
	return nil
}

func (r *userRoleAssignmentRepo) FindUserAggregate(_ context.Context, userID int64) (*domain.UserAggregate, error) {
	return &domain.UserAggregate{UserID: userID, Username: "operator", Enabled: true}, nil
}

func (r *userRoleAssignmentRepo) ListUserRoles(_ context.Context, userID int64) ([]domain.RoleRecord, error) {
	if userID == 9001 {
		return append([]domain.RoleRecord{}, r.operatorRoles...), nil
	}
	return []domain.RoleRecord{}, nil
}

func (r *userRoleAssignmentRepo) ListUserPermissions(_ context.Context, userID int64) ([]domain.PermissionRecord, error) {
	if userID == 9001 {
		result := make([]domain.PermissionRecord, 0, len(r.operatorPermissions))
		for _, code := range r.operatorPermissions {
			result = append(result, domain.PermissionRecord{Code: code})
		}
		return result, nil
	}
	return []domain.PermissionRecord{}, nil
}

func (r *userRoleAssignmentRepo) ListUserOrganizations(context.Context, int64) ([]domain.OrgRecord, error) {
	return []domain.OrgRecord{}, nil
}

func (r *userRoleAssignmentRepo) ListUserDepartments(context.Context, int64) ([]domain.DeptRecord, error) {
	return []domain.DeptRecord{}, nil
}

func (r *userRoleAssignmentRepo) ListUserPosts(context.Context, int64) ([]domain.PostRecord, error) {
	return []domain.PostRecord{}, nil
}

func (r *userRoleAssignmentRepo) ListRoleDeptIDs(context.Context, []int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *userRoleAssignmentRepo) ListDeptHierarchyMap(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{}, nil
}

func (r *userRoleAssignmentRepo) ListDeptIDsByHierarchies(context.Context, []string) (map[string][]int64, error) {
	return map[string][]int64{}, nil
}
