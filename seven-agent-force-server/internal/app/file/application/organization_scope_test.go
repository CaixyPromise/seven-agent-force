package application

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

func scopedActor(userID, orgID int64) Actor {
	return Actor{
		UserID:        userID,
		ScopeID:       organizationScopeID(orgID),
		ScopeSource:   "primary-org",
		Authenticated: true,
	}
}

func scopedContext(userID, orgID int64) context.Context {
	return securitycontext.WithUser(context.Background(), &securitycontext.UserContext{
		UserID:       userID,
		PrimaryOrgID: orgID,
		OrgIDs:       []int64{orgID},
	})
}

func organizationScopeID(orgID int64) string {
	scope, err := securitycontext.ResolveOrganizationScope(&securitycontext.UserContext{
		UserID:       1,
		PrimaryOrgID: orgID,
		OrgIDs:       []int64{orgID},
	})
	if err != nil {
		panic(err)
	}
	return scope.ScopeID
}

func TestSameUserCannotAccessUploadReceiptsAcrossOrganizations(t *testing.T) {
	repo := newUploadDedupRepository(nil)
	repo.task = &domain.UploadTask{
		ID:      "org-11-task",
		UserID:  101,
		ScopeID: "org:11",
		Status:  domain.UploadTaskInit,
	}
	repo.chunk = &domain.ChunkUpload{
		UploadID: "org-11-chunk",
		UserID:   101,
		ScopeID:  "org:11",
		Status:   domain.ChunkStatusInit,
	}
	service := newUploadDedupService(repo, newUploadDedupStorage())
	if _, err := service.GetUploadTaskStatus(context.Background(), scopedActor(101, 22), repo.task.ID); err == nil {
		t.Fatal("same user must not read an upload task from another organization")
	}
	if _, err := service.ChunkUploadStatus(context.Background(), scopedActor(101, 22), repo.chunk.UploadID); err == nil {
		t.Fatal("same user must not read a chunk task from another organization")
	}
}

func TestUploadCredentialRecordsAuditableScopeFallback(t *testing.T) {
	repo := newUploadDedupRepository(nil)
	storage := newUploadDedupStorage()
	service := newUploadDedupService(repo, storage)
	actor := scopedActor(101, 22)
	actor.ScopeSource = "single-org-fallback"
	if _, err := service.Upload(context.Background(), actor, UploadRequest{
		FileName:     "audit.txt",
		ContentType:  "text/plain",
		Reader:       strings.NewReader("audit"),
		ExpectedSize: 5,
	}); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if len(repo.tasks) != 1 || repo.tasks[0].ScopeID != "org:22" ||
		repo.tasks[0].BindingChannel != "upload-only;scope-source=single-org-fallback" {
		t.Fatalf("credential scope audit mismatch: %+v", repo.tasks)
	}
}

func TestUploadHTTPCommandsDoNotExposeScopeOverride(t *testing.T) {
	for _, command := range []any{
		UploadRequest{},
		UploadTaskInitRequest{},
		ChunkUploadInitRequest{},
		ChunkPartRequest{},
		UploadCallbackRequest{},
	} {
		commandType := reflect.TypeOf(command)
		if _, ok := commandType.FieldByName("ScopeID"); ok {
			t.Fatalf("%s exposes a client organization scope override", commandType.Name())
		}
	}
}
