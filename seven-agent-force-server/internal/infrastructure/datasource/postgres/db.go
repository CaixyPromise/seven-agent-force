package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

const driverName = "pgx"

func openDB(cfg config.PostgresConfig, log *zap.Logger) (*sql.DB, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.DSN == "" {
		return nil, errors.New("datasource.postgres.dsn must not be empty when postgres datasource is enabled")
	}

	db, err := sql.Open(driverName, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres datasource: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres datasource: %w", err)
	}

	if log != nil {
		log.Info("postgres datasource connected",
			zap.Int("max_open_conns", cfg.MaxOpenConns),
			zap.Int("max_idle_conns", cfg.MaxIdleConns),
			zap.Duration("conn_max_lifetime", cfg.ConnMaxLifetime),
			zap.Duration("conn_max_idle_time", cfg.ConnMaxIdleTime),
		)
	}

	return db, nil
}

func closeDB(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
