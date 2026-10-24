package checkpoint

import (
	"context"
	artifact "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSuspendFreezeRestoreRealBundleAndSession(t *testing.T) {
	t.Run("changed", func(t *testing.T) { testSuspendFreezeRestore(t, true) })
	t.Run("unchanged_repository_with_session_and_overlay", func(t *testing.T) { testSuspendFreezeRestore(t, false) })
}

func TestOperationSuspendManifestUsesOperationIdentityWithoutQuestion(t *testing.T) {
	root := t.TempDir()
	repo, state, spool := filepath.Join(root, "repo"), filepath.Join(root, "state"), filepath.Join(root, "spool")
	for _, dir := range []string{repo, filepath.Join(state, "codex")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "codex", "session.jsonl"), []byte("session\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repo, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	cmd = exec.Command("git", "-C", repo, "add", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	cmd = exec.Command("git", "-C", repo, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v %s", err, out)
	}
	baseCommit := strings.TrimSpace(string(mustCommand(t, "git", "-C", repo, "rev-parse", "HEAD")))
	in := Input{OperationID: 55, AdapterSessionGeneration: 2, Checkpoint: base.Input{TaskID: 1, TaskTurnID: 2, AttemptID: 3, Generation: 4, Decision: domain.PurposeInteractionSuspend, SessionDigest: "lineage", ExecutionSnapshotDigest: "snapshot", RepoRoot: repo, StateRoot: state, BaseCommit: baseCommit}, CurrentRound: 1, SuspendControlKeySHA256: strings.Repeat("b", 64), ExecutionBudget: ExecutionBudget{Version: 1, TaskID: 1, AttemptID: 3, Generation: 4, OwnerEpoch: 5, ActiveLimitMillis: 600000, AbsoluteDeadlineUnixMillis: time.Now().Add(time.Hour).UnixMilli()}}
	frozen, err := FreezeToSpool(context.Background(), in, spool)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(frozen.Manifest.Path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := DecodeManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.OperationID != 55 || manifest.InteractionID != 0 || manifest.CheckpointIdentity != domain.OperationCheckpointIdentity(1, 2, 3, 4, 55, 2, 1, "snapshot", strings.Repeat("b", 64)) {
		t.Fatalf("wrong operation manifest: %+v", manifest)
	}
}

func mustCommand(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v %s", name, err, out)
	}
	return out
}

func testSuspendFreezeRestore(t *testing.T, changed bool) {
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	state := filepath.Join(root, "state")
	spool := filepath.Join(root, "spool")
	for _, dir := range []string{repo, filepath.Join(state, "codex")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(repo, "file.txt"), "original\n")
	write(filepath.Join(repo, ".gitignore"), "cache.txt\n")
	git := func(args ...string) string {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "fixture")
	baseCommit := git("rev-parse", "HEAD")
	wantFile := "original\n"
	if changed {
		wantFile = "changed\n"
		write(filepath.Join(repo, "file.txt"), wantFile)
	} else {
		if _, err := artifact.ComputeResultIdentityForPlan(ctx, repo, baseCommit, nil, nil); err == nil || err.Error() != "Agent produced no repository changes" {
			t.Fatalf("RESULT no-diff rejection changed: %v", err)
		}
	}
	write(filepath.Join(repo, "cache.txt"), "recoverable ignored context\n")
	write(filepath.Join(state, "codex", "session.jsonl"), "native-private-session\n")
	write(filepath.Join(state, "codex", "secret.materialization.json"), "must not restore")
	input := Input{InteractionID: 5, AdapterSessionGeneration: 7, AutomaticRevisionCount: 2, Checkpoint: base.Input{TaskID: 1, TaskTurnID: 2, AttemptID: 3, Generation: 4, Decision: domain.PurposeInteractionSuspend, SessionDigest: "lineage", ExecutionSnapshotDigest: "snapshot", RepoRoot: repo, StateRoot: state, BaseCommit: baseCommit, ContinuationIncludes: []string{"cache.txt"}}}
	input.CurrentRound = 1
	input.SuspendControlKeySHA256 = strings.Repeat("a", 64)
	input.ExecutionBudget = ExecutionBudget{Version: 1, TaskID: 1, AttemptID: 3, Generation: 4, OwnerEpoch: 2, ActiveLimitMillis: 600000, ActiveElapsedMillis: 1200, InteractionWaitingMillis: 90000, AbsoluteDeadlineUnixMillis: time.Now().Add(-time.Hour).UnixMilli()}
	// Reservation occurs before native suspend creates a session descriptor.
	reservedIdentity := domain.InteractionCheckpointIdentity(1, 2, 3, 4, 5, input.AdapterSessionGeneration, input.CurrentRound, "snapshot", input.SuspendControlKeySHA256)
	frozen, err := FreezeToSpool(ctx, input, spool)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Identity != reservedIdentity {
		t.Fatal("native session descriptor changed the pre-stop reservation identity")
	}
	body, err := os.ReadFile(frozen.Manifest.Path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := DecodeManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Decision != "" || manifest.Purpose != domain.PurposeInteractionSuspend || git("rev-parse", "HEAD") != baseCommit {
		t.Fatal("purpose or source HEAD changed")
	}
	if !changed && (manifest.ResultCommit != baseCommit || manifest.ResultTree != git("rev-parse", baseCommit+"^{tree}")) {
		t.Fatal("unchanged suspension fabricated a result commit/tree")
	}
	outRepo, outState := filepath.Join(root, "restored-repo"), filepath.Join(root, "restored-state")
	restored, err := RestoreFromSpool(ctx, body, manifest, frozen, outRepo, outState)
	if err != nil {
		t.Fatal(err)
	}
	if restored.AdapterSessionGeneration != 7 || restored.AutomaticRevisionCount != 2 {
		t.Fatal("session/budget lost")
	}
	if restored.ExecutionBudget != input.ExecutionBudget {
		t.Fatal("trusted ledger changed across restore")
	}
	for path, want := range map[string]string{filepath.Join(outRepo, "file.txt"): wantFile, filepath.Join(outRepo, "cache.txt"): "recoverable ignored context\n", filepath.Join(outState, "codex", "session.jsonl"): "native-private-session\n"} {
		got, e := os.ReadFile(path)
		if e != nil || string(got) != want {
			t.Fatalf("restore %s %q %v", path, got, e)
		}
	}
	if _, err = os.Stat(filepath.Join(outState, "codex", "secret.materialization.json")); !os.IsNotExist(err) {
		t.Fatal("secret materialization restored")
	}
	wrong := manifest
	wrong.AutomaticRevisionCount = 0
	if _, err = RestoreFromSpool(ctx, body, wrong, frozen, filepath.Join(root, "badrepo"), filepath.Join(root, "badstate")); err == nil {
		t.Fatal("budget drift accepted")
	}
	write(frozen.Session.Path, "corrupt")
	if _, err = RestoreFromSpool(ctx, body, manifest, frozen, filepath.Join(root, "badrepo"), filepath.Join(root, "badstate")); err == nil {
		t.Fatal("corrupted session accepted")
	}
}

func TestSuspendPackerRejectsResultDecision(t *testing.T) {
	for _, decision := range []string{"PASS", "WAIT_USER", "MANUAL_REVIEW", ""} {
		if _, err := FreezeToSpool(context.Background(), Input{Checkpoint: base.Input{Decision: decision}}, t.TempDir()); err == nil {
			t.Fatalf("accepted %s", decision)
		}
	}
}
