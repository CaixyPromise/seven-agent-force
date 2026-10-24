package file

import (
	"testing"
)

func TestWithContextPathPrefixesDownloadGateway(t *testing.T) {
	if got := withContextPath("/api", "/file/download"); got != "/api/file/download" {
		t.Fatalf("expected context-prefixed gateway, got %q", got)
	}
	if got := withContextPath("/api", "/api/file/download"); got != "/api/file/download" {
		t.Fatalf("expected already-prefixed gateway unchanged, got %q", got)
	}
	if got := withContextPath("", "/file/download"); got != "/file/download" {
		t.Fatalf("expected root gateway unchanged, got %q", got)
	}
}
