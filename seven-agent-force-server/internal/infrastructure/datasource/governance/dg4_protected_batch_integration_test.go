package governance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	fileapp "github.com/CaixyPromise/seven-agent/force/internal/app/file/application"
	filedomain "github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	fileinfra "github.com/CaixyPromise/seven-agent/force/internal/app/file/infrastructure"
	configapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/application"
	configdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	configinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/infrastructure"
	dictapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/application"
	dictdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	dictfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/facade"
	dictinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/infrastructure"
	userapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/application"
	userdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userinfra "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

const (
	dg4ProtectedBatchDialectEnv       = "DG4_PROTECTED_BATCH_DIALECT"
	dg4ProtectedBatchDSNEnv           = "DG4_PROTECTED_BATCH_DSN"
	dg4ProtectedBatchMigrationEnv     = "DG4_PROTECTED_BATCH_MIGRATION"
	dg4ProtectedBindingChannelVersion = int64(20260731130000)
)

// protectedBatchProvider is intentionally small and uses the production
// repositories/application services against a real isolated database. It does
// not carry a compatibility table name or a test-only binding table.
type protectedBatchProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *protectedBatchProvider) Driver() string { return p.driver }

func (p *protectedBatchProvider) Dialect() string { return p.dialect }

func (p *protectedBatchProvider) DB() *sql.DB { return p.db }

func (p *protectedBatchProvider) SQLX() *sqlx.DB { return p.sqlxDB }

func (p *protectedBatchProvider) Transactor() store.Transactor {
	return store.NewSQLXTransactor(p.sqlxDB)
}

func (p *protectedBatchProvider) Configured() bool { return true }

func (p *protectedBatchProvider) Close() error { return p.db.Close() }

// TestDG4ProtectedBatchBindingChannelForwardMigration applies the only DG4.2
// schema change: an in-place widening required by the already-published,
// server-derived upload audit value. It intentionally rejects a destructive
// Down because narrowing can truncate committed audit values.
func TestDG4ProtectedBatchBindingChannelForwardMigration(t *testing.T) {
	if strings.ToLower(strings.TrimSpace(os.Getenv(dg4ProtectedBatchMigrationEnv))) != "apply" {
		t.Skip("set DG4_PROTECTED_BATCH_MIGRATION=apply for the exact isolated database")
	}
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv(dg4ProtectedBatchDialectEnv)))
	dsn := strings.TrimSpace(os.Getenv(dg4ProtectedBatchDSNEnv))
	if dialect == "" || dsn == "" {
		t.Skip("set DG4_PROTECTED_BATCH_DIALECT and DG4_PROTECTED_BATCH_DSN for an exact isolated database")
	}
	if dialect != "mysql" && dialect != "postgres" {
		t.Fatalf("unsupported protected batch dialect %q", dialect)
	}
	driver := "mysql"
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open protected batch database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := AssertConnectedDatabase(ctx, db, dialect); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName(goose.DefaultTablename)
	goose.SetVerbose(false)
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set %s goose dialect: %v", dialect, err)
	}
	versionBefore, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read pre-DG4.2 migration version: %v", err)
	}
	if versionBefore != dg4B3MigrationVersion && versionBefore != dg4ProtectedBindingChannelVersion {
		t.Fatalf("DG4.2 migration starts only from B3 or its completed version, got %d", versionBefore)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	migrationsDir := filepath.Join(root, "migrations", dialect)
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		t.Fatalf("apply DG4.2 upload binding channel expansion: %v", err)
	}
	versionAfter, err := goose.GetDBVersionContext(ctx, db)
	if err != nil || versionAfter != dg4ProtectedBindingChannelVersion {
		t.Fatalf("DG4.2 migration version=%d err=%v, want=%d", versionAfter, err, dg4ProtectedBindingChannelVersion)
	}
	assertBindingChannelCapacity(t, ctx, db, dialect)
	if err := goose.DownContext(ctx, db, migrationsDir); err == nil || !strings.Contains(strings.ToLower(err.Error()), "forward-only") {
		t.Fatalf("DG4.2 destructive Down error=%v, want forward-only rejection", err)
	}
	versionAfterDown, err := goose.GetDBVersionContext(ctx, db)
	if err != nil || versionAfterDown != versionAfter {
		t.Fatalf("rejected DG4.2 Down version=%d err=%v, want=%d", versionAfterDown, err, versionAfter)
	}
}

