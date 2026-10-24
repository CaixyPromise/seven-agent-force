package store

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type Provider interface {
	Driver() string
	Dialect() string
	DB() *sql.DB
	SQLX() *sqlx.DB
	Transactor() Transactor
	Configured() bool
	Close() error
}
