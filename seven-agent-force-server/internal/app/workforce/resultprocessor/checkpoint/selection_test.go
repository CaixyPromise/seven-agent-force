package checkpoint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

func TestSelectResultKeepsTrackedRegenerableAndExcludesUntracked(t *testing.T) {
	t.Parallel()
	repo := selectionRepository(t)
	writeSelectionFile(t, repo, "dist/tracked.js", "tracked")
	writeSelectionFile(t, repo, "src/main.go", "package main\n")
	gitSelection(t, repo, "add", "dist/tracked.js", "src/main.go")
	gitSelection(t, repo, "commit", "-qm", "base")
	writeSelectionFile(t, repo, "dist/tracked.js", "changed")
	writeSelectionFile(t, repo, "dist/generated.js", "regenerable")

	selection, err := SelectResult(context.Background(), SelectionInput{RepoRoot: repo, BaseCommit: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if !selection.ResultTreeIndex.Contains("dist/tracked.js") {
		t.Fatal("tracked dist file was excluded")
	}
	if selection.ResultTreeIndex.Contains("dist/generated.js") {
		t.Fatal("untracked regenerable dist file entered ResultTree")
	}
	if reason := selection.ExcludedIndex.Reason("dist/generated.js"); reason != "PLATFORM_REGENERABLE" {
		t.Fatalf("generated dist reason=%q", reason)
	}
}

func TestSelectResultUsesRepositoryGitignoreButNotInfoExclude(t *testing.T) {
	t.Parallel()
	repo := selectionRepository(t)
	writeSelectionFile(t, repo, ".gitignore", "generated/**\n!generated/keep.txt\n")
	writeSelectionFile(t, repo, "packages/widget/.gitignore", "cache/**\n!cache/keep.txt\n")
	writeSelectionFile(t, repo, "README.md", "base")
	gitSelection(t, repo, "add", ".gitignore", "packages/widget/.gitignore", "README.md")
	gitSelection(t, repo, "commit", "-qm", "base")
	writeSelectionFile(t, repo, "generated/drop.txt", "drop")
	writeSelectionFile(t, repo, "generated/keep.txt", "keep")
	writeSelectionFile(t, repo, "info-only.txt", "must remain")
	writeSelectionFile(t, repo, "packages/widget/cache/drop.txt", "drop")
	writeSelectionFile(t, repo, "packages/widget/cache/keep.txt", "keep")
	gitDir := gitSelectionOutput(t, repo, "rev-parse", "--git-dir")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repo, gitDir)
	}
	writeSelectionFile(t, gitDir, "info/exclude", "info-only.txt\n")

	selection, err := SelectResult(context.Background(), SelectionInput{RepoRoot: repo, BaseCommit: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if selection.ResultTreeIndex.Contains("generated/drop.txt") {
		t.Fatal("ignored file entered ResultTree")
	}
	if !selection.ResultTreeIndex.Contains("generated/keep.txt") {
		t.Fatal("gitignore negation was not honored")
	}
	if !selection.ResultTreeIndex.Contains("info-only.txt") {
		t.Fatal(".git/info/exclude affected controlled selection")
	}
	if selection.ResultTreeIndex.Contains("packages/widget/cache/drop.txt") || !selection.ResultTreeIndex.Contains("packages/widget/cache/keep.txt") {
		t.Fatal("nested .gitignore or its negation was not honored")
	}
}

func TestSelectResultExcludesNestedRegenerableAndRejectsProtectedPaths(t *testing.T) {
	t.Parallel()
	repo := selectionRepository(t)
	writeSelectionFile(t, repo, "README.md", "base")
	gitSelection(t, repo, "add", "README.md")
	gitSelection(t, repo, "commit", "-qm", "base")
	writeSelectionFile(t, repo, "packages/api/node_modules/pkg/index.js", "generated")

	selection, err := SelectResult(context.Background(), SelectionInput{RepoRoot: repo, BaseCommit: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if selection.ResultTreeIndex.Contains("packages/api/node_modules/pkg/index.js") || selection.ExcludedIndex.Reason("packages/api/node_modules/pkg/index.js") != "PLATFORM_REGENERABLE" {
		t.Fatal("nested regenerable directory entered ResultTree")
	}

	writeSelectionFile(t, repo, ".seven/runtime/lease", "must-not-freeze")
	if _, err = SelectResult(context.Background(), SelectionInput{RepoRoot: repo, BaseCommit: "HEAD"}); err == nil || !strings.Contains(err.Error(), "RESULT_PROTECTED_PATH_FORBIDDEN:.seven") {
		t.Fatalf("protected result path error=%v", err)
	}
}

func TestSelectResultExcludesRegenerableSymlinkButRejectsTrackedSymlink(t *testing.T) {
	t.Parallel()
	repo := selectionRepository(t)
	writeSelectionFile(t, repo, "README.md", "base")
	gitSelection(t, repo, "add", "README.md")
	gitSelection(t, repo, "commit", "-qm", "base")
	if err := os.MkdirAll(filepath.Join(repo, "node_modules", ".store", "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".store", "pkg"), filepath.Join(repo, "node_modules", "pkg")); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectResult(context.Background(), SelectionInput{RepoRoot: repo, BaseCommit: "HEAD"}); err != nil {
		t.Fatalf("regenerable dependency symlink blocked result selection: %v", err)
	}

	trackedRepo := selectionRepository(t)
	writeSelectionFile(t, trackedRepo, "README.md", "base")
	if err := os.Symlink("README.md", filepath.Join(trackedRepo, "linked-readme")); err != nil {
		t.Fatal(err)
	}
	gitSelection(t, trackedRepo, "add", "README.md", "linked-readme")
	gitSelection(t, trackedRepo, "commit", "-qm", "tracked symlink")
	if _, err := SelectResult(context.Background(), SelectionInput{RepoRoot: trackedRepo, BaseCommit: "HEAD"}); err == nil || !strings.Contains(err.Error(), "RESULT_SYMLINK_FORBIDDEN:linked-readme") {
		t.Fatalf("tracked symlink error=%v", err)
	}
}

func TestSelectResultRejectsGitLFSAndSubmodule(t *testing.T) {
	t.Parallel()
	lfsRepo := selectionRepository(t)
	writeSelectionFile(t, lfsRepo, ".gitattributes", "large.bin filter=lfs diff=lfs merge=lfs -text\n")
	writeSelectionFile(t, lfsRepo, "large.bin", "version https://git-lfs.github.com/spec/v1\n")
	gitSelection(t, lfsRepo, "add", ".gitattributes", "large.bin")
	gitSelection(t, lfsRepo, "commit", "-qm", "lfs")
	if _, err := SelectResult(context.Background(), SelectionInput{RepoRoot: lfsRepo, BaseCommit: "HEAD"}); err == nil || !strings.Contains(err.Error(), "RESULT_GIT_LFS_FORBIDDEN:large.bin") {
		t.Fatalf("LFS error=%v", err)
	}

	submoduleRepo := selectionRepository(t)
	writeSelectionFile(t, submoduleRepo, "README.md", "base")
	gitSelection(t, submoduleRepo, "add", "README.md")
	gitSelection(t, submoduleRepo, "commit", "-qm", "base")
	gitSelection(t, submoduleRepo, "update-index", "--add", "--cacheinfo", "160000,1111111111111111111111111111111111111111,vendor/module")
	gitSelection(t, submoduleRepo, "commit", "-qm", "gitlink")
	if _, err := SelectResult(context.Background(), SelectionInput{RepoRoot: submoduleRepo, BaseCommit: "HEAD"}); err == nil || !strings.Contains(err.Error(), "RESULT_SUBMODULE_FORBIDDEN:vendor/module") {
		t.Fatalf("submodule error=%v", err)
	}
}

func TestSelectResultDeclaredIgnoredFileEntersOverlayOnly(t *testing.T) {
	t.Parallel()
	repo := selectionRepository(t)
	writeSelectionFile(t, repo, ".gitignore", "private-report/**\n")
	writeSelectionFile(t, repo, "README.md", "base")
	gitSelection(t, repo, "add", ".gitignore", "README.md")
	gitSelection(t, repo, "commit", "-qm", "base")
	writeSelectionFile(t, repo, "private-report/report.json", "{}")
	selection, err := SelectResult(context.Background(), SelectionInput{
		RepoRoot: repo, BaseCommit: "HEAD",
		Outputs:              []processordomain.Output{{Name: "report", Kind: "PATH", Path: "private-report/report.json", Storage: "ON_DEMAND"}},
		ContinuationIncludes: []string{"private-report/report.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.ResultTreeIndex.Contains("private-report/report.json") {
		t.Fatal("ignored declaration expanded ResultTree")
	}
	if !selection.OverlayIndex.Contains("private-report/report.json") {
		t.Fatal("declared continuation file missing from Overlay")
	}
	if !selection.OutputIndexes["report"].Contains("private-report/report.json") {
		t.Fatal("declared output index missing file")
	}
}

func selectionRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitSelection(t, repo, "init", "-q")
	gitSelection(t, repo, "config", "user.name", "Selection Test")
	gitSelection(t, repo, "config", "user.email", "selection@example.invalid")
	return repo
}

func writeSelectionFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitSelection(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func gitSelectionOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(bytesTrimSpace(output))
}

func bytesTrimSpace(value []byte) []byte {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\n' || value[start] == '\r' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\t') {
		end--
	}
	return value[start:end]
}
