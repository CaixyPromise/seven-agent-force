package infrastructure

import (
	"context"
	"encoding/json"
	check "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

type cycleRow struct{ id int64 }

func (r cycleRow) Scan(dest ...any) error {
	if r.id == 0 {
		return pgx.ErrNoRows
	}
	*dest[0].(*int64) = r.id
	return nil
}

type cycleTx struct {
	pgx.Tx
	id      int64
	inserts int
}

func (tx *cycleTx) QueryRow(context.Context, string, ...any) pgx.Row { return cycleRow{id: tx.id} }
func (tx *cycleTx) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	tx.id = args[0].(int64)
	tx.inserts++
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

type cycleIDs struct{}

func (cycleIDs) NextID() int64 { return 99 }
func TestCheckCycleAuthorizationReplaysAndRejectsInventedID(t *testing.T) {
	store := &Store{idGen: cycleIDs{}}
	tx := &cycleTx{}
	scope := evaluationScope{TenantID: 1, TaskID: 2, TurnID: 3}
	id, err := store.authorizeCompletionCheckCycle(context.Background(), tx, scope, 4, 0, 1)
	if err != nil || id != 99 || tx.inserts != 1 {
		t.Fatalf("allocation %d %v", id, err)
	}
	id, err = store.authorizeCompletionCheckCycle(context.Background(), tx, scope, 4, 0, 1)
	if err != nil || id != 99 || tx.inserts != 1 {
		t.Fatal("response loss allocated another cycle")
	}
	if _, err = store.authorizeCompletionCheckCycle(context.Background(), tx, scope, 4, 100, 1); err == nil {
		t.Fatal("foreign cycle accepted")
	}
	fresh := &cycleTx{}
	if _, err = store.authorizeCompletionCheckCycle(context.Background(), fresh, scope, 4, 100, 1); err == nil || fresh.inserts != 0 {
		t.Fatal("worker invented cycle authority")
	}
}

type checkRow struct {
	raw    []byte
	digest string
}

func (r checkRow) Scan(dest ...any) error {
	*dest[0].(*[]byte) = r.raw
	*dest[1].(*string) = r.digest
	*dest[2].(*string) = r.digest
	*dest[3].(*[]byte) = []byte(`[]`)
	return nil
}

type checkTx struct {
	pgx.Tx
	row checkRow
}

func (tx checkTx) QueryRow(context.Context, string, ...any) pgx.Row { return tx.row }

type checkReader struct {
	t             *testing.T
	before, after check.Observation
	calls         int
}

type acceptedCheckReader struct {
	checkReader
	accepted    bool
	acceptCalls int
}

func (r *acceptedCheckReader) AcceptedLimitations(_ context.Context, _ pgx.Tx, tenant, task, turn int64, digest string, current check.Observation) (bool, error) {
	r.acceptCalls++
	if tenant != 1 || task != 2 || turn != 3 || digest != current.CheckDigest {
		r.t.Fatal("acceptance scope drift")
	}
	return r.accepted, nil
}

func TestCompletionGateUserAcceptanceCannotOverrideProcessorReject(t *testing.T) {
	input := check.Input{Mode: "COMMAND", Command: "false"}
	digest, _ := check.Digest(input)
	raw, _ := json.Marshal(input)
	tx := checkTx{row: checkRow{raw: raw, digest: digest}}
	scope := evaluationScope{TenantID: 1, TaskID: 2, TurnID: 3}
	reader := &acceptedCheckReader{checkReader: checkReader{t: t, before: check.Observation{CheckDigest: digest, Status: "FAILED"}, after: check.Observation{CheckDigest: digest, Status: "FAILED"}}, accepted: true}
	store := &Store{}
	store.SetCompletionCheckEvidenceReader(reader)
	result, err := store.completionCheckGate(context.Background(), tx, scope, 4, 5, processor.ResultContract{Decision: "PASS"})
	if err != nil || result.Decision != "PASS" || reader.acceptCalls != 1 {
		t.Fatalf("explicit authoritative acceptance %+v %v", result, err)
	}
	result, err = store.completionCheckGate(context.Background(), tx, scope, 4, 5, processor.ResultContract{Decision: "REJECT"})
	if err != nil || result.Decision != "REJECT" || reader.acceptCalls != 1 {
		t.Fatal("acceptance overrode mandatory processor decision")
	}
	reader.accepted = false
	result, err = store.completionCheckGate(context.Background(), tx, scope, 4, 5, processor.ResultContract{Decision: "PASS"})
	if err != nil || result.Decision != "WAIT_USER" {
		t.Fatal("new/unapproved evidence passed")
	}
}

func (r *checkReader) LoadComparison(_ context.Context, _ pgx.Tx, tenant, task, turn, attempt, cycle int64, digest string) (check.Observation, check.Observation, error) {
	r.calls++
	if tenant != 1 || task != 2 || turn != 3 || attempt != 4 || cycle != 5 || digest != r.before.CheckDigest {
		r.t.Fatal("evidence scope drift")
	}
	return r.before, r.after, nil
}
func TestCompletionGateLoadsFencedCycleEvidence(t *testing.T) {
	input := check.Input{Mode: "COMMAND", Command: "true"}
	digest, _ := check.Digest(input)
	raw, _ := json.Marshal(input)
	tx := checkTx{row: checkRow{raw: raw, digest: digest}}
	scope := evaluationScope{TenantID: 1, TaskID: 2, TurnID: 3}
	candidate := processor.ResultContract{Decision: "PASS", ReasonCode: "PROCESSOR_OK", Message: "done"}
	store := &Store{}
	result, err := store.completionCheckGate(context.Background(), tx, scope, 4, 5, candidate)
	if err != nil || result.Decision != "WAIT_USER" {
		t.Fatalf("missing authority must wait: %+v %v", result, err)
	}
	reader := &checkReader{t: t, before: check.Observation{CheckDigest: digest, Status: "PASSED"}, after: check.Observation{CheckDigest: digest, Status: "FAILED"}}
	store.SetCompletionCheckEvidenceReader(reader)
	result, err = store.completionCheckGate(context.Background(), tx, scope, 4, 5, candidate)
	if err != nil || result.Decision != "REVISE" || reader.calls != 1 {
		t.Fatalf("regression %+v %v", result, err)
	}
	reader.before.Status = "FAILED"
	result, err = store.completionCheckGate(context.Background(), tx, scope, 4, 5, candidate)
	if err != nil || result.Decision != "WAIT_USER" {
		t.Fatalf("history attribution guessed %+v %v", result, err)
	}
	reader.after.Status = "PASSED"
	result, err = store.completionCheckGate(context.Background(), tx, scope, 4, 5, candidate)
	if err != nil || result.Decision != "PASS" {
		t.Fatalf("custom command cannot pass %+v %v", result, err)
	}
	tx.row.digest = "bad"
	if _, err = store.completionCheckGate(context.Background(), tx, scope, 4, 5, candidate); err == nil {
		t.Fatal("snapshot tamper accepted")
	}
}
