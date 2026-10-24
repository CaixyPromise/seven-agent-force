package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeConfigRejectsUnknownAndUnsafeValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown", body: "openAPI:\n  baseURL: http://127.0.0.1:18080/api/\nunknown: true\n"},
		{name: "sse", body: "openAPI:\n  baseURL: http://127.0.0.1:18080/api/\nprotocol:\n  jsonResponseOnly: false\n"},
		{name: "wildcard origin", body: "openAPI:\n  baseURL: http://127.0.0.1:18080/api/\nserver:\n  allowedOrigins: ['*']\n"},
		{name: "body limit above ceiling", body: "openAPI:\n  baseURL: http://127.0.0.1:18080/api/\nprotocol:\n  maxRequestBodyBytes: 262145\n"},
		{name: "output limit above ceiling", body: "openAPI:\n  baseURL: http://127.0.0.1:18080/api/\nlimits:\n  outputTextLimitBytes: 262145\n"},
		{name: "unbounded read timeout", body: "openAPI:\n  baseURL: http://127.0.0.1:18080/api/\nserver:\n  readTimeoutSeconds: 0\n"},
		{name: "write budget below downstream maximum", body: "server:\n  writeTimeoutSeconds: 49\nopenAPI:\n  baseURL: http://127.0.0.1:18080/api/\n  timeoutSeconds: 15\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mcp.yaml")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadRuntimeConfig(path); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestLoadRuntimeConfigDefaultsAreBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.yaml")
	if err := os.WriteFile(path, []byte("openAPI:\n  baseURL: http://127.0.0.1:18080/api/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadRuntimeConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Protocol.Version != ProtocolVersion || config.Protocol.MaxRequestBodyBytes != DefaultMaxBody || config.Limits.OutputTextLimitBytes != DefaultMaxText || config.Limits.IngressConcurrency != 64 {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	if config.Server.ReadTimeoutSeconds != 10 || config.Server.WriteTimeoutSeconds != 60 {
		t.Fatalf("unexpected HTTP timeouts: %+v", config.Server)
	}
}
