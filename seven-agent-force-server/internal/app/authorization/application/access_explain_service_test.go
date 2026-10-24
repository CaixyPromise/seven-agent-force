package application

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
)

type accessTestSnapshotter struct{}

func (accessTestSnapshotter) Enabled() bool { return true }
func (accessTestSnapshotter) WithinReadOnlySnapshot(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

var _ store.Snapshotter = accessTestSnapshotter{}

func TestAccessExplainFailsClosedWithoutSnapshotter(t *testing.T) {
	repo := &accessExplainRepository{user: &domain.AccessUserRecord{UserID: 1001}}
	service := NewAccessExplainService(repo, domain.NewService(), features.Set{})

	if _, err := service.GetEffectiveAccess(context.Background(), 1001, authorizationfacade.EffectiveAccessQuery{}); err == nil {
		t.Fatal("expected access explain to fail without a consistent snapshot")
	}
	if repo.calls != 0 {
		t.Fatalf("repository was queried before snapshot validation: calls=%d", repo.calls)
	}
}

func TestAccessExplainServiceBuildsEffectiveAccessWithoutNPlusOne(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	repo := &accessExplainRepository{
		user: &domain.AccessUserRecord{UserID: 1001, Username: "operator", Status: 0},
		roles: []domain.AccessRoleSourceRecord{
			{RoleID: 10, RoleCode: "AUDITOR", RoleName: "审计员", RoleStatus: 0, RoleDataScope: 2, AssignmentSource: "DIRECT_USER"},
			{RoleID: 10, RoleCode: "AUDITOR", RoleName: "审计员", RoleStatus: 0, RoleDataScope: 2, AssignmentSource: "POST", PostID: 20, PostCode: "SEC", PostName: "安全岗", PostDeptID: 30, PostOrgID: 1},
		},
		grants: []domain.AccessGrantRecord{
			{PermissionID: 1, PermissionCode: "system:user:*", PermissionName: "用户管理", GrantSource: "ROLE_DIRECT", RoleID: 10, RoleCode: "AUDITOR", RoleName: "审计员", AssignmentSource: "DIRECT_USER"},
			{PermissionID: 2, PermissionCode: "system:role:list", PermissionName: "角色列表", GrantSource: "MENU_DERIVED", RoleID: 10, RoleCode: "AUDITOR", RoleName: "审计员", AssignmentSource: "POST", PostID: 20, PostCode: "SEC", PostName: "安全岗", MenuID: 101, MenuName: "角色管理"},
			{PermissionID: 3, PermissionCode: "admin:docker:view", PermissionName: "Docker", FeatureCode: "docker.admin", GrantSource: "TEMPORARY", PermissionType: 1, ExpireAt: timePointer(now.Add(time.Hour))},
			{PermissionID: 4, PermissionCode: "system:user:update", PermissionName: "更新用户", GrantSource: "TEMPORARY", PermissionType: 1, ExpireAt: timePointer(now.Add(-time.Hour))},
		},
		roleDepts:   []domain.AccessRoleDeptRecord{{RoleID: 10, DeptID: 99}},
		memberships: []domain.AccessMembershipRecord{{Kind: "ORG", ID: 1, OrgID: 1}, {Kind: "DEPT", ID: 30, OrgID: 1, Hierarchy: "1/30"}},
		descendants: map[string][]int64{"1/30": {30, 31}},
		menus: []domain.MenuRecord{
			{MenuID: 100, Name: "系统管理"},
			{MenuID: 101, ParentID: 100, Name: "角色管理"},
		},
	}
	service := NewAccessExplainService(repo, domain.NewService(), features.Set{}, accessTestSnapshotter{})
	service.now = func() time.Time { return now }

	result, err := service.GetEffectiveAccess(context.Background(), 1001, authorizationfacade.EffectiveAccessQuery{Current: 1, Size: 20})
	if err != nil {
		t.Fatalf("GetEffectiveAccess() error = %v", err)
	}
	if repo.calls != 7 {
		t.Fatalf("expected 7 set-based reads, got %d", repo.calls)
	}
	if len(result.RoleSources) != 2 || result.RoleSources[1].Post == nil {
		t.Fatalf("expected direct and post role sources: %#v", result.RoleSources)
	}
	if result.DataScope.ScopeType != "CUSTOM" || len(result.DataScope.DeptIDs) != 1 || result.DataScope.DeptIDs[0] != 99 {
		t.Fatalf("unexpected effective data scope: %#v", result.DataScope)
	}
	if result.PermissionSummary.EffectiveCount != 2 || result.PermissionSummary.FilteredCount != 1 || result.PermissionSummary.TemporaryCount != 2 {
		t.Fatalf("unexpected permission summary: %#v", result.PermissionSummary)
	}
	if result.Permissions.Total != 4 {
		t.Fatalf("expected four permission records, got %#v", result.Permissions)
	}
	menuPermission := findEffectivePermission(result.Permissions.Records, "system:role:list")
	if menuPermission == nil || len(menuPermission.Grants) != 1 || menuPermission.Grants[0].MenuPath != "系统管理 / 角色管理" {
		t.Fatalf("expected menu breadcrumb and source chain: %#v", menuPermission)
	}
}

func TestAccessExplainServiceExplainsWildcardAndDenials(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		repo       *accessExplainRepository
		permission string
		decision   string
		reason     string
	}{
		{
			name: "wildcard role permission",
			repo: &accessExplainRepository{
				user:   &domain.AccessUserRecord{UserID: 1001, Username: "operator"},
				roles:  []domain.AccessRoleSourceRecord{{RoleID: 10, RoleCode: "AUDITOR", RoleStatus: 0, RoleDataScope: 1, AssignmentSource: "DIRECT_USER"}},
				grants: []domain.AccessGrantRecord{{PermissionCode: "system:user:*", GrantSource: "ROLE_DIRECT", RoleID: 10, RoleCode: "AUDITOR", AssignmentSource: "DIRECT_USER"}},
			},
			permission: "system:user:update", decision: "ALLOW", reason: "WILDCARD_PERMISSION_MATCH",
		},
		{
			name: "feature disabled",
			repo: &accessExplainRepository{
				user:   &domain.AccessUserRecord{UserID: 1001, Username: "operator"},
				grants: []domain.AccessGrantRecord{{PermissionCode: "admin:docker:view", FeatureCode: "docker.admin", GrantSource: "TEMPORARY"}},
			},
			permission: "admin:docker:view", decision: "DENY", reason: "FEATURE_DISABLED",
		},
		{
			name: "expired temporary permission",
			repo: &accessExplainRepository{
				user:   &domain.AccessUserRecord{UserID: 1001, Username: "operator"},
				grants: []domain.AccessGrantRecord{{PermissionCode: "system:user:update", GrantSource: "TEMPORARY", PermissionType: 1, ExpireAt: timePointer(now.Add(-time.Minute))}},
			},
			permission: "system:user:update", decision: "DENY", reason: "TEMPORARY_PERMISSION_EXPIRED",
		},
		{
			name: "inactive user takes precedence",
			repo: &accessExplainRepository{
				user:   &domain.AccessUserRecord{UserID: 1001, Username: "operator", Status: 1},
				grants: []domain.AccessGrantRecord{{PermissionCode: "system:user:update", GrantSource: "TEMPORARY"}},
			},
			permission: "system:user:update", decision: "DENY", reason: "USER_INACTIVE",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewAccessExplainService(tt.repo, domain.NewService(), features.Set{}, accessTestSnapshotter{})
			service.now = func() time.Time { return now }
			result, err := service.ExplainPermission(context.Background(), 1001, tt.permission)
			if err != nil {
				t.Fatalf("ExplainPermission() error = %v", err)
			}
			if result.Decision != tt.decision || result.ReasonCode != tt.reason {
				t.Fatalf("unexpected explanation: %#v", result)
			}
		})
	}
}