// TestDG4ProtectedBatchBusinessAcceptance deliberately has no migration mode:
// every table in this protected batch is already lower snake_case. It proves
// that the existing application calls preserve configuration, dictionary, and
// upload/file-reference behavior without creating a compatibility path or a
// configuration-asset binding table.
func TestDG4ProtectedBatchBusinessAcceptance(t *testing.T) {
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv(dg4ProtectedBatchDialectEnv)))
	dsn := strings.TrimSpace(os.Getenv(dg4ProtectedBatchDSNEnv))
	if dialect == "" || dsn == "" {
		t.Skip("set DG4_PROTECTED_BATCH_DIALECT and DG4_PROTECTED_BATCH_DSN for an exact isolated database")
	}
	if dialect != "mysql" && dialect != "postgres" {
		t.Fatalf("unsupported protected batch dialect %q", dialect)
	}
	driver := "mysql"
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open protected batch database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := AssertConnectedDatabase(ctx, db, dialect); err != nil {
		t.Fatal(err)
	}
	provider := &protectedBatchProvider{driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver)}

	baseID := time.Now().UTC().UnixNano()
	actorID := baseID + 1
	configRepository, err := configinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new config repository: %v", err)
	}
	dictRepository, err := dictinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new dictionary repository: %v", err)
	}
	fileRepository, err := fileinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new file repository: %v", err)
	}
	userRepository, err := userinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new user repository: %v", err)
	}

	runProtectedConfigBusinessCalls(t, ctx, provider, configRepository, actorID, baseID)
	runProtectedDictionaryBusinessCalls(t, ctx, provider, dictRepository, actorID, baseID)
	runProtectedFileBusinessCalls(t, ctx, provider, fileRepository, userRepository, actorID, baseID)
	assertProtectedBatchPhysicalContract(t, ctx, db, dialect)
}

func runProtectedConfigBusinessCalls(t *testing.T, ctx context.Context, provider store.Provider, repository *configinfra.Repository, actorID, baseID int64) {
	t.Helper()
	service := configapp.NewService(
		provider.Transactor(),
		repository,
		configinfra.NewCacheStore(nil),
		configdomain.NewService(),
		nil,
		nil,
		nil,
	)
	actor := configapp.Actor{UserID: actorID, IsAdmin: true, Authenticated: true}
	groupCode := fmt.Sprintf("dg4_protected_%d", baseID)
	groupID, err := service.AddConfigGroup(ctx, actor, configfacade.ConfigGroupAddRequest{
		GroupCode: groupCode,
		GroupName: "DG4 Protected Batch",
		Module:    "governance",
	})
	if err != nil {
		t.Fatalf("create configuration group through application service: %v", err)
	}
	configID, err := service.AddConfig(ctx, actor, configfacade.ConfigAddRequest{
		GroupID:       groupID,
		ConfigKey:     "protected.title",
		ConfigValue:   "before",
		ValueType:     "STRING",
		UIWidget:      "INPUT",
		Exposure:      "INTERNAL",
		Sensitivity:   "NORMAL",
		EffectType:    "realtime",
		SchemaVersion: intPtr(configdomain.CurrentScalarSchemaVersion),
	})
	if err != nil {
		t.Fatalf("create configuration through application service: %v", err)
	}
	before, err := service.GetConfigByID(ctx, actor, configID)
	if err != nil || before == nil || before.ConfigValue != "before" {
		t.Fatalf("read pre-existing configuration through application service: item=%#v err=%v", before, err)
	}
	afterValue := "after"
	if err := service.UpdateConfig(ctx, actor, configfacade.ConfigUpdateRequest{
		ID:          configID,
		ConfigValue: &afterValue,
		Version:     &before.Version,
	}); err != nil {
		t.Fatalf("update existing configuration through application service: %v", err)
	}
	after, err := service.GetConfigByID(ctx, actor, configID)
	if err != nil || after == nil || after.ConfigValue != afterValue || after.Version != before.Version+1 {
		t.Fatalf("read updated configuration through application service: item=%#v err=%v", after, err)
	}
	if _, err := service.AddConfig(ctx, actor, configfacade.ConfigAddRequest{
		GroupID:       groupID,
		ConfigKey:     "protected.subtitle",
		ConfigValue:   "new",
		ValueType:     "STRING",
		UIWidget:      "INPUT",
		Exposure:      "INTERNAL",
		Sensitivity:   "NORMAL",
		EffectType:    "realtime",
		SchemaVersion: intPtr(configdomain.CurrentScalarSchemaVersion),
	}); err != nil {
		t.Fatalf("create configuration after update through application service: %v", err)
	}
	for _, requestType := range []reflect.Type{
		reflect.TypeOf(configfacade.ConfigAddRequest{}),
		reflect.TypeOf(configfacade.ConfigUpdateRequest{}),
	} {
		if _, found := requestType.FieldByName("FileID"); found {
			t.Fatalf("%s must not expose a configuration-asset fileId before DC2B", requestType.Name())
		}
	}
}

