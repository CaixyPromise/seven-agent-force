package infrastructure

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	fileapp "github.com/CaixyPromise/seven-agent/force/internal/app/file/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	configapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/application"
	configdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	configinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	mysqlDriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

// configAssetMigrationPreviousVersion is intentionally the immediately prior
// migration. Newer workspaces include older forward-only migrations whose
// ownership cannot be rolled back generically; DC2B verifies its own Down/Up
// without crossing that unrelated boundary.
const configAssetMigrationPreviousVersion int64 = 20260802100000

// The rollback snapshot migration has a deliberate non-destructive Down. The
// local acceptance proves a Down/Up at this exact boundary preserves private
// recovery evidence rather than silently making history rollback guessy.
const configAssetRollbackSnapshotPreviousVersion int64 = 20260810100000

// rollbackRaceTransactor waits until both competing application calls have
// completed their pre-transaction reads, then releases both real SQLX
// transactions together. This makes the acceptance exercise the actual
// optimistic-version and reference-lock race rather than relying on scheduler
// timing to turn it into a sequential stale-write test.
type rollbackRaceTransactor struct {
	inner   store.Transactor
	arrived chan<- struct{}
	release <-chan struct{}
}

func (t *rollbackRaceTransactor) Enabled() bool {
	return t != nil && t.inner != nil && t.inner.Enabled()
}

func (t *rollbackRaceTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if t == nil || t.inner == nil {
		return errors.New("rollback race transaction boundary is unavailable")
	}
	select {
	case t.arrived <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-t.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return t.inner.WithinTransaction(ctx, fn)
}

func TestLocalFileAssetDatabaseAcceptance(t *testing.T) {
	if strings.TrimSpace(os.Getenv("FILE_ASSET_LOCAL_DB_ACCEPTANCE")) != "1" {
		t.Skip("set FILE_ASSET_LOCAL_DB_ACCEPTANCE=1 to create disposable local databases")
	}
	configDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "configs"))
	if err != nil {
		t.Fatalf("resolve config directory: %v", err)
	}
	migrationRoot, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migration directory: %v", err)
	}
	loaded, err := config.Load(configDir)
	if err != nil {
		t.Fatalf("load local database configuration: %v", err)
	}

	tests := []struct {
		name       string
		dialect    string
		baseline   string
		migrations string
		create     func(*testing.T) (config.DatasourceConfig, func())
	}{
		{
			name:       "mysql",
			dialect:    "mysql",
			migrations: filepath.Join(migrationRoot, "mysql"),
			create: func(t *testing.T) (config.DatasourceConfig, func()) {
				return createDisposableMySQL(t, loaded.Datasource.MySQL.DSN)
			},
		},
		{
			name:       "postgres",
			dialect:    "postgres",
			baseline:   filepath.Join(migrationRoot, "postgres-baseline"),
			migrations: filepath.Join(migrationRoot, "postgres"),
			create:     createDisposablePostgres,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			datasourceConfig, cleanup := test.create(t)
			t.Cleanup(cleanup)
			provider, err := datasource.NewProvider(datasourceConfig, zap.NewNop())
			if err != nil {
				t.Fatalf("open disposable provider: %v", err)
			}
			t.Cleanup(func() { _ = provider.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			goose.SetTableName("goose_db_version")
			if err := goose.SetDialect(test.dialect); err != nil {
				t.Fatalf("set migration dialect: %v", err)
			}
			if test.baseline != "" {
				if err := goose.UpContext(ctx, provider.DB(), test.baseline); err != nil {
					t.Fatalf("apply clean-install baseline to disposable database: %v", err)
				}
			}
			if err := goose.UpContext(ctx, provider.DB(), test.migrations); err != nil {
				t.Fatalf("apply migrations to disposable database: %v", err)
			}
			repo, err := NewRepository(provider)
			if err != nil {
				t.Fatalf("build file repository: %v", err)
			}
			assertReplacementHistorySurvivesDownUp(t, ctx, provider.DB(), repo, test.dialect, test.migrations)
			assertConfigAssetMigrationSurvivesDownUp(t, ctx, provider.DB(), repo, test.dialect, test.migrations)
			runFileAssetRepositoryIntegration(t, ctx, provider.SQLX(), repo)
			runConfigAssetApplicationTransactionAcceptance(t, ctx, provider, repo)
			runConfigAssetGenericReferenceIsolationAcceptance(t, ctx, provider, repo)
			runConfigAssetRollbackAcceptance(t, ctx, provider, repo, test.dialect, test.migrations)
		})
	}
	assertNoDisposableDatabasesRemain(t, loaded.Datasource.MySQL.DSN)
}

func createDisposableMySQL(t *testing.T, configuredDSN string) (config.DatasourceConfig, func()) {
	t.Helper()
	parsed, err := mysqlDriver.ParseDSN(strings.TrimSpace(configuredDSN))
	if err != nil || parsed == nil {
		t.Fatalf("parse configured local MySQL connection")
	}
	parsed.DBName = ""
	admin, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		t.Fatalf("open local MySQL administrator connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		_ = admin.Close()
		t.Fatalf("ping local MySQL administrator connection: %v", err)
	}
	name := fmt.Sprintf("seven_dc1_accept_mysql_%d", time.Now().UTC().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("create disposable MySQL database: %v", err)
	}
	parsed.DBName = name
	testDSN := parsed.FormatDSN()
	cleanup := func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`")
		_ = admin.Close()
	}
	return config.DatasourceConfig{
		Driver: "mysql",
		MySQL:  config.MySQLConfig{Enabled: true, DSN: testDSN},
	}, cleanup
}

func createDisposablePostgres(t *testing.T) (config.DatasourceConfig, func()) {
	t.Helper()
	admin, err := sql.Open("pgx", "dbname=postgres sslmode=disable")
	if err != nil {
		t.Fatalf("open local PostgreSQL administrator connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		_ = admin.Close()
		t.Fatalf("ping local PostgreSQL administrator connection: %v", err)
	}
	name := fmt.Sprintf("seven_dc1_accept_postgres_%d", time.Now().UTC().UnixNano())
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		_ = admin.Close()
		t.Fatalf("create disposable PostgreSQL database: %v", err)
	}
	testDSN := "dbname=" + name + " sslmode=disable"
	cleanup := func() {
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`)
		_ = admin.Close()
	}
	return config.DatasourceConfig{
		Driver:   "postgres",
		Postgres: config.PostgresConfig{Enabled: true, DSN: testDSN},
	}, cleanup
}

func assertReplacementHistorySurvivesDownUp(t *testing.T, ctx context.Context, db *sql.DB, repo *Repository, dialect, migrations string) {
	t.Helper()
	baseID := time.Now().UTC().UnixNano()
	fileID := baseID + 1
	userID := baseID + 2
	bizID := baseID + 3
	if _, err := repo.InsertFile(ctx, &domain.FileInfo{
		ID: fileID, FileInnerName: "migration-history.txt", FileSize: 1, FileSha256: fmt.Sprintf("%064x", fileID),
		ContentType: "text/plain", StorageStrategyID: 1, StoragePath: fmt.Sprintf("migration/%d.txt", fileID),
		Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
	}); err != nil {
		t.Fatalf("insert migration history file: %v", err)
	}
	for index := 0; index < 3; index++ {
		if _, err := repo.InsertReference(ctx, &domain.FileReference{
			ID: baseID + int64(10+index), FileID: fileID, UserID: userID, ScopeID: "org:22",
			DisplayName: fmt.Sprintf("replacement-%d", index), BizType: "0", BizID: bizID,
			VisitStrategy: "PUBLIC_STATIC", AccessScope: "PUBLIC",
		}); err != nil {
			t.Fatalf("insert replacement %d: %v", index, err)
		}
		if index < 2 {
			if err := repo.SoftDeleteReference(ctx, fileID, userID, "0", bizID); err != nil {
				t.Fatalf("soft delete replacement %d: %v", index, err)
			}
		}
	}
	if err := goose.DownToContext(ctx, db, migrations, configAssetMigrationPreviousVersion); err != nil {
		t.Fatalf("rollback file asset migration with replacement history: %v", err)
	}
	assertReferenceHistoryCount(t, ctx, db, dialect, userID, bizID)
	if err := goose.UpContext(ctx, db, migrations); err != nil {
		t.Fatalf("reapply file asset migration after rollback: %v", err)
	}
	assertReferenceHistoryCount(t, ctx, db, dialect, userID, bizID)
}

