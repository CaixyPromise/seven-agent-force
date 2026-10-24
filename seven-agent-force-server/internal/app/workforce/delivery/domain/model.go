// Package domain owns the immutable Delivery target, request and remote-ref
// arbitration contracts. Task success is deliberately outside this package.
package domain

import (
	"crypto/sha1" // Git SHA-1 object format is the only admitted format in P0.9.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	gittransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
)

const (
	MessageKind          = "seven.delivery-attempt"
	ActionGitPush        = "GIT_PUSH"
	ActionCustomDelivery = "CUSTOM_DELIVERY"
	TransportHTTP        = gittransport.TransportHTTP
	TransportHTTPS       = gittransport.TransportHTTPS
	TransportSSH         = gittransport.TransportSSH
	TransportLocalBare   = gittransport.TransportLocalBare
	ApprovalAfter        = "AFTER_APPROVAL"
	ApprovalOnPass       = "ON_PASS"
	ApprovalManual       = "MANUAL_ONLY"
)

var (
	ErrInvalidTarget         = errors.New("DELIVERY_TARGET_INVALID")
	ErrPolicyDenied          = errors.New("DELIVERY_POLICY_DENIED")
	ErrApprovalRequired      = errors.New("DELIVERY_APPROVAL_REQUIRED")
	ErrIdentityConflict      = errors.New("DELIVERY_IDENTITY_CONFLICT")
	ErrRemoteChanged         = errors.New("REMOTE_REF_CHANGED")
	ErrBaseChanged           = errors.New("REMOTE_BASE_CHANGED")
	ErrBranchCreateForbidden = errors.New("BRANCH_CREATE_FORBIDDEN")
)

type Target struct {
	ID                      int64           `json:"id,string"`
	TenantID                int64           `json:"tenantId,string"`
	Name                    string          `json:"name"`
	ProviderType            string          `json:"providerType,omitempty"`
	Transport               string          `json:"transport"`
	Host                    string          `json:"host,omitempty"`
	Port                    int             `json:"port,omitempty"`
	RepositoryPath          string          `json:"repositoryPath"`
	WebRepositoryURL        string          `json:"webRepositoryUrl,omitempty"`
	CredentialRef           string          `json:"credentialRef,omitempty"`
	CredentialVersion       string          `json:"credentialVersion,omitempty"`
	SSHKnownHostFingerprint string          `json:"sshKnownHostFingerprint,omitempty"`
	TLSPolicy               json.RawMessage `json:"tlsPolicy,omitempty"`
	Egress                  json.RawMessage `json:"egress,omitempty"`
	EgressPolicyDigest      string          `json:"egressPolicyDigest"`
	CommitAuthorName        string          `json:"commitAuthorName"`
	CommitAuthorEmail       string          `json:"commitAuthorEmail"`
	Status                  string          `json:"status"`
	ConfigDigest            string          `json:"configDigest"`
	LastProbeStatus         string          `json:"lastProbeStatus,omitempty"`
	LastProbeReasonCode     string          `json:"lastProbeReasonCode,omitempty"`
	LastProbedAt            string          `json:"lastProbedAt,omitempty"`
}

// EgressPolicy is intentionally duplicated at the Delivery boundary. The
// Connection owns authoring and storage, while a frozen Delivery request owns
// the exact JSON snapshot that the Worker must enforce before making a Git
// side effect. Keeping this small value object here avoids letting the Worker
// reconstruct a policy from mutable Connection state.
type EgressPolicy struct {
	AllowPrivateNetwork bool     `json:"allowPrivateNetwork"`
	AllowedHosts        []string `json:"allowedHosts"`
	AllowedCIDRs        []string `json:"allowedCidrs"`
	AllowedPorts        []int    `json:"allowedPorts"`
}

