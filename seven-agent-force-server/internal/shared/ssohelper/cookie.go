// Package ssohelper contains neutral SSO token-hash and cookie serialization
// helpers. It contains no repository, cache, or domain-model dependency.
package ssohelper

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func BuildTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func BuildSessionCookie(cfg config.SSOCookieConfig, sessionID string, expiresAt time.Time) string {
	return (&http.Cookie{Name: cfg.Name, Value: sessionID, Path: choosePath(cfg.Path), HttpOnly: true, Secure: cfg.Secure, SameSite: resolveSameSite(cfg.SameSite), Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds())}).String()
}

func BuildExpiredSessionCookie(cfg config.SSOCookieConfig) string {
	return (&http.Cookie{Name: cfg.Name, Value: "", Path: choosePath(cfg.Path), HttpOnly: true, Secure: cfg.Secure, SameSite: resolveSameSite(cfg.SameSite), MaxAge: -1, Expires: time.Unix(0, 0).UTC()}).String()
}

func BuildRefreshCookie(cfg config.SSORefreshCookieConfig, token string, expiresAt time.Time) string {
	return (&http.Cookie{Name: refreshCookieWriteName(cfg), Value: token, Path: choosePath(cfg.Path), HttpOnly: cfg.HTTPOnly, Secure: cfg.Secure, SameSite: resolveSameSite(cfg.SameSite), Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds())}).String()
}

func BuildExpiredRefreshCookies(cfg config.SSORefreshCookieConfig) []string {
	result := make([]string, 0, 2)
	for _, name := range refreshCookieNames(cfg) {
		result = append(result, (&http.Cookie{Name: name, Value: "", Path: choosePath(cfg.Path), HttpOnly: cfg.HTTPOnly, Secure: cfg.Secure, SameSite: resolveSameSite(cfg.SameSite), MaxAge: -1, Expires: time.Unix(0, 0).UTC()}).String())
	}
	return result
}

func choosePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "/"
	}
	return path
}
func resolveSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
func refreshCookieWriteName(cfg config.SSORefreshCookieConfig) string {
	name := strings.TrimSpace(cfg.Name)
	if !cfg.Secure && strings.HasPrefix(name, "__Host-") {
		return strings.TrimPrefix(name, "__Host-")
	}
	return name
}
func refreshCookieNames(cfg config.SSORefreshCookieConfig) []string {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return nil
	}
	write := refreshCookieWriteName(cfg)
	result := []string{write}
	if name != write {
		result = append(result, name)
	}
	return result
}
