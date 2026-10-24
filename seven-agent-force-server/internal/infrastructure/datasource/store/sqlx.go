package store

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type sqlxContextKey struct{}

// SQLX is the recommended repository-facing interface for sqlx-based query
// repositories. It hides whether the current executor is a *sqlx.DB or a
// *sqlx.Tx while preserving sqlx conveniences such as Rebind and NamedExec.
type SQLX interface {
	sqlx.ExtContext
	Rebind(query string) string
	QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error)
	QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row
}

func NewSQLXDB(db *sql.DB, driver string) *sqlx.DB {
	if db == nil {
		return nil
	}
	return sqlx.NewDb(db, driver)
}

// SQLXExecutor returns the current sqlx executor for a repository method.
//
// The intended usage pattern is:
//
//	exec := store.SQLXExecutor(ctx, r.db)
//	sqlText := exec.Rebind(baseSQL)
//	if err := sqlx.SelectContext(ctx, exec, &items, sqlText, args...); err != nil { ... }
//
// Query repositories should prefer this helper instead of branching on whether
// ctx currently carries a transaction.
func SQLXExecutor(ctx context.Context, db SQLX) SQLX {
	if tx := SQLXFromContext(ctx); tx != nil {
		return tx
	}
	return db
}

func SQLXFromContext(ctx context.Context) SQLX {
	if ctx == nil {
		return nil
	}
	exec, _ := ctx.Value(sqlxContextKey{}).(SQLX)
	return exec
}
