package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/bytedance/sonic"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	postgres bool
}

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("system config repository requires datasource provider")
	}
	return &Repository{
		db:       provider.SQLX(),
		postgres: strings.EqualFold(strings.TrimSpace(provider.Dialect()), "postgres"),
	}, nil
}

var configPostgresIdentifiers = []string{
	"groupCode", "groupName", "permissionCode", "sortOrder", "createTime", "updateTime", "isDeleted",
	"configCount", "groupId", "configKey", "configValue", "valueType", "configDesc", "isSensitive",
	"isSystemConfig", "requiredLogin", "uiWidget", "validationJson", "schemaVersion", "extJson",
	"isReadonly", "isEnabled", "effectType", "createdBy", "updatedBy", "operationType", "oldValue",
	"newValue", "oldValueProtected", "newValueProtected", "configId", "parentLogId", "relatedLogId", "operatorId",
	"operatorName", "operationTime", "operationReason", "appliedBy", "appliedTime", "oldAssetSnapshot", "newAssetSnapshot", "roleId",
	"canRead", "canWrite", "canDelete",
}

var configPostgresRenderer = store.MustNewPostgresRenderer(configPostgresIdentifiers, "isDeleted")

func (r *Repository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = configPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}

func (r *Repository) databaseBool(value int) any {
	if r.postgres {
		return value != 0
	}
	return value
}

type databaseBoolInt int

func (value *databaseBoolInt) Scan(src any) error {
	switch typed := src.(type) {
	case bool:
		if typed {
			*value = 1
		} else {
			*value = 0
		}
		return nil
	case int64:
		*value = databaseBoolInt(typed)
		return nil
	case []byte:
		if string(typed) == "1" || strings.EqualFold(string(typed), "true") {
			*value = 1
		} else {
			*value = 0
		}
		return nil
	case nil:
		*value = 0
		return nil
	default:
		return fmt.Errorf("scan boolean integer from %T", src)
	}
}

func (r *Repository) executor(ctx context.Context) store.SQLX {
	return store.SQLXExecutor(ctx, r.db)
}

func (r *Repository) requireExecutor(ctx context.Context) (store.SQLX, error) {
	exec := r.executor(ctx)
	if exec == nil {
		return nil, fmt.Errorf("system config repository datasource is not configured")
	}
	return exec, nil
}

type configGroupRow struct {
	ID             int64           `db:"id"`
	GroupCode      string          `db:"groupCode"`
	GroupName      string          `db:"groupName"`
	Module         sql.NullString  `db:"module"`
	PermissionCode sql.NullString  `db:"permissionCode"`
	SortOrder      int             `db:"sortOrder"`
	Status         int             `db:"status"`
	CreateTime     sql.NullTime    `db:"createTime"`
	UpdateTime     sql.NullTime    `db:"updateTime"`
	IsDeleted      databaseBoolInt `db:"isDeleted"`
	ConfigCount    sql.NullInt64   `db:"configCount"`
}

type configRow struct {
	ID             int64           `db:"id"`
	GroupID        int64           `db:"groupId"`
	ConfigKey      string          `db:"configKey"`
	ConfigValue    sql.NullString  `db:"configValue"`
	ValueType      string          `db:"valueType"`
	ConfigDesc     sql.NullString  `db:"configDesc"`
	IsSensitive    int             `db:"isSensitive"`
	IsSystemConfig databaseBoolInt `db:"isSystemConfig"`
	RequiredLogin  databaseBoolInt `db:"requiredLogin"`
	UIWidget       string          `db:"uiWidget"`
	ValidationJSON sql.NullString  `db:"validationJson"`
	Exposure       string          `db:"exposure"`
	Sensitivity    string          `db:"sensitivity"`
	SchemaVersion  int             `db:"schemaVersion"`
	Version        int64           `db:"version"`
	ExtJSON        sql.NullString  `db:"extJson"`
	IsReadonly     int             `db:"isReadonly"`
	IsEnabled      int             `db:"isEnabled"`
	EffectType     sql.NullString  `db:"effectType"`
	CreatedBy      sql.NullInt64   `db:"createdBy"`
	CreateTime     sql.NullTime    `db:"createTime"`
	UpdatedBy      sql.NullInt64   `db:"updatedBy"`
	UpdateTime     sql.NullTime    `db:"updateTime"`
	IsDeleted      databaseBoolInt `db:"isDeleted"`
	GroupCode      sql.NullString  `db:"groupCode"`
	GroupName      sql.NullString  `db:"groupName"`
}

type configChangeLogRow struct {
	ID                int64          `db:"id"`
	ConfigID          int64          `db:"configId"`
	ConfigKey         string         `db:"configKey"`
	OperationType     string         `db:"operationType"`
	OldValue          sql.NullString `db:"oldValue"`
	NewValue          sql.NullString `db:"newValue"`
	OldValueProtected bool           `db:"oldValueProtected"`
	NewValueProtected bool           `db:"newValueProtected"`
	EffectType        sql.NullString `db:"effectType"`
	Status            sql.NullString `db:"status"`
	ParentLogID       sql.NullInt64  `db:"parentLogId"`
	RelatedLogID      sql.NullInt64  `db:"relatedLogId"`
	OperatorID        int64          `db:"operatorId"`
	OperatorName      sql.NullString `db:"operatorName"`
	OperationTime     sql.NullTime   `db:"operationTime"`
	OperationReason   sql.NullString `db:"operationReason"`
	AppliedBy         sql.NullInt64  `db:"appliedBy"`
	AppliedTime       sql.NullTime   `db:"appliedTime"`
	OldAssetSnapshot  sql.NullString `db:"oldAssetSnapshot"`
	NewAssetSnapshot  sql.NullString `db:"newAssetSnapshot"`
}

type configScopeGrantRow struct {
	ID         int64          `db:"id"`
	RoleID     int64          `db:"roleId"`
	GroupCode  string         `db:"groupCode"`
	ConfigKey  sql.NullString `db:"configKey"`
	CanRead    int            `db:"canRead"`
	CanWrite   int            `db:"canWrite"`
	CanDelete  int            `db:"canDelete"`
	CreatedBy  sql.NullInt64  `db:"createdBy"`
	CreateTime sql.NullTime   `db:"createTime"`
	UpdatedBy  sql.NullInt64  `db:"updatedBy"`
	UpdateTime sql.NullTime   `db:"updateTime"`
	IsDeleted  int            `db:"isDeleted"`
}

