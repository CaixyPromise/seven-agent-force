package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgreSQLGitPlanSaveOnlyAndTenantIsolation(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: INTERACTION_TEST_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("qa_git_plan_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	for _, table := range []string{"workforce_tasks", "workforce_task_git_plans"} {
		if _, err := admin.Exec(ctx, "CREATE TABLE "+pgx.Identifier{schema, table}.Sanitize()+" (LIKE "+pgx.Identifier{"public", table}.Sanitize()+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, query := range []string{
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','RUNNING',30,81,82,83,'{}','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`,
		`INSERT INTO workforce_task_git_plans(id,tenant_id,task_id,task_turn_id,project_id,source_commit,delivery_mode,plan_digest) VALUES(40,10,20,30,82,'qa-result','NONE','qa')`,
	} {
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	store := New(pool, nil)
	plan, err := store.GetGitPlan(ctx, 10, 20)
	if err != nil || plan.ReviewBaseSourceRevisionID != 0 || plan.DeliveryMode != "NONE" || plan.ProjectID != 82 {
		t.Fatalf("SAVE_ONLY without review base must remain readable: %+v, %v", plan, err)
	}
	if _, err := store.GetGitPlan(ctx, 11, 20); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("foreign tenant read: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workforce_task_git_plans SET review_base_source_revision_id=91`); err != nil {
		t.Fatal(err)
	}
	plan, err = store.GetGitPlan(ctx, 10, 20)
	if err != nil || plan.ReviewBaseSourceRevisionID != 91 {
		t.Fatalf("review base lost: %+v, %v", plan, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workforce_tasks SET current_task_turn_id=31`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetGitPlan(ctx, 10, 20); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("superseded turn read: %v", err)
	}
}
