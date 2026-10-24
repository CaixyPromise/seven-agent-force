package application

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type maintenanceQueryCountRepository struct {
	RepositoryPort
	processItems  []domain.FileProcessTask
	bindingItems  []domain.FileBindingTask
	uploadItems   []domain.UploadTask
	chunkItems    []domain.ChunkUpload
	getProcess    int
	updateProcess int
	resetProcess  int
	markBinding   int
	batchBinding  int
	expireUpload  int
	batchUpload   int
	getStrategy   int
	listStrategy  int
	updateChunk   int
	batchChunk    int
	updateHealth  int
	batchHealth   int
	healthItems   []domain.StorageStrategy
	healthUpdates []domain.StorageHealthUpdate
	partialUpload bool
	partialBind   bool
}

func (r *maintenanceQueryCountRepository) ListPendingRetryProcessTasks(context.Context, time.Time, int) ([]domain.FileProcessTask, error) {
	return append([]domain.FileProcessTask(nil), r.processItems...), nil
}
func (r *maintenanceQueryCountRepository) GetProcessTask(context.Context, int64) (*domain.FileProcessTask, error) {
	r.getProcess++
	return nil, nil
}
func (r *maintenanceQueryCountRepository) UpdateProcessTaskStatus(context.Context, int64, int, string, string) error {
	r.updateProcess++
	return nil
}
func (r *maintenanceQueryCountRepository) ResetPendingRetryProcessTasks(_ context.Context, ids []int64) (int64, error) {
	r.resetProcess++
	return int64(len(ids)), nil
}
func (r *maintenanceQueryCountRepository) ListProcessTasksByIDs(context.Context, []int64) ([]domain.FileProcessTask, error) {
	return append([]domain.FileProcessTask(nil), r.processItems...), nil
}
func (r *maintenanceQueryCountRepository) ResetProcessTasks(_ context.Context, ids []int64) (int64, error) {
	r.resetProcess++
	return int64(len(ids)), nil
}
func (r *maintenanceQueryCountRepository) ListRetryBindingTasks(context.Context, time.Time, int) ([]domain.FileBindingTask, error) {
	return append([]domain.FileBindingTask(nil), r.bindingItems...), nil
}
func (r *maintenanceQueryCountRepository) MarkBindingTask(context.Context, int64, string, string, string, string, string, string) error {
	r.markBinding++
	return nil
}
func (r *maintenanceQueryCountRepository) MarkBindingTasks(_ context.Context, items []domain.FileBindingTask) (int64, error) {
	r.batchBinding++
	if r.partialBind {
		return int64(len(items) - 1), nil
	}
	return int64(len(items)), nil
}
func (r *maintenanceQueryCountRepository) ListExpiredUploadTasks(context.Context, time.Time, int) ([]domain.UploadTask, error) {
	return append([]domain.UploadTask(nil), r.uploadItems...), nil
}
func (r *maintenanceQueryCountRepository) UpdateUploadTaskStatusIfMatch(context.Context, string, string, string) (bool, error) {
	r.expireUpload++
	return true, nil
}
func (r *maintenanceQueryCountRepository) ExpireUploadTasks(_ context.Context, items []domain.UploadTask) (int64, error) {
	r.batchUpload++
	if r.partialUpload {
		return int64(len(items) - 1), nil
	}
	return int64(len(items)), nil
}
func (r *maintenanceQueryCountRepository) ListExpiredChunkUploads(context.Context, time.Time, int) ([]domain.ChunkUpload, error) {
	return append([]domain.ChunkUpload(nil), r.chunkItems...), nil
}
func (r *maintenanceQueryCountRepository) GetStrategy(context.Context, int64) (*domain.StorageStrategy, error) {
	r.getStrategy++
	return &domain.StorageStrategy{ID: 9, ProviderType: domain.ProviderLocal, IsEnabled: true, RunState: domain.RunStateActive}, nil
}
func (r *maintenanceQueryCountRepository) ListStrategies(context.Context) ([]domain.StorageStrategy, error) {
	r.listStrategy++
	if r.healthItems != nil {
		return append([]domain.StorageStrategy(nil), r.healthItems...), nil
	}
	return []domain.StorageStrategy{{ID: 9, ProviderType: domain.ProviderLocal, IsEnabled: true, RunState: domain.RunStateActive}}, nil
}
func (r *maintenanceQueryCountRepository) UpdateStrategyHealth(context.Context, int64, int, bool) error {
	r.updateHealth++
	return nil
}
func (r *maintenanceQueryCountRepository) UpdateStrategyHealthBatch(_ context.Context, updates []domain.StorageHealthUpdate) (int64, error) {
	r.batchHealth++
	r.healthUpdates = append(r.healthUpdates, updates...)
	return int64(len(updates)), nil
}
func (r *maintenanceQueryCountRepository) UpdateChunkUpload(context.Context, *domain.ChunkUpload) error {
	r.updateChunk++
	return nil
}
func (r *maintenanceQueryCountRepository) ExpireChunkUploads(_ context.Context, ids []int64) (int64, error) {
	r.batchChunk++
	return int64(len(ids)), nil
}

