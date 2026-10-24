package outboundurl

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestGuardRejectsMixedDNSLocalAndMetadataDestinations(t *testing.T) {
	guard := newTestGuard(map[string][]net.IPAddr{
		"mixed.example":    {{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.20.1.9")}},
		"local.example":    {{IP: net.ParseIP("10.1.1.8")}},
		"metadata.example": {{IP: net.ParseIP("169.254.169.254")}},
	})

	tests := []struct {
		name   string
		rawURL string
		policy Policy
	}{
		{name: "mixed public and private A records", rawURL: "https://mixed.example/hook"},
		{name: "current host interface even when CIDR is allowed", rawURL: "https://local.example/hook", policy: Policy{Mode: ModePrivateAllowlist, AllowedCIDRs: []string{"10.0.0.0/8"}}},
		{name: "metadata endpoint", rawURL: "https://metadata.example/hook", policy: Policy{Mode: ModePrivateAllowlist, AllowedCIDRs: []string{"169.254.0.0/16"}}},
		{name: "IPv4 mapped loopback", rawURL: "https://[::ffff:127.0.0.1]/hook"},
		{name: "plaintext HTTP", rawURL: "http://public.example/hook"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := guard.Validate(context.Background(), tt.rawURL, tt.policy); !errors.Is(err, ErrDestinationDenied) && !errors.Is(err, ErrInvalidURL) {
				t.Fatalf("Validate(%q) error=%v, want destination or URL denial", tt.rawURL, err)
			}
		})
	}
}

func TestGuardAllowsOnlyExactTrustedLocalProviderFixture(t *testing.T) {
	guard := NewOutboundURLGuard(Options{InterfaceAddrs: noLocalInterfaceAddrs})
	policy := Policy{Mode: ModeLocalLoopback, AllowedHostnames: []string{"127.0.0.1"}, AllowedCIDRs: []string{"127.0.0.1/32"}, AllowedPorts: []int{18080}, AllowHTTP: true}
	target, err := guard.Validate(context.Background(), "http://127.0.0.1:18080/v1/responses", policy)
	if err != nil || target.DialIP != "127.0.0.1" {
		t.Fatalf("Validate() target=%+v error=%v", target, err)
	}
	for _, rawURL := range []string{"http://127.0.0.1:18081/v1/responses", "http://127.0.0.2:18080/v1/responses", "http://169.254.169.254:18080/latest/meta-data"} {
		if _, err := guard.Validate(context.Background(), rawURL, policy); !errors.Is(err, ErrDestinationDenied) {
			t.Fatalf("Validate(%q) error=%v, want destination denial", rawURL, err)
		}
	}
}

func TestGuardAllowsExplicitPrivateCIDRButNeverDirectFakeIP(t *testing.T) {
	guard := newTestGuard(map[string][]net.IPAddr{
		"enterprise.example": {{IP: net.ParseIP("10.20.1.9")}},
		"fake.example":       {{IP: net.ParseIP("198.18.1.5")}},
		"changed.example":    {{IP: net.ParseIP("93.184.216.34")}},
	})

	if _, err := guard.Validate(context.Background(), "https://enterprise.example/hook", Policy{Mode: ModePrivateAllowlist, AllowedHostnames: []string{"enterprise.example"}, AllowedCIDRs: []string{"10.20.0.0/16"}, AllowedPorts: []int{443}}); err != nil {
		t.Fatalf("explicit enterprise target rejected: %v", err)
	}
	for _, policy := range []Policy{
		{},
		{Mode: ModePrivateAllowlist, AllowedHostnames: []string{"fake.example"}, AllowedCIDRs: []string{"198.18.0.0/15"}, AllowedPorts: []int{443}},
	} {
		if _, err := guard.Validate(context.Background(), "https://fake.example/hook", policy); !errors.Is(err, ErrDestinationDenied) {
			t.Fatalf("fake IP direct policy=%+v error=%v, want denial", policy, err)
		}
	}
	if _, err := guard.Validate(context.Background(), "https://changed.example/hook", Policy{Mode: ModePrivateAllowlist, AllowedHostnames: []string{"changed.example"}, AllowedCIDRs: []string{"10.20.0.0/16"}, AllowedPorts: []int{443}}); !errors.Is(err, ErrDestinationDenied) {
		t.Fatalf("allowlisted hostname resolving outside its exact CIDR error=%v, want denial", err)
	}
}

func TestProviderPolicyMayExplicitlyAllowHTTPUserInfoAndLocalFakeIP(t *testing.T) {
	guard := newTestGuard(map[string][]net.IPAddr{
		"fake.example": {{IP: net.ParseIP("198.18.1.5")}},
	})
	policy := Policy{
		Mode: ModeFakeIPDirect, AllowHTTP: true, AllowUserInfo: true,
		AllowedHostnames: []string{"fake.example"}, AllowedCIDRs: []string{"198.18.0.0/15"}, AllowedPorts: []int{8080},
	}
	target, err := guard.Validate(context.Background(), "http://user:pass@fake.example:8080/v1", policy)
	if err != nil {
		t.Fatalf("explicit local provider policy rejected: %v", err)
	}
	if target.DialIP != "198.18.1.5" || target.ProxyURL != nil {
		t.Fatalf("target=%+v, want a pinned direct fake-IP target", target)
	}
}

func TestDisabledProviderSSRFStillRejectsMetadataAndRedirects(t *testing.T) {
	guard := newTestGuard(map[string][]net.IPAddr{
		"private.example":  {{IP: net.ParseIP("10.20.1.9")}},
		"metadata.example": {{IP: net.ParseIP("169.254.169.254")}},
	})
	policy := Policy{Mode: ModeRestrictedOnly, AllowHTTP: true, AllowUserInfo: true}
	if _, err := guard.Validate(context.Background(), "http://private.example:8080/v1", policy); err != nil {
		t.Fatalf("escape-hatch private target rejected: %v", err)
	}
	if _, err := guard.Validate(context.Background(), "http://metadata.example/latest/meta-data", policy); !errors.Is(err, ErrDestinationDenied) {
		t.Fatalf("metadata error=%v, want hard denial", err)
	}
}

func TestGuardRejectsProxyURLOutsideExplicitFakeIPProxyMode(t *testing.T) {
	guard := newTestGuard(map[string][]net.IPAddr{
		"public.example": {{IP: net.ParseIP("93.184.216.34")}},
	})
	_, err := guard.Validate(context.Background(), "https://public.example/hook", Policy{ProxyURL: "https://public.example:8443"})
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("Validate() error=%v, want unsupported proxyUrl denial", err)
	}
}

