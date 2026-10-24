package infrastructure

import (
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"strings"
	"testing"
)

func TestCompletionCheckPassedDefinitionDriftRequiresNewVerification(t *testing.T) {
	f := newDecisionFixture(t)
	before := facade.Observation{CheckDigest: f.digest, Status: "PASSED", DefinitionDigest: strings.Repeat("a", 64)}
	after := before
	after.DefinitionDigest = strings.Repeat("b", 64)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	f.exec(t, `UPDATE workforce_completion_check_results SET status='PASSED',observation_json=$2 WHERE id=$1`, f.baseline, a)
	f.exec(t, `UPDATE workforce_completion_check_results SET status='PASSED',observation_json=$2 WHERE id=$1`, f.current, b)
	state, err := f.store.Summary(f.ctx, f.tenant, f.task)
	if err != nil {
		t.Fatal(err)
	}
	if state.ReasonCode != "CHECK_DEFINITION_CHANGED" || len(state.AvailableActions) != 2 || state.AvailableActions[0] != facade.AuthorizeRepair || state.AvailableActions[1] != facade.AddVerification {
		t.Fatalf("definition drift stranded or bypassable: %+v", state)
	}
	if _, err = f.store.Decide(f.ctx, f.request(facade.ContinueWithLimitations)); !errors.Is(err, facade.ErrActionNotAllowed) {
		t.Fatal("definition downgrade waived", err)
	}
	f.assertUnchanged(t)
	result, err := f.store.Decide(f.ctx, f.request(facade.AddVerification))
	if err != nil || result.TaskTurnID <= 0 {
		t.Fatal("definition drift cannot be resolved", err)
	}
	var original []byte
	if err = f.db.QueryRow(f.ctx, `SELECT observation_json FROM workforce_completion_check_results WHERE id=$1`, f.current).Scan(&original); err != nil {
		t.Fatal(err)
	}
	var observed facade.Observation
	if json.Unmarshal(original, &observed) != nil || observed.DefinitionDigest != after.DefinitionDigest {
		t.Fatal("old definition evidence changed")
	}
}
