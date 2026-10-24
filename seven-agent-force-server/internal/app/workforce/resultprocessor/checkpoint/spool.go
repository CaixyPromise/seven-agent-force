package checkpoint

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const encryptedMagic = "SEVENCP1"

type SpoolBlob struct {
	Path      string
	SHA256    string
	SizeBytes int64
	FileCount int
}

type SpoolFrozen struct {
	Identity         string
	FormatVersion    int
	RepositoryBundle SpoolBlob
	WorkspaceOverlay SpoolBlob
	Session          SpoolBlob
	Manifest         SpoolBlob
	Selection        ResultFileSelection
}

// PackDeclaredOutput freezes one Processor-declared directory with the same
// canonical path and file-type rules as a Continuation workspace. The caller
// has already resolved the declaration against the trusted repository root.
func PackDeclaredOutput(ctx context.Context, repoRoot, source, destination string) (SpoolBlob, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return SpoolBlob{}, err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return SpoolBlob{}, err
	}
	if source != repoRoot && !strings.HasPrefix(source, repoRoot+string(os.PathSeparator)) {
		return SpoolBlob{}, errors.New("RESULT_OUTPUT_SOURCE_OUTSIDE_WORKSPACE")
	}
	return packArchiveFile(ctx, repoRoot, []string{source}, "output", nil, destination, MaxArchiveBytes)
}

// FreezeToSpool is the production checkpoint packer. It never holds an
// archive in memory: each canonical blob is written to a 0700 Worker-owned
// staging directory, fsynced, and atomically renamed before it is returned.
func FreezeToSpool(ctx context.Context, input Input, spoolRoot string) (SpoolFrozen, error) {
	if err := validateInput(input); err != nil {
		return SpoolFrozen{}, err
	}
	if err := os.MkdirAll(spoolRoot, 0o700); err != nil {
		return SpoolFrozen{}, err
	}
	if err := os.Chmod(spoolRoot, 0o700); err != nil {
		return SpoolFrozen{}, err
	}
	identity := Identity(input)
	selection, err := SelectResult(ctx, SelectionInput{RepoRoot: input.RepoRoot, BaseCommit: input.BaseCommit, Outputs: input.Outputs, ContinuationIncludes: input.ContinuationIncludes})
	if err != nil {
		return SpoolFrozen{}, fmt.Errorf("select checkpoint result: %w", err)
	}
	if input.ResultCommit == "" || input.ResultTree == "" {
		return SpoolFrozen{}, errors.New("CHECKPOINT_RESULT_IDENTITY_REQUIRED")
	}
	bundle, err := writeRepositoryBundle(ctx, input.RepoRoot, identity, input.ResultCommit, input.ResultTree, filepath.Join(spoolRoot, "repository.bundle"))
	if err != nil {
		return SpoolFrozen{}, fmt.Errorf("pack checkpoint repository bundle: %w", err)
	}
	overlaySources := make([]string, 0, len(selection.OverlayIndex))
	for _, entry := range selection.OverlayIndex {
		overlaySources = append(overlaySources, filepath.Join(input.RepoRoot, filepath.FromSlash(entry.Path)))
	}
	overlay, err := packArchiveFile(ctx, input.RepoRoot, overlaySources, "overlay", nil, filepath.Join(spoolRoot, "workspace-overlay.tar.gz"), MaxArchiveBytes)
	if err != nil {
		return SpoolFrozen{}, fmt.Errorf("pack checkpoint workspace overlay: %w", err)
	}
	sessionSources := []string{filepath.Join(input.StateRoot, "codex"), filepath.Join(input.StateRoot, "session.json")}
	session, err := packArchiveFile(ctx, input.StateRoot, sessionSources, "session", skipSecretMaterialization, filepath.Join(spoolRoot, "session.tar.gz"), MaxSessionArchiveBytes)
	if err != nil {
		return SpoolFrozen{}, fmt.Errorf("pack checkpoint session: %w", err)
	}
	manifestBody, err := buildManifest(input, identity, bundle, overlay, session, selection)
	if err != nil {
		return SpoolFrozen{}, err
	}
	manifest, err := writeAtomic(filepath.Join(spoolRoot, "identity.json"), func(writer io.Writer) error { _, e := writer.Write(manifestBody); return e })
	if err != nil {
		return SpoolFrozen{}, err
	}
	return SpoolFrozen{Identity: identity, FormatVersion: 2, RepositoryBundle: bundle, WorkspaceOverlay: overlay, Session: session, Manifest: manifest, Selection: selection}, nil
}

