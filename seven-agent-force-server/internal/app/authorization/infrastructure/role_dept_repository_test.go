package infrastructure

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestRoleDeptRepositoryListsAndReplacesRoleDepartments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT deptId FROM sys_role_dept WHERE roleId = ? ORDER BY deptId ASC`)).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"deptId"}).
			AddRow(int64(20)).
			AddRow(int64(20)).
			AddRow(int64(30)))

	deptIDs, err := repo.ListDeptIDsByRoleID(context.Background(), 10)
	if err != nil {
		t.Fatalf("list role dept ids: %v", err)
	}
	if len(deptIDs) != 2 || deptIDs[0] != 20 || deptIDs[1] != 30 {
		t.Fatalf("expected unique dept ids [20 30], got %#v", deptIDs)
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM sys_role_dept WHERE roleId = ?`)).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys_role_dept (id, roleId, deptId) VALUES (?, ?, ?), (?, ?, ?)`)).
		WithArgs(int64(1001), int64(10), int64(30), int64(1002), int64(10), int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	nextID := int64(1000)
	if err := repo.ReplaceRoleDepts(context.Background(), 10, []int64{30, 0, 20, 30}, 9001, func() int64 {
		nextID++
		return nextID
	}); err != nil {
		t.Fatalf("replace role depts: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(DISTINCT id) FROM sys_dept WHERE id IN (?, ?) AND isDeleted = 0 AND status = 0`)).
		WithArgs(int64(20), int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountDeptsByIDs(context.Background(), []int64{20, 30, 20})
	if err != nil {
		t.Fatalf("count depts by ids: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected dept count 2, got %d", count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
