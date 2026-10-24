package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexReviewContractIsFixedAndMachineOnly(t *testing.T) {
	contract := frozenCodexReviewContract()
	if contract.HarnessType != "codex" || contract.Adapter != "codex-app-server-v1" || contract.ReportSchema != "review-report/v1" {
		t.Fatalf("unexpected Reviewer contract: %+v", contract)
	}
	if contract.ProviderIdentitySource != "task.model-credential" || contract.VerdictSource != "codex-structured-output" {
		t.Fatalf("unexpected identity/verdict source: %+v", contract)
	}
	if contract.TaskVerdictSelectionAllowed || contract.EnvVerdictSelectionAllowed {
		t.Fatalf("verdict selection was not closed: %+v", contract)
	}

}

func TestCodexReviewRejectsAttemptAndTaskVerdictInputs(t *testing.T) {
	if err := validateCodexReviewArgs([]string{"--verdict", "PASS"}); !errors.Is(err, errCodexReviewArgs) {
		t.Fatalf("unexpected argument error: %v", err)
	}
	if err := validateCodexReviewArgs([]string{"PASS"}); !errors.Is(err, errCodexReviewArgs) {
		t.Fatalf("unexpected positional argument error: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"schemaVersion": 1, "reviewRunId": "910000000000000001", "taskId": "910000000000000002",
		"resultCommit": strings.Repeat("a", 40), "resultTree": strings.Repeat("b", 40),
		"reviewContract": map[string]any{}, "requirements": map[string]any{},
		"provider": validCodexReviewInput().Provider,
		"verdict":  "PASS",
	})
	if _, err = decodeCodexReviewInput(body); !errors.Is(err, errCodexReviewInput) {
		t.Fatalf("task verdict was not rejected as closed input: %v", err)
	}
}

func TestCodexReviewRejectsNonObjectFrozenDocuments(t *testing.T) {
	input := validCodexReviewInput()
	for _, document := range []json.RawMessage{[]byte(`[]`), []byte(`null`), []byte(`"requirements"`)} {
		candidate := input
		candidate.Requirements = document
		body, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = decodeCodexReviewInput(body); !errors.Is(err, errCodexReviewInput) {
			t.Fatalf("document %s error=%v, want errCodexReviewInput", document, err)
		}
	}
}

func TestCodexReviewProviderIdentityIsClosedAndFrozen(t *testing.T) {
	input := validCodexReviewInput()
	if err := validateCodexReviewProviderIdentity(*input.Provider); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*codexReviewProviderIdentity){
		"profile id": func(value *codexReviewProviderIdentity) { value.CredentialID = "0" },
		"version":    func(value *codexReviewProviderIdentity) { value.CredentialVersion = "0" },
		"protocol":   func(value *codexReviewProviderIdentity) { value.Protocol = "OPENAI_CHAT_COMPLETIONS" },
		"model":      func(value *codexReviewProviderIdentity) { value.Model = "" },
		"schema":     func(value *codexReviewProviderIdentity) { value.StructuredContract = "attempt-result/v1" },
	} {
		candidate := *input.Provider
		mutate(&candidate)
		if err := validateCodexReviewProviderIdentity(candidate); !errors.Is(err, errCodexReviewProviderIdentity) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}

	body := []byte(`{"schemaVersion":1,"reviewRunId":"910000000000000001","taskId":"910000000000000002","resultCommit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","resultTree":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","reviewContract":{},"requirements":{},"provider":{"profileId":"910000000000000003","configDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","protocol":"OPENAI_RESPONSES","model":"gpt-review","structuredContract":"review-report/v1","apiKey":"secret"}}`)
	if _, err := decodeCodexReviewInput(body); !errors.Is(err, errCodexReviewInput) {
		t.Fatalf("inline provider secret was accepted: %v", err)
	}
}

