package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// WebhookProfileConfig is the small non-secret configuration surface shared by
// the two fixed-group webhook profiles. The complete endpoint and provider
// signing material are deliberately absent: they are encrypted connection
// secrets and never appear in a read API.
type WebhookProfileConfig struct {
	TimeoutMilliseconds int   `json:"timeoutMilliseconds"`
	SuccessStatusCodes  []int `json:"successStatusCodes,omitempty"`
}

// WebhookProfileSecret is serialized only immediately before encryption. It
// is not a management API type and must never be logged or returned.
type WebhookProfileSecret struct {
	EndpointURL   string `json:"endpointUrl"`
	SigningSecret string `json:"signingSecret,omitempty"`
}

// ParseWebhookProfileConfig decodes the fixed profile's non-secret settings.
func ParseWebhookProfileConfig(raw string) (WebhookProfileConfig, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var config WebhookProfileConfig
	if err := decoder.Decode(&config); err != nil {
		return WebhookProfileConfig{}, fmt.Errorf("parse webhook profile configuration: %w", err)
	}
	if err := requireNoAdditionalJSONValue(decoder); err != nil {
		return WebhookProfileConfig{}, err
	}
	return NormalizeWebhookProfileConfig(config)
}

// EncodeWebhookProfileConfig stores only non-secret fixed-profile settings.
func EncodeWebhookProfileConfig(config WebhookProfileConfig) (string, error) {
	normalized, err := NormalizeWebhookProfileConfig(config)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode webhook profile configuration: %w", err)
	}
	return string(encoded), nil
}

// NormalizeWebhookProfileConfig bounds the profile request wait and success
// rule without adding a generic payload or header escape hatch.
func NormalizeWebhookProfileConfig(config WebhookProfileConfig) (WebhookProfileConfig, error) {
	if config.TimeoutMilliseconds == 0 {
		config.TimeoutMilliseconds = 5000
	}
	if config.TimeoutMilliseconds < httpConnectorMinTimeoutMillis || config.TimeoutMilliseconds > httpConnectorMaxTimeoutMillis {
		return WebhookProfileConfig{}, fmt.Errorf("webhook profile timeout must be between %d and %d milliseconds", httpConnectorMinTimeoutMillis, httpConnectorMaxTimeoutMillis)
	}
	if err := normalizeHTTPConnectorSuccessCodes(&config.SuccessStatusCodes); err != nil {
		return WebhookProfileConfig{}, fmt.Errorf("webhook profile success statuses: %w", err)
	}
	return config, nil
}

// EncodeWebhookProfileSecret validates a fixed provider endpoint before the
// sensitive values enter the existing envelope-encryption service.
func EncodeWebhookProfileSecret(channelType string, secret WebhookProfileSecret) (string, error) {
	normalized, err := NormalizeWebhookProfileSecret(channelType, secret)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode webhook profile secret: %w", err)
	}
	return string(encoded), nil
}

// ParseWebhookProfileSecret is intentionally an internal encrypted-value
// helper. Callers must not expose its result through records or diagnostics.
func ParseWebhookProfileSecret(channelType, raw string) (WebhookProfileSecret, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var secret WebhookProfileSecret
	if err := decoder.Decode(&secret); err != nil {
		return WebhookProfileSecret{}, fmt.Errorf("parse webhook profile secret: %w", err)
	}
	if err := requireNoAdditionalJSONValue(decoder); err != nil {
		return WebhookProfileSecret{}, err
	}
	return NormalizeWebhookProfileSecret(channelType, secret)
}

// NormalizeWebhookProfileSecret accepts only the official fixed webhook URL
// shape. It keeps Feishu's optional signing secret separate from its URL and
// never lets a caller select a target group or arbitrary endpoint.
func NormalizeWebhookProfileSecret(channelType string, secret WebhookProfileSecret) (WebhookProfileSecret, error) {
	secret.EndpointURL = strings.TrimSpace(secret.EndpointURL)
	secret.SigningSecret = strings.TrimSpace(secret.SigningSecret)
	parsed, err := url.Parse(secret.EndpointURL)
	if err != nil || parsed == nil || !parsed.IsAbs() || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.Fragment != "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return WebhookProfileSecret{}, fmt.Errorf("webhook profile URL is invalid")
	}
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeFeishuWebhook:
		if !strings.EqualFold(parsed.Hostname(), "open.feishu.cn") || parsed.Port() != "" || !strings.HasPrefix(parsed.EscapedPath(), "/open-apis/bot/v2/hook/") || strings.TrimPrefix(parsed.EscapedPath(), "/open-apis/bot/v2/hook/") == "" || parsed.RawQuery != "" {
			return WebhookProfileSecret{}, fmt.Errorf("Feishu webhook URL must use the fixed official hook path")
		}
	case ChannelTypeWeComWebhook:
		if !strings.EqualFold(parsed.Hostname(), "qyapi.weixin.qq.com") || parsed.Port() != "" || parsed.EscapedPath() != "/cgi-bin/webhook/send" || parsed.Fragment != "" {
			return WebhookProfileSecret{}, fmt.Errorf("WeCom webhook URL must use the fixed official send path")
		}
		query := parsed.Query()
		if strings.TrimSpace(query.Get("key")) == "" || len(query) != 1 || len(query["key"]) != 1 {
			return WebhookProfileSecret{}, fmt.Errorf("WeCom webhook URL must include exactly one key")
		}
		if secret.SigningSecret != "" {
			return WebhookProfileSecret{}, fmt.Errorf("WeCom webhook does not accept a separate signing secret")
		}
	default:
		return WebhookProfileSecret{}, fmt.Errorf("unsupported webhook profile")
	}
	return secret, nil
}
