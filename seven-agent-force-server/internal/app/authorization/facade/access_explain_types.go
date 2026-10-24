package facade

import "time"

const (
	AccessDecisionAllow = "ALLOW"
	AccessDecisionDeny  = "DENY"
)

type EffectiveAccessQuery struct {
	Current    int64
	Size       int64
	Keyword    string
	SourceType string
	Effective  *bool
}

type AccessPostSourceVO struct {
	PostID   int64  `json:"postId"`
	PostCode string `json:"postCode"`
	PostName string `json:"postName"`
	DeptID   int64  `json:"deptId"`
	OrgID    int64  `json:"orgId"`
}

type AccessRoleSourceVO struct {
	RoleID               int64               `json:"roleId"`
	RoleCode             string              `json:"roleCode"`
	RoleName             string              `json:"roleName"`
	RoleStatus           int                 `json:"roleStatus"`
	DeclaredDataScope    string              `json:"declaredDataScope"`
	RoleAssignmentSource string              `json:"roleAssignmentSource"`
	Post                 *AccessPostSourceVO `json:"post,omitempty"`
}

type DataScopeContributorVO struct {
	RoleID            int64   `json:"roleId"`
	RoleCode          string  `json:"roleCode"`
	DeclaredScopeType string  `json:"declaredScopeType"`
	Winning           bool    `json:"winning"`
	DeptIDs           []int64 `json:"deptIds"`
}

type EffectiveDataScopeVO struct {
	UserID       int64                    `json:"userId"`
	ScopeType    string                   `json:"scopeType"`
	DeptIDs      []int64                  `json:"deptIds"`
	OrgIDs       []int64                  `json:"orgIds"`
	Contributors []DataScopeContributorVO `json:"contributors"`
}

type PermissionGrantChainVO struct {
	PermissionGrantSource string              `json:"permissionGrantSource"`
	RoleID                int64               `json:"roleId,omitempty"`
	RoleCode              string              `json:"roleCode,omitempty"`
	RoleName              string              `json:"roleName,omitempty"`
	RoleAssignmentSource  string              `json:"roleAssignmentSource,omitempty"`
	Post                  *AccessPostSourceVO `json:"post,omitempty"`
	MenuID                int64               `json:"menuId,omitempty"`
	MenuName              string              `json:"menuName,omitempty"`
	MenuPath              string              `json:"menuPath,omitempty"`
	GrantedBy             int64               `json:"grantedBy,omitempty"`
	Source                string              `json:"source,omitempty"`
	ExpireAt              *time.Time          `json:"expireAt,omitempty"`
	Active                bool                `json:"active"`
	ReasonCode            string              `json:"reasonCode"`
}

type EffectivePermissionVO struct {
	PermissionID   int64                    `json:"permissionId"`
	PermissionCode string                   `json:"permissionCode"`
	PermissionName string                   `json:"permissionName"`
	Effective      bool                     `json:"effective"`
	FeatureCode    string                   `json:"featureCode,omitempty"`
	FeatureEnabled bool                     `json:"featureEnabled"`
	Grants         []PermissionGrantChainVO `json:"grants"`
}

type EffectivePermissionPageVO struct {
	Current int64                   `json:"current"`
	Size    int64                   `json:"size"`
	Total   int64                   `json:"total"`
	Records []EffectivePermissionVO `json:"records"`
}

type PermissionSummaryVO struct {
	EffectiveCount int64 `json:"effectiveCount"`
	FilteredCount  int64 `json:"filteredCount"`
	TemporaryCount int64 `json:"temporaryCount"`
}

type EffectiveAccessVO struct {
	UserID            int64                     `json:"userId"`
	Username          string                    `json:"username"`
	Status            int                       `json:"status"`
	AuthorizationRoot bool                      `json:"authorizationRoot"`
	RoleSources       []AccessRoleSourceVO      `json:"roleSources"`
	DataScope         EffectiveDataScopeVO      `json:"dataScope"`
	PermissionSummary PermissionSummaryVO       `json:"permissionSummary"`
	Permissions       EffectivePermissionPageVO `json:"permissions"`
}

type PermissionFeatureVO struct {
	Code    string `json:"code"`
	Enabled bool   `json:"enabled"`
}

type PermissionExplainVO struct {
	UserID                 int64                    `json:"userId"`
	PermissionCode         string                   `json:"permissionCode"`
	Decision               string                   `json:"decision"`
	ReasonCode             string                   `json:"reasonCode"`
	MatchedPermissionCodes []string                 `json:"matchedPermissionCodes"`
	Chains                 []PermissionGrantChainVO `json:"chains"`
	Feature                *PermissionFeatureVO     `json:"feature,omitempty"`
	EvaluatedAt            time.Time                `json:"evaluatedAt"`
}
