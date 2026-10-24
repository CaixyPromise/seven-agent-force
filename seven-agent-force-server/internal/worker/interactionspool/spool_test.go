package interactionspool

import (
	"bytes"
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var testIdentity = Identity{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, CurrentRound: "round:1", SessionGeneration: 1}
var testLimits = Limits{MaxEntries: 8, MaxBytes: 64 << 10, MaxFrameBytes: 1024}

func makeFIFO(path string) error { return unix.Mkfifo(path, 0600) }
func spoolPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(p, "spool")
}
func openTest(t *testing.T, path string, limits Limits, verify ServerAckVerifier) *Spool {
	t.Helper()
	s, err := Open(path, limits, verify)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func putTest(t *testing.T, s *Spool, key Key, frame []byte) Receipt {
	t.Helper()
	r, err := s.Put(context.Background(), key, frame)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRestartImmutableOpaqueAndServerAck(t *testing.T) {
	ctx := context.Background()
	path := spoolPath(t)
	var expected Receipt
	verified := 0
	verify := func(_ context.Context, r Receipt, proof []byte) error {
		verified++
		if r != expected || !bytes.Equal(proof, []byte("authenticated-server-http-response")) {
			return errors.New("private server response")
		}
		return nil
	}
	s := openTest(t, path, testLimits, verify)
	key := Key{Identity: testIdentity, Occurrence: "1"}
	frame := []byte("\x00opaque bytes {\"method\":\"control.ack\"}\n")
	expected = putTest(t, s, key, frame)
	frame[0] = 'x'
	if _, err := s.Put(ctx, key, frame); !errors.Is(err, ErrConflict) {
		t.Fatalf("mutable occurrence: %v", err)
	}
	if verified != 0 {
		t.Fatal("container ack invoked verifier")
	}
	_ = s.Close()
	s = openTest(t, path, testLimits, verify)
	entries, err := s.Replay(ctx, testIdentity)
	if err != nil || len(entries) != 1 || entries[0].Frame[0] != 0 {
		t.Fatalf("restart lost opaque frame: %v %v", entries, err)
	}
	if r := putTest(t, s, key, entries[0].Frame); r != expected {
		t.Fatal("duplicate changed receipt")
	}
	entries[0].Frame[0] = 'y'
	if err := s.AcknowledgeServer(ctx, expected, []byte("container-control.ack")); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("forged ack accepted: %v", err)
	}
	wrong := expected
	wrong.SHA256 = strings.Repeat("0", 64)
	if err := s.AcknowledgeServer(ctx, wrong, []byte("authenticated-server-http-response")); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong digest accepted: %v", err)
	}
	if err := s.AcknowledgeServer(ctx, expected, []byte("authenticated-server-http-response")); err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeServer(ctx, expected, []byte("authenticated-server-http-response")); err != nil {
		t.Fatal("duplicate ack", err)
	}
	_ = s.Close()
	s = openTest(t, path, testLimits, verify)
	entries, err = s.Replay(ctx, testIdentity)
	if err != nil || len(entries) != 0 {
		t.Fatalf("ack did not survive restart: %v", err)
	}
}

func TestIdentityIsolationBoundsAndNoVerifier(t *testing.T) {
	ctx := context.Background()
	limits := testLimits
	limits.MaxEntries = 2
	s := openTest(t, spoolPath(t), limits, nil)
	key := Key{Identity: testIdentity, Occurrence: "same"}
	r := putTest(t, s, key, []byte("a"))
	other := key
	other.Identity.CurrentRound = "round:2"
	putTest(t, s, other, []byte("b"))
	third := key
	third.Occurrence = "third"
	if _, err := s.Put(ctx, third, []byte("c")); !errors.Is(err, ErrFull) {
		t.Fatalf("overflow: %v", err)
	}
	putTest(t, s, key, []byte("a")) // idempotent retry still succeeds at capacity
	if _, err := s.Put(ctx, key, make([]byte, limits.MaxFrameBytes+1)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.AcknowledgeServer(ctx, r, []byte("server")); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("missing verifier: %v", err)
	}
	for _, identity := range []Identity{testIdentity, other.Identity} {
		entries, err := s.Replay(ctx, identity)
		if err != nil || len(entries) != 1 {
			t.Fatalf("cross identity: %v", err)
		}
	}
	for _, bad := range []string{"", "../../escape", "with space", strings.Repeat("a", 129), "非ASCII"} {
		k := key
		k.Identity.CurrentRound = bad
		if _, err := s.Put(ctx, k, []byte("a")); !errors.Is(err, ErrInvalid) {
			t.Fatalf("round %q: %v", bad, err)
		}
	}
	byteLimits := testLimits
	byteLimits.MaxBytes = 1
	b := openTest(t, spoolPath(t), byteLimits, nil)
	if _, err := b.Put(ctx, key, []byte("a")); !errors.Is(err, ErrFull) {
		t.Fatalf("encoded byte bound: %v", err)
	}
}

