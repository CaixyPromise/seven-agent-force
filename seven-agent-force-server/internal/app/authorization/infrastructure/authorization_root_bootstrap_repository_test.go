package infrastructure

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestBootstrapAuthorizationRootAppliesCustomIdentityForUserEmptyDatabase(t *testing.T) {
	repo, mock, closeDB := newAuthorizationRootBootstrapRepository(t)
	defer closeDB()
	now := time.Unix(1713830400, 0).UTC()
	expectLockedAuthorizationRoot(mock, "SUPER_ADMIN")
	mock.ExpectQuery(`(?s)SELECT bootstrapKey.*FOR UPDATE`).
		WithArgs(domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"bootstrapKey"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM sys_user WHERE isDeleted = 0`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM sys_role WHERE code = \?.*`).
		WithArgs("PLATFORM_OWNER", int64(1900300001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?s)UPDATE sys_role.*SET code = \?, name = \?.*systemKey = \?`).
		WithArgs("PLATFORM_OWNER", "平台所有者", now, int64(1900300001), domain.AuthorizationRootSystemKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO sys_security_bootstrap`).
		WithArgs(domain.AuthorizationRootSystemKey, int64(1900300001), "PLATFORM_OWNER", now, now, now).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := repo.BootstrapAuthorizationRoot(context.Background(), "PLATFORM_OWNER", "平台所有者", now)
	if err != nil {
		t.Fatalf("bootstrap authorization root: %v", err)
	}
	if result.AlreadyInitialized || result.Role.Code != "PLATFORM_OWNER" || result.Role.Name != "平台所有者" {
		t.Fatalf("unexpected bootstrap result: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestBootstrapAuthorizationRootKeepsPersistedIdentityAfterInitialization(t *testing.T) {
	repo, mock, closeDB := newAuthorizationRootBootstrapRepository(t)
	defer closeDB()
	now := time.Unix(1713830400, 0).UTC()
	expectLockedAuthorizationRoot(mock, "PERSISTED_OWNER")
	mock.ExpectQuery(`(?s)SELECT bootstrapKey.*FOR UPDATE`).
		WithArgs(domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"bootstrapKey"}).AddRow(domain.AuthorizationRootSystemKey))

	result, err := repo.BootstrapAuthorizationRoot(context.Background(), "CHANGED_CONFIG", "Changed", now)
	if err != nil {
		t.Fatalf("read initialized authorization root: %v", err)
	}
	if !result.AlreadyInitialized || result.Role.Code != "PERSISTED_OWNER" {
		t.Fatalf("initialized database must retain persisted role identity: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestBootstrapAuthorizationRootRejectsCodeConflictAtomically(t *testing.T) {
	repo, mock, closeDB := newAuthorizationRootBootstrapRepository(t)
	defer closeDB()
	now := time.Unix(1713830400, 0).UTC()
	expectLockedAuthorizationRoot(mock, "SUPER_ADMIN")
	mock.ExpectQuery(`(?s)SELECT bootstrapKey.*FOR UPDATE`).
		WithArgs(domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"bootstrapKey"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM sys_user WHERE isDeleted = 0`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM sys_role WHERE code = \?.*`).
		WithArgs("CONFLICTING_ROLE", int64(1900300001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	if _, err := repo.BootstrapAuthorizationRoot(context.Background(), "CONFLICTING_ROLE", "Owner", now); err == nil {
		t.Fatal("expected conflicting root code to fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func newAuthorizationRootBootstrapRepository(t *testing.T) (*Repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return &Repository{db: sqlx.NewDb(rawDB, "mysql")}, mock, func() { _ = rawDB.Close() }
}

func expectLockedAuthorizationRoot(mock sqlmock.Sqlmock, code string) {
	mock.ExpectQuery(`(?s)SELECT id AS role_id.*FROM sys_role.*systemKey = \?.*FOR UPDATE`).
		WithArgs(domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"role_id", "name", "code", "system_key", "type", "status", "data_scope", "sort_order", "remark", "create_time", "update_time"}).
			AddRow(int64(1900300001), "超级管理员", code, domain.AuthorizationRootSystemKey, 1, 0, 1, 0, "", nil, nil))
}