func TestCodexReviewOutputSchemaIsTheFixedReportContract(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(codexReviewOutputSchema), &schema); err != nil {
		t.Fatalf("output schema is not JSON: %v", err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("output schema is not closed: %v", schema["additionalProperties"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || properties["schemaVersion"] == nil || properties["verdict"] == nil {
		t.Fatalf("output schema lacks fixed report fields: %v", properties)
	}
}

func TestCodexReviewProviderSchemaIsAClosedCompatibilityEnvelope(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(codexReviewProviderOutputSchema), &schema); err != nil {
		t.Fatalf("provider output schema is not JSON: %v", err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("provider output schema is not a closed object: %v", schema)
	}
	required, ok := schema["required"].([]any)
	if !ok || len(required) != 7 {
		t.Fatalf("provider output schema must require the bounded report envelope: %v", schema["required"])
	}
	for _, forbidden := range []string{"const", "enum", "minLength", "maxLength", "minimum", "maximum", "maxItems"} {
		if strings.Contains(string(codexReviewProviderOutputSchema), `"`+forbidden+`"`) {
			t.Fatalf("provider compatibility schema contains unsupported keyword %q", forbidden)
		}
	}
	if !strings.Contains(string(codexReviewOutputSchema), `"enum"`) || !strings.Contains(string(codexReviewOutputSchema), `"const"`) {
		t.Fatal("full Server-side canonical report contract was weakened")
	}
}

func TestCodexReviewManifestMatchesTheCLIContract(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "workforce", "images", "codex-harness-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Harness struct {
			Type    string `json:"type"`
			Adapter string `json:"adapter"`
		} `json:"harness"`
		Reviewer struct {
			HarnessType                 string `json:"harnessType"`
			Adapter                     string `json:"adapter"`
			ReportSchema                string `json:"reportSchema"`
			ProviderIdentitySource      string `json:"providerIdentitySource"`
			VerdictSource               string `json:"verdictSource"`
			TaskVerdictSelection        string `json:"taskVerdictSelection"`
			EnvironmentVerdictSelection string `json:"environmentVerdictSelection"`
		} `json:"reviewer"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Harness.Type != codexReviewHarnessType || manifest.Harness.Adapter != codexReviewAdapter || manifest.Reviewer.HarnessType != codexReviewHarnessType || manifest.Reviewer.Adapter != codexReviewAdapter || manifest.Reviewer.ReportSchema != codexReviewReportSchema || manifest.Reviewer.ProviderIdentitySource != codexReviewProviderIdentityID || manifest.Reviewer.VerdictSource != "codex-structured-output" || manifest.Reviewer.TaskVerdictSelection != "rejected" || manifest.Reviewer.EnvironmentVerdictSelection != "rejected" {
		t.Fatalf("manifest does not match CLI contract: %+v", manifest)
	}
	capabilitySet := make(map[string]bool, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		capabilitySet[capability] = true
	}
	for _, capability := range []string{"harness.codex.review", "review.provider.gateway-client", "review.fixed-provider-identity", "review.task-verdict-rejected", "review.environment-verdict-rejected"} {
		if !capabilitySet[capability] {
			t.Fatalf("manifest capability %q is missing", capability)
		}
	}
}

func validCodexReviewInput() codexReviewInput {
	return codexReviewInput{
		SchemaVersion:  1,
		ReviewRunID:    "910000000000000001",
		TaskID:         "910000000000000002",
		ResultCommit:   strings.Repeat("a", 40),
		ResultTree:     strings.Repeat("b", 40),
		ReviewContract: json.RawMessage(`{"required":true}`),
		Requirements:   json.RawMessage(`{"offline":true}`),
		Provider: &codexReviewProviderIdentity{
			CredentialID:       "910000000000000003",
			CredentialVersion:  "1",
			Protocol:           codexReviewProviderProtocol,
			Model:              "gpt-review",
			StructuredContract: codexReviewStructuredContract,
		},
	}
}
