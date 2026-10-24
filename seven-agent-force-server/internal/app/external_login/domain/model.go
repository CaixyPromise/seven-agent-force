package domain

import "time"

const (
	ProtocolTypeOAuth2 = "OAUTH2"
	ProtocolTypeOIDC   = "OIDC"

	TokenEndpointAuthMethodClientSecretBasic = "client_secret_basic"

	ProviderStatusActive   = 0
	ProviderStatusDisabled = 1

	ProviderMethodStatusActive   = 0
	ProviderMethodStatusDisabled = 1

	IdentityStatusActive   = 0
	IdentityStatusDisabled = 1
	IdentityStatusUnlinked = 2

	LoginStateStatusActive   = 0
	LoginStateStatusConsumed = 1
	LoginStateStatusExpired  = 2

	TokenPurposeLogin = "LOGIN"
	TokenPurposeAPI   = "API"

	TokenStatusActive        = 0
	TokenStatusRevoked       = 1
	TokenStatusExpired       = 2
	TokenStatusRefreshFailed = 3
)

type Provider struct {
	ID                       int64
	ProviderCode             string
	ProviderName             string
	ProtocolType             string
	Issuer                   string
	AuthorizationEndpoint    string
	TokenEndpoint            string
	TokenEndpointAuthMethod  string
	UserinfoEndpoint         string
	JWKSURI                  string
	ClientID                 string
	ClientSecretCiphertext   string
	ClientSecretEDEK         string
	ClientSecretWrapKeyRef   string
	Scopes                   []string
	RedirectURI              string
	DisplayName              string
	Icon                     string
	SortOrder                int
	DisplayEnabled           bool
	LoginEnabled             bool
	BindEnabled              bool
	EmailAutoBindEnabled     bool
	AccountAutoCreateEnabled bool
	Status                   int
	MetadataJSON             string
	CreatorID                *int64
	UpdaterID                *int64
	CreateTime               time.Time
	UpdateTime               time.Time
}

type ProviderMethod struct {
	ID             int64
	ProviderCode   string
	MethodKey      string
	CapabilityCode string
	RequiredScopes []string
	Status         int
	MetadataJSON   string
	CreateTime     time.Time
	UpdateTime     time.Time
}

type ExternalIdentity struct {
	ID                  int64
	ProviderCode        string
	ExternalIssuer      string
	ExternalSubject     string
	SubjectCiphertext   string
	SubjectEDEK         string
	SubjectWrapKeyRef   string
	SubjectDigest       string
	SubjectDigestKeyRef string
	UserID              int64
	ExternalLogin       string
	ExternalEmail       string
	EmailVerified       bool
	DisplayName         string
	AvatarURL           string
	ProfileJSON         string
	Status              int
	FirstLinkedAt       time.Time
	LastLoginAt         *time.Time
	LastVerifiedAt      *time.Time
	MetadataJSON        string
	CreatorID           *int64
	UpdaterID           *int64
	CreateTime          time.Time
	UpdateTime          time.Time
}

type ManagedProviderCommand struct {
	ProviderCode      string
	ConnectionVersion string
	RequestHash       string
	CreateTime        time.Time
}

type LoginState struct {
	ID                      int64
	StateID                 string
	ProviderCode            string
	PlatformCode            string
	ProvisioningAuthorityID string
	LoginTransactionID      string
	RedirectAfterLogin      string
	BindUserID              int64
	StateHash               string
	NonceHash               string
	CodeVerifierCiphertext  string
	CodeVerifierEDEK        string
	CodeVerifierWrapKeyRef  string
	Issuer                  string
	ProviderConfigDigest    string
	RedirectURI             string
	ExpiresAt               time.Time
	ConsumedAt              *time.Time
	Status                  int
	LoginIP                 string
	UserAgent               string
	TraceID                 string
	CreateTime              time.Time
	UpdateTime              time.Time
}

type OAuthToken struct {
	ID                 int64
	ProviderCode       string
	IdentityID         int64
	UserID             int64
	TokenPurpose       string
	Scopes             []string
	ScopeHash          string
	TokenSetCiphertext string
	TokenSetEDEK       string
	TokenSetWrapKeyRef string
	AccessExpiresAt    *time.Time
	RefreshExpiresAt   *time.Time
	LastRefreshAt      *time.Time
	RevokedAt          *time.Time
	Status             int
	Version            int
	MetadataJSON       string
	CreateTime         time.Time
	UpdateTime         time.Time
}

type ExternalProfile struct {
	Subject       string
	Login         string
	Email         string
	EmailVerified bool
	DisplayName   string
	AvatarURL     string
	RawProfile    string
}

type ProviderQuery struct {
	Keyword      string
	ProviderCode string
	ProtocolType string
	Status       *int
	DisplayOnly  bool
	Current      int
	PageSize     int
}

type IdentityQuery struct {
	ProviderCode string
	UserID       *int64
	Status       *int
	Keyword      string
	Current      int
	PageSize     int
}

type TokenQuery struct {
	ProviderCode string
	IdentityID   *int64
	UserID       *int64
	TokenPurpose string
	Status       *int
	Current      int
	PageSize     int
}

type ProviderCapability struct {
	ProviderCode  string
	DisplayName   string
	ProtocolType  string
	Capabilities  []string
	DefaultScopes []string
}

type TokenSet struct {
	AccessToken  string     `json:"accessToken,omitempty"`
	RefreshToken string     `json:"refreshToken,omitempty"`
	IDToken      string     `json:"idToken,omitempty"`
	TokenType    string     `json:"tokenType,omitempty"`
	Scopes       []string   `json:"scopes,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	RawProfile   string     `json:"rawProfile,omitempty"`
}
