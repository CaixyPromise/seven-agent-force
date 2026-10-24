// Package egress turns the Framework-owned Provider SSRF configuration into
// the exact outbound policy used by both the control-plane Probe and the
// Worker-owned Provider Proxy.
package egress

import (
	"fmt"
	"net/url"
	"strings"

	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

type Resolver struct {
	enabled bool
	entries map[string]workforceconfig.ProviderSSRFAllowEntry
}

func NewResolver(cfg workforceconfig.ProviderSSRFConfig) (*Resolver, error) {
	resolver := &Resolver{enabled: cfg.Enabled, entries: make(map[string]workforceconfig.ProviderSSRFAllowEntry, len(cfg.AllowedList))}
	for _, entry := range cfg.AllowedList {
		host := normalizeHost(entry.Hostname)
		if host == "" || strings.Contains(host, "*") || len(entry.CIDRs) == 0 || len(entry.Ports) == 0 {
			return nil, fmt.Errorf("provider SSRF allowlist entry %q is incomplete", entry.Hostname)
		}
		if entry.AllowFakeIP && !cfg.LocalDevelopment {
			return nil, fmt.Errorf("provider SSRF allowFakeIP for %q requires local development mode", entry.Hostname)
		}
		if entry.AllowLoopback && (!cfg.LocalDevelopment || host != "127.0.0.1" || len(entry.CIDRs) != 1 || strings.TrimSpace(entry.CIDRs[0]) != "127.0.0.1/32" || entry.AllowFakeIP) {
			return nil, fmt.Errorf("provider SSRF allowLoopback for %q requires exact local 127.0.0.1/32 policy", entry.Hostname)
		}
		if _, exists := resolver.entries[host]; exists {
			return nil, fmt.Errorf("provider SSRF allowlist hostname %q is duplicated", entry.Hostname)
		}
		resolver.entries[host] = workforceconfig.ProviderSSRFAllowEntry{
			Hostname: host, CIDRs: append([]string(nil), entry.CIDRs...), Ports: append([]int(nil), entry.Ports...), AllowFakeIP: entry.AllowFakeIP, AllowLoopback: entry.AllowLoopback,
		}
	}
	return resolver, nil
}

func (r *Resolver) Resolve(rawURL string) (outboundurl.Policy, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return outboundurl.Policy{}, fmt.Errorf("provider base URL is invalid: %w", outboundurl.ErrInvalidURL)
	}
	base := outboundurl.Policy{AllowHTTP: true, AllowUserInfo: true}
	if r == nil || !r.enabled {
		base.Mode = outboundurl.ModeRestrictedOnly
		return base, nil
	}
	entry, allowed := r.entries[normalizeHost(parsed.Hostname())]
	if !allowed {
		base.Mode = outboundurl.ModePublic
		return base, nil
	}
	base.Mode = outboundurl.ModePrivateAllowlist
	if entry.AllowFakeIP {
		base.Mode = outboundurl.ModeFakeIPDirect
	} else if entry.AllowLoopback {
		base.Mode = outboundurl.ModeLocalLoopback
	}
	base.AllowedHostnames = []string{entry.Hostname}
	base.AllowedCIDRs = append([]string(nil), entry.CIDRs...)
	base.AllowedPorts = append([]int(nil), entry.Ports...)
	return base, nil
}

func normalizeHost(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}
