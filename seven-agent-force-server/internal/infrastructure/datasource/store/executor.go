package store

import (
	"context"
	"database/sql"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Executor returns the current query executor for a repository method.
//
// The intended usage pattern is:
//
//	rows, err := store.Executor(ctx, r.db).QueryContext(ctx, sql, args...)
//
// Repositories always receive a base DBTX at construction time and never branch
// on transaction state themselves. If ctx carries an active transaction,
// Executor returns that *sql.Tx; otherwise it returns the base db handle.
func Executor(ctx context.Context, db DBTX) DBTX {
	if tx := TxFromContext(ctx); tx != nil {
		return tx
	}
	return db
}
