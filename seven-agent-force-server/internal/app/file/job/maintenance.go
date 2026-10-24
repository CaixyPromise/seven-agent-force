package job

import (
	"context"
	"fmt"
	"time"
)

type MaintenanceService interface {
	CheckAllStorageHealth(ctx context.Context, limit int) error
	RetryPendingProcessTasks(ctx context.Context, limit int) error
	RetryPendingBindingTasks(ctx context.Context, limit int) error
	CleanupExpiredUploadTasks(ctx context.Context, limit int) error
	CleanupUnreferencedFiles(ctx context.Context, limit int) error
	DrainStorageStrategies(ctx context.Context, limit int) error
}

type MaintenanceJob struct {
	name    string
	spec    string
	limit   int
	runFunc func(context.Context, int) error
}

func NewStorageHealthJob(service MaintenanceService, intervalMs int) *MaintenanceJob {
	return newMaintenanceJob("file_storage_health_check", intervalMs, 100, service.CheckAllStorageHealth)
}

func NewProcessRetryJob(service MaintenanceService, intervalMs int, limit int) *MaintenanceJob {
	return newMaintenanceJob("file_process_retry", intervalMs, limit, service.RetryPendingProcessTasks)
}

func NewBindingRetryJob(service MaintenanceService, intervalMs int, limit int) *MaintenanceJob {
	return newMaintenanceJob("file_binding_retry", intervalMs, limit, service.RetryPendingBindingTasks)
}

func NewChunkCleanupJob(service interface {
	CleanupExpiredChunks(context.Context, int) error
}, intervalMs int, limit int) *MaintenanceJob {
	return newMaintenanceJob("file_chunk_cleanup", intervalMs, limit, service.CleanupExpiredChunks)
}

func NewUploadTaskCleanupJob(service MaintenanceService, intervalMs int, limit int) *MaintenanceJob {
	return newMaintenanceJob("file_upload_task_cleanup", intervalMs, limit, service.CleanupExpiredUploadTasks)
}

func NewFileCleanupJob(service MaintenanceService, intervalMs int, limit int) *MaintenanceJob {
	return newMaintenanceJob("file_unreferenced_cleanup", intervalMs, limit, service.CleanupUnreferencedFiles)
}

func NewStorageStrategyDrainJob(service MaintenanceService, intervalMs int, limit int) *MaintenanceJob {
	return newMaintenanceJob("file_storage_strategy_drain", intervalMs, limit, service.DrainStorageStrategies)
}

func newMaintenanceJob(name string, intervalMs int, limit int, run func(context.Context, int) error) *MaintenanceJob {
	if intervalMs <= 0 {
		intervalMs = int((5 * time.Minute) / time.Millisecond)
	}
	seconds := intervalMs / int(time.Second/time.Millisecond)
	if seconds <= 0 {
		seconds = 60
	}
	if limit <= 0 {
		limit = 100
	}
	return &MaintenanceJob{name: name, spec: fmt.Sprintf("@every %ds", seconds), limit: limit, runFunc: run}
}

func (j *MaintenanceJob) Name() string {
	return j.name
}

func (j *MaintenanceJob) Spec() string {
	return j.spec
}

func (j *MaintenanceJob) Run(ctx context.Context) error {
	if j == nil || j.runFunc == nil {
		return nil
	}
	return j.runFunc(ctx, j.limit)
}