func (r *Repository) FindGroupByID(ctx context.Context, id int64) (*domain.ConfigGroup, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row configGroupRow
	query := r.rebind(exec, `
SELECT id, groupCode, groupName, module, permissionCode, sortOrder, status, createTime, updateTime, isDeleted
FROM sys_config_group
WHERE id = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find config group by id: %w", err)
	}
	item, err := mapConfigGroup(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindGroupByCode(ctx context.Context, groupCode string) (*domain.ConfigGroup, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row configGroupRow
	query := r.rebind(exec, `
SELECT id, groupCode, groupName, module, permissionCode, sortOrder, status, createTime, updateTime, isDeleted
FROM sys_config_group
WHERE groupCode = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, strings.TrimSpace(groupCode)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find config group by code: %w", err)
	}
	item, err := mapConfigGroup(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListGroupsByCodes(ctx context.Context, groupCodes []string) ([]domain.ConfigGroup, error) {
	seen := make(map[string]struct{}, len(groupCodes))
	codes := make([]string, 0, len(groupCodes))
	for _, value := range groupCodes {
		code := strings.TrimSpace(value)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return []domain.ConfigGroup{}, nil
	}
	if len(codes) > 100 {
		return nil, fmt.Errorf("config group code set exceeds 100")
	}
	sort.Strings(codes)
	query, args, err := sqlx.In(`
SELECT id, groupCode, groupName, module, permissionCode, sortOrder, status, createTime, updateTime, isDeleted
FROM sys_config_group
WHERE groupCode IN (?) AND isDeleted = 0
ORDER BY groupCode ASC, id ASC
LIMIT ?`, codes, len(codes)+1)
	if err != nil {
		return nil, fmt.Errorf("build config groups by codes query: %w", err)
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []configGroupRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list config groups by codes: %w", err)
	}
	if len(rows) > len(codes) {
		return nil, fmt.Errorf("config group code lookup returned duplicate rows")
	}
	result := make([]domain.ConfigGroup, 0, len(rows))
	for _, row := range rows {
		item, mapErr := mapConfigGroup(row)
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) CountGroupByCode(ctx context.Context, groupCode string, excludeID int64) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	query := `SELECT COUNT(1) FROM sys_config_group WHERE groupCode = ? AND isDeleted = 0`
	args := []any{strings.TrimSpace(groupCode)}
	if excludeID > 0 {
		query += ` AND id <> ?`
		args = append(args, excludeID)
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, r.rebind(exec, query), args...); err != nil {
		return 0, fmt.Errorf("count config group by code: %w", err)
	}
	return count, nil
}

func (r *Repository) InsertGroup(ctx context.Context, item *domain.ConfigGroup) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	return r.insertWithID(ctx, exec, `
INSERT INTO sys_config_group (
	groupCode, groupName, module, permissionCode, sortOrder, status, createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"config group",
		strings.TrimSpace(item.GroupCode),
		nullIfBlank(item.GroupName),
		nullIfBlank(item.Module),
		nullIfBlank(item.PermissionCode),
		item.SortOrder,
		item.Status,
		timeOrNow(item.CreateTime),
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
	)
}

func (r *Repository) UpdateGroup(ctx context.Context, item *domain.ConfigGroup) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_config_group
SET groupCode = ?, groupName = ?, module = ?, permissionCode = ?, sortOrder = ?, status = ?, updateTime = ?, isDeleted = ?
WHERE id = ?`),
		strings.TrimSpace(item.GroupCode),
		nullIfBlank(item.GroupName),
		nullIfBlank(item.Module),
		nullIfBlank(item.PermissionCode),
		item.SortOrder,
		item.Status,
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
		item.ID,
	)
	if err != nil {
		return fmt.Errorf("update config group: %w", err)
	}
	return nil
}

