package gittransport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

const (
	ProviderGitHub  = "GITHUB"
	ProviderGitLab  = "GITLAB"
	ProviderGitea   = "GITEA"
	ProviderGeneric = "GENERIC_GIT"
	ProviderLocal   = "LOCAL_TEST"

	TransportHTTP      = "HTTP"
	TransportHTTPS     = "HTTPS"
	TransportSSH       = "SSH"
	TransportLocalBare = "LOCAL_BARE"

	CapabilityReadSource   = "READ_SOURCE"
	CapabilityPushBranch   = "PUSH_BRANCH"
	CapabilityCreateReview = "CREATE_REVIEW"

	CapabilityUnverified = "UNVERIFIED"
	CapabilityVerified   = "VERIFIED"
	CapabilityFailed     = "FAILED"
	CapabilityRevoked    = "REVOKED"

	RepositoryEnabled = "ENABLED"

	SourceGitRepository     = "GIT_REPOSITORY"
	SourceRevisionAvailable = "AVAILABLE"
	RefBranch               = "BRANCH"
	RefTag                  = "TAG"
	RefCommit               = "COMMIT"
)

var (
	ErrInvalidRepositoryTarget = errors.New("PROJECT_REPOSITORY_SNAPSHOT_INVALID")
	ErrCapabilityUnavailable   = errors.New("REPOSITORY_CAPABILITY_UNAVAILABLE")
	ErrEgressDenied            = errors.New("REPOSITORY_EGRESS_DENIED")
	ErrInvalidSourceRevision   = errors.New("SOURCE_REVISION_INVALID")
)

type AccessBinding struct {
	Capability          string     `json:"capability"`
	CredentialRef       string     `json:"credentialRef,omitempty"`
	CredentialVersion   string     `json:"credentialVersion,omitempty"`
	Status              string     `json:"status"`
	LastProbeReasonCode string     `json:"lastProbeReasonCode,omitempty"`
	LastProbedAt        *time.Time `json:"lastProbedAt,omitempty"`
}

type EgressPolicy struct {
	AllowPrivateNetwork bool     `json:"allowPrivateNetwork"`
	AllowedHosts        []string `json:"allowedHosts"`
	AllowedCIDRs        []string `json:"allowedCidrs"`
	AllowedPorts        []int    `json:"allowedPorts"`
}

func (p EgressPolicy) Authorize(host string, port int, resolvedIPs []string) error {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || port < 1 || port > 65535 || len(resolvedIPs) == 0 || !containsFold(p.AllowedHosts, host) || !containsInt(p.AllowedPorts, port) {
		return ErrEgressDenied
	}
	networks := make([]*net.IPNet, 0, len(p.AllowedCIDRs))
	for _, raw := range p.AllowedCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			return ErrEgressDenied
		}
		networks = append(networks, network)
	}
	for _, raw := range resolvedIPs {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return ErrEgressDenied
		}
		private := ip.IsPrivate() || ip.IsLoopback()
		if private && !p.AllowPrivateNetwork {
			return ErrEgressDenied
		}
		matched := false
		for _, network := range networks {
			if network.Contains(ip) {
				matched = true
				break
			}
		}
		if (len(networks) > 0 || ip.IsLoopback()) && !matched {
			return ErrEgressDenied
		}
	}
	return nil
}

type RepositoryTarget struct {
	ID                      int64           `json:"id,string"`
	TenantID                int64           `json:"tenantId,string"`
	Name                    string          `json:"name"`
	ProviderType            string          `json:"providerType"`
	Transport               string          `json:"transport"`
	Host                    string          `json:"host,omitempty"`
	Port                    int             `json:"port,omitempty"`
	RepositoryPath          string          `json:"repositoryPath"`
	WebRepositoryURL        string          `json:"webRepositoryUrl,omitempty"`
	Read                    AccessBinding   `json:"read"`
	Push                    AccessBinding   `json:"push"`
	Review                  AccessBinding   `json:"review"`
	SSHKnownHostFingerprint string          `json:"sshKnownHostFingerprint,omitempty"`
	TLSPolicy               json.RawMessage `json:"tlsPolicy,omitempty"`
	Egress                  EgressPolicy    `json:"egress"`
	EgressPolicyDigest      string          `json:"egressPolicyDigest"`
	CommitAuthorName        string          `json:"commitAuthorName"`
	CommitAuthorEmail       string          `json:"commitAuthorEmail"`
	Status                  string          `json:"status"`
	ConfigDigest            string          `json:"configDigest"`
	DisabledAt              *time.Time      `json:"disabledAt,omitempty"`
	DisabledBy              int64           `json:"disabledBy,string,omitempty"`
	DisabledReason          string          `json:"disabledReason,omitempty"`
	DecommissionedAt        *time.Time      `json:"decommissionedAt,omitempty"`
	DecommissionedBy        int64           `json:"decommissionedBy,string,omitempty"`
	DecommissionReason      string          `json:"decommissionReason,omitempty"`
	LifecycleGeneration     int64           `json:"lifecycleGeneration,string"`
}

