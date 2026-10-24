package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type cleanupRepository struct {
	RepositoryPort
	file                 domain.FileInfo
	activeReferences     bool
	protectedCredential  bool
	referenceAfterClaim  bool
	credentialAfterClaim bool
	claimCalls           int
	restoreCalls         int
	markDeletedCalls     int
	expiredUploadTasks   []domain.UploadTask
	uploadTaskStatuses   map[string]string
}

func (r *cleanupRepository) ListCleanupCandidates(context.Context, time.Time, int) ([]domain.FileInfo, error) {
	if r.file.IsDeleted != 0 || (r.file.Status != domain.FileStatusAvailable && r.file.Status != domain.FileStatusCleaning) {
		return nil, nil
	}
	return []domain.FileInfo{r.file}, nil
}

func (r *cleanupRepository) ClaimFileForCleanup(_ context.Context, fileID int64, _ time.Time) (bool, error) {
	r.claimCalls++
	if r.file.ID != fileID || r.file.Status != domain.FileStatusAvailable || r.activeReferences || r.protectedCredential {
		return false, nil
	}
	r.file.Status = domain.FileStatusCleaning
	if r.referenceAfterClaim {
		r.activeReferences = true
	}
	if r.credentialAfterClaim {
		r.protectedCredential = true
	}
	return true, nil
}

func (r *cleanupRepository) HasActiveReferences(context.Context, int64) (bool, error) {
	return r.activeReferences, nil
}

func (r *cleanupRepository) HasProtectedCredential(context.Context, int64, time.Time) (bool, error) {
	return r.protectedCredential, nil
}

func (r *cleanupRepository) RestoreFileAvailableIfCleaning(context.Context, int64) error {
	r.restoreCalls++
	if r.file.Status == domain.FileStatusCleaning {
		r.file.Status = domain.FileStatusAvailable
	}
	return nil
}

func (r *cleanupRepository) MarkFileDeletedIfCleaning(context.Context, int64) (bool, error) {
	r.markDeletedCalls++
	if r.file.Status != domain.FileStatusCleaning {
		return false, nil
	}
	r.file.Status = domain.FileStatusDeleted
	r.file.IsDeleted = 1
	return true, nil
}

func (r *cleanupRepository) ListExpiredUploadTasks(context.Context, time.Time, int) ([]domain.UploadTask, error) {
	return append([]domain.UploadTask(nil), r.expiredUploadTasks...), nil
}

func (r *cleanupRepository) UpdateUploadTaskStatusIfMatch(_ context.Context, id, from, to string) (bool, error) {
	if r.uploadTaskStatuses[id] != from {
		return false, nil
	}
	r.uploadTaskStatuses[id] = to
	return true, nil
}

func (r *cleanupRepository) ExpireUploadTasks(_ context.Context, items []domain.UploadTask) (int64, error) {
	var matched int64
	for _, item := range items {
		if r.uploadTaskStatuses[item.ID] == item.Status {
			matched++
		}
	}
	if matched != int64(len(items)) {
		return matched, nil
	}
	for _, item := range items {
		r.uploadTaskStatuses[item.ID] = domain.UploadTaskExpired
	}
	return matched, nil
}

func (r *cleanupRepository) ResetPendingRetryProcessTasks(_ context.Context, ids []int64) (int64, error) {
	return int64(len(ids)), nil
}

func (r *cleanupRepository) MarkBindingTasks(_ context.Context, items []domain.FileBindingTask) (int64, error) {
	return int64(len(items)), nil
}

func (r *cleanupRepository) ExpireChunkUploads(_ context.Context, ids []int64) (int64, error) {
	return int64(len(ids)), nil
}

func (r *cleanupRepository) UpdateStrategyHealthBatch(_ context.Context, updates []domain.StorageHealthUpdate) (int64, error) {
	return int64(len(updates)), nil
}

func (r *cleanupRepository) GetFile(_ context.Context, fileID int64) (*domain.FileInfo, error) {
	if r.file.ID != fileID || r.file.IsDeleted != 0 {
		return nil, nil
	}
	item := r.file
	return &item, nil
}

func (r *cleanupRepository) GetFileForUpdate(ctx context.Context, fileID int64) (*domain.FileInfo, error) {
	return r.GetFile(ctx, fileID)
}

func (r *cleanupRepository) GetStrategy(context.Context, int64) (*domain.StorageStrategy, error) {
	return &domain.StorageStrategy{ID: 1, ProviderType: domain.ProviderLocal, IsEnabled: true, RunState: domain.RunStateActive}, nil
}

func TestCleanupDeletesOnlyClaimedUnprotectedFiles(t *testing.T) {
	repo, storage, service := cleanupFixture()

	if err := service.CleanupUnreferencedFiles(context.Background(), 10); err != nil {
		t.Fatalf("CleanupUnreferencedFiles() error = %v", err)
	}
	if repo.file.Status != domain.FileStatusDeleted || repo.file.IsDeleted != 1 || repo.markDeletedCalls != 1 {
		t.Fatalf("cleanup did not reach terminal state: %+v", repo.file)
	}
	if storage.hasObject(repo.file.StoragePath) || len(storage.deleted) != 1 {
		t.Fatalf("cleanup did not remove exactly one physical object: objects=%d deleted=%v", len(storage.objects), storage.deleted)
	}
	if err := service.CleanupUnreferencedFiles(context.Background(), 10); err != nil {
		t.Fatalf("idempotent CleanupUnreferencedFiles() error = %v", err)
	}
	if len(storage.deleted) != 1 || repo.markDeletedCalls != 1 {
		t.Fatalf("terminal cleanup was repeated: deletes=%v marks=%d", storage.deleted, repo.markDeletedCalls)
	}
}

