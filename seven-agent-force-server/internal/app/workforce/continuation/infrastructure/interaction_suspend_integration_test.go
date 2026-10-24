package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

type suspendIDs struct{ n atomic.Int64 }

func (g *suspendIDs) NextID() int64 { return 1000 + g.n.Add(1) }

// Disposable schema tests the real transaction SQL, not an in-memory backend.
// It is deliberately a focused contract fixture, not global migration evidence.
func TestInteractionSuspendPostgresAtomicResume(t *testing.T) {
	dsn := os.Getenv("CONTINUATION_SUSPEND_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CONTINUATION_SUSPEND_TEST_POSTGRES_DSN to an isolated local PostgreSQL instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("suspend_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(ctx, suspendTestSchema); err != nil {
		t.Fatal(err)
	}
	store := New(db, &suspendIDs{})
	testInteractionCaptureAuthority(t, ctx, db, store)
	service := application.NewInteractionResume(store)
	r := application.InteractionResumeRequest{TenantID: 1, TaskID: 2, SourceTaskTurnID: 3, SourceAttemptID: 4, InteractionID: 5, CheckpointID: 6, AnswerID: 7, CreatorUserID: 8, Instruction: "cold answer"}
	r.InteractionResponse = []byte(`{"interactionKey":"fake:question","responseId":"7","answers":[]}`)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.FindInteractionResumeTx(ctx, tx, facade.InteractionResumeLookup{TenantID: 1, TaskID: 2, AttemptID: 4, InteractionID: 5})
	if err != nil || target == nil || target.CheckpointID != 6 {
		t.Fatalf("find %+v %v", target, err)
	}
	if _, err = service.ResumeInteractionTx(ctx, tx, r); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_task_turns WHERE source_checkpoint_id=6`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback count=%d err=%v", count, err)
	}
	// Cancellation is serialized on the same Task lock and refuses resume.
	if _, err = db.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=now() WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ResumeInteractionTx(ctx, tx, r); err == nil {
		t.Fatal("cancelled resume accepted")
	}
	_ = tx.Rollback(ctx)
	if _, err = db.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=NULL WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			tx, e := db.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			_, e = service.ResumeInteractionTx(ctx, tx, r)
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}()
	}
	success := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent successful resumes=%d", success)
	}
	var snapshot, session, phase, oldStatus string
	var consumed, current int64
	if err = db.QueryRow(ctx, `SELECT c.consumed_by_turn_id,t.current_task_turn_id,c.interaction_phase,tr.execution_snapshot_digest,c.session_lineage_digest,old.status FROM workforce_continuation_checkpoints c JOIN workforce_tasks t ON t.id=c.task_id JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id JOIN workforce_task_turns old ON old.id=3 WHERE c.id=6`).Scan(&consumed, &current, &phase, &snapshot, &session, &oldStatus); err != nil {
		t.Fatal(err)
	}
	if consumed != current || consumed <= 1000 || phase != "RESUMED" || snapshot != "frozen" || session != "private-lineage" || oldStatus != "SUPERSEDED" {
		t.Fatalf("identity/consumption mismatch: %d %d %s %s %s %s", consumed, current, phase, snapshot, session, oldStatus)
	}
	var goal, taskGoal string
	if err = db.QueryRow(ctx, `SELECT tr.instruction,t.task_instruction FROM workforce_task_turns tr JOIN workforce_tasks t ON t.current_task_turn_id=tr.id WHERE t.id=2`).Scan(&goal, &taskGoal); err != nil || goal != "original" || taskGoal != "original" {
		t.Fatalf("cold resume replaced original goal: %q %q %v", goal, taskGoal, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_task_turns WHERE review_revision_kind='AUTOMATIC'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("automatic budget changed: %d %v", count, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_task_turns WHERE source_checkpoint_id=6`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("checkpoint consumption count: %d %v", count, err)
	}
}

