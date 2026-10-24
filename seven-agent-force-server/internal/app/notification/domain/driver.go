package domain

import (
	"context"
	"regexp"
	"strings"
	"unicode"
)

const providerErrorTextLimit = 256

var (
	providerErrorSecretPattern     = regexp.MustCompile(`(?i)\b(bearer\s+|(?:access[_-]?token|tenant[_-]?access[_-]?token|app[_-]?secret|corpsecret|authorization)\s*(?:=|:)\s*)[^\s,;]+`)
	providerErrorTargetPattern     = regexp.MustCompile(`\b(?:ou|oc)_[A-Za-z0-9_-]+\b`)
	providerErrorIdentifierPattern = regexp.MustCompile(`(?i)\b(?:open[_-]?id|receive[_-]?id|chat[_-]?id|user(?:[_-]?id)?|touser|invaliduser|unlicenseduser)\s*(?:=|:)\s*[^\s,;]+`)
	providerErrorIPPattern         = regexp.MustCompile(`(?i)\b((?:from\s+)?ip(?:\s*(?:=|:)\s*|\s+))(?:\[[^\]]+\]|[^\s,;]+)`)
	providerErrorURLPattern        = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>]+`)
)

// ChannelDriver sends one already materialized notification through an
// infrastructure adapter.
type ChannelDriver interface {
	Send(ctx context.Context, message DriverMessage) error
}

// ResultChannelDriver additionally reports a provider-neutral delivery
// outcome for enterprise and static HTTP transports.
type ResultChannelDriver interface {
	ChannelDriver
	SendResult(ctx context.Context, message DriverMessage) (DriverResult, error)
}

// DriverRegistry resolves a channel driver by persisted channel type.
type DriverRegistry interface {
	Driver(channelType string) ChannelDriver
}

// CapabilityRegistry describes the deterministic presentation contract of a
// registered driver. Application code uses it before accepting a delivery so
// retries never change from a card to a second text message after an ambiguous
// provider outcome.
type CapabilityRegistry interface {
	DriverRegistry
	Capabilities(channelType string) DriverCapabilities
}

// ProviderError is the bounded provider diagnostic permitted only for a
// privileged, non-persistent connection probe.
type ProviderError struct {
	Provider   string
	HTTPStatus int
	Code       string
	Message    string
	LogID      string
}

// DriverResult is the provider-neutral result consumed by notification
// application orchestration.
type DriverResult struct {
	Status            string
	FailureClass      string
	ProviderReference string
	Diagnostic        string
	Retryable         bool
	ProviderError     *ProviderError
}

const (
	DriverResultProviderAccepted = DeliveryStatusProviderAccepted
	DriverResultFailed           = "FAILED"
	DriverResultUnknown          = DeliveryStatusUnknown
)

// DriverMessage contains only the materialized delivery snapshot and
// connection-owned secret needed by a channel adapter.
type DriverMessage struct {
	Channel        Channel
	SecretPlain    string
	IdentityKind   string
	Target         string
	Subject        string
	Text           string
	HTML           string
	Markdown       string
	Variables      map[string]any
	ProviderParams map[string]any
	DeliveryID     string
	EventKey       string
	Category       string
	Priority       string
	TraceID        string
	DeepLink       string
	// PrimaryAction and ExternalActionURL are platform-frozen values. Business
	// callers cannot submit either value directly to a driver.
	PrimaryAction     *NotificationAction
	ExternalActionURL string
	Presentation      string
	// Mention is resolved by the platform from an internal user identity. It is
	// never accepted as caller-supplied provider JSON.
	Mention *DriverMention
	// Probe marks a privileged non-persistent connection check.
	Probe bool
}

type DriverMention struct {
	Provider string
	Subject  string
	Label    string
}

// SanitizeProviderError strips credentials, targets, network addresses, URLs,
// control characters, and oversized text before a provider diagnostic crosses
// the adapter boundary.
func SanitizeProviderError(value *ProviderError) *ProviderError {
	if value == nil {
		return nil
	}
	detail := &ProviderError{
		Provider:   stableDriverCode(value.Provider, "PROVIDER"),
		HTTPStatus: safeDriverHTTPStatus(value.HTTPStatus),
		Code:       stableDriverCode(value.Code, ""),
		Message:    safeDriverErrorMessage(value.Message),
		LogID:      safeDriverReference(value.LogID),
	}
	if detail.HTTPStatus == 0 && detail.Code == "" && detail.Message == "" && detail.LogID == "" {
		return nil
	}
	return detail
}

func stableDriverCode(value, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	if len(value) > 64 {
		return fallback
	}
	for _, character := range value {
		if character != '_' && !unicode.IsDigit(character) && (character < 'A' || character > 'Z') {
			return fallback
		}
	}
	return value
}

func safeDriverHTTPStatus(status int) int {
	if status < 100 || status > 599 {
		return 0
	}
	return status
}

func safeDriverErrorMessage(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = providerErrorURLPattern.ReplaceAllString(value, "[provider-url]")
	value = providerErrorSecretPattern.ReplaceAllString(value, "${1}[redacted]")
	value = providerErrorTargetPattern.ReplaceAllString(value, "[redacted]")
	value = providerErrorIPPattern.ReplaceAllString(value, "${1}[redacted]")
	value = providerErrorIdentifierPattern.ReplaceAllStringFunc(value, func(match string) string {
		if separator := strings.IndexAny(match, "=:"); separator >= 0 {
			return strings.TrimSpace(match[:separator+1]) + " [redacted]"
		}
		return "[redacted]"
	})
	return trimDriverRunes(value, providerErrorTextLimit)
}

func safeDriverReference(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 191 {
		return ""
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' && character != '-' && character != '.' && character != ':' {
			return ""
		}
	}
	return value
}

func trimDriverRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
