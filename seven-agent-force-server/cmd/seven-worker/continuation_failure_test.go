package main

import (
	"context"
	"encoding/json"
	"errors"
	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWaitingCheckpointFailurePersistsTerminalOutcome(t *testing.T) {
	for _, decision := range []string{"WAIT_USER", "MANUAL_REVIEW"} {
		t.Run(decision, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/control") {
					w.Write([]byte(`{"code":0,"data":{"cancelling":false}}`))
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/outcome") {
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(500)
					return
				}
				var outcome control.Outcome
				if e := json.NewDecoder(r.Body).Decode(&outcome); e != nil {
					t.Error(e)
				}
				if outcome.Outcome != "FAILED" || outcome.ReasonCode != "CONTINUATION_CHECKPOINT_FAILED" {
					t.Errorf("unexpected outcome %+v", outcome)
				}
				writes++
				w.Write([]byte(`{"code":0,"data":null}`))
			}))
			defer server.Close()
			client, e := control.New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 1, "instance", "boot")
			if e != nil {
				t.Fatal(e)
			}
			w := &worker{control: client}
			w.cfg.Worker.WorkspaceRoot = t.TempDir()
			directive := facade.DirectiveWaitForUser
			if decision == "MANUAL_REVIEW" {
				directive = facade.DirectiveStopManualReview
			}
			e = w.finishAttempt(context.Background(), domain.WorkItem{TaskID: 1, TaskTurnID: 2}, domain.Attempt{ID: 3, Generation: 1}, attemptruntime.Workspace{}, attemptruntime.RunResult{TerminalDirective: directive, TerminalDecision: decision, EvaluationCycleID: 5}, func() bool { return false })
			var terminal *terminalFailure
			if !errors.As(e, &terminal) || writes != 1 {
				t.Fatalf("failure not persisted %v writes=%d", e, writes)
			}
		})
	}
}