func TestCleanupSkipsReferenceAndProtectionRaces(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cleanupRepository)
	}{
		{name: "active-reference", mutate: func(repo *cleanupRepository) { repo.activeReferences = true }},
		{name: "protected-credential", mutate: func(repo *cleanupRepository) { repo.protectedCredential = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, storage, service := cleanupFixture()
			test.mutate(repo)
			if err := service.CleanupUnreferencedFiles(context.Background(), 10); err != nil {
				t.Fatalf("CleanupUnreferencedFiles() error = %v", err)
			}
			if repo.file.Status != domain.FileStatusAvailable || !storage.hasObject(repo.file.StoragePath) || repo.markDeletedCalls != 0 {
				t.Fatalf("cleanup removed a protected/shared asset: file=%+v deleted=%v", repo.file, storage.deleted)
			}
		})
	}
}

func TestCleanupRestoresWhenProtectionAppearsAfterClaim(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cleanupRepository)
	}{
		{name: "reference-after-claim", mutate: func(repo *cleanupRepository) { repo.referenceAfterClaim = true }},
		{name: "credential-after-claim", mutate: func(repo *cleanupRepository) { repo.credentialAfterClaim = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, storage, service := cleanupFixture()
			test.mutate(repo)
			if err := service.CleanupUnreferencedFiles(context.Background(), 10); err != nil {
				t.Fatalf("CleanupUnreferencedFiles() error = %v", err)
			}
			if repo.claimCalls != 1 || repo.restoreCalls != 1 || repo.file.Status != domain.FileStatusAvailable {
				t.Fatalf("cleanup did not restore after post-claim protection: file=%+v claims=%d restores=%d", repo.file, repo.claimCalls, repo.restoreCalls)
			}
			if !storage.hasObject(repo.file.StoragePath) || len(storage.deleted) != 0 {
				t.Fatalf("cleanup removed an asset protected after claim: deleted=%v", storage.deleted)
			}
		})
	}
}

func TestCleanupRestoresAvailableAfterStorageFailure(t *testing.T) {
	repo, storage, service := cleanupFixture()
	storage.deleteErr = errors.New("injected storage failure")

	if err := service.CleanupUnreferencedFiles(context.Background(), 10); err == nil {
		t.Fatal("storage failure must be observable for retry")
	}
	if repo.file.Status != domain.FileStatusAvailable || repo.restoreCalls != 1 || !storage.hasObject(repo.file.StoragePath) {
		t.Fatalf("storage failure did not restore retryable state: file=%+v restores=%d", repo.file, repo.restoreCalls)
	}
}

func TestAdministrativeDeleteUsesGlobalReferenceAndCredentialProtection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cleanupRepository)
	}{
		{name: "shared-reference", mutate: func(repo *cleanupRepository) { repo.activeReferences = true }},
		{name: "unexpired-credential", mutate: func(repo *cleanupRepository) { repo.protectedCredential = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, storage, service := cleanupFixture()
			test.mutate(repo)
			if _, err := service.RemoveFile(context.Background(), repo.file.ID, 0, "", 0); err == nil {
				t.Fatal("administrative deletion must reject protected assets")
			}
			if repo.file.Status != domain.FileStatusAvailable || !storage.hasObject(repo.file.StoragePath) {
				t.Fatalf("administrative deletion bypassed protection: file=%+v", repo.file)
			}
		})
	}
}

func TestCleanupExpiredUploadTasksClosesOnlyPendingExpiredStates(t *testing.T) {
	repo, _, service := cleanupFixture()
	repo.expiredUploadTasks = []domain.UploadTask{
		{ID: "init-expired", Status: domain.UploadTaskInit},
		{ID: "uploaded-expired", Status: domain.UploadTaskUploaded},
	}
	repo.uploadTaskStatuses = map[string]string{
		"init-expired":     domain.UploadTaskInit,
		"uploaded-expired": domain.UploadTaskProcessing,
		"clean-terminal":   domain.UploadTaskClean,
	}
	if err := service.CleanupExpiredUploadTasks(context.Background(), 10); err == nil {
		t.Fatal("CleanupExpiredUploadTasks() must fail closed on a partial compare-and-set")
	}
	if repo.uploadTaskStatuses["init-expired"] != domain.UploadTaskInit {
		t.Fatalf("partial cleanup was not rolled back: %v", repo.uploadTaskStatuses)
	}
	if repo.uploadTaskStatuses["uploaded-expired"] != domain.UploadTaskProcessing {
		t.Fatalf("cleanup overwrote a concurrently claimed task: %v", repo.uploadTaskStatuses)
	}
	if repo.uploadTaskStatuses["clean-terminal"] != domain.UploadTaskClean {
		t.Fatalf("cleanup changed a terminal credential: %v", repo.uploadTaskStatuses)
	}
}

func cleanupFixture() (*cleanupRepository, *uploadDedupStorage, *Service) {
	repo := &cleanupRepository{
		file: domain.FileInfo{
			ID: 88, FileInnerName: "orphan.txt", FileSize: 6, ContentType: "text/plain",
			StorageStrategyID: 1, StoragePath: "file/88/orphan.txt",
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
		uploadTaskStatuses: map[string]string{},
	}
	storage := newUploadDedupStorage()
	storage.objects[repo.file.StoragePath] = []byte("orphan")
	service := NewService(maintenanceTransactor{}, repo, nil, storage, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
	return repo, storage, service
}