func (r *Repository) QueryGroups(ctx context.Context, query domain.ConfigGroupPageQuery) (*domain.ConfigGroupPage, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	whereParts := []string{"g.isDeleted = 0"}
	args := make([]any, 0, 4)
	if groupCode := strings.TrimSpace(query.GroupCode); groupCode != "" {
		whereParts = append(whereParts, "g.groupCode LIKE ?")
		args = append(args, "%"+groupCode+"%")
	}
	if groupName := strings.TrimSpace(query.GroupName); groupName != "" {
		whereParts = append(whereParts, "g.groupName LIKE ?")
		args = append(args, "%"+groupName+"%")
	}
	if module := strings.TrimSpace(query.Module); module != "" {
		whereParts = append(whereParts, "g.module = ?")
		args = append(args, module)
	}
	if query.Status != nil {
		whereParts = append(whereParts, "g.status = ?")
		args = append(args, *query.Status)
	}
	whereSQL := strings.Join(whereParts, " AND ")
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(1) FROM sys_config_group g WHERE `+whereSQL), args...); err != nil {
		return nil, fmt.Errorf("count config groups: %w", err)
	}
	page := &domain.ConfigGroupPage{
		Current: query.Current,
		Size:    query.PageSize,
		Total:   total,
		Records: []domain.ConfigGroup{},
	}
	if total == 0 {
		return page, nil
	}
	offset := (query.Current - 1) * query.PageSize
	listQuery := r.rebind(exec, `
SELECT
	g.id, g.groupCode, g.groupName, g.module, g.permissionCode, g.sortOrder, g.status, g.createTime, g.updateTime, g.isDeleted,
	COALESCE(COUNT(c.id), 0) AS configCount
FROM sys_config_group g
LEFT JOIN sys_config c ON c.groupId = g.id AND c.isDeleted = 0
WHERE `+whereSQL+`
GROUP BY g.id, g.groupCode, g.groupName, g.module, g.permissionCode, g.sortOrder, g.status, g.createTime, g.updateTime, g.isDeleted
ORDER BY g.sortOrder ASC, g.createTime DESC, g.id ASC
LIMIT ? OFFSET ?`)
	listArgs := append(append([]any{}, args...), query.PageSize, offset)
	rows := make([]configGroupRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, listQuery, listArgs...); err != nil {
		return nil, fmt.Errorf("query config groups: %w", err)
	}
	page.Records = make([]domain.ConfigGroup, 0, len(rows))
	for _, row := range rows {
		item, mapErr := mapConfigGroup(row)
		if mapErr != nil {
			return nil, mapErr
		}
		page.Records = append(page.Records, item)
	}
	return page, nil
}

func (r *Repository) CountConfigsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	query := r.rebind(exec, `SELECT COUNT(1) FROM sys_config WHERE groupId = ? AND isDeleted = 0`)
	if err := sqlx.GetContext(ctx, exec, &count, query, groupID); err != nil {
		return 0, fmt.Errorf("count configs by group id: %w", err)
	}
	return count, nil
}

func (r *Repository) ShiftGroupSort(ctx context.Context, targetID int64, oldOrder, newOrder int) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	if oldOrder < newOrder {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_config_group
SET sortOrder = sortOrder - 1
WHERE isDeleted = 0 AND id <> ? AND sortOrder > ? AND sortOrder <= ?`), targetID, oldOrder, newOrder)
	} else {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_config_group
SET sortOrder = sortOrder + 1
WHERE isDeleted = 0 AND id <> ? AND sortOrder >= ? AND sortOrder < ?`), targetID, newOrder, oldOrder)
	}
	if err != nil {
		return fmt.Errorf("shift config group sort: %w", err)
	}
	return nil
}

func (r *Repository) FindConfigByID(ctx context.Context, id int64) (*domain.Config, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row configRow
	query := r.rebind(exec, `
SELECT
	c.id, c.groupId, c.configKey, c.configValue, c.valueType, c.configDesc, c.isSensitive, c.isSystemConfig,
	c.requiredLogin, c.uiWidget, c.validationJson, c.exposure, c.sensitivity, c.schemaVersion, c.version,
	c.extJson, c.isReadonly, c.isEnabled, c.effectType, c.createdBy, c.createTime,
	c.updatedBy, c.updateTime, c.isDeleted, g.groupCode, g.groupName
FROM sys_config c
LEFT JOIN sys_config_group g ON g.id = c.groupId
WHERE c.id = ? AND c.isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find config by id: %w", err)
	}
	item, err := mapConfig(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindConfigByGroupAndKey(ctx context.Context, groupID int64, configKey string, includeDisabled bool) (*domain.Config, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query := `
SELECT
	c.id, c.groupId, c.configKey, c.configValue, c.valueType, c.configDesc, c.isSensitive, c.isSystemConfig,
	c.requiredLogin, c.uiWidget, c.validationJson, c.exposure, c.sensitivity, c.schemaVersion, c.version,
	c.extJson, c.isReadonly, c.isEnabled, c.effectType, c.createdBy, c.createTime,
	c.updatedBy, c.updateTime, c.isDeleted, g.groupCode, g.groupName
FROM sys_config c
LEFT JOIN sys_config_group g ON g.id = c.groupId
WHERE c.groupId = ? AND c.configKey = ? AND c.isDeleted = 0`
	args := []any{groupID, strings.TrimSpace(configKey)}
	if !includeDisabled {
		query += ` AND c.isEnabled = 1`
	}
	query += ` LIMIT 1`
	var row configRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, query), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find config by group and key: %w", err)
	}
	item, err := mapConfig(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListConfigsByGroupAndKeys(ctx context.Context, refs []domain.ConfigKeyRef) ([]domain.Config, error) {
	seen := make(map[string]struct{}, len(refs))
	normalized := make([]domain.ConfigKeyRef, 0, len(refs))
	for _, ref := range refs {
		key := strings.TrimSpace(ref.ConfigKey)
		if ref.GroupID <= 0 || key == "" {
			return nil, fmt.Errorf("config group/key reference is invalid")
		}
		dedupeKey := fmt.Sprintf("%d\x00%s", ref.GroupID, key)
		if _, exists := seen[dedupeKey]; exists {
			continue
		}
		seen[dedupeKey] = struct{}{}
		normalized = append(normalized, domain.ConfigKeyRef{GroupID: ref.GroupID, ConfigKey: key})
	}
	if len(normalized) == 0 {
		return []domain.Config{}, nil
	}
	if len(normalized) > 100 {
		return nil, fmt.Errorf("config group/key set exceeds 100")
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].GroupID == normalized[j].GroupID {
			return normalized[i].ConfigKey < normalized[j].ConfigKey
		}
		return normalized[i].GroupID < normalized[j].GroupID
	})
	var query strings.Builder
	query.WriteString(`
SELECT
	c.id, c.groupId, c.configKey, c.configValue, c.valueType, c.configDesc, c.isSensitive, c.isSystemConfig,
	c.requiredLogin, c.uiWidget, c.validationJson, c.exposure, c.sensitivity, c.schemaVersion, c.version,
	c.extJson, c.isReadonly, c.isEnabled, c.effectType, c.createdBy, c.createTime,
	c.updatedBy, c.updateTime, c.isDeleted, g.groupCode, g.groupName
FROM sys_config c
LEFT JOIN sys_config_group g ON g.id = c.groupId
WHERE c.isDeleted = 0 AND c.isEnabled = 1 AND (`)
	args := make([]any, 0, len(normalized)*2+1)
	for index, ref := range normalized {
		if index > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(c.groupId = ? AND c.configKey = ?)")
		args = append(args, ref.GroupID, ref.ConfigKey)
	}
	query.WriteString(`)
ORDER BY c.groupId ASC, c.configKey ASC, c.id ASC
LIMIT ?`)
	args = append(args, len(normalized)+1)
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []configRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query.String()), args...); err != nil {
		return nil, fmt.Errorf("list configs by group and keys: %w", err)
	}
	if len(rows) > len(normalized) {
		return nil, fmt.Errorf("config group/key lookup returned duplicate rows")
	}
	result := make([]domain.Config, 0, len(rows))
	for _, row := range rows {
		item, mapErr := mapConfig(row)
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) FindConfigsByRawKey(ctx context.Context, configKey string, includeDisabled bool) ([]domain.Config, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query := `
SELECT
	c.id, c.groupId, c.configKey, c.configValue, c.valueType, c.configDesc, c.isSensitive, c.isSystemConfig,
	c.requiredLogin, c.uiWidget, c.validationJson, c.exposure, c.sensitivity, c.schemaVersion, c.version,
	c.extJson, c.isReadonly, c.isEnabled, c.effectType, c.createdBy, c.createTime,
	c.updatedBy, c.updateTime, c.isDeleted, g.groupCode, g.groupName
FROM sys_config c
LEFT JOIN sys_config_group g ON g.id = c.groupId
WHERE c.configKey = ? AND c.isDeleted = 0`
	args := []any{strings.TrimSpace(configKey)}
	if !includeDisabled {
		query += ` AND c.isEnabled = 1`
	}
	query += ` ORDER BY c.id ASC`
	rows := make([]configRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("find configs by raw key: %w", err)
	}
	result := make([]domain.Config, 0, len(rows))
	for _, row := range rows {
		item, mapErr := mapConfig(row)
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) CountConfigByGroupAndKey(ctx context.Context, groupID int64, configKey string, excludeID int64) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	query := `SELECT COUNT(1) FROM sys_config WHERE groupId = ? AND configKey = ? AND isDeleted = 0`
	args := []any{groupID, strings.TrimSpace(configKey)}
	if excludeID > 0 {
		query += ` AND id <> ?`
		args = append(args, excludeID)
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, r.rebind(exec, query), args...); err != nil {
		return 0, fmt.Errorf("count config by group and key: %w", err)
	}
	return count, nil
}

