package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	postgres bool
}

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("system dict repository requires datasource provider")
	}
	return &Repository{
		db:       provider.SQLX(),
		postgres: strings.EqualFold(strings.TrimSpace(provider.Dialect()), "postgres"),
	}, nil
}

var dictPostgresIdentifiers = []string{
	"dictCode", "dictName", "dictDesc", "requiredLogin", "sortOrder", "isSystem", "createdBy",
	"createTime", "updatedBy", "updateTime", "isDeleted", "itemCount", "valueType", "uiWidget",
	"validationJson", "schemaVersion", "dictTypeId", "itemValue", "itemLabel", "itemDesc", "extJson",
	"colorToken", "iconToken", "presentationVersion",
}

var dictPostgresRenderer = store.MustNewPostgresRenderer(dictPostgresIdentifiers, "isDeleted")

func (r *Repository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = dictPostgresRenderer.RenderPostgres(query)
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
		return nil, fmt.Errorf("system dict repository datasource is not configured")
	}
	return exec, nil
}

type dictTypeRow struct {
	ID             int64           `db:"id"`
	DictCode       string          `db:"dictCode"`
	DictName       string          `db:"dictName"`
	DictDesc       sql.NullString  `db:"dictDesc"`
	Module         sql.NullString  `db:"module"`
	Status         int             `db:"status"`
	RequiredLogin  databaseBoolInt `db:"requiredLogin"`
	ValueType      string          `db:"valueType"`
	UIWidget       string          `db:"uiWidget"`
	ValidationJSON sql.NullString  `db:"validationJson"`
	Exposure       string          `db:"exposure"`
	Sensitivity    string          `db:"sensitivity"`
	SchemaVersion  int             `db:"schemaVersion"`
	Version        int64           `db:"version"`
	IsSystem       int             `db:"isSystem"`
	SortOrder      int             `db:"sortOrder"`
	CreatedBy      sql.NullInt64   `db:"createdBy"`
	CreateTime     sql.NullTime    `db:"createTime"`
	UpdatedBy      sql.NullInt64   `db:"updatedBy"`
	UpdateTime     sql.NullTime    `db:"updateTime"`
	IsDeleted      databaseBoolInt `db:"isDeleted"`
	ItemCount      sql.NullInt64   `db:"itemCount"`
}

type dictItemRow struct {
	ID                  int64           `db:"id"`
	DictTypeID          int64           `db:"dictTypeId"`
	ItemValue           string          `db:"itemValue"`
	ItemLabel           string          `db:"itemLabel"`
	ItemDesc            sql.NullString  `db:"itemDesc"`
	SortOrder           int             `db:"sortOrder"`
	Status              int             `db:"status"`
	ExtJSON             sql.NullString  `db:"extJson"`
	ColorToken          sql.NullString  `db:"colorToken"`
	IconToken           sql.NullString  `db:"iconToken"`
	PresentationVersion int             `db:"presentationVersion"`
	Version             int64           `db:"version"`
	CreatedBy           sql.NullInt64   `db:"createdBy"`
	CreateTime          sql.NullTime    `db:"createTime"`
	UpdatedBy           sql.NullInt64   `db:"updatedBy"`
	UpdateTime          sql.NullTime    `db:"updateTime"`
	IsDeleted           databaseBoolInt `db:"isDeleted"`
}

func (r *Repository) FindTypeByID(ctx context.Context, id int64) (*domain.DictType, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row dictTypeRow
	query := r.rebind(exec, `
SELECT
	id,
	dictCode,
	dictName,
	dictDesc,
	module,
	status,
	requiredLogin,
	valueType,
	uiWidget,
	validationJson,
	exposure,
	sensitivity,
	schemaVersion,
	version,
	isSystem,
	sortOrder,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_type
WHERE id = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find dict type by id: %w", err)
	}
	item := mapDictType(row)
	return &item, nil
}

func (r *Repository) FindTypeByCode(ctx context.Context, dictCode string) (*domain.DictType, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row dictTypeRow
	query := r.rebind(exec, `
SELECT
	id,
	dictCode,
	dictName,
	dictDesc,
	module,
	status,
	requiredLogin,
	valueType,
	uiWidget,
	validationJson,
	exposure,
	sensitivity,
	schemaVersion,
	version,
	isSystem,
	sortOrder,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_type
WHERE LOWER(dictCode) = LOWER(?) AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, dictCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find dict type by code: %w", err)
	}
	item := mapDictType(row)
	return &item, nil
}

