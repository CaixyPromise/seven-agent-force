package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestUploadReusesExistingFileAfterUploadingMatchingBytes(t *testing.T) {
	content := []byte("shared file contents")
	existing := reusableUploadedFile(71, content)
	repo := newUploadDedupRepository(existing)
	repo.refs = append(repo.refs, domain.FileReference{ID: 100, FileID: existing.ID, UserID: 1001})
	storage := newUploadDedupStorage()
	service := newUploadDedupService(repo, storage)

	result, err := service.Upload(context.Background(), scopedActor(2002, 22), UploadRequest{
		FileName:    "shared.txt",
		ContentType: "text/plain",
		Reader:      bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if result.FileID != existing.ID {
		t.Fatalf("Upload() fileID = %d, want existing file %d", result.FileID, existing.ID)
	}
	if repo.insertFileCalls != 0 {
		t.Fatalf("InsertFile calls = %d, want 0", repo.insertFileCalls)
	}
	if repo.getFileForUpdateCalls != 1 {
		t.Fatalf("GetFileForUpdate calls = %d, want 1", repo.getFileForUpdateCalls)
	}
	assertNoReferenceForUser(t, repo.refs, 2002)
	assertCompletedCredential(t, repo.tasks, existing.ID, 2002)
	if len(storage.deleted) != 1 || storage.deleted[0] == existing.StoragePath {
		t.Fatalf("deleted paths = %v, want only the newly uploaded duplicate", storage.deleted)
	}
}

func TestCompleteChunkUploadReusesExistingFileAfterVerifyingCombinedBytes(t *testing.T) {
	firstPart := []byte("shared ")
	secondPart := []byte("chunk contents")
	content := append(append([]byte(nil), firstPart...), secondPart...)
	existing := reusableUploadedFile(72, content)
	repo := newUploadDedupRepository(existing)
	repo.refs = append(repo.refs, domain.FileReference{ID: 101, FileID: existing.ID, UserID: 1001})
	repo.chunk = &domain.ChunkUpload{
		UploadID:          "upload-72",
		UserID:            2002,
		ScopeID:           "org:22",
		FileName:          "shared.txt",
		FileSize:          int64(len(content)),
		ChunkSize:         len(firstPart),
		TotalChunks:       2,
		UploadedChunks:    []int{1, 2},
		ChunkSha256Map:    map[int]string{1: digest(firstPart), 2: digest(secondPart)},
		PartETagsMap:      map[int]string{1: digest(firstPart), 2: digest(secondPart)},
		FileSha256:        digest(content),
		StorageStrategyID: repo.strategy.ID,
		BizType:           "1",
		ContentType:       "text/plain",
		Status:            domain.ChunkStatusUploading,
	}
	storage := newUploadDedupStorage()
	storage.objects[chunkPartPath(repo.chunk.UploadID, 1)] = firstPart
	storage.objects[chunkPartPath(repo.chunk.UploadID, 2)] = secondPart
	service := newUploadDedupService(repo, storage)

	result, err := service.CompleteChunkUpload(context.Background(), scopedActor(2002, 22), repo.chunk.UploadID)
	if err != nil {
		t.Fatalf("CompleteChunkUpload() error = %v", err)
	}
	if result.FileID != existing.ID {
		t.Fatalf("CompleteChunkUpload() fileID = %d, want existing file %d", result.FileID, existing.ID)
	}
	if repo.insertFileCalls != 0 {
		t.Fatalf("InsertFile calls = %d, want 0", repo.insertFileCalls)
	}
	if repo.getFileForUpdateCalls != 1 {
		t.Fatalf("GetFileForUpdate calls = %d, want 1", repo.getFileForUpdateCalls)
	}
	if repo.chunk.Status != domain.ChunkStatusCompleted {
		t.Fatalf("chunk status = %d, want completed", repo.chunk.Status)
	}
	assertNoReferenceForUser(t, repo.refs, 2002)
	assertCompletedCredential(t, repo.tasks, existing.ID, 2002)
	if len(storage.deleted) != 3 {
		t.Fatalf("deleted paths = %v, want combined duplicate and two chunks", storage.deleted)
	}
	if storage.hasObject(chunkPartPath(repo.chunk.UploadID, 1)) || storage.hasObject(chunkPartPath(repo.chunk.UploadID, 2)) {
		t.Fatal("completed chunk parts were not deleted")
	}
}

func TestUploadRetriesReferenceOnceAfterConcurrentFileInsert(t *testing.T) {
	content := []byte("concurrent shared contents")
	existing := reusableUploadedFile(73, content)
	insertErr := errors.New("duplicate file hash and size")
	repo := newUploadDedupRepository(nil)
	repo.fileAfterInsertFailure = existing
	repo.insertFileErr = insertErr
	repo.refs = append(repo.refs, domain.FileReference{ID: 102, FileID: existing.ID, UserID: 1001})
	storage := newUploadDedupStorage()
	service := newUploadDedupService(repo, storage)

	result, err := service.Upload(context.Background(), scopedActor(2002, 22), UploadRequest{
		FileName:    "shared.txt",
		ContentType: "text/plain",
		Reader:      bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if result.FileID != existing.ID {
		t.Fatalf("Upload() fileID = %d, want concurrent file %d", result.FileID, existing.ID)
	}
	if repo.insertFileCalls != 1 {
		t.Fatalf("InsertFile calls = %d, want 1", repo.insertFileCalls)
	}
	if repo.findFileCalls != 2 {
		t.Fatalf("FindFileBySha256AndSize calls = %d, want one precheck and one retry", repo.findFileCalls)
	}
	if repo.getFileForUpdateCalls != 1 {
		t.Fatalf("GetFileForUpdate calls = %d, want 1", repo.getFileForUpdateCalls)
	}
	assertNoReferenceForUser(t, repo.refs, 2002)
	assertCompletedCredential(t, repo.tasks, existing.ID, 2002)
	if len(storage.deleted) != 1 {
		t.Fatalf("deleted paths = %v, want concurrent duplicate object deleted", storage.deleted)
	}
}

func TestUploadDoesNotMaskUnresolvedInsertFailure(t *testing.T) {
	content := []byte("unresolved insert failure")
	insertErr := errors.New("insert file failed")
	repo := newUploadDedupRepository(nil)
	repo.insertFileErr = insertErr
	storage := newUploadDedupStorage()
	service := newUploadDedupService(repo, storage)

	_, err := service.Upload(context.Background(), scopedActor(2002, 22), UploadRequest{
		FileName:    "shared.txt",
		ContentType: "text/plain",
		Reader:      bytes.NewReader(content),
	})
	if !errors.Is(err, insertErr) {
		t.Fatalf("Upload() error = %v, want original insert error", err)
	}
	if repo.findFileCalls != 2 {
		t.Fatalf("FindFileBySha256AndSize calls = %d, want one controlled retry", repo.findFileCalls)
	}
	if len(repo.refs) != 0 {
		t.Fatalf("references = %v, want none", repo.refs)
	}
}

func TestUploadDoesNotRetryAfterCredentialFailure(t *testing.T) {
	content := []byte("credential failure")
	credentialErr := errors.New("insert credential failed")
	repo := newUploadDedupRepository(nil)
	repo.insertCredentialErr = credentialErr
	storage := newUploadDedupStorage()
	service := newUploadDedupService(repo, storage)

	_, err := service.Upload(context.Background(), scopedActor(2002, 22), UploadRequest{
		FileName:    "shared.txt",
		ContentType: "text/plain",
		Reader:      bytes.NewReader(content),
	})
	if !errors.Is(err, credentialErr) {
		t.Fatalf("Upload() error = %v, want credential error", err)
	}
	if repo.findFileCalls != 1 {
		t.Fatalf("FindFileBySha256AndSize calls = %d, want no retry after credential failure", repo.findFileCalls)
	}
}

func TestHandleUploadTaskMessageCompletesCredentialWithoutCreatingReference(t *testing.T) {
	content := []byte("async upload contents")
	repo := newUploadDedupRepository(nil)
	repo.task = &domain.UploadTask{
		ID:                "task-async",
		UserID:            2002,
		ScopeID:           "org:22",
		FileName:          "async.txt",
		ContentType:       "text/plain",
		StorageStrategyID: repo.strategy.ID,
		ObjectKeyStaging:  "staging/task-async",
		Status:            domain.UploadTaskUploaded,
		ExpectedSize:      int64(len(content)),
		ExpectedSha256:    digest(content),
	}
	storage := newUploadDedupStorage()
	storage.objects[repo.task.ObjectKeyStaging] = content
	service := newUploadDedupService(repo, storage)

	if err := service.HandleUploadTaskMessage(context.Background(), domain.UploadTaskMessage{
		MessageID: "message-async",
		TaskID:    repo.task.ID,
	}); err != nil {
		t.Fatalf("HandleUploadTaskMessage() error = %v", err)
	}
	if repo.task.Status != domain.UploadTaskClean || repo.task.FileID != 900 {
		t.Fatalf("task terminal state = (%s, %d), want (CLEAN, 900)", repo.task.Status, repo.task.FileID)
	}
	assertNoReferenceForUser(t, repo.refs, 2002)
}

func TestReusableCredentialPathsLockFileBeforeCompletion(t *testing.T) {
	content := []byte("credential-lock")
	existing := reusableUploadedFile(81, content)

	t.Run("faster upload", func(t *testing.T) {
		repo := newUploadDedupRepository(existing)
		repo.refs = append(repo.refs, domain.FileReference{ID: 181, FileID: existing.ID, UserID: 2002, ScopeID: "org:22"})
		service := newUploadDedupService(repo, newUploadDedupStorage())

		result, err := service.FasterUpload(context.Background(), scopedActor(2002, 22), digest(content), int64(len(content)), UploadRequest{
			FileName:    "locked.txt",
			ContentType: "text/plain",
		})
		if err != nil {
			t.Fatalf("FasterUpload() error = %v", err)
		}
		if result.FileID != existing.ID || repo.getFileForUpdateCalls != 1 {
			t.Fatalf("faster upload result=%+v locks=%d", result, repo.getFileForUpdateCalls)
		}
	})

	t.Run("instant confirmation", func(t *testing.T) {
		repo := newUploadDedupRepository(existing)
		repo.refs = append(repo.refs, domain.FileReference{ID: 182, FileID: existing.ID, UserID: 2002, ScopeID: "org:22"})
		repo.task = &domain.UploadTask{
			ID:             "instant-81",
			UserID:         2002,
			ScopeID:        "org:22",
			FileName:       "locked.txt",
			ContentType:    "text/plain",
			ExpectedSize:   int64(len(content)),
			ExpectedSha256: digest(content),
			Status:         domain.UploadTaskInit,
		}
		service := newUploadDedupService(repo, newUploadDedupStorage())

		result, err := service.ConfirmInstantUpload(context.Background(), scopedActor(2002, 22), repo.task.ID)
		if err != nil {
			t.Fatalf("ConfirmInstantUpload() error = %v", err)
		}
		if result.FileID != existing.ID || repo.getFileForUpdateCalls != 1 {
			t.Fatalf("instant result=%+v locks=%d", result, repo.getFileForUpdateCalls)
		}
	})

	t.Run("async reused file", func(t *testing.T) {
		repo := newUploadDedupRepository(existing)
		repo.task = &domain.UploadTask{
			ID:                "async-81",
			UserID:            2002,
			ScopeID:           "org:22",
			FileName:          "locked.txt",
			ContentType:       "text/plain",
			StorageStrategyID: repo.strategy.ID,
			ObjectKeyStaging:  "staging/async-81",
			Status:            domain.UploadTaskUploaded,
			ExpectedSize:      int64(len(content)),
			ExpectedSha256:    digest(content),
		}
		storage := newUploadDedupStorage()
		storage.objects[repo.task.ObjectKeyStaging] = content
		service := newUploadDedupService(repo, storage)

		if err := service.HandleUploadTaskMessage(context.Background(), domain.UploadTaskMessage{TaskID: repo.task.ID}); err != nil {
			t.Fatalf("HandleUploadTaskMessage() error = %v", err)
		}
		if repo.task.FileID != existing.ID || repo.getFileForUpdateCalls != 1 {
			t.Fatalf("async task=%+v locks=%d", repo.task, repo.getFileForUpdateCalls)
		}
	})
}

func TestUploadDoesNotIssueCredentialAfterCleanupClaim(t *testing.T) {
	content := []byte("cleanup-race")
	existing := reusableUploadedFile(82, content)
	cleaning := *existing
	cleaning.Status = domain.FileStatusCleaning
	repo := newUploadDedupRepository(existing)
	repo.fileForUpdate = &cleaning
	storage := newUploadDedupStorage()
	service := newUploadDedupService(repo, storage)

	_, err := service.Upload(context.Background(), scopedActor(2002, 22), UploadRequest{
		FileName:    "cleanup.txt",
		ContentType: "text/plain",
		Reader:      bytes.NewReader(content),
	})
	if err == nil {
		t.Fatal("Upload() must reject a reusable file claimed by cleanup")
	}
	if repo.getFileForUpdateCalls != 1 || len(repo.tasks) != 0 {
		t.Fatalf("locks=%d credentials=%d, want one lock and no credential", repo.getFileForUpdateCalls, len(repo.tasks))
	}
}

type uploadDedupRepository struct {
	RepositoryPort
	strategy               domain.StorageStrategy
	file                   *domain.FileInfo
	fileForUpdate          *domain.FileInfo
	fileAfterInsertFailure *domain.FileInfo
	chunk                  *domain.ChunkUpload
	task                   *domain.UploadTask
	refs                   []domain.FileReference
	tasks                  []domain.UploadTask
	findFileCalls          int
	getFileForUpdateCalls  int
	insertFileCalls        int
	insertFileErr          error
	insertCredentialErr    error
}

func newUploadDedupRepository(file *domain.FileInfo) *uploadDedupRepository {
	return &uploadDedupRepository{
		strategy: domain.StorageStrategy{
			ID:           1,
			ProviderType: domain.ProviderLocal,
			IsDefault:    true,
			IsEnabled:    true,
			RunState:     domain.RunStateActive,
		},
		file: file,
	}
}

func (r *uploadDedupRepository) GetDefaultStrategy(context.Context) (*domain.StorageStrategy, error) {
	strategy := r.strategy
	return &strategy, nil
}

func (r *uploadDedupRepository) GetStrategy(context.Context, int64) (*domain.StorageStrategy, error) {
	strategy := r.strategy
	return &strategy, nil
}

func (r *uploadDedupRepository) FindFileBySha256AndSize(context.Context, string, int64) (*domain.FileInfo, error) {
	r.findFileCalls++
	if r.findFileCalls > 1 && r.fileAfterInsertFailure != nil {
		return r.fileAfterInsertFailure, nil
	}
	return r.file, nil
}

func (r *uploadDedupRepository) GetFileForUpdate(_ context.Context, fileID int64) (*domain.FileInfo, error) {
	r.getFileForUpdateCalls++
	candidate := r.fileForUpdate
	if candidate == nil {
		candidate = r.file
	}
	if candidate == nil && r.fileAfterInsertFailure != nil && r.fileAfterInsertFailure.ID == fileID {
		candidate = r.fileAfterInsertFailure
	}
	if candidate == nil || candidate.ID != fileID {
		return nil, nil
	}
	copied := *candidate
	return &copied, nil
}

func (r *uploadDedupRepository) InsertFile(context.Context, *domain.FileInfo) (int64, error) {
	r.insertFileCalls++
	if r.insertFileErr != nil {
		return 0, r.insertFileErr
	}
	return 900, nil
}

func (r *uploadDedupRepository) GetUploadTask(_ context.Context, taskID string) (*domain.UploadTask, error) {
	if r.task == nil || r.task.ID != taskID {
		return nil, nil
	}
	copied := *r.task
	return &copied, nil
}

func (r *uploadDedupRepository) UpdateUploadTask(_ context.Context, task *domain.UploadTask) error {
	copied := *task
	r.task = &copied
	return nil
}

func (r *uploadDedupRepository) UpdateUploadTaskStatusIfMatch(_ context.Context, taskID, from, to string) (bool, error) {
	if r.task == nil || r.task.ID != taskID || r.task.Status != from {
		return false, nil
	}
	r.task.Status = to
	return true, nil
}

func (r *uploadDedupRepository) InsertUploadTask(_ context.Context, item *domain.UploadTask) error {
	if r.insertCredentialErr != nil {
		return r.insertCredentialErr
	}
	copied := *item
	r.tasks = append(r.tasks, copied)
	return nil
}

func (r *uploadDedupRepository) FindUploadCredential(_ context.Context, userID int64, scopeID string, fileID int64) (*domain.UploadTask, error) {
	for index := len(r.tasks) - 1; index >= 0; index-- {
		task := r.tasks[index]
		if task.UserID == userID && task.ScopeID == scopeID && task.FileID == fileID {
			copied := task
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *uploadDedupRepository) FindPublicReferenceByFile(context.Context, int64) (*domain.FileReference, error) {
	return nil, nil
}

func (r *uploadDedupRepository) ListReferencesByFile(_ context.Context, fileID int64) ([]domain.FileReference, error) {
	refs := make([]domain.FileReference, 0, len(r.refs))
	for _, ref := range r.refs {
		if ref.FileID == fileID {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func (r *uploadDedupRepository) GetChunkUpload(_ context.Context, uploadID string) (*domain.ChunkUpload, error) {
	if r.chunk == nil || r.chunk.UploadID != uploadID {
		return nil, nil
	}
	return r.chunk, nil
}

func (r *uploadDedupRepository) UpdateChunkUpload(_ context.Context, upload *domain.ChunkUpload) error {
	copied := *upload
	r.chunk = &copied
	return nil
}

type uploadDedupStorage struct {
	ObjectStorePort
	objects   map[string][]byte
	deleted   []string
	deleteErr error
}

func newUploadDedupStorage() *uploadDedupStorage {
	return &uploadDedupStorage{objects: map[string][]byte{}}
}

func (s *uploadDedupStorage) Save(_ context.Context, _ domain.StorageStrategy, storagePath string, reader io.Reader, contentType string) (domain.StoredObject, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return domain.StoredObject{}, err
	}
	s.objects[storagePath] = append([]byte(nil), content...)
	return domain.StoredObject{
		StoragePath: storagePath,
		Size:        int64(len(content)),
		SHA256:      digest(content),
		ContentType: contentType,
	}, nil
}

func (s *uploadDedupStorage) Open(_ context.Context, _ domain.StorageStrategy, file domain.FileInfo) (domain.DownloadObject, error) {
	content, ok := s.objects[file.StoragePath]
	if !ok {
		return domain.DownloadObject{}, errors.New("object not found")
	}
	return domain.DownloadObject{
		File:        io.NopCloser(bytes.NewReader(content)),
		Size:        int64(len(content)),
		ContentType: file.ContentType,
		Name:        file.FileInnerName,
	}, nil
}

func (s *uploadDedupStorage) Delete(_ context.Context, _ domain.StorageStrategy, storagePath string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, storagePath)
	delete(s.objects, storagePath)
	return nil
}

func (s *uploadDedupStorage) PublicURL(_ domain.StorageStrategy, storagePath string) string {
	return "/public/" + storagePath
}

func (s *uploadDedupStorage) hasObject(storagePath string) bool {
	_, ok := s.objects[storagePath]
	return ok
}

type uploadDedupTokens struct {
	DownloadTokenPort
}

func (uploadDedupTokens) Issue(context.Context, int64, int64, string, string) (string, error) {
	return "download-token", nil
}

func newUploadDedupService(repo RepositoryPort, storage ObjectStorePort) *Service {
	return NewService(nil, repo, nil, storage, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
}

func reusableUploadedFile(id int64, content []byte) *domain.FileInfo {
	return &domain.FileInfo{
		ID:                id,
		FileInnerName:     "owner-copy.txt",
		FileSize:          int64(len(content)),
		FileSha256:        digest(content),
		HashAlgorithm:     "SHA-256",
		ContentType:       "text/plain",
		StorageStrategyID: 1,
		StoragePath:       "file/1001/original.txt",
		Status:            domain.FileStatusAvailable,
		ScanStatus:        domain.ScanStatusClean,
		IntegrityStatus:   domain.IntegrityVerified,
	}
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func assertNoReferenceForUser(t *testing.T, refs []domain.FileReference, userID int64) {
	t.Helper()
	matches := referencesForUser(refs, userID)
	if len(matches) != 0 {
		t.Fatalf("user %d references = %v, want none from upload", userID, matches)
	}
}

func assertCompletedCredential(t *testing.T, tasks []domain.UploadTask, fileID, userID int64) {
	t.Helper()
	var matches []domain.UploadTask
	for _, task := range tasks {
		if task.UserID == userID && task.FileID == fileID {
			matches = append(matches, task)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("user %d completed credentials = %v, want exactly one", userID, matches)
	}
	if matches[0].Status != domain.UploadTaskClean {
		t.Fatalf("credential status = %q, want %q", matches[0].Status, domain.UploadTaskClean)
	}
}

func referencesForUser(refs []domain.FileReference, userID int64) []domain.FileReference {
	var matches []domain.FileReference
	for _, ref := range refs {
		if ref.UserID == userID {
			matches = append(matches, ref)
		}
	}
	return matches
}
