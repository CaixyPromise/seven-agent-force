package docker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

const (
	dg3MySQLDatabase    = "seven_database_governance_mysql"
	dg3PostgresDatabase = "seven_database_governance_pg"
)

func TestOperationIntegrityMySQL(t *testing.T) {
	runOperationIntegrityDatabaseSuite(t, "mysql", "DG3_TEST_MYSQL_DSN", dg3MySQLDatabase)
}

func TestOperationIntegrityPostgres(t *testing.T) {
	runOperationIntegrityDatabaseSuite(t, "postgres", "DG3_TEST_POSTGRES_DSN", dg3PostgresDatabase)
}

func TestDG3DatabaseDSNRequiresExactIsolatedName(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		dsn      string
		expected string
		wantErr  bool
	}{
		{
			name:     "mysql exact",
			dialect:  "mysql",
			dsn:      "fixture:fixture@tcp(127.0.0.1:43306)/" + dg3MySQLDatabase,
			expected: dg3MySQLDatabase,
		},
		{
			name:     "mysql development refused",
			dialect:  "mysql",
			dsn:      "fixture:fixture@tcp(127.0.0.1:3306)/lovely_seven",
			expected: dg3MySQLDatabase,
			wantErr:  true,
		},
		{
			name:     "postgres exact",
			dialect:  "postgres",
			dsn:      "postgres://fixture:fixture@127.0.0.1:45432/" + dg3PostgresDatabase,
			expected: dg3PostgresDatabase,
		},
		{
			name:     "postgres suffix refused",
			dialect:  "postgres",
			dsn:      "postgres://fixture:fixture@127.0.0.1:5432/" + dg3PostgresDatabase + "_backup",
			expected: dg3PostgresDatabase,
			wantErr:  true,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateDG3DatabaseDSN(testCase.dialect, testCase.dsn, testCase.expected)
			if testCase.wantErr && err == nil {
				t.Fatal("expected exact database-name guard to reject DSN")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("exact database-name guard: %v", err)
			}
		})
	}
}

