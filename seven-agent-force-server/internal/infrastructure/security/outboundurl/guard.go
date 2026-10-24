// Package outboundurl provides the single SSRF guard for outbound HTTP(S)
// connectors. It validates every DNS answer, pins direct dials to a validated
// address, disables ambient proxies, and treats fake-IP ranges as proxy-only.
package outboundurl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidURL        = errors.New("invalid outbound URL")
	ErrDestinationDenied = errors.New("outbound destination denied")
	ErrRedirectBlocked   = errors.New("outbound redirect denied")
	// ErrDNSResolutionFailed means the guarded resolver failed before a
	// connection attempt. Adapters may classify this as a known transient
	// failure; it is not evidence that a request reached the destination.
	ErrDNSResolutionFailed = errors.New("outbound DNS resolution failed")
	ErrPolicyNotFound      = errors.New("outbound policy reference is not configured")
)

type Mode string

const (
	// ModePublic permits only public, globally routable targets.
	ModePublic Mode = "PUBLIC"
	// ModePrivateAllowlist permits explicitly allowed private CIDRs but never
	// local machine, metadata, loopback, link-local, multicast, or fake-IP.
	ModePrivateAllowlist Mode = "PRIVATE_ALLOWLIST"
	// ModeFakeIPProxy is an explicit local/test proxy mode. Fake IP targets are
	// never dialed directly; their request must be sent via a safe proxy URL.
	ModeFakeIPProxy Mode = "FAKE_IP_PROXY"
	// ModeFakeIPDirect is a local-development-only provider mode. The caller
	// must construct it from trusted runtime configuration; it pins the direct
	// dial to an exact hostname/CIDR/port tuple in Clash's fake-IP range.
	ModeFakeIPDirect Mode = "FAKE_IP_DIRECT"
	// ModeLocalLoopback permits an exact literal loopback fixture only when a
	// trusted local Provider runtime constructs the policy. It is rejected from
	// environment policy registries and is never inferred from request data.
	ModeLocalLoopback Mode = "LOCAL_LOOPBACK"
	// ModeRestrictedOnly is the operational escape hatch: private and fake-IP
	// destinations are accepted, while metadata, loopback, local interfaces,
	// redirects and malformed URLs remain hard-denied.
	ModeRestrictedOnly Mode = "RESTRICTED_ONLY"
)

type Policy struct {
	Mode             Mode     `json:"mode"`
	AllowedHostnames []string `json:"allowedHostnames"`
	AllowedCIDRs     []string `json:"allowedCidrs"`
	AllowedPorts     []int    `json:"allowedPorts"`
	ProxyURL         string   `json:"proxyUrl"`
	AllowHTTP        bool     `json:"allowHttp,omitempty"`
	AllowUserInfo    bool     `json:"allowUserInfo,omitempty"`
	// allowLoopbackProxy is intentionally runtime-internal. It is set only
	// while constructing a local-development fake-IP environment policy; a
	// channel can reference a policy but can never turn its own loopback
	// service into a proxy target.
	allowLoopbackProxy bool
}

// EnvironmentPolicy is an egress exception owned by the runtime environment,
// not by a notification connection. Private and fake-IP entries must state an
// exact hostname, CIDR and port before they can be referenced by a channel.
type EnvironmentPolicy struct {
	Name             string
	Mode             Mode
	AllowedHostnames []string
	AllowedCIDRs     []string
	AllowedPorts     []int
	ProxyURL         string
}

// EnvironmentPolicyRegistryOptions prevents a production-like runtime from
// enabling a fake-IP proxy exception merely because a configuration row named
// one. The runtime decides whether local development permits that mode.
type EnvironmentPolicyRegistryOptions struct {
	AllowFakeIPProxy bool
}

// PolicyResolver resolves a named policy from a trusted runtime source. A
// channel configuration may carry only the reference name, never CIDRs,
// hostname exceptions, ports or a proxy URL.
type PolicyResolver interface {
	Resolve(reference string) (Policy, error)
}

// EnvironmentPolicyRegistry is an immutable in-memory registry assembled at
// startup from environment-owned configuration.
type EnvironmentPolicyRegistry struct {
	policies map[string]Policy
}

