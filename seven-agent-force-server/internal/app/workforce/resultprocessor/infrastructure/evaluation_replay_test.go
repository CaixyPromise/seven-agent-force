package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/jackc/pgx/v5"
	"reflect"
	"strings"
	"testing"
	"time"
)

type replayRow struct {
	values []any
	err    error
}

func (r replayRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		panic("unexpected replay query shape")
	}
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(r.values[i]))
	}
	return nil
}

type replayTx struct {
	pgx.Tx
	t       *testing.T
	rows    []replayRow
	queries []string
}

func (tx *replayTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	tx.queries = append(tx.queries, sql)
	if strings.HasPrefix(sql, "SELECT t.id FROM workforce_tasks t JOIN workforce_task_attempts") {
		return replayRow{values: []any{int64(2)}}
	}
	if len(tx.rows) == 0 {
		tx.t.Fatalf("unexpected query: %s", sql)
	}
	r := tx.rows[0]
	tx.rows = tx.rows[1:]
	return r
}
func replayFixture(t *testing.T) (*replayTx, facade.MachineIdentity, facade.EvaluationReplayIdentity) {
	return &replayTx{t: t, rows: []replayRow{
		{values: []any{int64(1), int64(2), int64(3), "4", int64(7), "instance", "boot", int64(1), int64(2), sha256Hex("lease"), time.Now().Add(time.Hour), "REVIEWING", "RUNNING", "RUNNING"}},
		{values: []any{int64(9), "processor", 3, json.RawMessage(`[]`), json.RawMessage(`{"bindings":[]}`)}},
		{values: []any{int64(3), false, "session"}},
		{values: []any{int64(5), "session", "WAIT_USER", "CHECK_UNAVAILABLE", "verification unavailable"}},
		{values: []any{json.RawMessage(`{"decision":"WAIT_USER"}`), "server-digest"}},
	}}, facade.MachineIdentity{WorkerID: 7, InstanceID: "instance", HostBootID: "boot", Generation: 1, OwnerEpoch: 2}, facade.EvaluationReplayIdentity{CycleID: 5, CycleNumber: 1, SessionDigest: "session"}
}

func TestEvaluationReplayPersistedWaitUserWithoutMaterialization(t *testing.T) {
	tx, owner, pending := replayFixture(t)
	got, err := (&Store{}).readEvaluationTx(context.Background(), tx, owner, 6, "lease", pending)
	if err != nil || !got.Found || got.Directive.Directive != facade.DirectiveWaitForUser || got.Directive.ContractDigest != "server-digest" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if len(tx.rows) != 0 {
		t.Fatal("did not fetch authoritative contract")
	}
	for _, q := range tx.queries {
		if strings.Contains(q, "completion_check") || strings.Contains(q, "INSERT") {
			t.Fatal("replayed execution or checkpoint")
		}
	}
}

func TestEvaluationReplayFencesBeforeDecisionRead(t *testing.T) {
	for _, name := range []string{"worker", "instance", "boot", "generation", "owner", "lease", "expired", "runtime", "cancel-status", "cancel-intent", "turn", "attempt", "session", "persisted-session", "cycle"} {
		t.Run(name, func(t *testing.T) {
			tx, owner, pending := replayFixture(t)
			lease := "lease"
			switch name {
			case "worker":
				owner.WorkerID++
			case "instance":
				owner.InstanceID = "old"
			case "boot":
				owner.HostBootID = "old"
			case "generation":
				owner.Generation++
			case "owner":
				owner.OwnerEpoch++
			case "lease":
				lease = "old"
			case "expired":
				tx.rows[0].values[10] = time.Now().Add(-time.Second)
			case "runtime":
				tx.rows[0].values[13] = "REATTACHING"
			case "cancel-status":
				tx.rows[0].values[11] = "CANCELLING"
			case "cancel-intent":
				tx.rows[2].values[1] = true
			case "turn":
				tx.rows[2].values[0] = int64(99)
			case "attempt":
				tx.rows[0].values[12] = "FAILED"
			case "session":
				pending.SessionDigest = "other"
			case "persisted-session":
				tx.rows[3].values[1] = "other"
			case "cycle":
				pending.CycleID = 99
			}
			got, err := (&Store{}).readEvaluationTx(context.Background(), tx, owner, 6, lease, pending)
			if !errors.Is(err, ErrConflict) || got.Found {
				t.Fatalf("accepted stale authority: %+v %v", got, err)
			}
			if strings.Contains(strings.Join(tx.queries, "\n"), "SELECT contract_json") {
				t.Fatal("read decision before fencing")
			}
		})
	}
}

func TestEvaluationReplayAbsenceAndReadFailureAreDistinct(t *testing.T) {
	for _, status := range []string{"RUNNING", "VALIDATING", "REVIEWING"} {
		t.Run(status, func(t *testing.T) {
			tx, owner, pending := replayFixture(t)
			tx.rows[0].values[11] = status
			tx.rows[3] = replayRow{err: pgx.ErrNoRows}
			got, err := (&Store{}).readEvaluationTx(context.Background(), tx, owner, 6, "lease", pending)
			if got.Found || (status == "REVIEWING" && !errors.Is(err, ErrConflict)) || (status != "REVIEWING" && err != nil) {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
	tx, owner, pending := replayFixture(t)
	failure := errors.New("read failed")
	tx.rows[3] = replayRow{err: failure}
	if _, err := (&Store{}).readEvaluationTx(context.Background(), tx, owner, 6, "lease", pending); !errors.Is(err, failure) {
		t.Fatal("read failure became missing candidate")
	}
}
