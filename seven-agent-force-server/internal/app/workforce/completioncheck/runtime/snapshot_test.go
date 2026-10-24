package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRejectsEscapeAndExcludesCredentials(t *testing.T) {
	source := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := Snapshot(context.Background(), source, t.TempDir()); err == nil {
		t.Fatal("symlink escape copied")
	}
	if err := os.Remove(filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".git", "node_modules"} {
		if err := os.Mkdir(filepath.Join(source, dir), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, dir, "private"), []byte("not copied"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "untracked"), []byte("current bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := Snapshot(context.Background(), source, destination); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".git", "node_modules"} {
		if _, err := os.Stat(filepath.Join(destination, dir)); !os.IsNotExist(err) {
			t.Fatal("private/dependency directory copied")
		}
	}
	if data, err := os.ReadFile(filepath.Join(destination, "untracked")); err != nil || string(data) != "current bytes" {
		t.Fatal("untracked edit lost")
	}
	if err := Snapshot(context.Background(), source, filepath.Join(source, "nested")); err == nil {
		t.Fatal("snapshot recursively overlapped source")
	}
}
