package domain

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	// ExternalIdentityFeishuOpenID identifies a Feishu application open_id.
	ExternalIdentityFeishuOpenID = "FEISHU_OPEN_ID"
	// ExternalIdentityFeishuChatID identifies a Feishu application chat_id.
	ExternalIdentityFeishuChatID = "FEISHU_CHAT_ID"
	// ExternalIdentityWeComUserID identifies a WeCom self-built app userid.
	ExternalIdentityWeComUserID  = "WECOM_USERID"
	ExternalIdentityEmailAddress = "EMAIL_ADDRESS"

	// ProviderParameterMentionedList is the optional WeCom text-message
	// mention list. It is provider-owned syntax, never a raw body fragment.
	ProviderParameterMentionedList = "mentionedList"

	// ProviderParameterWarningIgnored identifies an optional input that was
	// intentionally omitted without blocking the base notification.
	ProviderParameterWarningIgnored = "PARAMETER_IGNORED"

	externalTargetSubjectMaxBytes = 512
)

// ExternalTarget is an immutable encrypted snapshot of one third-party
// enterprise member. It has no platform user ID and creates no inbox state.
type ExternalTarget struct {
	// ID is the internal primary key.
	ID int64 `db:"id"`
	// ExternalTargetID is the stable diagnostic identifier.
	ExternalTargetID string `db:"externalTargetId"`
	// NotificationID references the logical notification that accepted this target.
	NotificationID int64 `db:"notificationId"`
	// ScopeID identifies the installation, Hub, or Node boundary.
	ScopeID                     string `db:"scopeId"`
	OrganizationID              int64  `db:"organizationId"`
	UserID                      int64  `db:"userId"`
	ExternalIdentityID          int64  `db:"externalIdentityId"`
	ExternalIdentityVersion     int64  `db:"externalIdentityVersion"`
	EnterpriseConnectionID      int64  `db:"enterpriseConnectionId"`
	EnterpriseConnectionVersion int64  `db:"enterpriseConnectionVersion"`
	AuthorityDigest             string `db:"authorityDigest"`
	GrantType                   string `db:"grantType"`
	// ConnectionRef identifies the operator-configured enterprise application.
	ConnectionRef string `db:"connectionRef"`
	// ProviderCode identifies FEISHU_APP or WECOM_APP.
	ProviderCode string `db:"providerCode"`
	// IdentityKind identifies the allowed third-party subject shape.
	IdentityKind string `db:"identityKind"`
	// SubjectCiphertext is the encrypted provider subject, never plaintext.
	SubjectCiphertext string `db:"subjectCiphertext"`
	// SubjectEDEK wraps the subject data-encryption key.
	SubjectEDEK string `db:"subjectEdek"`
	// SubjectWrapKeyRef identifies the encryption wrapping key.
	SubjectWrapKeyRef string `db:"subjectWrapKeyRef"`
	// SubjectDigest is a keyed one-way subject digest for uniqueness.
	SubjectDigest string `db:"subjectDigest"`
	// SubjectDigestKeyRef identifies the HMAC key version used for SubjectDigest.
	SubjectDigestKeyRef string `db:"subjectDigestKeyRef"`
	// ProviderParamsJSON is the canonical resolved optional-parameter snapshot.
	ProviderParamsJSON string `db:"providerParamsJson"`
	// CreateTime is the durable acceptance time.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is retained for standard repository mapping consistency.
	UpdateTime time.Time `db:"updateTime"`
}

// DeliveryAttempt is an append-only sanitized record of one external provider
// invocation outcome. It deliberately stores neither plaintext targets nor
// request bodies, credentials, or unrestricted response bodies.
type DeliveryAttempt struct {
	// ID is the internal primary key.
	ID int64 `db:"id"`
	// AttemptID is the stable diagnostic identifier.
	AttemptID string `db:"attemptId"`
	// DeliveryID references the delivery being attempted.
	DeliveryID string `db:"deliveryId"`
	// AttemptNo monotonically increases for retries of the same delivery.
	AttemptNo int `db:"attemptNo"`
	// Status is PROVIDER_ACCEPTED, FAILED, or UNKNOWN.
	Status string `db:"status"`
	// FailureClass is a normalized non-secret classification.
	FailureClass string `db:"failureClass"`
	// ProviderReference is a sanitized message reference when available.
	ProviderReference string `db:"providerReference"`
	// Diagnostic is a stable sanitized code, not a provider response body.
	Diagnostic string `db:"diagnostic"`
	// CreateTime is the durable attempt time.
	CreateTime time.Time `db:"createTime"`
}

