package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoredBindKeepsNativeSessionPrivate(t *testing.T) {
	root := t.TempDir()
	repo, state := filepath.Join(root, "repo"), filepath.Join(root, "state")
	for _, p := range []string{repo, filepath.Join(state, "codex")} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(state, "codex", "fake-session.json")
	if err := os.WriteFile(file, []byte(`{"session":"preserved"}`), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := prepareRestoredBindPermissions(repo, state); err != nil {
			t.Fatal(err)
		}
		for p, want := range map[string]os.FileMode{repo: 0777, state: 0700, filepath.Dir(file): 0700, file: 0600} {
			info, err := os.Stat(p)
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("%s: mode contract failed: %v", p, err)
			}
		}
	}
	body, err := os.ReadFile(file)
	if err != nil || string(body) != `{"session":"preserved"}` {
		t.Fatal("session changed", err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(state, "link")); err != nil {
		t.Fatal(err)
	}
	if err := prepareRestoredBindPermissions(repo, state); err == nil {
		t.Fatal("accepted symlink")
	}
	info, err := os.Stat(outside)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatal("changed symlink target", err)
	}
}