func (r *Repository) InsertConfig(ctx context.Context, item *domain.Config) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	extJSON, err := marshalExtJSON(item.ExtJSON)
	if err != nil {
		return 0, err
	}
	validationJSON, err := marshalValidation(item.Validation)
	if err != nil {
		return 0, err
	}
	return r.insertWithID(ctx, exec, `
INSERT INTO sys_config (
	groupId, configKey, configValue, valueType, configDesc, isSensitive, isSystemConfig, requiredLogin,
	uiWidget, validationJson, exposure, sensitivity, schemaVersion, version,
	extJson, isReadonly, isEnabled, effectType, createdBy, createTime, updatedBy, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"config",
		item.GroupID,
		strings.TrimSpace(item.ConfigKey),
		persistedConfigValue(item.ConfigValue),
		strings.TrimSpace(item.ValueType),
		nullIfBlank(item.ConfigDesc),
		item.IsSensitive,
		r.databaseBool(item.IsSystemConfig),
		r.databaseBool(item.RequiredLogin),
		item.UIWidget,
		validationJSON,
		item.Exposure,
		item.Sensitivity,
		item.SchemaVersion,
		item.Version,
		extJSON,
		item.IsReadonly,
		item.IsEnabled,
		nullIfBlank(item.EffectType),
		persistedActorID(item.CreatedBy),
		timeOrNow(item.CreateTime),
		persistedActorID(item.UpdatedBy),
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
	)
}

func (r *Repository) UpdateConfig(ctx context.Context, item *domain.Config) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	extJSON, err := marshalExtJSON(item.ExtJSON)
	if err != nil {
		return err
	}
	validationJSON, err := marshalValidation(item.Validation)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_config
SET groupId = ?, configKey = ?, configValue = ?, valueType = ?, configDesc = ?, isSensitive = ?, isSystemConfig = ?,
	requiredLogin = ?, uiWidget = ?, validationJson = ?, exposure = ?, sensitivity = ?, schemaVersion = ?,
	version = ?, extJson = ?, isReadonly = ?, isEnabled = ?, effectType = ?, updatedBy = ?, updateTime = ?, isDeleted = ?
WHERE id = ? AND version = ? AND isDeleted = 0`),
		item.GroupID,
		strings.TrimSpace(item.ConfigKey),
		persistedConfigValue(item.ConfigValue),
		strings.TrimSpace(item.ValueType),
		nullIfBlank(item.ConfigDesc),
		item.IsSensitive,
		r.databaseBool(item.IsSystemConfig),
		r.databaseBool(item.RequiredLogin),
		item.UIWidget,
		validationJSON,
		item.Exposure,
		item.Sensitivity,
		item.SchemaVersion,
		item.Version+1,
		extJSON,
		item.IsReadonly,
		item.IsEnabled,
		nullIfBlank(item.EffectType),
		persistedActorID(item.UpdatedBy),
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
		item.ID,
		item.Version,
	)
	if err != nil {
		return fmt.Errorf("update config: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect config update result: %w", err)
	}
	if rows != 1 {
		return apperrors.ObjectState("配置已被其他管理员更新，请刷新后重试").WithDetails(map[string]any{
			"reasonCode": "CONFIG_VERSION_CONFLICT",
			"configId":   item.ID,
			"version":    item.Version,
		})
	}
	item.Version++
	return nil
}