// assertConfigAssetMigrationSurvivesDownUp proves the additional active-slot
// constraint independent of a binding operator. The generic DC1 uniqueness
// key includes userId/scopeId, so this test uses a second operator and scope to
// demonstrate that CONFIG_ASSET still has one active reference per configId.
func assertConfigAssetMigrationSurvivesDownUp(t *testing.T, ctx context.Context, db *sql.DB, repo *Repository, dialect, migrations string) {
	t.Helper()
	configID := findConfigAssetSeedID(t, ctx, db, dialect, "loginLogo")
	baseID := time.Now().UTC().UnixNano()
	firstFileID := baseID + 101
	secondFileID := baseID + 102
	for _, item := range []domain.FileInfo{
		{
			ID: firstFileID, FileInnerName: "config-asset-first.png", FileSize: 1, FileSha256: fmt.Sprintf("%064x", firstFileID),
			ContentType: "image/png", StorageStrategyID: 1, StoragePath: fmt.Sprintf("config-assets/%d-first.png", configID),
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
		{
			ID: secondFileID, FileInnerName: "config-asset-second.png", FileSize: 1, FileSha256: fmt.Sprintf("%064x", secondFileID),
			ContentType: "image/png", StorageStrategyID: 1, StoragePath: fmt.Sprintf("config-assets/%d-second.png", configID),
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
	} {
		copyItem := item
		if _, err := repo.InsertFile(ctx, &copyItem); err != nil {
			t.Fatalf("insert config asset file %d: %v", item.ID, err)
		}
	}
	first := &domain.FileReference{
		ID: baseID + 103, FileID: firstFileID, UserID: baseID + 104, ScopeID: "org:config-a",
		DisplayName: "config-asset-first.png", BizType: "CONFIG_ASSET", BizID: configID,
		VisitURL: fmt.Sprintf("/api/config-assets/%d", configID), VisitStrategy: "PUBLIC_STATIC", AccessScope: "PUBLIC",
	}
	if _, err := repo.InsertReference(ctx, first); err != nil {
		t.Fatalf("insert first active config asset reference: %v", err)
	}
	crossOperator := *first
	crossOperator.ID = baseID + 105
	crossOperator.FileID = secondFileID
	crossOperator.UserID = baseID + 106
	crossOperator.ScopeID = "org:config-b"
	if _, err := repo.InsertReference(ctx, &crossOperator); err == nil {
		t.Fatal("CONFIG_ASSET uniqueness allowed a second operator/scope active reference for one config")
	}
	assertConcurrentConfigAssetInsertHasOneWinner(t, ctx, db, repo, dialect, baseID)
	if err := repo.SoftDeleteConfigAssetReference(ctx, configID, "org:config-a"); err != nil {
		t.Fatalf("soft delete first config asset reference: %v", err)
	}
	replacement := crossOperator
	replacement.ID = baseID + 107
	replacement.ScopeID = "org:config-a"
	if _, err := repo.InsertReference(ctx, &replacement); err != nil {
		t.Fatalf("insert replacement config asset reference: %v", err)
	}
	active, err := repo.FindConfigAssetReference(ctx, configID)
	if err != nil || active == nil || active.FileID != secondFileID || active.UserID != replacement.UserID || active.ScopeID != replacement.ScopeID {
		t.Fatalf("config asset replacement did not leave one expected active reference: ref=%+v err=%v", active, err)
	}

	if err := goose.DownToContext(ctx, db, migrations, configAssetMigrationPreviousVersion); err != nil {
		t.Fatalf("rollback config asset migration: %v", err)
	}
	assertConfigAssetHistoryCount(t, ctx, db, dialect, configID)
	if err := goose.UpContext(ctx, db, migrations); err != nil {
		t.Fatalf("reapply config asset migration: %v", err)
	}
	assertConfigAssetHistoryCount(t, ctx, db, dialect, configID)
	active, err = repo.FindConfigAssetReference(ctx, configID)
	if err != nil || active == nil || active.FileID != secondFileID {
		t.Fatalf("config asset active reference changed across Down/Up: ref=%+v err=%v", active, err)
	}
}

// assertConcurrentConfigAssetInsertHasOneWinner exercises the database's
// actual unique index under two independent SQL transactions. The contenders
// intentionally use different operators and organization scopes so the older
// DC1 active-slot key cannot be the reason one loses.
func assertConcurrentConfigAssetInsertHasOneWinner(t *testing.T, ctx context.Context, db *sql.DB, repo *Repository, dialect string, baseID int64) {
	t.Helper()
	configID := findConfigAssetSeedID(t, ctx, db, dialect, "favicon")
	firstFileID, secondFileID := baseID+121, baseID+122
	for _, item := range []domain.FileInfo{
		{
			ID: firstFileID, FileInnerName: "concurrent-first.png", FileSize: 1, FileSha256: fmt.Sprintf("%064x", firstFileID),
			ContentType: "image/png", StorageStrategyID: 1, StoragePath: fmt.Sprintf("config-assets/%d-concurrent-first.png", configID),
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
		{
			ID: secondFileID, FileInnerName: "concurrent-second.png", FileSize: 1, FileSha256: fmt.Sprintf("%064x", secondFileID),
			ContentType: "image/png", StorageStrategyID: 1, StoragePath: fmt.Sprintf("config-assets/%d-concurrent-second.png", configID),
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
	} {
		copyItem := item
		if _, err := repo.InsertFile(ctx, &copyItem); err != nil {
			t.Fatalf("insert concurrent config asset file %d: %v", item.ID, err)
		}
	}
	sqlxDB, ok := repo.db.(*sqlx.DB)
	if !ok || sqlxDB == nil {
		t.Fatal("CONFIG_ASSET concurrency acceptance requires SQLX database executor")
	}
	transactor := store.NewSQLXTransactor(sqlxDB)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for index, candidate := range []struct {
		fileID  int64
		userID  int64
		scopeID string
	}{
		{fileID: firstFileID, userID: baseID + 123, scopeID: "org:concurrent-a"},
		{fileID: secondFileID, userID: baseID + 124, scopeID: "org:concurrent-b"},
	} {
		candidate := candidate
		index := index
		go func() {
			<-start
			errs <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				_, err := repo.InsertReference(txCtx, &domain.FileReference{
					ID: baseID + int64(125+index), FileID: candidate.fileID, UserID: candidate.userID, ScopeID: candidate.scopeID,
					DisplayName: "concurrent-config-asset", BizType: filefacade.ConfigAssetBizType, BizID: configID,
					VisitURL: filefacade.ConfigAssetStablePath(configID), VisitStrategy: string(filefacade.VisitPublicStatic), AccessScope: string(filefacade.AccessPublic),
				})
				return err
			})
		}()
	}
	close(start)
	firstErr, secondErr := <-errs, <-errs
	successes := 0
	for _, err := range []error{firstErr, secondErr} {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent CONFIG_ASSET inserts should have exactly one winner: first=%v second=%v", firstErr, secondErr)
	}
	active, err := repo.FindConfigAssetReference(ctx, configID)
	if err != nil || active == nil || active.IsDeleted != 0 {
		t.Fatalf("concurrent CONFIG_ASSET winner is not discoverable: ref=%+v err=%v", active, err)
	}
}

func findConfigAssetSeedID(t *testing.T, ctx context.Context, db *sql.DB, dialect, key string) int64 {
	t.Helper()
	query := `SELECT c.id FROM sys_config c JOIN sys_config_group g ON g.id=c.groupId WHERE g.groupCode=? AND c.configKey=? AND c.valueType='IMAGE' AND c.configValue='' AND c.exposure='PUBLIC' AND c.sensitivity='NORMAL'`
	args := []any{"SEVEN_FRONTEND_METADATA", key}
	if dialect == "postgres" {
		query = `SELECT c.id FROM "sys_config" c JOIN "sys_config_group" g ON g.id=c."groupId" WHERE g."groupCode"=$1 AND c."configKey"=$2 AND c."valueType"='IMAGE' AND c."configValue"='' AND c.exposure='PUBLIC' AND c.sensitivity='NORMAL'`
	}
	var id int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil || id <= 0 {
		t.Fatalf("load canonical config asset seed %q: id=%d err=%v", key, id, err)
	}
	return id
}

func assertConfigAssetHistoryCount(t *testing.T, ctx context.Context, db *sql.DB, dialect string, configID int64) {
	t.Helper()
	query := `SELECT COUNT(*), SUM(CASE WHEN isDeleted=0 THEN 1 ELSE 0 END) FROM sys_file_reference WHERE bizType='CONFIG_ASSET' AND bizId=?`
	args := []any{configID}
	if dialect == "postgres" {
		query = `SELECT COUNT(*), SUM(CASE WHEN "isDeleted"=false THEN 1 ELSE 0 END) FROM "sys_file_reference" WHERE "bizType"='CONFIG_ASSET' AND "bizId"=$1`
	}
	var total, active int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&total, &active); err != nil {
		t.Fatalf("count CONFIG_ASSET history: %v", err)
	}
	if total != 2 || active != 1 {
		t.Fatalf("CONFIG_ASSET history changed across migration: total=%d active=%d", total, active)
	}
}

// runConfigAssetApplicationTransactionAcceptance uses the actual SQLX
// transaction context shared by system-config and sys_file_reference. Its
// narrow test facade deliberately performs real reference writes, then injects
// failures after the versioned config UPDATE to prove the outer transaction
// rolls both records back together.
func runConfigAssetApplicationTransactionAcceptance(t *testing.T, ctx context.Context, provider store.Provider, fileRepo *Repository) {
	t.Helper()
	if provider == nil || fileRepo == nil {
		t.Fatal("config asset transaction acceptance requires repositories")
	}
	configRepo, err := configinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("build config repository: %v", err)
	}
	baseID := time.Now().UTC().UnixNano()
	group := &configdomain.ConfigGroup{
		GroupCode: fmt.Sprintf("DC2B_ASSET_%d", baseID), GroupName: "DC2B asset transaction", Status: 1,
	}
	groupID, err := configRepo.InsertGroup(ctx, group)
	if err != nil {
		t.Fatalf("insert config asset group: %v", err)
	}
	firstFileID, secondFileID, thirdFileID := baseID+201, baseID+202, baseID+203
	for _, item := range []domain.FileInfo{
		{
			ID: firstFileID, FileInnerName: "transaction-first.png", FileSize: 1, FileSha256: fmt.Sprintf("%064x", firstFileID),
			ContentType: "image/png", StorageStrategyID: 1, StoragePath: fmt.Sprintf("transaction/%d-first.png", baseID),
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
		{
			ID: secondFileID, FileInnerName: "transaction-second.png", FileSize: 1, FileSha256: fmt.Sprintf("%064x", secondFileID),
			ContentType: "image/png", StorageStrategyID: 1, StoragePath: fmt.Sprintf("transaction/%d-second.png", baseID),
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
		{
			ID: thirdFileID, FileInnerName: "transaction-third.png", FileSize: 1, FileSha256: fmt.Sprintf("%064x", thirdFileID),
			ContentType: "image/png", StorageStrategyID: 1, StoragePath: fmt.Sprintf("transaction/%d-third.png", baseID),
			Status: domain.FileStatusAvailable, ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
		},
	} {
		copyItem := item
		if _, err := fileRepo.InsertFile(ctx, &copyItem); err != nil {
			t.Fatalf("insert transaction asset file %d: %v", item.ID, err)
		}
	}
	assets := &configAssetTransactionFacade{
		repo: fileRepo, operatorID: baseID + 204, scopeID: "org:dc2b", nextReferenceID: baseID + 300,
	}
	service := configapp.NewService(
		store.NewSQLXTransactor(provider.SQLX()), configRepo, &configAssetNoopCache{}, configdomain.NewService(), nil, nil, nil,
	)
	service.BindConfigAssets(assets)
	actor := configapp.Actor{UserID: assets.operatorID, IsAdmin: true, Authenticated: true, AccountID: assets.operatorID, ScopeID: assets.scopeID}

	configID, err := service.AddConfig(ctx, actor, configfacade.ConfigAddRequest{
		GroupID: groupID, ConfigKey: "loginLogo", ValueType: "IMAGE", Exposure: "PUBLIC", EffectType: "realtime", AssetFileID: &firstFileID,
	})
	if err != nil {
		t.Fatalf("create typed config asset: %v", err)
	}
	created, err := configRepo.FindConfigByID(ctx, configID)
	if err != nil || created == nil || created.ConfigValue != filefacade.ConfigAssetStablePath(configID) || created.Version != 2 {
		t.Fatalf("created config asset did not persist canonical value/version: config=%+v err=%v", created, err)
	}
	assertActiveConfigAssetReference(t, ctx, fileRepo, configID, firstFileID, assets.operatorID, assets.scopeID)

	versionAfterCreate := created.Version
	if err := service.UpdateConfig(ctx, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &versionAfterCreate, AssetFileID: &secondFileID}); err != nil {
		t.Fatalf("replace typed config asset: %v", err)
	}
	replaced, err := configRepo.FindConfigByID(ctx, configID)
	if err != nil || replaced == nil || replaced.ConfigValue != filefacade.ConfigAssetStablePath(configID) || replaced.Version != versionAfterCreate+1 {
		t.Fatalf("replacement did not retain stable value and advance version: config=%+v err=%v", replaced, err)
	}
	assertActiveConfigAssetReference(t, ctx, fileRepo, configID, secondFileID, assets.operatorID, assets.scopeID)

	// A successful independent edit makes the earlier version stale. The stale
	// asset replacement must fail before the facade receives a bind command.
	description := "metadata edit"
	versionBeforeMetadata := replaced.Version
	if err := service.UpdateConfig(ctx, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &versionBeforeMetadata, ConfigDesc: &description}); err != nil {
		t.Fatalf("update config metadata before stale test: %v", err)
	}
	bindsBeforeStale := assets.bindCalls
	if err := service.UpdateConfig(ctx, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &versionBeforeMetadata, AssetFileID: &thirdFileID}); err == nil {
		t.Fatal("stale config asset replacement unexpectedly succeeded")
	}
	if assets.bindCalls != bindsBeforeStale {
		t.Fatalf("stale config version invoked asset bind before conflict detection: before=%d after=%d", bindsBeforeStale, assets.bindCalls)
	}

	beforeFailedBind, err := configRepo.FindConfigByID(ctx, configID)
	if err != nil || beforeFailedBind == nil {
		t.Fatalf("load config before bind rollback: config=%+v err=%v", beforeFailedBind, err)
	}
	assets.failBind = true
	if err := service.UpdateConfig(ctx, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &beforeFailedBind.Version, AssetFileID: &thirdFileID}); err == nil {
		t.Fatal("injected config asset bind failure unexpectedly committed")
	}
	assets.failBind = false
	afterFailedBind, err := configRepo.FindConfigByID(ctx, configID)
	if err != nil || afterFailedBind == nil || afterFailedBind.Version != beforeFailedBind.Version || afterFailedBind.ConfigValue != beforeFailedBind.ConfigValue {
		t.Fatalf("failed asset bind leaked a config mutation outside transaction: before=%+v after=%+v err=%v", beforeFailedBind, afterFailedBind, err)
	}
	assertActiveConfigAssetReference(t, ctx, fileRepo, configID, secondFileID, assets.operatorID, assets.scopeID)

	assets.failClear = true
	if err := service.DeleteConfig(ctx, actor, configID); err == nil {
		t.Fatal("injected config asset clear failure unexpectedly deleted config")
	}
	assets.failClear = false
	afterFailedDelete, err := configRepo.FindConfigByID(ctx, configID)
	if err != nil || afterFailedDelete == nil || afterFailedDelete.IsDeleted != 0 || afterFailedDelete.Version != afterFailedBind.Version {
		t.Fatalf("failed asset clear leaked a config delete outside transaction: config=%+v err=%v", afterFailedDelete, err)
	}
	assertActiveConfigAssetReference(t, ctx, fileRepo, configID, secondFileID, assets.operatorID, assets.scopeID)
}

