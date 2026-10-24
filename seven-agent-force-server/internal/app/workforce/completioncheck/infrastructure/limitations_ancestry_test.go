package infrastructure

import (
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"testing"
)

func TestCompletionCheckLimitationsRequireCompleteSameTaskAncestry(t *testing.T) {
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
	observation, _, err := f.store.LoadComparison(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.attempt+1, f.cycle+1, f.digest)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := f.store.AcceptedLimitations(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.digest, observation)
	if err != nil || !allowed {
		t.Fatal("valid exact ancestor decision rejected", err)
	}
	// An approval on a descendant/sibling cannot authorize the original Turn.
	allowed, err = f.store.AcceptedLimitations(f.ctx, tx, f.tenant, f.task, f.turn, f.digest, observation)
	if err != nil || allowed {
		t.Fatal("non-ancestor decision was accepted", err)
	}
	for _, scope := range [][2]int64{{f.tenant + 1, f.task}, {f.tenant, f.task + 1}} {
		allowed, err = f.store.AcceptedLimitations(f.ctx, tx, scope[0], scope[1], result.TaskTurnID, f.digest, observation)
		if err != nil || allowed {
			t.Fatal("cross-scope ancestry was accepted", err)
		}
	}
	for _, parent := range []int64{result.TaskTurnID, decisionTestIDs.value.NextID()} {
		if _, err = tx.Exec(f.ctx, `UPDATE workforce_task_turns SET source_turn_id=$2 WHERE id=$1`, result.TaskTurnID, parent); err != nil {
			t.Fatal(err)
		}
		allowed, err = f.store.AcceptedLimitations(f.ctx, tx, f.tenant, f.task, result.TaskTurnID, f.digest, observation)
		if err != nil || allowed {
			t.Fatal("cyclic or missing ancestor was accepted", err)
		}
	}
}
