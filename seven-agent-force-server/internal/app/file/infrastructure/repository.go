package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

// configAssetBizType is a persisted storage discriminator. Keep it local to
// the repository so the infrastructure implementation does not depend on the
// outward-facing facade package merely to build SQL predicates.
const configAssetBizType = "CONFIG_ASSET"

type Repository struct {
	db       store.SQLX
	postgres bool
}

type dialectExecutor struct {
	store.SQLX
	postgres bool
}

func (e dialectExecutor) Rebind(query string) string {
	return e.SQLX.Rebind(prepareRepositoryQuery(query, e.postgres))
}

type dbFlag struct {
	value bool
	valid bool
}

func (f *dbFlag) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		f.valid = false
		f.value = false
		return nil
	case bool:
		f.valid = true
		f.value = value
		return nil
	case int64:
		f.valid = true
		f.value = value != 0
		return nil
	case []byte:
		parsed, err := strconv.ParseBool(string(value))
		if err != nil {
			number, numberErr := strconv.ParseInt(string(value), 10, 64)
			if numberErr != nil {
				return fmt.Errorf("scan database flag %q: %w", value, err)
			}
			parsed = number != 0
		}
		f.valid = true
		f.value = parsed
		return nil
	default:
		return fmt.Errorf("scan database flag from %T", src)
	}
}

func (f dbFlag) Bool() bool {
	return f.valid && f.value
}

func (f dbFlag) Int() int {
	if f.Bool() {
		return 1
	}
	return 0
}

const storageStrategyColumns = `id, strategyName, providerType, isDefault, isEnabled, runState, priority,
	configCiphertext, configEdek, wrapKeyRef, healthCheckUrl, healthStatus, lastHealthCheck,
	failureCount, totalRequests, failureRateThreshold, createTime, updateTime, isDeleted`

const fileInfoColumns = `id, fileInnerName, fileSize, fileSha256, fileCrc32c, hashAlgorithm, contentType,
	fileMetadata, thumbnailData, storageStrategyId, storagePath, status, scanStatus, integrityStatus,
	createTime, updateTime, isDeleted, deletedTime`

const fileReferenceColumns = `id, fileId, userId, scopeId, displayName, bizType, bizId, visitUrl, accessLevel,
	createTime, updateTime, isDeleted, visitStrategy, accessScope`

const uploadTaskColumns = `id, userId, scopeId, credentialId, credentialVersion, bizType, bizId, fileName, contentType, storageStrategyId,
	objectKeyStaging, objectKeyClean, status, uploadMode, multipartUploadId, partSize, totalParts,
	expectedSize, expectedSha256, expectedCrc32c, actualSize, etag, serverCrc32c, failureCategory,
	failureReason, fileId, bindingToken, bindingChannel, expireAt, protectedUntil, credentialExpireAt, revokedAt,
	userIp, createTime, updateTime`

const processTaskColumns = `id, fileId, taskType, taskParams, pipelineId, nodeId, idempotencyKey,
	dedupKey, replayToken, dependsOn, attempt, status, retryCount, maxRetry, errorMsg, resultData,
	priority, mqMessageId, nextRetryTime, createTime, updateTime, startTime, finishTime`

const chunkUploadColumns = `id, uploadId, uploadTaskId, userId, scopeId, fileName, contentType, fileSize, chunkSize, totalChunks,
	uploadedChunks, chunkSha256Map, fileSha256, expectedCrc32c, serverCrc32c, storageStrategyId,
	tempStoragePath, cloudUploadId, partETagsMap, bizType, bizId, status, expireTime, createTime, updateTime`

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("file repository requires datasource provider")
	}
	dialect := strings.ToLower(strings.TrimSpace(provider.Dialect()))
	driver := strings.ToLower(strings.TrimSpace(provider.Driver()))
	return &Repository{
		db:       provider.SQLX(),
		postgres: strings.Contains(dialect, "postgres") || strings.Contains(driver, "postgres") || strings.Contains(driver, "pgx"),
	}, nil
}

func (r *Repository) exec(ctx context.Context) (store.SQLX, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	if exec == nil {
		return nil, fmt.Errorf("file repository datasource is not configured")
	}
	return dialectExecutor{SQLX: exec, postgres: r.postgres}, nil
}

type storageStrategyRow struct {
	ID                   int64           `db:"id"`
	StrategyName         string          `db:"strategyName"`
	ProviderType         string          `db:"providerType"`
	IsDefault            dbFlag          `db:"isDefault"`
	IsEnabled            dbFlag          `db:"isEnabled"`
	RunState             string          `db:"runState"`
	Priority             int             `db:"priority"`
	ConfigCiphertext     sql.NullString  `db:"configCiphertext"`
	ConfigEDEK           sql.NullString  `db:"configEdek"`
	WrapKeyRef           sql.NullString  `db:"wrapKeyRef"`
	HealthCheckURL       sql.NullString  `db:"healthCheckUrl"`
	HealthStatus         sql.NullInt64   `db:"healthStatus"`
	LastHealthCheck      sql.NullTime    `db:"lastHealthCheck"`
	FailureCount         sql.NullInt64   `db:"failureCount"`
	TotalRequests        sql.NullInt64   `db:"totalRequests"`
	FailureRateThreshold sql.NullFloat64 `db:"failureRateThreshold"`
	CreateTime           sql.NullTime    `db:"createTime"`
	UpdateTime           sql.NullTime    `db:"updateTime"`
	IsDeleted            dbFlag          `db:"isDeleted"`
}

type fileInfoRow struct {
	ID                int64          `db:"id"`
	FileInnerName     string         `db:"fileInnerName"`
	FileSize          int64          `db:"fileSize"`
	FileSha256        sql.NullString `db:"fileSha256"`
	FileCrc32c        sql.NullString `db:"fileCrc32c"`
	HashAlgorithm     sql.NullString `db:"hashAlgorithm"`
	ContentType       string         `db:"contentType"`
	FileMetadata      sql.NullString `db:"fileMetadata"`
	ThumbnailData     sql.NullString `db:"thumbnailData"`
	StorageType       int            `db:"storageType"`
	StorageStrategyID sql.NullInt64  `db:"storageStrategyId"`
	StoragePath       string         `db:"storagePath"`
	Status            sql.NullString `db:"status"`
	ScanStatus        sql.NullString `db:"scanStatus"`
	IntegrityStatus   sql.NullString `db:"integrityStatus"`
	CreateTime        sql.NullTime   `db:"createTime"`
	UpdateTime        sql.NullTime   `db:"updateTime"`
	IsDeleted         dbFlag         `db:"isDeleted"`
	DeletedTime       sql.NullTime   `db:"deletedTime"`
}

type fileReferenceRow struct {
	ID            int64          `db:"id"`
	FileID        int64          `db:"fileId"`
	UserID        int64          `db:"userId"`
	ScopeID       sql.NullString `db:"scopeId"`
	DisplayName   string         `db:"displayName"`
	BizType       string         `db:"bizType"`
	BizID         int64          `db:"bizId"`
	VisitURL      sql.NullString `db:"visitUrl"`
	AccessLevel   sql.NullInt64  `db:"accessLevel"`
	VisitStrategy sql.NullString `db:"visitStrategy"`
	AccessScope   sql.NullString `db:"accessScope"`
	CreateTime    sql.NullTime   `db:"createTime"`
	UpdateTime    sql.NullTime   `db:"updateTime"`
	IsDeleted     dbFlag         `db:"isDeleted"`
}

type uploadTaskRow struct {
	ID                 string         `db:"id"`
	UserID             int64          `db:"userId"`
	ScopeID            sql.NullString `db:"scopeId"`
	CredentialID       sql.NullString `db:"credentialId"`
	CredentialVersion  int            `db:"credentialVersion"`
	BizType            sql.NullInt64  `db:"bizType"`
	BizID              sql.NullInt64  `db:"bizId"`
	FileName           sql.NullString `db:"fileName"`
	ContentType        sql.NullString `db:"contentType"`
	StorageStrategyID  sql.NullInt64  `db:"storageStrategyId"`
	ObjectKeyStaging   string         `db:"objectKeyStaging"`
	ObjectKeyClean     string         `db:"objectKeyClean"`
	Status             string         `db:"status"`
	UploadMode         sql.NullString `db:"uploadMode"`
	MultipartUploadID  sql.NullString `db:"multipartUploadId"`
	PartSize           sql.NullInt64  `db:"partSize"`
	TotalParts         sql.NullInt64  `db:"totalParts"`
	ExpectedSize       sql.NullInt64  `db:"expectedSize"`
	ExpectedSha256     sql.NullString `db:"expectedSha256"`
	ExpectedCrc32c     sql.NullString `db:"expectedCrc32c"`
	ActualSize         sql.NullInt64  `db:"actualSize"`
	ETag               sql.NullString `db:"etag"`
	ServerCrc32c       sql.NullString `db:"serverCrc32c"`
	FailureCategory    sql.NullString `db:"failureCategory"`
	FailureReason      sql.NullString `db:"failureReason"`
	FileID             sql.NullInt64  `db:"fileId"`
	BindingToken       sql.NullString `db:"bindingToken"`
	BindingChannel     sql.NullString `db:"bindingChannel"`
	ExpireAt           sql.NullTime   `db:"expireAt"`
	ProtectedUntil     sql.NullTime   `db:"protectedUntil"`
	CredentialExpireAt sql.NullTime   `db:"credentialExpireAt"`
	RevokedAt          sql.NullTime   `db:"revokedAt"`
	UserIP             sql.NullString `db:"userIp"`
	CreateTime         sql.NullTime   `db:"createTime"`
	UpdateTime         sql.NullTime   `db:"updateTime"`
}

