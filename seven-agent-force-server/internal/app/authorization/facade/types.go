package facade

import (
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

const AuthorizationRootSystemKey = "AUTHORIZATION_ROOT"

type RequestScope struct {
	UserID    int64
	Username  string
	IPAddress string
	UserAgent string
	DeviceID  string
	TenantID  string
	SessionID string
	Internal  bool
	Source    string
}

type UserVO struct {
	UserID        int64    `json:"userId"`
	Username      string   `json:"username"`
	Nickname      string   `json:"nickname,omitempty"`
	Avatar        string   `json:"avatar,omitempty"`
	Email         string   `json:"email,omitempty"`
	Phone         string   `json:"phone,omitempty"`
	IsAdmin       bool     `json:"isAdmin"`
	RoleCodes     []string `json:"roleCodes,omitempty"`
	RoleNames     []string `json:"roleNames,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	OrgIDs        []int64  `json:"orgIds,omitempty"`
	OrgCodes      []string `json:"orgCodes,omitempty"`
	OrgNames      []string `json:"orgNames,omitempty"`
	DeptIDs       []int64  `json:"deptIds,omitempty"`
	DeptCodes     []string `json:"deptCodes,omitempty"`
	DeptNames     []string `json:"deptNames,omitempty"`
	PostIDs       []int64  `json:"postIds,omitempty"`
	PostCodes     []string `json:"postCodes,omitempty"`
	PostNames     []string `json:"postNames,omitempty"`
	PrimaryOrgID  int64    `json:"primaryOrgId,omitempty"`
	PrimaryDeptID int64    `json:"primaryDeptId,omitempty"`
	PrimaryPostID int64    `json:"primaryPostId,omitempty"`
}

type CurrentUserResponse struct {
	ID            int64           `json:"id"`
	Username      string          `json:"username"`
	Nickname      string          `json:"nickname,omitempty"`
	UserAvatar    string          `json:"userAvatar,omitempty"`
	UserRole      []string        `json:"userRole,omitempty"`
	UserPosition  []string        `json:"userPosition,omitempty"`
	Organizations []string        `json:"organizations,omitempty"`
	Departments   []string        `json:"departments,omitempty"`
	Permissions   []string        `json:"permissions,omitempty"`
	RoleCodes     []string        `json:"roleCodes,omitempty"`
	PostCodes     []string        `json:"postCodes,omitempty"`
	OrgCodes      []string        `json:"orgCodes,omitempty"`
	DeptCodes     []string        `json:"deptCodes,omitempty"`
	IsAdmin       bool            `json:"isAdmin"`
	PrimaryOrgID  int64           `json:"primaryOrgId,omitempty"`
	AuthVersion   int64           `json:"authVersion"`
	DataScope     UserDataScopeVO `json:"dataScope"`
}

type UserDataScopeVO struct {
	UserID    int64   `json:"userId"`
	DeptIDs   []int64 `json:"deptIds"`
	OrgIDs    []int64 `json:"orgIds"`
	ScopeType string  `json:"scopeType"`
}

type StepUpChallengeRequest struct {
	BusinessAction         string   `json:"businessAction"`
	FlowNonce              string   `json:"flowNonce,omitempty"`
	RequestedTTLSeconds    int      `json:"requestedTimeToLiveSeconds,omitempty"`
	OperationBinding       string   `json:"operationBinding,omitempty"`
	ExpectedChallengeTypes []string `json:"expectedChallengeTypes,omitempty"`
	// TargetEmail is server-supplied for PROFILE_EMAIL_UPDATE. It is used only
	// by the VERIFY_NEW challenge step and is bound by OperationBinding.
	TargetEmail string `json:"targetEmail,omitempty"`
}

type StepUpVerifyRequest struct {
	ProofToken       string `json:"proofToken"`
	BusinessAction   string `json:"businessAction"`
	FlowNonce        string `json:"flowNonce"`
	OperationBinding string `json:"operationBinding,omitempty"`
	ConsumeOnce      bool   `json:"consumeOnce"`
}

type StepUpValidateRequest struct {
	ProofToken       string `json:"proofToken"`
	BusinessAction   string `json:"businessAction"`
	FlowNonce        string `json:"flowNonce"`
	OperationBinding string `json:"operationBinding,omitempty"`
	ConsumeOnce      bool   `json:"consumeOnce"`
}

type StepUpChallengeVO struct {
	ChallengeIdentifier        string            `json:"challengeIdentifier"`
	ChallengeState             string            `json:"challengeState"`
	EffectiveTimeToLiveSeconds int               `json:"effectiveTimeToLiveSeconds"`
	RequiredAssuranceLevel     string            `json:"requiredAssuranceLevel,omitempty"`
	ResolvedAssuranceLevel     string            `json:"resolvedAssuranceLevel,omitempty"`
	RecommendedStepIdentifier  string            `json:"recommendedStepIdentifier,omitempty"`
	ActualChallengeTypeNames   []string          `json:"actualChallengeTypeNames,omitempty"`
	Steps                      []ChallengeStepVO `json:"steps,omitempty"`
}

type StepUpTokenVO struct {
	ProofToken                string     `json:"proofToken"`
	ChallengeID               string     `json:"challengeIdentifier,omitempty"`
	TokenUniqueIdentifier     string     `json:"tokenUniqueIdentifier,omitempty"`
	BusinessAction            string     `json:"businessAction,omitempty"`
	FlowNonce                 string     `json:"flowNonce,omitempty"`
	OperationBinding          string     `json:"operationBinding,omitempty"`
	AuthenticationMethodNames []string   `json:"authenticationMethodNames,omitempty"`
	IssuedAt                  *time.Time `json:"issuedAt,omitempty"`
	ExpiresAt                 *time.Time `json:"expiresAt,omitempty"`
}

type ChallengeStepVO struct {
	StepIdentifier        string         `json:"stepIdentifier"`
	ChallengeType         string         `json:"challengeType"`
	StepPurpose           string         `json:"stepPurpose,omitempty"`
	StepState             string         `json:"stepState"`
	RemainingAttemptCount int            `json:"remainingAttemptCount"`
	CooldownSeconds       int            `json:"cooldownSeconds"`
	Switchable            bool           `json:"switchable"`
	UserInterfaceHints    map[string]any `json:"userInterfaceHints,omitempty"`
}

type RoleVO struct {
	ID                int64      `json:"id"`
	RoleID            int64      `json:"roleId"`
	Name              string     `json:"name"`
	Code              string     `json:"code"`
	Type              string     `json:"type,omitempty"`
	Status            int        `json:"status"`
	DataScope         int        `json:"dataScope"`
	SortOrder         int        `json:"sortOrder"`
	Remark            string     `json:"remark,omitempty"`
	CreateTime        *time.Time `json:"createTime,omitempty"`
	UpdateTime        *time.Time `json:"updateTime,omitempty"`
	SystemManaged     bool       `json:"systemManaged"`
	AuthorizationRoot bool       `json:"authorizationRoot"`
	GrantRevision     int64      `json:"grantRevision"`
}

type RoleSecurityStatusVO struct {
	RootRoleID         int64    `json:"rootRoleId"`
	RootRoleCode       string   `json:"rootRoleCode"`
	ActiveDirectAdmins int      `json:"activeDirectAdmins"`
	MinimumRequired    int      `json:"minimumRequired"`
	RecommendedMinimum int      `json:"recommendedMinimum"`
	Health             string   `json:"health"`
	Warnings           []string `json:"warnings"`
}

type BootstrapAuthorizationRootCommand struct {
	Code          string
	Name          string
	InitializedAt time.Time
}

type BootstrapAuthorizationRootResult struct {
	Role               RoleVO
	AlreadyInitialized bool
}

type MenuTreeNodeVO struct {
	ID          int64            `json:"id"`
	MenuID      int64            `json:"menuId"`
	ParentID    int64            `json:"parentId"`
	SortOrder   int              `json:"sortOrder"`
	Name        string           `json:"name"`
	Path        string           `json:"path,omitempty"`
	Component   string           `json:"component,omitempty"`
	Type        string           `json:"type,omitempty"`
	Permission  string           `json:"permission,omitempty"`
	FeatureCode string           `json:"featureCode,omitempty"`
	Icon        string           `json:"icon,omitempty"`
	Status      int              `json:"status"`
	Visible     int              `json:"visible"`
	IsFrame     int              `json:"isFrame"`
	IsCache     int              `json:"isCache"`
	Remark      string           `json:"remark,omitempty"`
	Checked     bool             `json:"checked"`
	Children    []MenuTreeNodeVO `json:"children,omitempty"`
	CreateTime  *time.Time       `json:"createTime,omitempty"`
	UpdateTime  *time.Time       `json:"updateTime,omitempty"`
}

type RolePageQuery struct {
	Current int64
	Size    int64
	Code    string
	Name    string
	Status  *int
}

type RolePageVO struct {
	Records []RoleVO `json:"records"`
	Total   int64    `json:"total"`
	Size    int64    `json:"size"`
	Current int64    `json:"current"`
}

type PermissionPageQuery struct {
	Current      int64
	Size         int64
	Code         string
	Name         string
	ResourceType string
	Method       string
	Path         string
	Status       *int
}

type PermissionPageVO struct {
	Records []PermissionVO `json:"records"`
	Total   int64          `json:"total"`
	Size    int64          `json:"size"`
	Current int64          `json:"current"`
}

type RoleCommand struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Code       string `json:"code"`
	Type       string `json:"type"`
	Status     *int   `json:"status"`
	DataScope  *int   `json:"dataScope"`
	SortOrder  *int   `json:"sortOrder"`
	Sort       *int   `json:"sort"`
	Remark     string `json:"remark"`
	OperatorID int64  `json:"operatorId,omitempty"`
}

type MenuCommand struct {
	ID          int64  `json:"id"`
	ParentID    int64  `json:"parentId"`
	Name        string `json:"name"`
	SortOrder   *int   `json:"sortOrder"`
	Path        string `json:"path"`
	Component   string `json:"component"`
	Type        string `json:"type"`
	Status      *int   `json:"status"`
	Permission  string `json:"permission"`
	FeatureCode string `json:"featureCode"`
	Icon        string `json:"icon"`
	IsFrame     *int   `json:"isFrame"`
	IsCache     *int   `json:"isCache"`
	Visible     *int   `json:"visible"`
	Remark      string `json:"remark"`
	OperatorID  int64  `json:"operatorId,omitempty"`
}

type PermissionVO struct {
	ID           int64      `json:"id"`
	Code         string     `json:"code"`
	FeatureCode  string     `json:"featureCode,omitempty"`
	Name         string     `json:"name"`
	ResourceType string     `json:"resourceType"`
	Method       string     `json:"method,omitempty"`
	Path         string     `json:"path,omitempty"`
	Status       int        `json:"status"`
	Description  string     `json:"description,omitempty"`
	CreateTime   *time.Time `json:"createTime,omitempty"`
	UpdateTime   *time.Time `json:"updateTime,omitempty"`
}

type PermissionQuery struct {
	Code         string
	Name         string
	ResourceType string
	Method       string
	Path         string
	Status       *int
}

type PermissionCommand struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	FeatureCode  string `json:"featureCode"`
	Name         string `json:"name"`
	ResourceType string `json:"resourceType"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	Status       *int   `json:"status"`
	Description  string `json:"description"`
	OperatorID   int64  `json:"operatorId,omitempty"`
}

type MenuPermissionAssignCommand struct {
	MenuID        int64   `json:"menuId"`
	PermissionIDs []int64 `json:"permissionIds,omitempty"`
	OperatorID    int64   `json:"operatorId,omitempty"`
	StepUpProof   stepup.ProofMetadata
}

type AssignRolePermissionsCommand struct {
	RoleID        int64   `json:"roleId"`
	PermissionIDs []int64 `json:"permissionIds,omitempty"`
	MenuIDs       []int64 `json:"menuIds,omitempty"`
	OperatorID    int64   `json:"operatorId,omitempty"`
	StepUpProof   stepup.ProofMetadata
}

type AssignRoleMenusCommand struct {
	RoleID      int64   `json:"roleId"`
	MenuIDs     []int64 `json:"menuIds,omitempty"`
	OperatorID  int64   `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata
}

type AssignRoleDeptsCommand struct {
	RoleID      int64   `json:"roleId"`
	DeptIDs     []int64 `json:"deptIds,omitempty"`
	OperatorID  int64   `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata
}

type RoleDeptIDsVO struct {
	RoleID  int64   `json:"roleId"`
	DeptIDs []int64 `json:"deptIds"`
}

// RoleConfigScopeGrantVO is the authorization-owned contract for one role config scope grant.
type RoleConfigScopeGrantVO struct {
	GroupCode string `json:"groupCode"`
	ConfigKey string `json:"configKey,omitempty"`
	CanRead   int    `json:"canRead"`
	CanWrite  int    `json:"canWrite"`
	CanDelete int    `json:"canDelete"`
}

// RoleGrantSnapshotVO is the editable, revisioned role grant state.
type RoleGrantSnapshotVO struct {
	Role              RoleVO                   `json:"role"`
	Revision          int64                    `json:"revision"`
	MenuIDs           []int64                  `json:"menuIds"`
	PermissionIDs     []int64                  `json:"permissionIds"`
	DataScope         int                      `json:"dataScope"`
	DeptIDs           []int64                  `json:"deptIds"`
	ConfigScopes      []RoleConfigScopeGrantVO `json:"configScopes"`
	ImpactedUserCount int                      `json:"impactedUserCount"`
}

// RoleGrantBundleRequest is the complete desired role grant state supplied by the API.
type RoleGrantBundleRequest struct {
	ExpectedRevision int64                    `json:"expectedRevision"`
	MenuIDs          []int64                  `json:"menuIds"`
	PermissionIDs    []int64                  `json:"permissionIds"`
	DataScope        int                      `json:"dataScope"`
	DeptIDs          []int64                  `json:"deptIds"`
	ConfigScopes     []RoleConfigScopeGrantVO `json:"configScopes"`
	Reason           string                   `json:"reason"`
	IdempotencyKey   string                   `json:"idempotencyKey"`
}

// PreviewRoleGrantBundleCommand adds server-owned actor identity to a preview request.
type PreviewRoleGrantBundleCommand struct {
	RoleID     int64
	OperatorID int64
	RoleGrantBundleRequest
}

// CommitRoleGrantBundleCommand adds server-owned actor identity and proof to a commit request.
type CommitRoleGrantBundleCommand struct {
	RoleID      int64
	OperatorID  int64
	StepUpProof stepup.ProofMetadata
	RoleGrantBundleRequest
}

// RoleGrantChangeSetVO contains stable identifiers for the proposed additions and removals.
type RoleGrantChangeSetVO struct {
	AddedMenuIDs         []int64                  `json:"addedMenuIds"`
	RemovedMenuIDs       []int64                  `json:"removedMenuIds"`
	AddedPermissionIDs   []int64                  `json:"addedPermissionIds"`
	RemovedPermissionIDs []int64                  `json:"removedPermissionIds"`
	AddedDeptIDs         []int64                  `json:"addedDeptIds"`
	RemovedDeptIDs       []int64                  `json:"removedDeptIds"`
	AddedConfigScopes    []RoleConfigScopeGrantVO `json:"addedConfigScopes"`
	RemovedConfigScopes  []RoleConfigScopeGrantVO `json:"removedConfigScopes"`
	DataScopeFrom        int                      `json:"dataScopeFrom"`
	DataScopeTo          int                      `json:"dataScopeTo"`
}

// RoleGrantPreviewVO is a non-authoritative preview calculated at one revision.
type RoleGrantPreviewVO struct {
	RoleID            int64                `json:"roleId"`
	Revision          int64                `json:"revision"`
	Changed           bool                 `json:"changed"`
	ImpactedUserCount int                  `json:"impactedUserCount"`
	Changes           RoleGrantChangeSetVO `json:"changes"`
}

// RoleGrantCommitVO reports an atomic role grant commit or idempotent replay.
type RoleGrantCommitVO struct {
	RoleID            int64 `json:"roleId"`
	Revision          int64 `json:"revision"`
	Changed           bool  `json:"changed"`
	ImpactedUserCount int   `json:"impactedUserCount"`
	IdempotentReplay  bool  `json:"idempotentReplay"`
}

type AssignUserRolesCommand struct {
	UserID      int64   `json:"userId"`
	RoleIDs     []int64 `json:"roleIds,omitempty"`
	OperatorID  int64   `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata
}

type AssignCreatedUserRolesCommand struct {
	UserID      int64   `json:"userId"`
	Username    string  `json:"username"`
	RoleIDs     []int64 `json:"roleIds,omitempty"`
	OperatorID  int64   `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata
}

type AssignProvisionedUserRolesCommand struct {
	UserID  int64   `json:"userId"`
	RoleIDs []int64 `json:"roleIds,omitempty"`
}

type BootstrapOwnerRolesCommand struct {
	UserID     int64   `json:"userId"`
	RoleIDs    []int64 `json:"roleIds,omitempty"`
	OperatorID int64   `json:"operatorId,omitempty"`
}

type TemporaryPermissionGrantCommand struct {
	UserID         int64      `json:"userId"`
	PermissionCode string     `json:"permissionCode"`
	ExpireAt       *time.Time `json:"expireAt,omitempty"`
	Source         string     `json:"source,omitempty"`
	Reason         string     `json:"reason"`
	GrantedBy      int64      `json:"grantedBy,omitempty"`
	StepUpProof    stepup.ProofMetadata
}

type TemporaryPermissionUpdateCommand struct {
	UserID         int64      `json:"userId"`
	PermissionCode string     `json:"permissionCode"`
	ExpireAt       *time.Time `json:"expireAt,omitempty"`
	OperatorID     int64      `json:"operatorId,omitempty"`
	Reason         string     `json:"reason"`
	StepUpProof    stepup.ProofMetadata
}

type TemporaryPermissionVO struct {
	UserID         int64      `json:"userId"`
	PermissionCode string     `json:"permissionCode"`
	PermissionName string     `json:"permissionName,omitempty"`
	Type           int        `json:"type"`
	ExpireAt       *time.Time `json:"expireAt,omitempty"`
	Source         string     `json:"source,omitempty"`
	Reason         string     `json:"reason,omitempty"`
	GrantedBy      int64      `json:"grantedBy,omitempty"`
	GrantedAt      *time.Time `json:"grantedAt,omitempty"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
	Status         string     `json:"status"`
}

type TemporaryPermissionStatsVO struct {
	TotalActive  int64 `json:"totalActive"`
	Temporary    int64 `json:"temporary"`
	Permanent    int64 `json:"permanent"`
	ExpiringSoon int64 `json:"expiringSoon"`
}
