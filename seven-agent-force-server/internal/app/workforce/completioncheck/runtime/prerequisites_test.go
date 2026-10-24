package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
)

func TestMissingProjectPrerequisitesDoNotRequestCodeRepair(t *testing.T) {
	for _, diagnostic := range []string{"ERR_PNPM_NO_SCRIPT Missing script: test", "Error: Cannot find module dependency", "connect ECONNREFUSED"} {
		out := Shell(context.Background(), domain.Input{Mode: "COMMAND", Command: "printf '%s' '" + diagnostic + "' >&2; exit 1"}, t.TempDir(), time.Second)
		if out.Status != domain.Unavailable {
			t.Fatalf("prerequisite became code failure: %s", out.Status)
		}
	}
	out := Shell(context.Background(), domain.Input{Mode: "COMMAND", Command: "printf 'No tests found' >&2; exit 1"}, t.TempDir(), time.Second)
	if out.Status != domain.NoTests {
		t.Fatalf("zero tests: %s", out.Status)
	}
	out = Shell(context.Background(), domain.Input{Mode: "COMMAND", Command: "printf 'assertion failed' >&2; exit 1"}, t.TempDir(), time.Second)
	if out.Status != domain.Failed {
		t.Fatalf("assertion: %s", out.Status)
	}
}
