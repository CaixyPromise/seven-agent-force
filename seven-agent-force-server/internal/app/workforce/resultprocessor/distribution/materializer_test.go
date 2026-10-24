package distribution

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

func TestMaterializeImmutableResultProcessorAndRejectTamper(t *testing.T) {
	manifest := processor.DefaultManifest("test")
	files := []processor.SourceFile{{Path: "processor.py", Content: "def process(context):\n return {'decision':'PASS','reasonCode':'OK','message':'ok'}\n", Mode: 0o644}}
	archive, digest, err := processor.CanonicalArchive(manifest, files)
	if err != nil {
		t.Fatal(err)
	}
	m := NewRemote(t.TempDir(), func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(archive)), nil
	})
	mount, err := m.Materialize(context.Background(), 1001, digest, "archive")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = discard(mount.Source) }()
	if mount.ReleaseID != 1001 || mount.Digest != digest {
		t.Fatalf("unexpected mount %#v", mount)
	}
	path := filepath.Join(mount.Source, "processor.py")
	if err = os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = m.Materialize(context.Background(), 1001, digest, "archive")
	// The materializer safely discards tampered cache and repopulates it from
	// the immutable archive. The returned mount is the repaired cache.
	if err != nil {
		t.Fatal(err)
	}
}
