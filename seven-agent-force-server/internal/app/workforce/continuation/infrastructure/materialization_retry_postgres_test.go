package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Only table definitions are cloned; all rows belong to this disposable schema.
func TestMaterializationRetryPreservesSuccessfulTaskAndDispatchesOnlyPostprocess(t *testing.T) {
	dsn := os.Getenv("CONTINUATION_SUSPEND_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires isolated CONTINUATION_SUSPEND_TEST_POSTGRES_DSN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := pgx.Identifier{fmt.Sprintf("materialization_retry_%d", time.Now().UnixNano())}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"workforce_result_materialization_runs", "workforce_principals", "workforce_outbox_events"} {
		if _, err = db.Exec(ctx, "CREATE TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(ctx, `CREATE TABLE sys_user(id BIGINT PRIMARY KEY);
CREATE TABLE workforce_tasks(id BIGINT PRIMARY KEY,tenant_id BIGINT,status VARCHAR(32),current_task_turn_id BIGINT);
INSERT INTO sys_user VALUES(8);
INSERT INTO workforce_principals(id,principal_type,user_id) VALUES(9,'USER',8);
INSERT INTO workforce_tasks VALUES(2,1,'SUCCEEDED',3);
INSERT INTO workforce_result_materialization_runs(id,task_id,task_turn_id,source_checkpoint_id,kind,input_identity_digest,worker_id,attempt_id,generation,status,run_number,max_runs,idempotency_key,request_digest,created_by_principal_id)
VALUES(10,2,3,4,'ZIP','fixture-input',20,5,6,'FAILED',3,3,'fixture-retry','fixture-request',9)`); err != nil {
		t.Fatal(err)
	}
	store := New(db, &suspendIDs{})
	if err := store.RetryMaterialization(ctx, 99, 8, 10, "wrong tenant"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant fence: %v", err)
	}
	if err := store.RetryMaterialization(ctx, 1, 8, 10, "retry packaging only"); err != nil {
		t.Fatal(err)
	}
	if err := store.RetryMaterialization(ctx, 1, 8, 10, "duplicate"); !errors.Is(err, domain.ErrMaterializationInvalid) {
		t.Fatalf("duplicate retry: %v", err)
	}
	var taskStatus, runStatus, input string
	var currentTurn, turn, checkpoint, attempt, generation int64
	var number, limit int
	if err := db.QueryRow(ctx, `SELECT t.status,t.current_task_turn_id,r.status,r.task_turn_id,r.source_checkpoint_id,r.attempt_id,r.generation,r.input_identity_digest,r.run_number,r.max_runs FROM workforce_tasks t JOIN workforce_result_materialization_runs r ON r.task_id=t.id WHERE r.id=10`).Scan(&taskStatus, &currentTurn, &runStatus, &turn, &checkpoint, &attempt, &generation, &input, &number, &limit); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "SUCCEEDED" || runStatus != "RETRY_WAIT" || currentTurn != 3 || turn != 3 || checkpoint != 4 || attempt != 5 || generation != 6 || input != "fixture-input" || number != 4 || limit != 4 {
		t.Fatalf("retry changed frozen input or task: %s/%s turn=%d checkpoint=%d attempt=%d generation=%d input=%s runs=%d/%d", taskStatus, runStatus, turn, checkpoint, attempt, generation, input, number, limit)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_outbox_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	var exchange, routing, event string
	var payload []byte
	if err := db.QueryRow(ctx, `SELECT exchange_name,routing_key,event_type,payload_json FROM workforce_outbox_events`).Scan(&exchange, &routing, &event, &payload); err != nil {
		t.Fatal(err)
	}
	var message domain.MaterializationMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatal(err)
	}
	if exchange != "seven.workforce.postprocess" || routing != "postprocess.20" || event != "result.materialization.manual_retry" || message.Kind != domain.MaterializationMessageKind || message.ResourceID != 10 || message.WorkerID != 20 || message.Generation != 6 {
		t.Fatalf("unexpected dispatch: %s/%s/%s %+v", exchange, routing, event, message)
	}
}
