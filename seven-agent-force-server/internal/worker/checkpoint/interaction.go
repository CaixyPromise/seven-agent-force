// Package checkpoint owns purpose-aware private Worker checkpoint formats.
package checkpoint

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
)

type Input struct {
	Checkpoint               base.Input
	InteractionID            int64
	OperationID              int64
	AdapterSessionGeneration int64
	AutomaticRevisionCount   int
	ExecutionBudget          ExecutionBudget
	CurrentRound             int64
	SuspendControlKeySHA256  string
}

type Blob struct {
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}
type Manifest struct {
	SchemaVersion            int             `json:"schemaVersion"`
	Format                   string          `json:"format"`
	Purpose                  string          `json:"purpose"`
	Decision                 string          `json:"decision"`
	CheckpointIdentity       string          `json:"checkpointIdentity"`
	TaskID                   int64           `json:"taskId,string"`
	TaskTurnID               int64           `json:"taskTurnId,string"`
	AttemptID                int64           `json:"attemptId,string"`
	Generation               int64           `json:"generation,string"`
	InteractionID            int64           `json:"interactionId,string"`
	OperationID              int64           `json:"operationId,string"`
	AdapterSessionGeneration int64           `json:"adapterSessionGeneration,string"`
	AutomaticRevisionCount   int             `json:"automaticRevisionCount"`
	SessionDigest            string          `json:"sessionDigest"`
	SnapshotDigest           string          `json:"taskExecutionSnapshotDigest"`
	BaseCommit               string          `json:"baseCommit"`
	ResultCommit             string          `json:"resultCommit"`
	ResultTree               string          `json:"resultTree"`
	SelectionDigest          string          `json:"selectionDigest"`
	RepositoryBundle         Blob            `json:"repositoryBundle"`
	WorkspaceOverlay         Blob            `json:"workspaceOverlay"`
	Session                  Blob            `json:"session"`
	ExecutionBudget          ExecutionBudget `json:"executionBudget"`
	CurrentRound             int64           `json:"currentRound,string"`
	SuspendControlKeySHA256  string          `json:"suspendControlKeySha256"`
}

func (m Manifest) Validate() error {
	controlHash, e := hex.DecodeString(m.SuspendControlKeySHA256)
	if e != nil || len(controlHash) != 32 || m.CurrentRound <= 0 {
		return domain.ErrInteractionSuspend
	}
	if err := m.ExecutionBudget.Validate(); err != nil {
		return err
	}
	if m.ExecutionBudget.TaskID != m.TaskID || m.ExecutionBudget.AttemptID != m.AttemptID || m.ExecutionBudget.Generation != m.Generation {
		return domain.ErrInteractionSuspend
	}
	if m.SchemaVersion != 2 || m.Format != "REPOSITORY_BUNDLE_OVERLAY" || m.Purpose != domain.PurposeInteractionSuspend || m.Decision != "" || m.TaskID <= 0 || m.TaskTurnID <= 0 || m.AttemptID <= 0 || m.Generation <= 0 || (m.InteractionID > 0) == (m.OperationID > 0) || m.AdapterSessionGeneration <= 0 || m.AutomaticRevisionCount < 0 || m.SessionDigest == "" || m.SnapshotDigest == "" || m.BaseCommit == "" || m.ResultCommit == "" || m.ResultTree == "" || m.SelectionDigest == "" {
		return domain.ErrInteractionSuspend
	}
	expected := domain.InteractionCheckpointIdentity(m.TaskID, m.TaskTurnID, m.AttemptID, m.Generation, m.InteractionID, m.AdapterSessionGeneration, m.CurrentRound, m.SnapshotDigest, m.SuspendControlKeySHA256)
	if m.OperationID > 0 {
		expected = domain.OperationCheckpointIdentity(m.TaskID, m.TaskTurnID, m.AttemptID, m.Generation, m.OperationID, m.AdapterSessionGeneration, m.CurrentRound, m.SnapshotDigest, m.SuspendControlKeySHA256)
	}
	if m.CheckpointIdentity != expected {
		return domain.ErrInteractionSuspend
	}
	for _, b := range []Blob{m.RepositoryBundle, m.WorkspaceOverlay, m.Session} {
		decoded, e := hex.DecodeString(b.SHA256)
		if e != nil || len(decoded) != 32 || b.SizeBytes <= 0 || b.SizeBytes > base.MaxArchiveBytes {
			return domain.ErrInteractionSuspend
		}
	}
	if m.Session.SizeBytes > base.MaxSessionArchiveBytes {
		return domain.ErrInteractionSuspend
	}
	return nil
}

func DecodeManifest(body []byte) (Manifest, error) {
	var m Manifest
	if len(body) > 1<<20 {
		return m, domain.ErrInteractionSuspend
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return m, domain.ErrInteractionSuspend
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return m, domain.ErrInteractionSuspend
	}
	return m, m.Validate()
}

