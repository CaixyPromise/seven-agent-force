package domain

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNodeCodeImmutable    = errors.New("nodeCode is immutable")
	ErrIssuerLocked         = errors.New("hub issuer is permanently locked")
	ErrProvisionReplay      = errors.New("connection version replay differs from the original request")
	ErrProvisionSuperseded  = errors.New("connection version has been superseded")
	ErrStaleProvisionResult = errors.New("connection provision result is stale")
)

const (
	NodeStatusEnabled  = 0
	NodeStatusDisabled = 1

	DiscoveryStatic = "STATIC"
	DiscoveryConsul = "CONSUL"

	ConnectionPending = "PENDING"
	ConnectionActive  = "ACTIVE"
	ConnectionError   = "ERROR"

	CommandPending    = "PENDING"
	CommandActive     = "ACTIVE"
	CommandError      = "ERROR"
	CommandSuperseded = "SUPERSEDED"
)

// EncryptedSecret is the persisted envelope representation of a secret.
type EncryptedSecret struct {
	Ciphertext string
	EDEK       string
	WrapKeyRef string
}

// Present reports whether all required envelope fields are populated.
func (s EncryptedSecret) Present() bool {
	return s.Ciphertext != "" && s.EDEK != "" && s.WrapKeyRef != ""
}

