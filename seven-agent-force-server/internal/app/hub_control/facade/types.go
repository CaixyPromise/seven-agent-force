package facade

import (
	"time"
)

// NodePageQuery defines the public Node registry page request.
type NodePageQuery struct {
	Current int    `json:"current,omitempty"`
	Size    int    `json:"size,omitempty"`
	Keyword string `json:"keyword,omitempty"`
	Status  *int   `json:"status,omitempty"`
}

// NodeDetail is a secret-free Node registry projection.
type NodeDetail struct {
	NodeCode              string     `json:"nodeCode"`
	NodeName              string     `json:"nodeName"`
	Status                int        `json:"status"`
	DiscoveryType         string     `json:"discoveryType"`
	ServiceName           string     `json:"serviceName,omitempty"`
	ManagementBaseURL     string     `json:"managementBaseUrl,omitempty"`
	HubIssuer             string     `json:"hubIssuer"`
	OIDCClientID          string     `json:"oidcClientId,omitempty"`
	CapabilitiesJSON      string     `json:"capabilitiesJson,omitempty"`
	ConnectionStatus      string     `json:"connectionStatus"`
	ConnectionVersion     string     `json:"connectionVersion,omitempty"`
	IssuerLockedAt        *time.Time `json:"issuerLockedAt,omitempty"`
	LastConnectionError   string     `json:"lastConnectionError,omitempty"`
	LastConnectionTraceID string     `json:"lastConnectionTraceId,omitempty"`
	LastHealthyAt         *time.Time `json:"lastHealthyAt,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

// NodePage is a registry page with decimal-safe JSON serialization handled globally.
type NodePage struct {
	Current int          `json:"current"`
	Size    int          `json:"size"`
	Total   int64        `json:"total"`
	Records []NodeDetail `json:"records"`
}

// SaveNodeCommand creates or updates Node metadata. Secret replacements are write-only.
type SaveNodeCommand struct {
	OriginalNodeCode  string `json:"-"`
	NodeCode          string `json:"nodeCode"`
	NodeName          string `json:"nodeName"`
	Status            int    `json:"status"`
	DiscoveryType     string `json:"discoveryType"`
	ServiceName       string `json:"serviceName,omitempty"`
	ManagementBaseURL string `json:"managementBaseUrl,omitempty"`
	HubIssuer         string `json:"hubIssuer"`
	ManagementBearer  string `json:"managementBearer,omitempty"`
	CapabilitiesJSON  string `json:"capabilitiesJson,omitempty"`
}

// CopyNodeCommand creates a disabled, non-active Node copy with a distinct code.
type CopyNodeCommand struct {
	NodeCode         string `json:"nodeCode"`
	NodeName         string `json:"nodeName"`
	ManagementBearer string `json:"managementBearer,omitempty"`
}

// SetNodeStatusCommand controls new Hub operations without revoking existing Node sessions.
type SetNodeStatusCommand struct {
	NodeCode string `json:"-"`
	Status   int    `json:"status"`
}

// NodeHealth reports live Node descriptor health.
type NodeHealth struct {
	NodeCode     string   `json:"nodeCode"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Health       string   `json:"health"`
	TraceID      string   `json:"traceId,omitempty"`
}

// NodeUserStatusCommand changes one remote user's absolute status.
type NodeUserStatusCommand struct {
	NodeCode       string `json:"-"`
	UserID         string `json:"-"`
	Status         int    `json:"status"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"-"`
}

// RevokeNodeSessionsCommand revokes remote opaque sessions.
type RevokeNodeSessionsCommand struct {
	NodeCode       string   `json:"-"`
	UserID         string   `json:"-"`
	All            bool     `json:"all"`
	SessionRefs    []string `json:"sessionRefs,omitempty"`
	Reason         string   `json:"reason"`
	IdempotencyKey string   `json:"-"`
}

// ProvisionConnectionCommand starts or replays one versioned connection Saga.
type ProvisionConnectionCommand struct {
	NodeCode          string `json:"-"`
	ConnectionVersion string `json:"connectionVersion"`
	DisplayName       string `json:"displayName"`
	RedirectURI       string `json:"redirectUri"`
	RotateSecret      bool   `json:"rotateSecret,omitempty"`
	Reason            string `json:"reason"`
	IdempotencyKey    string `json:"-"`
}

// ManagedSSOClientCommand is the narrow system-managed SSO facade input.
type ManagedSSOClientCommand struct {
	ClientID      string
	ClientName    string
	RedirectURI   string
	RotateSecret  bool
	OwnerNodeCode string
}

// ManagedSSOClientResult returns a one-time secret only when created or rotated.
type ManagedSSOClientResult struct {
	ClientID     string
	ClientSecret string
}

// ManagedSSOClientStatusCommand changes only the client owned by one Node.
type ManagedSSOClientStatusCommand struct {
	ClientID      string
	OwnerNodeCode string
	Status        int
}

// FederationStatus exposes safe local Saga state.
type FederationStatus struct {
	NodeCode              string `json:"nodeCode"`
	OIDCClientID          string `json:"oidcClientId,omitempty"`
	ConnectionStatus      string `json:"connectionStatus"`
	ConnectionVersion     string `json:"connectionVersion,omitempty"`
	LastConnectionError   string `json:"lastConnectionError,omitempty"`
	LastConnectionTraceID string `json:"lastConnectionTraceId,omitempty"`
}
