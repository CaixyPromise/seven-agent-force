package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type bindingRepository struct {
	RepositoryPort
	file              domain.FileInfo
	credential        domain.UploadTask
	files             map[int64]domain.FileInfo
	credentials       map[int64]domain.UploadTask
	refs              []domain.FileReference
	credentialLookups int
}

func (r *bindingRepository) FindUploadCredential(_ context.Context, userID int64, scopeID string, fileID int64) (*domain.UploadTask, error) {
	r.credentialLookups++
	task, found := r.credentials[fileID]
	if !found {
		task = r.credential
	}
	if task.UserID != userID || task.ScopeID != scopeID || task.FileID != fileID {
		return nil, nil
	}
	item := task
	return &item, nil
}

func (r *bindingRepository) GetFile(_ context.Context, fileID int64) (*domain.FileInfo, error) {
	file, found := r.files[fileID]
	if !found {
		file = r.file
	}
	if file.ID != fileID {
		return nil, nil
	}
	item := file
	return &item, nil
}

func (r *bindingRepository) GetFileForUpdate(ctx context.Context, fileID int64) (*domain.FileInfo, error) {
	return r.GetFile(ctx, fileID)
}

func (r *bindingRepository) GetStrategy(context.Context, int64) (*domain.StorageStrategy, error) {
	return &domain.StorageStrategy{ID: 1, ProviderType: domain.ProviderLocal, IsEnabled: true, RunState: domain.RunStateActive}, nil
}

func (r *bindingRepository) ListReferencesByBiz(_ context.Context, userID int64, bizType string, bizID int64) ([]domain.FileReference, error) {
	result := []domain.FileReference{}
	for _, ref := range r.refs {
		if ref.UserID == userID && ref.BizType == bizType && ref.BizID == bizID && ref.IsDeleted == 0 {
			result = append(result, ref)
		}
	}
	return result, nil
}

func (r *bindingRepository) InsertReference(_ context.Context, ref *domain.FileReference) (int64, error) {
	if ref.ID == 0 {
		ref.ID = int64(len(r.refs) + 1)
	}
	r.refs = append(r.refs, *ref)
	return ref.ID, nil
}

func (r *bindingRepository) SoftDeleteReference(_ context.Context, fileID, userID int64, bizType string, bizID int64) error {
	for index := range r.refs {
		ref := &r.refs[index]
		if ref.FileID == fileID && ref.UserID == userID && ref.BizType == bizType && ref.BizID == bizID {
			ref.IsDeleted = 1
		}
	}
	return nil
}

func (r *bindingRepository) SoftDeleteReferenceInScope(_ context.Context, fileID, userID int64, scopeID, bizType string, bizID int64) error {
	for index := range r.refs {
		ref := &r.refs[index]
		if ref.FileID == fileID && ref.UserID == userID && ref.ScopeID == scopeID && ref.BizType == bizType && ref.BizID == bizID {
			ref.IsDeleted = 1
		}
	}
	return nil
}

func (r *bindingRepository) FindConfigAssetReference(_ context.Context, configID int64) (*domain.FileReference, error) {
	for _, ref := range r.refs {
		if ref.BizType == filefacade.ConfigAssetBizType && ref.BizID == configID && ref.IsDeleted == 0 {
			copyRef := ref
			return &copyRef, nil
		}
	}
	return nil, nil
}

func (r *bindingRepository) UpdateConfigAssetReference(_ context.Context, item *domain.FileReference) error {
	if item == nil {
		return nil
	}
	for index := range r.refs {
		if r.refs[index].ID == item.ID {
			r.refs[index] = *item
			return nil
		}
	}
	return nil
}

func (r *bindingRepository) SoftDeleteConfigAssetReference(_ context.Context, configID int64, scopeID string) error {
	for index := range r.refs {
		ref := &r.refs[index]
		if ref.BizType == filefacade.ConfigAssetBizType && ref.BizID == configID && ref.ScopeID == scopeID && ref.IsDeleted == 0 {
			ref.IsDeleted = 1
		}
	}
	return nil
}

