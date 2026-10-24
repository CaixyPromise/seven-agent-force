package domain

import "time"

const (
	UserStatusNormal        = 0
	UserStatusDisabled      = 1
	UserStatusPendingReview = 2
)

type SubjectRecord struct {
	UserID                  int64      `db:"user_id"`
	AccountName             string     `db:"account_name"`
	NickName                string     `db:"nick_name"`
	Email                   string     `db:"email"`
	Phone                   string     `db:"phone"`
	Avatar                  string     `db:"avatar"`
	Profile                 string     `db:"profile"`
	Status                  int        `db:"status"`
	UnsealAt                *time.Time `db:"unseal_at"`
	EmailVerifiedAt         *time.Time `db:"email_verified_at"`
	EmailVersion            int64      `db:"email_version"`
	EmailVerificationSource string     `db:"email_verification_source"`
}

type OwnerUserRecord struct {
	UserID      int64
	AccountName string
	NickName    string
	Email       string
	Avatar      string
	Profile     string
	Status      int
	Gender      int
}

type AdminUserRecord struct {
	ID                int64      `db:"id"`
	AccountName       string     `db:"userAccount"`
	NickName          string     `db:"nickName"`
	Avatar            string     `db:"userAvatar"`
	Email             string     `db:"userEmail"`
	Phone             string     `db:"userPhone"`
	Gender            *int       `db:"userGender"`
	Profile           string     `db:"userProfile"`
	Status            int        `db:"status"`
	StatusVersion     uint64     `db:"statusVersion"`
	StatusCommandHash string     `db:"statusCommandHash"`
	CreateTime        *time.Time `db:"createTime"`
	UpdateTime        *time.Time `db:"updateTime"`
}

type AdminUserQuery struct {
	Current  int64
	Size     int64
	Account  string
	Nickname string
	Status   *int
	OrgID    int64
	DeptID   int64
	PostID   int64
	Scope    DataScopeFilter
}

type AdminUserCreateRecord struct {
	ID          int64
	AccountName string
	NickName    string
	Email       string
	Phone       string
	Gender      *int
	Profile     string
	Status      int
	OperatorID  int64
}

type ExternalSubjectCreateRecord struct {
	ID                   int64
	AccountName          string
	NickName             string
	Email                string
	Avatar               string
	RegisterPlatformCode string
	RegisterProviderCode string
	Status               int
}

type FormSubjectCreateRecord struct {
	ID                   int64
	AccountName          string
	NickName             string
	Email                string
	RegisterPlatformCode string
	RegisterProviderCode string
	Status               int
}

type AdminUserUpdateRecord struct {
	ID         int64
	NickName   string
	Email      string
	Phone      string
	Gender     *int
	Profile    *string
	Status     *int
	OperatorID int64
}

type OrgRecord struct {
	ID           int64  `db:"id"`
	Code         string `db:"code"`
	Name         string `db:"name"`
	ParentID     int64  `db:"parentId"`
	Status       int    `db:"status"`
	SortOrder    int    `db:"sortOrder"`
	LeaderUserID int64  `db:"leaderUserId"`
}

type DeptRecord struct {
	ID           int64  `db:"id"`
	Code         string `db:"code"`
	Name         string `db:"name"`
	OrgID        int64  `db:"orgId"`
	ParentID     int64  `db:"parentId"`
	LeaderUserID int64  `db:"leaderUserId"`
	Status       int    `db:"status"`
	SortOrder    int    `db:"sortOrder"`
	Hierarchy    string `db:"hierarchy"`
	Level        int    `db:"level"`
}

type PostRecord struct {
	ID        int64  `db:"id"`
	Code      string `db:"code"`
	Name      string `db:"name"`
	DeptID    int64  `db:"deptId"`
	OrgID     int64  `db:"orgId"`
	SortOrder int    `db:"sortOrder"`
	Status    int    `db:"status"`
	Remark    string `db:"remark"`
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