// runConfigAssetRollbackAcceptance is the DC2B regression that the old
// checkpoint was missing. Unlike the older transaction helper above, it uses
// the real file application facade, real sys_file_reference rows, the real
// private audit payload, and the shared SQLX transaction for every recovery
// assertion on both MySQL and PostgreSQL.
func runConfigAssetRollbackAcceptance(t *testing.T, ctx context.Context, provider store.Provider, fileRepo *Repository, dialect, migrations string) {
	t.Helper()
	if provider == nil || fileRepo == nil {
		t.Fatal("CONFIG_ASSET rollback acceptance requires repositories")
	}
	configRepo, err := configinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("build rollback config repository: %v", err)
	}
	baseID := time.Now().UTC().UnixNano()
	operatorID := baseID + 700
	scopeID := "org:22"
	operatorContext := securitycontext.WithUser(ctx, &securitycontext.UserContext{UserID: operatorID, PrimaryOrgID: 22, OrgIDs: []int64{22}})
	actor := configapp.Actor{UserID: operatorID, IsAdmin: true, Authenticated: true, AccountID: operatorID, ScopeID: scopeID}
	storage := &configAssetAcceptanceStorage{objects: map[string][]byte{}}
	fileService := fileapp.NewService(store.NewSQLXTransactor(provider.SQLX()), fileRepo, nil, storage, nil, nil, nil, config.FileDistributionConfig{}, false)
	newConfigService := func(assets filefacade.ConfigAssetFacade) *configapp.Service {
		service := configapp.NewService(store.NewSQLXTransactor(provider.SQLX()), configRepo, &configAssetNoopCache{}, configdomain.NewService(), nil, nil, nil)
		service.BindConfigAssets(assets)
		return service
	}
	service := newConfigService(fileService)
	groupID, err := configRepo.InsertGroup(operatorContext, &configdomain.ConfigGroup{
		GroupCode: fmt.Sprintf("DC2B_ROLLBACK_%d", baseID), GroupName: "DC2B rollback acceptance", Status: 1,
	})
	if err != nil {
		t.Fatalf("insert rollback group: %v", err)
	}
	payload := configAssetAcceptancePNG(t)
	nextFileID := baseID + 701
	newFile := func() int64 {
		fileID := nextFileID
		nextFileID++
		insertConfigAssetIsolationFileAndCredential(t, operatorContext, fileRepo, storage, operatorID, scopeID, fileID, payload)
		return fileID
	}
	addAssetConfig := func(key string, fileID int64) int64 {
		configID, addErr := service.AddConfig(operatorContext, actor, configfacade.ConfigAddRequest{
			GroupID: groupID, ConfigKey: key, ValueType: "IMAGE", Exposure: "PUBLIC", EffectType: "realtime", AssetFileID: &fileID,
		})
		if addErr != nil {
			t.Fatalf("add asset config %s: %v", key, addErr)
		}
		return configID
	}

	// A -> B -> rollback must restore A and retire B. The stable config value is
	// intentionally unchanged by replacement, so the private audit pair is the
	// only source of the former file identity.
	fileA, fileB := newFile(), newFile()
	configID := addAssetConfig("replaceRollback", fileA)
	created, err := configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || created == nil {
		t.Fatalf("load created rollback config: config=%+v err=%v", created, err)
	}
	version := created.Version
	if err := service.UpdateConfig(operatorContext, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &version, AssetFileID: &fileB}); err != nil {
		t.Fatalf("replace A with B: %v", err)
	}
	replaceLog := latestConfigAssetUpdateLog(t, operatorContext, configRepo, configID, "配置资产替换")
	assertPrivateConfigAssetSnapshotPair(t, replaceLog, fileA, fileB)
	replaced, err := configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || replaced == nil {
		t.Fatalf("load B config: config=%+v err=%v", replaced, err)
	}
	rollbackActor := actor
	rollbackActor.StepUpProof = configAssetRollbackProof(replaceLog.ID)
	if err := service.RollbackConfigChange(operatorContext, rollbackActor, replaceLog.ID, "restore A"); err != nil {
		t.Fatalf("rollback A -> B: %v", err)
	}
	assertActiveConfigAssetReference(t, operatorContext, fileRepo, configID, fileA, operatorID, scopeID)
	assertNoActiveConfigAssetReferenceForFile(t, operatorContext, provider.DB(), dialect, fileB)
	assertConfigChangeRolledBack(t, operatorContext, configRepo, replaceLog.ID)
	assertPrivateConfigAssetSnapshotsAreNotPublic(t, replaceLog)

	// A -> clear -> rollback must re-create the A reference rather than merely
	// putting its stable route back into sys_config.
	current, err := configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || current == nil {
		t.Fatalf("load A after replacement rollback: config=%+v err=%v", current, err)
	}
	clear := true
	version = current.Version
	if err := service.UpdateConfig(operatorContext, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &version, ClearAsset: &clear}); err != nil {
		t.Fatalf("clear A: %v", err)
	}
	clearLog := latestConfigAssetUpdateLog(t, operatorContext, configRepo, configID, "配置资产清除")
	assertPrivateConfigAssetSnapshotPair(t, clearLog, fileA, 0)
	rollbackActor.StepUpProof = configAssetRollbackProof(clearLog.ID)
	if err := service.RollbackConfigChange(operatorContext, rollbackActor, clearLog.ID, "restore cleared A"); err != nil {
		t.Fatalf("rollback A -> clear: %v", err)
	}
	assertActiveConfigAssetReference(t, operatorContext, fileRepo, configID, fileA, operatorID, scopeID)

	// PUBLIC -> AUTHENTICATED policy edits retain the same stable value and file
	// ID, so rollback must recover the private policy snapshots, restore
	// sys_config.exposure, and rewrite the reference policy atomically.
	current, err = configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || current == nil {
		t.Fatalf("load A before policy rollback: config=%+v err=%v", current, err)
	}
	authenticatedExposure := string(configdomain.ConfigExposureAuthenticated)
	version = current.Version
	if err := service.UpdateConfig(operatorContext, actor, configfacade.ConfigUpdateRequest{
		ID: configID, Version: &version, Exposure: &authenticatedExposure,
	}); err != nil {
		t.Fatalf("update A policy public -> authenticated: %v", err)
	}
	policyLog := latestConfigAssetUpdateLog(t, operatorContext, configRepo, configID, "配置资产读取策略更新")
	assertPrivateConfigAssetSnapshotExposures(t, policyLog, filefacade.ConfigAssetPublic, filefacade.ConfigAssetAuthenticated)
	assertConfigAssetReferenceExposure(t, operatorContext, fileRepo, configID, filefacade.ConfigAssetAuthenticated)
	current, err = configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || current == nil || current.Exposure != string(configdomain.ConfigExposureAuthenticated) {
		t.Fatalf("policy update did not persist authenticated exposure: config=%+v err=%v", current, err)
	}
	rollbackActor.StepUpProof = configAssetRollbackProof(policyLog.ID)
	if err := service.RollbackConfigChange(operatorContext, rollbackActor, policyLog.ID, "restore public policy"); err != nil {
		t.Fatalf("rollback public -> authenticated policy: %v", err)
	}
	assertConfigChangeRolledBack(t, operatorContext, configRepo, policyLog.ID)
	assertActiveConfigAssetReference(t, operatorContext, fileRepo, configID, fileA, operatorID, scopeID)
	assertConfigAssetReferenceExposure(t, operatorContext, fileRepo, configID, filefacade.ConfigAssetPublic)
	current, err = configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || current == nil || current.Exposure != string(configdomain.ConfigExposurePublic) {
		t.Fatalf("policy rollback did not restore public exposure: config=%+v err=%v", current, err)
	}

	// A -> B followed by B -> C makes the first history record stale even
	// though all three persisted config values are the same stable path. The
	// facade must compare the expected private B binding and leave config/ref/log
	// state untouched when it sees C.
	fileC := newFile()
	current, err = configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || current == nil {
		t.Fatalf("load A before stale rollback: config=%+v err=%v", current, err)
	}
	version = current.Version
	if err := service.UpdateConfig(operatorContext, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &version, AssetFileID: &fileB}); err != nil {
		t.Fatalf("replace A with B for stale test: %v", err)
	}
	staleLog := latestConfigAssetUpdateLog(t, operatorContext, configRepo, configID, "配置资产替换")
	current, err = configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || current == nil {
		t.Fatalf("load B before C: config=%+v err=%v", current, err)
	}
	version = current.Version
	if err := service.UpdateConfig(operatorContext, actor, configfacade.ConfigUpdateRequest{ID: configID, Version: &version, AssetFileID: &fileC}); err != nil {
		t.Fatalf("replace B with C: %v", err)
	}
	beforeStale, err := configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || beforeStale == nil {
		t.Fatalf("load C before stale rollback: config=%+v err=%v", beforeStale, err)
	}
	rollbackActor.StepUpProof = configAssetRollbackProof(staleLog.ID)
	if err := service.RollbackConfigChange(operatorContext, rollbackActor, staleLog.ID, "stale rollback"); err == nil {
		t.Fatal("stale A -> B rollback unexpectedly succeeded after B -> C")
	}
	afterStale, err := configRepo.FindConfigByID(operatorContext, configID)
	if err != nil || afterStale == nil || afterStale.Version != beforeStale.Version {
		t.Fatalf("stale rollback changed config version: before=%+v after=%+v err=%v", beforeStale, afterStale, err)
	}
	assertActiveConfigAssetReference(t, operatorContext, fileRepo, configID, fileC, operatorID, scopeID)
	assertConfigChangeApplied(t, operatorContext, configRepo, staleLog.ID)

	// Race a rollback of A -> B with a later B -> C replacement after both
	// operations have completed their reads. Exactly one real SQL transaction
	// may win: a failed rollback must leave the later reference and its source
	// log intact, while a winning rollback must leave no partial B/C binding.
	raceA, raceB, raceC := newFile(), newFile(), newFile()
	raceConfigID := addAssetConfig("concurrentRollback", raceA)
	raceCurrent, err := configRepo.FindConfigByID(operatorContext, raceConfigID)
	if err != nil || raceCurrent == nil {
		t.Fatalf("load A before concurrent rollback: config=%+v err=%v", raceCurrent, err)
	}
	version = raceCurrent.Version
	if err := service.UpdateConfig(operatorContext, actor, configfacade.ConfigUpdateRequest{ID: raceConfigID, Version: &version, AssetFileID: &raceB}); err != nil {
		t.Fatalf("replace concurrent A with B: %v", err)
	}
	raceLog := latestConfigAssetUpdateLog(t, operatorContext, configRepo, raceConfigID, "配置资产替换")
	raceCurrent, err = configRepo.FindConfigByID(operatorContext, raceConfigID)
	if err != nil || raceCurrent == nil {
		t.Fatalf("load B before concurrent rollback: config=%+v err=%v", raceCurrent, err)
	}
	racingContext, cancelRace := context.WithTimeout(operatorContext, 10*time.Second)
	defer cancelRace()
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	raceService := configapp.NewService(
		&rollbackRaceTransactor{inner: store.NewSQLXTransactor(provider.SQLX()), arrived: arrived, release: release},
		configRepo,
		&configAssetNoopCache{},
		configdomain.NewService(),
		nil,
		nil,
		nil,
	)
	raceService.BindConfigAssets(fileService)
	type raceOutcome struct {
		name string
		err  error
	}
	outcomes := make(chan raceOutcome, 2)
	startRace := make(chan struct{})
	rollbackRaceActor := actor
	rollbackRaceActor.StepUpProof = configAssetRollbackProof(raceLog.ID)
	mutationVersion := raceCurrent.Version
	go func() {
		<-startRace
		outcomes <- raceOutcome{name: "rollback", err: raceService.RollbackConfigChange(racingContext, rollbackRaceActor, raceLog.ID, "concurrent rollback")}
	}()
	go func() {
		<-startRace
		outcomes <- raceOutcome{name: "replacement", err: raceService.UpdateConfig(racingContext, actor, configfacade.ConfigUpdateRequest{
			ID: raceConfigID, Version: &mutationVersion, AssetFileID: &raceC,
		})}
	}()
	close(startRace)
	for index := 0; index < 2; index++ {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent rollback contenders did not reach their real transaction boundary")
		}
	}
	close(release)
	results := map[string]error{}
	for index := 0; index < 2; index++ {
		select {
		case outcome := <-outcomes:
			results[outcome.name] = outcome.err
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent rollback contenders did not complete")
		}
	}
	successes := 0
	for _, result := range results {
		if result == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent rollback/replacement should have exactly one winner: rollback=%v replacement=%v", results["rollback"], results["replacement"])
	}
	racedAfter, err := configRepo.FindConfigByID(operatorContext, raceConfigID)
	if err != nil || racedAfter == nil || racedAfter.ConfigValue != filefacade.ConfigAssetStablePath(raceConfigID) {
		t.Fatalf("concurrent rollback left invalid config state: config=%+v err=%v", racedAfter, err)
	}
	assertNoActiveConfigAssetReferenceForFile(t, operatorContext, provider.DB(), dialect, raceB)
	children, err := configRepo.ListChangeLogsReferencing(operatorContext, []int64{raceLog.ID})
	if err != nil {
		t.Fatalf("list concurrent rollback audit children: %v", err)
	}
	if results["rollback"] == nil {
		assertActiveConfigAssetReference(t, operatorContext, fileRepo, raceConfigID, raceA, operatorID, scopeID)
		assertConfigChangeRolledBack(t, operatorContext, configRepo, raceLog.ID)
		if len(children) != 1 {
			t.Fatalf("winning rollback did not write exactly one rollback audit child: children=%+v", children)
		}
		t.Logf("concurrent CONFIG_ASSET rollback won; later replacement was rejected: %v", results["replacement"])
	} else {
		assertActiveConfigAssetReference(t, operatorContext, fileRepo, raceConfigID, raceC, operatorID, scopeID)
		assertConfigChangeApplied(t, operatorContext, configRepo, raceLog.ID)
		if len(children) != 0 {
			t.Fatalf("failed rollback wrote audit children: children=%+v", children)
		}
		t.Logf("concurrent later CONFIG_ASSET replacement won; rollback was rejected: %v", results["rollback"])
	}

	// Missing and malformed legacy snapshots must be rejected before any config
	// update, reference mutation, status change, or rollback-log insertion.
	legacyStable := filefacade.ConfigAssetStablePath(configID)
	legacyLog := &configdomain.ConfigChangeLog{ConfigID: configID, ConfigKey: "replaceRollback", OperationType: "UPDATE", OldValue: legacyStable, NewValue: legacyStable, EffectType: "realtime", Status: "applied", OperatorID: operatorID}
	legacyID, err := configRepo.InsertChangeLog(operatorContext, legacyLog)
	if err != nil {
		t.Fatalf("insert legacy missing snapshot log: %v", err)
	}
	assertRejectedLegacyAssetRollback(t, operatorContext, service, rollbackActor, configRepo, fileRepo, configID, fileC, legacyID)
	malformedLog := &configdomain.ConfigChangeLog{ConfigID: configID, ConfigKey: "replaceRollback", OperationType: "UPDATE", OldValue: legacyStable, NewValue: legacyStable, EffectType: "realtime", Status: "applied", OperatorID: operatorID}
	malformedLog.HydratePrivateAssetSnapshotPayloads("not-json", "also-not-json")
	malformedID, err := configRepo.InsertChangeLog(operatorContext, malformedLog)
	if err != nil {
		t.Fatalf("insert malformed snapshot log: %v", err)
	}
	assertRejectedLegacyAssetRollback(t, operatorContext, service, rollbackActor, configRepo, fileRepo, configID, fileC, malformedID)

	// Failure injection is deliberately after the real file facade has removed
	// B and inserted A. Returning an error there verifies the outer config
	// transaction rolls config, reference, original-log status, and new rollback
	// log back together on both engines.
	failureA, failureB := newFile(), newFile()
	failingAssets := &failingRestoreConfigAssetFacade{ConfigAssetFacade: fileService}
	failingService := newConfigService(failingAssets)
	failureConfigID, addErr := failingService.AddConfig(operatorContext, actor, configfacade.ConfigAddRequest{GroupID: groupID, ConfigKey: "failureRollback", ValueType: "IMAGE", Exposure: "PUBLIC", EffectType: "realtime", AssetFileID: &failureA})
	if addErr != nil {
		t.Fatalf("add failure-injection config: %v", addErr)
	}
	failureConfig, err := configRepo.FindConfigByID(operatorContext, failureConfigID)
	if err != nil || failureConfig == nil {
		t.Fatalf("load failure-injection config: config=%+v err=%v", failureConfig, err)
	}
	version = failureConfig.Version
	if err := failingService.UpdateConfig(operatorContext, actor, configfacade.ConfigUpdateRequest{ID: failureConfigID, Version: &version, AssetFileID: &failureB}); err != nil {
		t.Fatalf("replace failure A with B: %v", err)
	}
	failureLog := latestConfigAssetUpdateLog(t, operatorContext, configRepo, failureConfigID, "配置资产替换")
	beforeFailure, err := configRepo.FindConfigByID(operatorContext, failureConfigID)
	if err != nil || beforeFailure == nil {
		t.Fatalf("load B before injected rollback failure: config=%+v err=%v", beforeFailure, err)
	}
	failingAssets.failAfterRestore = true
	rollbackActor.StepUpProof = configAssetRollbackProof(failureLog.ID)
	if err := failingService.RollbackConfigChange(operatorContext, rollbackActor, failureLog.ID, "inject rollback failure"); err == nil {
		t.Fatal("injected post-reference restore failure unexpectedly committed")
	}
	failingAssets.failAfterRestore = false
	afterFailure, err := configRepo.FindConfigByID(operatorContext, failureConfigID)
	if err != nil || afterFailure == nil || afterFailure.Version != beforeFailure.Version {
		t.Fatalf("failed rollback changed config: before=%+v after=%+v err=%v", beforeFailure, afterFailure, err)
	}
	assertActiveConfigAssetReference(t, operatorContext, fileRepo, failureConfigID, failureB, operatorID, scopeID)
	assertConfigChangeApplied(t, operatorContext, configRepo, failureLog.ID)
	children, err = configRepo.ListChangeLogsReferencing(operatorContext, []int64{failureLog.ID})
	if err != nil || len(children) != 0 {
		t.Fatalf("failed rollback left a rollback audit row: children=%+v err=%v", children, err)
	}

	// Finally prove the exact DC2B private snapshot migration can go down/up
	// without erasing the A/B recovery evidence that was just used above.
	if err := goose.DownToContext(operatorContext, provider.DB(), migrations, configAssetRollbackSnapshotPreviousVersion); err != nil {
		t.Fatalf("down private rollback snapshot migration: %v", err)
	}
	if err := goose.UpContext(operatorContext, provider.DB(), migrations); err != nil {
		t.Fatalf("re-up private rollback snapshot migration: %v", err)
	}
	persistedReplaceLog, err := configRepo.FindChangeLogByID(operatorContext, replaceLog.ID)
	if err != nil || persistedReplaceLog == nil {
		t.Fatalf("load replacement log after snapshot migration down/up: log=%+v err=%v", persistedReplaceLog, err)
	}
	assertPrivateConfigAssetSnapshotPair(t, persistedReplaceLog, fileA, fileB)
}