func (r *Repository) CountTypeByCode(ctx context.Context, dictCode string, excludeID int64) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	query := `SELECT COUNT(1) FROM sys_dict_type WHERE LOWER(dictCode) = LOWER(?) AND isDeleted = 0`
	args := []any{dictCode}
	if excludeID > 0 {
		query += ` AND id <> ?`
		args = append(args, excludeID)
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, r.rebind(exec, query), args...); err != nil {
		return 0, fmt.Errorf("count dict type by code: %w", err)
	}
	return count, nil
}

func (r *Repository) InsertType(ctx context.Context, item *domain.DictType) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	return r.insertWithID(ctx, exec, `
INSERT INTO sys_dict_type (
	dictCode, dictName, dictDesc, module, status, requiredLogin, isSystem, sortOrder,
	valueType, uiWidget, validationJson, exposure, sensitivity, schemaVersion, version,
	createdBy, createTime, updatedBy, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"dict type",
		item.DictCode,
		nullIfBlank(item.DictName),
		nullIfBlank(item.DictDesc),
		nullIfBlank(item.Module),
		item.Status,
		r.databaseBool(item.RequiredLogin),
		item.IsSystem,
		item.SortOrder,
		item.ValueType,
		item.UIWidget,
		nullIfBlank(item.ValidationJSON),
		item.Exposure,
		item.Sensitivity,
		item.SchemaVersion,
		item.Version,
		nullIfZero(item.CreatedBy),
		timeOrNow(item.CreateTime),
		nullIfZero(item.UpdatedBy),
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
	)
}

func (r *Repository) UpdateType(ctx context.Context, item *domain.DictType) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_dict_type
SET dictName = ?, dictDesc = ?, module = ?, status = ?, requiredLogin = ?, isSystem = ?, sortOrder = ?,
	valueType = ?, uiWidget = ?, validationJson = ?, exposure = ?, sensitivity = ?, schemaVersion = ?, version = ?,
	updatedBy = ?, updateTime = ?, isDeleted = ?
	WHERE id = ? AND version = ? AND isDeleted = 0`),
		nullIfBlank(item.DictName),
		nullIfBlank(item.DictDesc),
		nullIfBlank(item.Module),
		item.Status,
		r.databaseBool(item.RequiredLogin),
		item.IsSystem,
		item.SortOrder,
		item.ValueType,
		item.UIWidget,
		nullIfBlank(item.ValidationJSON),
		item.Exposure,
		item.Sensitivity,
		item.SchemaVersion,
		item.Version+1,
		nullIfZero(item.UpdatedBy),
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
		item.ID,
		item.Version,
	)
	if err != nil {
		return fmt.Errorf("update dict type: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect dict type update result: %w", err)
	}
	if rows != 1 {
		return apperrors.ObjectState("字典类型已被其他管理员更新，请刷新后重试").WithDetails(map[string]any{
			"reasonCode": "DICT_TYPE_VERSION_CONFLICT",
			"dictTypeId": item.ID,
			"version":    item.Version,
		})
	}
	item.Version++
	return nil
}

