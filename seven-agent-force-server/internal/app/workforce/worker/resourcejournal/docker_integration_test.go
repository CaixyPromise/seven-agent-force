package resourcejournal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerJournalRetryRestartOwnershipAndCapacity(t *testing.T) {
	image := os.Getenv("BATCH1_CHECK_DOCKER_IMAGE")
	if image == "" {
		t.Skip("BATCH1_CHECK_DOCKER_IMAGE required")
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		t.Fatal("exact digest required")
	}
	ctx := context.Background()
	run := func(c context.Context, n string, a ...string) (string, error) {
		b, e := exec.CommandContext(c, n, a...).CombinedOutput()
		return strings.TrimSpace(string(b)), e
	}
	prefix := fmt.Sprintf("seven-batch1-journal-%d", time.Now().UnixNano())
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	j, e := Open(filepath.Join(root, "journal"))
	if e != nil {
		t.Fatal(e)
	}
	j.SetContainerLimit(1)
	remove := func(c context.Context, r Resource) error { return Remove(c, run, r) }
	// Test-owned cleanup fallback is exact by name and never operates on other objects.
	for _, kind := range []string{"container", "volume", "network"} {
		kind := kind
		t.Cleanup(func() {
			args := []string{kind, "rm"}
			if kind == "container" {
				args = append(args, "-f")
			}
			_, _ = run(ctx, "docker", append(args, prefix)...)
		})
	}
	volume, e := j.Execute(ctx, run, "docker", "volume", "create", prefix)
	if e != nil {
		t.Fatal(e)
	}
	network, e := j.Execute(ctx, run, "docker", "network", "create", "--internal", prefix)
	if e != nil {
		t.Fatal(e)
	}
	id, e := j.Execute(ctx, run, "docker", "create", "--name", prefix, "--network", network, "--mount", "type=volume,src="+volume+",dst=/probe", "--entrypoint", "/bin/sh", image, "-c", "sleep 120")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = j.Execute(ctx, run, "docker", "start", id); e != nil {
		t.Fatal(e)
	}
	if _, e = j.Execute(ctx, run, "docker", "create", "--name", prefix+"-overflow", image); e == nil {
		t.Fatal("capacity overcommit")
	}
	r, ok := j.Find("container", id)
	if !ok {
		t.Fatal("missing container journal")
	}
	forged := r
	forged.Labels = map[string]string{OwnershipLabel: "forged"}
	if e = Remove(ctx, run, forged); e == nil {
		t.Fatal("forged ownership deleted resource")
	}
	if _, e = run(ctx, "docker", "container", "inspect", id); e != nil {
		t.Fatal("ownership probe deleted container")
	}
	// A failed real Docker removal is injected at the transport boundary only.
	failRemove := func(c context.Context, r Resource) error {
		return Remove(c, func(c context.Context, n string, a ...string) (string, error) {
			if len(a) > 1 && a[0] == "container" && a[1] == "rm" {
				return "", errors.New("controlled transport failure")
			}
			return run(c, n, a...)
		}, r)
	}
	if e = j.Cleanup(r.Key, failRemove); e == nil || j.Pending() == 0 {
		t.Fatal("failure released reservation")
	}
	if _, e = j.Begin("container", prefix+"-blocked", nil); e == nil {
		t.Fatal("pending cleanup did not fence capacity")
	}
	// Reopen from disk as a restarted Worker, protecting the still-authorized live runtime.
	restarted, e := Open(filepath.Join(root, "journal"))
	if e != nil {
		t.Fatal(e)
	}
	restarted.SetContainerLimit(1)
	if e = restarted.Recover(func(r Resource) bool { return r.Kind == "container" }); e != nil {
		t.Fatal(e)
	}
	if _, e = run(ctx, "docker", "container", "inspect", id); e != nil {
		t.Fatal("reopen removed active container")
	}
	// Once authority terminates, recovery makes the failed deletion immediately retryable.
	if e = restarted.Recover(func(Resource) bool { return false }); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		_ = restarted.Retry(remove)
		_ = restarted.Recover(func(Resource) bool { return false })
	}
	if restarted.Pending() != 0 || len(restarted.rows) != 0 {
		t.Fatalf("resources retained after retry: %+v", restarted.rows)
	}
	for _, target := range []struct{ k, id string }{{"container", id}, {"volume", volume}, {"network", network}} {
		b, e := run(ctx, "docker", target.k, "inspect", target.id)
		if e == nil || !missing(b, e) {
			t.Fatalf("%s remains %s %v", target.k, b, e)
		}
	}
	// Creation succeeded before Bind: recovery must find only the exact ownership nonce.
	intent, e := restarted.Begin("container", prefix, nil)
	if e != nil {
		t.Fatal(e)
	}
	created, e := run(ctx, "docker", "create", "--name", prefix, "--label", OwnershipLabel+"="+intent.Key, "--entrypoint", "/bin/true", image)
	if e != nil {
		t.Fatal(e)
	}
	again, e := Open(filepath.Join(root, "journal"))
	if e != nil {
		t.Fatal(e)
	}
	if e = again.Recover(func(Resource) bool { return false }); e != nil {
		t.Fatal(e)
	}
	if e = again.Retry(remove); e != nil || again.Pending() != 0 {
		t.Fatalf("unbound create recovery %v", e)
	}
	b, e := run(ctx, "docker", "container", "inspect", created)
	if e == nil || !missing(b, e) {
		t.Fatal("unbound container remains")
	}
	// Missing is idempotent; cancelled caller context cannot suppress cleanup.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = again.Execute(cancelled, run, "docker", "rm", "-f", created); e == nil {
		t.Fatal("cancelled caller cannot establish missing-resource evidence")
	}
	if _, e = again.Execute(ctx, run, "docker", "rm", "-f", created); e != nil {
		t.Fatal(e)
	}
	// For a journaled target the cleanup context is independent of a cancelled handler.
	live, e := again.Execute(ctx, run, "docker", "create", "--name", prefix, "--entrypoint", "/bin/sh", image, "-c", "sleep 120")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = run(ctx, "docker", "start", live); e != nil {
		t.Fatal(e)
	}
	if _, e = again.Execute(cancelled, run, "docker", "rm", "-f", live); e != nil {
		t.Fatal("cancelled handler prevented independent cleanup", e)
	}
	b, e = run(ctx, "docker", "container", "inspect", live)
	if e == nil || !missing(b, e) || len(again.rows) != 0 {
		t.Fatal("cancelled-handler cleanup left residue")
	}
	t.Log("real container/volume/network deleted and absence verified; failed rm retained capacity; reopen/retry and create-before-bind recovery passed; forged ownership rejected")
}
