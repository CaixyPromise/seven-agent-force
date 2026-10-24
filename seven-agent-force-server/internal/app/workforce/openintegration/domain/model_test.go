package domain

import (
	"encoding/json"
	"testing"

	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
)

func TestMCPOnlyApplicationOmitsSigningCredential(t *testing.T) {
	payload, err := json.Marshal(Application{
		ID: 9_007_199_254_740_993, Name: "Internal Codex", WebhookURL: "", Status: ApplicationEnabled,
		APICredential: CredentialSummary{ID: 9_007_199_254_740_995, Version: 1, Hint: "abcd1234"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["signingCredential"]; exists {
		t.Fatalf("MCP-only application exposed a signing credential: %s", payload)
	}
}

func TestOpenCapabilitiesPreserveApplicationSnowflakeAsString(t *testing.T) {
	normalized := jsoncompat.NormalizeForJSON(OpenCapabilitiesView{
		Schema: "seven.workforce.open-capabilities/v2", ApplicationID: 9_007_199_254_740_993,
		TaskSubmission: OpenTaskSubmissionCapability{RequiresProjectID: true, SupportsOutputs: true},
	}).(map[string]any)
	if normalized["applicationId"] != "9007199254740993" {
		t.Fatalf("applicationId=%#v", normalized["applicationId"])
	}
	submission, ok := normalized["taskSubmission"].(map[string]any)
	if !ok || submission["requiresProjectId"] != true || submission["supportsOutputs"] != true {
		t.Fatalf("unexpected task submission capability: %#v", normalized["taskSubmission"])
	}
}

func TestPublicStatusNeverLeaksInternalFailure(t *testing.T) {
	for internal, expected := range map[string]string{
		"CREATED": "QUEUED", "QUEUED": "QUEUED", "RETRY_WAIT": "QUEUED",
		"PREPARING": "RUNNING", "RUNNING": "RUNNING", "VALIDATING": "RUNNING", "PACKAGING": "RUNNING", "UPLOADING": "RUNNING", "CANCELLING": "RUNNING",
		"REVIEWING": "RUNNING", "INTERACTION_SUSPENDED": "ACTION_REQUIRED", "MANUAL_REVIEW": "ACTION_REQUIRED",
		"SUCCEEDED": "SUCCEEDED", "FAILED": "FAILED", "CANCELLED": "CANCELLED", "TIMED_OUT": "TIMED_OUT",
		"UNKNOWN_INTERNAL_VALUE": "FAILED",
	} {
		status, code, summary := PublicStatus(internal)
		if status != expected {
			t.Fatalf("PublicStatus(%q)=%q want %q", internal, status, expected)
		}
		if status == "FAILED" && (code != "TASK_FAILED" || summary != "Task could not be completed.") {
			t.Fatalf("failure mapping leaked unstable detail: %q %q", code, summary)
		}
	}
}

func TestOpenTaskOutputsNormalizeInt64AndEmptyList(t *testing.T) {
	view := OpenTaskOutputsView{
		Schema: "seven.workforce.open-task-outputs/v1", TaskID: 9_007_199_254_740_993,
		Outputs: []OpenTaskOutput{{
			OutputID: 9_007_199_254_740_995, Name: "result", OutputKind: "PATH", Status: "AVAILABLE",
			MediaType: "text/markdown", SHA256: "abc", SizeBytes: 9_007_199_254_740_997,
		}},
	}
	normalized := jsoncompat.NormalizeForJSON(view).(map[string]any)
	if normalized["taskId"] != "9007199254740993" {
		t.Fatalf("taskId=%#v", normalized["taskId"])
	}
	output := normalized["outputs"].([]any)[0].(map[string]any)
	if output["outputId"] != "9007199254740995" || output["sizeBytes"] != "9007199254740997" {
		t.Fatalf("unexpected normalized output: %#v", output)
	}

	empty := jsoncompat.NormalizeForJSON(OpenTaskOutputsView{Outputs: make([]OpenTaskOutput, 0)}).(map[string]any)
	if outputs, ok := empty["outputs"].([]any); !ok || len(outputs) != 0 {
		t.Fatalf("empty outputs must normalize as []: %#v", empty["outputs"])
	}
}

func TestPublicEventLinksExposeTaskAndOutputsOnly(t *testing.T) {
	normalized := jsoncompat.NormalizeForJSON(PublicEventEnvelope{Links: EventLinks{
		Task: "/api/open/workforce/v1/tasks/1", Outputs: "/api/open/workforce/v1/tasks/1/outputs",
	}}).(map[string]any)
	links := normalized["links"].(map[string]any)
	if len(links) != 2 || links["task"] == "" || links["outputs"] == "" {
		t.Fatalf("unexpected event links: %#v", links)
	}
	if _, exists := links["self"]; exists {
		t.Fatalf("event links retained self: %#v", links)
	}
}
