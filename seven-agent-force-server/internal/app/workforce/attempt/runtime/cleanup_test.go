package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

func TestAttemptCleanupPersistsDaemonFailureUnderIndependentContext(t *testing.T) {
	j, e := resourcejournal.Open(filepath.Join(t.TempDir(), "journal"))
	if e != nil {
		t.Fatal(e)
	}
	d := NewDocker(config.WorkerConfig{ID: 1}, config.ProviderProxyConfig{})
	d.commandText = func(ctx context.Context, name string, args ...string) (string, error) {
		deadline, ok := ctx.Deadline()
		if ctx.Err() != nil || !ok || time.Until(deadline) > 30*time.Second {
			t.Fatal("unbounded cleanup")
		}
		return "", errors.New("Docker unavailable")
	}
	d.BindResourceJournal(j)
	if e = d.cleanupContainer("exact-container", 2, 3); e == nil {
		t.Fatal("cleanup error lost")
	}
	if j.Pending() != 1 {
		t.Fatal("cleanup reservation missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = d.TerminateRuntimeForLoss(ctx, 2, 3, "old-name", "exact-container", 1, 1); e == nil {
		t.Fatal("loss gate ignored Docker failure")
	}
}
