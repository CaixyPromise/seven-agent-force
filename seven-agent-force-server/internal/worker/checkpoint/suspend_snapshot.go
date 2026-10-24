package checkpoint

import (
	"context"
	"os"
	"path/filepath"

	artifact "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/application"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
)

// A stopped interaction can have session state without repository changes.
// Recompute the selected tree in a private index; equal trees retain the exact
// base commit, not a fabricated result. RESULT finalization remains unchanged.
func suspendSnapshotIdentity(ctx context.Context, p base.Input, selection base.ResultFileSelection) (artifact.Result, error) {
	dir, err := os.MkdirTemp("", "seven-suspend-index-")
	if err != nil {
		return artifact.Result{}, err
	}
	defer os.RemoveAll(dir)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index")}
	git := func(args ...string) (string, error) { return gitWithEnvironment(ctx, p.RepoRoot, env, args...) }
	commit, err := git("rev-parse", "--verify", p.BaseCommit+"^{commit}")
	if err != nil {
		return artifact.Result{}, err
	}
	if _, err = git("read-tree", commit); err != nil {
		return artifact.Result{}, err
	}
	for _, entry := range selection.ResultTreeIndex {
		if entry.Type == "DELETED" {
			_, err = git("update-index", "--remove", "--", entry.Path)
		} else {
			var blob string
			blob, err = git("hash-object", "-w", "--", entry.Path)
			if err == nil {
				_, err = git("update-index", "--add", "--cacheinfo", entry.GitMode+","+blob+","+entry.Path)
			}
		}
		if err != nil {
			return artifact.Result{}, err
		}
	}
	tree, err := git("write-tree")
	if err != nil {
		return artifact.Result{}, err
	}
	baseTree, err := git("rev-parse", "--verify", commit+"^{tree}")
	if err != nil {
		return artifact.Result{}, err
	}
	if tree == baseTree {
		return artifact.Result{Commit: commit, Tree: tree, SelectionDigest: selection.SelectionDigest}, nil
	}
	return artifact.ComputeResultIdentityForPlan(ctx, p.RepoRoot, p.BaseCommit, nil, p.ContinuationIncludes)
}