func (r *Repository) QueryConfigs(ctx context.Context, query domain.ConfigPageQuery) (*domain.ConfigPage, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	whereParts := []string{"c.isDeleted = 0"}
	args := make([]any, 0, 6)
	if query.GroupID != nil {
		whereParts = append(whereParts, "c.groupId = ?")
		args = append(args, *query.GroupID)
	}
	searchText := strings.TrimSpace(query.SearchText)
	if searchText == "" {
		searchText = strings.TrimSpace(query.Keyword)
	}
	switch strings.ToLower(strings.TrimSpace(query.SearchType)) {
	case "key":
		if searchText != "" {
			whereParts = append(whereParts, "c.configKey LIKE ?")
			args = append(args, "%"+searchText+"%")
		}
	case "label":
		if searchText != "" {
			whereParts = append(whereParts, "c.configDesc LIKE ?")
			args = append(args, "%"+searchText+"%")
		}
	default:
		if searchText != "" {
			whereParts = append(whereParts, "(c.configKey LIKE ? OR c.configDesc LIKE ?)")
			args = append(args, "%"+searchText+"%", "%"+searchText+"%")
		}
	}
	if query.IsEnabled != nil {
		whereParts = append(whereParts, "c.isEnabled = ?")
		args = append(args, *query.IsEnabled)
	}
	whereSQL := strings.Join(whereParts, " AND ")
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(1) FROM sys_config c WHERE `+whereSQL), args...); err != nil {
		return nil, fmt.Errorf("count configs: %w", err)
	}
	page := &domain.ConfigPage{
		Current: query.Current,
		Size:    query.PageSize,
		Total:   total,
		Records: []domain.Config{},
	}
	if total == 0 {
		return page, nil
	}
	offset := (query.Current - 1) * query.PageSize
	listQuery := r.rebind(exec, `
SELECT
	c.id, c.groupId, c.configKey, c.configValue, c.valueType, c.configDesc, c.isSensitive, c.isSystemConfig,
	c.requiredLogin, c.uiWidget, c.validationJson, c.exposure, c.sensitivity, c.schemaVersion, c.version,
	c.extJson, c.isReadonly, c.isEnabled, c.effectType, c.createdBy, c.createTime,
	c.updatedBy, c.updateTime, c.isDeleted, g.groupCode, g.groupName
FROM sys_config c
LEFT JOIN sys_config_group g ON g.id = c.groupId
WHERE `+whereSQL+`
ORDER BY c.createTime DESC, c.id DESC
LIMIT ? OFFSET ?`)
	listArgs := append(append([]any{}, args...), query.PageSize, offset)
	rows := make([]configRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, listQuery, listArgs...); err != nil {
		return nil, fmt.Errorf("query configs: %w", err)
	}
	page.Records = make([]domain.Config, 0, len(rows))
	for _, row := range rows {
		item, mapErr := mapConfig(row)
		if mapErr != nil {
			return nil, mapErr
		}
		page.Records = append(page.Records, item)
	}
	return page, nil
}

func (r *Repository) ListConfigsByIDs(ctx context.Context, ids []int64) ([]domain.Config, error) {
	if len(ids) == 0 {
		return []domain.Config{}, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT
	c.id, c.groupId, c.configKey, c.configValue, c.valueType, c.configDesc, c.isSensitive, c.isSystemConfig,
	c.requiredLogin, c.uiWidget, c.validationJson, c.exposure, c.sensitivity, c.schemaVersion, c.version,
	c.extJson, c.isReadonly, c.isEnabled, c.effectType, c.createdBy, c.createTime,
	c.updatedBy, c.updateTime, c.isDeleted, g.groupCode, g.groupName
FROM sys_config c
LEFT JOIN sys_config_group g ON g.id = c.groupId
WHERE c.id IN (?)`, ids)
	if err != nil {
		return nil, fmt.Errorf("build list configs by ids query: %w", err)
	}
	rows := make([]configRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list configs by ids: %w", err)
	}
	result := make([]domain.Config, 0, len(rows))
	for _, row := range rows {
		item, mapErr := mapConfig(row)
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) InsertChangeLog(ctx context.Context, item *domain.ConfigChangeLog) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	return r.insertWithID(ctx, exec, `
INSERT INTO sys_config_change_log (
	configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"config change log",
		item.ConfigID,
		strings.TrimSpace(item.ConfigKey),
		strings.TrimSpace(item.OperationType),
		nullIfBlank(item.OldValue),
		nullIfBlank(item.NewValue),
		item.OldValueProtected || item.OldValue == "[PROTECTED]",
		item.NewValueProtected || item.NewValue == "[PROTECTED]",
		nullIfBlank(item.EffectType),
		nullIfBlank(item.Status),
		nullIfZeroPtr(item.ParentLogID),
		nullIfZeroPtr(item.RelatedLogID),
		item.OperatorID,
		nullIfBlank(item.OperatorName),
		timeOrNow(item.OperationTime),
		nullIfBlank(item.OperationReason),
		nullIfZeroPtr(item.AppliedBy),
		timeOrNil(item.AppliedTime),
		privateAssetSnapshotPayload(item, true),
		privateAssetSnapshotPayload(item, false),
	)
}

func (r *Repository) UpdateChangeLog(ctx context.Context, item *domain.ConfigChangeLog) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_config_change_log
SET configId = ?, configKey = ?, operationType = ?, oldValue = ?, newValue = ?, oldValueProtected = ?, newValueProtected = ?, effectType = ?, status = ?,
	parentLogId = ?, relatedLogId = ?, operatorId = ?, operatorName = ?, operationTime = ?, operationReason = ?,
	appliedBy = ?, appliedTime = ?, oldAssetSnapshot = ?, newAssetSnapshot = ?
WHERE id = ?`),
		item.ConfigID,
		strings.TrimSpace(item.ConfigKey),
		strings.TrimSpace(item.OperationType),
		nullIfBlank(item.OldValue),
		nullIfBlank(item.NewValue),
		item.OldValueProtected || item.OldValue == "[PROTECTED]",
		item.NewValueProtected || item.NewValue == "[PROTECTED]",
		nullIfBlank(item.EffectType),
		nullIfBlank(item.Status),
		nullIfZeroPtr(item.ParentLogID),
		nullIfZeroPtr(item.RelatedLogID),
		item.OperatorID,
		nullIfBlank(item.OperatorName),
		timeOrNow(item.OperationTime),
		nullIfBlank(item.OperationReason),
		nullIfZeroPtr(item.AppliedBy),
		timeOrNil(item.AppliedTime),
		privateAssetSnapshotPayload(item, true),
		privateAssetSnapshotPayload(item, false),
		item.ID,
	)
	if err != nil {
		return fmt.Errorf("update config change log: %w", err)
	}
	return nil
}

func (r *Repository) ClaimPendingChangeLog(ctx context.Context, id int64, appliedBy int64, appliedTime time.Time, operatorName string) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_config_change_log
SET status = 'applied', appliedBy = ?, appliedTime = ?, operatorName = ?
WHERE id = ? AND status = 'pending'`), persistedActorID(appliedBy), appliedTime.UTC(), nullIfBlank(operatorName), id)
	if err != nil {
		return false, fmt.Errorf("claim pending config change log: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect pending config claim result: %w", err)
	}
	return rows == 1, nil
}

func (r *Repository) ApplyPendingConfigBatch(ctx context.Context, items []domain.PendingConfigApply) ([]int64, error) {
	if len(items) == 0 {
		return []int64{}, nil
	}
	if len(items) > 50 {
		return nil, fmt.Errorf("apply pending config batch exceeds 50 items")
	}
	if store.SQLXFromContext(ctx) == nil {
		return nil, fmt.Errorf("apply pending config batch requires an active transaction")
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	byLogID := make(map[int64]domain.PendingConfigApply, len(items))
	logIDs := make([]int64, 0, len(items))
	for _, item := range items {
		if item.PendingLogID <= 0 || item.Config.ID <= 0 {
			return nil, fmt.Errorf("apply pending config batch contains invalid identifiers")
		}
		if _, exists := byLogID[item.PendingLogID]; exists {
			return nil, fmt.Errorf("apply pending config batch contains duplicate log id %d", item.PendingLogID)
		}
		byLogID[item.PendingLogID] = item
		logIDs = append(logIDs, item.PendingLogID)
	}
	lockQuery, lockArgs, err := sqlx.In(`
