package domain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestCanonicalArchiveIsStableAndIncludesModes(t *testing.T) {
	manifest := []byte(`{"name":"demo"}\n`)
	left, leftDigest, err := CanonicalArchive("processor.json", manifest, []SourceFile{{Path: "b.py", Content: "b", Mode: 0o644}, {Path: "a.py", Content: "a", Mode: 0o755}})
	if err != nil {
		t.Fatalf("canonical archive: %v", err)
	}
	right, rightDigest, err := CanonicalArchive("processor.json", manifest, []SourceFile{{Path: "a.py", Content: "a", Mode: 0o755}, {Path: "b.py", Content: "b", Mode: 0o644}})
	if err != nil {
		t.Fatalf("canonical archive reordered: %v", err)
	}
	if !bytes.Equal(left, right) || leftDigest != rightDigest {
		t.Fatalf("canonical archive drifted: %s != %s", leftDigest, rightDigest)
	}
	_, changedDigest, err := CanonicalArchive("processor.json", manifest, []SourceFile{{Path: "a.py", Content: "a", Mode: 0o644}, {Path: "b.py", Content: "b", Mode: 0o644}})
	if err != nil || changedDigest == leftDigest {
		t.Fatalf("mode change must change digest, digest=%s err=%v", changedDigest, err)
	}
}

func TestImportArchiveRejectsLinkAndTraversal(t *testing.T) {
	for name, header := range map[string]*tar.Header{
		"link":      {Name: "processor.json", Typeflag: tar.TypeSymlink, Linkname: "processor.py"},
		"traversal": {Name: "../processor.json", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2},
	} {
		t.Run(name, func(t *testing.T) {
			var raw bytes.Buffer
			gzipWriter := gzip.NewWriter(&raw)
			tarWriter := tar.NewWriter(gzipWriter)
			if err := tarWriter.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
			if header.Size > 0 {
				_, _ = tarWriter.Write([]byte("{}"))
			}
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			if _, _, err := ImportArchive("processor.json", raw.Bytes()); err == nil {
				t.Fatal("unsafe archive was accepted")
			}
		})
	}
}