func TestAccessExplainServiceUsesStableAuthorizationRootIdentity(t *testing.T) {
	repo := &accessExplainRepository{
		user: &domain.AccessUserRecord{UserID: 1001, Username: "owner"},
		roles: []domain.AccessRoleSourceRecord{{
			RoleID: 1, RoleCode: "CUSTOM_OWNER", RoleStatus: 0, RoleDataScope: 1,
			RoleSystemKey: domain.AuthorizationRootSystemKey, AssignmentSource: "DIRECT_USER",
		}},
	}
	service := NewAccessExplainService(repo, domain.NewService(), nil, accessTestSnapshotter{})
	result, err := service.ExplainPermission(context.Background(), 1001, "unregistered:permission")
	if err != nil {
		t.Fatalf("ExplainPermission() error = %v", err)
	}
	if result.Decision != "ALLOW" || result.ReasonCode != "AUTHORIZATION_ROOT_BYPASS" {
		t.Fatalf("unexpected root explanation: %#v", result)
	}
}

type accessExplainRepository struct {
	user        *domain.AccessUserRecord
	roles       []domain.AccessRoleSourceRecord
	grants      []domain.AccessGrantRecord
	roleDepts   []domain.AccessRoleDeptRecord
	memberships []domain.AccessMembershipRecord
	descendants map[string][]int64
	menus       []domain.MenuRecord
	calls       int
}

func (r *accessExplainRepository) FindAccessUser(context.Context, int64) (*domain.AccessUserRecord, error) {
	r.calls++
	return r.user, nil
}
func (r *accessExplainRepository) ListAccessRoleSources(context.Context, int64) ([]domain.AccessRoleSourceRecord, error) {
	r.calls++
	return r.roles, nil
}
func (r *accessExplainRepository) ListAccessGrantRecords(context.Context, int64) ([]domain.AccessGrantRecord, error) {
	r.calls++
	return r.grants, nil
}
func (r *accessExplainRepository) ListAccessRoleDeptRecords(context.Context, []int64) ([]domain.AccessRoleDeptRecord, error) {
	r.calls++
	return r.roleDepts, nil
}
func (r *accessExplainRepository) ListAccessMemberships(context.Context, int64) ([]domain.AccessMembershipRecord, error) {
	r.calls++
	return r.memberships, nil
}
func (r *accessExplainRepository) ListDeptIDsByHierarchies(context.Context, []string) (map[string][]int64, error) {
	r.calls++
	if r.descendants == nil {
		return map[string][]int64{}, nil
	}
	return r.descendants, nil
}
func (r *accessExplainRepository) ListAllMenus(context.Context) ([]domain.MenuRecord, error) {
	r.calls++
	return r.menus, nil
}

func timePointer(value time.Time) *time.Time { return &value }

func findEffectivePermission(records []authorizationfacade.EffectivePermissionVO, code string) *authorizationfacade.EffectivePermissionVO {
	for index := range records {
		if records[index].PermissionCode == code {
			return &records[index]
		}
	}
	return nil
}
