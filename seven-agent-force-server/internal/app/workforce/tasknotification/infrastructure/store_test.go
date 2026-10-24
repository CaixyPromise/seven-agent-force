package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
)

type testIDGen struct{ next int64 }

func (g *testIDGen) NextID() int64 { g.next++; return g.next }

type queryCall struct {
	query string
	args  []any
}

type execCall struct {
	query string
	args  []any
}

type fakeTransaction struct {
	rows    []fakeRow
	queries []queryCall
	execs   []execCall
}

func (t *fakeTransaction) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	t.queries = append(t.queries, queryCall{query: query, args: args})
	if len(t.rows) == 0 {
		return fakeRow{err: errors.New("unexpected QueryRow")}
	}
	row := t.rows[0]
	t.rows = t.rows[1:]
	return row
}

func (t *fakeTransaction) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, execCall{query: query, args: args})
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("unexpected Scan arity")
	}
	for index := range dest {
		target := reflect.ValueOf(dest[index])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return errors.New("Scan target must be a pointer")
		}
		value := reflect.ValueOf(r.values[index])
		if !value.IsValid() {
			target.Elem().SetZero()
			continue
		}
		if !value.Type().AssignableTo(target.Elem().Type()) {
			return errors.New("Scan value type mismatch")
		}
		target.Elem().Set(value)
	}
	return nil
}

func TestCreateSubscriptionPersistsDefaultPolicyAndReplays(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	store := NewStore(nil, &testIDGen{next: 100})
	tx := &fakeTransaction{rows: []fakeRow{{values: []any{now, now}}}}
	created, replay, err := store.createSubscription(context.Background(), tx, 11, 22, 33, domain.DefaultSubscriptionCommand())
	if err != nil {
		t.Fatal(err)
	}
	if replay || created.ID != 101 || created.SubscriberUserID != 33 || !created.Enabled {
		t.Fatalf("unexpected created subscription: %+v replay=%v", created, replay)
	}
	if len(tx.queries) != 1 || len(tx.queries[0].args) != 6 || len(tx.execs) != 2 {
		t.Fatalf("unexpected insert call: %+v", tx.queries)
	}
	if events := string(tx.queries[0].args[4].([]byte)); !strings.Contains(events, "TASK_ACTION_REQUIRED") || !strings.Contains(events, "TASK_COMPLETED") {
		t.Fatalf("default events not frozen: %s", events)
	}

	events, _ := json.Marshal(domain.DefaultSubscriptionCommand().Events)
	channels, _ := json.Marshal(domain.DefaultSubscriptionCommand().Channels)
	replayTx := &fakeTransaction{rows: []fakeRow{
		{err: pgx.ErrNoRows},
		{values: []any{int64(101), events, true, int64(1), int64(33), int64(33), now, now}},
		{values: []any{channels}},
	}}
	replayed, replay, err := store.createSubscription(context.Background(), replayTx, 11, 22, 33, domain.DefaultSubscriptionCommand())
	if err != nil || !replay || replayed.ID != 101 {
		t.Fatalf("replay=%v item=%+v err=%v", replay, replayed, err)
	}
}

