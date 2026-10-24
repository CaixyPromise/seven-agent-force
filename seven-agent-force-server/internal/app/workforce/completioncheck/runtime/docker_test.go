package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Executes real Git clone and POSIX shell, replacing only the unavailable daemon.
// Container mount isolation is asserted as configuration, not claimed as live.
func TestBaselineCloneAndCurrentActualShellOffline(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		body, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git fixture: %s", body)
		}
		return strings.TrimSpace(string(body))
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "value"), []byte("before\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "value")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@invalid", "commit", "-qm", "fixture")
	commit := git("rev-parse", "HEAD")
	input := domain.Input{Mode: "SCRIPT", Script: "mkdir -p dist .cache && printf 'built' > dist/output && printf 'cached' > .cache/state || exit 2; printf '1..1\\n'; if test \"$(cat value)\" = before; then printf 'ok 1\\n'; else printf 'not ok 1\\n'; exit 1; fi"}
	digest, _ := domain.Digest(input)
	journalParent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := resourcejournal.Open(filepath.Join(journalParent, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	cleanupFailure := false
	inputMount := ""
	created := false
	labels := map[string]string{}
	mounted := ""
	dependenciesMounted := false
	var output string
	containerID := strings.Repeat("d", 64)
	fake := func(ctx context.Context, name string, args ...string) (string, error) {
		if name != "docker" {
			t.Fatal(name)
		}
		switch args[0] {
		case "create":
			removed = false
			created = true
			labels = map[string]string{}
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "dst=/check/dependencies/node_modules,readonly") && strings.Contains(joined, "--dependencies /check/dependencies/node_modules") {
				dependenciesMounted = true
			}
			for _, required := range []string{"--network none", "--read-only", "--cap-drop ALL", "--security-opt no-new-privileges", "--entrypoint /usr/bin/env", " -i ", "dst=/check/source,readonly", "--tmpfs /workspace:rw", "uid=65534,gid=65534,mode=0700", "seven.attempt.id=1"} {
				if !strings.Contains(joined, required) {
					t.Fatalf("missing isolation %s", required)
				}
			}
			for n, arg := range args {
				if arg == "--mount" && strings.Contains(args[n+1], "dst=/check/input.json,") {
					inputMount = strings.TrimSuffix(strings.TrimPrefix(args[n+1], "type=bind,src="), ",dst=/check/input.json,readonly")
				}
				if arg == "--label" {
					k, v, _ := strings.Cut(args[n+1], "=")
					labels[k] = v
				}
				if arg == "--mount" && strings.Contains(args[n+1], "dst=/workspace") {
					t.Fatal("check workspace must be private tmpfs, not host bind")
				}
				if arg == "--mount" && strings.Contains(args[n+1], "src="+repo+",") {
					t.Fatal("Agent repository mounted in check container")
				}
				if arg == "--mount" && strings.Contains(args[n+1], "dst=/check/source,") {
					mounted = strings.TrimSuffix(strings.TrimPrefix(args[n+1], "type=bind,src="), ",dst=/check/source,readonly")
				}
			}
			return containerID, nil
		case "start":
			beforeDigest, err := SnapshotDigest(ctx, mounted)
			if err != nil {
				t.Fatal(err)
			}
			privateCopy := t.TempDir()
			if err := Snapshot(ctx, mounted, privateCopy); err != nil {
				t.Fatal(err)
			}
			result := Shell(ctx, input, privateCopy, time.Second)
			if data, err := os.ReadFile(filepath.Join(privateCopy, "dist/output")); err != nil || string(data) != "built" {
				t.Fatal("build output not writable")
			}
			afterDigest, err := SnapshotDigest(ctx, mounted)
			if err != nil || beforeDigest != afterDigest {
				t.Fatal("readonly source changed during check")
			}
			raw, _ := json.Marshal(result)
			output = string(raw)
			return containerID, nil
		case "wait":
			return "0", nil
		case "logs":
			return output, nil
		case "container":
			if args[1] == "rm" {
				if cleanupFailure {
					return "", errors.New("daemon unavailable")
				}
				removed = true
				return "", nil
			}
			if removed {
				return "", errors.New("No such container: container-id")
			}
			raw, _ := json.Marshal([]any{map[string]any{"Id": containerID, "Config": map[string]any{"Labels": labels}}})
			return string(raw), nil
		}
		t.Fatalf("unexpected %v", args)
		return "", nil
	}
	runner := Runner{Command: fake, Journal: journal}
	q := Request{Input: input, Digest: digest, Phase: "BASELINE", Repository: repo, BaseCommit: commit, Image: "sha256:" + strings.Repeat("a", 64), PrivateRoot: t.TempDir(), Labels: map[string]string{"seven.attempt.id": "1"}}
	before, err := runner.Run(context.Background(), q)
	if err != nil || before.Status != domain.Passed || mounted == repo || !removed || !created {
		t.Fatalf("baseline %+v %v", before, err)
	}
	if err = os.WriteFile(filepath.Join(repo, "value"), []byte("after\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(repo, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	q.Phase = "CURRENT"
	// A new check version still starts at the original source commit even
	// when the current workspace is already modified by a prior round.
	oldBaseline, baselineErr := runner.Run(context.Background(), qWithPhase(q, "BASELINE"))
	if baselineErr != nil || oldBaseline.Status != domain.Passed {
		t.Fatalf("baseline drifted to modified workspace %+v %v", oldBaseline, baselineErr)
	}
	after, err := runner.Run(context.Background(), q)
	if err != nil || after.Status != domain.Failed || mounted == repo || !removed || !dependenciesMounted {
		t.Fatalf("current %+v %v", after, err)
	}
	if result := domain.Compare(before, after, domain.ComparisonContext{}); result.Disposition != domain.Repair {
		t.Fatal(result)
	}
	data, _ := os.ReadFile(filepath.Join(repo, "value"))
	if string(data) != "after\n" {
		t.Fatal("user workspace repaired by runner")
	}
	if git("rev-parse", "HEAD") != commit {
		t.Fatal("history modified")
	}
	if _, err := os.Stat(filepath.Join(repo, "dist")); !os.IsNotExist(err) {
		t.Fatal("build output leaked into Agent source")
	}
	if before.WorkspaceDigest == "" || before.WorkspaceDigest == after.WorkspaceDigest {
		t.Fatal("Worker snapshot identity missing or ignored current changes")
	}
	input.ReferenceAssetIDs = []int64{9}
	q.Input = input
	q.InputDigest, _ = domain.Digest(input)
	q.References = []facade.Reference{{ReferenceAssetID: 9, Name: "requirements", ContentSHA256: strings.Repeat("b", 64), ObjectSHA256: strings.Repeat("c", 64), SizeBytes: 5}}
	q.Digest, _ = facade.FrozenDigest(input, q.References)
	bound, err := runner.Run(context.Background(), q)
	if err != nil || bound.Status != domain.Failed || bound.CheckDigest != q.Digest || bound.CheckDigest == q.InputDigest || bound.WorkspaceDigest != after.WorkspaceDigest {
		t.Fatalf("composed binding or optional-reference command execution failed %+v %v", bound, err)
	}
	q.References[0].SizeBytes++
	if _, err = runner.Run(context.Background(), q); err == nil {
		t.Fatal("reference metadata tamper accepted")
	}
	q.References[0].SizeBytes--
	cleanupFailure = true
	retained, err := runner.Run(context.Background(), q)
	if err != nil || retained.Status != domain.Failed {
		t.Fatalf("cleanup discarded execution result %+v %v", retained, err)
	}
	if journal.Pending() == 0 {
		t.Fatal("cleanup failure not durable")
	}
	if _, err = os.Stat(inputMount); err != nil {
		t.Fatal("mounted input deleted while container remains")
	}
}

func qWithPhase(q Request, phase string) Request { q.Phase = phase; return q }