func (r *bindingRepository) GetReference(_ context.Context, id int64) (*domain.FileReference, error) {
	for _, ref := range r.refs {
		if ref.ID == id && ref.IsDeleted == 0 {
			copyRef := ref
			return &copyRef, nil
		}
	}
	return nil, nil
}

func (r *bindingRepository) ListReferencesByFile(_ context.Context, fileID int64) ([]domain.FileReference, error) {
	result := make([]domain.FileReference, 0, len(r.refs))
	for _, ref := range r.refs {
		if ref.FileID == fileID && ref.IsDeleted == 0 {
			result = append(result, ref)
		}
	}
	return result, nil
}

func (r *bindingRepository) FindPublicReferenceByFile(_ context.Context, fileID int64) (*domain.FileReference, error) {
	for _, ref := range r.refs {
		if ref.FileID == fileID && ref.IsDeleted == 0 && ref.BizType != filefacade.ConfigAssetBizType && ref.AccessScope == string(filefacade.AccessPublic) && ref.VisitStrategy == string(filefacade.VisitPublicStatic) {
			copyRef := ref
			return &copyRef, nil
		}
	}
	return nil, nil
}

func TestBindUploadedFileDerivesAvatarPolicyAndIsIdempotent(t *testing.T) {
	payload := validPNG(t)
	repo, storage, service := bindingFixture(payload, "avatar.png", "image/png")

	command := filefacade.BindUploadedFileCommand{
		FileID: 77,
		Slot:   filefacade.FileAssetSlotUserAvatar,
	}
	first, err := service.BindUploadedFile(scopedContext(101, 22), command)
	if err != nil {
		t.Fatalf("BindUploadedFile() error = %v", err)
	}
	second, err := service.BindUploadedFile(scopedContext(101, 22), command)
	if err != nil {
		t.Fatalf("BindUploadedFile() retry error = %v", err)
	}
	if first.ID != second.ID || len(repo.refs) != 1 {
		t.Fatalf("idempotent binding created duplicate references: first=%d second=%d refs=%d", first.ID, second.ID, len(repo.refs))
	}
	if first.BizType != "0" || first.BizID != 101 || first.AccessScope != "PUBLIC" || first.VisitStrategy != "PUBLIC_STATIC" {
		t.Fatalf("server-derived avatar policy is invalid: %+v", first)
	}
	if first.VisitURL != "/public/avatar/101/avatar.png" || len(storage.objects) != 1 {
		t.Fatalf("public result/storage mismatch: result=%+v objects=%d", first, len(storage.objects))
	}
}

func TestBindUploadedFileAcceptsApprovedImageFormats(t *testing.T) {
	tests := []struct {
		name        string
		payload     func(*testing.T) []byte
		fileName    string
		contentType string
	}{
		{name: "png", payload: validPNG, fileName: "avatar.png", contentType: "image/png"},
		{name: "jpeg", payload: validJPEG, fileName: "avatar.jpg", contentType: "image/jpeg"},
		{name: "webp", payload: validWebP, fileName: "avatar.webp", contentType: "image/webp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, _, service := bindingFixture(test.payload(t), test.fileName, test.contentType)
			if _, err := service.BindUploadedFile(scopedContext(101, 22), filefacade.BindUploadedFileCommand{
				FileID: 77,
				Slot:   filefacade.FileAssetSlotUserAvatar,
			}); err != nil {
				t.Fatalf("approved image format must bind: %v", err)
			}
			if len(repo.refs) != 1 {
				t.Fatalf("approved image format created %d references, want 1", len(repo.refs))
			}
		})
	}
}

