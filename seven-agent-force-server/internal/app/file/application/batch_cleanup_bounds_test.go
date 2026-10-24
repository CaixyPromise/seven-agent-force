package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type boundedRemovalRepository struct {
	RepositoryPort
	files        map[int64]domain.FileInfo
	batchReadIDs []int64
	batchReads   int
	getFileCalls int
	claimOrder   []int64
	cleanupLimit int
}

func (r *boundedRemovalRepository) ListFilesByIDs(_ context.Context, ids []int64) ([]domain.FileInfo, error) {
	r.batchReads++
	r.batchReadIDs = append([]int64(nil), ids...)
	result := make([]domain.FileInfo, 0, len(ids))
	for _, id := range ids {
		if item, exists := r.files[id]; exists {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *boundedRemovalRepository) GetFile(_ context.Context, id int64) (*domain.FileInfo, error) {
	r.getFileCalls++
	item, exists := r.files[id]
	if !exists {
		return nil, nil
	}
	return &item, nil
}

func (r *boundedRemovalRepository) GetFileForUpdate(ctx context.Context, id int64) (*domain.FileInfo, error) {
	item, exists := r.files[id]
	if !exists {
		return nil, nil
	}
	return &item, nil
}

func (r *boundedRemovalRepository) HasActiveReferences(context.Context, int64) (bool, error) {
	return false, nil
}

func (r *boundedRemovalRepository) HasProtectedCredential(context.Context, int64, time.Time) (bool, error) {
	return false, nil
}

func (r *boundedRemovalRepository) ClaimFileForCleanup(_ context.Context, id int64, _ time.Time) (bool, error) {
	r.claimOrder = append(r.claimOrder, id)
	return true, nil
}

func (r *boundedRemovalRepository) GetStrategy(context.Context, int64) (*domain.StorageStrategy, error) {
	return &domain.StorageStrategy{ID: 1, ProviderType: domain.ProviderLocal, IsEnabled: true, RunState: domain.RunStateActive}, nil
}

func (r *boundedRemovalRepository) MarkFileDeletedIfCleaning(context.Context, int64) (bool, error) {
	return true, nil
}

func (r *boundedRemovalRepository) RestoreFileAvailableIfCleaning(context.Context, int64) error {
	return nil
}

func (r *boundedRemovalRepository) ListCleanupCandidates(context.Context, time.Time, int) ([]domain.FileInfo, error) {
	return nil, nil
}

type cleanupLimitRepository struct {
	RepositoryPort
	limit int
}

func (r *cleanupLimitRepository) ListCleanupCandidates(_ context.Context, _ time.Time, limit int) ([]domain.FileInfo, error) {
	r.limit = limit
	return nil, nil
}

func TestBatchRemoveByIDsIsBoundedDeduplicatedAndBatchPrefetched(t *testing.T) {
	repo := &boundedRemovalRepository{files: map[int64]domain.FileInfo{
		1: {ID: 1, Status: domain.FileStatusAvailable, StorageStrategyID: 1, StoragePath: "one"},
		2: {ID: 2, Status: domain.FileStatusAvailable, StorageStrategyID: 1, StoragePath: "two"},
	}}
	service := NewService(maintenanceTransactor{}, repo, nil, newUploadDedupStorage(), uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
	if _, err := service.BatchRemoveByIDs(context.Background(), []int64{2, 1, 2, -1}, 0, "", 0); err != nil {
		t.Fatalf("BatchRemoveByIDs() error=%v", err)
	}
	if repo.batchReads != 1 || repo.getFileCalls != 0 {
		t.Fatalf("file reads batch=%d single=%d", repo.batchReads, repo.getFileCalls)
	}
	if !reflect.DeepEqual(repo.batchReadIDs, []int64{1, 2}) || !reflect.DeepEqual(repo.claimOrder, []int64{1, 2}) {
		t.Fatalf("ids=%v claims=%v", repo.batchReadIDs, repo.claimOrder)
	}

	tooMany := make([]int64, 101)
	for index := range tooMany {
		tooMany[index] = int64(index + 1)
	}
	if _, err := service.BatchRemoveByIDs(context.Background(), tooMany, 0, "", 0); err == nil {
		t.Fatal("BatchRemoveByIDs() must reject more than 100 unique ids")
	}
}

func TestCleanupUnreferencedFilesCapsPositiveLimit(t *testing.T) {
	repo := &cleanupLimitRepository{}
	service := NewService(nil, repo, nil, nil, nil, nil, nil, config.FileDistributionConfig{}, false)
	if err := service.CleanupUnreferencedFiles(context.Background(), 10_000); err != nil {
		t.Fatalf("CleanupUnreferencedFiles() error=%v", err)
	}
	if repo.limit != 100 {
		t.Fatalf("cleanup repository limit=%d, want 100", repo.limit)
	}
}
