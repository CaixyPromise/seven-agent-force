package domain

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	trustedFetchSites = map[string]struct{}{
		"same-origin": {},
		"same-site":   {},
		"none":        {},
	}
	trustedFetchModes = map[string]struct{}{
		"cors":        {},
		"same-origin": {},
		"navigate":    {},
	}
)

type OriginCheckInput struct {
	Origin                string
	Referer               string
	SecFetchSite          string
	SecFetchMode          string
	AllowedOriginPatterns []string
	RequireOriginHeader   bool
}

func ValidateOrigin(input OriginCheckInput) bool {
	requestOrigin := strings.TrimSpace(input.Origin)
	if requestOrigin == "" {
		requestOrigin = strings.TrimSpace(input.Referer)
	}
	if requestOrigin == "" {
		if input.RequireOriginHeader {
			return false
		}
		return isTrustedBrowserFetch(input.SecFetchSite, input.SecFetchMode)
	}
	normalized := normalizeOrigin(requestOrigin)
	if normalized == "" || len(input.AllowedOriginPatterns) == 0 {
		return false
	}
	for _, pattern := range input.AllowedOriginPatterns {
		if wildcardMatch(strings.TrimSpace(pattern), normalized) {
			return true
		}
	}
	return false
}

func isTrustedBrowserFetch(site, mode string) bool {
	site = strings.ToLower(strings.TrimSpace(site))
	if site == "" {
		return false
	}
	if _, ok := trustedFetchSites[site]; !ok {
		return false
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return true
	}
	_, ok := trustedFetchModes[mode]
	return ok
}

func normalizeOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return ""
	}
	if port := parsed.Port(); port != "" {
		return strings.ToLower(parsed.Scheme) + "://" + parsed.Hostname() + ":" + port
	}
	return strings.ToLower(parsed.Scheme) + "://" + parsed.Hostname()
}

func wildcardMatch(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	expr := "^" + regexp.QuoteMeta(pattern) + "$"
	expr = strings.ReplaceAll(expr, "\\*", ".*")
	ok, _ := regexp.MatchString(expr, value)
	return ok
}
