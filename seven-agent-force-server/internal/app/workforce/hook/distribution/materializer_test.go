package distribution

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/domain"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func TestMaterializeReplacesIncompleteCacheEntry(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(cacheRoot, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	configurationDigest := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	manifest := domain.Manifest{
		SchemaVersion: domain.PackageSchema, Name: "guard", Version: "1.0.0", TrustClass: "PLATFORM_TRUSTED",
		Runtime:     domain.Runtime{Kind: "shell"},
		Entrypoints: []domain.Entrypoint{{Event: "attempt.prepare", Path: "hooks/a.sh", TimeoutMS: 5000, FailureMode: "BLOCK"}},
		Limits:      domain.Limits{StdinBytes: domain.MaxInputBytes, StdoutBytes: domain.MaxOutputBytes, StderrBytes: domain.MaxErrorBytes},
	}
	files := []domain.SourceFile{{Path: "hooks/a.sh", Content: "echo ok", Mode: 0o755}}
	archive, packageDigest, err := domain.CanonicalArchive(manifest, files)
	if err != nil {
		t.Fatal(err)
	}
	configurationRoot := filepath.Join(cacheRoot, "configurations", configurationDigest)
	if err := os.MkdirAll(filepath.Dir(configurationRoot), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(configurationRoot, 0o555); err != nil {
		t.Fatal(err)
	}
	materializer := NewRemote(cacheRoot, func(_ context.Context, key string) (io.ReadCloser, error) {
		if key != "hooks/guard.tar.gz" {
			t.Fatalf("object key = %q", key)
		}
		return io.NopCloser(bytes.NewReader(archive)), nil
	})
	snapshot := taskdomain.ExecutionSnapshot{
		HookConfigurationDigest: configurationDigest,
		PackageReleases:         []taskdomain.HookPackageReleaseSnapshot{{ID: 101, PackageID: 102, Name: "guard", Version: "1.0.0", Digest: packageDigest, RuntimeKind: "shell", ArchiveObjectKey: "hooks/guard.tar.gz"}},
	}
	mount, err := materializer.Materialize(context.Background(), 1001, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if mount.SpecPath != filepath.Join(configurationRoot, "hooks.json") {
		t.Fatalf("spec path = %q", mount.SpecPath)
	}
	if _, err := os.Stat(mount.SpecPath); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializeEmptyConfigurationNeedsNoMaterializerOrMount(t *testing.T) {
	var materializer *Materializer
	mount, err := materializer.Materialize(context.Background(), 1001, taskdomain.ExecutionSnapshot{})
	if err != nil || mount != nil {
		t.Fatalf("empty Hook configuration should not require a materializer: mount=%#v err=%v", mount, err)
	}
}

func TestMaterializeConfiguredHooksFailsClosedWithoutMaterializer(t *testing.T) {
	var materializer *Materializer
	_, err := materializer.Materialize(context.Background(), 1001, taskdomain.ExecutionSnapshot{
		HookConfigurationDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PackageReleases:         []taskdomain.HookPackageReleaseSnapshot{{ID: 101}},
	})
	if err == nil || err.Error() != "HOOK_MATERIALIZER_UNAVAILABLE" {
		t.Fatalf("configured Hook started without materializer: %v", err)
	}
}

func TestWritePackageCannotEscapeAndBecomesReadOnly(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	manifest := domain.Manifest{SchemaVersion: domain.PackageSchema}
	if err := writePackage(filepath.Join(root, "pkg"), manifest, []domain.SourceFile{{Path: "../escape", Content: "bad"}}); err == nil {
		t.Fatal("path traversal was accepted")
	}
	packageRoot := filepath.Join(root, "valid")
	if err := writePackage(packageRoot, manifest, []domain.SourceFile{{Path: "hooks/a.sh", Content: "echo ok"}}); err != nil {
		t.Fatal(err)
	}
	if err := makeReadOnly(packageRoot); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(packageRoot, "hooks/a.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o444 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

func TestVerifyCachedConfigurationRejectsTamperedPackageBytes(t *testing.T) {
	root := t.TempDir()
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	manifest := domain.Manifest{
		SchemaVersion: domain.PackageSchema, Name: "guard", Version: "1.0.0", TrustClass: "PLATFORM_TRUSTED",
		Runtime:     domain.Runtime{Kind: "shell"},
		Entrypoints: []domain.Entrypoint{{Event: "attempt.prepare", Path: "hooks/a.sh", TimeoutMS: 5000, FailureMode: "BLOCK"}},
		Limits:      domain.Limits{StdinBytes: domain.MaxInputBytes, StdoutBytes: domain.MaxOutputBytes, StderrBytes: domain.MaxErrorBytes},
	}
	files := []domain.SourceFile{{Path: "hooks/a.sh", Content: "echo ok", Mode: 0o755}}
	_, packageDigest, err := domain.CanonicalArchive(manifest, files)
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := filepath.Join(root, "packages", "000-guard")
	if err := writePackage(packageRoot, manifest, files); err != nil {
		t.Fatal(err)
	}
	if err := makeReadOnly(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	snapshot := taskdomain.ExecutionSnapshot{HookConfigurationDigest: digest, PackageReleases: []taskdomain.HookPackageReleaseSnapshot{{ID: 101, PackageID: 102, Name: "guard", Version: "1.0.0", Digest: packageDigest, RuntimeKind: "shell"}}}
	spec := configurationSpec{SchemaVersion: 1, ConfigurationDigest: digest, Hooks: []packageSpec{{ReleaseID: 101, PackageID: 102, Name: "guard", Version: "1.0.0", Digest: packageDigest, RuntimeKind: "shell", Root: "/opt/seven-worker/managed-hooks/" + digest + "/packages/000-guard", Manifest: manifest, Files: sourceFileSpecs(files)}}}
	if err := verifyCachedConfiguration(root, snapshot, spec); err != nil {
		t.Fatalf("valid cache rejected: %v", err)
	}
	target := filepath.Join(packageRoot, "hooks/a.sh")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("tampered"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := verifyCachedConfiguration(root, snapshot, spec); err == nil {
		t.Fatal("tampered cache was accepted")
	}
}