func runProtectedDictionaryBusinessCalls(t *testing.T, ctx context.Context, provider store.Provider, repository *dictinfra.Repository, actorID, baseID int64) {
	t.Helper()
	service := dictapp.NewService(provider.Transactor(), repository, dictinfra.NewCacheStore(nil), dictdomain.NewService())
	actor := dictapp.Actor{UserID: actorID, IsAdmin: true, Authenticated: true}
	dictCode := fmt.Sprintf("dg4_protected_%d", baseID)
	typeID, err := service.AddDictType(ctx, actor, dictfacade.DictTypeAddRequest{
		DictCode:      dictCode,
		DictName:      "DG4 Protected Batch",
		Module:        "governance",
		ValueType:     "STRING",
		UIWidget:      "SELECT",
		Exposure:      "INTERNAL",
		Sensitivity:   "NORMAL",
		SchemaVersion: intPtr(1),
	})
	if err != nil {
		t.Fatalf("create dictionary type through application service: %v", err)
	}
	itemID, err := service.AddDictItem(ctx, actor, typeID, dictfacade.DictItemAddRequest{ItemValue: "before", ItemLabel: "Before"})
	if err != nil {
		t.Fatalf("create dictionary item through application service: %v", err)
	}
	before, err := service.GetDictItemList(ctx, dictfacade.DictItemQueryRequest{DictTypeID: typeID, Force: true})
	if err != nil || len(before) != 1 || before[0].ID != itemID || before[0].ItemLabel != "Before" {
		t.Fatalf("read pre-existing dictionary item through application service: items=%#v err=%v", before, err)
	}
	updatedLabel := "After"
	if err := service.UpdateDictItem(ctx, actor, dictfacade.DictItemUpdateRequest{ID: itemID, ItemLabel: &updatedLabel, Version: &before[0].Version}); err != nil {
		t.Fatalf("update existing dictionary item through application service: %v", err)
	}
	after, err := service.GetDictItemList(ctx, dictfacade.DictItemQueryRequest{DictTypeID: typeID, Force: true})
	if err != nil || len(after) != 1 || after[0].ItemLabel != updatedLabel || after[0].Version != before[0].Version+1 {
		t.Fatalf("read updated dictionary item through application service: items=%#v err=%v", after, err)
	}
	if _, err := service.AddDictItem(ctx, actor, typeID, dictfacade.DictItemAddRequest{ItemValue: "new", ItemLabel: "New"}); err != nil {
		t.Fatalf("create dictionary item after update through application service: %v", err)
	}
}

