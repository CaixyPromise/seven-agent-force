package infrastructure

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	userdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestCompareAndSetManagedUserStatusUsesDatabaseRevisionPrecondition(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	query := regexp.QuoteMeta(`UPDATE sys_user
SET status = ?, unsealTime = ?, statusVersion = statusVersion + 1, statusCommandHash = ?, updateTime = NOW()
WHERE id = ? AND status = ? AND statusVersion = ? AND isDeleted = 0`)
	mock.ExpectExec(query).
		WithArgs(1, nil, "managed-command-hash", int64(2001), 0, uint64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	updated, err := repo.CompareAndSetManagedUserStatus(t.Context(), 2001, 0, 7, 1, nil, "managed-command-hash")
	if err != nil || !updated {
		t.Fatalf("matching CAS updated=%v err=%v", updated, err)
	}

	mock.ExpectExec(query).
		WithArgs(1, nil, "managed-command-hash", int64(2001), 0, uint64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	updated, err = repo.CompareAndSetManagedUserStatus(t.Context(), 2001, 0, 7, 1, nil, "managed-command-hash")
	if err != nil || updated {
		t.Fatalf("stale CAS updated=%v err=%v", updated, err)
	}
	assertSQLExpectations(t, mock)
}

func TestUpdateAdminUserStatusCycleAdvancesVersionForEachValueChange(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	query := regexp.QuoteMeta(`UPDATE sys_user SET nickName = ?, userEmail = ?, userPhone = ?, updaterId = ?, updateTime = NOW(), status = ?, statusVersion = statusVersion + 1, statusCommandHash = NULL WHERE id = ? AND isDeleted = 0`)
	for _, status := range []int{1, 0, 1} {
		mock.ExpectExec(query).
			WithArgs("Alice", "alice@example.com", nil, nil, status, int64(2001)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		if err := repo.UpdateAdminUser(t.Context(), userdomain.AdminUserUpdateRecord{ID: 2001, NickName: "Alice", Email: "alice@example.com", Status: &status}); err != nil {
			t.Fatalf("update admin user status=%d: %v", status, err)
		}
	}
	assertSQLExpectations(t, mock)
}

func TestUpdateLockStateClearsManagedStatusCommandHash(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	query := regexp.QuoteMeta(`UPDATE sys_user SET status = ?, unsealTime = ?, statusVersion = statusVersion + 1, statusCommandHash = NULL, updateTime = NOW() WHERE id = ? AND isDeleted = 0`)
	mock.ExpectExec(query).
		WithArgs(1, nil, int64(2001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateLockState(t.Context(), 2001, 1, nil); err != nil {
		t.Fatalf("update lock state: %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestFindSubjectByEmailExcludesDeletedUsers(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE LOWER(userEmail) = ? AND status = 0 AND isDeleted = 0 LIMIT 2`)).
		WithArgs("deleted@example.com").
		WillReturnRows(newSubjectRows())

	record, err := repo.FindSubjectByEmail(t.Context(), "Deleted@Example.COM")
	if err != nil {
		t.Fatalf("find subject by email: %v", err)
	}
	if record != nil {
		t.Fatalf("expected deleted user to be excluded from bindable email lookup, got %#v", record)
	}
	assertSQLExpectations(t, mock)
}

func TestFindSubjectByEmailRejectsDuplicateActiveUsers(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE LOWER(userEmail) = ? AND status = 0 AND isDeleted = 0 LIMIT 2`)).
		WithArgs("dup@example.com").
		WillReturnRows(newSubjectRows().
			AddRow(int64(1001), "alice", "Alice", "dup@example.com", "", "", "", 0, nil).
			AddRow(int64(1002), "bob", "Bob", "dup@example.com", "", "", "", 0, nil))

	record, err := repo.FindSubjectByEmail(t.Context(), "dup@example.com")
	if record != nil {
		t.Fatalf("expected nil subject for duplicate email, got %#v", record)
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Message() != "邮箱匹配到多个用户，禁止自动绑定" {
		t.Fatalf("expected duplicate email operation error, got %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestListActiveUserIDsByRoleIDPageQuotesPostgresCamelCaseColumns(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "postgres")}

	mock.ExpectQuery(`(?s)JOIN sys_user_role ur ON ur."userId" = u.id AND ur."isDeleted" = FALSE.*WHERE ur."roleId" = \$1 AND u."isDeleted" = FALSE AND u.status = 0 AND u.id > \$2.*LIMIT \$3`).
		WithArgs(int64(7), int64(100), 20).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)).AddRow(int64(102)))

	ids, err := repo.ListActiveUserIDsByRoleIDPage(t.Context(), 7, 100, 20)
	if err != nil {
		t.Fatalf("ListActiveUserIDsByRoleIDPage() error=%v", err)
	}
	if len(ids) != 2 || ids[0] != 101 || ids[1] != 102 {
		t.Fatalf("role page ids=%v", ids)
	}
	assertSQLExpectations(t, mock)
}

func TestListUserIDsByPostIDPageIsBoundedAndQuotesPostgresCamelCaseColumns(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "postgres"), postgres: true}

	mock.ExpectQuery(`(?s)SELECT "userId".*FROM sys_user_position.*WHERE "postId" = \$1 AND "userId" > \$2 AND "isDeleted" = FALSE.*ORDER BY "userId" ASC.*LIMIT \$3`).
		WithArgs(int64(9), int64(100), 200).
		WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(int64(101)).AddRow(int64(102)))

	ids, err := repo.ListUserIDsByPostIDPage(t.Context(), 9, 100, 500)
	if err != nil {
		t.Fatalf("ListUserIDsByPostIDPage() error=%v", err)
	}
	if len(ids) != 2 || ids[0] != 101 || ids[1] != 102 {
		t.Fatalf("post page ids=%v", ids)
	}
	assertSQLExpectations(t, mock)
}

func TestUserPostgresRendererPreservesHostileValuesAndComments(t *testing.T) {
	query := `SELECT userId FROM sys_user_org
WHERE userProfile = '{"userId":1,"isDeleted":0}'
  AND nickName = 'userId isDeleted=0'
  -- userId isDeleted=0
  /* isPrimary=1 */`
	got := userPostgresRenderer.RenderPostgres(query)
	for _, preserved := range []string{
		`'{"userId":1,"isDeleted":0}'`,
		`'userId isDeleted=0'`,
		`-- userId isDeleted=0`,
		`/* isPrimary=1 */`,
	} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("SQL data/comment was rewritten; missing %q in %s", preserved, got)
		}
	}
}

func TestQueryAdminUsersResolvesDimensionFiltersWithoutWideJoin(t *testing.T) {
	rawDB, mock, repo := newSystemUserRepositoryMock(t)
	defer rawDB.Close()

	mock.ExpectQuery(`(?s)SELECT userId\s+FROM sys_user_org\s+WHERE orgId IN \(\?\) AND isDeleted = 0\s+ORDER BY userId\s+LIMIT \?`).
		WithArgs(int64(10), 501).
		WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(int64(1001)).AddRow(int64(1002)))
	mock.ExpectQuery(`(?s)SELECT userId\s+FROM sys_user_dept\s+WHERE deptId IN \(\?\)\s+ORDER BY userId\s+LIMIT \?`).
		WithArgs(int64(20), 501).
		WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(int64(1001)))
	mock.ExpectQuery(`(?s)SELECT userId\s+FROM sys_user_position\s+WHERE postId IN \(\?\) AND isDeleted = 0\s+ORDER BY userId\s+LIMIT \?`).
		WithArgs(int64(30), 501).
		WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(int64(1001)))
	mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM sys_user u\s+WHERE u.isDeleted = 0 AND u.id IN \(\?\)`).
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT u.id, u.userAccount.*FROM sys_user u\s+WHERE u.isDeleted = 0 AND u.id IN \(\?\)\s+ORDER BY u.createTime DESC, u.id DESC LIMIT \? OFFSET \?`).
		WithArgs(int64(1001), int64(20), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "userAccount", "nickName", "userAvatar", "userEmail", "userPhone",
			"userGender", "userProfile", "status", "statusVersion", "statusCommandHash", "createTime", "updateTime",
		}))

	_, total, err := repo.QueryAdminUsers(context.Background(), userdomain.AdminUserQuery{
		Current: 1, Size: 20, OrgID: 10, DeptID: 20, PostID: 30,
	})
	if err != nil || total != 1 {
		t.Fatalf("QueryAdminUsers() total=%d err=%v", total, err)
	}
	assertSQLExpectations(t, mock)
}

func newSystemUserRepositoryMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Repository) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return rawDB, mock, &Repository{db: sqlx.NewDb(rawDB, "sqlmock")}
}

func newSubjectRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"user_id",
		"account_name",
		"nick_name",
		"email",
		"phone",
		"avatar",
		"profile",
		"status",
		"unseal_at",
	})
}

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
