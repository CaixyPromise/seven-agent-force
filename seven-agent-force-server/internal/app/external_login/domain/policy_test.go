package domain

import "testing"

func TestNormalizeProtocolTypeRejectsUnknownProtocol(t *testing.T) {
	_, err := NormalizeProtocolType("SAML")
	if err == nil {
		t.Fatal("expected invalid protocol error")
	}
}

func TestNormalizeProviderCodeTrimsAndLowercases(t *testing.T) {
	got, err := NormalizeProviderCode(" GitHub ")
	if err != nil {
		t.Fatalf("NormalizeProviderCode returned error: %v", err)
	}
	if got != "github" {
		t.Fatalf("expected github, got %q", got)
	}
}

func TestNormalizeIdentityStatusAllowsUnlinked(t *testing.T) {
	got, err := NormalizeIdentityStatus(IdentityStatusUnlinked)
	if err != nil {
		t.Fatalf("NormalizeIdentityStatus returned error: %v", err)
	}
	if got != IdentityStatusUnlinked {
		t.Fatalf("expected %d, got %d", IdentityStatusUnlinked, got)
	}
}

func TestValidateOAuthTokenRequiresScopeHash(t *testing.T) {
	err := ValidateOAuthToken(OAuthToken{
		ProviderCode: "github",
		IdentityID:   1,
		UserID:       2,
		TokenPurpose: TokenPurposeAPI,
		Status:       TokenStatusActive,
	})
	if err == nil {
		t.Fatal("expected missing scope hash error")
	}
}
