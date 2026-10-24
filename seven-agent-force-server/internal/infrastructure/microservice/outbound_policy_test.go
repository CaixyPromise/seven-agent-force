package microservice

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestOutboundTrustPolicyRejectsRestrictedDestinationsUnlessExplicitlyTrusted(t *testing.T) {
	resolver := staticHostResolver{addresses: map[string][]net.IPAddr{
		"private.example": {{IP: net.ParseIP("10.0.0.7")}},
		"mixed.example":   {{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("192.168.1.8")}},
		"trusted.example": {{IP: net.ParseIP("fd00::7")}},
	}}
	policy, err := NewOutboundTrustPolicy(OutboundTrustConfig{
		TrustedHosts:         []string{"trusted.example"},
		TrustedCIDRs:         []string{"10.20.0.0/16"},
		RegistryTrustedCIDRs: []string{"10.30.0.0/16"},
	}, resolver)
	if err != nil {
		t.Fatal(err)
	}

	for _, host := range []string{"127.0.0.1", "169.254.169.254", "::1", "ff02::1", "0.0.0.0", "private.example", "mixed.example"} {
		if _, err := policy.resolveAndValidate(context.Background(), host, TrustScopeDefault); err == nil {
			t.Fatalf("restricted destination %q accepted without trust", host)
		}
	}
	for _, host := range []string{"93.184.216.34", "10.20.4.9", "trusted.example"} {
		if _, err := policy.resolveAndValidate(context.Background(), host, TrustScopeDefault); err != nil {
			t.Fatalf("trusted/public destination %q rejected: %v", host, err)
		}
	}
	if _, err := policy.resolveAndValidate(context.Background(), "10.20.4.9", TrustScopeRegistry); err == nil {
		t.Fatal("ordinary trusted CIDR must not implicitly trust Consul instances")
	}
	if _, err := policy.resolveAndValidate(context.Background(), "10.30.4.9", TrustScopeRegistry); err != nil {
		t.Fatalf("explicit registry CIDR rejected: %v", err)
	}
}

func TestOutboundTrustPolicyRejectsSpecialPurposeDestinationsByDefault(t *testing.T) {
	policy, err := NewOutboundTrustPolicy(OutboundTrustConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		host    string
		allowed bool
	}{
		{name: "IPv4 shared address space", host: "100.64.0.1"},
		{name: "IPv4 benchmarking", host: "10.254.203.7"},
		{name: "IPv4 limited broadcast", host: "255.255.255.255"},
		{name: "IPv4 documentation TEST-NET-1", host: "192.0.2.1"},
		{name: "IPv4 documentation TEST-NET-2", host: "198.51.100.1"},
		{name: "IPv4 documentation TEST-NET-3", host: "203.0.113.1"},
		{name: "IPv4 reserved", host: "240.0.0.1"},
		{name: "IPv4 loopback", host: "127.0.0.1"},
		{name: "IPv4 link local", host: "169.254.1.1"},
		{name: "IPv4 multicast", host: "224.0.0.1"},
		{name: "IPv4 unspecified", host: "0.0.0.0"},
		{name: "IPv4-mapped shared address space", host: "::ffff:100.64.0.1"},
		{name: "IPv4-mapped documentation", host: "::ffff:192.0.2.1"},
		{name: "IPv4-mapped loopback", host: "::ffff:127.0.0.1"},
		{name: "IPv6 discard only", host: "100::1"},
		{name: "IPv6 local-use translation", host: "64:ff9b:1::1"},
		{name: "IPv6 documentation", host: "2001:db8::1"},
		{name: "IPv6 documentation 3fff", host: "3fff::1"},
		{name: "IPv6 unique local", host: "fd00::1"},
		{name: "IPv6 loopback", host: "::1"},
		{name: "IPv6 link local", host: "fe80::1"},
		{name: "IPv6 multicast", host: "ff02::1"},
		{name: "IPv6 unspecified", host: "::"},
		{name: "ordinary public IPv4", host: "93.184.216.34", allowed: true},
		{name: "ordinary public IPv6", host: "2606:4700:4700::1111", allowed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := policy.resolveAndValidate(context.Background(), tt.host, TrustScopeDefault)
			if tt.allowed && err != nil {
				t.Fatalf("public destination %s rejected: %v", tt.host, err)
			}
			if !tt.allowed && err == nil {
				t.Fatalf("special-purpose destination %s accepted without explicit trust", tt.host)
			}
		})
	}
}