func buildManifest(input Input, identity string, bundle, overlay, session SpoolBlob, selection ResultFileSelection) ([]byte, error) {
	value := map[string]any{
		"schemaVersion": 2, "format": "REPOSITORY_BUNDLE_OVERLAY", "checkpointIdentity": identity, "taskId": strconv.FormatInt(input.TaskID, 10), "taskTurnId": strconv.FormatInt(input.TaskTurnID, 10), "attemptId": strconv.FormatInt(input.AttemptID, 10),
		"generation": input.Generation, "evaluationCycleId": strconv.FormatInt(input.EvaluationCycleID, 10), "decision": input.Decision, "sessionDigest": input.SessionDigest,
		"baseCommit": input.BaseCommit, "resultCommit": input.ResultCommit, "resultTree": input.ResultTree, "taskExecutionSnapshotDigest": input.ExecutionSnapshotDigest,
		"harnessImageDigest": input.HarnessImageDigest, "hookConfigurationDigest": input.HookConfigurationDigest, "resultProcessorDigest": input.ResultProcessorDigest,
		"modelCredentialIdentity": input.ModelCredentialIdentity, "isolationProfile": input.IsolationProfile,
		"repositoryBundle": map[string]any{"sha256": bundle.SHA256, "sizeBytes": bundle.SizeBytes, "ref": "refs/seven/checkpoints/" + identity},
		"workspaceOverlay": map[string]any{"sha256": overlay.SHA256, "sizeBytes": overlay.SizeBytes, "files": overlay.FileCount, "selectionDigest": selection.SelectionDigest},
		"resultSelection":  map[string]any{"selectionDigest": selection.SelectionDigest, "gitignoreDigest": selection.GitignoreDigest, "platformIgnorePolicyDigest": selection.PolicyDigest},
		"session":          map[string]any{"sha256": session.SHA256, "sizeBytes": session.SizeBytes, "files": session.FileCount},
	}
	return jsonMarshalCanonical(value)
}

