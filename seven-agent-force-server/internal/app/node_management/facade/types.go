package facade

import (
	"context"
	"strconv"
	"time"

	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
)

const (
	// MaxSessionReferencesPerCommand is the shared Hub/Node revoke batch contract.
	MaxSessionReferencesPerCommand = 100

	UserStatusNormal        = 0
	UserStatusDisabled      = 1
	UserStatusPendingReview = 2

	SessionStatusActive  = "ACTIVE"
	SessionStatusExpired = "EXPIRED"
	SessionStatusRevoked = "REVOKED"
)

// NodeDescriptor describes the safe identity and capabilities of this Node.
type NodeDescriptor struct {
	NodeCode     string   `json:"nodeCode"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Health       string   `json:"health"`
}

// UserPageQuery defines bounded local-user search criteria.
type UserPageQuery struct {
	Current int64
	Size    int64
	Keyword string
	Status  *int
}

// UserSummary is a contact-masked user projection for Hub management.
type UserSummary struct {
	UserID      string     `json:"userId"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	EmailMasked string     `json:"emailMasked,omitempty"`
	PhoneMasked string     `json:"phoneMasked,omitempty"`
	Status      int        `json:"status"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

// UserDetail intentionally exposes no more sensitive data than UserSummary.
type UserDetail = UserSummary

// UserPage is a page of safe local-user projections.
type UserPage struct {
	Current int64         `json:"current"`
	Size    int64         `json:"size"`
	Total   int64         `json:"total"`
	Records []UserSummary `json:"records"`
}

// SessionPageQuery defines local-session pagination.
type SessionPageQuery struct {
	Current int64
	Size    int64
}

// SessionSummary exposes an opaque reference and effective session state.
type SessionSummary struct {
	SessionRef  string     `json:"sessionRef"`
	ClientID    string     `json:"clientId"`
	LoginMethod string     `json:"loginMethod,omitempty"`
	LoginAt     *time.Time `json:"loginAt,omitempty"`
	LastAccess  *time.Time `json:"lastAccessAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	Status      string     `json:"status"`
}

// SessionPage is a page of safe local-session projections.
type SessionPage struct {
	Current int64            `json:"current"`
	Size    int64            `json:"size"`
	Total   int64            `json:"total"`
	Records []SessionSummary `json:"records"`
}

// LoginMethod is the safe managed subset of a local login method.
type LoginMethod struct {
	MethodType     string `json:"methodType"`
	ProviderCode   string `json:"providerCode,omitempty"`
	DisplayName    string `json:"displayName"`
	Icon           string `json:"icon,omitempty"`
	SortOrder      int    `json:"sortOrder"`
	DisplayEnabled bool   `json:"displayEnabled"`
	LoginEnabled   bool   `json:"loginEnabled"`
}

// SourceRule is the safe managed subset of a platform source rule.
type SourceRule struct {
	MatchType  string `json:"matchType"`
	MatchValue string `json:"matchValue"`
	Priority   int    `json:"priority"`
	Status     int    `json:"status"`
}

// ManagedLoginPolicy excludes local RBAC IDs and opaque metadata JSON.
type ManagedLoginPolicy struct {
	PlatformCode      string        `json:"platformCode"`
	Status            int           `json:"status"`
	AllowAutoRegister bool          `json:"allowAutoRegister"`
	AllowFormRegister bool          `json:"allowFormRegister"`
	LoginMethods      []LoginMethod `json:"loginMethods"`
	SourceRules       []SourceRule  `json:"sourceRules"`
}

// SetUserStatusCommand sets one absolute local-user status.
type SetUserStatusCommand struct {
	UserID         string `json:"-"`
	Status         int    `json:"status"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"-"`
}

// RevokeUserSessionsCommand revokes all or an explicit set of opaque sessions.
type RevokeUserSessionsCommand struct {
	UserID         string   `json:"-"`
	All            bool     `json:"all"`
	SessionRefs    []string `json:"sessionRefs,omitempty"`
	Reason         string   `json:"reason"`
	IdempotencyKey string   `json:"-"`
}

// ApplyLoginPolicyCommand applies a complete safe policy snapshot.
type ApplyLoginPolicyCommand struct {
	ManagedLoginPolicy
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"-"`
}

// ApplyHubConnectionCommand applies one complete versioned Hub connection.
type ApplyHubConnectionCommand struct {
	ConnectionVersion string `json:"connectionVersion"`
	TargetRevision    int64  `json:"targetRevision,omitempty"`
	Enabled           bool   `json:"enabled"`
	DisplayName       string `json:"displayName,omitempty"`
	Issuer            string `json:"issuer"`
	ClientID          string `json:"clientId"`
	ClientSecret      string `json:"clientSecret,omitempty"`
	RedirectURI       string `json:"redirectUri"`
	Reason            string `json:"reason"`
	IdempotencyKey    string `json:"-"`
}

// ManagedHubConnectionCommand is the typed port payload consumed by Task 7.
type ManagedHubConnectionCommand struct {
	ConnectionVersion string
	TargetRevision    int64
	Enabled           bool
	DisplayName       string
	Issuer            string
	ClientID          string
	ClientSecret      string
	RedirectURI       string
}

// CommandResult reports the observed changed count and replay state.
type CommandResult struct {
	ChangedCount int64 `json:"changedCount"`
	Replayed     bool  `json:"replayed,omitempty"`
}

// RevokeResult is the session-revocation command result.
type RevokeResult = CommandResult

// SessionReference is the authenticated internal value behind an opaque ref.
type SessionReference struct {
	UserID    int64
	SessionID string
}

// SessionReferenceCodec creates and validates opaque Node-bound references.
type SessionReferenceCodec interface {
	Encode(ctx context.Context, record ssofacade.SessionRecord) (string, error)
	Decode(ctx context.Context, reference string) (SessionReference, error)
}

// HubConnectionPort is bound by the managed OIDC implementation in Task 7.
type HubConnectionPort interface {
	ApplyHubConnection(ctx context.Context, command ManagedHubConnectionCommand) error
}

// FormatID serializes a 64-bit identifier without JSON precision loss.
func FormatID(value int64) string {
	return strconv.FormatInt(value, 10)
}
