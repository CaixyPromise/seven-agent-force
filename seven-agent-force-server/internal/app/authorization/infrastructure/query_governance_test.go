package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestRoleGrantBatchLockAndAffectedUserPageAreBounded(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}
	now := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)FROM sys_menu\s+WHERE id IN \(\?, \?\) AND isDeleted = 0\s+ORDER BY id ASC\s+FOR UPDATE`).
		WithArgs(int64(9), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{
			"menu_id", "parent_id", "sort_order", "name", "path", "component", "type", "permission",
			"feature_code", "icon", "status", "visible", "is_frame", "is_cache", "remark", "create_time", "update_time",
		}).
			AddRow(int64(9), int64(0), 1, "Users", "/users", "", "M", "", "", "", 0, 1, 0, 0, "", now, now).
			AddRow(int64(11), int64(0), 2, "Roles", "/roles", "", "M", "", "", "", 0, 1, 0, 0, "", now, now))
	menus, err := repo.LockMenuGrants(context.Background(), []int64{9, 11})
	if err != nil || len(menus) != 2 {
		t.Fatalf("LockMenuGrants() menus=%#v err=%v", menus, err)
	}
	mock.ExpectExec(`UPDATE sys_menu SET updateTime = NOW\(\) WHERE id IN \(\?, \?\) AND isDeleted = 0`).
		WithArgs(int64(9), int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.TouchMenuGrantGuards(context.Background(), []int64{9, 11}); err != nil {
		t.Fatalf("TouchMenuGrantGuards(): %v", err)
	}

	mock.ExpectQuery(`(?s)FROM sys_role\s+WHERE id IN \(\?, \?, \?\) AND isDeleted = 0\s+ORDER BY id ASC\s+FOR UPDATE`).
		WithArgs(int64(10), int64(20), int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{
			"role_id", "name", "code", "system_key", "type", "status", "data_scope", "grant_revision", "sort_order", "remark", "create_time", "update_time",
		}).
			AddRow(int64(10), "Role 10", "ROLE_10", "", 0, 0, 5, int64(1), 10, "", now, now).
			AddRow(int64(20), "Role 20", "ROLE_20", "", 0, 0, 5, int64(2), 20, "", now, now).
			AddRow(int64(30), "Role 30", "ROLE_30", "", 0, 0, 5, int64(3), 30, "", now, now))

	roles, err := repo.LockRoleGrants(context.Background(), []int64{10, 20, 30})
	if err != nil || len(roles) != 3 {
		t.Fatalf("LockRoleGrants() roles=%#v err=%v", roles, err)
	}

	mock.ExpectQuery(`(?s)FROM sys_user_role.*UNION.*FROM sys_user_position sup\s+JOIN sys_post_role spr.*ORDER BY userId ASC\s+LIMIT \?`).
		WithArgs(int64(10), int64(20), int64(400), int64(10), int64(20), int64(400), 200).
		WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(int64(401)).AddRow(int64(402)))

	users, err := repo.ListUserIDsByRoleIDsPage(context.Background(), []int64{10, 20}, 400, 200)
	if err != nil || len(users) != 2 || users[0] != 401 || users[1] != 402 {
		t.Fatalf("ListUserIDsByRoleIDsPage() users=%#v err=%v", users, err)
	}
	if _, err := repo.ListUserIDsByRoleIDsPage(context.Background(), []int64{10}, 0, authorizationSetQueryChunkSize+1); err == nil {
		t.Fatal("expected oversized affected-user page to fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDerivedPermissionRefreshDoesNotRewriteRoleMenusOrDirectOnlyRows(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}

	mock.ExpectExec(`UPDATE sys_role_permission SET source = 'DIRECT'.*source = 'BOTH'`).
		WithArgs(int64(1), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM sys_role_permission.*source = 'MENU'`).
		WithArgs(int64(1), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE sys_role_permission SET source = 'BOTH'.*roleId = \? AND permissionId = \?`).
		WithArgs(int64(1), int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO sys_role_permission`).
		WithArgs(int64(101), int64(1), int64(12), rolePermissionSourceMenu, int64(7),
			int64(102), int64(2), int64(21), rolePermissionSourceMenu, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	nextID := int64(100)
	err = repo.ReplaceDerivedRolePermissionsBatch(context.Background(), []domain.RolePermissionAssignment{
		{RoleID: 1, DirectPermissionIDs: []int64{10, 11}, MenuPermissionIDs: []int64{11, 12}},
		{RoleID: 2, DirectPermissionIDs: []int64{20}, MenuPermissionIDs: []int64{21}},
	}, 7, func() int64 {
		nextID++
		return nextID
	})
	if err != nil {
		t.Fatalf("ReplaceDerivedRolePermissionsBatch(): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestAffectedUserCountAndExpiredCleanupPageStayBounded(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}

	mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM \(.*sys_user_role.*UNION.*sys_user_position.*sys_post_role`).
		WithArgs(int64(9), int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(450))
	count, err := repo.CountUserIDsByRoleID(context.Background(), 9)
	if err != nil || count != 450 {
		t.Fatalf("CountUserIDsByRoleID() count=%d err=%v", count, err)
	}

	mock.ExpectQuery(`(?s)SELECT DISTINCT userId.*userId > \?.*ORDER BY userId ASC.*LIMIT \?`).
		WithArgs(int64(400), 200).
		WillReturnRows(sqlmock.NewRows([]string{"userId"}).AddRow(int64(401)).AddRow(int64(402)))
	users, err := repo.ListExpiredTemporaryPermissionUserIDsPage(context.Background(), 400, 200)
	if err != nil || len(users) != 2 {
		t.Fatalf("ListExpiredTemporaryPermissionUserIDsPage() users=%v err=%v", users, err)
	}
	mock.ExpectExec(`(?s)UPDATE sys_user_permission.*WHERE userId IN \(\?, \?\).*expireTime <= NOW\(\)`).
		WithArgs(int64(401), int64(402)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.CleanupExpiredTemporaryPermissionsByUserIDs(context.Background(), users); err != nil {
		t.Fatalf("CleanupExpiredTemporaryPermissionsByUserIDs(): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestAuthorizationCreationGuardLocksAndTouchesRootRow(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}

	mock.ExpectQuery(`(?s)SELECT id\s+FROM sys_role\s+WHERE systemKey = \? AND isDeleted = 0\s+FOR UPDATE`).
		WithArgs(domain.AuthorizationRootSystemKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectExec(`UPDATE sys_role SET updateTime = NOW\(\) WHERE id = \? AND isDeleted = 0`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.LockAuthorizationCreationGuard(context.Background()); err != nil {
		t.Fatalf("LockAuthorizationCreationGuard(): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestBatchRoleGrantRevisionUsesOneUpdatePerBoundedChunk(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}
	roles := make([]domain.RoleRecord, authorizationSetQueryChunkSize+1)
	for index := range roles {
		roles[index] = domain.RoleRecord{RoleID: int64(index + 1), GrantRevision: int64(index)}
	}
	mock.ExpectExec(`(?s)UPDATE sys_role\s+SET grantRevision = CASE id .*WHERE isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, authorizationSetQueryChunkSize))
	mock.ExpectExec(`(?s)UPDATE sys_role\s+SET grantRevision = CASE id .*WHERE isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateRoleGrantRevisions(context.Background(), roles, 7); err != nil {
		t.Fatalf("UpdateRoleGrantRevisions(): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected exactly one update per chunk: %v", err)
	}
}

func TestListUserPermissionsUsesFixedBoundedSetQueries(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}

	mock.ExpectQuery(`(?s)FROM sys_user_role sur\s+JOIN sys_role sr`).
		WithArgs(int64(1001), authorizationSetQueryMaxIDs+1).
		WillReturnRows(sqlmock.NewRows([]string{"roleId"}).AddRow(int64(7)))
	mock.ExpectQuery(`(?s)FROM sys_role_permission srp\s+JOIN sys_permission sp`).
		WithArgs(int64(7), authorizationSetQueryMaxIDs+1).
		WillReturnRows(sqlmock.NewRows([]string{"code", "feature_code"}).AddRow("system:user:list", "user.admin"))
	mock.ExpectQuery(`(?s)FROM sys_role_menu srm\s+JOIN sys_menu sm`).
		WithArgs(int64(7), authorizationSetQueryMaxIDs).
		WillReturnRows(sqlmock.NewRows([]string{"code", "feature_code"}).AddRow("system:menu:list", "menu.admin"))
	mock.ExpectQuery(`(?s)FROM sys_user_permission sup\s+JOIN sys_permission sp`).
		WithArgs(int64(1001), authorizationSetQueryMaxIDs-1).
		WillReturnRows(sqlmock.NewRows([]string{"code", "feature_code"}).AddRow("system:temp:list", ""))

	records, err := repo.ListUserPermissions(context.Background(), 1001)
	if err != nil {
		t.Fatalf("ListUserPermissions(): %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("permission count=%d records=%#v", len(records), records)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListAccessRoleSourcesSplitsPostMembershipFromRoleLookup(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}

	mock.ExpectQuery(`(?s)FROM sys_user_role sur\s+JOIN sys_role sr`).
		WithArgs(int64(1001), authorizationSetQueryMaxIDs+1).
		WillReturnRows(sqlmock.NewRows([]string{
			"role_id", "role_code", "role_name", "role_status", "role_data_scope", "role_system_key",
			"assignment_source", "post_id", "post_code", "post_name", "post_dept_id", "post_org_id",
		}).AddRow(int64(7), "OPS", "Ops", 0, 5, "", "DIRECT_USER", 0, "", "", 0, 0))
	mock.ExpectQuery(`(?s)FROM sys_user_position sup\s+JOIN sys_post sp`).
		WithArgs(int64(1001), authorizationSetQueryMaxIDs+1).
		WillReturnRows(sqlmock.NewRows([]string{"post_id", "post_code", "post_name", "post_dept_id", "post_org_id"}).
			AddRow(int64(9), "ONCALL", "On-call", int64(10), int64(11)))
	mock.ExpectQuery(`(?s)FROM sys_post_role spr\s+JOIN sys_role sr`).
		WithArgs(int64(9), domain.AuthorizationRootSystemKey, authorizationSetQueryMaxIDs).
		WillReturnRows(sqlmock.NewRows([]string{"post_id", "role_id", "role_code", "role_name", "role_status", "role_data_scope", "role_system_key"}).
			AddRow(int64(9), int64(8), "SUPPORT", "Support", 0, 4, ""))

	records, err := repo.ListAccessRoleSources(context.Background(), 1001)
	if err != nil {
		t.Fatalf("ListAccessRoleSources(): %v", err)
	}
	if len(records) != 2 || records[1].PostID != 9 || records[1].RoleID != 8 {
		t.Fatalf("unexpected role sources: %#v", records)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListAccessRoleSourcesRejectsDirectRoleOverflowAtSQLBoundary(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}
	rows := sqlmock.NewRows([]string{
		"role_id", "role_code", "role_name", "role_status", "role_data_scope", "role_system_key",
		"assignment_source", "post_id", "post_code", "post_name", "post_dept_id", "post_org_id",
	})
	for i := 1; i <= authorizationSetQueryMaxIDs+1; i++ {
		rows.AddRow(int64(i), "ROLE", "Role", 0, 5, "", "DIRECT_USER", 0, "", "", 0, 0)
	}
	mock.ExpectQuery(`(?s)FROM sys_user_role sur\s+JOIN sys_role sr.*ORDER BY sr.id, sur.id\s+LIMIT \?`).
		WithArgs(int64(1001), authorizationSetQueryMaxIDs+1).
		WillReturnRows(rows)

	if _, err := repo.ListAccessRoleSources(context.Background(), 1001); err == nil {
		t.Fatal("expected direct role overflow to fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
