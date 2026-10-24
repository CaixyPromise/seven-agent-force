package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	interactiondomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	registryinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/infrastructure"
	"github.com/bwmarrin/snowflake"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authorityTestIDs struct{ node *snowflake.Node }

func (i authorityTestIDs) NextID() int64 { return i.node.Generate().Int64() }

// This test deliberately uses the production authority, principal resolver and
// registry reader, and clones real migrated table definitions, not mock SQL.
// Only the uniquely named QA schema is written; public data is never copied.
func TestPostgreSQLInteractionAuthority(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: INTERACTION_TEST_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid integration database configuration")
	}
	admin, err := pgxpool.NewWithConfig(ctx, config)
	authoritySQLCheck(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("qa_interaction_authority_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
	authoritySQLCheck(t, err)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, dropErr := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE")
		authoritySQLCheck(t, dropErr)
		var exists bool
		authoritySQLCheck(t, admin.QueryRow(cleanup, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)", schema).Scan(&exists))
		if exists {
			t.Error("QA schema was not removed")
		}
	}()
	for _, table := range []string{"sys_user", "workforce_tasks", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_workers", "workforce_worker_sessions", "workforce_worker_images", "workforce_principals"} {
		_, err = admin.Exec(ctx, "CREATE TABLE "+pgx.Identifier{schema, table}.Sanitize()+" (LIKE "+pgx.Identifier{"public", table}.Sanitize()+" INCLUDING ALL)")
		authoritySQLCheck(t, err)
	}
	qaConfig := config.Copy()
	qaConfig.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, qaConfig)
	authoritySQLCheck(t, err)
	defer pool.Close()
	var searchPath string
	authoritySQLCheck(t, pool.QueryRow(ctx, "SELECT current_schema()").Scan(&searchPath))
	if searchPath != schema {
		t.Fatal("integration writes are not isolated")
	}
	node, err := snowflake.NewNode(901)
	authoritySQLCheck(t, err)
	authority := NewInteractionAuthority(authorityTestIDs{node}, principalinfra.NewResolver(), registryinfra.New(pool))
	machine := interaction.Machine{TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, WorkerID: 8, InstanceID: "qa-instance", HostBootID: "qa-boot", Generation: 3, OwnerEpoch: 4, AdapterSessionGeneration: 1, ExecutionLease: "qa-test-only-lease"}
	hash := sha256.Sum256([]byte(machine.ExecutionLease))
	lease := hex.EncodeToString(hash[:])
	digest := "sha256:" + strings.Repeat("a", 64)
	caps := []string{"execution.start", "execution.interrupt", "activity", "interaction.request", "interaction.respond", "interaction.received", "interaction.closed", "user.message", "message.received"}
	manifest := func(capabilities []string) []byte {
		b, e := json.Marshal(map[string]any{"adapter": map[string]any{
			"id": "qa-adapter", "version": "1", "executable": "/opt/seven/adapter/qa",
			"binarySha256": strings.Repeat("b", 64), "sourceSha256": strings.Repeat("c", 64),
			"protocol": map[string]any{"name": "seven.agent-adapter", "min": 1, "max": 1}, "transport": "stdio", "capabilities": capabilities,
		}})
		authoritySQLCheck(t, e)
		return b
	}
	seed := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO sys_user(id,"userAccount","nickName") VALUES(7,'qa-history-reader','QA')`, nil},
		{`INSERT INTO workforce_workers(id,name,token_hint,current_instance_id) VALUES(8,'qa','qa','qa-instance')`, nil},
		{`INSERT INTO workforce_worker_sessions(instance_id,worker_id,host_boot_id,started_at) VALUES('qa-instance',8,'qa-boot',CURRENT_TIMESTAMP)`, nil},
		{`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','RUNNING',30,81,82,83,'{}','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`, nil},
		{`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,assignment_selection_digest,resolved_execution_snapshot_json) VALUES(40,20,30,1,8,'/qa','RUNNING',$1,CURRENT_TIMESTAMP+interval '1 hour','docker','qa-instance','qa-boot',3,4,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','{}')`, []any{lease}},
		{`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,status,execution_lease_sha256,execution_lease_expires_at,adapter_session_generation) VALUES(40,50,60,8,'qa-instance','qa-boot',3,4,'qa','qa',$1,'RUNNING',$2,CURRENT_TIMESTAMP+interval '1 hour',1)`, []any{digest, lease}},
		{`INSERT INTO workforce_worker_images(worker_id,instance_id,image_digest,role,contract_version,manifest_digest,manifest_json,admission_status,inventory_version,inventory_digest,scheduling_digest) VALUES(8,'qa-instance',$1,'attempt','qa','qa',$2,'ADMITTED',1,'qa','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`, []any{digest, manifest(caps)}},
	}
	for _, statement := range seed {
		_, err = pool.Exec(ctx, statement.sql, statement.args...)
		authoritySQLCheck(t, err)
	}
	begin := func(t *testing.T) pgx.Tx {
		t.Helper()
		tx, e := pool.Begin(ctx)
		authoritySQLCheck(t, e)
		t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
		return tx
	}
	t.Run("runtime authority locks Task before Attempt", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE workforce_task_attempts SET execution_node_id=50 WHERE id=40`)
		authoritySQLCheck(t, err)
		parent := begin(t)
		_, err = parent.Exec(ctx, `SELECT id FROM workforce_tasks WHERE id=20 FOR UPDATE`)
		authoritySQLCheck(t, err)
		runtimeTx := begin(t)
		var pid int
		authoritySQLCheck(t, runtimeTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
		done := make(chan error, 1)
		go func() { _, e := loadRuntimeAuthority(ctx, runtimeTx, 40); done <- e }()
		deadline := time.Now().Add(3 * time.Second)
		for {
			var waiting bool
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting))
			if waiting {
				break
			}
			select {
			case err := <-done:
				_ = parent.Rollback(ctx)
				t.Fatalf("runtime authority bypassed the parent Task lock: %v", err)
			default:
			}
			if time.Now().After(deadline) {
				_ = parent.Rollback(ctx)
				<-done
				t.Fatal("runtime authority did not wait on Task")
			}
			time.Sleep(10 * time.Millisecond)
		}
		// A question transaction holding Task can still lock Attempt without a
		// deadlock against concurrent lease authority loading.
		_, err = parent.Exec(ctx, `SELECT id FROM workforce_task_attempts WHERE id=40 FOR UPDATE NOWAIT`)
		authoritySQLCheck(t, err)
		authoritySQLCheck(t, parent.Commit(ctx))
		authoritySQLCheck(t, <-done)
		authoritySQLCheck(t, runtimeTx.Rollback(ctx))
	})
	for _, phase := range []string{"FAILED", "TIMED_OUT", "CANCELLED", "SUCCEEDED"} {
		t.Run("terminal closure "+phase, func(t *testing.T) {
			tx := begin(t)
			_, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status=$1`, phase)
			authoritySQLCheck(t, err)
			target, err := authority.AuthorizeUser(ctx, tx, interaction.User{TenantID: 10, TaskID: 20, UserID: 7}, interaction.Read)
			authoritySQLCheck(t, err)
			want := interactiondomain.Superseded
			if phase == "CANCELLED" {
				want = interactiondomain.Cancelled
			}
			if target.ClosedAttemptState != want {
				t.Fatalf("missing closure authority for %s: %+v", phase, target)
			}
		})
	}
	t.Run("rejected_inventory_keeps_history_readable_without_input_authority", func(t *testing.T) {
		tx := begin(t)
		_, err := tx.Exec(ctx, `UPDATE workforce_worker_images SET admission_status='REJECTED'`)
		authoritySQLCheck(t, err)
		u := interaction.User{TenantID: 10, TaskID: 20, UserID: 7, CanContinue: true}
		target, err := authority.AuthorizeUser(ctx, tx, u, interaction.Read)
		if err != nil || target.TaskID != 20 || len(target.Capabilities) != 0 {
			t.Fatalf("history should remain readable without runtime capabilities: %+v, %v", target, err)
		}
		for _, action := range target.AllowedActions {
			if action != interaction.DecideDelivery && action != interaction.RestartDelivery {
				t.Fatalf("unavailable runtime granted input action %s", action)
			}
		}
		for _, action := range []string{interaction.Answer, interaction.SendMessage, interaction.DecideOperation} {
			if _, err := authority.AuthorizeUser(ctx, tx, u, action); !errors.Is(err, interactiondomain.ErrUnavailable) {
				t.Fatalf("unavailable runtime must deny %s: %v", action, err)
			}
		}
	})
	for _, phase := range []string{"RUNNING", "VALIDATING", "REVIEWING"} {
		t.Run("actual_phase_"+phase, func(t *testing.T) {
			tx := begin(t)
			_, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status=$1`, phase)
			authoritySQLCheck(t, err)
			target, err := authority.Validate(ctx, tx, machine)
			if err != nil {
				t.Fatalf("actual authority rejected %s: %v", phase, err)
			}
			if target.TenantID != 10 || target.TaskID != 20 || target.TaskTurnID != 30 || target.AttemptID != 40 || target.AdapterSessionGeneration != 1 || !reflect.DeepEqual(target.Capabilities, caps) || target.AcceptsNativeInput != (phase == "RUNNING") {
				t.Fatalf("incorrect hydrated target: %+v", target)
			}
			var worker int64
			authoritySQLCheck(t, tx.QueryRow(ctx, `SELECT worker_id FROM workforce_principals WHERE id=$1 AND principal_type='WORKER'`, target.PrincipalID).Scan(&worker))
			if worker != 8 {
				t.Fatal("principal does not bind authenticated Worker")
			}
		})
	}
	mutations := map[string]string{
		"task_tenant":               `UPDATE workforce_tasks SET tenant_id=11`,
		"current_turn":              `UPDATE workforce_tasks SET current_task_turn_id=31`,
		"cancel_requested":          `UPDATE workforce_tasks SET cancel_requested_at=CURRENT_TIMESTAMP`,
		"cancelled":                 `UPDATE workforce_tasks SET status='CANCELLED'`,
		"task_terminal":             `UPDATE workforce_tasks SET status='SUCCEEDED'`,
		"phantom_evaluating_status": `UPDATE workforce_tasks SET status='EVALUATING'`,
		"attempt_expired":           `UPDATE workforce_task_attempts SET lease_expires_at=CURRENT_TIMESTAMP-interval '1 second'`,
		"attempt_lease":             `UPDATE workforce_task_attempts SET lease_token='wrong'`,
		"attempt_owner":             `UPDATE workforce_task_attempts SET owner_epoch=5`,
		"attempt_generation":        `UPDATE workforce_task_attempts SET generation=4`,
		"attempt_terminal":          `UPDATE workforce_task_attempts SET status='FAILED'`,
		"runtime_worker":            `UPDATE workforce_attempt_runtimes SET worker_id=9`,
		"runtime_instance":          `UPDATE workforce_attempt_runtimes SET worker_instance_id='other'`,
		"runtime_boot":              `UPDATE workforce_attempt_runtimes SET host_boot_id='other'`,
		"runtime_generation":        `UPDATE workforce_attempt_runtimes SET generation=4`,
		"runtime_owner":             `UPDATE workforce_attempt_runtimes SET owner_epoch=5`,
		"runtime_session":           `UPDATE workforce_attempt_runtimes SET adapter_session_generation=2`,
		"runtime_lease":             `UPDATE workforce_attempt_runtimes SET execution_lease_sha256='wrong'`,
		"runtime_expired":           `UPDATE workforce_attempt_runtimes SET execution_lease_expires_at=CURRENT_TIMESTAMP-interval '1 second'`,
		"runtime_null_lease":        `UPDATE workforce_attempt_runtimes SET execution_lease_sha256=NULL`,
		"runtime_null_expiry":       `UPDATE workforce_attempt_runtimes SET execution_lease_expires_at=NULL`,
		"runtime_lost":              `UPDATE workforce_attempt_runtimes SET status='LOST'`,
		"runtime_fenced":            `UPDATE workforce_attempt_runtimes SET fenced_at=CURRENT_TIMESTAMP`,
		"runtime_terminating":       `UPDATE workforce_attempt_runtimes SET terminating_at=CURRENT_TIMESTAMP`,
		"runtime_missing":           `DELETE FROM workforce_attempt_runtimes`,
		"worker_disabled":           `UPDATE workforce_workers SET desired_state='DISABLED'`,
		"worker_decommissioned":     `UPDATE workforce_workers SET decommissioned_at=CURRENT_TIMESTAMP`,
		"worker_new_instance":       `UPDATE workforce_workers SET current_instance_id='other'`,
		"session_ended":             `UPDATE workforce_worker_sessions SET ended_at=CURRENT_TIMESTAMP`,
		"session_worker":            `UPDATE workforce_worker_sessions SET worker_id=9`,
		"session_boot":              `UPDATE workforce_worker_sessions SET host_boot_id='other'`,
		"session_reconciling":       `UPDATE workforce_worker_sessions SET session_state='RECONCILING'`,
		"image_wrong_role":          `UPDATE workforce_worker_images SET role='provider-proxy',execution_kinds_json='[]'`,
		"image_not_admitted":        `UPDATE workforce_worker_images SET admission_status='REJECTED'`,
		"image_not_present":         `UPDATE workforce_worker_images SET presence_status='MISSING'`,
		"image_wrong_instance":      `UPDATE workforce_worker_images SET instance_id='other'`,
		"image_wrong_digest":        `UPDATE workforce_attempt_runtimes SET harness_image_digest='other'`,
		"image_missing":             `DELETE FROM workforce_worker_images`,
		"image_bad_manifest":        `UPDATE workforce_worker_images SET manifest_json='{}'`,
	}
	for name, sql := range mutations {
		t.Run("fence_"+name, func(t *testing.T) {
			tx := begin(t)
			_, err := tx.Exec(ctx, sql)
			authoritySQLCheck(t, err)
			_, err = authority.Validate(ctx, tx, machine)
			if !errors.Is(err, interactiondomain.ErrFenced) {
				t.Fatalf("expected fencing, got %v", err)
			}
			// An undefined column hidden by ErrFenced must still fail this test.
			_, err = tx.Exec(ctx, "SELECT 1")
			authoritySQLCheck(t, err)
		})
	}
	for _, field := range []string{"tenant", "task", "turn", "worker", "instance", "boot", "generation", "owner", "session", "lease"} {
		t.Run("machine_"+field, func(t *testing.T) {
			m := machine
			switch field {
			case "tenant":
				m.TenantID++
			case "task":
				m.TaskID++
			case "turn":
				m.TaskTurnID++
			case "worker":
				m.WorkerID++
			case "instance":
				m.InstanceID = "other"
			case "boot":
				m.HostBootID = "other"
			case "generation":
				m.Generation++
			case "owner":
				m.OwnerEpoch++
			case "session":
				m.AdapterSessionGeneration++
			case "lease":
				m.ExecutionLease = "other"
			}
			_, err := authority.Validate(ctx, begin(t), m)
			if !errors.Is(err, interactiondomain.ErrFenced) {
				t.Fatalf("expected fencing, got %v", err)
			}
		})
	}
	t.Run("optional_wire_scope_uses_database_tenant", func(t *testing.T) {
		m := machine
		m.TenantID, m.TaskID, m.TaskTurnID = 0, 0, 0
		target, err := authority.Validate(ctx, begin(t), m)
		if err != nil || target.TenantID != 10 || target.TaskID != 20 || target.TaskTurnID != 30 {
			t.Fatal("database scope hydration failed")
		}
	})
	t.Run("draining_worker_keeps_existing_authority", func(t *testing.T) {
		tx := begin(t)
		_, err := tx.Exec(ctx, `UPDATE workforce_workers SET desired_state='DRAINING'`)
		authoritySQLCheck(t, err)
		_, err = authority.Validate(ctx, tx, machine)
		if err != nil {
			t.Fatalf("existing execution fenced during Worker drain: %v", err)
		}
	})
	t.Run("capabilities_not_invented", func(t *testing.T) {
		tx := begin(t)
		minimal := []string{"execution.start", "execution.interrupt"}
		_, err := tx.Exec(ctx, `UPDATE workforce_worker_images SET manifest_json=$1,capabilities_json='{"interaction.respond":true}'`, manifest(minimal))
		authoritySQLCheck(t, err)
		target, err := authority.Validate(ctx, tx, machine)
		if err != nil || !reflect.DeepEqual(target.Capabilities, minimal) {
			t.Fatal("capabilities did not come exclusively from admitted descriptor")
		}
	})
	t.Run("session_binding_is_transactional", func(t *testing.T) {
		tx := begin(t)
		_, err := tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET adapter_session_generation=NULL`)
		authoritySQLCheck(t, err)
		_, err = tx.Exec(ctx, "SAVEPOINT before_binding")
		authoritySQLCheck(t, err)
		_, err = authority.Validate(ctx, tx, machine)
		if err != nil {
			t.Fatalf("first session binding failed: %v", err)
		}
		var generation int64
		authoritySQLCheck(t, tx.QueryRow(ctx, `SELECT adapter_session_generation FROM workforce_attempt_runtimes`).Scan(&generation))
		if generation != 1 {
			t.Fatal("session not persisted")
		}
		_, err = tx.Exec(ctx, "ROLLBACK TO before_binding")
		authoritySQLCheck(t, err)
		_, err = tx.Exec(ctx, `DELETE FROM workforce_worker_images`)
		authoritySQLCheck(t, err)
		_, err = authority.Validate(ctx, tx, machine)
		if !errors.Is(err, interactiondomain.ErrFenced) {
			t.Fatal("missing admitted image did not fence")
		}
		_, err = tx.Exec(ctx, "ROLLBACK TO before_binding")
		authoritySQLCheck(t, err)
		var unbound bool
		authoritySQLCheck(t, tx.QueryRow(ctx, `SELECT adapter_session_generation IS NULL FROM workforce_attempt_runtimes`).Scan(&unbound))
		if !unbound {
			t.Fatal("rolled-back session binding leaked")
		}
	})
}

// Do not include DSNs, database error detail or fixture bodies in diagnostics.
func authoritySQLCheck(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		t.Fatalf("integration SQL failed (SQLSTATE %s)", pgErr.Code)
	}
	t.Fatalf("integration operation failed (%T)", err)
}
