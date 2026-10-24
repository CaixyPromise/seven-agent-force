package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenPersistentWorkspaceRejectsUnsafeRoots(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"", "relative/workspaces", string(filepath.Separator), file} {
		if _, err := OpenPersistentWorkspace(root); err == nil {
			t.Fatalf("OpenPersistentWorkspace(%q) error = nil", root)
		}
	}

	workspace, err := OpenPersistentWorkspace(filepath.Join(t.TempDir(), "persistent"))
	if err != nil {
		t.Fatalf("OpenPersistentWorkspace() error = %v", err)
	}
	if !filepath.IsAbs(workspace.Root()) {
		t.Fatalf("Root() = %q, want absolute path", workspace.Root())
	}
}

func TestOpenPersistentWorkspaceRejectsUnwritableRoot(t *testing.T) {
	// A privileged test process can bypass directory permission bits, so this
	// assertion is meaningful only for the normal non-root Worker identity.
	if os.Geteuid() == 0 {
		t.Skip("root can write a mode-0500 directory")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o700)
	if _, err := OpenPersistentWorkspace(root); err == nil {
		t.Fatal("OpenPersistentWorkspace accepted an unwritable root")
	}
}

func TestPersistentWorkspaceBuildsTaskScopedLayoutAndRejectsEscapes(t *testing.T) {
	t.Parallel()

	workspace, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := workspace.PrepareAttempt(101, 202)
	if err != nil {
		t.Fatalf("PrepareAttempt() error = %v", err)
	}
	wantAttempt := filepath.Join(workspace.Root(), "101", "attempts", "202")
	if prepared.AttemptRoot != wantAttempt || prepared.SpecPath != filepath.Join(workspace.Root(), "101", "input", "attempts", "202", "attempt.json") {
		t.Fatalf("prepared workspace = %#v, want task-scoped attempt root %q", prepared, wantAttempt)
	}
	for _, path := range []string{prepared.InputRoot, prepared.AttemptRoot, prepared.Output, prepared.State, prepared.Runtime, prepared.Temporary} {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("prepared path %q is not a directory: %v", path, statErr)
		}
	}
	if other, otherErr := workspace.Attempt(102, 202); otherErr != nil || other.AttemptRoot == prepared.AttemptRoot {
		t.Fatalf("cross-task attempt isolation = %#v, %v", other, otherErr)
	}
	for _, ids := range [][2]int64{{0, 202}, {-1, 202}, {101, 0}, {101, -1}} {
		if _, err := workspace.Attempt(ids[0], ids[1]); err == nil {
			t.Fatalf("Attempt(%d, %d) error = nil", ids[0], ids[1])
		}
	}

	escapeRoot := filepath.Join(workspace.Root(), "303")
	if err := os.Symlink(t.TempDir(), escapeRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Attempt(303, 404); err == nil {
		t.Fatal("Attempt() accepted a task-root symlink")
	}

	review, err := workspace.Review(101, 505)
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if review.InputReport != filepath.Join(workspace.Root(), "101", "input", "review", "505", "review-report.json") || review.InputPath != filepath.Join(workspace.Root(), "101", "reviews", "505", "input.json") {
		t.Fatalf("review layout = %#v", review)
	}
	firstReviewAttempt, err := workspace.PrepareReviewAttempt(101, 505, 1)
	if err != nil {
		t.Fatalf("PrepareReviewAttempt(first) error = %v", err)
	}
	secondReviewAttempt, err := workspace.PrepareReviewAttempt(101, 505, 2)
	if err != nil {
		t.Fatalf("PrepareReviewAttempt(second) error = %v", err)
	}
	if firstReviewAttempt.Root != filepath.Join(workspace.Root(), "101", "reviews", "505", "attempts", "1") || secondReviewAttempt.Root != filepath.Join(workspace.Root(), "101", "reviews", "505", "attempts", "2") || firstReviewAttempt.Root == secondReviewAttempt.Root || firstReviewAttempt.InputRoot == secondReviewAttempt.InputRoot {
		t.Fatalf("review attempts were not owner-epoch isolated: first=%#v second=%#v", firstReviewAttempt, secondReviewAttempt)
	}
	for _, path := range []string{firstReviewAttempt.InputRoot, firstReviewAttempt.Root, firstReviewAttempt.RuntimeRoot, firstReviewAttempt.JournalRoot, secondReviewAttempt.InputRoot, secondReviewAttempt.Root, secondReviewAttempt.RuntimeRoot, secondReviewAttempt.JournalRoot} {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("prepared review attempt path %q is not a directory: %v", path, statErr)
		}
	}
	result, err := workspace.Result(101, 606, 707)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if result.Root != filepath.Join(workspace.Root(), "101", "results", "606", "707") || result.ManifestPath != filepath.Join(result.Root, "result-manifest.json") || result.ReadyPath != filepath.Join(result.Root, "READY") {
		t.Fatalf("result layout = %#v", result)
	}
}

