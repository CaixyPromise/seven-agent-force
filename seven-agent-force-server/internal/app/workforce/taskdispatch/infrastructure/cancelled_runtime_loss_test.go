package infrastructure

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"testing"
)

type cancelledLossRow struct{ task bool }

func (r cancelledLossRow) Scan(v ...any) error {
	if r.task {
		*v[0].(*string) = "CANCELLED"
		*v[1].(*int) = 1
		*v[2].(*int) = 3
		*v[3].(*int64) = 1
	} else {
		*v[0].(*int64) = 11
		*v[1].(*int64) = 12
		*v[2].(*int64) = 13
		*v[3].(*int64) = 2
		*v[4].(*string) = "RUNNING"
	}
	return nil
}

type cancelledLossTx struct {
	pgx.Tx
	queries int
	writes  []string
}

func (tx *cancelledLossTx) QueryRow(_ context.Context, q string, _ ...any) pgx.Row {
	tx.queries++
	return cancelledLossRow{task: strings.Contains(q, "SELECT t.status")}
}
func (tx *cancelledLossTx) Exec(_ context.Context, q string, _ ...any) (pgconn.CommandTag, error) {
	tx.writes = append(tx.writes, q)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func TestCancelledTaskRuntimeLossClosesResidualWithoutRetryOrDuplicateEvent(t *testing.T) {
	tx := &cancelledLossTx{}
	s := &Store{}
	result, e := s.fenceRuntimeLoss(context.Background(), tx, 14, "RUNTIME_MISSING", "")
	if e != nil || result.Disposition != "CANCELLED" || result.NextGeneration != 0 {
		t.Fatalf("cancelled recovery %+v %v", result, e)
	}
	joined := strings.Join(tx.writes, "\n")
	for _, want := range []string{"execution_lease_expires_at=CURRENT_TIMESTAMP", "lease_expires_at=CURRENT_TIMESTAMP", "SET status='CANCELLED',retry_class=''", "workforce_attempt_runtimes SET status='CANCELLED'", "workforce_execution_nodes SET status='CANCELLED'"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing cleanup write %s", want)
		}
	}
	for _, bad := range []string{"UPDATE workforce_tasks ", "INSERT", "RETRY_WAIT", "attempt_count="} {
		if strings.Contains(joined, bad) {
			t.Fatalf("cancelled task resurrected or re-notified: %s", bad)
		}
	}
	if tx.queries != 2 {
		t.Fatal("entered retry/finalization path")
	}
}