func writeRepositoryBundle(ctx context.Context, repo, checkpointIdentity, commit, expectedTree, destination string) (SpoolBlob, error) {
	tree, err := gitOutputControlled(ctx, repo, nil, "rev-parse", commit+"^{tree}")
	if err != nil || tree != expectedTree {
		return SpoolBlob{}, errors.New("CHECKPOINT_RESULT_IDENTITY_MISMATCH")
	}
	ref := "refs/seven/checkpoints/" + checkpointIdentity
	if _, err = gitOutputControlled(ctx, repo, nil, "update-ref", ref, commit); err != nil {
		return SpoolBlob{}, err
	}
	defer func() { _, _ = gitOutputControlled(context.Background(), repo, nil, "update-ref", "-d", ref) }()
	if _, err = gitOutputControlled(ctx, repo, nil, "bundle", "create", destination+".tmp", ref); err != nil {
		return SpoolBlob{}, err
	}
	if err = os.Chmod(destination+".tmp", 0o600); err != nil {
		_ = os.Remove(destination + ".tmp")
		return SpoolBlob{}, err
	}
	file, err := os.OpenFile(destination+".tmp", os.O_RDWR, 0o600)
	if err != nil {
		return SpoolBlob{}, err
	}
	if err = file.Sync(); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err != nil {
		_ = os.Remove(destination + ".tmp")
		return SpoolBlob{}, err
	}
	if err = os.Rename(destination+".tmp", destination); err != nil {
		return SpoolBlob{}, err
	}
	verifyRoot, err := os.MkdirTemp(filepath.Dir(destination), ".bundle-verify-")
	if err != nil {
		return SpoolBlob{}, err
	}
	defer os.RemoveAll(verifyRoot)
	clone := filepath.Join(verifyRoot, "repo")
	if output, initErr := exec.CommandContext(ctx, "git", "init", "-q", clone).CombinedOutput(); initErr != nil {
		return SpoolBlob{}, fmt.Errorf("CHECKPOINT_BUNDLE_VERIFY_FAILED: %w: %s", initErr, output)
	}
	if output, fetchErr := exec.CommandContext(ctx, "git", "-C", clone, "fetch", "-q", destination, ref+":"+ref).CombinedOutput(); fetchErr != nil {
		return SpoolBlob{}, fmt.Errorf("CHECKPOINT_BUNDLE_VERIFY_FAILED: %w: %s", fetchErr, output)
	}
	if _, err = gitOutputControlled(ctx, clone, nil, "checkout", "--detach", "-q", commit); err != nil {
		return SpoolBlob{}, err
	}
	verifiedCommit, err := gitOutputControlled(ctx, clone, nil, "rev-parse", "HEAD")
	if err != nil || verifiedCommit != commit {
		return SpoolBlob{}, errors.New("CHECKPOINT_BUNDLE_COMMIT_MISMATCH")
	}
	verifiedTree, err := gitOutputControlled(ctx, clone, nil, "rev-parse", "HEAD^{tree}")
	if err != nil || verifiedTree != expectedTree {
		return SpoolBlob{}, errors.New("CHECKPOINT_BUNDLE_TREE_MISMATCH")
	}
	sha, err := fileDigest(destination)
	if err != nil {
		return SpoolBlob{}, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return SpoolBlob{}, err
	}
	if info.Size() > MaxArchiveBytes {
		_ = os.Remove(destination)
		return SpoolBlob{}, errors.New("CHECKPOINT_ARCHIVE_TOO_LARGE")
	}
	return SpoolBlob{Path: destination, SHA256: sha, SizeBytes: info.Size()}, nil
}

func jsonMarshalCanonical(value any) ([]byte, error) {
	// encoding/json sorts string map keys, which is the canonical representation
	// used throughout the current Workforce identity contracts.
	return json.Marshal(value)
}

func packArchiveFile(ctx context.Context, root string, sources []string, prefix string, skip skipFunc, destination string, limit int64) (SpoolBlob, error) {
	entries, err := collectEntries(ctx, root, sources, prefix, skip)
	if err != nil {
		return SpoolBlob{}, err
	}
	if len(entries) > MaxArchiveFiles {
		return SpoolBlob{}, errors.New("CHECKPOINT_FILE_COUNT_EXCEEDED")
	}
	result, err := writeAtomic(destination, func(target io.Writer) error {
		gzipWriter := gzip.NewWriter(target)
		gzipWriter.Name = ""
		gzipWriter.Comment = ""
		gzipWriter.ModTime = time.Unix(0, 0).UTC()
		tarWriter := tar.NewWriter(gzipWriter)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			header := &tar.Header{Name: entry.name, Mode: 0o600, Size: entry.size, ModTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX}
			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}
			file, openErr := os.Open(entry.source)
			if openErr != nil {
				return openErr
			}
			written, copyErr := io.Copy(tarWriter, io.LimitReader(file, entry.size+1))
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil || written != entry.size {
				return errors.New("CHECKPOINT_SOURCE_READ_FAILED")
			}
		}
		if err := tarWriter.Close(); err != nil {
			return err
		}
		return gzipWriter.Close()
	})
	if err != nil {
		return SpoolBlob{}, err
	}
	if result.SizeBytes > limit {
		_ = os.Remove(result.Path)
		return SpoolBlob{}, errors.New("CHECKPOINT_ARCHIVE_TOO_LARGE")
	}
	result.FileCount = len(entries)
	return result, nil
}

