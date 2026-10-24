package domain

import "time"

const (
	StatusActive   = 0
	StatusDisabled = 1

	MethodPassword      = "PASSWORD"
	MethodPasskey       = "PASSKEY"
	MethodExternalOAuth = "EXTERNAL_OAUTH"

	MatchClientID       = "CLIENT_ID"
	MatchRedirectHost   = "REDIRECT_HOST"
	MatchRedirectPrefix = "REDIRECT_PREFIX"
	MatchHost           = "HOST"
	MatchOrigin         = "ORIGIN"
	MatchRefererHost    = "REFERER_HOST"

	// MatchExplicitCode is intentionally not accepted by source matching.
	// Public platformCode input is a UX hint only and must never grant
	// provisioning authority or override trusted source resolution.
	MatchExplicitCode = "EXPLICIT_CODE"

	AuthorityPresentation = "PRESENTATION"
	AuthorityProvisioning = "PROVISIONING"
)

type Platform struct {
	ID                 int64
	PlatformCode       string
	PlatformName       string
	PlatformType       string
	Description        string
	DefaultRedirectURL string
	AllowAutoRegister  bool
	AllowFormRegister  bool
	IsDefault          bool
	DefaultDeptID      *int64
	BrandJSON          string
	SettingsJSON       string
	Status             int
	CreateTime         time.Time
	UpdateTime         time.Time
}

type SSOClientBinding struct {
	ID           int64
	PlatformCode string
	ClientID     string
	Status       int
}

type LoginMethod struct {
	ID             int64
	PlatformCode   string
	MethodType     string
	ProviderCode   string
	DisplayName    string
	Icon           string
	SortOrder      int
	DisplayEnabled bool
	LoginEnabled   bool
	MetadataJSON   string
}

type SourceRule struct {
	ID           int64
	PlatformCode string
	MatchType    string
	MatchValue   string
	Priority     int
	Status       int
	MetadataJSON string
}

type DefaultRole struct {
	ID                int64
	PlatformCode      string
	RoleID            int64
	AutoAssignEnabled bool
	Status            int
}

type RequestSource struct {
	ClientID           string
	LoginTransactionID string
	RedirectURL        string
	Host               string
	Origin             string
	Referer            string
	ExplicitCodeHint   string
}

type LoginContext struct {
	ID                 string
	PlatformCode       string
	Authority          string
	ClientID           string
	LoginTransactionID string
	RedirectURL        string
	SourceFingerprint  string
	MethodListVersion  string
	ExpiresAt          time.Time
}