// FreezeToSpool must be called inside the trusted stopped/quiescent guard.
// Result identity is recomputed solely for a resumable workspace, not success.
func FreezeToSpool(ctx context.Context, in Input, spool string) (base.SpoolFrozen, error) {
	p := in.Checkpoint
	if err := in.ExecutionBudget.Validate(); err != nil {
		return base.SpoolFrozen{}, err
	}
	if strings.TrimSpace(p.RepoRoot) == "" || strings.TrimSpace(p.StateRoot) == "" || strings.TrimSpace(spool) == "" {
		return base.SpoolFrozen{}, domain.ErrInteractionSuspend
	}
	if p.Decision != domain.PurposeInteractionSuspend || p.EvaluationCycleID != 0 || (in.InteractionID > 0) == (in.OperationID > 0) || in.AdapterSessionGeneration <= 0 || in.AutomaticRevisionCount < 0 || p.TaskID <= 0 || p.TaskTurnID <= 0 || p.AttemptID <= 0 || p.Generation <= 0 || p.SessionDigest == "" || p.ExecutionSnapshotDigest == "" || len(p.Outputs) != 0 {
		return base.SpoolFrozen{}, domain.ErrInteractionSuspend
	}
	var pathErr error
	p.RepoRoot, pathErr = filepath.Abs(p.RepoRoot)
	if pathErr != nil {
		return base.SpoolFrozen{}, pathErr
	}
	p.StateRoot, pathErr = filepath.Abs(p.StateRoot)
	if pathErr != nil {
		return base.SpoolFrozen{}, pathErr
	}
	spool, pathErr = filepath.Abs(spool)
	if pathErr != nil {
		return base.SpoolFrozen{}, pathErr
	}
	for _, root := range []string{p.RepoRoot, p.StateRoot} {
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || spool == root || strings.HasPrefix(spool, root+string(os.PathSeparator)) {
			return base.SpoolFrozen{}, domain.ErrInteractionSuspend
		}
	}
	if err := os.MkdirAll(spool, 0700); err != nil {
		return base.SpoolFrozen{}, err
	}
	info, err := os.Lstat(spool)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return base.SpoolFrozen{}, domain.ErrInteractionSuspend
	}
	if err = os.Chmod(spool, 0700); err != nil {
		return base.SpoolFrozen{}, err
	}
	identity := domain.InteractionCheckpointIdentity(p.TaskID, p.TaskTurnID, p.AttemptID, p.Generation, in.InteractionID, in.AdapterSessionGeneration, in.CurrentRound, p.ExecutionSnapshotDigest, in.SuspendControlKeySHA256)
	if in.OperationID > 0 {
		identity = domain.OperationCheckpointIdentity(p.TaskID, p.TaskTurnID, p.AttemptID, p.Generation, in.OperationID, in.AdapterSessionGeneration, in.CurrentRound, p.ExecutionSnapshotDigest, in.SuspendControlKeySHA256)
	}
	selection, err := base.SelectResult(ctx, base.SelectionInput{RepoRoot: p.RepoRoot, BaseCommit: p.BaseCommit, ContinuationIncludes: p.ContinuationIncludes})
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	result, err := suspendSnapshotIdentity(ctx, p, selection)
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	ref := "refs/seven/checkpoints/" + identity
	if _, err = Git(ctx, p.RepoRoot, "update-ref", ref, result.Commit); err != nil {
		return base.SpoolFrozen{}, err
	}
	defer Git(context.Background(), p.RepoRoot, "update-ref", "-d", ref)
	bundlePath := filepath.Join(spool, "repository.bundle")
	if _, err = Git(ctx, p.RepoRoot, "bundle", "create", bundlePath, ref); err != nil {
		return base.SpoolFrozen{}, err
	}
	bundle, err := FileBlob(bundlePath, base.MaxArchiveBytes)
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	var overlayPaths []string
	for _, entry := range selection.OverlayIndex {
		if entry.Type != "DELETED" {
			overlayPaths = append(overlayPaths, entry.Path)
		}
	}
	overlay, err := pack(ctx, p.RepoRoot, overlayPaths, "overlay", filepath.Join(spool, "workspace-overlay.tar.gz"), base.MaxArchiveBytes)
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	sessionPaths, err := sessionFiles(p.StateRoot)
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	session, err := pack(ctx, p.StateRoot, sessionPaths, "session", filepath.Join(spool, "session.tar.gz"), base.MaxSessionArchiveBytes)
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	m := Manifest{SchemaVersion: 2, Format: "REPOSITORY_BUNDLE_OVERLAY", Purpose: domain.PurposeInteractionSuspend, CheckpointIdentity: identity, TaskID: p.TaskID, TaskTurnID: p.TaskTurnID, AttemptID: p.AttemptID, Generation: p.Generation, InteractionID: in.InteractionID, OperationID: in.OperationID, AdapterSessionGeneration: in.AdapterSessionGeneration, AutomaticRevisionCount: in.AutomaticRevisionCount, SessionDigest: p.SessionDigest, SnapshotDigest: p.ExecutionSnapshotDigest, BaseCommit: p.BaseCommit, ResultCommit: result.Commit, ResultTree: result.Tree, SelectionDigest: selection.SelectionDigest, RepositoryBundle: blobMeta(bundle), WorkspaceOverlay: blobMeta(overlay), Session: blobMeta(session), ExecutionBudget: in.ExecutionBudget}
	m.CurrentRound = in.CurrentRound
	m.SuspendControlKeySHA256 = in.SuspendControlKeySHA256
	if err = m.Validate(); err != nil {
		return base.SpoolFrozen{}, err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	manifestPath := filepath.Join(spool, "identity.json")
	if err = writeFile(manifestPath, func(w io.Writer) error { _, e := w.Write(body); return e }); err != nil {
		return base.SpoolFrozen{}, err
	}
	manifest, err := FileBlob(manifestPath, 1<<20)
	if err != nil {
		return base.SpoolFrozen{}, err
	}
	return base.SpoolFrozen{FormatVersion: 2, Identity: identity, RepositoryBundle: bundle, WorkspaceOverlay: overlay, Session: session, Manifest: manifest, Selection: selection}, nil
}