func runProtectedFileBusinessCalls(t *testing.T, ctx context.Context, provider store.Provider, repository *fileinfra.Repository, users *userinfra.Repository, actorID, baseID int64) {
	t.Helper()
	strategyID := baseID + 100
	if _, err := repository.InsertStrategy(ctx, &filedomain.StorageStrategy{
		ID:               strategyID,
		StrategyName:     fmt.Sprintf("dg4-protected-%d", baseID),
		ProviderType:     filedomain.ProviderLocal,
		IsDefault:        true,
		IsEnabled:        true,
		RunState:         filedomain.RunStateActive,
		Priority:         1_000_000,
		ConfigCiphertext: "{}",
		ConfigEDEK:       "dg4-protected",
		WrapKeyRef:       "dg4-protected",
	}); err != nil {
		t.Fatalf("create local upload strategy: %v", err)
	}
	storage := &protectedBatchStorage{objects: map[string][]byte{}}
	files := fileapp.NewService(provider.Transactor(), repository, nil, storage, nil, nil, nil, config.FileDistributionConfig{}, false)
	actor := fileapp.Actor{UserID: actorID, ScopeID: "org:22", ScopeSource: "dg4-protected", Authenticated: true}

	// The database deliberately survives test-process restarts while this storage
	// fixture does not. Make the payload unique to this acceptance run so a
	// deduplicated historical file cannot point at an object owned by a prior
	// in-memory fixture.
	first := uploadProtectedPNG(t, ctx, files, actor, baseID+1)
	assertUploadOnlyJSON(t, first.FileID)
	if count := activeReferenceCount(t, ctx, provider.SQLX(), provider.Dialect(), first.FileID); count != 0 {
		t.Fatalf("upload created %d references before a business submission", count)
	}
	uploadCredential, err := repository.FindUploadCredential(ctx, actorID, actor.ScopeID, first.FileID)
	const expectedBindingChannel = "upload-only;scope-source=dg4-protected"
	if err != nil {
		t.Fatalf("read stored upload audit channel: %v", err)
	}
	if uploadCredential == nil {
		t.Fatal("uploaded file must retain a scoped credential before business submission")
	}
	if uploadCredential.BindingChannel != expectedBindingChannel {
		t.Fatalf("stored upload audit channel=%q, want %q", uploadCredential.BindingChannel, expectedBindingChannel)
	}
	cancelled, err := files.InitChunkUpload(ctx, actor, fileapp.ChunkUploadInitRequest{
		FileName:    fmt.Sprintf("cancel-%d.png", baseID),
		FileSize:    256 * 1024,
		ChunkSize:   256 * 1024,
		FileSha256:  fmt.Sprintf("%064x", baseID+4),
		ContentType: "image/png",
	})
	if err != nil || cancelled == nil {
		t.Fatalf("start cancellable chunk upload through application service: result=%#v err=%v", cancelled, err)
	}
	if err := files.AbortChunkUpload(ctx, actor, cancelled.UploadID); err != nil {
		t.Fatalf("cancel chunk upload through application service: %v", err)
	}
	cancelledRecord, err := repository.GetChunkUpload(ctx, cancelled.UploadID)
	if err != nil || cancelledRecord == nil || cancelledRecord.Status != filedomain.ChunkStatusAborted {
		t.Fatalf("cancelled chunk upload state=%#v err=%v, want ABORTED", cancelledRecord, err)
	}
	if err := users.CreateOwnerUser(ctx, &userdomain.OwnerUserRecord{
		UserID: actorID, AccountName: fmt.Sprintf("dg4-%d", actorID), NickName: "DG4 Protected", Status: 0,
	}); err != nil {
		t.Fatalf("create avatar owner fixture: %v", err)
	}
	userService := userapp.NewService(users, userdomain.NewService(), nil, nil, userapp.WithTransactor(provider.Transactor()))
	userService.BindFileAssets(files)
	userCtx := securitycontext.WithUser(ctx, &securitycontext.UserContext{UserID: actorID, PrimaryOrgID: 22, OrgIDs: []int64{22}})
	if _, err := userService.CommitCurrentUserAvatar(userCtx, actorID, first.FileID); err != nil {
		t.Fatalf("business avatar submission must bind first uploaded file atomically: %v", err)
	}
	if count := activeReferenceCount(t, ctx, provider.SQLX(), provider.Dialect(), first.FileID); count != 1 {
		t.Fatalf("first business submission active references=%d, want 1", count)
	}

	second := uploadProtectedPNG(t, ctx, files, actor, baseID+2)
	if count := activeReferenceCount(t, ctx, provider.SQLX(), provider.Dialect(), second.FileID); count != 0 {
		t.Fatalf("replacement upload created %d references before a business submission", count)
	}
	if _, err := userService.CommitCurrentUserAvatar(userCtx, actorID, second.FileID); err != nil {
		t.Fatalf("business avatar submission must replace the active reference atomically: %v", err)
	}
	if count := activeReferenceCount(t, ctx, provider.SQLX(), provider.Dialect(), first.FileID); count != 0 {
		t.Fatalf("replacement left %d active references for the retired file", count)
	}
	if count := activeReferenceCount(t, ctx, provider.SQLX(), provider.Dialect(), second.FileID); count != 1 {
		t.Fatalf("replacement active references=%d, want 1", count)
	}

	assertUnauthorizedBindRejected(t, ctx, provider, files, first.FileID, actorID+1, 22)
	assertUnauthorizedBindRejected(t, ctx, provider, files, first.FileID, actorID, 99)
	textUpload, err := files.Upload(ctx, actor, fileapp.UploadRequest{
		FileName: "unsupported.txt", ContentType: "text/plain", Reader: strings.NewReader("DG4 protected type rejection"), ExpectedSize: int64(len("DG4 protected type rejection")),
	})
	if err != nil {
		t.Fatalf("create ordinary upload for type rejection: %v", err)
	}
	assertBindRejected(t, ctx, provider, files, textUpload.FileID, actorID, 22, "incompatible content type")
	scanUpload := uploadProtectedPNG(t, ctx, files, actor, baseID+3)
	scanFile, err := repository.GetFile(ctx, scanUpload.FileID)
	if err != nil || scanFile == nil {
		t.Fatalf("read uploaded image before scan rejection: file=%#v err=%v", scanFile, err)
	}
	scanFile.ScanStatus = filedomain.ScanStatusPending
	if err := repository.UpdateFile(ctx, scanFile); err != nil {
		t.Fatalf("mark upload pending scan for bind rejection: %v", err)
	}
	assertBindRejected(t, ctx, provider, files, scanUpload.FileID, actorID, 22, "pending scan")
}