func collectEntries(ctx context.Context, root string, sources []string, prefix string, skip skipFunc) ([]packEntry, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	entries := make([]packEntry, 0, 64)
	seen := map[string]struct{}{}
	seenFold := map[string]struct{}{}
	for _, source := range sources {
		source = filepath.Clean(source)
		if source != root && !strings.HasPrefix(source, root+string(os.PathSeparator)) {
			return nil, errors.New("CHECKPOINT_SOURCE_OUTSIDE_ROOT")
		}
		if _, err = os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if relative == "." {
				return nil
			}
			relative = filepath.ToSlash(relative)
			if skip != nil && skip(relative, entry) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("CHECKPOINT_SYMLINK_FORBIDDEN")
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
				return errors.New("CHECKPOINT_FILE_TYPE_FORBIDDEN")
			}
			if info.Size() > MaxFileBytes {
				return errors.New("CHECKPOINT_FILE_TOO_LARGE")
			}
			name := pathJoin(prefix, relative)
			fold := strings.ToLower(name)
			if _, ok := seen[name]; ok {
				return errors.New("CHECKPOINT_ARCHIVE_PATH_DUPLICATE")
			}
			if _, ok := seenFold[fold]; ok {
				return errors.New("CHECKPOINT_ARCHIVE_CASE_CONFLICT")
			}
			seen[name] = struct{}{}
			seenFold[fold] = struct{}{}
			entries = append(entries, packEntry{source: path, name: name, size: info.Size()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries, nil
}

func writeAtomic(destination string, write func(io.Writer) error) (SpoolBlob, error) {
	temporary := destination + ".tmp"
	_ = os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return SpoolBlob{}, err
	}
	hash := sha256.New()
	writer := io.MultiWriter(file, hash)
	if err = write(writer); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return SpoolBlob{}, err
	}
	if err = os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return SpoolBlob{}, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return SpoolBlob{}, err
	}
	return SpoolBlob{Path: destination, SHA256: hex.EncodeToString(hash.Sum(nil)), SizeBytes: info.Size()}, nil
}

// EncryptStream implements a bounded chunked AES-GCM format so large
// checkpoints remain streaming while every chunk is independently authenticated.
func EncryptStream(source io.Reader, destination io.Writer, key, aad []byte) error {
	return cryptStream(source, destination, key, aad, true)
}
func DecryptStream(source io.Reader, destination io.Writer, key, aad []byte) error {
	return cryptStream(source, destination, key, aad, false)
}

