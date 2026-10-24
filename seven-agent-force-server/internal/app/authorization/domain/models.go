package domain

import (
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type UserAggregate struct {
	UserID   int64  `db:"user_id"`
	Username string `db:"username"`
	Nickname string `db:"nickname"`
	Avatar   string `db:"avatar"`
	Email    string `db:"email"`
	Phone    string `db:"phone"`
	// Enabled and Locked are source-authoritative account gates. They are
	// checked before any governed authorization candidate is considered.
	Enabled       bool  `db:"enabled"`
	Locked        bool  `db:"locked"`
	PrimaryOrgID  int64 `db:"primary_org_id"`
	PrimaryDeptID int64 `db:"primary_dept_id"`
	PrimaryPostID int64 `db:"primary_post_id"`
	IsAdmin       bool  `db:"is_admin"`
}

type RoleRecord struct {
	RoleID        int64      `db:"role_id"`
	Name          string     `db:"name"`
	Code          string     `db:"code"`
	SystemKey     string     `db:"system_key"`
	Type          int        `db:"type"`
	Status        int        `db:"status"`
	DataScope     int        `db:"data_scope"`
	SortOrder     int        `db:"sort_order"`
	Remark        string     `db:"remark"`
	GrantRevision int64      `db:"grant_revision"`
	CreateTime    *time.Time `db:"create_time"`
	UpdateTime    *time.Time `db:"update_time"`
}

// RoleGrantRequestRecord records an idempotent role grant commit result.
type RoleGrantRequestRecord struct {
	RoleID            int64  `db:"role_id"`
	IdempotencyKey    string `db:"idempotency_key"`
	RequestHash       string `db:"request_hash"`
	ResultRevision    int64  `db:"result_revision"`
	ImpactedUserCount int    `db:"impacted_user_count"`
	Changed           int    `db:"changed"`
}

type MenuRecord struct {
	MenuID      int64      `db:"menu_id"`
	ParentID    int64      `db:"parent_id"`
	SortOrder   int        `db:"sort_order"`
	Name        string     `db:"name"`
	Path        string     `db:"path"`
	Component   string     `db:"component"`
	Type        string     `db:"type"`
	Permission  string     `db:"permission"`
	FeatureCode string     `db:"feature_code"`
	Icon        string     `db:"icon"`
	Status      int        `db:"status"`
	Visible     int        `db:"visible"`
	IsFrame     int        `db:"is_frame"`
	IsCache     int        `db:"is_cache"`
	Remark      string     `db:"remark"`
	CreateTime  *time.Time `db:"create_time"`
	UpdateTime  *time.Time `db:"update_time"`
}

type PermissionRecord struct {
	PermissionID int64      `db:"permission_id"`
	Code         string     `db:"code"`
	FeatureCode  string     `db:"feature_code"`
	Name         string     `db:"name"`
	ResourceType string     `db:"resource_type"`
	Method       string     `db:"method"`
	Path         string     `db:"path"`
	Status       int        `db:"status"`
	Description  string     `db:"description"`
	CreateTime   *time.Time `db:"create_time"`
	UpdateTime   *time.Time `db:"update_time"`
}

type RolePermissionAssignment struct {
	RoleID              int64
	DirectPermissionIDs []int64
	MenuPermissionIDs   []int64
}

type OrgRecord struct {
	OrgID     int64  `db:"org_id"`
	Code      string `db:"code"`
	Name      string `db:"name"`
	IsPrimary bool   `db:"is_primary"`
}

type DeptRecord struct {
	DeptID    int64  `db:"dept_id"`
	OrgID     int64  `db:"org_id"`
	Code      string `db:"code"`
	Name      string `db:"name"`
	Hierarchy string `db:"hierarchy"`
	IsPrimary bool   `db:"is_primary"`
}

type PostRecord struct {
	PostID    int64  `db:"post_id"`
	OrgID     int64  `db:"org_id"`
	DeptID    int64  `db:"dept_id"`
	Code      string `db:"code"`
	Name      string `db:"name"`
	IsPrimary bool   `db:"is_primary"`
}

type UserDataScope struct {
	DeptIDs   []int64
	OrgIDs    []int64
	ScopeType securitycontext.DataScopeType
}

type TemporaryPermissionRecord struct {
	UserID         int64      `db:"user_id"`
	PermissionCode string     `db:"permission_code"`
	PermissionName string     `db:"permission_name"`
	Type           int        `db:"type"`
	ExpireAt       *time.Time `db:"expire_at"`
	Source         string     `db:"source"`
	Reason         string     `db:"reason"`
	GrantedBy      int64      `db:"granted_by"`
	GrantedAt      *time.Time `db:"granted_at"`
	UpdatedAt      *time.Time `db:"updated_at"`
}

type TemporaryPermissionStats struct {
	TotalActive  int64 `db:"total_active"`
	Temporary    int64 `db:"temporary"`
	Permanent    int64 `db:"permanent"`
	ExpiringSoon int64 `db:"expiring_soon"`
}
