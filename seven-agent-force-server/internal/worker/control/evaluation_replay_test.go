package control

import (
	"context"
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEvaluationReplayClientUsesCurrentAuthorityWithoutCachedResponse(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/internal/workforce/v1/attempts/9007199254740993/result-evaluations/read" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("incorrect replay route or cached read")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for key, want := range map[string]string{"workerId": "7", "instanceId": "current", "hostBootId": "boot", "generation": "1", "ownerEpoch": "2", "attemptLeaseToken": "current-lease"} {
			if body[key] != want {
				t.Errorf("wrong %s", key)
			}
		}
		pending, ok := body["pending"].(map[string]any)
		if !ok || pending["cycleId"] != "5" || pending["sessionDigest"] != "session" {
			t.Error("wrong pending identity")
		}
		switch calls {
		case 1:
			w.Write([]byte(`{"code":0,"data":{"found":false}}`))
		case 2:
			w.Write([]byte(`{"code":0,"data":{"found":true,"directive":{"cycleId":"5","decision":"WAIT_USER","directive":"WAIT_FOR_USER"}}}`))
		default:
			w.WriteHeader(409)
			w.Write([]byte(`{"code":409,"data":{"reasonCode":"FENCED"}}`))
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "local-test-token-with-at-least-thirty-two-bytes", 7, "current", "boot")
	if err != nil {
		t.Fatal(err)
	}
	a := domain.Attempt{ID: 9007199254740993, Generation: 1, OwnerEpoch: 2, LeaseToken: "current-lease"}
	p := facade.EvaluationReplayIdentity{CycleID: 5, CycleNumber: 1, SessionDigest: "session"}
	first, err := c.ReadEvaluation(context.Background(), a, p)
	if err != nil || first.Found {
		t.Fatalf("first %v %v", first, err)
	}
	second, err := c.ReadEvaluation(context.Background(), a, p)
	if err != nil || !second.Found || second.Directive.Decision != "WAIT_USER" {
		t.Fatalf("second %v %v", second, err)
	}
	if _, err = c.ReadEvaluation(context.Background(), a, p); err == nil {
		t.Fatal("conflict became absence")
	}
}

func TestEvaluationReplayClientRejectsMissingOrMismatchedRead(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"found":true,"directive":{"cycleId":"6"}}`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"code":0,"data":` + body + `}`)) }))
			defer s.Close()
			c, _ := New(s.URL, "local-test-token-with-at-least-thirty-two-bytes", 7, "instance", "boot")
			if _, err := c.ReadEvaluation(context.Background(), domain.Attempt{ID: 3}, facade.EvaluationReplayIdentity{CycleID: 5}); err == nil {
				t.Fatal("invalid response became authorized read")
			}
		})
	}
}
