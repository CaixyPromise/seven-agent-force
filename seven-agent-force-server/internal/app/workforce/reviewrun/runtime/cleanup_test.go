package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

func TestReviewRunPreservesReportAndFrozenResultWhenCleanupFails(t *testing.T) {
	ctx := context.Background()
	workspace, e := attemptruntime.OpenPersistentWorkspace(filepath.Join(t.TempDir(), "workspace"))
	if e != nil {
		t.Fatal(e)
	}
	commit, tree, frozen := installFrozenDockerResult(t, ctx, workspace, 9101, 9102, 9103)
	j, e := resourcejournal.Open(filepath.Join(t.TempDir(), "journal"))
	if e != nil {
		t.Fatal(e)
	}
	type object struct {
		ID     string            `json:"Id"`
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	objects := map[string]object{}
	count := 0
	report := `{"schemaVersion":"review-report/v1","verdict":"PASS","summary":"checked","findings":[]}`
	d := NewDocker(config.WorkerConfig{ID: 9105})
	d.BindPersistentWorkspace(workspace)
	d.exec = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "docker" {
			t.Fatalf("unexpected %s", name)
		}
		if args[0] == "create" || (len(args) > 1 && args[0] == "volume" && args[1] == "create") {
			labels := map[string]string{}
			target := ""
			for i, a := range args {
				if a == "--label" && i+1 < len(args) {
					k, v, _ := strings.Cut(args[i+1], "=")
					labels[k] = v
				}
				if a == "--name" && i+1 < len(args) {
					target = args[i+1]
				}
			}
			count++
			id := fmt.Sprintf("%064x", count)
			if args[0] == "volume" {
				target = args[len(args)-1]
				id = target
			}
			obj := object{ID: id, Name: target, Labels: labels}
			obj.Config.Labels = labels
			objects[id] = obj
			return []byte(id), nil
		}
		if args[0] == "start" {
			return nil, nil
		}
		if args[0] == "cp" {
			return nil, os.WriteFile(args[len(args)-1], []byte(report), 0600)
		}
		if len(args) > 1 && args[1] == "inspect" {
			obj, ok := objects[args[len(args)-1]]
			if !ok {
				return nil, errors.New("No such object: removed")
			}
			return json.Marshal([]object{obj})
		}
		if len(args) > 1 && args[1] == "rm" {
			id := args[len(args)-1]
			obj := objects[id]
			if obj.Config.Labels["seven.review.run.id"] != "" && args[0] == "container" {
				return nil, errors.New("daemon refused removal")
			}
			delete(objects, id)
			return nil, nil
		}
		t.Fatalf("unexpected command %v", args)
		return nil, nil
	}
	d.BindResourceJournal(j)
	result, e := d.Run(ctx, Work{ReviewRunID: 9104, WorkerID: 9105, TaskID: 9101, TaskTurnID: 9102, FinalizationRunID: 9103, ResultCommit: commit, ResultTree: tree, ReviewerImage: "sha256:" + strings.Repeat("c", 64), OwnerEpoch: 1, ReviewContract: []byte(`{"version":"independent-review/v1"}`), Requirements: []byte(`{"instruction":"inspect frozen result"}`)}, Gate{})
	if e != nil {
		t.Fatal(e)
	}
	if string(result.Report) != report {
		t.Fatalf("cleanup replaced report: %s", result.Report)
	}
	if j.Pending() == 0 {
		t.Fatal("failed cleanup released capacity")
	}
	if _, e = os.Stat(frozen.ReadyPath); e != nil {
		t.Fatal("trusted result removed", e)
	}
}
