package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	completionapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/application"
	completion "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	completioninfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/infrastructure"
	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	continuationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/infrastructure"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"github.com/bwmarrin/snowflake"
)

// Real authority + resolver + revision application/store + continuation creator.
// No mock authority, no invented interaction/question, no provider/container call.
func TestPostgreSQLCompletionRevisionSafePoint(t *testing.T) {
	for _, decision := range []string{"PASS", "WAIT_USER"} {
		t.Run(decision, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			pool := dispatchQATables(t, "workforce_tasks", "workforce_task_turns", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_execution_nodes", "workforce_task_assignments", "workforce_workers", "workforce_worker_sessions", "workforce_task_input_snapshots", "workforce_task_input_references", "workforce_task_git_plans", "workforce_result_evaluation_cycles", "workforce_continuation_checkpoints", "workforce_continuation_restore_runs", "workforce_task_events")
			exec := func(sql string, args ...any) {
				t.Helper()
				_, err := pool.Exec(ctx, sql, args...)
				authoritySQLCheck(t, err)
			}
			// LIKE copies nextval defaults; replace this one with QA-owned storage so
			// even event sequence allocation cannot touch a public sequence.
			exec(`CREATE SEQUENCE qa_event_ids`)
			exec(`ALTER TABLE workforce_task_events ALTER COLUMN id SET DEFAULT nextval('qa_event_ids')`)
			// Fresh baseline predates the unpublished Batch3 extension. Apply the
			// exact module schema only to QA tables, never align the shared database.
			exec(continuationinfra.InteractionSchemaSQL)
			exec(completioninfra.RevisionSchemaSQL)
			digest := strings.Repeat("a", 64)
			processor := strings.Repeat("b", 64)
			base := completion.Input{Mode: "COMMAND", Command: "go test ./...", Instructions: "existing checks"}
			next := base
			next.Instructions += "\nadditional checks"
			b, _ := completion.Freeze(&base)
			n, _ := completion.Freeze(&next)
			baseCheck, _ := completion.FrozenDigest(base, nil)
			nextCheck, _ := completion.FrozenDigest(next, nil)
			baseJSON, _ := json.Marshal(base)
			nextJSON, _ := json.Marshal(next)
			taskState, attemptState, runtimeState, turnState := "VALIDATING", "RUNNING", "RUNNING", "ACTIVE"
			if decision == "WAIT_USER" {
				taskState, attemptState, runtimeState, turnState = "REVIEWING", "WAITING_USER", "SUCCEEDED", "REVIEWING"
			}
			exec(`INSERT INTO workforce_workers(id,name,token_hint,current_instance_id) VALUES(8,'qa','qa','qa-instance')`)
			exec(`INSERT INTO workforce_worker_sessions(instance_id,worker_id,host_boot_id,started_at) VALUES('qa-instance',8,'qa-boot',CURRENT_TIMESTAMP)`)
			exec(`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,status,current_task_turn_id,latest_attempt_id) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','original instruction','qa',$1,30,40)`, taskState)
			exec(`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,environment_config_digest,environment_snapshot_json,secret_version_identities,parameters_json,status,completion_check_json,completion_check_input_digest,completion_check_digest,completion_check_references_json,completion_check_baseline_result_id) VALUES(30,20,10,1,'original instruction',$1,30,7,90,$2,$1,'{"mandatoryGate":"preserved"}','[]','{"qa":true}',$3,$4,$5,$6,'[]',99)`, digest, processor, turnState, baseJSON, b.Digest, baseCheck)
			exec(`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,execution_node_id,assignment_id) VALUES(40,20,30,1,8,'/qa',$1,$2,CURRENT_TIMESTAMP+interval '1 hour','docker','qa-instance','qa-boot',3,4,50,60)`, attemptState, hashToken("qa-lease"))
			exec(`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,status,execution_lease_sha256,execution_lease_expires_at) VALUES(40,50,60,8,'qa-instance','qa-boot',3,4,$1,'qa',$1,$2,$3,CURRENT_TIMESTAMP+interval '1 hour')`, digest, runtimeState, hashToken("qa-lease"))
			exec(`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest) VALUES(50,20,30,'main','RUNNING',3,$1)`, digest)
			exec(`INSERT INTO workforce_task_assignments(id,task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,lease_token_sha256,lease_expires_at,status,message_id) VALUES(60,20,50,8,'qa-instance',3,$1,$1,CURRENT_TIMESTAMP+interval '1 hour','CLAIMED',70)`, digest)
			exec(`INSERT INTO workforce_task_input_snapshots(id,tenant_id,task_id,task_turn_id,source_kind,source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest) VALUES(80,10,20,30,'GIT',$1,$1,$2,'original/input',$2,$2,123,$2)`, strings.Repeat("d", 40), digest)
			exec(`INSERT INTO workforce_task_git_plans(id,tenant_id,task_id,task_turn_id,source_commit,delivery_mode,plan_digest) VALUES(81,10,20,30,$1,'NONE',$2)`, strings.Repeat("d", 40), digest)
			exec(`INSERT INTO workforce_result_evaluation_cycles(id,task_id,task_turn_id,execution_node_id,attempt_id,cycle_number,session_digest,contract_digest,decision,reason_code,contract_json,persisted_at,completed_at) VALUES(100,20,30,50,40,1,'qa-session',$1,$2,'QA','{}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, digest, decision)
			ci := checkpoint.Input{TaskID: 20, TaskTurnID: 30, AttemptID: 40, Generation: 3, EvaluationCycleID: 100, Decision: decision, SessionDigest: "qa-session"}
			identity := checkpoint.Identity(ci)
			bundle, overlay, sessionKey, manifest := checkpoint.ObjectKeysV2(ci, identity)
			exec(`INSERT INTO workforce_continuation_checkpoints(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,decision,checkpoint_identity_sha256,purpose,repository_bundle_object_key,repository_bundle_object_sha256,workspace_overlay_object_key,workspace_overlay_object_sha256,session_object_key,session_sha256,session_object_sha256,identity_manifest_object_key,manifest_sha256,manifest_object_sha256,task_execution_snapshot_digest,encryption_algorithm,encryption_key_id,wrapped_data_key,status,expires_at,available_at) VALUES(110,20,30,40,100,$1,$2,'RESULT',$3,$4,$5,$4,$6,$4,$4,$7,$4,$4,$4,'AES-256-GCM-CHUNKED','qa','qa','AVAILABLE',CURRENT_TIMESTAMP+interval '1 hour',CURRENT_TIMESTAMP)`, decision, identity, bundle, digest, overlay, sessionKey, manifest)
			exec(`INSERT INTO workforce_completion_check_revisions(id,tenant_id,task_id,source_task_turn_id,source_attempt_id,source_version,revision_number,version,status,creator_user_id,actor_principal_id,idempotency_key,request_digest,base_input_json,base_digest,input_json,input_digest,check_digest,references_json,source_baseline_result_id,source_commit,source_identity_digest,execution_snapshot_digest,processor_release_id,processor_digest) VALUES(120,10,20,30,40,1,1,1,'PENDING',7,9,'qa',$1,$2,$3,$4,$5,$6,'[]',99,$7,$1,$1,90,$8)`, digest, baseJSON, baseCheck, nextJSON, n.Digest, nextCheck, strings.Repeat("d", 40), processor)
			node, err := snowflake.NewNode(902)
			authoritySQLCheck(t, err)
			ids := authorityTestIDs{node}
			authority := NewRevisionSafePointAuthority()
			service := completionapp.NewRevisions(pool, ids, completioninfra.NewRevisionStore(), principalinfra.NewResolver(), authority, continuationinfra.New(pool, ids))
			authority.WithRevisionLookup(service)
			m := completion.MachineAuthority{WorkerID: 8, InstanceID: "qa-instance", HostBootID: "qa-boot", Generation: 3, OwnerEpoch: 4}
			point := completion.RevisionSafePoint{TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, CheckpointID: 110, RevisionID: 120, ExpectedVersion: 1, Authority: m, ExecutionLease: "qa-lease"}
			for name, sql := range map[string]string{
				"cancel":                   `UPDATE workforce_tasks SET cancel_requested_at=CURRENT_TIMESTAMP`,
				"worker_ended":             `UPDATE workforce_worker_sessions SET ended_at=CURRENT_TIMESTAMP`,
				"runtime_owner":            `UPDATE workforce_attempt_runtimes SET owner_epoch=5`,
				"runtime_expiry":           `UPDATE workforce_attempt_runtimes SET execution_lease_expires_at=CURRENT_TIMESTAMP-interval '1 second'`,
				"attempt_owner":            `UPDATE workforce_task_attempts SET owner_epoch=5`,
				"checkpoint_not_available": `UPDATE workforce_continuation_checkpoints SET status='PENDING'`,
				"checkpoint_expired":       `UPDATE workforce_continuation_checkpoints SET expires_at=CURRENT_TIMESTAMP-interval '1 second'`,
				"checkpoint_other_attempt": `UPDATE workforce_continuation_checkpoints SET attempt_id=41`,
				"checkpoint_identity":      `UPDATE workforce_continuation_checkpoints SET checkpoint_identity_sha256=repeat('f',64)`,
				"checkpoint_snapshot":      `UPDATE workforce_continuation_checkpoints SET task_execution_snapshot_digest=repeat('f',64)`,
				"checkpoint_key":           `UPDATE workforce_continuation_checkpoints SET repository_bundle_object_key='other'`,
				"cycle_decision":           `UPDATE workforce_result_evaluation_cycles SET decision='REVISE'`,
				"cycle_not_persisted":      `UPDATE workforce_result_evaluation_cycles SET persisted_at=NULL`,
				"newer_cycle":              `INSERT INTO workforce_result_evaluation_cycles(id,task_id,task_turn_id,execution_node_id,attempt_id,cycle_number,contract_digest,decision,reason_code,contract_json) VALUES(101,20,30,50,40,2,repeat('a',64),'REVISE','QA','{}')`,
			} {
				t.Run(name, func(t *testing.T) {
					tx, e := pool.Begin(ctx)
					authoritySQLCheck(t, e)
					defer tx.Rollback(ctx)
					_, e = tx.Exec(ctx, sql)
					authoritySQLCheck(t, e)
					e = authority.ValidateRevisionSafePoint(ctx, tx, point)
					if !errors.Is(e, completion.ErrStale) {
						t.Fatalf("expected stale authority: %v", e)
					}
					_, e = tx.Exec(ctx, `SELECT 1`)
					authoritySQLCheck(t, e)
				})
			}
			t.Run("authorized_no_pending_is_not_apply", func(t *testing.T) {
				tx, e := pool.Begin(ctx)
				authoritySQLCheck(t, e)
				defer tx.Rollback(ctx)
				_, e = tx.Exec(ctx, `DELETE FROM workforce_completion_check_revisions`)
				authoritySQLCheck(t, e)
				p, e := authority.ResolveRevisionSafePoint(ctx, tx, m, 40, "qa-lease", 100)
				if e != nil || p != nil {
					t.Fatalf("no pending: %v", e)
				}
				bad := m
				bad.OwnerEpoch++
				_, e = authority.ResolveRevisionSafePoint(ctx, tx, bad, 40, "qa-lease", 100)
				if !errors.Is(e, completion.ErrStale) {
					t.Fatal("no pending bypassed authorization")
				}
			})
			t.Run("rollback_after_creator", func(t *testing.T) {
				tx, e := pool.Begin(ctx)
				authoritySQLCheck(t, e)
				defer tx.Rollback(ctx)
				// Call the real creator once in a savepoint so SQL failures are
				// localized before the application deliberately redacts them.
				_, e = tx.Exec(ctx, "SAVEPOINT inspect_creator")
				authoritySQLCheck(t, e)
				_, e = continuationinfra.New(pool, ids).CreateCompletionRevisionTurnTx(ctx, tx, continuationfacade.CompletionRevisionTurnRequest{TenantID: 10, TaskID: 20, SourceTaskTurnID: 30, SourceAttemptID: 40, CheckpointID: 110, RevisionID: 120, CreatorUserID: 7, OriginalBaselineResultID: 99, SourceCommit: strings.Repeat("d", 40), SourceIdentityDigest: digest, ExecutionSnapshotDigest: digest, InputJSON: nextJSON, ReferencesJSON: json.RawMessage(`[]`), InputDigest: n.Digest, CheckDigest: nextCheck})
				if e != nil {
					t.Fatalf("real creator SQL: %v", e)
				}
				_, e = tx.Exec(ctx, "ROLLBACK TO inspect_creator")
				authoritySQLCheck(t, e)
				_, e = service.ApplyRevisionTx(ctx, tx, point)
				if e != nil {
					t.Fatalf("apply failed: %v", e)
				}
				authoritySQLCheck(t, tx.Rollback(ctx))
				var turn int64
				authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT current_task_turn_id FROM workforce_tasks`).Scan(&turn))
				if turn != 30 {
					t.Fatal("creator escaped caller transaction")
				}
			})
			t.Run("weaker_check_cannot_apply", func(t *testing.T) {
				tx, e := pool.Begin(ctx)
				authoritySQLCheck(t, e)
				defer tx.Rollback(ctx)
				weak := completion.Input{Mode: "NONE"}
				wf, _ := completion.Freeze(&weak)
				wc, _ := completion.FrozenDigest(weak, nil)
				wj, _ := json.Marshal(weak)
				_, e = tx.Exec(ctx, `UPDATE workforce_completion_check_revisions SET input_json=$1,input_digest=$2,check_digest=$3`, wj, wf.Digest, wc)
				authoritySQLCheck(t, e)
				_, e = service.ApplyRevisionTx(ctx, tx, point)
				if !errors.Is(e, completion.ErrConflict) {
					t.Fatalf("weaker check applied: %v", e)
				}
				var current int64
				authoritySQLCheck(t, tx.QueryRow(ctx, `SELECT current_task_turn_id FROM workforce_tasks`).Scan(&current))
				if current != 30 {
					t.Fatal("weak revision advanced task")
				}
			})
			out, err := service.ApplyPendingAtSafePoint(ctx, m, 40, "qa-lease", 100)
			if err != nil || !out.Applied || out.TaskTurnID == 30 {
				t.Fatalf("actual machine apply: %+v %v", out, err)
			}
			var actual string
			var newBaseline *int64
			var oldBaseline int64
			var sourceTurn, cpID int64
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT completion_check_digest,completion_check_baseline_result_id,source_turn_id,source_checkpoint_id FROM workforce_task_turns WHERE id=$1`, out.TaskTurnID).Scan(&actual, &newBaseline, &sourceTurn, &cpID))
			if actual != nextCheck || newBaseline != nil || sourceTurn != 30 || cpID != 110 {
				t.Fatal("new turn checkpoint/check/baseline mismatch")
			}
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT completion_check_baseline_result_id FROM workforce_task_turns WHERE id=30`).Scan(&oldBaseline))
			if oldBaseline != 99 {
				t.Fatal("historical baseline lost")
			}
			var sourceOK bool
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT i.source_commit=o.source_commit AND i.source_identity_digest=o.source_identity_digest AND i.repository_bundle_object_key=o.repository_bundle_object_key AND i.input_snapshot_digest=o.input_snapshot_digest FROM workforce_task_input_snapshots i JOIN workforce_task_input_snapshots o ON o.id=80 WHERE i.task_turn_id=$1`, out.TaskTurnID).Scan(&sourceOK))
			if !sourceOK {
				t.Fatal("original input was replaced by checkpoint result")
			}
			var configOK bool
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT n.instruction=o.instruction AND n.execution_snapshot_digest=o.execution_snapshot_digest AND n.processor_release_id=o.processor_release_id AND n.processor_digest=o.processor_digest AND n.environment_snapshot_json=o.environment_snapshot_json AND n.parameters_json=o.parameters_json FROM workforce_task_turns n JOIN workforce_task_turns o ON o.id=30 WHERE n.id=$1`, out.TaskTurnID).Scan(&configOK))
			if !configOK {
				t.Fatal("frozen execution/mandatory configuration changed")
			}
			replay, err := service.ApplyPendingAtSafePoint(ctx, m, 40, "qa-lease", 100)
			if err != nil || replay != out {
				t.Fatalf("lost response replay: %+v %v", replay, err)
			}
			var turns int
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT count(*) FROM workforce_task_turns`).Scan(&turns))
			if turns != 2 {
				t.Fatal("duplicate successor")
			}
			bad := m
			bad.OwnerEpoch++
			if _, err = service.ApplyPendingAtSafePoint(ctx, bad, 40, "qa-lease", 100); !errors.Is(err, completion.ErrStale) {
				t.Fatal("replay bypassed owner fence")
			}
			// Direct wrong checkpoint must not replay an already applied revision.
			tx, err := pool.Begin(ctx)
			authoritySQLCheck(t, err)
			defer tx.Rollback(ctx)
			point.CheckpointID++
			if err = authority.ValidateRevisionSafePoint(ctx, tx, point); !errors.Is(err, completion.ErrStale) {
				t.Fatal("wrong checkpoint replay accepted")
			}
		})
	}
}
