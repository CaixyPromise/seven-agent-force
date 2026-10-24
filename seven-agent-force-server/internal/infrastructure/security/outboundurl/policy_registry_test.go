package outboundurl

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestEnvironmentPolicyRegistryAllowsOnlyExactPrivateHostnameCIDRAndPort(t *testing.T) {
	registry, err := NewEnvironmentPolicyRegistry([]EnvironmentPolicy{
		{
			Name:             "corp-orders",
			Mode:             ModePrivateAllowlist,
			AllowedHostnames: []string{"orders.corp.example"},
			AllowedCIDRs:     []string{"10.20.0.0/16"},
			AllowedPorts:     []int{8443},
		},
	}, EnvironmentPolicyRegistryOptions{})
	if err != nil {
		t.Fatalf("NewEnvironmentPolicyRegistry() error = %v", err)
	}
	policy, err := registry.Resolve("corp-orders")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	guard := NewOutboundURLGuard(Options{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"orders.corp.example": {{IP: net.ParseIP("10.20.1.9")}},
			"other.corp.example":  {{IP: net.ParseIP("10.20.1.9")}},
		}},
		InterfaceAddrs: noLocalInterfaceAddrs,
	})

	if _, err := guard.Validate(context.Background(), "https://orders.corp.example:8443/notify", policy); err != nil {
		t.Fatalf("exact governed target rejected: %v", err)
	}
	for _, rawURL := range []string{
		"https://other.corp.example:8443/notify",
		"https://orders.corp.example:9443/notify",
	} {
		if _, err := guard.Validate(context.Background(), rawURL, policy); !errors.Is(err, ErrDestinationDenied) {
			t.Fatalf("Validate(%q) error = %v, want governed-target denial", rawURL, err)
		}
	}
}

func TestEnvironmentPolicyRegistryRejectsFakeIPProxyOutsideLocalDevelopment(t *testing.T) {
	_, err := NewEnvironmentPolicyRegistry([]EnvironmentPolicy{
		{
			Name:             "local-fake",
			Mode:             ModeFakeIPProxy,
			AllowedHostnames: []string{"receiver.example"},
			AllowedCIDRs:     []string{"198.18.0.0/15"},
			AllowedPorts:     []int{443},
			ProxyURL:         "https://proxy.example:8443",
		},
	}, EnvironmentPolicyRegistryOptions{AllowFakeIPProxy: false})
	if err == nil {
		t.Fatal("NewEnvironmentPolicyRegistry() accepted a fake-IP proxy policy outside local development")
	}
}

func TestEnvironmentPolicyRegistryRejectsIncompletePrivateEntry(t *testing.T) {
	_, err := NewEnvironmentPolicyRegistry([]EnvironmentPolicy{{
		Name:         "too-broad",
		Mode:         ModePrivateAllowlist,
		AllowedCIDRs: []string{"10.0.0.0/8"},
	}}, EnvironmentPolicyRegistryOptions{})
	if err == nil {
		t.Fatal("NewEnvironmentPolicyRegistry() accepted a private entry without an exact hostname and port")
	}
}
