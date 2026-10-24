package infrastructure

import (
	"context"
	"regexp"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestReplaceUserRelationsRequireTransactionAndUseMultiValueInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	dbx := sqlx.NewDb(db, "sqlmock")
	repo := &Repository{db: dbx}
	if err := repo.ReplaceUserOrgs(context.Background(), 7, []int64{1, 2}, 1, 9); err == nil {
		t.Fatal("replace user orgs unexpectedly ran without transaction")
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM sys_user_org WHERE userId = ?`)).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys_user_org (userId, orgId, isPrimary, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, ?, NOW(), NOW(), ?), (?, ?, ?, ?, ?, NOW(), NOW(), ?)`)).
		WithArgs(int64(7), int64(1), 1, int64(9), int64(9), 0, int64(7), int64(2), 0, int64(9), int64(9), 0).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	tx := store.NewSQLXTransactor(dbx)
	if err := tx.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.ReplaceUserOrgs(txCtx, 7, []int64{2, 1, 2}, 1, 9)
	}); err != nil {
		t.Fatalf("replace user orgs: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDeletePostsRequireTransactionAndDeleteRoleChildrenFirst(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	dbx := sqlx.NewDb(db, "sqlmock")
	repo := &Repository{db: dbx}
	if err := repo.DeletePost(context.Background(), 7); err == nil {
		t.Fatal("single post delete unexpectedly ran without transaction")
	}
	if err := repo.DeletePosts(context.Background(), []int64{7, 8}); err == nil {
		t.Fatal("batch post delete unexpectedly ran without transaction")
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM sys_post_role WHERE postId = ?`)).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_post SET isDeleted = 1, updateTime = NOW() WHERE id = ? AND isDeleted = 0`)).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx := store.NewSQLXTransactor(dbx)
	if err := tx.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.DeletePost(txCtx, 7)
	}); err != nil {
		t.Fatalf("single post delete: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM sys_post_role WHERE postId IN (?, ?)`)).
		WithArgs(int64(8), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_post SET isDeleted = 1, updateTime = NOW() WHERE id IN (?, ?) AND isDeleted = 0`)).
		WithArgs(int64(8), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	if err := tx.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.DeletePosts(txCtx, []int64{8, 7, 8})
	}); err != nil {
		t.Fatalf("batch post delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