func TestFakeIPProxyModePinsTheProxyAndNeverDialsFakeTarget(t *testing.T) {
	var mu sync.Mutex
	var dialed string
	guard := NewOutboundURLGuard(Options{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"fake.example":  {{IP: net.ParseIP("198.18.1.5")}},
			"proxy.example": {{IP: net.ParseIP("93.184.216.34")}},
		}},
		InterfaceAddrs: noLocalInterfaceAddrs,
		DialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			mu.Lock()
			dialed = address
			mu.Unlock()
			return nil, errors.New("dial intercepted")
		},
	})
	policy := Policy{Mode: ModeFakeIPProxy, AllowedHostnames: []string{"fake.example"}, AllowedCIDRs: []string{"198.18.0.0/15"}, AllowedPorts: []int{443}, ProxyURL: "https://proxy.example:8443"}
	target, err := guard.Validate(context.Background(), "https://fake.example/notification", policy)
	if err != nil {
		t.Fatalf("fake proxy validation failed: %v", err)
	}
	if target.DialIP != "93.184.216.34" || target.ProxyURL == nil {
		t.Fatalf("fake target=%+v, want proxy dial target", target)
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://fake.example/notification", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = guard.HTTPClient(policy).Do(request)
	mu.Lock()
	defer mu.Unlock()
	if dialed != "93.184.216.34:8443" {
		t.Fatalf("dialed=%q, want validated proxy address; fake target must never be dialed", dialed)
	}
}

func TestLocalDevelopmentFakeIPPolicyMayUseOnlyItsEnvironmentOwnedLoopbackProxy(t *testing.T) {
	var mu sync.Mutex
	var dialed string
	guard := NewOutboundURLGuard(Options{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"fake.example": {{IP: net.ParseIP("198.18.1.5")}},
		}},
		InterfaceAddrs: noLocalInterfaceAddrs,
		DialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			mu.Lock()
			dialed = address
			mu.Unlock()
			return nil, errors.New("dial intercepted")
		},
	})
	registry, err := NewEnvironmentPolicyRegistry([]EnvironmentPolicy{{
		Name:             "local-fake",
		Mode:             ModeFakeIPProxy,
		AllowedHostnames: []string{"fake.example"},
		AllowedCIDRs:     []string{"198.18.0.0/15"},
		AllowedPorts:     []int{443},
		ProxyURL:         "http://127.0.0.1:7897",
	}}, EnvironmentPolicyRegistryOptions{AllowFakeIPProxy: true})
	if err != nil {
		t.Fatalf("NewEnvironmentPolicyRegistry() error=%v", err)
	}
	policy, err := registry.Resolve("local-fake")
	if err != nil {
		t.Fatalf("Resolve() error=%v", err)
	}
	target, err := guard.Validate(context.Background(), "https://fake.example/notification", policy)
	if err != nil {
		t.Fatalf("Validate() error=%v", err)
	}
	if target.DialIP != "127.0.0.1" || target.ProxyURL == nil || target.ProxyURL.String() != "http://127.0.0.1:7897" {
		t.Fatalf("target=%+v, want the exact loopback proxy", target)
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://fake.example/notification", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = guard.HTTPClient(policy).Do(request)
	mu.Lock()
	defer mu.Unlock()
	if dialed != "127.0.0.1:7897" {
		t.Fatalf("dialed=%q, want only the configured local proxy; fake target must never be dialed", dialed)
	}
}

