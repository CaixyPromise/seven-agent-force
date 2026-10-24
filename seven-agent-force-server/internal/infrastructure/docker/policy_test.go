package docker

import (
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestEvaluatePolicyProfiles(t *testing.T) {
	command := ContainerCreateRequest{
		ImageReference: "unknown.local/root/app:latest",
		NetworkMode:    "host",
		Privileged:     true,
		VolumeBindings: []VolumeBindingCommand{
			{Type: "bind", Source: "/var/run/docker.sock", Target: "/var/run/docker.sock"},
		},
		Environment: []KeyValueCommand{{Key: "DB_PASSWORD", Value: "secret"}},
	}

	compatible := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "compatible"}, OperationActor{}, OperationTypeContainerCreate, command)
	if len(compatible.Violations) != 0 || len(compatible.Warnings) == 0 {
		t.Fatalf("compatible should warn only, got warnings=%d violations=%d", len(compatible.Warnings), len(compatible.Violations))
	}

	safe := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "safe"}, OperationActor{}, OperationTypeContainerCreate, command)
	if len(safe.Violations) == 0 {
		t.Fatalf("safe should deny dangerous container create")
	}

	opsActor := OperationActor{Permissions: []string{permissionDockerDangerous}}
	safeOps := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "safe"}, opsActor, OperationTypeContainerCreate, command)
	if len(safeOps.Violations) != 0 || len(safeOps.Warnings) == 0 {
		t.Fatalf("safe with dangerous permission should warn only, got warnings=%d violations=%d", len(safeOps.Warnings), len(safeOps.Violations))
	}

	locked := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "locked-down"}, opsActor, OperationTypeContainerCreate, command)
	if len(locked.Violations) == 0 {
		t.Fatalf("locked-down should still deny without override")
	}

	override := OperationActor{Permissions: []string{permissionDockerPolicyOverride}}
	lockedOverride := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "locked-down"}, override, OperationTypeContainerCreate, command)
	if len(lockedOverride.Violations) != 0 {
		t.Fatalf("locked-down override should warn only, got violations=%d", len(lockedOverride.Violations))
	}

	adminWithoutPermission := OperationActor{IsAdmin: true}
	adminDecision := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "safe"}, adminWithoutPermission, OperationTypeContainerCreate, command)
	if len(adminDecision.Violations) == 0 {
		t.Fatalf("policy must use permission codes, not IsAdmin shortcut")
	}
}

func TestSanitizePayloadJSONMasksSecrets(t *testing.T) {
	payload := []byte(`{"environment":[{"key":"API_TOKEN","value":"raw-token"},{"key":"DISPLAY_NAME","value":"public-name"}],"password":"plain","configKey":"payment.gateway.secret","configValue":"plain-secret-updated","isSensitive":1,"composeYaml":"services:\n  app:\n    environment:\n      DB_PASSWORD: raw-password\n"}`)
	masked := string(sanitizePayloadJSON(payload, config.DockerSecurityConfig{}))
	if masked == string(payload) || containsAny(masked, "plain", "raw-token", "raw-password", "plain-secret-updated", `"isSensitive":1`) {
		t.Fatalf("payload should be masked, got %s", masked)
	}
	if !strings.Contains(masked, `"configKey":"payment.gateway.secret"`) || !strings.Contains(masked, `"value":"public-name"`) {
		t.Fatalf("non-secret docker payload metadata should remain visible, got %s", masked)
	}
}

func TestSanitizeTextBySensitiveKeysMasksConfigAssignments(t *testing.T) {
	masked := sanitizeTextBySensitiveKeys("configKey=payment.gateway configValue=plain-secret-updated isSensitive=1", config.DockerSecurityConfig{})
	for _, leaked := range []string{"plain-secret-updated", "isSensitive=1"} {
		if strings.Contains(masked, leaked) {
			t.Fatalf("docker operation message leaked %q in %s", leaked, masked)
		}
	}
	if !strings.Contains(masked, "configKey=payment.gateway") {
		t.Fatalf("configKey should remain visible in %s", masked)
	}
}

func TestInspectComposePolicyDetectsYamlVariants(t *testing.T) {
	command := ComposeUpRequest{ComposeYaml: `
services:
  app:
    image: unknown.local/root/app:latest
    privileged : "yes"
    network_mode : "host"
    pid: host
    ipc: host
    environment:
      DB_PASSWORD: raw
    labels:
      api.token: raw
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - type: bind
        source: /etc
        target: /host-etc
`}
	decision := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "safe"}, OperationActor{}, OperationTypeComposeUp, command)
	if len(decision.Violations) < 5 {
		t.Fatalf("expected compose policy to detect quoted/spacing variants, got %+v", decision)
	}
}

func TestComposeValidateReportsButDoesNotDeny(t *testing.T) {
	command := ComposeUpRequest{ComposeYaml: `
services:
  app:
    image: unknown.local/root/app:latest
    privileged: true
`}
	decision := evaluatePolicy(config.DockerSecurityConfig{PolicyProfile: "safe"}, OperationActor{}, OperationTypeComposeValidate, command)
	if len(decision.Violations) != 0 || len(decision.Warnings) == 0 {
		t.Fatalf("compose validate should report warnings without denial, got %+v", decision)
	}
}

func TestCleanupTokenIsDeterministicAndResourceBound(t *testing.T) {
	left := cleanupToken("image", []string{"b", "a"})
	right := cleanupToken("image", []string{"a", "b"})
	if left != right {
		t.Fatalf("cleanup token should ignore resource order: %s != %s", left, right)
	}
	if err := requireCleanupToken("image", []string{"a", "b"}, left); err != nil {
		t.Fatalf("expected cleanup token to validate: %v", err)
	}
	if err := requireCleanupToken("image", []string{"a", "c"}, left); err == nil {
		t.Fatalf("expected cleanup token mismatch to fail")
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
