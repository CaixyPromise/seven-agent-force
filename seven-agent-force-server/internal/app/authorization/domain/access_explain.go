package domain

import "time"

type AccessUserRecord struct {
	UserID   int64  `db:"user_id"`
	Username string `db:"username"`
	Status   int    `db:"status"`
}

type AccessRoleSourceRecord struct {
	RoleID           int64  `db:"role_id"`
	RoleCode         string `db:"role_code"`
	RoleName         string `db:"role_name"`
	RoleStatus       int    `db:"role_status"`
	RoleDataScope    int    `db:"role_data_scope"`
	RoleSystemKey    string `db:"role_system_key"`
	AssignmentSource string `db:"assignment_source"`
	PostID           int64  `db:"post_id"`
	PostCode         string `db:"post_code"`
	PostName         string `db:"post_name"`
	PostDeptID       int64  `db:"post_dept_id"`
	PostOrgID        int64  `db:"post_org_id"`
}

type AccessGrantRecord struct {
	PermissionID     int64      `db:"permission_id"`
	PermissionCode   string     `db:"permission_code"`
	PermissionName   string     `db:"permission_name"`
	PermissionStatus int        `db:"permission_status"`
	FeatureCode      string     `db:"feature_code"`
	GrantSource      string     `db:"grant_source"`
	RoleID           int64      `db:"role_id"`
	RoleCode         string     `db:"role_code"`
	RoleName         string     `db:"role_name"`
	RoleStatus       int        `db:"role_status"`
	AssignmentSource string     `db:"assignment_source"`
	PostID           int64      `db:"post_id"`
	PostCode         string     `db:"post_code"`
	PostName         string     `db:"post_name"`
	PostDeptID       int64      `db:"post_dept_id"`
	PostOrgID        int64      `db:"post_org_id"`
	MenuID           int64      `db:"menu_id"`
	MenuName         string     `db:"menu_name"`
	MenuStatus       int        `db:"menu_status"`
	GrantedBy        int64      `db:"granted_by"`
	Source           string     `db:"source"`
	PermissionType   int        `db:"permission_type"`
	ExpireAt         *time.Time `db:"expire_at"`
}

type AccessRoleDeptRecord struct {
	RoleID int64 `db:"role_id"`
	DeptID int64 `db:"dept_id"`
}

type AccessMembershipRecord struct {
	Kind      string `db:"kind"`
	ID        int64  `db:"id"`
	OrgID     int64  `db:"org_id"`
	Hierarchy string `db:"hierarchy"`
}
