package main

import (
	"encoding/json"
	check "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCommandExecutesFrozenCustomInput(t *testing.T) {
	t.Setenv("CHECK_PRIVATE_SECRET", "never-persist-raw-secret")
	input := check.Input{Mode: "SCRIPT", Script: "test -z \"$CHECK_PRIVATE_SECRET\" || exit 1; mkdir -p dist && printf built > dist/output || exit 2; printf 'raw command output must not persist\\n'", ReferenceAssetIDs: []int64{9}}
	raw, _ := json.Marshal(input)
	root := t.TempDir()
	path := filepath.Join(root, "input.json")
	if err := os.WriteFile(path, raw, 0444); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	saved := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = saved }()
	workspace := t.TempDir()
	err = runCheck([]string{"--input", path, "--source", root, "--repository", workspace, "--timeout", "1s"})
	writer.Close()
	os.Stdout = saved
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(reader)
	var result check.Observation
	if json.Unmarshal(body, &result) != nil || result.Status != check.Passed || result.TestCountKnown {
		t.Fatalf("invalid command evidence %s", body)
	}
	if strings.Contains(string(body), "raw command output") || strings.Contains(string(body), "never-persist") {
		t.Fatal("raw output leaked")
	}
	digest, _ := check.Digest(input)
	if result.CheckDigest != digest {
		t.Fatal("frozen definition lost")
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "dist/output")); err != nil || string(data) != "built" {
		t.Fatal("private copy was not writable")
	}
	if _, err := os.Stat(filepath.Join(root, "dist")); !os.IsNotExist(err) {
		t.Fatal("source changed by check command")
	}
}

func TestCheckCommandMakesPreparedDependenciesReadOnlyInputAvailable(t *testing.T) {
	input := check.Input{Mode: "SCRIPT", Script: "test -f node_modules/pkg/value && printf '1..1\nok 1\n'"}
	raw, _ := json.Marshal(input)
	source := t.TempDir()
	inputPath := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(filepath.Join(source, "check.sh"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	dependencies := filepath.Join(t.TempDir(), "node_modules")
	if err := os.MkdirAll(filepath.Join(dependencies, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependencies, "pkg", "value"), []byte("installed"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, raw, 0o444); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	saved := os.Stdout
	os.Stdout = writer
	workspace := t.TempDir()
	err = runCheck([]string{"--input", inputPath, "--source", source, "--repository", workspace, "--dependencies", dependencies, "--timeout", "1s"})
	_ = writer.Close()
	os.Stdout = saved
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(reader)
	var result check.Observation
	if json.Unmarshal(body, &result) != nil || result.Status != check.Passed || !result.TestCountKnown || result.TestCount != 1 {
		t.Fatalf("invalid dependency-backed check evidence %s", body)
	}
	info, err := os.Lstat(filepath.Join(workspace, "node_modules"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("prepared dependencies were not linked into the private workspace")
	}
}
