package infrastructure

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Never point the ordinary fixture helper at the live schema. This wrapper
// clones empty tables and owns its event sequence before running the contract.
func TestRecoveredSourceOwnerIndependentReviewReport(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires PostgreSQL for an isolated QA schema")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("qa_review_owner_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	rows, err := admin.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND (tablename LIKE 'workforce_%' OR tablename='sys_user')`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		if _, err = admin.Exec(ctx, "CREATE TABLE "+pgx.Identifier{schema, name}.Sanitize()+" (LIKE "+pgx.Identifier{"public", name}.Sanitize()+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
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
	for _, sql := range []string{
		`CREATE SEQUENCE qa_event_ids`,
		`ALTER TABLE workforce_task_events ALTER COLUMN id DROP IDENTITY IF EXISTS`,
		`ALTER TABLE workforce_task_events ALTER COLUMN id SET DEFAULT nextval('qa_event_ids'::regclass)`,
		`ALTER TABLE workforce_task_attempts ALTER COLUMN owner_epoch SET DEFAULT 2`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	isolatedURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := isolatedURL.Query()
	query.Set("search_path", quoted+",pg_catalog")
	isolatedURL.RawQuery = query.Encode()
	probe, err := pgxpool.New(ctx, isolatedURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	var actualSchema, ownerDefault string
	if err = probe.QueryRow(ctx, `SELECT current_schema(),column_default FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='workforce_task_attempts' AND column_name='owner_epoch'`).Scan(&actualSchema, &ownerDefault); err != nil {
		t.Fatal(err)
	}
	if actualSchema != schema || ownerDefault != "2" {
		t.Fatal("QA schema or recovered source owner not isolated")
	}
	t.Setenv(reviewRunTestDSNEnv, isolatedURL.String())
	TestReviewRunCanonicalReportVerdictsAndFencing(t)
}