type failingRestoreConfigAssetFacade struct {
	filefacade.ConfigAssetFacade
	failAfterRestore bool
}

func (f *failingRestoreConfigAssetFacade) RestoreConfigAssetBinding(ctx context.Context, command filefacade.RestoreConfigAssetBindingCommand) error {
	if err := f.ConfigAssetFacade.RestoreConfigAssetBinding(ctx, command); err != nil {
		return err
	}
	if f.failAfterRestore {
		return errors.New("injected failure after CONFIG_ASSET historical reference restore")
	}
	return nil
}

func configAssetRollbackProof(logID int64) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction: "CONFIG_ROLLBACK", OperationBinding: "config:rollback:" + strconv.FormatInt(logID, 10),
		ProofIdentifier: "local-db-proof", ChallengeIdentifier: "local-db-challenge", AssuranceLevel: "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
}

func latestConfigAssetUpdateLog(t *testing.T, ctx context.Context, repo configdomain.Repository, configID int64, reason string) *configdomain.ConfigChangeLog {
	t.Helper()
	logs, err := repo.ListHistoryByConfigID(ctx, configID, 100)
	if err != nil {
		t.Fatalf("list config history %d: %v", configID, err)
	}
	var latest *configdomain.ConfigChangeLog
	for index := range logs {
		candidate := logs[index]
		if candidate.OperationType != string(configdomain.ConfigOperationUpdate) || candidate.Status != string(configdomain.ConfigStatusApplied) || candidate.OperationReason != reason {
			continue
		}
		if latest == nil || candidate.ID > latest.ID {
			copyCandidate := candidate
			latest = &copyCandidate
		}
	}
	if latest == nil {
		t.Fatalf("missing applied config asset update history: config=%d reason=%q logs=%+v", configID, reason, logs)
	}
	return latest
}

