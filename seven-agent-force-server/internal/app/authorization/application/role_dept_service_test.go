package application

import (
	"context"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestAssignRoleDeptsRequiresServiceProofMetadata(t *testing.T) {
	repo := &roleDeptRepo{
		role:           &domain.RoleRecord{RoleID: 10, DataScope: 2},
		validDeptCount: 1,
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)

	base := authorizationfacade.AssignRoleDeptsCommand{
		RoleID:     int64(10),
		DeptIDs:    []int64{20},
		OperatorID: 9001,
	}
	for _, tt := range []struct {
		name  string
		proof stepup.ProofMetadata
	}{
		{name: "missing proof"},
		{name: "wrong action", proof: stepup.ProofMetadata{
			BusinessAction:   "RBAC_ASSIGN_USER_ROLES",
			OperationBinding: "role:10|depts:20",
			ProofIdentifier:  "proof-jti",
		}},
		{name: "wrong binding", proof: stepup.ProofMetadata{
			BusinessAction:   "RBAC_ASSIGN_ROLE_DEPTS",
			OperationBinding: "role:10|depts:30",
			ProofIdentifier:  "proof-jti",
		}},
		{name: "missing proof id", proof: stepup.ProofMetadata{
			BusinessAction:   "RBAC_ASSIGN_ROLE_DEPTS",
			OperationBinding: "role:10|depts:20",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command := base
			command.StepUpProof = tt.proof
			repo.replaceCalled = false

			err := service.AssignRoleDepts(context.Background(), command)

			appErr := apperrors.From(err)
			if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected permission denied for invalid service proof metadata, got %v", err)
			}
			if repo.replaceCalled {
				t.Fatalf("service mutation should not run without matching proof metadata")
			}
		})
	}

	valid := base
	valid.StepUpProof = stepup.ProofMetadata{
		BusinessAction:        "RBAC_ASSIGN_ROLE_DEPTS",
		OperationBinding:      "role:10|depts:20",
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
	if err := service.AssignRoleDepts(context.Background(), valid); err != nil {
		t.Fatalf("expected valid service proof metadata to allow mutation: %v", err)
	}
	if !repo.replaceCalled {
		t.Fatalf("expected valid proof metadata to reach role dept replacement")
	}
}

func TestAssignRoleDeptsRequiresCustomDataScope(t *testing.T) {
	repo := &roleDeptRepo{
		role: &domain.RoleRecord{RoleID: 10, DataScope: 3},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)

	err := service.AssignRoleDepts(context.Background(), authorizationfacade.AssignRoleDeptsCommand{
		RoleID:      int64(10),
		DeptIDs:     []int64{20},
		StepUpProof: validStepUpProof("RBAC_ASSIGN_ROLE_DEPTS", "role:10|depts:20"),
	})
	if err == nil {
		t.Fatalf("expected non-custom role dept assignment to fail")
	}
	appErr, ok := err.(*apperrors.AppError)
	if !ok || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params app error, got %#v", err)
	}
	if !strings.Contains(appErr.Message(), "自定数据权限角色") {
		t.Fatalf("unexpected error message: %s", appErr.Message())
	}
	if repo.replaceCalled {
		t.Fatalf("non-custom role must not replace role depts")
	}
}

func TestAssignRoleDeptsReplacesAndRefreshesAffectedUsers(t *testing.T) {
	repo := &roleDeptRepo{
		role:           &domain.RoleRecord{RoleID: 10, DataScope: 2},
		validDeptCount: 2,
		affectedUsers:  []int64{1001, 1002},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)

	err := service.AssignRoleDepts(context.Background(), authorizationfacade.AssignRoleDeptsCommand{
		RoleID:      int64(10),
		DeptIDs:     []int64{20, 30, 20},
		OperatorID:  9001,
		StepUpProof: validStepUpProof("RBAC_ASSIGN_ROLE_DEPTS", "role:10|depts:20,30"),
	})
	if err != nil {
		t.Fatalf("assign role depts: %v", err)
	}
	if !repo.replaceCalled {
		t.Fatalf("expected role depts to be replaced")
	}
	if repo.replacedRoleID != 10 || repo.replacedOperatorID != 9001 {
		t.Fatalf("unexpected replace args role=%d operator=%d", repo.replacedRoleID, repo.replacedOperatorID)
	}
	if len(repo.replacedDeptIDs) != 2 || repo.replacedDeptIDs[0] != 20 || repo.replacedDeptIDs[1] != 30 {
		t.Fatalf("expected deduped dept ids [20 30], got %#v", repo.replacedDeptIDs)
	}
}

func TestGetRoleDeptIDsReturnsDedupedDepartments(t *testing.T) {
	repo := &roleDeptRepo{
		role:    &domain.RoleRecord{RoleID: 10, DataScope: 2},
		deptIDs: []int64{30, 20, 30},
	}
	service := NewService(nilConfig(), nil, nil, repo, nil, nil, nil, nil, nil, nil)

	result, err := service.GetRoleDeptIDs(context.Background(), 10)
	if err != nil {
		t.Fatalf("get role dept ids: %v", err)
	}
	if result.RoleID != 10 {
		t.Fatalf("expected role id 10, got %d", result.RoleID)
	}
	if len(result.DeptIDs) != 2 || result.DeptIDs[0] != 30 || result.DeptIDs[1] != 20 {
		t.Fatalf("expected deduped dept ids [30 20], got %#v", result.DeptIDs)
	}
}

func nilConfig() config.AuthorizationConfig {
	return config.AuthorizationConfig{}
}

func validStepUpProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        action,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
}

type roleDeptRepo struct {
	Repository
	role               *domain.RoleRecord
	deptIDs            []int64
	validDeptCount     int
	affectedUsers      []int64
	replaceCalled      bool
	replacedRoleID     int64
	replacedDeptIDs    []int64
	replacedOperatorID int64
}

func (r *roleDeptRepo) CountAuthorizationRootRolesByIDs(context.Context, []int64) (int, error) {
	if r.role != nil && r.role.IsAuthorizationRoot() {
		return 1, nil
	}
	return 0, nil
}

func (r *roleDeptRepo) FindRoleByID(context.Context, int64) (*domain.RoleRecord, error) {
	return r.role, nil
}

func (r *roleDeptRepo) LockRoleGrant(context.Context, int64) (*domain.RoleRecord, error) {
	return r.role, nil
}

func (r *roleDeptRepo) UpdateRoleGrantRevision(context.Context, int64, int64, int64, int64) error {
	return nil
}

func (r *roleDeptRepo) CountDeptsByIDs(_ context.Context, deptIDs []int64) (int, error) {
	return r.validDeptCount, nil
}

func (r *roleDeptRepo) ListUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	return append([]int64{}, r.affectedUsers...), nil
}

func (r *roleDeptRepo) ReplaceRoleDepts(_ context.Context, roleID int64, deptIDs []int64, operatorID int64, _ func() int64) error {
	r.replaceCalled = true
	r.replacedRoleID = roleID
	r.replacedDeptIDs = append([]int64{}, deptIDs...)
	r.replacedOperatorID = operatorID
	return nil
}

func (r *roleDeptRepo) ListDeptIDsByRoleID(context.Context, int64) ([]int64, error) {
	return append([]int64{}, r.deptIDs...), nil
}
