package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/domain"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

type ArchiveReader func(context.Context, string) (io.ReadCloser, error)
type Materializer struct {
	db        *pgxpool.Pool
	cacheRoot string
	read      ArchiveReader
}
type Mount struct {
	Source   string
	Digest   string
	SpecPath string
}

type configurationSpec struct {
	SchemaVersion       int           `json:"schemaVersion"`
	ConfigurationDigest string        `json:"configurationDigest"`
	Hooks               []packageSpec `json:"hooks"`
}
type packageSpec struct {
	ReleaseID   int64           `json:"releaseId,string"`
	PackageID   int64           `json:"packageId,string"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Digest      string          `json:"digest"`
	RuntimeKind string          `json:"runtimeKind"`
	Root        string          `json:"root"`
	Manifest    domain.Manifest `json:"manifest"`
	Files       []fileSpec      `json:"files"`
}

type fileSpec struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   int64  `json:"mode"`
}

func New(db *pgxpool.Pool, cacheRoot string, read ArchiveReader) *Materializer {
	return &Materializer{db: db, cacheRoot: cacheRoot, read: read}
}

// NewRemote constructs the Worker data-plane materializer. Package archive
// identities and object keys are frozen by the Server in the execution
// contract, so the Worker does not need a PostgreSQL connection.
func NewRemote(cacheRoot string, read ArchiveReader) *Materializer {
	return &Materializer{cacheRoot: cacheRoot, read: read}
}

func (m *Materializer) Materialize(ctx context.Context, tenant int64, snapshot taskdomain.ExecutionSnapshot) (*Mount, error) {
	if len(snapshot.PackageReleases) == 0 {
		return nil, nil
	}
	if m == nil || m.read == nil {
		return nil, errors.New("HOOK_MATERIALIZER_UNAVAILABLE")
	}
	if len(snapshot.HookConfigurationDigest) != 64 {
		return nil, errors.New("HOOK_CONFIGURATION_DIGEST_INVALID")
	}
	if _, err := hex.DecodeString(snapshot.HookConfigurationDigest); err != nil {
		return nil, errors.New("HOOK_CONFIGURATION_DIGEST_INVALID")
	}
	configurationRoot := filepath.Join(m.cacheRoot, "configurations", snapshot.HookConfigurationDigest)
	if err := os.MkdirAll(filepath.Dir(configurationRoot), 0o700); err != nil {
		return nil, err
	}
	releaseLock, err := acquireConfigurationCacheLock(ctx, configurationRoot)
	if err != nil {
		return nil, err
	}
	defer releaseLock()
	specPath := filepath.Join(configurationRoot, "hooks.json")
	if body, err := os.ReadFile(specPath); err == nil {
		var cached configurationSpec
		if json.Unmarshal(body, &cached) == nil && verifyCachedConfiguration(configurationRoot, snapshot, cached) == nil {
			return &Mount{Source: configurationRoot, Digest: snapshot.HookConfigurationDigest, SpecPath: specPath}, nil
		}
		// This exact path is wholly Worker-owned, content-addressed cache. An
		// invalid entry must never be reused or mounted into an Attempt.
		if err := discardCache(configurationRoot); err != nil {
			return nil, fmt.Errorf("HOOK_CACHE_TAMPERED: %w", err)
		}
	} else if _, statErr := os.Lstat(configurationRoot); statErr == nil {
		// A killed writer can leave a directory without hooks.json. Treat it
		// as an incomplete cache entry, never as an immutable winner.
		if err := discardCache(configurationRoot); err != nil {
			return nil, fmt.Errorf("HOOK_CACHE_INCOMPLETE: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("HOOK_CACHE_STAT_FAILED: %w", statErr)
	}
	temporary, err := os.MkdirTemp(filepath.Dir(configurationRoot), ".hooks-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	spec := configurationSpec{SchemaVersion: 1, ConfigurationDigest: snapshot.HookConfigurationDigest, Hooks: []packageSpec{}}
	for index, item := range snapshot.PackageReleases {
		key, storedDigest := item.ArchiveObjectKey, item.Digest
		if key == "" {
			if m.db == nil {
				return nil, errors.New("HOOK_PACKAGE_ARCHIVE_KEY_MISSING")
			}
			if err := m.db.QueryRow(ctx, `SELECT archive_object_key,archive_sha256 FROM workforce_hook_package_releases WHERE id=$1 AND tenant_id=$2`, item.ID, tenant).Scan(&key, &storedDigest); err != nil {
				return nil, fmt.Errorf("HOOK_PACKAGE_RELEASE_MISSING: %w", err)
			}
			if storedDigest != item.Digest {
				return nil, errors.New("HOOK_PACKAGE_DIGEST_MISMATCH")
			}
		}
		reader, err := m.read(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("HOOK_PACKAGE_DOWNLOAD_FAILED: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(reader, domain.MaxArchiveBytes+1))
		reader.Close()
		if readErr != nil || len(body) > domain.MaxArchiveBytes {
			return nil, errors.New("HOOK_PACKAGE_ARCHIVE_OVERSIZED")
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != item.Digest {
			return nil, errors.New("HOOK_PACKAGE_DIGEST_MISMATCH")
		}
		manifest, files, err := domain.ImportArchive(body)
		if err != nil {
			return nil, fmt.Errorf("HOOK_PACKAGE_ARCHIVE_INVALID: %w", err)
		}
		packageRoot := filepath.Join(temporary, "packages", fmt.Sprintf("%03d-%s", index, safeName(item.Name)))
		if err := writePackage(packageRoot, manifest, files); err != nil {
			return nil, err
		}
		containerRoot := filepath.ToSlash(filepath.Join("/opt/seven-worker/managed-hooks", snapshot.HookConfigurationDigest, "packages", fmt.Sprintf("%03d-%s", index, safeName(item.Name))))
		spec.Hooks = append(spec.Hooks, packageSpec{ReleaseID: item.ID, PackageID: item.PackageID, Name: item.Name, Version: item.Version, Digest: item.Digest, RuntimeKind: item.RuntimeKind, Root: containerRoot, Manifest: manifest, Files: sourceFileSpecs(files)})
	}
	specBody, _ := json.MarshalIndent(spec, "", "  ")
	if err := os.WriteFile(filepath.Join(temporary, "hooks.json"), specBody, 0o444); err != nil {
		return nil, err
	}
	if err := makeReadOnly(temporary); err != nil {
		return nil, err
	}
	if err := os.Rename(temporary, configurationRoot); err != nil {
		// The lock serializes current writers, but retain a strict winner check
		// for compatibility with an older process still finishing a write.
		body, readErr := os.ReadFile(specPath)
		var winner configurationSpec
		if readErr != nil || json.Unmarshal(body, &winner) != nil || verifyCachedConfiguration(configurationRoot, snapshot, winner) != nil {
			return nil, fmt.Errorf("HOOK_CACHE_PUBLISH_FAILED: %w", err)
		}
	}
	return &Mount{Source: configurationRoot, Digest: snapshot.HookConfigurationDigest, SpecPath: specPath}, nil
}

func acquireConfigurationCacheLock(ctx context.Context, configurationRoot string) (func(), error) {
	lockRoot := configurationRoot + ".lock"
	for {
		if err := os.Mkdir(lockRoot, 0o700); err == nil {
			return func() { _ = os.RemoveAll(lockRoot) }, nil
		} else if !os.IsExist(err) {
			return nil, fmt.Errorf("HOOK_CACHE_LOCK_FAILED: %w", err)
		}
		if info, err := os.Stat(lockRoot); err == nil && time.Since(info.ModTime()) > 2*time.Minute {
			_ = os.RemoveAll(lockRoot)
			continue
		}
		select {
		case <-ctx.Done():
			return nil, errors.New("HOOK_CACHE_LOCK_CANCELLED")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func discardCache(root string) error {
	if err := filepath.Walk(root, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			return os.Chmod(current, 0o700)
		}
		return os.Chmod(current, 0o600)
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func sourceFileSpecs(files []domain.SourceFile) []fileSpec {
	result := make([]fileSpec, 0, len(files))
	for _, file := range files {
		digest := sha256.Sum256([]byte(file.Content))
		result = append(result, fileSpec{Path: file.Path, SHA256: hex.EncodeToString(digest[:]), Mode: file.Mode})
	}
	return result
}

func verifyCachedConfiguration(root string, snapshot taskdomain.ExecutionSnapshot, spec configurationSpec) error {
	if spec.SchemaVersion != 1 || spec.ConfigurationDigest != snapshot.HookConfigurationDigest || len(spec.Hooks) != len(snapshot.PackageReleases) {
		return errors.New("HOOK_CACHE_IDENTITY_MISMATCH")
	}
	for index, pkg := range spec.Hooks {
		expected := snapshot.PackageReleases[index]
		if pkg.ReleaseID != expected.ID || pkg.PackageID != expected.PackageID || pkg.Name != expected.Name || pkg.Version != expected.Version || pkg.Digest != expected.Digest || pkg.RuntimeKind != expected.RuntimeKind {
			return errors.New("HOOK_CACHE_IDENTITY_MISMATCH")
		}
		packageRoot := filepath.Join(root, "packages", fmt.Sprintf("%03d-%s", index, safeName(expected.Name)))
		expectedContainerRoot := filepath.ToSlash(filepath.Join("/opt/seven-worker/managed-hooks", snapshot.HookConfigurationDigest, "packages", fmt.Sprintf("%03d-%s", index, safeName(expected.Name))))
		if pkg.Root != expectedContainerRoot {
			return errors.New("HOOK_CACHE_PATH_MISMATCH")
		}
		files := make([]domain.SourceFile, 0, len(pkg.Files))
		allowed := map[string]bool{"seven-hook-package.json": true}
		for _, item := range pkg.Files {
			clean, err := safeCacheRelativePath(item.Path)
			if err != nil || clean != item.Path || allowed[clean] {
				return errors.New("HOOK_CACHE_PATH_MISMATCH")
			}
			allowed[clean] = true
			target := filepath.Join(packageRoot, filepath.FromSlash(clean))
			info, err := os.Lstat(target)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
				return errors.New("HOOK_CACHE_FILE_INVALID")
			}
			body, err := os.ReadFile(target)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(body)
			if hex.EncodeToString(digest[:]) != item.SHA256 {
				return errors.New("HOOK_CACHE_FILE_DIGEST_MISMATCH")
			}
			files = append(files, domain.SourceFile{Path: clean, Content: string(body), Mode: item.Mode})
		}
		if err := filepath.Walk(packageRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return errors.New("HOOK_CACHE_SPECIAL_FILE")
			}
			if info.Mode().IsRegular() {
				relative, _ := filepath.Rel(packageRoot, path)
				if !allowed[filepath.ToSlash(relative)] {
					return errors.New("HOOK_CACHE_UNDECLARED_FILE")
				}
			}
			return nil
		}); err != nil {
			return err
		}
		_, digest, err := domain.CanonicalArchive(pkg.Manifest, files)
		if err != nil || digest != expected.Digest {
			return errors.New("HOOK_CACHE_PACKAGE_DIGEST_MISMATCH")
		}
	}
	return nil
}

func safeCacheRelativePath(value string) (string, error) {
	clean := path.Clean(strings.TrimSpace(value))
	if clean == "." || clean == "" || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(value, "\\") {
		return "", errors.New("unsafe cache path")
	}
	return clean, nil
}

func writePackage(root string, manifest domain.Manifest, files []domain.SourceFile) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	manifestBody, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "seven-hook-package.json"), manifestBody, 0o600); err != nil {
		return err
	}
	for _, file := range files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		cleanRoot := filepath.Clean(root) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(target), cleanRoot) {
			return errors.New("HOOK_PACKAGE_PATH_ESCAPE")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			return err
		}
	}
	return nil
}
func makeReadOnly(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			return os.Chmod(path, 0o444)
		}
		if info.IsDir() {
			return os.Chmod(path, 0o555)
		}
		return errors.New("HOOK_CACHE_SPECIAL_FILE")
	})
}
func safeName(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, value)
	if value == "" {
		return "package"
	}
	return value
}
