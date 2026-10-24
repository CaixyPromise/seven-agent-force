// Package eventlog owns the small persistence primitive shared by Workforce
// modules that append auditable task facts. A task's events form one ordered
// timeline even when concurrent Worker, Supervisor, Result Processor, and
// post-process writers report facts at the same time.
package eventlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Append serializes sequence allocation for one Task and records an event in
// the caller's transaction. The advisory lock is transaction scoped, so it is
// released on both commit and rollback. Callers remain responsible for domain
// fencing and identity checks; this helper only makes the append-only audit
// timeline safe under concurrent writers.
func Append(ctx context.Context, tx pgx.Tx, taskID int64, attemptID int64, eventType string, payload any) error {
	_, err := AppendWithSequence(ctx, tx, taskID, attemptID, eventType, payload)
	return err
}

// AppendWithSequence appends an event and returns the authoritative occurrence
// identity allocated inside the caller's transaction. Notification facts use
// this value so repeated states of the same aggregate remain independently
// addressable without inventing a second counter.
func AppendWithSequence(ctx context.Context, tx pgx.Tx, taskID int64, attemptID int64, eventType string, payload any) (int64, error) {
	if taskID <= 0 {
		return 0, fmt.Errorf("event log task id is required")
	}
	if strings.TrimSpace(eventType) == "" {
		return 0, fmt.Errorf("event log event type is required")
	}
	body, err := marshalLosslessPayload(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal event payload: %w", err)
	}

	// The unique constraint is the final integrity guard. Serializing before
	// reading MAX(sequence) avoids converting normal concurrent facts into an
	// audit failure or silently dropping a record.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, taskID); err != nil {
		return 0, fmt.Errorf("lock task event sequence: %w", err)
	}
	var sequence int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM workforce_task_events WHERE task_id=$1`, taskID).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("allocate task event sequence: %w", err)
	}
	var attempt any
	if attemptID > 0 {
		attempt = attemptID
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_events (task_id,attempt_id,event_type,sequence,payload_json) VALUES ($1,$2,$3,$4,$5)`, taskID, attempt, eventType, sequence, body); err != nil {
		return 0, fmt.Errorf("append task event: %w", err)
	}
	return sequence, nil
}

// marshalLosslessPayload is the final audit-event transport guard. Most
// domain DTOs already use json:",string" on int64 identities, but event
// payloads are intentionally flexible maps. Converting identity-shaped map
// values here prevents a newly-added event from exposing a Snowflake as a
// JavaScript Number while leaving counters, epochs, sizes, and durations
// numeric.
func marshalLosslessPayload(payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err = decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("event payload contains trailing JSON")
	}
	return json.Marshal(stringifyIdentityValues(value, ""))
}

func stringifyIdentityValues(value any, key string) any {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			typed[childKey] = stringifyIdentityValues(child, childKey)
		}
		return typed
	case []any:
		for index, child := range typed {
			typed[index] = stringifyIdentityValues(child, key)
		}
		return typed
	case json.Number:
		if !eventIdentityKey(key) {
			return typed
		}
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err == nil && parsed > 0 {
			return strconv.FormatInt(parsed, 10)
		}
	}
	return value
}

func eventIdentityKey(key string) bool {
	return strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID") || strings.HasSuffix(key, "Ids") || strings.HasSuffix(key, "IDs") || key == "actor" || key == "createdBy" || key == "approvedBy" || key == "publishedBy" || key == "cancelRequestedBy"
}
