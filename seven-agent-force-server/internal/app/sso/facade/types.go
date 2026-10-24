package facade

import "time"

type RequestContext struct {
	DeviceID  string `json:"deviceId,omitempty"`
	TenantID  string `json:"tenantId,omitempty"`
	LoginIP   string `json:"loginIp,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
}

type AuthorizationSessionSnapshot struct {
	LoginTransactionID  string     `json:"loginTransactionId"`
	ClientID            string     `json:"clientId"`
	RedirectURI         string     `json:"redirectUri"`
	Scopes              []string   `json:"scopes,omitempty"`
	State               string     `json:"state,omitempty"`
	Nonce               string     `json:"nonce,omitempty"`
	CodeChallenge       string     `json:"codeChallenge,omitempty"`
	CodeChallengeMethod string     `json:"codeChallengeMethod,omitempty"`
	DeviceID            string     `json:"deviceId,omitempty"`
	TenantID            string     `json:"tenantId,omitempty"`
	LoginIP             string     `json:"loginIp,omitempty"`
	UserAgent           string     `json:"userAgent,omitempty"`
	TraceID             string     `json:"traceId,omitempty"`
	CreatedAt           *time.Time `json:"createdAt,omitempty"`
	ExpiresAt           *time.Time `json:"expiresAt,omitempty"`
}

type CreateAuthorizationSessionRequest struct {
	ClientID            string          `json:"clientId"`
	ResponseType        string          `json:"responseType,omitempty"`
	RedirectURI         string          `json:"redirectUri"`
	Scopes              []string        `json:"scopes,omitempty"`
	State               string          `json:"state,omitempty"`
	Nonce               string          `json:"nonce,omitempty"`
	Prompt              string          `json:"prompt,omitempty"`
	CodeChallenge       string          `json:"codeChallenge,omitempty"`
	CodeChallengeMethod string          `json:"codeChallengeMethod,omitempty"`
	RequestContext      *RequestContext `json:"requestContext,omitempty"`
}

type CompleteInteractiveAuthenticationCommand struct {
	LoginTransactionID   string          `json:"loginTransactionId"`
	UserID               int64           `json:"userId"`
	PlatformCode         string          `json:"platformCode,omitempty"`
	ACR                  string          `json:"acr,omitempty"`
	AMR                  []string        `json:"amr,omitempty"`
	LoginMethod          string          `json:"loginMethod,omitempty"`
	ExternalProviderCode string          `json:"externalProviderCode,omitempty"`
	ExternalIdentityID   int64           `json:"externalIdentityId,omitempty"`
	AuthTime             *time.Time      `json:"authTime,omitempty"`
	RequestContext       *RequestContext `json:"requestContext,omitempty"`
}

type AuthenticationCompletionResult struct {
	Authenticated            bool   `json:"authenticated"`
	LoginTransactionID       string `json:"loginTransactionId,omitempty"`
	RedirectURL              string `json:"redirectUrl,omitempty"`
	SessionCookieHeaderValue string `json:"sessionCookieHeaderValue,omitempty"`
}

type BootstrapSessionCommand struct {
	UserID               int64           `json:"userId"`
	ClientID             string          `json:"clientId,omitempty"`
	PlatformCode         string          `json:"platformCode,omitempty"`
	ACR                  string          `json:"acr,omitempty"`
	AMR                  []string        `json:"amr,omitempty"`
	LoginMethod          string          `json:"loginMethod,omitempty"`
	ExternalProviderCode string          `json:"externalProviderCode,omitempty"`
	ExternalIdentityID   int64           `json:"externalIdentityId,omitempty"`
	RequestContext       *RequestContext `json:"requestContext,omitempty"`
}

type BootstrapSessionResult struct {
	AccessToken              string `json:"accessToken,omitempty"`
	TokenType                string `json:"tokenType,omitempty"`
	AccessTTLSeconds         int64  `json:"accessTtlSec,omitempty"`
	SessionCookieHeaderValue string `json:"sessionCookieHeaderValue,omitempty"`
	RefreshCookieHeaderValue string `json:"refreshCookieHeaderValue,omitempty"`
}

type SessionRecord struct {
	SessionID            string     `json:"sessionId"`
	UserID               int64      `json:"userId"`
	ClientID             string     `json:"clientId"`
	PlatformCode         string     `json:"platformCode,omitempty"`
	DeviceID             string     `json:"deviceId,omitempty"`
	LoginIP              string     `json:"loginIp,omitempty"`
	UserAgent            string     `json:"userAgent,omitempty"`
	ACR                  string     `json:"acr,omitempty"`
	AMR                  []string   `json:"amr,omitempty"`
	LoginMethod          string     `json:"loginMethod,omitempty"`
	ExternalProviderCode string     `json:"externalProviderCode,omitempty"`
	ExternalIdentityID   int64      `json:"externalIdentityId,omitempty"`
	LoginAt              *time.Time `json:"loginAt,omitempty"`
	LastAccessAt         *time.Time `json:"lastAccessAt,omitempty"`
	ExpiresAt            *time.Time `json:"expiresAt,omitempty"`
	RevokedAt            *time.Time `json:"revokedAt,omitempty"`
	Status               string     `json:"status,omitempty"`
	MetadataJSON         string     `json:"metadataJson,omitempty"`
}

type AccessTokenPrincipal struct {
	TokenID   string     `json:"tokenId"`
	UserID    int64      `json:"userId"`
	Subject   string     `json:"subject"`
	ClientID  string     `json:"clientId"`
	SessionID string     `json:"sessionId"`
	Scopes    []string   `json:"scopes,omitempty"`
	ACR       string     `json:"acr,omitempty"`
	AMR       []string   `json:"amr,omitempty"`
	IssuedAt  *time.Time `json:"issuedAt,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type AuditEventRecord struct {
	ID         int64      `json:"id"`
	EventType  string     `json:"eventType"`
	ClientID   string     `json:"clientId,omitempty"`
	UserID     *int64     `json:"userId,omitempty"`
	SessionID  string     `json:"sessionId,omitempty"`
	DeviceID   string     `json:"deviceId,omitempty"`
	TenantID   string     `json:"tenantId,omitempty"`
	LoginIP    string     `json:"loginIp,omitempty"`
	UserAgent  string     `json:"userAgent,omitempty"`
	Result     string     `json:"result,omitempty"`
	ReasonCode string     `json:"reasonCode,omitempty"`
	DetailJSON string     `json:"detailJson,omitempty"`
	TraceID    string     `json:"traceId,omitempty"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
}

type ClientRecord struct {
	ID                 int64    `json:"id"`
	ClientID           string   `json:"clientId"`
	ClientName         string   `json:"clientName"`
	ClientType         string   `json:"clientType,omitempty"`
	ClientAuthMethod   string   `json:"clientAuthMethod,omitempty"`
	GrantTypes         []string `json:"grantTypes,omitempty"`
	Scopes             []string `json:"scopes,omitempty"`
	RequirePKCE        bool     `json:"requirePkce"`
	RequireConsent     bool     `json:"requireConsent"`
	TrustedFirstParty  bool     `json:"trustedFirstParty"`
	AccessTokenTTLSec  int      `json:"accessTokenTtlSec,omitempty"`
	RefreshTokenTTLSec int      `json:"refreshTokenTtlSec,omitempty"`
	Status             int      `json:"status"`
	MetadataJSON       string   `json:"metadataJson,omitempty"`
}

type ClientAdminQuery struct {
	Keyword    string `json:"keyword,omitempty"`
	Status     *int   `json:"status,omitempty"`
	ClientType string `json:"clientType,omitempty"`
	Current    int    `json:"current,omitempty"`
	PageSize   int    `json:"pageSize,omitempty"`
}

type ClientAdminRecord struct {
	ID                     int64     `json:"id"`
	ClientID               string    `json:"clientId"`
	ClientName             string    `json:"clientName"`
	ClientType             string    `json:"clientType"`
	ClientAuthMethod       string    `json:"clientAuthMethod"`
	GrantTypes             []string  `json:"grantTypes"`
	Scopes                 []string  `json:"scopes"`
	RequirePKCE            bool      `json:"requirePkce"`
	RequireConsent         bool      `json:"requireConsent"`
	TrustedFirstParty      bool      `json:"trustedFirstParty"`
	AccessTokenTTLSec      int       `json:"accessTokenTtlSec"`
	RefreshTokenTTLSec     int       `json:"refreshTokenTtlSec"`
	Status                 int       `json:"status"`
	MetadataJSON           string    `json:"metadataJson,omitempty"`
	ActiveRedirectURICount int       `json:"activeRedirectUriCount"`
	ActiveSecretCount      int       `json:"activeSecretCount"`
	CreateTime             time.Time `json:"createTime"`
	UpdateTime             time.Time `json:"updateTime"`
}

type ClientAdminPage struct {
	Records  []ClientAdminRecord `json:"records"`
	Total    int64               `json:"total"`
	Current  int                 `json:"current"`
	PageSize int                 `json:"pageSize"`
}

type ClientAdminDetail struct {
	ClientAdminRecord
	RedirectURIs []ClientRedirectURIRecord   `json:"redirectUris,omitempty"`
	Secrets      []ClientSecretSummaryRecord `json:"secrets,omitempty"`
}

type CreateClientAdminRequest struct {
	ClientID           string   `json:"clientId"`
	ClientName         string   `json:"clientName"`
	ClientType         string   `json:"clientType"`
	ClientAuthMethod   string   `json:"clientAuthMethod"`
	GrantTypes         []string `json:"grantTypes"`
	Scopes             []string `json:"scopes"`
	RequirePKCE        bool     `json:"requirePkce"`
	RequireConsent     bool     `json:"requireConsent"`
	TrustedFirstParty  bool     `json:"trustedFirstParty"`
	AccessTokenTTLSec  int      `json:"accessTokenTtlSec"`
	RefreshTokenTTLSec int      `json:"refreshTokenTtlSec"`
	MetadataJSON       string   `json:"metadataJson,omitempty"`
}

type UpdateClientAdminRequest struct {
	ClientName         string   `json:"clientName"`
	ClientType         string   `json:"clientType"`
	ClientAuthMethod   string   `json:"clientAuthMethod"`
	GrantTypes         []string `json:"grantTypes"`
	Scopes             []string `json:"scopes"`
	RequirePKCE        bool     `json:"requirePkce"`
	RequireConsent     bool     `json:"requireConsent"`
	TrustedFirstParty  bool     `json:"trustedFirstParty"`
	AccessTokenTTLSec  int      `json:"accessTokenTtlSec"`
	RefreshTokenTTLSec int      `json:"refreshTokenTtlSec"`
	MetadataJSON       string   `json:"metadataJson,omitempty"`
}

type UpdateClientStatusRequest struct {
	Status               int    `json:"status"`
	Reason               string `json:"reason,omitempty"`
	RevokeActiveSessions *bool  `json:"revokeActiveSessions,omitempty"`
}

type ClientAdminSaveRequest = CreateClientAdminRequest
type ClientStatusRequest = UpdateClientStatusRequest

type ClientRedirectURIRecord struct {
	ID                    int64     `json:"id"`
	ClientID              string    `json:"clientId"`
	RedirectURI           string    `json:"redirectUri"`
	PostLogoutRedirectURI string    `json:"postLogoutRedirectUri,omitempty"`
	Status                int       `json:"status"`
	CreateTime            time.Time `json:"createTime"`
	UpdateTime            time.Time `json:"updateTime"`
}

type UpdateClientRedirectURIsRequest struct {
	RedirectURIs           []string `json:"redirectUris"`
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris,omitempty"`
}