// NewEnvironmentPolicyRegistry validates trusted runtime policy entries once
// at startup. It intentionally does not accept arbitrary connection data.
func NewEnvironmentPolicyRegistry(entries []EnvironmentPolicy, options EnvironmentPolicyRegistryOptions) (*EnvironmentPolicyRegistry, error) {
	registry := &EnvironmentPolicyRegistry{policies: make(map[string]Policy, len(entries))}
	for _, entry := range entries {
		name, err := normalizePolicyReference(entry.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := registry.policies[name]; exists {
			return nil, fmt.Errorf("duplicate outbound environment policy %q: %w", entry.Name, ErrInvalidURL)
		}
		mode := Mode(strings.ToUpper(strings.TrimSpace(string(entry.Mode))))
		if mode == ModeFakeIPDirect || mode == ModeLocalLoopback || mode == ModeRestrictedOnly {
			return nil, fmt.Errorf("outbound environment policy mode %q is reserved for the Provider runtime: %w", entry.Mode, ErrDestinationDenied)
		}
		if mode == ModeFakeIPProxy && !options.AllowFakeIPProxy {
			return nil, fmt.Errorf("fake-IP proxy policy %q is only available in local development: %w", entry.Name, ErrDestinationDenied)
		}
		policy := Policy{
			Mode:               mode,
			AllowedHostnames:   append([]string(nil), entry.AllowedHostnames...),
			AllowedCIDRs:       append([]string(nil), entry.AllowedCIDRs...),
			AllowedPorts:       append([]int(nil), entry.AllowedPorts...),
			ProxyURL:           strings.TrimSpace(entry.ProxyURL),
			allowLoopbackProxy: mode == ModeFakeIPProxy && options.AllowFakeIPProxy,
		}
		if _, err := parsePolicy(policy); err != nil {
			return nil, fmt.Errorf("invalid outbound environment policy %q: %w", entry.Name, err)
		}
		registry.policies[name] = policy
	}
	return registry, nil
}

// Resolve returns the immutable policy referenced by a channel. Callers get a
// copy so no request can mutate the registry for another request.
func (r *EnvironmentPolicyRegistry) Resolve(reference string) (Policy, error) {
	if r == nil {
		return Policy{}, fmt.Errorf("outbound policy registry is not configured: %w", ErrPolicyNotFound)
	}
	name, err := normalizePolicyReference(reference)
	if err != nil {
		return Policy{}, err
	}
	policy, ok := r.policies[name]
	if !ok {
		return Policy{}, fmt.Errorf("outbound policy reference %q: %w", reference, ErrPolicyNotFound)
	}
	policy.AllowedHostnames = append([]string(nil), policy.AllowedHostnames...)
	policy.AllowedCIDRs = append([]string(nil), policy.AllowedCIDRs...)
	policy.AllowedPorts = append([]int(nil), policy.AllowedPorts...)
	return policy, nil
}

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type Options struct {
	Resolver       Resolver
	InterfaceAddrs func() ([]net.Addr, error)
	DialContext    func(context.Context, string, string) (net.Conn, error)
}

type HTTPClientOptions struct {
	TLSHandshakeTimeout    time.Duration
	ResponseHeaderTimeout  time.Duration
	MaxResponseHeaderBytes int64
	// RootCAs lets a trusted runtime add enterprise certificate authorities
	// without weakening hostname verification or destination pinning. Request
	// data must never populate this field.
	RootCAs *x509.CertPool
}

// Target is the validated URL and the address that will actually be dialed.
// For FAKE_IP_PROXY, DialIP belongs to ProxyURL rather than the fake target.
type Target struct {
	URL      *url.URL
	DialIP   string
	ProxyURL *url.URL
	Mode     Mode
}

type Guard struct {
	resolver       Resolver
	interfaceAddrs func() ([]net.Addr, error)
	dialContext    func(context.Context, string, string) (net.Conn, error)
}

// OutboundURLGuard is the explicit name used by channel adapters. Guard is
// retained as a concise package-local alias for callers that construct it.
type OutboundURLGuard = Guard

func NewGuard(options Options) *Guard {
	resolver := options.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	interfaceAddrs := options.InterfaceAddrs
	if interfaceAddrs == nil {
		interfaceAddrs = net.InterfaceAddrs
	}
	dialContext := options.DialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	return &Guard{resolver: resolver, interfaceAddrs: interfaceAddrs, dialContext: dialContext}
}

func NewOutboundURLGuard(options Options) *OutboundURLGuard {
	return NewGuard(options)
}

// Validate resolves every A/AAAA record and rejects the URL when any answer
// violates the policy. Direct callers must use HTTPClient so the connection is
// pinned to the validated address instead of performing a second DNS lookup.
func (g *Guard) Validate(ctx context.Context, rawURL string, policy Policy) (*Target, error) {
	if g == nil || g.resolver == nil || g.interfaceAddrs == nil {
		return nil, fmt.Errorf("outbound URL guard is not configured: %w", ErrDestinationDenied)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	parsedPolicy, err := parsePolicy(policy)
	if err != nil {
		return nil, err
	}
	return g.validate(ctx, rawURL, parsedPolicy, false)
}

func (g *Guard) validate(ctx context.Context, rawURL string, policy parsedPolicy, proxyValidation bool) (*Target, error) {
	parsed, err := parseOutboundURL(rawURL, policy.allowHTTP, policy.allowUserInfo)
	if err != nil {
		return nil, err
	}
	host := normalizedHost(parsed.Hostname())
	if host == "" || isMetadataHost(host) {
		return nil, fmt.Errorf("outbound hostname %q: %w", host, ErrDestinationDenied)
	}
	port, err := outboundPort(parsed)
	if err != nil {
		return nil, err
	}
	if err := policy.allowsAuthority(host, port); err != nil {
		return nil, fmt.Errorf("outbound authority %q:%d: %w", host, port, err)
	}
	localAddresses, err := g.localAddresses()
	if err != nil {
		return nil, fmt.Errorf("inspect local network interfaces: %w", err)
	}
	addresses, err := g.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("outbound hostname %q resolved no addresses: %w", host, ErrDestinationDenied)
	}
	for _, address := range addresses {
		if err := policy.allows(address, localAddresses); err != nil {
			return nil, fmt.Errorf("outbound hostname %q address %s: %w", host, address, err)
		}
	}

	target := &Target{URL: parsed, DialIP: addresses[0].String(), Mode: policy.mode}
	if policy.mode != ModeFakeIPProxy || proxyValidation {
		return target, nil
	}
	if strings.TrimSpace(policy.proxyURL) == "" {
		return nil, fmt.Errorf("fake-IP proxy mode requires proxyUrl: %w", ErrInvalidURL)
	}
	proxyTarget, err := g.validateFakeIPProxy(ctx, policy)
	if err != nil {
		return nil, err
	}
	target.DialIP = proxyTarget.DialIP
	target.ProxyURL = proxyTarget.URL
	return target, nil
}

// validateFakeIPProxy permits an HTTP loopback proxy only when the policy was
// constructed by the local-development environment registry. That narrow
// exception accommodates DNS fake-IP engines such as Clash without making a
// channel-owned URL an SSRF path into the local machine. All other proxies
// remain public HTTPS endpoints and go through the normal guard validation.
func (g *Guard) validateFakeIPProxy(ctx context.Context, policy parsedPolicy) (*Target, error) {
	proxyURL, loopback, err := parseLoopbackHTTPProxyURL(policy.proxyURL)
	if err != nil {
		return nil, fmt.Errorf("validate fake-IP proxy: %w", err)
	}
	if loopback {
		if !policy.allowLoopbackProxy {
			return nil, fmt.Errorf("loopback fake-IP proxy is not enabled by the local runtime: %w", ErrDestinationDenied)
		}
		host, _ := netip.ParseAddr(proxyURL.Hostname())
		return &Target{URL: proxyURL, DialIP: host.Unmap().String(), Mode: ModeFakeIPProxy}, nil
	}
	// A non-loopback proxy must be a public HTTPS endpoint. It is never
	// permitted to turn a private network address into a proxy control plane.
	proxyTarget, err := g.validate(ctx, policy.proxyURL, parsedPolicy{mode: ModePublic}, true)
	if err != nil {
		return nil, fmt.Errorf("validate fake-IP proxy: %w", err)
	}
	return proxyTarget, nil
}

// parseLoopbackHTTPProxyURL recognizes only a literal loopback HTTP proxy
// with an explicit port. Non-loopback URLs are left to the public-proxy
// validation path so this helper never grants them extra authority.
func parseLoopbackHTTPProxyURL(raw string) (*url.URL, bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || !parsed.IsAbs() || strings.TrimSpace(parsed.Host) == "" {
		return nil, false, fmt.Errorf("parse fake-IP proxy URL: %w", ErrInvalidURL)
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return nil, false, fmt.Errorf("fake-IP proxy URL scheme is invalid: %w", ErrInvalidURL)
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || strings.TrimSpace(parsed.Port()) == "" {
		return nil, false, fmt.Errorf("fake-IP proxy URL must not contain credentials, query, fragment, or an implicit port: %w", ErrInvalidURL)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, false, fmt.Errorf("fake-IP proxy URL port is invalid: %w", ErrInvalidURL)
	}
	host, err := netip.ParseAddr(parsed.Hostname())
	if err != nil || !host.Unmap().IsLoopback() {
		return parsed, false, nil
	}
	return parsed, true, nil
}

// HTTPClient returns a transport that performs validation for every request,
// pins the subsequent TCP dial, disables environment proxies, and rejects all
// redirects. URL-channel adapters must use this client instead of a raw
// http.Client.
func (g *Guard) HTTPClient(policy Policy) *http.Client {
	return g.HTTPClientWithOptions(policy, HTTPClientOptions{})
}

// HTTPClientWithOptions retains the same SSRF and redirect guarantees while
// allowing an owning subsystem to choose larger, explicitly bounded header
// and timeout values for long-running model requests.
func (g *Guard) HTTPClientWithOptions(policy Policy, options HTTPClientOptions) *http.Client {
	tlsHandshakeTimeout := options.TLSHandshakeTimeout
	if tlsHandshakeTimeout <= 0 {
		tlsHandshakeTimeout = 5 * time.Second
	}
	maxResponseHeaderBytes := options.MaxResponseHeaderBytes
	if maxResponseHeaderBytes <= 0 {
		maxResponseHeaderBytes = 32 << 10
	}
	transport := &guardedTransport{
		guard:  g,
		policy: policy,
		base: &http.Transport{
			Proxy:             nil,
			DialContext:       g.pinnedDialContext,
			ForceAttemptHTTP2: true,
			// Connector responses are never decompressed or interpreted during
			// durable delivery. Keep headers bounded explicitly; a receiver must
			// not be able to consume a worker with an unbounded header block.
			DisableCompression:  true,
			MaxIdleConns:        32,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: tlsHandshakeTimeout,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    options.RootCAs,
			},
			// The owning connector request context is the total request deadline.
			// A transport-wide timeout would silently shorten an operator-approved
			// connection timeout and make the persisted setting untruthful.
			ResponseHeaderTimeout:  options.ResponseHeaderTimeout,
			MaxResponseHeaderBytes: maxResponseHeaderBytes,
			ExpectContinueTimeout:  time.Second,
		},
	}
	return &http.Client{
		Transport: transport,
		// Each connector creates a bounded request context. Do not layer a
		// second global client deadline over it: it would override the typed
		// per-connection timeout configured by an operator.
		Timeout: 0,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type guardedTransport struct {
	guard  *Guard
	policy Policy
	base   http.RoundTripper
}

func (t *guardedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil || t.guard == nil || t.base == nil || request == nil || request.URL == nil {
		return nil, fmt.Errorf("outbound URL transport is not configured: %w", ErrDestinationDenied)
	}
	target, err := t.guard.Validate(request.Context(), request.URL.String(), t.policy)
	if err != nil {
		return nil, err
	}
	dialIP := target.DialIP
	if dialIP == "" {
		return nil, fmt.Errorf("outbound URL has no validated dial address: %w", ErrDestinationDenied)
	}
	request = request.Clone(withPinnedDialTarget(request.Context(), dialIP))
	if target.ProxyURL != nil {
		transport, ok := t.base.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("outbound proxy transport is not HTTP: %w", ErrDestinationDenied)
		}
		copyTransport := transport.Clone()
		copyTransport.Proxy = http.ProxyURL(target.ProxyURL)
		response, err := copyTransport.RoundTrip(request)
		if err != nil {
			return nil, err
		}
		return rejectRedirect(response)
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	return rejectRedirect(response)
}

func (g *Guard) pinnedDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if g == nil || g.dialContext == nil {
		return nil, fmt.Errorf("outbound URL dialer is not configured: %w", ErrDestinationDenied)
	}
	dialIP, ok := pinnedDialTarget(ctx)
	if !ok {
		return nil, fmt.Errorf("outbound URL dial bypassed validation: %w", ErrDestinationDenied)
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split validated dial address %q: %w", address, err)
	}
	return g.dialContext(ctx, network, net.JoinHostPort(dialIP, port))
}

func rejectRedirect(response *http.Response) (*http.Response, error) {
	if response == nil || response.StatusCode < http.StatusMultipleChoices || response.StatusCode >= http.StatusBadRequest {
		return response, nil
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	return nil, ErrRedirectBlocked
}

type parsedPolicy struct {
	mode               Mode
	allowedHosts       map[string]struct{}
	allowedCIDRs       []netip.Prefix
	allowedPorts       map[int]struct{}
	proxyURL           string
	allowLoopbackProxy bool
	allowHTTP          bool
	allowUserInfo      bool
}

func parsePolicy(policy Policy) (parsedPolicy, error) {
	mode := Mode(strings.ToUpper(strings.TrimSpace(string(policy.Mode))))
	if mode == "" {
		mode = ModePublic
	}
	if mode != ModePublic && mode != ModePrivateAllowlist && mode != ModeFakeIPProxy && mode != ModeFakeIPDirect && mode != ModeLocalLoopback && mode != ModeRestrictedOnly {
		return parsedPolicy{}, fmt.Errorf("unsupported outbound URL policy mode %q: %w", policy.Mode, ErrInvalidURL)
	}
	parsed := parsedPolicy{
		mode:               mode,
		allowedHosts:       make(map[string]struct{}, len(policy.AllowedHostnames)),
		allowedPorts:       make(map[int]struct{}, len(policy.AllowedPorts)),
		proxyURL:           strings.TrimSpace(policy.ProxyURL),
		allowLoopbackProxy: policy.allowLoopbackProxy,
		allowHTTP:          policy.AllowHTTP,
		allowUserInfo:      policy.AllowUserInfo,
	}
	if parsed.proxyURL != "" && mode != ModeFakeIPProxy {
		return parsedPolicy{}, fmt.Errorf("proxyUrl is only supported by fake-IP proxy mode: %w", ErrInvalidURL)
	}
	for _, rawHost := range policy.AllowedHostnames {
		host := normalizedHost(rawHost)
		if host == "" || strings.Contains(host, "*") || isMetadataHost(host) {
			return parsedPolicy{}, fmt.Errorf("invalid outbound URL allowed hostname %q: %w", rawHost, ErrInvalidURL)
		}
		parsed.allowedHosts[host] = struct{}{}
	}
	for _, rawCIDR := range policy.AllowedCIDRs {
		cidr := strings.TrimSpace(rawCIDR)
		if cidr == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return parsedPolicy{}, fmt.Errorf("invalid outbound URL allowed CIDR %q: %w", rawCIDR, ErrInvalidURL)
		}
		parsed.allowedCIDRs = append(parsed.allowedCIDRs, prefix.Masked())
	}
	for _, port := range policy.AllowedPorts {
		if port < 1 || port > 65535 {
			return parsedPolicy{}, fmt.Errorf("invalid outbound URL allowed port %d: %w", port, ErrInvalidURL)
		}
		parsed.allowedPorts[port] = struct{}{}
	}
	if mode == ModePublic || mode == ModeRestrictedOnly {
		if len(parsed.allowedHosts) > 0 || len(parsed.allowedCIDRs) > 0 || len(parsed.allowedPorts) > 0 {
			return parsedPolicy{}, fmt.Errorf("public and restricted-only outbound URL policies must not carry an allowlist: %w", ErrInvalidURL)
		}
		return parsed, nil
	}
	if len(parsed.allowedHosts) == 0 || len(parsed.allowedCIDRs) == 0 || len(parsed.allowedPorts) == 0 {
		return parsedPolicy{}, fmt.Errorf("private and fake-IP outbound URL policies require exact hostname, CIDR and port allowlists: %w", ErrInvalidURL)
	}
	if mode == ModeFakeIPProxy || mode == ModeFakeIPDirect {
		for _, prefix := range parsed.allowedCIDRs {
			if !prefix.Addr().Is4() || prefix.Bits() < fakeIPPrefix.Bits() || !fakeIPPrefix.Contains(prefix.Addr()) {
				return parsedPolicy{}, fmt.Errorf("fake-IP policy CIDR %s is outside 198.18.0.0/15: %w", prefix, ErrInvalidURL)
			}
		}
	}
	if mode == ModeLocalLoopback {
		if len(parsed.allowedHosts) != 1 || len(parsed.allowedCIDRs) != 1 || len(parsed.allowedPorts) == 0 {
			return parsedPolicy{}, fmt.Errorf("local loopback policy requires exact hostname, CIDR and port: %w", ErrInvalidURL)
		}
		for _, prefix := range parsed.allowedCIDRs {
			if prefix.String() != "127.0.0.1/32" {
				return parsedPolicy{}, fmt.Errorf("local loopback policy requires 127.0.0.1/32: %w", ErrInvalidURL)
			}
		}
	}
	return parsed, nil
}

func (p parsedPolicy) allowsAuthority(host string, port int) error {
	if p.mode == ModePublic || p.mode == ModeRestrictedOnly {
		return nil
	}
	if _, allowed := p.allowedHosts[normalizedHost(host)]; !allowed {
		return fmt.Errorf("hostname is outside the exact environment allowlist: %w", ErrDestinationDenied)
	}
	if _, allowed := p.allowedPorts[port]; !allowed {
		return fmt.Errorf("port is outside the exact environment allowlist: %w", ErrDestinationDenied)
	}
	return nil
}

func (p parsedPolicy) allows(address netip.Addr, localAddresses map[netip.Addr]struct{}) error {
	address = address.Unmap()
	if p.mode == ModeLocalLoopback {
		if address.String() == "127.0.0.1" && prefixContains(p.allowedCIDRs, address) {
			return nil
		}
		return fmt.Errorf("local fixture policy permits only exact loopback: %w", ErrDestinationDenied)
	}
	if _, local := localAddresses[address]; local {
		return fmt.Errorf("local interface address: %w", ErrDestinationDenied)
	}
	if alwaysDenied(address) {
		return fmt.Errorf("local, metadata, loopback, link-local, unspecified, or multicast address: %w", ErrDestinationDenied)
	}
	fakeIP := isFakeIP(address)
	if p.mode == ModeFakeIPProxy || p.mode == ModeFakeIPDirect {
		if !fakeIP {
			return fmt.Errorf("fake-IP proxy mode requires a 198.18.0.0/15 target: %w", ErrDestinationDenied)
		}
		if !prefixContains(p.allowedCIDRs, address) {
			return fmt.Errorf("fake-IP address outside the exact environment allowlist: %w", ErrDestinationDenied)
		}
		return nil
	}
	if p.mode == ModeRestrictedOnly {
		return nil
	}
	if fakeIP {
		return fmt.Errorf("fake-IP is never a direct or ordinary allowlisted target: %w", ErrDestinationDenied)
	}
	if p.mode == ModePrivateAllowlist {
		if prefixContains(p.allowedCIDRs, address) {
			return nil
		}
		return fmt.Errorf("address is outside the exact environment CIDR allowlist: %w", ErrDestinationDenied)
	}
	if !restricted(address) {
		return nil
	}
	return fmt.Errorf("restricted address outside the explicit private allowlist: %w", ErrDestinationDenied)
}

func (g *Guard) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if literal, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{literal.Unmap()}, nil
	}
	resolved, err := g.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		// Do not expose a resolver's unbounded error text to connector callers.
		// The typed sentinel also distinguishes a pre-dial lookup failure from a
		// transport error that may have happened after a request was written.
		return nil, fmt.Errorf("resolve outbound hostname %q: %w", host, ErrDNSResolutionFailed)
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	seen := make(map[netip.Addr]struct{}, len(resolved))
	for _, candidate := range resolved {
		address, ok := netip.AddrFromSlice(candidate.IP)
		if !ok {
			return nil, fmt.Errorf("resolve outbound hostname %q returned invalid address: %w", host, ErrDestinationDenied)
		}
		address = address.Unmap()
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

func (g *Guard) localAddresses() (map[netip.Addr]struct{}, error) {
	addresses, err := g.interfaceAddrs()
	if err != nil {
		return nil, err
	}
	result := make(map[netip.Addr]struct{}, len(addresses))
	for _, rawAddress := range addresses {
		var ip net.IP
		switch typed := rawAddress.(type) {
		case *net.IPNet:
			ip = typed.IP
		case *net.IPAddr:
			ip = typed.IP
		}
		if ip == nil {
			continue
		}
		if address, ok := netip.AddrFromSlice(ip); ok {
			result[address.Unmap()] = struct{}{}
		}
	}
	return result, nil
}

func parseOutboundURL(rawURL string, allowHTTP, allowUserInfo bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, fmt.Errorf("parse outbound URL: %w", ErrInvalidURL)
	}
	if !strings.EqualFold(parsed.Scheme, "https") && !(allowHTTP && strings.EqualFold(parsed.Scheme, "http")) {
		return nil, fmt.Errorf("outbound URL scheme is not allowed: %w", ErrInvalidURL)
	}
	if (!allowUserInfo && parsed.User != nil) || parsed.Fragment != "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("outbound URL contains forbidden authority or fragment: %w", ErrInvalidURL)
	}
	return parsed, nil
}