type processTaskRow struct {
	ID             int64          `db:"id"`
	FileID         int64          `db:"fileId"`
	TaskType       string         `db:"taskType"`
	TaskParams     sql.NullString `db:"taskParams"`
	PipelineID     sql.NullString `db:"pipelineId"`
	NodeID         sql.NullString `db:"nodeId"`
	IdempotencyKey sql.NullString `db:"idempotencyKey"`
	DedupKey       sql.NullString `db:"dedupKey"`
	ReplayToken    sql.NullString `db:"replayToken"`
	DependsOn      sql.NullString `db:"dependsOn"`
	Attempt        sql.NullInt64  `db:"attempt"`
	Status         int            `db:"status"`
	RetryCount     int            `db:"retryCount"`
	MaxRetry       int            `db:"maxRetry"`
	ErrorMsg       sql.NullString `db:"errorMsg"`
	ResultData     sql.NullString `db:"resultData"`
	Priority       int            `db:"priority"`
	MQMessageID    sql.NullString `db:"mqMessageId"`
	NextRetryTime  sql.NullTime   `db:"nextRetryTime"`
	CreateTime     sql.NullTime   `db:"createTime"`
	UpdateTime     sql.NullTime   `db:"updateTime"`
	StartTime      sql.NullTime   `db:"startTime"`
	FinishTime     sql.NullTime   `db:"finishTime"`
}

type chunkUploadRow struct {
	ID                int64          `db:"id"`
	UploadID          string         `db:"uploadId"`
	UploadTaskID      sql.NullString `db:"uploadTaskId"`
	UserID            int64          `db:"userId"`
	ScopeID           sql.NullString `db:"scopeId"`
	FileName          string         `db:"fileName"`
	ContentType       sql.NullString `db:"contentType"`
	FileSize          int64          `db:"fileSize"`
	ChunkSize         int            `db:"chunkSize"`
	TotalChunks       int            `db:"totalChunks"`
	UploadedChunks    sql.NullString `db:"uploadedChunks"`
	ChunkSha256Map    sql.NullString `db:"chunkSha256Map"`
	FileSha256        sql.NullString `db:"fileSha256"`
	ExpectedCrc32c    sql.NullString `db:"expectedCrc32c"`
	ServerCrc32c      sql.NullString `db:"serverCrc32c"`
	StorageStrategyID int64          `db:"storageStrategyId"`
	TempStoragePath   sql.NullString `db:"tempStoragePath"`
	CloudUploadID     sql.NullString `db:"cloudUploadId"`
	PartETagsMap      sql.NullString `db:"partETagsMap"`
	BizType           sql.NullString `db:"bizType"`
	BizID             sql.NullInt64  `db:"bizId"`
	Status            int            `db:"status"`
	ExpireTime        sql.NullTime   `db:"expireTime"`
	CreateTime        sql.NullTime   `db:"createTime"`
	UpdateTime        sql.NullTime   `db:"updateTime"`
}

func (r *Repository) GetDefaultStrategy(ctx context.Context) (*domain.StorageStrategy, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row storageStrategyRow
	query := exec.Rebind(`SELECT ` + storageStrategyColumns + ` FROM sys_storage_strategy WHERE isDefault = 1 AND isEnabled = 1 AND isDeleted = 0 ORDER BY priority DESC, id ASC LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get default storage strategy: %w", err)
	}
	item := mapStorage(row)
	return &item, nil
}

func (r *Repository) GetStrategy(ctx context.Context, id int64) (*domain.StorageStrategy, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row storageStrategyRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+storageStrategyColumns+` FROM sys_storage_strategy WHERE id = ? AND isDeleted = 0 LIMIT 1`), id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get storage strategy: %w", err)
	}
	item := mapStorage(row)
	return &item, nil
}

func (r *Repository) ListStrategies(ctx context.Context) ([]domain.StorageStrategy, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	rows := []storageStrategyRow{}
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT `+storageStrategyColumns+` FROM sys_storage_strategy WHERE isDeleted = 0 ORDER BY priority DESC, id ASC`)); err != nil {
		return nil, fmt.Errorf("list storage strategies: %w", err)
	}
	result := make([]domain.StorageStrategy, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapStorage(row))
	}
	return result, nil
}

func (r *Repository) InsertStrategy(ctx context.Context, item *domain.StorageStrategy) (int64, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	if item.ID <= 0 {
		item.ID = time.Now().UnixNano()
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_storage_strategy (
	id, strategyName, providerType, isDefault, isEnabled, runState, priority,
	configCiphertext, configEdek, wrapKeyRef, healthCheckUrl, healthStatus,
	failureCount, totalRequests, failureRateThreshold, windowStartTime, createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		item.ID, item.StrategyName, item.ProviderType, r.boolValue(item.IsDefault), r.boolValue(item.IsEnabled), nullIfBlank(item.RunState), item.Priority,
		item.ConfigCiphertext, item.ConfigEDEK, item.WrapKeyRef, nullIfBlank(item.HealthCheckURL), defaultInt(item.HealthStatus, domain.HealthHealthy),
		item.FailureCount, item.TotalRequests, defaultFloat(item.FailureRateThreshold, 10), time.Now(), time.Now(), time.Now(), r.boolValue(false))
	if err != nil {
		return 0, fmt.Errorf("insert storage strategy: %w", err)
	}
	return insertedID(ctx, exec, result, item.ID)
}

func (r *Repository) UpdateStrategy(ctx context.Context, item *domain.StorageStrategy) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_storage_strategy
SET strategyName=?, providerType=?, isDefault=?, isEnabled=?, runState=?, priority=?, configCiphertext=?, configEdek=?, wrapKeyRef=?,
	healthCheckUrl=?, failureRateThreshold=?, updateTime=?
WHERE id=? AND isDeleted=0`),
		item.StrategyName, item.ProviderType, r.boolValue(item.IsDefault), r.boolValue(item.IsEnabled), item.RunState, item.Priority, item.ConfigCiphertext, item.ConfigEDEK, item.WrapKeyRef,
		nullIfBlank(item.HealthCheckURL), defaultFloat(item.FailureRateThreshold, 10), time.Now(), item.ID)
	if err != nil {
		return fmt.Errorf("update storage strategy: %w", err)
	}
	return nil
}

func (r *Repository) SetOnlyDefaultStrategy(ctx context.Context, id int64) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	if _, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_storage_strategy SET isDefault = 0, runState = ? WHERE id <> ? AND isDeleted = 0`), domain.RunStateDraining, id); err != nil {
		return fmt.Errorf("clear default storage strategy: %w", err)
	}
	if _, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_storage_strategy SET isDefault = 1, isEnabled = 1, runState = ?, updateTime = ? WHERE id = ? AND isDeleted = 0`), domain.RunStateActive, time.Now(), id); err != nil {
		return fmt.Errorf("set default storage strategy: %w", err)
	}
	return nil
}

func (r *Repository) EnableStrategy(ctx context.Context, id int64, enabled bool) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	runState := domain.RunStateDisabled
	if enabled {
		runState = domain.RunStateActive
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_storage_strategy SET isEnabled=?, runState=?, updateTime=? WHERE id=? AND isDeleted=0`), r.boolValue(enabled), runState, time.Now(), id)
	return err
}

