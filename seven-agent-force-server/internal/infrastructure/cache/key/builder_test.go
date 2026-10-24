package key

import (
	"strings"
	"testing"
)

func TestBuilderBuildsKeyWithEscapedParts(t *testing.T) {
	builder := NewBuilder("seven")
	cacheKey := builder.Build("sso", "client", "a/b", 12)

	if cacheKey != "seven:sso:client:a%2Fb:12" {
		t.Fatalf("unexpected cache key: %s", cacheKey)
	}
}

func TestBuilderFallsBackToHashWhenKeyIsTooLong(t *testing.T) {
	builder := NewBuilder("seven")
	builder.maxKeyLength = 24

	cacheKey := builder.Build("sso", "client", strings.Repeat("x", 128))
	if !strings.Contains(cacheKey, ":sha1:") {
		t.Fatalf("expected hashed fallback key, got: %s", cacheKey)
	}
}
