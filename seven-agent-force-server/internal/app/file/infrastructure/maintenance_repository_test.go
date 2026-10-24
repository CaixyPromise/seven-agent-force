package infrastructure

import (
	"context"
	"regexp"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestMaintenanceBatchWritesKeepCompareAndSetGuards(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	ctx := context.Background()

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE sys_file_process_task
SET status=?, errorMsg=NULL, resultData=NULL, updateTime=?
WHERE id IN (?, ?) AND status=?`)).
		WithArgs(domain.ProcessTaskPending, sqlmock.AnyArg(), int64(1), int64(2), domain.ProcessTaskPendingRetry).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if matched, err := repo.ResetPendingRetryProcessTasks(ctx, []int64{2, 1}); err != nil || matched != 2 {
		t.Fatalf("ResetPendingRetryProcessTasks(): %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE sys_file_chunk_upload
SET status=?, updateTime=?
WHERE id IN (?, ?) AND status IN (?, ?)`)).
		WithArgs(domain.ChunkStatusExpired, sqlmock.AnyArg(), int64(1), int64(2), domain.ChunkStatusInit, domain.ChunkStatusUploading).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if matched, err := repo.ExpireChunkUploads(ctx, []int64{2, 1}); err != nil || matched != 2 {
		t.Fatalf("ExpireChunkUploads(): %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_upload_task SET status=?, updateTime=? WHERE (id=? AND status=?) OR (id=? AND status=?)`)).
		WithArgs(domain.UploadTaskExpired, sqlmock.AnyArg(), "task-a", domain.UploadTaskInit, "task-b", domain.UploadTaskUploaded).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if matched, err := repo.ExpireUploadTasks(ctx, []domain.UploadTask{
		{ID: "task-a", Status: domain.UploadTaskInit},
		{ID: "task-b", Status: domain.UploadTaskUploaded},
	}); err != nil || matched != 1 {
		t.Fatalf("ExpireUploadTasks(): %v", err)
	}

	mock.ExpectExec(`UPDATE sys_file_binding_task SET status=.*WHERE status=.*AND \(\(id=.*bindingToken=.*\) OR \(id=.*bindingToken=.*\)\)`).
		WithArgs(
			domain.BindingBound,
			int64(1), "A", int64(2), "B",
			int64(1), "PRIVATE_PREVIEW", int64(2), "PRIVATE_PREVIEW",
			int64(1), "OWNER_ONLY", int64(2), "OWNER_ONLY",
			int64(1), nil, int64(2), nil,
			sqlmock.AnyArg(), domain.BindingFailed,
			int64(1), "token-a", int64(2), "token-b",
		).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if matched, err := repo.MarkBindingTasks(ctx, []domain.FileBindingTask{
		{ID: 1, BindingToken: "token-a", Status: domain.BindingBound, DisplayName: "A", VisitStrategy: "PRIVATE_PREVIEW", AccessScope: "OWNER_ONLY"},
		{ID: 2, BindingToken: "token-b", Status: domain.BindingBound, DisplayName: "B", VisitStrategy: "PRIVATE_PREVIEW", AccessScope: "OWNER_ONLY"},
	}); err != nil || matched != 2 {
		t.Fatalf("MarkBindingTasks(): %v", err)
	}

	mock.ExpectExec(`UPDATE sys_storage_strategy SET healthStatus=CASE id.*failureCount=failureCount\+CASE id.*WHERE isDeleted=0 AND id IN \(\?,\?\)`).
		WithArgs(
			int64(1), domain.HealthHealthy, int64(2), domain.HealthUnhealthy,
			int64(1), 0, int64(2), 1,
			sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1), int64(2),
		).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if matched, err := repo.UpdateStrategyHealthBatch(ctx, []domain.StorageHealthUpdate{
		{StrategyID: 1, HealthStatus: domain.HealthHealthy, Healthy: true},
		{StrategyID: 2, HealthStatus: domain.HealthUnhealthy, Healthy: false},
	}); err != nil || matched != 2 {
		t.Fatalf("UpdateStrategyHealthBatch(): matched=%d err=%v", matched, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}
