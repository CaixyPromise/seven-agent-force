package facade

import "time"

type RequestContext struct {
	DeviceID  string `json:"deviceId,omitempty"`
	TenantID  string `json:"tenantId,omitempty"`
	LoginIP   string `json:"loginIp,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
}

type ListLoginMethodsRequest struct {
	LoginTransactionID string          `json:"loginTransactionId,omitempty" query:"loginTransactionId"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
}

type LoginMethodRecord struct {
	ProviderCode string `json:"providerCode"`
	DisplayName  string `json:"displayName"`
	Icon         string `json:"icon,omitempty"`
	SortOrder    int    `json:"sortOrder"`
	LoginURL     string `json:"loginUrl"`
}

type StartExternalLoginRequest struct {
	ProviderCode       string          `json:"providerCode"`
	LoginTransactionID string          `json:"loginTransactionId,omitempty"`
	LoginContextID     string          `json:"loginContextId,omitempty"`
	PlatformCode       string          `json:"platformCode,omitempty"`
	RedirectAfterLogin string          `json:"redirectAfterLogin,omitempty"`
	BindUserID         int64           `json:"bindUserId,omitempty"`
	RequestContext     *RequestContext `json:"requestContext,omitempty"`
	TrustedSource      TrustedSource   `json:"-"`
}

type TrustedSource struct {
	ClientID    string
	RedirectURL string
	Host        string
	Origin      string
	Referer     string
}

type StartExternalLoginResult struct {
	RedirectURL string `json:"redirectUrl"`
	StateID     string `json:"stateId,omitempty"`
}

type CompleteExternalCallbackRequest struct {
	ProviderCode   string          `json:"providerCode"`
	Code           string          `json:"code"`
	State          string          `json:"state"`
	Issuer         string          `json:"iss,omitempty"`
	RequestContext *RequestContext `json:"requestContext,omitempty"`
}

type ExternalLoginResult struct {
	Authenticated            bool   `json:"authenticated"`
	Outcome                  string `json:"outcome,omitempty"`
	Purpose                  string `json:"purpose,omitempty"`
	LoginTransactionID       string `json:"loginTransactionId,omitempty"`
	UserID                   int64  `json:"userId,omitempty"`
	ExternalIdentityID       int64  `json:"externalIdentityId,omitempty"`
	ProviderCode             string `json:"providerCode,omitempty"`
	PlatformCode             string `json:"platformCode,omitempty"`
	RedirectURL              string `json:"redirectUrl,omitempty"`
	AccessToken              string `json:"accessToken,omitempty"`
	TokenType                string `json:"tokenType,omitempty"`
	AccessTTLSeconds         int64  `json:"accessTtlSec,omitempty"`
	SessionCookieHeaderValue string `json:"sessionCookieHeaderValue,omitempty"`
	RefreshCookieHeaderValue string `json:"refreshCookieHeaderValue,omitempty"`
	BindRequired             bool   `json:"bindRequired,omitempty"`
}

type ProviderCapabilityCatalog map[string]ProviderCapabilityDescriptor

type ProviderCapabilityDescriptor struct {
	ProviderCode  string   `json:"providerCode"`
	DisplayName   string   `json:"displayName"`
	ProtocolType  string   `json:"protocolType"`
	Capabilities  []string `json:"capabilities"`
	DefaultScopes []string `json:"defaultScopes,omitempty"`
}

type ProviderQuery struct {
	Keyword      string `json:"keyword,omitempty"`
	ProviderCode string `json:"providerCode,omitempty"`
	ProtocolType string `json:"protocolType,omitempty"`
	Status       *int   `json:"status,omitempty"`
	Current      int    `json:"current,omitempty"`
	PageSize     int    `json:"pageSize,omitempty"`
}

type ProviderPage struct {
	Records  []ProviderDetail `json:"records"`
	Total    int64            `json:"total"`
	Current  int              `json:"current"`
	PageSize int              `json:"pageSize"`
}

