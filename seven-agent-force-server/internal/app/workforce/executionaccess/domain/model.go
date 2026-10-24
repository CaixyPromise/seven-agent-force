// Package domain owns the controlled execution-access state machines.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

var (
	ErrInvalid             = errors.New("EXECUTION_OPERATION_INVALID")
	ErrNotFound            = errors.New("EXECUTION_OPERATION_NOT_FOUND")
	ErrStale               = errors.New("EXECUTION_OPERATION_STALE")
	ErrActionNotAllowed    = errors.New("EXECUTION_OPERATION_ACTION_NOT_ALLOWED")
	ErrIdempotencyConflict = errors.New("EXECUTION_OPERATION_IDEMPOTENCY_CONFLICT")
	ErrTargetDenied        = errors.New("EXECUTION_OPERATION_TARGET_DENIED")
	ErrOutcomeUnknown      = errors.New("EXECUTION_OPERATION_OUTCOME_UNKNOWN")
	ErrGrantRevoked        = errors.New("TASK_NETWORK_GRANT_REVOKED")
	ErrGrantExpired        = errors.New("TASK_NETWORK_GRANT_EXPIRED")
	ErrFenced              = errors.New("EXECUTION_OPERATION_FENCED")
	ErrUnavailable         = errors.New("EXECUTION_OPERATION_UNAVAILABLE")
)

const (
	KindNetworkCommand = "NETWORK_COMMAND"

	StatusWaitingApproval = "WAITING_APPROVAL"
	StatusReady           = "READY"
	StatusDispatched      = "DISPATCHED"
	StatusRunning         = "RUNNING"
	StatusSucceeded       = "SUCCEEDED"
	StatusFailed          = "FAILED"
	StatusOutcomeUnknown  = "OUTCOME_UNKNOWN"
	StatusDenied          = "DENIED"
	StatusExpired         = "EXPIRED"
	StatusCancelled       = "CANCELLED"

	DecisionAllowOnce    = "ALLOW_ONCE"
	DecisionAllowForTask = "ALLOW_FOR_TASK"
	DecisionDeny         = "DENY"

	GrantActive  = "ACTIVE"
	GrantRevoked = "REVOKED"
	GrantExpired = "EXPIRED"

	DeliverySaved          = "SAVED"
	DeliveryAdapter        = "DELIVERED_TO_ADAPTER"
	DeliveryNativeResolved = "NATIVE_CALL_RESOLVED"

	AttemptDispatched = "DISPATCHED"
	AttemptRunning    = "RUNNING"
	AttemptSucceeded  = "SUCCEEDED"
	AttemptFailed     = "FAILED"
	AttemptUnknown    = "OUTCOME_UNKNOWN"

	MaxPurposeBytes          = 2 << 10
	MaxCommandBytes          = 16 << 10
	MaxCwdBytes              = 4 << 10
	MaxTargets               = 8
	MaxReasonBytes           = 2 << 10
	MaxSummaryBytes          = 2 << 10
	MaxOutputBytes           = 64 << 10
	MaxNativeToolCallIDBytes = 512
)

var (
	hostLabel               = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	keyPattern              = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	nativeToolCallIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,511}$`)
	// Purpose is copied into the ordinary task conversation. Keep that safe
	// projection from becoming a credential transport while leaving the full
	// command encrypted behind the task-authorized detail endpoint.
	secretAssignmentText = regexp.MustCompile(`(?i)\b(?:[a-z0-9]+[_-])*(?:api[_-]?key|token|password|secret(?:[_-][a-z0-9]+){0,3}|authorization)\b\s*[:=]\s*(?:bearer\s+)?[^\s,;]+`)
	credentialURLText    = regexp.MustCompile(`(?i)(https?://)[^/\s:@]+:[^/\s@]+@`)
	secretTokenText      = regexp.MustCompile(`\b(?:sk|pk|ghp|github_pat|xox[baprs]|AKIA)[-_A-Za-z0-9]{16,}\b`)
	longOpaqueText       = regexp.MustCompile(`[A-Za-z0-9+/=_-]{128,}`)
)

