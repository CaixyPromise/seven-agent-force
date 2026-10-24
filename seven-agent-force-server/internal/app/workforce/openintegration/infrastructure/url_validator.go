package infrastructure

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

type WebhookURLValidator struct {
	guard         *outboundurl.OutboundURLGuard
	allowLoopback bool
}

func NewWebhookURLValidator(guard *outboundurl.OutboundURLGuard, allowLoopback bool) *WebhookURLValidator {
	return &WebhookURLValidator{guard: guard, allowLoopback: allowLoopback}
}

func (v *WebhookURLValidator) Policy(rawURL string) (outboundurl.Policy, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return outboundurl.Policy{}, fmt.Errorf("invalid webhook URL")
	}
	if parsed.Hostname() != "127.0.0.1" {
		if !strings.EqualFold(parsed.Scheme, "https") || parsed.Port() != "" && parsed.Port() != "443" {
			return outboundurl.Policy{}, fmt.Errorf("public webhook URL must use HTTPS port 443")
		}
		return outboundurl.Policy{Mode: outboundurl.ModePublic, AllowedPorts: []int{443}}, nil
	}
	if !v.allowLoopback {
		return outboundurl.Policy{}, fmt.Errorf("local webhook URL is disabled")
	}
	port := 443
	if strings.EqualFold(parsed.Scheme, "http") {
		port = 80
	}
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return outboundurl.Policy{}, fmt.Errorf("invalid webhook port")
		}
	}
	return outboundurl.Policy{Mode: outboundurl.ModeLocalLoopback, AllowedHostnames: []string{"127.0.0.1"}, AllowedCIDRs: []string{"127.0.0.1/32"}, AllowedPorts: []int{port}, AllowHTTP: true}, nil
}

func (v *WebhookURLValidator) Validate(ctx context.Context, rawURL string) error {
	if v == nil || v.guard == nil {
		return fmt.Errorf("webhook URL guard is unavailable")
	}
	policy, err := v.Policy(rawURL)
	if err != nil {
		return err
	}
	_, err = v.guard.Validate(ctx, rawURL, policy)
	return err
}

func (v *WebhookURLValidator) HTTPClient(rawURL string) (*http.Client, error) {
	if v == nil || v.guard == nil {
		return nil, fmt.Errorf("webhook URL guard is unavailable")
	}
	policy, err := v.Policy(rawURL)
	if err != nil {
		return nil, err
	}
	return v.guard.HTTPClient(policy), nil
}
