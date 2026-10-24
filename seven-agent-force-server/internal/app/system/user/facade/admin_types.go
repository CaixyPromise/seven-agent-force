package facade

import (
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type PageResult[T any] struct {
	Current int64 `json:"current"`
	Size    int64 `json:"size"`
	Total   int64 `json:"total"`
	Records []T   `json:"records"`
}

type AdminUserQuery struct {
	Current  int64
	Size     int64
	Username string
	Nickname string
	Status   *int
	OrgID    int64
	DeptID   int64
	PostID   int64
	Scope    DataScopeFilter
}

type AdminUserVO struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Avatar      string     `json:"avatar,omitempty"`
	Email       string     `json:"email,omitempty"`
	UserPhone   string     `json:"userPhone,omitempty"`
	UserGender  *int       `json:"userGender,omitempty"`
	Status      int        `json:"status"`
	UserProfile string     `json:"userProfile,omitempty"`
	RoleIDs     []int64    `json:"roleIds,omitempty"`
	OrgIDs      []int64    `json:"orgIds,omitempty"`
	DeptIDs     []int64    `json:"deptIds,omitempty"`
	PostIDs     []int64    `json:"postIds,omitempty"`
	CreateTime  *time.Time `json:"createTime,omitempty"`
	UpdateTime  *time.Time `json:"updateTime,omitempty"`
}

type AdminUserCreateCommand struct {
	Username    string               `json:"username"`
	Nickname    string               `json:"nickname"`
	Password    string               `json:"password"`
	Email       string               `json:"email,omitempty"`
	UserPhone   string               `json:"userPhone,omitempty"`
	UserGender  *int                 `json:"userGender,omitempty"`
	Status      *int                 `json:"status,omitempty"`
	Remark      string               `json:"remark,omitempty"`
	OperatorID  int64                `json:"operatorId,omitempty"`
	RoleIDs     []int64              `json:"roleIds,omitempty"`
	OrgIDs      []int64              `json:"orgIds,omitempty"`
	DeptIDs     []int64              `json:"deptIds,omitempty"`
	PostIDs     []int64              `json:"postIds,omitempty"`
	StepUpProof stepup.ProofMetadata `json:"-"`
}

type AdminUserUpdateCommand struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username,omitempty"`
	Nickname   string  `json:"nickname,omitempty"`
	Email      string  `json:"email,omitempty"`
	UserPhone  string  `json:"userPhone,omitempty"`
	UserGender *int    `json:"userGender,omitempty"`
	Status     *int    `json:"status,omitempty"`
	Remark     *string `json:"remark,omitempty"`
	OperatorID int64   `json:"operatorId,omitempty"`
}

type AdminPasswordResetCommand struct {
	UserID      int64  `json:"userId"`
	RawPassword string `json:"rawPassword,omitempty"`
	OperatorID  int64  `json:"operatorId,omitempty"`
}

type AdminUserDeleteCommand struct {
	UserID      int64                `json:"userId"`
	OperatorID  int64                `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata `json:"-"`
}

type AdminUserStatusCommand struct {
	UserID      int64                `json:"userId"`
	Status      int                  `json:"status"`
	OperatorID  int64                `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata `json:"-"`
}

type RelationAssignCommand struct {
	UserID      int64                `json:"userId"`
	IDs         []int64              `json:"ids"`
	PrimaryID   int64                `json:"primaryId,omitempty"`
	OperatorID  int64                `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata `json:"-"`
}

type OrgVO struct {
	ID           int64   `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	ParentID     int64   `json:"parentId"`
	Status       int     `json:"status"`
	SortOrder    int     `json:"sortOrder"`
	LeaderUserID int64   `json:"leaderUserId,omitempty"`
	Children     []OrgVO `json:"children,omitempty"`
}

type OrgCommand struct {
	ID           int64  `json:"id,omitempty"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	ParentID     int64  `json:"parentId"`
	Status       *int   `json:"status,omitempty"`
	SortOrder    *int   `json:"sortOrder,omitempty"`
	LeaderUserID int64  `json:"leaderUserId,omitempty"`
	OperatorID   int64  `json:"operatorId,omitempty"`
}

type DeptVO struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Code         string   `json:"code"`
	OrgID        int64    `json:"orgId"`
	ParentID     int64    `json:"parentId"`
	LeaderUserID int64    `json:"leaderUserId,omitempty"`
	Status       int      `json:"status"`
	SortOrder    int      `json:"sortOrder"`
	Hierarchy    string   `json:"hierarchy,omitempty"`
	Level        int      `json:"level"`
	Children     []DeptVO `json:"children,omitempty"`
}

type DeptCommand struct {
	ID           int64  `json:"id,omitempty"`
	Name         string `json:"name"`
	Code         string `json:"code"`
	OrgID        int64  `json:"orgId"`
	ParentID     int64  `json:"parentId"`
	LeaderUserID int64  `json:"leaderUserId,omitempty"`
	Status       *int   `json:"status,omitempty"`
	SortOrder    *int   `json:"sortOrder,omitempty"`
	OperatorID   int64  `json:"operatorId,omitempty"`
}

type PostVO struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	DeptID    int64  `json:"deptId,omitempty"`
	OrgID     int64  `json:"orgId,omitempty"`
	SortOrder int    `json:"sortOrder"`
	Status    int    `json:"status"`
	Remark    string `json:"remark,omitempty"`
}

type PostCommand struct {
	ID         int64  `json:"id,omitempty"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	DeptID     int64  `json:"deptId,omitempty"`
	OrgID      int64  `json:"orgId,omitempty"`
	SortOrder  *int   `json:"sortOrder,omitempty"`
	Status     *int   `json:"status,omitempty"`
	Remark     string `json:"remark,omitempty"`
	OperatorID int64  `json:"operatorId,omitempty"`
}

type PostQuery struct {
	Current int64
	Size    int64
	Name    string
	Code    string
	Status  *int
	Scope   DataScopeFilter
}

type DataScopeFilter struct {
	Enabled    bool
	None       bool
	ScopeType  string
	SelfUserID int64
	DeptIDs    []int64
	OrgIDs     []int64
}

type PostRoleAssignCommand struct {
	PostID      int64                `json:"postId"`
	RoleIDs     []int64              `json:"roleIds"`
	OperatorID  int64                `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata `json:"-"`
}

type AuthorizationUserAggregate struct {
	UserID   int64
	Username string
	Nickname string
	Avatar   string
	Email    string
	Phone    string
	// Enabled and Locked are authoritative source-state gates for an
	// authorization snapshot. They are not session state and must be checked
	// before a caller can use a cached authorization result.
	Enabled       bool
	Locked        bool
	PrimaryOrgID  int64
	PrimaryDeptID int64
	PrimaryPostID int64
}

type AuthorizationOrgRecord struct {
	OrgID     int64
	Code      string
	Name      string
	IsPrimary bool
}

type AuthorizationDeptRecord struct {
	DeptID    int64
	OrgID     int64
	Code      string
	Name      string
	Hierarchy string
	IsPrimary bool
}

type AuthorizationPostRecord struct {
	PostID    int64
	OrgID     int64
	DeptID    int64
	Code      string
	Name      string
	IsPrimary bool
}
