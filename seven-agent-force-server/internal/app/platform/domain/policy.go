package domain

import (
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
)

func VisibleLoginMethods(methods []LoginMethod) []LoginMethod {
	out := make([]LoginMethod, 0, len(methods))
	for _, method := range methods {
		if method.DisplayEnabled && method.LoginEnabled {
			out = append(out, method)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].DisplayName < out[j].DisplayName
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	return out
}

func MatchPlatformCode(rules []SourceRule, source RequestSource) (string, bool) {
	ordered := append([]SourceRule(nil), rules...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftRank := sourceTrustRank(ordered[i].MatchType)
		rightRank := sourceTrustRank(ordered[j].MatchType)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if ordered[i].Priority == ordered[j].Priority {
			return ordered[i].PlatformCode < ordered[j].PlatformCode
		}
		return ordered[i].Priority > ordered[j].Priority
	})
	for _, rule := range ordered {
		if rule.Status != StatusActive {
			continue
		}
		if sourceMatches(rule, source) {
			return strings.TrimSpace(rule.PlatformCode), true
		}
	}
	return "", false
}

func sourceMatches(rule SourceRule, source RequestSource) bool {
	value := strings.TrimSpace(rule.MatchValue)
	switch rule.MatchType {
	case MatchClientID:
		return value != "" && value == strings.TrimSpace(source.ClientID)
	case MatchHost:
		return value != "" && normalizeHost(value) == normalizeHost(source.Host)
	case MatchOrigin:
		return value != "" && normalizeOrigin(value) == normalizeOrigin(source.Origin)
	case MatchRefererHost:
		return value != "" && hostOf(source.Referer) == normalizeHost(value)
	case MatchRedirectHost:
		return value != "" && hostOf(source.RedirectURL) == normalizeHost(value)
	case MatchRedirectPrefix:
		return value != "" && redirectPrefixMatches(strings.TrimSpace(source.RedirectURL), value)
	default:
		return false
	}
}

func sourceTrustRank(matchType string) int {
	switch matchType {
	case MatchClientID:
		return 600
	case MatchRedirectHost, MatchRedirectPrefix:
		return 500
	case MatchOrigin:
		return 300
	case MatchRefererHost:
		return 200
	case MatchHost:
		return 100
	default:
		return 0
	}
}

func redirectPrefixMatches(rawRedirect string, rawPrefix string) bool {
	redirectURL, err := url.Parse(strings.TrimSpace(rawRedirect))
	if err != nil || redirectURL.Scheme == "" || redirectURL.Host == "" {
		return false
	}
	prefixURL, err := url.Parse(strings.TrimSpace(rawPrefix))
	if err != nil || prefixURL.Scheme == "" || prefixURL.Host == "" {
		return false
	}
	if !strings.EqualFold(redirectURL.Scheme, prefixURL.Scheme) ||
		normalizeURLHost(redirectURL.Scheme, redirectURL.Host) != normalizeURLHost(prefixURL.Scheme, prefixURL.Host) {
		return false
	}
	prefixPath, ok := canonicalPath(prefixURL)
	if !ok {
		return false
	}
	redirectPath, ok := canonicalPath(redirectURL)
	if !ok {
		return false
	}
	if prefixPath == "" {
		return true
	}
	return redirectPath == prefixPath || strings.HasPrefix(redirectPath, prefixPath+"/")
}

func canonicalPath(parsed *url.URL) (string, bool) {
	if hasDotPathSegmentAfterDecode(parsed.EscapedPath(), 2) {
		return "", false
	}
	unescaped, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", false
	}
	cleaned := path.Clean("/" + strings.TrimLeft(unescaped, "/"))
	if cleaned == "." || cleaned == "/" {
		return "", true
	}
	return strings.TrimRight(cleaned, "/"), true
}

func normalizeOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.ToLower(strings.TrimRight(strings.TrimSpace(raw), "/"))
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func normalizeHost(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeURLHost(scheme string, rawHost string) string {
	host := normalizeHost(rawHost)
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	if (strings.EqualFold(scheme, "https") && port == "443") || (strings.EqualFold(scheme, "http") && port == "80") {
		return name
	}
	return host
}

func hostOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return normalizeHost(parsed.Host)
}

func hasDotPathSegmentAfterDecode(raw string, maxDecodePasses int) bool {
	current := raw
	for pass := 0; pass <= maxDecodePasses; pass++ {
		for _, segment := range strings.Split(current, "/") {
			if segment == "." || segment == ".." {
				return true
			}
		}
		if pass == maxDecodePasses {
			break
		}
		next, err := url.PathUnescape(current)
		if err != nil || next == current {
			break
		}
		current = next
	}
	return false
}