func assertPrivateConfigAssetSnapshotPair(t *testing.T, log *configdomain.ConfigChangeLog, oldFileID, newFileID int64) {
	t.Helper()
	if log == nil {
		t.Fatal("config asset log is nil")
	}
	oldSnapshot, newSnapshot, err := log.PrivateAssetSnapshots()
	if err != nil || oldSnapshot == nil || newSnapshot == nil || oldSnapshot.FileID != oldFileID || newSnapshot.FileID != newFileID {
		t.Fatalf("private config asset snapshot mismatch: old=%+v new=%+v err=%v", oldSnapshot, newSnapshot, err)
	}
}

func assertPrivateConfigAssetSnapshotExposures(t *testing.T, log *configdomain.ConfigChangeLog, oldExposure, newExposure filefacade.ConfigAssetExposure) {
	t.Helper()
	if log == nil {
		t.Fatal("config asset policy log is nil")
	}
	oldSnapshot, newSnapshot, err := log.PrivateAssetSnapshots()
	if err != nil || oldSnapshot == nil || newSnapshot == nil ||
		oldSnapshot.Exposure != string(oldExposure) || newSnapshot.Exposure != string(newExposure) {
		t.Fatalf("private config asset policy snapshots mismatch: old=%+v new=%+v err=%v", oldSnapshot, newSnapshot, err)
	}
}