func TestBindUploadedFileRejectsInvalidCredentialSubjectsAndLifetime(t *testing.T) {
	payload := validPNG(t)
	tests := []struct {
		name   string
		mutate func(*domain.UploadTask)
	}{
		{name: "cross-user", mutate: func(task *domain.UploadTask) { task.UserID = 202 }},
		{name: "same-user-cross-organization", mutate: func(task *domain.UploadTask) { task.ScopeID = "org:99" }},
		{name: "expired", mutate: func(task *domain.UploadTask) {
			expired := time.Now().UTC().Add(-time.Minute)
			task.CredentialExpireAt = &expired
		}},
		{name: "revoked", mutate: func(task *domain.UploadTask) {
			revoked := time.Now().UTC()
			task.RevokedAt = &revoked
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, _, service := bindingFixture(payload, "avatar.png", "image/png")
			test.mutate(&repo.credential)
			_, err := service.BindUploadedFile(scopedContext(101, 22), filefacade.BindUploadedFileCommand{
				FileID: 77,
				Slot:   filefacade.FileAssetSlotUserAvatar,
			})
			if err == nil {
				t.Fatal("invalid credential must not bind")
			}
			if len(repo.refs) != 0 {
				t.Fatal("failed credential validation must not create a reference")
			}
		})
	}
}

func TestBindUploadedFileRejectsUnsafeImageCorpus(t *testing.T) {
	valid := validPNG(t)
	tests := []struct {
		name        string
		payload     []byte
		fileName    string
		contentType string
	}{
		{name: "svg", payload: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), fileName: "avatar.svg", contentType: "image/svg+xml"},
		{name: "html", payload: []byte(`<html><body>active</body></html>`), fileName: "avatar.png", contentType: "image/png"},
		{name: "forged-mime", payload: valid, fileName: "avatar.png", contentType: "image/jpeg"},
		{name: "forged-extension", payload: valid, fileName: "avatar.jpg", contentType: "image/png"},
		{name: "mixed-trailing-payload", payload: append(append([]byte{}, valid...), []byte(`<script>alert(1)</script>`)...), fileName: "avatar.png", contentType: "image/png"},
		{name: "malformed", payload: valid[:len(valid)/2], fileName: "avatar.png", contentType: "image/png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, _, service := bindingFixture(test.payload, test.fileName, test.contentType)
			_, err := service.BindUploadedFile(scopedContext(101, 22), filefacade.BindUploadedFileCommand{
				FileID: 77,
				Slot:   filefacade.FileAssetSlotUserAvatar,
			})
			if err == nil {
				t.Fatal("unsafe image must not bind")
			}
			if len(repo.refs) != 0 {
				t.Fatal("unsafe image must not create a reference")
			}
		})
	}
}

func TestBindUploadedFileReplacementDoesNotDeleteAnotherOrganization(t *testing.T) {
	payload := validPNG(t)
	repo, _, service := bindingFixture(payload, "avatar.png", "image/png")
	repo.refs = []domain.FileReference{
		{ID: 1, FileID: 88, UserID: 101, ScopeID: "org:22", BizType: "0", BizID: 101},
		{ID: 2, FileID: 77, UserID: 101, ScopeID: "org:99", BizType: "0", BizID: 101},
	}
	if _, err := service.BindUploadedFile(scopedContext(101, 22), filefacade.BindUploadedFileCommand{
		FileID: 77,
		Slot:   filefacade.FileAssetSlotUserAvatar,
	}); err != nil {
		t.Fatalf("BindUploadedFile() error = %v", err)
	}
	var otherOrgActive bool
	for _, ref := range repo.refs {
		if ref.ID == 2 && ref.ScopeID == "org:99" && ref.IsDeleted == 0 {
			otherOrgActive = true
		}
	}
	if !otherOrgActive {
		t.Fatalf("organization-scoped replacement deleted another organization: %+v", repo.refs)
	}
}

