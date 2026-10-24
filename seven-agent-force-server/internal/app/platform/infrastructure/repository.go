package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	postgres bool
}

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("platform repository requires datasource provider")
	}
	return &Repository{
		db:       provider.SQLX(),
		postgres: strings.EqualFold(strings.TrimSpace(provider.Dialect()), "postgres"),
	}, nil
}

func (r *Repository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = platformPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}

func (r *Repository) executor(ctx context.Context) store.SQLX {
	return store.SQLXExecutor(ctx, r.db)
}

func (r *Repository) requireExecutor(ctx context.Context) (store.SQLX, error) {
	exec := r.executor(ctx)
	if exec == nil {
		return nil, fmt.Errorf("platform repository datasource is not configured")
	}
	return exec, nil
}

func (r *Repository) requireTransactionExecutor(ctx context.Context) (store.SQLX, error) {
	exec := store.SQLXFromContext(ctx)
	if exec == nil {
		return nil, fmt.Errorf("platform policy replacement requires an active transaction")
	}
	return exec, nil
}

type platformRow struct {
	ID                 int64          `db:"id"`
	PlatformCode       string         `db:"platformCode"`
	PlatformName       string         `db:"platformName"`
	PlatformType       string         `db:"platformType"`
	Description        sql.NullString `db:"description"`
	DefaultRedirectURL sql.NullString `db:"defaultRedirectUrl"`
	AllowAutoRegister  int            `db:"allowAutoRegister"`
	AllowFormRegister  int            `db:"allowFormRegister"`
	IsDefault          int            `db:"isDefault"`
	DefaultDeptID      sql.NullInt64  `db:"defaultDeptId"`
	BrandJSON          sql.NullString `db:"brandJson"`
	SettingsJSON       sql.NullString `db:"settingsJson"`
	Status             int            `db:"status"`
	CreateTime         time.Time      `db:"createTime"`
	UpdateTime         time.Time      `db:"updateTime"`
}

type ssoClientBindingRow struct {
	PlatformCode string `db:"platformCode"`
	ClientID     string `db:"clientId"`
	Status       int    `db:"status"`
}

type sourceRuleRow struct {
	ID           int64          `db:"id"`
	PlatformCode string         `db:"platformCode"`
	MatchType    string         `db:"matchType"`
	MatchValue   string         `db:"matchValue"`
	Priority     int            `db:"priority"`
	Status       int            `db:"status"`
	MetadataJSON sql.NullString `db:"metadataJson"`
}

type loginMethodRow struct {
	ID             int64          `db:"id"`
	PlatformCode   string         `db:"platformCode"`
	MethodType     string         `db:"methodType"`
	ProviderCode   string         `db:"providerCode"`
	DisplayName    string         `db:"displayName"`
	Icon           sql.NullString `db:"icon"`
	SortOrder      int            `db:"sortOrder"`
	DisplayEnabled int            `db:"displayEnabled"`
	LoginEnabled   int            `db:"loginEnabled"`
	MetadataJSON   sql.NullString `db:"metadataJson"`
}

type roleSafetyRow struct {
	RoleID    int64  `db:"roleId"`
	Code      string `db:"code"`
	SystemKey string `db:"systemKey"`
	Status    int    `db:"status"`
	Type      int    `db:"type"`
}

type permissionCodeRow struct {
	RoleID int64  `db:"roleId"`
	Code   string `db:"code"`
}

type defaultRoleRow struct {
	ID                int64  `db:"id"`
	PlatformCode      string `db:"platformCode"`
	RoleID            int64  `db:"roleId"`
	AutoAssignEnabled int    `db:"autoAssignEnabled"`
	Status            int    `db:"status"`
}

const (
	platformDetailMaxCodes  = 200
	platformDetailChunkSize = 100
	platformDetailResultMax = 1000

	platformLoginMethodMaxCount = 100
	platformSourceRuleMaxCount  = 200
	platformDefaultRoleMaxCount = 3
	platformReplaceChunkSize    = 50
)

func (r *Repository) ListActivePlatforms(ctx context.Context) ([]domain.Platform, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []platformRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, platformSelectSQL+`
FROM sys_platform
WHERE status = ? AND isDeleted = 0
ORDER BY isDefault DESC, platformCode ASC`), domain.StatusActive); err != nil {
		return nil, fmt.Errorf("list active platforms: %w", err)
	}
	return mapPlatformRows(rows), nil
}

