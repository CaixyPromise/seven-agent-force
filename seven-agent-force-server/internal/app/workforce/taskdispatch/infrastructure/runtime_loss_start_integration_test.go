package infrastructure

import (
	"context"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	notificationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
	notification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

type runtimeLossNotificationRecorder struct {
	transitions []notification.TaskNotificationTransition
}

func (r *runtimeLossNotificationRecorder) RecordTransition(_ context.Context, _ pgx.Tx, tr notification.TaskNotificationTransition) (notification.TransitionDisposition, error) {
	decision, err := notificationdomain.ClassifyTransition(tr)
	if err == nil {
		r.transitions = append(r.transitions, tr)
	}
	return decision.Disposition, err
}

func TestPostgreSQLRuntimeLossNeverImplicitlyReexecutesStartedOrUncertainAgent(t *testing.T) {
	for _, tc := range []struct{ name, status, previous, taskStatus, mutation, event, want string }{
		{"pre-harness", "SUPERVISOR_READY", "", "PREPARING", "", "", "RETRY_SCHEDULED"},
		{"pre-harness-recovery", "RECOVERING", "SUPERVISOR_READY", "PREPARING", "", "", "RETRY_SCHEDULED"},
		{"started-ack", "RECOVERING", "RUNNING", "RUNNING", `harness_started_at=CURRENT_TIMESTAMP,harness_pid=123,run_nonce_accepted_at=CURRENT_TIMESTAMP`, "", domain.ReasonRuntimeLostAfterHarnessStart},
		{"nonce-accepted", "RECOVERING", "SUPERVISOR_READY", "PREPARING", `run_nonce_accepted_at=CURRENT_TIMESTAMP`, "", domain.ReasonRuntimeLostAfterHarnessStart},
		{"permit-issued-ack-lost", "STARTING", "", "PREPARING", `run_permit_id=91,run_permit_issued_at=CURRENT_TIMESTAMP,run_nonce_sha256='test-digest'`, "", domain.ReasonRuntimeStartOutcomeUnknown},
		{"permit-history-survives-clear", "SUPERVISOR_READY", "", "PREPARING", "", "runtime.run_permit_issued", domain.ReasonRuntimeStartOutcomeUnknown},
		{"started-history-survives-clear", "SUPERVISOR_READY", "", "PREPARING", "", "runtime.harness_started", domain.ReasonRuntimeLostAfterHarnessStart},
		{"unknown-recovery", "RECOVERING", "", "PREPARING", "", "", domain.ReasonRuntimeStartOutcomeUnknown},
		{"cancel-wins", "RUNNING", "", "CANCELLING", `harness_started_at=CURRENT_TIMESTAMP`, "", "CANCELLED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			pool := dispatchQATables(t, "workforce_tasks", "workforce_task_turns", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_execution_nodes", "workforce_task_assignments", "workforce_finalization_runs", "workforce_task_events")
			// LIKE INCLUDING ALL may copy a public SERIAL default. Replace it before
			// any event insert, so even sequence writes remain inside this QA schema.
			for index, sql := range []string{`CREATE SEQUENCE qa_runtime_loss_event_ids`, `ALTER TABLE workforce_task_events ALTER COLUMN id DROP IDENTITY IF EXISTS`, `ALTER TABLE workforce_task_events ALTER COLUMN id SET DEFAULT nextval('qa_runtime_loss_event_ids'::regclass)`,
				`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,attempt_count,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest,execution_timeout_seconds,execution_active_milliseconds) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','PREPARING',30,1,81,82,83,'{}','` + strings.Repeat("a", 64) + `',60,40000)`,
				`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest) VALUES(30,20,10,1,'qa','qa',30,7,81,'qa')`,
				`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,execution_node_id,assignment_id,assignment_selection_digest,resolved_execution_snapshot_json) VALUES(40,20,30,1,8,'/qa','PREPARING','qa',CURRENT_TIMESTAMP+interval '1 hour','docker','qa-instance','qa-boot',3,5,50,60,'` + strings.Repeat("a", 64) + `','{}')`,
				`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,status,execution_lease_sha256,execution_lease_expires_at) VALUES(40,50,60,8,'qa-instance','qa-boot',3,5,'qa','qa','qa','SUPERVISOR_READY','qa',CURRENT_TIMESTAMP+interval '1 hour')`,
				`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,max_attempts,execution_requirement_digest) VALUES(50,20,30,'qa','PREPARING',3,3,'qa')`,
				`INSERT INTO workforce_task_assignments(id,task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,agent_runtime_release_id,runtime_image_ref,runtime_image_digest,adapter_identity,adapter_version,adapter_protocol,model_credential_id,credential_version,model_protocol,primary_model,provider_proxy_identity,worker_image_contract,isolation_profile,resolved_execution_snapshot_json,selection_digest,lease_token_sha256,lease_expires_at,status,message_id) VALUES(60,20,50,8,'qa-instance',3,'qa',84,'qa@sha256:` + strings.Repeat("a", 64) + `','sha256:` + strings.Repeat("a", 64) + `','qa','1','seven.agent-adapter/v1',85,1,'OPENAI','qa','qa','seven.worker-image/v1','qa','{}','` + strings.Repeat("a", 64) + `','qa',CURRENT_TIMESTAMP+interval '1 hour','CLAIMED',70)`,
			} {
				_, err := pool.Exec(ctx, sql)
				if err != nil {
					t.Fatalf("fixture statement %d: %v", index, err)
				}
			}
			_, err := pool.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status=$1,recovery_previous_status=$2 WHERE attempt_id=40`, tc.status, tc.previous)
			authoritySQLCheck(t, err)
			_, err = pool.Exec(ctx, `UPDATE workforce_tasks SET status=$1 WHERE id=20`, tc.taskStatus)
			authoritySQLCheck(t, err)
			if tc.mutation != "" {
				_, err = pool.Exec(ctx, `UPDATE workforce_attempt_runtimes SET `+tc.mutation+` WHERE attempt_id=40`)
				authoritySQLCheck(t, err)
			}
			if tc.event != "" {
				_, err = pool.Exec(ctx, `INSERT INTO workforce_task_events(task_id,attempt_id,event_type,sequence,payload_json) VALUES(20,40,$1,1,'{}')`, tc.event)
				authoritySQLCheck(t, err)
			}
			var beforeStarted, beforeNonce, beforePermit bool
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT harness_started_at IS NOT NULL,run_nonce_accepted_at IS NOT NULL,run_permit_id IS NOT NULL FROM workforce_attempt_runtimes WHERE attempt_id=40`).Scan(&beforeStarted, &beforeNonce, &beforePermit))
			s := New(pool, nil)
			recorder := &runtimeLossNotificationRecorder{}
			s.SetTaskNotificationRecorder(recorder)
			tx, err := pool.Begin(ctx)
			authoritySQLCheck(t, err)
			defer tx.Rollback(ctx)
			result, err := s.fenceRuntimeLoss(ctx, tx, 40, domain.ReasonRuntimeMissing, "qa-evidence-digest")
			authoritySQLCheck(t, err)
			authoritySQLCheck(t, tx.Commit(ctx))
			var taskState, nodeState, retryClass string
			var gen, owner int64
			var confirmedActiveMillis int64
			var attemptCount, retryEvents, failedEvents int
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT t.status,t.attempt_count,t.execution_active_milliseconds,n.status,n.generation,a.owner_epoch,a.retry_class FROM workforce_tasks t JOIN workforce_execution_nodes n ON n.task_id=t.id JOIN workforce_task_attempts a ON a.task_id=t.id WHERE a.id=40`).Scan(&taskState, &attemptCount, &confirmedActiveMillis, &nodeState, &gen, &owner, &retryClass))
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event_type='attempt.retry_scheduled'),count(*) FILTER(WHERE event_type='task.failed') FROM workforce_task_events WHERE attempt_id=40`).Scan(&retryEvents, &failedEvents))
			if owner != 6 || attemptCount != 1 || confirmedActiveMillis != 40000 {
				t.Fatal("fencing or attempt evidence changed incorrectly")
			}
			switch tc.want {
			case "RETRY_SCHEDULED":
				if result.Disposition != tc.want || gen != 4 || taskState != "RETRY_WAIT" || retryEvents != 1 || failedEvents != 0 {
					t.Fatalf("pre-Harness retry lost: %+v %s %d", result, taskState, gen)
				}
			case "CANCELLED":
				if result.Disposition != tc.want || taskState != "CANCELLED" || gen != 3 || retryEvents != 0 || failedEvents != 0 {
					t.Fatal("cancel lost priority")
				}
			default:
				if result.Disposition != "FAILED" || result.ReasonCode != tc.want || result.NextGeneration != 0 || gen != 3 || taskState != "FAILED" || nodeState != "FAILED" || retryClass != "" || retryEvents != 0 || failedEvents != 1 {
					t.Fatalf("implicit replay: %+v task=%s node=%s gen=%d retries=%d", result, taskState, nodeState, gen, retryEvents)
				}
				var lossReason, digest string
				authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT payload_json->>'lossReasonCode',payload_json->>'evidenceDigest' FROM workforce_task_events WHERE event_type='task.failed'`).Scan(&lossReason, &digest))
				if lossReason != domain.ReasonRuntimeMissing || digest != "qa-evidence-digest" {
					t.Fatal("loss evidence discarded")
				}
				var turnState, attemptState, runtimeState, summary string
				var closed bool
				authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT tt.status,a.status,r.status,tt.result_summary,tt.finished_at IS NOT NULL AND t.finished_at IS NOT NULL AND a.finished_at IS NOT NULL FROM workforce_task_turns tt JOIN workforce_tasks t ON t.id=tt.task_id JOIN workforce_task_attempts a ON a.task_turn_id=tt.id JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE tt.id=30`).Scan(&turnState, &attemptState, &runtimeState, &summary, &closed))
				if turnState != "FAILED" || attemptState != "FAILED" || runtimeState != "LOST" || summary != tc.want || !closed {
					t.Fatal("failure closure incomplete or runtime evidence replaced")
				}
				if len(recorder.transitions) != 1 {
					t.Fatal("missing existing failure notification")
				}
				tr := recorder.transitions[0]
				if tr.ReasonCode != "RUNTIME_UNRECOVERABLE" || tr.SourceType != notification.SourceRuntimeRecovery || tr.SourceID != 40 || tr.TaskTurnID != 30 || tr.CurrentStatus != "FAILED" || tr.PreviousStatus != tc.taskStatus || tr.RetryBudgetExhausted || tr.SourceOccurrence < 1 {
					t.Fatal("incorrect interruption fact")
				}
			}
			var afterStarted, afterNonce, afterPermit bool
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT harness_started_at IS NOT NULL,run_nonce_accepted_at IS NOT NULL,run_permit_id IS NOT NULL FROM workforce_attempt_runtimes WHERE attempt_id=40`).Scan(&afterStarted, &afterNonce, &afterPermit))
			if beforeStarted != afterStarted || beforeNonce != afterNonce || beforePermit != afterPermit {
				t.Fatal("trusted startup evidence erased")
			}
			replay, err := pool.Begin(ctx)
			authoritySQLCheck(t, err)
			defer replay.Rollback(ctx)
			_, err = s.fenceRuntimeLoss(ctx, replay, 40, domain.ReasonRuntimeMissing, "qa-evidence-digest")
			if !errors.Is(err, application.ErrNotClaimable) {
				t.Fatalf("duplicate loss refenced execution: %v", err)
			}
			if len(recorder.transitions) > 1 {
				t.Fatal("duplicate failure notification")
			}
		})
	}
}