type maintenanceTransactor struct{}

func (maintenanceTransactor) Enabled() bool { return true }
func (maintenanceTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type maintenanceHealthStorage struct{ ObjectStorePort }

func (maintenanceHealthStorage) Health(context.Context, domain.StorageStrategy) error { return nil }

type maintenanceSelectiveHealthStorage struct {
	ObjectStorePort
	calls []int64
}

func (s *maintenanceSelectiveHealthStorage) Health(ctx context.Context, strategy domain.StorageStrategy) error {
	s.calls = append(s.calls, strategy.ID)
	if strategy.Writable() {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

type maintenanceOutbox struct {
	OutboxPort
	batchCalls int
	eventCount int
}

func (o *maintenanceOutbox) AppendOutboxBatch(_ context.Context, events []domain.OutboxEvent) error {
	o.batchCalls++
	o.eventCount += len(events)
	return nil
}

func TestMaintenanceJobsUseBoundedSetDatabaseWrites(t *testing.T) {
	repo := &maintenanceQueryCountRepository{}
	for index := 1; index <= 51; index++ {
		id := int64(index)
		taskID := "task-" + strconv.Itoa(index)
		repo.processItems = append(repo.processItems, domain.FileProcessTask{ID: id, FileID: id, TaskType: "THUMBNAIL"})
		repo.bindingItems = append(repo.bindingItems, domain.FileBindingTask{ID: id, FileID: id, BindingToken: "binding-" + strconv.Itoa(index)})
		repo.uploadItems = append(repo.uploadItems, domain.UploadTask{ID: taskID, Status: domain.UploadTaskInit})
		repo.chunkItems = append(repo.chunkItems, domain.ChunkUpload{ID: id, UploadID: taskID, StorageStrategyID: 9})
	}
	storage := newUploadDedupStorage()
	outbox := &maintenanceOutbox{}
	service := NewService(maintenanceTransactor{}, repo, outbox, storage, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, true)

	if err := service.RetryPendingProcessTasks(context.Background(), 100); err != nil {
		t.Fatalf("RetryPendingProcessTasks() error=%v", err)
	}
	if repo.resetProcess != 2 || repo.getProcess != 0 || repo.updateProcess != 0 {
		t.Fatalf("process DB calls reset=%d get=%d update=%d", repo.resetProcess, repo.getProcess, repo.updateProcess)
	}
	if outbox.batchCalls != 2 || outbox.eventCount != 51 {
		t.Fatalf("process outbox calls=%d events=%d", outbox.batchCalls, outbox.eventCount)
	}
	if err := service.RetryPendingBindingTasks(context.Background(), 100); err != nil {
		t.Fatalf("RetryPendingBindingTasks() error=%v", err)
	}
	if repo.batchBinding != 2 || repo.markBinding != 0 {
		t.Fatalf("binding DB calls batch=%d single=%d", repo.batchBinding, repo.markBinding)
	}
	if err := service.CleanupExpiredUploadTasks(context.Background(), 100); err != nil {
		t.Fatalf("CleanupExpiredUploadTasks() error=%v", err)
	}
	if repo.batchUpload != 2 || repo.expireUpload != 0 {
		t.Fatalf("upload DB calls batch=%d single=%d", repo.batchUpload, repo.expireUpload)
	}
	if err := service.CleanupExpiredChunks(context.Background(), 100); err != nil {
		t.Fatalf("CleanupExpiredChunks() error=%v", err)
	}
	if repo.listStrategy != 1 || repo.batchChunk != 2 || repo.getStrategy != 0 || repo.updateChunk != 0 {
		t.Fatalf("chunk DB calls listStrategy=%d batch=%d get=%d update=%d", repo.listStrategy, repo.batchChunk, repo.getStrategy, repo.updateChunk)
	}
}

func TestBatchRetryProcessTasksPersistsOneOutboxBatchInTransaction(t *testing.T) {
	repo := &maintenanceQueryCountRepository{processItems: []domain.FileProcessTask{
		{ID: 1, FileID: 11, TaskType: "THUMBNAIL", Status: domain.ProcessTaskFailed},
		{ID: 2, FileID: 12, TaskType: "OCR", Status: domain.ProcessTaskPendingRetry},
	}}
	outbox := &maintenanceOutbox{}
	service := NewService(maintenanceTransactor{}, repo, outbox, nil, nil, nil, nil, config.FileDistributionConfig{}, true)
	if err := service.BatchRetryProcessTasks(context.Background(), []int64{2, 1}); err != nil {
		t.Fatalf("BatchRetryProcessTasks() error=%v", err)
	}
	if repo.resetProcess != 1 || outbox.batchCalls != 1 || outbox.eventCount != 2 {
		t.Fatalf("retry persistence shape reset=%d outboxCalls=%d events=%d",
			repo.resetProcess, outbox.batchCalls, outbox.eventCount)
	}
}

func TestMaintenancePartialCompareAndSetFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Service) error
		repo *maintenanceQueryCountRepository
	}{
		{
			name: "binding",
			repo: &maintenanceQueryCountRepository{
				bindingItems: []domain.FileBindingTask{{ID: 1, FileID: 1, BindingToken: "binding-1"}},
				partialBind:  true,
			},
			run: func(service *Service) error { return service.RetryPendingBindingTasks(context.Background(), 100) },
		},
		{
			name: "upload",
			repo: &maintenanceQueryCountRepository{
				uploadItems:   []domain.UploadTask{{ID: "task-1", Status: domain.UploadTaskInit}},
				partialUpload: true,
			},
			run: func(service *Service) error { return service.CleanupExpiredUploadTasks(context.Background(), 100) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(maintenanceTransactor{}, test.repo, nil, newUploadDedupStorage(), uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
			if err := test.run(service); err == nil {
				t.Fatal("partial compare-and-set must fail closed")
			}
		})
	}
}

func TestCheckAllStorageHealthUsesBoundedDatabaseWrites(t *testing.T) {
	repo := &maintenanceQueryCountRepository{}
	for index := 1; index <= 51; index++ {
		repo.healthItems = append(repo.healthItems, domain.StorageStrategy{
			ID: int64(index), ProviderType: domain.ProviderLocal, IsEnabled: true, RunState: domain.RunStateActive,
		})
	}
	service := NewService(maintenanceTransactor{}, repo, nil, maintenanceHealthStorage{}, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
	if err := service.CheckAllStorageHealth(context.Background(), 100); err != nil {
		t.Fatalf("CheckAllStorageHealth() error=%v", err)
	}
	if repo.listStrategy != 1 || repo.getStrategy != 0 || repo.updateHealth != 0 || repo.batchHealth != 2 {
		t.Fatalf("health DB calls list=%d get=%d update=%d batch=%d", repo.listStrategy, repo.getStrategy, repo.updateHealth, repo.batchHealth)
	}
}

func TestCheckAllStorageHealthDoesNotProbeNonWritableStrategies(t *testing.T) {
	repo := &maintenanceQueryCountRepository{healthItems: []domain.StorageStrategy{
		{ID: 1, ProviderType: domain.ProviderLocal, IsEnabled: true, RunState: domain.RunStateActive},
		{ID: 2, ProviderType: domain.ProviderAWSS3, IsEnabled: true, RunState: domain.RunStateDraining},
		{ID: 3, ProviderType: domain.ProviderAWSS3, IsEnabled: false, RunState: domain.RunStateDisabled},
	}}
	storage := &maintenanceSelectiveHealthStorage{}
	service := NewService(maintenanceTransactor{}, repo, nil, storage, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := service.CheckAllStorageHealth(ctx, 100); err != nil {
		t.Fatalf("CheckAllStorageHealth() error=%v", err)
	}
	if len(storage.calls) != 1 || storage.calls[0] != 1 {
		t.Fatalf("health calls=%v, want only active strategy 1", storage.calls)
	}
	if len(repo.healthUpdates) != 3 {
		t.Fatalf("health updates=%d, want 3", len(repo.healthUpdates))
	}
	want := map[int64]int{1: domain.HealthHealthy, 2: domain.HealthUnhealthy, 3: domain.HealthUnhealthy}
	for _, update := range repo.healthUpdates {
		if got := want[update.StrategyID]; got != update.HealthStatus {
			t.Fatalf("strategy %d health=%d, want %d", update.StrategyID, update.HealthStatus, got)
		}
	}
}
