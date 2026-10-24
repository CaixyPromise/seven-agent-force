package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

type GroupRouteProvider string
type GroupRouteTransport string
type GroupRouteStatus string

const (
	GroupRouteProviderFeishu GroupRouteProvider = "FEISHU"
	GroupRouteProviderWeCom  GroupRouteProvider = "WECOM"

	GroupRouteTransportFeishuAppChat GroupRouteTransport = "FEISHU_APP_CHAT"
	GroupRouteTransportFeishuWebhook GroupRouteTransport = "FEISHU_WEBHOOK"
	GroupRouteTransportWeComWebhook  GroupRouteTransport = "WECOM_WEBHOOK"

	GroupRouteDraft    GroupRouteStatus = "DRAFT"
	GroupRouteVerified GroupRouteStatus = "VERIFIED"
	GroupRouteEnabled  GroupRouteStatus = "ENABLED"
	GroupRouteDisabled GroupRouteStatus = "DISABLED"
	GroupRouteRetired  GroupRouteStatus = "RETIRED"
)

var (
	ErrGroupRouteInvalid           = errors.New("NOTIFICATION_GROUP_ROUTE_INVALID")
	ErrGroupRouteNotFound          = errors.New("NOTIFICATION_GROUP_ROUTE_NOT_FOUND")
	ErrGroupRouteCrossOrganization = errors.New("NOTIFICATION_GROUP_ROUTE_CROSS_ORGANIZATION")
	ErrGroupRouteNotEnabled        = errors.New("NOTIFICATION_GROUP_ROUTE_NOT_ENABLED")
	ErrGroupRoutePermissionDenied  = errors.New("NOTIFICATION_GROUP_ROUTE_PERMISSION_DENIED")
	ErrGroupRouteLimitExceeded     = errors.New("NOTIFICATION_GROUP_ROUTE_LIMIT_EXCEEDED")
	ErrGroupRouteDrift             = errors.New("NOTIFICATION_GROUP_ROUTE_DRIFT")
	ErrGroupRouteInUse             = errors.New("NOTIFICATION_GROUP_ROUTE_IN_USE")
	ErrGroupRouteTargetInvalid     = errors.New("NOTIFICATION_GROUP_ROUTE_TARGET_INVALID")
	ErrGroupRouteDiscoveryExpired  = errors.New("NOTIFICATION_GROUP_ROUTE_DISCOVERY_EXPIRED")
	ErrGroupRouteDiscoveryTampered = errors.New("NOTIFICATION_GROUP_ROUTE_DISCOVERY_TAMPERED")
	ErrGroupRouteUnsupported       = errors.New("NOTIFICATION_GROUP_ROUTE_PRESENTATION_UNSUPPORTED")
	ErrMentionTargetInvalid        = errors.New("MENTION_TARGET_INVALID")
	ErrMentionIdentityNotAvailable = errors.New("MENTION_IDENTITY_NOT_AVAILABLE")
	ErrMentionGrantRequired        = errors.New("MENTION_GRANT_REQUIRED")
	ErrMentionAuthorityStale       = errors.New("MENTION_AUTHORITY_STALE")
	ErrMentionProviderMismatch     = errors.New("MENTION_PROVIDER_MISMATCH")
)

var permissionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(?::[a-z][a-z0-9_-]*){2,5}$`)

type GroupRoute struct {
	ID                     int64
	OrganizationID         int64
	DisplayName            string
	Provider               GroupRouteProvider
	Transport              GroupRouteTransport
	DeliveryChannelID      int64
	RequiredPermissionCode string
	TargetCiphertext       string
	TargetEDEK             string
	TargetWrapKeyRef       string
	TargetDigest           string
	Status                 GroupRouteStatus
	RouteVersion           int64
	ChannelVersion         int64
	ConnectionVersion      int64
	ConfigDigest           string
	Capabilities           []string
	LastProbeStatus        string
	LastProbeReasonCode    string
	LastProbeAt            *time.Time
	CreatorID              int64
	UpdaterID              int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (r GroupRoute) Validate() error {
	if r.ID <= 0 || r.OrganizationID <= 0 || r.DeliveryChannelID <= 0 || r.RouteVersion <= 0 || r.ChannelVersion <= 0 || r.CreatorID <= 0 || r.UpdaterID <= 0 {
		return ErrGroupRouteInvalid
	}
	if name := strings.TrimSpace(r.DisplayName); name == "" || len([]rune(name)) > 80 || containsControl(name) {
		return ErrGroupRouteInvalid
	}
	if !r.Provider.Valid() || !r.Transport.Valid() || !r.Status.Valid() || !permissionCodePattern.MatchString(strings.TrimSpace(r.RequiredPermissionCode)) {
		return ErrGroupRouteInvalid
	}
	if (r.Provider == GroupRouteProviderFeishu && r.Transport == GroupRouteTransportWeComWebhook) ||
		(r.Provider == GroupRouteProviderWeCom && r.Transport != GroupRouteTransportWeComWebhook) {
		return ErrGroupRouteInvalid
	}
	if (r.Transport == GroupRouteTransportFeishuAppChat) != (strings.TrimSpace(r.TargetCiphertext) != "") {
		return ErrGroupRouteTargetInvalid
	}
	if r.Transport == GroupRouteTransportFeishuAppChat && (r.TargetEDEK == "" || r.TargetWrapKeyRef == "" || r.TargetDigest == "" || r.ConnectionVersion <= 0) {
		return ErrGroupRouteTargetInvalid
	}
	if r.ConfigDigest == "" || len(r.ConfigDigest) > 128 {
		return ErrGroupRouteInvalid
	}
	return nil
}

func (p GroupRouteProvider) Valid() bool {
	return p == GroupRouteProviderFeishu || p == GroupRouteProviderWeCom
}
func (t GroupRouteTransport) Valid() bool {
	return t == GroupRouteTransportFeishuAppChat || t == GroupRouteTransportFeishuWebhook || t == GroupRouteTransportWeComWebhook
}
func (s GroupRouteStatus) Valid() bool {
	return s == GroupRouteDraft || s == GroupRouteVerified || s == GroupRouteEnabled || s == GroupRouteDisabled || s == GroupRouteRetired
}

func (r GroupRoute) CanEnable() bool {
	return r.Status == GroupRouteVerified && r.LastProbeStatus == "PASSED"
}

func (r *GroupRoute) MarkProbe(passed bool, reason string, at time.Time) {
	r.LastProbeAt = ptrTime(at.UTC())
	if passed {
		r.LastProbeStatus, r.LastProbeReasonCode, r.Status = "PASSED", "", GroupRouteVerified
	} else {
		r.LastProbeStatus, r.LastProbeReasonCode, r.Status = "FAILED", strings.TrimSpace(reason), GroupRouteDraft
	}
	r.RouteVersion++
	r.UpdatedAt = at.UTC()
}

func (r *GroupRoute) Enable(at time.Time) error {
	if !r.CanEnable() {
		return ErrGroupRouteDrift
	}
	r.Status, r.RouteVersion, r.UpdatedAt = GroupRouteEnabled, r.RouteVersion+1, at.UTC()
	return nil
}

func (r *GroupRoute) Disable(at time.Time) error {
	if r.Status == GroupRouteRetired {
		return ErrGroupRouteInvalid
	}
	r.Status, r.RouteVersion, r.UpdatedAt = GroupRouteDisabled, r.RouteVersion+1, at.UTC()
	return nil
}

func (r *GroupRoute) Retire(at time.Time) {
	r.Status, r.RouteVersion, r.UpdatedAt = GroupRouteRetired, r.RouteVersion+1, at.UTC()
}

func GroupRouteConfigDigest(r GroupRoute) string {
	caps := slices.Clone(r.Capabilities)
	slices.Sort(caps)
	body, _ := json.Marshal([]any{r.OrganizationID, r.Provider, r.Transport, r.DeliveryChannelID, r.RequiredPermissionCode, r.TargetDigest, r.ChannelVersion, r.ConnectionVersion, caps})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type GroupRouteSnapshot struct {
	ID                          int64
	NotificationIdentity        string
	DeliveryIdentity            string
	RouteID                     int64
	OrganizationID              int64
	RouteVersion                int64
	Provider                    GroupRouteProvider
	Transport                   GroupRouteTransport
	RouteConfigDigest           string
	DeliveryChannelID           int64
	ChannelVersion              int64
	EnterpriseConnectionVersion int64
	TargetDigest                string
	Presentation                string
	MentionRequested            bool
	MentionUserID               int64
	MentionIdentityID           int64
	MentionIdentityVersion      int64
	MentionAuthorityDigest      string
	WarningCode                 string
	RenderedPayloadDigest       string
	TargetCiphertext            string
	TargetEDEK                  string
	TargetWrapKeyRef            string
	ActionURLSnapshot           string
	CreatedAt                   time.Time
}

type PublicationChannelStatus struct {
	ChannelKind, DisplayName, State, MentionState, SafeReasonCode string
	AcceptedAt                                                    *time.Time
	UpdatedAt                                                     time.Time
}

type GroupRouteCandidate struct {
	ID                     int64
	TokenHash              string
	OrganizationID         int64
	ConnectionID           int64
	ConnectionVersion      int64
	ConnectionConfigDigest string
	DisplayName            string
	Description            string
	TargetCiphertext       string
	TargetEDEK             string
	TargetWrapKeyRef       string
	TargetDigest           string
	ExpiresAt              time.Time
	ConsumedAt             *time.Time
	CreatedAt              time.Time
}

func ptrTime(value time.Time) *time.Time { return &value }

func containsControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0
}
