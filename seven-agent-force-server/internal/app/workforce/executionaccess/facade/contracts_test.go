package facade

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBoundaryIDsEncodeAsDecimalStrings(t *testing.T) {
	raw, err := json.Marshal(OperationView{ID: 9007199254740993, TaskID: 9007199254740995, TaskTurnID: 7, AttemptID: 8, Version: 9, AgentContinued: true})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{`"id":"9007199254740993"`, `"taskId":"9007199254740995"`, `"taskTurnId":"7"`, `"attemptId":"8"`, `"version":"9"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
	if !strings.Contains(text, `"agentContinued":true`) {
		t.Fatalf("agent continuation fact missing from %s", text)
	}
}

func TestToolResultCarriesExactAdapterCorrelation(t *testing.T) {
	raw, err := json.Marshal(ToolResult{OperationID: 11, RoundKey: "round-1", LocalToolCallKey: "call-1", NativeToolCallID: "item_tool_call_01HXYZ", AdapterSessionGeneration: 12})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{`"operationId":"11"`, `"roundKey":"round-1"`, `"localToolCallKey":"call-1"`, `"nativeToolCallId":"item_tool_call_01HXYZ"`, `"adapterSessionGeneration":"12"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
}

func TestResultReceiptRequestKeepsAttemptAndOperationIDsAsDecimalStrings(t *testing.T) {
	raw, err := json.Marshal(ResultReceiptRequest{OperationID: 9007199254740993, AttemptID: 9007199254740995, ResultDigest: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{`"operationId":"9007199254740993"`, `"attemptId":"9007199254740995"`, `"resultDigest":"` + strings.Repeat("a", 64) + `"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
}

func TestNetworkCommandToolDefinitionIsFrozenAndClosed(t *testing.T) {
	definition := NetworkCommandToolDefinition()
	if definition.Name != "run_command_with_network_access" || definition.Description != NetworkCommandToolDescription {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	raw, err := json.Marshal(definition.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"additionalProperties":false`) || !strings.Contains(string(raw), `"maxItems":8`) {
		t.Fatalf("schema is not closed: %s", raw)
	}
}