SELECT id
FROM sys_config_change_log
WHERE id IN (?) AND status = 'pending'
ORDER BY id
FOR UPDATE`, logIDs)
	if err != nil {
		return nil, fmt.Errorf("build pending config batch lock: %w", err)
	}
	claimedIDs := make([]int64, 0, len(items))
	if err := sqlx.SelectContext(ctx, exec, &claimedIDs, r.rebind(exec, lockQuery), lockArgs...); err != nil {
		return nil, fmt.Errorf("lock pending config batch: %w", err)
	}
	if len(claimedIDs) == 0 {
		return []int64{}, nil
	}
	sort.Slice(claimedIDs, func(i, j int) bool { return claimedIDs[i] < claimedIDs[j] })
	winners := make([]domain.PendingConfigApply, 0, len(claimedIDs))
	configIDs := make(map[int64]struct{}, len(claimedIDs))
	for _, id := range claimedIDs {
		item, ok := byLogID[id]
		if !ok {
			return nil, fmt.Errorf("locked unexpected pending config log %d", id)
		}
		if _, duplicate := configIDs[item.Config.ID]; duplicate {
			return nil, fmt.Errorf("apply pending config batch contains duplicate config id %d", item.Config.ID)
		}
		configIDs[item.Config.ID] = struct{}{}
		winners = append(winners, item)
	}

	var update strings.Builder
	update.WriteString("UPDATE sys_config SET ")
	updateArgs := make([]any, 0, len(winners)*12)
	writeCase := func(column string, value func(domain.PendingConfigApply) (any, error)) error {
		update.WriteString(column)
		update.WriteString(" = CASE id")
		for _, item := range winners {
			resolved, valueErr := value(item)
			if valueErr != nil {
				return valueErr
			}
			update.WriteString(" WHEN ? THEN ?")
			updateArgs = append(updateArgs, item.Config.ID, resolved)
		}
		update.WriteString(" ELSE ")
		update.WriteString(column)
		update.WriteString(" END, ")
		return nil
	}
	if err := writeCase("configValue", func(item domain.PendingConfigApply) (any, error) {
		return persistedConfigValue(item.Config.ConfigValue), nil
	}); err != nil {
		return nil, err
	}
	if err := writeCase("extJson", func(item domain.PendingConfigApply) (any, error) {
		return marshalExtJSON(item.Config.ExtJSON)
	}); err != nil {
		return nil, err
	}
	if err := writeCase("updatedBy", func(item domain.PendingConfigApply) (any, error) {
		return persistedActorID(item.Config.UpdatedBy), nil
	}); err != nil {
		return nil, err
	}
	if err := writeCase("updateTime", func(item domain.PendingConfigApply) (any, error) {
		return timeOrNow(item.Config.UpdateTime), nil
	}); err != nil {
		return nil, err
	}
	update.WriteString("version = version + 1 WHERE isDeleted = 0 AND (")
	for idx, item := range winners {
		if idx > 0 {
			update.WriteString(" OR ")
		}
		update.WriteString("(id = ? AND version = ?)")
		updateArgs = append(updateArgs, item.Config.ID, item.Config.Version)
	}
	update.WriteString(")")
	result, err := exec.ExecContext(ctx, r.rebind(exec, update.String()), updateArgs...)
	if err != nil {
		return nil, fmt.Errorf("batch update pending configs: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("inspect batch pending config update: %w", err)
	}
	if updated != int64(len(winners)) {
		return nil, apperrors.ObjectState("配置已被其他管理员更新，请刷新后重试").WithDetails(map[string]any{
			"reasonCode": "CONFIG_VERSION_CONFLICT",
		})
	}

	claimQuery, claimArgs, err := sqlx.In(`
UPDATE sys_config_change_log
SET status = 'applied', appliedBy = ?, appliedTime = ?, operatorName = ?
WHERE id IN (?) AND status = 'pending'`,
		persistedActorID(winners[0].ApplyLog.OperatorID),
		timeOrNow(winners[0].ApplyLog.AppliedTime),
		nullIfBlank(winners[0].ApplyLog.OperatorName),
		claimedIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build pending config batch claim: %w", err)
	}
	result, err = exec.ExecContext(ctx, r.rebind(exec, claimQuery), claimArgs...)
	if err != nil {
		return nil, fmt.Errorf("claim pending config batch: %w", err)
	}
	claimed, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("inspect pending config batch claim: %w", err)
	}
	if claimed != int64(len(winners)) {
		return nil, fmt.Errorf("pending config batch claim changed %d rows, expected %d", claimed, len(winners))
	}

	var audit strings.Builder
	audit.WriteString(`
INSERT INTO sys_config_change_log (
	configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
) VALUES `)
	auditArgs := make([]any, 0, len(winners)*19)
	for idx, item := range winners {
		if idx > 0 {
			audit.WriteString(",")
		}
		audit.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		log := item.ApplyLog
		auditArgs = append(auditArgs,
			log.ConfigID,
			strings.TrimSpace(log.ConfigKey),
			strings.TrimSpace(log.OperationType),
			nullIfBlank(log.OldValue),
			nullIfBlank(log.NewValue),
			log.OldValueProtected || log.OldValue == "[PROTECTED]",
			log.NewValueProtected || log.NewValue == "[PROTECTED]",
			nullIfBlank(log.EffectType),
			nullIfBlank(log.Status),
			nullIfZeroPtr(log.ParentLogID),
			nullIfZeroPtr(log.RelatedLogID),
			log.OperatorID,
			nullIfBlank(log.OperatorName),
			timeOrNow(log.OperationTime),
			nullIfBlank(log.OperationReason),
			nullIfZeroPtr(log.AppliedBy),
			timeOrNil(log.AppliedTime),
			privateAssetSnapshotPayload(&log, true),
			privateAssetSnapshotPayload(&log, false),
		)
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, audit.String()), auditArgs...); err != nil {
		return nil, fmt.Errorf("insert pending config apply audit batch: %w", err)
	}
	return claimedIDs, nil
}

func (r *Repository) FindChangeLogByID(ctx context.Context, id int64) (*domain.ConfigChangeLog, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row configChangeLogRow
	query := r.rebind(exec, `
SELECT
	id, configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
FROM sys_config_change_log
WHERE id = ?
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find config change log by id: %w", err)
	}
	item := mapChangeLog(row)
	return &item, nil
}

func (r *Repository) ListPendingLogs(ctx context.Context) ([]domain.ConfigChangeLog, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]configChangeLogRow, 0)
	query := r.rebind(exec, `
SELECT
	id, configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
FROM sys_config_change_log
WHERE status = 'pending'
ORDER BY operationTime ASC, id ASC
LIMIT ?`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, 501); err != nil {
		return nil, fmt.Errorf("list pending config change logs: %w", err)
	}
	result := make([]domain.ConfigChangeLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapChangeLog(row))
	}
	return result, nil
}

func (r *Repository) ListHistoryByConfigID(ctx context.Context, configID int64, limit int) ([]domain.ConfigChangeLog, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query := `
SELECT
	id, configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
FROM sys_config_change_log
WHERE configId = ?
ORDER BY operationTime DESC`
	args := []any{configID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows := make([]configChangeLogRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list config history: %w", err)
	}
	result := make([]domain.ConfigChangeLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapChangeLog(row))
	}
	return result, nil
}

func (r *Repository) ListAuditLogs(ctx context.Context, query domain.AuditLogQuery) ([]domain.ConfigChangeLog, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	whereParts := []string{"1=1"}
	args := make([]any, 0, 6)
	if query.ConfigID != nil {
		whereParts = append(whereParts, "configId = ?")
		args = append(args, *query.ConfigID)
	}
	if strings.TrimSpace(query.OperationType) != "" {
		whereParts = append(whereParts, "operationType = ?")
		args = append(args, strings.TrimSpace(query.OperationType))
	}
	if strings.TrimSpace(query.Status) != "" {
		whereParts = append(whereParts, "status = ?")
		args = append(args, strings.TrimSpace(query.Status))
	}
	if query.StartTime != nil {
		whereParts = append(whereParts, "operationTime >= ?")
		args = append(args, query.StartTime.UTC())
	}
	if query.EndTime != nil {
		whereParts = append(whereParts, "operationTime <= ?")
		args = append(args, query.EndTime.UTC())
	}
	sqlText := `
SELECT
	id, configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
FROM sys_config_change_log
WHERE ` + strings.Join(whereParts, " AND ") + `
ORDER BY operationTime DESC`
	if query.Limit > 0 {
		sqlText += ` LIMIT ?`
		args = append(args, query.Limit)
	}
	rows := make([]configChangeLogRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, sqlText), args...); err != nil {
		return nil, fmt.Errorf("list config audit logs: %w", err)
	}
	result := make([]domain.ConfigChangeLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapChangeLog(row))
	}
	return result, nil
}

