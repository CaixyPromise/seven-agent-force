package domain

import "time"

const (
	ClientStatusActive   = 0
	ClientStatusDisabled = 1

	SessionStatusActive  = 0
	SessionStatusRevoked = 1
	SessionStatusExpired = 2

	CodeStatusActive   = 0
	CodeStatusConsumed = 1
	CodeStatusExpired  = 2

	RefreshFamilyStatusActive  = 0
	RefreshFamilyStatusRevoked = 1
	RefreshFamilyStatusExpired = 2

	ConsentStatusActive  = 0
	ConsentStatusRevoked = 1
)

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

type Client struct {
	ID                  int64
	ClientID            string
	ClientName          string
	ClientType          string
	ClientAuthMethod    string
	GrantTypes          []string
	Scopes              []string
	RequirePKCE         bool
	RequireConsent      bool
	TrustedFirstParty   bool
	AccessTokenTTLSec   int
	RefreshTokenTTLSec  int
	Status              int
	MetadataJSON        string
	RedirectURIs        []string
	PostLogoutRedirects []string
	SecretHashes        []string
	ActiveRedirectCount int
	ActiveSecretCount   int
	CreateTime          time.Time
	UpdateTime          time.Time
}

type ClientRedirectURI struct {
	ID                    int64
	ClientID              string
	RedirectURI           string
	PostLogoutRedirectURI string
	Status                int
	CreateTime            time.Time
	UpdateTime            time.Time
}

type ClientSecret struct {
	ID         int64
	ClientID   string
	SecretHash string
	SecretHint string
	ExpiresAt  *time.Time
	Status     int
	CreateTime time.Time
	UpdateTime time.Time
}

type ClientSecretSummary struct {
	ID         int64
	ClientID   string
	SecretHint string
	ExpiresAt  *time.Time
	Status     int
	CreateTime time.Time
	UpdateTime time.Time
}

type Session struct {
	ID                   int64
	SessionID            string
	UserID               int64
	ClientID             string
	PlatformCode         string
	DeviceID             string
	LoginIP              string
	UserAgent            string
	ACR                  string
	AMR                  []string
	LoginMethod          string
	ExternalProviderCode string
	ExternalIdentityID   int64
	LoginAt              time.Time
	LastAccessAt         *time.Time
	ExpiresAt            time.Time
	RevokedAt            *time.Time
	Status               int
	MetadataJSON         string
	CreateTime           time.Time
	UpdateTime           time.Time
}

type AuthorizationCode struct {
	ID                  int64
	Code                string
	ClientID            string
	UserID              int64
	SessionID           string
	RedirectURI         string
	Scopes              []string
	CodeChallenge       string
	CodeChallengeMethod string
	Nonce               string
	ACR                 string
	AMR                 []string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	Status              int
	MetadataJSON        string
	CreateTime          time.Time
	UpdateTime          time.Time
}

type RefreshTokenFamily struct {
	ID                int64
	FamilyID          string
	SessionID         string
	ClientID          string
	UserID            int64
	CurrentTokenHash  string
	PreviousTokenHash string
	ReuseDetected     bool
	RotatedAt         *time.Time
	ExpiresAt         time.Time
	RevokedAt         *time.Time
	Status            int
	MetadataJSON      string
	CreateTime        time.Time
	UpdateTime        time.Time
}

type ConsentGrant struct {
	ID           int64
	UserID       int64
	ClientID     string
	Scopes       []string
	GrantedAt    time.Time
	RevokedAt    *time.Time
	Status       int
	MetadataJSON string
	CreateTime   time.Time
	UpdateTime   time.Time
}

type AuditLog struct {
	EventType  string
	ClientID   string
	UserID     *int64
	SessionID  string
	DeviceID   string
	TenantID   string
	LoginIP    string
	UserAgent  string
	Result     string
	ReasonCode string
	DetailJSON string
	TraceID    string
}

type AuditEvent struct {
	ID         int64
	EventType  string
	ClientID   string
	UserID     *int64
	SessionID  string
	DeviceID   string
	TenantID   string
	LoginIP    string
	UserAgent  string
	Result     string
	ReasonCode string
	DetailJSON string
	TraceID    string
	CreatedAt  time.Time
}
