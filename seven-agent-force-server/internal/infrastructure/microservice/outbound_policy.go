package microservice

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

type TrustScope uint8

const (
	TrustScopeDefault TrustScope = iota
	TrustScopeRegistry
)

type OutboundTrustConfig struct {
	TrustedHosts         []string
	TrustedCIDRs         []string
	RegistryTrustedHosts []string
	RegistryTrustedCIDRs []string
}

type HostResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type OutboundTrustPolicy struct {
	resolver             HostResolver
	trustedHosts         map[string]struct{}
	trustedCIDRs         []*net.IPNet
	registryTrustedHosts map[string]struct{}
	registryTrustedCIDRs []*net.IPNet
}

var nonRoutableOutboundPrefixes = []netip.Prefix{
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

func NewOutboundTrustPolicy(cfg OutboundTrustConfig, resolver HostResolver) (*OutboundTrustPolicy, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	trustedCIDRs, err := parseTrustCIDRs(cfg.TrustedCIDRs)
	if err != nil {
		return nil, err
	}
	registryCIDRs, err := parseTrustCIDRs(cfg.RegistryTrustedCIDRs)
	if err != nil {
		return nil, err
	}
	return &OutboundTrustPolicy{
		resolver: resolver, trustedHosts: trustHostSet(cfg.TrustedHosts), trustedCIDRs: trustedCIDRs,
		registryTrustedHosts: trustHostSet(cfg.RegistryTrustedHosts), registryTrustedCIDRs: registryCIDRs,
	}, nil
}

func (p *OutboundTrustPolicy) resolveAndValidate(ctx context.Context, rawHost string, scope TrustScope) ([]net.IP, error) {
	if p == nil || p.resolver == nil {
		return nil, fmt.Errorf("outbound trust policy is not configured: %w", ErrInvalidDependency)
	}
	host := normalizeTrustHost(rawHost)
	if host == "" {
		return nil, fmt.Errorf("outbound host is empty: %w", ErrInvalidRequest)
	}
	addresses := make([]net.IP, 0, 2)
	if literal := net.ParseIP(host); literal != nil {
		addresses = append(addresses, literal)
	} else {
		resolved, err := p.resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve outbound host %s: %w", host, err)
		}
		for _, address := range resolved {
			if address.IP != nil {
				addresses = append(addresses, address.IP)
			}
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("outbound host %s resolved no addresses: %w", host, ErrNoHealthyInstance)
	}
	hosts, cidrs := p.trustedHosts, p.trustedCIDRs
	if scope == TrustScopeRegistry {
		hosts, cidrs = p.registryTrustedHosts, p.registryTrustedCIDRs
	}
	_, hostTrusted := hosts[host]
	for _, address := range addresses {
		if restrictedOutboundIP(address) && !hostTrusted && !ipInTrustCIDRs(address, cidrs) {
			return nil, fmt.Errorf("outbound destination %s resolves to untrusted restricted address %s: %w", host, address.String(), ErrInvalidRequest)
		}
	}
	return addresses, nil
}

func parseTrustCIDRs(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid outbound trusted CIDR %q: %w", value, err)
		}
		result = append(result, network)
	}
	return result, nil
}

func trustHostSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if normalized := normalizeTrustHost(value); normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}

func normalizeTrustHost(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(value), "[]"), "."))
}

func restrictedOutboundIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range nonRoutableOutboundPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func ipInTrustCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, network := range cidrs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
