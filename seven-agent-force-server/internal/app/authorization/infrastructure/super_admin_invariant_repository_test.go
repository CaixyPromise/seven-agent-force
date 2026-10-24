package infrastructure

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestLockSuperAdminInvariantLocksRoleAndReadsDirectActiveUsers(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}

	mock.ExpectQuery(`(?s)SELECT id.*FROM sys_role.*systemKey = \?.*FOR UPDATE`).
		WithArgs(domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1900300001)))
	mock.ExpectQuery(`(?s)SELECT su\.id.*sys_user_role.*FOR UPDATE`).
		WithArgs(int64(1900300001), domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1001)).AddRow(int64(1002)))

	snapshot, err := repo.LockSuperAdminInvariant(context.Background(), 1001)
	if err != nil {
		t.Fatalf("lock invariant: %v", err)
	}
	if snapshot.ActiveUserCount != 2 || !snapshot.TargetUserActive {
		t.Fatalf("unexpected invariant snapshot: %#v", snapshot)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestLockSuperAdminInvariantReturnsEmptySnapshotWhenRoleIsMissing(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}

	mock.ExpectQuery(`(?s)SELECT id.*FROM sys_role.*systemKey = \?.*FOR UPDATE`).
		WithArgs(domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	snapshot, err := repo.LockSuperAdminInvariant(context.Background(), 1001)
	if err != nil {
		t.Fatalf("lock missing invariant: %v", err)
	}
	if snapshot.ActiveUserCount != 0 || snapshot.TargetUserActive {
		t.Fatalf("unexpected missing-role snapshot: %#v", snapshot)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
