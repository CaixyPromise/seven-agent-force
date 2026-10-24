package schemaalignment

import (
	"context"
	"fmt"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
)

func TestBatch1CompletionCheckAlignment(t *testing.T) {
	dsn := os.Getenv("BATCH1_ALIGNMENT_DSN")
	if dsn == "" {
		t.Skip("BATCH1_ALIGNMENT_DSN required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "seven_worker_batch1_") {
		t.Fatal("requires authorized isolated Batch1 database")
	}
	ids, err := xid.New(41)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = Align(ctx, db, ids); err != nil {
			t.Fatal(err)
		}
	}
	if err = Verify(ctx, db); err != nil {
		t.Fatal(err)
	}
	var kind string
	var length int
	if err = db.QueryRow(ctx, `SELECT data_type,character_maximum_length FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_runs' AND column_name='completion_check_digest'`).Scan(&kind, &length); err != nil {
		t.Fatal(err)
	}
	if kind != "character varying" || length != 64 {
		t.Fatal("review digest schema mismatch")
	}
	// Exercise numeric USER -> principal conversion with a real legacy row,
	// inside a rolled-back transaction so the aligned clone stays canonical.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	user, decision := ids.NextID(), ids.NextID()
	if _, err = tx.Exec(ctx, `INSERT INTO sys_user(id,"userAccount","nickName") VALUES($1,$2,'principal alignment fixture')`, user, fmt.Sprintf("cc-align-%d", user)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE workforce_completion_check_decisions DROP COLUMN actor_principal_id, ADD COLUMN actor_user_id BIGINT`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_completion_check_decisions(id,tenant_id,task_id,source_task_turn_id,task_turn_id,checkpoint_id,actor_user_id,action,result_id,baseline_result_id,reason,idempotency_key,request_digest) VALUES($1,1,$1,$1,$1,$1,$2,'AUTHORIZE_REPAIR',$1,$1,'fixture','fixture','fixture')`, decision, user); err != nil {
		t.Fatal(err)
	}
	if _, err = alignCompletionDecisionPrincipals(ctx, tx, ids); err != nil {
		t.Fatal(err)
	}
	if _, err = alignCompletionDecisionPrincipals(ctx, tx, ids); err != nil {
		t.Fatal(err)
	}
	var principal, linkedUser int64
	var principalType string
	if err = tx.QueryRow(ctx, `SELECT d.actor_principal_id,p.user_id,p.principal_type FROM workforce_completion_check_decisions d JOIN workforce_principals p ON p.id=d.actor_principal_id WHERE d.id=$1`, decision).Scan(&principal, &linkedUser, &principalType); err != nil {
		t.Fatal(err)
	}
	if principal == user || linkedUser != user || principalType != "USER" {
		t.Fatal("legacy user ID was not resolved to a distinct USER principal")
	}
}