func (r *Repository) ListPlatforms(ctx context.Context, query domain.PlatformQuery) ([]domain.Platform, int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, 0, err
	}
	where, args := platformWhere(query)
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(1) FROM sys_platform `+where), args...); err != nil {
		return nil, 0, fmt.Errorf("count platforms: %w", err)
	}
	current, pageSize := pageArgs(query.Current, query.PageSize)
	selectArgs := append(append([]any{}, args...), pageSize, (current-1)*pageSize)
	var rows []platformRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, platformSelectSQL+`
FROM sys_platform `+where+`
ORDER BY isDefault DESC, updateTime DESC, id DESC
LIMIT ? OFFSET ?`), selectArgs...); err != nil {
		return nil, 0, fmt.Errorf("list platforms: %w", err)
	}
	return mapPlatformRows(rows), total, nil
}

func (r *Repository) FindPlatform(ctx context.Context, platformCode string) (*domain.Platform, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row platformRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, platformSelectSQL+`
FROM sys_platform
WHERE platformCode = ? AND isDeleted = 0
LIMIT 1`), strings.TrimSpace(platformCode)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find platform: %w", err)
	}
	item := mapPlatformRow(row)
	return &item, nil
}

func (r *Repository) FindDefaultPlatform(ctx context.Context) (*domain.Platform, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row platformRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, platformSelectSQL+`
FROM sys_platform
WHERE isDefault = 1 AND status = ? AND isDeleted = 0
ORDER BY id
LIMIT 1`), domain.StatusActive); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find default platform: %w", err)
	}
	item := mapPlatformRow(row)
	return &item, nil
}

func (r *Repository) FindManagedDefaultPlatform(ctx context.Context) (*domain.Platform, error) {
	return r.findManagedDefaultPlatform(ctx, false)
}

func (r *Repository) FindManagedDefaultPlatformForUpdate(ctx context.Context) (*domain.Platform, error) {
	return r.findManagedDefaultPlatform(ctx, true)
}

func (r *Repository) findManagedDefaultPlatform(ctx context.Context, forUpdate bool) (*domain.Platform, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query := platformSelectSQL + `
FROM sys_platform
WHERE isDefault = 1 AND isDeleted = 0
ORDER BY id
LIMIT 1`
	if forUpdate {
		query += "\nFOR UPDATE"
	}
	var row platformRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, query)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find managed default platform: %w", err)
	}
	item := mapPlatformRow(row)
	return &item, nil
}

func (r *Repository) ListActiveSSOClientBindings(ctx context.Context) ([]domain.SSOClientBinding, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []ssoClientBindingRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `
SELECT platformCode, clientId, status
FROM sys_platform_sso_client
WHERE status = ? AND isDeleted = 0`), domain.StatusActive); err != nil {
		return nil, fmt.Errorf("list active platform sso clients: %w", err)
	}
	result := make([]domain.SSOClientBinding, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.SSOClientBinding{PlatformCode: row.PlatformCode, ClientID: row.ClientID, Status: row.Status})
	}
	return result, nil
}

func (r *Repository) ListActiveSourceRules(ctx context.Context) ([]domain.SourceRule, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []sourceRuleRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `
SELECT platformCode, matchType, matchValue, priority, status, metadataJson
FROM sys_platform_source_rule
WHERE status = ? AND isDeleted = 0`), domain.StatusActive); err != nil {
		return nil, fmt.Errorf("list active platform source rules: %w", err)
	}
	result := make([]domain.SourceRule, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.SourceRule{
			PlatformCode: row.PlatformCode,
			MatchType:    row.MatchType,
			MatchValue:   row.MatchValue,
			Priority:     row.Priority,
			Status:       row.Status,
			MetadataJSON: nullStringValue(row.MetadataJSON),
		})
	}
	return result, nil
}

func (r *Repository) ListLoginMethods(ctx context.Context, platformCode string) ([]domain.LoginMethod, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []loginMethodRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `
SELECT id, platformCode, methodType, providerCode, displayName, icon, sortOrder, displayEnabled, loginEnabled, metadataJson
FROM sys_platform_login_method
WHERE platformCode = ? AND isDeleted = 0
  AND (
      methodType <> 'EXTERNAL_OAUTH'
      OR EXISTS (
          SELECT 1
          FROM sys_external_login_provider p
          WHERE p.providerCode = sys_platform_login_method.providerCode
            AND p.status = 0
            AND p.displayEnabled = 1
            AND p.loginEnabled = 1
            AND p.isDeleted = 0
      )
  )
