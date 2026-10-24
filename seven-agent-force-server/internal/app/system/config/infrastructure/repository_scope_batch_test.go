package infrastructure

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestConfigScopeValidationBatchQueriesSupportBothDialects(t *testing.T) {
	for _, dialect := range []struct {
		name     string
		driver   string
		postgres bool
	}{
		{name: "mysql", driver: "mysql"},
		{name: "postgres", driver: "postgres", postgres: true},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			rawDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer rawDB.Close()
			repo := &Repository{db: sqlx.NewDb(rawDB, dialect.driver), postgres: dialect.postgres}

			mock.ExpectQuery(`(?s)FROM sys_config_group.*groupCode.* IN.*ORDER BY.*LIMIT`).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "groupCode", "groupName", "module", "permissionCode", "sortOrder",
					"status", "createTime", "updateTime", "isDeleted",
				}))
			if _, err := repo.ListGroupsByCodes(context.Background(), []string{"ops", "app"}); err != nil {
				t.Fatalf("ListGroupsByCodes(): %v", err)
			}

			mock.ExpectQuery(`(?s)FROM sys_config c.*LEFT JOIN sys_config_group g.*groupId.*configKey.*ORDER BY.*LIMIT`).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "groupId", "configKey", "configValue", "valueType", "configDesc",
					"isSensitive", "isSystemConfig", "requiredLogin", "uiWidget", "validationJson",
					"exposure", "sensitivity", "schemaVersion", "version", "extJson", "isReadonly",
					"isEnabled", "effectType", "createdBy", "createTime", "updatedBy", "updateTime",
					"isDeleted", "groupCode", "groupName",
				}))
			if _, err := repo.ListConfigsByGroupAndKeys(context.Background(), []domain.ConfigKeyRef{
				{GroupID: 2, ConfigKey: "token"},
				{GroupID: 1, ConfigKey: "title"},
			}); err != nil {
				t.Fatalf("ListConfigsByGroupAndKeys(): %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("SQL expectations: %v", err)
			}
		})
	}
}

func TestConfigScopeValidationBatchQueriesRejectUnboundedInput(t *testing.T) {
	repo := &Repository{}
	codes := make([]string, 101)
	refs := make([]domain.ConfigKeyRef, 101)
	for index := range codes {
		codes[index] = string(rune(index + 1))
		refs[index] = domain.ConfigKeyRef{GroupID: int64(index + 1), ConfigKey: "key"}
	}
	if _, err := repo.ListGroupsByCodes(context.Background(), codes); err == nil {
		t.Fatal("expected group-code input above 100 to fail closed")
	}
	if _, err := repo.ListConfigsByGroupAndKeys(context.Background(), refs); err == nil {
		t.Fatal("expected group/key input above 100 to fail closed")
	}
}
