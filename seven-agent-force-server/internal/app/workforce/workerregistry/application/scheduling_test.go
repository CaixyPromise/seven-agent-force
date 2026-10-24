package application

import (
	"encoding/json"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
)

func TestRejectedAttemptInventoryIsPersistedAsUnroutableWithoutManifest(t *testing.T) {
	image := domain.Image{Role: "attempt", AdmissionStatus: "REJECTED"}
	setUnroutableScheduling(&image)
	if image.SchedulingScope != "TEST_ONLY" || image.SchedulingPriority != 0 || string(image.EnvironmentCapabilities) != `[]` || string(image.ExecutionKinds) != `["AGENT_ATTEMPT"]` || len(image.SchedulingDigest) != 64 {
		t.Fatalf("rejected image did not receive a stable unroutable identity: %+v", image)
	}
}

func TestProviderProxyInventoryHasNoExecutionKinds(t *testing.T) {
	image := domain.Image{Role: "provider-proxy", AdmissionStatus: "ADMITTED"}
	setUnroutableScheduling(&image)
	if image.SchedulingScope != "TEST_ONLY" || string(image.ExecutionKinds) != `[]` || len(image.SchedulingDigest) != 64 {
		t.Fatalf("provider proxy received an executable scheduling identity: %+v", image)
	}
}

func TestParseManifestSchedulingCanonicalizesAndDigests(t *testing.T) {
	first, err := ParseManifestScheduling(json.RawMessage(`{
		"contract":"seven.worker-image/v1",
		"scheduling":{
			"scope":"PRODUCTION",
			"priority":80,
			"environmentCapabilities":["shell.posix","language.node"],
			"providerProtocols":["OPENAI","ANTHROPIC"],
			"hookEvents":["codex.PostToolUse","codex.PreToolUse"]
		},
		"executionKinds":["REVIEW_RUN","AGENT_ATTEMPT"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseManifestScheduling(json.RawMessage(`{
		"scheduling":{
			"providerProtocols":["ANTHROPIC","OPENAI"],
			"environmentCapabilities":["language.node","shell.posix"],
			"hookEvents":["codex.PreToolUse","codex.PostToolUse"],
			"priority":80,
			"scope":"PRODUCTION"
		},
		"executionKinds":["AGENT_ATTEMPT","REVIEW_RUN"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.Scope != "PRODUCTION" || first.Priority != 80 {
		t.Fatalf("unexpected scheduling values: %+v", first)
	}
	if string(first.EnvironmentCapabilitiesJSON) != `["language.node","shell.posix"]` || string(first.ExecutionKindsJSON) != `["AGENT_ATTEMPT","REVIEW_RUN"]` {
		t.Fatalf("scheduling lists were not canonicalized: %s %s", first.EnvironmentCapabilitiesJSON, first.ExecutionKindsJSON)
	}
	if len(first.Digest) != 64 || first.Digest != second.Digest {
		t.Fatalf("scheduling digest is not deterministic: %q %q", first.Digest, second.Digest)
	}
}

func TestParseManifestSchedulingRejectsUnsafeOrAmbiguousContracts(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
	}{
		{name: "missing scheduling", manifest: `{}`},
		{name: "missing priority", manifest: `{"scheduling":{"scope":"PRODUCTION","environmentCapabilities":[],"providerProtocols":["OPENAI"]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "null capabilities", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":null,"providerProtocols":["OPENAI"]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "missing provider protocols", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":[]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "empty provider protocols", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":[],"providerProtocols":[]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "unknown provider protocol", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":[],"providerProtocols":["OTHER"]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "unknown scope", manifest: `{"scheduling":{"scope":"ANY","priority":100,"environmentCapabilities":[],"providerProtocols":["OPENAI"]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "priority outside schema", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100001,"environmentCapabilities":[],"providerProtocols":["OPENAI"]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "duplicate capability", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":["language.go","language.go"],"providerProtocols":["OPENAI"]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "invalid capability token", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":["language/go"],"providerProtocols":["OPENAI"]},"executionKinds":["AGENT_ATTEMPT"]}`},
		{name: "empty execution kinds", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":[],"providerProtocols":["OPENAI"]},"executionKinds":[]}`},
		{name: "unknown execution kind", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":[],"providerProtocols":["OPENAI"]},"executionKinds":["DELIVERY"]}`},
		{name: "unknown scheduling field", manifest: `{"scheduling":{"scope":"PRODUCTION","priority":100,"environmentCapabilities":[],"providerProtocols":["OPENAI"],"worker":"chosen"},"executionKinds":["AGENT_ATTEMPT"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseManifestScheduling(json.RawMessage(test.manifest)); err == nil {
				t.Fatal("invalid scheduling contract accepted")
			}
		})
	}
}
