package domain

import (
	"strings"
	"testing"
)

func TestCompletionCheckDigestCompatibility(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("0", 64), strings.Repeat("abcdef0123456789", 4)} {
		if !ValidCompletionCheckDigest(value) {
			t.Fatalf("valid digest rejected: %q", value)
		}
	}
	for _, value := range []string{" ", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64), "sha256:" + strings.Repeat("a", 64), " " + strings.Repeat("a", 64)} {
		if ValidCompletionCheckDigest(value) {
			t.Fatalf("invalid digest accepted: %q", value)
		}
	}
}
