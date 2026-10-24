package application

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	processorcheckpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

func TestLoadFrozenResultReplaysExactBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	payload := []byte("immutable patch")
	artifact := FrozenArtifact{Type: "PATCH", Name: "changes.patch", MediaType: "text/x-diff", Path: digest(payload) + "-changes.patch", SHA256: digest(payload), SizeBytes: int64(len(payload))}
	if err := os.WriteFile(filepath.Join(root, artifact.Path), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 1, "finalizationRunId": "1", "taskId": "2", "taskTurnId": "4", "attemptId": "3", "resultCommit": strings.Repeat("1", 40), "resultTree": strings.Repeat("2", 40), "selectionDigest": strings.Repeat("a", 64), "artifacts": []FrozenArtifact{artifact}})
	if err := os.WriteFile(filepath.Join(root, "result-manifest.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	ready, _ := json.Marshal(map[string]any{"schemaVersion": 1, "manifest": "result-manifest.json", "manifestDigest": digest(manifest)})
	if err := os.WriteFile(filepath.Join(root, "READY"), ready, 0o600); err != nil {
		t.Fatal(err)
	}
	result, ok, err := loadFrozenResult(root, 1, 2, 4, 3)
	if err != nil || !ok {
		t.Fatalf("loadFrozenResult() = (%v,%v), want replay", ok, err)
	}
	if result.SpoolIdentity != digest(manifest) || result.Commit != strings.Repeat("1", 40) || len(result.Artifacts) != 1 {
		t.Fatalf("unexpected replay result: %+v", result)
	}
	if _, ok, err = loadFrozenResult(root, 1, 2, 5, 3); err == nil || ok {
		t.Fatalf("loadFrozenResult() accepted a mismatched TaskTurn: ok=%v err=%v", ok, err)
	}
}

func TestDeclaredOutputCandidatesDefersArchiveOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	candidates, err := declaredOutputCandidates(root, []processordomain.Output{{
		Name: "release", Kind: "ARCHIVE", ArchiveFormat: "ZIP", Storage: "IMMEDIATE",
	}})
	if err != nil {
		t.Fatalf("ARCHIVE declaration must be handled by MaterializationRun: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("ARCHIVE declaration produced eager candidates: %#v", candidates)
	}
}

func TestValidateResultSelectionAcceptsRegularFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "result.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateResultSelection(root, processorcheckpoint.SelectionIndex{{Path: "result.txt", Type: "FILE", GitMode: "100644", SizeBytes: 2, SHA256: digest([]byte("ok"))}}); err != nil {
		t.Fatalf("validateResultSelection() error = %v", err)
	}
}

func TestValidateResultSelectionRejectsSecretFilename(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.production"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := validateResultSelection(root, processorcheckpoint.SelectionIndex{{Path: ".env.production", Type: "FILE", GitMode: "100644", SizeBytes: 6, SHA256: digest([]byte("secret"))}})
	if err == nil || !strings.Contains(err.Error(), "denied secret filename") {
		t.Fatalf("validateResultSelection() error = %v, want secret filename rejection", err)
	}
}

func TestValidateResultSelectionRejectsSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	err := validateResultSelection(root, processorcheckpoint.SelectionIndex{{Path: "escape", Type: "FILE", GitMode: "100644"}})
	if err == nil || !strings.Contains(err.Error(), "forbidden file type") {
		t.Fatalf("validateResultSelection() error = %v, want symlink rejection", err)
	}
}

func TestComputeResultIdentityForPlanExcludesRegenerableDependencySymlinks(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "test")
	runTestGit(t, root, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "result.txt"), []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", "result.txt")
	runTestGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "result.txt"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	packageDir := filepath.Join(root, "node_modules", ".pnpm", "pkg@1.0.0", "node_modules", "pkg")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".pnpm", "pkg@1.0.0", "node_modules", "pkg"), filepath.Join(root, "node_modules", "pkg")); err != nil {
		t.Fatal(err)
	}
	result, err := ComputeResultIdentityForPlan(context.Background(), root, base, nil, nil)
	if err != nil {
		t.Fatalf("ComputeResultIdentityForPlan() error = %v", err)
	}
	evidence, err := reviewEvidenceIndex(context.Background(), root, result.Commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range evidence {
		if strings.HasPrefix(item.Path, "node_modules/") {
			t.Fatalf("regenerable dependency entered trusted result: %+v", item)
		}
	}
}

