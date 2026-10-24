package imageadmission

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	runtimeconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

func TestAdapterImageDockerAdmission(t *testing.T) {
	image := os.Getenv("BATCH2_ADAPTER_DOCKER_IMAGE")
	if image == "" {
		t.Skip("BATCH2_ADAPTER_DOCKER_IMAGE requires a newly built exact image")
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		t.Fatal("exact image digest required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := Inspect(ctx, runtimeconfig.HarnessConfig{Image: image, Contract: "seven.worker-image/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if report.ImageDigest != image || !containsString(report.Checks, "adapter_handshake") || !containsString(report.Checks, "processor_boundary_conformance") {
		t.Fatal("missing actual admission evidence")
	}
	t.Log("exact image handshake, module identity and isolated Processor execution passed; not native model acceptance")
}
