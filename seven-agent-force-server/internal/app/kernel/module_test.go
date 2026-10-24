package kernel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestRuntimeFeaturesEndpointReturnsPlatformAndDockerCapabilities(t *testing.T) {
	module := &Module{config: config.Config{
		Platform: config.PlatformConfig{Mode: config.PlatformMode("hub")},
		Docker:   config.DockerConfig{Enabled: false},
	}}
	engine := server.Default()
	module.Mount(engine)

	resp := ut.PerformRequest(engine.Engine, http.MethodGet, "/system/features/runtime", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != 0 {
		t.Fatalf("business code=%d body=%s", body.Code, resp.Body.String())
	}
	featurePayload, ok := body.Data["features"].(map[string]any)
	if !ok {
		t.Fatalf("features payload=%#v", body.Data["features"])
	}
	enabled, ok := featurePayload["enabled"].([]any)
	if !ok || len(enabled) != 2 || enabled[0] != "federation.hub" || enabled[1] != "platform.control" {
		t.Fatalf("features.enabled=%#v", featurePayload["enabled"])
	}
	platform, ok := body.Data["platform"].(map[string]any)
	if !ok {
		t.Fatalf("platform payload=%#v", body.Data["platform"])
	}
	if platform["mode"] != "hub" {
		t.Fatalf("platform mode=%q", platform["mode"])
	}
	assertRuntimeCapabilities(t, platform, true, false, false)
	for _, removedField := range []string{"managementEnabled", "loginPolicySource", "remote"} {
		if _, found := platform[removedField]; found {
			t.Fatalf("platform payload must not include %q: %#v", removedField, platform)
		}
	}
	docker, ok := body.Data["docker"].(map[string]any)
	if !ok || docker["enabled"] != false {
		t.Fatal("docker.enabled should be false")
	}
	notification, ok := body.Data["notification"].(map[string]any)
	if !ok || notification["managedByPlatform"] != false {
		t.Fatal("notification should not be platform managed")
	}
	runtimeLog, ok := body.Data["runtimeLog"].(map[string]any)
	if !ok || runtimeLog["managedByPlatform"] != false {
		t.Fatal("runtimeLog should not be platform managed")
	}
}

func TestRuntimeFeaturesFromConfigDerivesPlatformCapabilities(t *testing.T) {
	for _, tc := range []struct {
		mode                                     string
		controlPlane, federatedHubLogin, nodeAPI bool
	}{
		{mode: "local"},
		{mode: "hub", controlPlane: true},
		{mode: "node", federatedHubLogin: true, nodeAPI: true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			features := RuntimeFeaturesFromConfig(config.Config{
				Platform: config.PlatformConfig{Mode: config.PlatformMode(tc.mode)},
				Docker:   config.DockerConfig{Enabled: true},
			})
			encoded, err := json.Marshal(features)
			if err != nil {
				t.Fatalf("marshal runtime features: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatalf("unmarshal runtime features: %v", err)
			}
			platform := payload["platform"].(map[string]any)
			if platform["mode"] != tc.mode {
				t.Fatalf("platform mode=%q want %q", platform["mode"], tc.mode)
			}
			assertRuntimeCapabilities(t, platform, tc.controlPlane, tc.federatedHubLogin, tc.nodeAPI)
		})
	}
}

func TestRuntimeFeaturesNodePayloadOmitsManagementBearer(t *testing.T) {
	const managementBearer = "node-management-bearer-secret"
	features := RuntimeFeaturesFromConfig(config.Config{
		Platform: config.PlatformConfig{
			Mode: config.PlatformModeNode,
			Node: config.PlatformNodeConfig{
				ManagementBearer: managementBearer,
			},
		},
	})

	encoded, err := json.Marshal(features)
	if err != nil {
		t.Fatalf("marshal runtime features: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, managementBearer) {
		t.Fatalf("runtime payload must not contain management bearer: %s", payload)
	}
	if strings.Contains(payload, "managementBearer") {
		t.Fatalf("runtime payload must not contain managementBearer key: %s", payload)
	}
}

func TestRuntimeFeaturesFromSetUsesSharedCapabilitySetForDockerCompatibilityField(t *testing.T) {
	runtimeFeatures := RuntimeFeaturesFromSet(config.Config{
		Docker: config.DockerConfig{Enabled: true},
	}, features.Set{})

	if runtimeFeatures.Docker.Enabled {
		t.Fatal("docker compatibility field must use the shared feature set")
	}
}

func assertRuntimeCapabilities(t *testing.T, platform map[string]any, controlPlane, federatedHubLogin, nodeAPI bool) {
	t.Helper()
	capabilities, ok := platform["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("platform capabilities=%#v", platform["capabilities"])
	}
	for key, want := range map[string]bool{
		"controlPlane":      controlPlane,
		"federatedHubLogin": federatedHubLogin,
		"nodeApi":           nodeAPI,
	} {
		if got, ok := capabilities[key].(bool); !ok || got != want {
			t.Fatalf("capabilities.%s=%#v want %v", key, capabilities[key], want)
		}
	}
}
