package infrastructure

import (
	"context"
	"errors"
	accessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	access "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestPlanningOperationAuthorityUsesFrozenAttempt(t *testing.T) {
	dsn := os.Getenv("FORCE_WORKITEM_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated FORCE_WORKITEM_TEST_DSN")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE workforce_task_attempts(id BIGINT PRIMARY KEY, resolved_execution_snapshot_json JSONB NOT NULL) ON COMMIT DROP`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_attempts VALUES (1,'{"stagePurpose":"PLAN"}'),(2,'{"stagePurpose":"IMPLEMENT"}'),(3,'{}')`); err != nil {
		t.Fatal(err)
	}
	if err = checkPlanningOperation(ctx, tx, 1, access.ActionPublish); !errors.Is(err, accessdomain.ErrActionNotAllowed) {
		t.Fatal("planning operation accepted", err)
	}
	for _, id := range []int64{2, 3} {
		if err = checkPlanningOperation(ctx, tx, id, access.ActionPublish); err != nil {
			t.Fatal("ordinary operation rejected", err)
		}
	}
	if err = checkPlanningOperation(ctx, tx, 1, access.ActionRead); err != nil {
		t.Fatal("planning read rejected", err)
	}
	if err = checkPlanningOperation(ctx, tx, 99, access.ActionPublish); err == nil {
		t.Fatal("unknown attempt accepted")
	}
}
