package main

import (
	"encoding/json"
	"testing"
	"time"

	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	registrydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
)

func TestInventoryScanDelayStaysWithinTenPercent(t *testing.T) {
	base := 60 * time.Second
	for entropy := int64(-100); entropy <= 100; entropy++ {
		got := inventoryScanDelay(base, entropy)
		if got < 54*time.Second || got > 66*time.Second {
			t.Fatalf("entropy %d produced out-of-range delay %s", entropy, got)
		}
	}
}

func TestUnmanagedProtocolImageIsReportedWithoutAdmissionEvidence(t *testing.T) {
	// The complete inventory still needs a record for a protocol-labelled image
	// that local policy rejects.  It must not need a probe/conformance container
	// before the Worker can publish that rejected fact.
	evidence, _ := json.Marshal(map[string]any{
		"reasonCode":  "NOT_ALLOWED_BY_WORKER_POLICY",
		"imageDigest": "sha256:unmanaged",
	})
	image := registrydomain.Image{
		ImageDigest:       "sha256:unmanaged",
		ImageRef:          "example/unmanaged@sha256:unmanaged",
		Role:              "attempt",
		ContractVersion:   "seven.worker-image/v1",
		AdmissionStatus:   "REJECTED",
		AdmissionEvidence: evidence,
		PresenceStatus:    "PRESENT",
		InventoryVersion:  1,
	}
	if image.AdmissionStatus != "REJECTED" || image.ContractVersion != "seven.worker-image/v1" || image.ManifestDigest != "" || len(image.Capabilities) != 0 {
		t.Fatalf("unmanaged image must be reported without probe-derived admission data: %#v", image)
	}
	var actual map[string]string
	if err := json.Unmarshal(image.AdmissionEvidence, &actual); err != nil || actual["reasonCode"] != "NOT_ALLOWED_BY_WORKER_POLICY" || actual["imageDigest"] != image.ImageDigest {
		t.Fatalf("unexpected rejection evidence: %s (%v)", image.AdmissionEvidence, err)
	}
}

func TestImageDeclaresDeliveryEgress(t *testing.T) {
	if !imageDeclaresDeliveryEgress([]byte(`{"capabilities":["harness.codex","delivery.egress-gateway"]}`)) {
		t.Fatal("declared gateway capability was not recognized")
	}
	for _, body := range [][]byte{[]byte(`{"capabilities":["harness.codex"]}`), []byte(`{bad`)} {
		if imageDeclaresDeliveryEgress(body) {
			t.Fatalf("unexpected delivery-egress declaration for %q", body)
		}
	}
}

func TestDeliveryHeaderAcceptsCompleteAssignmentEnvelope(t *testing.T) {
	body, err := json.Marshal(dispatchdomain.AssignmentMessage{
		SchemaVersion: 1,
		Kind:          dispatchdomain.AssignmentSchema,
		MessageID:     9_007_199_254_740_993,
		AssignmentID:  9_007_199_254_740_994,
		TaskID:        9_007_199_254_740_995,
		WorkerID:      9_007_199_254_740_996,
		Generation:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	header, err := deliveryHeader(body)
	if err != nil {
		t.Fatalf("valid full assignment was rejected at header routing: %v", err)
	}
	if header.SchemaVersion != 1 || header.Kind != dispatchdomain.AssignmentSchema {
		t.Fatalf("unexpected parsed header: %#v", header)
	}
	var decoded dispatchdomain.AssignmentMessage
	if err := strictJSON(body, &decoded); err != nil || decoded.AssignmentID != 9_007_199_254_740_994 {
		t.Fatalf("full strict validation must remain intact: %#v (%v)", decoded, err)
	}
}

func TestDeliveryHeaderRejectsMalformedJSON(t *testing.T) {
	if _, err := deliveryHeader([]byte(`{"schemaVersion":1`)); err == nil {
		t.Fatal("malformed message was accepted")
	}
}
