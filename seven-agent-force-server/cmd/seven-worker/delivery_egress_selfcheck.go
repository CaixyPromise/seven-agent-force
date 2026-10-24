package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/ids"
)

// deliveryEgressEnforced performs the same network topology that a custom
// Delivery receives.  A Worker advertises the capability only when a request
// through the gateway succeeds *and* a direct connection to the target fails.
// The resources are named with a self-check-only prefix and are cleaned up
// precisely; this never touches user containers or networks.
func deliveryEgressEnforced(ctx context.Context, cfg workforceconfig.Config) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for _, image := range deliveryEgressProbeImages(checkCtx, cfg) {
		if deliveryEgressEnforcedForImage(checkCtx, image) {
			return true
		}
	}
	return false
}

// deliveryEgressProbeImages derives candidate gateway images from the same
// managed-digest allowlist as Worker inventory.  The boolean capability is
// therefore evidence that at least one currently admitted-runtime candidate
// has exercised the Docker topology; the Delivery dispatcher separately
// matches the claimed Harness digest to its image-level probe capability.
func deliveryEgressProbeImages(ctx context.Context, cfg workforceconfig.Config) []string {
	allowed := map[string]struct{}{}
	for _, digest := range cfg.Worker.ManagedImageDigests {
		if value := strings.TrimSpace(digest); value != "" {
			allowed[value] = struct{}{}
		}
	}
	candidates, err := discoverProtocolImages(ctx)
	if err != nil {
		return nil
	}
	images := make([]string, 0, 1)
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate.Role != "attempt" || len(allowed) > 0 && !containsImageDigest(allowed, candidate.Digest) {
			continue
		}
		body, runErr := dockerCheck(ctx, "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/cat", candidate.Ref, "/opt/seven-worker/contract/manifest.json")
		if runErr != nil || len(body) > 1<<20 || !imageDeclaresDeliveryEgress([]byte(body)) {
			continue
		}
		if _, exists := seen[candidate.Ref]; !exists {
			seen[candidate.Ref] = struct{}{}
			images = append(images, candidate.Ref)
		}
	}
	// A Worker configured with an exact custom Harness may have no historic
	// protocol inventory yet (for example first boot after image preload).
	// Prefer it only when it explicitly declares the gateway capability.
	if image := strings.TrimSpace(cfg.Harness.Image); image != "" {
		body, runErr := dockerCheck(ctx, "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/cat", image, "/opt/seven-worker/contract/manifest.json")
		if runErr == nil && len(body) <= 1<<20 && imageDeclaresDeliveryEgress([]byte(body)) {
			if _, exists := seen[image]; !exists {
				images = append(images, image)
			}
		}
	}
	sort.Strings(images)
	return images
}

func containsImageDigest(values map[string]struct{}, digest string) bool {
	_, ok := values[strings.TrimSpace(digest)]
	return ok
}

func imageDeclaresDeliveryEgress(body []byte) bool {
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	if json.Unmarshal(body, &manifest) != nil {
		return false
	}
	for _, capability := range manifest.Capabilities {
		if capability == "delivery.egress-gateway" {
			return true
		}
	}
	return false
}

func deliveryEgressEnforcedForImage(checkCtx context.Context, image string) bool {
	suffix := strings.ToLower(ids.New("egress"))
	internal := "seven-delivery-check-in-" + suffix
	outbound := "seven-delivery-check-out-" + suffix
	target := "seven-delivery-check-target-" + suffix
	gateway := "seven-delivery-check-gateway-" + suffix
	defer func() {
		_, _ = dockerCheck(checkCtx, "rm", "-f", gateway, target)
		_, _ = dockerCheck(checkCtx, "network", "rm", internal, outbound)
	}()
	if _, err := dockerCheck(checkCtx, "network", "create", "--driver", "bridge", outbound); err != nil {
		return false
	}
	if _, err := dockerCheck(checkCtx, "network", "create", "--driver", "bridge", "--internal", internal); err != nil {
		return false
	}
	security := []string{"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--user", "10001:10001", "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m"}
	if _, err := dockerCheck(checkCtx, append([]string{"run", "-d", "--name", target, "--network", outbound}, append(security, "--entrypoint", "/usr/local/bin/seven-worker-cli", image, "delivery-egress-probe-target", "--port", "18080")...)...); err != nil {
		return false
	}
	targetIP, err := dockerCheck(checkCtx, "inspect", "--format", "{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}", target)
	if err != nil || strings.TrimSpace(targetIP) == "" {
		return false
	}
	deadline := time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339)
	gatewayArgs := append([]string{"run", "-d", "--name", gateway, "--network", outbound}, security...)
	gatewayArgs = append(gatewayArgs, "--cap-add", "NET_BIND_SERVICE", "--entrypoint", "/usr/local/bin/seven-worker-cli", image, "delivery-egress-gateway", "--listen-port", "18080", "--target-ip", strings.TrimSpace(targetIP), "--target-port", "18080", "--deadline", deadline)
	if _, err = dockerCheck(checkCtx, gatewayArgs...); err != nil {
		return false
	}
	if _, err = dockerCheck(checkCtx, "network", "connect", "--alias", "allowed.delivery.selfcheck", internal, gateway); err != nil {
		return false
	}
	time.Sleep(150 * time.Millisecond)
	allowedArgs := append([]string{"run", "--rm", "--network", internal}, security...)
	allowedArgs = append(allowedArgs, "--entrypoint", "node", image, "-e", `fetch("http://allowed.delivery.selfcheck:18080").then(r=>process.exit(r.status===204?0:2)).catch(()=>process.exit(1))`)
	if _, err = dockerCheck(checkCtx, allowedArgs...); err != nil {
		return false
	}
	directArgs := append([]string{"run", "--rm", "--network", internal}, security...)
	directArgs = append(directArgs, "--entrypoint", "node", image, "-e", fmt.Sprintf(`fetch("http://%s:18080",{signal:AbortSignal.timeout(1500)}).then(()=>process.exit(2)).catch(()=>process.exit(0))`, strings.TrimSpace(targetIP)))
	_, err = dockerCheck(checkCtx, directArgs...)
	return err == nil
}

func dockerCheck(ctx context.Context, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if len(output) > 4<<10 {
		return "", fmt.Errorf("docker output exceeded bound")
	}
	if err != nil {
		return "", err
	}
	return string(output), nil
}