func ValidateRepositoryTarget(c RepositoryTarget, localMode bool) error {
	if c.ID <= 0 || c.TenantID <= 0 || strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.RepositoryPath) == "" {
		return ErrInvalidRepositoryTarget
	}
	if c.ProviderType != ProviderGitHub && c.ProviderType != ProviderGitLab && c.ProviderType != ProviderGitea && c.ProviderType != ProviderGeneric && !(localMode && c.ProviderType == ProviderLocal) {
		return ErrInvalidRepositoryTarget
	}
	switch c.Transport {
	case TransportHTTP, TransportHTTPS, TransportSSH:
		if strings.TrimSpace(c.Host) == "" || c.Port < 1 || c.Port > 65535 {
			return ErrInvalidRepositoryTarget
		}
	case TransportLocalBare:
		if !localMode || c.ProviderType != ProviderLocal || c.Host != "" {
			return ErrInvalidRepositoryTarget
		}
	default:
		return ErrInvalidRepositoryTarget
	}
	for _, binding := range []AccessBinding{c.Read, c.Push, c.Review} {
		if binding.Capability != CapabilityReadSource && binding.Capability != CapabilityPushBranch && binding.Capability != CapabilityCreateReview {
			return ErrInvalidRepositoryTarget
		}
		if binding.Status != CapabilityUnverified && binding.Status != CapabilityVerified && binding.Status != CapabilityFailed && binding.Status != CapabilityRevoked {
			return ErrInvalidRepositoryTarget
		}
		if binding.Status == CapabilityVerified && c.Transport != TransportLocalBare && (strings.TrimSpace(binding.CredentialRef) == "" || strings.TrimSpace(binding.CredentialVersion) == "") {
			return ErrInvalidRepositoryTarget
		}
	}
	if c.Read.Capability != CapabilityReadSource || c.Push.Capability != CapabilityPushBranch || c.Review.Capability != CapabilityCreateReview {
		return ErrInvalidRepositoryTarget
	}
	if strings.TrimSpace(c.CommitAuthorName) == "" || strings.TrimSpace(c.CommitAuthorEmail) == "" || strings.ContainsAny(c.CommitAuthorName+c.CommitAuthorEmail, "\r\n<>\x00") {
		return ErrInvalidRepositoryTarget
	}
	if c.Status != RepositoryEnabled {
		return ErrInvalidRepositoryTarget
	}
	return nil
}

func RequireCapability(c RepositoryTarget, capability string) error {
	if c.Status != RepositoryEnabled {
		return ErrCapabilityUnavailable
	}
	var binding AccessBinding
	switch capability {
	case CapabilityReadSource:
		binding = c.Read
	case CapabilityPushBranch:
		binding = c.Push
	case CapabilityCreateReview:
		binding = c.Review
	default:
		return ErrCapabilityUnavailable
	}
	if binding.Capability != capability || binding.Status != CapabilityVerified {
		return ErrCapabilityUnavailable
	}
	return nil
}

