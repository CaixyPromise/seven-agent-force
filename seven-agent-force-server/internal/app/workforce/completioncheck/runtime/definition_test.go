package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
)

func TestPackageCheckCannotSilentlyChangeToTrue(t *testing.T) {
	root := t.TempDir()
	input := domain.Input{Mode: "COMMAND", Command: "pnpm test"}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"scripts":{"test":"node --test"},"description":"old"}`)
	a, err := checkDefinitionDigest(root, input)
	if err != nil || a == "" {
		t.Fatal(err)
	}
	write(`{"description":"new","scripts":{"test":"node --test"}}`)
	b, err := checkDefinitionDigest(root, input)
	if err != nil || a != b {
		t.Fatal("noncheck metadata changed identity")
	}
	write(`{"scripts":{"test":"true"}}`)
	b, err = checkDefinitionDigest(root, input)
	if err != nil || a == b {
		t.Fatal("weakened script escaped identity")
	}
	if err := os.Remove(filepath.Join(root, "package.json")); err != nil {
		t.Fatal(err)
	}
	c, err := checkDefinitionDigest(root, input)
	if err != nil || c == a || c == b {
		t.Fatal("deleted definition escaped identity")
	}
}

func TestDirectScriptDefinitionAndBoundary(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "verify.sh")
	if err := os.WriteFile(file, []byte("exit 1"), 0600); err != nil {
		t.Fatal(err)
	}
	input := domain.Input{Mode: "COMMAND", Command: "sh ./verify.sh"}
	a, err := checkDefinitionDigest(root, input)
	if err != nil || a == "" {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("exit 0"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := checkDefinitionDigest(root, input)
	if err != nil || a == b {
		t.Fatal("script drift ignored")
	}
	input.Command = "sh ../outside.sh"
	if _, err = checkDefinitionDigest(root, input); err == nil {
		t.Fatal("outside entry accepted")
	}
	input.Command = "custom-check --mode project"
	if digest, err := checkDefinitionDigest(root, input); err != nil || digest != "" {
		t.Fatal("unrecognized semantics claimed")
	}
}
