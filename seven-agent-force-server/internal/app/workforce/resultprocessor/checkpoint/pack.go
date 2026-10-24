// Package checkpoint freezes the local, Attempt-private material required for
// a later continuation.  It is intentionally a Worker-side byte operation:
// the Server records only identities and object keys, while the Worker uploads
// and byte-for-byte verifies the encrypted object-store payloads.
package checkpoint

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

const (
	MaxArchiveBytes        = 512 << 20
	HardMaxArchiveBytes    = 2 << 30
	MaxSessionArchiveBytes = 64 << 20
	MaxFileBytes           = 256 << 20
	MaxArchiveFiles        = 100000
	MaxCompressionRatio    = 100
)

type Input struct {
	TaskID                  int64
	TaskTurnID              int64
	AttemptID               int64
	Generation              int64
	EvaluationCycleID       int64
	Decision                string
	SessionDigest           string
	RepoRoot                string
	StateRoot               string
	BaseCommit              string
	ResultCommit            string
	ResultTree              string
	ExecutionSnapshotDigest string
	HarnessImageDigest      string
	HookConfigurationDigest string
	ResultProcessorDigest   string
	ModelCredentialIdentity string
	IsolationProfile        string
	Outputs                 []processordomain.Output
	ContinuationIncludes    []string
}

type Frozen struct {
	Identity           string
	Workspace          []byte
	WorkspaceSHA256    string
	Session            []byte
	SessionSHA256      string
	IdentityManifest   []byte
	ManifestSHA256     string
	WorkspaceFileCount int
	SessionFileCount   int
}

// Freeze produces three private blobs: a repository checkpoint, a narrowly
// selected Session archive, and an identity manifest binding both byte sets to
// the exact TaskTurn/Attempt/EvaluationCycle.  Runtime materialization files
// are excluded unconditionally because they may contain a short-lived secret.
func Freeze(ctx context.Context, input Input) (Frozen, error) {
	if err := validateInput(input); err != nil {
		return Frozen{}, err
	}
	identity := Identity(input)
	workspace, workspaceCount, err := packTree(ctx, input.RepoRoot, "workspace", nil)
	if err != nil {
		return Frozen{}, fmt.Errorf("pack checkpoint workspace: %w", err)
	}
	allowedSession := []string{
		filepath.Join(input.StateRoot, "codex"),
		filepath.Join(input.StateRoot, "session.json"),
	}
	session, sessionCount, err := packSelected(ctx, input.StateRoot, "session", allowedSession, skipSecretMaterialization)
	if err != nil {
		return Frozen{}, fmt.Errorf("pack checkpoint session: %w", err)
	}
	workspaceDigest, sessionDigest := digest(workspace), digest(session)
	manifest := map[string]any{
		"schemaVersion":               1,
		"checkpointIdentity":          identity,
		"taskId":                      strconv.FormatInt(input.TaskID, 10),
		"taskTurnId":                  strconv.FormatInt(input.TaskTurnID, 10),
		"attemptId":                   strconv.FormatInt(input.AttemptID, 10),
		"generation":                  input.Generation,
		"evaluationCycleId":           strconv.FormatInt(input.EvaluationCycleID, 10),
		"decision":                    input.Decision,
		"sessionDigest":               input.SessionDigest,
		"baseCommit":                  input.BaseCommit,
		"resultCommit":                input.ResultCommit,
		"resultTree":                  input.ResultTree,
		"taskExecutionSnapshotDigest": input.ExecutionSnapshotDigest,
		"harnessImageDigest":          input.HarnessImageDigest,
		"hookConfigurationDigest":     input.HookConfigurationDigest,
		"resultProcessorDigest":       input.ResultProcessorDigest,
		"modelCredentialIdentity":     input.ModelCredentialIdentity,
		"isolationProfile":            input.IsolationProfile,
		"workspace": map[string]any{
			"sha256": workspaceDigest, "sizeBytes": len(workspace), "files": workspaceCount,
		},
		"session": map[string]any{
			"sha256": sessionDigest, "sizeBytes": len(session), "files": sessionCount,
		},
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		return Frozen{}, err
	}
	return Frozen{
		Identity: identity, Workspace: workspace, WorkspaceSHA256: workspaceDigest,
		Session: session, SessionSHA256: sessionDigest, IdentityManifest: manifestBody,
		ManifestSHA256: digest(manifestBody), WorkspaceFileCount: workspaceCount, SessionFileCount: sessionCount,
	}, nil
}

func Identity(input Input) string {
	value := strings.Join([]string{
		"seven.continuation-checkpoint", strconv.FormatInt(input.TaskID, 10), strconv.FormatInt(input.TaskTurnID, 10), strconv.FormatInt(input.AttemptID, 10),
		fmt.Sprint(input.Generation), strconv.FormatInt(input.EvaluationCycleID, 10), input.Decision, input.SessionDigest,
	}, "\x00")
	return digest([]byte(value))
}

// ObjectKeys returns the only three object-store locations a Worker may use
// for this checkpoint identity.  Keeping the namespace deterministic lets the
// Server reject arbitrary object keys while making a response retry harmless.
func ObjectKeys(input Input, identity string) (workspace, session, manifest string) {
	prefix := strings.Join([]string{
		"continuations", keySegment(input.TaskID), keySegment(input.TaskTurnID), keySegment(input.AttemptID), identity,
	}, "/")
	return prefix + "/workspace.tar.gz", prefix + "/session.tar.gz", prefix + "/identity.json"
}

