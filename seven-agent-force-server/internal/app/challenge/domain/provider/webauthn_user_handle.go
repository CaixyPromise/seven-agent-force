package provider

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
)

func canonicalWebAuthnUserHandleForSession(session *domain.ChallengeSession) string {
	if session == nil {
		return ""
	}
	raw := ""
	if session.SubjectUserID > 0 {
		raw = fmt.Sprintf("user:%d", session.SubjectUserID)
	} else {
		raw = strings.TrimSpace(session.SubjectIdentifier)
	}
	if raw == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func webauthnUserHandleMatches(stored, presented string) bool {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return true
	}
	return webauthnCanonicalUserHandleMatches(stored, presented)
}

func webauthnCanonicalUserHandleMatches(expected, presented string) bool {
	expectedBytes, ok := decodeCanonicalWebAuthnUserHandle(expected)
	if !ok {
		return false
	}
	presentedBytes, ok := decodeCanonicalWebAuthnUserHandle(presented)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare(expectedBytes, presentedBytes) == 1
}

func decodeCanonicalWebAuthnUserHandle(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "=+/") {
		return nil, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, false
	}
	return decoded, true
}
