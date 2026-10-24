package outbox

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestStoreListsEventsInOwnerAndTypeScopeBeforeLimit(t *testing.T) {
	store, mock, closeDB := newMockStore(t)
	defer closeDB()
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	mock.ExpectQuery(`(?s)SELECT .* FROM sys_outbox_event\s+WHERE eventOwner=\? AND eventType IN \(\?,\?\).*ORDER BY createTime ASC LIMIT \?`).
		WithArgs("file", "UPLOAD_TASK_READY", "FILE_PROCESS_TASK", now, now, 10).
		WillReturnRows(outboxRows().AddRow(
			int64(11), "file-event", "file", "UPLOAD_TASK_READY", "upload", "task-1", `{}`, "PENDING", 0,
			nil, nil, nil, nil, nil, now, now,
		))

	events, err := store.ListReady(context.Background(), "file", []string{"UPLOAD_TASK_READY", "FILE_PROCESS_TASK"}, 10)
	if err != nil {
		t.Fatalf("ListReady() error = %v", err)
	}
	if len(events) != 1 || events[0].EventOwner != "file" || events[0].EventType != "UPLOAD_TASK_READY" {
		t.Fatalf("ListReady() events = %+v", events)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestStoreListsPayloadBoundedEventsWithoutReturningOversizedBody(t *testing.T) {
	store, mock, closeDB := newMockStore(t)
	defer closeDB()
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	mock.ExpectQuery(`(?s)SELECT .*OCTET_LENGTH\(COALESCE\(payload, ''\)\).*FROM sys_outbox_event\s+WHERE eventOwner=\? AND scopeId=\? AND eventType IN \(\?\).*ORDER BY createTime ASC LIMIT \?`).
		WithArgs(1024, 1024, "cache-governance", "system:global", "CACHE_INVALIDATE_V1", now, now, 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "eventId", "eventOwner", "scopeId", "eventType", "aggregateType", "aggregateId", "payload", "payload_oversized", "status",
			"retryCount", "nextRetryAt", "errorMsg", "leaseOwner", "leaseToken", "leaseUntil", "createTime", "updateTime",
		}).AddRow(
			int64(15), "bounded-event", "cache-governance", "system:global", "CACHE_INVALIDATE_V1", "cache-invalidation", "digest", "", 1, "PENDING", 0,
			nil, nil, nil, nil, nil, now, now,
		))

	events, err := store.ListReadyForScopePayloadBounded(context.Background(), "cache-governance", "system:global", []string{"CACHE_INVALIDATE_V1"}, 1024, 5)
	if err != nil {
		t.Fatalf("ListReadyForScopePayloadBounded() error = %v", err)
	}
	if len(events) != 1 || !events[0].PayloadOversized || events[0].Payload != "" {
		t.Fatalf("bounded list exposed an oversized payload: %+v", events)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestStoreFindsOnlyExactReadyOwnerTypeAndEventID(t *testing.T) {
	store, mock, closeDB := newMockStore(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	mock.ExpectQuery(`(?s)SELECT .* FROM sys_outbox_event\s+WHERE eventOwner=\? AND eventType=\? AND eventId=\? AND \(.*LIMIT 1`).
		WithArgs("notification", "notification.dispatch", "notification:delivery-acceptance", now, now).
		WillReturnRows(outboxRows().AddRow(
			int64(14), "notification:delivery-acceptance", "notification", "notification.dispatch", "notification", "delivery-acceptance", `{}`, "PENDING", 0,
			nil, nil, nil, nil, nil, now, now,
		))

	event, err := store.FindReady(context.Background(), "notification", "notification.dispatch", "notification:delivery-acceptance")
	if err != nil || event == nil {
		t.Fatalf("FindReady() event=%+v err=%v", event, err)
	}
	if event.ID != 14 || event.EventID != "notification:delivery-acceptance" || event.EventType != "notification.dispatch" {
		t.Fatalf("FindReady() event=%+v", event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestStoreClaimAndMarkRequireOwnerTypeAndFencingToken(t *testing.T) {
	store, mock, closeDB := newMockStore(t)
	defer closeDB()
	now := time.Date(2026, 7, 22, 10, 5, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	mock.ExpectExec(`(?s)UPDATE sys_outbox_event\s+SET status='PROCESSING', leaseOwner=\?, leaseToken=\?, leaseUntil=\?, updateTime=\?.*WHERE id=\? AND eventOwner=\? AND eventType=\?`).
		WithArgs("file-relay", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, int64(12), "file", "UPLOAD_TASK_READY", now, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	lease, claimed, err := store.Claim(context.Background(), "file", "UPLOAD_TASK_READY", 12, "file-relay")
	if err != nil || !claimed || lease == nil || lease.Token == "" {
		t.Fatalf("Claim() lease=%+v claimed=%t err=%v", lease, claimed, err)
	}

	mock.ExpectExec(`(?s)UPDATE sys_outbox_event\s+SET status=\?, errorMsg=\?, retryCount=\?, nextRetryAt=\?, leaseOwner=NULL, leaseToken=NULL, leaseUntil=NULL, updateTime=\?.*WHERE id=\? AND eventOwner=\? AND eventType=\? AND status='PROCESSING' AND leaseToken=\?`).
		WithArgs("SENT", nil, 0, nil, now, int64(12), "file", "UPLOAD_TASK_READY", lease.Token).
		WillReturnResult(sqlmock.NewResult(0, 1))
	applied, err := store.Mark(context.Background(), "file", "UPLOAD_TASK_READY", 12, lease.Token, "SENT", "", 0, nil)
	if err != nil || !applied {
		t.Fatalf("Mark() applied=%t err=%v", applied, err)
	}

	// A stale completion uses a different token and cannot regress the state.
	mock.ExpectExec(`(?s)UPDATE sys_outbox_event\s+SET status=\?.*leaseToken=\?`).
		WithArgs("FAILED", "late worker", 1, nil, now, int64(12), "file", "UPLOAD_TASK_READY", "stale-token").
		WillReturnResult(sqlmock.NewResult(0, 0))
	applied, err = store.Mark(context.Background(), "file", "UPLOAD_TASK_READY", 12, "stale-token", "FAILED", "late worker", 1, nil)
	if err != nil || applied {
		t.Fatalf("stale Mark() applied=%t err=%v", applied, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestStoreScopedClaimAndMarkNeverUseLocalOrNullCompatibility(t *testing.T) {
	store, mock, closeDB := newMockStore(t)
	defer closeDB()
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	// Exact scopeId=? is deliberately required here. The generic local reader
	// may drain legacy NULL rows, but a DG5 system:global worker must not claim
	// or mark local/NULL rows even if an adversary races a scope rewrite after
	// the initial scoped list.
	mock.ExpectExec(`(?s)UPDATE sys_outbox_event\s+SET status='PROCESSING'.*WHERE id=\? AND eventOwner=\? AND scopeId=\? AND eventType=\?`).
		WithArgs("cache-relay", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, int64(91), "cache-governance", "system:global", "CACHE_INVALIDATE_V1", now, now).
		WillReturnResult(sqlmock.NewResult(0, 0))
	lease, claimed, err := store.ClaimForScope(context.Background(), "cache-governance", "system:global", "CACHE_INVALIDATE_V1", 91, "cache-relay")
	if err != nil || claimed || lease != nil {
		t.Fatalf("strict scoped claim unexpectedly crossed scope: lease=%+v claimed=%t err=%v", lease, claimed, err)
	}

	mock.ExpectExec(`(?s)UPDATE sys_outbox_event\s+SET status=\?.*WHERE id=\? AND eventOwner=\? AND scopeId=\? AND eventType=\? AND status='PROCESSING' AND leaseToken=\?`).
		WithArgs("DONE", nil, 0, nil, now, int64(91), "cache-governance", "system:global", "CACHE_INVALIDATE_V1", "stale-token").
		WillReturnResult(sqlmock.NewResult(0, 0))
	applied, err := store.MarkForScope(context.Background(), "cache-governance", "system:global", "CACHE_INVALIDATE_V1", 91, "stale-token", "DONE", "", 0, nil)
	if err != nil || applied {
		t.Fatalf("strict scoped mark unexpectedly crossed scope: applied=%t err=%v", applied, err)
	}
	if _, _, err := store.ClaimForScope(context.Background(), "cache-governance", "", "CACHE_INVALIDATE_V1", 91, "cache-relay"); err == nil {
		t.Fatal("empty scope unexpectedly widened to local/NULL compatibility")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestStoreListsUnknownOwnerEventsForFailClosedHandling(t *testing.T) {
	store, mock, closeDB := newMockStore(t)
	defer closeDB()
	now := time.Date(2026, 7, 22, 10, 7, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	mock.ExpectQuery(`(?s)SELECT .* FROM sys_outbox_event\s+WHERE eventOwner=\? AND eventType NOT IN \(\?\).*LIMIT \?`).
		WithArgs("notification", "notification.dispatch", now, now, 5).
		WillReturnRows(outboxRows().AddRow(
			int64(13), "unknown-event", "notification", "notification.unknown", "notification", "delivery-2", `{}`, "PENDING", 0,
			nil, nil, nil, nil, nil, now, now,
		))
	events, err := store.ListUnknownReady(context.Background(), "notification", []string{"notification.dispatch"}, 5)
	if err != nil {
		t.Fatalf("ListUnknownReady() error = %v", err)
	}
	if len(events) != 1 || events[0].EventType != "notification.unknown" {
		t.Fatalf("ListUnknownReady() events=%+v", events)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestConsumeGuardReclaimsFailedLeaseAndFencesLateCompletion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	guard := NewConsumeGuard(sqlx.NewDb(db, "sqlmock"))
	now := time.Date(2026, 7, 22, 10, 10, 0, 0, time.UTC)
	guard.now = func() time.Time { return now }

	mock.ExpectExec(`(?s)INSERT INTO sys_message_consume_log`).
		WithArgs(now.UnixNano(), "message-1", "notification-dispatch", "delivery-1", "worker-b", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, now).
		WillReturnError(errors.New("duplicate key"))
	mock.ExpectExec(`(?s)UPDATE sys_message_consume_log\s+SET status='PROCESSING'.*status='FAILED' OR \(status='PROCESSING'`).
		WithArgs("delivery-1", "worker-b", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, "message-1", "notification-dispatch", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	lease, claimed, err := guard.Begin(context.Background(), "message-1", "notification-dispatch", "worker-b", "delivery-1")
	if err != nil || !claimed || lease == nil {
		t.Fatalf("Begin() lease=%+v claimed=%t err=%v", lease, claimed, err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_message_consume_log
SET status=?, detail=?, leaseOwner=NULL, leaseToken=NULL, leaseUntil=NULL, updateTime=?
WHERE messageId=? AND consumer=? AND status='PROCESSING' AND leaseToken=?`)).
		WithArgs("DONE", "delivery-1", now, "message-1", "notification-dispatch", "late-token").
		WillReturnResult(sqlmock.NewResult(0, 0))
	applied, err := guard.Finish(context.Background(), "message-1", "notification-dispatch", "late-token", "delivery-1")
	if err != nil || applied {
		t.Fatalf("late Finish() applied=%t err=%v", applied, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestConsumeGuardSignalsLiveLeaseContentionForBrokerRequeue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	guard := NewConsumeGuard(sqlx.NewDb(db, "sqlmock"))
	now := time.Date(2026, 7, 22, 10, 20, 0, 0, time.UTC)
	guard.now = func() time.Time { return now }

	mock.ExpectExec(`(?s)INSERT INTO sys_message_consume_log`).
		WithArgs(now.UnixNano(), "message-2", "notification-dispatch", "delivery-2", "worker-b", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, now).
		WillReturnError(errors.New("duplicate key"))
	mock.ExpectExec(`(?s)UPDATE sys_message_consume_log\s+SET status='PROCESSING'`).
		WithArgs("delivery-2", "worker-b", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, "message-2", "notification-dispatch", now).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT status, leaseUntil\s+FROM sys_message_consume_log WHERE messageId=\? AND consumer=\? LIMIT 1`).
		WithArgs("message-2", "notification-dispatch").
		WillReturnRows(sqlmock.NewRows([]string{"status", "leaseUntil"}).AddRow("PROCESSING", now.Add(5*time.Minute)))

	lease, claimed, err := guard.Begin(context.Background(), "message-2", "notification-dispatch", "worker-b", "delivery-2")
	if lease != nil || claimed || !errors.Is(err, ErrConsumeLeaseHeld) {
		t.Fatalf("Begin() lease=%+v claimed=%t err=%v, want retryable live-lease contention", lease, claimed, err)
	}
	var requeueable interface{ Requeue() bool }
	if !errors.As(err, &requeueable) || !requeueable.Requeue() {
		t.Fatalf("Begin() error=%v, want a requeueable consume lease error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestConsumeGuardTreatsCompletedDuplicateAsAnAcknowledgableNoOp(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	guard := NewConsumeGuard(sqlx.NewDb(db, "sqlmock"))
	now := time.Date(2026, 7, 22, 10, 25, 0, 0, time.UTC)
	guard.now = func() time.Time { return now }

	mock.ExpectExec(`(?s)INSERT INTO sys_message_consume_log`).
		WithArgs(now.UnixNano(), "message-3", "notification-dispatch", "delivery-3", "worker-c", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, now).
		WillReturnError(errors.New("duplicate key"))
	mock.ExpectExec(`(?s)UPDATE sys_message_consume_log\s+SET status='PROCESSING'`).
		WithArgs("delivery-3", "worker-c", sqlmock.AnyArg(), now.Add(defaultLeaseTTL), now, "message-3", "notification-dispatch", now).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT status, leaseUntil\s+FROM sys_message_consume_log WHERE messageId=\? AND consumer=\? LIMIT 1`).
		WithArgs("message-3", "notification-dispatch").
		WillReturnRows(sqlmock.NewRows([]string{"status", "leaseUntil"}).AddRow("DONE", nil))

	lease, claimed, err := guard.Begin(context.Background(), "message-3", "notification-dispatch", "worker-c", "delivery-3")
	if lease != nil || claimed || err != nil {
		t.Fatalf("Begin() lease=%+v claimed=%t err=%v, want completed no-op", lease, claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestStoreRejectsEventsWithoutAnExplicitOwner(t *testing.T) {
	store, _, closeDB := newMockStore(t)
	defer closeDB()
	if err := store.Append(context.Background(), &Event{EventID: "e", EventType: "t"}); err == nil {
		t.Fatal("Append() accepted event without eventOwner")
	}
}

func TestStoreRejectsZeroIDInsteadOfTimestampCollisionFallback(t *testing.T) {
	store, _, closeDB := newMockStore(t)
	defer closeDB()
	if err := store.Append(context.Background(), &Event{EventID: "e", EventOwner: "cache-governance", ScopeID: "system:global", EventType: "CACHE_INVALIDATE_V1"}); err == nil {
		t.Fatal("Append() accepted zero ID and would have used a timestamp collision fallback")
	}
	if err := store.AppendBatch(context.Background(), []*Event{{EventID: "batch-e", EventOwner: "cache-governance", ScopeID: "system:global", EventType: "CACHE_INVALIDATE_V1"}}); err == nil {
		t.Fatal("AppendBatch() accepted zero ID and would have used a timestamp collision fallback")
	}
}

func TestPostgresOutboxSQLQuotesCamelCaseColumns(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	store := NewStore(sqlx.NewDb(db, "postgres"))
	query := store.sql(`UPDATE sys_outbox_event SET eventOwner=?, leaseToken=?, updateTime=? WHERE eventId=?`)
	for _, expected := range []string{`"eventOwner"`, `"leaseToken"`, `"updateTime"`, `"eventId"`} {
		if !regexp.MustCompile(regexp.QuoteMeta(expected)).MatchString(query) {
			t.Fatalf("PostgreSQL outbox SQL missing %s: %s", expected, query)
		}
	}

	guard := NewConsumeGuard(sqlx.NewDb(db, "postgres"))
	guardQuery := guard.sql(`SELECT leaseUntil FROM sys_message_consume_log WHERE messageId=?`)
	for _, expected := range []string{`"leaseUntil"`, `"messageId"`} {
		if !regexp.MustCompile(regexp.QuoteMeta(expected)).MatchString(guardQuery) {
			t.Fatalf("PostgreSQL consume SQL missing %s: %s", expected, guardQuery)
		}
	}
}

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return NewStore(sqlx.NewDb(db, "sqlmock")), mock, func() { _ = db.Close() }
}

func outboxRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "eventId", "eventOwner", "eventType", "aggregateType", "aggregateId", "payload", "status",
		"retryCount", "nextRetryAt", "errorMsg", "leaseOwner", "leaseToken", "leaseUntil", "createTime", "updateTime",
	})
}
