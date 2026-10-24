package runtime

import (
	"context"
	"errors"
	"strconv"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

func (d *Docker) BindResourceJournal(j *resourcejournal.Journal) {
	d.cleanupJournal = j
	raw := d.commandText
	d.cleanupCommand = raw
	d.commandText = func(ctx context.Context, name string, args ...string) (string, error) {
		return j.Execute(ctx, raw, name, args...)
	}
}

func (d *Docker) cleanupContainer(containerID string, taskID, attemptID int64) error {
	labels := map[string]string{"seven.managed": "true", "seven.task.id": strconv.FormatInt(taskID, 10), "seven.attempt.id": strconv.FormatInt(attemptID, 10)}
	if d.cleanupJournal != nil {
		r, err := d.cleanupJournal.Adopt("container", containerID, labels)
		if err != nil {
			return err
		}
		return d.cleanupJournal.Cleanup(r.Key, func(ctx context.Context, r resourcejournal.Resource) error {
			return resourcejournal.Remove(ctx, d.cleanupCommand, r)
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), resourcejournal.Timeout)
	defer cancel()
	return resourcejournal.Remove(ctx, d.commandText, resourcejournal.Resource{Kind: "container", Name: containerID, ID: containerID, Labels: labels})
}

func (d *Docker) cleanupLost(containerID string, labels map[string]string) error {
	if containerID == "" {
		return errors.New("CLEANUP_EXACT_ID_REQUIRED")
	}
	if d.cleanupJournal == nil {
		ctx, cancel := context.WithTimeout(context.Background(), resourcejournal.Timeout)
		defer cancel()
		return resourcejournal.Remove(ctx, d.commandText, resourcejournal.Resource{Kind: "container", ID: containerID, Name: containerID, Labels: labels})
	}
	r, err := d.cleanupJournal.Adopt("container", containerID, labels)
	if err != nil {
		return err
	}
	return d.cleanupJournal.Cleanup(r.Key, func(ctx context.Context, r resourcejournal.Resource) error {
		return resourcejournal.Remove(ctx, d.cleanupCommand, r)
	})
}
