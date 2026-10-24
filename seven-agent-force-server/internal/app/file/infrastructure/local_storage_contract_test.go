package infrastructure

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestLocalStorageContract(t *testing.T) {
	storage, err := NewLocalStorage(config.StorageConfig{Location: t.TempDir(), ForceCreated: true, StaticPath: "/static"})
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	stored, err := storage.Save(context.Background(), "a/b/file.txt", strings.NewReader("hello"), "text/plain")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if stored.Size != 5 || stored.SHA256 == "" || stored.StoragePath != "a/b/file.txt" {
		t.Fatalf("unexpected stored object: %+v", stored)
	}
	opened, err := storage.Open(domain.FileInfo{StoragePath: stored.StoragePath, ContentType: "text/plain", FileInnerName: "file.txt"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer opened.File.Close()
	body, err := io.ReadAll(opened.File)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("unexpected body: %q", string(body))
	}
	if url := storage.PublicURL(stored.StoragePath); url != "/static/a%2Fb%2Ffile.txt" {
		t.Fatalf("unexpected public url: %s", url)
	}
	if err := storage.Delete(stored.StoragePath); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := storage.Open(domain.FileInfo{StoragePath: stored.StoragePath}); err == nil {
		t.Fatal("expected deleted object to be missing")
	}
}

func TestLocalStorageRejectsPathTraversal(t *testing.T) {
	storage, err := NewLocalStorage(config.StorageConfig{Location: t.TempDir(), ForceCreated: true})
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	if _, err := storage.Save(context.Background(), "../escape.txt", strings.NewReader("x"), "text/plain"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}
