package runtime

import (
	"encoding/json"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageInputsAreSeparateAndReadOnly(t *testing.T) {
	store, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.PrepareAttempt(101, 102)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(filepath.Dir(workspace.SpecPath), "planning-source")
	if err = os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	d := &Docker{workspace: store}
	attempt := task.Attempt{ID: 102}
	plan := json.RawMessage(`{"contentDigest":"digest","plan":{"title":"approved"}}`)
	item := task.WorkItem{ExecutionSnapshot: task.ExecutionSnapshot{StagePurpose: "PLAN", ApprovedPlan: plan}}
	instruction, err := d.materializeStageInput(item, attempt, workspace, "/workspace/101", "original")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, "planning-source") || !strings.Contains(instruction, "previous-plan.json") || !strings.Contains(instruction, "plan.json") {
		t.Fatal("planner input paths omitted")
	}
	item.ExecutionSnapshot.StagePurpose = "IMPLEMENT"
	instruction, err = d.materializeStageInput(item, attempt, workspace, "/workspace/101", "original")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, "approved-plan.json") || strings.Contains(instruction, "approved\"") {
		t.Fatal("plan must be passed in a separate file")
	}
	path := filepath.Join(filepath.Dir(workspace.SpecPath), "approved-plan.json")
	body, err := os.ReadFile(path)
	if err != nil || string(body) != string(plan) {
		t.Fatal("approved plan changed", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0222 != 0 {
		t.Fatal("approved plan is writable")
	}
	item.ExecutionSnapshot.ApprovedPlan = nil
	if _, err = d.materializeStageInput(item, attempt, workspace, "/workspace/101", "original"); err == nil {
		t.Fatal("implementation without approved plan accepted")
	}
}