func TestBindConfigAssetDerivesOneServerOwnedSlotAndRejectsCrossScope(t *testing.T) {
	payload := validPNG(t)
	repo, storage, service := bindingFixture(payload, "login-logo.png", "image/png")
	expires := time.Now().UTC().Add(time.Hour)
	repo.files = map[int64]domain.FileInfo{
		78: {
			ID:                78,
			FileInnerName:     "login-logo-next.png",
			FileSize:          int64(len(payload)),
			ContentType:       "image/png",
			StorageStrategyID: 1,
			StoragePath:       "config/next-logo.png",
			Status:            domain.FileStatusAvailable,
			ScanStatus:        domain.ScanStatusClean,
			IntegrityStatus:   domain.IntegrityVerified,
		},
	}
	repo.credentials = map[int64]domain.UploadTask{
		78: {
			ID:                 "task-78",
			UserID:             101,
			ScopeID:            "org:22",
			CredentialID:       "credential-78",
			CredentialVersion:  domain.UploadCredentialVersion1,
			Status:             domain.UploadTaskClean,
			FileID:             78,
			ProtectedUntil:     &expires,
			CredentialExpireAt: &expires,
		},
	}
	storage.objects["config/next-logo.png"] = append([]byte(nil), payload...)

	first := filefacade.BindConfigAssetCommand{
		FileID: 77, ConfigID: 9001, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetPublic,
	}
	if err := service.BindConfigAsset(scopedContext(101, 22), first); err != nil {
		t.Fatalf("bind first config asset: %v", err)
	}
	if len(repo.refs) != 1 {
		t.Fatalf("first config asset created %d references", len(repo.refs))
	}
	ref := repo.refs[0]
	if ref.UserID != 101 || ref.ScopeID != "org:22" || ref.BizType != filefacade.ConfigAssetBizType || ref.BizID != 9001 ||
		ref.VisitURL != filefacade.ConfigAssetStablePath(9001) || ref.AccessScope != string(filefacade.AccessPublic) || ref.VisitStrategy != string(filefacade.VisitPublicStatic) {
		t.Fatalf("CONFIG_ASSET did not derive server-owned operator/slot/policy: %+v", ref)
	}
	if err := service.BindConfigAsset(scopedContext(101, 22), first); err != nil || len(repo.refs) != 1 {
		t.Fatalf("same config slot retry must remain idempotent: err=%v refs=%+v", err, repo.refs)
	}
	if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
		FileID: 78, ConfigID: 9001, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetPublic,
	}); err != nil {
		t.Fatalf("replace config asset: %v", err)
	}
	if len(repo.refs) != 2 || repo.refs[0].IsDeleted != 1 || repo.refs[1].IsDeleted != 0 || repo.refs[1].FileID != 78 {
		t.Fatalf("CONFIG_ASSET replacement did not retain history with one active reference: %+v", repo.refs)
	}

	// A valid upload credential in another organization still cannot replace a
	// configuration reference owned by org:22. configId is global; scope comes
	// only from the authenticated operator context and is never client input.
	crossScopeCredential := repo.credential
	crossScopeCredential.ScopeID = "org:99"
	repo.credentials[77] = crossScopeCredential
	if err := service.BindConfigAsset(scopedContext(101, 99), first); err == nil {
		t.Fatal("cross-organization CONFIG_ASSET replacement unexpectedly succeeded")
	}
	active, _ := repo.FindConfigAssetReference(context.Background(), 9001)
	if active == nil || active.FileID != 78 || active.ScopeID != "org:22" {
		t.Fatalf("cross-scope failure changed active CONFIG_ASSET reference: %+v", active)
	}
}

