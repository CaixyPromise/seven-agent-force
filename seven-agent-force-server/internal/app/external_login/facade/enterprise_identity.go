package facade

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type EnterpriseProvider string

const (
	EnterpriseProviderFeishu EnterpriseProvider = "FEISHU"
	EnterpriseProviderWeCom  EnterpriseProvider = "WECOM"
)

type EnterpriseConnectionRecord struct {
	ID                         int64      `json:"id,string"`
	Provider                   string     `json:"provider"`
	DisplayName                string     `json:"displayName"`
	AppIDHint                  string     `json:"appIdHint,omitempty"`
	CorpIDHint                 string     `json:"corpIdHint,omitempty"`
	AgentIDHint                string     `json:"agentIdHint,omitempty"`
	CallbackURL                string     `json:"callbackUrl,omitempty"`
	CredentialConfigured       bool       `json:"credentialConfigured"`
	CredentialVersion          int64      `json:"credentialVersion,string"`
	BindingEnabled             bool       `json:"bindingEnabled"`
	NotificationDirectEnabled  bool       `json:"notificationDirectEnabled"`
	NotificationMentionEnabled bool       `json:"notificationMentionEnabled"`
	LoginEnabled               bool       `json:"loginEnabled"`
	Status                     string     `json:"status"`
	LastProbeStatus            string     `json:"lastProbeStatus"`
	LastProbeReasonCode        string     `json:"lastProbeReasonCode,omitempty"`
	LastProbeAt                *time.Time `json:"lastProbeAt,omitempty"`
	Version                    int64      `json:"version,string"`
	CreatedAt                  time.Time  `json:"createdAt"`
	UpdatedAt                  time.Time  `json:"updatedAt"`
}

type EnterpriseConnectionProviderSetup struct {
	Provider    string `json:"provider"`
	CallbackURL string `json:"callbackUrl,omitempty"`
}

type EnterpriseConnectionSetup struct {
	OrganizationCode string                              `json:"organizationCode"`
	LoginURL         string                              `json:"loginUrl"`
	Providers        []EnterpriseConnectionProviderSetup `json:"providers"`
}

type SaveEnterpriseConnectionRequest struct {
	Provider                   string `json:"provider"`
	DisplayName                string `json:"displayName"`
	AppID                      string `json:"appId,omitempty"`
	CorpID                     string `json:"corpId,omitempty"`
	AgentID                    string `json:"agentId,omitempty"`
	ClientSecret               string `json:"clientSecret,omitempty"`
	BindingEnabled             bool   `json:"bindingEnabled"`
	NotificationDirectEnabled  bool   `json:"notificationDirectEnabled"`
	NotificationMentionEnabled bool   `json:"notificationMentionEnabled"`
	LoginEnabled               bool   `json:"loginEnabled"`
	ExpectedVersion            int64  `json:"expectedVersion,string,omitempty"`
}

type EnterpriseConnectionStatusRequest struct {
	Enabled         bool   `json:"enabled"`
	ExpectedVersion int64  `json:"expectedVersion,string"`
	Reason          string `json:"reason"`
}

type EnterpriseAuthorizationStartRequest struct {
	Provider      string `json:"provider"`
	RedirectAfter string `json:"redirectAfter,omitempty"`
}

type EnterpriseLoginStartRequest struct {
	Provider         string `json:"provider"`
	OrganizationCode string `json:"organizationCode"`
	RedirectAfter    string `json:"redirectAfter,omitempty"`
}

type EnterpriseAuthorizationStartResult struct {
	AuthorizationIdentity string    `json:"authorizationIdentity"`
	RedirectURL           string    `json:"redirectUrl"`
	ExpiresAt             time.Time `json:"expiresAt"`
}

