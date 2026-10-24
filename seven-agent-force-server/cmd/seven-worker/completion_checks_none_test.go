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

func TestNoExecutableCheckRecordsNotConfiguredWithoutSourceOrDocker(t *testing.T) {
	input := checkdomain.Input{Mode: "NONE", Instructions: "read requirements"}
	digest, err := checkdomain.Digest(input)
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/read") {
			_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			return
		}
		var body struct {
			Evidence checkfacade.Evidence `json:"evidence"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Evidence.Status != string(checkdomain.NotConfigured) || body.Evidence.Observation.CheckDigest != digest {
			t.Errorf("unexpected observation %+v", body.Evidence)
		}
		writes++
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": body.Evidence})
	}))
	defer server.Close()
	client, err := control.New(server.URL, "worker-token-with-at-least-thirty-two-bytes", 1, "instance", "boot")
	if err != nil {
		t.Fatal(err)
	}
	w := &worker{control: client} // No source, filesystem config, journal or Docker.
	item := taskdomain.WorkItem{TaskID: 1, TaskTurnID: 2, CompletionCheck: &input, CompletionCheckInputDigest: digest, CompletionCheckDigest: digest}
	if err = w.runCompletionCheck(context.Background(), item, taskdomain.Attempt{ID: 3}, attemptruntime.Workspace{}, "BASELINE", 0); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatal("missing NOT_CONFIGURED evidence")
	}
}