func (r *Repository) ListChangeLogsByIDs(ctx context.Context, ids []int64) ([]domain.ConfigChangeLog, error) {
	if len(ids) == 0 {
		return []domain.ConfigChangeLog{}, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT
	id, configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
FROM sys_config_change_log
WHERE id IN (?)`, ids)
	if err != nil {
		return nil, fmt.Errorf("build change log ids query: %w", err)
	}
	rows := make([]configChangeLogRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list change logs by ids: %w", err)
	}
	result := make([]domain.ConfigChangeLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapChangeLog(row))
	}
	return result, nil
}

func (r *Repository) ListChangeLogsReferencing(ctx context.Context, ids []int64) ([]domain.ConfigChangeLog, error) {
	if len(ids) == 0 {
		return []domain.ConfigChangeLog{}, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT
	id, configId, configKey, operationType, oldValue, newValue, oldValueProtected, newValueProtected, effectType, status, parentLogId, relatedLogId,
	operatorId, operatorName, operationTime, operationReason, appliedBy, appliedTime, oldAssetSnapshot, newAssetSnapshot
FROM sys_config_change_log
WHERE parentLogId IN (?) OR relatedLogId IN (?)`, ids, ids)
	if err != nil {
		return nil, fmt.Errorf("build referencing logs query: %w", err)
	}
	rows := make([]configChangeLogRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list referencing change logs: %w", err)
	}
	result := make([]domain.ConfigChangeLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapChangeLog(row))
	}
	return result, nil
}

func (r *Repository) ListConfigScopeGrantsByRoleIDs(ctx context.Context, roleIDs []int64) ([]domain.ConfigScopeGrant, error) {
	ids := uniquePositiveInt64(roleIDs)
	if len(ids) == 0 {
		return []domain.ConfigScopeGrant{}, nil
	}
	if len(ids) > 100 {
		return nil, fmt.Errorf("config scope role set exceeds 100")
	}
	query, args, err := sqlx.In(`
SELECT id, roleId, groupCode, configKey, canRead, canWrite, canDelete, createdBy, createTime, updatedBy, updateTime, isDeleted
FROM sys_role_config_scope
WHERE roleId IN (?) AND isDeleted = 0
ORDER BY roleId ASC, groupCode ASC, configKey ASC
LIMIT ?`, ids, 1001)
	if err != nil {
		return nil, err
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]configScopeGrantRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list config scope grants by role ids: %w", err)
	}
	if len(rows) > 1000 {
		return nil, fmt.Errorf("config scope grant result exceeds 1000")
	}
	return mapConfigScopeGrantRows(rows), nil
}

func (r *Repository) ListConfigScopeGrantsByRoleID(ctx context.Context, roleID int64) ([]domain.ConfigScopeGrant, error) {
	if roleID <= 0 {
		return []domain.ConfigScopeGrant{}, nil
	}
	return r.ListConfigScopeGrantsByRoleIDs(ctx, []int64{roleID})
}

func (r *Repository) ReplaceRoleConfigScopes(ctx context.Context, roleID int64, grants []domain.ConfigScopeGrant, operatorID int64, nextID func() int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `DELETE FROM sys_role_config_scope WHERE roleId = ?`), roleID); err != nil {
		return fmt.Errorf("delete role config scopes: %w", err)
	}
	normalized := normalizeConfigScopeGrants(roleID, grants, operatorID, nextID)
	if len(normalized) == 0 {
		return nil
	}
	valueClause := `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	clauses := make([]string, 0, len(normalized))
	args := make([]any, 0, len(normalized)*10)
	now := time.Now().UTC()
	for _, item := range normalized {
		clauses = append(clauses, valueClause)
		args = append(args,
			item.ID,
			item.RoleID,
			item.GroupCode,
			item.ConfigKey,
			item.CanRead,
			item.CanWrite,
			item.CanDelete,
			item.CreatedBy,
			now,
			item.UpdatedBy,
		)
	}
	query := r.rebind(exec, `
