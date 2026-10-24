package egress

import (
	"testing"

	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

func TestResolverSelectsPublicPrivateFakeIPAndEscapeHatchPolicies(t *testing.T) {
	cfg := workforceconfig.ProviderSSRFConfig{Enabled: true, LocalDevelopment: true, AllowedList: []workforceconfig.ProviderSSRFAllowEntry{
		{Hostname: "internal.example", CIDRs: []string{"10.20.0.0/16"}, Ports: []int{8443}},
		{Hostname: "provider.example.test", CIDRs: []string{"198.18.0.0/15"}, Ports: []int{443}, AllowFakeIP: true},
		{Hostname: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}, Ports: []int{18080}, AllowLoopback: true},
	}}
	resolver, err := NewResolver(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		url  string
		want outboundurl.Mode
	}{
		{"https://public.example/v1", outboundurl.ModePublic},
		{"http://internal.example:8443/v1", outboundurl.ModePrivateAllowlist},
		{"https://provider.example.test/v1", outboundurl.ModeFakeIPDirect},
		{"http://127.0.0.1:18080/v1", outboundurl.ModeLocalLoopback},
	}
	for _, test := range tests {
		policy, err := resolver.Resolve(test.url)
		if err != nil || policy.Mode != test.want || !policy.AllowHTTP || !policy.AllowUserInfo {
			t.Fatalf("Resolve(%q) policy=%+v error=%v", test.url, policy, err)
		}
	}

	disabled, err := NewResolver(workforceconfig.ProviderSSRFConfig{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := disabled.Resolve("http://private.example:8080/v1")
	if err != nil || policy.Mode != outboundurl.ModeRestrictedOnly {
		t.Fatalf("disabled policy=%+v error=%v", policy, err)
	}
}

func TestResolverRejectsLoopbackFixtureOutsideExactLocalPolicy(t *testing.T) {
	for _, cfg := range []workforceconfig.ProviderSSRFConfig{
		{Enabled: true, AllowedList: []workforceconfig.ProviderSSRFAllowEntry{{Hostname: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}, Ports: []int{18080}, AllowLoopback: true}}},
		{Enabled: true, LocalDevelopment: true, AllowedList: []workforceconfig.ProviderSSRFAllowEntry{{Hostname: "localhost", CIDRs: []string{"127.0.0.0/8"}, Ports: []int{18080}, AllowLoopback: true}}},
	} {
		if _, err := NewResolver(cfg); err == nil {
			t.Fatalf("NewResolver(%+v) error=nil", cfg)
		}
	}
}

func TestResolverRejectsFakeIPOutsideLocalMode(t *testing.T) {
	_, err := NewResolver(workforceconfig.ProviderSSRFConfig{Enabled: true, AllowedList: []workforceconfig.ProviderSSRFAllowEntry{{
		Hostname: "provider.example.test", CIDRs: []string{"198.18.0.0/15"}, Ports: []int{443}, AllowFakeIP: true,
	}}})
	if err == nil {
		t.Fatal("NewResolver() error=nil, want local-only rejection")
	}
}
