package facade

import "time"

const (
	AuthorityPresentation = "PRESENTATION"
	AuthorityProvisioning = "PROVISIONING"
)

type ResolvePlatformRequest struct {
	ClientID           string        `json:"clientId,omitempty" query:"clientId"`
	LoginTransactionID string        `json:"loginTransactionId,omitempty" query:"loginTransactionId"`
	LoginContextID     string        `json:"loginContextId,omitempty" query:"loginContextId"`
	RedirectURL        string        `json:"redirectUrl,omitempty" query:"redirectUrl"`
	TrustedSource      TrustedSource `json:"-" query:"-" form:"-"`
	// ExplicitCode is a public UX hint only. Implementations must ignore it
	// unless it matches a trusted platform resolution.
	ExplicitCode string `json:"platformCode,omitempty" query:"platformCode"`
}

type TrustedSource struct {
	ClientID    string
	RedirectURL string
	Host        string
	Origin      string
	Referer     string
}

type LoginOptionResult struct {
	LoginContextID string              `json:"loginContextId"`
	PlatformCode   string              `json:"platformCode"`
	PlatformName   string              `json:"platformName"`
	Brand          PlatformBrand       `json:"brand"`
	Registration   RegistrationOptions `json:"registration"`
	Methods        []LoginMethodRecord `json:"methods"`
}

type RegistrationOptions struct {
	FormRegisterEnabled bool     `json:"formRegisterEnabled"`
	RequireCaptcha      bool     `json:"requireCaptcha"`
	RequiredFields      []string `json:"requiredFields"`
}

type PlatformBrand struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Theme    string `json:"theme,omitempty"`
}

type LoginMethodRecord struct {
	MethodType   string `json:"methodType"`
	ProviderCode string `json:"providerCode"`
	DisplayName  string `json:"displayName"`
	Icon         string `json:"icon,omitempty"`
	SortOrder    int    `json:"sortOrder"`
	LoginURL     string `json:"loginUrl,omitempty"`
}

type PlatformDetail struct {
	ID                 int64               `json:"id"`
	PlatformCode       string              `json:"platformCode"`
	PlatformName       string              `json:"platformName"`
	PlatformType       string              `json:"platformType"`
	Description        string              `json:"description,omitempty"`
	DefaultRedirectURL string              `json:"defaultRedirectUrl,omitempty"`
	AllowAutoRegister  bool                `json:"allowAutoRegister"`
	AllowFormRegister  bool                `json:"allowFormRegister"`
	IsDefault          bool                `json:"isDefault"`
	DefaultDeptID      *int64              `json:"defaultDeptId,omitempty"`
	BrandJSON          string              `json:"brandJson,omitempty"`
	SettingsJSON       string              `json:"settingsJson,omitempty"`
	Status             int                 `json:"status"`
	LoginMethods       []LoginMethodDetail `json:"loginMethods,omitempty"`
	SourceRules        []SourceRuleRecord  `json:"sourceRules,omitempty"`
	DefaultRoles       []DefaultRoleRecord `json:"defaultRoles,omitempty"`
	CreateTime         time.Time           `json:"createTime"`
	UpdateTime         time.Time           `json:"updateTime"`
}

type LoginMethodDetail struct {
	ID             int64  `json:"id,omitempty"`
	MethodType     string `json:"methodType"`
	ProviderCode   string `json:"providerCode"`
	DisplayName    string `json:"displayName"`
	Icon           string `json:"icon,omitempty"`
	SortOrder      int    `json:"sortOrder"`
	DisplayEnabled bool   `json:"displayEnabled"`
	LoginEnabled   bool   `json:"loginEnabled"`
	MetadataJSON   string `json:"metadataJson,omitempty"`
}

type SourceRuleRecord struct {
	ID           int64  `json:"id,omitempty"`
	MatchType    string `json:"matchType"`
	MatchValue   string `json:"matchValue"`
	Priority     int    `json:"priority"`
	Status       int    `json:"status"`
	MetadataJSON string `json:"metadataJson,omitempty"`
}

type PlatformQuery struct {
	Keyword      string `json:"keyword,omitempty" query:"keyword"`
	PlatformCode string `json:"platformCode,omitempty" query:"platformCode"`
	PlatformType string `json:"platformType,omitempty" query:"platformType"`
	Status       *int   `json:"status,omitempty" query:"status"`
	Current      int    `json:"current,omitempty" query:"current"`
	PageSize     int    `json:"pageSize,omitempty" query:"pageSize"`
}

type PlatformPage struct {
	Records  []PlatformDetail `json:"records"`
	Total    int64            `json:"total"`
	Current  int              `json:"current"`
	PageSize int              `json:"pageSize"`
}

type PlatformSaveRequest struct {
	PlatformCode       string `json:"platformCode"`
	PlatformName       string `json:"platformName"`
	PlatformType       string `json:"platformType"`
	Description        string `json:"description,omitempty"`
	DefaultRedirectURL string `json:"defaultRedirectUrl,omitempty"`
	AllowAutoRegister  bool   `json:"allowAutoRegister"`
	AllowFormRegister  bool   `json:"allowFormRegister"`
	IsDefault          bool   `json:"isDefault"`
	DefaultDeptID      *int64 `json:"defaultDeptId,omitempty"`
	BrandJSON          string `json:"brandJson,omitempty"`
	SettingsJSON       string `json:"settingsJson,omitempty"`
	Status             *int   `json:"status,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type PlatformStatusRequest struct {
	Status int    `json:"status"`
	Reason string `json:"reason"`
}

type LoginMethodSaveRequest struct {
	MethodType     string `json:"methodType"`
	ProviderCode   string `json:"providerCode"`
	DisplayName    string `json:"displayName"`
	Icon           string `json:"icon,omitempty"`
	SortOrder      int    `json:"sortOrder"`
	DisplayEnabled bool   `json:"displayEnabled"`
	LoginEnabled   bool   `json:"loginEnabled"`
	MetadataJSON   string `json:"metadataJson,omitempty"`
}

type SourceRuleSaveRequest struct {
	MatchType    string `json:"matchType"`
	MatchValue   string `json:"matchValue"`
	Priority     int    `json:"priority"`
	Status       int    `json:"status"`
	MetadataJSON string `json:"metadataJson,omitempty"`
}

type DefaultRoleSaveRequest struct {
	RoleID            int64 `json:"roleId"`
	AutoAssignEnabled bool  `json:"autoAssignEnabled"`
}

type DefaultRoleRecord struct {
	ID                int64 `json:"id,omitempty"`
	RoleID            int64 `json:"roleId"`
	AutoAssignEnabled bool  `json:"autoAssignEnabled"`
	Status            int   `json:"status"`
}

type LoginContextValidation struct {
	LoginContextID       string
	PlatformCode         string
	Authority            string
	SourceFingerprint    string
	ProvisioningEligible bool
}

type ProvisioningAuthority struct {
	AuthorityID    string
	LoginContextID string
	PlatformCode   string
	Authority      string
}

type ProvisioningPolicy struct {
	PlatformCode        string  `json:"platformCode"`
	AllowAutoRegister   bool    `json:"allowAutoRegister"`
	AllowFormRegister   bool    `json:"allowFormRegister"`
	DefaultOrgID        *int64  `json:"defaultOrgId,omitempty"`
	DefaultDeptID       *int64  `json:"defaultDeptId,omitempty"`
	DefaultPostIDs      []int64 `json:"defaultPostIds,omitempty"`
	DefaultRoleIDs      []int64 `json:"defaultRoleIds"`
	DefaultRoleMaxCount int     `json:"defaultRoleMaxCount,omitempty"`
}
