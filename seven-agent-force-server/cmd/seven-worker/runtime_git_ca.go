package main

import (
	"os"
	"path/filepath"
	"strings"
)

// appendRuntimeGitCA carries only operator-configured public trust material
// into Git's otherwise isolated environment. Repository input cannot select
// these paths, and credentials remain confined to GIT_ASKPASS files.
func appendRuntimeGitCA(environment []string) []string {
	seen := map[string]struct{}{}
	for _, name := range []string{"GIT_SSL_CAINFO", "SSL_CERT_FILE"} {
		path := strings.TrimSpace(os.Getenv(name))
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 1<<20 {
			continue
		}
		key := name + "=" + path
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		environment = append(environment, key)
	}
	return environment
}
