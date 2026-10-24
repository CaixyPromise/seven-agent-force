package domain

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"strings"
	"unicode"
)

const (
	NotificationActionTypeOpenInternal = "OPEN_INTERNAL"
	NotificationActionStylePrimary     = "PRIMARY"

	DeliveryPresentationAuto     = "AUTO"
	DeliveryPresentationCard     = "CARD"
	DeliveryPresentationHTML     = "HTML"
	DeliveryPresentationMarkdown = "MARKDOWN"
	DeliveryPresentationText     = "TEXT"

	ReasonNotificationActionInvalid           = "NOTIFICATION_ACTION_INVALID"
	ReasonNotificationDeepLinkInvalid         = "NOTIFICATION_DEEP_LINK_INVALID"
	ReasonNotificationPresentationUnsupported = "NOTIFICATION_PRESENTATION_UNSUPPORTED"
	ReasonNotificationRecipientNotVerified    = "NOTIFICATION_RECIPIENT_NOT_VERIFIED"
	ReasonNotificationRouteNotConfigured      = "NOTIFICATION_ROUTE_NOT_CONFIGURED"
	ReasonEnterpriseConnectionNotConfigured   = "ENTERPRISE_CONNECTION_NOT_CONFIGURED"
	ReasonPublicWebBaseURLNotConfigured       = "PUBLIC_WEB_BASE_URL_NOT_CONFIGURED"
)

// InternalDeepLink is a validated, application-owned relative navigation
// target. Business modules may supply only this value; they never supply an
// external URL or provider-specific button payload.
type InternalDeepLink struct{ value string }

func ParseInternalDeepLink(value string) (InternalDeepLink, error) {
	value = strings.TrimSpace(value)
	if value == "" || !IsSafeInternalDeepLink(value) {
		return InternalDeepLink{}, fmt.Errorf("%s", ReasonNotificationDeepLinkInvalid)
	}
	return InternalDeepLink{value: value}, nil
}

func (value InternalDeepLink) String() string { return value.value }

// NotificationAction is frozen with an accepted logical notification. Gate 2
// intentionally supports exactly one platform-rendered primary action.
type NotificationAction struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Link  string `json:"link"`
	Style string `json:"style"`
}

func NewPrimaryInternalAction(label string, link InternalDeepLink) (NotificationAction, error) {
	label = strings.TrimSpace(label)
	if label == "" || len([]rune(label)) > 32 || hasUnsafeActionLabel(label) || link.String() == "" {
		return NotificationAction{}, fmt.Errorf("%s", ReasonNotificationActionInvalid)
	}
	return NotificationAction{
		Type:  NotificationActionTypeOpenInternal,
		Label: label,
		Link:  link.String(),
		Style: NotificationActionStylePrimary,
	}, nil
}

func hasUnsafeActionLabel(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

// PublicWebBaseURL is the trusted platform origin used only to render external
// channel actions. It is never inferred from request headers or browser CORS
// configuration.
type PublicWebBaseURL struct{ value *url.URL }

func ParsePublicWebBaseURL(raw string, production bool) (PublicWebBaseURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PublicWebBaseURL{}, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return PublicWebBaseURL{}, fmt.Errorf("public web base URL is invalid")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" {
		if production || parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname()) {
			return PublicWebBaseURL{}, fmt.Errorf("public web base URL must use HTTPS")
		}
	}
	normalizedPath := strings.TrimRight(parsed.Path, "/")
	if normalizedPath == "" {
		normalizedPath = "/"
	}
	if strings.Contains(parsed.EscapedPath(), "%") || path.Clean(normalizedPath) != normalizedPath || strings.Contains(normalizedPath, "//") {
		return PublicWebBaseURL{}, fmt.Errorf("public web base URL path is invalid")
	}
	parsed.Path = strings.TrimRight(normalizedPath, "/")
	parsed.RawPath = ""
	return PublicWebBaseURL{value: parsed}, nil
}

func (value PublicWebBaseURL) Configured() bool { return value.value != nil }

func (value PublicWebBaseURL) String() string {
	if value.value == nil {
		return ""
	}
	return value.value.String()
}

func (value PublicWebBaseURL) Resolve(link InternalDeepLink) (string, error) {
	if value.value == nil {
		return "", fmt.Errorf("%s", ReasonPublicWebBaseURLNotConfigured)
	}
	parsedLink, err := url.Parse(link.String())
	if err != nil || !IsSafeInternalDeepLink(link.String()) {
		return "", fmt.Errorf("%s", ReasonNotificationDeepLinkInvalid)
	}
	resolved := *value.value
	basePath := strings.TrimRight(resolved.Path, "/")
	resolved.Path = basePath + parsedLink.Path
	resolved.RawQuery = parsedLink.RawQuery
	resolved.Fragment = ""
	return resolved.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	address := net.ParseIP(strings.Trim(host, "[]"))
	return address != nil && address.IsLoopback()
}

// DriverCapabilities are declared by a local adapter. They are configuration
// facts, not claims received from a provider response.
type DriverCapabilities struct {
	SupportedPresentations      []string
	SupportsPrimaryAction       bool
	SupportsExternalURL         bool
	SupportsProviderIdempotency bool
	OutcomeSemantics            string
}

func SelectDeliveryPresentation(preferred string, capabilities DriverCapabilities) (string, error) {
	preferred = strings.ToUpper(strings.TrimSpace(preferred))
	if preferred == "" {
		preferred = DeliveryPresentationAuto
	}
	available := make(map[string]struct{}, len(capabilities.SupportedPresentations))
	for _, presentation := range capabilities.SupportedPresentations {
		available[strings.ToUpper(strings.TrimSpace(presentation))] = struct{}{}
	}
	if preferred != DeliveryPresentationAuto {
		if _, ok := available[preferred]; !ok {
			return "", fmt.Errorf("%s", ReasonNotificationPresentationUnsupported)
		}
		return preferred, nil
	}
	for _, presentation := range []string{DeliveryPresentationCard, DeliveryPresentationHTML, DeliveryPresentationMarkdown, DeliveryPresentationText} {
		if _, ok := available[presentation]; ok {
			return presentation, nil
		}
	}
	return "", fmt.Errorf("%s", ReasonNotificationPresentationUnsupported)
}