func (p EgressPolicy) Authorize(host string, port int, resolvedIPs []string) error {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || port < 1 || port > 65535 || len(resolvedIPs) == 0 || !containsFold(p.AllowedHosts, host) || !containsInt(p.AllowedPorts, port) {
		return ErrPolicyDenied
	}
	networks := make([]*net.IPNet, 0, len(p.AllowedCIDRs))
	for _, raw := range p.AllowedCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			return ErrPolicyDenied
		}
		networks = append(networks, network)
	}
	for _, raw := range resolvedIPs {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return ErrPolicyDenied
		}
		if (ip.IsPrivate() || ip.IsLoopback()) && !p.AllowPrivateNetwork {
			return ErrPolicyDenied
		}
		matched := false
		for _, network := range networks {
			if network.Contains(ip) {
				matched = true
				break
			}
		}
		if (len(networks) > 0 || ip.IsLoopback()) && !matched {
			return ErrPolicyDenied
		}
	}
	return nil
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(value), "."), want) {
			return true
		}
	}
	return false
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type Action struct {
	Type       string          `json:"type"`
	When       string          `json:"when"`
	TargetID   int64           `json:"targetId,string"`
	BaseBranch string          `json:"baseBranch,omitempty"`
	Branch     string          `json:"branch,omitempty"`
	Operation  string          `json:"operation,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

// FrozenGitPlan is created with the TaskTurn and is the sole authority for a
// Git side effect. Processor output may opt into delivery, but cannot choose a
// repository, base, branch, or parent commit at evaluation time.
type FrozenGitPlan struct {
	TargetID               int64
	BaseBranch             string
	BaseCommit             string
	DestinationBranch      string
	DeliveryMode           string
	PreviousDeliveryCommit string
}

func BindActionToGitPlan(action Action, plan FrozenGitPlan) (Action, string, error) {
	if plan.DeliveryMode != "NEW_BRANCH" && plan.DeliveryMode != "NEW_BRANCH_AND_DRAFT_REVIEW" {
		return Action{}, "", ErrPolicyDenied
	}
	if action.Type != ActionGitPush || plan.TargetID <= 0 || !validRef(plan.BaseBranch) || !validRef(plan.DestinationBranch) || !hexObject(plan.BaseCommit) {
		return Action{}, "", ErrPolicyDenied
	}
	if (action.TargetID != 0 && action.TargetID != plan.TargetID) || (action.BaseBranch != "" && action.BaseBranch != plan.BaseBranch) || (action.Branch != "" && action.Branch != plan.DestinationBranch) {
		return Action{}, "", ErrIdentityConflict
	}
	action.TargetID = plan.TargetID
	action.BaseBranch = plan.BaseBranch
	action.Branch = plan.DestinationBranch
	expected := plan.BaseCommit
	if plan.PreviousDeliveryCommit != "" {
		if !hexObject(plan.PreviousDeliveryCommit) {
			return Action{}, "", ErrIdentityConflict
		}
		expected = plan.PreviousDeliveryCommit
	}
	return action, expected, nil
}

type RequestInput struct {
	TenantID            int64
	TaskID              int64
	TaskTurnID          int64
	CheckpointID        int64
	ProcessorReleaseID  int64
	ProcessorDigest     string
	ResultCommit        string
	ResultTree          string
	SelectionDigest     string
	ExpectedBaseCommit  string
	Action              Action
	Target              Target
	TargetConfigDigest  string
	CredentialVersion   string
	AuthorizationDigest string
	Now                 time.Time
}

type PreparedRequest struct {
	ActionDigest        string
	TargetConfigDigest  string
	PolicyDigest        string
	CredentialVersion   string
	AuthorizationDigest string
	BaseBranch          string
	DestinationBranch   string
	ExpectedBaseCommit  string
	DeliveryCommit      string
	CommitTimestamp     time.Time
	ApprovalMode        string
	InitialStatus       string
}

type Message struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	MessageID     int64  `json:"messageId,string"`
	WorkerID      int64  `json:"workerId,string"`
	ResourceID    int64  `json:"resourceId,string"`
	Generation    int64  `json:"generation,string"`
}

type RemoteDecision struct {
	Status, ReasonCode string
	SafeToRetry        bool
}

func ValidateTarget(target Target, localMode bool) error {
	if target.ID <= 0 || target.TenantID <= 0 || strings.TrimSpace(target.Name) == "" || strings.TrimSpace(target.RepositoryPath) == "" {
		return ErrInvalidTarget
	}
	switch target.Transport {
	case TransportHTTP, TransportHTTPS, TransportSSH:
		if strings.TrimSpace(target.Host) == "" || target.Port < 1 || target.Port > 65535 {
			return ErrInvalidTarget
		}
	case TransportLocalBare:
		if !localMode || target.Host != "" || target.CredentialRef != "" {
			return ErrInvalidTarget
		}
	default:
		return ErrInvalidTarget
	}
	if strings.TrimSpace(target.CommitAuthorName) == "" || strings.TrimSpace(target.CommitAuthorEmail) == "" || strings.ContainsAny(target.CommitAuthorName+target.CommitAuthorEmail, "\r\n<>\x00") {
		return ErrInvalidTarget
	}
	return nil
}

// AuthorizationDigest binds a frozen delivery operation to one specific
// verified capability binding.  It intentionally includes the Connection
// digest as well: a credential alone is not authority to contact a different
// repository, endpoint, TLS pin, or egress policy after a connection edit.
// The clear-text credential is never part of this value.
func AuthorizationDigest(connectionDigest, capability, credentialRef, credentialVersion string) string {
	value := strings.Join([]string{
		strings.TrimSpace(connectionDigest),
		strings.TrimSpace(capability),
		strings.TrimSpace(credentialRef),
		strings.TrimSpace(credentialVersion),
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func ComputeTargetDigest(target Target) (string, error) {
	target.ConfigDigest = ""
	target.Status, target.LastProbeStatus, target.LastProbeReasonCode, target.LastProbedAt = "", "", "", ""
	body, err := json.Marshal(target)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// ComputeTargetIdentityDigest freezes the target properties that make a
// delivery mean "the same remote side effect". Credential references and
// versions deliberately do not participate: they are rotated through the
// explicit reauthorization flow. Everything else that could redirect or
// broaden a push (repository, transport, trust, egress policy, or bot
// identity) remains part of this identity. Branches live in TaskGitPlan.
//
// This is intentionally separate from ConfigDigest. ConfigDigest includes the
// live access bindings, so using it alone would make a safe credential rotation
// indistinguishable from a repository/policy replacement.
func ComputeTargetIdentityDigest(target Target) (string, error) {
	target.ConfigDigest = ""
	target.CredentialRef = ""
	target.CredentialVersion = ""
	target.LastProbeStatus = ""
	target.LastProbeReasonCode = ""
	target.LastProbedAt = ""
	target.Status = ""
	body, err := json.Marshal(target)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func Prepare(input RequestInput) (PreparedRequest, error) {
	if err := ValidateTarget(input.Target, input.Target.Transport == TransportLocalBare); err != nil {
		return PreparedRequest{}, err
	}
	if input.Target.Status != "ENABLED" || input.Target.ID != input.Action.TargetID || input.Target.ConfigDigest != input.TargetConfigDigest || len(input.AuthorizationDigest) != 64 || strings.TrimSpace(input.CredentialVersion) == "" || len(input.ResultTree) != 40 || len(input.ExpectedBaseCommit) != 40 || (input.Action.Type != ActionGitPush && input.Action.Type != ActionCustomDelivery) {
		return PreparedRequest{}, ErrPolicyDenied
	}
	if input.Action.Type == ActionGitPush {
		if !validRef(input.Action.BaseBranch) || !validRef(input.Action.Branch) || input.Action.BaseBranch == input.Action.Branch {
			return PreparedRequest{}, ErrPolicyDenied
		}
	} else if strings.TrimSpace(input.Action.Operation) == "" || len(input.Action.Operation) > 128 || len(input.Action.Payload) > 64<<10 || (len(input.Action.Payload) > 0 && !json.Valid(input.Action.Payload)) {
		return PreparedRequest{}, ErrPolicyDenied
	}
	when := input.Action.When
	if when == "" {
		when = ApprovalAfter
	}
	if when != ApprovalAfter && when != ApprovalManual && when != ApprovalOnPass {
		return PreparedRequest{}, ErrPolicyDenied
	}
	if when == ApprovalOnPass {
		return PreparedRequest{}, ErrPolicyDenied
	}
	timestamp := input.Now.UTC().Truncate(time.Second)
	actionBody, _ := json.Marshal(input.Action)
	actionSum := sha256.Sum256(actionBody)
	baseBranch, destinationBranch, commit := input.Action.BaseBranch, input.Action.Branch, ""
	if input.Action.Type == ActionGitPush {
		message := fmt.Sprintf("Seven Agent Force result\n\nTask: %d\nTurn: %d\nResult: %s\n", input.TaskID, input.TaskTurnID, input.ResultCommit)
		var err error
		commit, err = ComputeDeliveryCommit(input.ResultTree, input.ExpectedBaseCommit, input.Target.CommitAuthorName, input.Target.CommitAuthorEmail, timestamp, message)
		if err != nil {
			return PreparedRequest{}, err
		}
	} else {
		baseBranch = input.Action.BaseBranch
		destinationBranch = "custom/" + hex.EncodeToString(actionSum[:16])
	}
	policyBody, _ := json.Marshal(map[string]any{"target": input.Target.ConfigDigest, "approval": when, "branch": input.Action.Branch, "processor": input.ProcessorDigest})
	policySum := sha256.Sum256(policyBody)
	status := "WAITING_APPROVAL"
	if when == ApprovalManual {
		status = "MANUAL_ONLY"
	}
	return PreparedRequest{ActionDigest: hex.EncodeToString(actionSum[:]), TargetConfigDigest: input.TargetConfigDigest, PolicyDigest: hex.EncodeToString(policySum[:]), CredentialVersion: input.CredentialVersion, AuthorizationDigest: input.AuthorizationDigest, BaseBranch: baseBranch, DestinationBranch: destinationBranch, ExpectedBaseCommit: input.ExpectedBaseCommit, DeliveryCommit: commit, CommitTimestamp: timestamp, ApprovalMode: when, InitialStatus: status}, nil
}

func ComputeDeliveryCommit(tree, parent, name, email string, timestamp time.Time, message string) (string, error) {
	if !hexObject(tree) || !hexObject(parent) || strings.ContainsAny(name+email, "\r\n<>\x00") {
		return "", ErrIdentityConflict
	}
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	seconds := timestamp.UTC().Unix()
	body := fmt.Sprintf("tree %s\nparent %s\nauthor %s <%s> %d +0000\ncommitter %s <%s> %d +0000\n\n%s", tree, parent, name, email, seconds, name, email, seconds, message)
	header := fmt.Sprintf("commit %d\x00", len(body))
	sum := sha1.Sum([]byte(header + body))
	return hex.EncodeToString(sum[:]), nil
}

func Preflight(target Target, base, destination, expectedBase, deliveryCommit string) (RemoteDecision, error) {
	if destination == deliveryCommit {
		return RemoteDecision{Status: "SUCCEEDED", ReasonCode: "REMOTE_ALREADY_DELIVERED"}, nil
	}
	if base != expectedBase {
		return RemoteDecision{}, ErrBaseChanged
	}
	if destination != "" && destination != expectedBase {
		return RemoteDecision{}, ErrRemoteChanged
	}
	return RemoteDecision{Status: "PUSH_AUTHORIZED"}, nil
}

func ReconcileAfter(expectedBase, deliveryCommit, remoteAfter string, readable bool) RemoteDecision {
	if !readable {
		return RemoteDecision{Status: "OUTCOME_UNKNOWN", ReasonCode: "REMOTE_REF_UNAVAILABLE"}
	}
	if remoteAfter == deliveryCommit {
		return RemoteDecision{Status: "SUCCEEDED", ReasonCode: "REMOTE_REF_VERIFIED"}
	}
	if remoteAfter == "" || remoteAfter == expectedBase {
		return RemoteDecision{Status: "RETRY_WAIT", ReasonCode: "REMOTE_UNCHANGED", SafeToRetry: true}
	}
	return RemoteDecision{Status: "MANUAL_REVIEW", ReasonCode: "REMOTE_REF_UNEXPECTED"}
}

// An expired prepared push has an unknown outcome. Recovery must fence the
// old owner and observe the remote before any further push can be authorized.
func CanReconcileExpiredPreparedPush(action, requestStatus, attemptStatus, phase string, expires *time.Time, now time.Time) bool {
	return action == ActionGitPush && requestStatus == "CONFIRMING" && attemptStatus == "RUNNING" && phase == "PREPARED" && expires != nil && expires.Before(now)
}

// Explicit operator reauthorization opens a new bounded observation epoch for
// the same frozen result. It neither replaces credentials nor authorizes push.
func CanReauthorizeExhaustedPush(action, status, reason string, epochAttempts int, active bool) bool {
	return action == ActionGitPush && epochAttempts >= 3 && !active &&
		((status == "MANUAL_REVIEW" && reason == "DELIVERY_READBACK_RETRY_EXHAUSTED") ||
			(status == "FAILED" && reason == "DELIVERY_RETRY_EXHAUSTED"))
}

func validRef(value string) bool {
	return value != "" && len(value) <= 255 && !strings.ContainsAny(value, " ~^:?*[\\\x00") && !strings.Contains(value, "..") && !strings.Contains(value, "@{") && !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/") && !strings.HasSuffix(value, ".lock")
}
func hexObject(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
