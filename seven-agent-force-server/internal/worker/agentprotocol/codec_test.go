package agentprotocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSharedFrames(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "../../../../deploy/workforce/runtime-sdk/protocol/frames.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name, Direction, Wire string
		Valid                 bool
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			_, err := Decode([]byte(fixture.Wire), fixture.Direction)
			if (err == nil) != fixture.Valid {
				t.Fatalf("valid=%v error=%v", fixture.Valid, err)
			}
		})
	}
}
