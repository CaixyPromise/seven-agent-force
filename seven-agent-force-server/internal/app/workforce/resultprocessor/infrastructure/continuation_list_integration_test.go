package infrastructure

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Read-only regression for INTERACTION_SUSPEND rows, whose evaluation cycle is
// intentionally NULL. It never creates or updates a Task.
func TestListContinuationCheckpointsPostgresInteractionSuspend(t *testing.T) {
	dsn := os.Getenv("RESULT_PROCESSOR_TEST_POSTGRES_DSN")
	taskText := os.Getenv("RESULT_PROCESSOR_TEST_CONTINUATION_TASK_ID")
	tenantText := os.Getenv("RESULT_PROCESSOR_TEST_CONTINUATION_TENANT_ID")
	if dsn == "" || taskText == "" || tenantText == "" {
		t.Skip("isolated migrated database and continuation Task identity required")
	}
	taskID, taskErr := strconv.ParseInt(taskText, 10, 64)
	tenantID, tenantErr := strconv.ParseInt(tenantText, 10, 64)
	if taskErr != nil || tenantErr != nil || taskID <= 0 || tenantID <= 0 {
		t.Fatal("positive decimal Task and tenant identities required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var database string
	if err = db.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil || !strings.Contains(database, "batch3") {
		t.Fatal("isolated Batch3 database required")
	}
	items, err := New(db, nil).ListContinuationCheckpoints(ctx, tenantID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EvaluationCycleID != 0 || items[0].Decision != "" || items[0].Status != "AVAILABLE" {
		t.Fatalf("interaction checkpoint projection changed: %#v", items)
	}
}