type ClientRedirectURIUpdateRequest = UpdateClientRedirectURIsRequest

type ClientSecretSummaryRecord struct {
	SecretID   int64      `json:"secretId"`
	SecretHint string     `json:"secretHint,omitempty"`
	Status     int        `json:"status"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreateTime time.Time  `json:"createTime"`
}

type ClientSecretRecord = ClientSecretSummaryRecord

type GenerateClientSecretRequest struct {
	ExpiresInDays int    `json:"expiresInDays,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type GenerateClientSecretResponse struct {
	SecretID     int64      `json:"secretId"`
	ClientSecret string     `json:"clientSecret"`
	SecretHint   string     `json:"secretHint,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
}

type UpdateClientSecretStatusRequest struct {
	Status              int    `json:"status"`
	Reason              string `json:"reason,omitempty"`
	AllowNoActiveSecret bool   `json:"allowNoActiveSecret,omitempty"`
}

type ClientSecretGenerateRequest = GenerateClientSecretRequest
type ClientSecretGenerateResponse = GenerateClientSecretResponse
type ClientSecretStatusRequest = UpdateClientSecretStatusRequest

// ManagedClientCommand is restricted to the fixed Hub-to-Node OIDC profile.
type ManagedClientCommand struct {
	ClientID      string
	ClientName    string
	RedirectURI   string
	RotateSecret  bool
	OwnerNodeCode string
}

// ManagedClientResult returns plaintext only for a newly created or rotated secret.
type ManagedClientResult struct {
	ClientID     string
	ClientSecret string
}

// ManagedClientStatusCommand changes only an exact-owner system-managed client.
type ManagedClientStatusCommand struct {
	ClientID      string
	OwnerNodeCode string
	Status        int
}
