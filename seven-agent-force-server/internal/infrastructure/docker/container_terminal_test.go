package docker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestTerminalNormalizeDefaultsAndBounds(t *testing.T) {
	got, err := NormalizeContainerTerminalRequest(ContainerTerminalRequest{Rows: 999, Cols: 999})
	if err != nil {
		t.Fatalf("NormalizeContainerTerminalRequest returned error: %v", err)
	}
	if got.Shell != "/bin/sh" {
		t.Fatalf("default shell = %q, want /bin/sh", got.Shell)
	}
	if got.Rows != maxTerminalRows || got.Cols != maxTerminalCols {
		t.Fatalf("bounds = %dx%d, want %dx%d", got.Cols, got.Rows, maxTerminalCols, maxTerminalRows)
	}
}

func TestTerminalNormalizeAllowsOnlySupportedShells(t *testing.T) {
	for _, shell := range []string{"/bin/sh", "/bin/bash"} {
		if _, err := NormalizeContainerTerminalRequest(ContainerTerminalRequest{Shell: shell}); err != nil {
			t.Fatalf("shell %q should be allowed: %v", shell, err)
		}
	}
	if _, err := NormalizeContainerTerminalRequest(ContainerTerminalRequest{Shell: "/usr/bin/zsh"}); err == nil {
		t.Fatal("unsupported shell should fail")
	}
}

func TestValidateContainerTerminalOrigin(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		origin   string
		patterns []string
		wantErr  bool
	}{
		{name: "missing origin", host: "api.example.com", wantErr: true},
		{name: "malformed origin", host: "api.example.com", origin: "https://", wantErr: true},
		{name: "unsupported origin scheme", host: "api.example.com", origin: "ftp://api.example.com", wantErr: true},
		{name: "same origin", host: "api.example.com", origin: "https://api.example.com", wantErr: false},
		{name: "unlisted cross origin", host: "api.example.com", origin: "https://evil.example.com", wantErr: true},
		{
			name:     "configured cross origin",
			host:     "api.example.com",
			origin:   "https://console.example.com",
			patterns: []string{"https://console.example.com"},
			wantErr:  false,
		},
		{
			name:     "configured cross origin with port",
			host:     "api.example.com",
			origin:   "http://127.0.0.1:5177",
			patterns: []string{"http://127.0.0.1:5177"},
			wantErr:  false,
		},
		{
			name:     "invalid pattern fails closed",
			host:     "api.example.com",
			origin:   "https://console.example.com",
			patterns: []string{"["},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://api.example.com/admin/docker/containers/c1/terminal", nil)
			request.Host = tt.host
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}

			err := validateContainerTerminalOrigin(request, tt.patterns)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateContainerTerminalOrigin() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestServeContainerTerminalRejectsOriginBeforeDockerAccess(t *testing.T) {
	for _, origin := range []string{"", "https://evil.example.com"} {
		name := "missing-origin"
		if origin != "" {
			name = "unlisted-origin"
		}
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://api.example.com/admin/docker/containers/c1/terminal", nil)
			request.Host = "api.example.com"
			if origin != "" {
				request.Header.Set("Origin", origin)
			}
			service := &service{cfg: config.DockerConfig{Enabled: true}}

			err := service.ServeContainerTerminal(context.Background(), httptest.NewRecorder(), request, "container-1", ContainerTerminalRequest{})
			if err == nil || !strings.Contains(err.Error(), "Origin") {
				t.Fatalf("ServeContainerTerminal() error = %v, want Origin rejection before Docker access", err)
			}
		})
	}
}

func TestLogNormalizeDefaultsBoundsAndTimeValidation(t *testing.T) {
	got, err := NormalizeContainerLogQuery(ContainerLogQuery{
		Tail:  999999,
		Since: "2026-06-11T00:00:00Z",
		Until: "2026-06-11T00:01:00Z",
	})
	if err != nil {
		t.Fatalf("NormalizeContainerLogQuery returned error: %v", err)
	}
	if got.Tail != dockerLogStreamMaxTail {
		t.Fatalf("tail = %d, want %d", got.Tail, dockerLogStreamMaxTail)
	}
	if _, err := NormalizeContainerLogQuery(ContainerLogQuery{Since: "2026-06-12T00:00:00Z", Until: "2026-06-11T00:00:00Z"}); err == nil {
		t.Fatal("until before since should fail")
	}
	if _, err := NormalizeContainerLogQuery(ContainerLogQuery{Grep: strings.Repeat("x", dockerLogGrepMaxLength+1)}); err == nil {
		t.Fatal("oversized grep should fail")
	}
}

func TestLogPlainWriterFiltersGrep(t *testing.T) {
	var builder strings.Builder
	writer := &dockerPlainLogWriter{
		service: &service{},
		writer:  &builder,
		grep:    "keep",
	}
	if _, err := writer.Write([]byte("drop this\nkeep this\n")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	if got := builder.String(); got != "keep this\n" {
		t.Fatalf("filtered logs = %q", got)
	}
}

func TestStatsPipeEmitsStatsAndDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	go (&service{}).pipeContainerStats(ctx, cancel, io.NopCloser(strings.NewReader(`{"cpu_stats":{"cpu_usage":{"total_usage":1}}}`)), writer)

	done := make(chan string, 1)
	go func() {
		payload, _ := io.ReadAll(reader)
		done <- string(payload)
	}()

	select {
	case got := <-done:
		if !strings.Contains(got, "event: stats") || !strings.Contains(got, "event: done") {
			t.Fatalf("stats SSE missing events: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stats pipe did not finish")
	}
}