func TestRestoreConfigAssetBindingUsesPrivateStateWithoutUploadCredential(t *testing.T) {
	payload := validPNG(t)
	repo, storage, service := bindingFixture(payload, "original-logo.png", "image/png")
	expires := time.Now().UTC().Add(time.Hour)
	repo.files = map[int64]domain.FileInfo{
		78: {
			ID: 78, FileInnerName: "replacement-logo.png", FileSize: int64(len(payload)), ContentType: "image/png",
			StorageStrategyID: 1, StoragePath: "config/replacement-logo.png", Status: domain.FileStatusAvailable,
			ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
	}
	repo.credentials = map[int64]domain.UploadTask{
		78: {ID: "task-78", UserID: 101, ScopeID: "org:22", CredentialID: "credential-78", CredentialVersion: domain.UploadCredentialVersion1,
			Status: domain.UploadTaskClean, FileID: 78, ProtectedUntil: &expires, CredentialExpireAt: &expires},
	}
	storage.objects["config/replacement-logo.png"] = append([]byte(nil), payload...)
	command := filefacade.CaptureConfigAssetBindingCommand{ConfigID: 9060, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetPublic}
	if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{FileID: 77, ConfigID: command.ConfigID, AssetType: command.AssetType, Exposure: command.Exposure}); err != nil {
		t.Fatalf("bind original asset: %v", err)
	}
	original, err := service.CaptureConfigAssetBinding(scopedContext(101, 22), command)
	if err != nil || original.State != filefacade.ConfigAssetBindingBound || original.FileID != 77 {
		t.Fatalf("capture original binding: state=%#v err=%v", original, err)
	}
	if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{FileID: 78, ConfigID: command.ConfigID, AssetType: command.AssetType, Exposure: command.Exposure}); err != nil {
		t.Fatalf("bind replacement asset: %v", err)
	}
	replacement, err := service.CaptureConfigAssetBinding(scopedContext(101, 22), command)
	if err != nil || replacement.State != filefacade.ConfigAssetBindingBound || replacement.FileID != 78 {
		t.Fatalf("capture replacement binding: state=%#v err=%v", replacement, err)
	}

	// History recovery is deliberately not an upload-credential reuse. A new
	// same-scope operator can restore only the captured state; the service must
	// not query a normal upload credential for file 77 while doing so.
	repo.credentialLookups = 0
	if err := service.RestoreConfigAssetBinding(scopedContext(202, 22), filefacade.RestoreConfigAssetBindingCommand{
		ConfigID: command.ConfigID, AssetType: command.AssetType, Exposure: command.Exposure, Expected: replacement, Restore: original,
	}); err != nil {
		t.Fatalf("restore original historical asset: %v", err)
	}
	if repo.credentialLookups != 0 {
		t.Fatalf("history restore consulted an upload credential %d times", repo.credentialLookups)
	}
	active, err := repo.FindConfigAssetReference(context.Background(), command.ConfigID)
	if err != nil || active == nil || active.FileID != 77 || active.UserID != 202 || active.ScopeID != "org:22" {
		t.Fatalf("history restore did not derive current operator/scope or restore A: ref=%+v err=%v", active, err)
	}
	if len(repo.refs) != 3 || repo.refs[1].IsDeleted != 1 {
		t.Fatalf("replacement B was not detached before restoring A: refs=%+v", repo.refs)
	}

	// A stale expected state and a cross-scope caller must both fail before a
	// reference mutation; neither may turn the private snapshot into a generic
	// fileId authority.
	if err := service.RestoreConfigAssetBinding(scopedContext(101, 99), filefacade.RestoreConfigAssetBindingCommand{
		ConfigID: command.ConfigID, AssetType: command.AssetType, Exposure: command.Exposure, Expected: original, Restore: replacement,
	}); err == nil {
		t.Fatal("cross-scope historical restore unexpectedly succeeded")
	}
	if err := service.RestoreConfigAssetBinding(scopedContext(202, 22), filefacade.RestoreConfigAssetBindingCommand{
		ConfigID: command.ConfigID, AssetType: command.AssetType, Exposure: command.Exposure, Expected: replacement, Restore: original,
	}); err == nil {
		t.Fatal("stale historical restore unexpectedly succeeded")
	}
	active, _ = repo.FindConfigAssetReference(context.Background(), command.ConfigID)
	if active == nil || active.FileID != 77 {
		t.Fatalf("rejected restore changed active binding: %+v", active)
	}
}

