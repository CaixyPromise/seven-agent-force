package domain

import (
	"net"
	"net/url"
	"path"
	"slices"
	"strings"
)

type EnterpriseIdentityGrant string

const (
	GrantIdentityLink        EnterpriseIdentityGrant = "IDENTITY_LINK"
	GrantNotificationDirect  EnterpriseIdentityGrant = "NOTIFICATION_DIRECT"
	GrantNotificationMention EnterpriseIdentityGrant = "NOTIFICATION_MENTION"
	GrantLogin               EnterpriseIdentityGrant = "LOGIN"
)

func (g EnterpriseIdentityGrant) Valid() bool {
	return g == GrantIdentityLink || g == GrantNotificationDirect || g == GrantNotificationMention || g == GrantLogin
}

func NormalizeEnterpriseGrants(values []EnterpriseIdentityGrant) ([]EnterpriseIdentityGrant, error) {
	seen := make(map[EnterpriseIdentityGrant]struct{}, len(values))
	result := make([]EnterpriseIdentityGrant, 0, len(values))
	for _, value := range values {
		value = EnterpriseIdentityGrant(strings.ToUpper(strings.TrimSpace(string(value))))
		if !value.Valid() {
			return nil, ErrEnterpriseGrant
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result, nil
}

type PublicWebBaseURL struct{ value string }

func ParsePublicWebBaseURL(raw string, production bool) (PublicWebBaseURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PublicWebBaseURL{}, ErrPublicWebURLNotConfigured
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return PublicWebBaseURL{}, ErrDeepLinkInvalid
	}
	if production && parsed.Scheme != "https" {
		return PublicWebBaseURL{}, ErrDeepLinkInvalid
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return PublicWebBaseURL{}, ErrDeepLinkInvalid
	}
	normalizedPath := strings.TrimSuffix(parsed.Path, "/")
	if strings.Contains(parsed.EscapedPath(), "%") || (normalizedPath != "" && path.Clean(normalizedPath) != normalizedPath) {
		return PublicWebBaseURL{}, ErrDeepLinkInvalid
	}
	parsed.Path = normalizedPath
	return PublicWebBaseURL{value: parsed.String()}, nil
}

func (u PublicWebBaseURL) String() string { return u.value }

// ResolveTaskLink is the only supported Gate 0 URL composition path. Callers
// provide relation identities and an allowlisted focus, never a caller-owned
// absolute URL or arbitrary relative path.
func (u PublicWebBaseURL) ResolveTaskLink(taskID, turnID int64, focus Focus) (string, error) {
	if u.value == "" {
		return "", ErrPublicWebURLNotConfigured
	}
	deepLink, err := BuildTaskDeepLink(taskID, turnID, focus)
	if err != nil {
		return "", err
	}
	return u.value + deepLink, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
