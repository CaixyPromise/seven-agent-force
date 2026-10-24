package facade

import (
	"context"
	"time"
)

type SubjectFacade interface {
	FindSubjectByID(ctx context.Context, userID int64) (*SubjectRecord, error)
	FindSubjectByAccount(ctx context.Context, account string) (*SubjectRecord, error)
	FindSubjectByEmail(ctx context.Context, email string) (*SubjectRecord, error)
	CreateExternalSubject(ctx context.Context, command CreateExternalSubjectCommand) (*SubjectRecord, error)
	CreateFormSubject(ctx context.Context, command CreateFormSubjectCommand) (*SubjectRecord, error)
	ExistsByID(ctx context.Context, userID int64) (bool, error)
	BuildPrincipalSeed(ctx context.Context, userID int64) (*UserPrincipalSeed, error)
}

type ProfileFacade interface {
	GetProfileByUserID(ctx context.Context, userID int64) (*UserProfile, error)
	UpdateSelfProfile(ctx context.Context, command UpdateSelfProfileCommand) error
	CommitCurrentUserAvatar(ctx context.Context, userID, fileID int64) (string, error)
	UpdateSelfEmail(ctx context.Context, command UpdateSelfEmailCommand) error
	SyncExternalProfile(ctx context.Context, command SyncExternalProfileCommand) error
}

// NotificationRecipientFacade exposes only verified delivery readiness. It
// never returns unverified historical addresses to Notification Center.
type NotificationRecipientFacade interface {
	ResolveVerifiedNotificationEmail(ctx context.Context, userID int64) (email string, ready bool, err error)
}

type AccountFacade interface {
	VerifyPassword(ctx context.Context, userID int64, rawPassword string) (bool, error)
	UpdatePassword(ctx context.Context, command UpdatePasswordCommand) error
	UpdateLockState(ctx context.Context, command UpdateLockStateCommand) error
}

type ProvisioningFacade interface {
	CreateOwnerUser(ctx context.Context, command CreateOwnerUserCommand) (*ProvisionedUser, error)
	FindUserByAccount(ctx context.Context, account string) (*SubjectRecord, error)
}

type AdminUserFacade interface {
	QueryUsers(ctx context.Context, query AdminUserQuery) (*PageResult[AdminUserVO], error)
	GetAdminUser(ctx context.Context, userID int64) (*AdminUserVO, error)
	CreateAdminUser(ctx context.Context, command AdminUserCreateCommand) (int64, error)
	UpdateAdminUser(ctx context.Context, command AdminUserUpdateCommand) error
	DeleteAdminUser(ctx context.Context, command AdminUserDeleteCommand) error
	UpdateAdminUserStatus(ctx context.Context, command AdminUserStatusCommand) error
	ResetAdminUserPassword(ctx context.Context, command AdminPasswordResetCommand) error
}

// UserSelectorFacade exposes bounded, minimum-field lookups for logged-in selectors.
type UserSelectorFacade interface {
	ListUserOptions(ctx context.Context, query UserSelectorQuery) ([]SimpleUserVO, error)
	GetSimpleUser(ctx context.Context, userID int64, scope DataScopeFilter) (*SimpleUserVO, error)
}

// SetManagedUserStatusCommand sets an absolute status for a trusted Node caller.
type SetManagedUserStatusCommand struct {
	UserID            int64
	Status            int
	ExpectedStatus    int
	ExpectedVersion   uint64
	Cutoff            time.Time
	StatusCommandHash string
}

// ManagedUserStatusSnapshot is an immutable optimistic status precondition.
type ManagedUserStatusSnapshot struct {
	Status            int
	Version           uint64
	StatusCommandHash string
}

// ManagedUserStatusFacade reuses user domain rules without admin step-up proof.
type ManagedUserStatusFacade interface {
	GetManagedUserStatusSnapshot(ctx context.Context, userID int64) (*ManagedUserStatusSnapshot, error)
	SetManagedUserStatus(ctx context.Context, command SetManagedUserStatusCommand) (int64, error)
}

