package resourcejournal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTest(t *testing.T) *Journal {
	t.Helper()
	j, e := Open(filepath.Join(t.TempDir(), "private"))
	if e != nil {
		t.Fatal(e)
	}
	return j
}

func TestCancellationPersistsFailureAndRestartRetry(t *testing.T) {
	j := openTest(t)
	r, e := j.Begin("container", "owned", map[string]string{"seven.attempt.id": "42"})
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Bind(r.Key, "exact-id"); e != nil {
		t.Fatal(e)
	}
	if e = j.Cleanup(r.Key, func(ctx context.Context, r Resource) error {
		deadline, ok := ctx.Deadline()
		if ctx.Err() != nil || !ok || time.Until(deadline) > Timeout || time.Until(deadline) < Timeout-time.Second {
			t.Fatal("cleanup context is not independently bounded")
		}
		return errors.New("secret Docker stderr")
	}); e == nil {
		t.Fatal("failure lost")
	}
	if j.Pending() != 1 {
		t.Fatal("capacity released")
	}
	if _, e = j.Begin("container", "other", nil); e == nil {
		t.Fatal("pending cleanup allowed create")
	}
	reopened, e := Open(j.root)
	if e != nil {
		t.Fatal(e)
	}
	row, ok := reopened.Find("container", "exact-id")
	if !ok || row.Failures != 1 || row.LastError != "RESOURCE_CLEANUP_FAILED" {
		t.Fatalf("bad recovery: %+v", row)
	}
	body, _ := os.ReadFile(filepath.Join(j.root, row.Key+".json"))
	if strings.Contains(string(body), "secret") {
		t.Fatal("unsafe error persisted")
	}
	if e = reopened.Recover(func(Resource) bool { return false }); e != nil {
		t.Fatal(e)
	}
	if e = reopened.Retry(func(context.Context, Resource) error { return nil }); e != nil || reopened.Pending() != 0 {
		t.Fatal("cleanup not released", e)
	}
	if _, ok = reopened.Find("container", "exact-id"); ok {
		t.Fatal("record retained after verified cleanup")
	}
}

func TestRetryCapAndProtectedRecovery(t *testing.T) {
	if RetryDelay(1) != time.Second || RetryDelay(1000) != 5*time.Minute {
		t.Fatal("unbounded retry")
	}
	j := openTest(t)
	r, e := j.Begin("container", "live", map[string]string{"seven.attempt.id": "42"})
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Recover(func(row Resource) bool { return row.Key == r.Key }); e != nil {
		t.Fatal(e)
	}
	if e = j.Retry(func(context.Context, Resource) error { t.Fatal("removed recoverable live runtime"); return nil }); e != nil {
		t.Fatal(e)
	}
}

func TestCreateIntentPrecedesDockerAndOwnershipIsExact(t *testing.T) {
	immutableID := strings.Repeat("a", 64)
	j := openTest(t)
	j.SetContainerLimit(1)
	var labels map[string]string
	removed := false
	run := func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "create" {
			r, ok := j.Find("container", "owned")
			if !ok {
				t.Fatal("create before journal")
			}
			labels = r.Labels
			if !strings.Contains(strings.Join(args, " "), OwnershipLabel+"="+r.Key) {
				t.Fatal("ownership label absent")
			}
			return immutableID, nil
		}
		if len(args) > 1 && args[1] == "inspect" {
			if removed {
				return "", errors.New("Error: No such container: " + immutableID)
			}
			body, _ := json.Marshal([]any{map[string]any{"Id": immutableID, "Config": map[string]any{"Labels": labels}}})
			return string(body), nil
		}
		if len(args) > 1 && args[1] == "rm" {
			if args[len(args)-1] != immutableID {
				t.Fatal("removed by name")
			}
			removed = true
			return "", nil
		}
		t.Fatalf("unexpected %v", args)
		return "", nil
	}
	if _, e := j.Execute(context.Background(), run, "docker", "create", "--name", "owned", "sha256:image"); e != nil {
		t.Fatal(e)
	}
	if _, e := j.Begin("container", "second", nil); e == nil {
		t.Fatal("container ceiling bypassed")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := j.Execute(cancelled, run, "docker", "rm", "-f", immutableID); e != nil {
		t.Fatal(e)
	}
	if !removed || j.Pending() != 0 {
		t.Fatal("not cleaned")
	}
}

func TestRemoveFailsClosedOnIdentityDriftAndDaemonFailure(t *testing.T) {
	for _, mode := range []string{"wrong-id", "wrong-owner", "daemon-offline", "absence-unconfirmed"} {
		t.Run(mode, func(t *testing.T) {
			r := Resource{Kind: "container", Name: "owned", ID: "exact", Labels: map[string]string{OwnershipLabel: "owner"}}
			removes := 0
			run := func(ctx context.Context, name string, args ...string) (string, error) {
				if args[1] == "rm" {
					removes++
					return "", nil
				}
				if mode == "daemon-offline" {
					return "", errors.New("cannot connect to daemon")
				}
				id, owner := "exact", "owner"
				if mode == "wrong-id" {
					id = "replacement"
				}
				if mode == "wrong-owner" {
					owner = "other"
				}
				body, _ := json.Marshal([]any{map[string]any{"Id": id, "Config": map[string]any{"Labels": map[string]string{OwnershipLabel: owner}}}})
				return string(body), nil
			}
			if e := Remove(context.Background(), run, r); e == nil {
				t.Fatal("unverified success")
			}
			if mode != "absence-unconfirmed" && removes != 0 {
				t.Fatal("deleted unrelated/unknown resource")
			}
		})
	}
}

func TestUnknownTargetIsNeverDeleted(t *testing.T) {
	j := openTest(t)
	run := func(ctx context.Context, name string, args ...string) (string, error) {
		if args[1] != "inspect" {
			t.Fatal("unrelated delete")
		}
		return "[]", nil
	}
	if _, e := j.Execute(context.Background(), run, "docker", "rm", "-f", "unrelated"); e == nil {
		t.Fatal("accepted unknown")
	}
}

func TestRejectSymlinkJournal(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if e := os.Mkdir(target, 0700); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "link")
	if e := os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(link); e == nil {
		t.Fatal("symlink root accepted")
	}
}