func TestPersistenceFaultBoundaries(t *testing.T) {
	for _, fault := range []string{"file-sync", "rename", "directory-sync"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			path := spoolPath(t)
			s := openTest(t, path, testLimits, nil)
			privateErr := errors.New("secret path/provider body")
			switch fault {
			case "file-sync":
				s.syncFile = func(*os.File) error { return privateErr }
			case "rename":
				s.rename = func(string, string) error { return privateErr }
			case "directory-sync":
				s.syncDirectory = func() error { return privateErr }
			}
			key := Key{Identity: testIdentity, Occurrence: "1"}
			r, err := s.Put(ctx, key, []byte("replay-me"))
			if !errors.Is(err, ErrIO) || r != (Receipt{}) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("uncertain persistence returned receipt/raw error: %v %v", r, err)
			}
			if _, err = s.Put(ctx, key, []byte("replay-me")); !errors.Is(err, ErrIO) {
				t.Fatal("poisoned handle reused", err)
			}
			_ = s.Close()
			s = openTest(t, path, testLimits, nil)
			entries, err := s.Replay(ctx, testIdentity)
			want := 0
			if fault == "directory-sync" {
				want = 1
			}
			if err != nil || len(entries) != want {
				t.Fatalf("recovery %s: %d %v", fault, len(entries), err)
			}
			files, err := os.ReadDir(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range files {
				if strings.HasPrefix(f.Name(), ".pending-") {
					t.Fatal("crash temp remained")
				}
			}
		})
	}
}

func TestPathAndFileGuards(t *testing.T) {
	ctx := context.Background()
	t.Run("directory symlink and traversal", func(t *testing.T) {
		p := spoolPath(t)
		base := filepath.Dir(p)
		if err := os.Symlink(base, p); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{p, p + "/child", base + "/../escape", "relative", "/"} {
			if s, err := Open(path, testLimits, nil); err == nil {
				_ = s.Close()
				t.Fatalf("unsafe root accepted %q", path)
			}
		}
	})
	t.Run("permissions and lock", func(t *testing.T) {
		p := spoolPath(t)
		s := openTest(t, p, testLimits, nil)
		if duplicate, err := Open(p, testLimits, nil); err == nil {
			_ = duplicate.Close()
			t.Fatal("second writer admitted")
		}
		_ = s.Close()
		if err := os.Chmod(p, 0755); err != nil {
			t.Fatal(err)
		}
		if s, err := Open(p, testLimits, nil); err == nil {
			_ = s.Close()
			t.Fatal("shared spool admitted")
		}
	})
	for _, kind := range []string{"symlink", "hardlink", "fifo", "corrupt", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			p := spoolPath(t)
			s := openTest(t, p, testLimits, nil)
			key := Key{Identity: testIdentity, Occurrence: "1"}
			putTest(t, s, key, []byte("safe"))
			_ = s.Close()
			name := filepath.Join(p, nameFor(key))
			target := filepath.Join(filepath.Dir(p), "outside")
			if err := os.WriteFile(target, []byte("private"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(target, name)
			case "hardlink":
				err = os.Link(target, name)
			case "fifo":
				err = makeFIFO(name)
			case "corrupt":
				err = os.WriteFile(name, []byte("private broken body"), 0600)
			case "oversized":
				err = os.WriteFile(name, make([]byte, 10000), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if opened, err := Open(p, testLimits, nil); err == nil {
				_ = opened.Close()
				t.Fatal("unsafe entry accepted")
			} else if strings.Contains(err.Error(), target) || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe error", err)
			}
			body, err := os.ReadFile(target)
			if err != nil || string(body) != "private" {
				t.Fatal("external target altered")
			}
		})
	}
	t.Run("pinned root", func(t *testing.T) {
		p := spoolPath(t)
		s := openTest(t, p, testLimits, nil)
		moved := p + "-moved"
		if err := os.Rename(p, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
		putTest(t, s, Key{Identity: testIdentity, Occurrence: "1"}, []byte("pinned"))
		files, _ := os.ReadDir(p)
		if len(files) != 0 {
			t.Fatal("followed replacement root")
		}
		entries, err := s.Replay(ctx, testIdentity)
		if err != nil || len(entries) != 1 {
			t.Fatal(err)
		}
	})
}

func TestConcurrentPutReplayAndServerAck(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, spoolPath(t), testLimits, func(context.Context, Receipt, []byte) error { return nil })
	key := Key{Identity: testIdentity, Occurrence: "1"}
	frame := []byte("immutable")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				r, err := s.Put(ctx, key, frame)
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := s.Replay(ctx, testIdentity); err != nil {
					t.Error(err)
					return
				}
				if err := s.AcknowledgeServer(ctx, r, []byte("verified")); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	_ = s.Close()
	if _, err := s.Put(ctx, key, frame); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
