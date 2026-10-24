package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

func TestPackSelectionArchiveIsDeterministicAndHonorsSelection(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"dist/app.js": "ok", "dist/app.js.map": "map", "README.md": "readme", "secret.txt": "skip"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	selection := processordomain.PathSelection{Includes: []string{"dist/**", "README.md"}, Excludes: []string{"dist/**/*.map"}}
	first := filepath.Join(t.TempDir(), "first.zip")
	second := filepath.Join(t.TempDir(), "second.zip")
	a, entries, err := PackSelectionArchive(context.Background(), root, selection, "ZIP", first)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := PackSelectionArchive(context.Background(), root, selection, "ZIP", second)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != b.SHA256 || a.SizeBytes != b.SizeBytes {
		t.Fatalf("archive is not deterministic: %#v %#v", a, b)
	}
	if len(entries) != 2 || entries[0].Path != "README.md" || entries[1].Path != "dist/app.js" {
		t.Fatalf("unexpected selection: %#v", entries)
	}
}
