package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

func Open(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	parsed, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	parsed.MaxConns = cfg.MaxConns
	parsed.MinConns = cfg.MinConns
	if cfg.ConnectTimeout > 0 {
		parsed.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	}
	pool, err := pgxpool.NewWithConfig(ctx, parsed)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, cfg config.DatabaseConfig) error {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return fmt.Errorf("open postgres for migration: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres for migration: %w", err)
	}
	dir, err := filepath.Abs(cfg.MigrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations directory: %w", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