func testInteractionCaptureAuthority(t *testing.T, ctx context.Context, db *pgxpool.Pool, store *Store) {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	sum := sha256.Sum256([]byte("test-lease"))
	if _, err = tx.Exec(ctx, `ALTER TABLE workforce_task_attempts ADD worker_id BIGINT,ADD worker_instance_id TEXT,ADD host_boot_id TEXT,ADD generation BIGINT,ADD lease_token TEXT;
ALTER TABLE workforce_continuation_checkpoints ADD checkpoint_identity_sha256 TEXT;
CREATE TABLE workforce_workers(id BIGINT,current_instance_id TEXT);
CREATE TABLE workforce_worker_sessions(instance_id TEXT,host_boot_id TEXT,ended_at TIMESTAMPTZ);
INSERT INTO workforce_workers VALUES(20,'worker-session');INSERT INTO workforce_worker_sessions VALUES('worker-session','boot',NULL);
UPDATE workforce_tasks SET status='RUNNING';UPDATE workforce_task_turns SET status='ACTIVE';UPDATE workforce_task_attempts SET status='RUNNING';
UPDATE workforce_continuation_checkpoints SET interaction_phase='CAPTURING',status='PENDING',checkpoint_identity_sha256=repeat('a',64);`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET worker_id=20,worker_instance_id='worker-session',host_boot_id='boot',generation=1,owner_epoch=2,lease_token=$1,lease_expires_at=now()+interval '1 hour'`, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	r := application.InteractionCaptureRequest{TenantID: 1, TaskID: 2, TaskTurnID: 3, InteractionID: 5, CheckpointID: 6, CheckpointIdentity: fmt.Sprintf("%064s", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), MachineAuthority: application.MachineAuthority{WorkerID: 20, InstanceID: "worker-session", HostBootID: "boot", AttemptID: 4, Generation: 1, OwnerEpoch: 2, LeaseToken: "test-lease"}}
	if err = store.AuthorizeInteractionCaptureTx(ctx, tx, r); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*application.InteractionCaptureRequest){func(r *application.InteractionCaptureRequest) { r.OwnerEpoch++ }, func(r *application.InteractionCaptureRequest) { r.Generation++ }, func(r *application.InteractionCaptureRequest) { r.LeaseToken = "stale" }, func(r *application.InteractionCaptureRequest) { r.InstanceID = "stale" }, func(r *application.InteractionCaptureRequest) { r.InteractionID++ }} {
		bad := r
		mutate(&bad)
		if err = store.AuthorizeInteractionCaptureTx(ctx, tx, bad); err == nil {
			t.Fatal("stale capture authority accepted")
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_workers SET current_instance_id='replacement'`); err != nil {
		t.Fatal(err)
	}
	if err = store.AuthorizeInteractionCaptureTx(ctx, tx, r); err == nil {
		t.Fatal("replaced current instance accepted")
	}
}