func (r *Repository) DeleteStrategy(ctx context.Context, id int64) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_storage_strategy SET isDeleted=1, isEnabled=0, runState=?, updateTime=? WHERE id=?`), domain.RunStateDisabled, time.Now(), id)
	return err
}

func (r *Repository) UpdateStrategyHealth(ctx context.Context, id int64, health int, success bool) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	if success {
		_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_storage_strategy SET healthStatus=?, lastHealthCheck=?, totalRequests=totalRequests+1, updateTime=? WHERE id=?`), health, time.Now(), time.Now(), id)
	} else {
		_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_storage_strategy SET healthStatus=?, lastHealthCheck=?, failureCount=failureCount+1, totalRequests=totalRequests+1, updateTime=? WHERE id=?`), health, time.Now(), time.Now(), id)
	}
	return err
}

func (r *Repository) UpdateStrategyHealthBatch(ctx context.Context, updates []domain.StorageHealthUpdate) (int64, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	if len(updates) > 50 {
		return 0, fmt.Errorf("storage health update set exceeds 50")
	}
	seen := make(map[int64]struct{}, len(updates))
	normalized := make([]domain.StorageHealthUpdate, 0, len(updates))
	for _, update := range updates {
		if update.StrategyID <= 0 {
			return 0, fmt.Errorf("storage strategy id is required")
		}
		if _, exists := seen[update.StrategyID]; exists {
			continue
		}
		seen[update.StrategyID] = struct{}{}
		normalized = append(normalized, update)
	}
	var query strings.Builder
	args := make([]any, 0, len(normalized)*6+2)
	query.WriteString(`UPDATE sys_storage_strategy SET healthStatus=CASE id`)
	for _, update := range normalized {
		query.WriteString(" WHEN ? THEN ?")
		args = append(args, update.StrategyID, update.HealthStatus)
	}
	query.WriteString(" ELSE healthStatus END, failureCount=failureCount+CASE id")
	for _, update := range normalized {
		increment := 0
		if !update.Healthy {
			increment = 1
		}
		query.WriteString(" WHEN ? THEN ?")
		args = append(args, update.StrategyID, increment)
	}
	now := time.Now().UTC()
	query.WriteString(" ELSE 0 END, totalRequests=totalRequests+1, lastHealthCheck=?, updateTime=? WHERE isDeleted=0 AND id IN (")
	args = append(args, now, now)
	for index, update := range normalized {
		if index > 0 {
			query.WriteString(",")
		}
		query.WriteString("?")
		args = append(args, update.StrategyID)
	}
	query.WriteString(")")
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(query.String()), args...)
	if err != nil {
		return 0, fmt.Errorf("update storage strategy health batch: %w", err)
	}
	return result.RowsAffected()
}

func (r *Repository) FindFileBySha256AndSize(ctx context.Context, sha256 string, size int64) (*domain.FileInfo, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row fileInfoRow
	err = sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+fileInfoColumns+` FROM sys_file_info WHERE fileSha256=? AND fileSize=? AND isDeleted=0 LIMIT 1`), strings.TrimSpace(sha256), size)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find file by sha256 and size: %w", err)
	}
	item := mapFile(row)
	return &item, nil
}

func (r *Repository) GetFile(ctx context.Context, id int64) (*domain.FileInfo, error) {
	return r.getFile(ctx, id, false)
}

func (r *Repository) ListFilesByIDs(ctx context.Context, ids []int64) ([]domain.FileInfo, error) {
	ids = uniquePositiveIDs(ids)
	if len(ids) == 0 {
		return []domain.FileInfo{}, nil
	}
	if len(ids) > 100 {
		return nil, fmt.Errorf("file set exceeds 100")
	}
	query, args, err := sqlx.In(`SELECT `+fileInfoColumns+` FROM sys_file_info WHERE id IN (?) AND isDeleted=0 ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var rows []fileInfoRow
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list files by ids: %w", err)
	}
	result := make([]domain.FileInfo, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapFile(row))
	}
	return result, nil
}

func (r *Repository) GetFileForUpdate(ctx context.Context, id int64) (*domain.FileInfo, error) {
	return r.getFile(ctx, id, true)
}

func (r *Repository) getFile(ctx context.Context, id int64, forUpdate bool) (*domain.FileInfo, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row fileInfoRow
	query := `SELECT ` + fileInfoColumns + ` FROM sys_file_info WHERE id=? AND isDeleted=0 LIMIT 1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(query), id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get file: %w", err)
	}
	item := mapFile(row)
	return &item, nil
}

func (r *Repository) InsertFile(ctx context.Context, item *domain.FileInfo) (int64, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	if item.ID <= 0 {
		item.ID = time.Now().UnixNano()
	}
	status := item.Status
	if status == "" {
		status = domain.FileStatusPendingBind
	}
	scan := item.ScanStatus
	if scan == "" {
		scan = domain.ScanStatusClean
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_file_info (
	id, fileInnerName, fileSize, fileSha256, fileCrc32c, hashAlgorithm, contentType,
	storageStrategyId, storagePath, status, scanStatus, integrityStatus,
	createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		item.ID, item.FileInnerName, item.FileSize, nullIfBlank(item.FileSha256), nullIfBlank(item.FileCrc32c), nullIfBlank(defaultString(item.HashAlgorithm, "SHA-256")), item.ContentType,
		nullIfZero(item.StorageStrategyID), item.StoragePath, status, scan, nullIfBlank(item.IntegrityStatus),
		time.Now(), time.Now(), r.boolValue(false))
	if err != nil {
		return 0, fmt.Errorf("insert file info: %w", err)
	}
	return insertedID(ctx, exec, result, item.ID)
}

func (r *Repository) UpdateFileStatus(ctx context.Context, id int64, status string) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_file_info SET status=?, updateTime=? WHERE id=? AND isDeleted=0`), status, time.Now(), id)
	return err
}

func (r *Repository) UpdateFile(ctx context.Context, item *domain.FileInfo) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_info
SET fileInnerName=?, fileSize=?, fileSha256=?, fileCrc32c=?, hashAlgorithm=?, contentType=?,
	storageStrategyId=?, storagePath=?, status=?, scanStatus=?, integrityStatus=?, updateTime=?
WHERE id=? AND isDeleted=0`),
		item.FileInnerName, item.FileSize, nullIfBlank(item.FileSha256), nullIfBlank(item.FileCrc32c), nullIfBlank(defaultString(item.HashAlgorithm, "SHA-256")), item.ContentType,
		nullIfZero(item.StorageStrategyID), item.StoragePath, item.Status, item.ScanStatus, item.IntegrityStatus, time.Now(), item.ID)
	return err
}

func (r *Repository) SoftDeleteFile(ctx context.Context, id int64) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_file_info SET status=?, isDeleted=1, deletedTime=?, updateTime=? WHERE id=?`), domain.FileStatusDeleted, time.Now(), time.Now(), id)
	return err
}

func (r *Repository) ListCleanupCandidates(ctx context.Context, now time.Time, limit int) ([]domain.FileInfo, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows := []fileInfoRow{}
	query := `
SELECT ` + fileInfoColumns + `
FROM sys_file_info f
WHERE f.status IN (?, ?) AND f.isDeleted=0
	AND NOT EXISTS (
		SELECT 1 FROM sys_file_reference r
		WHERE r.fileId=f.id AND r.isDeleted=0
	)
	AND NOT EXISTS (
		SELECT 1 FROM sys_upload_task t
		WHERE t.fileId=f.id AND t.status=? AND t.credentialVersion>=?
			AND t.revokedAt IS NULL AND t.protectedUntil>? AND t.credentialExpireAt>?
	)
ORDER BY f.updateTime ASC
LIMIT ?`
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(query), domain.FileStatusAvailable, domain.FileStatusCleaning, domain.UploadTaskClean, domain.UploadCredentialVersion1, now, now, limit); err != nil {
		return nil, fmt.Errorf("list file cleanup candidates: %w", err)
	}
	result := make([]domain.FileInfo, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapFile(row))
	}
	return result, nil
}

func (r *Repository) ClaimFileForCleanup(ctx context.Context, fileID int64, _ time.Time) (bool, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_info
SET status=?, updateTime=?
WHERE id=? AND status=? AND isDeleted=0`),
		domain.FileStatusCleaning, time.Now().UTC(), fileID, domain.FileStatusAvailable)
	if err != nil {
		return false, fmt.Errorf("claim file for cleanup: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (r *Repository) HasActiveReferences(ctx context.Context, fileID int64) (bool, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return false, err
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, exec.Rebind(`SELECT COUNT(1) FROM sys_file_reference WHERE fileId=? AND isDeleted=0`), fileID); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) HasProtectedCredential(ctx context.Context, fileID int64, now time.Time) (bool, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return false, err
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, exec.Rebind(`
SELECT COUNT(1)
FROM sys_upload_task
WHERE fileId=? AND status=? AND credentialVersion>=?
	AND revokedAt IS NULL AND protectedUntil>? AND credentialExpireAt>?`),
		fileID, domain.UploadTaskClean, domain.UploadCredentialVersion1, now, now); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) RestoreFileAvailableIfCleaning(ctx context.Context, fileID int64) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_file_info SET status=?, updateTime=? WHERE id=? AND status=? AND isDeleted=0`), domain.FileStatusAvailable, time.Now().UTC(), fileID, domain.FileStatusCleaning)
	return err
}

