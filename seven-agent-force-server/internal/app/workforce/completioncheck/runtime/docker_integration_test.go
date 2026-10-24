package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

// Explicit opt-in runs the production Runner and current image CLI against Docker.
func TestDockerCompletionCheckIsolationAndCleanup(t *testing.T) {
	image := os.Getenv("BATCH1_CHECK_DOCKER_IMAGE")
	if image == "" {
		t.Skip("BATCH1_CHECK_DOCKER_IMAGE required")
	}
	if !imageRE.MatchString(image) {
		t.Fatal("exact image digest required")
	}
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	if err = os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("git: %s %v", b, e)
		}
		return strings.TrimSpace(string(b))
	}
	git("init", "-q")
	if err = os.WriteFile(filepath.Join(repo, "value"), []byte("broken\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "value")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@invalid", "commit", "-qm", "baseline")
	commit := git("rev-parse", "HEAD")
	script := `set -eu
test "$(id -u)" = 65534 || exit 31
test ! -S /var/run/docker.sock || exit 32
test -z "${WORKER_TOKEN:-}${OPENAI_API_KEY:-}${PROVIDER_API_KEY:-}" || exit 33
if (printf forged > /check/input.json) 2>/dev/null; then exit 20; fi
if (printf forged > /check/source/value) 2>/dev/null; then exit 21; fi
if (printf forged > /root/probe) 2>/dev/null; then exit 22; fi
test "$(awk 'NR>1 {n++} END {print n+0}' /proc/net/route)" = 0 || exit 34
if wget -T 2 -O /dev/null http://1.1.1.1 >/dev/null 2>&1; then exit 35; fi
mkdir -p dist .cache
printf built > dist/output
printf changed > value.local
test "$(cat value)" = fixed
`
	input := domain.Input{Mode: "SCRIPT", Script: script}
	digest, _ := domain.Digest(input)
	j, err := resourcejournal.Open(filepath.Join(root, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	j.SetContainerLimit(1)
	private := filepath.Join(root, "private")
	if err = os.Mkdir(private, 0755); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Journal: j}
	q := Request{Input: input, Digest: digest, Phase: "BASELINE", Repository: repo, BaseCommit: commit, Image: image, PrivateRoot: private, Timeout: 15 * time.Second, Labels: map[string]string{"seven.batch1.test": "completion-isolation"}}
	before, e := runner.Run(ctx, q)
	if e != nil || before.Status != domain.Failed {
		t.Fatalf("baseline %+v %v", before, e)
	}
	if err = os.WriteFile(filepath.Join(repo, "value"), []byte("fixed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sourceDigest, e := SnapshotDigest(ctx, repo)
	if e != nil {
		t.Fatal(e)
	}
	q.Phase = "CURRENT"
	after, e := runner.Run(ctx, q)
	if e != nil || after.Status != domain.Passed {
		t.Fatalf("current %+v exit=%v %v", after, *after.ExitCode, e)
	}
	finalDigest, e := SnapshotDigest(ctx, repo)
	if e != nil || sourceDigest != finalDigest {
		t.Fatal("source modified by check")
	}
	q.Phase = "BASELINE"
	again, e := runner.Run(ctx, q)
	if e != nil || again.Status != domain.Failed || before.WorkspaceDigest != again.WorkspaceDigest {
		t.Fatalf("original baseline changed %+v %v", again, e)
	}
	if git("rev-parse", "HEAD") != commit {
		t.Fatal("commit changed")
	}
	if before.WorkspaceDigest == after.WorkspaceDigest {
		t.Fatal("current workspace not observed")
	}
	entries, e := os.ReadDir(private)
	if e != nil || len(entries) != 0 {
		t.Fatalf("temporary resources remain %v %v", entries, e)
	}
	records, e := os.ReadDir(filepath.Join(root, "journal"))
	if e != nil || len(records) != 0 || j.Pending() != 0 {
		t.Fatal("journal not released")
	}
	b, e := exec.Command("docker", "ps", "-aq", "--filter", "label=seven.batch1.test=completion-isolation").CombinedOutput()
	if e != nil || strings.TrimSpace(string(b)) != "" {
		t.Fatalf("containers remain %s %v", b, e)
	}
	t.Log("real baseline FAILED -> current PASSED; readonly input/source, writable scratch, no route/public egress/secrets/socket, original baseline unchanged; containers and directories absent")
}
