package infrastructure

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestOutboxAdapterRequiresActiveTransaction(t *testing.T) {
	raw, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	adapter := NewOutboxAdapter(sqlx.NewDb(raw, "sqlmock"), func() int64 { return 1 })
	event, _ := cachepolicy.NewInvalidationEnvelope("event-no-tx", cachepolicy.DataClassConfigPublicScalar)
	if err := adapter.Append(context.Background(), event); err == nil {
		t.Fatal("unbound cache invalidation append unexpectedly succeeded")
	}
}

func TestOutboxAdapterAppendsWithinCommitAndRollsBackWithBusinessTransaction(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	db := sqlx.NewDb(raw, "sqlmock")
	adapter := NewOutboxAdapter(db, func() int64 { return 2 })
	transactor := store.NewSQLXTransactor(db)
	event, _ := cachepolicy.NewInvalidationEnvelope("event-commit", cachepolicy.DataClassDictPublicItems)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO sys_outbox_event")).
		WithArgs(sqlmock.AnyArg(), event.EventID, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.CacheInvalidationEventType, cachepolicy.CacheInvalidationAggregate, event.TargetDigest, sqlmock.AnyArg(), "PENDING", 0, nil, nil, nil, nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return adapter.Append(txCtx, event)
	}); err != nil {
		t.Fatalf("append committed invalidation: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("commit expectations: %v", err)
	}

	rollbackEvent, _ := cachepolicy.NewInvalidationEnvelope("event-rollback", cachepolicy.DataClassConfigPublicScalar)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO sys_outbox_event")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()
	err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		if appendErr := adapter.Append(txCtx, rollbackEvent); appendErr != nil {
			return appendErr
		}
		return errors.New("force business rollback")
	})
	if err == nil {
		t.Fatal("forced business rollback unexpectedly committed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("rollback expectations: %v", err)
	}
}

func TestOutstandingInvalidationQueryFailsClosedForEveryNonDoneState(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		query := outstandingInvalidationQuery(postgres)
		if !strings.Contains(query, "status <> 'DONE'") {
			t.Fatalf("freshness query does not fail closed for an unknown non-DONE state: %s", query)
		}
		for _, required := range []string{"eventOwner", "scopeId", "eventType", "aggregateType", "aggregateId"} {
			if !strings.Contains(query, required) {
				t.Fatalf("freshness query omitted strict scope dimension %q: %s", required, query)
			}
		}
	}
}
