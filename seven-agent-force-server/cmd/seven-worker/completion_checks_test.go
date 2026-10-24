package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	checkdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	checkfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
)

func TestBaselineReusesServerOriginalAcrossAttemptAndTurnWithoutRestoringWorkspace(t *testing.T) {
	input := checkdomain.Input{Mode: "COMMAND", Command: "true"}
	digest, err := checkdomain.Digest(input)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.HasSuffix(r.URL.Path, "/completion-check-results/read") {
			t.Errorf("unexpected write %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": checkfacade.Evidence{TaskID: 1, TaskTurnID: 2, AttemptID: 3, Phase: "BASELINE", CheckDigest: digest, Status: "PASSED"}})
	}))
	defer server.Close()
	client, err := control.New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 1, "instance", "boot")
	if err != nil {
		t.Fatal(err)
	}
	w := &worker{control: client}
	item := taskdomain.WorkItem{TaskID: 1, TaskTurnID: 20, CompletionCheck: &input, CompletionCheckInputDigest: digest, CompletionCheckDigest: digest}
	// No TaskInput, journal, artifacts or real workspace: an inherited baseline
	// must return before any original-source reconstruction or Docker operation.
	if err = w.runCompletionCheck(context.Background(), item, taskdomain.Attempt{ID: 30}, attemptruntime.Workspace{Repo: "/not-a-baseline"}, "BASELINE", 0); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatal("baseline executed again")
	}
}
