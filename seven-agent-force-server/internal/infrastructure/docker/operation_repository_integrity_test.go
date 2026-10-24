package docker

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

type operationRepositoryTestProvider struct {
	db      *sqlx.DB
	dialect string
}

func (p operationRepositoryTestProvider) Driver() string  { return p.dialect }
func (p operationRepositoryTestProvider) Dialect() string { return p.dialect }
func (p operationRepositoryTestProvider) DB() *sql.DB     { return p.db.DB }
func (p operationRepositoryTestProvider) SQLX() *sqlx.DB  { return p.db }
func (p operationRepositoryTestProvider) Transactor() store.Transactor {
	return store.NewSQLXTransactor(p.db)
}
func (p operationRepositoryTestProvider) Configured() bool { return true }
func (p operationRepositoryTestProvider) Close() error     { return nil }

func newOperationRepositorySQLMock(t *testing.T) (*OperationRepository, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	repository, err := NewOperationRepository(operationRepositoryTestProvider{
		db:      sqlx.NewDb(rawDB, "mysql"),
		dialect: "mysql",
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository, mock
}

func TestComposeOperationLookupsUseBoundedSetQueries(t *testing.T) {
	repository, mock := newOperationRepositorySQLMock(t)
	mock.ExpectQuery(`(?s)FROM docker_operation.*WHERE id IN \(\?, \?\)`).
		WithArgs(int64(3), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	byID, err := repository.FindOperationsByIDs(context.Background(), []int64{5, 3, 5})
	if err != nil || len(byID) != 0 {
		t.Fatalf("find operations by ids=%v err=%v", byID, err)
	}

	mock.ExpectQuery(`(?s)ROW_NUMBER\(\) OVER \(PARTITION BY targetId.*WHERE targetType = \? AND targetId IN \(\?, \?\).*target_rank = 1`).
		WithArgs("compose", "compose-a", "compose-b").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	latest, err := repository.LatestOperationsByTargetIDs(context.Background(), "compose", []string{"compose-b", "compose-a", "compose-b"})
	if err != nil || len(latest) != 0 {
		t.Fatalf("latest operations by targets=%v err=%v", latest, err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestAppendEventRejectsMissingParentInsideTransaction(t *testing.T) {
	repository, mock := newOperationRepositorySQLMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT operationId FROM docker_operation_integrity_guard WHERE operationId = ? FOR UPDATE")).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("INSERT IGNORE INTO docker_operation_integrity_guard (operationId, createTime) VALUES (?, NOW())")).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT operationId FROM docker_operation_integrity_guard WHERE operationId = ? FOR UPDATE")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"operationId"}).AddRow(42))
	mock.ExpectQuery("SELECT id, operationType,.*FROM docker_operation.*WHERE id = .*FOR UPDATE").
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err := repository.AppendEvent(context.Background(), OperationEventRecord{
		ID:          101,
		OperationID: 42,
		EventType:   string(OperationEventState),
	})
	if !errors.Is(err, ErrOperationParentNotFound) {
		t.Fatalf("AppendEvent error = %v, want ErrOperationParentNotFound", err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestAppendEventRetryReturnsPriorMatchingOutcome(t *testing.T) {
	repository, mock := newOperationRepositorySQLMock(t)
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT operationId FROM docker_operation_integrity_guard").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"operationId"}).AddRow(42))
	mock.ExpectQuery("SELECT id, operationType,.*FROM docker_operation.*WHERE id = .*FOR UPDATE").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery("SELECT id, operationId, sequence, eventType, stage, percent, message, payloadJson, occurredAt").
		WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "operationId", "sequence", "eventType", "stage", "percent", "message", "payloadJson", "occurredAt",
		}).AddRow(101, 42, 7, "STATE", "running", 10, "started", nil, now))
	mock.ExpectCommit()

	err := repository.AppendEvent(context.Background(), OperationEventRecord{
		ID:          101,
		OperationID: 42,
		EventType:   "STATE",
		Stage:       sql.NullString{String: "running", Valid: true},
		Percent:     sql.NullInt64{Int64: 10, Valid: true},
		Message:     sql.NullString{String: "started", Valid: true},
	})
	if err != nil {
		t.Fatalf("AppendEvent retry: %v", err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestDeleteOperationLocksParentThenDeletesChildrenFirst(t *testing.T) {
	repository, mock := newOperationRepositorySQLMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT operationId FROM docker_operation_integrity_guard").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"operationId"}).AddRow(42))
	mock.ExpectQuery("SELECT id, operationType,.*FROM docker_operation.*WHERE id = .*FOR UPDATE").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectExec("DELETE FROM docker_operation_event WHERE operationId = .*").WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM docker_operation WHERE id = .*").WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	deleted, err := repository.DeleteOperationWithEvents(context.Background(), 42)
	if err != nil {
		t.Fatalf("DeleteOperationWithEvents: %v", err)
	}
	if !deleted {
		t.Fatal("DeleteOperationWithEvents deleted=false, want true")
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestTrimEventsUsesGuardParentChildLockOrder(t *testing.T) {
	repository, mock := newOperationRepositorySQLMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT operationId FROM docker_operation_integrity_guard").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"operationId"}).AddRow(42))
	mock.ExpectQuery("SELECT id, operationType,.*FROM docker_operation.*WHERE id = .*FOR UPDATE").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectExec("DELETE FROM docker_operation_event.*sequence <= COALESCE").
		WithArgs(int64(42), 100, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectCommit()

	if err := repository.TrimEvents(context.Background(), 42, 100); err != nil {
		t.Fatalf("TrimEvents: %v", err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestCleanupOrphanRechecksVersionAndWritesAudit(t *testing.T) {
	repository, mock := newOperationRepositorySQLMock(t)
	command := OperationEventOrphanCleanupCommand{
		AuditID:                  901,
		DiagnosticID:             "docker-operation-event:101:42:7",
		EventID:                  101,
		OperationID:              42,
		Sequence:                 7,
		ExpectedIntegrityVersion: 3,
		ActorUserID:              88,
		ActorUsername:            "integrity-admin",
		Reason:                   "approved DG3 orphan cleanup",
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT operationId FROM docker_operation_integrity_guard").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"operationId"}).AddRow(42))
	mock.ExpectQuery("SELECT diagnosticId, eventId, operationId, sequence, expectedIntegrityVersion, action, result, actorUserId, actorUsername, reason").
		WithArgs(int64(901)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT id, operationType,.*FROM docker_operation.*WHERE id = .*FOR UPDATE").WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT id, operationId, sequence, diagnosticId, integrityVersion").
		WithArgs(int64(101), "QUARANTINED").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "operationId", "sequence", "diagnosticId", "integrityVersion",
			"integrityScope", "integrityRelationshipType", "integrityReason", "occurredAt",
		}).AddRow(101, 42, 7, command.DiagnosticID, 3,
			operationEventIntegrityScope, operationEventIntegrityRelationshipType, operationEventIntegrityReason,
			time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)))
	mock.ExpectExec("DELETE FROM docker_operation_event.*integrityVersion = .*").
		WithArgs(int64(101), int64(42), int64(7), command.DiagnosticID, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO docker_operation_event_orphan_audit").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repository.CleanupOrphanEvent(context.Background(), command)
	if err != nil {
		t.Fatalf("CleanupOrphanEvent: %v", err)
	}
	if result != OperationEventOrphanCleanupDeleted {
		t.Fatalf("CleanupOrphanEvent result=%q, want %q", result, OperationEventOrphanCleanupDeleted)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestOperationInsertMatchesDatabaseTimestampPrecision(t *testing.T) {
	requested := OperationRecord{
		ID:              42,
		OperationType:   "CREATE",
		TargetType:      "container",
		Status:          string(OperationStatusPending),
		TimeoutAt:       sql.NullTime{Time: time.Date(2026, 7, 30, 12, 0, 0, 987654321, time.UTC), Valid: true},
		ProgressPercent: 0,
	}
	persisted := requested
	persisted.TimeoutAt.Time = persisted.TimeoutAt.Time.Truncate(time.Second)

	if !operationInsertMatches(persisted, requested) {
		t.Fatal("same operation command should survive database timestamp precision normalization")
	}
}

func TestCleanupOrphanAuditReplayBindsExpectedIntegrityVersion(t *testing.T) {
	repository, mock := newOperationRepositorySQLMock(t)
	command := OperationEventOrphanCleanupCommand{
		AuditID:                  901,
		DiagnosticID:             "docker-operation-event:101:42:7",
		EventID:                  101,
		OperationID:              42,
		Sequence:                 7,
		ExpectedIntegrityVersion: 4,
		ActorUserID:              88,
		ActorUsername:            "integrity-admin",
		Reason:                   "approved DG3 orphan cleanup",
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT operationId FROM docker_operation_integrity_guard").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"operationId"}).AddRow(42))
	mock.ExpectQuery("SELECT diagnosticId, eventId, operationId, sequence, expectedIntegrityVersion, action, result, actorUserId, actorUsername, reason").
		WithArgs(int64(901)).
		WillReturnRows(sqlmock.NewRows([]string{
			"diagnosticId", "eventId", "operationId", "sequence", "expectedIntegrityVersion",
			"action", "result", "actorUserId", "actorUsername", "reason",
		}).AddRow(command.DiagnosticID, 101, 42, 7, 3, "DELETE", "DELETED", 88, "integrity-admin", command.Reason))
	mock.ExpectRollback()

	_, err := repository.CleanupOrphanEvent(context.Background(), command)
	if !errors.Is(err, ErrOperationEventMutationConflict) {
		t.Fatalf("CleanupOrphanEvent error=%v, want ErrOperationEventMutationConflict", err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestOperationIntegrityRequiresExplicitUnseededPermission(t *testing.T) {
	if operationActorHasExplicitPermission(OperationActor{IsAdmin: true}, OperationIntegrityCleanupPermission) {
		t.Fatal("administrator bypass must not expose an unseeded integrity cleanup capability")
	}
	if operationActorHasExplicitPermission(OperationActor{Permissions: []string{"admin:docker:*"}}, OperationIntegrityCleanupPermission) {
		t.Fatal("wildcard permission must not expose the independent integrity cleanup capability")
	}
	if !operationActorHasExplicitPermission(OperationActor{
		Permissions: []string{OperationIntegrityCleanupPermission},
	}, OperationIntegrityCleanupPermission) {
		t.Fatal("exact integrity cleanup permission should authorize the protected service boundary")
	}
}

func TestDG3MigrationsRejectUnregisteredNonConstraintIndexes(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve DG3 migration test path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	migrationPaths := []string{
		filepath.Join(repositoryRoot, "migrations", "mysql", "20260730150000_docker_operation_integrity.sql"),
		filepath.Join(repositoryRoot, "migrations", "mysql", "20260730160000_docker_operation_event_drop_foreign_key.sql"),
		filepath.Join(repositoryRoot, "migrations", "postgres", "20260730150000_docker_operation_integrity.sql"),
		filepath.Join(repositoryRoot, "migrations", "postgres", "20260730160000_docker_operation_event_drop_foreign_key.sql"),
	}
	mysqlKeyPattern := regexp.MustCompile(`(?im)(?:\bADD\s+KEY|^\s*KEY)\s+([A-Za-z0-9_]+)`)
	postgresIndexPattern := regexp.MustCompile(`(?im)^\s*CREATE\s+INDEX(?:\s+IF\s+NOT\s+EXISTS)?\s+([A-Za-z0-9_]+)`)
	for _, migrationPath := range migrationPaths {
		content, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range append(mysqlKeyPattern.FindAllSubmatch(content, -1), postgresIndexPattern.FindAllSubmatch(content, -1)...) {
			t.Errorf("%s contains unregistered non-constraint index %q; DG3 has no EXPLAIN-approved index registry", migrationPath, match[1])
		}
	}
}
