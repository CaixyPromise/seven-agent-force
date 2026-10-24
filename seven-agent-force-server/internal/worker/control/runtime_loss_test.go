package control

import (
	"context"
	registry "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeLossReplayKeyBindsCompleteObservation(t *testing.T) {
	keys := []string{}
	bodies := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		bodies = append(bodies, string(b))
		w.Write([]byte(`{"code":0,"data":{"disposition":"CANCELLED"}}`))
	}))
	defer server.Close()
	runtime := registry.RecoverableRuntime{AttemptID: 123, Generation: 1, OwnerEpoch: 2}
	for _, tc := range []struct {
		instance, boot, evidence string
		generation, owner        int64
	}{{"i1", "b1", "", 1, 2}, {"i1", "b1", "", 1, 2}, {"i2", "b1", "", 1, 2}, {"i1", "b2", "", 1, 2}, {"i1", "b1", "", 2, 2}, {"i1", "b1", "", 1, 3}, {"i1", "b1", strings.Repeat("a", 64), 1, 2}} {
		client, e := New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 1, tc.instance, tc.boot)
		if e != nil {
			t.Fatal(e)
		}
		runtime.Generation = tc.generation
		runtime.OwnerEpoch = tc.owner
		if _, e = client.ReportRuntimeLoss(context.Background(), runtime, "RUNTIME_MISSING", tc.evidence); e != nil {
			t.Fatal(e)
		}
	}
	if keys[0] == "" || keys[0] != keys[1] || bodies[0] != bodies[1] {
		t.Fatal("identical retry identity drift")
	}
	for i := 2; i < len(keys); i++ {
		if keys[i] == keys[0] || bodies[i] == bodies[0] {
			t.Fatalf("changed observation reused key at %d", i)
		}
	}
}
