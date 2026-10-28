package main

import (
	"context"
	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	input "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanningSourceRestoredAndVerified(t *testing.T) {
	root := t.TempDir()
	spec := filepath.Join(root, "input", "attempts", "123", "spec.json")
	if err := os.MkdirAll(filepath.Dir(spec), 0755); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmp, 0755); err != nil {
		t.Fatal(err)
	}
	identity := input.CanonicalEmptyWorkspaceIdentity()
	item := task.WorkItem{ExecutionSnapshot: task.ExecutionSnapshot{StagePurpose: "PLAN"}, TaskInput: &task.TaskInputContract{SourceKind: input.SourceKindEmptyWorkspace, SourceCommit: identity.Commit, SourceTree: identity.Tree, SourceIdentityDigest: identity.IdentityDigest}}
	workspace := runtime.Workspace{SpecPath: spec, Temporary: tmp}
	w := &worker{}
	if err := w.ensurePlanningSource(context.Background(), item, workspace); err != nil {
		t.Fatal(err)
	}
	if err := w.ensurePlanningSource(context.Background(), item, workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planningSourcePath(workspace), "tampered"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.ensurePlanningSource(context.Background(), item, workspace); err == nil {
		t.Fatal("dirty source accepted")
	}
}
