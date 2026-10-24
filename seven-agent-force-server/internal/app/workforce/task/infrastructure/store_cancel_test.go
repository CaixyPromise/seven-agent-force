package infrastructure

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Preparing the actual statement against the migrated VARCHAR columns catches
// parameter inference failures before cancellation can change any task state.
func TestCancelSQLAgainstMigratedStatusType(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN to a migrated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Prepare(ctx, "cancel_status_types", cancelTaskUpdateSQL); err != nil {
		t.Fatalf("prepare cancellation against migrated status column: %v", err)
	}
}

func TestCancelSQLPreservesStageAndFinishesTerminalTask(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN to PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	// Temporary relations shadow public tables on this connection only. The
	// production statement is executed, but no business row is read or written.
	if _, err := conn.Exec(ctx, `CREATE TEMP TABLE workforce_tasks (
		id BIGINT PRIMARY KEY,status VARCHAR(32) NOT NULL,version INTEGER NOT NULL,
		finished_at TIMESTAMPTZ,cancel_request_id BIGINT,cancel_generation INTEGER NOT NULL,
		cancel_requested_by_principal_id BIGINT,cancel_requested_at TIMESTAMPTZ,cancel_reason TEXT
	); INSERT INTO workforce_tasks(id,status,version,cancel_generation)
	VALUES(10,'RUNNING',1,0),(20,'REVIEWING',1,0)`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id       int64
		status   string
		finished bool
	}{
		{10, "CANCELLING", false},
		{20, "CANCELLED", true},
	} {
		if _, err := conn.Exec(ctx, cancelTaskUpdateSQL, test.id, test.status, int64(30), int64(40), "user cancel"); err != nil {
			t.Fatal(err)
		}
		var status, reason string
		var version, generation int
		var requestID, principalID int64
		var finished, requested bool
		if err := conn.QueryRow(ctx, `SELECT status,version,cancel_generation,cancel_request_id,
			cancel_requested_by_principal_id,cancel_reason,finished_at IS NOT NULL,cancel_requested_at IS NOT NULL
			FROM workforce_tasks WHERE id=$1`, test.id).Scan(&status, &version, &generation, &requestID, &principalID, &reason, &finished, &requested); err != nil {
			t.Fatal(err)
		}
		if status != test.status || finished != test.finished || version != 2 || generation != 1 || requestID != 30 || principalID != 40 || reason != "user cancel" || !requested {
			t.Fatalf("cancel stage/timestamp/audit fields incorrect: status=%s finished=%v version=%d generation=%d request=%d principal=%d requested=%v", status, finished, version, generation, requestID, principalID, requested)
		}
	}
}

func TestCanCancelWithoutRuntime(t *testing.T) {
	tests := []struct {
		name          string
		taskStatus    string
		attemptStatus string
		runtimeExists bool
		want          bool
	}{
		{name: "preparing before runtime", taskStatus: "PREPARING", attemptStatus: "PREPARING", want: true},
		{name: "reconcile cancelling before runtime", taskStatus: "CANCELLING", attemptStatus: "PREPARING", want: true},
		{name: "authorized runtime exists", taskStatus: "PREPARING", attemptStatus: "PREPARING", runtimeExists: true, want: false},
		{name: "running task", taskStatus: "RUNNING", attemptStatus: "PREPARING", want: false},
		{name: "terminal attempt", taskStatus: "PREPARING", attemptStatus: "FAILED", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canCancelWithoutRuntime(test.taskStatus, test.attemptStatus, test.runtimeExists); got != test.want {
				t.Fatalf("canCancelWithoutRuntime(%q, %q, %v) = %v, want %v", test.taskStatus, test.attemptStatus, test.runtimeExists, got, test.want)
			}
		})
	}
}
