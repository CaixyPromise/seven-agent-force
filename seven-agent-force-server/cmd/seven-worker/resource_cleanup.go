package main

import (
	"context"
	"os/exec"
	"strconv"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
	registrydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	"go.uber.org/zap"
)

func cleanupCommand(ctx context.Context, name string, args ...string) (string, error) {
	body, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(body), err
}

func removeResource(ctx context.Context, r resourcejournal.Resource) error {
	return resourcejournal.Remove(ctx, cleanupCommand, r)
}

func (w *worker) recoverResourceJournal(ctx context.Context, recovery registrydomain.Reconciliation) error {
	protectedAttempts := map[string]bool{}
	for _, r := range recovery.Runtimes {
		protectedAttempts[strconv.FormatInt(r.AttemptID, 10)] = true
		labels := map[string]string{"seven.managed": "true", "seven.worker.id": strconv.FormatInt(w.cfg.Worker.ID, 10), "seven.task.id": strconv.FormatInt(r.TaskID, 10), "seven.attempt.id": strconv.FormatInt(r.AttemptID, 10), "seven.attempt.generation": strconv.FormatInt(r.Generation, 10)}
		if r.ContainerID != "" {
			if _, err := w.cleanupJournal.Adopt("container", r.ContainerID, labels); err != nil {
				return err
			}
		}
		if r.ProviderBacked && r.ProviderProxyContainerID != "" {
			if _, err := w.cleanupJournal.Adopt("container", r.ProviderProxyContainerID, labels); err != nil {
				return err
			}
		}
	}
	protectedReviews := map[string]bool{}
	if w.cfg.Reviewer.Enabled {
		rows, err := w.reviewRuntime.ManagedContainers(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			protectedReviews[strconv.FormatInt(r.ReviewRunID, 10)+":"+strconv.FormatInt(r.OwnerEpoch, 10)] = true
			if _, err := w.cleanupJournal.Adopt("container", r.ID, map[string]string{"seven.managed": "true", "seven.review.worker.id": strconv.FormatInt(w.cfg.Worker.ID, 10), "seven.review.run.id": strconv.FormatInt(r.ReviewRunID, 10), "seven.review.owner.epoch": strconv.FormatInt(r.OwnerEpoch, 10)}); err != nil {
				return err
			}
		}
	}
	if err := w.cleanupJournal.Recover(func(r resourcejournal.Resource) bool {
		if r.Labels["seven.completion.check"] == "true" || r.Labels["seven.cleanup.ephemeral"] == "true" {
			return false
		}
		return protectedAttempts[r.Labels["seven.attempt.id"]] || protectedReviews[r.Labels["seven.review.run.id"]+":"+r.Labels["seven.review.owner.epoch"]]
	}); err != nil {
		return err
	}
	// Failure remains a durable reservation. The Worker can still reconcile
	// Server-owned live work, but cannot consume new assignments until retry.
	if err := w.cleanupJournal.Retry(removeResource); err != nil {
		w.logger.Warn("resource cleanup remains pending", zap.Int("pending", w.cleanupJournal.Pending()))
	}
	return nil
}

func (w *worker) resourceCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if err := w.cleanupJournal.Retry(removeResource); err != nil {
			w.logger.Warn("resource cleanup retry pending", zap.Int("pending", w.cleanupJournal.Pending()))
		}
	}
}
