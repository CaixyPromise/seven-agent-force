package mysql

import (
	"database/sql"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type Provider struct {
	cfg        config.MySQLConfig
	db         *sql.DB
	sqlxDB     *sqlx.DB
	transactor store.Transactor
}

func NewProvider(cfg config.MySQLConfig, log *zap.Logger) (*Provider, error) {
	db, err := openDB(cfg, log)
	if err != nil {
		return nil, err
	}
	sqlxDB := store.NewSQLXDB(db, driverName)
	return &Provider{
		cfg:        cfg,
		db:         db,
		sqlxDB:     sqlxDB,
		transactor: store.NewSQLXTransactor(sqlxDB),
	}, nil
}

func (p *Provider) Driver() string {
	return "mysql"
}

func (p *Provider) Dialect() string {
	return "mysql"
}

func (p *Provider) DB() *sql.DB {
	return p.db
}

func (p *Provider) SQLX() *sqlx.DB {
	return p.sqlxDB
}

func (p *Provider) Transactor() store.Transactor {
	return p.transactor
}

func (p *Provider) Configured() bool {
	return p.cfg.Configured()
}

func (p *Provider) Close() error {
	return closeDB(p.db)
}
