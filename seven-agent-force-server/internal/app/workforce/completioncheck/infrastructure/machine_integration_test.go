package infrastructure

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"strings"
	"testing"
)

func TestCompletionCheckMachineFencingAndEvidence(t *testing.T) {
	f := newDecisionFixture(t)
	lease := "isolated-check-lease"
	sum := sha256.Sum256([]byte(lease))
	worker := decisionTestIDs.value.NextID()
	authority := facade.MachineAuthority{WorkerID: worker, InstanceID: "fixture-instance", HostBootID: "fixture-boot", Generation: 1, OwnerEpoch: 1}
	f.exec(t, `UPDATE workforce_tasks SET status='PREPARING' WHERE id=$1`, f.task)
	f.exec(t, `UPDATE workforce_task_attempts SET status='PREPARING',worker_id=$2,worker_instance_id=$3,host_boot_id=$4,generation=1,owner_epoch=1,lease_token=$5 WHERE id=$1`, f.attempt, worker, authority.InstanceID, authority.HostBootID, hex.EncodeToString(sum[:]))
	f.exec(t, `UPDATE workforce_task_turns SET completion_check_baseline_result_id=NULL WHERE id=$1`, f.turn)
	f.exec(t, `DELETE FROM workforce_completion_check_results WHERE task_id=$1`, f.task)
	e := facade.Evidence{TaskID: f.task, TaskTurnID: f.turn, AttemptID: f.attempt, Phase: "BASELINE", CheckDigest: f.digest, Status: "FAILED", Summary: "raw secret MUST NOT survive", Observation: facade.Observation{CheckDigest: f.digest, Status: "FAILED", WorkspaceDigest: strings.Repeat("a", 64)}}
	first, err := f.store.RecordMachineEvidence(f.ctx, authority, f.attempt, lease, e)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID <= 0 || strings.Contains(first.Summary, "secret") || first.SourceCommit == "" || first.SourceIdentityDigest == "" {
		t.Fatal("missing safe server evidence identity")
	}
	replay, err := f.store.RecordMachineEvidence(f.ctx, authority, f.attempt, lease, e)
	if err != nil || first.ID != replay.ID {
		t.Fatal("baseline replay failed", err)
	}
	tampered := e
	tampered.Observation.CheckDigest = strings.Repeat("b", 64)
	if _, err = f.store.RecordMachineEvidence(f.ctx, authority, f.attempt, lease, tampered); !errors.Is(err, facade.ErrInvalid) {
		t.Fatal("inner digest mismatch accepted", err)
	}
	stale := authority
	stale.OwnerEpoch++
	if _, err = f.store.RecordMachineEvidence(f.ctx, stale, f.attempt, lease, e); !errors.Is(err, facade.ErrConflict) {
		t.Fatal("old owner accepted", err)
	}
	f.exec(t, `UPDATE workforce_tasks SET status='CANCELLING' WHERE id=$1`, f.task)
	if _, err = f.store.ReadMachineEvidence(f.ctx, authority, f.attempt, lease, "BASELINE", 0, f.digest); !errors.Is(err, facade.ErrConflict) {
		t.Fatal("cancelled task read accepted", err)
	}
	if _, err = f.store.RecordMachineEvidence(f.ctx, authority, f.attempt, lease, e); !errors.Is(err, facade.ErrConflict) {
		t.Fatal("cancelled task write accepted", err)
	}
	f.exec(t, `UPDATE workforce_tasks SET status='RUNNING' WHERE id=$1`, f.task)
	f.exec(t, `UPDATE workforce_task_attempts SET status='RUNNING' WHERE id=$1`, f.attempt)
	if _, err = f.store.RecordMachineEvidence(f.ctx, authority, f.attempt, lease, e); !errors.Is(err, facade.ErrConflict) {
		t.Fatal("baseline after start accepted", err)
	}
	e.Phase = "CURRENT"
	e.EvaluationCycleID = decisionTestIDs.value.NextID()
	if _, err = f.store.RecordMachineEvidence(f.ctx, authority, f.attempt, lease, e); !errors.Is(err, facade.ErrConflict) {
		t.Fatal("invented cycle accepted", err)
	}
	f.exec(t, `INSERT INTO workforce_completion_check_cycle_authorizations(cycle_id,tenant_id,task_id,task_turn_id,attempt_id,cycle_number)VALUES($1,$2,$3,$4,$5,1)`, e.EvaluationCycleID, f.tenant, f.task, f.turn, f.attempt)
	t.Cleanup(func() {
		f.exec(t, `DELETE FROM workforce_completion_check_cycle_authorizations WHERE task_id=$1`, f.task)
	})
	if _, err = f.store.RecordMachineEvidence(f.ctx, authority, f.attempt, lease, e); err != nil {
		t.Fatal("authorized cycle rejected", err)
	}
}

func TestCompletionCheckInheritedBaselineAndBoundedLimitations(t *testing.T) {
	f := newDecisionFixture(t)
	result, err := f.store.Decide(f.ctx, f.request(facade.ContinueWithLimitations))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	before, _, err := f.store.LoadComparison(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.attempt+1, f.cycle+1, f.digest)
	if err != nil || before.CheckDigest != f.digest || before.Status != "FAILED" {
		t.Fatal("inherited baseline missing", err)
	}
	allowed, err := f.store.AcceptedLimitations(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.digest, before)
	if err != nil || !allowed {
		t.Fatal("exact authorized limitation rejected", err)
	}
	before.WorkspaceDigest = strings.Repeat("b", 64)
	allowed, err = f.store.AcceptedLimitations(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.digest, before)
	if err != nil || allowed {
		t.Fatal("changed workspace inherited authorization", err)
	}
}

func TestCompletionCheckUnstructuredLimitationsRequireSameCompleteDiagnostic(t *testing.T) {
	f := newDecisionFixture(t)
	original := facade.Observation{CheckDigest: f.digest, Status: "FAILED", WorkspaceDigest: strings.Repeat("a", 64), DiagnosticKnown: true, DiagnosticDigest: strings.Repeat("b", 64)}
	raw, _ := json.Marshal(original)
	f.exec(t, `UPDATE workforce_completion_check_results SET observation_json=$2 WHERE id=$1`, f.current, raw)
	result, err := f.store.Decide(f.ctx, f.request(facade.ContinueWithLimitations))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	allowed, err := f.store.AcceptedLimitations(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.digest, original)
	if err != nil || !allowed {
		t.Fatal("same complete diagnostic rejected", err)
	}
	for _, change := range []func(*facade.Observation){func(o *facade.Observation) { o.DiagnosticDigest = strings.Repeat("c", 64) }, func(o *facade.Observation) { o.DiagnosticKnown = false }, func(o *facade.Observation) { o.DiagnosticTruncated = true }} {
		next := original
		change(&next)
		allowed, err = f.store.AcceptedLimitations(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.digest, next)
		if err != nil || allowed {
			t.Fatal("changed or incomplete diagnostic authorized", err)
		}
	}
}
