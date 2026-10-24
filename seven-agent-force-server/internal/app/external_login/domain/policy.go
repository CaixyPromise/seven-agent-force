package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/federation"
)

var providerCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

const ManagedProviderPrefix = "hub:"

func NormalizeProtocolType(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case ProtocolTypeOAuth2, ProtocolTypeOIDC:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid external login protocol type: %s", value)
	}
}

func NormalizeProviderCode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if !providerCodePattern.MatchString(normalized) && !isValidManagedProviderCode(normalized) {
		return "", fmt.Errorf("invalid external login provider code: %s", value)
	}
	return normalized, nil
}

func ManagedProviderCode(ownerNodeCode string) (string, error) {
	owner, err := federation.CanonicalManagedOwner(ownerNodeCode)
	if err != nil {
		return "", fmt.Errorf("invalid managed external login owner node code: %s", ownerNodeCode)
	}
	code := ManagedProviderPrefix + owner
	if len(code) > 64 {
		return "", fmt.Errorf("managed external login provider code exceeds 64 characters")
	}
	return code, nil
}

func IsManagedProviderCode(value string) bool {
	return isValidManagedProviderCode(strings.ToLower(strings.TrimSpace(value)))
}

func isValidManagedProviderCode(value string) bool {
	if !strings.HasPrefix(value, ManagedProviderPrefix) || len(value) > 64 {
		return false
	}
	_, err := federation.CanonicalManagedOwner(strings.TrimPrefix(value, ManagedProviderPrefix))
	return err == nil
}

func CanonicalIssuer(value string) (string, error) {
	return federation.CanonicalOIDCIssuer(value, true)
}

func NormalizeProviderStatus(status int) (int, error) {
	switch status {
	case ProviderStatusActive, ProviderStatusDisabled:
		return status, nil
	default:
		return 0, fmt.Errorf("invalid external login provider status: %d", status)
	}
}

func NormalizeProviderMethodStatus(status int) (int, error) {
	switch status {
	case ProviderMethodStatusActive, ProviderMethodStatusDisabled:
		return status, nil
	default:
		return 0, fmt.Errorf("invalid external login provider method status: %d", status)
	}
}

func NormalizeIdentityStatus(status int) (int, error) {
	switch status {
	case IdentityStatusActive, IdentityStatusDisabled, IdentityStatusUnlinked:
		return status, nil
	default:
		return 0, fmt.Errorf("invalid external identity status: %d", status)
	}
}

func NormalizeLoginStateStatus(status int) (int, error) {
	switch status {
	case LoginStateStatusActive, LoginStateStatusConsumed, LoginStateStatusExpired:
		return status, nil
	default:
		return 0, fmt.Errorf("invalid external login state status: %d", status)
	}
}

func NormalizeTokenStatus(status int) (int, error) {
	switch status {
	case TokenStatusActive, TokenStatusRevoked, TokenStatusExpired, TokenStatusRefreshFailed:
		return status, nil
	default:
		return 0, fmt.Errorf("invalid external oauth token status: %d", status)
	}
}

func NormalizeTokenPurpose(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case TokenPurposeLogin, TokenPurposeAPI:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid external oauth token purpose: %s", value)
	}
}

func ValidateProvider(provider Provider) error {
	if _, err := NormalizeProviderCode(provider.ProviderCode); err != nil {
		return err
	}
	if strings.TrimSpace(provider.ProviderName) == "" {
		return fmt.Errorf("external login provider name is required")
	}
	if _, err := NormalizeProtocolType(provider.ProtocolType); err != nil {
		return err
	}
	if strings.TrimSpace(provider.AuthorizationEndpoint) == "" {
		return fmt.Errorf("external login authorization endpoint is required")
	}
	if strings.TrimSpace(provider.TokenEndpoint) == "" {
		return fmt.Errorf("external login token endpoint is required")
	}
	if strings.TrimSpace(provider.ClientID) == "" {
		return fmt.Errorf("external login client id is required")
	}
	if strings.TrimSpace(provider.RedirectURI) == "" {
		return fmt.Errorf("external login redirect uri is required")
	}
	if strings.TrimSpace(provider.DisplayName) == "" {
		return fmt.Errorf("external login display name is required")
	}
	_, err := NormalizeProviderStatus(provider.Status)
	return err
}

func ValidateProviderMethod(method ProviderMethod) error {
	if _, err := NormalizeProviderCode(method.ProviderCode); err != nil {
		return err
	}
	if strings.TrimSpace(method.MethodKey) == "" {
		return fmt.Errorf("external login provider method key is required")
	}
	if strings.TrimSpace(method.CapabilityCode) == "" {
		return fmt.Errorf("external login provider method capability is required")
	}
	_, err := NormalizeProviderMethodStatus(method.Status)
	return err
}

func ValidateExternalIdentity(identity ExternalIdentity) error {
	if _, err := NormalizeProviderCode(identity.ProviderCode); err != nil {
		return err
	}
	if strings.TrimSpace(identity.ExternalSubject) == "" {
		return fmt.Errorf("external subject is required")
	}
	if identity.UserID <= 0 {
		return fmt.Errorf("external identity user id is required")
	}
	_, err := NormalizeIdentityStatus(identity.Status)
	return err
}

func ValidateOAuthToken(token OAuthToken) error {
	if _, err := NormalizeProviderCode(token.ProviderCode); err != nil {
		return err
	}
	if token.IdentityID <= 0 {
		return fmt.Errorf("external oauth token identity id is required")
	}
	if token.UserID <= 0 {
		return fmt.Errorf("external oauth token user id is required")
	}
	if _, err := NormalizeTokenPurpose(token.TokenPurpose); err != nil {
		return err
	}
	if strings.TrimSpace(token.ScopeHash) == "" {
		return fmt.Errorf("external oauth token scope hash is required")
	}
	_, err := NormalizeTokenStatus(token.Status)
	return err
}
