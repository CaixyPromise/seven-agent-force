package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRestoreSessionArchiveReplacesPreparedAttemptState(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "partial.json"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	sessionID := "agent-session\x00101\x00202"
	digest := sha256.Sum256([]byte(sessionID))
	digestText := hex.EncodeToString(digest[:])
	body, err := json.Marshal(map[string]any{"schemaVersion": 1, "kind": "agent-session", "sessionId": sessionID, "sessionDigest": digestText})
	if err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.WriteHeader(&tar.Header{Name: "session/session.json", Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	if err := restoreSessionArchive(context.Background(), bytes.NewReader(archive.Bytes()), state); err != nil {
		t.Fatalf("restore session archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "partial.json")); !os.IsNotExist(err) {
		t.Fatalf("partially restored state survived replacement: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(state, "session.json")); err != nil || string(got) != string(body) {
		t.Fatalf("restored session = %q, %v", got, err)
	}
	if err := prepareRestoredSessionState(context.Background(), state, 101, 202, digestText); err != nil {
		t.Fatalf("prepare restored agent session: %v", err)
	}
}

func TestPrepareRestoredSessionStateAcceptsFakeHarnessWithoutCodexIndex(t *testing.T) {
	for _, kind := range []string{"agent-session", "fake-session"} {
		t.Run(kind, func(t *testing.T) { testPrepareRestoredAdapterSession(t, kind) })
	}
}

func testPrepareRestoredAdapterSession(t *testing.T, kind string) {
	state := t.TempDir()
	sessionID := kind + "\x00101\x00201"
	digest := sha256.Sum256([]byte(sessionID))
	digestText := hex.EncodeToString(digest[:])
	body, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"kind":          kind,
		"sessionId":     sessionID,
		"sessionDigest": digestText,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(state, "session.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = prepareRestoredSessionState(context.Background(), state, 101, 202, digestText); err != nil {
		t.Fatalf("prepare %s: %v", kind, err)
	}
}

func TestPrepareRestoredSessionStateRejectsUnknownNonCodexKind(t *testing.T) {
	state := t.TempDir()
	sessionID := "unknown-session"
	digest := sha256.Sum256([]byte(sessionID))
	digestText := hex.EncodeToString(digest[:])
	body, err := json.Marshal(map[string]any{"schemaVersion": 1, "kind": "unknown-session", "sessionId": sessionID, "sessionDigest": digestText})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(state, "session.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = prepareRestoredSessionState(context.Background(), state, 101, 202, digestText); err == nil {
		t.Fatal("unknown native session kind accepted")
	}
}

func TestPrepareRestoredSessionStateRejectsDigestDrift(t *testing.T) {
	state := t.TempDir()
	body := []byte(`{"schemaVersion":1,"kind":"fake-session","sessionId":"session","sessionDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	if err := os.WriteFile(filepath.Join(state, "session.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareRestoredSessionState(context.Background(), state, 101, 202, strings.Repeat("a", 64)); err == nil {
		t.Fatal("expected digest drift to fail closed")
	}
}

func TestRelocateRestoredCodexState(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	codex := filepath.Join(state, "codex")
	rollout := filepath.Join(codex, "sessions", "2026", "08", "29", "rollout-thread.jsonl")
	if err := os.MkdirAll(filepath.Dir(rollout), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rollout, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(codex, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY,rollout_path TEXT NOT NULL,cwd TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	oldRoot := "/workspace/101/attempts/201"
	if _, err = db.Exec(`INSERT INTO threads(id,rollout_path,cwd) VALUES(?,?,?)`, "thread-1", oldRoot+"/state/codex/sessions/2026/08/29/rollout-thread.jsonl", oldRoot+"/repo"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	if err = relocateRestoredCodexState(context.Background(), state, 101, 202); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", filepath.Join(codex, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var gotRollout, gotCWD string
	if err = db.QueryRow(`SELECT rollout_path,cwd FROM threads WHERE id='thread-1'`).Scan(&gotRollout, &gotCWD); err != nil {
		t.Fatal(err)
	}
	if gotRollout != "/workspace/101/attempts/202/state/codex/sessions/2026/08/29/rollout-thread.jsonl" {
		t.Fatalf("rollout path = %q", gotRollout)
	}
	if gotCWD != "/workspace/101/attempts/202/repo" {
		t.Fatalf("cwd = %q", gotCWD)
	}
}

func TestRelocateRestoredCodexStateRejectsForeignTaskPath(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	codex := filepath.Join(state, "codex")
	if err := os.MkdirAll(codex, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(codex, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY,rollout_path TEXT NOT NULL,cwd TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO threads(id,rollout_path,cwd) VALUES('thread-1','/workspace/tsk_other/attempts/att_old/state/codex/sessions/rollout.jsonl','/workspace/tsk_other/attempts/att_old/repo')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = relocateRestoredCodexState(context.Background(), state, 101, 202); err == nil {
		t.Fatal("expected foreign task path rejection")
	}
}
