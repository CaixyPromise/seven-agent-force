// Package distribution materializes an immutable Result Processor release in
// the Worker-owned content-addressed cache.  It intentionally has no database
// dependency: the Server freezes the release ID/digest and only supplies the
// matching object key in the Attempt execution contract.
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
	"path/filepath"
	"strings"

	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

type ArchiveReader func(context.Context, string) (io.ReadCloser, error)

type Materializer struct {
	cacheRoot string
	read      ArchiveReader
}

type Mount struct {
	Source    string
	Digest    string
	ReleaseID int64
}

type cachedRelease struct {
	SchemaVersion int                    `json:"schemaVersion"`
	ReleaseID     int64                  `json:"releaseId,string"`
	Digest        string                 `json:"digest"`
	Manifest      processor.Manifest     `json:"manifest"`
	Files         []processor.SourceFile `json:"files"`
}

func NewRemote(cacheRoot string, read ArchiveReader) *Materializer {
	return &Materializer{cacheRoot: cacheRoot, read: read}
}

func (m *Materializer) Materialize(ctx context.Context, releaseID int64, digest string, objectKey string) (*Mount, error) {
	if m == nil || m.read == nil || strings.TrimSpace(m.cacheRoot) == "" {
		return nil, errors.New("RESULT_PROCESSOR_MATERIALIZER_UNAVAILABLE")
	}
	digest, objectKey = strings.TrimSpace(digest), strings.TrimSpace(objectKey)
	if releaseID == 0 || objectKey == "" || len(digest) != 64 {
		return nil, errors.New("RESULT_PROCESSOR_EXECUTION_IDENTITY_INVALID")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return nil, errors.New("RESULT_PROCESSOR_EXECUTION_IDENTITY_INVALID")
	}
	root := filepath.Join(m.cacheRoot, "result-processors", digest)
	if release, err := readCached(root); err == nil {
		if err = verifyCached(root, releaseID, digest, release); err == nil {
			return &Mount{Source: root, Digest: digest, ReleaseID: releaseID}, nil
		}
		if err = discard(root); err != nil {
			return nil, fmt.Errorf("RESULT_PROCESSOR_CACHE_TAMPERED: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		if err := discard(root); err != nil {
			return nil, fmt.Errorf("RESULT_PROCESSOR_CACHE_TAMPERED: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		return nil, err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(root), ".processor-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	reader, err := m.read(ctx, objectKey)
	if err != nil {
		return nil, fmt.Errorf("RESULT_PROCESSOR_DOWNLOAD_FAILED: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(reader, 8<<20+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(body) > 8<<20 {
		return nil, errors.New("RESULT_PROCESSOR_ARCHIVE_OVERSIZED")
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("RESULT_PROCESSOR_DIGEST_MISMATCH")
	}
	manifest, files, err := processor.ImportArchive(body)
	if err != nil {
		return nil, fmt.Errorf("RESULT_PROCESSOR_ARCHIVE_INVALID: %w", err)
	}
	if err = writeRelease(temporary, cachedRelease{SchemaVersion: 1, ReleaseID: releaseID, Digest: digest, Manifest: manifest, Files: files}); err != nil {
		return nil, err
	}
	if err = makeReadOnly(temporary); err != nil {
		return nil, err
	}
	if err = os.Rename(temporary, root); err != nil {
		if existing, readExisting := readCached(root); readExisting != nil || verifyCached(root, releaseID, digest, existing) != nil {
			return nil, err
		}
	}
	return &Mount{Source: root, Digest: digest, ReleaseID: releaseID}, nil
}

func readCached(root string) (cachedRelease, error) {
	body, err := os.ReadFile(filepath.Join(root, "release.json"))
	if err != nil {
		return cachedRelease{}, err
	}
	var release cachedRelease
	if err = json.Unmarshal(body, &release); err != nil {
		return cachedRelease{}, err
	}
	return release, nil
}

func verifyCached(root string, releaseID int64, digest string, release cachedRelease) error {
	if release.SchemaVersion != 1 || release.ReleaseID != releaseID || release.Digest != digest {
		return errors.New("RESULT_PROCESSOR_CACHE_IDENTITY_MISMATCH")
	}
	archive, canonical, err := processor.CanonicalArchive(release.Manifest, release.Files)
	if err != nil || canonical != digest || len(archive) == 0 {
		return errors.New("RESULT_PROCESSOR_CACHE_DIGEST_MISMATCH")
	}
	for _, file := range release.Files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
			return errors.New("RESULT_PROCESSOR_CACHE_FILE_INVALID")
		}
		body, err := os.ReadFile(path)
		if err != nil || string(body) != file.Content {
			return errors.New("RESULT_PROCESSOR_CACHE_FILE_DIGEST_MISMATCH")
		}
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("RESULT_PROCESSOR_CACHE_SPECIAL_FILE")
		}
		return nil
	})
}

func writeRelease(root string, release cachedRelease) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	for _, file := range release.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(root)+string(os.PathSeparator)) {
			return errors.New("RESULT_PROCESSOR_PATH_ESCAPE")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			return err
		}
	}
	body, _ := json.MarshalIndent(release, "", "  ")
	return os.WriteFile(filepath.Join(root, "release.json"), body, 0o600)
}

func makeReadOnly(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			return os.Chmod(path, 0o444)
		}
		return os.Chmod(path, 0o555)
	})
}

func discard(root string) error {
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if info.IsDir() {
				return os.Chmod(path, 0o700)
			}
			return os.Chmod(path, 0o600)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}