func TestRestoreConfigAssetBindingRestoresPriorExposurePolicy(t *testing.T) {
	payload := validPNG(t)
	repo, _, service := bindingFixture(payload, "policy-logo.png", "image/png")
	configID := int64(9061)
	publicCapture := filefacade.CaptureConfigAssetBindingCommand{
		ConfigID: configID, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetPublic,
	}
	if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
		FileID: 77, ConfigID: configID, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetPublic,
	}); err != nil {
		t.Fatalf("bind public config asset: %v", err)
	}
	prior, err := service.CaptureConfigAssetBinding(scopedContext(101, 22), publicCapture)
	if err != nil || prior.State != filefacade.ConfigAssetBindingBound || prior.Exposure != filefacade.ConfigAssetPublic {
		t.Fatalf("capture public policy: state=%#v err=%v", prior, err)
	}
	if err := service.UpdateConfigAssetPolicy(scopedContext(101, 22), filefacade.UpdateConfigAssetPolicyCommand{
		ConfigID: configID, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetAuthenticated,
	}); err != nil {
		t.Fatalf("update config asset policy: %v", err)
	}
	authenticatedCapture := filefacade.CaptureConfigAssetBindingCommand{
		ConfigID: configID, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetAuthenticated,
	}
	expected, err := service.CaptureConfigAssetBinding(scopedContext(101, 22), authenticatedCapture)
	if err != nil || expected.State != filefacade.ConfigAssetBindingBound || expected.FileID != prior.FileID || expected.Exposure != filefacade.ConfigAssetAuthenticated {
		t.Fatalf("capture authenticated policy: state=%#v err=%v", expected, err)
	}

	repo.credentialLookups = 0
	if err := service.RestoreConfigAssetBinding(scopedContext(202, 22), filefacade.RestoreConfigAssetBindingCommand{
		ConfigID: configID, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetAuthenticated,
		Expected: expected, Restore: prior,
	}); err != nil {
		t.Fatalf("restore public policy from private state: %v", err)
	}
	if repo.credentialLookups != 0 {
		t.Fatalf("policy history restore consulted upload credentials %d times", repo.credentialLookups)
	}
	active, err := repo.FindConfigAssetReference(context.Background(), configID)
	if err != nil || active == nil || active.FileID != 77 || active.AccessScope != string(filefacade.AccessPublic) || active.VisitStrategy != string(filefacade.VisitPublicStatic) || active.UserID != 202 {
		t.Fatalf("policy history restore did not re-derive public reference: ref=%+v err=%v", active, err)
	}
}

func TestBindConfigAssetRejectsActiveContentAndRawFileTypeMismatch(t *testing.T) {
	tests := []struct {
		name        string
		payload     []byte
		fileName    string
		contentType string
		assetType   filefacade.ConfigAssetType
	}{
		{name: "svg-as-image", payload: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), fileName: "logo.svg", contentType: "image/svg+xml", assetType: filefacade.ConfigAssetImage},
		{name: "html-as-text", payload: []byte(`<html><script>alert(1)</script></html>`), fileName: "notice.txt", contentType: "text/plain", assetType: filefacade.ConfigAssetFile},
		{name: "pdf-extension-with-html", payload: []byte(`<html>not a pdf</html>`), fileName: "notice.pdf", contentType: "application/pdf", assetType: filefacade.ConfigAssetFile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, _, service := bindingFixture(test.payload, test.fileName, test.contentType)
			if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
				FileID: 77, ConfigID: 9010, AssetType: test.assetType, Exposure: filefacade.ConfigAssetPublic,
			}); err == nil {
				t.Fatal("unsafe configuration asset unexpectedly bound")
			}
			if len(repo.refs) != 0 {
				t.Fatalf("unsafe configuration asset created references: %+v", repo.refs)
			}
		})
	}
}

