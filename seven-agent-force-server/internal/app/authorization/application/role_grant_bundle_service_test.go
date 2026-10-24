package application

import (
	"context"
	"errors"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestCommitRoleGrantBundleRejectsStaleRevisionBeforeMutation(t *testing.T) {
	repo := &roleGrantBundleRepo{role: &domain.RoleRecord{RoleID: 10, DataScope: 2, GrantRevision: 7}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)
	service.BindRoleGrantConfigScopes(&roleGrantConfigPort{})

	_, err := service.CommitRoleGrantBundle(context.Background(), authorizationfacade.CommitRoleGrantBundleCommand{
		RoleID: 10,
		RoleGrantBundleRequest: authorizationfacade.RoleGrantBundleRequest{
			ExpectedRevision: 6, IdempotencyKey: "grant-10-stale", Reason: "reviewed change", DataScope: 2,
		},
	})

	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	details, _ := appErr.Details().(map[string]any)
	if details["reasonCode"] != "ROLE_GRANT_REVISION_CONFLICT" {
		t.Fatalf("unexpected conflict details: %#v", details)
	}
	if repo.replacePermissionsCalled || repo.replaceDeptsCalled || repo.revisionUpdated {
		t.Fatalf("stale commit must not mutate grants")
	}
}

func TestCommitRoleGrantBundleRejectsAuthorizationRoot(t *testing.T) {
	repo := &roleGrantBundleRepo{role: &domain.RoleRecord{
		RoleID: 10, SystemKey: domain.AuthorizationRootSystemKey, DataScope: 1, GrantRevision: 2,
	}}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)
	service.BindRoleGrantConfigScopes(&roleGrantConfigPort{})

	_, err := service.CommitRoleGrantBundle(context.Background(), authorizationfacade.CommitRoleGrantBundleCommand{
		RoleID: 10,
		RoleGrantBundleRequest: authorizationfacade.RoleGrantBundleRequest{
			ExpectedRevision: 2, IdempotencyKey: "grant-root", Reason: "must fail", DataScope: 1,
		},
	})

	if err == nil || !errors.Is(err, apperrors.From(err)) && apperrors.From(err) == nil {
		t.Fatalf("expected authorization root mutation to fail")
	}
	if repo.replacePermissionsCalled || repo.replaceDeptsCalled || repo.revisionUpdated {
		t.Fatalf("authorization root commit must not mutate grants")
	}
}

func TestCommitRoleGrantBundleDoesNotAdvanceRevisionWhenConfigScopeFails(t *testing.T) {
	repo := &roleGrantBundleRepo{
		role:                 &domain.RoleRecord{RoleID: 10, DataScope: 2, GrantRevision: 3},
		validMenuCount:       1,
		validPermissionCount: 1,
		validDeptCount:       1,
	}
	configPort := &roleGrantConfigPort{replaceErr: errors.New("config scope write failed")}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)
	service.BindRoleGrantConfigScopes(configPort)

	_, err := service.CommitRoleGrantBundle(context.Background(), authorizationfacade.CommitRoleGrantBundleCommand{
		RoleID:      10,
		OperatorID:  0,
		StepUpProof: validRoleGrantBundleProof(10, 3, []int64{20}, []int64{30}, 2, []int64{40}, "reviewed change"),
		RoleGrantBundleRequest: authorizationfacade.RoleGrantBundleRequest{
			ExpectedRevision: 3, MenuIDs: []int64{20}, PermissionIDs: []int64{30}, DataScope: 2, DeptIDs: []int64{40},
			ConfigScopes:   []authorizationfacade.RoleConfigScopeGrantVO{{GroupCode: "security", CanRead: 1}},
			IdempotencyKey: "grant-10-config-failure", Reason: "reviewed change",
		},
	})

	if err == nil {
		t.Fatalf("expected config scope failure")
	}
	if !configPort.replaceCalled {
		t.Fatalf("expected config scope port to participate in transaction, got %v", err)
	}
	if repo.revisionUpdated {
		t.Fatalf("revision must not advance when any grant partition fails")
	}
}

