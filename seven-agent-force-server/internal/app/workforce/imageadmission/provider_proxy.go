package imageadmission

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"

	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

const ProviderGatewayClientMode = "gateway-client"

type ProviderProxyReport struct {
	ImageRef    string   `json:"imageRef"`
	ImageDigest string   `json:"imageDigest"`
	Version     string   `json:"version"`
	Contract    string   `json:"contract"`
	Protocol    string   `json:"protocol"`
	Manifest    string   `json:"manifest"`
	Checks      []string `json:"checks"`
}

func VerifyProviderProxy(ctx context.Context, config workforceconfig.ProviderProxyConfig) (ProviderProxyReport, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(checkCtx, "docker", "image", "inspect", config.Image).Output()
	if err != nil {
		return ProviderProxyReport{}, err
	}
	var images []struct {
		ID     string `json:"Id"`
		Config struct {
			User         string            `json:"User"`
			Entrypoint   []string          `json:"Entrypoint"`
			ExposedPorts map[string]any    `json:"ExposedPorts"`
			Volumes      map[string]any    `json:"Volumes"`
			Labels       map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal(output, &images) != nil || len(images) != 1 {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_INSPECT_INVALID")
	}
	image := images[0]
	if image.ID != config.ImageDigest {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_DIGEST_MISMATCH")
	}
	if image.Config.User == "" || image.Config.User == "0" || strings.HasPrefix(image.Config.User, "root") {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_ROOT_USER_FORBIDDEN")
	}
	if len(image.Config.ExposedPorts) != 0 || len(image.Config.Volumes) != 0 {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_PORT_OR_VOLUME_FORBIDDEN")
	}
	if len(image.Config.Entrypoint) != 1 || image.Config.Entrypoint[0] != "/usr/local/bin/seven-provider-proxy" {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_ENTRYPOINT_INVALID")
	}
	if image.Config.Labels["io.github.caixypromise.seven-worker.image.contract"] != "seven.worker-image/v1" ||
		image.Config.Labels["io.github.caixypromise.seven-worker.image.role"] != "provider-proxy" ||
		image.Config.Labels["io.github.caixypromise.seven-worker.image.manifest"] != "/opt/seven-worker/contract/manifest.json" {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_IMAGE_CONTRACT_INVALID")
	}
	if image.Config.Labels["io.github.caixypromise.seven-worker.provider-gateway.mode"] != ProviderGatewayClientMode || image.Config.Labels["io.github.caixypromise.seven-worker.version"] != config.Version {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_PROTOCOL_OR_VERSION_MISMATCH")
	}
	versionOutput, err := exec.CommandContext(checkCtx, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", config.Image, "--version").Output()
	if err != nil || !strings.Contains(string(versionOutput), config.Version) {
		return ProviderProxyReport{}, errors.New("PROVIDER_PROXY_VERSION_PROBE_FAILED")
	}
	return ProviderProxyReport{ImageRef: config.Image, ImageDigest: image.ID, Version: config.Version, Contract: "seven.worker-image/v1", Protocol: ProviderGatewayClientMode, Manifest: "/opt/seven-worker/contract/manifest.json", Checks: []string{"image.inspect", "digest", "non_root", "no_exposed_ports", "no_declared_volumes", "entrypoint", "image_contract", "manifest", "gateway_client_mode", "isolated_version_probe"}}, nil
}