func TestPrepareAttemptKeepsAdapterStatePrivate(t *testing.T) {
	for _, mode := range []os.FileMode{0, 0o755, 0o777, 0o700} {
		t.Run(mode.String(), func(t *testing.T) {
			storage, err := OpenPersistentWorkspace(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			paths, err := storage.Attempt(101, 202)
			if err != nil {
				t.Fatal(err)
			}
			if mode != 0 {
				if err := os.MkdirAll(paths.State, mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(paths.State, mode); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(paths.State, "session.json"), []byte("private fixture"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			sibling, err := storage.Attempt(101, 203)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(sibling.State, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(sibling.State, 0o755); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				prepared, err := storage.PrepareAttempt(101, 202)
				if err != nil {
					t.Fatal(err)
				}
				// Supervisor's existing MkdirAll must retain the Worker contract.
				if err := os.MkdirAll(prepared.State, 0o700); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(prepared.State)
				if err != nil || info.Mode().Perm() != 0o700 {
					t.Fatalf("Adapter state must be 0700: info=%v err=%v", info, err)
				}
			}
			if mode != 0 {
				body, err := os.ReadFile(filepath.Join(paths.State, "session.json"))
				if err != nil || string(body) != "private fixture" {
					t.Fatal("session contents changed")
				}
			}
			info, err := os.Stat(sibling.State)
			if err != nil || info.Mode().Perm() != 0o755 {
				t.Fatal("sibling Attempt permissions changed")
			}
		})
	}
}

func TestPrepareAttemptRejectsLinkedStateWithoutChmodTarget(t *testing.T) {
	storage, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths, err := storage.Attempt(101, 202)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.AttemptRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, paths.State); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.PrepareAttempt(101, 202); err == nil {
		t.Fatal("linked state accepted")
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatal("linked target permissions changed")
	}
}

func TestPersistentWorkspaceScanClassifiesReadyPartialCorruptAndUnknownSchemas(t *testing.T) {
	t.Parallel()

	workspace, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	makeResult := func(taskID, turnID, runID int64, mutate func(root string, manifest map[string]any, ready map[string]any)) string {
		t.Helper()
		paths, pathErr := workspace.Result(taskID, turnID, runID)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		if err := os.MkdirAll(filepath.Dir(paths.Root), 0o700); err != nil {
			t.Fatal(err)
		}
		staging, err := os.MkdirTemp(filepath.Dir(paths.Root), ".result-staging-")
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte("trusted-result")
		sum := sha256.Sum256(payload)
		if err = os.WriteFile(filepath.Join(staging, "payload"), payload, 0o600); err != nil {
			t.Fatal(err)
		}
		manifest := map[string]any{
			"schemaVersion":     1,
			"taskId":            "" + decimal(taskID),
			"taskTurnId":        decimal(turnID),
			"finalizationRunId": decimal(runID),
			"attemptId":         "808",
			"resultCommit":      "0123456789abcdef0123456789abcdef01234567",
			"resultTree":        "abcdef0123456789abcdef0123456789abcdef01",
			"selectionDigest":   hex.EncodeToString(sum[:]),
			"artifacts": []map[string]any{{
				"path": "payload", "sha256": hex.EncodeToString(sum[:]), "sizeBytes": len(payload),
			}},
		}
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		ready := map[string]any{"schemaVersion": 1, "manifest": "result-manifest.json", "manifestDigest": digestBytes(body)}
		if mutate != nil {
			mutate(staging, manifest, ready)
			body, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err = os.WriteFile(filepath.Join(staging, "result-manifest.json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
		readyBody, err := json.Marshal(ready)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(staging, "READY"), readyBody, 0o600); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(staging, paths.Root); err != nil {
			t.Fatal(err)
		}
		return paths.Root
	}

	makeResult(11, 21, 31, nil)
	partial, err := workspace.Result(12, 22, 32)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(partial.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	makeResult(13, 23, 33, func(_ string, _ map[string]any, ready map[string]any) { ready["manifestDigest"] = "not-a-digest" })
	makeResult(14, 24, 34, func(_ string, _ map[string]any, ready map[string]any) { ready["schemaVersion"] = 2 })

	records, err := workspace.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	got := map[int64]ScanState{}
	for _, record := range records {
		got[record.TaskID] = record.State
	}
	if got[11] != ScanReady || got[12] != ScanPartial || got[13] != ScanCorrupt || got[14] != ScanUnknownSchema {
		t.Fatalf("Scan() states = %#v", got)
	}
}

func TestPersistentWorkspaceGarbageCollectFailsClosedAndNeverFollowsSymlinks(t *testing.T) {
	t.Parallel()

	workspace, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []int64{71, 72, 73, 74} {
		if err := os.MkdirAll(filepath.Join(workspace.Root(), decimal(taskID), "attempts"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(filepath.Join(workspace.Root(), "74")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(workspace.Root(), "74")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	authority := gcAuthorityFunc(func(_ context.Context, taskID int64) (GCEligibility, error) {
		switch taskID {
		case 71:
			return GCEligibility{TaskID: taskID, Terminal: true, TerminalAt: old, PortableStateAvailable: true}, nil
		case 72:
			return GCEligibility{TaskID: taskID, Terminal: true, TerminalAt: old, ActiveReferences: true, PortableStateAvailable: true}, nil
		case 73:
			return GCEligibility{}, errors.New("server unavailable")
		default:
			return GCEligibility{TaskID: taskID, Terminal: true, TerminalAt: old, PortableStateAvailable: true}, nil
		}
	})
	var audits []GCAudit
	report, err := workspace.GarbageCollect(context.Background(), authority, func(audit GCAudit) { audits = append(audits, audit) })
	if err != nil {
		t.Fatalf("GarbageCollect() error = %v", err)
	}
	if report.Deleted != 1 {
		t.Fatalf("GarbageCollect() deleted = %d, want 1; report=%#v", report.Deleted, report)
	}
	if _, err = os.Lstat(filepath.Join(workspace.Root(), "71")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("eligible task directory still exists: %v", err)
	}
	for _, taskID := range []string{"72", "73", "74"} {
		if _, statErr := os.Lstat(filepath.Join(workspace.Root(), taskID)); statErr != nil {
			t.Fatalf("protected task %s missing: %v", taskID, statErr)
		}
	}
	if len(audits) < 4 {
		t.Fatalf("GC audits = %#v, want a safe fact per candidate", audits)
	}
	second, err := workspace.GarbageCollect(context.Background(), authority, nil)
	if err != nil || second.Deleted != 0 {
		t.Fatalf("repeated GarbageCollect() = %#v, %v", second, err)
	}
}

func TestPersistentWorkspaceGarbageCollectIsConcurrentAndRepeatSafe(t *testing.T) {
	workspace, err := OpenPersistentWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(workspace.Root(), "81", "attempts"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	authority := gcAuthorityFunc(func(_ context.Context, taskID int64) (GCEligibility, error) {
		return GCEligibility{TaskID: taskID, Terminal: true, TerminalAt: workspace.now().Add(-8 * 24 * time.Hour), PortableStateAvailable: true}, nil
	})
	reports := make(chan GCReport, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			report, collectErr := workspace.GarbageCollect(context.Background(), authority, nil)
			reports <- report
			errs <- collectErr
		}()
	}
	deleted := 0
	for range 2 {
		if collectErr := <-errs; collectErr != nil {
			t.Fatalf("concurrent GarbageCollect() error = %v", collectErr)
		}
		deleted += (<-reports).Deleted
	}
	if deleted != 1 {
		t.Fatalf("concurrent GarbageCollect() deleted = %d, want 1", deleted)
	}
	if _, statErr := os.Lstat(filepath.Join(workspace.Root(), "81")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("collected task directory still exists: %v", statErr)
	}
	repeat, err := workspace.GarbageCollect(context.Background(), authority, nil)
	if err != nil || repeat.Deleted != 0 {
		t.Fatalf("repeat GarbageCollect() = %#v, %v", repeat, err)
	}
}

type gcAuthorityFunc func(context.Context, int64) (GCEligibility, error)

func (f gcAuthorityFunc) Eligibility(ctx context.Context, taskID int64) (GCEligibility, error) {
	return f(ctx, taskID)
}