// ProviderParameterDescriptor declares one optional, user-visible provider
// parameter. It is a narrow contract rather than a generic provider payload.
type ProviderParameterDescriptor struct {
	Key           string
	Label         string
	ValueType     string
	MaxItems      int
	MaxValueBytes int
	AllowDefault  bool
}

// ProviderParameterSetting is the persisted operator enable/default setting
// for one declared parameter. JSON is only its storage representation.
type ProviderParameterSetting struct {
	Key          string `json:"key"`
	Enabled      bool   `json:"enabled"`
	DefaultValue any    `json:"defaultValue,omitempty"`
}

// ProviderParameterWarning is intentionally value-free so it can be returned
// to a business caller and logged without exposing sensitive data.
type ProviderParameterWarning struct {
	Provider string
	Key      string
	Reason   string
}

// EnterpriseApplicationConfig contains the non-secret application identifiers
// required by the two G4 enterprise app adapters.
type EnterpriseApplicationConfig struct {
	AppID   string `json:"appId,omitempty"`
	CorpID  string `json:"corpId,omitempty"`
	AgentID string `json:"agentId,omitempty"`
}

// ExternalRecipientFingerprint contains only canonical non-plaintext values
// used to calculate semantic request idempotency.
type ExternalRecipientFingerprint struct {
	ConnectionRef      string `json:"connectionRef"`
	ProviderCode       string `json:"providerCode"`
	IdentityKind       string `json:"identityKind"`
	SubjectDigest      string `json:"subjectDigest"`
	ProviderParamsJSON string `json:"providerParamsJson"`
}

type StaticRouteFingerprint struct {
	ConnectionRef string `json:"connectionRef"`
	ProviderCode  string `json:"providerCode"`
}

// IsEnterpriseApplicationChannelType reports whether a channel is one of the
// two active G4 dynamically addressed enterprise application connectors.
func IsEnterpriseApplicationChannelType(channelType string) bool {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeFeishuApp, ChannelTypeWeComApp:
		return true
	default:
		return false
	}
}

// IsDeferredChannelType reports connector codes intentionally documented but
// not executable in the current delivery phase.
func IsDeferredChannelType(channelType string) bool {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case "WEB_PUSH", "FCM", "APNS", "WECHAT_MINIPROGRAM", "SLACK", "TEAMS", ChannelTypeDingTalk:
		return true
	default:
		return false
	}
}

// IsStaticHTTPChannelType identifies G5.2 outbound channels whose complete
// destination belongs to a saved connection. They never accept a per-call
// person, group, URL, request body or credential override.
func IsStaticHTTPChannelType(channelType string) bool {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeHTTPConnector, ChannelTypeFeishuWebhook, ChannelTypeWeComWebhook:
		return true
	default:
		return false
	}
}

// IsWebhookProfileChannelType identifies fixed Feishu and WeCom group
// profiles. Their endpoint is a secret-owned connection value, not a generic
// URL field or a dynamic recipient target.
func IsWebhookProfileChannelType(channelType string) bool {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeFeishuWebhook, ChannelTypeWeComWebhook:
		return true
	default:
		return false
	}
}

// ExpectedExternalIdentityKind returns the direct-recipient identity kind used
// only when a legacy driver call has no explicit identity kind. New semantic
// external-recipient paths must use SupportsEnterpriseApplicationIdentityKind.
func ExpectedExternalIdentityKind(channelType string) string {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeFeishuApp:
		return ExternalIdentityFeishuOpenID
	case ChannelTypeWeComApp:
		return ExternalIdentityWeComUserID
	default:
		return ""
	}
}

// SupportsEnterpriseApplicationIdentityKind reports whether a concrete
// provider application channel accepts the supplied external target kind.
func SupportsEnterpriseApplicationIdentityKind(channelType, identityKind string) bool {
	channelType = strings.ToUpper(strings.TrimSpace(channelType))
	identityKind = strings.ToUpper(strings.TrimSpace(identityKind))
	switch channelType {
	case ChannelTypeFeishuApp:
		return identityKind == ExternalIdentityFeishuOpenID || identityKind == ExternalIdentityFeishuChatID
	case ChannelTypeWeComApp:
		return identityKind == ExternalIdentityWeComUserID
	default:
		return false
	}
}