ORDER BY sortOrder ASC, displayName ASC`), platformCode); err != nil {
		return nil, fmt.Errorf("list platform login methods: %w", err)
	}
	result := make([]domain.LoginMethod, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.LoginMethod{
			ID:             row.ID,
			PlatformCode:   row.PlatformCode,
			MethodType:     row.MethodType,
			ProviderCode:   row.ProviderCode,
			DisplayName:    row.DisplayName,
			Icon:           nullStringValue(row.Icon),
			SortOrder:      row.SortOrder,
			DisplayEnabled: row.DisplayEnabled == 1,
			LoginEnabled:   row.LoginEnabled == 1,
			MetadataJSON:   nullStringValue(row.MetadataJSON),
		})
	}
	return result, nil
}

func (r *Repository) ListLoginMethodsByPlatformCodes(ctx context.Context, platformCodes []string) ([]domain.LoginMethod, error) {
	codes, err := normalizedPlatformCodes(platformCodes)
	if err != nil || len(codes) == 0 {
		return []domain.LoginMethod{}, err
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.LoginMethod, 0)
	for _, chunk := range platformCodeChunks(codes) {
		remaining := platformDetailResultMax - len(result)
		query, args, inErr := sqlx.In(`
SELECT id, platformCode, methodType, providerCode, displayName, icon, sortOrder, displayEnabled, loginEnabled, metadataJson
FROM sys_platform_login_method
WHERE platformCode IN (?) AND isDeleted = 0
  AND (
      methodType <> 'EXTERNAL_OAUTH'
      OR EXISTS (
          SELECT 1
          FROM sys_external_login_provider p
          WHERE p.providerCode = sys_platform_login_method.providerCode
            AND p.status = 0
            AND p.displayEnabled = 1
            AND p.loginEnabled = 1
            AND p.isDeleted = 0
      )
  )
ORDER BY platformCode ASC, sortOrder ASC, displayName ASC, id ASC
LIMIT ?`, chunk, remaining+1)
		if inErr != nil {
			return nil, inErr
		}
		var rows []loginMethodRow
		if selectErr := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); selectErr != nil {
			return nil, fmt.Errorf("list platform login methods by platform codes: %w", selectErr)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("platform login method result exceeds %d", platformDetailResultMax)
		}
		result = append(result, mapLoginMethodRows(rows)...)
	}
	return result, nil
}

func (r *Repository) ListManagedLoginMethods(ctx context.Context, platformCode string) ([]domain.LoginMethod, error) {
	return r.listManagedLoginMethods(ctx, platformCode, false)
}

func (r *Repository) ListManagedLoginMethodsForUpdate(ctx context.Context, platformCode string) ([]domain.LoginMethod, error) {
	return r.listManagedLoginMethods(ctx, platformCode, true)
}

func (r *Repository) listManagedLoginMethods(ctx context.Context, platformCode string, forUpdate bool) ([]domain.LoginMethod, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query := `
SELECT id, platformCode, methodType, providerCode, displayName, icon, sortOrder, displayEnabled, loginEnabled, metadataJson
FROM sys_platform_login_method
WHERE platformCode = ? AND isDeleted = 0
ORDER BY sortOrder ASC, displayName ASC, id ASC`
	if forUpdate {
		query += "\nFOR UPDATE"
	}
	var rows []loginMethodRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), strings.TrimSpace(platformCode)); err != nil {
		return nil, fmt.Errorf("list managed platform login methods: %w", err)
	}
	return mapLoginMethodRows(rows), nil
}

func (r *Repository) ListSourceRules(ctx context.Context, platformCode string) ([]domain.SourceRule, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []sourceRuleRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `
SELECT id, platformCode, matchType, matchValue, priority, status, metadataJson
FROM sys_platform_source_rule
WHERE platformCode = ? AND isDeleted = 0
ORDER BY priority DESC, id ASC`), strings.TrimSpace(platformCode)); err != nil {
		return nil, fmt.Errorf("list platform source rules: %w", err)
	}
	return mapSourceRuleRows(rows), nil
}

func (r *Repository) ListSourceRulesByPlatformCodes(ctx context.Context, platformCodes []string) ([]domain.SourceRule, error) {
	codes, err := normalizedPlatformCodes(platformCodes)
	if err != nil || len(codes) == 0 {
		return []domain.SourceRule{}, err
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.SourceRule, 0)
	for _, chunk := range platformCodeChunks(codes) {
		remaining := platformDetailResultMax - len(result)
		query, args, inErr := sqlx.In(`
