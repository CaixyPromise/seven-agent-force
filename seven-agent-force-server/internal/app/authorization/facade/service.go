package facade

import "context"

type AuthFacade interface {
	GetLoginUser(ctx context.Context, request RequestScope) (*UserVO, error)
	GetLoginUserPermitNull(ctx context.Context, request RequestScope) (*UserVO, error)
	GetLoginUserID(ctx context.Context, request RequestScope) (int64, error)
	GetLoginUsername(ctx context.Context, request RequestScope) (string, error)
	IsLogin(ctx context.Context, request RequestScope) bool
	IsAdmin(ctx context.Context, request RequestScope) bool
	IsCurrentUser(ctx context.Context, request RequestScope, userID int64) bool
	IsAdminOrCurrentUser(ctx context.Context, request RequestScope, userID int64) bool
	GetUserVO(ctx context.Context, userID int64) (*UserVO, error)
	RefreshUserPermissionCache(ctx context.Context, userID int64) error
	GetUserPermissionsByModule(ctx context.Context, request RequestScope, module string) ([]string, error)
	CreateStepUpChallenge(ctx context.Context, request RequestScope, command StepUpChallengeRequest) (*StepUpChallengeVO, error)
	VerifyStepUp(ctx context.Context, request RequestScope, command StepUpVerifyRequest) (*StepUpTokenVO, error)
	ValidateStepUpToken(ctx context.Context, request RequestScope, command StepUpValidateRequest) (bool, error)
}

type PermissionFacade interface {
	GetUserPermissions(ctx context.Context, userID int64) ([]string, error)
	GetUserRoles(ctx context.Context, userID int64) ([]string, error)
	HasPermission(ctx context.Context, userID int64, permission string) (bool, error)
	HasRole(ctx context.Context, userID int64, role string) (bool, error)
	GetUserDataScope(ctx context.Context, userID int64) (*UserDataScopeVO, error)
	RefreshUserPermissionCache(ctx context.Context, userID int64) error
	ValidatePostRoleAssignment(ctx context.Context, userID, postID, roleID int64) (bool, error)
	ValidateUserPostAssignment(ctx context.Context, userID, postID int64) (bool, error)
}

// PostRoleAssignmentGuardFacade validates role parents while holding their
// write guards in the caller's active consistent transaction.
type PostRoleAssignmentGuardFacade interface {
	LockAndValidatePostRoleAssignments(ctx context.Context, userID, postID int64, roleIDs []int64) (bool, error)
}

type AccessExplainFacade interface {
	GetEffectiveAccess(ctx context.Context, userID int64, query EffectiveAccessQuery) (*EffectiveAccessVO, error)
	ExplainPermission(ctx context.Context, userID int64, permissionCode string) (*PermissionExplainVO, error)
}

type UserRoleAssignmentFacade interface {
	AssignUserRoles(ctx context.Context, command AssignUserRolesCommand) error
	ValidateCreatedUserRoles(ctx context.Context, command AssignCreatedUserRolesCommand) error
	AssignCreatedUserRoles(ctx context.Context, command AssignCreatedUserRolesCommand) error
	AssignProvisionedUserRoles(ctx context.Context, command AssignProvisionedUserRolesCommand) error
	BootstrapOwnerRoles(ctx context.Context, command BootstrapOwnerRolesCommand) error
	GuardUserDeactivation(ctx context.Context, userID int64) error
	IsAuthorizationRootUser(ctx context.Context, userID int64) (bool, error)
}

type RoleFacade interface {
	UserRoleAssignmentFacade
	PageRoles(ctx context.Context, query RolePageQuery) (*RolePageVO, error)
	GetRoleList(ctx context.Context) ([]RoleVO, error)
	GetRole(ctx context.Context, roleID int64) (*RoleVO, error)
	GetRootSecurityStatus(ctx context.Context) (*RoleSecurityStatusVO, error)
	BootstrapAuthorizationRoot(ctx context.Context, command BootstrapAuthorizationRootCommand) (*BootstrapAuthorizationRootResult, error)
	CreateRole(ctx context.Context, command RoleCommand) (*RoleVO, error)
	UpdateRole(ctx context.Context, command RoleCommand) (*RoleVO, error)
	DeleteRole(ctx context.Context, roleID int64, operatorID int64) error
	GetRoleDeptIDs(ctx context.Context, roleID int64) (*RoleDeptIDsVO, error)
	AssignRoleDepts(ctx context.Context, command AssignRoleDeptsCommand) error
	GetRoleMenuIDs(ctx context.Context, roleID int64) ([]int64, error)
	GetRoleMenuTree(ctx context.Context, roleID int64) ([]MenuTreeNodeVO, error)
	AssignRoleMenus(ctx context.Context, command AssignRoleMenusCommand) error
	AssignRolePermissions(ctx context.Context, command AssignRolePermissionsCommand) error
	GetRoleGrantSnapshot(ctx context.Context, roleID int64) (*RoleGrantSnapshotVO, error)
	PreviewRoleGrantBundle(ctx context.Context, command PreviewRoleGrantBundleCommand) (*RoleGrantPreviewVO, error)
	CommitRoleGrantBundle(ctx context.Context, command CommitRoleGrantBundleCommand) (*RoleGrantCommitVO, error)
	AdvanceRoleGrantRevision(ctx context.Context, roleID int64, operatorID int64) error
	GetMenuTree(ctx context.Context, enabledOnly bool) ([]MenuTreeNodeVO, error)
	GetMenu(ctx context.Context, menuID int64) (*MenuTreeNodeVO, error)
	CreateMenu(ctx context.Context, command MenuCommand) (*MenuTreeNodeVO, error)
	UpdateMenu(ctx context.Context, command MenuCommand) (*MenuTreeNodeVO, error)
	DeleteMenu(ctx context.Context, menuID int64, operatorID int64) error
	ListPermissions(ctx context.Context, query PermissionQuery) ([]PermissionVO, error)
	PagePermissions(ctx context.Context, query PermissionPageQuery) (*PermissionPageVO, error)
	GetPermission(ctx context.Context, permissionID int64) (*PermissionVO, error)
	CreatePermission(ctx context.Context, command PermissionCommand) (*PermissionVO, error)
	UpdatePermission(ctx context.Context, permissionID int64, command PermissionCommand) (*PermissionVO, error)
	DeletePermission(ctx context.Context, permissionID int64, operatorID int64) error
	GetMenuPermissionIDs(ctx context.Context, menuID int64) ([]int64, error)
	BindMenuPermissions(ctx context.Context, command MenuPermissionAssignCommand) error
}

// RoleGrantConfigScopePort lets the role grant policy participate in the config module's persistence rules.
// Callers pass the shared transaction context; implementations must not start an independent transaction.
type RoleGrantConfigScopePort interface {
	ListRoleConfigScopes(ctx context.Context, roleID int64) ([]RoleConfigScopeGrantVO, error)
	NormalizeRoleConfigScopes(ctx context.Context, grants []RoleConfigScopeGrantVO) ([]RoleConfigScopeGrantVO, error)
	ReplaceRoleConfigScopes(ctx context.Context, roleID int64, grants []RoleConfigScopeGrantVO, operatorID int64) error
}
