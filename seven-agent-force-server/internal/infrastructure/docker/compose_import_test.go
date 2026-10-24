package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDiscoveredComposeImportAllowsOriginalFileOutsideWorkspaceRoots(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	writeComposeImportFile(t, path, "nginx:1.25")

	resolved, err := resolveDiscoveredComposeFile(dir, []string{path}, 1024)
	if err != nil {
		t.Fatalf("resolve discovered compose file: %v", err)
	}
	expectedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval expected path: %v", err)
	}
	if resolved.Path != expectedPath || resolved.SHA256 == "" || resolved.Content == "" {
		t.Fatalf("unexpected resolved file: %+v", resolved)
	}
}

func TestResolveDiscoveredComposeImportCleansRelativePathAndSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real-compose.yaml")
	writeComposeImportFile(t, target, "redis:7")
	link := filepath.Join(dir, "nested", "compose-link.yaml")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatalf("mkdir link dir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	resolved, err := resolveDiscoveredComposeFile(dir, []string{"nested/../nested/compose-link.yaml"}, 1024)
	if err != nil {
		t.Fatalf("resolve symlink compose file: %v", err)
	}
	expectedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval expected target: %v", err)
	}
	if resolved.Path != expectedTarget {
		t.Fatalf("expected symlink target path %q, got %q", expectedTarget, resolved.Path)
	}
}

func TestResolveDiscoveredComposeImportRejectsMissingLabel(t *testing.T) {
	if _, err := resolveDiscoveredComposeFile(t.TempDir(), nil, 1024); err == nil {
		t.Fatalf("expected missing config file label rejection")
	}
}

func TestResolveDiscoveredComposeImportRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveDiscoveredComposeFile(dir, []string{dir}, 1024); err == nil {
		t.Fatalf("expected non-regular file rejection")
	}
}

func TestResolveDiscoveredComposeImportRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	writeComposeImportFile(t, path, "nginx:1.25")
	if _, err := resolveDiscoveredComposeFile(dir, []string{path}, 8); err == nil {
		t.Fatalf("expected size cap rejection")
	}
}

func TestComposeImportRejectsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte("services: ["), 0600); err != nil {
		t.Fatalf("write invalid yaml: %v", err)
	}
	if _, err := resolveDiscoveredComposeFile(dir, []string{path}, 1024); err == nil {
		t.Fatalf("expected invalid yaml rejection")
	}
}

func TestWriteExternalManagedComposeFileWritesOriginalPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yaml")
	writeComposeImportFile(t, path, "nginx:1.25")

	result, err := writeExternalManagedComposeFile(path, "services:\n  app:\n    image: redis:7\n", 1024)
	if err != nil {
		t.Fatalf("write external managed compose file: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated compose file: %v", err)
	}
	if string(content) != "services:\n  app:\n    image: redis:7\n" {
		t.Fatalf("unexpected compose file content: %q", string(content))
	}
	expectedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval expected path: %v", err)
	}
	expectedDir := filepath.Dir(expectedPath)
	if result.ComposeFilePath != expectedPath || result.WorkingDir != expectedDir {
		t.Fatalf("unexpected write result: %+v", result)
	}
}

func writeComposeImportFile(t *testing.T, path, image string) {
	t.Helper()
	content := "services:\n  app:\n    image: " + image + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
}