func TestCommitRoleGrantBundleReplacesAllPartitionsAndAdvancesRevision(t *testing.T) {
	repo := &roleGrantBundleRepo{
		role:           &domain.RoleRecord{RoleID: 10, DataScope: 2, GrantRevision: 3},
		validMenuCount: 1, validPermissionCount: 1, validDeptCount: 1,
	}
	configPort := &roleGrantConfigPort{}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)
	service.BindRoleGrantConfigScopes(configPort)
	request := authorizationfacade.RoleGrantBundleRequest{
		ExpectedRevision: 3, MenuIDs: []int64{20}, PermissionIDs: []int64{30}, DataScope: 2, DeptIDs: []int64{40},
		ConfigScopes:   []authorizationfacade.RoleConfigScopeGrantVO{{GroupCode: "security", CanRead: 1}},
		IdempotencyKey: "grant-10-success", Reason: "approved by security review",
	}

	result, err := service.CommitRoleGrantBundle(context.Background(), authorizationfacade.CommitRoleGrantBundleCommand{
		RoleID: 10, RoleGrantBundleRequest: request,
		StepUpProof: validRoleGrantBundleProof(10, 3, []int64{20}, []int64{30}, 2, []int64{40}, "approved by security review"),
	})

	if err != nil {
		t.Fatalf("commit role grants: %v", err)
	}
	if result.Revision != 4 || !result.Changed || result.ImpactedUserCount != 2 || result.IdempotentReplay {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !repo.replacePermissionsCalled || !repo.replaceDeptsCalled || !configPort.replaceCalled || !repo.revisionUpdated || !repo.requestCreated {
		t.Fatalf("all role grant partitions, revision, and idempotency record must be written")
	}
}

func TestCommitRoleGrantBundleReplaysSameIdempotencyKey(t *testing.T) {
	request := authorizationfacade.RoleGrantBundleRequest{
		ExpectedRevision: 3, DataScope: 2, IdempotencyKey: "grant-10-replay", Reason: "approved",
	}
	hash, err := authorizationfacade.RoleGrantRequestHash(request)
	if err != nil {
		t.Fatalf("hash request: %v", err)
	}
	repo := &roleGrantBundleRepo{
		role: &domain.RoleRecord{RoleID: 10, DataScope: 2, GrantRevision: 4},
		previousRequest: &domain.RoleGrantRequestRecord{
			RoleID: 10, IdempotencyKey: request.IdempotencyKey, RequestHash: hash,
			ResultRevision: 4, ImpactedUserCount: 2, Changed: 1,
		},
	}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, nil, nil, nil, nil, nil, nil)
	service.BindRoleGrantConfigScopes(&roleGrantConfigPort{})

	result, err := service.CommitRoleGrantBundle(context.Background(), authorizationfacade.CommitRoleGrantBundleCommand{
		RoleID: 10, RoleGrantBundleRequest: request,
	})

	if err != nil {
		t.Fatalf("replay role grants: %v", err)
	}
	if !result.IdempotentReplay || result.Revision != 4 || !result.Changed {
		t.Fatalf("unexpected replay result: %#v", result)
	}
	if repo.replacePermissionsCalled || repo.replaceDeptsCalled || repo.revisionUpdated {
		t.Fatalf("idempotent replay must not write grants again")
	}
}

type roleGrantBundleRepo struct {
	Repository
	role                     *domain.RoleRecord
	validMenuCount           int
	validPermissionCount     int
	validDeptCount           int
	replacePermissionsCalled bool
	replaceDeptsCalled       bool
	revisionUpdated          bool
	requestCreated           bool
	previousRequest          *domain.RoleGrantRequestRecord
}

func (r *roleGrantBundleRepo) LockRoleGrant(context.Context, int64) (*domain.RoleRecord, error) {
	return r.role, nil
}

func (r *roleGrantBundleRepo) LockMenuGrants(_ context.Context, menuIDs []int64) ([]domain.MenuRecord, error) {
	result := make([]domain.MenuRecord, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		result = append(result, domain.MenuRecord{MenuID: menuID})
	}
	return result, nil
}

func (r *roleGrantBundleRepo) TouchMenuGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *roleGrantBundleRepo) FindRoleGrantRequest(context.Context, int64, string) (*domain.RoleGrantRequestRecord, error) {
	return r.previousRequest, nil
}

func (r *roleGrantBundleRepo) CountMenusByIDs(context.Context, []int64) (int, error) {
	return r.validMenuCount, nil
}

func (r *roleGrantBundleRepo) CountPermissionsByIDs(context.Context, []int64) (int, error) {
	return r.validPermissionCount, nil
}

func (r *roleGrantBundleRepo) CountDeptsByIDs(context.Context, []int64) (int, error) {
	return r.validDeptCount, nil
}

func (r *roleGrantBundleRepo) ListMenuPermissionCodes(context.Context, []int64) ([]string, error) {
	return []string{}, nil
}