// Node is Hub-owned Node metadata. It never contains remote identity snapshots.
type Node struct {
	ID                    int64
	NodeCode              string
	NodeName              string
	Status                int
	DiscoveryType         string
	ServiceName           string
	ManagementBaseURL     string
	HubIssuer             string
	OIDCClientID          string
	OIDCClientSecret      EncryptedSecret
	ManagementBearer      EncryptedSecret
	CapabilitiesJSON      string
	ConnectionStatus      string
	ConnectionVersion     string
	ConnectionRequestHash string
	TargetRevision        int64
	IssuerLockedAt        *time.Time
	LastConnectionError   string
	LastConnectionTraceID string
	LastHealthyAt         *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// NodeMetadata contains administrator-owned descriptive and discovery fields.
type NodeMetadata struct {
	NodeCode          string
	NodeName          string
	DiscoveryType     string
	ServiceName       string
	ManagementBaseURL string
	HubIssuer         string
	CapabilitiesJSON  string
}

// ProvisionDecision describes the local work needed for one replayable connection step.
type ProvisionDecision struct {
	AlreadyActive      bool
	NewVersion         bool
	NeedsManagedClient bool
	RotateSecret       bool
	TargetRevision     int64
}

// ConnectionCommand is durable metadata for one accepted connection version.
type ConnectionCommand struct {
	NodeCode          string
	ConnectionVersion string
	RequestHash       string
	TargetRevision    int64
	State             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// EditMetadata applies metadata while preserving the Node's stable identity and issuer lock.
func (n *Node) EditMetadata(metadata NodeMetadata, now time.Time) error {
	if n == nil {
		return errors.New("node is nil")
	}
	if metadata.NodeCode != n.NodeCode {
		return ErrNodeCodeImmutable
	}
	if (n.IssuerLockedAt != nil || n.ConnectionStatus == ConnectionActive) && metadata.HubIssuer != n.HubIssuer {
		return ErrIssuerLocked
	}
	targetChanged := n.DiscoveryType != metadata.DiscoveryType || n.ServiceName != metadata.ServiceName || n.ManagementBaseURL != metadata.ManagementBaseURL
	n.NodeName = metadata.NodeName
	n.DiscoveryType = metadata.DiscoveryType
	n.ServiceName = metadata.ServiceName
	n.ManagementBaseURL = metadata.ManagementBaseURL
	n.HubIssuer = metadata.HubIssuer
	n.CapabilitiesJSON = metadata.CapabilitiesJSON
	if targetChanged {
		n.invalidateTarget(now, "Node路由目标已变更")
	}
	n.UpdatedAt = now
	return nil
}

// ReplaceManagementBearer records a newly encrypted management credential.
func (n *Node) ReplaceManagementBearer(secret EncryptedSecret, now time.Time) {
	n.ManagementBearer = secret
	n.invalidateTarget(now, "Node管理凭证已变更")
	n.UpdatedAt = now
}

// SetStatus updates only the administrator-owned enablement state.
func (n *Node) SetStatus(status int, now time.Time) {
	if n.Status != status {
		n.ensureTargetRevision()
		n.TargetRevision++
	}
	if n.Status != NodeStatusDisabled && status == NodeStatusDisabled {
		if n.ConnectionStatus == ConnectionPending {
			n.ConnectionStatus = ConnectionError
			n.LastConnectionError = "Node已禁用"
			n.LastConnectionTraceID = ""
		}
	}
	n.Status = status
	n.UpdatedAt = now
}

// RecordHealthy updates only remote health probe state.
func (n *Node) RecordHealthy(now time.Time) {
	n.LastHealthyAt = &now
	n.UpdatedAt = now
}

// Copy creates a disabled Node identity without carrying managed connection state.
func (n Node) Copy(id int64, nodeCode, nodeName string, bearer EncryptedSecret, now time.Time) Node {
	return Node{
		ID: id, NodeCode: nodeCode, NodeName: nodeName, Status: NodeStatusDisabled,
		DiscoveryType: n.DiscoveryType, ServiceName: n.ServiceName, ManagementBaseURL: n.ManagementBaseURL,
		HubIssuer: n.HubIssuer, ManagementBearer: bearer, CapabilitiesJSON: n.CapabilitiesJSON,
		ConnectionStatus: ConnectionPending, TargetRevision: 1, CreatedAt: now, UpdatedAt: now,
	}
}

// StartProvision applies the replay/rotation decision and enters PENDING.
func (n *Node) StartProvision(version, requestHash string, rotateRequested bool, now time.Time) (ProvisionDecision, error) {
	n.ensureTargetRevision()
	newVersion := n.ConnectionVersion != version
	if !newVersion && n.ConnectionRequestHash != "" && n.ConnectionRequestHash != requestHash {
		return ProvisionDecision{}, ErrProvisionReplay
	}
	if n.ConnectionStatus == ConnectionActive && !newVersion {
		return ProvisionDecision{AlreadyActive: true}, nil
	}
	if n.ConnectionStatus == ConnectionActive && n.IssuerLockedAt == nil {
		lockedAt := now
		n.IssuerLockedAt = &lockedAt
	}
	decision := ProvisionDecision{
		NewVersion:         newVersion,
		NeedsManagedClient: newVersion || !n.OIDCClientSecret.Present() || n.OIDCClientID == "",
		RotateSecret:       rotateRequested && newVersion,
		TargetRevision:     n.TargetRevision,
	}
	n.ConnectionStatus = ConnectionPending
	n.ConnectionVersion = version
	n.ConnectionRequestHash = requestHash
	n.LastConnectionError = ""
	n.LastConnectionTraceID = ""
	n.UpdatedAt = now
	return decision, nil
}

// CompleteProvisionForTarget activates only the same enabled routing/credential generation.
func (n *Node) CompleteProvisionForTarget(version, requestHash string, targetRevision int64, now time.Time) error {
	n.ensureTargetRevision()
	if n.Status != NodeStatusEnabled || n.TargetRevision != targetRevision {
		return ErrStaleProvisionResult
	}
	return n.CompleteProvision(version, requestHash, now)
}

// AcceptManagedClient stores the local one-time secret handoff before commit.
func (n *Node) AcceptManagedClient(clientID string, secret EncryptedSecret, now time.Time) {
	if clientID != "" {
		n.OIDCClientID = clientID
	}
	if secret.Present() {
		n.OIDCClientSecret = secret
	}
	n.UpdatedAt = now
}

// CompleteProvision activates only the matching current Saga result.
func (n *Node) CompleteProvision(version, requestHash string, now time.Time) error {
	if n.ConnectionVersion != version || n.ConnectionRequestHash != requestHash {
		return ErrStaleProvisionResult
	}
	n.ConnectionStatus = ConnectionActive
	n.LastConnectionError = ""
	n.LastConnectionTraceID = ""
	n.UpdatedAt = now
	if n.IssuerLockedAt == nil {
		lockedAt := now
		n.IssuerLockedAt = &lockedAt
	}
	return nil
}

// FailProvision records a sanitized failure only for the matching non-active Saga.
func (n *Node) FailProvision(version, requestHash, message, traceID string, now time.Time) bool {
	if n.ConnectionVersion != version || n.ConnectionRequestHash != requestHash || n.ConnectionStatus == ConnectionActive {
		return false
	}
	n.ConnectionStatus = ConnectionError
	n.LastConnectionError = message
	n.LastConnectionTraceID = traceID
	n.UpdatedAt = now
	return true
}

// FailProvisionForTarget records a failure only for the same routing/credential generation.
func (n *Node) FailProvisionForTarget(version, requestHash string, targetRevision int64, message, traceID string, now time.Time) bool {
	n.ensureTargetRevision()
	if n.TargetRevision != targetRevision {
		return false
	}
	return n.FailProvision(version, requestHash, message, traceID, now)
}

// EffectiveTargetRevision returns the persisted generation, backfilling legacy in-memory values.
func (n *Node) EffectiveTargetRevision() int64 {
	if n == nil {
		return 0
	}
	n.ensureTargetRevision()
	return n.TargetRevision
}

func (n *Node) ensureTargetRevision() {
	if n.TargetRevision < 1 {
		n.TargetRevision = 1
	}
}

func (n *Node) invalidateTarget(now time.Time, reason string) {
	n.ensureTargetRevision()
	n.TargetRevision++
	n.ConnectionStatus = ConnectionPending
	n.LastConnectionError = reason
	n.LastConnectionTraceID = ""
	n.UpdatedAt = now
}

// NodePageQuery defines bounded registry search.
type NodePageQuery struct {
	Current int
	Size    int
	Keyword string
	Status  *int
}

// Repository persists only Hub-owned Node metadata.
type Repository interface {
	Page(ctx context.Context, query NodePageQuery) ([]Node, int64, error)
	Find(ctx context.Context, nodeCode string) (*Node, error)
	FindForUpdate(ctx context.Context, nodeCode string) (*Node, error)
	Insert(ctx context.Context, node *Node) error
	UpdateMetadata(ctx context.Context, node *Node) error
	ReplaceManagementBearer(ctx context.Context, node *Node) error
	UpdateStatus(ctx context.Context, node *Node) error
	UpdateTargetState(ctx context.Context, node *Node) error
	UpdateHealth(ctx context.Context, node *Node) error
	UpdateConnection(ctx context.Context, node *Node) error
	FindConnectionCommandForUpdate(ctx context.Context, nodeCode, version string) (*ConnectionCommand, error)
	SaveConnectionCommand(ctx context.Context, command *ConnectionCommand) error
}

// SecretService protects Hub-owned plaintext without exposing infrastructure types.
type SecretService interface {
	Encrypt(ctx context.Context, plaintext string) (EncryptedSecret, error)
	Decrypt(ctx context.Context, value EncryptedSecret) (string, error)
}

// Transactor serializes local Hub and managed-SSO mutations on one datasource transaction.
type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}

// ValidateStaticManagementURL enforces the Hub's strict static discovery endpoint contract.
func ValidateStaticManagementURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Port() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errInvalidManagementURL
	}
	if address, parseErr := netip.ParseAddr(parsed.Hostname()); parseErr == nil &&
		(address.IsLoopback() || address.IsUnspecified() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsPrivate() || address.IsMulticast()) {
		return errInvalidManagementURL
	}
	return nil
}

var errInvalidManagementURL = &managementURLValidationError{}

type managementURLValidationError struct{}

func (*managementURLValidationError) Error() string { return "invalid static management URL" }
