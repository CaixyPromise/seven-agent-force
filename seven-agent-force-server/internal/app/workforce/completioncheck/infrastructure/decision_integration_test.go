package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	continuationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/infrastructure"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests exercise Decide and the real transactional Continuation adapter.
// They never align schema or contact a provider, broker, Worker, or object store.
var decisionTestIDs struct {
	sync.Once
	value *xid.Generator
	err   error
}

type decisionFixture struct {
	ctx                                                                      context.Context
	db                                                                       *pgxpool.Pool
	store                                                                    *Store
	tenant, task, turn, actor, attempt, cycle, checkpoint, baseline, current int64
	version                                                                  int64
	digest                                                                   string
}

func newDecisionFixture(t *testing.T) *decisionFixture {
	t.Helper()
	dsn := os.Getenv("COMPLETION_CHECK_DECISION_TEST_DSN")
	if dsn == "" {
		t.Skip("COMPLETION_CHECK_DECISION_TEST_DSN requires the aligned isolated decision test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var database string
	if err = db.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(database, "seven_worker_batch1_") {
		t.Fatal("decision fixture requires the authorized isolated Batch1 database")
	}
	decisionTestIDs.Do(func() { decisionTestIDs.value, decisionTestIDs.err = xid.New(47) })
	if decisionTestIDs.err != nil {
		t.Fatal(decisionTestIDs.err)
	}
	ids := decisionTestIDs.value
	f := &decisionFixture{ctx: ctx, db: db, tenant: ids.NextID(), task: ids.NextID(), turn: ids.NextID(), actor: ids.NextID(), attempt: ids.NextID(), cycle: ids.NextID(), checkpoint: ids.NextID(), baseline: ids.NextID(), current: ids.NextID(), version: 7}
	f.store = New(db, ids, continuationinfra.New(db, ids), principalinfra.NewResolver())
	t.Cleanup(func() { f.cleanup(t) })
	input := facade.Input{Mode: "COMMAND", Command: "go test ./...", Instructions: "verify existing behavior"}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	f.digest, err = facade.Digest(input)
	if err != nil {
		t.Fatal(err)
	}
	hex := strings.Repeat("a", 64)
	commit := strings.Repeat("b", 40)
	f.exec(t, `INSERT INTO sys_user(id,"userAccount","nickName") VALUES($1,$2,'Decision fixture')`, f.actor, fmt.Sprintf("decision-%d", f.actor))
	f.exec(t, `INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,status,current_task_turn_id,current_turn_number,version) VALUES($1,$2,$3,$4,$5,'','','Original task','fixture','REVIEWING',$6,1,$7)`, f.task, f.tenant, f.actor, fmt.Sprintf("decision-%d", f.task), hex, f.turn, f.version)
	f.exec(t, `INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,status,completion_check_json,completion_check_digest,completion_check_references_json) VALUES($1,$2,$3,1,'Original task',$4,$2,$5,$6,$4,'REVIEWING',$7::jsonb,$8,'[]'::jsonb)`, f.turn, f.task, f.tenant, hex, f.actor, ids.NextID(), string(raw), f.digest)
	node := ids.NextID()
	f.exec(t, `INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,execution_requirement_digest) VALUES($1,$2,$3,'main','REVIEWING',$4)`, node, f.task, f.turn, hex)
	f.exec(t, `INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver) VALUES($1,$2,$3,1,$4,'/fixture','REVIEWING','fixture',CURRENT_TIMESTAMP+INTERVAL '1 hour','docker')`, f.attempt, f.task, f.turn, ids.NextID())
	f.exec(t, `INSERT INTO workforce_result_evaluation_cycles(id,task_id,task_turn_id,execution_node_id,attempt_id,cycle_number,contract_digest,decision,reason_code,contract_json) VALUES($1,$2,$3,$4,$5,1,$6,'WAIT_USER','CHECK_ATTRIBUTION_UNCERTAIN','{"decision":"WAIT_USER","reasonCode":"CHECK_ATTRIBUTION_UNCERTAIN","message":"Optional check requires user action"}'::jsonb)`, f.cycle, f.task, f.turn, node, f.attempt, hex)
	f.exec(t, `INSERT INTO workforce_task_input_snapshots(id,tenant_id,task_id,task_turn_id,source_kind,source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest) VALUES($1,$2,$3,$4,'REPOSITORY',$5,$5,$6,'fixture/input.bundle',$6,$6,1,$6)`, ids.NextID(), f.tenant, f.task, f.turn, commit, hex)
	f.exec(t, `INSERT INTO workforce_task_git_plans(id,tenant_id,task_id,task_turn_id,source_ref_type,source_ref_name,source_commit,delivery_mode,plan_digest,status) VALUES($1,$2,$3,$4,'COMMIT',$5,$5,'PATCH',$6,'ACTIVE')`, ids.NextID(), f.tenant, f.task, f.turn, commit, hex)
	f.exec(t, `INSERT INTO workforce_continuation_checkpoints(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,decision,checkpoint_identity_sha256,format_version,session_object_key,session_sha256,identity_manifest_object_key,manifest_sha256,status,expires_at) VALUES($1,$2,$3,$4,$5,'WAIT_USER',$6,2,'fixture/session',$6,'fixture/identity',$6,'AVAILABLE',CURRENT_TIMESTAMP+INTERVAL '1 hour')`, f.checkpoint, f.task, f.turn, f.attempt, f.cycle, hex)
	for _, e := range []struct {
		id    int64
		phase string
		cycle any
	}{{f.baseline, "BASELINE", nil}, {f.current, "CURRENT", f.cycle}} {
		observation := fmt.Sprintf(`{"checkDigest":%q,"workspaceDigest":%q,"status":"FAILED","failures":[{"key":"existing-case","path":"test.go","fingerprint":%q}]}`, f.digest, hex, hex)
		f.exec(t, `INSERT INTO workforce_completion_check_results(id,tenant_id,task_id,task_turn_id,attempt_id,phase,check_digest,status,summary,observation_json,evaluation_cycle_id) VALUES($1,$2,$3,$4,$5,$6,$7,'FAILED','检查未通过。',$8::jsonb,$9)`, e.id, f.tenant, f.task, f.turn, f.attempt, e.phase, f.digest, observation, e.cycle)
	}
	f.exec(t, `UPDATE workforce_task_turns SET completion_check_baseline_result_id=$2,completion_check_input_digest=$3 WHERE id=$1`, f.turn, f.baseline, f.digest)
	return f
}

func (f *decisionFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(f.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}
func (f *decisionFixture) cleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, sql := range []string{
		`DELETE FROM workforce_completion_check_decisions WHERE task_id=$1`,
		`DELETE FROM workforce_completion_check_results WHERE task_id=$1`,
		`DELETE FROM workforce_task_input_references WHERE task_input_snapshot_id IN (SELECT id FROM workforce_task_input_snapshots WHERE task_id=$1)`,
		`DELETE FROM workforce_task_input_snapshots WHERE task_id=$1`,
		`DELETE FROM workforce_task_git_plans WHERE task_id=$1`,
		`DELETE FROM workforce_continuation_checkpoints WHERE task_id=$1`,
		`DELETE FROM workforce_result_evaluation_cycles WHERE task_id=$1`,
		`DELETE FROM workforce_task_events WHERE task_id=$1`,
		`DELETE FROM workforce_execution_nodes WHERE task_id=$1`,
		`DELETE FROM workforce_task_attempts WHERE task_id=$1`,
		`DELETE FROM workforce_task_turns WHERE task_id=$1`,
		`DELETE FROM workforce_tasks WHERE id=$1`,
	} {
		if _, err := f.db.Exec(ctx, sql, f.task); err != nil {
			t.Errorf("fixture cleanup failed: %v", err)
		}
	}
	if _, err := f.db.Exec(ctx, `DELETE FROM sys_user WHERE id=$1`, f.actor); err != nil {
		t.Errorf("fixture actor cleanup failed: %v", err)
	}
}

func (f *decisionFixture) request(action string) application.Decision {
	d := application.Decision{TenantID: f.tenant, TaskID: f.task, ActorID: f.actor, ResultID: f.current, ExpectedVersion: f.version, Action: action, Reason: "Acknowledge the recorded optional check result", IdempotencyKey: "decision-test"}
	if action == facade.AuthorizeRepair {
		d.RepairScope = "Repair only the regression in parser.go"
	}
	if action == facade.AddVerification {
		d.CompletionCheck = &facade.Input{Mode: "COMMAND", Command: "go test ./parser"}
	}
	return d
}

func (f *decisionFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	var turn, version, decisions, turns int64
	if err := f.db.QueryRow(f.ctx, `SELECT current_task_turn_id,version,(SELECT count(*) FROM workforce_completion_check_decisions WHERE task_id=$1),(SELECT count(*) FROM workforce_task_turns WHERE task_id=$1) FROM workforce_tasks WHERE id=$1`, f.task).Scan(&turn, &version, &decisions, &turns); err != nil {
		t.Fatal(err)
	}
	if turn != f.turn || version != f.version || decisions != 0 || turns != 1 {
		t.Fatalf("rejected decision mutated task: turn=%d version=%d decisions=%d turns=%d", turn, version, decisions, turns)
	}
}

func TestCompletionCheckDecisionActionsAndIdempotency(t *testing.T) {
	for _, action := range []string{facade.ContinueWithLimitations, facade.AuthorizeRepair, facade.AddVerification} {
		t.Run(action, func(t *testing.T) {
			f := newDecisionFixture(t)
			d := f.request(action)
			first, err := f.store.Decide(f.ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			if first.DecisionID <= 0 || first.TaskTurnID <= 0 || first.TaskTurnID == f.turn || first.Replayed {
				t.Fatalf("invalid first decision: %+v", first)
			}
			replay, err := f.store.Decide(f.ctx, d)
			if err != nil || !replay.Replayed || replay.DecisionID != first.DecisionID || replay.TaskTurnID != first.TaskTurnID {
				t.Fatalf("exact replay did not converge: %+v %v", replay, err)
			}
			for _, mutate := range []func(*application.Decision){func(v *application.Decision) { v.Reason = "Different justification" }, func(v *application.Decision) { v.ActorID++ }, func(v *application.Decision) { v.IdempotencyKey = "new-key-old-result" }} {
				candidate := d
				mutate(&candidate)
				if _, err = f.store.Decide(f.ctx, candidate); !errors.Is(err, facade.ErrConflict) {
					t.Fatalf("changed/stale request should conflict: %v", err)
				}
			}
			var current, version, consumed, decisionCount, turnCount, baseline, result, actor int64
			if err = f.db.QueryRow(f.ctx, `SELECT t.current_task_turn_id,t.version,c.consumed_by_turn_id,(SELECT count(*) FROM workforce_completion_check_decisions WHERE task_id=t.id),(SELECT count(*) FROM workforce_task_turns WHERE task_id=t.id),d.baseline_result_id,d.result_id,(SELECT p.user_id FROM workforce_principals p WHERE p.id=d.actor_principal_id AND p.principal_type='USER') FROM workforce_tasks t JOIN workforce_continuation_checkpoints c ON c.id=$2 JOIN workforce_completion_check_decisions d ON d.id=$3 WHERE t.id=$1`, f.task, f.checkpoint, first.DecisionID).Scan(&current, &version, &consumed, &decisionCount, &turnCount, &baseline, &result, &actor); err != nil {
				t.Fatal(err)
			}
			if current != first.TaskTurnID || consumed != first.TaskTurnID || version != f.version+1 || decisionCount != 1 || turnCount != 2 || baseline != f.baseline || result != f.current || actor != f.actor {
				t.Fatal("decision/turn/checkpoint/evidence atomicity mismatch")
			}
			var sourceDigest, targetDigest, instruction, mandatoryDecision string
			var principalID, creatorID int64
			if err = f.db.QueryRow(f.ctx, `SELECT d.actor_principal_id,tr.created_by FROM workforce_completion_check_decisions d JOIN workforce_task_turns tr ON tr.id=d.task_turn_id WHERE d.id=$1`, first.DecisionID).Scan(&principalID, &creatorID); err != nil {
				t.Fatal(err)
			}
			if principalID == f.actor || principalID <= 0 || creatorID != f.actor {
				t.Fatal("principal identity confused with Framework creator identity")
			}
			if err = f.db.QueryRow(f.ctx, `SELECT source.completion_check_digest,target.completion_check_digest,target.instruction FROM workforce_task_turns source JOIN workforce_task_turns target ON target.id=$2 WHERE source.id=$1`, f.turn, first.TaskTurnID).Scan(&sourceDigest, &targetDigest, &instruction); err != nil {
				t.Fatal(err)
			}
			wantDigest := f.digest
			if d.CompletionCheck != nil {
				wantDigest, err = facade.Digest(*d.CompletionCheck)
				if err != nil {
					t.Fatal(err)
				}
			}
			if sourceDigest != f.digest || targetDigest != wantDigest {
				t.Fatal("decision mutated old check or failed to freeze new check")
			}
			// The approved contract preserves the original goal; a human action
			// narrows the follow-up rather than replacing it with a new task.
			if !strings.HasPrefix(instruction, "Original task\n") || strings.Count(instruction, "Original task") != 1 {
				t.Fatal("human decision replaced or duplicated the original goal")
			}
			if action == facade.AuthorizeRepair && (!strings.Contains(instruction, d.RepairScope) || strings.Count(instruction, d.RepairScope) != 1) {
				t.Fatal("authorized repair scope was lost, changed, or duplicated")
			}
			if strings.Contains(instruction, d.Reason) {
				t.Fatal("audit-only decision reason became an Agent instruction")
			}
			var auditReason, auditScope, auditAction string
			var auditSource, auditCheckpoint int64
			if err = f.db.QueryRow(f.ctx, `SELECT reason,repair_scope,action,source_task_turn_id,checkpoint_id FROM workforce_completion_check_decisions WHERE id=$1`, first.DecisionID).Scan(&auditReason, &auditScope, &auditAction, &auditSource, &auditCheckpoint); err != nil {
				t.Fatal(err)
			}
			if auditReason != d.Reason || auditScope != d.RepairScope || auditAction != d.Action || auditSource != f.turn || auditCheckpoint != f.checkpoint {
				t.Fatal("decision justification/scope/source audit does not match authorization")
			}
			if err = f.db.QueryRow(f.ctx, `SELECT decision FROM workforce_result_evaluation_cycles WHERE id=$1`, f.cycle).Scan(&mandatoryDecision); err != nil {
				t.Fatal(err)
			}
			if mandatoryDecision != "WAIT_USER" {
				t.Fatal("human decision rewrote processor evidence")
			}
		})
	}
}

func TestCompletionCheckDecisionCrossTenantAndStale(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *decisionFixture, *application.Decision)
		want   error
	}{
		{"cross-tenant", func(t *testing.T, f *decisionFixture, d *application.Decision) {
			d.TenantID = newDecisionFixture(t).tenant
		}, facade.ErrNotFound},
		{"stale-version", func(t *testing.T, f *decisionFixture, d *application.Decision) { d.ExpectedVersion-- }, facade.ErrConflict},
		{"wrong-result", func(t *testing.T, f *decisionFixture, d *application.Decision) { d.ResultID = f.baseline }, facade.ErrConflict},
		{"expired-checkpoint", func(t *testing.T, f *decisionFixture, d *application.Decision) {
			f.exec(t, `UPDATE workforce_continuation_checkpoints SET expires_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, f.checkpoint)
		}, facade.ErrConflict},
		{"wrong-checkpoint-cycle", func(t *testing.T, f *decisionFixture, d *application.Decision) {
			f.exec(t, `UPDATE workforce_continuation_checkpoints SET evaluation_cycle_id=evaluation_cycle_id+1 WHERE id=$1`, f.checkpoint)
		}, facade.ErrConflict},
		{"mandatory-rejected-despite-wait-checkpoint", func(t *testing.T, f *decisionFixture, d *application.Decision) {
			f.exec(t, `UPDATE workforce_result_evaluation_cycles SET decision='REJECT',reason_code='MANDATORY_CHECK_FAILED',contract_json='{"decision":"REJECT","reasonCode":"MANDATORY_CHECK_FAILED","message":"Necessary validation failed"}'::jsonb WHERE id=$1`, f.cycle)
		}, facade.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDecisionFixture(t)
			d := f.request(facade.ContinueWithLimitations)
			tc.mutate(t, f, &d)
			if _, err := f.store.Decide(f.ctx, d); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			f.assertUnchanged(t)
		})
	}
}

func TestCompletionCheckDecisionCannotOverrideMandatoryRejection(t *testing.T) {
	f := newDecisionFixture(t)
	f.exec(t, `UPDATE workforce_result_evaluation_cycles SET decision='REJECT',reason_code='MANDATORY_CHECK_FAILED',contract_json='{"decision":"REJECT","reasonCode":"MANDATORY_CHECK_FAILED","message":"Necessary validation failed"}'::jsonb WHERE id=$1`, f.cycle)
	f.exec(t, `UPDATE workforce_tasks SET status='MANUAL_REVIEW' WHERE id=$1`, f.task)
	f.exec(t, `UPDATE workforce_task_turns SET status='MANUAL_REVIEW' WHERE id=$1`, f.turn)
	f.exec(t, `UPDATE workforce_continuation_checkpoints SET decision='MANUAL_REVIEW' WHERE id=$1`, f.checkpoint)
	state, err := f.store.Summary(f.ctx, f.tenant, f.task)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.AvailableActions) != 0 || state.CheckpointReady {
		t.Fatal("mandatory rejection offered optional-check bypass")
	}
	for _, action := range []string{facade.ContinueWithLimitations, facade.AuthorizeRepair, facade.AddVerification} {
		if _, err = f.store.Decide(f.ctx, f.request(action)); !errors.Is(err, facade.ErrConflict) {
			t.Fatalf("%s bypassed mandatory rejection: %v", action, err)
		}
	}
	f.assertUnchanged(t)
	var decision, reason, status string
	if err = f.db.QueryRow(f.ctx, `SELECT e.decision,e.contract_json->>'reasonCode',t.status FROM workforce_result_evaluation_cycles e JOIN workforce_tasks t ON t.id=e.task_id WHERE e.id=$1`, f.cycle).Scan(&decision, &reason, &status); err != nil {
		t.Fatal(err)
	}
	if decision != "REJECT" || reason != "MANDATORY_CHECK_FAILED" || status != "MANUAL_REVIEW" {
		t.Fatal("mandatory evidence or Task status overwritten")
	}
}

func TestCompletionCheckDecisionConcurrentReplay(t *testing.T) {
	f := newDecisionFixture(t)
	request := f.request(facade.ContinueWithLimitations)
	type outcome struct {
		result application.DecisionResult
		err    error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; result, err := f.store.Decide(f.ctx, request); results <- outcome{result, err} }()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent decision errors: %v / %v", first.err, second.err)
	}
	if first.result.DecisionID != second.result.DecisionID || first.result.TaskTurnID != second.result.TaskTurnID || first.result.Replayed == second.result.Replayed {
		t.Fatalf("concurrent replay diverged: %+v / %+v", first.result, second.result)
	}
	var count int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM workforce_task_turns WHERE task_id=$1`, f.task).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("concurrent replay created %d turns", count)
	}
}

func TestCompletionCheckDecisionRollsBackUnfrozenReference(t *testing.T) {
	f := newDecisionFixture(t)
	d := f.request(facade.AddVerification)
	d.CompletionCheck.ReferenceAssetIDs = []int64{decisionTestIDs.value.NextID()}
	if _, err := f.store.Decide(f.ctx, d); !errors.Is(err, facade.ErrInvalid) {
		t.Fatalf("unfrozen replacement reference must be rejected: %v", err)
	}
	f.assertUnchanged(t)
	var consumed, events, nodes int64
	if err := f.db.QueryRow(f.ctx, `SELECT COALESCE(consumed_by_turn_id,0),(SELECT count(*) FROM workforce_task_events WHERE task_id=$2),(SELECT count(*) FROM workforce_execution_nodes WHERE task_id=$2) FROM workforce_continuation_checkpoints WHERE id=$1`, f.checkpoint, f.task).Scan(&consumed, &events, &nodes); err != nil {
		t.Fatal(err)
	}
	if consumed != 0 || events != 0 || nodes != 1 {
		t.Fatal("failed decision retained continuation side effects")
	}
}