func assertPrivateConfigAssetSnapshotsAreNotPublic(t *testing.T, log *configdomain.ConfigChangeLog) {
	t.Helper()
	if log == nil {
		t.Fatal("config asset log is nil")
	}
	payload, err := json.Marshal(log)
	if err != nil || strings.Contains(string(payload), "fileId") || strings.Contains(string(payload), "scopeId") || strings.Contains(string(payload), "oldAssetSnapshot") || strings.Contains(string(payload), "newAssetSnapshot") {
		t.Fatalf("private CONFIG_ASSET snapshot leaked from audit JSON: payload=%s err=%v", payload, err)
	}
}

func assertNoActiveConfigAssetReferenceForFile(t *testing.T, ctx context.Context, db *sql.DB, dialect string, fileID int64) {
	t.Helper()
	query := `SELECT COUNT(*) FROM sys_file_reference WHERE fileId=? AND bizType='CONFIG_ASSET' AND isDeleted=0`
	args := []any{fileID}
	if dialect == "postgres" {
		query = `SELECT COUNT(*) FROM "sys_file_reference" WHERE "fileId"=$1 AND "bizType"='CONFIG_ASSET' AND "isDeleted"=false`
	}
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil || count != 0 {
		t.Fatalf("replacement file still has active CONFIG_ASSET reference: file=%d count=%d err=%v", fileID, count, err)
	}
}

func assertConfigAssetReferenceExposure(t *testing.T, ctx context.Context, repo *Repository, configID int64, exposure filefacade.ConfigAssetExposure) {
	t.Helper()
	ref, err := repo.FindConfigAssetReference(ctx, configID)
	if err != nil || ref == nil {
		t.Fatalf("load CONFIG_ASSET reference policy: ref=%+v err=%v", ref, err)
	}
	accessScope, visitStrategy, accessLevel := configAssetTestPolicy(exposure)
	if ref.AccessScope != accessScope || ref.VisitStrategy != visitStrategy || ref.AccessLevel != accessLevel || ref.VisitURL != filefacade.ConfigAssetStablePath(configID) {
		t.Fatalf("CONFIG_ASSET reference policy mismatch: ref=%+v want exposure=%s", ref, exposure)
	}
}

func assertConfigChangeRolledBack(t *testing.T, ctx context.Context, repo configdomain.Repository, logID int64) {
	t.Helper()
	log, err := repo.FindChangeLogByID(ctx, logID)
	if err != nil || log == nil || log.Status != string(configdomain.ConfigStatusRolledBack) {
		t.Fatalf("config change was not marked rolled_back: log=%+v err=%v", log, err)
	}
}

func assertConfigChangeApplied(t *testing.T, ctx context.Context, repo configdomain.Repository, logID int64) {
	t.Helper()
	log, err := repo.FindChangeLogByID(ctx, logID)
	if err != nil || log == nil || log.Status != string(configdomain.ConfigStatusApplied) {
		t.Fatalf("config change was not left applied: log=%+v err=%v", log, err)
	}
}

func assertRejectedLegacyAssetRollback(t *testing.T, ctx context.Context, service *configapp.Service, actor configapp.Actor, configRepo configdomain.Repository, fileRepo *Repository, configID, activeFileID, logID int64) {
	t.Helper()
	before, err := configRepo.FindConfigByID(ctx, configID)
	if err != nil || before == nil {
		t.Fatalf("load config before legacy rollback: config=%+v err=%v", before, err)
	}
	actor.StepUpProof = configAssetRollbackProof(logID)
	if err := service.RollbackConfigChange(ctx, actor, logID, "legacy audit rollback"); err == nil {
		t.Fatal("legacy CONFIG_ASSET rollback unexpectedly succeeded")
	}
	after, err := configRepo.FindConfigByID(ctx, configID)
	if err != nil || after == nil || after.Version != before.Version {
		t.Fatalf("legacy rollback changed config state: before=%+v after=%+v err=%v", before, after, err)
	}
	assertActiveConfigAssetReference(t, ctx, fileRepo, configID, activeFileID, actor.UserID, actor.ScopeID)
	assertConfigChangeApplied(t, ctx, configRepo, logID)
}

// runConfigAssetGenericReferenceIsolationAcceptance exercises the actual
// MySQL/PostgreSQL repository and transaction lock used by both binders. It
// proves a physical object cannot acquire both a CONFIG_ASSET presentation and
// a generic public presentation, even if both requests race for the same file.
// It also checks that a deliberately injected historical conflict cannot be
// reached through generic fileId, referenceId, or download-token routes.
func runConfigAssetGenericReferenceIsolationAcceptance(t *testing.T, ctx context.Context, provider store.Provider, repo *Repository) {
	t.Helper()
	if provider == nil || repo == nil {
		t.Fatal("CONFIG_ASSET reference isolation acceptance requires repositories")
	}
	baseID := time.Now().UTC().UnixNano()
	operatorID := baseID + 401
	scopeID := "org:22"
	payload := configAssetAcceptancePNG(t)
	storage := &configAssetAcceptanceStorage{objects: map[string][]byte{}}
	service := fileapp.NewService(
		store.NewSQLXTransactor(provider.SQLX()), repo, nil, storage, nil, nil, nil, config.FileDistributionConfig{}, false,
	)
	operatorContext := securitycontext.WithUser(ctx, &securitycontext.UserContext{
		UserID: operatorID, PrimaryOrgID: 22, OrgIDs: []int64{22},
	})
	actor := fileapp.Actor{UserID: operatorID, ScopeID: scopeID, Authenticated: true}

	configFirstFileID := baseID + 402
	insertConfigAssetIsolationFileAndCredential(t, operatorContext, repo, storage, operatorID, scopeID, configFirstFileID, payload)
	if err := service.BindConfigAsset(operatorContext, filefacade.BindConfigAssetCommand{
		FileID: configFirstFileID, ConfigID: baseID + 403, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetInternal,
	}); err != nil {
		t.Fatalf("bind CONFIG_ASSET first: %v", err)
	}
	if _, err := service.BindUploadedFile(operatorContext, filefacade.BindUploadedFileCommand{
		FileID: configFirstFileID, Slot: filefacade.FileAssetSlotUserAvatar,
	}); err == nil {
		t.Fatal("generic avatar binding unexpectedly reused CONFIG_ASSET file")
	}
	assertSingleReferenceType(t, operatorContext, repo, configFirstFileID, filefacade.ConfigAssetBizType)

	avatarFirstFileID := baseID + 404
	insertConfigAssetIsolationFileAndCredential(t, operatorContext, repo, storage, operatorID, scopeID, avatarFirstFileID, payload)
	if _, err := service.BindUploadedFile(operatorContext, filefacade.BindUploadedFileCommand{
		FileID: avatarFirstFileID, Slot: filefacade.FileAssetSlotUserAvatar,
	}); err != nil {
		t.Fatalf("bind avatar first: %v", err)
	}
	if err := service.BindConfigAsset(operatorContext, filefacade.BindConfigAssetCommand{
		FileID: avatarFirstFileID, ConfigID: baseID + 405, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetAuthenticated,
	}); err == nil {
		t.Fatal("CONFIG_ASSET binding unexpectedly reused generic avatar file")
	}
	assertSingleNonConfigReference(t, operatorContext, repo, avatarFirstFileID)

	// A legacy database could contain a conflicting pair from before this
	// release. The generic endpoints must fail closed even though the injected
	// avatar reference would otherwise be PUBLIC.
	legacyFileID := baseID + 406
	legacyConfigID := baseID + 407
	insertConfigAssetIsolationFileAndCredential(t, operatorContext, repo, storage, operatorID, scopeID, legacyFileID, payload)
	if err := service.BindConfigAsset(operatorContext, filefacade.BindConfigAssetCommand{
		FileID: legacyFileID, ConfigID: legacyConfigID, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetInternal,
	}); err != nil {
		t.Fatalf("bind legacy CONFIG_ASSET: %v", err)
	}
	legacyAvatarID := baseID + 408
	legacyAvatarUserID := operatorID + 1
	if _, err := repo.InsertReference(operatorContext, &domain.FileReference{
		ID: legacyAvatarID, FileID: legacyFileID, UserID: legacyAvatarUserID, ScopeID: scopeID,
		DisplayName: "legacy-public-avatar.png", BizType: strconv.Itoa(int(filefacade.UserAvatar)), BizID: legacyAvatarUserID,
		VisitURL: "/file/download?referenceId=" + fmt.Sprintf("%d", legacyAvatarID), AccessLevel: 3,
		VisitStrategy: string(filefacade.VisitPublicStatic), AccessScope: string(filefacade.AccessPublic),
	}); err != nil {
		t.Fatalf("inject historical generic reference: %v", err)
	}
	if _, err := service.BuildDownloadURL(operatorContext, actor, legacyFileID); err == nil {
		t.Fatal("generic download token unexpectedly issued for historical CONFIG_ASSET conflict")
	}
	if _, err := service.OpenDownload(operatorContext, fileapp.Actor{}, legacyFileID, ""); err == nil {
		t.Fatal("generic fileId download unexpectedly opened historical CONFIG_ASSET conflict")
	}
	if _, err := service.OpenReferenceDownload(operatorContext, fileapp.Actor{}, legacyAvatarID); err == nil {
		t.Fatal("generic referenceId download unexpectedly opened historical CONFIG_ASSET conflict")
	}

	// Both binders first lock the same file row. Starting them together must
	// produce exactly one active reference rather than a CONFIG_ASSET/generic
	// alias pair under either database's transaction semantics.
	raceFileID := baseID + 409
	insertConfigAssetIsolationFileAndCredential(t, operatorContext, repo, storage, operatorID, scopeID, raceFileID, payload)
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		errs <- service.BindConfigAsset(operatorContext, filefacade.BindConfigAssetCommand{
			FileID: raceFileID, ConfigID: baseID + 410, AssetType: filefacade.ConfigAssetImage, Exposure: filefacade.ConfigAssetAuthenticated,
		})
	}()
	go func() {
		<-start
		_, err := service.BindUploadedFile(operatorContext, filefacade.BindUploadedFileCommand{
			FileID: raceFileID, Slot: filefacade.FileAssetSlotUserAvatar,
		})
		errs <- err
	}()
	close(start)
	firstErr, secondErr := <-errs, <-errs
	successes := 0
	for _, err := range []error{firstErr, secondErr} {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent CONFIG_ASSET/generic bind should have one winner: first=%v second=%v", firstErr, secondErr)
	}
	refs, err := repo.ListReferencesByFile(operatorContext, raceFileID)
	if err != nil || len(refs) != 1 || refs[0].IsDeleted != 0 {
		t.Fatalf("concurrent bind left unexpected references: refs=%+v err=%v", refs, err)
	}
}