// ObjectKeysV2 returns the immutable Bundle + Overlay checkpoint namespace.
// Legacy ObjectKeys remains available only for historical audit rows.
func ObjectKeysV2(input Input, identity string) (bundle, overlay, session, manifest string) {
	prefix := strings.Join([]string{
		"continuations", keySegment(input.TaskID), keySegment(input.TaskTurnID), keySegment(input.AttemptID), identity,
	}, "/")
	return prefix + "/repository.bundle", prefix + "/workspace-overlay.tar.gz", prefix + "/session.tar.gz", prefix + "/identity.json"
}

func validateInput(input Input) error {
	if input.TaskID <= 0 || input.TaskTurnID <= 0 || input.AttemptID <= 0 ||
		input.Generation < 1 || input.EvaluationCycleID <= 0 ||
		(input.Decision != "PASS" && input.Decision != "WAIT_USER" && input.Decision != "MANUAL_REVIEW") {
		return errors.New("CHECKPOINT_IDENTITY_INVALID")
	}
	if info, err := os.Stat(input.RepoRoot); err != nil || !info.IsDir() {
		return errors.New("CHECKPOINT_WORKSPACE_UNAVAILABLE")
	}
	if info, err := os.Stat(input.StateRoot); err != nil || !info.IsDir() {
		return errors.New("CHECKPOINT_SESSION_UNAVAILABLE")
	}
	return nil
}

type skipFunc func(relative string, entry fs.DirEntry) bool

func packTree(ctx context.Context, root, prefix string, skip skipFunc) ([]byte, int, error) {
	return packSelected(ctx, root, prefix, []string{root}, skip)
}

func packSelected(ctx context.Context, root, prefix string, sources []string, skip skipFunc) ([]byte, int, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]packEntry, 0, 64)
	seen := map[string]struct{}{}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		source = filepath.Clean(source)
		if source != root && !strings.HasPrefix(source, root+string(os.PathSeparator)) {
			return nil, 0, errors.New("CHECKPOINT_SOURCE_OUTSIDE_ROOT")
		}
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, 0, err
		}
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
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
			if !info.Mode().IsRegular() {
				return errors.New("CHECKPOINT_FILE_TYPE_FORBIDDEN")
			}
			if info.Size() > MaxFileBytes {
				return errors.New("CHECKPOINT_FILE_TOO_LARGE")
			}
			archiveName := pathJoin(prefix, relative)
			if _, exists := seen[archiveName]; exists {
				return errors.New("CHECKPOINT_ARCHIVE_PATH_DUPLICATE")
			}
			seen[archiveName] = struct{}{}
			entries = append(entries, packEntry{source: path, name: archiveName, size: info.Size()})
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	gzipWriter.Name = ""
	gzipWriter.Comment = ""
	gzipWriter.ModTime = time.Unix(0, 0).UTC()
	tarWriter := tar.NewWriter(gzipWriter)
	var total int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return nil, 0, err
		}
		total += entry.size
		if total > MaxArchiveBytes {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return nil, 0, errors.New("CHECKPOINT_ARCHIVE_TOO_LARGE")
		}
		header := &tar.Header{Name: entry.name, Mode: 0o600, Size: entry.size, ModTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, 0, err
		}
		file, err := os.Open(entry.source)
		if err != nil {
			return nil, 0, err
		}
		written, copyErr := io.Copy(tarWriter, io.LimitReader(file, entry.size+1))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || written != entry.size {
			return nil, 0, errors.New("CHECKPOINT_SOURCE_READ_FAILED")
		}
	}
	if err := tarWriter.Close(); err != nil {
		return nil, 0, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, 0, err
	}
	if output.Len() > MaxArchiveBytes {
		return nil, 0, errors.New("CHECKPOINT_ARCHIVE_TOO_LARGE")
	}
	return output.Bytes(), len(entries), nil
}

type packEntry struct {
	source string
	name   string
	size   int64
}

func pathJoin(prefix, relative string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Join(prefix, relative)), "/")
}

func skipSecretMaterialization(relative string, entry fs.DirEntry) bool {
	slash := strings.TrimPrefix(filepath.ToSlash(relative), "./")
	// Codex App Server writes executable helper symlinks and coordination locks
	// below these attempt-private cache directories. They are process-temporary,
	// are not part of thread/resume, and must never weaken the archive-wide
	// symlink prohibition. The durable sessions and state remain included.
	for _, root := range []string{"codex/tmp", "codex/.tmp", "codex/thread-writer-locks"} {
		if slash == root || strings.HasPrefix(slash, root+"/") {
			return true
		}
	}
	base := filepath.Base(relative)
	return strings.HasSuffix(base, ".materialization.json") || strings.Contains(filepath.ToSlash(relative), "/evaluations/") && strings.HasSuffix(base, ".context.json")
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func keySegment(value int64) string {
	if value <= 0 {
		return "unknown"
	}
	return strconv.FormatInt(value, 10)
}
