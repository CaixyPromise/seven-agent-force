package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/imageadmission"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func TestInteractionInventoryManifestReachesServerDescriptorParser(t *testing.T) {
	d := agentprotocol.Descriptor{ID: "fake", Version: "1", Executable: "/opt/seven/adapter/run", BinarySHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64), Transport: "stdio", Capabilities: []string{"execution.start", "execution.interrupt", "interaction.request", "interaction.respond", "interaction.received", "interaction.closed"}}
	d.Protocol.Name = "seven.agent-adapter"
	d.Protocol.Min = 1
	d.Protocol.Max = 1
	processor := map[string]any{"pythonRuntimeIdentity": strings.Repeat("d", 64), "bootstrapSha256": strings.Repeat("e", 64), "sdkSha256": strings.Repeat("f", 64), "moduleIndexSha256": strings.Repeat("0", 64), "moduleRoots": []string{"/opt/seven-worker/result-runtime"}}
	r := imageadmission.Report{HarnessType: "fake", HarnessVersion: "1", SupervisorVersion: "1", Probe: map[string]any{"adapterHandshakeVerified": true, "agentAdapter": d, "resultProcessor": processor}, SchedulingScope: "TEST_ONLY", SchedulingPriority: 100, ProviderProtocols: []string{"OPENAI"}, EnvironmentCapabilities: []string{"shell.posix"}, HookEvents: []string{"attempt.prepare"}, ExecutionKinds: []string{"AGENT_ATTEMPT"}, SchedulingDigest: strings.Repeat("c", 64), Metadata: imageadmission.ManifestMetadata{Name: "Fake image"}}
	// Actual Inspect returns a JSON-decoded probe, not a typed descriptor.
	b, _ := json.Marshal(r)
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	body, err := admittedInteractionInventoryManifest(r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := agentprotocol.ParseDescriptor(body)
	if err != nil || !reflect.DeepEqual(got, d) {
		t.Fatalf("registry parser lost admitted descriptor: %v", err)
	}
	var wire map[string]json.RawMessage
	if err = json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"probe", "harnessType", "harnessVersion", "supervisorVersion", "adapter", "resultProcessor", "scheduling", "executionKinds", "schedulingDigest"} {
		if len(wire[key]) == 0 {
			t.Fatalf("missing %s", key)
		}
	}
	var scheduling struct {
		ProviderProtocols []string `json:"providerProtocols"`
		HookEvents        []string `json:"hookEvents"`
	}
	if err = json.Unmarshal(wire["scheduling"], &scheduling); err != nil || !reflect.DeepEqual(scheduling.ProviderProtocols, r.ProviderProtocols) || !reflect.DeepEqual(scheduling.HookEvents, r.HookEvents) {
		t.Fatalf("registry manifest lost routing identity: %+v err=%v", scheduling, err)
	}
	var metadata imageadmission.ManifestMetadata
	if err = json.Unmarshal(wire["metadata"], &metadata); err != nil || metadata.Name != r.Metadata.Name {
		t.Fatalf("registry manifest lost display metadata: %+v err=%v", metadata, err)
	}
	var gotProcessor map[string]any
	if err = json.Unmarshal(wire["resultProcessor"], &gotProcessor); err != nil || gotProcessor["pythonRuntimeIdentity"] != processor["pythonRuntimeIdentity"] || gotProcessor["bootstrapSha256"] != processor["bootstrapSha256"] || gotProcessor["sdkSha256"] != processor["sdkSha256"] || gotProcessor["moduleIndexSha256"] != processor["moduleIndexSha256"] {
		t.Fatalf("registry manifest lost admitted Result Processor identity: %v", err)
	}
	delete(r.Probe, "agentAdapter")
	if _, err = admittedInteractionInventoryManifest(r); err == nil {
		t.Fatal("missing adapter accepted")
	}
	r.Probe["agentAdapter"] = d
	r.Probe["adapterHandshakeVerified"] = false
	if _, err = admittedInteractionInventoryManifest(r); err == nil {
		t.Fatal("unverified handshake accepted")
	}
	r.Probe["adapterHandshakeVerified"] = true
	d.Capabilities = []string{"execution.start"}
	r.Probe["agentAdapter"] = d
	if _, err = admittedInteractionInventoryManifest(r); err == nil {
		t.Fatal("invalid descriptor accepted")
	}
}

func TestAdmittedRegistryImageCarriesSchedulingIdentity(t *testing.T) {
	report := imageadmission.Report{
		ImageDigest: "sha256:" + strings.Repeat("a", 64), Contract: "seven.worker-image/v1",
		Probe: map[string]any{"adapterHandshakeVerified": true}, SchedulingScope: "PRODUCTION",
		SchedulingPriority: 100, EnvironmentCapabilities: []string{"language.node", "shell.posix"},
		ExecutionKinds: []string{"AGENT_ATTEMPT", "REVIEW_RUN"}, SchedulingDigest: strings.Repeat("b", 64),
	}
	image := admittedRegistryImage("seven/codex@sha256:"+strings.Repeat("a", 64), report, json.RawMessage(`{"scheduling":{}}`), json.RawMessage(`{}`), strings.Repeat("c", 64))
	var environmentCapabilities, executionKinds []string
	if err := json.Unmarshal(image.EnvironmentCapabilities, &environmentCapabilities); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(image.ExecutionKinds, &executionKinds); err != nil {
		t.Fatal(err)
	}
	if image.SchedulingScope != report.SchedulingScope || image.SchedulingPriority != report.SchedulingPriority || image.SchedulingDigest != report.SchedulingDigest {
		t.Fatalf("scheduling identity was not copied: %+v", image)
	}
	if !reflect.DeepEqual(environmentCapabilities, report.EnvironmentCapabilities) || !reflect.DeepEqual(executionKinds, report.ExecutionKinds) {
		t.Fatalf("scheduling arrays were not copied: capabilities=%v executionKinds=%v", environmentCapabilities, executionKinds)
	}
}
