package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	processorcheckpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
)

type bundleOverlayRestore struct {
	CheckpointIdentity   string
	BundleObjectKey      string
	BundleObjectSHA256   string
	BundleContentSHA256  string
	BundleSizeBytes      int64
	OverlayObjectKey     string
	OverlayObjectSHA256  string
	OverlayContentSHA256 string
	OverlaySizeBytes     int64
	ExpectedCommit       string
	ExpectedTree         string
}

// restoreBundleOverlay reconstructs a repository solely from the frozen full
// Git bundle, then applies the narrowly selected non-ResultTree overlay. It
// never consults a mutable remote, user Git config, hooks, or credentials.
func (w *worker) restoreBundleOverlay(ctx context.Context, spec bundleOverlayRestore, key []byte, spoolRoot, repoRoot string) error {
	if len(spec.CheckpointIdentity) != 64 || len(spec.ExpectedCommit) != 40 || len(spec.ExpectedTree) != 40 {
		return errors.New("CONTINUATION_IDENTITY_INCOMPATIBLE")
	}
	bundle, err := w.downloadCheckpointPlain(ctx, spec.BundleObjectKey, spec.BundleObjectSHA256, spec.BundleContentSHA256, spec.BundleSizeBytes, key, spec.CheckpointIdentity, "repository-bundle", spoolRoot, processorcheckpoint.MaxArchiveBytes)
	if err != nil {
		return err
	}
	defer removeClosed(bundle)
	overlay, err := w.downloadCheckpointPlain(ctx, spec.OverlayObjectKey, spec.OverlayObjectSHA256, spec.OverlayContentSHA256, spec.OverlaySizeBytes, key, spec.CheckpointIdentity, "workspace-overlay", spoolRoot, processorcheckpoint.MaxArchiveBytes)
	if err != nil {
		return err
	}
	defer removeClosed(overlay)
	if err = os.RemoveAll(repoRoot); err != nil {
		return err
	}
	if err = os.MkdirAll(repoRoot, 0o700); err != nil {
		return err
	}
	if _, err = controlledWorkerGit(ctx, repoRoot, "init", "-q"); err != nil {
		return fmt.Errorf("CONTINUATION_ARCHIVE_INVALID: %w", err)
	}
	ref := "refs/seven/checkpoints/" + spec.CheckpointIdentity
	if _, err = controlledWorkerGit(ctx, repoRoot, "-c", "protocol.file.allow=always", "fetch", "-q", bundle.Name(), ref+":"+ref); err != nil {
		return fmt.Errorf("CONTINUATION_ARCHIVE_INVALID: %w", err)
	}
	if _, err = controlledWorkerGit(ctx, repoRoot, "checkout", "--detach", "-q", spec.ExpectedCommit); err != nil {
		return fmt.Errorf("CONTINUATION_ARCHIVE_INVALID: %w", err)
	}
	commit, err := controlledWorkerGit(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil || commit != spec.ExpectedCommit {
		return errors.New("CONTINUATION_RESULT_IDENTITY_MISMATCH")
	}
	tree, err := controlledWorkerGit(ctx, repoRoot, "rev-parse", "HEAD^{tree}")
	if err != nil || tree != spec.ExpectedTree {
		return errors.New("CONTINUATION_RESULT_IDENTITY_MISMATCH")
	}
	if _, err = overlay.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err = processorcheckpoint.ExtractOverlay(ctx, overlay, repoRoot, "overlay", processorcheckpoint.MaxArchiveBytes); err != nil {
		return fmt.Errorf("CONTINUATION_ARCHIVE_INVALID: %w", err)
	}
	return nil
}

func restoreSessionArchive(ctx context.Context, archive io.Reader, stateRoot string) error {
	if err := os.RemoveAll(stateRoot); err != nil {
		return err
	}
	return processorcheckpoint.Extract(ctx, archive, stateRoot, "session", processorcheckpoint.MaxSessionArchiveBytes)
}

func (w *worker) downloadCheckpointPlain(ctx context.Context, objectKey, objectSHA, contentSHA string, size int64, key []byte, identity, aadType, spoolRoot string, maxPlain int64) (*os.File, error) {
	if objectKey == "" || len(objectSHA) != 64 || len(contentSHA) != 64 || size < 1 {
		return nil, errors.New("CONTINUATION_CHECKPOINT_MISSING")
	}
	object, err := w.artifacts.Get(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	defer object.Close()
	encrypted, err := os.CreateTemp(spoolRoot, ".checkpoint-encrypted-*")
	if err != nil {
		return nil, err
	}
	_ = encrypted.Chmod(0o600)
	hash := sha256.New()
	read, copyErr := io.Copy(io.MultiWriter(encrypted, hash), io.LimitReader(object, size+1))
	if copyErr != nil || read != size || hex.EncodeToString(hash.Sum(nil)) != objectSHA {
		removeClosed(encrypted)
		return nil, errors.New("CONTINUATION_CHECKPOINT_TAMPERED")
	}
	if _, err = encrypted.Seek(0, io.SeekStart); err != nil {
		removeClosed(encrypted)
		return nil, err
	}
	plain, err := decryptCheckpointObject(encrypted, spoolRoot, key, checkpointAAD(identity, aadType), contentSHA, maxPlain)
	removeClosed(encrypted)
	return plain, err
}

func controlledWorkerGit(ctx context.Context, repo string, args ...string) (string, error) {
	home, err := os.MkdirTemp(filepath.Dir(repo), ".git-home-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(home)
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	command.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+home, "XDG_CONFIG_HOME="+home,
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, "config"), "GIT_TERMINAL_PROMPT=0",
		"GIT_ATTR_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func removeClosed(file *os.File) {
	if file == nil {
		return
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
}
