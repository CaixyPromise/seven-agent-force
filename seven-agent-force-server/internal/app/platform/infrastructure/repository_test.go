package infrastructure

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestManagedRepositoryReadsIncludeDisabledPlatformAndHiddenMethods(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM sys_platform\s+WHERE isDefault = 1 AND isDeleted = 0\s+ORDER BY id\s+LIMIT 1`).
		WillReturnRows(platformRows().AddRow(1, "seven-admin", "Admin", "ADMIN", nil, nil, 1, 1, 1, nil, nil, nil, domain.StatusDisabled, now, now))
	platform, err := repo.FindManagedDefaultPlatform(context.Background())
	if err != nil || platform == nil || platform.Status != domain.StatusDisabled {
		t.Fatalf("FindManagedDefaultPlatform()=%+v err=%v", platform, err)
	}

	mock.ExpectQuery(`FROM sys_platform_login_method\s+WHERE platformCode = \? AND isDeleted = 0\s+ORDER BY sortOrder ASC, displayName ASC, id ASC`).
		WithArgs("seven-admin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "platformCode", "methodType", "providerCode", "displayName", "icon", "sortOrder", "displayEnabled", "loginEnabled", "metadataJson"}).
			AddRow(9, "seven-admin", "EXTERNAL_OAUTH", "hidden-provider", "Hidden", nil, 10, 0, 0, `{"local":true}`))
	methods, err := repo.ListManagedLoginMethods(context.Background(), "seven-admin")
	if err != nil || len(methods) != 1 || methods[0].ProviderCode != "hidden-provider" {
		t.Fatalf("ListManagedLoginMethods()=%+v err=%v", methods, err)
	}
	mock.ExpectQuery(`SELECT providerCode\s+FROM sys_external_login_provider\s+WHERE providerCode IN \(\?\)\s+AND isDeleted = 0\s+ORDER BY providerCode\s+LIMIT \?`).
		WithArgs("hidden-provider", 2).WillReturnRows(sqlmock.NewRows([]string{"providerCode"}).AddRow("hidden-provider"))
	codes, err := repo.ListManagedExternalProviderCodes(context.Background(), []string{"hidden-provider"})
	if err != nil || len(codes) != 1 || codes[0] != "hidden-provider" {
		t.Fatalf("ListManagedExternalProviderCodes()=%v err=%v", codes, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestManagedRepositoryLocksCompletePolicyAndUpdatesStatusColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM sys_platform\s+WHERE isDefault = 1 AND isDeleted = 0\s+ORDER BY id\s+LIMIT 1\s+FOR UPDATE`).
		WillReturnRows(platformRows().AddRow(1, "seven-admin", "Admin", "ADMIN", nil, nil, 1, 1, 1, nil, nil, nil, domain.StatusActive, now, now))
	if _, err := repo.FindManagedDefaultPlatformForUpdate(context.Background()); err != nil {
		t.Fatalf("lock managed platform: %v", err)
	}
	mock.ExpectQuery(`FROM sys_platform_login_method\s+WHERE platformCode = \? AND isDeleted = 0\s+ORDER BY sortOrder ASC, displayName ASC, id ASC\s+FOR UPDATE`).
		WithArgs("seven-admin").WillReturnRows(sqlmock.NewRows([]string{"id", "platformCode", "methodType", "providerCode", "displayName", "icon", "sortOrder", "displayEnabled", "loginEnabled", "metadataJson"}))
	if _, err := repo.ListManagedLoginMethodsForUpdate(context.Background(), "seven-admin"); err != nil {
		t.Fatalf("lock managed methods: %v", err)
	}
	mock.ExpectQuery(`FROM sys_platform_source_rule\s+WHERE platformCode = \? AND isDeleted = 0\s+ORDER BY priority DESC, id ASC\s+FOR UPDATE`).
		WithArgs("seven-admin").WillReturnRows(sqlmock.NewRows([]string{"id", "platformCode", "matchType", "matchValue", "priority", "status", "metadataJson"}))
	if _, err := repo.ListManagedSourceRulesForUpdate(context.Background(), "seven-admin"); err != nil {
		t.Fatalf("lock managed rules: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_platform
SET status = ?, updaterId = ?, updateTime = NOW()
WHERE platformCode = ? AND isDeleted = 0`)).
		WithArgs(domain.StatusDisabled, nil, "seven-admin").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdatePlatformStatus(context.Background(), "seven-admin", domain.StatusDisabled, 0); err != nil {
		t.Fatalf("update status: %v", err)
	}
	mock.ExpectQuery(`FROM sys_platform\s+WHERE isDefault = 1 AND isDeleted = 0\s+ORDER BY id\s+LIMIT 1`).
		WillReturnRows(platformRows().AddRow(1, "seven-admin", "Admin", "ADMIN", nil, nil, 1, 1, 1, nil, nil, nil, domain.StatusDisabled, now, now))
	managed, err := repo.FindManagedDefaultPlatform(context.Background())
	if err != nil || managed == nil || managed.Status != domain.StatusDisabled {
		t.Fatalf("managed read after status update=%+v err=%v", managed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListDefaultRolesUsesThreeBoundedSetQueries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}

	mock.ExpectQuery(`(?s)FROM sys_platform_default_role pdr\s+JOIN sys_role r`).
		WithArgs("seven-admin", 201).
		WillReturnRows(sqlmock.NewRows([]string{"id", "platformCode", "roleId", "autoAssignEnabled", "status"}).
			AddRow(int64(1), "seven-admin", int64(101), 1, 0).
			AddRow(int64(2), "seven-admin", int64(102), 1, 0))
	mock.ExpectQuery(`(?s)FROM sys_role_permission rp\s+JOIN sys_permission p`).
		WithArgs(int64(101), int64(102)).
		WillReturnRows(sqlmock.NewRows([]string{"roleId", "code"}).AddRow(int64(102), "system:user:delete"))
	mock.ExpectQuery(`(?s)FROM sys_role_menu rm\s+JOIN sys_menu_permission mp.*JOIN sys_permission p`).
		WithArgs(int64(101), int64(102)).
		WillReturnRows(sqlmock.NewRows([]string{"roleId", "code"}))

	roles, err := repo.ListDefaultRoles(context.Background(), "seven-admin", 3)
	if err != nil {
		t.Fatalf("ListDefaultRoles(): %v", err)
	}
	if len(roles) != 1 || roles[0].RoleID != 101 {
		t.Fatalf("safe roles=%#v", roles)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func platformRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "platformCode", "platformName", "platformType", "description", "defaultRedirectUrl", "allowAutoRegister", "allowFormRegister", "isDefault", "defaultDeptId", "brandJson", "settingsJson", "status", "createTime", "updateTime"})
}
