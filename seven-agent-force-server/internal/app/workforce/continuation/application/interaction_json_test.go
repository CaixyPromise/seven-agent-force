package application

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSuspendMachineDTOIDsAreDecimalStringsAndAuthorityIsPrivate(t *testing.T) {
	const id int64 = 9007199254740993
	r := ReserveInteractionRequest{InteractionCaptureRequest: InteractionCaptureRequest{MachineAuthority: MachineAuthority{WorkerID: id, InstanceID: "private-instance", LeaseToken: "private-token", AttemptID: id}, TenantID: id, TaskID: id, TaskTurnID: id, InteractionID: id, CheckpointID: id}, AdapterSessionGeneration: id}
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"taskId", "taskTurnId", "interactionId", "checkpointId", "adapterSessionGeneration"} {
		if decoded[field] != "9007199254740993" {
			t.Fatalf("%s not lossless: %s", field, body)
		}
	}
	for _, secret := range []string{"private-instance", "private-token", "WorkerID", "AttemptID", "TenantID"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("private authority serialized: %s", secret)
		}
	}
	result, err := json.Marshal(InteractionReservationResult{Reservation: InteractionReservation{ID: id, AdapterSessionGeneration: id}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"id":"9007199254740993"`) || !strings.Contains(string(result), `"adapterSessionGeneration":"9007199254740993"`) {
		t.Fatalf("reservation ids not strings %s", result)
	}
}
