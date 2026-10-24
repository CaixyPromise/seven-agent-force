package agentprotocol

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBuiltPythonProjectStartsAndRejectsMissingOrTamperedFiles(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	_, source, _, _ := runtime.Caller(0)
	sdk := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../deploy/workforce/runtime-sdk"))
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	script := `from pathlib import Path
import sys
from test_project import ProjectTests
from seven_agent.force.__main__ import main
root=Path(sys.argv[1])
ProjectTests().project(root)
raise SystemExit(main(["build",str(root),"--module","example","--output",str(root/"built")]))
`
	command := exec.CommandContext(ctx, python, "-c", script, root)
	command.Env = []string{"PYTHONPATH=" + filepath.Join(sdk, "src") + string(os.PathListSeparator) + filepath.Join(sdk, "tests"), "PYTHONDONTWRITEBYTECODE=1"}
	if body, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v %s", err, body)
	}
	output := filepath.Join(root, "built")
	body, err := os.ReadFile(filepath.Join(output, "adapter-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := ParseDescriptor(body)
	if err != nil {
		t.Fatal(err)
	}
	// Local verification relocates only the container root, not file contents.
	descriptor.Executable = filepath.Join(output, "entrypoint")
	if err := descriptor.VerifyEntrypoint(); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProcess(ctx, python, []string{"-I", filepath.Join(output, "runtime-entry.py")}, []string{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.ValidateReady(descriptor); err != nil {
		t.Fatal(err)
	}
	result, err := p.Round(ctx, "project:1", map[string]any{}, "", nil, nil)
	if err != nil || result.Outcome != "FINISHED" || result.SessionReference != "fake-session" {
		t.Fatalf("project execution: %+v %v", result, err)
	}
	p.Close()
	// Every declared file is covered, including resources and bundled SDK.
	for _, name := range []string{"example/events.py", "example/session.py", "example/data/value.json", "seven_agent/force/agent.py"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(output, name)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path, path+".held"); err != nil {
				t.Fatal(err)
			}
			if descriptor.VerifyEntrypoint() == nil {
				t.Fatal("missing file accepted")
			}
			if err := os.Rename(path+".held", path); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o444); err != nil {
				t.Fatal(err)
			}
			if descriptor.VerifyEntrypoint() == nil {
				t.Fatal("tampered file accepted")
			}
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o444); err != nil {
				t.Fatal(err)
			}
			if err := descriptor.VerifyEntrypoint(); err != nil {
				t.Fatal(err)
			}
		})
	}
	var index []json.RawMessage
	indexBody, _ := os.ReadFile(filepath.Join(output, "source-index.json"))
	if json.Unmarshal(indexBody, &index) != nil || len(index) < 10 {
		t.Fatal("incomplete source index")
	}
}
