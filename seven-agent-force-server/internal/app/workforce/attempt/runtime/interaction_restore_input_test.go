package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func TestNewAttemptSpecPayloadFreshStartOmitsNilRestoreFields(t *testing.T) {
	item := task.WorkItem{TaskID: 123}
	body, err := json.Marshal(newAttemptSpecPayload(item, task.Attempt{ID: 456}, "/workspace/123/attempts/456"))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err = json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"interactionResponse", "interactionQuestionRequest"} {
		if _, exists := wire[key]; exists {
			t.Fatalf("fresh start serialized optional restore field %s", key)
		}
	}
	if string(wire["taskId"]) != `"123"` || string(wire["attemptId"]) != `"456"` {
		t.Fatal("attempt identity changed")
	}
}

func TestNewAttemptSpecPayloadPreservesNonNilRestoreInput(t *testing.T) {
	// The producer must not normalize or silently discard supplied values;
	// validation of explicit null, partial pairs and actual objects stays downstream.
	for _, value := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`{"interactionKey":"question-1"}`)} {
		item := task.WorkItem{TaskID: 123, InteractionResponse: value}
		body, err := json.Marshal(newAttemptSpecPayload(item, task.Attempt{ID: 456}, "/workspace/123/attempts/456"))
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err = json.Unmarshal(body, &wire); err != nil {
			t.Fatal(err)
		}
		if string(wire["interactionResponse"]) != string(value) {
			t.Fatal("non-nil input altered")
		}
		if _, exists := wire["interactionQuestionRequest"]; exists {
			t.Fatal("missing pair member fabricated")
		}
	}
}

func TestInteractionRestoreAnswerIsPrivateAndExplicitlyBoundToAttemptSpec(t *testing.T) {
	answer := json.RawMessage(`{"interactionKey":"question-1","responseId":"9007199254740993","answers":[]}`)
	item := task.WorkItem{TaskID: 123, InteractionResponse: answer, InteractionBudgetRestored: true}
	item.InteractionQuestionRequest = json.RawMessage(`{"interactionKey":"question-1","questions":[]}`)
	public, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "interactionResponse") || strings.Contains(string(public), "question-1") || strings.Contains(string(public), "interactionBudgetRestored") {
		t.Fatal("private restore fields leaked into WorkItem transport")
	}
	var decoded task.WorkItem
	if err = json.Unmarshal([]byte(`{"interactionResponse":`+string(answer)+`,"interactionQuestionRequest":`+string(item.InteractionQuestionRequest)+`,"interactionBudgetRestored":true}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.InteractionResponse) != 0 || len(decoded.InteractionQuestionRequest) != 0 || decoded.InteractionBudgetRestored {
		t.Fatal("untrusted WorkItem JSON injected restore state")
	}
	spec := newAttemptSpecPayload(item, task.Attempt{ID: 456}, "/workspace/123/attempts/456")
	actual, err := json.Marshal(spec["interactionResponse"])
	if err != nil || string(actual) != string(answer) {
		t.Fatal("verified private answer missing from target attempt spec")
	}
	actual, err = json.Marshal(spec["interactionQuestionRequest"])
	if err != nil || string(actual) != string(item.InteractionQuestionRequest) {
		t.Fatal("trusted question missing from private spec")
	}
}
