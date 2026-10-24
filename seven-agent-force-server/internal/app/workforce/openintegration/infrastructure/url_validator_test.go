package infrastructure

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

func TestWebhookURLPolicyRestrictsPublicDestinationsToHTTPS443(t *testing.T) {
	validator := NewWebhookURLValidator(outboundurl.NewOutboundURLGuard(outboundurl.Options{}), false)
	for _, rawURL := range []string{"https://example.com/hook", "https://example.com:443/hook"} {
		policy, err := validator.Policy(rawURL)
		if err != nil {
			t.Fatalf("Policy(%q): %v", rawURL, err)
		}
		if policy.Mode != outboundurl.ModePublic || len(policy.AllowedPorts) != 1 || policy.AllowedPorts[0] != 443 {
			t.Fatalf("unexpected public policy for %q: %+v", rawURL, policy)
		}
	}
	for _, rawURL := range []string{"http://example.com/hook", "https://example.com:22/hook", "https://example.com:8443/hook"} {
		if _, err := validator.Policy(rawURL); err == nil {
			t.Fatalf("Policy accepted dangerous public target %q", rawURL)
		}
	}
}

func TestWebhookURLPolicyKeepsLoopbackBehindExplicitLocalFlag(t *testing.T) {
	if _, err := NewWebhookURLValidator(outboundurl.NewOutboundURLGuard(outboundurl.Options{}), false).Policy("http://127.0.0.1:9388/hook"); err == nil {
		t.Fatal("loopback webhook was accepted without the local flag")
	}
	policy, err := NewWebhookURLValidator(outboundurl.NewOutboundURLGuard(outboundurl.Options{}), true).Policy("http://127.0.0.1:9388/hook")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != outboundurl.ModeLocalLoopback || len(policy.AllowedPorts) != 1 || policy.AllowedPorts[0] != 9388 {
		t.Fatalf("unexpected loopback policy: %+v", policy)
	}
}
