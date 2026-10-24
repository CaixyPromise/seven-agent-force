package infrastructure

import (
	"context"
	"strings"
	"testing"
	"time"

	continuationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

// Exercise the actual receipt transaction and current machine fencing against
// migration-cloned QA tables. Cold key/input metadata is tested by continuation.
func TestPostgreSQLRestoreReceiptReadback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := dispatchQATables(t, "workforce_tasks", "workforce_task_turns", "workforce_task_attempts", "workforce_workers", "workforce_worker_sessions", "workforce_continuation_checkpoints", "workforce_continuation_restore_runs", "workforce_task_events")
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		authoritySQLCheck(t, err)
	}
	exec(`CREATE SEQUENCE qa_restore_events`)
	exec(`ALTER TABLE workforce_task_events ALTER COLUMN id SET DEFAULT nextval('qa_restore_events')`)
	exec(continuationinfra.InteractionSchemaSQL)
	exec(`INSERT INTO workforce_workers(id,name,token_hint,current_instance_id) VALUES(8,'qa','qa','qa-instance')`)
	exec(`INSERT INTO workforce_worker_sessions(instance_id,worker_id,host_boot_id,started_at) VALUES('qa-instance',8,'qa-boot',CURRENT_TIMESTAMP)`)
	exec(`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,status,current_task_turn_id,latest_attempt_id) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','qa','PREPARING',30,40)`)
	exec(`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,execution_node_id,assignment_id) VALUES(40,20,30,1,8,'/qa','PREPARING',$1,CURRENT_TIMESTAMP+interval '1 hour','docker','qa-instance','qa-boot',3,4,50,60)`, hashToken("qa-lease"))
	exec(`INSERT INTO workforce_continuation_checkpoints(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,decision,checkpoint_identity_sha256,purpose,base_commit,result_commit,result_tree,session_lineage_digest,task_execution_snapshot_digest,status,expires_at,available_at,consumed_by_turn_id,consumed_at,session_object_key,session_sha256,identity_manifest_object_key,manifest_sha256) VALUES(110,20,29,39,100,'PASS',$1,'RESULT','base','result','tree','session',$1,'AVAILABLE',CURRENT_TIMESTAMP+interval '1 hour',CURRENT_TIMESTAMP,30,CURRENT_TIMESTAMP,'qa/session',$1,'qa/manifest',$1)`, strings.Repeat("a", 64))
	exec(`INSERT INTO workforce_continuation_restore_runs(id,task_id,source_turn_id,target_turn_id,execution_node_id,attempt_id,checkpoint_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,expected_identity_digest,status) VALUES(120,20,29,30,50,40,110,8,'qa-instance','qa-boot',3,4,'qa','PENDING')`)
	s := New(pool, nil)
	m := domain.MachineEnvelope{WorkerID: 8, InstanceID: "qa-instance", HostBootID: "qa-boot", Generation: 3, OwnerEpoch: 4}
	digest := strings.Repeat("b", 64)
	complete := func() error {
		return s.CompleteContinuationRestore(ctx, m, 40, "qa-lease", 120, digest, "base", "result", "tree")
	}
	authoritySQLCheck(t, complete())
	authoritySQLCheck(t, complete()) // committed response was lost; exact readback
	var count int
	authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT count(*) FROM workforce_task_events WHERE event_type='continuation.restore.completed'`).Scan(&count))
	if count != 1 {
		t.Fatalf("duplicate receipt event: %d", count)
	}
	if err := s.CompleteContinuationRestore(ctx, m, 40, "qa-lease", 120, strings.Repeat("c", 64), "base", "result", "tree"); err == nil {
		t.Fatal("different receipt accepted")
	}
	if err := s.CompleteContinuationRestore(ctx, m, 40, "qa-lease", 120, digest, "other", "result", "tree"); err == nil {
		t.Fatal("different base accepted")
	}
	for _, tc := range []struct{ name, change, undo string }{
		{"cancel", `UPDATE workforce_tasks SET cancel_requested_at=CURRENT_TIMESTAMP`, `UPDATE workforce_tasks SET cancel_requested_at=NULL`},
		{"launched", `UPDATE workforce_task_attempts SET status='RUNNING'`, `UPDATE workforce_task_attempts SET status='PREPARING'`},
		{"lease", `UPDATE workforce_task_attempts SET lease_expires_at=CURRENT_TIMESTAMP-interval '1 hour'`, `UPDATE workforce_task_attempts SET lease_expires_at=CURRENT_TIMESTAMP+interval '1 hour'`},
		{"ended-session", `UPDATE workforce_worker_sessions SET ended_at=CURRENT_TIMESTAMP`, `UPDATE workforce_worker_sessions SET ended_at=NULL`},
		{"restore-owner", `UPDATE workforce_continuation_restore_runs SET owner_epoch=5`, `UPDATE workforce_continuation_restore_runs SET owner_epoch=4`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec(tc.change)
			defer exec(tc.undo)
			if err := complete(); err == nil {
				t.Fatal("stale authority accepted")
			}
		})
	}
}
