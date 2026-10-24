package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	dispatch "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJournalReplayUsesFixedIngressAndOwnerScopedRequest(t *testing.T) {
	var ids []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/internal/workforce/v1/attempts/40/journal-facts/batch" {
			t.Error("journal used ordinary facts ingress")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["producerSource"]; ok {
			t.Error("source supplied by body")
		}
		var requestID string
		_ = json.Unmarshal(body["requestId"], &requestID)
		ids = append(ids, requestID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"ack":true}}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "test-worker-token-not-used-outside-test", 8, "qa-new", "qa-boot")
	if err != nil {
		t.Fatal(err)
	}
	facts := []dispatch.Fact{{FactID: "jrn_40_3", ProducerSequence: 3, EventType: "owner.adopted", Payload: json.RawMessage(`{}`)}}
	for _, owner := range []int64{2, 3} {
		if err = client.ReplayJournalFacts(context.Background(), task.Attempt{ID: 40, Generation: 1, OwnerEpoch: owner, LeaseToken: "test-only"}, facts); err != nil {
			t.Fatal(err)
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatal("new owner reused old transport request identity")
	}
}

func TestJournalReplayPreservesOriginalPayloadDigest(t *testing.T) {
	// Deliberately not map-key order: this models the actual typed ACK producer.
	payload := json.RawMessage(`{"reattachPermitId":"9007199254740993","oldOwnerEpoch":"1","newOwnerEpoch":"2","supervisorState":"RUNNING","lastJournalSequence":"2"}`)
	digest := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Facts []dispatch.Fact `json:"facts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if len(req.Facts) != 1 {
			t.Error("missing fact")
			w.WriteHeader(400)
			return
		}
		got := req.Facts[0]
		sum := sha256.Sum256(got.Payload)
		if !bytes.Equal(got.Payload, payload) || got.PayloadDigest != hex.EncodeToString(sum[:]) {
			t.Error("transport rewrote digest-bound payload")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"ack":true}}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "test-worker-token-not-used-outside-test", 8, "qa-new", "qa-boot")
	if err != nil {
		t.Fatal(err)
	}
	facts := []dispatch.Fact{{FactID: "jrn_40_3", ProducerSequence: 3, EventType: "owner.adopted", Payload: payload, PayloadDigest: hex.EncodeToString(digest[:])}}
	if err = client.ReplayJournalFacts(context.Background(), task.Attempt{ID: 40, Generation: 1, OwnerEpoch: 2, LeaseToken: "test-only"}, facts); err != nil {
		t.Fatal(err)
	}
}
