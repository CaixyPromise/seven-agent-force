package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestApplyPendingConfigBatchUsesFixedTransactionalQueryShape(t *testing.T) {
	for _, dialect := range []struct {
		name     string
		postgres bool
		driver   string
	}{
		{name: "mysql", driver: "mysql"},
		{name: "postgres", postgres: true, driver: "postgres"},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			rawDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer rawDB.Close()
			db := sqlx.NewDb(rawDB, dialect.driver)
			repo := &Repository{db: db, postgres: dialect.postgres}
			transactor := store.NewSQLXTransactor(db)
			now := time.Now().UTC()
			appliedBy := int64(1001)
			parentID := int64(301)
			item := domain.PendingConfigApply{
				PendingLogID: parentID,
				Config: domain.Config{
					ID: 21, GroupID: 1, GroupCode: "app", ConfigKey: "theme",
					ConfigValue: "v2", Version: 7, UpdatedBy: appliedBy, UpdateTime: &now,
				},
				ApplyLog: domain.ConfigChangeLog{
					ConfigID: 21, ConfigKey: "theme", OperationType: "APPLY",
					OldValue: "v1", NewValue: "v2", Status: "applied",
					ParentLogID: &parentID, OperatorID: appliedBy, OperatorName: "admin",
					OperationTime: &now, AppliedBy: &appliedBy, AppliedTime: &now,
				},
			}

			mock.ExpectBegin()
			mock.ExpectQuery(`(?s)SELECT id.*sys_config_change_log.*FOR UPDATE`).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(parentID))
			mock.ExpectExec(`(?s)UPDATE sys_config SET .*version = version \+ 1`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`(?s)UPDATE sys_config_change_log.*status = 'applied'`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`(?s)INSERT INTO sys_config_change_log`).
				WillReturnResult(sqlmock.NewResult(501, 1))
			mock.ExpectCommit()

			var claimed []int64
			err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
				var applyErr error
				claimed, applyErr = repo.ApplyPendingConfigBatch(txCtx, []domain.PendingConfigApply{item})
				return applyErr
			})
			if err != nil {
				t.Fatalf("apply pending batch: %v", err)
			}
			if len(claimed) != 1 || claimed[0] != parentID {
				t.Fatalf("unexpected claimed ids: %#v", claimed)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("sql expectations: %v", err)
			}
		})
	}
}

func TestApplyPendingConfigBatchRejectsMissingTransaction(t *testing.T) {
	rawDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql")}
	if _, err := repo.ApplyPendingConfigBatch(context.Background(), []domain.PendingConfigApply{{
		PendingLogID: 301,
		Config:       domain.Config{ID: 21},
	}}); err == nil {
		t.Fatal("expected batch apply without transaction to fail closed")
	}
}