func (r *Repository) QueryTypes(ctx context.Context, query domain.DictTypePageQuery) (*domain.DictTypePage, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	whereParts := []string{"isDeleted = 0"}
	args := make([]any, 0, 4)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		whereParts = append(whereParts, "(dictCode LIKE ? OR dictName LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	if module := strings.TrimSpace(query.Module); module != "" {
		whereParts = append(whereParts, "module = ?")
		args = append(args, module)
	}
	if query.Status != nil {
		whereParts = append(whereParts, "status = ?")
		args = append(args, *query.Status)
	}
	whereSQL := strings.Join(whereParts, " AND ")
	var total int64
	countQuery := r.rebind(exec, `SELECT COUNT(1) FROM sys_dict_type WHERE `+whereSQL)
	if err := sqlx.GetContext(ctx, exec, &total, countQuery, args...); err != nil {
		return nil, fmt.Errorf("count dict types: %w", err)
	}
	page := &domain.DictTypePage{
		Current: query.Current,
		Size:    query.PageSize,
		Total:   total,
		Records: []domain.DictType{},
	}
	if total == 0 {
		return page, nil
	}
	offset := (query.Current - 1) * query.PageSize
	listQuery := r.rebind(exec, `
SELECT
	id,
	dictCode,
	dictName,
	dictDesc,
	module,
	status,
	requiredLogin,
	valueType,
	uiWidget,
	validationJson,
	exposure,
	sensitivity,
	schemaVersion,
	version,
	isSystem,
	sortOrder,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_type
WHERE `+whereSQL+`
ORDER BY createTime DESC, sortOrder ASC
LIMIT ? OFFSET ?`)
	listArgs := append(append([]any{}, args...), query.PageSize, offset)
	rows := make([]dictTypeRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, listQuery, listArgs...); err != nil {
		return nil, fmt.Errorf("query dict types: %w", err)
	}
	typeIDs := make([]int64, 0, len(rows))
	page.Records = make([]domain.DictType, 0, len(rows))
	for _, row := range rows {
		item := mapDictType(row)
		page.Records = append(page.Records, item)
		typeIDs = append(typeIDs, item.ID)
	}
	counts, err := r.CountItemsByTypeIDs(ctx, typeIDs)
	if err != nil {
		return nil, err
	}
	for idx := range page.Records {
		page.Records[idx].ItemCount = counts[page.Records[idx].ID]
	}
	return page, nil
}

func (r *Repository) CountItemsByTypeID(ctx context.Context, typeID int64) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	query := r.rebind(exec, `SELECT COUNT(1) FROM sys_dict_item WHERE dictTypeId = ? AND isDeleted = 0`)
	if err := sqlx.GetContext(ctx, exec, &count, query, typeID); err != nil {
		return 0, fmt.Errorf("count dict items by type: %w", err)
	}
	return count, nil
}

func (r *Repository) CountItemsByTypeIDs(ctx context.Context, typeIDs []int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(typeIDs))
	if len(typeIDs) == 0 {
		return result, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT dictTypeId, COUNT(1) AS itemCount
FROM sys_dict_item
WHERE dictTypeId IN (?) AND isDeleted = 0
GROUP BY dictTypeId`, typeIDs)
	if err != nil {
		return nil, fmt.Errorf("build count items by type ids query: %w", err)
	}
	query = r.rebind(exec, query)
	rows := []struct {
		DictTypeID int64 `db:"dictTypeId"`
		ItemCount  int64 `db:"itemCount"`
	}{}
	if err := sqlx.SelectContext(ctx, exec, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("count items by type ids: %w", err)
	}
	for _, row := range rows {
		result[row.DictTypeID] = row.ItemCount
	}
	return result, nil
}

func (r *Repository) SoftDeleteItemsByTypeID(ctx context.Context, typeID, actorID int64, updatedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_dict_item
SET isDeleted = 1, version = version + 1, updatedBy = ?, updateTime = ?
WHERE dictTypeId = ? AND isDeleted = 0`), nullIfZero(actorID), updatedAt.UTC(), typeID)
	if err != nil {
		return fmt.Errorf("soft delete dict items by type: %w", err)
	}
	return nil
}

func (r *Repository) ShiftTypeSort(ctx context.Context, targetID int64, oldOrder, newOrder int) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	if oldOrder < newOrder {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_dict_type
SET sortOrder = sortOrder - 1, version = version + 1
WHERE isDeleted = 0 AND id <> ? AND sortOrder > ? AND sortOrder <= ?`), targetID, oldOrder, newOrder)
	} else {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_dict_type
SET sortOrder = sortOrder + 1, version = version + 1
WHERE isDeleted = 0 AND id <> ? AND sortOrder >= ? AND sortOrder < ?`), targetID, newOrder, oldOrder)
	}
	if err != nil {
		return fmt.Errorf("shift dict type sort: %w", err)
	}
	return nil
}