func outboundPort(parsed *url.URL) (int, error) {
	if parsed == nil {
		return 0, fmt.Errorf("outbound URL is empty: %w", ErrInvalidURL)
	}
	rawPort := strings.TrimSpace(parsed.Port())
	if rawPort == "" {
		if strings.EqualFold(parsed.Scheme, "http") {
			return 80, nil
		}
		return 443, nil
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid outbound URL port %q: %w", rawPort, ErrInvalidURL)
	}
	return port, nil
}

func normalizePolicyReference(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" || len(name) > 64 {
		return "", fmt.Errorf("invalid outbound policy reference %q: %w", raw, ErrInvalidURL)
	}
	for index, runeValue := range name {
		if (runeValue >= 'a' && runeValue <= 'z') || (runeValue >= '0' && runeValue <= '9') || (runeValue == '-' && index > 0 && index < len(name)-1) {
			continue
		}
		return "", fmt.Errorf("invalid outbound policy reference %q: %w", raw, ErrInvalidURL)
	}
	return name, nil
}

func normalizedHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(host), "[]"), "."))
}

func isMetadataHost(host string) bool {
	host = normalizedHost(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "metadata.google.internal" || host == "metadata"
}

func alwaysDenied(address netip.Addr) bool {
	if !address.IsValid() || address.IsLoopback() || address.IsUnspecified() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return true
	}
	return metadataAddresses[address]
}

func restricted(address netip.Addr) bool {
	if !address.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range specialPurposePrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func isFakeIP(address netip.Addr) bool {
	return fakeIPPrefix.Contains(address.Unmap())
}

func prefixContains(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

var fakeIPPrefix = netip.MustParsePrefix("198.18.0.0/15")

var metadataAddresses = map[netip.Addr]bool{
	netip.MustParseAddr("169.254.169.254"): true,
	netip.MustParseAddr("fd00:ec2::254"):   true,
}

var specialPurposePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

type dialTargetContextKey struct{}

func withPinnedDialTarget(ctx context.Context, dialIP string) context.Context {
	return context.WithValue(ctx, dialTargetContextKey{}, dialIP)
}

func pinnedDialTarget(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(dialTargetContextKey{}).(string)
	return value, ok && value != ""
}