SELECT id, platformCode, matchType, matchValue, priority, status, metadataJson
FROM sys_platform_source_rule
WHERE platformCode IN (?) AND isDeleted = 0
ORDER BY platformCode ASC, priority DESC, id ASC
LIMIT ?`, chunk, remaining+1)
		if inErr != nil {
			return nil, inErr
		}
		var rows []sourceRuleRow
		if selectErr := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); selectErr != nil {
			return nil, fmt.Errorf("list platform source rules by platform codes: %w", selectErr)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("platform source rule result exceeds %d", platformDetailResultMax)
		}
		result = append(result, mapSourceRuleRows(rows)...)
	}
	return result, nil
}

func (r *Repository) ListManagedSourceRulesForUpdate(ctx context.Context, platformCode string) ([]domain.SourceRule, error) {
	return r.listManagedSourceRules(ctx, platformCode, true)
}

func (r *Repository) ListManagedSourceRules(ctx context.Context, platformCode string) ([]domain.SourceRule, error) {
	return r.listManagedSourceRules(ctx, platformCode, false)
}

func (r *Repository) listManagedSourceRules(ctx context.Context, platformCode string, forUpdate bool) ([]domain.SourceRule, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query := `
SELECT id, platformCode, matchType, matchValue, priority, status, metadataJson
FROM sys_platform_source_rule
WHERE platformCode = ? AND isDeleted = 0
ORDER BY priority DESC, id ASC`
	if forUpdate {
		query += "\nFOR UPDATE"
	}
	var rows []sourceRuleRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), strings.TrimSpace(platformCode)); err != nil {
		return nil, fmt.Errorf("list managed platform source rules: %w", err)
	}
	return mapSourceRuleRows(rows), nil
}

func (r *Repository) ListDefaultRoleRecords(ctx context.Context, platformCode string) ([]domain.DefaultRole, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []defaultRoleRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `
SELECT id, platformCode, roleId, autoAssignEnabled, status
FROM sys_platform_default_role
WHERE platformCode = ? AND isDeleted = 0
ORDER BY id ASC`), strings.TrimSpace(platformCode)); err != nil {
		return nil, fmt.Errorf("list platform default role records: %w", err)
	}
	return mapDefaultRoleRows(rows), nil
}

func (r *Repository) ListDefaultRoleRecordsByPlatformCodes(ctx context.Context, platformCodes []string) ([]domain.DefaultRole, error) {
	codes, err := normalizedPlatformCodes(platformCodes)
	if err != nil || len(codes) == 0 {
		return []domain.DefaultRole{}, err
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.DefaultRole, 0)
	for _, chunk := range platformCodeChunks(codes) {
		remaining := platformDetailResultMax - len(result)
		query, args, inErr := sqlx.In(`
SELECT id, platformCode, roleId, autoAssignEnabled, status
FROM sys_platform_default_role
WHERE platformCode IN (?) AND isDeleted = 0
ORDER BY platformCode ASC, id ASC
LIMIT ?`, chunk, remaining+1)
		if inErr != nil {
			return nil, inErr
		}
		var rows []defaultRoleRow
		if selectErr := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); selectErr != nil {
			return nil, fmt.Errorf("list platform default role records by platform codes: %w", selectErr)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("platform default role result exceeds %d", platformDetailResultMax)
		}
		result = append(result, mapDefaultRoleRows(rows)...)
	}
	return result, nil
}

func (r *Repository) ListDefaultRoles(ctx context.Context, platformCode string, maxCount int) ([]domain.DefaultRole, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	if maxCount <= 0 {
		maxCount = 3
	}
	const maxDefaultRoleCandidates = 200
	var rows []defaultRoleRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `
SELECT pdr.id, pdr.platformCode, pdr.roleId, pdr.autoAssignEnabled, pdr.status
FROM sys_platform_default_role pdr
JOIN sys_role r ON r.id = pdr.roleId
  AND r.status = 0
  AND NOT r.isDeleted
WHERE pdr.platformCode = ?
  AND pdr.status = 0
  AND pdr.autoAssignEnabled = 1
  AND pdr.isDeleted = 0