func (r *Repository) FindReadableTypesByCodes(ctx context.Context, dictCodes []string) ([]domain.DictType, error) {
	if len(dictCodes) == 0 {
		return []domain.DictType{}, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT
	id,
	dictCode,
	dictName,
	dictDesc,
	module,
	status,
	requiredLogin,
	valueType,
	uiWidget,
	validationJson,
	exposure,
	sensitivity,
	schemaVersion,
	version,
	isSystem,
	sortOrder,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_type
WHERE dictCode IN (?) AND isDeleted = 0 AND status = 1`, dictCodes)
	if err != nil {
		return nil, fmt.Errorf("build readable dict types query: %w", err)
	}
	query = r.rebind(exec, query)
	rows := make([]dictTypeRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, args...); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("find readable dict types by codes: %w", err)
	}
	result := make([]domain.DictType, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapDictType(row))
	}
	return result, nil
}

func (r *Repository) FindItemByID(ctx context.Context, id int64) (*domain.DictItem, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row dictItemRow
	query := r.rebind(exec, `
SELECT
	id,
	dictTypeId,
	itemValue,
	itemLabel,
	itemDesc,
	sortOrder,
	status,
	extJson,
	colorToken,
	iconToken,
	presentationVersion,
	version,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_item
WHERE id = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find dict item by id: %w", err)
	}
	item := mapDictItem(row)
	return &item, nil
}

func (r *Repository) CountItemByValue(ctx context.Context, typeID int64, itemValue string, excludeID int64) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	query := `SELECT COUNT(1) FROM sys_dict_item WHERE dictTypeId = ? AND itemValue = ? AND isDeleted = 0`
	args := []any{typeID, itemValue}
	if excludeID > 0 {
		query += ` AND id <> ?`
		args = append(args, excludeID)
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, r.rebind(exec, query), args...); err != nil {
		return 0, fmt.Errorf("count dict item by value: %w", err)
	}
	return count, nil
}

func (r *Repository) InsertItem(ctx context.Context, item *domain.DictItem) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	return r.insertWithID(ctx, exec, `
INSERT INTO sys_dict_item (
	dictTypeId, itemValue, itemLabel, itemDesc, sortOrder, status, extJson,
	colorToken, iconToken, presentationVersion, version,
	createdBy, createTime, updatedBy, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"dict item",
		item.DictTypeID,
		item.ItemValue,
		nullIfBlank(item.ItemLabel),
		nullIfBlank(item.ItemDesc),
		item.SortOrder,
		item.Status,
		nullIfBlank(item.ExtJSON),
		nullIfBlank(item.ColorToken),
		nullIfBlank(item.IconToken),
		item.PresentationVersion,
		item.Version,
		nullIfZero(item.CreatedBy),
		timeOrNow(item.CreateTime),
		nullIfZero(item.UpdatedBy),
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
	)
}

func (r *Repository) UpdateItem(ctx context.Context, item *domain.DictItem) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_dict_item
SET itemLabel = ?, itemDesc = ?, sortOrder = ?, status = ?, extJson = ?,
	colorToken = ?, iconToken = ?, presentationVersion = ?, version = ?,
	updatedBy = ?, updateTime = ?, isDeleted = ?
WHERE id = ? AND version = ? AND isDeleted = 0`),
		nullIfBlank(item.ItemLabel),
		nullIfBlank(item.ItemDesc),
		item.SortOrder,
		item.Status,
		nullIfBlank(item.ExtJSON),
		nullIfBlank(item.ColorToken),
		nullIfBlank(item.IconToken),
		item.PresentationVersion,
		item.Version+1,
		nullIfZero(item.UpdatedBy),
		timeOrNow(item.UpdateTime),
		r.databaseBool(item.IsDeleted),
		item.ID,
		item.Version,
	)
	if err != nil {
		return fmt.Errorf("update dict item: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect dict item update result: %w", err)
	}
	if rows != 1 {
		return apperrors.ObjectState("字典项已被其他管理员更新，请刷新后重试").WithDetails(map[string]any{
			"reasonCode": "DICT_ITEM_VERSION_CONFLICT",
			"dictItemId": item.ID,
			"version":    item.Version,
		})
	}
	item.Version++
	return nil
}

