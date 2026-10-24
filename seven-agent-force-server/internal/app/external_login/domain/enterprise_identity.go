package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type EnterpriseProvider string

const (
	EnterpriseProviderFeishu EnterpriseProvider = "FEISHU"
	EnterpriseProviderWeCom  EnterpriseProvider = "WECOM"
)

type AuthorizationPurpose string

const (
	AuthorizationPurposeNotificationBind AuthorizationPurpose = "NOTIFICATION_BIND"
	AuthorizationPurposeLogin            AuthorizationPurpose = "LOGIN"
)

type EnterpriseGrantType string

const (
	EnterpriseGrantIdentityLink        EnterpriseGrantType = "IDENTITY_LINK"
	EnterpriseGrantNotificationDirect  EnterpriseGrantType = "NOTIFICATION_DIRECT"
	EnterpriseGrantNotificationMention EnterpriseGrantType = "NOTIFICATION_MENTION"
	EnterpriseGrantLogin               EnterpriseGrantType = "LOGIN"
)

type EnterpriseConnectionStatus string

const (
	EnterpriseConnectionActive   EnterpriseConnectionStatus = "ACTIVE"
	EnterpriseConnectionDisabled EnterpriseConnectionStatus = "DISABLED"
)

type EnterpriseIdentityStatus string

const (
	EnterpriseIdentityActive   EnterpriseIdentityStatus = "ACTIVE"
	EnterpriseIdentityRevoked  EnterpriseIdentityStatus = "REVOKED"
	EnterpriseIdentityDisabled EnterpriseIdentityStatus = "DISABLED"
)

type EnterpriseGrantStatus string

const (
	EnterpriseGrantActive  EnterpriseGrantStatus = "ACTIVE"
	EnterpriseGrantRevoked EnterpriseGrantStatus = "REVOKED"
)

type EnterpriseAuthorizationStatus string

const (
	EnterpriseAuthorizationActive         EnterpriseAuthorizationStatus = "ACTIVE"
	EnterpriseAuthorizationProcessing     EnterpriseAuthorizationStatus = "PROCESSING"
	EnterpriseAuthorizationSucceeded      EnterpriseAuthorizationStatus = "SUCCEEDED"
	EnterpriseAuthorizationFailed         EnterpriseAuthorizationStatus = "FAILED"
	EnterpriseAuthorizationOutcomeUnknown EnterpriseAuthorizationStatus = "OUTCOME_UNKNOWN"
)

type EnterpriseProtocolCapabilities struct {
	SupportsAuthorizationCode bool `json:"supportsAuthorizationCode"`
	SupportsPKCE              bool `json:"supportsPkce"`
	SupportsOIDCNonce         bool `json:"supportsOidcNonce"`
	SupportsUserInfo          bool `json:"supportsUserInfo"`
}