func blobMeta(b base.SpoolBlob) Blob { return Blob{SHA256: b.SHA256, SizeBytes: b.SizeBytes} }

func FileBlob(path string, limit int64) (base.SpoolBlob, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return base.SpoolBlob{}, domain.ErrInteractionSuspend
	}
	f, e := os.Open(path)
	if e != nil {
		return base.SpoolBlob{}, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, limit+1))
	if e != nil || n != info.Size() {
		return base.SpoolBlob{}, domain.ErrInteractionSuspend
	}
	if e = os.Chmod(path, 0600); e != nil {
		return base.SpoolBlob{}, e
	}
	return base.SpoolBlob{Path: path, SHA256: hex.EncodeToString(h.Sum(nil)), SizeBytes: n}, nil
}

func sessionFiles(root string) ([]string, error) {
	var paths []string
	for _, name := range []string{"codex", "session.json"} {
		err := filepath.WalkDir(filepath.Join(root, name), func(path string, d fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			rel = filepath.ToSlash(rel)
			if d.Type()&os.ModeSymlink != 0 {
				return domain.ErrInteractionSuspend
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(d.Name(), ".materialization.json") || (strings.Contains(rel, "/evaluations/") && strings.HasSuffix(d.Name(), ".context.json")) {
				return nil
			}
			info, e := d.Info()
			if e != nil || !info.Mode().IsRegular() {
				return domain.ErrInteractionSuspend
			}
			paths = append(paths, rel)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 {
		return nil, domain.ErrInteractionSuspend
	}
	return paths, nil
}

func pack(ctx context.Context, root string, paths []string, prefix, destination string, limit int64) (base.SpoolBlob, error) {
	sort.Strings(paths)
	if len(paths) > base.MaxArchiveFiles {
		return base.SpoolBlob{}, domain.ErrInteractionSuspend
	}
	err := writeFile(destination, func(w io.Writer) error {
		gz := gzip.NewWriter(w)
		gz.ModTime = time.Unix(0, 0)
		tw := tar.NewWriter(gz)
		var total int64
		for _, rel := range paths {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !fs.ValidPath(rel) || strings.Contains(rel, "\\") {
				return domain.ErrInteractionSuspend
			}
			path := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > base.MaxFileBytes {
				return domain.ErrInteractionSuspend
			}
			// Ancestors must also be ordinary directories; a symlink ancestor is
			// not legitimized by a regular final path component.
			for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
				pi, e := os.Lstat(parent)
				if e != nil || !pi.IsDir() || pi.Mode()&os.ModeSymlink != 0 {
					return domain.ErrInteractionSuspend
				}
				if parent == root {
					break
				}
				if parent == filepath.Dir(parent) {
					return domain.ErrInteractionSuspend
				}
			}
			total += info.Size()
			if total > limit {
				return domain.ErrInteractionSuspend
			}
			if err = tw.WriteHeader(&tar.Header{Name: prefix + "/" + rel, Mode: 0600, Size: info.Size(), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}); err != nil {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			n, e := io.CopyN(tw, f, info.Size())
			closeErr := f.Close()
			if e != nil || closeErr != nil || n != info.Size() {
				return domain.ErrInteractionSuspend
			}
		}
		if err := tw.Close(); err != nil {
			return err
		}
		return gz.Close()
	})
	if err != nil {
		return base.SpoolBlob{}, err
	}
	return FileBlob(destination, limit)
}

func writeFile(path string, write func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".suspend-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = write(f)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// Git only runs local plumbing with repository hooks, external transport,
// replacement objects and host Git configuration disabled.
func Git(ctx context.Context, repo string, args ...string) (string, error) {
	return gitWithEnvironment(ctx, repo, nil, args...)
}

func gitWithEnvironment(ctx context.Context, repo string, environment []string, args ...string) (string, error) {
	argv := append([]string{"-c", "core.hooksPath=/dev/null", "-c", "protocol.allow=never", "-c", "protocol.file.allow=always", "-c", "core.fsmonitor=false", "-C", repo}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_ASKPASS=/bin/false", "LC_ALL=C"}
	cmd.Env = append(cmd.Env, environment...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("INTERACTION_CHECKPOINT_GIT_FAILED: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
