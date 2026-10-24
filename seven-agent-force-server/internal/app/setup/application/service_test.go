package application

import (
	"context"
	"testing"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	setupdomain "github.com/CaixyPromise/seven-agent/force/internal/app/setup/domain"
	setupfacade "github.com/CaixyPromise/seven-agent/force/internal/app/setup/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestGetSetupStatusReturnsTokenOnlyWhenUninitialized(t *testing.T) {
	service := newTestSetupService(t)
	status, err := service.GetSetupStatus(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Initialized || !status.OwnerRequired || status.SetupToken == nil || *status.SetupToken == "" {
		t.Fatalf("unexpected uninitialized status: %#v", status)
	}
	service.relations.(*fakeUserRelationFacade).activeUserIDs = []int64{1001}
	status, err = service.GetSetupStatus(context.Background())
	if err != nil {
		t.Fatalf("initialized status: %v", err)
	}
	if !status.Initialized || status.OwnerRequired || status.SetupToken != nil || !status.LoginEnabled {
		t.Fatalf("unexpected initialized status: %#v", status)
	}
	relations := service.relations.(*fakeUserRelationFacade)
	if relations.unboundedCalls != 0 || relations.pageCalls != 2 {
		t.Fatalf("setup root existence unbounded=%d pages=%d want 0/2", relations.unboundedCalls, relations.pageCalls)
	}
}

func TestCreateOwnerRejectsReplayAndBootstrapsSession(t *testing.T) {
	service := newTestSetupService(t)
	token, err := service.tokens.Generate(service.now())
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	result, err := service.CreateOwner(context.Background(), setupfacade.SetupOwnerRequestDTO{
		Username:        "Owner1",
		Password:        "Owner123",
		ConfirmPassword: "Owner123",
	}, token, &ssofacade.RequestContext{LoginIP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if result.Owner == nil || result.Owner.ID != 1001 || result.Owner.AccessToken != "access-token" {
		t.Fatalf("unexpected owner result: %#v", result)
	}
	if service.users.(*fakeProvisioning).created == nil || service.roles.(*fakeRoleFacade).bootstrappedUserID != 1001 {
		t.Fatalf("expected user provisioning and role assignment")
	}
	if service.permissions.(*fakePermissionFacade).refreshedUserID != 1001 {
		t.Fatalf("expected permission cache refresh")
	}
	if _, err := service.CreateOwner(context.Background(), setupfacade.SetupOwnerRequestDTO{
		Username:        "Owner2",
		Password:        "Owner123",
		ConfirmPassword: "Owner123",
	}, token, nil); apperrors.From(err).Code() != apperrors.CodeNoAuth {
		t.Fatalf("expected replay to be rejected as no-auth, got %v", err)
	}
}

func TestCreateOwnerRunsBootstrapInsideTransaction(t *testing.T) {
	service := newTestSetupService(t)
	tx := &fakeTransactor{enabled: true}
	service.transactor = tx
	token, err := service.tokens.Generate(service.now())
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := service.CreateOwner(context.Background(), setupfacade.SetupOwnerRequestDTO{
		Username:        "Owner1",
		Password:        "Owner123",
		ConfirmPassword: "Owner123",
	}, token, nil); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if tx.calls != 1 {
		t.Fatalf("expected one setup transaction, got %d", tx.calls)
	}
	if !service.users.(*fakeProvisioning).sawTx || !service.roles.(*fakeRoleFacade).sawTx || !service.ssoBootstrap.(*fakeBootstrapFacade).sawTx {
		t.Fatalf("expected user, role and sso bootstrap to receive transaction context")
	}
}

func TestCreateOwnerAppliesConfiguredRootCodeOnlyDuringFirstBootstrap(t *testing.T) {
	service := newTestSetupService(t)
	service.settings.Setup.Bootstrap.SuperAdminRoleCode = "PLATFORM_OWNER"
	service.settings.Setup.Bootstrap.SuperAdminRoleName = "平台所有者"
	token, err := service.tokens.Generate(service.now())
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := service.CreateOwner(context.Background(), setupfacade.SetupOwnerRequestDTO{Username: "Owner1", Password: "Owner123", ConfirmPassword: "Owner123"}, token, nil); err != nil {
		t.Fatalf("create owner with custom root: %v", err)
	}
	root := service.roles.(*fakeRoleFacade).roles[0]
	if root.Code != "PLATFORM_OWNER" || root.Name != "平台所有者" || !root.AuthorizationRoot {
		t.Fatalf("unexpected bootstrapped root: %#v", root)
	}
}

func newTestSetupService(t *testing.T) *Service {
	t.Helper()
	start := time.Unix(1713830400, 0).UTC()
	tokens, err := setupdomain.NewTokenService("12345678901234567890123456789012", 300, start.UnixMilli())
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	service := NewService(
		Settings{
			Setup: config.SetupConfig{
				Enabled:                   true,
				TokenTTLSeconds:           300,
				OwnerBootstrapLockSeconds: 30,
				BootstrapClientID:         "authorization-console",
				Bootstrap: config.SetupBootstrapConfig{
					SuperAdminRoleCode: "SUPER_ADMIN",
					SuperAdminRoleName: "超级管理员",
				},
			},
			LoginEnabled:       true,
			SSOFrontendPrimary: true,
			AppVersion:         "dev",
			AppCommit:          "dev",
			StartTime:          start,
		},
		tokens,
		newFakeStateStore(),
		&fakeTransactor{enabled: true},
		&fakeProvisioning{},
		&fakeUserRelationFacade{},
		&fakeRoleFacade{roles: []authorizationfacade.RoleVO{{RoleID: 10, Code: "SUPER_ADMIN", Status: 0, AuthorizationRoot: true}}},
		&fakePermissionFacade{},
		&fakeBootstrapFacade{},
	)
	service.now = func() time.Time { return start.Add(time.Second) }
	return service
}

type fakeStateStore struct {
	nonces map[string]bool
	locked bool
}

func newFakeStateStore() *fakeStateStore {
	return &fakeStateStore{nonces: map[string]bool{}}
}

func (f *fakeStateStore) ConsumeNonce(_ context.Context, nonce string, _ time.Duration) (bool, error) {
	if f.nonces[nonce] {
		return false, nil
	}
	f.nonces[nonce] = true
	return true, nil
}

func (f *fakeStateStore) AcquireBootstrapLock(context.Context, string, time.Duration) (bool, error) {
	if f.locked {
		return false, nil
	}
	f.locked = true
	return true, nil
}

func (f *fakeStateStore) ReleaseBootstrapLock(context.Context, string) error {
	f.locked = false
	return nil
}

type fakeProvisioning struct {
	created *userfacade.CreateOwnerUserCommand
	sawTx   bool
}

func (f *fakeProvisioning) CreateOwnerUser(ctx context.Context, command userfacade.CreateOwnerUserCommand) (*userfacade.ProvisionedUser, error) {
	f.sawTx = ctx.Value(fakeTxContextKey{}) == true
	copied := command
	f.created = &copied
	nickname := command.NickName
	if nickname == "" {
		nickname = command.AccountName
	}
	return &userfacade.ProvisionedUser{UserID: 1001, AccountName: command.AccountName, NickName: nickname}, nil
}

func (f *fakeProvisioning) FindUserByAccount(context.Context, string) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

type fakeUserRelationFacade struct {
	activeUserIDs  []int64
	unboundedCalls int
	pageCalls      int
}

func (f *fakeUserRelationFacade) ListUserRoleIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (f *fakeUserRelationFacade) AssignUserRoles(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}

func (f *fakeUserRelationFacade) ListUserOrgIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (f *fakeUserRelationFacade) AssignUserOrgs(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}

func (f *fakeUserRelationFacade) ListUserDeptIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (f *fakeUserRelationFacade) AssignUserDepts(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}

func (f *fakeUserRelationFacade) ListUserPostIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (f *fakeUserRelationFacade) AssignUserPosts(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}

func (f *fakeUserRelationFacade) ListActiveUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	f.unboundedCalls++
	return f.activeUserIDs, nil
}

func (f *fakeUserRelationFacade) ListActiveUserIDsByRoleIDPage(context.Context, int64, int64, int) ([]int64, error) {
	f.pageCalls++
	if len(f.activeUserIDs) == 0 {
		return []int64{}, nil
	}
	return append([]int64(nil), f.activeUserIDs[:1]...), nil
}

type fakeRoleFacade struct {
	authorizationfacade.UserRoleAssignmentFacade
	roles              []authorizationfacade.RoleVO
	bootstrappedUserID int64
	sawTx              bool
}

func (f *fakeRoleFacade) GetRoleList(context.Context) ([]authorizationfacade.RoleVO, error) {
	return f.roles, nil
}

func (f *fakeRoleFacade) PageRoles(context.Context, authorizationfacade.RolePageQuery) (*authorizationfacade.RolePageVO, error) {
	return &authorizationfacade.RolePageVO{Records: f.roles, Total: int64(len(f.roles)), Current: 1, Size: 10}, nil
}

func (f *fakeRoleFacade) GetRole(context.Context, int64) (*authorizationfacade.RoleVO, error) {
	if len(f.roles) == 0 {
		return nil, nil
	}
	return &f.roles[0], nil
}

func (f *fakeRoleFacade) GetRootSecurityStatus(context.Context) (*authorizationfacade.RoleSecurityStatusVO, error) {
	return &authorizationfacade.RoleSecurityStatusVO{}, nil
}

func (f *fakeRoleFacade) BootstrapAuthorizationRoot(ctx context.Context, command authorizationfacade.BootstrapAuthorizationRootCommand) (*authorizationfacade.BootstrapAuthorizationRootResult, error) {
	f.sawTx = ctx.Value(fakeTxContextKey{}) == true
	if len(f.roles) == 0 {
		return nil, nil
	}
	role := f.roles[0]
	role.Code = command.Code
	role.Name = command.Name
	role.AuthorizationRoot = true
	f.roles[0] = role
	return &authorizationfacade.BootstrapAuthorizationRootResult{Role: role}, nil
}

func (f *fakeRoleFacade) CreateRole(context.Context, authorizationfacade.RoleCommand) (*authorizationfacade.RoleVO, error) {
	return &authorizationfacade.RoleVO{RoleID: 1, ID: 1}, nil
}

func (f *fakeRoleFacade) UpdateRole(context.Context, authorizationfacade.RoleCommand) (*authorizationfacade.RoleVO, error) {
	return &authorizationfacade.RoleVO{RoleID: 1, ID: 1}, nil
}

func (f *fakeRoleFacade) DeleteRole(context.Context, int64, int64) error {
	return nil
}

func (f *fakeRoleFacade) GetRoleDeptIDs(context.Context, int64) (*authorizationfacade.RoleDeptIDsVO, error) {
	return &authorizationfacade.RoleDeptIDsVO{}, nil
}

func (f *fakeRoleFacade) AssignRoleDepts(context.Context, authorizationfacade.AssignRoleDeptsCommand) error {
	return nil
}

func (f *fakeRoleFacade) GetRoleMenuTree(context.Context, int64) ([]authorizationfacade.MenuTreeNodeVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) GetRoleMenuIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (f *fakeRoleFacade) AssignRoleMenus(context.Context, authorizationfacade.AssignRoleMenusCommand) error {
	return nil
}

func (f *fakeRoleFacade) AssignRolePermissions(context.Context, authorizationfacade.AssignRolePermissionsCommand) error {
	return nil
}

func (f *fakeRoleFacade) GetRoleGrantSnapshot(context.Context, int64) (*authorizationfacade.RoleGrantSnapshotVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) PreviewRoleGrantBundle(context.Context, authorizationfacade.PreviewRoleGrantBundleCommand) (*authorizationfacade.RoleGrantPreviewVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) CommitRoleGrantBundle(context.Context, authorizationfacade.CommitRoleGrantBundleCommand) (*authorizationfacade.RoleGrantCommitVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) AdvanceRoleGrantRevision(context.Context, int64, int64) error {
	return nil
}

func (f *fakeRoleFacade) AssignUserRoles(ctx context.Context, command authorizationfacade.AssignUserRolesCommand) error {
	return nil
}

func (f *fakeRoleFacade) BootstrapOwnerRoles(ctx context.Context, command authorizationfacade.BootstrapOwnerRolesCommand) error {
	f.sawTx = ctx.Value(fakeTxContextKey{}) == true
	f.bootstrappedUserID = command.UserID
	return nil
}

func (f *fakeRoleFacade) GetMenuTree(context.Context, bool) ([]authorizationfacade.MenuTreeNodeVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) GetMenu(context.Context, int64) (*authorizationfacade.MenuTreeNodeVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) CreateMenu(context.Context, authorizationfacade.MenuCommand) (*authorizationfacade.MenuTreeNodeVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) UpdateMenu(context.Context, authorizationfacade.MenuCommand) (*authorizationfacade.MenuTreeNodeVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) DeleteMenu(context.Context, int64, int64) error {
	return nil
}

func (f *fakeRoleFacade) ListPermissions(context.Context, authorizationfacade.PermissionQuery) ([]authorizationfacade.PermissionVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) PagePermissions(context.Context, authorizationfacade.PermissionPageQuery) (*authorizationfacade.PermissionPageVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) GetPermission(context.Context, int64) (*authorizationfacade.PermissionVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) CreatePermission(context.Context, authorizationfacade.PermissionCommand) (*authorizationfacade.PermissionVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) UpdatePermission(context.Context, int64, authorizationfacade.PermissionCommand) (*authorizationfacade.PermissionVO, error) {
	return nil, nil
}

func (f *fakeRoleFacade) DeletePermission(context.Context, int64, int64) error {
	return nil
}

func (f *fakeRoleFacade) GetMenuPermissionIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (f *fakeRoleFacade) BindMenuPermissions(context.Context, authorizationfacade.MenuPermissionAssignCommand) error {
	return nil
}

type fakePermissionFacade struct {
	refreshedUserID int64
}

func (f *fakePermissionFacade) GetUserPermissions(context.Context, int64) ([]string, error) {
	return []string{"*"}, nil
}

func (f *fakePermissionFacade) GetUserRoles(context.Context, int64) ([]string, error) {
	return []string{"SUPER_ADMIN"}, nil
}

func (f *fakePermissionFacade) HasPermission(context.Context, int64, string) (bool, error) {
	return true, nil
}

func (f *fakePermissionFacade) HasRole(context.Context, int64, string) (bool, error) {
	return true, nil
}

func (f *fakePermissionFacade) GetUserDataScope(context.Context, int64) (*authorizationfacade.UserDataScopeVO, error) {
	return nil, nil
}

func (f *fakePermissionFacade) RefreshUserPermissionCache(_ context.Context, userID int64) error {
	f.refreshedUserID = userID
	return nil
}

func (f *fakePermissionFacade) ValidatePostRoleAssignment(context.Context, int64, int64, int64) (bool, error) {
	return true, nil
}

func (f *fakePermissionFacade) ValidateUserPostAssignment(context.Context, int64, int64) (bool, error) {
	return true, nil
}

type fakeBootstrapFacade struct {
	sawTx bool
}

func (f *fakeBootstrapFacade) BootstrapFirstPartySession(ctx context.Context, _ ssofacade.BootstrapSessionCommand) (*ssofacade.BootstrapSessionResult, error) {
	f.sawTx = ctx.Value(fakeTxContextKey{}) == true
	return &ssofacade.BootstrapSessionResult{
		AccessToken:              "access-token",
		TokenType:                "Bearer",
		AccessTTLSeconds:         1800,
		SessionCookieHeaderValue: "SEVEN_SSO_SESSION=s1",
		RefreshCookieHeaderValue: "seven_sso_rt=r1",
	}, nil
}

type fakeTxContextKey struct{}

type fakeTransactor struct {
	enabled bool
	calls   int
}

func (f *fakeTransactor) Enabled() bool {
	return f.enabled
}

func (f *fakeTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.calls++
	return fn(context.WithValue(ctx, fakeTxContextKey{}, true))
}

func (f *fakeTransactor) WithinConsistentTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.calls++
	return fn(context.WithValue(ctx, fakeTxContextKey{}, true))
}

var _ store.Transactor = (*fakeTransactor)(nil)
var _ store.ConsistentTransactor = (*fakeTransactor)(nil)