// SupportsExternalDeliveryIdentityKind covers every dynamically addressed
// external channel accepted by Notification Center. Email is kept separate
// from enterprise identities but uses the same encrypted target snapshot.
func SupportsExternalDeliveryIdentityKind(channelType, identityKind string) bool {
	if SupportsEnterpriseApplicationIdentityKind(channelType, identityKind) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(channelType), ChannelTypeEmail) && strings.EqualFold(strings.TrimSpace(identityKind), ExternalIdentityEmailAddress)
}

func NormalizeEmailTarget(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed == nil || strings.ToLower(strings.TrimSpace(parsed.Address)) != value || len(value) > 254 {
		return "", fmt.Errorf("email target is invalid")
	}
	return value, nil
}

// NormalizeExternalTargetSubject returns the canonical third-party subject for
// one dynamic G4 target. A WeCom subject must name exactly one member;
// provider aggregate syntax is not part of this Goal. Feishu open_id and
// chat_id remain opaque provider identifiers.
func NormalizeExternalTargetSubject(identityKind, raw string) (string, error) {
	identityKind = strings.ToUpper(strings.TrimSpace(identityKind))
	subject := strings.TrimSpace(raw)
	if subject == "" || len(subject) > externalTargetSubjectMaxBytes {
		return "", fmt.Errorf("enterprise target subject is invalid")
	}
	switch identityKind {
	case ExternalIdentityFeishuOpenID, ExternalIdentityFeishuChatID:
		return subject, nil
	case ExternalIdentityWeComUserID:
		if !isSingleWeComMemberID(subject) {
			return "", fmt.Errorf("WeCom member subject must identify exactly one member")
		}
		return subject, nil
	default:
		return "", fmt.Errorf("unsupported enterprise target identity kind %q", identityKind)
	}
}

// NormalizeExternalMemberSubject is retained for direct-member call sites;
// group-aware paths should use NormalizeExternalTargetSubject.
func NormalizeExternalMemberSubject(identityKind, raw string) (string, error) {
	return NormalizeExternalTargetSubject(identityKind, raw)
}

// NormalizeEnterpriseApplicationTarget applies the provider's concrete target
// contract before an adapter can construct an outbound provider request.
func NormalizeEnterpriseApplicationTarget(channelType, identityKind, raw string) (string, error) {
	if !SupportsEnterpriseApplicationIdentityKind(channelType, identityKind) {
		return "", fmt.Errorf("unsupported enterprise application channel %q", channelType)
	}
	return NormalizeExternalTargetSubject(identityKind, raw)
}

// ProviderParameterCatalog returns the small declared optional-parameter
// catalogue for a G4 application channel.
func ProviderParameterCatalog(channelType string) []ProviderParameterDescriptor {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeWeComApp:
		return []ProviderParameterDescriptor{{
			Key:           ProviderParameterMentionedList,
			Label:         "提醒成员",
			ValueType:     "stringList",
			MaxItems:      100,
			MaxValueBytes: 64,
			AllowDefault:  true,
		}}
	default:
		return []ProviderParameterDescriptor{}
	}
}

// ParseEnterpriseApplicationConfig parses only the public configuration keys
// supported by the selected G4 enterprise application channel.
func ParseEnterpriseApplicationConfig(channelType, raw string) (EnterpriseApplicationConfig, error) {
	config := EnterpriseApplicationConfig{}
	if strings.TrimSpace(raw) == "" {
		return config, fmt.Errorf("enterprise application configuration is required")
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return config, fmt.Errorf("parse enterprise application configuration: %w", err)
	}
	config.AppID = strings.TrimSpace(config.AppID)
	config.CorpID = strings.TrimSpace(config.CorpID)
	config.AgentID = strings.TrimSpace(config.AgentID)
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeFeishuApp:
		if config.AppID == "" {
			return config, fmt.Errorf("Feishu appId is required")
		}
	case ChannelTypeWeComApp:
		if config.CorpID == "" || config.AgentID == "" {
			return config, fmt.Errorf("WeCom corpId and agentId are required")
		}
		if _, err := strconv.ParseInt(config.AgentID, 10, 64); err != nil {
			return config, fmt.Errorf("WeCom agentId must be numeric")
		}
	default:
		return config, fmt.Errorf("unsupported enterprise application channel %q", channelType)
	}
	for _, value := range []string{config.AppID, config.CorpID, config.AgentID} {
		if len(value) > 128 {
			return config, fmt.Errorf("enterprise application identifier exceeds 128 bytes")
		}
	}
	return config, nil
}

