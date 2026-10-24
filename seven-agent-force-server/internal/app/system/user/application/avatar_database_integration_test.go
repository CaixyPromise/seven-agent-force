package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	fileapp "github.com/CaixyPromise/seven-agent/force/internal/app/file/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	fileinfra "github.com/CaixyPromise/seven-agent/force/internal/app/file/infrastructure"
	userdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource"
	dbgovernance "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

func TestAvatarBindingDatabaseTransactionIntegration(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		build    func(string) config.DatasourceConfig
		postgres bool
	}{
		{name: "mysql", env: "FILE_ASSET_MYSQL_DSN", build: func(dsn string) config.DatasourceConfig {
			return config.DatasourceConfig{Driver: "mysql", MySQL: config.MySQLConfig{Enabled: true, DSN: dsn}}
		}},
		{name: "postgres", env: "FILE_ASSET_POSTGRES_DSN", postgres: true, build: func(dsn string) config.DatasourceConfig {
			return config.DatasourceConfig{Driver: "postgres", Postgres: config.PostgresConfig{Enabled: true, DSN: dsn}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.env))
			if dsn == "" {
				t.Skipf("%s is not configured", test.env)
			}
			provider, err := datasource.NewProvider(test.build(dsn), zap.NewNop())
			if err != nil {
				t.Fatalf("open isolated provider: %v", err)
			}
			t.Cleanup(func() { _ = provider.Close() })
			dialect := "mysql"
			if test.postgres {
				dialect = "postgres"
			}
			if err := dbgovernance.AssertConnectedDatabase(context.Background(), provider.DB(), dialect); err != nil {
				t.Fatal(err)
			}
			runAvatarTransactionIntegration(t, provider, test.postgres)
		})
	}
}

func runAvatarTransactionIntegration(t *testing.T, provider store.Provider, postgres bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fileRepo, err := fileinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new file repository: %v", err)
	}
	userRepo, err := userinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new user repository: %v", err)
	}
	baseID := time.Now().UTC().UnixNano()
	storage := &avatarIntegrationStorage{objects: map[string][]byte{}}
	fileService := fileapp.NewService(provider.Transactor(), fileRepo, nil, storage, nil, nil, nil, config.FileDistributionConfig{}, false)

	strategyID := baseID
	if _, err := fileRepo.InsertStrategy(ctx, &domain.StorageStrategy{
		ID: strategyID, StrategyName: fmt.Sprintf("avatar-dc1-%d", baseID), ProviderType: domain.ProviderLocal,
		IsEnabled: true, RunState: domain.RunStateActive, ConfigCiphertext: "{}", ConfigEDEK: "dc1", WrapKeyRef: "dc1",
	}); err != nil {
		t.Fatalf("insert avatar storage strategy: %v", err)
	}

	createFixture := func(userID, fileID int64, suffix string) {
		t.Helper()
		payload := avatarPNG(t, fileID)
		path := fmt.Sprintf("avatar/%d/%s.png", userID, suffix)
		sum := sha256.Sum256(payload)
		file := &domain.FileInfo{
			ID: fileID, FileInnerName: suffix + ".png", FileSize: int64(len(payload)), FileSha256: fmt.Sprintf("%x", sum[:]),
			ContentType: "image/png", StorageStrategyID: strategyID, StoragePath: path,
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		}
		if _, err := fileRepo.InsertFile(ctx, file); err != nil {
			t.Fatalf("insert avatar file: %v", err)
		}
		expires := time.Now().UTC().Add(time.Hour)
		if err := fileRepo.InsertUploadTask(ctx, &domain.UploadTask{
			ID: fmt.Sprintf("avatar-task-%d", fileID), UserID: userID, ScopeID: "org:22",
			CredentialID: fmt.Sprintf("avatar-credential-%d", fileID), CredentialVersion: domain.UploadCredentialVersion1,
			ObjectKeyStaging: path, ObjectKeyClean: path, Status: domain.UploadTaskClean, FileID: fileID,
			ProtectedUntil: &expires, CredentialExpireAt: &expires,
		}); err != nil {
			t.Fatalf("insert avatar credential: %v", err)
		}
		if err := userRepo.CreateOwnerUser(ctx, &userdomain.OwnerUserRecord{
			UserID: userID, AccountName: fmt.Sprintf("dc1-user-%d", userID), NickName: "DC1 User", Status: 0,
		}); err != nil {
			t.Fatalf("insert avatar user: %v", err)
		}
		storage.objects[path] = append([]byte(nil), payload...)
	}

	successUserID := baseID + 1
	successFileID := baseID + 2
	createFixture(successUserID, successFileID, "success")
	userService := NewService(userRepo, userdomain.NewService(), nil, nil, WithTransactor(provider.Transactor()))
	userService.BindFileAssets(fileService)
	successCtx := securitycontext.WithUser(ctx, &securitycontext.UserContext{UserID: successUserID, PrimaryOrgID: 22, OrgIDs: []int64{22}})
	avatar, err := userService.CommitCurrentUserAvatar(successCtx, successUserID, successFileID)
	if err != nil {
		t.Fatalf("commit avatar transaction: %v", err)
	}
	if _, err := userService.CommitCurrentUserAvatar(successCtx, successUserID, successFileID); err != nil {
		t.Fatalf("idempotent avatar retry: %v", err)
	}
	assertAvatarDatabaseState(t, ctx, provider.SQLX(), postgres, successUserID, successFileID, avatar, 1)

	rollbackUserID := baseID + 3
	rollbackFileID := baseID + 4
	createFixture(rollbackUserID, rollbackFileID, "rollback")
	failingRepo := &failingAvatarUserRepository{Repository: userRepo}
	rollbackService := NewService(failingRepo, userdomain.NewService(), nil, nil, WithTransactor(provider.Transactor()))
	rollbackService.BindFileAssets(fileService)
	rollbackCtx := securitycontext.WithUser(ctx, &securitycontext.UserContext{UserID: rollbackUserID, PrimaryOrgID: 22, OrgIDs: []int64{22}})
	if _, err := rollbackService.CommitCurrentUserAvatar(rollbackCtx, rollbackUserID, rollbackFileID); err == nil {
		t.Fatal("injected user update failure must roll back file binding")
	}
	assertAvatarDatabaseState(t, ctx, provider.SQLX(), postgres, rollbackUserID, rollbackFileID, "", 0)
}

