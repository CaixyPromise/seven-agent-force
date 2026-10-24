// sql-governance: sqlite-private
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// prepareRestoredSessionState validates the encrypted, Attempt-private
// session envelope before the Runtime is launched. Native adapter sessions
// have no Codex SQLite index to relocate; Codex sessions do, because their
// durable thread paths contain the source Attempt's absolute workspace path.
func prepareRestoredSessionState(ctx context.Context, stateRoot string, taskID, targetAttemptID int64, expectedDigest string) error {
	body, err := os.ReadFile(filepath.Join(stateRoot, "session.json"))
	if err != nil || len(body) == 0 || len(body) > 4096 {
		return errors.New("CONTINUATION_SESSION_STATE_INVALID")
	}
	var envelope struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		SessionID     string `json:"sessionId"`
		SessionDigest string `json:"sessionDigest"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("CONTINUATION_SESSION_STATE_INVALID")
	}
	digest := sha256.Sum256([]byte(envelope.SessionID))
	if envelope.SchemaVersion != 1 || strings.TrimSpace(envelope.SessionID) == "" || len(envelope.SessionID) > 512 || len(expectedDigest) != 64 || envelope.SessionDigest != expectedDigest || envelope.SessionDigest != hex.EncodeToString(digest[:]) {
		return errors.New("CONTINUATION_SESSION_IDENTITY_INVALID")
	}
	switch envelope.Kind {
	case "agent-session", "fake-session":
		return nil
	case "codex-session":
		return relocateRestoredCodexState(ctx, stateRoot, taskID, targetAttemptID)
	default:
		return errors.New("CONTINUATION_SESSION_STATE_INVALID")
	}
}

// relocateRestoredCodexState moves Codex's durable thread index from the
// source Attempt's container path to the target Attempt's container path.
// The encrypted Session archive deliberately contains no active Runtime
// state, but Codex keeps rollout_path and cwd as absolute paths in state_*.sqlite.
// A new Turn always has a new Attempt, so leaving those paths untouched makes
// an otherwise valid cross-Worker restore impossible to resume.
func relocateRestoredCodexState(ctx context.Context, stateRoot string, taskID, targetAttemptID int64) error {
	if taskID <= 0 || targetAttemptID <= 0 {
		return errors.New("CONTINUATION_SESSION_IDENTITY_INVALID")
	}
	codexRoot := filepath.Join(stateRoot, "codex")
	databases, err := filepath.Glob(filepath.Join(codexRoot, "state_*.sqlite"))
	if err != nil || len(databases) != 1 {
		return errors.New("CONTINUATION_SESSION_STATE_INVALID")
	}
	db, err := sql.Open("sqlite", databases[0])
	if err != nil {
		return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,rollout_path,cwd FROM threads ORDER BY id`)
	if err != nil {
		return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
	}
	type relocation struct {
		id          string
		rolloutPath string
		cwd         string
	}
	items := make([]relocation, 0, 1)
	for rows.Next() {
		var item relocation
		if err = rows.Scan(&item.id, &item.rolloutPath, &item.cwd); err != nil {
			rows.Close()
			return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
		}
		items = append(items, item)
	}
	if err = rows.Close(); err != nil {
		return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
	}
	if len(items) == 0 {
		return errors.New("CONTINUATION_SESSION_UNAVAILABLE")
	}
	taskPrefix := filepath.ToSlash(filepath.Join("/workspace", strconv.FormatInt(taskID, 10), "attempts")) + "/"
	targetAttemptRoot := taskPrefix + strconv.FormatInt(targetAttemptID, 10)
	for _, item := range items {
		if item.id == "" || !strings.HasPrefix(item.rolloutPath, taskPrefix) || !strings.HasPrefix(item.cwd, taskPrefix) {
			return errors.New("CONTINUATION_SESSION_IDENTITY_INVALID")
		}
		rest := strings.TrimPrefix(item.rolloutPath, taskPrefix)
		separator := "/state/codex/"
		index := strings.Index(rest, separator)
		if index <= 0 || strings.Contains(rest[:index], "/") {
			return errors.New("CONTINUATION_SESSION_IDENTITY_INVALID")
		}
		relativeRollout := rest[index+len(separator):]
		if relativeRollout == "" || strings.Contains(relativeRollout, "..") || !strings.HasSuffix(relativeRollout, ".jsonl") {
			return errors.New("CONTINUATION_SESSION_IDENTITY_INVALID")
		}
		hostRollout := filepath.Join(codexRoot, filepath.FromSlash(relativeRollout))
		info, statErr := os.Stat(hostRollout)
		if statErr != nil || !info.Mode().IsRegular() {
			return errors.New("CONTINUATION_SESSION_UNAVAILABLE")
		}
		expectedSourceRoot := taskPrefix + rest[:index]
		if item.cwd != expectedSourceRoot+"/repo" {
			return errors.New("CONTINUATION_SESSION_IDENTITY_INVALID")
		}
		newRollout := targetAttemptRoot + "/state/codex/" + relativeRollout
		newCWD := targetAttemptRoot + "/repo"
		result, updateErr := tx.ExecContext(ctx, `UPDATE threads SET rollout_path=?,cwd=? WHERE id=? AND rollout_path=? AND cwd=?`, newRollout, newCWD, item.id, item.rolloutPath, item.cwd)
		if updateErr != nil {
			return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", updateErr)
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return errors.New("CONTINUATION_SESSION_RELOCATION_CONFLICT")
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("CONTINUATION_SESSION_STATE_INVALID: %w", err)
	}
	return nil
}
