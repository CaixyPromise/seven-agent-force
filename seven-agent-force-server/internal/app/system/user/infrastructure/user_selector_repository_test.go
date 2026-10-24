package infrastructure

import (
	"regexp"
	"testing"

	userdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestListUserOptionsUsesMinimalProjectionAndDataScope(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	query := regexp.QuoteMeta(`
SELECT u.id, u.userAccount, u.nickName, COALESCE(u.userAvatar, '') AS userAvatar, u.status
FROM sys_user u WHERE u.isDeleted = 0 AND u.status = 0 AND (u.userAccount LIKE ? OR u.nickName LIKE ?) AND EXISTS (SELECT 1 FROM sys_user_dept selected_dept WHERE selected_dept.userId = u.id AND selected_dept.deptId = ?) AND EXISTS (SELECT 1 FROM sys_user_dept scoped WHERE scoped.userId = u.id AND scoped.deptId IN (?))
ORDER BY u.nickName ASC, u.id ASC
LIMIT ?`)
	mock.ExpectQuery(query).
		WithArgs("%ali%", "%ali%", int64(7), int64(7), 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "userAccount", "nickName", "userAvatar", "status"}).
			AddRow(int64(2065424359060983808), "alice", "Alice", "/avatar.png", 0))

	records, err := repo.ListUserOptions(t.Context(), userdomain.UserSelectorQuery{
		Keyword: "ali",
		Limit:   20,
		DeptID:  7,
		Scope:   userdomain.DataScopeFilter{Enabled: true, DeptIDs: []int64{7}},
	})
	if err != nil {
		t.Fatalf("list user options: %v", err)
	}
	if len(records) != 1 || records[0].ID != 2065424359060983808 || records[0].AccountName != "alice" {
		t.Fatalf("unexpected selector records: %#v", records)
	}
	assertSQLExpectations(t, mock)
}

func TestFindVisibleUserOptionByIDReturnsNilOutsideScope(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	query := regexp.QuoteMeta(`
SELECT u.id, u.userAccount, u.nickName, COALESCE(u.userAvatar, '') AS userAvatar, u.status
FROM sys_user u
WHERE u.id = ? AND u.isDeleted = 0 AND 1 = 0
LIMIT 1`)
	mock.ExpectQuery(query).
		WithArgs(int64(2065424359060983808)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "userAccount", "nickName", "userAvatar", "status"}))

	record, err := repo.FindVisibleUserOptionByID(t.Context(), 2065424359060983808, userdomain.DataScopeFilter{Enabled: true, None: true})
	if err != nil {
		t.Fatalf("find visible user option: %v", err)
	}
	if record != nil {
		t.Fatalf("expected hidden user to be absent, got %#v", record)
	}
	assertSQLExpectations(t, mock)
}

func TestListUserOptionsUsesPostgresRendererForCamelCaseProjectionAndScopes(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "postgres"), postgres: true}

	mock.ExpectQuery(`(?s)SELECT u.id, u."userAccount", u."nickName", COALESCE\(u."userAvatar", ''\) AS "userAvatar".*u."isDeleted" = FALSE.*selected_dept."userId" = u.id AND selected_dept."deptId" = \$1.*scoped."userId" = u.id AND scoped."deptId" IN \(\$2\).*ORDER BY u."nickName".*LIMIT \$3`).
		WithArgs(int64(7), int64(7), 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "userAccount", "nickName", "userAvatar", "status"}).
			AddRow(int64(2065424359060983808), "alice", "Alice", "/avatar.png", 0))

	records, err := repo.ListUserOptions(t.Context(), userdomain.UserSelectorQuery{
		Limit:  20,
		DeptID: 7,
		Scope:  userdomain.DataScopeFilter{Enabled: true, DeptIDs: []int64{7}},
	})
	if err != nil {
		t.Fatalf("list PostgreSQL user options: %v", err)
	}
	if len(records) != 1 || records[0].AccountName != "alice" {
		t.Fatalf("unexpected PostgreSQL selector records: %#v", records)
	}
	assertSQLExpectations(t, mock)
}