const suspendTestSchema = `
CREATE TABLE workforce_tasks(id BIGINT PRIMARY KEY,tenant_id BIGINT,current_task_turn_id BIGINT,latest_attempt_id BIGINT,current_turn_number INT,status TEXT,cancel_requested_at TIMESTAMPTZ,task_instruction TEXT,result_summary TEXT,started_at TIMESTAMPTZ,finished_at TIMESTAMPTZ,verified_at TIMESTAMPTZ,queued_at TIMESTAMPTZ,version BIGINT DEFAULT 1,project_id BIGINT,project_version_id BIGINT,review_enabled BOOLEAN NOT NULL DEFAULT TRUE);
CREATE TABLE workforce_task_turns(id BIGINT PRIMARY KEY,task_id BIGINT,tenant_id BIGINT,turn_number INT,instruction TEXT,status TEXT,source_turn_id BIGINT,source_checkpoint_id BIGINT UNIQUE,source_execution_operation_id BIGINT,execution_snapshot_digest TEXT,continuation_lineage_id BIGINT,created_by BIGINT,processor_release_id BIGINT,processor_digest TEXT,environment_config_digest TEXT,environment_snapshot_json JSONB,secret_version_identities JSONB,parameters_json JSONB,idempotency_key TEXT,request_digest TEXT,result_commit TEXT,result_tree TEXT,result_summary TEXT,created_at TIMESTAMPTZ DEFAULT now(),started_at TIMESTAMPTZ,finished_at TIMESTAMPTZ,completion_check_json JSONB,completion_check_digest TEXT,completion_check_references_json JSONB,completion_check_input_digest TEXT,completion_check_baseline_result_id BIGINT,review_revision_kind TEXT,UNIQUE(task_id,idempotency_key));
CREATE TABLE workforce_continuation_checkpoints(id BIGINT PRIMARY KEY,task_id BIGINT,task_turn_id BIGINT,attempt_id BIGINT,interaction_id BIGINT UNIQUE,operation_id BIGINT,purpose TEXT,interaction_phase TEXT,format_version INT,decision TEXT,status TEXT,expires_at TIMESTAMPTZ,consumed_by_turn_id BIGINT,consumed_at TIMESTAMPTZ,approved_at TIMESTAMPTZ,task_execution_snapshot_digest TEXT,session_lineage_digest TEXT);
CREATE TABLE workforce_task_attempts(id BIGINT PRIMARY KEY,task_id BIGINT,task_turn_id BIGINT,status TEXT,finished_at TIMESTAMPTZ,resolved_execution_snapshot_json JSONB NOT NULL DEFAULT '{"executionDeadlineSeconds":600}'::jsonb);
ALTER TABLE workforce_task_attempts ADD owner_epoch BIGINT DEFAULT 2,ADD lease_expires_at TIMESTAMPTZ DEFAULT now(),ADD assignment_id BIGINT DEFAULT 15;
CREATE TABLE workforce_attempt_runtimes(attempt_id BIGINT PRIMARY KEY,status TEXT,owner_epoch BIGINT DEFAULT 2,execution_lease_expires_at TIMESTAMPTZ,launch_permit_expires_at TIMESTAMPTZ,run_permit_expires_at TIMESTAMPTZ,reattach_permit_expires_at TIMESTAMPTZ,fenced_at TIMESTAMPTZ,terminal_reason_code TEXT,updated_at TIMESTAMPTZ);
CREATE TABLE workforce_task_assignments(id BIGINT PRIMARY KEY,status TEXT,completed_at TIMESTAMPTZ,release_reason_code TEXT,delivery_disposition TEXT);
CREATE TABLE workforce_execution_nodes(id BIGINT PRIMARY KEY,task_id BIGINT,task_turn_id BIGINT,node_key TEXT,status TEXT,generation BIGINT,execution_requirement_digest TEXT,wait_reason_code TEXT,max_attempts INT,retry_count INT,updated_at TIMESTAMPTZ);
CREATE TABLE workforce_task_input_snapshots(id BIGINT PRIMARY KEY,tenant_id BIGINT,task_id BIGINT,task_turn_id BIGINT,source_kind TEXT,workspace_source_revision_id BIGINT,source_commit TEXT,source_tree TEXT,source_identity_digest TEXT,repository_bundle_object_key TEXT,repository_bundle_content_sha256 TEXT,repository_bundle_object_sha256 TEXT,repository_bundle_size_bytes BIGINT,input_snapshot_digest TEXT);
CREATE TABLE workforce_task_input_references(task_input_snapshot_id BIGINT,reference_asset_id BIGINT,reference_content_sha256 TEXT,display_order INT);
CREATE TABLE workforce_task_git_plans(id BIGINT PRIMARY KEY,tenant_id BIGINT,task_id BIGINT,task_turn_id BIGINT,project_id BIGINT,project_version_id BIGINT,repository_provider_type TEXT,repository_scheme TEXT,repository_host TEXT,repository_port INT,repository_path TEXT,repository_display_url TEXT,repository_web_url TEXT,repository_config_digest TEXT,repository_credential_ciphertext TEXT,repository_credential_edek TEXT,repository_credential_wrap_key_ref TEXT,repository_credential_version BIGINT,review_base_source_revision_id BIGINT,source_ref_type TEXT,source_ref_name TEXT,source_commit TEXT,review_base_branch TEXT,review_base_commit TEXT,result_branch TEXT,delivery_mode TEXT,delivery_authorization TEXT,delivery_authorized_by_principal_id BIGINT,delivery_authorized_at TIMESTAMPTZ,target_config_digest TEXT,push_authorization_digest TEXT,review_authorization_digest TEXT,full_project_import BOOL,previous_delivery_commit TEXT,plan_digest TEXT,status TEXT);
CREATE TABLE workforce_delivery_requests(task_turn_id BIGINT,delivery_commit TEXT,status TEXT,finished_at TIMESTAMPTZ);
CREATE TABLE workforce_task_events(task_id BIGINT,attempt_id BIGINT,event_type TEXT,sequence BIGINT,payload_json JSONB,UNIQUE(task_id,sequence));
ALTER TABLE workforce_task_turns ADD source_interaction_response_id BIGINT,ADD interaction_response_json JSONB;
INSERT INTO workforce_tasks(id,tenant_id,current_task_turn_id,latest_attempt_id,current_turn_number,status)VALUES(2,1,3,4,1,'RUNNING');
INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,status,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,environment_config_digest,environment_snapshot_json,secret_version_identities,parameters_json,review_revision_kind)VALUES(3,2,1,1,'original','ACTIVE','frozen',9,8,10,'processor','env','{}','[]','{}','AUTOMATIC');
INSERT INTO workforce_continuation_checkpoints VALUES(6,2,3,4,5,NULL,'INTERACTION_SUSPEND','SUSPENDED',2,'','AVAILABLE',now()+interval '1 hour',NULL,NULL,NULL,'frozen','private-lineage');
INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,status)VALUES(4,2,3,'RUNNING');
INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,status)VALUES(11,2,3,'RUNNING');
INSERT INTO workforce_task_input_snapshots(id,tenant_id,task_id,task_turn_id,source_kind,input_snapshot_digest)VALUES(12,1,2,3,'EMPTY_WORKSPACE','input');
INSERT INTO workforce_task_git_plans(id,tenant_id,task_id,task_turn_id)VALUES(13,1,2,3);
INSERT INTO workforce_attempt_runtimes(attempt_id,status,execution_lease_expires_at,fenced_at)VALUES(4,'INTERACTION_SUSPENDED',now(),now());
INSERT INTO workforce_task_assignments(id,status)VALUES(15,'CLAIMED');
UPDATE workforce_tasks SET status='INTERACTION_SUSPENDED';UPDATE workforce_task_turns SET status='INTERACTION_SUSPENDED';UPDATE workforce_task_attempts SET status='INTERACTION_SUSPENDED';UPDATE workforce_execution_nodes SET status='INTERACTION_SUSPENDED';
`