func TestOpenConfigAssetKeepsAuthenticatedAndInternalAssetsInTheirBoundScope(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		fileName  string
		mimeType  string
		assetType filefacade.ConfigAssetType
		exposure  filefacade.ConfigAssetExposure
	}{
		{
			name:      "authenticated-image",
			payload:   validPNG(t),
			fileName:  "tenant-logo.png",
			mimeType:  "image/png",
			assetType: filefacade.ConfigAssetImage,
			exposure:  filefacade.ConfigAssetAuthenticated,
		},
		{
			name:      "internal-file",
			payload:   []byte("approved internal notice"),
			fileName:  "notice.txt",
			mimeType:  "text/plain",
			assetType: filefacade.ConfigAssetFile,
			exposure:  filefacade.ConfigAssetInternal,
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, service := bindingFixture(test.payload, test.fileName, test.mimeType)
			configID := int64(9030 + index)
			if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
				FileID: 77, ConfigID: configID, AssetType: test.assetType, Exposure: test.exposure,
			}); err != nil {
				t.Fatalf("bind config asset: %v", err)
			}

			// Knowing a stable config ID must not let an authenticated user from
			// another organization open an AUTHENTICATED or INTERNAL asset.
			if result, err := service.OpenConfigAsset(scopedContext(202, 99), configID); err == nil {
				if result != nil && result.Reader != nil {
					_ = result.Reader.Close()
				}
				t.Fatal("cross-organization config asset read unexpectedly succeeded")
			}
			if result, err := service.OpenConfigAsset(context.Background(), configID); err == nil {
				if result != nil && result.Reader != nil {
					_ = result.Reader.Close()
				}
				t.Fatal("anonymous non-public config asset read unexpectedly succeeded")
			}

			result, err := service.OpenConfigAsset(scopedContext(202, 22), configID)
			if err != nil || result == nil || result.Reader == nil || result.AccessScope != test.exposure {
				t.Fatalf("same-scope config asset read failed: result=%+v err=%v", result, err)
			}
			_ = result.Reader.Close()
		})
	}

	_, _, service := bindingFixture(validPNG(t), "public-logo.png", "image/png")
	if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
		FileID: 77, ConfigID: 9040, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetPublic,
	}); err != nil {
		t.Fatalf("bind public config asset: %v", err)
	}
	result, err := service.OpenConfigAsset(scopedContext(202, 99), 9040)
	if err != nil || result == nil || result.Reader == nil || result.AccessScope != filefacade.ConfigAssetPublic {
		t.Fatalf("public config asset must remain cross-scope readable: result=%+v err=%v", result, err)
	}
	_ = result.Reader.Close()
}

func TestConfigAssetAndGenericBindingsCannotShareFileID(t *testing.T) {
	t.Run("config-asset-first-blocks-public-avatar-alias", func(t *testing.T) {
		repo, _, service := bindingFixture(validPNG(t), "internal-logo.png", "image/png")
		if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
			FileID: 77, ConfigID: 9051, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetInternal,
		}); err != nil {
			t.Fatalf("bind config asset: %v", err)
		}
		if _, err := service.BindUploadedFile(scopedContext(101, 22), filefacade.BindUploadedFileCommand{
			FileID: 77, Slot: filefacade.FileAssetSlotUserAvatar,
		}); err == nil {
			t.Fatal("public avatar alias unexpectedly accepted a CONFIG_ASSET file")
		}
		if len(repo.refs) != 1 || repo.refs[0].BizType != filefacade.ConfigAssetBizType {
			t.Fatalf("failed avatar bind changed config asset references: %+v", repo.refs)
		}
	})

	t.Run("public-avatar-first-blocks-config-asset-alias", func(t *testing.T) {
		repo, _, service := bindingFixture(validPNG(t), "avatar.png", "image/png")
		if _, err := service.BindUploadedFile(scopedContext(101, 22), filefacade.BindUploadedFileCommand{
			FileID: 77, Slot: filefacade.FileAssetSlotUserAvatar,
		}); err != nil {
			t.Fatalf("bind avatar: %v", err)
		}
		if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
			FileID: 77, ConfigID: 9052, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetAuthenticated,
		}); err == nil {
			t.Fatal("CONFIG_ASSET unexpectedly accepted a generic public avatar file")
		}
		if len(repo.refs) != 1 || repo.refs[0].BizType == filefacade.ConfigAssetBizType {
			t.Fatalf("failed config bind changed generic references: %+v", repo.refs)
		}
	})

	t.Run("historical-conflict-cannot-use-generic-download-or-token", func(t *testing.T) {
		repo, _, service := bindingFixture(validPNG(t), "legacy-avatar.png", "image/png")
		avatar, err := service.BindUploadedFile(scopedContext(101, 22), filefacade.BindUploadedFileCommand{
			FileID: 77, Slot: filefacade.FileAssetSlotUserAvatar,
		})
		if err != nil {
			t.Fatalf("bind avatar: %v", err)
		}
		// Simulate a legacy conflict that predates the two-way binding guard.
		repo.refs = append(repo.refs, domain.FileReference{
			ID: 9053, FileID: 77, UserID: 101, ScopeID: "org:22", BizType: filefacade.ConfigAssetBizType, BizID: 9053,
			AccessScope: string(filefacade.AccessOwnerOnly), VisitStrategy: string(filefacade.VisitPrivatePreview),
		})
		if _, err := service.BuildDownloadURL(context.Background(), scopedActor(101, 22), 77); err == nil {
			t.Fatal("generic download token unexpectedly issued for conflicted CONFIG_ASSET file")
		}
		if _, err := service.OpenDownload(context.Background(), Actor{}, 77, ""); err == nil {
			t.Fatal("generic fileId download unexpectedly opened conflicted CONFIG_ASSET file")
		}
		if _, err := service.OpenReferenceDownload(context.Background(), Actor{}, avatar.ID); err == nil {
			t.Fatal("generic avatar reference unexpectedly opened conflicted CONFIG_ASSET file")
		}
	})
}