type EnterpriseConnection struct {
	ID                         int64
	OrganizationID             int64
	Provider                   EnterpriseProvider
	DisplayName                string
	AppID                      string
	CorpID                     string
	AgentID                    string
	EndpointProfile            string
	ClientSecretCiphertext     string
	ClientSecretEDEK           string
	ClientSecretWrapKeyRef     string
	CredentialVersion          int64
	BindingEnabled             bool
	NotificationDirectEnabled  bool
	NotificationMentionEnabled bool
	LoginEnabled               bool
	ProtocolCapabilities       EnterpriseProtocolCapabilities
	ConfigDigest               string
	Status                     EnterpriseConnectionStatus
	LastProbeStatus            string
	LastProbeReasonCode        string
	LastProbeAt                *time.Time
	Version                    int64
	CreatorID                  int64
	UpdaterID                  int64
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type EnterpriseExternalIdentity struct {
	ID                  int64
	OrganizationID      int64
	ConnectionID        int64
	Provider            EnterpriseProvider
	UserID              int64
	SubjectKind         string
	SubjectCiphertext   string
	SubjectEDEK         string
	SubjectWrapKeyRef   string
	SubjectDigest       string
	SubjectDigestKeyRef string
	DisplayHint         string
	Status              EnterpriseIdentityStatus
	IdentityVersion     int64
	VerifiedAt          time.Time
	RevokedAt           *time.Time
	CreatorID           int64
	UpdaterID           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type EnterpriseIdentityGrant struct {
	ID                 int64
	OrganizationID     int64
	ExternalIdentityID int64
	GrantType          EnterpriseGrantType
	Status             EnterpriseGrantStatus
	Version            int64
	GrantedBy          int64
	GrantedAt          time.Time
	RevokedBy          *int64
	RevokedAt          *time.Time
	ReasonCode         string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type EnterpriseAuthorization struct {
	ID                     int64
	AuthorizationIdentity  string
	Purpose                AuthorizationPurpose
	OrganizationID         int64
	ConnectionID           int64
	ConnectionVersion      int64
	ConnectionConfigDigest string
	BindUserID             int64
	RequestedGrants        []EnterpriseGrantType
	RequestedGrantsDigest  string
	StateHash              string
	NonceHash              string
	CodeVerifierCiphertext string
	CodeVerifierEDEK       string
	CodeVerifierWrapKeyRef string
	RedirectAfter          string
	ExpiresAt              time.Time
	Status                 EnterpriseAuthorizationStatus
	ResultIdentityID       *int64
	ReasonCode             string
	ConsumedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
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
	GrantType              EnterpriseGrantType
	AuthorityDigest        string
	ExpiresAt              time.Time
}

func NormalizeEnterpriseProvider(raw string) (EnterpriseProvider, error) {
	provider := EnterpriseProvider(strings.ToUpper(strings.TrimSpace(raw)))
	switch provider {
	case EnterpriseProviderFeishu, EnterpriseProviderWeCom:
		return provider, nil
	default:
		return "", fmt.Errorf("unsupported enterprise provider")
	}
}

func NormalizeAuthorizationPurpose(raw string) (AuthorizationPurpose, error) {
	purpose := AuthorizationPurpose(strings.ToUpper(strings.TrimSpace(raw)))
	switch purpose {
	case AuthorizationPurposeNotificationBind, AuthorizationPurposeLogin:
		return purpose, nil
	default:
		return "", fmt.Errorf("invalid enterprise authorization purpose")
	}
}

func NormalizeEnterpriseGrant(raw string) (EnterpriseGrantType, error) {
	grant := EnterpriseGrantType(strings.ToUpper(strings.TrimSpace(raw)))
	switch grant {
	case EnterpriseGrantIdentityLink, EnterpriseGrantNotificationDirect, EnterpriseGrantNotificationMention, EnterpriseGrantLogin:
		return grant, nil
	default:
		return "", fmt.Errorf("invalid enterprise identity grant")
	}
}

func NormalizeRequestedGrants(items []EnterpriseGrantType, purpose AuthorizationPurpose) ([]EnterpriseGrantType, error) {
	if _, err := NormalizeAuthorizationPurpose(string(purpose)); err != nil {
		return nil, err
	}
	seen := make(map[EnterpriseGrantType]struct{}, len(items))
	result := make([]EnterpriseGrantType, 0, len(items))
	for _, raw := range items {
		grant, err := NormalizeEnterpriseGrant(string(raw))
		if err != nil {
			return nil, err
		}
		if purpose == AuthorizationPurposeNotificationBind && grant == EnterpriseGrantLogin {
			return nil, fmt.Errorf("notification binding cannot request login grant")
		}
		if _, ok := seen[grant]; ok {
			continue
		}
		seen[grant] = struct{}{}
		result = append(result, grant)
	}
	if purpose == AuthorizationPurposeNotificationBind {
		for _, required := range []EnterpriseGrantType{EnterpriseGrantIdentityLink, EnterpriseGrantNotificationDirect, EnterpriseGrantNotificationMention} {
			if _, ok := seen[required]; !ok {
				return nil, fmt.Errorf("notification binding grant set is incomplete")
			}
		}
	}
	if purpose == AuthorizationPurposeLogin {
		if len(result) != 0 {
			return nil, fmt.Errorf("login authorization cannot mutate grants")
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func SubjectKindForProvider(provider EnterpriseProvider) string {
	switch provider {
	case EnterpriseProviderFeishu:
		return "FEISHU_OPEN_ID"
	case EnterpriseProviderWeCom:
		return "WECOM_USERID"
	default:
		return ""
	}
}