type Target struct {
	Host     string `json:"host"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

func (t Target) Key() string { return t.Protocol + "://" + t.Host + ":" + itoa(t.Port) }

type Descriptor struct {
	Purpose string   `json:"purpose"`
	Command string   `json:"command"`
	Cwd     string   `json:"cwd"`
	Targets []Target `json:"targets"`
}

type EncryptedDescriptor struct {
	Ciphertext string
	EDEK       string
	KeyRef     string
}

type Operation struct {
	ID, TenantID, TaskID, TaskTurnID, AttemptID  int64
	AuthorizationGrantID                         int64
	AdapterSessionGeneration                     int64
	LocalToolCallKey, NativeToolCallID, RoundKey string
	Kind, DescriptorDigest                       string
	Descriptor                                   EncryptedDescriptor
	ResultOutput                                 EncryptedDescriptor
	Purpose                                      string
	Targets                                      []Target
	Status                                       string
	Version                                      int64
	ResultDeliveryStatus                         string
	AgentContinued                               bool
	ResultDigest, ResultSummary                  string
	ExitCode                                     *int
	ExpiresAt, CreatedAt, UpdatedAt              time.Time
	StartedAt, FinishedAt                        *time.Time
}

type Decision struct {
	ID, TenantID, TaskID, OperationID, ActorPrincipalID int64
	Action, IdempotencyKey, RequestDigest, Reason       string
	Status                                              string
	CreatedAt                                           time.Time
}

type Grant struct {
	ID, TenantID, TaskID, SourceDecisionID, ActorPrincipalID int64
	Targets                                                  []Target
	TargetsDigest, Status                                    string
	RevokeIdempotencyKey, RevokeRequestDigest                string
	Version                                                  int64
	ExpiresAt, CreatedAt, UpdatedAt                          time.Time
	RevokedAt                                                *time.Time
}

// GrantAudit is the safe, task-scoped read model used by the operator UI. It
// adds the human decision and latest operation outcome without exposing the
// encrypted command, worker identity, relay identity, or execution lease.
type GrantAudit struct {
	Grant
	ActorUserID       int64
	SourceOperationID int64
	Purpose           string
	LastOperation     *GrantOperationAudit
}

type GrantOperationAudit struct {
	Status, ResultSummary, ResultDeliveryStatus string
	ExitCode                                    *int
	AgentContinued                              bool
	FinishedAt                                  *time.Time
}

type Attempt struct {
	ID, TenantID, TaskID, OperationID, WorkerID, OwnerEpoch int64
	AttemptNumber                                           int
	WorkerInstanceID, HostBootID, LeaseDigest               string
	Status, ContainerID, RelayIdentity                      string
	ResultDigest                                            string
	ExitCode                                                *int
	LeaseExpiresAt, CreatedAt, UpdatedAt                    time.Time
	StartedAt, FinishedAt                                   *time.Time
}

func ValidKey(v string) bool { return keyPattern.MatchString(v) }

func ValidNativeToolCallID(v string) bool {
	return len(v) <= MaxNativeToolCallIDBytes && nativeToolCallIDPattern.MatchString(v)
}

func ValidDigest(v string) bool { return isHexDigest(v) }

func NormalizeDescriptor(in Descriptor) (Descriptor, error) {
	var out Descriptor
	out.Purpose = strings.TrimSpace(in.Purpose)
	out.Command = strings.TrimSpace(in.Command)
	out.Cwd = strings.TrimSpace(in.Cwd)
	if out.Cwd == "" {
		out.Cwd = "."
	}
	if !validText(out.Purpose, MaxPurposeBytes, false) || containsSensitiveProjection(out.Purpose) || !validText(out.Command, MaxCommandBytes, true) {
		return Descriptor{}, ErrInvalid
	}
	clean := path.Clean(strings.ReplaceAll(out.Cwd, `\`, "/"))
	if !validText(out.Cwd, MaxCwdBytes, false) || clean == "/" || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return Descriptor{}, ErrInvalid
	}
	out.Cwd = clean
	if len(in.Targets) == 0 || len(in.Targets) > MaxTargets {
		return Descriptor{}, ErrInvalid
	}
	seen := map[string]bool{}
	seenHost := map[string]bool{}
	for _, raw := range in.Targets {
		t, err := NormalizeTarget(raw)
		if err != nil {
			return Descriptor{}, err
		}
		if seen[t.Key()] {
			return Descriptor{}, ErrInvalid
		}
		if seenHost[t.Host] {
			// The first implementation maps one approved hostname to one fixed
			// relay. Multiple ports for the same hostname would make container
			// name resolution nondeterministic, so fail closed until a grouped
			// multi-port relay exists.
			return Descriptor{}, ErrInvalid
		}
		seen[t.Key()] = true
		seenHost[t.Host] = true
		out.Targets = append(out.Targets, t)
	}
	sort.Slice(out.Targets, func(i, j int) bool { return out.Targets[i].Key() < out.Targets[j].Key() })
	return out, nil
}

func containsSensitiveProjection(value string) bool {
	return secretAssignmentText.MatchString(value) || credentialURLText.MatchString(value) || secretTokenText.MatchString(value) || longOpaqueText.MatchString(value)
}

func NormalizeTarget(in Target) (Target, error) {
	protocol := strings.ToLower(strings.TrimSpace(in.Protocol))
	if protocol != "http" && protocol != "https" || in.Port < 1 || in.Port > 65535 {
		return Target{}, ErrInvalid
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Host)), ".")
	if host == "" || strings.ContainsAny(host, "*/:@/?#[]\\") || net.ParseIP(host) != nil {
		return Target{}, ErrTargetDenied
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || len(ascii) > 253 || ascii == "localhost" || !strings.Contains(ascii, ".") {
		return Target{}, ErrTargetDenied
	}
	for _, label := range strings.Split(ascii, ".") {
		if !hostLabel.MatchString(label) {
			return Target{}, ErrTargetDenied
		}
	}
	return Target{Host: ascii, Protocol: protocol, Port: in.Port}, nil
}

func NormalizeReason(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !validText(value, MaxReasonBytes, false) {
		return "", ErrInvalid
	}
	return value, nil
}

func ValidateDecision(action string) bool {
	return action == DecisionAllowOnce || action == DecisionAllowForTask || action == DecisionDeny
}

func Digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (o *Operation) ApplyDecision(expected int64, action string, now time.Time) error {
	if o == nil || expected != o.Version {
		return ErrStale
	}
	if o.Status != StatusWaitingApproval || !ValidateDecision(action) || !now.Before(o.ExpiresAt) {
		return ErrActionNotAllowed
	}
	if action == DecisionDeny {
		o.Status = StatusDenied
		o.ResultDeliveryStatus = DeliverySaved
		o.FinishedAt = ptrTime(now)
	} else {
		o.Status = StatusReady
	}
	o.Version++
	o.UpdatedAt = now
	return nil
}

func (o *Operation) Dispatch(now time.Time) error {
	if o.Status != StatusReady || !now.Before(o.ExpiresAt) {
		return ErrActionNotAllowed
	}
	o.Status, o.UpdatedAt = StatusDispatched, now
	o.Version++
	return nil
}

func (o *Operation) Start(now time.Time) error {
	if o.Status != StatusDispatched {
		return ErrActionNotAllowed
	}
	o.Status, o.UpdatedAt = StatusRunning, now
	o.StartedAt = ptrTime(now)
	o.Version++
	return nil
}

func (o *Operation) Finish(status, digest, summary string, exitCode *int, now time.Time) error {
	if o.Status != StatusRunning && o.Status != StatusDispatched {
		return ErrActionNotAllowed
	}
	if status != StatusSucceeded && status != StatusFailed && status != StatusOutcomeUnknown {
		return ErrInvalid
	}
	if !isHexDigest(digest) || !validText(strings.TrimSpace(summary), MaxSummaryBytes, true) {
		return ErrInvalid
	}
	o.Status, o.ResultDigest, o.ResultSummary, o.ExitCode = status, digest, strings.TrimSpace(summary), exitCode
	o.ResultDeliveryStatus, o.UpdatedAt, o.FinishedAt = DeliverySaved, now, ptrTime(now)
	o.Version++
	return nil
}

func NormalizeOutput(value string) (string, error) {
	if len(value) > MaxOutputBytes || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return "", ErrInvalid
	}
	return value, nil
}

func ResultDigest(status string, exitCode *int, summary, output string) string {
	return Digest(struct {
		Status   string `json:"status"`
		ExitCode *int   `json:"exitCode"`
		Summary  string `json:"summary"`
		Output   string `json:"output"`
	}{status, exitCode, summary, output})
}

func (o *Operation) Close(nativeClosed bool, now time.Time) error {
	switch o.Status {
	case StatusWaitingApproval, StatusReady, StatusDispatched, StatusRunning:
		delivery := DeliverySaved
		if nativeClosed {
			delivery = DeliveryNativeResolved
		}
		o.Status, o.ResultDeliveryStatus, o.UpdatedAt, o.FinishedAt = StatusCancelled, delivery, now, ptrTime(now)
		o.Version++
		return nil
	case StatusCancelled, StatusDenied, StatusExpired, StatusSucceeded, StatusFailed, StatusOutcomeUnknown:
		return nil
	default:
		return ErrActionNotAllowed
	}
}

func (o *Operation) MarkDelivered(native, continued bool, now time.Time) error {
	if o.Status != StatusSucceeded && o.Status != StatusFailed && o.Status != StatusOutcomeUnknown && o.Status != StatusDenied && o.Status != StatusCancelled && o.Status != StatusExpired {
		return ErrActionNotAllowed
	}
	if continued && !native {
		return ErrInvalid
	}
	want := DeliveryAdapter
	if native {
		if o.ResultDeliveryStatus == DeliveryNativeResolved {
			if continued && !o.AgentContinued {
				o.AgentContinued, o.UpdatedAt = true, now
				o.Version++
			}
			return nil
		}
		if o.ResultDeliveryStatus != DeliveryAdapter {
			return ErrStale
		}
		want = DeliveryNativeResolved
	} else if o.ResultDeliveryStatus != DeliverySaved {
		if o.ResultDeliveryStatus == DeliveryAdapter || o.ResultDeliveryStatus == DeliveryNativeResolved {
			return nil
		}
		return ErrStale
	}
	o.ResultDeliveryStatus, o.UpdatedAt = want, now
	if continued {
		o.AgentContinued = true
	}
	o.Version++
	return nil
}

func (g *Grant) Refresh(now time.Time) {
	if g.Status == GrantActive && !now.Before(g.ExpiresAt) {
		g.Status, g.UpdatedAt = GrantExpired, now
		g.Version++
	}
}

func (g Grant) Covers(requested []Target, now time.Time) bool {
	if g.Status != GrantActive || !now.Before(g.ExpiresAt) {
		return false
	}
	allowed := map[string]bool{}
	for _, t := range g.Targets {
		allowed[t.Key()] = true
	}
	for _, t := range requested {
		if !allowed[t.Key()] {
			return false
		}
	}
	return true
}

func (g *Grant) Revoke(expected int64, now time.Time) error {
	if g.Version != expected {
		return ErrStale
	}
	g.Refresh(now)
	if g.Status == GrantExpired {
		return ErrGrantExpired
	}
	if g.Status != GrantActive {
		return ErrGrantRevoked
	}
	g.Status, g.UpdatedAt, g.RevokedAt = GrantRevoked, now, ptrTime(now)
	g.Version++
	return nil
}

func validText(value string, max int, allowNewline bool) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r == 0 || (unicode.IsControl(r) && !(allowNewline && (r == '\n' || r == '\r' || r == '\t'))) {
			return false
		}
	}
	return true
}

func isHexDigest(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && strings.ToLower(v) == v
}

func ptrTime(v time.Time) *time.Time { return &v }

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
