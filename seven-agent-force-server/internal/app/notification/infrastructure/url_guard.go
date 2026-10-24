package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

// ChannelURLValidator is the notification adapter for the shared
// OutboundURLGuard. Every configured HTTP(S) endpoint in a URL channel is
// resolved and checked before the channel configuration is persisted; actual
// connector transport must use the same guard's HTTPClient method.
type ChannelURLValidator struct {
	guard    *outboundurl.OutboundURLGuard
	policies outboundurl.PolicyResolver
}

const (
	outboundURLPolicyReferenceKey = "outboundUrlPolicyRef"
	egressPolicyReferenceKey      = "egressPolicyRef"
)

func NewChannelURLValidator(guard *outboundurl.OutboundURLGuard) *ChannelURLValidator {
	return NewChannelURLValidatorWithPolicyRegistry(guard, nil)
}

// NewChannelURLValidatorWithPolicyRegistry binds channel URL validation to
// runtime-owned egress policy references. The registry is deliberately
// separate from connection configuration so a connection cannot approve its
// own private range, port or proxy.
func NewChannelURLValidatorWithPolicyRegistry(guard *outboundurl.OutboundURLGuard, policies outboundurl.PolicyResolver) *ChannelURLValidator {
	if guard == nil {
		guard = outboundurl.NewOutboundURLGuard(outboundurl.Options{})
	}
	return &ChannelURLValidator{guard: guard, policies: policies}
}

func (v *ChannelURLValidator) ValidateChannel(ctx context.Context, channel domain.Channel) error {
	if v == nil || v.guard == nil || !domain.IsURLChannelType(channel.ChannelType) || strings.TrimSpace(channel.ConfigJSON) == "" {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(channel.ConfigJSON), &config); err != nil {
		return fmt.Errorf("parse URL channel config: %w", err)
	}
	policy, err := policyFromConfig(config, v.policies)
	if err != nil {
		return err
	}
	urls := make([]string, 0, 2)
	collectConfiguredURLs(config, "", &urls)
	for _, endpoint := range urls {
		if _, err := v.guard.Validate(ctx, endpoint, policy); err != nil {
			return fmt.Errorf("validate notification channel URL: %w", err)
		}
	}
	return nil
}

// ValidateWebhookProfileEndpoint applies the same guard to a fixed-group URL
// that is intentionally stored inside the encrypted secret envelope rather
// than ConfigJSON. The domain validator has already restricted the provider
// host/path; this method supplies DNS/IP and dial-policy enforcement.
func (v *ChannelURLValidator) ValidateWebhookProfileEndpoint(ctx context.Context, channelType, endpoint string) error {
	if v == nil || v.guard == nil {
		return fmt.Errorf("notification outbound URL guard is not configured")
	}
	if _, err := domain.NormalizeWebhookProfileSecret(channelType, domain.WebhookProfileSecret{EndpointURL: endpoint}); err != nil {
		return err
	}
	if _, err := v.guard.Validate(ctx, endpoint, outboundurl.Policy{Mode: outboundurl.ModePublic}); err != nil {
		return fmt.Errorf("validate notification webhook profile URL: %w", err)
	}
	return nil
}

func resolveHTTPConnectorPolicy(reference string, policies outboundurl.PolicyResolver) (outboundurl.Policy, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return outboundurl.Policy{Mode: outboundurl.ModePublic}, nil
	}
	if policies == nil {
		return outboundurl.Policy{}, fmt.Errorf("outbound URL policy reference %q cannot be resolved because environment policy registry is unavailable", reference)
	}
	policy, err := policies.Resolve(reference)
	if err != nil {
		return outboundurl.Policy{}, fmt.Errorf("resolve outbound URL policy reference %q: %w", reference, err)
	}
	return policy, nil
}

func policyFromConfig(config map[string]any, policies outboundurl.PolicyResolver) (outboundurl.Policy, error) {
	if err := rejectConnectionOwnedPolicy(config, ""); err != nil {
		return outboundurl.Policy{}, err
	}
	reference, configured, err := configuredPolicyReference(config)
	if err != nil {
		return outboundurl.Policy{}, err
	}
	if !configured {
		return resolveHTTPConnectorPolicy("", policies)
	}
	return resolveHTTPConnectorPolicy(reference, policies)
}

func configuredPolicyReference(config map[string]any) (string, bool, error) {
	var (
		reference string
		key       string
	)
	for _, candidate := range []string{outboundURLPolicyReferenceKey, egressPolicyReferenceKey} {
		raw, ok := config[candidate]
		if !ok || raw == nil {
			continue
		}
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			return "", false, fmt.Errorf("%s must be a configured environment policy name", candidate)
		}
		if key != "" {
			return "", false, fmt.Errorf("channel configuration must use either %s or %s, not both", outboundURLPolicyReferenceKey, egressPolicyReferenceKey)
		}
		key = candidate
		reference = value
	}
	return strings.TrimSpace(reference), key != "", nil
}

func rejectConnectionOwnedPolicy(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if isConnectionOwnedPolicyKey(normalized) {
				return fmt.Errorf("channel configuration must not set %q; use an environment-governed outboundUrlPolicyRef", key)
			}
			if normalized == strings.ToLower(outboundURLPolicyReferenceKey) || normalized == strings.ToLower(egressPolicyReferenceKey) {
				if path != "" {
					return fmt.Errorf("%s must appear only at the channel configuration root", key)
				}
				continue
			}
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if err := rejectConnectionOwnedPolicy(child, childPath); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := rejectConnectionOwnedPolicy(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func isConnectionOwnedPolicyKey(normalized string) bool {
	switch normalized {
	case "outboundurlpolicy", "proxyurl", "allowedcidrs", "allowedhostnames", "allowedports":
		return true
	default:
		return false
	}
}

func collectConfiguredURLs(value any, key string, urls *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			if strings.EqualFold(childKey, "outboundUrlPolicy") || strings.EqualFold(childKey, outboundURLPolicyReferenceKey) || strings.EqualFold(childKey, egressPolicyReferenceKey) {
				continue
			}
			collectConfiguredURLs(childValue, childKey, urls)
		}
	case []any:
		for _, childValue := range typed {
			collectConfiguredURLs(childValue, key, urls)
		}
	case string:
		if isURLConfigKey(key) && strings.TrimSpace(typed) != "" {
			*urls = append(*urls, strings.TrimSpace(typed))
		}
	}
}

func isURLConfigKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "url" || strings.HasSuffix(normalized, "url")
}