// EncodeEnterpriseApplicationConfig produces the internal JSON persistence
// representation from the structured management request.
func EncodeEnterpriseApplicationConfig(channelType string, config EnterpriseApplicationConfig) (string, error) {
	parsed, err := ParseEnterpriseApplicationConfig(channelType, mustEnterpriseConfigJSON(config))
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// NormalizeProviderParameterSettings validates operator settings against the
// provider catalogue. Unknown or duplicate settings are configuration errors;
// callers cannot enable an undeclared provider capability.
func NormalizeProviderParameterSettings(channelType string, settings []ProviderParameterSetting) ([]ProviderParameterSetting, error) {
	catalogue := descriptorMap(ProviderParameterCatalog(channelType))
	if len(settings) == 0 {
		return []ProviderParameterSetting{}, nil
	}
	seen := make(map[string]struct{}, len(settings))
	result := make([]ProviderParameterSetting, 0, len(settings))
	for _, setting := range settings {
		key := strings.TrimSpace(setting.Key)
		descriptor, ok := catalogue[key]
		if !ok {
			return nil, fmt.Errorf("provider parameter %q is not supported", key)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("provider parameter %q is configured more than once", key)
		}
		seen[key] = struct{}{}
		if setting.DefaultValue != nil {
			if !descriptor.AllowDefault {
				return nil, fmt.Errorf("provider parameter %q does not allow defaults", key)
			}
			if _, _, reason := normalizeProviderParameterValue(descriptor, setting.DefaultValue); reason != "" {
				return nil, fmt.Errorf("provider parameter %q default is invalid: %s", key, reason)
			}
		}
		result = append(result, ProviderParameterSetting{Key: key, Enabled: setting.Enabled, DefaultValue: setting.DefaultValue})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

// ResolveProviderParameters returns resolved values, valid caller-supplied
// overrides, and value-free warnings. Bad optional values never fail the base
// business notification.
func ResolveProviderParameters(channelType string, settings []ProviderParameterSetting, supplied map[string]any) (map[string]any, map[string]any, []ProviderParameterWarning) {
	catalogue := descriptorMap(ProviderParameterCatalog(channelType))
	resolved := make(map[string]any)
	provided := make(map[string]any)
	for _, setting := range settings {
		descriptor, ok := catalogue[strings.TrimSpace(setting.Key)]
		if !ok || !setting.Enabled || setting.DefaultValue == nil {
			continue
		}
		if value, _, reason := normalizeProviderParameterValue(descriptor, setting.DefaultValue); reason == "" {
			resolved[descriptor.Key] = value
		}
	}
	warnings := make([]ProviderParameterWarning, 0)
	keys := make([]string, 0, len(supplied))
	for key := range supplied {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		descriptor, ok := catalogue[key]
		if !ok {
			reason := "UNSUPPORTED"
			if isProtectedProviderParameterKey(key) {
				reason = "PROTECTED_KEY"
			}
			warnings = append(warnings, ProviderParameterWarning{Provider: strings.ToUpper(strings.TrimSpace(channelType)), Key: key, Reason: reason})
			continue
		}
		value, _, reason := normalizeProviderParameterValue(descriptor, supplied[rawKey])
		if reason != "" {
			warnings = append(warnings, ProviderParameterWarning{Provider: strings.ToUpper(strings.TrimSpace(channelType)), Key: key, Reason: reason})
			continue
		}
		resolved[key] = value
		provided[key] = value
	}
	return resolved, provided, warnings
}

// SanitizeProviderParameterSnapshot is a defensive adapter-side filter for an
// already materialized parameter snapshot. Semantic callers use
// ResolveProviderParameters so they can return value-free warnings; adapters
// repeat the filtering to ensure direct driver use cannot pass raw provider
// fields or disallowed aggregate syntax to the wire.
func SanitizeProviderParameterSnapshot(channelType string, values map[string]any) map[string]any {
	catalogue := descriptorMap(ProviderParameterCatalog(channelType))
	sanitized := make(map[string]any)
	for key, raw := range values {
		descriptor, ok := catalogue[strings.TrimSpace(key)]
		if !ok {
			continue
		}
		value, _, reason := normalizeProviderParameterValue(descriptor, raw)
		if reason == "" {
			sanitized[descriptor.Key] = value
		}
	}
	return sanitized
}

// CanonicalProviderParamsJSON serializes normalized optional values with a
// stable map-key order for idempotency and immutable target snapshots.
func CanonicalProviderParamsJSON(values map[string]any) (string, error) {
	if len(values) == 0 {
		return "{}", nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ParseProviderParamsJSON restores a target's previously accepted parameter
// snapshot. It is never used to accept arbitrary new provider payload fields.
func ParseProviderParamsJSON(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("parse provider parameter snapshot: %w", err)
	}
	if values == nil {
		values = map[string]any{}
	}
	return values, nil
}

// ParseProviderParameterSettings reads only the structured parameter-settings
// member from a channel metadata document.
func ParseProviderParameterSettings(raw string) ([]ProviderParameterSetting, error) {
	if strings.TrimSpace(raw) == "" {
		return []ProviderParameterSetting{}, nil
	}
	var metadata struct {
		ProviderParameterSettings []ProviderParameterSetting `json:"providerParameterSettings"`
	}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("parse provider parameter settings: %w", err)
	}
	return metadata.ProviderParameterSettings, nil
}

// MergeProviderParameterSettings stores structured settings inside the
// existing metadata JSON without requiring the operator to edit raw JSON.
func MergeProviderParameterSettings(raw string, settings []ProviderParameterSetting) (string, error) {
	metadata := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return "", fmt.Errorf("parse channel metadata: %w", err)
		}
	}
	metadata["providerParameterSettings"] = settings
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// MaskExternalSubject returns a compact, non-reversible display form for an
// external provider subject used only in diagnostics.
func MaskExternalSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "***"
	}
	runes := []rune(subject)
	if len(runes) <= 2 {
		return "***"
	}
	return string(runes[:1]) + "***" + string(runes[len(runes)-1:])
}

