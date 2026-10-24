package infrastructure

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestConfigScopeRepositoryListsAndReplacesGrants(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, roleId, groupCode, configKey, canRead, canWrite, canDelete, createdBy, createTime, updatedBy, updateTime, isDeleted
FROM sys_role_config_scope
WHERE roleId IN (?, ?) AND isDeleted = 0
ORDER BY roleId ASC, groupCode ASC, configKey ASC
LIMIT ?`)).
		WithArgs(int64(1001), int64(1002), 1001).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "roleId", "groupCode", "configKey", "canRead", "canWrite", "canDelete",
			"createdBy", "createTime", "updatedBy", "updateTime", "isDeleted",
		}).
			AddRow(int64(1), int64(1001), "title", "", 1, 0, 0, int64(9001), now, int64(9001), now, 0).
			AddRow(int64(2), int64(1002), "docker", "docker.operation.retentionDays", 1, 1, 0, int64(9001), now, int64(9001), now, 0))

	grants, err := repo.ListConfigScopeGrantsByRoleIDs(context.Background(), []int64{1001, 1002, 1001})
	if err != nil {
		t.Fatalf("list config scope grants: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants, got %#v", grants)
	}
	if grants[0].GroupCode != "title" || grants[0].ConfigKey != "" || grants[0].CanRead != 1 || grants[0].CanWrite != 0 {
		t.Fatalf("unexpected group grant: %#v", grants[0])
	}
	if grants[1].GroupCode != "docker" || grants[1].ConfigKey != "docker.operation.retentionDays" || grants[1].CanWrite != 1 {
		t.Fatalf("unexpected key grant: %#v", grants[1])
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM sys_role_config_scope WHERE roleId = ?`)).
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO sys_role_config_scope (
	id, roleId, groupCode, configKey, canRead, canWrite, canDelete, createdBy, createTime, updatedBy
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)).
		WithArgs(
			int64(5001), int64(1001), "title", "", 1, 0, 0, int64(9001), sqlmock.AnyArg(), int64(9001),
			int64(5002), int64(1001), "docker", "docker.operation.retentionDays", 1, 1, 0, int64(9001), sqlmock.AnyArg(), int64(9001),
		).
		WillReturnResult(sqlmock.NewResult(0, 2))

	nextID := int64(5000)
	err = repo.ReplaceRoleConfigScopes(context.Background(), 1001, []domain.ConfigScopeGrant{
		{GroupCode: " title ", CanRead: 1},
		{GroupCode: "title", CanRead: 1, CanWrite: 1},
		{GroupCode: "", CanRead: 1},
		{GroupCode: "docker", ConfigKey: " docker.operation.retentionDays ", CanRead: 1, CanWrite: 2},
	}, 9001, func() int64 {
		nextID++
		return nextID
	})
	if err != nil {
		t.Fatalf("replace config scope grants: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