func (r *Repository) UpdateItemSorts(ctx context.Context, items []domain.DictItem) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) > 100 {
		return fmt.Errorf("dict item sort set exceeds 100")
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	caseParts := make([]string, 0, len(items))
	whereParts := make([]string, 0, len(items))
	args := make([]any, 0, len(items)*4+3)
	for _, item := range items {
		caseParts = append(caseParts, "WHEN ? THEN ?")
		args = append(args, item.ID, item.SortOrder)
	}
	actorID := items[0].UpdatedBy
	updateTime := timeOrNow(items[0].UpdateTime)
	args = append(args, nullIfZero(actorID), updateTime)
	for _, item := range items {
		whereParts = append(whereParts, "(id = ? AND version = ?)")
		args = append(args, item.ID, item.Version)
	}
	query := `
UPDATE sys_dict_item
SET sortOrder = CASE id ` + strings.Join(caseParts, " ") + ` ELSE sortOrder END,
    version = version + 1, updatedBy = ?, updateTime = ?
WHERE isDeleted = 0 AND (` + strings.Join(whereParts, " OR ") + `)`
	result, err := exec.ExecContext(ctx, r.rebind(exec, query), args...)
	if err != nil {
		return fmt.Errorf("batch update dict item sorts: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect dict item sort update result: %w", err)
	}
	if rows != int64(len(items)) {
		return apperrors.ObjectState("部分字典项已被其他管理员更新，请刷新后重试").WithDetails(map[string]any{
			"reasonCode": "DICT_ITEM_VERSION_CONFLICT",
		})
	}
	return nil
}

func (r *Repository) QueryItems(ctx context.Context, query domain.DictItemListQuery) ([]domain.DictItem, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	whereParts := []string{"dictTypeId = ?", "isDeleted = 0"}
	args := []any{query.DictTypeID}
	if query.Status != nil {
		whereParts = append(whereParts, "status = ?")
		args = append(args, *query.Status)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		whereParts = append(whereParts, "(itemValue LIKE ? OR itemLabel LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	sqlQuery := `
SELECT
	id,
	dictTypeId,
	itemValue,
	itemLabel,
	itemDesc,
	sortOrder,
	status,
	extJson,
	colorToken,
	iconToken,
	presentationVersion,
	version,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_item
WHERE ` + strings.Join(whereParts, " AND ") + `
ORDER BY sortOrder ASC, id ASC`
	rows := make([]dictItemRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, sqlQuery), args...); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("query dict items: %w", err)
	}
	result := make([]domain.DictItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapDictItem(row))
	}
	return result, nil
}

func (r *Repository) ListItemsByIDs(ctx context.Context, ids []int64) ([]domain.DictItem, error) {
	if len(ids) == 0 {
		return []domain.DictItem{}, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT
	id,
	dictTypeId,
	itemValue,
	itemLabel,
	itemDesc,
	sortOrder,
	status,
	extJson,
	colorToken,
	iconToken,
	presentationVersion,
	version,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_item
WHERE id IN (?)`, ids)
	if err != nil {
		return nil, fmt.Errorf("build list dict items by ids query: %w", err)
	}
	query = r.rebind(exec, query)
	rows := make([]dictItemRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, args...); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list dict items by ids: %w", err)
	}
	result := make([]domain.DictItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapDictItem(row))
	}
	return result, nil
}