func descriptorMap(catalogue []ProviderParameterDescriptor) map[string]ProviderParameterDescriptor {
	result := make(map[string]ProviderParameterDescriptor, len(catalogue))
	for _, descriptor := range catalogue {
		result[descriptor.Key] = descriptor
	}
	return result
}

func normalizeProviderParameterValue(descriptor ProviderParameterDescriptor, raw any) (any, bool, string) {
	switch descriptor.ValueType {
	case "stringList":
		values, reason := normalizeProviderStringList(raw, descriptor.MaxItems, descriptor.MaxValueBytes)
		if reason != "" {
			return nil, false, reason
		}
		if descriptor.Key == ProviderParameterMentionedList {
			for _, value := range values {
				if !isSingleWeComMemberID(value) {
					return nil, false, "DISALLOWED_VALUE"
				}
			}
		}
		return values, true, ""
	default:
		return nil, false, "UNSUPPORTED"
	}
}

func normalizeProviderStringList(raw any, maxItems, maxValueBytes int) ([]string, string) {
	values := make([]string, 0)
	switch typed := raw.(type) {
	case string:
		values = strings.Split(typed, ",")
	case []string:
		values = append(values, typed...)
	case []any:
		for _, value := range typed {
			stringValue, ok := value.(string)
			if !ok {
				return nil, "INVALID_VALUE"
			}
			values = append(values, stringValue)
		}
	default:
		return nil, "INVALID_VALUE"
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > maxValueBytes {
			return nil, "INVALID_VALUE"
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) > maxItems {
		return nil, "TOO_LARGE"
	}
	sort.Strings(normalized)
	return normalized, ""
}

func isSingleWeComMemberID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "@all") {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || strings.ContainsRune("|,;", r) {
			return false
		}
	}
	return true
}

func isProtectedProviderParameterKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, protected := range []string{"connection", "connectionref", "recipient", "target", "subject", "url", "headers", "header", "secret", "token", "authorization", "auth", "messagetype", "template", "body", "content"} {
		if normalized == protected {
			return true
		}
	}
	return false
}

func mustEnterpriseConfigJSON(config EnterpriseApplicationConfig) string {
	raw, _ := json.Marshal(config)
	return string(raw)
}