INSERT INTO sys_role_config_scope (
	id, roleId, groupCode, configKey, canRead, canWrite, canDelete, createdBy, createTime, updatedBy
) VALUES `+strings.Join(clauses, ", "))
	if _, err := exec.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("replace role config scopes: %w", err)
	}
	return nil
}

func mapConfigScopeGrantRows(rows []configScopeGrantRow) []domain.ConfigScopeGrant {
	result := make([]domain.ConfigScopeGrant, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.ConfigScopeGrant{
			ID:         row.ID,
			RoleID:     row.RoleID,
			GroupCode:  strings.TrimSpace(row.GroupCode),
			ConfigKey:  strings.TrimSpace(nullableString(row.ConfigKey)),
			CanRead:    normalizeFlag(row.CanRead),
			CanWrite:   normalizeFlag(row.CanWrite),
			CanDelete:  normalizeFlag(row.CanDelete),
			CreatedBy:  nullableInt64(row.CreatedBy),
			CreateTime: nullableTime(row.CreateTime),
			UpdatedBy:  nullableInt64(row.UpdatedBy),
			UpdateTime: nullableTime(row.UpdateTime),
			IsDeleted:  normalizeFlag(row.IsDeleted),
		})
	}
	return result
}

func normalizeConfigScopeGrants(roleID int64, grants []domain.ConfigScopeGrant, operatorID int64, nextID func() int64) []domain.ConfigScopeGrant {
	seen := make(map[string]struct{}, len(grants))
	result := make([]domain.ConfigScopeGrant, 0, len(grants))
	if nextID == nil {
		nextID = func() int64 { return time.Now().UTC().UnixNano() }
	}
	for _, grant := range grants {
		groupCode := strings.TrimSpace(grant.GroupCode)
		if groupCode == "" {
			continue
		}
		configKey := strings.TrimSpace(grant.ConfigKey)
		key := groupCode + "\x00" + configKey
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, domain.ConfigScopeGrant{
			ID:        nextID(),
			RoleID:    roleID,
			GroupCode: groupCode,
			ConfigKey: configKey,
			CanRead:   normalizeFlag(grant.CanRead),
			CanWrite:  normalizeFlag(grant.CanWrite),
			CanDelete: normalizeFlag(grant.CanDelete),
			CreatedBy: operatorID,
			UpdatedBy: operatorID,
			IsDeleted: 0,
		})
	}
	return result
}

func normalizeFlag(value int) int {
	if value != 0 {
		return 1
	}
	return 0
}

func uniquePositiveInt64(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mapConfigGroup(row configGroupRow) (domain.ConfigGroup, error) {
	return domain.ConfigGroup{
		ID:             row.ID,
		GroupCode:      row.GroupCode,
		GroupName:      row.GroupName,
		Module:         nullableString(row.Module),
		PermissionCode: nullableString(row.PermissionCode),
		SortOrder:      row.SortOrder,
		Status:         row.Status,
		CreateTime:     nullableTime(row.CreateTime),
		UpdateTime:     nullableTime(row.UpdateTime),
		IsDeleted:      int(row.IsDeleted),
		ConfigCount:    nullableInt64(row.ConfigCount),
	}, nil
}

func mapConfig(row configRow) (domain.Config, error) {
	ext, err := unmarshalExtJSON(row.ExtJSON)
	if err != nil {
		return domain.Config{}, err
	}
	validation, err := unmarshalValidation(row.ValidationJSON)
	if err != nil {
		return domain.Config{}, err
	}
	return domain.Config{
		ID:             row.ID,
		GroupID:        row.GroupID,
		ConfigKey:      row.ConfigKey,
		ConfigValue:    nullableString(row.ConfigValue),
		ValueType:      row.ValueType,
		ConfigDesc:     nullableString(row.ConfigDesc),
		IsSensitive:    row.IsSensitive,
		IsSystemConfig: int(row.IsSystemConfig),
		RequiredLogin:  int(row.RequiredLogin),
		UIWidget:       row.UIWidget,
		Validation:     validation,
		Exposure:       row.Exposure,
		Sensitivity:    row.Sensitivity,
		SchemaVersion:  row.SchemaVersion,
		Version:        row.Version,
		ExtJSON:        ext,
		IsReadonly:     row.IsReadonly,
		IsEnabled:      row.IsEnabled,
		EffectType:     nullableString(row.EffectType),
		CreatedBy:      nullableInt64(row.CreatedBy),
		CreateTime:     nullableTime(row.CreateTime),
		UpdatedBy:      nullableInt64(row.UpdatedBy),
		UpdateTime:     nullableTime(row.UpdateTime),
		IsDeleted:      int(row.IsDeleted),
		GroupCode:      nullableString(row.GroupCode),
		GroupName:      nullableString(row.GroupName),
	}, nil
}

func mapChangeLog(row configChangeLogRow) domain.ConfigChangeLog {
	item := domain.ConfigChangeLog{
		ID:                row.ID,
		ConfigID:          row.ConfigID,
		ConfigKey:         row.ConfigKey,
		OperationType:     row.OperationType,
		OldValue:          nullableString(row.OldValue),
		NewValue:          nullableString(row.NewValue),
		OldValueProtected: row.OldValueProtected,
		NewValueProtected: row.NewValueProtected,
		EffectType:        nullableString(row.EffectType),
		Status:            nullableString(row.Status),
		ParentLogID:       nullableInt64Ptr(row.ParentLogID),
		RelatedLogID:      nullableInt64Ptr(row.RelatedLogID),
		OperatorID:        row.OperatorID,
		OperatorName:      nullableString(row.OperatorName),
		OperationTime:     nullableTime(row.OperationTime),
		OperationReason:   nullableString(row.OperationReason),
		AppliedBy:         nullableInt64Ptr(row.AppliedBy),
		AppliedTime:       nullableTime(row.AppliedTime),
	}
	item.HydratePrivateAssetSnapshotPayloads(nullableString(row.OldAssetSnapshot), nullableString(row.NewAssetSnapshot))
	return item
}

func privateAssetSnapshotPayload(item *domain.ConfigChangeLog, old bool) any {
	if item == nil {
		return nil
	}
	oldPayload, newPayload := item.PrivateAssetSnapshotPayloads()
	if old {
		return nullIfBlank(oldPayload)
	}
	return nullIfBlank(newPayload)
}

func marshalExtJSON(ext *domain.ConfigExtJSON) (any, error) {
	if ext == nil {
		return nil, nil
	}
	copied := ext.Copy()
	if copied.Secret != nil {
		copied.Secret.Plain = ""
	}
	payload, err := sonic.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshal config ext json: %w", err)
	}
	return string(payload), nil
}

func marshalValidation(validation *domain.ScalarValidation) (any, error) {
	if validation == nil {
		return nil, nil
	}
	payload, err := sonic.Marshal(validation)
	if err != nil {
		return nil, fmt.Errorf("marshal scalar validation: %w", err)
	}
	return string(payload), nil
}

func unmarshalValidation(raw sql.NullString) (*domain.ScalarValidation, error) {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil, nil
	}
	var validation domain.ScalarValidation
	if err := sonic.Unmarshal([]byte(raw.String), &validation); err != nil {
		return nil, fmt.Errorf("unmarshal scalar validation: %w", err)
	}
	return &validation, nil
}

func unmarshalExtJSON(raw sql.NullString) (*domain.ConfigExtJSON, error) {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil, nil
	}
	var ext domain.ConfigExtJSON
	if err := sonic.Unmarshal([]byte(raw.String), &ext); err != nil {
		return nil, fmt.Errorf("unmarshal config ext json: %w", err)
	}
	return &ext, nil
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func persistedConfigValue(value string) string {
	return value
}

func persistedActorID(value int64) int64 {
	return value
}

func nullableInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func nullIfZero(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullIfZeroPtr(value *int64) any {
	if value == nil || *value == 0 {
		return nil
	}
	return *value
}

func timeOrNow(value *time.Time) time.Time {
	if value == nil || value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func timeOrNil(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

func (r *Repository) insertWithID(ctx context.Context, exec store.SQLX, query, label string, args ...any) (int64, error) {
	query = r.rebind(exec, query)
	if r.postgres {
		var id int64
		if err := exec.QueryRowxContext(ctx, query+` RETURNING id`, args...).Scan(&id); err != nil {
			return 0, fmt.Errorf("insert %s: %w", label, err)
		}
		return id, nil
	}
	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("insert %s: %w", label, err)
	}
	id, err := result.LastInsertId()
	if err == nil && id > 0 {
		return id, nil
	}
	var fallback int64
	if err := sqlx.GetContext(ctx, exec, &fallback, r.rebind(exec, `SELECT LAST_INSERT_ID()`)); err != nil {
		return 0, fmt.Errorf("resolve %s id: %w", label, err)
	}
	return fallback, nil
}