func (r *Repository) MarkFileDeletedIfCleaning(ctx context.Context, fileID int64) (bool, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	result, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_info
SET status=?, isDeleted=?, deletedTime=?, updateTime=?
WHERE id=? AND status=? AND isDeleted=0`),
		domain.FileStatusDeleted, r.boolValue(true), now, now, fileID, domain.FileStatusCleaning)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (r *Repository) QueryFiles(ctx context.Context, current, size int64, fileName, fileType string, bizType *int, startTime, endTime string) (*domain.Page[domain.FileInfo], error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	where := []string{"f.isDeleted=0"}
	args := []any{}
	if fileName = strings.TrimSpace(fileName); fileName != "" {
		where = append(where, "(f.fileInnerName LIKE ? OR r.displayName LIKE ?)")
		like := "%" + fileName + "%"
		args = append(args, like, like)
	}
	if fileType = strings.TrimSpace(fileType); fileType != "" {
		where = append(where, "f.contentType LIKE ?")
		args = append(args, "%"+fileType+"%")
	}
	if bizType != nil {
		where = append(where, "r.bizType = ?")
		args = append(args, fmt.Sprintf("%d", *bizType))
	}
	if startTime = strings.TrimSpace(startTime); startTime != "" {
		where = append(where, "f.createTime >= ?")
		args = append(args, startTime)
	}
	if endTime = strings.TrimSpace(endTime); endTime != "" {
		where = append(where, "f.createTime <= ?")
		args = append(args, endTime)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, exec.Rebind(`SELECT COUNT(DISTINCT f.id) FROM sys_file_info f LEFT JOIN sys_file_reference r ON r.fileId=f.id AND r.isDeleted=0 WHERE `+whereSQL), args...); err != nil {
		return nil, fmt.Errorf("count files: %w", err)
	}
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}
	page := &domain.Page[domain.FileInfo]{Current: current, Size: size, Total: total, Records: []domain.FileInfo{}}
	if total == 0 {
		return page, nil
	}
	rows := []fileInfoRow{}
	listArgs := append(append([]any{}, args...), size, (current-1)*size)
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT DISTINCT f.id, f.fileInnerName, f.fileSize, f.fileSha256, f.fileCrc32c, f.hashAlgorithm, f.contentType, f.fileMetadata, f.thumbnailData, f.storageStrategyId, f.storagePath, f.status, f.scanStatus, f.integrityStatus, f.createTime, f.updateTime, f.isDeleted, f.deletedTime FROM sys_file_info f LEFT JOIN sys_file_reference r ON r.fileId=f.id AND r.isDeleted=0 WHERE `+whereSQL+` ORDER BY f.createTime DESC LIMIT ? OFFSET ?`), listArgs...); err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	for _, row := range rows {
		page.Records = append(page.Records, mapFile(row))
	}
	return page, nil
}

func (r *Repository) InsertReference(ctx context.Context, ref *domain.FileReference) (int64, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	if ref.ID <= 0 {
		ref.ID = time.Now().UnixNano()
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_file_reference (
	id, fileId, userId, scopeId, displayName, bizType, bizId, visitUrl, accessLevel, visitStrategy, accessScope, createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		ref.ID, ref.FileID, ref.UserID, nullIfBlank(ref.ScopeID), ref.DisplayName, ref.BizType, ref.BizID, nullIfBlank(ref.VisitURL), ref.AccessLevel, nullIfBlank(ref.VisitStrategy), nullIfBlank(ref.AccessScope), time.Now(), time.Now(), r.boolValue(false))
	if err != nil {
		return 0, fmt.Errorf("insert file reference: %w", err)
	}
	return insertedID(ctx, exec, result, ref.ID)
}

func (r *Repository) ListReferencesByFile(ctx context.Context, fileID int64) ([]domain.FileReference, error) {
	return r.queryReferences(ctx, `fileId=? AND isDeleted=0`, fileID)
}

func (r *Repository) ListReferencesByBiz(ctx context.Context, userID int64, bizType string, bizID int64) ([]domain.FileReference, error) {
	return r.queryReferences(ctx, `userId=? AND bizType=? AND bizId=? AND isDeleted=0`, userID, bizType, bizID)
}

// FindConfigAssetReference deliberately does not use userId. For a
// CONFIG_ASSET, userId is the binding operator/audit subject while bizId is
// the server-owned configuration target. The database has a dedicated active
// slot constraint for this query shape.
func (r *Repository) FindConfigAssetReference(ctx context.Context, configID int64) (*domain.FileReference, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row fileReferenceRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+fileReferenceColumns+` FROM sys_file_reference WHERE bizType=? AND bizId=? AND isDeleted=0 LIMIT 1`), configAssetBizType, configID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find config asset reference: %w", err)
	}
	item := mapReference(row)
	return &item, nil
}

func (r *Repository) GetReference(ctx context.Context, id int64) (*domain.FileReference, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row fileReferenceRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+fileReferenceColumns+` FROM sys_file_reference WHERE id=? AND isDeleted=0 LIMIT 1`), id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get file reference: %w", err)
	}
	item := mapReference(row)
	return &item, nil
}

func (r *Repository) FindPublicReferenceByFile(ctx context.Context, fileID int64) (*domain.FileReference, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row fileReferenceRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+fileReferenceColumns+` FROM sys_file_reference WHERE fileId=? AND isDeleted=0 AND bizType<>? AND visitStrategy='PUBLIC_STATIC' AND accessScope='PUBLIC' LIMIT 1`), fileID, configAssetBizType); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find public reference: %w", err)
	}
	item := mapReference(row)
	return &item, nil
}

func (r *Repository) UpdateReferenceAccess(ctx context.Context, id int64, accessScope, visitStrategy string, accessLevel int, visitURL string) (*domain.FileReference, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_file_reference SET accessScope=?, visitStrategy=?, accessLevel=?, visitUrl=?, updateTime=? WHERE id=? AND isDeleted=0`), accessScope, visitStrategy, accessLevel, nullIfBlank(visitURL), time.Now(), id); err != nil {
		return nil, fmt.Errorf("update reference access: %w", err)
	}
	return r.GetReference(ctx, id)
}

// UpdateConfigAssetReference is private to the configuration asset facade.
// Generic file-management access updates must never mutate this policy.
func (r *Repository) UpdateConfigAssetReference(ctx context.Context, item *domain.FileReference) error {
	if item == nil || item.ID <= 0 || item.BizType != configAssetBizType || item.BizID <= 0 {
		return fmt.Errorf("invalid config asset reference")
	}
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_reference
SET displayName=?, visitUrl=?, accessLevel=?, visitStrategy=?, accessScope=?, updateTime=?
WHERE id=? AND bizType=? AND bizId=? AND isDeleted=0`),
		item.DisplayName, nullIfBlank(item.VisitURL), item.AccessLevel, nullIfBlank(item.VisitStrategy), nullIfBlank(item.AccessScope), time.Now(),
		item.ID, configAssetBizType, item.BizID)
	if err != nil {
		return fmt.Errorf("update config asset reference: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read config asset reference update result: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("config asset reference was changed concurrently")
	}
	return nil
}

func (r *Repository) DeleteReferencesByBiz(ctx context.Context, userID int64, bizType string, bizID int64) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`DELETE FROM sys_file_reference WHERE userId=? AND bizType=? AND bizId=?`), userID, bizType, bizID)
	return err
}

func (r *Repository) SoftDeleteReference(ctx context.Context, fileID, userID int64, bizType string, bizID int64) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_file_reference SET isDeleted=1, updateTime=? WHERE fileId=? AND userId=? AND bizType=? AND bizId=?`), time.Now(), fileID, userID, bizType, bizID)
	return err
}

func (r *Repository) SoftDeleteReferenceInScope(ctx context.Context, fileID, userID int64, scopeID, bizType string, bizID int64) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_reference
SET isDeleted=1, updateTime=?
WHERE fileId=? AND userId=? AND scopeId=? AND bizType=? AND bizId=?`),
		time.Now(), fileID, userID, strings.TrimSpace(scopeID), bizType, bizID)
	return err
}