type UserRelationFacade interface {
	ListUserRoleIDs(ctx context.Context, userID int64) ([]int64, error)
	AssignUserRoles(ctx context.Context, command RelationAssignCommand) error
	ListUserOrgIDs(ctx context.Context, userID int64) ([]int64, error)
	AssignUserOrgs(ctx context.Context, command RelationAssignCommand) error
	ListUserDeptIDs(ctx context.Context, userID int64) ([]int64, error)
	AssignUserDepts(ctx context.Context, command RelationAssignCommand) error
	ListUserPostIDs(ctx context.Context, userID int64) ([]int64, error)
	AssignUserPosts(ctx context.Context, command RelationAssignCommand) error
	ListActiveUserIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error)
}

// NotificationAudienceFacade exposes only the bounded role-member projection
// needed by the notification module. It is separate from the wider relation
// facade so existing cross-module callers do not accidentally acquire a bulk
// recipient expansion dependency.
type NotificationAudienceFacade interface {
	ListActiveUserIDsByRoleIDPage(ctx context.Context, roleID, afterUserID int64, limit int) ([]int64, error)
}

type AuthorizationContextFacade interface {
	GetAuthorizationUserAggregate(ctx context.Context, userID int64) (*AuthorizationUserAggregate, error)
	ListAuthorizationOrganizations(ctx context.Context, userID int64) ([]AuthorizationOrgRecord, error)
	ListAuthorizationDepartments(ctx context.Context, userID int64) ([]AuthorizationDeptRecord, error)
	ListAuthorizationPosts(ctx context.Context, userID int64) ([]AuthorizationPostRecord, error)
	ListDeptHierarchyMap(ctx context.Context, deptIDs []int64) (map[int64]string, error)
	ListDeptIDsByHierarchies(ctx context.Context, hierarchies []string) (map[string][]int64, error)
}

type OrgFacade interface {
	CreateOrg(ctx context.Context, command OrgCommand) error
	UpdateOrg(ctx context.Context, command OrgCommand) error
	DeleteOrg(ctx context.Context, orgID int64) error
	GetOrgByID(ctx context.Context, orgID int64) (*OrgVO, error)
	GetOrgByCode(ctx context.Context, code string) (*OrgVO, error)
	GetOrgByUserID(ctx context.Context, userID int64) (*OrgVO, error)
	GetOrgTree(ctx context.Context) ([]OrgVO, error)
	ListActiveOrgs(ctx context.Context) ([]OrgVO, error)
	ListOrgChildren(ctx context.Context, parentID int64) ([]OrgVO, error)
	CheckOrgCode(ctx context.Context, code string, excludeID int64) (bool, error)
	ChangeOrgStatus(ctx context.Context, orgID int64, status int, operatorID int64) error
	MoveOrg(ctx context.Context, orgID, newParentID int64, operatorID int64) error
}

type DeptFacade interface {
	GetDeptTree(ctx context.Context, enabledOnly bool) ([]DeptVO, error)
	SearchDepts(ctx context.Context, keyword string, orgID int64, status *int, limit int) ([]DeptVO, error)
	GetDeptByID(ctx context.Context, deptID int64) (*DeptVO, error)
	CreateDept(ctx context.Context, command DeptCommand) error
	UpdateDept(ctx context.Context, command DeptCommand) error
	DeleteDept(ctx context.Context, deptID int64) error
	GetChildDeptIDs(ctx context.Context, deptID int64) ([]int64, error)
}

type PostFacade interface {
	QueryPosts(ctx context.Context, query PostQuery) (*PageResult[PostVO], error)
	ListEnabledPosts(ctx context.Context) ([]PostVO, error)
	GetPostByID(ctx context.Context, postID int64) (*PostVO, error)
	ListPostsByIDs(ctx context.Context, postIDs []int64) ([]PostVO, error)
	CreatePost(ctx context.Context, command PostCommand) error
	UpdatePost(ctx context.Context, command PostCommand) error
	DeletePost(ctx context.Context, postID int64) error
	BatchDeletePosts(ctx context.Context, postIDs []int64) error
	ChangePostStatus(ctx context.Context, postID int64, status int, operatorID int64) error
	ListPostRoleIDs(ctx context.Context, postID int64) ([]int64, error)
	AssignPostRoles(ctx context.Context, command PostRoleAssignCommand) error
	ListPostIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error)
}