func insertConfigAssetIsolationFileAndCredential(t *testing.T, ctx context.Context, repo *Repository, storage *configAssetAcceptanceStorage, operatorID int64, scopeID string, fileID int64, payload []byte) {
	t.Helper()
	path := fmt.Sprintf("dc2b-reference-isolation/%d.png", fileID)
	if _, err := repo.InsertFile(ctx, &domain.FileInfo{
		ID: fileID, FileInnerName: "reference-isolation.png", FileSize: int64(len(payload)), FileSha256: fmt.Sprintf("%064x", fileID),
		ContentType: "image/png", StoragePath: path, Status: domain.FileStatusAvailable,
		ScanStatus: domain.ScanStatusClean, IntegrityStatus: domain.IntegrityVerified,
	}); err != nil {
		t.Fatalf("insert reference-isolation file %d: %v", fileID, err)
	}
	expires := time.Now().UTC().Add(time.Hour)
	if err := repo.InsertUploadTask(ctx, &domain.UploadTask{
		ID: fmt.Sprintf("dc2b-reference-isolation-%d", fileID), UserID: operatorID, ScopeID: scopeID,
		CredentialID: fmt.Sprintf("credential-%d", fileID), CredentialVersion: domain.UploadCredentialVersion1,
		FileName: "reference-isolation.png", ContentType: "image/png", ObjectKeyStaging: path, ObjectKeyClean: path,
		Status: domain.UploadTaskClean, UploadMode: "single", ExpectedSize: int64(len(payload)), ActualSize: int64(len(payload)),
		FileID: fileID, ProtectedUntil: &expires, CredentialExpireAt: &expires,
	}); err != nil {
		t.Fatalf("insert reference-isolation upload credential %d: %v", fileID, err)
	}
	storage.objects[path] = append([]byte(nil), payload...)
}

func assertSingleReferenceType(t *testing.T, ctx context.Context, repo *Repository, fileID int64, wantBizType string) {
	t.Helper()
	refs, err := repo.ListReferencesByFile(ctx, fileID)
	if err != nil || len(refs) != 1 || refs[0].BizType != wantBizType || refs[0].IsDeleted != 0 {
		t.Fatalf("unexpected active reference after rejected bind: refs=%+v err=%v", refs, err)
	}
}

func assertSingleNonConfigReference(t *testing.T, ctx context.Context, repo *Repository, fileID int64) {
	t.Helper()
	refs, err := repo.ListReferencesByFile(ctx, fileID)
	if err != nil || len(refs) != 1 || refs[0].BizType == filefacade.ConfigAssetBizType || refs[0].IsDeleted != 0 {
		t.Fatalf("unexpected generic reference after rejected config bind: refs=%+v err=%v", refs, err)
	}
}

func configAssetAcceptancePNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	canvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	canvas.Set(0, 0, color.NRGBA{R: 12, G: 34, B: 56, A: 255})
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("encode config asset acceptance image: %v", err)
	}
	return buffer.Bytes()
}

type configAssetAcceptanceStorage struct {
	objects map[string][]byte
}

func (s *configAssetAcceptanceStorage) Save(_ context.Context, _ domain.StorageStrategy, storagePath string, reader io.Reader, contentType string) (domain.StoredObject, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return domain.StoredObject{}, err
	}
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[storagePath] = append([]byte(nil), payload...)
	return domain.StoredObject{StoragePath: storagePath, Size: int64(len(payload)), ContentType: contentType}, nil
}

func (s *configAssetAcceptanceStorage) Open(_ context.Context, _ domain.StorageStrategy, file domain.FileInfo) (domain.DownloadObject, error) {
	payload, found := s.objects[file.StoragePath]
	if !found {
		return domain.DownloadObject{}, fmt.Errorf("acceptance object %q is missing", file.StoragePath)
	}
	return domain.DownloadObject{
		File: io.NopCloser(bytes.NewReader(payload)), Size: int64(len(payload)), ContentType: file.ContentType, Name: file.FileInnerName,
	}, nil
}

func (s *configAssetAcceptanceStorage) Delete(_ context.Context, _ domain.StorageStrategy, storagePath string) error {
	delete(s.objects, storagePath)
	return nil
}

func (*configAssetAcceptanceStorage) PublicURL(_ domain.StorageStrategy, storagePath string) string {
	return "/public/" + strings.TrimLeft(storagePath, "/")
}

func (*configAssetAcceptanceStorage) PresignPut(context.Context, domain.StorageStrategy, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("presigned upload is not used by this acceptance test")
}

func (*configAssetAcceptanceStorage) Health(context.Context, domain.StorageStrategy) error {
	return nil
}

func assertActiveConfigAssetReference(t *testing.T, ctx context.Context, repo *Repository, configID, fileID, operatorID int64, scopeID string) {
	t.Helper()
	ref, err := repo.FindConfigAssetReference(ctx, configID)
	if err != nil || ref == nil || ref.FileID != fileID || ref.UserID != operatorID || ref.ScopeID != scopeID || ref.BizType != filefacade.ConfigAssetBizType || ref.BizID != configID || ref.IsDeleted != 0 {
		t.Fatalf("unexpected active CONFIG_ASSET reference: ref=%+v err=%v", ref, err)
	}
}

type configAssetTransactionFacade struct {
	repo            *Repository
	operatorID      int64
	scopeID         string
	nextReferenceID int64
	bindCalls       int
	failBind        bool
	failClear       bool
	failRestore     bool
}

func (f *configAssetTransactionFacade) BindConfigAsset(ctx context.Context, command filefacade.BindConfigAssetCommand) error {
	f.bindCalls++
	if f.failBind {
		return fmt.Errorf("injected CONFIG_ASSET bind failure")
	}
	if f.repo == nil || command.FileID <= 0 || command.ConfigID <= 0 {
		return fmt.Errorf("invalid CONFIG_ASSET test bind command")
	}
	file, err := f.repo.GetFile(ctx, command.FileID)
	if err != nil || file == nil {
		return fmt.Errorf("load CONFIG_ASSET test file: %w", err)
	}
	if existing, findErr := f.repo.FindConfigAssetReference(ctx, command.ConfigID); findErr != nil {
		return findErr
	} else if existing != nil {
		if err := f.repo.SoftDeleteConfigAssetReference(ctx, command.ConfigID, existing.ScopeID); err != nil {
			return err
		}
	}
	f.nextReferenceID++
	accessScope, visitStrategy, accessLevel := configAssetTestPolicy(command.Exposure)
	_, err = f.repo.InsertReference(ctx, &domain.FileReference{
		ID: f.nextReferenceID, FileID: file.ID, UserID: f.operatorID, ScopeID: f.scopeID,
		DisplayName: file.FileInnerName, BizType: filefacade.ConfigAssetBizType, BizID: command.ConfigID,
		VisitURL: filefacade.ConfigAssetStablePath(command.ConfigID), AccessScope: accessScope, VisitStrategy: visitStrategy, AccessLevel: accessLevel,
	})
	return err
}