func TestComputeCheckpointIdentityForPlanPreservesUnchangedWaitingRepository(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	runTestGit(t, repo, "init", "--initial-branch=main")
	runTestGit(t, repo, "config", "user.name", "test")
	runTestGit(t, repo, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, repo, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runTestGit(t, repo, "rev-parse", "HEAD^{tree}"))
	for _, decision := range []string{"WAIT_USER", "MANUAL_REVIEW", "PASS", "UNKNOWN"} {
		t.Run(decision, func(t *testing.T) {
			identity, err := ComputeCheckpointIdentityForPlan(ctx, repo, base, decision, nil, nil)
			if decision == "PASS" || decision == "UNKNOWN" {
				if err == nil {
					t.Fatal("unchanged or invalid decision accepted as a successful result")
				}
				return
			}
			if err != nil || identity.Commit != base || identity.Tree != tree || identity.SelectionDigest == "" {
				t.Fatalf("waiting identity = %+v, err = %v", identity, err)
			}
			frozen, err := processorcheckpoint.FreezeToSpool(ctx, processorcheckpoint.Input{
				TaskID: 1, TaskTurnID: 2, AttemptID: 3, Generation: 1, EvaluationCycleID: 4,
				Decision: decision, RepoRoot: repo, StateRoot: t.TempDir(),
				BaseCommit: base, ResultCommit: identity.Commit, ResultTree: identity.Tree,
			}, t.TempDir())
			if err != nil {
				t.Fatalf("pack unchanged waiting checkpoint: %v", err)
			}
			if frozen.RepositoryBundle.SizeBytes == 0 || len(frozen.Selection.ResultTreeIndex) != 1 || !frozen.Selection.ResultTreeIndex.Contains("README.md") {
				t.Fatal("waiting checkpoint must retain the base without inventing changes")
			}
			restored := filepath.Join(t.TempDir(), "repo")
			runTestGit(t, repo, "init", "-q", restored)
			runTestGit(t, restored, "fetch", "-q", frozen.RepositoryBundle.Path, "refs/seven/checkpoints/*:refs/seven/checkpoints/*")
			if got := strings.TrimSpace(runTestGit(t, restored, "rev-parse", base+"^{tree}")); got != tree {
				t.Fatalf("restored waiting tree = %s, want %s", got, tree)
			}
		})
	}
	if _, err := ComputeResultIdentityForPlan(ctx, repo, base, nil, nil); err == nil {
		t.Fatal("Finalization accepted an unchanged repository")
	}
	if got := strings.TrimSpace(runTestGit(t, repo, "rev-parse", "HEAD")); got != base {
		t.Fatalf("waiting checkpoint moved Agent HEAD: %s", got)
	}
	if status := strings.TrimSpace(runTestGit(t, repo, "status", "--porcelain")); status != "" {
		t.Fatalf("waiting checkpoint changed Agent index/worktree: %q", status)
	}
}