func TestRecordFactFreezesPresentationAndWritesSharedOutbox(t *testing.T) {
	store := NewStore(nil, &testIDGen{next: 700})
	subscribers := []byte(`[{"id":"50","subscriberUserId":"60","version":"3","events":["TASK_ACTION_REQUIRED"],"channels":[{"type":"IN_APP"}],"enabled":true}]`)
	tx := &fakeTransaction{rows: []fakeRow{
		{values: []any{"补充用户查询接口", int64(202), int64(60), 2}},
		{values: []any{subscribers}},
		{values: []any{int64(701)}},
	}}
	fact := domain.TaskNotificationFact{
		TenantID: 101, TaskID: 201, TaskTurnID: 202, FactType: domain.FactActionRequired,
		SourceType: domain.SourceContinuationCheckpoint, SourceID: 203, ReasonCode: "MORE_INFORMATION_REQUIRED", Focus: domain.FocusEvaluation,
		OccurredAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	}
	if err := store.recordFact(context.Background(), tx, fact); err != nil {
		t.Fatal(err)
	}
	if len(tx.execs) != 3 || !strings.Contains(tx.execs[2].query, "workforce_outbox_events") {
		t.Fatalf("shared outbox was not written: %+v", tx.execs)
	}
	if got := tx.execs[2].args[1]; got != domain.OutboxAggregate {
		t.Fatalf("aggregate type=%v", got)
	}
	payload := tx.execs[2].args[4].([]byte)
	want := `{"schemaVersion":1,"recordId":"701","taskId":"201","taskTurnId":"202"}`
	if string(payload) != want {
		t.Fatalf("outbox payload=%s want=%s", payload, want)
	}
	insertArgs := tx.queries[2].args
	if got := insertArgs[22]; got != "/workforce/tasks/201?turn=202&focus=evaluation" {
		t.Fatalf("deep link snapshot=%v", got)
	}
	if got := insertArgs[23]; got != "workforce-task:201:turn:202:TASK_ACTION_REQUIRED:CONTINUATION_CHECKPOINT:203:user:60" {
		t.Fatalf("idempotency identity=%v", got)
	}
	if got := insertArgs[26]; !got.(time.Time).Equal(fact.OccurredAt) {
		t.Fatalf("occurredAt=%v want=%v", got, fact.OccurredAt)
	}
}

func TestRecordFactFreezesGroupMentionOnlyForActionRequired(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		factType    domain.FactType
		sourceType  domain.SourceType
		focus       domain.Focus
		wantMention int64
	}{
		{name: "action required mentions creator", factType: domain.FactActionRequired, sourceType: domain.SourceContinuationCheckpoint, focus: domain.FocusEvaluation, wantMention: 60},
		{name: "completed does not mention", factType: domain.FactCompleted, sourceType: domain.SourceFinalizationRun, focus: domain.FocusResult, wantMention: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := NewStore(nil, &testIDGen{next: 800})
			subscribers := []byte(`[{"id":"50","subscriberUserId":"60","version":"3","events":["` + string(testCase.factType) + `"],"channels":[{"kind":"GROUP_ROUTE","routeId":"9007199254740993"}],"enabled":true}]`)
			tx := &fakeTransaction{rows: []fakeRow{
				{values: []any{"群通知任务", int64(202), int64(60), 2}},
				{values: []any{subscribers}},
				{values: []any{int64(801)}},
			}}
			err := store.recordFact(context.Background(), tx, domain.TaskNotificationFact{
				TenantID: 101, TaskID: 201, TaskTurnID: 202, FactType: testCase.factType,
				SourceType: testCase.sourceType, SourceID: 203, ReasonCode: "TEST", Focus: testCase.focus,
				OccurredAt: time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(tx.execs) != 3 || !strings.Contains(tx.execs[0].query, "workforce_task_notification_record_channels") {
				t.Fatalf("record channel was not frozen: %#v", tx.execs)
			}
			if got := tx.execs[0].args[4]; got != int64(9007199254740993) {
				t.Fatalf("route ID=%v", got)
			}
			if got := tx.execs[0].args[5]; got != testCase.wantMention {
				t.Fatalf("mention user=%v want=%v", got, testCase.wantMention)
			}
		})
	}
}

