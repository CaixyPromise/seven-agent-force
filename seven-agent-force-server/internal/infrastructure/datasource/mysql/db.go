package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

const driverName = "mysql"

func openDB(cfg config.MySQLConfig, log *zap.Logger) (*sql.DB, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.DSN == "" {
		return nil, errors.New("datasource.mysql.dsn must not be empty when mysql datasource is enabled")
	}

	db, err := sql.Open(driverName, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql datasource: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql datasource: %w", err)
	}

	if log != nil {
		log.Info("mysql datasource connected",
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
