package main

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func TestAgentObservationProjectionAndBoundedReconnectWindow(t *testing.T) {
	old := supervisorEvents
	supervisorEvents = &eventWriter{writer: io.Discard}
	defer func() { supervisorEvents = old }()
	state := &supervisorState{}
	for i := 0; i < 12; i++ {
		state.observeAgent(agentprotocol.Frame{Method: "message", Payload: json.RawMessage(`{"roundKey":"r1","text":"token: safe, api_key=sk-privatefixture012345","providerBody":"forbidden","nativePid":123}`)})
	}
	snapshot, _ := json.Marshal(protocolStateSnapshot(state))
	if !state.observationGap || len(state.observations) != observationWindow || state.observations[0].Sequence != 5 {
		t.Fatal("bounded buffer failed to disclose observation gap")
	}
	if len(snapshot) > 64<<10 || strings.Contains(string(snapshot), "sk-privatefixture") || strings.Contains(string(snapshot), "providerBody") {
		t.Fatal("unsafe or oversized observation snapshot")
	}
	for _, body := range []string{`{"roundKey":"r1","text":{"secret":"value"}}`, `{"roundKey":"r1","status":"APPROVED","title":"a","activityKey":"a"}`} {
		method := "message"
		if strings.Contains(body, "status") {
			method = "activity"
		}
		if _, ok := projectAgentObservation(agentprotocol.Frame{Method: method, Payload: json.RawMessage(body)}); ok {
			t.Fatal("untyped/native-authority output projected")
		}
	}
}