func TestRecordFactAcceptsOneHundredTwentyUnicodeCharacters(t *testing.T) {
	store := NewStore(nil, &testIDGen{next: 720})
	subscribers := []byte(`[{"id":"50","subscriberUserId":"60","version":"1","events":["TASK_COMPLETED"],"channels":[{"type":"IN_APP"}],"enabled":true}]`)
	tx := &fakeTransaction{rows: []fakeRow{
		{values: []any{strings.Repeat("任", 120), int64(202), int64(60), 1}},
		{values: []any{subscribers}},
		{values: []any{int64(721)}},
	}}
	err := store.recordFact(context.Background(), tx, domain.TaskNotificationFact{
		TenantID: 101, TaskID: 201, TaskTurnID: 202, FactType: domain.FactCompleted,
		SourceType: domain.SourceFinalizationRun, SourceID: 203, ReasonCode: "TASK_TURN_SUCCEEDED", Focus: domain.FocusResult,
		OccurredAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("120-character Unicode Task name rejected: %v", err)
	}
	if len(tx.execs) != 3 {
		t.Fatalf("Unicode Task fact did not write outbox: %+v", tx.execs)
	}
}

func TestRecordFactRejectsStaleTurnBeforeWriting(t *testing.T) {
	store := NewStore(nil, &testIDGen{next: 10})
	tx := &fakeTransaction{rows: []fakeRow{{values: []any{"Task", int64(999), int64(4), 1}}}}
	err := store.recordFact(context.Background(), tx, domain.TaskNotificationFact{
		TenantID: 1, TaskID: 2, TaskTurnID: 3, FactType: domain.FactCompleted,
		SourceType: domain.SourceFinalizationRun, SourceID: 4, Focus: domain.FocusResult, OccurredAt: time.Now().UTC(),
	})
	if !errors.Is(err, domain.ErrStaleTurn) {
		t.Fatalf("error=%v, want stale turn", err)
	}
	if len(tx.execs) != 0 {
		t.Fatal("stale Turn wrote an outbox record")
	}
}

func TestRecordFactFreezesSuppressedRecordWithoutOutbox(t *testing.T) {
	store := NewStore(nil, &testIDGen{next: 80})
	subscribers := []byte(`[{"id":"50","subscriberUserId":"60","version":"3","events":["TASK_COMPLETED"],"channels":[{"type":"IN_APP"}],"enabled":false}]`)
	tx := &fakeTransaction{rows: []fakeRow{
		{values: []any{"Task", int64(22), int64(60), 1}},
		{values: []any{subscribers}},
		{values: []any{int64(81)}},
	}}
	err := store.recordFact(context.Background(), tx, domain.TaskNotificationFact{
		TenantID: 1, TaskID: 21, TaskTurnID: 22, FactType: domain.FactCompleted,
		SourceType: domain.SourceFinalizationRun, SourceID: 23, Focus: domain.FocusResult, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.execs) != 0 {
		t.Fatal("suppressed notification wrote an outbox event")
	}
	if got := tx.queries[2].args[25]; got != domain.RecordSuppressed {
		t.Fatalf("record status=%v, want SUPPRESSED", got)
	}
}

func TestRecordFactDetectsSameIdentityDifferentFrozenContent(t *testing.T) {
	store := NewStore(nil, &testIDGen{next: 70})
	subscribers := []byte(`[{"id":"50","subscriberUserId":"60","version":"3","events":["TASK_COMPLETED"],"channels":[{"type":"IN_APP"}],"enabled":true}]`)
	tx := &fakeTransaction{rows: []fakeRow{
		{values: []any{"Task", int64(22), int64(60), 1}},
		{values: []any{subscribers}},
		{err: pgx.ErrNoRows},
		{values: []any{"different-digest"}},
	}}
	err := store.recordFact(context.Background(), tx, domain.TaskNotificationFact{
		TenantID: 1, TaskID: 21, TaskTurnID: 22, FactType: domain.FactCompleted,
		SourceType: domain.SourceFinalizationRun, SourceID: 23, Focus: domain.FocusResult, OccurredAt: time.Now().UTC(),
	})
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("error=%v, want idempotency conflict", err)
	}
	if len(tx.execs) != 0 {
		t.Fatal("conflicting replay wrote outbox")
	}
}
