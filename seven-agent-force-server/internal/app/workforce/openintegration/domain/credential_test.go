package domain

import "testing"

func TestGenerateAndParseAPIKey(t *testing.T) {
	credentialID, token, digest, hint, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseAPIKey(token)
	if err != nil || parsed != credentialID {
		t.Fatalf("ParseAPIKey()=%q,%v want %q", parsed, err, credentialID)
	}
	if !DigestMatches(token, digest) || hint == "" {
		t.Fatal("generated credential digest or hint is invalid")
	}
	if DigestMatches(token+"x", digest) {
		t.Fatal("modified token matched credential digest")
	}
}

func TestParseAPIKeyRejectsMalformedTokens(t *testing.T) {
	for _, token := range []string{"", "Bearer x", "swk_bad", "swk_000000000000000000000000.short", "swk_zzzzzzzzzzzzzzzzzzzzzzzz.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, err := ParseAPIKey(token); err == nil {
			t.Fatalf("ParseAPIKey(%q) succeeded", token)
		}
	}
}