func TestOutboundTrustPolicyKeepsSpecialPurposeTrustScopesIsolated(t *testing.T) {
	resolver := staticHostResolver{addresses: map[string][]net.IPAddr{
		"static-special.example":   {{IP: net.ParseIP("100.64.0.7")}},
		"registry-special.example": {{IP: net.ParseIP("192.0.2.7")}},
	}}
	policy, err := NewOutboundTrustPolicy(OutboundTrustConfig{
		TrustedHosts:         []string{"static-special.example"},
		TrustedCIDRs:         []string{"198.18.0.0/15"},
		RegistryTrustedHosts: []string{"registry-special.example"},
		RegistryTrustedCIDRs: []string{"203.0.113.0/24"},
	}, resolver)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		host    string
		scope   TrustScope
		allowed bool
	}{
		{name: "Static host trust in Static scope", host: "static-special.example", scope: TrustScopeDefault, allowed: true},
		{name: "Static CIDR trust in Static scope", host: "198.18.1.1", scope: TrustScopeDefault, allowed: true},
		{name: "Static CIDR trust in IPv4-mapped form", host: "::ffff:198.18.1.1", scope: TrustScopeDefault, allowed: true},
		{name: "registry host trust in registry scope", host: "registry-special.example", scope: TrustScopeRegistry, allowed: true},
		{name: "registry CIDR trust in registry scope", host: "203.0.113.9", scope: TrustScopeRegistry, allowed: true},
		{name: "Static host trust excluded from registry scope", host: "static-special.example", scope: TrustScopeRegistry},
		{name: "Static CIDR trust excluded from registry scope", host: "198.18.1.1", scope: TrustScopeRegistry},
		{name: "Static mapped CIDR trust excluded from registry scope", host: "::ffff:198.18.1.1", scope: TrustScopeRegistry},
		{name: "registry host trust excluded from Static scope", host: "registry-special.example", scope: TrustScopeDefault},
		{name: "registry CIDR trust excluded from Static scope", host: "203.0.113.9", scope: TrustScopeDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := policy.resolveAndValidate(context.Background(), tt.host, tt.scope)
			if tt.allowed && err != nil {
				t.Fatalf("trusted destination %s rejected: %v", tt.host, err)
			}
			if !tt.allowed && err == nil {
				t.Fatalf("destination %s was trusted across scope boundary", tt.host)
			}
		})
	}
}

func TestHTTPServiceClientPinsValidatedDNSAddressThroughDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "node.internal:"+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port) {
			t.Errorf("Host header=%q", r.Host)
		}
		_, _ = io.WriteString(w, `{"code":0}`)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	resolver := staticHostResolver{addresses: map[string][]net.IPAddr{"node.internal": {{IP: net.ParseIP("127.0.0.1")}}}}
	policy, err := NewOutboundTrustPolicy(OutboundTrustConfig{TrustedHosts: []string{"node.internal"}}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var dialed string
	dialer := &net.Dialer{}
	client := NewHTTPServiceClient(nil, NewRoundRobin(), HTTPClientOptions{
		OutboundPolicy: policy,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			mu.Lock()
			dialed = address
			mu.Unlock()
			return dialer.DialContext(ctx, network, address)
		},
	})
	port := listener.Addr().(*net.TCPAddr).Port
	_, err = client.Do(context.Background(), ServiceRequest{ServiceName: "node-a", Method: http.MethodPost, Path: "/", ResolvedInstances: []ServiceInstance{{ID: "node-a", ServiceName: "node-a", Host: "node.internal", Port: port, Scheme: "http", Healthy: true}}})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.HasPrefix(dialed, "127.0.0.1:") {
		t.Fatalf("dial address=%q was not pinned to validated IP", dialed)
	}
}

type staticHostResolver struct {
	addresses map[string][]net.IPAddr
}

func (r staticHostResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), r.addresses[host]...), nil
}
