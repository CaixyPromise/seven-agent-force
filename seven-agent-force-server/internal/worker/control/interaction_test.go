package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func TestPublishInteractionHTTPPreservesSpooledFrameDigest(t *testing.T) {
	// Match the hub: typed frame marshaled once before Put. Field order and
	// escaped text must survive Machine envelope composition, not just mocks.
	payload, _ := json.Marshal(map[string]any{"roundKey": "round:1", "text": "观察 <tag> & \u2028", "value": json.Number("9007199254740993")})
	frame, err := json.Marshal(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "message", Key: "original-occurrence", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(frame)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Frame json.RawMessage `json:"frame"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if !bytes.Equal(req.Frame, frame) {
			t.Error("HTTP envelope altered immutable spooled frame bytes")
		}
		sum := sha256.Sum256(req.Frame)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": map[string]any{"frameSha256": hex.EncodeToString(sum[:]), "receipt": map[string]any{"saved": true}}})
	}))
	defer server.Close()
	c, err := New(server.URL, strings.Repeat("t", 32), 1, "test-instance", "test-boot")
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.PublishInteraction(context.Background(), task.Attempt{ID: 2, Generation: 1, OwnerEpoch: 1, LeaseToken: "test-only"}, 1, frame)
	if err != nil {
		t.Fatal(err)
	}
	if result.FrameSHA256 != hex.EncodeToString(want[:]) {
		t.Fatal("Server receipt digest differs from private spool receipt")
	}
}
