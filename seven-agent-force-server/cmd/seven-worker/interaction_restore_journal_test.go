package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
)

func coldJournalFixture(t *testing.T) (*privateInteractionBudgets, coldRestoreIdentity, runtime.Workspace, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(dir, "private")
	if err = os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := openPrivateInteractionBudgets(private, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.root.Close() })
	w := runtime.Workspace{Repo: filepath.Join(dir, "repo"), State: filepath.Join(dir, "state")}
	for _, p := range []string{w.Repo, w.State} {
		if err = os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	id := coldRestoreIdentity{BudgetIdentity: runtime.BudgetIdentity{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4}, RestoreRunID: 5, CheckpointID: 6, BindingSHA256: strings.Repeat("a", 64), ResultSHA256: strings.Repeat("b", 64)}
	return s, id, w, private
}

func TestColdRestoreJournalResponseLossRestart(t *testing.T) {
	s, id, w, private := coldJournalFixture(t)
	installed := 0
	install := func() error {
		installed++
		if err := os.WriteFile(filepath.Join(w.Repo, "result"), []byte("result"), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(w.State, "session"), []byte("native-session"), 0600)
	}
	err := recoverColdRestore(context.Background(), s, id, w, install, func() error { return errColdRestoreConfirmationPending })
	if !errors.Is(err, errColdRestoreConfirmationPending) {
		t.Fatalf("lost response: %v", err)
	}
	s.root.Close()
	reopened, err := openPrivateInteractionBudgets(private, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.root.Close()
	// PrepareAttempt legitimately changes the root mode before retry.
	if err = os.Chmod(w.State, 0755); err != nil {
		t.Fatal(err)
	}
	confirmed := 0
	err = recoverColdRestore(context.Background(), reopened, id, w, install, func() error { confirmed++; return nil })
	if err != nil || installed != 1 || confirmed != 1 {
		t.Fatalf("retry err=%v installs=%d confirms=%d", err, installed, confirmed)
	}
	body, err := os.ReadFile(filepath.Join(w.State, "session"))
	if err != nil || string(body) != "native-session" {
		t.Fatal("state not preserved", err)
	}
}

func TestColdRestoreJournalRejectsChangedEvidence(t *testing.T) {
	for _, kind := range []string{"binding", "owner", "run", "contents", "journal", "symlink", "unknown-partial"} {
		t.Run(kind, func(t *testing.T) {
			s, id, w, private := coldJournalFixture(t)
			install := func() error { return os.WriteFile(filepath.Join(w.State, "session"), []byte("original"), 0600) }
			if kind != "unknown-partial" {
				if err := recoverColdRestore(context.Background(), s, id, w, install, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			switch kind {
			case "binding":
				id.BindingSHA256 = strings.Repeat("c", 64)
			case "owner":
				id.OwnerEpoch++
			case "run":
				id.RestoreRunID++
			case "contents", "unknown-partial":
				err = os.WriteFile(filepath.Join(w.State, "session"), []byte("changed"), 0600)
			case "journal":
				err = os.WriteFile(filepath.Join(private, coldRestoreRecordName(id)), []byte(`{}`), 0600)
			case "symlink":
				err = os.Symlink("session", filepath.Join(w.State, "link"))
			}
			if err != nil {
				t.Fatal(err)
			}
			called := false
			err = recoverColdRestore(context.Background(), s, id, w, func() error { called = true; return nil }, func() error { called = true; return nil })
			if !errors.Is(err, errColdRestoreRecovery) || called {
				t.Fatalf("accepted changed evidence: %v called=%v", err, called)
			}
			if _, err = os.Stat(filepath.Join(w.State, "session")); err != nil {
				t.Fatal("existing state removed", err)
			}
		})
	}
}

func TestColdRestoreConfirmationSentinel(t *testing.T) {
	for _, tc := range []struct {
		name                string
		first, second, want error
		calls               int
	}{
		{"lost-then-readback", io.ErrUnexpectedEOF, nil, nil, 2},
		{"still-pending", io.ErrUnexpectedEOF, io.EOF, errColdRestoreConfirmationPending, 2},
		{"authority-denied", &control.APIError{StatusCode: 409}, nil, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := confirmColdRestore(context.Background(), func() error {
				calls++
				if calls == 1 {
					return tc.first
				}
				return tc.second
			})
			want := tc.want
			if tc.name == "authority-denied" {
				want = tc.first
			}
			if !errors.Is(err, want) || calls != tc.calls {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}
