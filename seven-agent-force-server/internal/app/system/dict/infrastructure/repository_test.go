package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestUpdateTypeReturnsConflictWhenVersionWasConcurrentlyChanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	mock.ExpectExec(`(?s)UPDATE sys_dict_type.*WHERE id = \? AND version = \? AND isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	item := &domain.DictType{
		ID: 17, DictCode: "runtime_mode", DictName: "Mode", Status: 1, ValueType: "STRING",
		UIWidget: "SELECT", Exposure: "INTERNAL", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 4,
	}
	err = repo.UpdateType(context.Background(), item)
	if apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected dict type version conflict, got %v", err)
	}
	if item.Version != 4 {
		t.Fatalf("conflicting update must not advance version, got %d", item.Version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUpdateItemReturnsConflictWhenVersionWasConcurrentlyChanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	mock.ExpectExec(`(?s)UPDATE sys_dict_item.*WHERE id = \? AND version = \? AND isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	item := &domain.DictItem{
		ID: 19, DictTypeID: 17, ItemValue: "safe", ItemLabel: "Safe", Status: 1,
		PresentationVersion: 1, Version: 9,
	}
	err = repo.UpdateItem(context.Background(), item)
	if apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected dict item version conflict, got %v", err)
	}
	if item.Version != 9 {
		t.Fatalf("conflicting update must not advance version, got %d", item.Version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestBulkDictionaryMutationsAdvanceAffectedVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}

	mock.ExpectExec(`(?s)UPDATE sys_dict_item.*SET isDeleted = 1, version = version \+ 1.*WHERE dictTypeId = \? AND isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.SoftDeleteItemsByTypeID(context.Background(), 17, 1001, time.Now().UTC()); err != nil {
		t.Fatalf("soft delete items: %v", err)
	}

	mock.ExpectExec(`(?s)UPDATE sys_dict_type.*SET sortOrder = sortOrder - 1, version = version \+ 1.*WHERE isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	if err := repo.ShiftTypeSort(context.Background(), 17, 1, 4); err != nil {
		t.Fatalf("shift type sort: %v", err)
	}

	mock.ExpectExec(`(?s)UPDATE sys_dict_item.*SET sortOrder = sortOrder \+ 1, version = version \+ 1.*WHERE dictTypeId = \? AND isDeleted = 0`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.ShiftItemSort(context.Background(), 17, 19, 5, 2); err != nil {
		t.Fatalf("shift item sort: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresInsertDictTypeAndItemUseReturningID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock"), postgres: true}
	mock.ExpectQuery(`(?s)INSERT INTO sys_dict_type.*"dictCode".*"requiredLogin".*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(31)))
	typeID, err := repo.InsertType(context.Background(), &domain.DictType{
		DictCode: "runtime_mode", DictName: "Mode", Status: 1, ValueType: "STRING",
		UIWidget: "SELECT", Exposure: "INTERNAL", Sensitivity: "NORMAL", SchemaVersion: 1, Version: 1,
	})
	if err != nil || typeID != 31 {
		t.Fatalf("postgres insert type: id=%d err=%v", typeID, err)
	}
	mock.ExpectQuery(`(?s)INSERT INTO sys_dict_item.*"dictTypeId".*"itemValue".*"extJson".*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(32)))
	itemID, err := repo.InsertItem(context.Background(), &domain.DictItem{
		DictTypeID: typeID, ItemValue: "safe", ItemLabel: "Safe", Status: 1,
		PresentationVersion: 1, Version: 1,
	})
	if err != nil || itemID != 32 {
		t.Fatalf("postgres insert item: id=%d err=%v", itemID, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
