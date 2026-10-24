package domain

import (
	"slices"
	"testing"
)

func TestBuiltInProviderCapabilitiesContainGitHubAndGoogle(t *testing.T) {
	catalog := BuiltInProviderCapabilities()
	if _, ok := catalog["github"]; !ok {
		t.Fatalf("github provider capability missing: %#v", catalog)
	}
	if _, ok := catalog["google"]; !ok {
		t.Fatalf("google provider capability missing: %#v", catalog)
	}
	if !slices.Contains(catalog["google"].Capabilities, CapabilityOIDCLogin) {
		t.Fatalf("google should advertise OIDC_LOGIN: %#v", catalog["google"])
	}
}
