package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
)

// Reattach never adds or swaps mounts on a recovered container. Required
// references must already be the exact owner-attempt copy, mounted read-only.
func (d *Docker) verifyReferenceMount(ctx context.Context, id, root string, present bool) error {
	if !present {
		return nil
	}
	body, err := d.exec(ctx, "docker", "container", "inspect", id)
	if err != nil {
		return errors.New("REVIEW_REFERENCE_MOUNT_UNAVAILABLE")
	}
	var rows []struct {
		ID     string `json:"Id"`
		Mounts []struct {
			Type, Source, Destination string
			RW                        bool
		} `json:"Mounts"`
	}
	if json.Unmarshal(body, &rows) != nil || len(rows) != 1 || rows[0].ID != id {
		return errors.New("REVIEW_REFERENCE_MOUNT_INVALID")
	}
	count := 0
	for _, mount := range rows[0].Mounts {
		if mount.Destination == "/review/references" {
			if mount.Type != "bind" || mount.RW || filepath.Clean(mount.Source) != filepath.Clean(root) {
				return errors.New("REVIEW_REFERENCE_MOUNT_INVALID")
			}
			count++
		}
	}
	if count != 1 {
		return errors.New("REVIEW_REFERENCE_MOUNT_INVALID")
	}
	return nil
}