func uploadProtectedPNG(t *testing.T, ctx context.Context, files *fileapp.Service, actor fileapp.Actor, seed int64) *fileapp.UploadResult {
	t.Helper()
	payload := protectedPNG(t, seed)
	result, err := files.Upload(ctx, actor, fileapp.UploadRequest{
		FileName:     fmt.Sprintf("protected-%d.png", seed),
		ContentType:  "image/png",
		Reader:       bytes.NewReader(payload),
		ExpectedSize: int64(len(payload)),
	})
	if err != nil || result == nil || result.FileID <= 0 {
		t.Fatalf("upload protected PNG: result=%#v err=%v", result, err)
	}
	return result
}

func assertUploadOnlyJSON(t *testing.T, fileID int64) {
	t.Helper()
	payload, err := json.Marshal(fileapp.UploadResult{FileID: fileID})
	if err != nil {
		t.Fatalf("marshal upload result: %v", err)
	}
	if got, want := string(payload), fmt.Sprintf(`{"fileId":%d}`, fileID); got != want {
		t.Fatalf("terminal upload response=%s, want only %s", got, want)
	}
}

func assertUnauthorizedBindRejected(t *testing.T, ctx context.Context, provider store.Provider, files *fileapp.Service, fileID, userID, orgID int64) {
	t.Helper()
	assertBindRejected(t, ctx, provider, files, fileID, userID, orgID, "credential subject or scope mismatch")
}

func assertBindRejected(t *testing.T, ctx context.Context, provider store.Provider, files *fileapp.Service, fileID, userID, orgID int64, reason string) {
	t.Helper()
	before := activeReferenceCount(t, ctx, provider.SQLX(), provider.Dialect(), fileID)
	bindCtx := securitycontext.WithUser(ctx, &securitycontext.UserContext{UserID: userID, PrimaryOrgID: orgID, OrgIDs: []int64{orgID}})
	if _, err := files.BindUploadedFile(bindCtx, filefacade.BindUploadedFileCommand{FileID: fileID, Slot: filefacade.FileAssetSlotUserAvatar}); err == nil {
		t.Fatalf("bind must reject %s", reason)
	}
	after := activeReferenceCount(t, ctx, provider.SQLX(), provider.Dialect(), fileID)
	if after != before {
		t.Fatalf("rejected %s bind changed active references from %d to %d", reason, before, after)
	}
}