func (r *roleGrantBundleRepo) ListMenuPermissionIDs(context.Context, []int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *roleGrantBundleRepo) ListPermissionsByIDs(context.Context, []int64) ([]domain.PermissionRecord, error) {
	return []domain.PermissionRecord{}, nil
}

func (r *roleGrantBundleRepo) LockPermissionGrants(_ context.Context, permissionIDs []int64) ([]domain.PermissionRecord, error) {
	result := make([]domain.PermissionRecord, 0, len(permissionIDs))
	for _, permissionID := range permissionIDs {
		result = append(result, domain.PermissionRecord{PermissionID: permissionID})
	}
	return result, nil
}

func (r *roleGrantBundleRepo) TouchPermissionGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *roleGrantBundleRepo) ListDirectRolePermissionIDsByRoleIDs(context.Context, []int64) (map[int64][]int64, error) {
	return map[int64][]int64{10: {}}, nil
}

func (r *roleGrantBundleRepo) ListRoleMenuIDs(context.Context, int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *roleGrantBundleRepo) ListAllMenus(context.Context) ([]domain.MenuRecord, error) {
	return []domain.MenuRecord{{MenuID: 20, Status: 0}}, nil
}

func (r *roleGrantBundleRepo) ListDeptIDsByRoleID(context.Context, int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *roleGrantBundleRepo) ListUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	return []int64{1001, 1002}, nil
}

func (r *roleGrantBundleRepo) CountUserIDsByRoleID(context.Context, int64) (int, error) {
	return 2, nil
}

func (r *roleGrantBundleRepo) ListUserIDsByRoleIDsPage(_ context.Context, _ []int64, afterUserID int64, _ int) ([]int64, error) {
	switch afterUserID {
	case 0:
		return []int64{1001, 1002}, nil
	default:
		return []int64{}, nil
	}
}

func (r *roleGrantBundleRepo) ReplaceRolePermissions(context.Context, int64, []int64, []int64, []int64, int64, func() int64) error {
	r.replacePermissionsCalled = true
	return nil
}

func (r *roleGrantBundleRepo) ReplaceRoleDepts(context.Context, int64, []int64, int64, func() int64) error {
	r.replaceDeptsCalled = true
	return nil
}

func (r *roleGrantBundleRepo) UpdateRoleGrantRevision(context.Context, int64, int64, int64, int64) error {
	r.revisionUpdated = true
	return nil
}

func (r *roleGrantBundleRepo) CreateRoleGrantRequest(context.Context, domain.RoleGrantRequestRecord, int64) error {
	r.requestCreated = true
	return nil
}

type roleGrantConfigPort struct {
	replaceCalled bool
	replaceErr    error
}

func (p *roleGrantConfigPort) ListRoleConfigScopes(context.Context, int64) ([]authorizationfacade.RoleConfigScopeGrantVO, error) {
	return []authorizationfacade.RoleConfigScopeGrantVO{}, nil
}

func (p *roleGrantConfigPort) NormalizeRoleConfigScopes(_ context.Context, grants []authorizationfacade.RoleConfigScopeGrantVO) ([]authorizationfacade.RoleConfigScopeGrantVO, error) {
	return append([]authorizationfacade.RoleConfigScopeGrantVO(nil), grants...), nil
}

func (p *roleGrantConfigPort) ReplaceRoleConfigScopes(context.Context, int64, []authorizationfacade.RoleConfigScopeGrantVO, int64) error {
	p.replaceCalled = true
	return p.replaceErr
}

func validRoleGrantBundleProof(roleID, revision int64, menuIDs, permissionIDs []int64, dataScope int, deptIDs []int64, reason string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction: stepUpActionRBACCommitRoleGrants,
		OperationBinding: mustRoleGrantBinding(roleID, authorizationfacade.RoleGrantBundleRequest{
			ExpectedRevision: revision, MenuIDs: menuIDs, PermissionIDs: permissionIDs,
			DataScope: dataScope, DeptIDs: deptIDs,
			ConfigScopes: []authorizationfacade.RoleConfigScopeGrantVO{{GroupCode: "security", CanRead: 1}},
			Reason:       reason,
		}),
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
}

func mustRoleGrantBinding(roleID int64, request authorizationfacade.RoleGrantBundleRequest) string {
	binding, err := authorizationfacade.RoleGrantOperationBinding(roleID, request)
	if err != nil {
		panic(err)
	}
	return binding
}
