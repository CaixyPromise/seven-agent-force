package application

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type fileReuseRepository struct {
	RepositoryPort
	file       *domain.FileInfo
	refs       []domain.FileReference
	credential *domain.UploadTask
}

func (r *fileReuseRepository) FindFileBySha256AndSize(context.Context, string, int64) (*domain.FileInfo, error) {
	return r.file, nil
}

func (r *fileReuseRepository) FindUploadCredential(_ context.Context, userID int64, scopeID string, fileID int64) (*domain.UploadTask, error) {
	if r.credential == nil || r.credential.UserID != userID || r.credential.ScopeID != scopeID || r.credential.FileID != fileID {
		return nil, nil
	}
	item := *r.credential
	return &item, nil
}

func (r *fileReuseRepository) ListReferencesByFile(context.Context, int64) ([]domain.FileReference, error) {
	return r.refs, nil
}

func (r *fileReuseRepository) FindPublicReferenceByFile(context.Context, int64) (*domain.FileReference, error) {
	return nil, nil
}

func TestCheckFileDoesNotRevealAnotherUsersReusableFile(t *testing.T) {
	repo := &fileReuseRepository{
		file: reusableFile(),
		refs: []domain.FileReference{{FileID: 71, UserID: 1001, ScopeID: "org:11"}},
	}
	service := NewService(nil, repo, nil, nil, nil, nil, nil, config.FileDistributionConfig{}, false)

	result, err := service.CheckFile(context.Background(), scopedActor(2002, 22), "abc", 128)
	if err != nil {
		t.Fatalf("CheckFile() error = %v", err)
	}
	if result.Exists {
		t.Fatal("another user's file must not be exposed as reusable")
	}
}

func TestFasterUploadRejectsAnotherUsersFile(t *testing.T) {
	repo := &fileReuseRepository{
		file: reusableFile(),
		refs: []domain.FileReference{{FileID: 71, UserID: 1001, ScopeID: "org:11"}},
	}
	service := NewService(nil, repo, nil, nil, nil, nil, nil, config.FileDistributionConfig{}, false)

	_, err := service.FasterUpload(
		context.Background(),
		scopedActor(2002, 22),
		"abc",
		128,
		UploadRequest{FileName: "private.txt"},
	)
	if err == nil {
		t.Fatal("FasterUpload() must reject a file owned only by another user")
	}
}

func TestFileReuseAllowsTheCurrentOwner(t *testing.T) {
	repo := &fileReuseRepository{
		file: reusableFile(),
		refs: []domain.FileReference{{FileID: 71, UserID: 1001, ScopeID: "org:11"}},
	}
	service := NewService(nil, repo, nil, nil, nil, nil, nil, config.FileDistributionConfig{}, false)

	owned, err := service.userOwnsFileReference(context.Background(), 71, 1001, "org:11")
	if err != nil {
		t.Fatalf("userOwnsFileReference() error = %v", err)
	}
	if !owned {
		t.Fatal("current owner should be allowed to reuse the file")
	}
}

func TestUnboundPreviewRequiresCurrentScopedCredentialAndCreatesNoReference(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour)
	repo := &fileReuseRepository{
		file: reusableFile(),
		credential: &domain.UploadTask{
			ID:                 "preview-task",
			UserID:             1001,
			ScopeID:            "org:11",
			CredentialID:       "preview-credential",
			CredentialVersion:  domain.UploadCredentialVersion1,
			Status:             domain.UploadTaskClean,
			FileID:             71,
			CredentialExpireAt: &expires,
		},
	}
	service := NewService(nil, repo, nil, nil, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)

	url, err := service.BuildDownloadURL(context.Background(), scopedActor(1001, 11), 71)
	if err != nil {
		t.Fatalf("BuildDownloadURL() error = %v", err)
	}
	if url != "/file/download?token=download-token" {
		t.Fatalf("unexpected preview URL: %q", url)
	}
	if len(repo.refs) != 0 {
		t.Fatal("unbound preview must not create a file reference")
	}
	if _, err := service.BuildDownloadURL(context.Background(), scopedActor(2002, 22), 71); err == nil {
		t.Fatal("another user must not preview by fileId")
	}
}

func TestSameUserCannotProbeOrPreviewAcrossOrganizations(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour)
	repo := &fileReuseRepository{
		file: reusableFile(),
		credential: &domain.UploadTask{
			ID:                 "org-11-task",
			UserID:             1001,
			ScopeID:            "org:11",
			CredentialID:       "org-11-credential",
			CredentialVersion:  domain.UploadCredentialVersion1,
			Status:             domain.UploadTaskClean,
			FileID:             71,
			CredentialExpireAt: &expires,
		},
	}
	service := NewService(nil, repo, nil, nil, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
	result, err := service.CheckFile(context.Background(), scopedActor(1001, 22), "abc", 128)
	if err != nil {
		t.Fatalf("cross-organization CheckFile() error = %v", err)
	}
	if result.Exists {
		t.Fatal("same user must not probe a credential from another organization")
	}
	if _, err := service.BuildDownloadURL(context.Background(), scopedActor(1001, 22), 71); err == nil {
		t.Fatal("same user must not preview a credential from another organization")
	}
}

func reusableFile() *domain.FileInfo {
	return &domain.FileInfo{
		ID:              71,
		FileSize:        128,
		FileSha256:      "abc",
		ContentType:     "text/plain",
		Status:          domain.FileStatusAvailable,
		ScanStatus:      domain.ScanStatusClean,
		IntegrityStatus: domain.IntegrityVerified,
	}
}