func activeReferenceCount(t *testing.T, ctx context.Context, db *sqlx.DB, dialect string, fileID int64) int64 {
	t.Helper()
	query := `SELECT COUNT(1) FROM sys_file_reference WHERE fileId=? AND isDeleted=0`
	if dialect == "postgres" {
		query = `SELECT COUNT(1) FROM sys_file_reference WHERE "fileId"=$1 AND "isDeleted"=FALSE`
	}
	var count int64
	if err := db.GetContext(ctx, &count, query, fileID); err != nil {
		t.Fatalf("count active file references: %v", err)
	}
	return count
}

func assertProtectedBatchPhysicalContract(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	tables := []string{
		"sys_config", "sys_config_change_log", "sys_config_group", "sys_role_config_scope",
		"sys_dict_item", "sys_dict_type",
		"sys_file_binding_task", "sys_file_chunk_upload", "sys_file_info", "sys_file_integrity_audit",
		"sys_file_process_run", "sys_file_process_task", "sys_file_reference", "sys_storage_alert_log",
		"sys_storage_strategy", "sys_upload_task",
	}
	for _, table := range tables {
		if count := physicalTableCount(t, ctx, db, dialect, table); count != 1 {
			t.Fatalf("protected physical table %s count=%d, want 1", table, count)
		}
	}
	for _, legacy := range []string{"sysConfig", "sysConfigChangeLog", "sysConfigGroup", "sysDictItem", "sysDictType", "sysFileReference", "sysUploadTask"} {
		if count := physicalTableCount(t, ctx, db, dialect, legacy); count != 0 {
			t.Fatalf("retired protected table %s count=%d, want 0", legacy, count)
		}
	}
	if count := physicalTableCount(t, ctx, db, dialect, "sys_config_asset"); count != 0 {
		t.Fatalf("unexpected configuration-asset binding table count=%d", count)
	}
	assertReferenceIndex(t, ctx, db, dialect)
	assertNoProtectedForeignKeys(t, ctx, db, dialect, tables)
}

func physicalTableCount(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string) int {
	t.Helper()
	query := `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	if dialect == "postgres" {
		query = `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`
	}
	var count int
	if err := db.QueryRowContext(ctx, query, table).Scan(&count); err != nil {
		t.Fatalf("count physical table %s: %v", table, err)
	}
	return count
}

func assertReferenceIndex(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	query := `SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='sys_file_reference' AND index_name='idx_file_reference_scope_file'`
	if dialect == "postgres" {
		query = `SELECT COUNT(1) FROM pg_indexes WHERE schemaname='public' AND tablename='sys_file_reference' AND indexname='idx_file_reference_scope_file'`
	}
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		t.Fatalf("inspect sys_file_reference scope/file index: %v", err)
	}
	if count != 1 {
		t.Fatalf("sys_file_reference scope/file index count=%d, want 1", count)
	}
	if dialect == "mysql" {
		var columns string
		if err := db.QueryRowContext(ctx, `SELECT GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='sys_file_reference' AND index_name='idx_file_reference_scope_file'`).Scan(&columns); err != nil {
			t.Fatalf("inspect MySQL sys_file_reference scope/file index columns: %v", err)
		}
		if columns != "scopeId,fileId,isDeleted" {
			t.Fatalf("MySQL sys_file_reference scope/file index columns=%q, want scopeId,fileId,isDeleted", columns)
		}
		return
	}
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname='public' AND tablename='sys_file_reference' AND indexname='idx_file_reference_scope_file'`).Scan(&definition); err != nil {
		t.Fatalf("inspect PostgreSQL sys_file_reference scope/file index definition: %v", err)
	}
	if !strings.Contains(definition, `("scopeId", "fileId", "isDeleted")`) {
		t.Fatalf("PostgreSQL sys_file_reference scope/file index definition=%q, want scopeId,fileId,isDeleted order", definition)
	}
}