// SoftDeleteConfigAssetReference retires the single server-owned active
// configuration slot. Scope is required so an administrator in another
// organization cannot replace or clear a file uploaded in this scope.
func (r *Repository) SoftDeleteConfigAssetReference(ctx context.Context, configID int64, scopeID string) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_reference
SET isDeleted=1, updateTime=?
WHERE bizType=? AND bizId=? AND scopeId=? AND isDeleted=0`),
		time.Now(), configAssetBizType, configID, strings.TrimSpace(scopeID))
	if err != nil {
		return fmt.Errorf("soft delete config asset reference: %w", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("read config asset reference delete result: %w", err)
	}
	return nil
}

func (r *Repository) queryReferences(ctx context.Context, where string, args ...any) ([]domain.FileReference, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	rows := []fileReferenceRow{}
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT `+fileReferenceColumns+` FROM sys_file_reference WHERE `+where+` ORDER BY createTime DESC`), args...); err != nil {
		return nil, fmt.Errorf("query file references: %w", err)
	}
	result := make([]domain.FileReference, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapReference(row))
	}
	return result, nil
}

func (r *Repository) InsertUploadTask(ctx context.Context, task *domain.UploadTask) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_upload_task (
	id, userId, scopeId, credentialId, credentialVersion, bizType, bizId, fileName, contentType, storageStrategyId, objectKeyStaging, objectKeyClean, status,
	uploadMode, multipartUploadId, partSize, totalParts, expectedSize, expectedSha256, expectedCrc32c, actualSize,
	etag, serverCrc32c, failureCategory, failureReason, fileId, bindingToken, bindingChannel, expireAt,
	protectedUntil, credentialExpireAt, revokedAt, userIp, createTime, updateTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		task.ID, task.UserID, nullIfBlank(task.ScopeID), nullIfBlank(task.CredentialID), task.CredentialVersion,
		nullIfZero(int64(task.BizType)), nullIfZero(task.BizID), nullIfBlank(task.FileName), nullIfBlank(task.ContentType), nullIfZero(task.StorageStrategyID), task.ObjectKeyStaging, task.ObjectKeyClean, task.Status,
		nullIfBlank(task.UploadMode), nullIfBlank(task.MultipartUploadID), nullIfZero(task.PartSize), nullIfZero(int64(task.TotalParts)), nullIfZero(task.ExpectedSize), nullIfBlank(task.ExpectedSha256), nullIfBlank(task.ExpectedCrc32c), nullIfZero(task.ActualSize),
		nullIfBlank(task.ETag), nullIfBlank(task.ServerCrc32c), nullIfBlank(task.FailureCategory), nullIfBlank(task.FailureReason), nullIfZero(task.FileID), nullIfBlank(task.BindingToken), nullIfBlank(task.BindingChannel), task.ExpireAt,
		task.ProtectedUntil, task.CredentialExpireAt, task.RevokedAt, nullIfBlank(task.UserIP), time.Now(), time.Now())
	return err
}

func (r *Repository) GetUploadTask(ctx context.Context, id string) (*domain.UploadTask, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row uploadTaskRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+uploadTaskColumns+` FROM sys_upload_task WHERE id=? LIMIT 1`), id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get upload task: %w", err)
	}
	item := mapUploadTask(row)
	return &item, nil
}

func (r *Repository) FindUploadCredential(ctx context.Context, userID int64, scopeID string, fileID int64) (*domain.UploadTask, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row uploadTaskRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`
SELECT `+uploadTaskColumns+`
FROM sys_upload_task
WHERE userId=? AND scopeId=? AND fileId=? AND credentialVersion>=? AND credentialId IS NOT NULL
	AND status=? AND revokedAt IS NULL AND credentialExpireAt>?
ORDER BY credentialExpireAt DESC, updateTime DESC
LIMIT 1`), userID, strings.TrimSpace(scopeID), fileID, domain.UploadCredentialVersion1, domain.UploadTaskClean, time.Now().UTC()); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find upload credential: %w", err)
	}
	item := mapUploadTask(row)
	return &item, nil
}

func (r *Repository) ListExpiredUploadTasks(ctx context.Context, now time.Time, limit int) ([]domain.UploadTask, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows := []uploadTaskRow{}
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`
SELECT `+uploadTaskColumns+`
FROM sys_upload_task
WHERE expireAt IS NOT NULL AND expireAt<=? AND status IN (?, ?, ?)
ORDER BY expireAt ASC, updateTime ASC
LIMIT ?`), now, domain.UploadTaskInit, domain.UploadTaskUploading, domain.UploadTaskUploaded, limit); err != nil {
		return nil, fmt.Errorf("list expired upload tasks: %w", err)
	}
	result := make([]domain.UploadTask, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapUploadTask(row))
	}
	return result, nil
}

func (r *Repository) UpdateUploadTask(ctx context.Context, task *domain.UploadTask) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_upload_task SET status=?, actualSize=?, etag=?, serverCrc32c=?, failureCategory=?, failureReason=?, fileId=?,
	scopeId=?, credentialId=?, credentialVersion=?, protectedUntil=?, credentialExpireAt=?, revokedAt=?, updateTime=?
WHERE id=?`),
		task.Status, nullIfZero(task.ActualSize), nullIfBlank(task.ETag), nullIfBlank(task.ServerCrc32c), nullIfBlank(task.FailureCategory), nullIfBlank(task.FailureReason), nullIfZero(task.FileID),
		nullIfBlank(task.ScopeID), nullIfBlank(task.CredentialID), task.CredentialVersion, task.ProtectedUntil, task.CredentialExpireAt, task.RevokedAt, time.Now(), task.ID)
	return err
}

func (r *Repository) UpdateUploadTaskStatusIfMatch(ctx context.Context, id, from, to string) (bool, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_upload_task SET status=?, updateTime=? WHERE id=? AND status=?`), to, time.Now(), id, from)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *Repository) ExpireUploadTasks(ctx context.Context, items []domain.UploadTask) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}
	if len(items) > 50 {
		return 0, fmt.Errorf("upload task maintenance set exceeds 50")
	}
	clauses := make([]string, 0, len(items))
	args := []any{domain.UploadTaskExpired, time.Now().UTC()}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return 0, fmt.Errorf("upload task id is required")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		clauses = append(clauses, `(id=? AND status=?)`)
		args = append(args, id, item.Status)
	}
	if len(clauses) == 0 {
		return 0, nil
	}
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	query := `UPDATE sys_upload_task SET status=?, updateTime=? WHERE ` + strings.Join(clauses, " OR ")
	result, err := exec.ExecContext(ctx, exec.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("expire upload tasks: %w", err)
	}
	return result.RowsAffected()
}

func (r *Repository) InsertChunkUpload(ctx context.Context, upload *domain.ChunkUpload) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	if upload.ID <= 0 {
		upload.ID = time.Now().UnixNano()
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_file_chunk_upload (
	id, uploadId, uploadTaskId, userId, scopeId, fileName, contentType, fileSize, chunkSize, totalChunks, uploadedChunks,
	chunkSha256Map, fileSha256, expectedCrc32c, serverCrc32c, storageStrategyId, tempStoragePath,
	cloudUploadId, partETagsMap, bizType, bizId, status, expireTime, createTime, updateTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		upload.ID, upload.UploadID, nullIfBlank(upload.UploadTaskID), upload.UserID, nullIfBlank(upload.ScopeID), upload.FileName, nullIfBlank(upload.ContentType), upload.FileSize, upload.ChunkSize, upload.TotalChunks, JSON(upload.UploadedChunks),
		JSON(upload.ChunkSha256Map), nullIfBlank(upload.FileSha256), nullIfBlank(upload.ExpectedCrc32c), nullIfBlank(upload.ServerCrc32c), upload.StorageStrategyID, nullIfBlank(upload.TempStoragePath),
		nullIfBlank(upload.CloudUploadID), JSON(upload.PartETagsMap), nullIfBlank(upload.BizType), nullIfZero(upload.BizID), upload.Status, upload.ExpireTime, time.Now(), time.Now())
	return err
}

func (r *Repository) GetChunkUpload(ctx context.Context, uploadID string) (*domain.ChunkUpload, error) {
	return r.getChunkUpload(ctx, uploadID, false)
}

func (r *Repository) GetChunkUploadForUpdate(ctx context.Context, uploadID string) (*domain.ChunkUpload, error) {
	return r.getChunkUpload(ctx, uploadID, true)
}

