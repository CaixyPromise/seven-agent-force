package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	check "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func TestAgentReceivesFrozenCompletionCriteriaReadOnly(t *testing.T) {
	storage, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.PrepareAttempt(101, 102)
	if err != nil {
		t.Fatal(err)
	}
	input := &check.Input{Mode: "SCRIPT", Script: "printf 'controlled-check'", Instructions: "Preserve pagination compatibility"}
	digest, err := check.FrozenDigest(*input, nil)
	if err != nil {
		t.Fatal(err)
	}
	item := task.WorkItem{TaskID: 101, Instruction: "Fix pagination only", CompletionCheck: input, CompletionCheckDigest: digest}
	d := &Docker{workspace: storage}
	instruction, err := d.materializeCompletionCheckInput(item, task.Attempt{ID: 102}, workspace, "/workspace/101")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(instruction, item.Instruction) || !strings.Contains(instruction, "/workspace/101/input/attempts/102/completion-check.json") || strings.Contains(instruction, input.Script) {
		t.Fatal("criteria not referenced separately")
	}
	path := filepath.Join(workspace.InputRoot, "attempts", "102", "completion-check.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{input.Script, input.Instructions, digest} {
		if !strings.Contains(string(body), value) {
			t.Fatalf("missing frozen value %s", value)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0222 != 0 {
		t.Fatal("criteria writable")
	}
	item.CompletionCheckDigest = strings.Repeat("a", 64)
	if _, err = d.materializeCompletionCheckInput(item, task.Attempt{ID: 102}, workspace, "/workspace/101"); err == nil {
		t.Fatal("changed identity accepted")
	}
}
