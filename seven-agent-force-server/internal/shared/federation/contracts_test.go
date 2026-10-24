package federation

import (
	"strings"
	"testing"
)

func TestCanonicalOIDCIssuerPreservesExactIdentifierAndLimitsLength(t *testing.T) {
	issuer, err := CanonicalOIDCIssuer("  https://hub.example.com/  ", false)
	if err != nil {
		t.Fatalf("canonical issuer: %v", err)
	}
	if issuer != "https://hub.example.com/" {
		t.Fatalf("issuer=%q, want exact trailing slash", issuer)
	}
	tooLong := "https://hub.example.com/" + strings.Repeat("a", OIDCIssuerMaxLength-len("https://hub.example.com/")+1)
	if _, err := CanonicalOIDCIssuer(tooLong, false); err == nil {
		t.Fatal("accepted issuer exceeding storage contract")
	}
	if _, err := CanonicalOIDCIssuer("http://hub.example.com", false); err == nil {
		t.Fatal("accepted HTTP issuer when disabled")
	}
}

func TestCanonicalManagedOwnerEnforcesProviderRepresentationLimit(t *testing.T) {
	owner := strings.Repeat("a", ManagedOwnerMaxLength)
	if got, err := CanonicalManagedOwner(owner); err != nil || got != owner {
		t.Fatalf("maximum owner got=%q err=%v", got, err)
	}
	if _, err := CanonicalManagedOwner(owner + "a"); err == nil {
		t.Fatal("accepted owner exceeding managed provider representation")
	}
}
