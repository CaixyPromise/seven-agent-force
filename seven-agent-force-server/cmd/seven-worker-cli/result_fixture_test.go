package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactedProcessorDiagnosticNeverReturnsMaterializedValue(t *testing.T) {
	t.Parallel()
	got := redactedProcessorDiagnostic([]byte("processor failed: top-secret-token / token"), map[string]string{"API_TOKEN": "top-secret-token", "OTHER": "token"}, 4096)
	if strings.Contains(got, "top-secret-token") || strings.Contains(got, " token") {
		t.Fatalf("diagnostic leaked materialized value: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("diagnostic was not redacted: %q", got)
	}
}

func TestValidateResultFixtureSpecRequiresProtectedPaths(t *testing.T) {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	spec := resultFixtureSpec{
		SchemaVersion: 1, RunID: 8756178, RevisionDigest: digest,
		ProcessorRoot: filepath.Join(fixtureProcessorRoot, digest), WorkspaceRoot: fixtureWorkspaceRoot,
		RuntimeRoot: fixtureRuntimeRoot, ResultPath: filepath.Join(fixtureRuntimeRoot, "result.json"),
		EvaluationContext: json.RawMessage(`{"parameters":{"answer":42}}`), ConfigOverrides: json.RawMessage(`{"MODE":"fixture"}`),
	}
	if err := validateResultFixtureSpec(spec); err != nil {
		t.Fatalf("valid fixture spec rejected: %v", err)
	}
	spec.ProcessorRoot = "/tmp/untrusted"
	if err := validateResultFixtureSpec(spec); err == nil {
		t.Fatal("untrusted Processor root was accepted")
	}
}

func TestFixtureEvaluationContextCannotOverrideExecutionIdentity(t *testing.T) {
	spec := resultFixtureSpec{RunID: 8756178, EvaluationContext: json.RawMessage(`{"taskId":"spoofed","parameters":{"mode":"strict"}}`)}
	body, err := fixtureEvaluationContext(spec)
	if err != nil {
		t.Fatalf("build fixture context: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if got := value["taskId"]; got != "8756178" {
		t.Fatalf("fixture task identity was overridden: %#v", got)
	}
	parameters, ok := value["parameters"].(map[string]any)
	if !ok || parameters["mode"] != "strict" {
		t.Fatalf("fixture parameters were not preserved: %#v", value["parameters"])
	}
}

func TestMergeFixtureDeliveryActionsPreservesContractAndRejectsMalformedEnvelope(t *testing.T) {
	t.Parallel()
	merged, err := mergeFixtureDeliveryActions(
		[]byte(`{"decision":"PASS","reasonCode":"OK","message":"done"}`),
		[]byte(`{"actions":[{"type":"GIT_PUSH","when":"AFTER_APPROVAL","targetId":"gdt_fixture","baseBranch":"main","branch":"seven/fixture"}]}`),
	)
	if err != nil {
		t.Fatalf("merge Fixture delivery actions: %v", err)
	}
	var value struct {
		Decision string            `json:"decision"`
		Actions  []json.RawMessage `json:"actions"`
	}
	if err = json.Unmarshal(merged, &value); err != nil || value.Decision != "PASS" || len(value.Actions) != 1 {
		t.Fatalf("merged Fixture result=%s err=%v", merged, err)
	}
	if _, err := mergeFixtureDeliveryActions([]byte(`{}`), []byte(`{"actions":{}}`)); err == nil {
		t.Fatal("malformed Fixture delivery envelope was accepted")
	}
}
