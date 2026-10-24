package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestReviewReferencesIsolatedVerifiedAndRecoverable(t *testing.T) {
	task := t.TempDir()
	source := filepath.Join(task, "input", "references", "123")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte("Acceptance criteria, not proof of passing tests.")
	if err := os.WriteFile(filepath.Join(source, "criteria.txt"), body, 0444); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	doc, _ := json.Marshal(map[string]any{"references": []any{map[string]any{"referenceAssetId": "123", "name": "criteria.txt", "sizeBytes": strconv.Itoa(len(body)), "contentSha256": hex.EncodeToString(sum[:])}}})
	destination := filepath.Join(t.TempDir(), "references")
	for i := 0; i < 2; i++ {
		ok, err := materializeReviewReferences(task, destination, doc)
		if err != nil || !ok {
			t.Fatalf("copy/recover: %v", err)
		}
	}
	target := filepath.Join(destination, "123", "content")
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm()&0222 != 0 {
		t.Fatal("reference writable")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "criteria.txt"), target); err != nil {
		t.Fatal(err)
	}
	if _, err := materializeReviewReferences(task, destination, doc); err == nil {
		t.Fatal("recovery accepted symlink")
	}
}

func TestReviewReferencesMissingOrTamperedFailClosed(t *testing.T) {
	root := t.TempDir()
	doc := []byte(`{"references":[{"referenceAssetId":"1","name":"missing","contentSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sizeBytes":"1"}]}`)
	if _, err := materializeReviewReferences(root, filepath.Join(t.TempDir(), "refs"), doc); err == nil {
		t.Fatal("missing evidence accepted")
	}
	if present, err := materializeReviewReferences(root, filepath.Join(t.TempDir(), "refs"), []byte(`{"instruction":"x"}`)); err != nil || present {
		t.Fatal("legacy input changed")
	}
}