func (r *Repository) ListReadableItemsByTypeIDs(ctx context.Context, typeIDs []int64) ([]domain.DictItem, error) {
	if len(typeIDs) == 0 {
		return []domain.DictItem{}, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT
	id,
	dictTypeId,
	itemValue,
	itemLabel,
	itemDesc,
	sortOrder,
	status,
	extJson,
	colorToken,
	iconToken,
	presentationVersion,
	version,
	createdBy,
	createTime,
	updatedBy,
	updateTime,
	isDeleted
FROM sys_dict_item
WHERE dictTypeId IN (?) AND isDeleted = 0 AND status = 1
ORDER BY dictTypeId ASC, sortOrder ASC, id ASC`, typeIDs)
	if err != nil {
		return nil, fmt.Errorf("build readable dict items query: %w", err)
	}
	query = r.rebind(exec, query)
	rows := make([]dictItemRow, 0)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, args...); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list readable dict items by type ids: %w", err)
	}
	result := make([]domain.DictItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapDictItem(row))
	}
	return result, nil
}

func (r *Repository) ShiftItemSort(ctx context.Context, typeID, targetID int64, oldOrder, newOrder int) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	if oldOrder < newOrder {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_dict_item
SET sortOrder = sortOrder - 1, version = version + 1
WHERE dictTypeId = ? AND isDeleted = 0 AND id <> ? AND sortOrder > ? AND sortOrder <= ?`),
			typeID, targetID, oldOrder, newOrder)
	} else {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_dict_item
SET sortOrder = sortOrder + 1, version = version + 1
WHERE dictTypeId = ? AND isDeleted = 0 AND id <> ? AND sortOrder >= ? AND sortOrder < ?`),
			typeID, targetID, newOrder, oldOrder)
	}
	if err != nil {
		return fmt.Errorf("shift dict item sort: %w", err)
	}
	return nil
}

func mapDictType(row dictTypeRow) domain.DictType {
	return domain.DictType{
		ID:             row.ID,
		DictCode:       row.DictCode,
		DictName:       row.DictName,
		DictDesc:       nullableString(row.DictDesc),
		Module:         nullableString(row.Module),
		Status:         row.Status,
		RequiredLogin:  int(row.RequiredLogin),
		ValueType:      row.ValueType,
		UIWidget:       row.UIWidget,
		ValidationJSON: nullableString(row.ValidationJSON),
		Exposure:       row.Exposure,
		Sensitivity:    row.Sensitivity,
		SchemaVersion:  row.SchemaVersion,
		Version:        row.Version,
		IsSystem:       row.IsSystem,
		SortOrder:      row.SortOrder,
		CreatedBy:      nullableInt64(row.CreatedBy),
		CreateTime:     nullableTime(row.CreateTime),
		UpdatedBy:      nullableInt64(row.UpdatedBy),
		UpdateTime:     nullableTime(row.UpdateTime),
		IsDeleted:      int(row.IsDeleted),
		ItemCount:      nullableInt64(row.ItemCount),
	}
}

func mapDictItem(row dictItemRow) domain.DictItem {
	return domain.DictItem{
		ID:                  row.ID,
		DictTypeID:          row.DictTypeID,
		ItemValue:           row.ItemValue,
		ItemLabel:           row.ItemLabel,
		ItemDesc:            nullableString(row.ItemDesc),
		SortOrder:           row.SortOrder,
		Status:              row.Status,
		ExtJSON:             nullableString(row.ExtJSON),
		ColorToken:          nullableString(row.ColorToken),
		IconToken:           nullableString(row.IconToken),
		PresentationVersion: row.PresentationVersion,
		Version:             row.Version,
		CreatedBy:           nullableInt64(row.CreatedBy),
		CreateTime:          nullableTime(row.CreateTime),
		UpdatedBy:           nullableInt64(row.UpdatedBy),
		UpdateTime:          nullableTime(row.UpdateTime),
		IsDeleted:           int(row.IsDeleted),
	}
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func nullableInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	current := value.Time.UTC()
	return &current
}

func nullIfBlank(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullIfZero(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func timeOrNow(value *time.Time) time.Time {
	if value == nil {
		return time.Now().UTC()
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
