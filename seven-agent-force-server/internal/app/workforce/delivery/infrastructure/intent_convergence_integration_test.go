package infrastructure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeliveryIntentConvergesAfterDraftReviewReadback(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("DELIVERY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set DELIVERY_TEST_POSTGRES_DSN to an isolated PostgreSQL database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("delivery_intent_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(ctx, `
		CREATE TABLE workforce_delivery_requests (id BIGINT PRIMARY KEY,tenant_id BIGINT NOT NULL,task_turn_id BIGINT NOT NULL,status TEXT NOT NULL,reason_code TEXT NOT NULL DEFAULT '');
		CREATE TABLE workforce_delivery_intents (delivery_request_id BIGINT NOT NULL,delivery_mode TEXT NOT NULL,status TEXT NOT NULL,version BIGINT NOT NULL,updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE workforce_delivery_operations (id BIGINT PRIMARY KEY,delivery_request_id BIGINT NOT NULL,operation_kind TEXT NOT NULL,status TEXT NOT NULL);
		INSERT INTO workforce_delivery_requests VALUES (101,1,11,'SUCCEEDED','REMOTE_REF_VERIFIED');
		INSERT INTO workforce_delivery_intents VALUES (101,'NEW_BRANCH_AND_DRAFT_REVIEW','DELIVERY_RUNNING',3,CURRENT_TIMESTAMP);
		INSERT INTO workforce_delivery_operations VALUES (201,101,'CREATE_PULL_REQUEST','SUCCEEDED');`); err != nil {
		t.Fatal(err)
	}

	store := &Store{}
	for run := 0; run < 2; run++ {
		tx, beginErr := db.Begin(ctx)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		if err = store.projectDeliveryRequest(ctx, tx, 101); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	var version int64
	if err = db.QueryRow(ctx, `SELECT status,version FROM workforce_delivery_intents WHERE delivery_request_id=101`).Scan(&status, &version); err != nil {
		t.Fatal(err)
	}
	if status != "DELIVERED" || version != 4 {
		t.Fatalf("DeliveryRequest=SUCCEEDED and PR=SUCCEEDED converged intent to status=%s version=%d, want DELIVERED version=4", status, version)
	}
}
