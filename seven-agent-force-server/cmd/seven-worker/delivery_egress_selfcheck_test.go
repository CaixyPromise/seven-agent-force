package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestDockerDeliveryEgressEnforcement is an opt-in live Docker proof for the
// topology used by a networked custom delivery: the internal runner reaches
// the pinned gateway alias but cannot reach the target IP directly. It creates
// only seven-delivery-check-* resources and the implementation cleans every
// one of them before it returns.
func TestDockerDeliveryEgressEnforcement(t *testing.T) {
	if os.Getenv("SEVEN_WORKFORCE_DOCKER_EGRESS_E2E") != "1" {
		t.Skip("set SEVEN_WORKFORCE_DOCKER_EGRESS_E2E=1 to run the Docker egress enforcement proof")
	}
	image := os.Getenv("SEVEN_WORKFORCE_DOCKER_EGRESS_IMAGE")
	if image == "" {
		image = "seven-workforce/codex-harness:0.4.1-collaboration-live"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if !deliveryEgressEnforcedForImage(ctx, image) {
		t.Fatalf("Docker egress proof failed for %q", image)
	}
}
