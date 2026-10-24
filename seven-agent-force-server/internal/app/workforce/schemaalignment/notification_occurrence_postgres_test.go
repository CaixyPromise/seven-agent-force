package schemaalignment

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgreSQLNotificationOccurrenceWidening(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: INTERACTION_TEST_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("invalid test configuration")
	}
	defer admin.Close()
	schema := fmt.Sprintf("qa_occurrence_widening_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test configuration")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE workforce_task_notification_records(id BIGINT PRIMARY KEY,source_occurrence INTEGER NOT NULL DEFAULT 0 CHECK(source_occurrence>=0)); INSERT INTO workforce_task_notification_records VALUES(1,0),(2,42)`); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = alignTaskNotificationOccurrence(ctx, tx); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	const questionID int64 = 8000000000000000012
	if _, err = pool.Exec(ctx, `INSERT INTO workforce_task_notification_records VALUES(3,$1)`, questionID); err != nil {
		t.Fatal(err)
	}
	var kind string
	if err = pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='workforce_task_notification_records' AND column_name='source_occurrence'`).Scan(&kind); err != nil || kind != "bigint" {
		t.Fatalf("kind=%s err=%v", kind, err)
	}
	var retained int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM workforce_task_notification_records WHERE (id=1 AND source_occurrence=0) OR (id=2 AND source_occurrence=42) OR (id=3 AND source_occurrence=$1)`, questionID).Scan(&retained); err != nil || retained != 3 {
		t.Fatalf("retained=%d err=%v", retained, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO workforce_task_notification_records VALUES(4,-1)`); err == nil {
		t.Fatal("nonnegative occurrence constraint lost")
	}
}
