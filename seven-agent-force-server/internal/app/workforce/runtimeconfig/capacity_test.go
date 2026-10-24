package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequiredContainerBudget(t *testing.T) {
	for _, test := range []struct{ a, r, d, want int }{{1, 1, 0, 7}, {1, 1, 1, 7}, {1, 1, 2, 9}, {2, 1, 0, 10}, {40, 3, 1, 128}, {int(^uint(0) >> 1), 1, 0, 129}} {
		if got := RequiredContainerBudget(test.a, test.r, test.d); got != test.want {
			t.Fatalf("budget(%d,%d,%d)=%d want %d", test.a, test.r, test.d, got, test.want)
		}
	}
}

func TestWorkerLoaderCapacityDefaultAndExplicitValues(t *testing.T) {
	for _, test := range []struct {
		name, total string
		want        int
		valid       bool
	}{{"default", "", 7, true}, {"explicit zero", "    maxTotalContainers: 0\n", 0, false}, {"old two", "    maxTotalContainers: 2\n", 2, false}, {"agent review only five", "    maxTotalContainers: 5\n", 5, false}, {"seven", "    maxTotalContainers: 7\n", 7, true}, {"above maximum", "    maxTotalContainers: 129\n", 129, false}} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			body := "workforce:\n  enabled: true\n  worker:\n    id: 1\n    serverBaseURL: http://127.0.0.1:9277\n    tokenFile: /tmp/worker.token\n    workspaceRoot: /tmp/workspace\n    maxActiveContainers: 1\n" + test.total + "  harness:\n    alias: test\n    image: test-image\n    contract: seven.worker-image/v1\n"
			if err := os.WriteFile(filepath.Join(root, "application.yaml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadWorker(root)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Worker.MaxTotalContainers != test.want || cfg.Worker.MaxActiveContainers != 1 || cfg.Worker.MaxActiveReviews != 1 || cfg.Worker.MaxActiveDeliveries != 0 {
				t.Fatalf("changed logical concurrency or explicit budget: %+v", cfg.Worker)
			}
			cfg.ProviderSSRF = ProviderSSRFConfig{MaxRequestBytes: 1, MaxResponseHeaderBytes: 1, RequestTimeout: time.Second, ProbeTimeout: time.Second, ResponseHeaderTimeout: time.Second, TLSHandshakeTimeout: time.Second}
			err = cfg.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
			if err != nil && !strings.Contains(err.Error(), "worker.maxTotalContainers") {
				t.Fatal(err)
			}
			if !test.valid && test.want < 7 && !strings.Contains(err.Error(), fmt.Sprintf("at least %d", 7)) {
				t.Fatal("missing necessary budget", err)
			}
		})
	}
}