func (r *Repository) getChunkUpload(ctx context.Context, uploadID string, forUpdate bool) (*domain.ChunkUpload, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row chunkUploadRow
	query := `SELECT ` + chunkUploadColumns + ` FROM sys_file_chunk_upload WHERE uploadId=? LIMIT 1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(query), uploadID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get chunk upload: %w", err)
	}
	item := mapChunkUpload(row)
	return &item, nil
}

func (r *Repository) UpdateChunkUpload(ctx context.Context, upload *domain.ChunkUpload) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_chunk_upload SET uploadedChunks=?, chunkSha256Map=?, partETagsMap=?, fileSha256=?, serverCrc32c=?, status=?, updateTime=? WHERE uploadId=?`),
		JSON(upload.UploadedChunks), JSON(upload.ChunkSha256Map), JSON(upload.PartETagsMap), nullIfBlank(upload.FileSha256), nullIfBlank(upload.ServerCrc32c), upload.Status, time.Now(), upload.UploadID)
	return err
}

func (r *Repository) ExpireChunkUploads(ctx context.Context, ids []int64) (int64, error) {
	ids = uniquePositiveIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > 50 {
		return 0, fmt.Errorf("chunk upload maintenance set exceeds 50")
	}
	query, args, err := sqlx.In(`
UPDATE sys_file_chunk_upload
SET status=?, updateTime=?
WHERE id IN (?) AND status IN (?, ?)`,
		domain.ChunkStatusExpired, time.Now().UTC(), ids, domain.ChunkStatusInit, domain.ChunkStatusUploading)
	if err != nil {
		return 0, err
	}
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("expire chunk uploads: %w", err)
	}
	return result.RowsAffected()
}

func (r *Repository) ListActiveChunkUploads(ctx context.Context, userID int64, scopeID string) ([]domain.ChunkUpload, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	rows := []chunkUploadRow{}
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT `+chunkUploadColumns+` FROM sys_file_chunk_upload WHERE userId=? AND scopeId=? AND status IN (?, ?) AND expireTime > ? ORDER BY updateTime DESC`), userID, strings.TrimSpace(scopeID), domain.ChunkStatusInit, domain.ChunkStatusUploading, time.Now()); err != nil {
		return nil, err
	}
	result := make([]domain.ChunkUpload, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapChunkUpload(row))
	}
	return result, nil
}

func (r *Repository) ListExpiredChunkUploads(ctx context.Context, now time.Time, limit int) ([]domain.ChunkUpload, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows := []chunkUploadRow{}
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT `+chunkUploadColumns+` FROM sys_file_chunk_upload WHERE status IN (?, ?) AND expireTime <= ? ORDER BY expireTime ASC LIMIT ?`), domain.ChunkStatusInit, domain.ChunkStatusUploading, now, limit); err != nil {
		return nil, err
	}
	result := make([]domain.ChunkUpload, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapChunkUpload(row))
	}
	return result, nil
}

func (r *Repository) InsertProcessTask(ctx context.Context, task *domain.FileProcessTask) (int64, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	if task.ID <= 0 {
		task.ID = time.Now().UnixNano()
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_file_process_task (
	id, fileId, taskType, taskParams, pipelineId, nodeId, idempotencyKey, dedupKey, replayToken, dependsOn,
	status, retryCount, maxRetry, errorMsg, resultData, priority, mqMessageId, nextRetryTime, createTime, updateTime, startTime, finishTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		task.ID, task.FileID, task.TaskType, nullIfBlank(task.TaskParams), nullIfBlank(task.PipelineID), nullIfBlank(task.NodeID), nullIfBlank(task.IdempotencyKey), nullIfBlank(task.DedupKey), nullIfBlank(task.ReplayToken), nullIfBlank(task.DependsOn),
		task.Status, task.RetryCount, defaultInt(task.MaxRetry, 3), nullIfBlank(task.ErrorMsg), nullIfBlank(task.ResultData), task.Priority, nullIfBlank(task.MQMessageID), task.NextRetryTime, time.Now(), time.Now(), task.StartTime, task.FinishTime)
	if err != nil {
		return 0, fmt.Errorf("insert process task: %w", err)
	}
	return insertedID(ctx, exec, result, task.ID)
}

func (r *Repository) GetProcessTask(ctx context.Context, id int64) (*domain.FileProcessTask, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var row processTaskRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+processTaskColumns+` FROM sys_file_process_task WHERE id=? LIMIT 1`), id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get process task: %w", err)
	}
	item := mapProcessTask(row)
	return &item, nil
}

func (r *Repository) ListProcessTasksByIDs(ctx context.Context, ids []int64) ([]domain.FileProcessTask, error) {
	ids = uniquePositiveIDs(ids)
	if len(ids) == 0 {
		return []domain.FileProcessTask{}, nil
	}
	if len(ids) > 100 {
		return nil, fmt.Errorf("process task set exceeds 100")
	}
	query, args, err := sqlx.In(`SELECT `+processTaskColumns+` FROM sys_file_process_task WHERE id IN (?) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var rows []processTaskRow
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list process tasks by ids: %w", err)
	}
	result := make([]domain.FileProcessTask, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapProcessTask(row))
	}
	return result, nil
}

func (r *Repository) ResetProcessTasks(ctx context.Context, ids []int64) (int64, error) {
	ids = uniquePositiveIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > 100 {
		return 0, fmt.Errorf("process task set exceeds 100")
	}
	if store.SQLXFromContext(ctx) == nil {
		return 0, fmt.Errorf("reset process tasks requires active transaction")
	}
	query, args, err := sqlx.In(`
UPDATE sys_file_process_task
SET status = 0, errorMsg = NULL, resultData = NULL, updateTime = NOW()
WHERE id IN (?) AND status IN (?, ?)`, ids, domain.ProcessTaskFailed, domain.ProcessTaskPendingRetry)
	if err != nil {
		return 0, err
	}
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("reset process tasks: %w", err)
	}
	return result.RowsAffected()
}

func (r *Repository) ResetPendingRetryProcessTasks(ctx context.Context, ids []int64) (int64, error) {
	ids = uniquePositiveIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > 50 {
		return 0, fmt.Errorf("process task maintenance set exceeds 50")
	}
	query, args, err := sqlx.In(`
UPDATE sys_file_process_task
SET status=?, errorMsg=NULL, resultData=NULL, updateTime=?
WHERE id IN (?) AND status=?`,
		domain.ProcessTaskPending, time.Now().UTC(), ids, domain.ProcessTaskPendingRetry)
	if err != nil {
		return 0, err
	}
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("reset pending retry process tasks: %w", err)
	}
	return result.RowsAffected()
}

func (r *Repository) QueryProcessTasks(ctx context.Context, current, size int64, status *int, taskType string) (*domain.Page[domain.FileProcessTask], error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	where := []string{"1=1"}
	args := []any{}
	if status != nil {
		where = append(where, "status=?")
		args = append(args, *status)
	}
	if strings.TrimSpace(taskType) != "" {
		where = append(where, "taskType=?")
		args = append(args, strings.TrimSpace(taskType))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, exec.Rebind(`SELECT COUNT(1) FROM sys_file_process_task WHERE `+whereSQL), args...); err != nil {
		return nil, err
	}
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	page := &domain.Page[domain.FileProcessTask]{Current: current, Size: size, Total: total, Records: []domain.FileProcessTask{}}
	if total == 0 {
		return page, nil
	}
	rows := []processTaskRow{}
	listArgs := append(append([]any{}, args...), size, (current-1)*size)
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT `+processTaskColumns+` FROM sys_file_process_task WHERE `+whereSQL+` ORDER BY createTime DESC LIMIT ? OFFSET ?`), listArgs...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		page.Records = append(page.Records, mapProcessTask(row))
	}
	return page, nil
}

func (r *Repository) UpdateProcessTaskStatus(ctx context.Context, id int64, status int, errMsg, result string) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	var finishTime any
	if status == domain.ProcessTaskCompleted || status == domain.ProcessTaskFailed {
		finishTime = time.Now()
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_file_process_task SET status=?, errorMsg=?, resultData=?, updateTime=?, finishTime=? WHERE id=?`), status, nullIfBlank(errMsg), nullIfBlank(result), time.Now(), finishTime, id)
	return err
}

func (r *Repository) ClaimProcessTask(ctx context.Context, id int64) (bool, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE sys_file_process_task
SET status=?, errorMsg=NULL, updateTime=?, startTime=COALESCE(startTime, ?)
WHERE id=? AND status IN (?, ?, ?)`),
		domain.ProcessTaskProcessing, time.Now(), time.Now(), id, domain.ProcessTaskPending, domain.ProcessTaskPendingRetry, domain.ProcessTaskFailed)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *Repository) InsertProcessRun(ctx context.Context, run *domain.FileProcessRun) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	if run.ID <= 0 {
		run.ID = time.Now().UnixNano()
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_file_process_run (
	id, taskId, fileId, taskType, status, attempt, errorMsg, resultData, startedAt, finishedAt, createTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		run.ID, run.TaskID, run.FileID, run.TaskType, run.Status, run.Attempt, nullIfBlank(run.ErrorMsg), nullIfBlank(run.ResultData), run.StartedAt, run.FinishedAt, time.Now())
	return err
}