func ComputeConfigDigest(c RepositoryTarget) (string, error) {
	c.ConfigDigest = ""
	// Lifecycle metadata is audit state, not runtime configuration identity.
	c.Status = ""
	c.DisabledAt, c.DisabledBy, c.DisabledReason = nil, 0, ""
	c.DecommissionedAt, c.DecommissionedBy, c.DecommissionReason = nil, 0, ""
	c.LifecycleGeneration = 0
	for _, binding := range []*AccessBinding{&c.Read, &c.Push, &c.Review} {
		binding.Status = ""
		binding.LastProbeReasonCode = ""
		binding.LastProbedAt = nil
	}
	c.Egress.AllowedHosts = sortedLower(c.Egress.AllowedHosts)
	c.Egress.AllowedCIDRs = sorted(c.Egress.AllowedCIDRs)
	c.Egress.AllowedPorts = append([]int(nil), c.Egress.AllowedPorts...)
	sort.Ints(c.Egress.AllowedPorts)
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

type FreezeSourceRevisionInput struct {
	ID                                                                                     int64
	SourceID                                                                               int64
	TenantID                                                                               int64
	Kind                                                                                   string
	ProjectID                                                                              int64
	RefType                                                                                string
	RefName                                                                                string
	ResolvedCommit, ResolvedTree                                                           string
	RepositoryBundleContentSHA256, RepositoryBundleObjectSHA256, RepositoryBundleObjectKey string
	ResolvedAt                                                                             time.Time
}

type SourceRevision struct {
	ID                            int64     `json:"id,string"`
	SourceID                      int64     `json:"sourceId,string"`
	TenantID                      int64     `json:"tenantId,string"`
	Kind                          string    `json:"kind"`
	ProjectID                     int64     `json:"projectId,string,omitempty"`
	RefType                       string    `json:"refType,omitempty"`
	RefName                       string    `json:"refName,omitempty"`
	ResolvedCommit                string    `json:"resolvedCommit"`
	ResolvedTree                  string    `json:"resolvedTree"`
	RepositoryBundleContentSHA256 string    `json:"repositoryBundleContentSha256"`
	RepositoryBundleObjectSHA256  string    `json:"repositoryBundleObjectSha256"`
	RepositoryBundleObjectKey     string    `json:"-"`
	IdentityDigest                string    `json:"identityDigest"`
	Status                        string    `json:"status"`
	ResolvedAt                    time.Time `json:"resolvedAt"`
}

func FreezeSourceRevision(in FreezeSourceRevisionInput) (SourceRevision, error) {
	item := SourceRevision{ID: in.ID, SourceID: in.SourceID, TenantID: in.TenantID, Kind: in.Kind, ProjectID: in.ProjectID, RefType: in.RefType, RefName: in.RefName, ResolvedCommit: in.ResolvedCommit, ResolvedTree: in.ResolvedTree, RepositoryBundleContentSHA256: in.RepositoryBundleContentSHA256, RepositoryBundleObjectSHA256: in.RepositoryBundleObjectSHA256, RepositoryBundleObjectKey: in.RepositoryBundleObjectKey, Status: SourceRevisionAvailable, ResolvedAt: in.ResolvedAt.UTC()}
	digest, err := ComputeSourceIdentity(item)
	if err != nil {
		return SourceRevision{}, err
	}
	item.IdentityDigest = digest
	return item, nil
}

func ComputeSourceIdentity(item SourceRevision) (string, error) {
	gitSource := item.Kind == SourceGitRepository
	uploadedSource := item.Kind == "UPLOADED_ARCHIVE" || item.Kind == "UPLOADED_DIRECTORY"
	if item.ID <= 0 || item.SourceID <= 0 || item.TenantID <= 0 || !gitSource && !uploadedSource || gitSource && (item.RefType != RefBranch && item.RefType != RefTag && item.RefType != RefCommit || strings.TrimSpace(item.RefName) == "") || !hexLen(item.ResolvedCommit, 40) || !hexLen(item.ResolvedTree, 40) || !hexLen(item.RepositoryBundleContentSHA256, 64) || !hexLen(item.RepositoryBundleObjectSHA256, 64) || strings.TrimSpace(item.RepositoryBundleObjectKey) == "" {
		return "", ErrInvalidSourceRevision
	}
	body, _ := json.Marshal(struct {
		ID            int64
		Source        int64
		Tenant        int64
		Kind          string
		Connection    int64
		RefType       string
		RefName       string
		Commit        string
		Tree          string
		BundleContent string
		BundleObject  string
	}{item.ID, item.SourceID, item.TenantID, item.Kind, item.ProjectID, item.RefType, item.RefName, item.ResolvedCommit, item.ResolvedTree, item.RepositoryBundleContentSHA256, item.RepositoryBundleObjectSHA256})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func validBranch(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 255 && !strings.HasPrefix(value, "-") && !strings.Contains(value, "..") && !strings.ContainsAny(value, " ~^:?*[\\\x00\r\n") && !strings.HasSuffix(value, ".") && !strings.HasSuffix(value, "/") && !strings.Contains(value, "//")
}
func hexLen(value string, n int) bool {
	if len(value) != n {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func sorted(v []string) []string { out := append([]string(nil), v...); sort.Strings(out); return out }
func sortedLower(v []string) []string {
	out := make([]string, 0, len(v))
	for _, x := range v {
		out = append(out, strings.ToLower(strings.TrimSpace(x)))
	}
	sort.Strings(out)
	return out
}
func containsFold(v []string, target string) bool {
	for _, x := range v {
		if strings.EqualFold(strings.TrimSpace(x), target) {
			return true
		}
	}
	return false
}
func containsInt(v []int, target int) bool {
	for _, x := range v {
		if x == target {
			return true
		}
	}
	return false
}

func (c RepositoryTarget) RemoteURL() (string, error) {
	switch c.Transport {
	case TransportHTTP:
		return fmt.Sprintf("http://%s:%d/%s", c.Host, c.Port, strings.TrimPrefix(c.RepositoryPath, "/")), nil
	case TransportHTTPS:
		return fmt.Sprintf("https://%s:%d/%s", c.Host, c.Port, strings.TrimPrefix(c.RepositoryPath, "/")), nil
	case TransportSSH:
		return fmt.Sprintf("ssh://git@%s:%d/%s", c.Host, c.Port, strings.TrimPrefix(c.RepositoryPath, "/")), nil
	case TransportLocalBare:
		return c.RepositoryPath, nil
	default:
		return "", ErrInvalidRepositoryTarget
	}
}
