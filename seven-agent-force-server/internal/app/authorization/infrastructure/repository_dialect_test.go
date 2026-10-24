package infrastructure

import (
	"strings"
	"testing"
)

func TestAuthorizationPostgresRendererCoversSplitAccessReads(t *testing.T) {
	queries := []string{
		`SELECT sr.dataScope, sr.systemKey FROM sys_user_role sur JOIN sys_role sr ON sr.id = sur.roleId WHERE sur.userId = ? AND sur.isDeleted = 0 AND sr.isDeleted = 0`,
		`SELECT spr.postId, sr.roleId FROM sys_post_role spr JOIN sys_role sr ON sr.id = spr.roleId WHERE spr.postId IN (?) AND sr.isDeleted = 0`,
		`SELECT rp.roleId, p.permissionId, p.featureCode FROM sys_role_permission rp JOIN sys_permission p ON p.id = rp.permissionId AND p.isDeleted = 0`,
		`SELECT mp.menuId, p.permissionId FROM sys_menu_permission mp JOIN sys_permission p ON p.id = mp.permissionId AND p.isDeleted = 0`,
	}
	for _, query := range queries {
		got := authorizationPostgresRenderer.RenderPostgres(query)
		for _, unquoted := range []string{
			".dataScope", ".systemKey", ".roleId", ".userId", ".postId",
			".permissionId", ".featureCode", ".menuId", ".isDeleted",
		} {
			if strings.Contains(got, unquoted) {
				t.Fatalf("renderer left reviewed identifier %q unquoted:\n%s", unquoted, got)
			}
		}
		if strings.Contains(got, `"isDeleted" = 0`) {
			t.Fatalf("renderer left PostgreSQL boolean comparison numeric:\n%s", got)
		}
	}
}

func TestAuthorizationPostgresRendererDoesNotRewriteRuntimeLiterals(t *testing.T) {
	query := `SELECT systemKey FROM sys_role WHERE systemKey = 'systemKey isDeleted = 0' -- roleId`
	got := authorizationPostgresRenderer.RenderPostgres(query)
	if !strings.Contains(got, `'systemKey isDeleted = 0' -- roleId`) {
		t.Fatalf("renderer changed literal or comment:\n%s", got)
	}
}

func TestAuthorizationPostgresRendererCoversAcceptanceFixture(t *testing.T) {
	query := `INSERT INTO sys_user (id, userAccount, nickName, userGender, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, NOW(), NOW(), ?)`
	got := authorizationPostgresRenderer.RenderPostgres(query)
	for _, want := range []string{
		`"userAccount"`, `"nickName"`, `"userGender"`,
		`"createTime"`, `"updateTime"`, `"isDeleted"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fixture query missing quoted identifier %q:\n%s", want, got)
		}
	}
}

func TestAuthorizationBooleanProjectionAndDeadlineAreDialectSafe(t *testing.T) {
	query := `
SELECT CASE WHEN up.type THEN 1 ELSE 0 END AS permission_type
FROM sys_user_permission up
WHERE up.isDeleted = 0 AND (NOT up.type OR up.expireTime <= ` +
		(&Repository{postgres: true}).temporaryPermissionDeadlineSQL() + `)`
	got := authorizationPostgresRenderer.RenderPostgres(query)
	for _, want := range []string{
		`CASE WHEN up.type THEN 1 ELSE 0 END`,
		`up."isDeleted" = FALSE`,
		`NOT up.type`,
		`up."expireTime" <= NOW() + INTERVAL '24 hours'`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("PostgreSQL boolean query missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "COALESCE(up.type, 0)") {
		t.Fatalf("PostgreSQL query retained boolean/integer COALESCE:\n%s", got)
	}
	if got := (&Repository{}).temporaryPermissionDeadlineSQL(); got != "DATE_ADD(NOW(), INTERVAL 24 HOUR)" {
		t.Fatalf("MySQL deadline expression=%q", got)
	}
}

func TestAuthorizationBooleanValuesAreDialectSafe(t *testing.T) {
	if got := (&Repository{postgres: true}).boolValue(false); got != false {
		t.Fatalf("PostgreSQL false value=%#v", got)
	}
	if got := (&Repository{}).boolValue(false); got != 0 {
		t.Fatalf("MySQL false value=%#v", got)
	}
}
