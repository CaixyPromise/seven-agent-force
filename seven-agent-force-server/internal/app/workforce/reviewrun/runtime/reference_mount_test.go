package runtime

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReattachReferenceMountMustAlreadyBeExactAndReadOnly(t *testing.T) {
	for _, test := range []struct {
		name, source, kind string
		rw, valid          bool
	}{
		{"exact", "/private/review/references", "bind", false, true},
		{"task input", "/private/task/input", "bind", false, false},
		{"other owner", "/private/other/references", "bind", false, false},
		{"writable", "/private/review/references", "bind", true, false},
		{"volume", "/private/review/references", "volume", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := &Docker{exec: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if name != "docker" || len(args) != 3 || args[0] != "container" || args[1] != "inspect" || args[2] != "exact-id" {
					t.Fatalf("reattach changed mount: %v", args)
				}
				return json.Marshal([]any{map[string]any{"Id": "exact-id", "Mounts": []any{map[string]any{"Source": test.source, "Destination": "/review/references", "Type": test.kind, "RW": test.rw}}}})
			}}
			err := d.verifyReferenceMount(context.Background(), "exact-id", "/private/review/references", true)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}