type ProviderDetail struct {
	ID                       int64                      `json:"id"`
	ProviderCode             string                     `json:"providerCode"`
	ProviderName             string                     `json:"providerName"`
	ProtocolType             string                     `json:"protocolType"`
	Issuer                   string                     `json:"issuer,omitempty"`
	AuthorizationEndpoint    string                     `json:"authorizationEndpoint"`
	TokenEndpoint            string                     `json:"tokenEndpoint"`
	UserinfoEndpoint         string                     `json:"userinfoEndpoint,omitempty"`
	JWKSURI                  string                     `json:"jwksUri,omitempty"`
	ClientID                 string                     `json:"clientId"`
	Scopes                   []string                   `json:"scopes"`
	RedirectURI              string                     `json:"redirectUri"`
	DisplayName              string                     `json:"displayName"`
	Icon                     string                     `json:"icon,omitempty"`
	SortOrder                int                        `json:"sortOrder"`
	DisplayEnabled           bool                       `json:"displayEnabled"`
	LoginEnabled             bool                       `json:"loginEnabled"`
	BindEnabled              bool                       `json:"bindEnabled"`
	EmailAutoBindEnabled     bool                       `json:"emailAutoBindEnabled"`
	AccountAutoCreateEnabled bool                       `json:"accountAutoCreateEnabled"`
	Status                   int                        `json:"status"`
	MetadataJSON             string                     `json:"metadataJson,omitempty"`
	Methods                  []ProviderMethodDescriptor `json:"methods,omitempty"`
	CreateTime               time.Time                  `json:"createTime"`
	UpdateTime               time.Time                  `json:"updateTime"`
}

type ProviderSaveRequest struct {
	ProviderCode             string   `json:"providerCode"`
	ProviderName             string   `json:"providerName"`
	ProtocolType             string   `json:"protocolType"`
	Issuer                   string   `json:"issuer,omitempty"`
	AuthorizationEndpoint    string   `json:"authorizationEndpoint"`
	TokenEndpoint            string   `json:"tokenEndpoint"`
	UserinfoEndpoint         string   `json:"userinfoEndpoint,omitempty"`
	JWKSURI                  string   `json:"jwksUri,omitempty"`
	ClientID                 string   `json:"clientId"`
	ClientSecret             string   `json:"clientSecret,omitempty"`
	Scopes                   []string `json:"scopes"`
	RedirectURI              string   `json:"redirectUri"`
	DisplayName              string   `json:"displayName"`
	Icon                     string   `json:"icon,omitempty"`
	SortOrder                int      `json:"sortOrder"`
	DisplayEnabled           bool     `json:"displayEnabled"`
	LoginEnabled             bool     `json:"loginEnabled"`
	BindEnabled              bool     `json:"bindEnabled"`
	EmailAutoBindEnabled     bool     `json:"emailAutoBindEnabled"`
	AccountAutoCreateEnabled bool     `json:"accountAutoCreateEnabled"`
	MetadataJSON             string   `json:"metadataJson,omitempty"`
}

type ProviderUpdateRequest struct {
	ProviderName             string   `json:"providerName"`
	ProtocolType             string   `json:"protocolType"`
	Issuer                   string   `json:"issuer,omitempty"`
	AuthorizationEndpoint    string   `json:"authorizationEndpoint"`
	TokenEndpoint            string   `json:"tokenEndpoint"`
	UserinfoEndpoint         string   `json:"userinfoEndpoint,omitempty"`
	JWKSURI                  string   `json:"jwksUri,omitempty"`
	ClientID                 string   `json:"clientId"`
	Scopes                   []string `json:"scopes"`
	RedirectURI              string   `json:"redirectUri"`
	DisplayName              string   `json:"displayName"`
	Icon                     string   `json:"icon,omitempty"`
	SortOrder                int      `json:"sortOrder"`
	DisplayEnabled           bool     `json:"displayEnabled"`
	LoginEnabled             bool     `json:"loginEnabled"`
	BindEnabled              bool     `json:"bindEnabled"`
	EmailAutoBindEnabled     bool     `json:"emailAutoBindEnabled"`
	AccountAutoCreateEnabled bool     `json:"accountAutoCreateEnabled"`
	MetadataJSON             string   `json:"metadataJson,omitempty"`
}

type ProviderStatusRequest struct {
	Status               int    `json:"status"`
	Reason               string `json:"reason,omitempty"`
	RevokeActiveSessions *bool  `json:"revokeActiveSessions,omitempty"`
}

type RotateClientSecretRequest struct {
	ClientSecret string `json:"clientSecret"`
	Reason       string `json:"reason,omitempty"`
}

type IdentityQuery struct {
	ProviderCode string `json:"providerCode,omitempty"`
	UserID       *int64 `json:"userId,omitempty"`
	Status       *int   `json:"status,omitempty"`
	Keyword      string `json:"keyword,omitempty"`
	Current      int    `json:"current,omitempty"`
	PageSize     int    `json:"pageSize,omitempty"`
}

type IdentityPage struct {
	Records  []ExternalIdentityRecord `json:"records"`
	Total    int64                    `json:"total"`
	Current  int                      `json:"current"`
	PageSize int                      `json:"pageSize"`
}

