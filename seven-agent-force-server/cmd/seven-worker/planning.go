package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	input "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

func planningSourcePath(w runtime.Workspace) string {
	return filepath.Join(filepath.Dir(w.SpecPath), "planning-source")
}

// finishTaskInput separates the immutable project checkout from the writable
// plan repository. Docker exposes InputRoot read-only; AttemptRoot never aliases
// the project checkout. A chmod-only restriction would not provide that boundary.
func (w *worker) finishTaskInput(ctx context.Context, item task.WorkItem, space runtime.Workspace) (runtime.Workspace, string, error) {
	if item.ExecutionSnapshot.StagePurpose != "PLAN" {
		return space, item.TaskInput.SourceCommit, nil
	}
	source := planningSourcePath(space)
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		return space, "", errors.New("PLANNING_SOURCE_ALREADY_EXISTS")
	}
	if err := os.Rename(space.Repo, source); err != nil {
		return space, "", err
	}
	if err := os.MkdirAll(space.Repo, 0777); err != nil {
		return space, "", err
	}
	identity := input.CanonicalEmptyWorkspaceIdentity()
	expected := task.TaskInputContract{SourceCommit: identity.Commit, SourceTree: identity.Tree, SourceIdentityDigest: identity.IdentityDigest}
	if err := initializeEmptyTaskWorkspace(ctx, space.Repo, space.Temporary, expected); err != nil {
		return space, "", err
	}
	if err := os.Chmod(space.Repo, 0777); err != nil {
		return space, "", err
	}
	return space, identity.Commit, nil
}

// Continuation may restore on another Worker. Rebuild the read-only source from
// the original verified bundle, not from the plan checkpoint's writable repo.
func (w *worker) ensurePlanningSource(ctx context.Context, item task.WorkItem, space runtime.Workspace) error {
	if item.ExecutionSnapshot.StagePurpose != "PLAN" {
		return nil
	}
	if item.TaskInput == nil {
		return errors.New("PLANNING_SOURCE_MISSING")
	}
	source := planningSourcePath(space)
	if info, e := os.Lstat(source); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("PLANNING_SOURCE_INVALID")
		}
		return verifyPlanningSource(ctx, source, *item.TaskInput)
	} else if !os.IsNotExist(e) {
		return e
	}
	tmp, err := os.MkdirTemp(filepath.Dir(source), ".planning-source-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	// The Attempt runs under its own uid; readability comes from permissions,
	// while the Docker bind mount enforces the read-only security boundary.
	if err = os.Chmod(tmp, 0755); err != nil {
		return err
	}
	i := item.TaskInput
	if i.SourceKind == input.SourceKindEmptyWorkspace {
		if err = initializeEmptyTaskWorkspace(ctx, tmp, space.Temporary, *i); err != nil {
			return err
		}
	} else {
		bundle := filepath.Join(space.Temporary, "planning-source.bundle")
		if e := w.downloadVerifiedObject(ctx, i.RepositoryBundleObjectKey, i.RepositoryBundleObjectSHA256, i.RepositoryBundleContentSHA256, i.RepositoryBundleSizeBytes, bundle, maxSourceBytes); e != nil {
			return e
		}
		for _, args := range [][]string{{"init", "-q"}, {"-c", "protocol.file.allow=always", "fetch", "-q", bundle, "refs/*:refs/*"}, {"checkout", "--detach", "-q", i.SourceCommit}} {
			if _, e := controlledWorkerGit(ctx, tmp, args...); e != nil {
				return errors.New("PLANNING_SOURCE_RESTORE_FAILED")
			}
		}
	}
	if err = verifyPlanningSource(ctx, tmp, *i); err != nil {
		return err
	}
	return os.Rename(tmp, source)
}

func verifyPlanningSource(ctx context.Context, source string, i task.TaskInputContract) error {
	commit, err := controlledWorkerGit(ctx, source, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(commit) != i.SourceCommit {
		return errors.New("PLANNING_SOURCE_IDENTITY_MISMATCH")
	}
	tree, err := controlledWorkerGit(ctx, source, "rev-parse", "HEAD^{tree}")
	if err != nil || strings.TrimSpace(tree) != i.SourceTree {
		return errors.New("PLANNING_SOURCE_IDENTITY_MISMATCH")
	}
	status, err := controlledWorkerGit(ctx, source, "status", "--porcelain", "--untracked-files=all")
	if err != nil || strings.TrimSpace(status) != "" {
		return errors.New("PLANNING_SOURCE_DIRTY")
	}
	return nil
}
