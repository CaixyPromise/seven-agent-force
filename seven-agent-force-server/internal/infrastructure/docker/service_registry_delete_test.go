package docker

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestDockerRegistryDeleteHardDeletesLocalConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &RegistryRepository{db: sqlx.NewDb(db, "sqlmock")}

	mock.ExpectExec(regexp.QuoteMeta(`
DELETE FROM docker_remote_registry
WHERE id = ?`)).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Delete(context.Background(), 42); err != nil {
		t.Fatalf("delete registry: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
