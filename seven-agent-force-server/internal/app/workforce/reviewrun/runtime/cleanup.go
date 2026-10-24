package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

func (d *Docker) BindResourceJournal(j *resourcejournal.Journal) {
	d.cleanupJournal = j
	raw := d.exec
	d.cleanupCommand = func(c context.Context, n string, a ...string) (string, error) {
		b, e := raw(c, n, a...)
		return string(b), e
	}
	d.exec = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		// Every newly created review resource carries the stable Worker owner,
		// including volumes and relay sidecars that predate the main container.
		if name == "docker" && len(args) > 0 {
			offset := 0
			if args[0] == "create" {
				offset = 1
			}
			if len(args) > 1 && (args[0] == "volume" || args[0] == "network") && args[1] == "create" {
				offset = 2
			}
			if offset > 0 {
				wrapped := append([]string{}, args[:offset]...)
				wrapped = append(wrapped, "--label", "seven.review.worker.id="+strconv.FormatInt(d.cfg.ID, 10))
				args = append(wrapped, args[offset:]...)
			}
		}
		out, err := j.Execute(ctx, func(c context.Context, n string, a ...string) (string, error) {
			b, e := raw(c, n, a...)
			return string(b), e
		}, name, args...)
		return []byte(out), err
	}
}

func (d *Docker) cleanupReviewer(ctx context.Context, id string) error {
	// Existing journal ownership is sufficient even when Docker is offline:
	// the cleanup request is persisted and capacity stays reserved for retry.
	r, ok := d.cleanupJournal.Find("container", id)
	if !ok {
		body, err := d.exec(ctx, "docker", "container", "inspect", id)
		if err != nil {
			return errors.New("REVIEW_CLEANUP_INSPECT_FAILED")
		}
		var rows []struct {
			ID     string `json:"Id"`
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
		}
		if json.Unmarshal(body, &rows) != nil || len(rows) != 1 || rows[0].ID != id || rows[0].Config.Labels["seven.managed"] != "true" || rows[0].Config.Labels["seven.review.worker.id"] != strconv.FormatInt(d.cfg.ID, 10) {
			return errors.New("REVIEW_CLEANUP_OWNER_CONFLICT")
		}
		r, err = d.cleanupJournal.Adopt("container", id, rows[0].Config.Labels)
		if err != nil {
			return err
		}
	}
	// Remove the container first, then its exact run/epoch resources. Docker
	// refuses a volume/network still referenced by another container.
	_, err := d.exec(ctx, "docker", "rm", "-f", id)
	if err != nil {
		return err
	}
	labels := map[string]string{"seven.review.run.id": r.Labels["seven.review.run.id"], "seven.review.owner.epoch": r.Labels["seven.review.owner.epoch"]}
	if labels["seven.review.run.id"] == "" || labels["seven.review.owner.epoch"] == "" {
		return nil
	}
	return d.cleanupJournal.CleanupMatching(labels, func(c context.Context, r resourcejournal.Resource) error {
		return resourcejournal.Remove(c, d.cleanupCommand, r)
	})
}
