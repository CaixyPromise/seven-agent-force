package infrastructure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	dbgovernance "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestFileAssetRepositoryDatabaseIntegration(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		sqlDriver string
		dialect   string
		postgres  bool
	}{
		{name: "mysql", env: "FILE_ASSET_MYSQL_DSN", sqlDriver: "mysql", dialect: "mysql"},
		{name: "postgres", env: "FILE_ASSET_POSTGRES_DSN", sqlDriver: "pgx", dialect: "postgres", postgres: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.env))
			if dsn == "" {
				t.Skipf("%s is not configured", test.env)
			}
			db, err := sqlx.Open(test.sqlDriver, dsn)
			if err != nil {
				t.Fatalf("open isolated database: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := db.PingContext(ctx); err != nil {
				t.Fatalf("ping isolated database: %v", err)
			}
			if err := dbgovernance.AssertConnectedDatabase(ctx, db.DB, test.dialect); err != nil {
				t.Fatal(err)
			}
			repo := &Repository{db: db, postgres: test.postgres}
			runFileAssetRepositoryIntegration(t, ctx, db, repo)
		})
	}
}

func runFileAssetRepositoryIntegration(t *testing.T, ctx context.Context, db *sqlx.DB, repo *Repository) {
	t.Helper()
	now := time.Now().UTC()
	baseID := now.UnixNano()
	strategyID := baseID
	fileID := baseID + 1
	secondFileID := baseID + 2
	userID := baseID + 3
	bizID := baseID + 4
	expires := now.Add(time.Hour)

	strategy := &domain.StorageStrategy{
		ID:               strategyID,
		StrategyName:     "dc1-" + time.Now().UTC().Format("150405.000000000"),
		ProviderType:     domain.ProviderLocal,
		IsDefault:        false,
		IsEnabled:        true,
		RunState:         domain.RunStateActive,
		ConfigCiphertext: "{}",
		ConfigEDEK:       "dc1",
		WrapKeyRef:       "dc1",
	}
	if _, err := repo.InsertStrategy(ctx, strategy); err != nil {
		t.Fatalf("insert storage strategy through dialect command path: %v", err)
	}
	gotStrategy, err := repo.GetStrategy(ctx, strategyID)
	if err != nil || gotStrategy == nil || !gotStrategy.IsEnabled || gotStrategy.IsDeleted != 0 {
		t.Fatalf("read storage strategy boolean path: strategy=%+v err=%v", gotStrategy, err)
	}

	for _, file := range []*domain.FileInfo{
		{ID: fileID, FileInnerName: "dc1.png", FileSize: 68, FileSha256: fmt.Sprintf("%064x", fileID), ContentType: "image/png", StorageStrategyID: strategyID, StoragePath: "dc1/a.png", Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified},
		{ID: secondFileID, FileInnerName: "dc1-2.png", FileSize: 69, FileSha256: fmt.Sprintf("%064x", secondFileID), ContentType: "image/png", StorageStrategyID: strategyID, StoragePath: "dc1/b.png", Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified},
	} {
		if _, err := repo.InsertFile(ctx, file); err != nil {
			t.Fatalf("insert file %d: %v", file.ID, err)
		}
	}

	historical := &domain.UploadTask{
		ID: "dc1-historical-" + time.Now().UTC().Format("150405.000000000"), UserID: userID, ScopeID: "org:22",
		CredentialID: "dc1-historical-credential-" + time.Now().UTC().Format("150405.000000000"), CredentialVersion: 0,
		ObjectKeyStaging: "dc1/a.png", ObjectKeyClean: "dc1/a.png", Status: domain.UploadTaskClean,
		FileID: fileID, CredentialExpireAt: &expires,
	}
	if err := repo.InsertUploadTask(ctx, historical); err != nil {
		t.Fatalf("insert historical upload task: %v", err)
	}
	if credential, err := repo.FindUploadCredential(ctx, userID, "org:22", fileID); err != nil || credential != nil {
		t.Fatalf("historical version-zero task gained authority: credential=%+v err=%v", credential, err)
	}

	task := &domain.UploadTask{
		ID: "dc1-task-" + time.Now().UTC().Format("150405.000000000"), UserID: userID, ScopeID: "org:22",
		CredentialID: "dc1-credential-" + time.Now().UTC().Format("150405.000000000"), CredentialVersion: domain.UploadCredentialVersion1,
		ObjectKeyStaging: "dc1/a.png", ObjectKeyClean: "dc1/a.png", Status: domain.UploadTaskClean,
		FileID: fileID, ProtectedUntil: &expires, CredentialExpireAt: &expires,
	}
	transactor := store.NewSQLXTransactor(db)
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repo.InsertUploadTask(txCtx, task)
	}); err != nil {
		t.Fatalf("insert credential through transaction/dialect path: %v", err)
	}
	credential, err := repo.FindUploadCredential(ctx, userID, "org:22", fileID)
	if err != nil || credential == nil || !credential.Authorizes(userID, "org:22", fileID, now) {
		t.Fatalf("current credential round trip failed: credential=%+v err=%v", credential, err)
	}

	expiredAt := now.Add(-time.Minute)
	expiredTask := &domain.UploadTask{
		ID: "dc1-expired-" + time.Now().UTC().Format("150405.000000000"), UserID: userID, ScopeID: "org:22",
		ObjectKeyStaging: "dc1/expired.txt", ObjectKeyClean: "dc1/expired.txt",
		Status: domain.UploadTaskInit, ExpireAt: &expiredAt,
	}
	if err := repo.InsertUploadTask(ctx, expiredTask); err != nil {
		t.Fatalf("insert expired upload task: %v", err)
	}
	expiredTasks, err := repo.ListExpiredUploadTasks(ctx, now, 10)
	if err != nil {
		t.Fatalf("list expired upload tasks: %v", err)
	}
	foundExpired := false
	for _, candidate := range expiredTasks {
		if candidate.ID == expiredTask.ID {
			foundExpired = true
		}
		if candidate.Status == domain.UploadTaskClean {
			t.Fatalf("terminal credential appeared in upload expiry candidates: %+v", candidate)
		}
	}
	if !foundExpired {
		t.Fatalf("expired pending upload task was not listed: %+v", expiredTasks)
	}
	if matched, err := repo.UpdateUploadTaskStatusIfMatch(ctx, expiredTask.ID, domain.UploadTaskInit, domain.UploadTaskExpired); err != nil || !matched {
		t.Fatalf("close expired upload task: matched=%t err=%v", matched, err)
	}
	closedTask, err := repo.GetUploadTask(ctx, expiredTask.ID)
	if err != nil || closedTask == nil || closedTask.Status != domain.UploadTaskExpired {
		t.Fatalf("expired upload task did not reach terminal state: task=%+v err=%v", closedTask, err)
	}

	chunk := &domain.ChunkUpload{
		ID: baseID + 5, UploadID: "dc1-chunk-" + time.Now().UTC().Format("150405.000000000"),
		UploadTaskID: task.ID, UserID: userID, ScopeID: "org:22", FileName: "dc1.png",
		ContentType: "image/png", FileSize: 68, ChunkSize: 68, TotalChunks: 1,
		UploadedChunks: []int{}, ChunkSha256Map: map[int]string{}, PartETagsMap: map[int]string{},
		StorageStrategyID: strategyID, Status: domain.ChunkStatusInit, ExpireTime: expires,
	}
	if err := repo.InsertChunkUpload(ctx, chunk); err != nil {
		t.Fatalf("insert scoped chunk task: %v", err)
	}
	gotChunk, err := repo.GetChunkUpload(ctx, chunk.UploadID)
	if err != nil || gotChunk == nil || gotChunk.ScopeID != "org:22" || gotChunk.UploadTaskID != task.ID {
		t.Fatalf("chunk credential linkage round trip failed: chunk=%+v err=%v", gotChunk, err)
	}

	firstRef := &domain.FileReference{
		ID: baseID + 6, FileID: fileID, UserID: userID, ScopeID: "org:22",
		DisplayName: "avatar", BizType: "0", BizID: bizID, VisitStrategy: "PUBLIC_STATIC", AccessScope: "PUBLIC",
	}
	if _, err := repo.InsertReference(ctx, firstRef); err != nil {
		t.Fatalf("insert first active reference: %v", err)
	}
	duplicate := *firstRef
	duplicate.ID = baseID + 7
	duplicate.FileID = secondFileID
	if _, err := repo.InsertReference(ctx, &duplicate); err == nil {
		t.Fatal("active-only business slot uniqueness did not reject a duplicate")
	}
	crossOrg := duplicate
	crossOrg.ID = baseID + 70
	crossOrg.ScopeID = "org:99"
	if _, err := repo.InsertReference(ctx, &crossOrg); err != nil {
		t.Fatalf("organization-scoped active slot rejected another organization: %v", err)
	}
	if err := repo.SoftDeleteReferenceInScope(ctx, fileID, userID, "org:22", "0", bizID); err != nil {
		t.Fatalf("soft delete first reference: %v", err)
	}
	refsAfterScopedDelete, err := repo.ListReferencesByBiz(ctx, userID, "0", bizID)
	if err != nil {
		t.Fatalf("list references after scoped replacement: %v", err)
	}
	otherOrgActive := false
	for _, ref := range refsAfterScopedDelete {
		if ref.ID == crossOrg.ID && ref.ScopeID == "org:99" && ref.IsDeleted == 0 {
			otherOrgActive = true
		}
	}
	if !otherOrgActive {
		t.Fatalf("scoped replacement deleted another organization: %+v", refsAfterScopedDelete)
	}
	duplicate.ID = baseID + 8
	duplicate.ScopeID = "org:22"
	if _, err := repo.InsertReference(ctx, &duplicate); err != nil {
		t.Fatalf("deleted history blocked replacement active reference: %v", err)
	}

	orphanID := baseID + 20
	protectedID := baseID + 21
	referencedID := baseID + 22
	raceID := baseID + 23
	credentialRaceID := baseID + 24
	for offset, id := range []int64{orphanID, protectedID, referencedID, raceID, credentialRaceID} {
		file := &domain.FileInfo{
			ID: id, FileInnerName: fmt.Sprintf("lifecycle-%d.txt", offset), FileSize: int64(100 + offset),
			FileSha256: fmt.Sprintf("%064x", id), ContentType: "text/plain", StorageStrategyID: strategyID,
			StoragePath: fmt.Sprintf("dc1/lifecycle-%d.txt", offset), Status: domain.FileStatusAvailable,
			ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		}
		if _, err := repo.InsertFile(ctx, file); err != nil {
			t.Fatalf("insert lifecycle file %d: %v", id, err)
		}
	}

	protectedTask := &domain.UploadTask{
		ID: "dc1-protected-" + time.Now().UTC().Format("150405.000000000"), UserID: userID, ScopeID: "org:22",
		CredentialID:      "dc1-protected-credential-" + time.Now().UTC().Format("150405.000000000"),
		CredentialVersion: domain.UploadCredentialVersion1, ObjectKeyStaging: "dc1/protected.txt",
		ObjectKeyClean: "dc1/protected.txt", Status: domain.UploadTaskClean, FileID: protectedID,
		ProtectedUntil: &expires, CredentialExpireAt: &expires,
	}
	if err := repo.InsertUploadTask(ctx, protectedTask); err != nil {
		t.Fatalf("insert protected lifecycle credential: %v", err)
	}
	if claimed, err := claimFileWithLifecycleLock(ctx, transactor, repo, protectedID, now); err != nil || claimed {
		t.Fatalf("cleanup claimed protected credential file: claimed=%t err=%v", claimed, err)
	}

	referenced := &domain.FileReference{
		ID: baseID + 24, FileID: referencedID, UserID: userID, ScopeID: "org:22", DisplayName: "shared",
		BizType: "1", BizID: baseID + 25, VisitStrategy: "PRIVATE_PREVIEW", AccessScope: "OWNER_ONLY",
	}
	if _, err := repo.InsertReference(ctx, referenced); err != nil {
		t.Fatalf("insert shared lifecycle reference: %v", err)
	}
	if claimed, err := claimFileWithLifecycleLock(ctx, transactor, repo, referencedID, now); err != nil || claimed {
		t.Fatalf("cleanup claimed referenced file: claimed=%t err=%v", claimed, err)
	}

	if claimed, err := claimFileWithLifecycleLock(ctx, transactor, repo, orphanID, now); err != nil || !claimed {
		t.Fatalf("cleanup did not claim unprotected orphan: claimed=%t err=%v", claimed, err)
	}
	if err := repo.RestoreFileAvailableIfCleaning(ctx, orphanID); err != nil {
		t.Fatalf("restore orphan after simulated storage failure: %v", err)
	}
	if claimed, err := claimFileWithLifecycleLock(ctx, transactor, repo, orphanID, now); err != nil || !claimed {
		t.Fatalf("cleanup did not reclaim restored orphan: claimed=%t err=%v", claimed, err)
	}
	if deleted, err := repo.MarkFileDeletedIfCleaning(ctx, orphanID); err != nil || !deleted {
		t.Fatalf("cleanup did not mark claimed orphan terminal: deleted=%t err=%v", deleted, err)
	}

	locked := make(chan struct{})
	releaseBind := make(chan struct{})
	bindErr := make(chan error, 1)
	go func() {
		bindErr <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			file, err := repo.GetFileForUpdate(txCtx, raceID)
			if err != nil {
				return err
			}
			if file == nil || file.Status != domain.FileStatusAvailable {
				return fmt.Errorf("race file is not bindable: %+v", file)
			}
			close(locked)
			<-releaseBind
			_, err = repo.InsertReference(txCtx, &domain.FileReference{
				ID: baseID + 26, FileID: raceID, UserID: userID, ScopeID: "org:22", DisplayName: "race",
				BizType: "1", BizID: baseID + 27, VisitStrategy: "PRIVATE_PREVIEW", AccessScope: "OWNER_ONLY",
			})
			return err
		})
	}()
	<-locked
	type cleanupResult struct {
		claimed bool
		err     error
	}
	cleanupDone := make(chan cleanupResult, 1)
	go func() {
		claimed, err := claimFileWithLifecycleLock(ctx, transactor, repo, raceID, time.Now().UTC())
		cleanupDone <- cleanupResult{claimed: claimed, err: err}
	}()
	time.Sleep(50 * time.Millisecond)
	close(releaseBind)
	if err := <-bindErr; err != nil {
		t.Fatalf("concurrent reference binding failed: %v", err)
	}
	raceResult := <-cleanupDone
	if raceResult.err != nil || raceResult.claimed {
		t.Fatalf("cleanup won after a valid concurrent bind: claimed=%t err=%v", raceResult.claimed, raceResult.err)
	}
	raceFile, err := repo.GetFile(ctx, raceID)
	if err != nil || raceFile == nil || raceFile.Status != domain.FileStatusAvailable {
		t.Fatalf("concurrent bind did not preserve file availability: file=%+v err=%v", raceFile, err)
	}

	cleanupLocked := make(chan struct{})
	releaseCleanup := make(chan struct{})
	cleanupRaceDone := make(chan cleanupResult, 1)
	go func() {
		var claimed bool
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			file, err := repo.GetFileForUpdate(txCtx, credentialRaceID)
			if err != nil {
				return err
			}
			if file == nil || file.Status != domain.FileStatusAvailable {
				return fmt.Errorf("credential race file is not cleanable: %+v", file)
			}
			close(cleanupLocked)
			<-releaseCleanup
			hasReferences, err := repo.HasActiveReferences(txCtx, credentialRaceID)
			if err != nil {
				return err
			}
			hasCredential, err := repo.HasProtectedCredential(txCtx, credentialRaceID, now)
			if err != nil {
				return err
			}
			if hasReferences || hasCredential {
				return nil
			}
			claimed, err = repo.ClaimFileForCleanup(txCtx, credentialRaceID, now)
			return err
		})
		cleanupRaceDone <- cleanupResult{claimed: claimed, err: err}
	}()
	<-cleanupLocked

	credentialDone := make(chan error, 1)
	go func() {
		credentialDone <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			file, err := repo.GetFileForUpdate(txCtx, credentialRaceID)
			if err != nil {
				return err
			}
			if file == nil || file.Status != domain.FileStatusAvailable {
				return fmt.Errorf("credential completion rejected file status %v", file)
			}
			task := &domain.UploadTask{
				ID: "dc1-race-credential-" + time.Now().UTC().Format("150405.000000000"), UserID: userID, ScopeID: "org:22",
				CredentialID:      "dc1-race-credential-id-" + time.Now().UTC().Format("150405.000000000"),
				CredentialVersion: domain.UploadCredentialVersion1, ObjectKeyStaging: "dc1/race-credential.txt",
				ObjectKeyClean: "dc1/race-credential.txt", Status: domain.UploadTaskClean, FileID: credentialRaceID,
				ProtectedUntil: &expires, CredentialExpireAt: &expires,
			}
			return repo.InsertUploadTask(txCtx, task)
		})
	}()
	time.Sleep(50 * time.Millisecond)
	close(releaseCleanup)
	cleanupRace := <-cleanupRaceDone
	if cleanupRace.err != nil || !cleanupRace.claimed {
		t.Fatalf("cleanup did not win credential completion race: claimed=%t err=%v", cleanupRace.claimed, cleanupRace.err)
	}
	if err := <-credentialDone; err == nil {
		t.Fatal("credential completion succeeded after cleanup claimed the file")
	}
	if protected, err := repo.HasProtectedCredential(ctx, credentialRaceID, now); err != nil || protected {
		t.Fatalf("cleanup race left a protected credential: protected=%t err=%v", protected, err)
	}
}

func claimFileWithLifecycleLock(ctx context.Context, transactor store.Transactor, repo *Repository, fileID int64, now time.Time) (bool, error) {
	var claimed bool
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		file, err := repo.GetFileForUpdate(txCtx, fileID)
		if err != nil || file == nil || file.Status != domain.FileStatusAvailable {
			return err
		}
		hasReferences, err := repo.HasActiveReferences(txCtx, fileID)
		if err != nil {
			return err
		}
		hasCredential, err := repo.HasProtectedCredential(txCtx, fileID, now)
		if err != nil {
			return err
		}
		if hasReferences || hasCredential {
			return nil
		}
		claimed, err = repo.ClaimFileForCleanup(txCtx, fileID, now)
		return err
	})
	return claimed, err
}
