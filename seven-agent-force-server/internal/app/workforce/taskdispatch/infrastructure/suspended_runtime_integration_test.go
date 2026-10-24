package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Clones schema only from the explicitly selected migrated test database.
// search_path excludes public so a missing QA table cannot mutate real data.
func dispatchQATables(t *testing.T, tables ...string) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: INTERACTION_TEST_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid integration database configuration")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	authoritySQLCheck(t, err)
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("qa_dispatch_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
	authoritySQLCheck(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
		authoritySQLCheck(t, err)
		var exists bool
		authoritySQLCheck(t, admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)", schema).Scan(&exists))
		if exists {
			t.Error("QA schema cleanup failed")
		}
	})
	for _, table := range tables {
		_, err := admin.Exec(ctx, "CREATE TABLE "+pgx.Identifier{schema, table}.Sanitize()+" (LIKE "+pgx.Identifier{"public", table}.Sanitize()+" INCLUDING ALL)")
		authoritySQLCheck(t, err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	authoritySQLCheck(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestPostgreSQLSuspendedRuntimeReconcileAndWorkspaceRetention(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := dispatchQATables(t, "workforce_tasks", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_execution_nodes", "workforce_task_assignments")
	for _, sql := range []string{
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,status,current_task_turn_id,finished_at,created_at,started_at) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','qa','INTERACTION_SUSPENDED',30,CURRENT_TIMESTAMP-interval '30 days',CURRENT_TIMESTAMP-interval '31 days',CURRENT_TIMESTAMP-interval '31 days')`,
		`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,execution_node_id,assignment_id,finished_at) VALUES(40,20,30,1,8,'/qa','INTERACTION_SUSPENDED','qa',CURRENT_TIMESTAMP-interval '1 day','docker','qa-instance','qa-boot',3,5,50,60,CURRENT_TIMESTAMP)`,
		`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,status,execution_lease_sha256,execution_lease_expires_at,fenced_at,reattach_deadline) VALUES(40,50,60,8,'qa-instance','qa-boot',3,5,'qa','qa','qa','INTERACTION_SUSPENDED','qa',CURRENT_TIMESTAMP-interval '1 day',CURRENT_TIMESTAMP-interval '1 day',CURRENT_TIMESTAMP-interval '1 day')`,
		`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest) VALUES(50,20,30,'qa','INTERACTION_SUSPENDED',3,'qa')`,
		`INSERT INTO workforce_task_assignments(id,task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,lease_token_sha256,lease_expires_at,status,message_id,completed_at) VALUES(60,20,50,8,'qa-instance',3,'qa','qa',CURRENT_TIMESTAMP-interval '1 day','COMPLETED',70,CURRENT_TIMESTAMP)`,
	} {
		_, err := pool.Exec(ctx, sql)
		authoritySQLCheck(t, err)
	}
	s := New(pool, nil)
	for name, reconcile := range map[string]func(context.Context, int) (int, error){"timeouts": s.ReconcileTimeouts, "expired_assignment": s.ReconcileExpired, "runtime_loss": s.ReconcileRuntimeLosses} {
		t.Run(name, func(t *testing.T) {
			count, err := reconcile(ctx, 8)
			authoritySQLCheck(t, err)
			if count != 0 {
				t.Fatal("sealed suspended execution was reconciled")
			}
		})
	}
	for _, taskStatus := range []string{"INTERACTION_SUSPENDED", "RUNNING"} {
		t.Run("late_loss_"+taskStatus, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			authoritySQLCheck(t, err)
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status=$1`, taskStatus)
			authoritySQLCheck(t, err)
			_, err = s.fenceRuntimeLoss(ctx, tx, 40, domain.ReasonWorkerLostWithoutRuntime, "")
			if !errors.Is(err, application.ErrNotClaimable) {
				t.Fatalf("sealed runtime loss not rejected: %v", err)
			}
			var status string
			var owner int64
			authoritySQLCheck(t, tx.QueryRow(ctx, `SELECT status,owner_epoch FROM workforce_attempt_runtimes WHERE attempt_id=40`).Scan(&status, &owner))
			if status != "INTERACTION_SUSPENDED" || owner != 5 {
				t.Fatal("sealed runtime authority changed")
			}
		})
	}
	t.Run("task_suspend_rejects_stale_recovering_runtime", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		authoritySQLCheck(t, err)
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='RECOVERING'`)
		authoritySQLCheck(t, err)
		_, err = s.fenceRuntimeLoss(ctx, tx, 40, domain.ReasonWorkerLostWithoutRuntime, "")
		if !errors.Is(err, application.ErrNotClaimable) {
			t.Fatal("suspended Task was retried through stale runtime")
		}
	})
	t.Run("workspace_retained_even_with_old_finished_at", func(t *testing.T) {
		result, err := taskinfra.NewStore(pool, nil).WorkspaceGCEligibility(ctx, 20)
		authoritySQLCheck(t, err)
		if result.Terminal || result.PortableStateAvailable || result.ReasonCode != "TASK_NOT_TERMINAL" {
			t.Fatalf("suspended workspace became GC eligible: %+v", result)
		}
	})
}

func TestSuspendedRuntimeCannotRenewEvenWithinLeaseGrace(t *testing.T) {
	a := runtimeAuthority{WorkerID: 8, InstanceID: "qa", HostBootID: "boot", Generation: 3, OwnerEpoch: 5, LeaseHash: hashToken("qa"), LeaseExpires: time.Now().Add(time.Hour), Status: "INTERACTION_SUSPENDED"}
	e := domain.MachineEnvelope{WorkerID: 8, InstanceID: "qa", HostBootID: "boot", Generation: 3, OwnerEpoch: 5}
	if a.validAllowExpired(e, "qa", time.Hour) {
		t.Fatal("suspension was not an execution-authority terminal state")
	}
	if isTerminalTaskStatus("INTERACTION_SUSPENDED") {
		t.Fatal("business Task must remain resumable")
	}
}

func TestPostgreSQLRuntimeReconcileReleasesTerminalAttemptAssignment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := dispatchQATables(t, "workforce_tasks", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_execution_nodes", "workforce_task_assignments")
	for _, sql := range []string{
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,status,created_at) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','qa','FAILED',CURRENT_TIMESTAMP)`,
		`INSERT INTO workforce_execution_nodes(id,task_id,node_key,status,generation,execution_requirement_digest) VALUES(50,20,'qa','FAILED',3,'qa')`,
		`INSERT INTO workforce_task_assignments(id,task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,lease_token_sha256,lease_expires_at,status,message_id) VALUES(60,20,50,8,'qa-instance',3,'qa','qa',CURRENT_TIMESTAMP-interval '1 day','CLAIMED',70)`,
		`INSERT INTO workforce_task_attempts(id,task_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,execution_node_id,assignment_id,finished_at) VALUES(40,20,1,8,'/qa','FAILED','qa',CURRENT_TIMESTAMP-interval '1 day','docker','qa-instance','qa-boot',3,5,50,60,CURRENT_TIMESTAMP)`,
	} {
		_, err := pool.Exec(ctx, sql)
		authoritySQLCheck(t, err)
	}
	_, err := New(pool, nil).ReconcileRuntimeLosses(ctx, 8)
	authoritySQLCheck(t, err)
	var status, reason string
	authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT status,release_reason_code FROM workforce_task_assignments WHERE id=60`).Scan(&status, &reason))
	if status != "COMPLETED" || reason != "TERMINAL_ATTEMPT_RECONCILED" {
		t.Fatalf("terminal assignment retained capacity: %s %s", status, reason)
	}
}