ORDER BY pdr.id ASC
LIMIT ?`), platformCode, maxDefaultRoleCandidates+1); err != nil {
		return nil, fmt.Errorf("list platform default role candidates: %w", err)
	}
	if len(rows) > maxDefaultRoleCandidates {
		return nil, fmt.Errorf("platform default role candidate set exceeds %d", maxDefaultRoleCandidates)
	}
	roleIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		roleIDs = append(roleIDs, row.RoleID)
	}
	if len(roleIDs) == 0 {
		return []domain.DefaultRole{}, nil
	}
	directCodes, err := r.rolePermissionCodes(ctx, exec, roleIDs)
	if err != nil {
		return nil, err
	}
	menuCodes, err := r.roleMenuPermissionCodes(ctx, exec, roleIDs)
	if err != nil {
		return nil, err
	}
	result := make([]domain.DefaultRole, 0, len(rows))
	for _, row := range rows {
		if defaultRoleHasPrivilegedPermission(directCodes[row.RoleID]) || defaultRoleHasPrivilegedPermission(menuCodes[row.RoleID]) {
			continue
		}
		result = append(result, domain.DefaultRole{
			ID:                row.ID,
			PlatformCode:      row.PlatformCode,
			RoleID:            row.RoleID,
			AutoAssignEnabled: row.AutoAssignEnabled == 1,
			Status:            row.Status,
		})
		if len(result) > maxCount {
			break
		}
	}
	return result, nil
}

func defaultRoleHasPrivilegedPermission(codes []string) bool {
	for _, code := range codes {
		code = strings.ToLower(strings.TrimSpace(code))
		if code == "*" || strings.HasPrefix(code, "system:") || strings.HasPrefix(code, "admin:") {
			return true
		}
	}
	return false
}

func (r *Repository) InsertPlatform(ctx context.Context, platform domain.Platform, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_platform (
  platformCode, platformName, platformType, description, defaultRedirectUrl,
  allowAutoRegister, allowFormRegister, isDefault, defaultDeptId, brandJson, settingsJson, status,
  creatorId, updaterId, createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`),
		platform.PlatformCode, platform.PlatformName, platform.PlatformType, nullableString(platform.Description), nullableString(platform.DefaultRedirectURL),
		boolInt(platform.AllowAutoRegister), boolInt(platform.AllowFormRegister), boolInt(platform.IsDefault), nullableInt64(platform.DefaultDeptID), nullableString(platform.BrandJSON), nullableString(platform.SettingsJSON), platform.Status,
		nullableActor(actorID), nullableActor(actorID))
	if err != nil {
		return fmt.Errorf("insert platform: %w", err)
	}
	return nil
}

func (r *Repository) UpdatePlatform(ctx context.Context, platform domain.Platform, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_platform
SET platformName = ?, platformType = ?, description = ?, defaultRedirectUrl = ?,
    allowAutoRegister = ?, allowFormRegister = ?, isDefault = ?, defaultDeptId = ?, brandJson = ?, settingsJson = ?,
    updaterId = ?, updateTime = NOW()
WHERE platformCode = ? AND isDeleted = 0`),
		platform.PlatformName, platform.PlatformType, nullableString(platform.Description), nullableString(platform.DefaultRedirectURL),
		boolInt(platform.AllowAutoRegister), boolInt(platform.AllowFormRegister), boolInt(platform.IsDefault), nullableInt64(platform.DefaultDeptID), nullableString(platform.BrandJSON), nullableString(platform.SettingsJSON),
		nullableActor(actorID), platform.PlatformCode)
	if err != nil {
		return fmt.Errorf("update platform: %w", err)
	}
	return nil
}

func (r *Repository) UpdatePlatformStatus(ctx context.Context, platformCode string, status int, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_platform
SET status = ?, updaterId = ?, updateTime = NOW()
WHERE platformCode = ? AND isDeleted = 0`), status, nullableActor(actorID), strings.TrimSpace(platformCode))
	if err != nil {
		return fmt.Errorf("update platform status: %w", err)
	}
	return nil
}