type EnterpriseAuthorizationCallbackRequest struct {
	Provider string `json:"provider"`
	Code     string `json:"code"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
}

type EnterpriseAuthorizationResult struct {
	Outcome                  string `json:"outcome"`
	Purpose                  string `json:"purpose"`
	Provider                 string `json:"provider"`
	ExternalIdentityID       int64  `json:"externalIdentityId,string,omitempty"`
	RedirectAfter            string `json:"redirectAfter,omitempty"`
	Authenticated            bool   `json:"authenticated"`
	SessionCookieHeaderValue string `json:"-"`
	RefreshCookieHeaderValue string `json:"-"`
}

type EnterpriseIdentityRecord struct {
	ID              int64      `json:"id,string"`
	UserID          int64      `json:"userId,string"`
	UserDisplayName string     `json:"userDisplayName,omitempty"`
	Provider        string     `json:"provider"`
	DisplayHint     string     `json:"displayHint"`
	Status          string     `json:"status"`
	IdentityVersion int64      `json:"identityVersion,string"`
	DirectEnabled   bool       `json:"directEnabled"`
	MentionEnabled  bool       `json:"mentionEnabled"`
	LoginEnabled    bool       `json:"loginEnabled"`
	VerifiedAt      time.Time  `json:"verifiedAt"`
	RevokedAt       *time.Time `json:"revokedAt,omitempty"`
}

type EnterpriseNotificationSettingsRequest struct {
	DirectEnabled           bool  `json:"directEnabled"`
	MentionEnabled          bool  `json:"mentionEnabled"`
	ExpectedIdentityVersion int64 `json:"expectedIdentityVersion,string"`
}

type EnterpriseUnbindRequest struct {
	ExpectedIdentityVersion int64  `json:"expectedIdentityVersion,string"`
	Reason                  string `json:"reason"`
}

type EnterpriseLoginAccessRequest struct {
	Enabled                 bool   `json:"enabled"`
	ExpectedIdentityVersion int64  `json:"expectedIdentityVersion,string"`
	Reason                  string `json:"reason"`
}

type NotificationAuthorityRequest struct {
	OrganizationID int64
	UserID         int64
	Provider       string
	GrantType      string
}

type NotificationAuthority struct {
	OrganizationID         int64
	UserID                 int64
	ExternalIdentityID     int64
	IdentityVersion        int64
	EnterpriseConnectionID int64
	ConnectionVersion      int64
	ConnectionConfigDigest string
	IdentityKind           string
	Subject                string
	GrantType              string
	AuthorityDigest        string
	ExpiresAt              time.Time
}

// EnterpriseApplicationCredentialLease is a short-lived in-memory capability.
// It is never serialized by HTTP and Notification must discard it after one
// provider attempt.
type EnterpriseApplicationCredentialLease struct {
	OrganizationID         int64
	EnterpriseConnectionID int64
	ConnectionVersion      int64
	ConnectionConfigDigest string
	Provider               string
	AppID                  string
	CorpID                 string
	AgentID                string
	ClientSecret           string
	CredentialVersion      int64
	ExpiresAt              time.Time
}

type NotificationAuthoritySnapshot struct {
	OrganizationID         int64
	UserID                 int64
	ExternalIdentityID     int64
	IdentityVersion        int64
	EnterpriseConnectionID int64
	ConnectionVersion      int64
	AuthorityDigest        string
	GrantType              string
}

type EnterpriseIdentityReadiness struct {
	Available    bool
	DirectReady  bool
	MentionReady bool
	ReasonCode   string
}

type EnterpriseLoginOption struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
}

type EnterpriseLoginOptions struct {
	OrganizationName string                  `json:"organizationName"`
	Methods          []EnterpriseLoginOption `json:"methods"`
}

type EnterpriseIdentityFacade interface {
	ResolveEnterpriseLoginOptions(context.Context, string) (*EnterpriseLoginOptions, error)
	GetEnterpriseConnectionSetup(ctx context.Context, organizationID int64) (*EnterpriseConnectionSetup, error)
	ListEnterpriseConnections(ctx context.Context, organizationID int64) ([]EnterpriseConnectionRecord, error)
	ListEnterpriseIdentities(ctx context.Context, organizationID int64) ([]EnterpriseIdentityRecord, error)
	SaveEnterpriseConnection(ctx context.Context, organizationID, actorID int64, req SaveEnterpriseConnectionRequest, proof stepup.ProofMetadata) (*EnterpriseConnectionRecord, error)
	SetEnterpriseConnectionStatus(ctx context.Context, organizationID, actorID, connectionID int64, req EnterpriseConnectionStatusRequest, proof stepup.ProofMetadata) error
	StartEnterpriseBinding(ctx context.Context, organizationID, userID int64, req EnterpriseAuthorizationStartRequest) (*EnterpriseAuthorizationStartResult, error)
	StartEnterpriseLogin(ctx context.Context, req EnterpriseLoginStartRequest) (*EnterpriseAuthorizationStartResult, error)
	CompleteEnterpriseAuthorization(ctx context.Context, req EnterpriseAuthorizationCallbackRequest) (*EnterpriseAuthorizationResult, error)
	ListMyEnterpriseIdentities(ctx context.Context, organizationID, userID int64) ([]EnterpriseIdentityRecord, error)
	SendMyEnterpriseTestMessage(ctx context.Context, organizationID, userID, identityID int64) error
	UpdateMyEnterpriseNotificationSettings(ctx context.Context, organizationID, userID, identityID int64, req EnterpriseNotificationSettingsRequest) error
	UnbindMyEnterpriseIdentity(ctx context.Context, organizationID, userID, identityID int64, req EnterpriseUnbindRequest, proof stepup.ProofMetadata) error
	SetEnterpriseLoginAccess(ctx context.Context, organizationID, actorID, identityID int64, req EnterpriseLoginAccessRequest, proof stepup.ProofMetadata) error
	ResolveNotificationAuthority(ctx context.Context, req NotificationAuthorityRequest) (*NotificationAuthority, error)
	ValidateNotificationAuthority(ctx context.Context, snapshot NotificationAuthoritySnapshot) error
	AcquireApplicationCredentialLease(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionConfigDigest string) (*EnterpriseApplicationCredentialLease, error)
	ResolveEnterpriseReadiness(ctx context.Context, organizationID, userID int64, provider string) (*EnterpriseIdentityReadiness, error)
	DiscoverEnterpriseGroups(ctx context.Context, organizationID, connectionID int64) ([]EnterpriseGroupTarget, error)
}

// EnterpriseGroupTarget is a short-lived server-to-server discovery result.
// Target is deliberately excluded from JSON and must be encrypted by the
// notification route service before any response is returned to a browser.
type EnterpriseGroupTarget struct {
	DisplayName            string `json:"displayName"`
	Description            string `json:"description,omitempty"`
	Target                 string `json:"-"`
	ConnectionID           int64  `json:"-"`
	ConnectionVersion      int64  `json:"-"`
	ConnectionConfigDigest string `json:"-"`
}