func assertBindingChannelCapacity(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	query := `SELECT character_maximum_length FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='sys_upload_task' AND column_name='bindingChannel'`
	if dialect == "postgres" {
		query = `SELECT character_maximum_length FROM information_schema.columns WHERE table_schema='public' AND table_name='sys_upload_task' AND column_name='bindingChannel'`
	}
	var capacity int
	if err := db.QueryRowContext(ctx, query).Scan(&capacity); err != nil {
		t.Fatalf("inspect sys_upload_task.bindingChannel capacity: %v", err)
	}
	if capacity != 64 {
		t.Fatalf("sys_upload_task.bindingChannel capacity=%d, want 64", capacity)
	}
}

func assertNoProtectedForeignKeys(t *testing.T, ctx context.Context, db *sql.DB, dialect string, tables []string) {
	t.Helper()
	if len(tables) == 0 {
		t.Fatal("protected table list is empty")
	}
	placeholders := make([]string, 0, len(tables))
	args := make([]any, 0, len(tables))
	for index, table := range tables {
		if dialect == "postgres" {
			placeholders = append(placeholders, fmt.Sprintf("$%d", index+1))
		} else {
			placeholders = append(placeholders, "?")
		}
		args = append(args, table)
	}
	query := `SELECT COUNT(1) FROM information_schema.table_constraints WHERE table_schema=DATABASE() AND constraint_type='FOREIGN KEY' AND table_name IN (` + strings.Join(placeholders, ",") + `)`
	if dialect == "postgres" {
		query = `SELECT COUNT(1) FROM information_schema.table_constraints WHERE table_schema='public' AND constraint_type='FOREIGN KEY' AND table_name IN (` + strings.Join(placeholders, ",") + `)`
	}
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("inspect protected batch foreign keys: %v", err)
	}
	if count != 0 {
		t.Fatalf("protected batch physical foreign keys=%d, want 0", count)
	}
}

func protectedPNG(t *testing.T, seed int64) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{R: uint8(seed), G: uint8(seed >> 8), B: uint8(seed >> 16), A: 255})
	canvas.Set(1, 1, color.RGBA{R: uint8(seed >> 24), G: uint8(seed >> 32), B: uint8(seed >> 40), A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		t.Fatalf("encode protected PNG: %v", err)
	}
	return output.Bytes()
}

type protectedBatchStorage struct {
	objects map[string][]byte
}

func (s *protectedBatchStorage) Save(_ context.Context, _ filedomain.StorageStrategy, storagePath string, reader io.Reader, contentType string) (filedomain.StoredObject, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return filedomain.StoredObject{}, err
	}
	s.objects[storagePath] = append([]byte(nil), payload...)
	sum := sha256.Sum256(payload)
	return filedomain.StoredObject{StoragePath: storagePath, Size: int64(len(payload)), SHA256: fmt.Sprintf("%x", sum[:]), ContentType: contentType}, nil
}

func (s *protectedBatchStorage) Open(_ context.Context, _ filedomain.StorageStrategy, file filedomain.FileInfo) (filedomain.DownloadObject, error) {
	payload, ok := s.objects[file.StoragePath]
	if !ok {
		return filedomain.DownloadObject{}, fmt.Errorf("object %s is absent", file.StoragePath)
	}
	return filedomain.DownloadObject{File: io.NopCloser(bytes.NewReader(payload)), Size: int64(len(payload)), ContentType: file.ContentType, Name: file.FileInnerName}, nil
}

func (s *protectedBatchStorage) Delete(_ context.Context, _ filedomain.StorageStrategy, storagePath string) error {
	delete(s.objects, storagePath)
	return nil
}

func (s *protectedBatchStorage) PublicURL(_ filedomain.StorageStrategy, storagePath string) string {
	return "/protected/" + storagePath
}

func (s *protectedBatchStorage) PresignPut(context.Context, filedomain.StorageStrategy, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("presigned uploads are not used by protected-batch acceptance")
}

func (s *protectedBatchStorage) Health(context.Context, filedomain.StorageStrategy) error { return nil }

func intPtr(value int) *int { return &value }
