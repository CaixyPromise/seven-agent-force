package infrastructure

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	projectdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
)

func compatibleManifest() runtimeManifest {
	var m runtimeManifest
	m.Adapter.ID, m.Adapter.Version = "test", "1"
	m.Adapter.Protocol.Name, m.Adapter.Protocol.Min, m.Adapter.Protocol.Max = "seven.agent-adapter", 1, 1
	m.Adapter.Capabilities = []string{"interaction.request", "interaction.respond", "user.message", "execution.suspend", "execution.resume", "tool.request", "tool.result"}
	m.ResultProcessor.PythonRuntimeIdentity = strings.Repeat("a", 64)
	m.ResultProcessor.BootstrapDigest = strings.Repeat("b", 64)
	m.ResultProcessor.SDKDigest = strings.Repeat("c", 64)
	m.ResultProcessor.ModuleIndexDigest = strings.Repeat("d", 64)
	return m
}

type compatibilityProjectRepository struct{ projectapp.Repository }

func (compatibilityProjectRepository) Get(context.Context, int64, int64) (projectdomain.Project, error) {
	return projectdomain.Project{ID: 1, VersionID: 2, Status: "ACTIVE", Kind: "GENERAL", DeliveryMode: "NONE"}, nil
}

func compiledProjectRequirements(t *testing.T) projectfacade.ExecutionRequirements {
	t.Helper()
	var compiler projectfacade.RequirementsCompiler = projectapp.New(compatibilityProjectRepository{})
	compiled, err := compiler.CompileExecutionRequirements(context.Background(), 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	return compiled.Requirements
}

// Source manifests contain build placeholders. Only the four Result Processor
// identities read by routing are materialized with deterministic test digests;
// Adapter capabilities, protocol, and scheduling facts remain source-derived.
func sourceCatalogRelease(t *testing.T, name string) catalogRelease {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate source manifest")
	}
	path := filepath.Join(filepath.Dir(filename), "../../../../../../deploy/workforce/images", name+"-harness-manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var release catalogRelease
	if err = json.Unmarshal(raw, &release.manifest); err != nil {
		t.Fatal(err)
	}
	if validRuntimeManifest(release.manifest) {
		t.Fatal("expected unresolved source digest placeholders")
	}
	for _, placeholder := range []string{"__PYTHON_RUNTIME_IDENTITY__", "__RESULT_BOOTSTRAP_SHA256__", "__RESULT_SDK_SHA256__", "__RESULT_MODULE_INDEX_SHA256__"} {
		if !strings.Contains(string(raw), placeholder) {
			t.Fatalf("missing source build placeholder %s", placeholder)
		}
		raw = []byte(strings.ReplaceAll(string(raw), placeholder, strings.Repeat("a", 64)))
	}
	var source struct {
		Contract       string   `json:"contract"`
		ExecutionKinds []string `json:"executionKinds"`
		Scheduling     struct {
			Scope       string   `json:"scope"`
			Environment []string `json:"environmentCapabilities"`
		} `json:"scheduling"`
	}
	if err = json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &release.manifest); err != nil {
		t.Fatal(err)
	}
	release.status, release.scope, release.contract = "ACTIVE", source.Scheduling.Scope, source.Contract
	release.imageDigest = "sha256:" + strings.Repeat("a", 64)
	release.executionKinds, release.environment = source.ExecutionKinds, source.Scheduling.Environment
	release.protocols, release.hookEvents = release.manifest.Scheduling.ProviderProtocols, release.manifest.Scheduling.HookEvents
	return release
}

