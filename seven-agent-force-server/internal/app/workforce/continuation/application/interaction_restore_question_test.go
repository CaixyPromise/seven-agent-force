package application

import (
	"bytes"
	"encoding/json"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"testing"
)

func TestRestoreQuestionProjectionBindsSavedResponse(t *testing.T) {
	var q interaction.PublishRequest
	if err := json.Unmarshal([]byte(`{"roundKey":"round:old","localRequestKey":"question-1","questions":[{"key":"scope","title":"Scope?","description":"Full original context","kind":"SINGLE_CHOICE","required":true,"allowFreeText":false,"options":[{"key":"small","label":"Small"}]}]}`), &q); err != nil {
		t.Fatal(err)
	}
	answer := json.RawMessage(`{"interactionKey":"question-1","responseId":"9007199254740993","answers":[]}`)
	body, err := ProjectInteractionRestoreQuestion(q, answer, 9007199254740993)
	if err != nil || !bytes.Contains(body, []byte("Full original context")) || !bytes.Contains(body, []byte(`"required":true`)) || bytes.Contains(body, []byte("roundKey")) || bytes.Contains(body, []byte("localRequestKey")) {
		t.Fatalf("projection %s %v", body, err)
	}
	for _, wrong := range []json.RawMessage{json.RawMessage(`{"interactionKey":"other","responseId":"9007199254740993"}`), json.RawMessage(`{"interactionKey":"question-1","responseId":"7"}`), json.RawMessage(`{"interactionKey":"question-1","responseId":9007199254740993}`)} {
		if _, err = ProjectInteractionRestoreQuestion(q, wrong, 9007199254740993); err == nil {
			t.Fatal("unbound response projected")
		}
	}
}
