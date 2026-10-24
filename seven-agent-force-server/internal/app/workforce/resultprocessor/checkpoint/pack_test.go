package checkpoint

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFreezeBindsExactIdentityAndExcludesMaterialization(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(filepath.Join(state, "codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(state, "runtime", "evaluations"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(repo, "README.md"):                                         "result",
		filepath.Join(state, "codex", "session.jsonl"):                           "session",
		filepath.Join(state, "runtime", "evaluations", "x.materialization.json"): "secret-value",
		filepath.Join(state, "runtime-journal.jsonl"):                            "old-owner-lease",
		filepath.Join(state, "hook-events.jsonl"):                                "old-hook-event",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	frozen, err := Freeze(context.Background(), Input{TaskID: 2610240832, TaskTurnID: 637649102, AttemptID: 3107414342, Generation: 1, EvaluationCycleID: 1002, Decision: "WAIT_USER", SessionDigest: "session-digest", RepoRoot: repo, StateRoot: state})
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Identity == "" || frozen.ManifestSHA256 == "" || frozen.WorkspaceSHA256 == "" || frozen.SessionSHA256 == "" {
		t.Fatalf("incomplete frozen checkpoint: %+v", frozen)
	}
	if got := archiveContents(t, frozen.Session); bytes.Contains(got, []byte("secret-value")) || bytes.Contains(got, []byte("materialization")) || bytes.Contains(got, []byte("old-owner-lease")) || bytes.Contains(got, []byte("old-hook-event")) {
		t.Fatalf("secret materialization escaped into Session archive: %q", got)
	}
	if got := archiveContents(t, frozen.Workspace); !bytes.Contains(got, []byte("result")) {
		t.Fatalf("workspace result missing: %q", got)
	}
}

func TestFreezeExcludesCodexProcessTemporarySymlinksButKeepsSession(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo, state := filepath.Join(root, "repo"), filepath.Join(root, "state")
	if err := os.MkdirAll(filepath.Join(state, "codex", "sessions", "2026"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(state, "codex", "tmp", "arg0"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("result"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "codex", "sessions", "2026", "rollout.jsonl"), []byte("durable-session"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr/local/bin/codex", filepath.Join(state, "codex", "tmp", "arg0", "apply_patch")); err != nil {
		t.Fatal(err)
	}
	frozen, err := Freeze(context.Background(), Input{TaskID: 2610240832, TaskTurnID: 637649102, AttemptID: 3107414342, Generation: 1, EvaluationCycleID: 1002, Decision: "PASS", SessionDigest: "session-digest", RepoRoot: repo, StateRoot: state})
	if err != nil {
		t.Fatal(err)
	}
	contents := archiveContents(t, frozen.Session)
	if !bytes.Contains(contents, []byte("durable-session")) || bytes.Contains(contents, []byte("apply_patch")) || bytes.Contains(contents, []byte("usr/local/bin/codex")) {
		t.Fatalf("unexpected App Server session archive: %q", contents)
	}
}

func TestFreezeToSpoolAndSafeRestore(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo, state := filepath.Join(root, "repo"), filepath.Join(root, "state")
	if err := os.MkdirAll(filepath.Join(state, "codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "codex", "session.jsonl"), []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitSelection(t, repo, "init", "-q")
	gitSelection(t, repo, "config", "user.name", "Checkpoint Test")
	gitSelection(t, repo, "config", "user.email", "checkpoint@example.invalid")
	gitSelection(t, repo, "add", ".")
	gitSelection(t, repo, "commit", "-qm", "result")
	commit := gitSelectionOutput(t, repo, "rev-parse", "HEAD")
	tree := gitSelectionOutput(t, repo, "rev-parse", "HEAD^{tree}")
	frozen, err := FreezeToSpool(context.Background(), Input{TaskID: 2610240832, TaskTurnID: 637649102, AttemptID: 3107414342, Generation: 1, EvaluationCycleID: 687498126, Decision: "PASS", RepoRoot: repo, StateRoot: state, BaseCommit: commit, ResultCommit: commit, ResultTree: tree}, filepath.Join(root, "spool"))
	if err != nil {
		t.Fatal(err)
	}
	if frozen.FormatVersion != 2 || filepath.Base(frozen.RepositoryBundle.Path) != "repository.bundle" || filepath.Base(frozen.WorkspaceOverlay.Path) != "workspace-overlay.tar.gz" {
		t.Fatalf("unexpected checkpoint format: %+v", frozen)
	}
	restored := filepath.Join(root, "restored")
	command := exec.Command("git", "init", "-q", restored)
	if output, initErr := command.CombinedOutput(); initErr != nil {
		t.Fatalf("init restore: %v: %s", initErr, output)
	}
	command = exec.Command("git", "-C", restored, "fetch", "-q", frozen.RepositoryBundle.Path, "refs/seven/checkpoints/*:refs/seven/checkpoints/*")
	if output, fetchErr := command.CombinedOutput(); fetchErr != nil {
		t.Fatalf("fetch bundle: %v: %s", fetchErr, output)
	}
	gitSelection(t, restored, "checkout", "--detach", "-q", commit)
	body, err := os.ReadFile(filepath.Join(restored, "src", "main.go"))
	if err != nil || string(body) != "package main\n" {
		t.Fatalf("restored body=%q err=%v", body, err)
	}
}

func TestCheckpointStreamingEncryptionRejectsTamper(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{7}, 32)
	plain := bytes.Repeat([]byte("continuation"), 200000)
	var ciphertext bytes.Buffer
	if err := EncryptStream(bytes.NewReader(plain), &ciphertext, key, []byte("checkpoint:workspace")); err != nil {
		t.Fatal(err)
	}
	var restored bytes.Buffer
	if err := DecryptStream(bytes.NewReader(ciphertext.Bytes()), &restored, key, []byte("checkpoint:workspace")); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.Bytes(), plain) {
		t.Fatal("decrypted checkpoint differs")
	}
	tampered := append([]byte(nil), ciphertext.Bytes()...)
	tampered[len(tampered)-1] ^= 1
	if err := DecryptStream(bytes.NewReader(tampered), io.Discard, key, []byte("checkpoint:workspace")); err == nil || err.Error() != "CONTINUATION_CHECKPOINT_TAMPERED" {
		t.Fatalf("tamper error=%v", err)
	}
}

func TestFreezeRejectsWorkspaceSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(repo, "escape")); err != nil {
		t.Fatal(err)
	}
	_, err := Freeze(context.Background(), Input{TaskID: 2610240832, TaskTurnID: 637649102, AttemptID: 3107414342, Generation: 1, EvaluationCycleID: 1002, Decision: "PASS", RepoRoot: repo, StateRoot: state})
	if err == nil || err.Error() != "pack checkpoint workspace: CHECKPOINT_SYMLINK_FORBIDDEN" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPackDeclaredOutputRestoresOnlyDeclaredPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "reports"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "reports", "summary.md"), []byte("safe result"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "private.txt"), []byte("not declared"), 0o600); err != nil {
		t.Fatal(err)
	}
	blob, err := PackDeclaredOutput(context.Background(), repo, filepath.Join(repo, "reports"), filepath.Join(root, "output.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.Open(blob.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	restored := filepath.Join(root, "restored")
	if err = Extract(context.Background(), archive, restored, "output", MaxArchiveBytes); err != nil {
		t.Fatal(err)
	}
	if body, readErr := os.ReadFile(filepath.Join(restored, "reports", "summary.md")); readErr != nil || string(body) != "safe result" {
		t.Fatalf("declared output body=%q err=%v", body, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(restored, "private.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("undeclared file escaped into output: %v", statErr)
	}
}

func TestPackDeclaredOutputRejectsPathOutsideWorkspace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PackDeclaredOutput(context.Background(), repo, outside, filepath.Join(root, "output.tar.gz")); err == nil || err.Error() != "RESULT_OUTPUT_SOURCE_OUTSIDE_WORKSPACE" {
		t.Fatalf("outside path error=%v", err)
	}
}

func TestExtractOverlayMergesOnlyNonTreeFilesAndRejectsCollision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "repository")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "generated.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "README.md"), []byte("trusted tree"), 0o600); err != nil {
		t.Fatal(err)
	}
	blob, err := packArchiveFile(context.Background(), source, []string{filepath.Join(source, "generated.json")}, "overlay", nil, filepath.Join(root, "overlay.tar.gz"), MaxArchiveBytes)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.Open(blob.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err = ExtractOverlay(context.Background(), archive, destination, "overlay", MaxArchiveBytes); err != nil {
		archive.Close()
		t.Fatal(err)
	}
	archive.Close()
	if body, readErr := os.ReadFile(filepath.Join(destination, "generated.json")); readErr != nil || string(body) != `{"ok":true}` {
		t.Fatalf("overlay body=%q err=%v", body, readErr)
	}

	archive, err = os.Open(blob.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if err = ExtractOverlay(context.Background(), archive, destination, "overlay", MaxArchiveBytes); err == nil || err.Error() != "CONTINUATION_RESTORE_CONFLICT" {
		t.Fatalf("second overlay extraction error=%v", err)
	}
}

func archiveContents(t *testing.T, body []byte) []byte {
	t.Helper()
	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var output bytes.Buffer
	for {
		_, err := tarReader.Next()
		if err == io.EOF {
			return output.Bytes()
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(&output, tarReader); err != nil {
			t.Fatal(err)
		}
	}
}