func TestConfigAssetCannotEscapeThroughGenericDownloadOrReferenceManagement(t *testing.T) {
	repo, _, service := bindingFixture(validPNG(t), "protected-logo.png", "image/png")
	if err := service.BindConfigAsset(scopedContext(101, 22), filefacade.BindConfigAssetCommand{
		FileID: 77, ConfigID: 9020, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetPublic,
	}); err != nil {
		t.Fatalf("bind config asset: %v", err)
	}
	ref, err := repo.FindConfigAssetReference(context.Background(), 9020)
	if err != nil || ref == nil {
		t.Fatalf("load config asset reference: ref=%+v err=%v", ref, err)
	}
	if _, err := service.OpenDownload(context.Background(), Actor{}, 77, ""); err == nil {
		t.Fatal("anonymous generic fileId download accepted CONFIG_ASSET")
	}
	if _, err := service.OpenReferenceDownload(context.Background(), Actor{}, ref.ID); err == nil {
		t.Fatal("generic referenceId download accepted CONFIG_ASSET")
	}
	if _, err := service.UpdateReferenceAccess(context.Background(), ref.ID, string(filefacade.AccessPublic), string(filefacade.VisitPublicStatic), nil); err == nil {
		t.Fatal("generic reference access mutation accepted CONFIG_ASSET")
	}
	items, err := service.ListReferences(context.Background(), 77)
	if err != nil {
		t.Fatalf("list generic file references: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("generic file references exposed CONFIG_ASSET authority: %+v", items)
	}
}

func bindingFixture(payload []byte, fileName, contentType string) (*bindingRepository, *uploadDedupStorage, *Service) {
	expires := time.Now().UTC().Add(time.Hour)
	repo := &bindingRepository{
		file: domain.FileInfo{
			ID:                77,
			FileInnerName:     fileName,
			FileSize:          int64(len(payload)),
			ContentType:       contentType,
			StorageStrategyID: 1,
			StoragePath:       "avatar/101/avatar.png",
			Status:            domain.FileStatusAvailable,
			ScanStatus:        domain.ScanStatusClean,
			IntegrityStatus:   domain.IntegrityVerified,
		},
		credential: domain.UploadTask{
			ID:                 "task-77",
			UserID:             101,
			ScopeID:            "org:22",
			CredentialID:       "credential-77",
			CredentialVersion:  domain.UploadCredentialVersion1,
			Status:             domain.UploadTaskClean,
			FileID:             77,
			ProtectedUntil:     &expires,
			CredentialExpireAt: &expires,
		},
	}
	storage := newUploadDedupStorage()
	storage.objects[repo.file.StoragePath] = append([]byte(nil), payload...)
	service := NewService(nil, repo, nil, storage, uploadDedupTokens{}, nil, nil, config.FileDistributionConfig{}, false)
	return repo, storage, service
}

func validPNG(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		t.Fatalf("encode PNG fixture: %v", err)
	}
	return output.Bytes()
}

func validJPEG(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{G: 255, A: 255})
	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, nil); err != nil {
		t.Fatalf("encode JPEG fixture: %v", err)
	}
	return output.Bytes()
}

func validWebP(t *testing.T) []byte {
	t.Helper()
	payload, err := base64.StdEncoding.DecodeString("UklGRhwAAABXRUJQVlA4TA8AAAAvAUAAAAcQ/Y/+ByKi/wEA")
	if err != nil {
		t.Fatalf("decode WebP fixture: %v", err)
	}
	return payload
}