func cryptStream(source io.Reader, destination io.Writer, key, aad []byte, encrypt bool) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	const chunkSize = 1 << 20
	if encrypt {
		base := make([]byte, gcm.NonceSize())
		if _, err = io.ReadFull(rand.Reader, base); err != nil {
			return err
		}
		if _, err = destination.Write([]byte(encryptedMagic)); err != nil {
			return err
		}
		if _, err = destination.Write(base); err != nil {
			return err
		}
		reader := bufio.NewReader(source)
		counter := uint32(0)
		for {
			plain := make([]byte, chunkSize)
			n, readErr := io.ReadFull(reader, plain)
			if readErr == io.EOF {
				break
			}
			if readErr != nil && readErr != io.ErrUnexpectedEOF {
				return readErr
			}
			nonce := append([]byte(nil), base...)
			binary.BigEndian.PutUint32(nonce[len(nonce)-4:], counter)
			sealed := gcm.Seal(nil, nonce, plain[:n], append(aad, byte(counter>>24), byte(counter>>16), byte(counter>>8), byte(counter)))
			var length [4]byte
			binary.BigEndian.PutUint32(length[:], uint32(len(sealed)))
			if _, err = destination.Write(length[:]); err != nil {
				return err
			}
			if _, err = destination.Write(sealed); err != nil {
				return err
			}
			counter++
			if readErr == io.ErrUnexpectedEOF {
				break
			}
		}
		return nil
	}
	magic := make([]byte, len(encryptedMagic))
	if _, err = io.ReadFull(source, magic); err != nil || string(magic) != encryptedMagic {
		return errors.New("CONTINUATION_ARCHIVE_INVALID")
	}
	base := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(source, base); err != nil {
		return errors.New("CONTINUATION_ARCHIVE_INVALID")
	}
	counter := uint32(0)
	for {
		var length [4]byte
		_, readErr := io.ReadFull(source, length[:])
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		size := binary.BigEndian.Uint32(length[:])
		if size > chunkSize+uint32(gcm.Overhead()) {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		sealed := make([]byte, size)
		if _, err = io.ReadFull(source, sealed); err != nil {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		nonce := append([]byte(nil), base...)
		binary.BigEndian.PutUint32(nonce[len(nonce)-4:], counter)
		plain, openErr := gcm.Open(nil, nonce, sealed, append(aad, byte(counter>>24), byte(counter>>16), byte(counter>>8), byte(counter)))
		if openErr != nil {
			return errors.New("CONTINUATION_CHECKPOINT_TAMPERED")
		}
		if _, err = destination.Write(plain); err != nil {
			return err
		}
		counter++
	}
}

// Extract restores a canonical archive into a staging directory and renames it
// into place only after all path, type, size, count, ratio, and digest checks pass.
func Extract(ctx context.Context, archive io.Reader, destination, expectedPrefix string, maxBytes int64) error {
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".seven-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return errors.New("CONTINUATION_ARCHIVE_INVALID")
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	seen := map[string]struct{}{}
	seenFold := map[string]struct{}{}
	var total int64
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		count++
		if count > MaxArchiveFiles {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		clean := filepath.ToSlash(filepath.Clean(header.Name))
		prefix := strings.TrimSuffix(expectedPrefix, "/") + "/"
		if !strings.HasPrefix(clean, prefix) || strings.HasPrefix(clean, "/") || strings.Contains(clean, "../") {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		relative := strings.TrimPrefix(clean, prefix)
		if relative == "" || relative == "." {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		fold := strings.ToLower(relative)
		if _, ok := seen[relative]; ok {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		if _, ok := seenFold[fold]; ok {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		seen[relative] = struct{}{}
		seenFold[fold] = struct{}{}
		if header.Typeflag != tar.TypeReg || header.Linkname != "" || header.Mode&(int64(os.ModeSetuid)|int64(os.ModeSetgid)) != 0 {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		if header.Size < 0 || header.Size > MaxFileBytes {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		total += header.Size
		if total > maxBytes {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		target := filepath.Join(staging, filepath.FromSlash(relative))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(staging)+string(os.PathSeparator)) {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		file, openErr := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if openErr != nil {
			return openErr
		}
		written, copyErr := io.Copy(file, io.LimitReader(tarReader, header.Size+1))
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil || written != header.Size {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
	}
	if _, err = os.Lstat(destination); err == nil {
		return errors.New("CONTINUATION_RESTORE_CONFLICT")
	}
	return os.Rename(staging, destination)
}

// ExtractOverlay safely merges a canonical archive into an already restored
// ResultTree checkout. Overlay files are intentionally outside the trusted
// Git tree; a path collision is therefore an identity conflict, never an
// overwrite. The full archive is validated and staged before any destination
// path is touched.
func ExtractOverlay(ctx context.Context, archive io.Reader, destination, expectedPrefix string, maxBytes int64) error {
	parent := filepath.Dir(destination)
	stagingRoot, err := os.MkdirTemp(parent, ".seven-overlay-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagingRoot)
	staging := filepath.Join(stagingRoot, "content")
	if err = Extract(ctx, archive, staging, expectedPrefix, maxBytes); err != nil {
		return err
	}
	type move struct{ source, target string }
	moves := make([]move, 0, 64)
	err = filepath.WalkDir(staging, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(staging, path)
		if relErr != nil || relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return errors.New("CONTINUATION_ARCHIVE_INVALID")
		}
		target := filepath.Join(destination, relative)
		if _, statErr := os.Lstat(target); statErr == nil {
			return errors.New("CONTINUATION_RESTORE_CONFLICT")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		moves = append(moves, move{source: path, target: target})
		return nil
	})
	if err != nil {
		return err
	}
	for _, item := range moves {
		if err = os.MkdirAll(filepath.Dir(item.target), 0o700); err != nil {
			return err
		}
		if err = os.Rename(item.source, item.target); err != nil {
			return err
		}
	}
	return nil
}
