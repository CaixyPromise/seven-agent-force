package main

import (
	"encoding/json"
	"testing"

	dispatch "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

func TestReattachAckActualSnapshotWorkerWire(t *testing.T) {
	const large int64 = 9007199254740993
	state := &supervisorState{ReattachPermitID: large, ReattachPermitDigest: "test-only-digest", OwnerEpoch: large + 1, State: "RUNNING", HarnessPID: 123, LastJournalSequence: large + 2}
	body, err := json.Marshal(map[string]any{"ok": true, "command": "adopt-owner", "reattachAck": reattachAckSnapshot(state, large)})
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		OK  bool                 `json:"ok"`
		Ack dispatch.ReattachAck `json:"reattachAck"`
	}
	if err = json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("actual CLI ACK is not decodable by Worker: %v", err)
	}
	if !wire.OK || wire.Ack.PermitID != large || wire.Ack.OldOwnerEpoch != large || wire.Ack.NewOwnerEpoch != large+1 || wire.Ack.LastJournalSequence != large+2 {
		t.Fatal("reattach identity lost precision")
	}
	var raw struct {
		Ack map[string]json.RawMessage `json:"reattachAck"`
	}
	if err = json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"reattachPermitId", "oldOwnerEpoch", "newOwnerEpoch", "lastJournalSequence"} {
		var decimal string
		if err = json.Unmarshal(raw.Ack[field], &decimal); err != nil || decimal == "" {
			t.Fatalf("%s must be decimal string", field)
		}
	}
	// Keep the general domain decoder strict; the producer must honor the wire.
	for _, field := range []string{"reattachPermitId", "oldOwnerEpoch", "newOwnerEpoch", "lastJournalSequence"} {
		bad := map[string]json.RawMessage{}
		original, _ := json.Marshal(reattachAckSnapshot(state, large))
		if err = json.Unmarshal(original, &bad); err != nil {
			t.Fatal(err)
		}
		bad[field] = json.RawMessage("9007199254740993")
		encoded, _ := json.Marshal(bad)
		if json.Unmarshal(encoded, &dispatch.ReattachAck{}) == nil {
			t.Fatalf("unquoted %s unexpectedly accepted", field)
		}
	}
}
