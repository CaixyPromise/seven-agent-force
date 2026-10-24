package domain

import (
	"strings"
	"testing"
)

func TestParseRepositoryURLSeparatesCredentialAndSafeAddress(t *testing.T) {
	item, err := ParseRepositoryURL("https://seven:glpat-X%2Fabc@gitlab.example.com:8443/tpa/claims/2c-frontend.git")
	if err != nil {
		t.Fatal(err)
	}
	if item.Scheme != "https" || item.Host != "gitlab.example.com" || item.Port != 8443 || item.RepositoryPath != "tpa/claims/2c-frontend.git" {
		t.Fatalf("unexpected target: %+v", item)
	}
	if item.Credential.Username != "seven" || item.Credential.Token != "glpat-X/abc" {
		t.Fatal("credential was not decoded")
	}
	if strings.Contains(item.DisplayURL, "seven") || strings.Contains(item.DisplayURL, "glpat") || strings.Contains(item.ConfigDigest, "glpat") {
		t.Fatal("safe projection leaked credential")
	}
}

func TestParseRepositoryURLRecognizesGitHub(t *testing.T) {
	item, err := ParseRepositoryURL("https://x:y@github.com/CaixyPromise/example.git")
	if err != nil {
		t.Fatal(err)
	}
	if item.ProviderType != RepositoryProviderGitHub || item.DisplayURL != "https://github.com/CaixyPromise/example.git" {
		t.Fatalf("unexpected result: %+v", item)
	}
}

func TestParseRepositoryURLAllowsPublicReadAddress(t *testing.T) {
	item, err := ParseRepositoryURL("http://git.example.com/group/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if item.Credential != (RepositoryCredential{}) || item.DisplayURL != "http://git.example.com/group/repo.git" {
		t.Fatalf("unexpected result: %+v", item)
	}
}

func TestParseRepositoryURLRejectsUnsafeShapes(t *testing.T) {
	values := []string{
		"ssh://git@example.com/group/repo.git",
		"https://example.com",
		"https://example.com/repo.git?token=x",
		"https://example.com/repo.git#fragment",
		"https://user@example.com/repo.git",
		" https://user:token@example.com/repo.git",
		"https://user:token@example.com/../repo.git",
		"https://user:to%0Aken@example.com/repo.git",
	}
	for _, value := range values {
		if _, err := ParseRepositoryURL(value); err == nil {
			t.Fatalf("expected rejection for %q", value)
		}
	}
}