func (r *Repository) ReplaceLoginMethods(ctx context.Context, platformCode string, methods []domain.LoginMethod, actorID int64) error {
	if len(methods) > platformLoginMethodMaxCount {
		return fmt.Errorf("replace platform login methods: item count %d exceeds limit %d", len(methods), platformLoginMethodMaxCount)
	}
	exec, err := r.requireTransactionExecutor(ctx)
	if err != nil {
		return err
	}
	code := strings.TrimSpace(platformCode)
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `DELETE FROM sys_platform_login_method WHERE platformCode = ? AND isDeleted = 1`), code); err != nil {
		return fmt.Errorf("purge deleted platform login methods: %w", err)
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_platform_login_method SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE platformCode = ? AND isDeleted = 0`), nullableActor(actorID), code); err != nil {
		return fmt.Errorf("delete platform login methods: %w", err)
	}
	for start := 0; start < len(methods); start += platformReplaceChunkSize {
		end := min(start+platformReplaceChunkSize, len(methods))
		chunk := methods[start:end]
		tuples := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*11)
		for _, method := range chunk {
			tuples = append(tuples, `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`)
			args = append(args,
				code, method.MethodType, method.ProviderCode, method.DisplayName, nullableString(method.Icon), method.SortOrder,
				boolInt(method.DisplayEnabled), boolInt(method.LoginEnabled), nullableString(method.MetadataJSON), nullableActor(actorID), nullableActor(actorID),
			)
		}
		query := `
INSERT INTO sys_platform_login_method (
  platformCode, methodType, providerCode, displayName, icon, sortOrder,
  displayEnabled, loginEnabled, metadataJson, creatorId, updaterId, createTime, updateTime, isDeleted
) VALUES ` + strings.Join(tuples, ", ")
		if _, err := exec.ExecContext(ctx, r.rebind(exec, query), args...); err != nil {
			return fmt.Errorf("insert platform login method: %w", err)
		}
	}
	return nil
}

func (r *Repository) ReplaceSourceRules(ctx context.Context, platformCode string, rules []domain.SourceRule, actorID int64) error {
	if len(rules) > platformSourceRuleMaxCount {
		return fmt.Errorf("replace platform source rules: item count %d exceeds limit %d", len(rules), platformSourceRuleMaxCount)
	}
	exec, err := r.requireTransactionExecutor(ctx)
	if err != nil {
		return err
	}
	code := strings.TrimSpace(platformCode)
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `DELETE FROM sys_platform_source_rule WHERE platformCode = ? AND isDeleted = 1`), code); err != nil {
		return fmt.Errorf("purge deleted platform source rules: %w", err)
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_platform_source_rule SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE platformCode = ? AND isDeleted = 0`), nullableActor(actorID), code); err != nil {
		return fmt.Errorf("delete platform source rules: %w", err)
	}
	for start := 0; start < len(rules); start += platformReplaceChunkSize {
		end := min(start+platformReplaceChunkSize, len(rules))
		chunk := rules[start:end]
		tuples := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*8)
		for _, rule := range chunk {
			tuples = append(tuples, `(?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`)
			args = append(args,
				code, rule.MatchType, rule.MatchValue, rule.Priority, rule.Status, nullableString(rule.MetadataJSON), nullableActor(actorID), nullableActor(actorID),
			)
		}
		query := `
INSERT INTO sys_platform_source_rule (
  platformCode, matchType, matchValue, priority, status, metadataJson,
  creatorId, updaterId, createTime, updateTime, isDeleted
) VALUES ` + strings.Join(tuples, ", ")
		if _, err := exec.ExecContext(ctx, r.rebind(exec, query), args...); err != nil {
			return fmt.Errorf("insert platform source rule: %w", err)
		}
	}
	return nil
}

