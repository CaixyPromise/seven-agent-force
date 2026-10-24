package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/bwmarrin/snowflake"
)

func TestPostgreSQLInteractionReattachCommitReadback(t *testing.T) {
	for _, scenario := range []struct{ interactive, start bool }{{true, false}, {false, false}, {false, true}} {
		interactive := scenario.interactive
		name := "ordinary-no-interaction"
		if interactive {
			name = "interaction-exact-replay"
		}
		if scenario.start {
			name = "ordinary-start-needed"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			pool := dispatchQATables(t, "workforce_workers", "workforce_worker_sessions", "workforce_tasks", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_execution_nodes", "workforce_task_assignments", "workforce_runtime_reconciliation_runs", "workforce_task_events", "workforce_task_execution_snapshots", "workforce_harness_releases", "workforce_task_turns", "workforce_task_input_snapshots", "workforce_task_input_references", "workforce_reference_assets", "workforce_result_processor_releases")
			for _, sql := range []string{
				`CREATE SEQUENCE qa_reattach_event_ids`, `ALTER TABLE workforce_task_events ALTER COLUMN id DROP IDENTITY IF EXISTS`, `ALTER TABLE workforce_task_events ALTER COLUMN id SET DEFAULT nextval('qa_reattach_event_ids'::regclass)`,
				`INSERT INTO workforce_workers(id,name,token_hint,current_instance_id) VALUES(8,'qa','qa','qa-new')`,
				`INSERT INTO workforce_worker_sessions(instance_id,worker_id,host_boot_id,started_at,session_state) VALUES('qa-new',8,'qa-boot',CURRENT_TIMESTAMP,'RECONCILING')`,
				`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,harness_release_id,status,current_task_turn_id,attempt_count,execution_snapshot_json) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','qa',80,'RUNNING',30,1,'{"resultProcessorReleaseId":"81","resultProcessorDigest":"qa"}')`,
				`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,execution_node_id,assignment_id) VALUES(40,20,30,1,8,'/qa','RECOVERING','qa',CURRENT_TIMESTAMP+interval '1 hour','docker','qa-old','qa-boot',3,5,50,60)`,
				`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,container_id,execution_identity_digest,expected_container_name,harness_image_digest,status,recovery_previous_status,reattach_deadline,execution_lease_sha256,execution_lease_expires_at) VALUES(40,50,60,8,'qa-old','qa-boot',3,5,'qa-container','qa-identity','qa-name','qa-image','RECOVERING','RUNNING',CURRENT_TIMESTAMP+interval '1 hour','qa',CURRENT_TIMESTAMP+interval '1 hour')`,
				`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest) VALUES(50,20,30,'qa','RECOVERING',3,'qa')`,
				`INSERT INTO workforce_task_assignments(id,task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,lease_token_sha256,lease_expires_at,status,message_id) VALUES(60,20,50,8,'qa-old',3,'qa','qa',CURRENT_TIMESTAMP+interval '1 hour','CLAIMED',70)`,
				`INSERT INTO workforce_task_execution_snapshots(task_id,tenant_id,harness_release_id,harness_image_digest,isolation_profile,snapshot_json) VALUES(20,10,80,'qa-image','qa','{}')`,
				`INSERT INTO workforce_harness_releases(id,alias,image_ref,image_digest,contract_version,supervisor_version,harness_type,harness_version,admission_evidence_json,status) VALUES(80,'qa','qa','qa-image','qa','qa','qa','qa','{}','ACTIVE')`,
				`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest) VALUES(30,20,10,1,'qa','qa',30,7,81,'qa')`,
				`INSERT INTO workforce_task_input_snapshots(id,tenant_id,task_id,task_turn_id,source_kind,source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest) VALUES(82,10,20,30,'EMPTY_WORKSPACE','qa','qa','qa','','','',0,'qa')`,
				`INSERT INTO workforce_result_processor_releases(id,processor_id,scope_type,revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,published_by_principal_id) VALUES(81,81,'SYSTEM',81,'qa','{}','{}','qa','qa',1,7)`,
			} {
				_, err := pool.Exec(ctx, sql)
				authoritySQLCheck(t, err)
			}
			node, err := snowflake.NewNode(91)
			authoritySQLCheck(t, err)
			s := New(pool, authorityTestIDs{node: node})
			envelope := domain.MachineEnvelope{WorkerID: 8, InstanceID: "qa-new", HostBootID: "qa-boot", Generation: 3, OwnerEpoch: 5}
			evidence := domain.RuntimeEvidence{AttemptID: 40, ExecutionNodeID: 50, AssignmentID: 60, WorkerID: 8, HostBootID: "qa-boot", Generation: 3, OwnerEpoch: 5, ContainerID: "qa-container", ContainerName: "qa-name", ExecutionIdentityDigest: "qa-identity", HarnessImageDigest: "qa-image", LabelsDigest: strings.Repeat("a", 64), SupervisorProtocol: domain.SupervisorControlV2, SupervisorState: domain.RuntimeRunning, HarnessPID: 123, RunNonceDigest: "qa-nonce", InteractionReplayRequested: interactive}
			if interactive {
				evidence.AdapterSessionGeneration = 1
				evidence.RoundKey = "round:1"
			}
			if scenario.start {
				evidence.SupervisorState = domain.RuntimeSupervisorReady
				evidence.HarnessPID = 0
				evidence.RunNonceDigest = ""
				_, err = pool.Exec(ctx, `UPDATE workforce_attempt_runtimes SET recovery_previous_status='SUPERVISOR_READY' WHERE attempt_id=40`)
				authoritySQLCheck(t, err)
			}
			permit, err := s.AcquireReattachPermit(ctx, envelope, 40, evidence)
			authoritySQLCheck(t, err)
			if permit.StartNeeded != scenario.start || permit.InteractionReplayRequested != interactive {
				t.Fatal("permit semantics changed")
			}
			envelope.OwnerEpoch = permit.NewOwnerEpoch
			ack := domain.ReattachAck{PermitID: permit.PermitID, PermitDigest: hashToken(permit.PermitToken), OldOwnerEpoch: 5, NewOwnerEpoch: permit.NewOwnerEpoch, SupervisorState: domain.RuntimeRunning, HarnessPID: 123, RunNonceDigest: "qa-nonce"}
			if scenario.start {
				ack.SupervisorState = domain.RuntimeSupervisorReady
				ack.HarnessPID = 0
				ack.RunNonceDigest = ""
			}
			if interactive {
				ack.ContainerID = evidence.ContainerID
				ack.ExecutionIdentityDigest = evidence.ExecutionIdentityDigest
				ack.AdapterSessionGeneration = 1
				ack.RoundKey = "round:1"
				for _, mutate := range []func(*domain.ReattachAck){func(a *domain.ReattachAck) { a.RoundKey = "round:2" }, func(a *domain.ReattachAck) { a.AdapterSessionGeneration++ }, func(a *domain.ReattachAck) { a.ContainerID = "other" }, func(a *domain.ReattachAck) { a.ExecutionIdentityDigest = "other" }, func(a *domain.ReattachAck) { a.HarnessPID++ }, func(a *domain.ReattachAck) { a.SupervisorState = domain.RuntimeSupervisorReady; a.HarnessPID = 0 }} {
					bad := ack
					mutate(&bad)
					if _, err := s.CommitReattach(ctx, envelope, 40, permit.LeaseToken, bad); !errors.Is(err, application.ErrIdentityConflict) {
						t.Fatalf("drift not fenced: %v", err)
					}
				}
			}
			first, err := s.CommitReattach(ctx, envelope, 40, permit.LeaseToken, ack)
			authoritySQLCheck(t, err)
			if first.StartNeeded != scenario.start || (first.InteractionReplayBinding != nil) != interactive {
				t.Fatal("incorrect optional replay authority")
			}
			// Simulate a lost Commit response: retry the exact ACK after its permit TTL.
			_, err = pool.Exec(ctx, `UPDATE workforce_attempt_runtimes SET reattach_permit_expires_at=CURRENT_TIMESTAMP-interval '1 second' WHERE attempt_id=40`)
			authoritySQLCheck(t, err)
			second, err := s.CommitReattach(ctx, envelope, 40, permit.LeaseToken, ack)
			authoritySQLCheck(t, err)
			a, _ := json.Marshal(first.InteractionReplayBinding)
			b, _ := json.Marshal(second.InteractionReplayBinding)
			if string(a) != string(b) {
				t.Fatal("readback binding changed")
			}
			if interactive {
				binding := second.InteractionReplayBinding
				if binding.TaskID != 20 || binding.AttemptID != 40 || binding.Generation != 3 || binding.OldOwnerEpoch != 5 || binding.NewOwnerEpoch != 6 || binding.AdapterSessionGeneration != 1 || binding.RoundKey != "round:1" || binding.ReconciliationID <= 0 {
					t.Fatal("incomplete binding")
				}
			}
			var count int
			authoritySQLCheck(t, pool.QueryRow(ctx, `SELECT count(*) FROM workforce_task_events WHERE event_type='runtime.reattached'`).Scan(&count))
			if count != 1 {
				t.Fatal("duplicate commit event")
			}
			bad := ack
			bad.LastJournalSequence++
			if _, err = s.CommitReattach(ctx, envelope, 40, permit.LeaseToken, bad); !errors.Is(err, application.ErrIdentityConflict) {
				t.Fatal("different ACK replay accepted")
			}
			_, err = pool.Exec(ctx, `UPDATE workforce_workers SET current_instance_id='qa-next' WHERE id=8`)
			authoritySQLCheck(t, err)
			if _, err = s.CommitReattach(ctx, envelope, 40, permit.LeaseToken, ack); !errors.Is(err, application.ErrStaleInstance) {
				t.Fatal("stale instance replay accepted")
			}
		})
	}
}
