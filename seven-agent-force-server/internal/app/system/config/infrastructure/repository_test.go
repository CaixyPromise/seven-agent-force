package infrastructure

import (
	"context"
	"regexp"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestPersistedConfigValueKeepsBlankString(t *testing.T) {
	if got := persistedConfigValue(""); got != "" {
		t.Fatalf("expected blank config value to stay blank, got %q", got)
	}
	if got := persistedConfigValue("top-secret-value"); got != "top-secret-value" {
		t.Fatalf("expected runtime config value preserved, got %q", got)
	}
}

func TestUpdateConfigReturnsConflictWhenVersionWasConcurrentlyChanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	mock.ExpectExec(`(?s)UPDATE sys_config.*WHERE id = \? AND version = \? AND isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	item := &domain.Config{
		ID: 41, GroupID: 7, ConfigKey: "runtime.title", ConfigValue: "Seven", ValueType: "STRING",
		UIWidget: "INPUT", Exposure: "INTERNAL", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 3,
	}
	err = repo.UpdateConfig(context.Background(), item)
	appErr := apperrors.From(err)
	if appErr.Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected version conflict, got %v", err)
	}
	if item.Version != 3 {
		t.Fatalf("conflicting update must not advance local version, got %d", item.Version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresInsertConfigUsesQuotedCamelCaseAndReturningID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock"), postgres: true}
	mock.ExpectQuery(`(?s)INSERT INTO sys_config.*"groupId".*"configKey".*"requiredLogin".*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(71)))
	id, err := repo.InsertConfig(context.Background(), &domain.Config{
		GroupID: 7, ConfigKey: "runtime.title", ConfigValue: "Seven", ValueType: "STRING",
		UIWidget: "INPUT", Exposure: "INTERNAL", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 1,
	})
	if err != nil || id != 71 {
		t.Fatalf("postgres insert config: id=%d err=%v", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPersistedActorIDKeepsZero(t *testing.T) {
	if got := persistedActorID(0); got != 0 {
		t.Fatalf("expected zero actor id to stay zero, got %d", got)
	}
	if got := persistedActorID(123); got != 123 {
		t.Fatalf("expected actor id preserved, got %d", got)
	}
}

func TestQueryGroupsFiltersByGroupCodeAndName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM sys_config_group g WHERE g.isDeleted = 0 AND g.groupCode LIKE ? AND g.groupName LIKE ?`)).
		WithArgs("%cfg_ops%", "%Ops%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	page, err := repo.QueryGroups(context.Background(), domain.ConfigGroupPageQuery{
		Current:   1,
		PageSize:  20,
		GroupCode: "cfg_ops",
		GroupName: "Ops",
	})
	if err != nil {
		t.Fatalf("query groups: %v", err)
	}
	if page.Total != 0 || len(page.Records) != 0 {
		t.Fatalf("expected empty filtered page, got %#v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
