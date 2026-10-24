package federation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	OIDCIssuerMaxLength   = 512
	ManagedOwnerMaxLength = 60
)

var managedOwnerPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,59}$`)

func CanonicalOIDCIssuer(value string, allowHTTP bool) (string, error) {
	issuer := strings.TrimSpace(value)
	if issuer == "" || len(issuer) > OIDCIssuerMaxLength {
		return "", fmt.Errorf("OIDC issuer is required and must not exceed %d bytes", OIDCIssuerMaxLength)
	}
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid OIDC issuer: %s", value)
	}
	if parsed.Scheme != "https" && (!allowHTTP || parsed.Scheme != "http") {
		return "", fmt.Errorf("invalid OIDC issuer scheme: %s", parsed.Scheme)
	}
	if parsed.String() != issuer {
		return "", fmt.Errorf("OIDC issuer must use its exact validated URL representation")
	}
	return issuer, nil
}

func CanonicalManagedOwner(value string) (string, error) {
	owner := strings.TrimSpace(value)
	if len(owner) > ManagedOwnerMaxLength || !managedOwnerPattern.MatchString(owner) {
		return "", fmt.Errorf("invalid managed owner: %s", value)
	}
	return owner, nil
}
