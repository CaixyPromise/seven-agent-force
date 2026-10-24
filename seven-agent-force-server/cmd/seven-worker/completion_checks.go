package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	checkdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	checkfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	checkruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/runtime"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

// runCompletionCheck only records a fenced observation. The Server compares
// it to its original source baseline and owns the completion decision.
func (w *worker) runCompletionCheck(ctx context.Context, item taskdomain.WorkItem, attempt taskdomain.Attempt, workspace attemptruntime.Workspace, phase string, cycle int64) error {
	input, inputDigest, frozenDigest := item.CompletionCheck, item.CompletionCheckInputDigest, item.CompletionCheckDigest
	if input == nil {
		return nil
	}
	digest, err := checkdomain.Digest(*input)
	if err != nil || inputDigest == "" || digest != inputDigest || frozenDigest == "" {
		return errors.New("CHECK_FROZEN_IDENTITY_INVALID")
	}
	composed, err := checkfacade.FrozenDigest(*input, item.CompletionCheckReferences)
	if err != nil || composed != frozenDigest {
		return errors.New("CHECK_FROZEN_IDENTITY_INVALID")
	}
	existing, err := w.control.ReadCompletionCheck(ctx, attempt, phase, cycle, frozenDigest)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.CheckDigest != frozenDigest || existing.Phase != phase {
			return errors.New("CHECK_EVIDENCE_IDENTITY_INVALID")
		}
		if phase == "CURRENT" && (existing.TaskID != item.TaskID || existing.TaskTurnID != item.TaskTurnID || existing.AttemptID != attempt.ID || existing.EvaluationCycleID != cycle) {
			return errors.New("CHECK_EVIDENCE_IDENTITY_INVALID")
		}
		return nil
	}
	recordCtx := ctx
	record := func(observation checkdomain.Observation) error {
		_, err := w.control.RecordCompletionCheck(recordCtx, attempt, checkfacade.Evidence{TaskID: item.TaskID, TaskTurnID: item.TaskTurnID, AttemptID: attempt.ID, EvaluationCycleID: cycle, Phase: phase, CheckDigest: frozenDigest, Status: string(observation.Status), Observation: observation})
		return err
	}
	if input.Mode == "NONE" {
		return record(checkdomain.Observation{CheckDigest: frozenDigest, Status: checkdomain.NotConfigured})
	}
	checkTimeout := 5 * time.Minute
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < checkTimeout {
		checkTimeout = time.Until(deadline)
	}
	if checkTimeout <= 0 {
		return context.DeadlineExceeded
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	privateRoot := filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "completion-checks")
	if err = os.MkdirAll(privateRoot, 0700); err != nil {
		return err
	}
	repository, baseCommit := workspace.Repo, item.ResolvedBaseCommit
	if phase == "BASELINE" {
		// Restore from the authenticated immutable source object, never the
		// mutable Attempt or a continuation checkpoint. New verification digests
		// get their own baseline; existing digests were reused above.
		sourceRoot, e := os.MkdirTemp(privateRoot, "source-")
		if e != nil {
			return e
		}
		row, e := w.cleanupJournal.TrackDirectory(sourceRoot, map[string]string{"seven.completion.check": "true"})
		if e != nil {
			return e
		}
		defer w.cleanupJournal.Cleanup(row.Key, nil)
		repository, err = w.restoreCompletionCheckSource(ctx, item, row.Path)
		if err != nil {
			return err
		}
		baseCommit = item.TaskInput.SourceCommit
	}
	runner := checkruntime.Runner{Journal: w.cleanupJournal}
	observation, runErr := runner.Run(ctx, checkruntime.Request{
		Input: *input, InputDigest: inputDigest, Digest: frozenDigest, References: item.CompletionCheckReferences, Phase: phase, Repository: repository, BaseCommit: baseCommit,
		Image: item.HarnessImageDigest, PrivateRoot: privateRoot, Timeout: checkTimeout,
		Labels: map[string]string{"seven.worker.id": strconv.FormatInt(w.cfg.Worker.ID, 10), "seven.task.id": strconv.FormatInt(item.TaskID, 10), "seven.attempt.id": strconv.FormatInt(attempt.ID, 10)},
	})
	if runErr != nil && observation.Status != checkdomain.TimedOut {
		observation.Status = checkdomain.Unavailable
	}
	if observation.CheckDigest != frozenDigest {
		return errors.New("CHECK_OBSERVATION_IDENTITY_INVALID")
	}
	return record(observation)
}

func (w *worker) restoreCompletionCheckSource(ctx context.Context, item taskdomain.WorkItem, root string) (string, error) {
	if item.TaskInput == nil || item.TaskInput.SourceCommit == "" || item.TaskInput.SourceTree == "" {
		return "", errors.New("CHECK_FROZEN_SOURCE_MISSING")
	}
	repo := filepath.Join(root, "repository")
	if err := os.Mkdir(repo, 0700); err != nil {
		return "", err
	}
	input := *item.TaskInput
	if input.SourceKind == workspaceinputdomain.SourceKindEmptyWorkspace {
		if err := initializeEmptyTaskWorkspace(ctx, repo, root, input); err != nil {
			return "", err
		}
		return repo, nil
	}
	if input.SourceKind != workspaceinputdomain.SourceKindSourceRevision || input.RepositoryBundleObjectKey == "" {
		return "", errors.New("CHECK_FROZEN_SOURCE_INVALID")
	}
	bundle := filepath.Join(root, "source.bundle")
	if err := w.downloadVerifiedObject(ctx, input.RepositoryBundleObjectKey, input.RepositoryBundleObjectSHA256, input.RepositoryBundleContentSHA256, input.RepositoryBundleSizeBytes, bundle, maxSourceBytes); err != nil {
		return "", err
	}
	for _, args := range [][]string{{"init", "-q"}, {"-c", "protocol.file.allow=always", "fetch", "-q", bundle, "refs/*:refs/*"}, {"checkout", "--detach", "-q", input.SourceCommit}} {
		if _, err := controlledWorkerGit(ctx, repo, args...); err != nil {
			return "", errors.New("CHECK_FROZEN_SOURCE_INVALID")
		}
	}
	tree, err := controlledWorkerGit(ctx, repo, "rev-parse", "HEAD^{tree}")
	if err != nil || tree != input.SourceTree {
		return "", errors.New("CHECK_FROZEN_SOURCE_IDENTITY_MISMATCH")
	}
	return repo, nil
}