type ExternalIdentityRecord struct {
	ID              int64      `json:"id"`
	ProviderCode    string     `json:"providerCode"`
	ExternalIssuer  string     `json:"externalIssuer,omitempty"`
	ExternalSubject string     `json:"externalSubject"`
	UserID          int64      `json:"userId"`
	ExternalLogin   string     `json:"externalLogin,omitempty"`
	ExternalEmail   string     `json:"externalEmail,omitempty"`
	EmailVerified   bool       `json:"emailVerified"`
	DisplayName     string     `json:"displayName,omitempty"`
	AvatarURL       string     `json:"avatarUrl,omitempty"`
	Status          int        `json:"status"`
	FirstLinkedAt   time.Time  `json:"firstLinkedAt"`
	LastLoginAt     *time.Time `json:"lastLoginAt,omitempty"`
	LastVerifiedAt  *time.Time `json:"lastVerifiedAt,omitempty"`
	CreateTime      time.Time  `json:"createTime"`
	UpdateTime      time.Time  `json:"updateTime"`
}

type ManagedOIDCProviderCommand struct {
	OwnerNodeCode     string
	ConnectionVersion string
	TargetRevision    int64
	Enabled           bool
	DisplayName       string
	Issuer            string
	ClientID          string
	ClientSecret      string
	RedirectURI       string
}

type CurrentUserBinding struct {
	ProviderCode   string     `json:"providerCode"`
	DisplayName    string     `json:"displayName"`
	Icon           string     `json:"icon,omitempty"`
	BindEnabled    bool       `json:"bindEnabled"`
	Bound          bool       `json:"bound"`
	IdentityID     int64      `json:"identityId,omitempty"`
	ExternalLogin  string     `json:"externalLogin,omitempty"`
	ExternalEmail  string     `json:"externalEmail,omitempty"`
	EmailVerified  bool       `json:"emailVerified"`
	AvatarURL      string     `json:"avatarUrl,omitempty"`
	Status         int        `json:"status,omitempty"`
	LastLoginAt    *time.Time `json:"lastLoginAt,omitempty"`
	LastVerifiedAt *time.Time `json:"lastVerifiedAt,omitempty"`
	BindURL        string     `json:"bindUrl,omitempty"`
	SortOrder      int        `json:"sortOrder"`
}

type IdentityStatusRequest struct {
	Status int    `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type TokenQuery struct {
	ProviderCode string `json:"providerCode,omitempty"`
	IdentityID   *int64 `json:"identityId,omitempty"`
	UserID       *int64 `json:"userId,omitempty"`
	TokenPurpose string `json:"tokenPurpose,omitempty"`
	Status       *int   `json:"status,omitempty"`
	Current      int    `json:"current,omitempty"`
	PageSize     int    `json:"pageSize,omitempty"`
}

type TokenPage struct {
	Records  []OAuthTokenRecord `json:"records"`
	Total    int64              `json:"total"`
	Current  int                `json:"current"`
	PageSize int                `json:"pageSize"`
}

type OAuthTokenRecord struct {
	ID               int64      `json:"id"`
	ProviderCode     string     `json:"providerCode"`
	IdentityID       int64      `json:"identityId"`
	UserID           int64      `json:"userId"`
	TokenPurpose     string     `json:"tokenPurpose"`
	Scopes           []string   `json:"scopes,omitempty"`
	ScopeHash        string     `json:"scopeHash"`
	AccessExpiresAt  *time.Time `json:"accessExpiresAt,omitempty"`
	RefreshExpiresAt *time.Time `json:"refreshExpiresAt,omitempty"`
	LastRefreshAt    *time.Time `json:"lastRefreshAt,omitempty"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty"`
	Status           int        `json:"status"`
	Version          int        `json:"version"`
	MetadataJSON     string     `json:"metadataJson,omitempty"`
	CreateTime       time.Time  `json:"createTime"`
	UpdateTime       time.Time  `json:"updateTime"`
}

type AcquireAccessTokenRequest struct {
	ProviderCode string   `json:"providerCode"`
	IdentityID   int64    `json:"identityId"`
	UserID       int64    `json:"userId"`
	TokenPurpose string   `json:"tokenPurpose"`
	Scopes       []string `json:"scopes,omitempty"`
}

type AccessTokenLease struct {
	TokenID      int64      `json:"tokenId"`
	ProviderCode string     `json:"providerCode"`
	IdentityID   int64      `json:"identityId"`
	UserID       int64      `json:"userId"`
	AccessToken  string     `json:"accessToken"`
	TokenType    string     `json:"tokenType,omitempty"`
	Scopes       []string   `json:"scopes,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
}

type ProviderMethodDescriptor struct {
	ID             int64    `json:"id,omitempty"`
	ProviderCode   string   `json:"providerCode"`
	MethodKey      string   `json:"methodKey"`
	CapabilityCode string   `json:"capabilityCode"`
	RequiredScopes []string `json:"requiredScopes,omitempty"`
	Status         int      `json:"status"`
	MetadataJSON   string   `json:"metadataJson,omitempty"`
}