func TestSourceCodexClaudeManifestCompatibility(t *testing.T) {
	requirements := compiledProjectRequirements(t)
	for _, tc := range []struct{ name, protocol, otherProtocol string }{
		{"codex", "OPENAI", "ANTHROPIC"}, {"claude", "ANTHROPIC", "OPENAI"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := sourceCatalogRelease(t, tc.name)
			if !compatibleTaskRuntime(release, requirements, release.hookEvents, tc.protocol) {
				t.Fatal("source manifest rejects Project-compiled requirements")
			}
			if compatibleTaskRuntime(release, requirements, nil, tc.otherProtocol) {
				t.Fatal("source manifest accepts wrong provider protocol")
			}
			for _, capability := range []string{"interaction.request", "interaction.respond", "tool.request", "tool.result"} {
				t.Run("missing_"+capability, func(t *testing.T) {
					mutated := release
					mutated.manifest.Adapter.Capabilities = slices.DeleteFunc(slices.Clone(release.manifest.Adapter.Capabilities), func(value string) bool { return value == capability })
					if compatibleTaskRuntime(mutated, requirements, nil, tc.protocol) {
						t.Fatalf("accepted missing %s", capability)
					}
				})
			}
		})
	}
}

func TestCompatibleTaskRuntimeRejectsEveryAdmissionMismatch(t *testing.T) {
	requirements := projectfacade.ExecutionRequirements{ExecutionKind: "AGENT_ATTEMPT", AdapterCapabilities: []string{"execution.resume"}, EnvironmentCapabilities: []string{"shell"}}
	release := func() catalogRelease {
		return catalogRelease{status: "ACTIVE", scope: "PRODUCTION", contract: "seven.worker-image/v1", imageDigest: "sha256:" + strings.Repeat("a", 64), executionKinds: []string{"AGENT_ATTEMPT"}, protocols: []string{"OPENAI"}, hookEvents: []string{"before_run"}, environment: []string{"shell"}, manifest: compatibleManifest()}
	}
	if !compatibleTaskRuntime(release(), requirements, []string{"before_run"}, "OPENAI") {
		t.Fatal("rejected compatible release")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*catalogRelease)
	}{
		{"status", func(r *catalogRelease) { r.status = "DISABLED" }},
		{"scope", func(r *catalogRelease) { r.scope = "TEST" }},
		{"kind", func(r *catalogRelease) { r.executionKinds = []string{"REVIEW_RUN"} }},
		{"contract", func(r *catalogRelease) { r.contract = "seven.worker-image/v2" }},
		{"image_digest", func(r *catalogRelease) { r.imageDigest = "sha256:" + strings.Repeat("z", 64) }},
		{"adapter", func(r *catalogRelease) { r.manifest.Adapter.Capabilities = nil }},
		{"adapter_protocol", func(r *catalogRelease) { r.manifest.Adapter.Protocol.Min = 2 }},
		{"provider_protocol", func(r *catalogRelease) { r.protocols = nil }},
		{"hook", func(r *catalogRelease) { r.hookEvents = nil }},
		{"environment", func(r *catalogRelease) { r.environment = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := release()
			tc.mutate(&r)
			if compatibleTaskRuntime(r, requirements, []string{"before_run"}, "OPENAI") {
				t.Fatal("accepted incompatible release")
			}
		})
	}
	requirements.ExecutionKind = "REVIEW_RUN"
	if compatibleTaskRuntime(release(), requirements, nil, "OPENAI") {
		t.Fatal("accepted non-Agent requirements")
	}
}

func TestRuntimeManifestRejectsNonHexDigests(t *testing.T) {
	for _, field := range []string{"python", "bootstrap", "sdk", "index"} {
		t.Run(field, func(t *testing.T) {
			m := compatibleManifest()
			invalid := strings.Repeat("z", 64)
			switch field {
			case "python":
				m.ResultProcessor.PythonRuntimeIdentity = invalid
			case "bootstrap":
				m.ResultProcessor.BootstrapDigest = invalid
			case "sdk":
				m.ResultProcessor.SDKDigest = invalid
			case "index":
				m.ResultProcessor.ModuleIndexDigest = invalid
			}
			if validRuntimeManifest(m) {
				t.Fatal("accepted non-hex runtime identity/digest")
			}
		})
	}
}