type failingAvatarUserRepository struct {
	userdomain.Repository
}

func (r *failingAvatarUserRepository) UpdateProfile(context.Context, int64, *string, *string, *string, *string) error {
	return fmt.Errorf("injected avatar persistence failure")
}

type avatarIntegrationStorage struct {
	objects map[string][]byte
}

func (s *avatarIntegrationStorage) Save(_ context.Context, _ domain.StorageStrategy, storagePath string, reader io.Reader, contentType string) (domain.StoredObject, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return domain.StoredObject{}, err
	}
	s.objects[storagePath] = payload
	return domain.StoredObject{StoragePath: storagePath, Size: int64(len(payload)), ContentType: contentType}, nil
}

func (s *avatarIntegrationStorage) Open(_ context.Context, _ domain.StorageStrategy, file domain.FileInfo) (domain.DownloadObject, error) {
	payload, ok := s.objects[file.StoragePath]
	if !ok {
		return domain.DownloadObject{}, fmt.Errorf("object not found")
	}
	return domain.DownloadObject{File: io.NopCloser(bytes.NewReader(payload)), Size: int64(len(payload)), ContentType: file.ContentType, Name: file.FileInnerName}, nil
}

func (s *avatarIntegrationStorage) Delete(_ context.Context, _ domain.StorageStrategy, storagePath string) error {
	delete(s.objects, storagePath)
	return nil
}

func (s *avatarIntegrationStorage) PublicURL(_ domain.StorageStrategy, storagePath string) string {
	return "/public/" + storagePath
}

func (s *avatarIntegrationStorage) PresignPut(context.Context, domain.StorageStrategy, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("not supported")
}

func (s *avatarIntegrationStorage) Health(context.Context, domain.StorageStrategy) error { return nil }

func assertAvatarDatabaseState(t *testing.T, ctx context.Context, db *sqlx.DB, postgres bool, userID, fileID int64, expectedAvatar string, expectedReferences int64) {
	t.Helper()
	column := "userAvatar"
	fileColumn := "fileId"
	deletedColumn := "isDeleted"
	if postgres {
		column = `"userAvatar"`
		fileColumn = `"fileId"`
		deletedColumn = `"isDeleted"`
	}
	var avatar sql.NullString
	if err := db.GetContext(ctx, &avatar, db.Rebind(`SELECT `+column+` FROM sys_user WHERE id=?`), userID); err != nil {
		t.Fatalf("query persisted avatar: %v", err)
	}
	value := ""
	if avatar.Valid {
		value = avatar.String
	}
	if value != expectedAvatar {
		t.Fatalf("persisted avatar = %q, want %q", value, expectedAvatar)
	}
	var referenceCount int64
	if err := db.GetContext(ctx, &referenceCount, db.Rebind(`SELECT COUNT(1) FROM sys_file_reference WHERE `+fileColumn+`=? AND `+deletedColumn+`=`+databaseFalse(postgres)), fileID); err != nil {
		t.Fatalf("query avatar references: %v", err)
	}
	if referenceCount != expectedReferences {
		t.Fatalf("active avatar references = %d, want %d", referenceCount, expectedReferences)
	}
}

func databaseFalse(postgres bool) string {
	if postgres {
		return "FALSE"
	}
	return "0"
}

func avatarPNG(t *testing.T, seed int64) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{R: uint8(seed), G: uint8(seed >> 8), B: uint8(seed >> 16), A: 255})
	var payload bytes.Buffer
	if err := png.Encode(&payload, canvas); err != nil {
		t.Fatalf("encode avatar PNG: %v", err)
	}
	return payload.Bytes()
}