func TestFakeIPPolicyCannotUseLoopbackProxyWithoutLocalRuntimeRegistry(t *testing.T) {
	guard := NewOutboundURLGuard(Options{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"fake.example": {{IP: net.ParseIP("198.18.1.5")}},
		}},
		InterfaceAddrs: noLocalInterfaceAddrs,
	})
	policy := Policy{
		Mode:             ModeFakeIPProxy,
		AllowedHostnames: []string{"fake.example"},
		AllowedCIDRs:     []string{"198.18.0.0/15"},
		AllowedPorts:     []int{443},
		ProxyURL:         "http://127.0.0.1:7897",
	}
	if _, err := guard.Validate(context.Background(), "https://fake.example/notification", policy); !errors.Is(err, ErrDestinationDenied) {
		t.Fatalf("Validate() error=%v, want loopback proxy denial without local runtime policy", err)
	}
}

func TestFakeIPProxyModeRejectsMixedOrLocalFakeAnswers(t *testing.T) {
	policy := Policy{Mode: ModeFakeIPProxy, AllowedHostnames: []string{"fake.example"}, AllowedCIDRs: []string{"198.18.0.0/15"}, AllowedPorts: []int{443}, ProxyURL: "https://proxy.example:8443"}
	mixed := NewOutboundURLGuard(Options{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"fake.example":  {{IP: net.ParseIP("198.18.1.5")}, {IP: net.ParseIP("93.184.216.34")}},
			"proxy.example": {{IP: net.ParseIP("93.184.216.34")}},
		}},
		InterfaceAddrs: noLocalInterfaceAddrs,
	})
	if _, err := mixed.Validate(context.Background(), "https://fake.example/hook", policy); !errors.Is(err, ErrDestinationDenied) {
		t.Fatalf("mixed fake-IP answer error=%v, want denial", err)
	}

	localFake := NewOutboundURLGuard(Options{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"fake.example":  {{IP: net.ParseIP("198.18.1.5")}},
			"proxy.example": {{IP: net.ParseIP("93.184.216.34")}},
		}},
		InterfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{&net.IPNet{IP: net.ParseIP("198.18.1.5"), Mask: net.CIDRMask(15, 32)}}, nil
		},
	})
	if _, err := localFake.Validate(context.Background(), "https://fake.example/hook", policy); !errors.Is(err, ErrDestinationDenied) {
		t.Fatalf("local fake-IP answer error=%v, want local-address denial", err)
	}
}

func TestHTTPClientDisablesAmbientProxyAndBlocksRedirects(t *testing.T) {
	guard := newTestGuard(map[string][]net.IPAddr{"public.example": {{IP: net.ParseIP("93.184.216.34")}}})
	client := guard.HTTPClient(Policy{})
	transport, ok := client.Transport.(*guardedTransport)
	if !ok {
		t.Fatalf("HTTPClient transport=%T, want guarded transport", client.Transport)
	}
	base, ok := transport.base.(*http.Transport)
	if !ok {
		t.Fatalf("HTTPClient base=%T, want *http.Transport", transport.base)
	}
	if base.Proxy != nil {
		t.Fatal("HTTPClient must not inherit an ambient proxy")
	}
	if base.MaxResponseHeaderBytes != 32<<10 || !base.DisableCompression {
		t.Fatalf("HTTPClient response safeguards=%#v, want bounded headers and disabled compression", base)
	}
	if base.ResponseHeaderTimeout != 0 || client.Timeout != 0 {
		t.Fatalf("HTTPClient must defer the total deadline to the connector request context: responseHeader=%s client=%s", base.ResponseHeaderTimeout, client.Timeout)
	}
	if client.CheckRedirect == nil {
		t.Fatal("HTTPClient must reject redirects")
	}
}

func TestGuardedTransportPinsDirectDialAndFailsClosedOnRedirect(t *testing.T) {
	guard := newTestGuard(map[string][]net.IPAddr{
		"public.example": {{IP: net.ParseIP("93.184.216.34")}},
	})
	transport := &guardedTransport{
		guard:  guard,
		policy: Policy{},
		base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if dialIP, ok := pinnedDialTarget(request.Context()); !ok || dialIP != "93.184.216.34" {
				return nil, errors.New("missing validated dial target")
			}
			return &http.Response{StatusCode: http.StatusFound, Body: io.NopCloser(strings.NewReader("redirect")), Request: request}, nil
		}),
	}
	request, err := http.NewRequest(http.MethodGet, "https://public.example/redirect", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); !errors.Is(err, ErrRedirectBlocked) {
		t.Fatalf("RoundTrip() error=%v, want redirect denial", err)
	}
}

func newTestGuard(addresses map[string][]net.IPAddr) *OutboundURLGuard {
	return NewOutboundURLGuard(Options{
		Resolver: staticResolver{addresses: addresses},
		InterfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{&net.IPNet{IP: net.ParseIP("10.1.1.8"), Mask: net.CIDRMask(24, 32)}}, nil
		},
		DialContext: (&net.Dialer{}).DialContext,
	})
}

func noLocalInterfaceAddrs() ([]net.Addr, error) { return nil, nil }

type staticResolver struct {
	addresses map[string][]net.IPAddr
}

func (r staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), r.addresses[host]...), nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
