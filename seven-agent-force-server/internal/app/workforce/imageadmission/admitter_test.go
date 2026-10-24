package imageadmission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexManifestCapabilitiesIncludeProviderInvocationPath(t *testing.T) {
	want := map[string]bool{
		"provider.responses":      false,
		"provider.local-facade":   false,
		"provider.gateway-client": false,
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "deploy", "workforce", "images", "codex-harness-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest contractManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	capabilities, err := probeCapabilities(capabilityProbeFixture(manifest.Capabilities))
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range capabilities {
		if _, ok := want[capability]; ok {
			want[capability] = true
		}
	}
	for capability, found := range want {
		if !found {
			t.Fatalf("missing Codex Provider invocation capability %q", capability)
		}
	}
}

func TestAdapterCapabilitiesAreDeclaredNotInferredFromBrand(t *testing.T) {
	if !validHarnessName("third-party-agent") || validHarnessName("../../agent") {
		t.Fatal("native display identity must not be a two-brand execution switch")
	}
	want := []string{"harness.headless", "completion.proposal", "review.provider.gateway-client"}
	got, err := probeCapabilities(capabilityProbeFixture(want))
	if err != nil || !sameStrings(want, got) {
		t.Fatalf("declared capability projection failed: %v", err)
	}
	for _, invalid := range []any{nil, []string{}, []string{"activity", "activity"}, []string{"../../secret"}, []any{1}} {
		if _, err := probeCapabilities(map[string]any{"capabilities": invalid}); err == nil {
			t.Fatal("invalid capability projection accepted")
		}
	}
}

func capabilityProbeFixture(values []string) map[string]any {
	entries := make([]map[string]string, 0, len(values))
	for _, value := range values {
		entries = append(entries, map[string]string{"name": value, "status": "supported"})
	}
	return map[string]any{"capabilities": entries}
}

func TestReviewerManifestsDeclareMachineCheckedFilesystemContract(t *testing.T) {
	for _, name := range []string{"codex-harness-manifest.json", "claude-harness-manifest.json", "fake-harness-manifest.json"} {
		body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "deploy", "workforce", "images", name))
		if err != nil {
			t.Fatal(err)
		}
		var manifest contractManifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := validateReviewerManifest(manifest, "seven.worker-image/v1"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestReviewerManifestRejectsMissingRunScopedMount(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "deploy", "workforce", "images", "fake-harness-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest contractManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ReviewFilesystem.Report.RunScoped = false
	if err := validateReviewerManifest(manifest, "seven.worker-image/v1"); err == nil {
		t.Fatal("reviewer manifest without run-scoped report volume was accepted")
	}
}

func TestNormalizeSchedulingFreezesCanonicalIdentity(t *testing.T) {
	first, err := normalizeScheduling("production", 100, []string{"OPENAI"}, []string{"shell.posix", "language.node"}, []string{"codex.PostToolUse", "codex.PreToolUse"}, []string{"REVIEW_RUN", "AGENT_ATTEMPT"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeScheduling("PRODUCTION", 100, []string{"OPENAI"}, []string{"language.node", "shell.posix"}, []string{"codex.PreToolUse", "codex.PostToolUse"}, []string{"AGENT_ATTEMPT", "REVIEW_RUN"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == "" || first.Digest != second.Digest || first.Scope != "PRODUCTION" {
		t.Fatalf("scheduling identity is not canonical: first=%+v second=%+v", first, second)
	}
	if !sameStrings(first.HookEvents, []string{"codex.PostToolUse", "codex.PreToolUse"}) {
		t.Fatalf("Hook event identity changed case: %v", first.HookEvents)
	}
	variants := []normalizedScheduling{
		mustNormalizeScheduling(t, "PRODUCTION", 101, []string{"OPENAI"}, []string{"language.node", "shell.posix"}, []string{"codex.PostToolUse", "codex.PreToolUse"}, []string{"AGENT_ATTEMPT", "REVIEW_RUN"}),
		mustNormalizeScheduling(t, "PRODUCTION", 100, []string{"ANTHROPIC"}, []string{"language.node", "shell.posix"}, []string{"codex.PostToolUse", "codex.PreToolUse"}, []string{"AGENT_ATTEMPT", "REVIEW_RUN"}),
		mustNormalizeScheduling(t, "PRODUCTION", 100, []string{"OPENAI"}, []string{"language.node", "shell.posix", "package-manager.pnpm"}, []string{"codex.PostToolUse", "codex.PreToolUse"}, []string{"AGENT_ATTEMPT", "REVIEW_RUN"}),
		mustNormalizeScheduling(t, "PRODUCTION", 100, []string{"OPENAI"}, []string{"language.node", "shell.posix"}, []string{"codex.PostToolUse"}, []string{"AGENT_ATTEMPT", "REVIEW_RUN"}),
		mustNormalizeScheduling(t, "PRODUCTION", 100, []string{"OPENAI"}, []string{"language.node", "shell.posix"}, []string{"codex.PostToolUse", "codex.PreToolUse"}, []string{"AGENT_ATTEMPT"}),
	}
	for _, variant := range variants {
		if variant.Digest == first.Digest {
			t.Fatalf("scheduling digest ignored a routing field: %+v", variant)
		}
	}
	for _, invalid := range []struct {
		scope string
		env   []string
		kinds []string
	}{
		{scope: "", env: []string{"shell.posix"}, kinds: []string{"AGENT_ATTEMPT"}},
		{scope: "PRODUCTION", env: []string{"shell.*"}, kinds: []string{"AGENT_ATTEMPT"}},
		{scope: "TEST_ONLY", env: []string{"shell.posix"}, kinds: []string{"UNKNOWN"}},
		{scope: "TEST_ONLY", env: []string{"shell.posix", "shell.posix"}, kinds: []string{"AGENT_ATTEMPT"}},
	} {
		if _, err = normalizeScheduling(invalid.scope, 100, []string{"OPENAI"}, invalid.env, nil, invalid.kinds); err == nil {
			t.Fatalf("invalid scheduling accepted: %+v", invalid)
		}
	}
}

func mustNormalizeScheduling(t *testing.T, scope string, priority int, protocols, environment, hooks, kinds []string) normalizedScheduling {
	t.Helper()
	value, err := normalizeScheduling(scope, priority, protocols, environment, hooks, kinds)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
