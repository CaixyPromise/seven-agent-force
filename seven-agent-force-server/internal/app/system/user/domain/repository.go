package domain

import (
	"context"
	"time"
)

type Repository interface {
	FindSubjectByID(ctx context.Context, userID int64) (*SubjectRecord, error)
	FindSubjectByAccount(ctx context.Context, account string) (*SubjectRecord, error)
	FindSubjectByEmail(ctx context.Context, email string) (*SubjectRecord, error)
	ExistsByID(ctx context.Context, userID int64) (bool, error)
	CountByPhoneExcludingUserID(ctx context.Context, userID int64, phone string) (int, error)
	CountByEmailExcludingUserID(ctx context.Context, userID int64, email string) (int, error)
	CountByEmail(ctx context.Context, email string) (int, error)
	CreateOwnerUser(ctx context.Context, record *OwnerUserRecord) error
	CreateExternalSubject(ctx context.Context, record ExternalSubjectCreateRecord) error
	CreateFormSubject(ctx context.Context, record FormSubjectCreateRecord) error
	UpdateProfile(ctx context.Context, userID int64, nickName, phone, avatar, profile *string) error
	UpdateEmail(ctx context.Context, userID int64, email string) error
	CompareAndSetVerifiedEmail(ctx context.Context, userID int64, email string, expectedVersion int64, verificationSource string, verifiedAt time.Time) (bool, error)
	UpdateLockState(ctx context.Context, userID int64, status int, unsealAt *time.Time) error
	CompareAndSetManagedUserStatus(ctx context.Context, userID int64, expectedStatus int, expectedVersion uint64, status int, unsealAt *time.Time, statusCommandHash string) (bool, error)
	QueryAdminUsers(ctx context.Context, query AdminUserQuery) ([]AdminUserRecord, int64, error)
	FindAdminUserByID(ctx context.Context, userID int64) (*AdminUserRecord, error)
	ListUserOptions(ctx context.Context, query UserSelectorQuery) ([]UserSelectorRecord, error)
	FindVisibleUserOptionByID(ctx context.Context, userID int64, scope DataScopeFilter) (*UserSelectorRecord, error)
	CreateAdminUser(ctx context.Context, record AdminUserCreateRecord) error
	UpdateAdminUser(ctx context.Context, record AdminUserUpdateRecord) error
	SoftDeleteUser(ctx context.Context, userID, operatorID int64) error
	CountByAccountExcludingUserID(ctx context.Context, userID int64, account string) (int, error)
	ReplaceUserOrgs(ctx context.Context, userID int64, orgIDs []int64, primaryOrgID int64, operatorID int64) error
	ReplaceUserDepts(ctx context.Context, userID int64, deptIDs []int64, primaryDeptID int64, operatorID int64) error
	ReplaceUserPosts(ctx context.Context, userID int64, postIDs []int64, primaryPostID int64, operatorID int64) error
	ListUserRoleIDs(ctx context.Context, userID int64) ([]int64, error)
	ListUserOrgIDs(ctx context.Context, userID int64) ([]int64, error)
	ListUserDeptIDs(ctx context.Context, userID int64) ([]int64, error)
	ListUserPostIDs(ctx context.Context, userID int64) ([]int64, error)
	ListActiveUserIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error)
	ListActiveUserIDsByRoleIDPage(ctx context.Context, roleID, afterUserID int64, limit int) ([]int64, error)

	CreateOrg(ctx context.Context, record OrgRecord, operatorID int64) error
	UpdateOrg(ctx context.Context, record OrgRecord, operatorID int64) error
	DeleteOrg(ctx context.Context, orgID int64) error
	UpdateOrgStatus(ctx context.Context, orgID int64, status int, operatorID int64) error
	UpdateOrgParent(ctx context.Context, orgID, parentID, operatorID int64) error
	FindOrgByID(ctx context.Context, orgID int64) (*OrgRecord, error)
	FindOrgsByIDs(ctx context.Context, orgIDs []int64) ([]OrgRecord, error)
	FindOrgByCode(ctx context.Context, code string) (*OrgRecord, error)
	FindOrgByUserID(ctx context.Context, userID int64) (*OrgRecord, error)
	ListOrgs(ctx context.Context, enabledOnly bool) ([]OrgRecord, error)
	ListOrgChildren(ctx context.Context, parentID int64) ([]OrgRecord, error)
	CountOrgCodeExcludingID(ctx context.Context, orgID int64, code string) (int, error)
	CountOrgChildren(ctx context.Context, orgID int64) (int, error)
	CountDeptByOrgID(ctx context.Context, orgID int64) (int, error)
	CountUserOrgByOrgID(ctx context.Context, orgID int64) (int, error)

	CreateDept(ctx context.Context, record DeptRecord, operatorID int64) error
	UpdateDept(ctx context.Context, record DeptRecord, operatorID int64) error
	DeleteDept(ctx context.Context, deptID int64) error
	FindDeptByID(ctx context.Context, deptID int64) (*DeptRecord, error)
	FindDeptsByIDs(ctx context.Context, deptIDs []int64) ([]DeptRecord, error)
	ListDepts(ctx context.Context, enabledOnly bool, keyword string, orgID int64, status *int, limit int) ([]DeptRecord, error)
	ListChildDeptIDs(ctx context.Context, deptID int64) ([]int64, error)
	CountDeptNameUnderParent(ctx context.Context, deptID, parentID int64, name string) (int, error)
	CountDeptChildren(ctx context.Context, deptID int64) (int, error)
	CountUserDeptByDeptID(ctx context.Context, deptID int64) (int, error)

	QueryPosts(ctx context.Context, query PostQuery) ([]PostRecord, int64, error)
	ListEnabledPosts(ctx context.Context) ([]PostRecord, error)
	FindPostByID(ctx context.Context, postID int64) (*PostRecord, error)
	FindPostsByIDs(ctx context.Context, postIDs []int64) ([]PostRecord, error)
	CreatePost(ctx context.Context, record PostRecord, operatorID int64) error
	UpdatePost(ctx context.Context, record PostRecord, operatorID int64) error
	DeletePost(ctx context.Context, postID int64) error
	UpdatePostStatus(ctx context.Context, postID int64, status int, operatorID int64) error
	CountPostCodeExcludingID(ctx context.Context, postID int64, code string) (int, error)
	CountPostNameExcludingID(ctx context.Context, postID int64, name string) (int, error)
	CountUserPostByPostID(ctx context.Context, postID int64) (int, error)
	ListPostRoleIDs(ctx context.Context, postID int64) ([]int64, error)
	ReplacePostRoles(ctx context.Context, postID int64, roleIDs []int64, operatorID int64) error
	ListPostIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error)
}
