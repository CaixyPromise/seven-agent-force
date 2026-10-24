package resourcejournal

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestCleanupExactOptionalEmptyLabel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		labels map[string]string
		owner  string
		pass   bool
	}{
		{"present-empty", map[string]string{OwnershipLabel: "owner", "seven.hook.configuration.digest": ""}, "owner", true},
		{"missing-empty-key", map[string]string{OwnershipLabel: "owner"}, "owner", false},
		{"changed-value", map[string]string{OwnershipLabel: "owner", "seven.hook.configuration.digest": "changed"}, "owner", false},
		{"wrong-owner", map[string]string{OwnershipLabel: "other", "seven.hook.configuration.digest": ""}, "owner", false},
		{"empty-owner", map[string]string{OwnershipLabel: "", "seven.hook.configuration.digest": ""}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			removed := false
			r := Resource{Kind: "container", ID: "exact-id", Labels: map[string]string{OwnershipLabel: tc.owner, "seven.hook.configuration.digest": ""}}
			run := func(_ context.Context, _ string, args ...string) (string, error) {
				if args[1] == "rm" {
					removed = true
					return "", nil
				}
				if removed {
					return "", errors.New("No such container: exact-id")
				}
				b, _ := json.Marshal([]any{map[string]any{"Id": "exact-id", "Config": map[string]any{"Labels": tc.labels}}})
				return string(b), nil
			}
			err := Remove(context.Background(), run, r)
			if (err == nil) != tc.pass || removed != tc.pass {
				t.Fatalf("removed=%v err=%v", removed, err)
			}
		})
	}
}
