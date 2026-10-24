package facade

import "time"

const (
	UserStatusNormal        = 0
	UserStatusDisabled      = 1
	UserStatusPendingReview = 2
)

type SubjectRecord struct {
	UserID      int64      `json:"userId"`
	AccountName string     `json:"accountName"`
	Email       string     `json:"email,omitempty"`
	Phone       string     `json:"phone,omitempty"`
	Status      int        `json:"status"`
	Enabled     bool       `json:"enabled"`
	LockStatus  bool       `json:"lockStatus"`
	UnsealAt    *time.Time `json:"unsealAt,omitempty"`
}

type UserProfile struct {
	UserID            int64      `json:"userId"`
	AccountName       string     `json:"accountName"`
	NickName          string     `json:"nickName"`
	Email             string     `json:"email"`
	Phone             string     `json:"phone,omitempty"`
	Avatar            string     `json:"avatar,omitempty"`
	Profile           string     `json:"profile,omitempty"`
	Enabled           bool       `json:"enabled"`
	PasswordChangedAt *time.Time `json:"passwordChangedAt,omitempty"`
	EmailVerified     bool       `json:"emailVerified"`
	EmailVerifiedAt   *time.Time `json:"emailVerifiedAt,omitempty"`
	EmailVersion      int64      `json:"emailVersion,string"`
}

type UserPrincipalSeed struct {
	UserID      int64  `json:"userId"`
	AccountName string `json:"accountName"`
	NickName    string `json:"nickName"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type UpdateSelfProfileCommand struct {
	UserID      int64   `json:"userId"`
	NickName    *string `json:"nickName,omitempty"`
	UserPhone   *string `json:"userPhone,omitempty"`
	UserAvatar  *string `json:"userAvatar,omitempty"`
	UserProfile *string `json:"userProfile,omitempty"`
}

type UpdateSelfEmailCommand struct {
	UserID               int64  `json:"userId"`
	UserEmail            string `json:"userEmail"`
	ExpectedEmailVersion int64  `json:"expectedEmailVersion,string"`
}

type SyncExternalProfileCommand struct {
	UserID        int64  `json:"userId"`
	ProviderCode  string `json:"providerCode"`
	ExternalLogin string `json:"externalLogin,omitempty"`
	NickName      string `json:"nickName,omitempty"`
	UserEmail     string `json:"userEmail,omitempty"`
	EmailVerified bool   `json:"emailVerified"`
	UserAvatar    string `json:"userAvatar,omitempty"`
	RawProfile    string `json:"rawProfile,omitempty"`
}

type UpdatePasswordCommand struct {
	UserID      int64  `json:"userId"`
	RawPassword string `json:"rawPassword"`
	OperatorID  int64  `json:"operatorId"`
}

type CreateExternalSubjectCommand struct {
	AccountName          string  `json:"accountName"`
	NickName             string  `json:"nickName"`
	UserEmail            string  `json:"userEmail"`
	UserAvatar           string  `json:"userAvatar,omitempty"`
	RegisterPlatformCode string  `json:"registerPlatformCode,omitempty"`
	RegisterProviderCode string  `json:"registerProviderCode,omitempty"`
	DefaultOrgID         *int64  `json:"defaultOrgId,omitempty"`
	DefaultDeptID        *int64  `json:"defaultDeptId,omitempty"`
	DefaultPostIDs       []int64 `json:"defaultPostIds,omitempty"`
	DefaultRoleIDs       []int64 `json:"defaultRoleIds,omitempty"`
	DisableEmailMerge    bool    `json:"-"`
}

type CreateFormSubjectCommand struct {
	AccountName          string  `json:"accountName"`
	NickName             string  `json:"nickName"`
	UserEmail            string  `json:"userEmail"`
	RawPassword          string  `json:"rawPassword"`
	RegisterPlatformCode string  `json:"registerPlatformCode,omitempty"`
	DefaultOrgID         *int64  `json:"defaultOrgId,omitempty"`
	DefaultDeptID        *int64  `json:"defaultDeptId,omitempty"`
	DefaultPostIDs       []int64 `json:"defaultPostIds,omitempty"`
	DefaultRoleIDs       []int64 `json:"defaultRoleIds,omitempty"`
}

type UpdateLockStateCommand struct {
	UserID     int64      `json:"userId"`
	Status     int        `json:"status"`
	UnsealTime *time.Time `json:"unsealTime,omitempty"`
}

type CreateOwnerUserCommand struct {
	AccountName string `json:"accountName"`
	NickName    string `json:"nickName"`
	RawPassword string `json:"rawPassword"`
}

type ProvisionedUser struct {
	UserID      int64  `json:"userId"`
	AccountName string `json:"accountName"`
	NickName    string `json:"nickName"`
	Avatar      string `json:"avatar"`
}
