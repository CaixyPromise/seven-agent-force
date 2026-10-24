package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

type fakeDBTX struct{}

func (fakeDBTX) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return nil, nil
}

func (fakeDBTX) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, nil
}

func (fakeDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, nil
}

func (fakeDBTX) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

func TestExecutorFallsBackToDB(t *testing.T) {
	db := fakeDBTX{}
	if Executor(context.Background(), db) != db {
		t.Fatal("expected executor to return base db when no transaction exists")
	}
}

func TestTransactorEnabledWithoutDB(t *testing.T) {
	transactor := NewSQLTransactor(nil)
	if transactor.Enabled() {
		t.Fatal("expected nil-db transactor to be disabled")
	}
	if err := transactor.WithinTransaction(context.Background(), func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected transaction execution to fail without configured db")
	}
}

func TestSQLXExecutorFallsBackToDB(t *testing.T) {
	db := sqlx.NewDb(&sql.DB{}, "mysql")
	if SQLXExecutor(context.Background(), db) != db {
		t.Fatal("expected sqlx executor to return base db when no transaction exists")
	}
}

func TestSQLXTransactorEnabledWithoutDB(t *testing.T) {
	transactor := NewSQLXTransactor(nil)
	if transactor.Enabled() {
		t.Fatal("expected nil-db sqlx transactor to be disabled")
	}
	if err := transactor.WithinTransaction(context.Background(), func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected sqlx transaction execution to fail without configured db")
	}
}

func TestSQLXTransactorRunsAfterCommitOnlyAfterOutermostCommit(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()
	transactor := NewSQLXTransactor(sqlx.NewDb(rawDB, "sqlmock"))
	called := false
	if err := transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		if !transactor.RegisterAfterCommit(txCtx, func() { called = true }) {
			t.Fatal("expected store-managed transaction to accept after-commit callback")
		}
		if called {
			t.Fatal("after-commit callback ran before transaction commit")
		}
		return transactor.WithinTransaction(txCtx, func(nestedCtx context.Context) error {
			if !transactor.RegisterAfterCommit(nestedCtx, func() { called = true }) {
				t.Fatal("expected nested callback registration")
			}
			return nil
		})
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	if !called {
		t.Fatal("after-commit callback did not run after successful outer commit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSQLXTransactorDiscardsAfterCommitOnRollback(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()
	transactor := NewSQLXTransactor(sqlx.NewDb(rawDB, "sqlmock"))
	called := false
	err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		if !transactor.RegisterAfterCommit(txCtx, func() { called = true }) {
			t.Fatal("expected store-managed transaction to accept after-commit callback")
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if called {
		t.Fatal("after-commit callback survived rollback")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSQLXTransactorRunsAfterRollbackOnlyOnRollback(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()
	transactor := NewSQLXTransactor(sqlx.NewDb(rawDB, "sqlmock"))
	rolledBack := false
	err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		if !transactor.RegisterAfterRollback(txCtx, func() { rolledBack = true }) {
			t.Fatal("expected store-managed transaction to accept rollback callback")
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if !rolledBack {
		t.Fatal("after-rollback callback did not run after failed transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSQLXTransactorWithinReadOnlySnapshotUsesRepeatableRead(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()
	transactor := NewSQLXTransactor(sqlx.NewDb(rawDB, "sqlmock"))
	if err := transactor.WithinReadOnlySnapshot(context.Background(), func(ctx context.Context) error {
		if SQLXFromContext(ctx) == nil {
			t.Fatal("expected snapshot executor in context")
		}
		return nil
	}); err != nil {
		t.Fatalf("read-only snapshot: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSQLXTransactorWithinReadOnlySnapshotRejectsOrdinaryTransaction(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()
	transactor := NewSQLXTransactor(sqlx.NewDb(rawDB, "sqlmock"))
	called := false
	err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return transactor.WithinReadOnlySnapshot(txCtx, func(context.Context) error {
			called = true
			return nil
		})
	})
	if err == nil {
		t.Fatal("expected ordinary transaction to be rejected as a read-only snapshot")
	}
	if called {
		t.Fatal("snapshot callback ran inside an ordinary transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSQLXTransactorWithinReadOnlySnapshotAllowsNestedSnapshot(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()
	transactor := NewSQLXTransactor(sqlx.NewDb(rawDB, "sqlmock"))
	if err := transactor.WithinReadOnlySnapshot(context.Background(), func(snapshotCtx context.Context) error {
		return transactor.WithinReadOnlySnapshot(snapshotCtx, func(nestedCtx context.Context) error {
			if SQLXFromContext(nestedCtx) != SQLXFromContext(snapshotCtx) {
				t.Fatal("nested snapshot did not reuse the marked snapshot transaction")
			}
			return nil
		})
	}); err != nil {
		t.Fatalf("nested read-only snapshot: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSQLXTransactorConsistentTransactionAllowsNestedSnapshot(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()
	transactor := NewSQLXTransactor(sqlx.NewDb(rawDB, "sqlmock"))
	if err := transactor.WithinConsistentTransaction(context.Background(), func(txCtx context.Context) error {
		return transactor.WithinReadOnlySnapshot(txCtx, func(snapshotCtx context.Context) error {
			if SQLXFromContext(snapshotCtx) != SQLXFromContext(txCtx) {
				t.Fatal("snapshot did not reuse the marked consistent transaction")
			}
			return nil
		})
	}); err != nil {
		t.Fatalf("consistent transaction with nested snapshot: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
