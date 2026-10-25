package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAcceptsWorkerRuntimeConfigWithoutDatabaseOrSharedQueue(t *testing.T) {
	t.Parallel()

	cfg := Config{
		RabbitMQ: RabbitMQConfig{
			Enabled: true, Host: "127.0.0.1", Port: 5672, Username: "guest",
			Prefetch: 1,
		},
		Worker: WorkerConfig{
			ID: 2568667825, ServerBaseURL: "http://127.0.0.1:9277",
			TokenFile: "/tmp/seven-worker.token", MaxActiveContainers: 1, MaxActiveReviews: 1, MaxTotalContainers: 7,
			WorkspaceRoot: "/tmp/seven-workforce-test",
		},
		Harness: HarnessConfig{Alias: "fake-local-v1", Image: "seven-workforce/fake-harness:test", Contract: "seven.worker-image/v1"},
		ProviderSSRF: ProviderSSRFConfig{
			Enabled: true, MaxRequestBytes: 128 << 20, MaxResponseHeaderBytes: 256 << 10,
			RequestTimeout: 10 * time.Minute, ProbeTimeout: 2 * time.Minute, ResponseHeaderTimeout: 2 * time.Minute, TLSHandshakeTimeout: 30 * time.Second,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsMissingWorkerDependency(t *testing.T) {
	t.Parallel()

	if err := (Config{}).Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing dependency error")
	}
}

func TestValidateRejectsUnsafeOperationDNSResolver(t *testing.T) {
	cfg := Config{
		RabbitMQ:     RabbitMQConfig{Enabled: true, Host: "127.0.0.1", Port: 5672, Username: "guest", Prefetch: 1},
		Worker:       WorkerConfig{ID: 2568667825, ServerBaseURL: "http://127.0.0.1:9277", TokenFile: "/tmp/seven-worker.token", MaxActiveContainers: 1, MaxActiveReviews: 1, MaxTotalContainers: 7, WorkspaceRoot: "/tmp/seven-workforce-test", OperationDNSOverHTTPSURL: "http://resolver.example/dns-query"},
		Harness:      HarnessConfig{Alias: "fake-local-v1", Image: "seven-workforce/fake-harness:test", Contract: "seven.worker-image/v1"},
		ProviderSSRF: ProviderSSRFConfig{Enabled: true, MaxRequestBytes: 128 << 20, MaxResponseHeaderBytes: 256 << 10, RequestTimeout: 10 * time.Minute, ProbeTimeout: 2 * time.Minute, ResponseHeaderTimeout: 2 * time.Minute, TLSHandshakeTimeout: 30 * time.Second},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an unauthenticated operation DNS resolver")
	}
}

func TestValidateRejectsRelativeOrFilesystemRootWorkspace(t *testing.T) {
	t.Parallel()

	base := Config{
		Worker:       WorkerConfig{ID: 2568667825, ServerBaseURL: "http://127.0.0.1:9277", TokenFile: "/tmp/seven-worker.token", MaxActiveContainers: 1, MaxActiveReviews: 1, MaxTotalContainers: 7, WorkspaceRoot: "/tmp/seven-workforce-test"},
		Harness:      HarnessConfig{Alias: "fake-local-v1", Image: "seven-workforce/fake-harness:test", Contract: "seven.worker-image/v1"},
		ProviderSSRF: ProviderSSRFConfig{Enabled: true, MaxRequestBytes: 128 << 20, MaxResponseHeaderBytes: 256 << 10, RequestTimeout: 10 * time.Minute, ProbeTimeout: 2 * time.Minute, ResponseHeaderTimeout: 2 * time.Minute, TLSHandshakeTimeout: 30 * time.Second},
	}
	for _, root := range []string{"relative/workspaces", "/"} {
		cfg := base
		cfg.Worker.WorkspaceRoot = root
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate() with workspaceRoot=%q error = nil", root)
		}
	}
}

func TestValidateRejectsFloatingProviderProxyImage(t *testing.T) {
	t.Parallel()

	base := Config{
		Worker:        WorkerConfig{ID: 2568667825, ServerBaseURL: "http://127.0.0.1:9277", TokenFile: "/tmp/seven-worker.token", MaxActiveContainers: 1, MaxActiveReviews: 1, MaxTotalContainers: 7, WorkspaceRoot: "/tmp/seven-workforce-test"},
		Harness:       HarnessConfig{Alias: "fake-local-v1", Image: "seven-workforce/fake-harness:test", Contract: "seven.worker-image/v1"},
		ProviderSSRF:  ProviderSSRFConfig{Enabled: true, MaxRequestBytes: 128 << 20, MaxResponseHeaderBytes: 256 << 10, RequestTimeout: 10 * time.Minute, ProbeTimeout: 2 * time.Minute, ResponseHeaderTimeout: 2 * time.Minute, TLSHandshakeTimeout: 30 * time.Second},
		ProviderProxy: ProviderProxyConfig{Image: "seven-workforce/provider-proxy:latest", ImageDigest: "sha256:" + strings.Repeat("a", 64), Version: "slice7", ControlRoot: "/tmp/provider-proxy"},
	}
	if err := base.Validate(); err == nil {
		t.Fatal("Validate() accepted a floating Provider Proxy image")
	}

	base.ProviderProxy.Image = "seven-workforce/provider-proxy:slice7@sha256:" + strings.Repeat("a", 64)
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() rejected an exact Provider Proxy image: %v", err)
	}
}

func TestValidateProxySeparatesManifestReferenceFromDockerConfigIdentity(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	if !exactImageReference(digest, digest) {
		t.Fatal("immutable local Docker image ID rejected")
	}
	if !exactImageReference("registry.example.invalid/proxy@sha256:"+strings.Repeat("b", 64), digest) {
		t.Fatal("OCI manifest and Docker config IDs are distinct identities")
	}
	for _, ref := range []string{"proxy:latest", "proxy@sha256:short", "proxy@sha256:" + strings.Repeat("B", 64)} {
		if exactImageReference(ref, digest) {
			t.Fatalf("unqualified image accepted: %s", ref)
		}
	}
}