func (r *Repository) ReplaceDefaultRoles(ctx context.Context, platformCode string, roles []domain.DefaultRole, actorID int64) error {
	if len(roles) > platformDefaultRoleMaxCount {
		return fmt.Errorf("replace platform default roles: item count %d exceeds limit %d", len(roles), platformDefaultRoleMaxCount)
	}
	exec, err := r.requireTransactionExecutor(ctx)
	if err != nil {
		return err
	}
	code := strings.TrimSpace(platformCode)
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `DELETE FROM sys_platform_default_role WHERE platformCode = ? AND isDeleted = 1`), code); err != nil {
		return fmt.Errorf("purge deleted platform default roles: %w", err)
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_platform_default_role SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE platformCode = ? AND isDeleted = 0`), nullableActor(actorID), code); err != nil {
		return fmt.Errorf("delete platform default roles: %w", err)
	}
	for start := 0; start < len(roles); start += platformReplaceChunkSize {
		end := min(start+platformReplaceChunkSize, len(roles))
		chunk := roles[start:end]
		tuples := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*6)
		for _, role := range chunk {
			tuples = append(tuples, `(?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`)
			args = append(args,
				code, role.RoleID, boolInt(role.AutoAssignEnabled), role.Status, nullableActor(actorID), nullableActor(actorID),
			)
		}
		query := `
INSERT INTO sys_platform_default_role (
  platformCode, roleId, autoAssignEnabled, status,
  creatorId, updaterId, createTime, updateTime, isDeleted
) VALUES ` + strings.Join(tuples, ", ")
		if _, err := exec.ExecContext(ctx, r.rebind(exec, query), args...); err != nil {
			return fmt.Errorf("insert platform default role: %w", err)
		}
	}
	return nil
}

func (r *Repository) ListAvailableExternalProviderCodes(ctx context.Context, providerCodes []string) ([]string, error) {
	return r.listExternalProviderCodes(ctx, providerCodes, false)
}

func (r *Repository) ListManagedExternalProviderCodes(ctx context.Context, providerCodes []string) ([]string, error) {
	return r.listExternalProviderCodes(ctx, providerCodes, true)
}

func (r *Repository) listExternalProviderCodes(ctx context.Context, providerCodes []string, managed bool) ([]string, error) {
	codes := uniqueProviderCodes(providerCodes)
	if len(codes) == 0 {
		return nil, nil
	}
	if len(codes) > platformLoginMethodMaxCount {
		return nil, fmt.Errorf("list external providers: item count %d exceeds limit %d", len(codes), platformLoginMethodMaxCount)
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	filter := "AND status = 0 AND loginEnabled = 1"
	if managed {
		filter = ""
	}
	query, args, err := sqlx.In(`
SELECT providerCode
FROM sys_external_login_provider
WHERE providerCode IN (?) `+filter+`
  AND isDeleted = 0
ORDER BY providerCode
LIMIT ?`, codes, len(codes)+1)
	if err != nil {
		return nil, fmt.Errorf("build external provider set query: %w", err)
	}
	var result []string
	if err := sqlx.SelectContext(ctx, exec, &result, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list external providers: %w", err)
	}
	if len(result) > len(codes) {
		return nil, fmt.Errorf("list external providers exceeded bounded result size")
	}
	return result, nil
}

func (r *Repository) ValidateDefaultRoles(ctx context.Context, roleIDs []int64) ([]domain.RoleSafety, error) {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`SELECT id AS roleId, code, COALESCE(systemKey, '') AS systemKey, status, type FROM sys_role WHERE id IN (?) AND NOT isDeleted`, ids)
	if err != nil {
		return nil, err
	}
	var roleRows []roleSafetyRow
	if err := sqlx.SelectContext(ctx, exec, &roleRows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("validate default roles: %w", err)
	}
	result := make(map[int64]domain.RoleSafety, len(ids))
	for _, id := range ids {
		result[id] = domain.RoleSafety{RoleID: id}
	}
	for _, row := range roleRows {
		item := result[row.RoleID]
		item.Exists = true
		item.Active = row.Status == domain.StatusActive
		item.AutoAssignable = !strings.EqualFold(strings.TrimSpace(row.SystemKey), authorizationfacade.AuthorizationRootSystemKey)
		result[row.RoleID] = item
	}
	permissionCodes, err := r.rolePermissionCodes(ctx, exec, ids)
	if err != nil {
		return nil, err
	}
	menuPermissionCodes, err := r.roleMenuPermissionCodes(ctx, exec, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RoleSafety, 0, len(ids))
	for _, id := range ids {
		item := result[id]
		item.PermissionCodes = permissionCodes[id]
		item.MenuPermissions = menuPermissionCodes[id]
		out = append(out, item)
	}
	return out, nil
}

const platformSelectSQL = `
SELECT id, platformCode, platformName, platformType, description, defaultRedirectUrl,
       allowAutoRegister, allowFormRegister, isDefault, defaultDeptId, brandJson, settingsJson, status,
       createTime, updateTime
`

func mapPlatformRows(rows []platformRow) []domain.Platform {
	result := make([]domain.Platform, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapPlatformRow(row))
	}
	return result
}

func mapPlatformRow(row platformRow) domain.Platform {
	return domain.Platform{
		ID:                 row.ID,
		PlatformCode:       row.PlatformCode,
		PlatformName:       row.PlatformName,
		PlatformType:       row.PlatformType,
		Description:        nullStringValue(row.Description),
		DefaultRedirectURL: nullStringValue(row.DefaultRedirectURL),
		AllowAutoRegister:  row.AllowAutoRegister == 1,
		AllowFormRegister:  row.AllowFormRegister == 1,
		IsDefault:          row.IsDefault == 1,
		DefaultDeptID:      nullInt64Ptr(row.DefaultDeptID),
		BrandJSON:          nullStringValue(row.BrandJSON),
		SettingsJSON:       nullStringValue(row.SettingsJSON),
		Status:             row.Status,
	}
}

func mapSourceRuleRows(rows []sourceRuleRow) []domain.SourceRule {
	result := make([]domain.SourceRule, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.SourceRule{
			ID:           row.ID,
			PlatformCode: row.PlatformCode,
			MatchType:    row.MatchType,
			MatchValue:   row.MatchValue,
			Priority:     row.Priority,
			Status:       row.Status,
			MetadataJSON: nullStringValue(row.MetadataJSON),
		})
	}
	return result
}

func mapLoginMethodRows(rows []loginMethodRow) []domain.LoginMethod {
	result := make([]domain.LoginMethod, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.LoginMethod{
			ID: row.ID, PlatformCode: row.PlatformCode, MethodType: row.MethodType,
			ProviderCode: row.ProviderCode, DisplayName: row.DisplayName, Icon: nullStringValue(row.Icon),
			SortOrder: row.SortOrder, DisplayEnabled: row.DisplayEnabled == 1, LoginEnabled: row.LoginEnabled == 1,
			MetadataJSON: nullStringValue(row.MetadataJSON),
		})
	}
	return result
}

func mapDefaultRoleRows(rows []defaultRoleRow) []domain.DefaultRole {
	result := make([]domain.DefaultRole, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.DefaultRole{
			ID:                row.ID,
			PlatformCode:      row.PlatformCode,
			RoleID:            row.RoleID,
			AutoAssignEnabled: row.AutoAssignEnabled == 1,
			Status:            row.Status,
		})
	}
	return result
}

func platformWhere(query domain.PlatformQuery) (string, []any) {
	conditions := []string{"isDeleted = 0"}
	args := []any{}
	if value := strings.TrimSpace(query.Keyword); value != "" {
		conditions = append(conditions, "(platformCode LIKE ? OR platformName LIKE ?)")
		args = append(args, "%"+value+"%", "%"+value+"%")
	}
	if value := strings.TrimSpace(query.PlatformCode); value != "" {
		conditions = append(conditions, "platformCode = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.PlatformType); value != "" {
		conditions = append(conditions, "platformType = ?")
		args = append(args, strings.ToUpper(value))
	}
	if query.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *query.Status)
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func pageArgs(current, pageSize int) (int, int) {
	if current <= 0 {
		current = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return current, pageSize
}

func (r *Repository) rolePermissionCodes(ctx context.Context, exec store.SQLX, roleIDs []int64) (map[int64][]string, error) {
	query, args, err := sqlx.In(`
SELECT rp.roleId, p.code
FROM sys_role_permission rp
JOIN sys_permission p ON p.id = rp.permissionId AND p.status = 0 AND NOT p.isDeleted
WHERE rp.roleId IN (?)`, roleIDs)
	if err != nil {
		return nil, err
	}
	var rows []permissionCodeRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list role permission codes: %w", err)
	}
	return permissionCodesByRole(rows), nil
}

func (r *Repository) roleMenuPermissionCodes(ctx context.Context, exec store.SQLX, roleIDs []int64) (map[int64][]string, error) {
	query, args, err := sqlx.In(`
SELECT rm.roleId, p.code
FROM sys_role_menu rm
JOIN sys_menu_permission mp ON mp.menuId = rm.menuId
JOIN sys_permission p ON p.id = mp.permissionId AND p.status = 0 AND NOT p.isDeleted
WHERE rm.roleId IN (?)`, roleIDs)
	if err != nil {
		return nil, err
	}
	var rows []permissionCodeRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list role menu permission codes: %w", err)
	}
	return permissionCodesByRole(rows), nil
}

func permissionCodesByRole(rows []permissionCodeRow) map[int64][]string {
	result := map[int64][]string{}
	for _, row := range rows {
		result[row.RoleID] = append(result[row.RoleID], row.Code)
	}
	return result
}

func uniquePositiveIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func uniqueProviderCodes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizedPlatformCodes(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) > platformDetailMaxCodes {
		return nil, fmt.Errorf("platform detail set exceeds %d", platformDetailMaxCodes)
	}
	sort.Strings(result)
	return result, nil
}

func platformCodeChunks(values []string) [][]string {
	result := make([][]string, 0, (len(values)+platformDetailChunkSize-1)/platformDetailChunkSize)
	for start := 0; start < len(values); start += platformDetailChunkSize {
		end := start + platformDetailChunkSize
		if end > len(values) {
			end = len(values)
		}
		result = append(result, values[start:end])
	}
	return result
}

func nullStringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	out := value.Int64
	return &out
}

func nullableInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func nullableActor(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