func (r *Repository) FindCompletedProcessTask(ctx context.Context, dedupKey, taskType string) (*domain.FileProcessTask, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(dedupKey) == "" || strings.TrimSpace(taskType) == "" {
		return nil, nil
	}
	var row processTaskRow
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(`SELECT `+processTaskColumns+` FROM sys_file_process_task WHERE dedupKey=? AND taskType=? AND status=? ORDER BY finishTime DESC LIMIT 1`), dedupKey, taskType, domain.ProcessTaskCompleted); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	task := mapProcessTask(row)
	return &task, nil
}

func (r *Repository) ListPendingRetryProcessTasks(ctx context.Context, now time.Time, limit int) ([]domain.FileProcessTask, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows := []processTaskRow{}
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT `+processTaskColumns+` FROM sys_file_process_task WHERE status=? AND (nextRetryTime IS NULL OR nextRetryTime <= ?) ORDER BY priority DESC, updateTime ASC LIMIT ?`), domain.ProcessTaskPendingRetry, now, limit); err != nil {
		return nil, err
	}
	result := make([]domain.FileProcessTask, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapProcessTask(row))
	}
	return result, nil
}

func (r *Repository) InsertBindingTask(ctx context.Context, task *domain.FileBindingTask) (int64, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	if task.ID <= 0 {
		task.ID = time.Now().UnixNano()
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO sys_file_binding_task (
	id, fileId, userId, bizType, bizId, bindingToken, channel, status, attemptCount, nextRetryTime, lastError, fileName, displayName, visitStrategy, accessScope, createTime, updateTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		task.ID, task.FileID, task.UserID, task.BizType, nullIfZero(task.BizID), task.BindingToken, task.Channel, task.Status, task.AttemptCount, task.NextRetryTime, nullIfBlank(task.LastError), nullIfBlank(task.FileName), nullIfBlank(task.DisplayName), nullIfBlank(task.VisitStrategy), nullIfBlank(task.AccessScope), time.Now(), time.Now())
	if err != nil {
		return 0, err
	}
	return insertedID(ctx, exec, result, task.ID)
}

func (r *Repository) ListRetryBindingTasks(ctx context.Context, now time.Time, limit int) ([]domain.FileBindingTask, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	type bindingRow struct {
		ID            int64          `db:"id"`
		FileID        int64          `db:"fileId"`
		UserID        int64          `db:"userId"`
		BizType       int            `db:"bizType"`
		BizID         sql.NullInt64  `db:"bizId"`
		BindingToken  string         `db:"bindingToken"`
		Channel       string         `db:"channel"`
		Status        string         `db:"status"`
		AttemptCount  int            `db:"attemptCount"`
		NextRetryTime sql.NullTime   `db:"nextRetryTime"`
		LastError     sql.NullString `db:"lastError"`
		FileName      sql.NullString `db:"fileName"`
		DisplayName   sql.NullString `db:"displayName"`
		VisitStrategy sql.NullString `db:"visitStrategy"`
		AccessScope   sql.NullString `db:"accessScope"`
		CreateTime    sql.NullTime   `db:"createTime"`
		UpdateTime    sql.NullTime   `db:"updateTime"`
	}
	rows := []bindingRow{}
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(`SELECT id, fileId, userId, bizType, bizId, bindingToken, channel, status, attemptCount, nextRetryTime, lastError, fileName, displayName, visitStrategy, accessScope, createTime, updateTime FROM sys_file_binding_task WHERE status=? AND (nextRetryTime IS NULL OR nextRetryTime <= ?) ORDER BY updateTime ASC LIMIT ?`), domain.BindingFailed, now, limit); err != nil {
		return nil, err
	}
	result := make([]domain.FileBindingTask, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.FileBindingTask{
			ID: row.ID, FileID: row.FileID, UserID: row.UserID, BizType: row.BizType, BizID: row.BizID.Int64,
			BindingToken: row.BindingToken, Channel: row.Channel, Status: row.Status, AttemptCount: row.AttemptCount,
			NextRetryTime: timePtr(row.NextRetryTime), LastError: row.LastError.String, FileName: row.FileName.String,
			DisplayName: row.DisplayName.String, VisitStrategy: row.VisitStrategy.String, AccessScope: row.AccessScope.String,
			CreateTime: row.CreateTime.Time, UpdateTime: row.UpdateTime.Time,
		})
	}
	return result, nil
}

func (r *Repository) MarkBindingTask(ctx context.Context, fileID int64, bindingToken, status, displayName, visitStrategy, accessScope, lastError string) error {
	exec, err := r.exec(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_file_binding_task SET status=?, displayName=?, visitStrategy=?, accessScope=?, lastError=?, updateTime=? WHERE fileId=? AND bindingToken=?`), status, nullIfBlank(displayName), nullIfBlank(visitStrategy), nullIfBlank(accessScope), nullIfBlank(lastError), time.Now(), fileID, bindingToken)
	return err
}

func (r *Repository) MarkBindingTasks(ctx context.Context, items []domain.FileBindingTask) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}
	if len(items) > 50 {
		return 0, fmt.Errorf("binding task maintenance set exceeds 50")
	}
	seen := make(map[int64]struct{}, len(items))
	normalized := make([]domain.FileBindingTask, 0, len(items))
	for _, item := range items {
		if item.ID <= 0 || strings.TrimSpace(item.BindingToken) == "" {
			return 0, fmt.Errorf("binding task id and token are required")
		}
		if item.Status != domain.BindingBound {
			return 0, fmt.Errorf("binding task maintenance target status must be bound")
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return 0, nil
	}

	var query strings.Builder
	args := []any{domain.BindingBound}
	query.WriteString(`UPDATE sys_file_binding_task SET status=?`)
	appendCase := func(column string, value func(domain.FileBindingTask) any) {
		query.WriteString(", ")
		query.WriteString(column)
		query.WriteString("=CASE id")
		for _, item := range normalized {
			query.WriteString(" WHEN ? THEN ?")
			args = append(args, item.ID, value(item))
		}
		query.WriteString(" ELSE ")
		query.WriteString(column)
		query.WriteString(" END")
	}
	appendCase("displayName", func(item domain.FileBindingTask) any { return nullIfBlank(item.DisplayName) })
	appendCase("visitStrategy", func(item domain.FileBindingTask) any { return nullIfBlank(item.VisitStrategy) })
	appendCase("accessScope", func(item domain.FileBindingTask) any { return nullIfBlank(item.AccessScope) })
	appendCase("lastError", func(item domain.FileBindingTask) any { return nullIfBlank(item.LastError) })
	query.WriteString(", updateTime=? WHERE status=? AND (")
	args = append(args, time.Now().UTC(), domain.BindingFailed)
	for index, item := range normalized {
		if index > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(id=? AND bindingToken=?)")
		args = append(args, item.ID, strings.TrimSpace(item.BindingToken))
	}
	query.WriteString(")")

	exec, err := r.exec(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(query.String()), args...)
	if err != nil {
		return 0, fmt.Errorf("mark binding tasks: %w", err)
	}
	return result.RowsAffected()
}

func (r *Repository) FileStats(ctx context.Context) (map[string]any, error) {
	exec, err := r.exec(ctx)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, exec.Rebind(`SELECT COUNT(1) FROM sys_file_info WHERE isDeleted=0`)); err != nil {
		return nil, err
	}
	var totalSize int64
	if err := sqlx.GetContext(ctx, exec, &totalSize, exec.Rebind(`SELECT COALESCE(SUM(fileSize),0) FROM sys_file_info WHERE isDeleted=0`)); err != nil {
		return nil, err
	}
	var imageCount int64
	if err := sqlx.GetContext(ctx, exec, &imageCount, exec.Rebind(`SELECT COUNT(1) FROM sys_file_info WHERE isDeleted=0 AND LOWER(contentType) LIKE 'image/%'`)); err != nil {
		return nil, err
	}
	var videoCount int64
	if err := sqlx.GetContext(ctx, exec, &videoCount, exec.Rebind(`SELECT COUNT(1) FROM sys_file_info WHERE isDeleted=0 AND LOWER(contentType) LIKE 'video/%'`)); err != nil {
		return nil, err
	}
	var docCount int64
	if err := sqlx.GetContext(ctx, exec, &docCount, exec.Rebind(`SELECT COUNT(1) FROM sys_file_info WHERE isDeleted=0 AND (
		LOWER(contentType) LIKE 'text/%'
		OR LOWER(contentType) IN ('application/pdf','application/msword','application/vnd.openxmlformats-officedocument.wordprocessingml.document','application/vnd.ms-excel','application/vnd.openxmlformats-officedocument.spreadsheetml.sheet')
	)`)); err != nil {
		return nil, err
	}
	return map[string]any{
		"total":              total,
		"totalCount":         total,
		"totalSize":          totalSize,
		"totalSizeFormatted": formatBytes(totalSize),
		"imageCount":         imageCount,
		"docCount":           docCount,
		"videoCount":         videoCount,
	}, nil
}

func formatBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(size)
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PB", value/1024)
}

func mapStorage(row storageStrategyRow) domain.StorageStrategy {
	return domain.StorageStrategy{ID: row.ID, StrategyName: row.StrategyName, ProviderType: row.ProviderType, IsDefault: row.IsDefault.Bool(), IsEnabled: row.IsEnabled.Bool(), RunState: row.RunState, Priority: row.Priority, ConfigCiphertext: row.ConfigCiphertext.String, ConfigEDEK: row.ConfigEDEK.String, WrapKeyRef: row.WrapKeyRef.String, HealthCheckURL: row.HealthCheckURL.String, HealthStatus: int(row.HealthStatus.Int64), LastHealthCheck: timePtr(row.LastHealthCheck), FailureCount: int(row.FailureCount.Int64), TotalRequests: int(row.TotalRequests.Int64), FailureRateThreshold: row.FailureRateThreshold.Float64, CreateTime: row.CreateTime.Time, UpdateTime: row.UpdateTime.Time, IsDeleted: row.IsDeleted.Int()}
}

func mapFile(row fileInfoRow) domain.FileInfo {
	return domain.FileInfo{ID: row.ID, FileInnerName: row.FileInnerName, FileSize: row.FileSize, FileSha256: row.FileSha256.String, FileCrc32c: row.FileCrc32c.String, HashAlgorithm: row.HashAlgorithm.String, ContentType: row.ContentType, StorageType: row.StorageType, StorageStrategyID: row.StorageStrategyID.Int64, StoragePath: row.StoragePath, Status: row.Status.String, ScanStatus: row.ScanStatus.String, IntegrityStatus: row.IntegrityStatus.String, CreateTime: row.CreateTime.Time, UpdateTime: row.UpdateTime.Time, IsDeleted: row.IsDeleted.Int(), DeletedTime: timePtr(row.DeletedTime)}
}

func mapReference(row fileReferenceRow) domain.FileReference {
	return domain.FileReference{ID: row.ID, FileID: row.FileID, UserID: row.UserID, ScopeID: row.ScopeID.String, DisplayName: row.DisplayName, BizType: row.BizType, BizID: row.BizID, VisitURL: row.VisitURL.String, AccessLevel: int(row.AccessLevel.Int64), VisitStrategy: row.VisitStrategy.String, AccessScope: row.AccessScope.String, CreateTime: row.CreateTime.Time, UpdateTime: row.UpdateTime.Time, IsDeleted: row.IsDeleted.Int()}
}

func mapUploadTask(row uploadTaskRow) domain.UploadTask {
	return domain.UploadTask{ID: row.ID, UserID: row.UserID, ScopeID: row.ScopeID.String, CredentialID: row.CredentialID.String, CredentialVersion: row.CredentialVersion, BizType: int(row.BizType.Int64), BizID: row.BizID.Int64, FileName: row.FileName.String, ContentType: row.ContentType.String, StorageStrategyID: row.StorageStrategyID.Int64, ObjectKeyStaging: row.ObjectKeyStaging, ObjectKeyClean: row.ObjectKeyClean, Status: row.Status, UploadMode: row.UploadMode.String, MultipartUploadID: row.MultipartUploadID.String, PartSize: row.PartSize.Int64, TotalParts: int(row.TotalParts.Int64), ExpectedSize: row.ExpectedSize.Int64, ExpectedSha256: row.ExpectedSha256.String, ExpectedCrc32c: row.ExpectedCrc32c.String, ActualSize: row.ActualSize.Int64, ETag: row.ETag.String, ServerCrc32c: row.ServerCrc32c.String, FailureCategory: row.FailureCategory.String, FailureReason: row.FailureReason.String, FileID: row.FileID.Int64, BindingToken: row.BindingToken.String, BindingChannel: row.BindingChannel.String, ExpireAt: timePtr(row.ExpireAt), ProtectedUntil: timePtr(row.ProtectedUntil), CredentialExpireAt: timePtr(row.CredentialExpireAt), RevokedAt: timePtr(row.RevokedAt), UserIP: row.UserIP.String, CreateTime: row.CreateTime.Time, UpdateTime: row.UpdateTime.Time}
}

func mapProcessTask(row processTaskRow) domain.FileProcessTask {
	return domain.FileProcessTask{ID: row.ID, FileID: row.FileID, TaskType: row.TaskType, TaskParams: row.TaskParams.String, Status: row.Status, RetryCount: row.RetryCount, MaxRetry: row.MaxRetry, ErrorMsg: row.ErrorMsg.String, ResultData: row.ResultData.String, Priority: row.Priority, MQMessageID: row.MQMessageID.String, NextRetryTime: timePtr(row.NextRetryTime), CreateTime: row.CreateTime.Time, UpdateTime: row.UpdateTime.Time, StartTime: timePtr(row.StartTime), FinishTime: timePtr(row.FinishTime)}
}

func mapChunkUpload(row chunkUploadRow) domain.ChunkUpload {
	return domain.ChunkUpload{
		ID:                row.ID,
		UploadID:          row.UploadID,
		UploadTaskID:      row.UploadTaskID.String,
		UserID:            row.UserID,
		ScopeID:           row.ScopeID.String,
		FileName:          row.FileName,
		FileSize:          row.FileSize,
		ChunkSize:         row.ChunkSize,
		TotalChunks:       row.TotalChunks,
		UploadedChunks:    parseIntSlice(row.UploadedChunks.String),
		ChunkSha256Map:    parseIntStringMap(row.ChunkSha256Map.String),
		PartETagsMap:      parseIntStringMap(row.PartETagsMap.String),
		FileSha256:        row.FileSha256.String,
		ExpectedCrc32c:    row.ExpectedCrc32c.String,
		ServerCrc32c:      row.ServerCrc32c.String,
		StorageStrategyID: row.StorageStrategyID,
		TempStoragePath:   row.TempStoragePath.String,
		CloudUploadID:     row.CloudUploadID.String,
		BizType:           row.BizType.String,
		BizID:             row.BizID.Int64,
		ContentType:       row.ContentType.String,
		Status:            row.Status,
		ExpireTime:        row.ExpireTime.Time,
		CreateTime:        row.CreateTime.Time,
		UpdateTime:        row.UpdateTime.Time,
	}
}

func parseIntSlice(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int{}
	}
	var values []int
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return values
	}
	return []int{}
}

func parseIntStringMap(raw string) map[int]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[int]string{}
	}
	var values map[int]string
	if err := json.Unmarshal([]byte(raw), &values); err == nil && values != nil {
		return values
	}
	stringKeyed := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &stringKeyed); err != nil {
		return map[int]string{}
	}
	result := make(map[int]string, len(stringKeyed))
	for key, value := range stringKeyed {
		var parsed int
		if _, err := fmt.Sscanf(key, "%d", &parsed); err == nil {
			result[parsed] = value
		}
	}
	return result
}

func insertedID(ctx context.Context, exec store.SQLX, result sql.Result, fallback int64) (int64, error) {
	if fallback > 0 {
		return fallback, nil
	}
	return lastInsertID(ctx, exec, result)
}

func lastInsertID(ctx context.Context, exec store.SQLX, result sql.Result) (int64, error) {
	if result != nil {
		if id, err := result.LastInsertId(); err == nil && id > 0 {
			return id, nil
		}
	}
	var id int64
	if err := sqlx.GetContext(ctx, exec, &id, exec.Rebind(`SELECT LAST_INSERT_ID()`)); err != nil {
		return 0, err
	}
	return id, nil
}

func timePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullIfZero(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func (r *Repository) boolValue(value bool) any {
	if r.postgres {
		return value
	}
	if value {
		return 1
	}
	return 0
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func defaultFloat(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func uniquePositiveIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func JSON(value any) string {
	b, _ := json.Marshal(value)
	return string(b)
}
