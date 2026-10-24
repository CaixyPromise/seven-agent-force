package bootstrap

import (
	"context"
	"database/sql"
	"sync"

	"github.com/pressly/goose/v3"
)

type Runner interface {
	Up(ctx context.Context, db *sql.DB, dialect, dir, table string) (int64, error)
	UpTo(ctx context.Context, db *sql.DB, dialect, dir, table string, version int64) (int64, error)
	Version(ctx context.Context, db *sql.DB, dialect, table string) (int64, error)
}

type GooseRunner struct {
	mu sync.Mutex
}

func NewGooseRunner() *GooseRunner {
	return &GooseRunner{}
}

func (r *GooseRunner) Up(ctx context.Context, db *sql.DB, dialect, dir, table string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	goose.SetTableName(table)
	if err := goose.SetDialect(dialect); err != nil {
		return 0, err
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return 0, err
	}
	return goose.GetDBVersionContext(ctx, db)
}

func (r *GooseRunner) UpTo(ctx context.Context, db *sql.DB, dialect, dir, table string, version int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	goose.SetTableName(table)
	if err := goose.SetDialect(dialect); err != nil {
		return 0, err
	}
	if err := goose.UpToContext(ctx, db, dir, version); err != nil {
		return 0, err
	}
	return goose.GetDBVersionContext(ctx, db)
}

func (r *GooseRunner) Version(ctx context.Context, db *sql.DB, dialect, table string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	goose.SetTableName(table)
	if err := goose.SetDialect(dialect); err != nil {
		return 0, err
	}
	return goose.GetDBVersionContext(ctx, db)
}