func (f *configAssetTransactionFacade) UpdateConfigAssetPolicy(ctx context.Context, command filefacade.UpdateConfigAssetPolicyCommand) error {
	if f.repo == nil {
		return fmt.Errorf("CONFIG_ASSET test repository is unavailable")
	}
	ref, err := f.repo.FindConfigAssetReference(ctx, command.ConfigID)
	if err != nil || ref == nil {
		return err
	}
	ref.AccessScope, ref.VisitStrategy, ref.AccessLevel = configAssetTestPolicy(command.Exposure)
	return f.repo.UpdateConfigAssetReference(ctx, ref)
}

func (f *configAssetTransactionFacade) ClearConfigAsset(ctx context.Context, configID int64) error {
	if f.failClear {
		return fmt.Errorf("injected CONFIG_ASSET clear failure")
	}
	if f.repo == nil {
		return fmt.Errorf("CONFIG_ASSET test repository is unavailable")
	}
	ref, err := f.repo.FindConfigAssetReference(ctx, configID)
	if err != nil || ref == nil {
		return err
	}
	return f.repo.SoftDeleteConfigAssetReference(ctx, configID, ref.ScopeID)
}

func (f *configAssetTransactionFacade) CaptureConfigAssetBinding(ctx context.Context, command filefacade.CaptureConfigAssetBindingCommand) (filefacade.ConfigAssetBindingState, error) {
	if f.repo == nil || command.ConfigID <= 0 {
		return filefacade.ConfigAssetBindingState{}, fmt.Errorf("CONFIG_ASSET test capture is unavailable")
	}
	ref, err := f.repo.FindConfigAssetReference(ctx, command.ConfigID)
	if err != nil {
		return filefacade.ConfigAssetBindingState{}, err
	}
	state := filefacade.ConfigAssetBindingState{
		ConfigID: command.ConfigID, State: filefacade.ConfigAssetBindingEmpty, ScopeID: f.scopeID,
		AssetType: command.AssetType, Exposure: command.Exposure,
	}
	if ref != nil {
		state.State = filefacade.ConfigAssetBindingBound
		state.FileID = ref.FileID
		state.ScopeID = ref.ScopeID
	}
	return state, nil
}

func (f *configAssetTransactionFacade) RestoreConfigAssetBinding(ctx context.Context, command filefacade.RestoreConfigAssetBindingCommand) error {
	if f.failRestore {
		return fmt.Errorf("injected CONFIG_ASSET restore failure")
	}
	if f.repo == nil || command.ConfigID <= 0 {
		return fmt.Errorf("CONFIG_ASSET test restore is unavailable")
	}
	ref, err := f.repo.FindConfigAssetReference(ctx, command.ConfigID)
	if err != nil {
		return err
	}
	actual := filefacade.ConfigAssetBindingState{ConfigID: command.ConfigID, State: filefacade.ConfigAssetBindingEmpty, ScopeID: f.scopeID, AssetType: command.AssetType, Exposure: command.Exposure}
	if ref != nil {
		actual.State, actual.FileID, actual.ScopeID = filefacade.ConfigAssetBindingBound, ref.FileID, ref.ScopeID
	}
	if actual != command.Expected {
		return fmt.Errorf("CONFIG_ASSET test restore expected state mismatch")
	}
	if ref != nil {
		if err := f.repo.SoftDeleteConfigAssetReference(ctx, command.ConfigID, ref.ScopeID); err != nil {
			return err
		}
	}
	if command.Restore.State == filefacade.ConfigAssetBindingEmpty {
		return nil
	}
	file, err := f.repo.GetFile(ctx, command.Restore.FileID)
	if err != nil || file == nil {
		return fmt.Errorf("load CONFIG_ASSET test restore file: %w", err)
	}
	f.nextReferenceID++
	accessScope, visitStrategy, accessLevel := configAssetTestPolicy(command.Restore.Exposure)
	_, err = f.repo.InsertReference(ctx, &domain.FileReference{
		ID: f.nextReferenceID, FileID: file.ID, UserID: f.operatorID, ScopeID: f.scopeID,
		DisplayName: file.FileInnerName, BizType: filefacade.ConfigAssetBizType, BizID: command.ConfigID,
		VisitURL: filefacade.ConfigAssetStablePath(command.ConfigID), AccessScope: accessScope, VisitStrategy: visitStrategy, AccessLevel: accessLevel,
	})
	return err
}

func (f *configAssetTransactionFacade) OpenConfigAsset(context.Context, int64) (*filefacade.ConfigAssetOpenResult, error) {
	return nil, fmt.Errorf("CONFIG_ASSET test facade does not stream objects")
}

func configAssetTestPolicy(exposure filefacade.ConfigAssetExposure) (string, string, int) {
	switch exposure {
	case filefacade.ConfigAssetPublic:
		return string(filefacade.AccessPublic), string(filefacade.VisitPublicStatic), 2
	case filefacade.ConfigAssetAuthenticated:
		return string(filefacade.AccessLoginUsers), string(filefacade.VisitPrivatePreview), 1
	default:
		return string(filefacade.AccessOwnerOnly), string(filefacade.VisitPrivateDownload), 0
	}
}

// configAssetNoopCache keeps the transaction acceptance focused on durable
// database state. Cache governance is intentionally disabled in this local
// harness, so these methods cannot hide a rollback behind a cached response.
type configAssetNoopCache struct{}

func (*configAssetNoopCache) GetConfigByKey(context.Context, string) (*configdomain.Config, bool, error) {
	return nil, false, nil
}
func (*configAssetNoopCache) SetConfigByKey(context.Context, string, *configdomain.Config) error {
	return nil
}
func (*configAssetNoopCache) GetGroupByCode(context.Context, string) (*configdomain.ConfigGroup, bool, error) {
	return nil, false, nil
}
func (*configAssetNoopCache) SetGroupByCode(context.Context, string, *configdomain.ConfigGroup) error {
	return nil
}
func (*configAssetNoopCache) GetListByGroup(context.Context, int64) ([]configdomain.Config, bool, error) {
	return nil, false, nil
}
func (*configAssetNoopCache) SetListByGroup(context.Context, int64, []configdomain.Config) error {
	return nil
}
func (*configAssetNoopCache) GetBatch(context.Context, string) (map[string]configdomain.Config, bool, error) {
	return nil, false, nil
}
func (*configAssetNoopCache) SetBatch(context.Context, string, map[string]configdomain.Config) error {
	return nil
}
func (*configAssetNoopCache) CurrentBatchVersion(context.Context) (int64, error) { return 0, nil }
func (*configAssetNoopCache) BumpBatchVersion(context.Context) error             { return nil }
func (*configAssetNoopCache) InvalidateConfig(context.Context, string) error     { return nil }
func (*configAssetNoopCache) InvalidateGroup(context.Context, string) error      { return nil }
func (*configAssetNoopCache) InvalidateGroupList(context.Context, int64) error   { return nil }
func (*configAssetNoopCache) InvalidateConfigBatch(context.Context, []configdomain.Config) error {
	return nil
}

func assertReferenceHistoryCount(t *testing.T, ctx context.Context, db *sql.DB, dialect string, userID, bizID int64) {
	t.Helper()
	query := `SELECT COUNT(*), SUM(CASE WHEN isDeleted=0 THEN 1 ELSE 0 END) FROM sys_file_reference WHERE userId=? AND bizType='0' AND bizId=?`
	if dialect == "postgres" {
		query = `SELECT COUNT(*), SUM(CASE WHEN "isDeleted"=false THEN 1 ELSE 0 END) FROM "sys_file_reference" WHERE "userId"=$1 AND "bizType"='0' AND "bizId"=$2`
	}
	var total, active int
	if err := db.QueryRowContext(ctx, query, userID, bizID).Scan(&total, &active); err != nil {
		t.Fatalf("count replacement history: %v", err)
	}
	if total != 3 || active != 1 {
		t.Fatalf("replacement history changed across migration: total=%d active=%d", total, active)
	}
}

func assertNoDisposableDatabasesRemain(t *testing.T, configuredMySQLDSN string) {
	t.Helper()
	parsed, err := mysqlDriver.ParseDSN(strings.TrimSpace(configuredMySQLDSN))
	if err != nil || parsed == nil {
		t.Fatalf("parse configured local MySQL connection for cleanup verification")
	}
	parsed.DBName = ""
	mysqlAdmin, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL cleanup verification connection: %v", err)
	}
	defer mysqlAdmin.Close()
	var mysqlCount int
	if err := mysqlAdmin.QueryRow(`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name LIKE 'seven_dc1_accept_%'`).Scan(&mysqlCount); err != nil {
		t.Fatalf("verify MySQL disposable database cleanup: %v", err)
	}
	if mysqlCount != 0 {
		t.Fatalf("MySQL disposable database cleanup left %d databases", mysqlCount)
	}

	postgresAdmin, err := sql.Open("pgx", "dbname=postgres sslmode=disable")
	if err != nil {
		t.Fatalf("open PostgreSQL cleanup verification connection: %v", err)
	}
	defer postgresAdmin.Close()
	var postgresCount int
	if err := postgresAdmin.QueryRow(`SELECT COUNT(*) FROM pg_database WHERE datname LIKE 'seven_dc1_accept_%'`).Scan(&postgresCount); err != nil {
		t.Fatalf("verify PostgreSQL disposable database cleanup: %v", err)
	}
	if postgresCount != 0 {
		t.Fatalf("PostgreSQL disposable database cleanup left %d databases", postgresCount)
	}
}
