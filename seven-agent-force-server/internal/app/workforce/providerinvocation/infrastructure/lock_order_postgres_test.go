package infrastructure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	domain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Uses Reserve itself and migrated table definitions, without credentials,
// invocation writes, or any Provider call. Only this private schema is written.
func TestPostgreSQLProviderReservationLocksTaskBeforeAttempt(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: INTERACTION_TEST_POSTGRES_DSN is required")
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("qa_provider_lock_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE") }()
	for _, table := range []string{"workforce_tasks", "workforce_task_attempts", "workforce_attempt_runtimes"} {
		if _, err = admin.Exec(ctx, "CREATE TABLE "+pgx.Identifier{schema, table}.Sanitize()+" (LIKE "+pgx.Identifier{"public", table}.Sanitize()+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	config.ConnConfig.RuntimeParams["application_name"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, sql := range []string{
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','RUNNING',30,81,82,83,'{}','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`,
		`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,assignment_selection_digest,resolved_execution_snapshot_json) VALUES(40,20,30,1,8,'/qa','RUNNING','qa',CURRENT_TIMESTAMP+interval '1 hour','docker','qa-instance','qa-boot',3,4,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','{}')`,
		`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,status,execution_lease_sha256,execution_lease_expires_at,adapter_session_generation) VALUES(40,50,60,8,'qa-instance','qa-boot',3,4,'qa','qa','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','RUNNING','qa',CURRENT_TIMESTAMP+interval '1 hour',1)`,
	} {
		if _, err = pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	parent, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Rollback(context.Background())
	if _, err = parent.Exec(ctx, `SELECT id FROM workforce_tasks WHERE id=20 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, e := NewStore(pool).Reserve(requestCtx, 90, domain.RequestIdentity{WorkerID: 8, InstanceID: "qa-instance", HostBootID: "qa-boot", TaskID: 20, AttemptID: 40, Generation: 3, OwnerEpoch: 4, LeaseToken: "qa", ProtocolInvocationKey: "qa", Nonce: "qa", BodySHA256: strings.Repeat("a", 64), Timestamp: time.Now()}, domain.Operation("RESPONSES"), []byte(`{"model":"qa"}`), time.Now())
		done <- e
	}()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if err = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')`, schema).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Reserve did not wait for parent Task")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The native-interaction transaction must still be able to acquire both
	// child rows while Reserve waits for Task, rather than form a deadlock.
	if _, err = parent.Exec(ctx, `SELECT id FROM workforce_task_attempts WHERE id=40 FOR UPDATE NOWAIT`); err != nil {
		t.Fatalf("Reserve occupied Attempt before Task: %v", err)
	}
	if _, err = parent.Exec(ctx, `SELECT attempt_id FROM workforce_attempt_runtimes WHERE attempt_id=40 FOR UPDATE NOWAIT`); err != nil {
		t.Fatalf("Reserve occupied Runtime before Task: %v", err)
	}
}