func TestComputeCheckpointIdentityForPlanPreservesChangedResult(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "--initial-branch=main")
	runTestGit(t, repo, "config", "user.name", "test")
	runTestGit(t, repo, "config", "user.email", "test@example.invalid")
	path := filepath.Join(repo, "README.md")
	if err := os.WriteFile(path, []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := ComputeResultIdentityForPlan(context.Background(), repo, base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, decision := range []string{"WAIT_USER", "MANUAL_REVIEW", "PASS"} {
		got, err := ComputeCheckpointIdentityForPlan(context.Background(), repo, base, decision, nil, nil)
		if err != nil || got.Commit != want.Commit || got.Tree != want.Tree || got.SelectionDigest != want.SelectionDigest {
			t.Fatalf("%s changed result = %+v, want %+v, err = %v", decision, got, want, err)
		}
	}
}

func TestDeclaredOutputCandidatesDefersOnDemandOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	candidates, err := declaredOutputCandidates(root, []processordomain.Output{{
		Name: "later.md", Kind: "PATH", Path: "not-materialized-yet.md", Storage: "ON_DEMAND",
	}})
	if err != nil {
		t.Fatalf("ON_DEMAND declaration must not be materialized by FinalizationRun: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("ON_DEMAND declaration produced eager candidates: %#v", candidates)
	}
}

func TestComputeResultIdentityForPlanUsesPrivateIndexForDeclaredOutput(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "test")
	runTestGit(t, root, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "base.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", "base.txt")
	runTestGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))

	if err := os.WriteFile(filepath.Join(root, "SEVEN_WORKER_RESULT.md"), []byte("reviewable result\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ComputeResultIdentityForPlan(context.Background(), root, base, []processordomain.Output{{
		Name: "result", Kind: "PATH", Path: "SEVEN_WORKER_RESULT.md", Storage: "IMMEDIATE",
	}}, nil)
	if err != nil {
		t.Fatalf("ComputeResultIdentityForPlan() error = %v", err)
	}
	if result.Commit == base || result.Tree == "" || result.SelectionDigest == "" {
		t.Fatalf("unexpected trusted result identity: %+v", result)
	}
	evidence, err := reviewEvidenceIndex(context.Background(), root, result.Commit)
	if err != nil {
		t.Fatalf("reviewEvidenceIndex() error = %v", err)
	}
	if len(evidence) != 2 || evidence[0].Path != "SEVEN_WORKER_RESULT.md" || evidence[0].LineCount != 1 {
		t.Fatalf("reviewEvidenceIndex() = %+v, want the result-commit files and line counts", evidence)
	}
	if got := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD")); got != base {
		t.Fatalf("ComputeResultIdentityForPlan() changed Agent HEAD = %s, want %s", got, base)
	}
	if status := strings.TrimSpace(runTestGit(t, root, "status", "--porcelain")); status != "?? SEVEN_WORKER_RESULT.md" {
		t.Fatalf("ComputeResultIdentityForPlan() changed Agent index = %q", status)
	}
}

func TestMaterializeReviewRepositoryFreezesOnlyTrustedCommit(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, source, "init", "--initial-branch=main")
	runTestGit(t, source, "config", "user.name", "test")
	runTestGit(t, source, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "result.md"), []byte("frozen result\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, source, "add", "result.md")
	runTestGit(t, source, "commit", "-m", "result")
	commit := strings.TrimSpace(runTestGit(t, source, "rev-parse", "HEAD^{commit}"))
	tree := strings.TrimSpace(runTestGit(t, source, "rev-parse", "HEAD^{tree}"))
	destination := filepath.Join(t.TempDir(), "repository.git")
	if err := materializeReviewRepository(ctx, source, destination, commit, tree); err != nil {
		t.Fatalf("materializeReviewRepository() error = %v", err)
	}
	// The production Result directory intentionally remains sealed. Restore
	// test-only permissions before t.TempDir's automatic cleanup runs.
	t.Cleanup(func() {
		_ = filepath.WalkDir(destination, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				_ = os.Chmod(path, 0o755)
			} else {
				_ = os.Chmod(path, 0o644)
			}
			return nil
		})
	})
	if got, err := gitBareOutput(ctx, destination, "rev-parse", commit+"^{commit}"); err != nil || got != commit {
		t.Fatalf("frozen bare ResultCommit = %q, %v; want %q", got, err, commit)
	}
	if got, err := gitBareOutput(ctx, destination, "rev-parse", commit+"^{tree}"); err != nil || got != tree {
		t.Fatalf("frozen bare ResultTree = %q, %v; want %q", got, err, tree)
	}
	if remotes, err := gitBareOutput(ctx, destination, "remote"); err != nil || remotes != "" {
		t.Fatalf("frozen bare repository retained remote=%q err=%v", remotes, err)
	}
	if info, err := os.Stat(destination); err != nil || info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("frozen repository directory remained writable: info=%v err=%v", info, err)
	}
	if err := os.WriteFile(filepath.Join(destination, "must-not-write"), []byte("x"), 0o600); err == nil {
		t.Fatal("frozen review repository accepted a local write")
	}
	if err := os.WriteFile(filepath.Join(source, "later.md"), []byte("mutable agent file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, source, "add", "later.md")
	runTestGit(t, source, "commit", "-m", "later")
	if _, err := gitBareOutput(ctx, destination, "rev-parse", "HEAD"); err == nil {
		t.Fatal("frozen repository unexpectedly has a mutable Agent HEAD ref")
	}
}

func runTestGit(t *testing.T, repo string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