func runOperationIntegrityDatabaseSuite(t *testing.T, dialect, environment, expectedDatabase string) {
	t.Helper()
	dsn := os.Getenv(environment)
	if dsn == "" {
		t.Skipf("set %s to the exact isolated %s database after applying migrations", environment, expectedDatabase)
	}
	foreignKeyMode := os.Getenv("DG3_EXPECT_FOREIGN_KEY")
	if foreignKeyMode != "present" && foreignKeyMode != "absent" {
		t.Fatalf("DG3_EXPECT_FOREIGN_KEY must be exactly present or absent, got %q", foreignKeyMode)
	}
	expectForeignKey := foreignKeyMode == "present"
	if err := validateDG3DatabaseDSN(dialect, dsn, expectedDatabase); err != nil {
		t.Fatal(err)
	}
	driver := dialect
	if dialect == "postgres" {
		driver = "pgx"
	}
	rawDB, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open isolated %s database: %v", dialect, err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	rawDB.SetMaxOpenConns(32)
	rawDB.SetMaxIdleConns(8)
	if err := rawDB.PingContext(context.Background()); err != nil {
		t.Fatalf("ping isolated %s database: %v", dialect, err)
	}
	db := sqlx.NewDb(rawDB, driver)
	provider := operationRepositoryTestProvider{db: db, dialect: dialect}
	repository, err := NewOperationRepository(provider)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := assertDG3CurrentDatabase(ctx, db, dialect, expectedDatabase); err != nil {
		t.Fatal(err)
	}
	if err := assertDG3SchemaAndConstraints(ctx, db, dialect, expectForeignKey); err != nil {
		t.Fatal(err)
	}
	if err := resetDG3Rows(ctx, db, dialect); err != nil {
		t.Fatal(err)
	}

	t.Run("missing-parent-and-idempotent-retry", func(t *testing.T) {
		missing := OperationEventRecord{ID: 1001, OperationID: 9001, EventType: "STATE"}
		if err := repository.AppendEvent(ctx, missing); !errors.Is(err, ErrOperationParentNotFound) {
			t.Fatalf("missing parent error=%v, want ErrOperationParentNotFound", err)
		}
		parent := dg3Operation(9001)
		if err := repository.InsertOperation(ctx, parent); err != nil {
			t.Fatal(err)
		}
		event := OperationEventRecord{
			ID:          1001,
			OperationID: parent.ID,
			EventType:   "STATE",
			Stage:       sql.NullString{String: "running", Valid: true},
			Message:     sql.NullString{String: "started", Valid: true},
		}
		if err := repository.AppendEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
		if err := repository.AppendEvent(ctx, event); err != nil {
			t.Fatalf("idempotent retry: %v", err)
		}
		conflict := event
		conflict.Message = sql.NullString{String: "different", Valid: true}
		if err := repository.AppendEvent(ctx, conflict); !errors.Is(err, ErrOperationEventMutationConflict) {
			t.Fatalf("conflicting retry error=%v", err)
		}
		if count := dg3EventCount(t, ctx, repository, parent.ID); count != 1 {
			t.Fatalf("event count=%d, want 1", count)
		}
	})

	t.Run("concurrent-new-parent-idempotency-and-conflict", func(t *testing.T) {
		const workerCount = 16
		parent := dg3Operation(9004)
		start := make(chan struct{})
		var wait sync.WaitGroup
		errs := make(chan error, workerCount)
		for index := 0; index < workerCount; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				errs <- repository.InsertOperation(ctx, parent)
			}()
		}
		close(start)
		wait.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("concurrent idempotent InsertOperation: %v", err)
			}
		}
		var parentCount int
		if err := db.GetContext(ctx, &parentCount, dg3SQL(dialect,
			`SELECT COUNT(1) FROM docker_operation WHERE id = ?`,
			`SELECT COUNT(1) FROM docker_operation WHERE id = $1`), parent.ID); err != nil {
			t.Fatal(err)
		}
		if parentCount != 1 {
			t.Fatalf("parent count=%d, want 1", parentCount)
		}

		conflicting := parent
		conflicting.OperationType = "REMOVE"
		if err := repository.InsertOperation(ctx, conflicting); !errors.Is(err, ErrOperationMutationConflict) {
			t.Fatalf("changed parent retry error=%v, want ErrOperationMutationConflict", err)
		}
	})

	t.Run("concurrent-sequence-allocation", func(t *testing.T) {
		parent := dg3Operation(9002)
		if err := repository.InsertOperation(ctx, parent); err != nil {
			t.Fatal(err)
		}
		const eventCount = 24
		var wait sync.WaitGroup
		errs := make(chan error, eventCount)
		for index := 0; index < eventCount; index++ {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				errs <- repository.AppendEvent(ctx, OperationEventRecord{
					ID:          2000 + int64(index),
					OperationID: parent.ID,
					EventType:   "PROGRESS",
					Message:     sql.NullString{String: fmt.Sprintf("event-%d", index), Valid: true},
				})
			}(index)
		}
		wait.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("concurrent AppendEvent: %v", err)
			}
		}
		events, err := repository.ListEvents(ctx, parent.ID, 0, eventCount+1)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != eventCount {
			t.Fatalf("events=%d, want %d", len(events), eventCount)
		}
		for index, event := range events {
			if event.Sequence != int64(index+1) {
				t.Fatalf("sequence[%d]=%d, want %d", index, event.Sequence, index+1)
			}
		}
	})

	t.Run("timeout-rollback-and-restart-recovery", func(t *testing.T) {
		parent := dg3Operation(9003)
		if err := repository.InsertOperation(ctx, parent); err != nil {
			t.Fatal(err)
		}
		lockTx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lockTx.ExecContext(ctx, dg3SQL(dialect,
			`SELECT operationId FROM docker_operation_integrity_guard WHERE operationId = ? FOR UPDATE`,
			`SELECT "operationId" FROM docker_operation_integrity_guard WHERE "operationId" = $1 FOR UPDATE`), parent.ID); err != nil {
			_ = lockTx.Rollback()
			t.Fatal(err)
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		event := OperationEventRecord{ID: 3001, OperationID: parent.ID, EventType: "STATE"}
		err = repository.AppendEvent(timeoutCtx, event)
		if err == nil {
			_ = lockTx.Rollback()
			t.Fatal("AppendEvent succeeded while integrity guard stayed locked past deadline")
		}
		if rollbackErr := lockTx.Rollback(); rollbackErr != nil {
			t.Fatal(rollbackErr)
		}
		if count := dg3EventCount(t, ctx, repository, parent.ID); count != 0 {
			t.Fatalf("timed-out event count=%d, want 0", count)
		}
		if err := repository.AppendEvent(ctx, event); err != nil {
			t.Fatalf("retry after timeout: %v", err)
		}
		restarted, err := NewOperationRepository(provider)
		if err != nil {
			t.Fatal(err)
		}
		if err := restarted.AppendEvent(ctx, event); err != nil {
			t.Fatalf("retry after repository restart: %v", err)
		}
		if count := dg3EventCount(t, ctx, restarted, parent.ID); count != 1 {
			t.Fatalf("restart recovery event count=%d, want 1", count)
		}
	})

	t.Run("concurrent-create-delete-final-invariant", func(t *testing.T) {
		for attempt := 0; attempt < 12; attempt++ {
			parent := dg3Operation(9100 + int64(attempt))
			if err := repository.InsertOperation(ctx, parent); err != nil {
				t.Fatal(err)
			}
			event := OperationEventRecord{ID: 4000 + int64(attempt), OperationID: parent.ID, EventType: "STATE"}
			start := make(chan struct{})
			var appendErr, deleteErr error
			var wait sync.WaitGroup
			wait.Add(2)
			go func() {
				defer wait.Done()
				<-start
				appendErr = repository.AppendEvent(ctx, event)
			}()
			go func() {
				defer wait.Done()
				<-start
				_, deleteErr = repository.DeleteOperationWithEvents(ctx, parent.ID)
			}()
			close(start)
			wait.Wait()
			if appendErr != nil && !errors.Is(appendErr, ErrOperationParentNotFound) {
				t.Fatalf("append/delete race append error: %v", appendErr)
			}
			if deleteErr != nil {
				t.Fatalf("append/delete race delete error: %v", deleteErr)
			}
			if orphanCount := dg3OrphanCount(t, ctx, db, dialect); orphanCount != 0 {
				t.Fatalf("append/delete race left %d orphans", orphanCount)
			}
		}
	})

	t.Run("diagnose-recheck-audit-cleanup", func(t *testing.T) {
		const (
			eventID     = int64(5001)
			operationID = int64(9999)
			sequence    = int64(1)
		)
		if err := dg3InsertFixtureOrphan(ctx, db, dialect, expectForeignKey, eventID, operationID, sequence); err != nil {
			t.Fatal(err)
		}
		diagnostics, err := repository.DiagnoseOrphanEvents(ctx, 0, 20)
		if err != nil {
			t.Fatal(err)
		}
		var diagnostic *OperationEventOrphanDiagnostic
		for index := range diagnostics {
			if diagnostics[index].EventID == eventID {
				diagnostic = &diagnostics[index]
				break
			}
		}
		if diagnostic == nil || !diagnostic.DiagnosticID.Valid {
			t.Fatalf("diagnostic not recorded: %#v", diagnostics)
		}
		if !operationEventDiagnosticMetadataMatches(*diagnostic) {
			t.Fatalf("diagnostic metadata not recorded: %#v", diagnostic)
		}
		command := OperationEventOrphanCleanupCommand{
			AuditID:                  6001,
			DiagnosticID:             diagnostic.DiagnosticID.String,
			EventID:                  eventID,
			OperationID:              operationID,
			Sequence:                 sequence,
			ExpectedIntegrityVersion: diagnostic.IntegrityVersion,
			ActorUserID:              7001,
			ActorUsername:            "dg3-test-auditor",
			Reason:                   "isolated DG3 recovery fixture",
		}
		result, err := repository.CleanupOrphanEvent(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if result != OperationEventOrphanCleanupDeleted {
			t.Fatalf("cleanup result=%q", result)
		}
		replayed, err := repository.CleanupOrphanEvent(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if replayed != OperationEventOrphanCleanupDeleted {
			t.Fatalf("cleanup replay result=%q", replayed)
		}
		collision := command
		collision.Reason = "different cleanup identity"
		if _, err := repository.CleanupOrphanEvent(ctx, collision); !errors.Is(err, ErrOperationEventMutationConflict) {
			t.Fatalf("audit identity collision error=%v", err)
		}
		if orphanCount := dg3OrphanCount(t, ctx, db, dialect); orphanCount != 0 {
			t.Fatalf("cleanup left %d orphans", orphanCount)
		}
		var auditCount int
		if err := db.GetContext(ctx, &auditCount, dg3SQL(dialect,
			`SELECT COUNT(1) FROM docker_operation_event_orphan_audit WHERE id = ? AND actorUserId = ? AND reason = ? AND expectedIntegrityVersion = ?`,
			`SELECT COUNT(1) FROM docker_operation_event_orphan_audit WHERE id = $1 AND "actorUserId" = $2 AND reason = $3 AND "expectedIntegrityVersion" = $4`),
			command.AuditID, command.ActorUserID, command.Reason, command.ExpectedIntegrityVersion); err != nil {
			t.Fatal(err)
		}
		if auditCount != 1 {
			t.Fatalf("audit count=%d, want 1", auditCount)
		}

		const (
			repairedEventID     = int64(5002)
			repairedOperationID = int64(9998)
		)
		if err := dg3InsertFixtureOrphan(ctx, db, dialect, expectForeignKey, repairedEventID, repairedOperationID, 1); err != nil {
			t.Fatal(err)
		}
		repairedDiagnostics, err := repository.DiagnoseOrphanEvents(ctx, eventID, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(repairedDiagnostics) != 1 || repairedDiagnostics[0].EventID != repairedEventID {
			t.Fatalf("repaired diagnostic=%#v", repairedDiagnostics)
		}
		repaired := repairedDiagnostics[0]
		if err := repository.InsertOperation(ctx, dg3Operation(repairedOperationID)); err != nil {
			t.Fatal(err)
		}
		repairedResult, err := repository.CleanupOrphanEvent(ctx, OperationEventOrphanCleanupCommand{
			AuditID:                  6002,
			DiagnosticID:             repaired.DiagnosticID.String,
			EventID:                  repaired.EventID,
			OperationID:              repaired.OperationID,
			Sequence:                 repaired.Sequence,
			ExpectedIntegrityVersion: repaired.IntegrityVersion,
			ActorUserID:              7001,
			ActorUsername:            "dg3-test-auditor",
			Reason:                   "parent restored before cleanup",
		})
		if err != nil {
			t.Fatal(err)
		}
		if repairedResult != OperationEventOrphanCleanupParentPresent {
			t.Fatalf("repaired cleanup result=%q", repairedResult)
		}
		var repairedStatus string
		var repairedDiagnostic sql.NullString
		if err := db.QueryRowxContext(ctx, dg3SQL(dialect,
			`SELECT integrityStatus, diagnosticId FROM docker_operation_event WHERE id = ?`,
			`SELECT "integrityStatus", "diagnosticId" FROM docker_operation_event WHERE id = $1`),
			repairedEventID).Scan(&repairedStatus, &repairedDiagnostic); err != nil {
			t.Fatal(err)
		}
		if repairedStatus != "ACTIVE" || repairedDiagnostic.Valid {
			t.Fatalf("repaired event status=%q diagnostic=%v", repairedStatus, repairedDiagnostic)
		}
	})

	if err := assertDG3SchemaAndConstraints(ctx, db, dialect, expectForeignKey); err != nil {
		t.Fatalf("post-suite constraint verification: %v", err)
	}
}

func validateDG3DatabaseDSN(dialect, dsn, expected string) error {
	switch dialect {
	case "mysql":
		parsed, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			return fmt.Errorf("parse DG3 MySQL DSN: %w", err)
		}
		if parsed.DBName != expected {
			return fmt.Errorf("DG3 MySQL tests refuse database %q; require exactly %q", parsed.DBName, expected)
		}
	case "postgres":
		parsed, err := pgx.ParseConfig(dsn)
		if err != nil {
			return fmt.Errorf("parse DG3 PostgreSQL DSN: %w", err)
		}
		if parsed.Database != expected {
			return fmt.Errorf("DG3 PostgreSQL tests refuse database %q; require exactly %q", parsed.Database, expected)
		}
	default:
		return fmt.Errorf("unsupported DG3 database dialect %q", dialect)
	}
	return nil
}

func assertDG3CurrentDatabase(ctx context.Context, db *sqlx.DB, dialect, expected string) error {
	var current string
	query := `SELECT DATABASE()`
	if dialect == "postgres" {
		query = `SELECT current_database()`
	}
	if err := db.GetContext(ctx, &current, query); err != nil {
		return fmt.Errorf("read current DG3 database: %w", err)
	}
	if current != expected {
		return fmt.Errorf("DG3 tests connected to %q; require exactly %q", current, expected)
	}
	return nil
}

func assertDG3SchemaAndConstraints(ctx context.Context, db *sqlx.DB, dialect string, expectForeignKey bool) error {
	var guardCount, auditCount, auditVersionNotNullCount, fkCount, primaryCount, uniqueCount, notNullCount, checkCount int
	if dialect == "mysql" {
		if err := db.GetContext(ctx, &guardCount, `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'docker_operation_integrity_guard'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &auditCount, `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event_orphan_audit'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &auditVersionNotNullCount, `SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event_orphan_audit' AND column_name = 'expectedIntegrityVersion' AND is_nullable = 'NO'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &fkCount, `SELECT COUNT(1) FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE() AND constraint_name = 'fk_docker_operation_event_operation'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &primaryCount, `SELECT COUNT(1) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'docker_operation_event' AND constraint_type = 'PRIMARY KEY'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &uniqueCount, `SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND index_name = 'uk_docker_operation_event_sequence' AND non_unique = 0`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &notNullCount, `SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name IN ('operationId','sequence','eventType','occurredAt','integrityStatus','integrityVersion') AND is_nullable = 'NO'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &checkCount, `SELECT COUNT(1) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'docker_operation_event' AND constraint_name IN ('chk_docker_operation_event_integrity_status','chk_docker_operation_event_diagnostic_metadata') AND constraint_type = 'CHECK'`); err != nil {
			return err
		}
	} else {
		if err := db.GetContext(ctx, &guardCount, `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'docker_operation_integrity_guard'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &auditCount, `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'docker_operation_event_orphan_audit'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &auditVersionNotNullCount, `SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'docker_operation_event_orphan_audit' AND column_name = 'expectedIntegrityVersion' AND is_nullable = 'NO'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &fkCount, `SELECT COUNT(1) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = 'docker_operation_event' AND constraint_name = 'fk_docker_operation_event_operation' AND constraint_type = 'FOREIGN KEY'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &primaryCount, `SELECT COUNT(1) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = 'docker_operation_event' AND constraint_type = 'PRIMARY KEY'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &uniqueCount, `SELECT COUNT(1) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'docker_operation_event' AND indexname = 'idx_3095061_uk_docker_operation_event_sequence'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &notNullCount, `SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'docker_operation_event' AND column_name IN ('operationId','sequence','eventType','occurredAt','integrityStatus','integrityVersion') AND is_nullable = 'NO'`); err != nil {
			return err
		}
		if err := db.GetContext(ctx, &checkCount, `SELECT COUNT(1) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = 'docker_operation_event' AND constraint_name IN ('chk_docker_operation_event_integrity_status','chk_docker_operation_event_diagnostic_metadata') AND constraint_type = 'CHECK'`); err != nil {
			return err
		}
	}
	expectedForeignKeyCount := 0
	if expectForeignKey {
		expectedForeignKeyCount = 1
	}
	if guardCount != 1 || auditCount != 1 || auditVersionNotNullCount != 1 || fkCount != expectedForeignKeyCount || primaryCount != 1 || uniqueCount < 1 || notNullCount != 6 || checkCount != 2 {
		return fmt.Errorf(
			"DG3 schema guard=%d audit=%d auditVersionNotNull=%d fk=%d primary=%d unique=%d notNull=%d check=%d, want 1/1/1/%d/1/>=1/6/2",
			guardCount, auditCount, auditVersionNotNullCount, fkCount, primaryCount, uniqueCount, notNullCount, checkCount, expectedForeignKeyCount,
		)
	}
	return nil
}

func resetDG3Rows(ctx context.Context, db *sqlx.DB, dialect string) error {
	statements := []string{
		`DELETE FROM docker_operation_event_orphan_audit`,
		`DELETE FROM docker_operation_event`,
		`DELETE FROM docker_operation`,
		`DELETE FROM docker_operation_integrity_guard`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("reset isolated DG3 rows with %q: %w", statement, err)
		}
	}
	return nil
}

func dg3Operation(id int64) OperationRecord {
	return OperationRecord{
		ID:              id,
		OperationType:   "DG3_TEST",
		TargetType:      "fixture",
		Status:          string(OperationStatusPending),
		ProgressPercent: 0,
		TimeoutAt:       sql.NullTime{Time: time.Now().UTC().Add(time.Minute), Valid: true},
	}
}

func dg3EventCount(t *testing.T, ctx context.Context, repository *OperationRepository, operationID int64) int {
	t.Helper()
	events, err := repository.ListEvents(ctx, operationID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

func dg3OrphanCount(t *testing.T, ctx context.Context, db *sqlx.DB, dialect string) int {
	t.Helper()
	var count int
	query := dg3SQL(dialect,
		`SELECT COUNT(1) FROM docker_operation_event e LEFT JOIN docker_operation o ON o.id = e.operationId WHERE o.id IS NULL`,
		`SELECT COUNT(1) FROM docker_operation_event e LEFT JOIN docker_operation o ON o.id = e."operationId" WHERE o.id IS NULL`)
	if err := db.GetContext(ctx, &count, query); err != nil {
		t.Fatal(err)
	}
	return count
}

func dg3InsertFixtureOrphan(ctx context.Context, db *sqlx.DB, dialect string, expectForeignKey bool, eventID, operationID, sequence int64) (returnErr error) {
	if expectForeignKey {
		connection, err := db.Connx(ctx)
		if err != nil {
			return err
		}
		defer connection.Close()
		if dialect == "mysql" {
			if _, err := connection.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS = 0`); err != nil {
				return err
			}
			_, insertErr := connection.ExecContext(ctx, `
INSERT INTO docker_operation_event (id, operationId, sequence, eventType, occurredAt)
VALUES (?, ?, ?, 'STATE', NOW())`, eventID, operationID, sequence)
			_, restoreErr := connection.ExecContext(context.Background(), `SET FOREIGN_KEY_CHECKS = 1`)
			if insertErr != nil {
				return insertErr
			}
			return restoreErr
		}
		if _, err := connection.ExecContext(ctx, `SET session_replication_role = replica`); err != nil {
			return err
		}
		_, insertErr := connection.ExecContext(ctx, `
INSERT INTO docker_operation_event (id, "operationId", sequence, "eventType", "occurredAt")
VALUES ($1, $2, $3, 'STATE', NOW())`, eventID, operationID, sequence)
		_, restoreErr := connection.ExecContext(context.Background(), `SET session_replication_role = origin`)
		if insertErr != nil {
			return insertErr
		}
		return restoreErr
	}
	if dialect == "mysql" {
		_, err := db.ExecContext(ctx, `
INSERT INTO docker_operation_event (id, operationId, sequence, eventType, occurredAt)
VALUES (?, ?, ?, 'STATE', NOW())`, eventID, operationID, sequence)
		return err
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO docker_operation_event (id, "operationId", sequence, "eventType", "occurredAt")
VALUES ($1, $2, $3, 'STATE', NOW())`, eventID, operationID, sequence)
	return err
}

func dg3SQL(dialect, mysqlSQL, postgresSQL string) string {
	if dialect == "postgres" {
		return postgresSQL
	}
	return mysqlSQL
}
