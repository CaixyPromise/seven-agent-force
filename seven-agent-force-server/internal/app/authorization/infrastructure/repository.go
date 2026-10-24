package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	users    userfacade.AuthorizationContextFacade
	postgres bool
}

const authorizationBulkInsertChunkSize = 500

const (
	authorizationSetQueryChunkSize  = 200
	authorizationSetQueryMaxIDs     = 1000
	authorizationDerivedRelationMax = 10000
	authorizationFeatureCodeMax     = 32
)

const (
	rolePermissionSourceDirect = "DIRECT"
	rolePermissionSourceMenu   = "MENU"
	rolePermissionSourceBoth   = "BOTH"
)

func NewRepository(provider store.Provider, users userfacade.AuthorizationContextFacade) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("authorization repository requires datasource provider")
	}
	return &Repository{
		db:       provider.SQLX(),
		users:    users,
		postgres: strings.EqualFold(strings.TrimSpace(provider.Dialect()), "postgres"),
	}, nil
}

func (r *Repository) FindUserAggregate(ctx context.Context, userID int64) (*domain.UserAggregate, error) {
	if r == nil || r.users == nil || userID <= 0 {
		return nil, nil
	}
	item, err := r.users.GetAuthorizationUserAggregate(ctx, userID)
	if err != nil || item == nil {
		return nil, err
	}
	return &domain.UserAggregate{
		UserID:        item.UserID,
		Username:      item.Username,
		Nickname:      item.Nickname,
		Avatar:        item.Avatar,
		Email:         item.Email,
		Phone:         item.Phone,
		Enabled:       item.Enabled,
		Locked:        item.Locked,
		PrimaryOrgID:  item.PrimaryOrgID,
		PrimaryDeptID: item.PrimaryDeptID,
		PrimaryPostID: item.PrimaryPostID,
	}, nil
}

func (r *Repository) ListUserRoles(ctx context.Context, userID int64) ([]domain.RoleRecord, error) {
	roleIDs, err := r.listEffectiveRoleIDs(ctx, userID)
	if err != nil || len(roleIDs) == 0 {
		return []domain.RoleRecord{}, err
	}
	query, args, err := sqlx.In(`
SELECT DISTINCT
	sr.id AS role_id,
	sr.name AS name,
	sr.code AS code,
	COALESCE(sr.systemKey, '') AS system_key,
	COALESCE(sr.type, 0) AS type,
	sr.status AS status,
	COALESCE(sr.dataScope, 5) AS data_scope,
	COALESCE(sr.sortOrder, 0) AS sort_order,
	COALESCE(sr.remark, '') AS remark
FROM sys_role sr
WHERE sr.isDeleted = 0 AND sr.status = 0 AND sr.id IN (?)`, roleIDs)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	var items []domain.RoleRecord
	if err := sqlx.SelectContext(ctx, exec, &items, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list authorization user roles: %w", err)
	}
	return items, nil
}

func (r *Repository) ListUserPermissions(ctx context.Context, userID int64) ([]domain.PermissionRecord, error) {
	roleIDs, err := r.listEffectiveRoleIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	records := make([]domain.PermissionRecord, 0)
	if len(roleIDs) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list authorization user permissions: role set exceeds %d", authorizationSetQueryMaxIDs)
	}
	for _, ids := range chunkInt64s(roleIDs, authorizationSetQueryChunkSize) {
		remaining := authorizationSetQueryMaxIDs - len(records)
		query, args, err := sqlx.In(`
SELECT sp.code AS code, COALESCE(sp.featureCode, '') AS feature_code
FROM sys_role_permission srp
JOIN sys_permission sp ON sp.id = srp.permissionId
WHERE srp.roleId IN (?) AND sp.isDeleted = 0 AND sp.status = 0
ORDER BY sp.code, sp.id
LIMIT ?`, ids, remaining+1)
		if err != nil {
			return nil, err
		}
		var rows []domain.PermissionRecord
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list direct role permissions: %w", err)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("list authorization user permissions: result exceeds %d", authorizationSetQueryMaxIDs)
		}
		records = append(records, rows...)

		remaining = authorizationSetQueryMaxIDs - len(records)
		query, args, err = sqlx.In(`
SELECT sm.permission AS code, COALESCE(sm.featureCode, '') AS feature_code
FROM sys_role_menu srm
JOIN sys_menu sm ON sm.id = srm.menuId
WHERE srm.roleId IN (?) AND sm.isDeleted = 0 AND sm.status = 0
  AND COALESCE(sm.permission, '') <> ''
ORDER BY sm.permission, sm.id
LIMIT ?`, ids, remaining+1)
		if err != nil {
			return nil, err
		}
		rows = nil
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list role menu permissions: %w", err)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("list authorization user permissions: result exceeds %d", authorizationSetQueryMaxIDs)
		}
		records = append(records, rows...)
	}
	remaining := authorizationSetQueryMaxIDs - len(records)
	var temporary []domain.PermissionRecord
	if err := sqlx.SelectContext(ctx, exec, &temporary, r.rebind(exec, `
SELECT sp.code AS code, COALESCE(sp.featureCode, '') AS feature_code
FROM sys_user_permission sup
JOIN sys_permission sp ON sp.id = sup.permissionId
WHERE sup.userId = ? AND sup.isDeleted = 0 AND sp.isDeleted = 0 AND sp.status = 0
  AND (NOT sup.type OR sup.expireTime IS NULL OR sup.expireTime > NOW())
ORDER BY sp.code, sp.id, sup.id
LIMIT ?`), userID, remaining+1); err != nil {
		return nil, fmt.Errorf("list temporary user permissions: %w", err)
	}
	if len(temporary) > remaining {
		return nil, fmt.Errorf("list authorization user permissions: result exceeds %d", authorizationSetQueryMaxIDs)
	}
	records = append(records, temporary...)
	byKey := make(map[string]domain.PermissionRecord, len(records))
	for _, item := range records {
		item.Code = strings.TrimSpace(item.Code)
		item.FeatureCode = strings.TrimSpace(item.FeatureCode)
		if item.Code != "" {
			byKey[item.Code+"\x00"+item.FeatureCode] = item
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]domain.PermissionRecord, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result, nil
}

func (r *Repository) ListUserMenus(ctx context.Context, userID int64) ([]domain.MenuRecord, error) {
	roleIDs, err := r.listEffectiveRoleIDs(ctx, userID)
	if err != nil || len(roleIDs) == 0 {
		return []domain.MenuRecord{}, err
	}
	query, args, err := sqlx.In(`
SELECT DISTINCT
	sm.id AS menu_id,
	COALESCE(sm.parentId, 0) AS parent_id,
	COALESCE(sm.sortOrder, 0) AS sort_order,
	sm.name AS name,
	COALESCE(sm.path, '') AS path,
	COALESCE(sm.component, '') AS component,
	COALESCE(sm.type, '') AS type,
	COALESCE(sm.permission, '') AS permission,
	COALESCE(sm.featureCode, '') AS feature_code,
	COALESCE(sm.icon, '') AS icon,
	COALESCE(sm.status, 0) AS status,
	CASE WHEN sm.visible THEN 1 ELSE 0 END AS visible,
	CASE WHEN sm.isFrame THEN 1 ELSE 0 END AS is_frame,
	CASE WHEN sm.isCache THEN 1 ELSE 0 END AS is_cache,
	COALESCE(sm.remark, '') AS remark,
	sm.createTime AS create_time,
	sm.updateTime AS update_time
FROM sys_menu sm
JOIN sys_role_menu srm ON srm.menuId = sm.id
WHERE sm.isDeleted = 0 AND sm.status = 0 AND srm.roleId IN (?)
ORDER BY parent_id ASC, sort_order ASC, menu_id ASC`, roleIDs)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	var items []domain.MenuRecord
	if err := sqlx.SelectContext(ctx, exec, &items, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list authorization user menus: %w", err)
	}
	return items, nil
}

func (r *Repository) listEffectiveRoleIDs(ctx context.Context, userID int64) ([]int64, error) {
	if userID <= 0 {
		return []int64{}, nil
	}
	exec := store.SQLXExecutor(ctx, r.db)
	roleIDs := []int64{}
	if err := sqlx.SelectContext(ctx, exec, &roleIDs, r.rebind(exec, `
SELECT sur.roleId
FROM sys_user_role sur
JOIN sys_role sr ON sr.id = sur.roleId
WHERE sur.userId = ? AND sur.isDeleted = 0 AND sr.isDeleted = 0 AND sr.status = 0
ORDER BY sur.roleId, sur.id
LIMIT ?`), userID, authorizationSetQueryMaxIDs+1); err != nil {
		return nil, fmt.Errorf("list direct user roles: %w", err)
	}
	if len(roleIDs) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list effective role ids: direct role set exceeds %d", authorizationSetQueryMaxIDs)
	}
	postIDs := []int64{}
	if r.users != nil {
		posts, err := r.users.ListAuthorizationPosts(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, post := range posts {
			if post.PostID > 0 {
				postIDs = append(postIDs, post.PostID)
			}
		}
		if len(postIDs) > authorizationSetQueryMaxIDs {
			return nil, fmt.Errorf("list effective role ids: post set exceeds %d", authorizationSetQueryMaxIDs)
		}
	}
	if len(postIDs) > 0 {
		remaining := authorizationSetQueryMaxIDs - len(roleIDs)
		query, args, err := sqlx.In(`
SELECT spr.roleId
FROM sys_post_role spr
JOIN sys_role sr ON sr.id = spr.roleId
WHERE spr.postId IN (?) AND sr.isDeleted = 0 AND sr.status = 0
  AND COALESCE(sr.systemKey, '') <> ?
ORDER BY spr.roleId, spr.postId
LIMIT ?`, postIDs, domain.AuthorizationRootSystemKey, remaining+1)
		if err != nil {
			return nil, err
		}
		postRoleIDs := []int64{}
		if err := sqlx.SelectContext(ctx, exec, &postRoleIDs, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list post roles: %w", err)
		}
		if len(postRoleIDs) > remaining {
			return nil, fmt.Errorf("list effective role ids: accumulated role set exceeds %d", authorizationSetQueryMaxIDs)
		}
		roleIDs = append(roleIDs, postRoleIDs...)
	}
	return uniquePositiveIDs(roleIDs), nil
}

func (r *Repository) ListUserOrganizations(ctx context.Context, userID int64) ([]domain.OrgRecord, error) {
	if r == nil || r.users == nil {
		return []domain.OrgRecord{}, nil
	}
	items, err := r.users.ListAuthorizationOrganizations(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OrgRecord, 0, len(items))
	for _, item := range items {
		result = append(result, domain.OrgRecord{OrgID: item.OrgID, Code: item.Code, Name: item.Name, IsPrimary: item.IsPrimary})
	}
	return result, nil
}

func (r *Repository) ListUserDepartments(ctx context.Context, userID int64) ([]domain.DeptRecord, error) {
	if r == nil || r.users == nil {
		return []domain.DeptRecord{}, nil
	}
	items, err := r.users.ListAuthorizationDepartments(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.DeptRecord, 0, len(items))
	for _, item := range items {
		result = append(result, domain.DeptRecord{DeptID: item.DeptID, OrgID: item.OrgID, Code: item.Code, Name: item.Name, Hierarchy: item.Hierarchy, IsPrimary: item.IsPrimary})
	}
	return result, nil
}

func (r *Repository) ListUserPosts(ctx context.Context, userID int64) ([]domain.PostRecord, error) {
	if r == nil || r.users == nil {
		return []domain.PostRecord{}, nil
	}
	items, err := r.users.ListAuthorizationPosts(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.PostRecord, 0, len(items))
	for _, item := range items {
		result = append(result, domain.PostRecord{PostID: item.PostID, OrgID: item.OrgID, DeptID: item.DeptID, Code: item.Code, Name: item.Name, IsPrimary: item.IsPrimary})
	}
	return result, nil
}

func (r *Repository) ListRoleDeptIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	if len(roleIDs) == 0 {
		return []int64{}, nil
	}
	query, args, err := sqlx.In(`SELECT DISTINCT deptId FROM sys_role_dept WHERE roleId IN (?)`, roleIDs)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	query = r.rebind(exec, query)
	var values []int64
	if err := sqlx.SelectContext(ctx, exec, &values, query, args...); err != nil {
		return nil, fmt.Errorf("list authorization role dept ids: %w", err)
	}
	return values, nil
}

func (r *Repository) ListDeptIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	if roleID <= 0 {
		return []int64{}, nil
	}
	exec := store.SQLXExecutor(ctx, r.db)
	values := []int64{}
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, `SELECT deptId FROM sys_role_dept WHERE roleId = ? ORDER BY deptId ASC`), roleID); err != nil {
		return nil, fmt.Errorf("list authorization role dept ids by role: %w", err)
	}
	return uniquePositiveIDs(values), nil
}

func (r *Repository) ListDeptHierarchyMap(ctx context.Context, deptIDs []int64) (map[int64]string, error) {
	if r == nil || r.users == nil {
		return map[int64]string{}, nil
	}
	return r.users.ListDeptHierarchyMap(ctx, deptIDs)
}

func (r *Repository) ListDeptIDsByHierarchies(ctx context.Context, hierarchies []string) (map[string][]int64, error) {
	if r == nil || r.users == nil {
		return map[string][]int64{}, nil
	}
	if len(hierarchies) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list department ids by hierarchies: hierarchy set exceeds %d", authorizationSetQueryMaxIDs)
	}
	result, err := r.users.ListDeptIDsByHierarchies(ctx, hierarchies)
	if err != nil {
		return nil, err
	}
	total := 0
	for _, ids := range result {
		total += len(ids)
		if total > authorizationSetQueryMaxIDs {
			return nil, fmt.Errorf("list department ids by hierarchies: result exceeds %d", authorizationSetQueryMaxIDs)
		}
	}
	return result, nil
}

func (r *Repository) ListRoleList(ctx context.Context) ([]domain.RoleRecord, error) {
	query := `
SELECT
	id AS role_id,
	name AS name,
	code AS code,
	COALESCE(systemKey, '') AS system_key,
	COALESCE(type, 0) AS type,
	status AS status,
	COALESCE(dataScope, 5) AS data_scope,
	COALESCE(grantRevision, 0) AS grant_revision,
	COALESCE(sortOrder, 0) AS sort_order,
	COALESCE(remark, '') AS remark,
	createTime AS create_time,
	updateTime AS update_time
FROM sys_role
WHERE isDeleted = 0 AND status = 0
ORDER BY sortOrder ASC, id ASC`
	exec := store.SQLXExecutor(ctx, r.db)
	var items []domain.RoleRecord
	if err := sqlx.SelectContext(ctx, exec, &items, r.rebind(exec, query)); err != nil {
		return nil, fmt.Errorf("list role list: %w", err)
	}
	return items, nil
}

func (r *Repository) PageRoles(ctx context.Context, query authorizationfacade.RolePageQuery) ([]domain.RoleRecord, int64, error) {
	conditions := []string{"isDeleted = 0"}
	args := []any{}
	if strings.TrimSpace(query.Code) != "" {
		conditions = append(conditions, "code LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Code)+"%")
	}
	if strings.TrimSpace(query.Name) != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Name)+"%")
	}
	if query.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *query.Status)
	}
	where := " WHERE " + strings.Join(conditions, " AND ")
	total, err := r.count(ctx, `SELECT COUNT(1) FROM sys_role`+where, args...)
	if err != nil {
		return nil, 0, err
	}
	current, size := pageArgs(query.Current, query.Size)
	selectArgs := append(append([]any{}, args...), size, (current-1)*size)
	exec := store.SQLXExecutor(ctx, r.db)
	var records []domain.RoleRecord
	err = sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, `
SELECT
	id AS role_id,
	name AS name,
	code AS code,
	COALESCE(systemKey, '') AS system_key,
	COALESCE(type, 0) AS type,
	status AS status,
	COALESCE(dataScope, 5) AS data_scope,
	COALESCE(grantRevision, 0) AS grant_revision,
	COALESCE(sortOrder, 0) AS sort_order,
	COALESCE(remark, '') AS remark,
	createTime AS create_time,
	updateTime AS update_time
FROM sys_role`+where+`
ORDER BY sortOrder ASC, id ASC
LIMIT ? OFFSET ?`), selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page roles: %w", err)
	}
	return records, int64(total), nil
}

func (r *Repository) FindRoleByID(ctx context.Context, roleID int64) (*domain.RoleRecord, error) {
	return findOne[domain.RoleRecord](ctx, r.db, r.rebind, `
SELECT id AS role_id, name, code, COALESCE(systemKey, '') AS system_key, COALESCE(type, 0) AS type, status, COALESCE(dataScope, 5) AS data_scope,
       COALESCE(grantRevision, 0) AS grant_revision, COALESCE(sortOrder, 0) AS sort_order, COALESCE(remark, '') AS remark, createTime AS create_time, updateTime AS update_time
FROM sys_role WHERE id = ? AND isDeleted = 0 LIMIT 1`, roleID)
}

func (r *Repository) LockRoleGrant(ctx context.Context, roleID int64) (*domain.RoleRecord, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	var role domain.RoleRecord
	err := sqlx.GetContext(ctx, exec, &role, r.rebind(exec, `
SELECT id AS role_id, name, code, COALESCE(systemKey, '') AS system_key, COALESCE(type, 0) AS type,
       status, COALESCE(dataScope, 5) AS data_scope, COALESCE(grantRevision, 0) AS grant_revision,
       COALESCE(sortOrder, 0) AS sort_order, COALESCE(remark, '') AS remark,
       createTime AS create_time, updateTime AS update_time
FROM sys_role
WHERE id = ? AND isDeleted = 0
LIMIT 1
FOR UPDATE`), roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock role grant: %w", err)
	}
	return &role, nil
}

func (r *Repository) LockRoleGrants(ctx context.Context, roleIDs []int64) ([]domain.RoleRecord, error) {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return []domain.RoleRecord{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("lock role grants exceeds limit")
	}
	query, args, err := sqlx.In(`
SELECT id AS role_id, name, code, COALESCE(systemKey, '') AS system_key, COALESCE(type, 0) AS type,
       status, COALESCE(dataScope, 5) AS data_scope, COALESCE(grantRevision, 0) AS grant_revision,
       COALESCE(sortOrder, 0) AS sort_order, COALESCE(remark, '') AS remark,
       createTime AS create_time, updateTime AS update_time
FROM sys_role
WHERE id IN (?) AND isDeleted = 0
ORDER BY id ASC
FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	roles := make([]domain.RoleRecord, 0, len(ids))
	if err := sqlx.SelectContext(ctx, exec, &roles, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("lock role grants: %w", err)
	}
	return roles, nil
}

func (r *Repository) TouchRoleGrantGuards(ctx context.Context, roleIDs []int64) error {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return fmt.Errorf("touch role grant guards exceeds limit")
	}
	query, args, err := sqlx.In(`UPDATE sys_role SET updateTime = NOW() WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return err
	}
	return r.exec(ctx, query, args...)
}

func (r *Repository) FindRoleGrantRequest(ctx context.Context, roleID int64, idempotencyKey string) (*domain.RoleGrantRequestRecord, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	var record domain.RoleGrantRequestRecord
	err := sqlx.GetContext(ctx, exec, &record, r.rebind(exec, `
SELECT roleId AS role_id, idempotencyKey AS idempotency_key, requestHash AS request_hash,
       resultRevision AS result_revision, impactedUserCount AS impacted_user_count, changed
FROM sys_role_grant_request
WHERE roleId = ? AND idempotencyKey = ?
LIMIT 1`), roleID, strings.TrimSpace(idempotencyKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find role grant request: %w", err)
	}
	return &record, nil
}

func (r *Repository) CreateRoleGrantRequest(ctx context.Context, record domain.RoleGrantRequestRecord, operatorID int64) error {
	return r.exec(ctx, `
INSERT INTO sys_role_grant_request
  (roleId, idempotencyKey, requestHash, resultRevision, impactedUserCount, changed, operatorId, createTime)
VALUES (?, ?, ?, ?, ?, ?, ?, NOW())`,
		record.RoleID, strings.TrimSpace(record.IdempotencyKey), record.RequestHash, record.ResultRevision,
		record.ImpactedUserCount, record.Changed, nullableInt64(operatorID))
}

func (r *Repository) UpdateRoleGrantDataScope(ctx context.Context, roleID int64, dataScope int, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_role SET dataScope = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, dataScope, nullableInt64(operatorID), roleID)
}

func (r *Repository) UpdateRoleGrantRevision(ctx context.Context, roleID, expectedRevision, nextRevision, operatorID int64) error {
	exec := store.SQLXExecutor(ctx, r.db)
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_role
SET grantRevision = ?, updaterId = ?, updateTime = NOW()
WHERE id = ? AND isDeleted = 0 AND grantRevision = ?`), nextRevision, nullableInt64(operatorID), roleID, expectedRevision)
	if err != nil {
		return fmt.Errorf("update role grant revision: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read role grant revision result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("role grant revision changed concurrently")
	}
	return nil
}

func (r *Repository) UpdateRoleGrantRevisions(ctx context.Context, roles []domain.RoleRecord, operatorID int64) error {
	normalized := make([]domain.RoleRecord, 0, len(roles))
	seen := make(map[int64]struct{}, len(roles))
	for _, role := range roles {
		if role.RoleID <= 0 {
			continue
		}
		if _, ok := seen[role.RoleID]; ok {
			return fmt.Errorf("batch role grant revision contains duplicate role")
		}
		seen[role.RoleID] = struct{}{}
		normalized = append(normalized, role)
	}
	if len(normalized) == 0 {
		return nil
	}
	if len(normalized) > authorizationSetQueryMaxIDs {
		return fmt.Errorf("batch role grant revision exceeds role limit")
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].RoleID < normalized[j].RoleID })
	exec := store.SQLXExecutor(ctx, r.db)
	for start := 0; start < len(normalized); start += authorizationSetQueryChunkSize {
		end := min(start+authorizationSetQueryChunkSize, len(normalized))
		chunk := normalized[start:end]
		caseClauses := make([]string, 0, len(chunk))
		whereClauses := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*4+1)
		for _, role := range chunk {
			caseClauses = append(caseClauses, `WHEN ? THEN ?`)
			args = append(args, role.RoleID, role.GrantRevision+1)
		}
		args = append(args, nullableInt64(operatorID))
		for _, role := range chunk {
			whereClauses = append(whereClauses, `(id = ? AND grantRevision = ?)`)
			args = append(args, role.RoleID, role.GrantRevision)
		}
		query := `UPDATE sys_role
SET grantRevision = CASE id ` + strings.Join(caseClauses, ` `) + ` ELSE grantRevision END,
    updaterId = ?, updateTime = NOW()
WHERE isDeleted = 0 AND (` + strings.Join(whereClauses, ` OR `) + `)`
		result, err := exec.ExecContext(ctx, r.rebind(exec, query), args...)
		if err != nil {
			return fmt.Errorf("batch update role grant revisions: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("batch update role grant revisions affected rows: %w", err)
		}
		if affected != int64(len(chunk)) {
			return fmt.Errorf("role grant revision conflict")
		}
	}
	return nil
}

func (r *Repository) CountRoleCodeExcludingID(ctx context.Context, roleID int64, code string) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_role WHERE code = ? AND id <> ? AND isDeleted = 0`, strings.TrimSpace(code), roleID)
}

func (r *Repository) CountRolesByIDs(ctx context.Context, roleIDs []int64) (int, error) {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	query, args, err := sqlx.In(`SELECT COUNT(DISTINCT id) FROM sys_role WHERE id IN (?) AND isDeleted = 0 AND status = 0`, ids)
	if err != nil {
		return 0, err
	}
	return r.count(ctx, query, args...)
}

func (r *Repository) CountAuthorizationRootRolesByIDs(ctx context.Context, roleIDs []int64) (int, error) {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return 0, fmt.Errorf("count authorization root roles: role set exceeds %d", authorizationSetQueryMaxIDs)
	}
	query, args, err := sqlx.In(`
SELECT COUNT(DISTINCT id)
FROM sys_role
WHERE id IN (?) AND systemKey = ? AND isDeleted = 0`, ids, domain.AuthorizationRootSystemKey)
	if err != nil {
		return 0, err
	}
	return r.count(ctx, query, args...)
}

func (r *Repository) LockSuperAdminInvariant(ctx context.Context, targetUserID int64) (domain.SuperAdminInvariantSnapshot, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	var roleID int64
	if err := sqlx.GetContext(ctx, exec, &roleID, r.rebind(exec, `
SELECT id
FROM sys_role
WHERE systemKey = ? AND isDeleted = 0
ORDER BY id ASC
LIMIT 1
FOR UPDATE`), domain.AuthorizationRootSystemKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.SuperAdminInvariantSnapshot{}, nil
		}
		return domain.SuperAdminInvariantSnapshot{}, fmt.Errorf("lock authorization root invariant: %w", err)
	}
	var userIDs []int64
	if err := sqlx.SelectContext(ctx, exec, &userIDs, r.rebind(exec, `
SELECT su.id
FROM sys_user su
JOIN sys_user_role sur ON sur.userId = su.id AND sur.isDeleted = 0
JOIN sys_role sr ON sr.id = sur.roleId
WHERE su.isDeleted = 0
	AND su.status = 0
	AND sr.id = ?
	AND sr.systemKey = ?
	AND sr.status = 0
	AND sr.isDeleted = 0
FOR UPDATE`), roleID, domain.AuthorizationRootSystemKey); err != nil {
		return domain.SuperAdminInvariantSnapshot{}, fmt.Errorf("read authorization root invariant: %w", err)
	}
	activeUsers := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		activeUsers[userID] = struct{}{}
	}
	_, targetUserActive := activeUsers[targetUserID]
	return domain.SuperAdminInvariantSnapshot{ActiveUserCount: len(activeUsers), TargetUserActive: targetUserActive}, nil
}

func (r *Repository) GetAuthorizationRootSecuritySnapshot(ctx context.Context) (*domain.AuthorizationRootSecuritySnapshot, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	role, err := findOne[domain.RoleRecord](ctx, r.db, r.rebind, `
SELECT id AS role_id, name, code, COALESCE(systemKey, '') AS system_key, COALESCE(type, 0) AS type,
       status, COALESCE(dataScope, 5) AS data_scope, COALESCE(grantRevision, 0) AS grant_revision, COALESCE(sortOrder, 0) AS sort_order,
       COALESCE(remark, '') AS remark, createTime AS create_time, updateTime AS update_time
FROM sys_role WHERE systemKey = ? AND isDeleted = 0 LIMIT 1`, domain.AuthorizationRootSystemKey)
	if err != nil || role == nil {
		return nil, err
	}
	var count int
	if err := sqlx.GetContext(ctx, exec, &count, r.rebind(exec, `
SELECT COUNT(DISTINCT su.id)
FROM sys_user su
JOIN sys_user_role sur ON sur.userId = su.id AND sur.isDeleted = 0
WHERE sur.roleId = ? AND su.status = 0 AND su.isDeleted = 0`), role.RoleID); err != nil {
		return nil, fmt.Errorf("count active authorization root users: %w", err)
	}
	return &domain.AuthorizationRootSecuritySnapshot{Role: *role, ActiveUserCount: count}, nil
}

func (r *Repository) BootstrapAuthorizationRoot(ctx context.Context, code, name string, initializedAt time.Time) (*domain.AuthorizationRootBootstrapResult, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	role := domain.RoleRecord{}
	if err := sqlx.GetContext(ctx, exec, &role, r.rebind(exec, `
SELECT id AS role_id, name, code, COALESCE(systemKey, '') AS system_key, COALESCE(type, 0) AS type,
       status, COALESCE(dataScope, 5) AS data_scope, COALESCE(grantRevision, 0) AS grant_revision, COALESCE(sortOrder, 0) AS sort_order,
       COALESCE(remark, '') AS remark, createTime AS create_time, updateTime AS update_time
FROM sys_role
WHERE systemKey = ? AND isDeleted = 0
LIMIT 1
FOR UPDATE`), domain.AuthorizationRootSystemKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("authorization root role is missing")
		}
		return nil, fmt.Errorf("lock authorization root bootstrap: %w", err)
	}

	var bootstrapKey string
	err := sqlx.GetContext(ctx, exec, &bootstrapKey, r.rebind(exec, `
SELECT bootstrapKey
FROM sys_security_bootstrap
WHERE bootstrapKey = ?
FOR UPDATE`), domain.AuthorizationRootSystemKey)
	if err == nil {
		return &domain.AuthorizationRootBootstrapResult{Role: role, AlreadyInitialized: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read authorization root bootstrap: %w", err)
	}

	var userCount int
	if err := sqlx.GetContext(ctx, exec, &userCount, r.rebind(exec, `SELECT COUNT(1) FROM sys_user WHERE isDeleted = 0`)); err != nil {
		return nil, fmt.Errorf("count users before authorization root bootstrap: %w", err)
	}
	if userCount > 0 {
		if _, err := exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_security_bootstrap (bootstrapKey, rootRoleId, rootRoleCode, initializedAt, createTime, updateTime)
VALUES (?, ?, ?, ?, ?, ?)`), domain.AuthorizationRootSystemKey, role.RoleID, role.Code, initializedAt, initializedAt, initializedAt); err != nil {
			return nil, fmt.Errorf("record upgraded authorization root bootstrap: %w", err)
		}
		return &domain.AuthorizationRootBootstrapResult{Role: role, AlreadyInitialized: true}, nil
	}

	var conflictCount int
	if err := sqlx.GetContext(ctx, exec, &conflictCount, r.rebind(exec, `
SELECT COUNT(1) FROM sys_role WHERE code = ? AND id <> ? AND isDeleted = 0`), code, role.RoleID); err != nil {
		return nil, fmt.Errorf("check authorization root code conflict: %w", err)
	}
	if conflictCount > 0 {
		return nil, fmt.Errorf("authorization root code conflicts with an existing role")
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_role
SET code = ?, name = ?, updateTime = ?
WHERE id = ? AND systemKey = ? AND isDeleted = 0`), code, name, initializedAt, role.RoleID, domain.AuthorizationRootSystemKey); err != nil {
		return nil, fmt.Errorf("apply authorization root bootstrap identity: %w", err)
	}
	role.Code = code
	role.Name = name
	role.UpdateTime = &initializedAt
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_security_bootstrap (bootstrapKey, rootRoleId, rootRoleCode, initializedAt, createTime, updateTime)
VALUES (?, ?, ?, ?, ?, ?)`), domain.AuthorizationRootSystemKey, role.RoleID, role.Code, initializedAt, initializedAt, initializedAt); err != nil {
		return nil, fmt.Errorf("record authorization root bootstrap: %w", err)
	}
	return &domain.AuthorizationRootBootstrapResult{Role: role}, nil
}

func (r *Repository) LockAuthorizationCreationGuard(ctx context.Context) error {
	exec := store.SQLXExecutor(ctx, r.db)
	var roleID int64
	if err := sqlx.GetContext(ctx, exec, &roleID, r.rebind(exec, `
SELECT id
FROM sys_role
WHERE systemKey = ? AND isDeleted = 0
FOR UPDATE`), domain.AuthorizationRootSystemKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("authorization creation guard is missing")
		}
		return fmt.Errorf("lock authorization creation guard: %w", err)
	}
	return r.exec(ctx, `UPDATE sys_role SET updateTime = NOW() WHERE id = ? AND isDeleted = 0`, roleID)
}

func (r *Repository) CountDeptsByIDs(ctx context.Context, deptIDs []int64) (int, error) {
	ids := uniquePositiveIDs(deptIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	query, args, err := sqlx.In(`SELECT COUNT(DISTINCT id) FROM sys_dept WHERE id IN (?) AND isDeleted = 0 AND status = 0`, ids)
	if err != nil {
		return 0, err
	}
	return r.count(ctx, query, args...)
}

func (r *Repository) ListPermissionCodesByRoleIDs(ctx context.Context, roleIDs []int64) ([]string, error) {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return []string{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list permission codes by role ids: role set exceeds %d", authorizationSetQueryMaxIDs)
	}
	exec := store.SQLXExecutor(ctx, r.db)
	type row struct {
		PermissionCode string `db:"permission_code"`
	}
	codeSet := make(map[string]struct{})
	for _, chunk := range chunkInt64s(ids, authorizationSetQueryChunkSize) {
		remaining := authorizationSetQueryMaxIDs - len(codeSet)
		query, args, err := sqlx.In(`
SELECT sp.code AS permission_code
FROM sys_role_permission srp
JOIN sys_permission sp ON sp.id = srp.permissionId
WHERE srp.roleId IN (?) AND sp.isDeleted = 0 AND sp.status = 0
ORDER BY sp.code, sp.id
LIMIT ?`, chunk, remaining+1)
		if err != nil {
			return nil, err
		}
		var rows []row
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list direct permission codes by role ids: %w", err)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("list permission codes by role ids: result exceeds %d", authorizationSetQueryMaxIDs)
		}
		for _, item := range rows {
			if code := strings.TrimSpace(item.PermissionCode); code != "" {
				codeSet[code] = struct{}{}
			}
		}
		remaining = authorizationSetQueryMaxIDs - len(codeSet)
		query, args, err = sqlx.In(`
SELECT sm.permission AS permission_code
FROM sys_role_menu srm
JOIN sys_menu sm ON sm.id = srm.menuId
WHERE srm.roleId IN (?) AND sm.isDeleted = 0 AND sm.status = 0
  AND COALESCE(sm.permission, '') <> ''
ORDER BY sm.permission, sm.id
LIMIT ?`, chunk, remaining+1)
		if err != nil {
			return nil, err
		}
		rows = nil
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list menu permission codes by role ids: %w", err)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("list permission codes by role ids: result exceeds %d", authorizationSetQueryMaxIDs)
		}
		for _, item := range rows {
			if code := strings.TrimSpace(item.PermissionCode); code != "" {
				codeSet[code] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(codeSet))
	for code := range codeSet {
		result = append(result, code)
	}
	sort.Strings(result)
	return result, nil
}

func (r *Repository) CreateRole(ctx context.Context, record domain.RoleRecord, operatorID int64) error {
	return r.exec(ctx, `INSERT INTO sys_role (id, name, code, type, status, dataScope, sortOrder, remark, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`,
		record.RoleID, strings.TrimSpace(record.Name), strings.TrimSpace(record.Code), record.Type, record.Status, record.DataScope, record.SortOrder, nullableString(record.Remark), nullableInt64(operatorID), nullableInt64(operatorID))
}

func (r *Repository) UpdateRole(ctx context.Context, record domain.RoleRecord, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_role SET name = ?, code = ?, type = ?, status = ?, dataScope = ?, sortOrder = ?, remark = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`,
		strings.TrimSpace(record.Name), strings.TrimSpace(record.Code), record.Type, record.Status, record.DataScope, record.SortOrder, nullableString(record.Remark), nullableInt64(operatorID), record.RoleID)
}

func (r *Repository) DeleteRole(ctx context.Context, roleID int64, operatorID int64) error {
	if err := r.exec(ctx, `DELETE FROM sys_role_menu WHERE roleId = ?`, roleID); err != nil {
		return err
	}
	if err := r.exec(ctx, `DELETE FROM sys_role_permission WHERE roleId = ?`, roleID); err != nil {
		return err
	}
	if err := r.exec(ctx, `DELETE FROM sys_role_dept WHERE roleId = ?`, roleID); err != nil {
		return err
	}
	return r.exec(ctx, `UPDATE sys_role SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, nullableInt64(operatorID), roleID)
}

func (r *Repository) CountUserRoleReferences(ctx context.Context, roleID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user_role WHERE roleId = ? AND isDeleted = 0`, roleID)
}

func (r *Repository) CountPostRoleReferences(ctx context.Context, roleID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_post_role WHERE roleId = ?`, roleID)
}

func (r *Repository) CountUserIDsByRoleID(ctx context.Context, roleID int64) (int, error) {
	return r.count(ctx, `
SELECT COUNT(1) FROM (
	SELECT userId FROM sys_user_role WHERE roleId = ? AND isDeleted = 0
	UNION
	SELECT sup.userId
	FROM sys_user_position sup
	JOIN sys_post_role spr ON spr.postId = sup.postId
	WHERE spr.roleId = ? AND sup.isDeleted = 0
) affected_users`, roleID, roleID)
}

func (r *Repository) ListUserIDsByRoleIDsPage(ctx context.Context, roleIDs []int64, afterUserID int64, limit int) ([]int64, error) {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list user ids by role ids page exceeds role limit")
	}
	if afterUserID < 0 {
		return nil, fmt.Errorf("list user ids by role ids page cursor is invalid")
	}
	if limit <= 0 || limit > authorizationSetQueryChunkSize {
		return nil, fmt.Errorf("list user ids by role ids page limit is invalid")
	}
	query, args, err := sqlx.In(`
SELECT userId FROM (
	SELECT userId
	FROM sys_user_role
	WHERE roleId IN (?) AND isDeleted = 0 AND userId > ?
	UNION
	SELECT sup.userId
	FROM sys_user_position sup
	JOIN sys_post_role spr ON spr.postId = sup.postId
	WHERE spr.roleId IN (?) AND sup.isDeleted = 0 AND sup.userId > ?
) t
ORDER BY userId ASC
LIMIT ?`, ids, afterUserID, ids, afterUserID, limit)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	values := make([]int64, 0, limit)
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list user ids by role ids page: %w", err)
	}
	return uniquePositiveIDs(values), nil
}

func (r *Repository) ListAllMenus(ctx context.Context) ([]domain.MenuRecord, error) {
	return r.ListMenus(ctx, false)
}

func (r *Repository) ListMenus(ctx context.Context, enabledOnly bool) ([]domain.MenuRecord, error) {
	where := "WHERE isDeleted = 0"
	if enabledOnly {
		where += " AND status = 0"
	}
	query := `
SELECT
	id AS menu_id,
	COALESCE(parentId, 0) AS parent_id,
	COALESCE(sortOrder, 0) AS sort_order,
	name AS name,
	COALESCE(path, '') AS path,
	COALESCE(component, '') AS component,
	COALESCE(type, '') AS type,
	COALESCE(permission, '') AS permission,
	COALESCE(featureCode, '') AS feature_code,
	COALESCE(icon, '') AS icon,
	COALESCE(status, 0) AS status,
	CASE WHEN visible THEN 1 ELSE 0 END AS visible,
	CASE WHEN isFrame THEN 1 ELSE 0 END AS is_frame,
	CASE WHEN isCache THEN 1 ELSE 0 END AS is_cache,
	COALESCE(remark, '') AS remark,
	createTime AS create_time,
	updateTime AS update_time
FROM sys_menu
` + where + `
ORDER BY parent_id ASC, sort_order ASC, menu_id ASC
LIMIT ?`
	exec := store.SQLXExecutor(ctx, r.db)
	var items []domain.MenuRecord
	if err := sqlx.SelectContext(ctx, exec, &items, r.rebind(exec, query), authorizationSetQueryMaxIDs+1); err != nil {
		return nil, fmt.Errorf("list all menus: %w", err)
	}
	if len(items) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list all menus: result exceeds %d", authorizationSetQueryMaxIDs)
	}
	return items, nil
}

func (r *Repository) FindMenuByID(ctx context.Context, menuID int64) (*domain.MenuRecord, error) {
	return findOne[domain.MenuRecord](ctx, r.db, r.rebind, `
SELECT id AS menu_id, COALESCE(parentId, 0) AS parent_id, COALESCE(sortOrder, 0) AS sort_order, name,
       COALESCE(path, '') AS path, COALESCE(component, '') AS component, COALESCE(type, '') AS type,
       COALESCE(permission, '') AS permission, COALESCE(featureCode, '') AS feature_code,
       COALESCE(icon, '') AS icon, COALESCE(status, 0) AS status,
       CASE WHEN visible THEN 1 ELSE 0 END AS visible,
       CASE WHEN isFrame THEN 1 ELSE 0 END AS is_frame,
       CASE WHEN isCache THEN 1 ELSE 0 END AS is_cache,
       COALESCE(remark, '') AS remark, createTime AS create_time, updateTime AS update_time
FROM sys_menu WHERE id = ? AND isDeleted = 0 LIMIT 1`, menuID)
}

func (r *Repository) LockMenuGrants(ctx context.Context, menuIDs []int64) ([]domain.MenuRecord, error) {
	ids := uniquePositiveIDs(menuIDs)
	if len(ids) == 0 {
		return []domain.MenuRecord{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("lock menu grants exceeds limit")
	}
	query, args, err := sqlx.In(`
SELECT id AS menu_id, COALESCE(parentId, 0) AS parent_id, COALESCE(sortOrder, 0) AS sort_order, name,
       COALESCE(path, '') AS path, COALESCE(component, '') AS component, COALESCE(type, '') AS type,
       COALESCE(permission, '') AS permission, COALESCE(featureCode, '') AS feature_code,
       COALESCE(icon, '') AS icon, COALESCE(status, 0) AS status,
       CASE WHEN visible THEN 1 ELSE 0 END AS visible,
       CASE WHEN isFrame THEN 1 ELSE 0 END AS is_frame,
       CASE WHEN isCache THEN 1 ELSE 0 END AS is_cache,
       COALESCE(remark, '') AS remark, createTime AS create_time, updateTime AS update_time
FROM sys_menu
WHERE id IN (?) AND isDeleted = 0
ORDER BY id ASC
FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	menus := make([]domain.MenuRecord, 0, len(ids))
	if err := sqlx.SelectContext(ctx, exec, &menus, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("lock menu grants: %w", err)
	}
	return menus, nil
}

func (r *Repository) TouchMenuGrantGuards(ctx context.Context, menuIDs []int64) error {
	ids := uniquePositiveIDs(menuIDs)
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return fmt.Errorf("touch menu grant guards exceeds limit")
	}
	query, args, err := sqlx.In(`UPDATE sys_menu SET updateTime = NOW() WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return err
	}
	return r.exec(ctx, query, args...)
}

func (r *Repository) CountMenuPermissionExcludingID(ctx context.Context, menuID int64, permission string) (int, error) {
	trimmed := strings.TrimSpace(permission)
	if trimmed == "" {
		return 0, nil
	}
	return r.count(ctx, `SELECT COUNT(1) FROM sys_menu WHERE permission = ? AND id <> ? AND isDeleted = 0`, trimmed, menuID)
}

func (r *Repository) CountMenuChildren(ctx context.Context, menuID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_menu WHERE parentId = ? AND isDeleted = 0`, menuID)
}

func (r *Repository) CountRoleMenuReferences(ctx context.Context, menuID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_role_menu WHERE menuId = ?`, menuID)
}

func (r *Repository) CountMenusByIDs(ctx context.Context, menuIDs []int64) (int, error) {
	ids := uniquePositiveIDs(menuIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	query, args, err := sqlx.In(`SELECT COUNT(DISTINCT id) FROM sys_menu WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return 0, err
	}
	return r.count(ctx, query, args...)
}

func (r *Repository) ListMenuPermissionCodes(ctx context.Context, menuIDs []int64) ([]string, error) {
	ids := uniquePositiveIDs(menuIDs)
	if len(ids) == 0 {
		return []string{}, nil
	}
	query, args, err := sqlx.In(`
SELECT DISTINCT COALESCE(permission, '')
FROM sys_menu
WHERE id IN (?) AND isDeleted = 0 AND TRIM(COALESCE(permission, '')) <> ''`, ids)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	var items []string
	if err := sqlx.SelectContext(ctx, exec, &items, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list menu permission codes: %w", err)
	}
	return items, nil
}

func (r *Repository) CreateMenu(ctx context.Context, record domain.MenuRecord, operatorID int64) error {
	return r.exec(ctx, `INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, featureCode, isFrame, isCache, visible, status, remark, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`,
		record.MenuID, strings.TrimSpace(record.Name), record.ParentID, record.SortOrder, record.Path, nullableString(record.Component), nullableString(record.Icon), record.Type, nullableString(record.Permission), nullableString(record.FeatureCode), record.IsFrame, record.IsCache, record.Visible, record.Status, nullableString(record.Remark), nullableInt64(operatorID), nullableInt64(operatorID))
}

func (r *Repository) UpdateMenu(ctx context.Context, record domain.MenuRecord, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_menu SET name = ?, parentId = ?, sortOrder = ?, path = ?, component = ?, icon = ?, type = ?, permission = ?, featureCode = ?, isFrame = ?, isCache = ?, visible = ?, status = ?, remark = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`,
		strings.TrimSpace(record.Name), record.ParentID, record.SortOrder, record.Path, nullableString(record.Component), nullableString(record.Icon), record.Type, nullableString(record.Permission), nullableString(record.FeatureCode), record.IsFrame, record.IsCache, record.Visible, record.Status, nullableString(record.Remark), nullableInt64(operatorID), record.MenuID)
}

func (r *Repository) DeleteMenu(ctx context.Context, menuID int64, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_menu SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, nullableInt64(operatorID), menuID)
}

func (r *Repository) DeleteMenuPermissionsByMenuID(ctx context.Context, menuID int64) error {
	return r.exec(ctx, `DELETE FROM sys_menu_permission WHERE menuId = ?`, menuID)
}

func (r *Repository) ListRoleMenuIDs(ctx context.Context, roleID int64) ([]int64, error) {
	query := `
SELECT srm.menuId
FROM sys_role_menu srm
JOIN sys_menu sm ON sm.id = srm.menuId
WHERE srm.roleId = ? AND sm.isDeleted = 0
ORDER BY srm.menuId ASC
LIMIT ?`
	exec := store.SQLXExecutor(ctx, r.db)
	var values []int64
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, query), roleID, authorizationSetQueryMaxIDs+1); err != nil {
		return nil, fmt.Errorf("list role menu ids: %w", err)
	}
	if len(values) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list role menu ids exceeds %d", authorizationSetQueryMaxIDs)
	}
	return values, nil
}

func (r *Repository) ListRoleMenuIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return result, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list role menu ids by role ids exceeds role limit")
	}
	query, args, err := sqlx.In(`
SELECT srm.roleId, srm.menuId
FROM sys_role_menu srm
JOIN sys_menu sm ON sm.id = srm.menuId
WHERE srm.roleId IN (?) AND sm.isDeleted = 0
ORDER BY srm.roleId ASC, srm.menuId ASC
LIMIT ?`, ids, authorizationDerivedRelationMax+1)
	if err != nil {
		return nil, err
	}
	type row struct {
		RoleID int64 `db:"roleId"`
		MenuID int64 `db:"menuId"`
	}
	exec := store.SQLXExecutor(ctx, r.db)
	rows := []row{}
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list role menu ids by role ids: %w", err)
	}
	if len(rows) > authorizationDerivedRelationMax {
		return nil, fmt.Errorf("list role menu ids by role ids exceeds relation limit")
	}
	for _, item := range rows {
		if item.RoleID > 0 && item.MenuID > 0 {
			result[item.RoleID] = append(result[item.RoleID], item.MenuID)
		}
	}
	return result, nil
}

func (r *Repository) ListDirectRolePermissionIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return result, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list direct role permission ids exceeds role limit")
	}
	query, args, err := sqlx.In(`
SELECT roleId, permissionId
FROM sys_role_permission
WHERE roleId IN (?) AND COALESCE(source, 'DIRECT') IN ('DIRECT', 'BOTH')
ORDER BY roleId ASC, permissionId ASC
LIMIT ?`, ids, authorizationDerivedRelationMax+1)
	if err != nil {
		return nil, err
	}
	type row struct {
		RoleID       int64 `db:"roleId"`
		PermissionID int64 `db:"permissionId"`
	}
	exec := store.SQLXExecutor(ctx, r.db)
	rows := []row{}
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list direct role permission ids by role ids: %w", err)
	}
	if len(rows) > authorizationDerivedRelationMax {
		return nil, fmt.Errorf("list direct role permission ids exceeds relation limit")
	}
	for _, item := range rows {
		if item.RoleID > 0 && item.PermissionID > 0 {
			result[item.RoleID] = append(result[item.RoleID], item.PermissionID)
		}
	}
	return result, nil
}

func (r *Repository) ListMenuPermissionIDs(ctx context.Context, menuIDs []int64) ([]int64, error) {
	ids := uniquePositiveIDs(menuIDs)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list menu permission ids exceeds menu limit")
	}
	query, args, err := sqlx.In(`
SELECT DISTINCT smp.permissionId
FROM sys_menu_permission smp
JOIN sys_permission sp ON sp.id = smp.permissionId
WHERE smp.menuId IN (?) AND sp.isDeleted = 0 AND sp.status = 0
ORDER BY smp.permissionId ASC
LIMIT ?`, ids, authorizationSetQueryMaxIDs+1)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	values := []int64{}
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list menu permission ids: %w", err)
	}
	if len(values) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list menu permission ids exceeds result limit")
	}
	return uniquePositiveIDs(values), nil
}

func (r *Repository) ListMenuPermissionIDsByMenuIDs(ctx context.Context, menuIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	ids := uniquePositiveIDs(menuIDs)
	if len(ids) == 0 {
		return result, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list menu permission ids by menu ids exceeds menu limit")
	}
	query, args, err := sqlx.In(`
SELECT smp.menuId, smp.permissionId
FROM sys_menu_permission smp
JOIN sys_permission sp ON sp.id = smp.permissionId
WHERE smp.menuId IN (?) AND sp.isDeleted = 0 AND sp.status = 0
ORDER BY smp.menuId ASC, smp.permissionId ASC
LIMIT ?`, ids, authorizationDerivedRelationMax+1)
	if err != nil {
		return nil, err
	}
	type row struct {
		MenuID       int64 `db:"menuId"`
		PermissionID int64 `db:"permissionId"`
	}
	exec := store.SQLXExecutor(ctx, r.db)
	rows := []row{}
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list menu permission ids by menu ids: %w", err)
	}
	if len(rows) > authorizationDerivedRelationMax {
		return nil, fmt.Errorf("list menu permission ids by menu ids exceeds relation limit")
	}
	for _, item := range rows {
		if item.MenuID > 0 && item.PermissionID > 0 {
			result[item.MenuID] = append(result[item.MenuID], item.PermissionID)
		}
	}
	return result, nil
}

func (r *Repository) ListPermissions(ctx context.Context, query authorizationfacade.PermissionQuery) ([]domain.PermissionRecord, error) {
	conditions := []string{"isDeleted = 0"}
	args := []any{}
	if strings.TrimSpace(query.Code) != "" {
		conditions = append(conditions, "code LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Code)+"%")
	}
	if strings.TrimSpace(query.Name) != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.ResourceType) != "" {
		conditions = append(conditions, "resourceType = ?")
		args = append(args, strings.TrimSpace(query.ResourceType))
	}
	if strings.TrimSpace(query.Method) != "" {
		conditions = append(conditions, "method = ?")
		args = append(args, strings.TrimSpace(query.Method))
	}
	if strings.TrimSpace(query.Path) != "" {
		conditions = append(conditions, "path LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Path)+"%")
	}
	if query.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *query.Status)
	}
	exec := store.SQLXExecutor(ctx, r.db)
	records := []domain.PermissionRecord{}
	err := sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, `
SELECT id AS permission_id, code, COALESCE(featureCode, '') AS feature_code,
       name, COALESCE(resourceType, 'API') AS resource_type,
       COALESCE(method, '') AS method, COALESCE(path, '') AS path, status,
       COALESCE(description, '') AS description, createTime AS create_time, updateTime AS update_time
FROM sys_permission
WHERE `+strings.Join(conditions, " AND ")+`
ORDER BY code ASC, id ASC`), args...)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return records, nil
}

func (r *Repository) PagePermissions(ctx context.Context, query authorizationfacade.PermissionPageQuery, filterFeatures bool, enabledFeatureCodes []string) ([]domain.PermissionRecord, int64, error) {
	conditions := []string{"isDeleted = 0"}
	args := []any{}
	if strings.TrimSpace(query.Code) != "" {
		conditions = append(conditions, "code LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Code)+"%")
	}
	if strings.TrimSpace(query.Name) != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.ResourceType) != "" {
		conditions = append(conditions, "resourceType = ?")
		args = append(args, strings.TrimSpace(query.ResourceType))
	}
	if strings.TrimSpace(query.Method) != "" {
		conditions = append(conditions, "method = ?")
		args = append(args, strings.TrimSpace(query.Method))
	}
	if strings.TrimSpace(query.Path) != "" {
		conditions = append(conditions, "path LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Path)+"%")
	}
	if query.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *query.Status)
	}
	if filterFeatures {
		featureCodes, err := normalizedFeatureCodes(enabledFeatureCodes)
		if err != nil {
			return nil, 0, err
		}
		if len(featureCodes) == 0 {
			conditions = append(conditions, "(featureCode IS NULL OR featureCode = '')")
		} else {
			conditions = append(conditions, "(featureCode IS NULL OR featureCode = '' OR featureCode IN ("+strings.TrimSuffix(strings.Repeat("?,", len(featureCodes)), ",")+"))")
			for _, featureCode := range featureCodes {
				args = append(args, featureCode)
			}
		}
	}
	where := " WHERE " + strings.Join(conditions, " AND ")
	total, err := r.count(ctx, `SELECT COUNT(1) FROM sys_permission`+where, args...)
	if err != nil {
		return nil, 0, err
	}
	current, size := pageArgs(query.Current, query.Size)
	selectArgs := append(append([]any{}, args...), size, (current-1)*size)
	exec := store.SQLXExecutor(ctx, r.db)
	records := []domain.PermissionRecord{}
	err = sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, `
SELECT id AS permission_id, code, COALESCE(featureCode, '') AS feature_code,
       name, COALESCE(resourceType, 'API') AS resource_type,
       COALESCE(method, '') AS method, COALESCE(path, '') AS path, status,
       COALESCE(description, '') AS description, createTime AS create_time, updateTime AS update_time
FROM sys_permission`+where+`
ORDER BY code ASC, id ASC
LIMIT ? OFFSET ?`), selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page permissions: %w", err)
	}
	return records, int64(total), nil
}

func (r *Repository) ListPermissionCodesByIDs(ctx context.Context, permissionIDs []int64) (map[int64]string, error) {
	ids := uniquePositiveIDs(permissionIDs)
	result := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
SELECT id, code
FROM sys_permission
WHERE id IN (?) AND isDeleted = 0 AND status = 0`, ids)
	if err != nil {
		return nil, err
	}
	type row struct {
		ID   int64  `db:"id"`
		Code string `db:"code"`
	}
	exec := store.SQLXExecutor(ctx, r.db)
	rows := []row{}
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list permission codes by ids: %w", err)
	}
	for _, item := range rows {
		code := strings.TrimSpace(item.Code)
		if item.ID > 0 && code != "" {
			result[item.ID] = code
		}
	}
	return result, nil
}

func (r *Repository) ListPermissionsByIDs(ctx context.Context, permissionIDs []int64) ([]domain.PermissionRecord, error) {
	ids := uniquePositiveIDs(permissionIDs)
	if len(ids) == 0 {
		return []domain.PermissionRecord{}, nil
	}
	query, args, err := sqlx.In(`
SELECT id AS permission_id, code, COALESCE(featureCode, '') AS feature_code,
       name, COALESCE(resourceType, 'API') AS resource_type,
       COALESCE(method, '') AS method, COALESCE(path, '') AS path, status,
       COALESCE(description, '') AS description, createTime AS create_time, updateTime AS update_time
FROM sys_permission
WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	records := []domain.PermissionRecord{}
	if err := sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list permissions by ids: %w", err)
	}
	return records, nil
}

func (r *Repository) FindPermissionByID(ctx context.Context, permissionID int64) (*domain.PermissionRecord, error) {
	return findOne[domain.PermissionRecord](ctx, r.db, r.rebind, `
SELECT id AS permission_id, code, COALESCE(featureCode, '') AS feature_code,
       name, COALESCE(resourceType, 'API') AS resource_type,
       COALESCE(method, '') AS method, COALESCE(path, '') AS path, status,
       COALESCE(description, '') AS description, createTime AS create_time, updateTime AS update_time
FROM sys_permission WHERE id = ? AND isDeleted = 0 LIMIT 1`, permissionID)
}

func (r *Repository) LockPermissionGrants(ctx context.Context, permissionIDs []int64) ([]domain.PermissionRecord, error) {
	ids := uniquePositiveIDs(permissionIDs)
	if len(ids) == 0 {
		return []domain.PermissionRecord{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("lock permission grants exceeds limit")
	}
	query, args, err := sqlx.In(`
SELECT id AS permission_id, code, COALESCE(featureCode, '') AS feature_code,
       name, COALESCE(resourceType, 'API') AS resource_type,
       COALESCE(method, '') AS method, COALESCE(path, '') AS path, status,
       COALESCE(description, '') AS description, createTime AS create_time, updateTime AS update_time
FROM sys_permission
WHERE id IN (?) AND isDeleted = 0
ORDER BY id ASC
FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	records := make([]domain.PermissionRecord, 0, len(ids))
	if err := sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("lock permission grants: %w", err)
	}
	return records, nil
}

func (r *Repository) TouchPermissionGrantGuards(ctx context.Context, permissionIDs []int64) error {
	ids := uniquePositiveIDs(permissionIDs)
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return fmt.Errorf("touch permission grant guards exceeds limit")
	}
	query, args, err := sqlx.In(`UPDATE sys_permission SET updateTime = NOW() WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return err
	}
	return r.exec(ctx, query, args...)
}

func (r *Repository) CountPermissionCodeExcludingID(ctx context.Context, permissionID int64, code string) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_permission WHERE code = ? AND id <> ? AND isDeleted = 0`, strings.TrimSpace(code), permissionID)
}

func (r *Repository) CountPermissionsByIDs(ctx context.Context, permissionIDs []int64) (int, error) {
	ids := uniquePositiveIDs(permissionIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	query, args, err := sqlx.In(`SELECT COUNT(DISTINCT id) FROM sys_permission WHERE id IN (?) AND isDeleted = 0 AND status = 0`, ids)
	if err != nil {
		return 0, err
	}
	return r.count(ctx, query, args...)
}

func (r *Repository) CreatePermission(ctx context.Context, record domain.PermissionRecord, operatorID int64) error {
	return r.exec(ctx, `INSERT INTO sys_permission (id, code, featureCode, name, resourceType, method, path, status, description, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`,
		record.PermissionID, strings.TrimSpace(record.Code), nullableString(record.FeatureCode), strings.TrimSpace(record.Name), defaultString(record.ResourceType, "API"), nullableString(record.Method), nullableString(record.Path), record.Status, nullableString(record.Description), nullableInt64(operatorID), nullableInt64(operatorID))
}

func (r *Repository) UpdatePermission(ctx context.Context, record domain.PermissionRecord, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_permission SET code = ?, featureCode = ?, name = ?, resourceType = ?, method = ?, path = ?, status = ?, description = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`,
		strings.TrimSpace(record.Code), nullableString(record.FeatureCode), strings.TrimSpace(record.Name), defaultString(record.ResourceType, "API"), nullableString(record.Method), nullableString(record.Path), record.Status, nullableString(record.Description), nullableInt64(operatorID), record.PermissionID)
}

func (r *Repository) DeletePermission(ctx context.Context, permissionID int64, operatorID int64) error {
	if err := r.exec(ctx, `UPDATE sys_permission SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, nullableInt64(operatorID), permissionID); err != nil {
		return err
	}
	if err := r.exec(ctx, `DELETE FROM sys_role_permission WHERE permissionId = ?`, permissionID); err != nil {
		return err
	}
	return r.exec(ctx, `DELETE FROM sys_menu_permission WHERE permissionId = ?`, permissionID)
}

func (r *Repository) SoftDeleteUserPermissionsByPermissionID(ctx context.Context, permissionID int64, operatorID int64) error {
	return r.exec(ctx, `
UPDATE sys_user_permission
SET isDeleted = 1, updaterId = ?, updateTime = NOW()
WHERE permissionId = ? AND isDeleted = 0`, nullableInt64(operatorID), permissionID)
}

func (r *Repository) ListUserIDsByPermissionIDPage(ctx context.Context, permissionID, afterUserID int64, limit int) ([]int64, error) {
	if permissionID <= 0 || afterUserID < 0 {
		return nil, fmt.Errorf("list user ids by permission id page arguments are invalid")
	}
	if limit <= 0 || limit > authorizationSetQueryChunkSize {
		return nil, fmt.Errorf("list user ids by permission id page limit is invalid")
	}
	query := `
SELECT DISTINCT userId
FROM sys_user_permission
WHERE permissionId = ? AND userId > ?
ORDER BY userId ASC
LIMIT ?`
	exec := store.SQLXExecutor(ctx, r.db)
	users := make([]int64, 0, limit)
	if err := sqlx.SelectContext(ctx, exec, &users, r.rebind(exec, query), permissionID, afterUserID, limit); err != nil {
		return nil, fmt.Errorf("list user ids by permission id page: %w", err)
	}
	return uniquePositiveIDs(users), nil
}

func (r *Repository) ListRoleIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error) {
	ids := uniquePositiveIDs(permissionIDs)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list role ids by permission ids exceeds permission limit")
	}
	query, args, err := sqlx.In(`
SELECT DISTINCT roleId
FROM sys_role_permission
WHERE permissionId IN (?)
ORDER BY roleId ASC
LIMIT ?`, ids, authorizationSetQueryMaxIDs+1)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	values := []int64{}
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list role ids by permission ids: %w", err)
	}
	if len(values) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list role ids by permission ids exceeds role limit")
	}
	return uniquePositiveIDs(values), nil
}

func (r *Repository) ListMenuIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error) {
	ids := uniquePositiveIDs(permissionIDs)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	if len(ids) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list menu ids by permission ids exceeds permission limit")
	}
	query, args, err := sqlx.In(`
SELECT DISTINCT menuId
FROM sys_menu_permission
WHERE permissionId IN (?)
ORDER BY menuId ASC
LIMIT ?`, ids, authorizationSetQueryMaxIDs+1)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	values := []int64{}
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list menu ids by permission ids: %w", err)
	}
	if len(values) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list menu ids by permission ids exceeds menu limit")
	}
	return uniquePositiveIDs(values), nil
}

func (r *Repository) ListRoleIDsByMenuID(ctx context.Context, menuID int64) ([]int64, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	values := []int64{}
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, `
SELECT DISTINCT roleId
FROM sys_role_menu
WHERE menuId = ?
ORDER BY roleId ASC
LIMIT ?`), menuID, authorizationSetQueryMaxIDs+1); err != nil {
		return nil, fmt.Errorf("list role ids by menu id: %w", err)
	}
	return uniquePositiveIDs(values), nil
}

func (r *Repository) ReplaceMenuPermissions(ctx context.Context, menuID int64, permissionIDs []int64, operatorID int64, nextID func() int64) error {
	if err := r.exec(ctx, `DELETE FROM sys_menu_permission WHERE menuId = ?`, menuID); err != nil {
		return err
	}
	rows := make([][]any, 0, len(permissionIDs))
	for _, permissionID := range uniquePositiveIDs(permissionIDs) {
		rows = append(rows, []any{nextID(), menuID, permissionID, nullableInt64(operatorID)})
	}
	return r.bulkInsert(ctx, `INSERT INTO sys_menu_permission (id, menuId, permissionId, creatorId, createTime) VALUES `, `(?, ?, ?, ?, NOW())`, rows)
}

func (r *Repository) ReplaceUserRoles(ctx context.Context, userID int64, roleIDs []int64, operatorID int64, nextID func() int64) error {
	if err := r.exec(ctx, `DELETE FROM sys_user_role WHERE userId = ?`, userID); err != nil {
		return err
	}
	rows := make([][]any, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if roleID <= 0 {
			continue
		}
		rows = append(rows, []any{userID, roleID, operatorID, operatorID, r.boolValue(false)})
	}
	return r.bulkInsert(ctx, `INSERT INTO sys_user_role (userId, roleId, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES `, `(?, ?, ?, ?, NOW(), NOW(), ?)`, rows)
}

func (r *Repository) ReplaceRoleDepts(ctx context.Context, roleID int64, deptIDs []int64, operatorID int64, nextID func() int64) error {
	if err := r.exec(ctx, `DELETE FROM sys_role_dept WHERE roleId = ?`, roleID); err != nil {
		return err
	}
	rows := make([][]any, 0, len(deptIDs))
	for _, deptID := range uniquePositiveIDs(deptIDs) {
		rows = append(rows, []any{nextID(), roleID, deptID})
	}
	return r.bulkInsert(ctx, `INSERT INTO sys_role_dept (id, roleId, deptId) VALUES `, `(?, ?, ?)`, rows)
}

func (r *Repository) ReplaceRolePermissions(ctx context.Context, roleID int64, directPermissionIDs, menuPermissionIDs, menuIDs []int64, operatorID int64, nextID func() int64) error {
	if err := r.exec(ctx, `DELETE FROM sys_role_permission WHERE roleId = ?`, roleID); err != nil {
		return err
	}
	permissionRows := rolePermissionRows(roleID, directPermissionIDs, menuPermissionIDs, operatorID, nextID)
	if err := r.bulkInsert(ctx, `INSERT INTO sys_role_permission (id, roleId, permissionId, source, creatorId, createTime, updateTime) VALUES `, `(?, ?, ?, ?, ?, NOW(), NOW())`, permissionRows); err != nil {
		return err
	}
	if err := r.exec(ctx, `DELETE FROM sys_role_menu WHERE roleId = ?`, roleID); err != nil {
		return err
	}
	menuRows := make([][]any, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		if menuID <= 0 {
			continue
		}
		menuRows = append(menuRows, []any{nextID(), roleID, menuID, operatorID})
	}
	return r.bulkInsert(ctx, `INSERT INTO sys_role_menu (id, roleId, menuId, updaterId, createTime, updateTime) VALUES `, `(?, ?, ?, ?, NOW(), NOW())`, menuRows)
}

func (r *Repository) ReplaceDerivedRolePermissionsBatch(ctx context.Context, assignments []domain.RolePermissionAssignment, operatorID int64, nextID func() int64) error {
	roleIDs := make([]int64, 0, len(assignments))
	menuOnlyRows := make([][]any, 0)
	bothPairs := make([][2]int64, 0)
	totalRelations := 0
	for _, assignment := range assignments {
		if assignment.RoleID <= 0 {
			continue
		}
		roleIDs = append(roleIDs, assignment.RoleID)
		direct := make(map[int64]struct{}, len(assignment.DirectPermissionIDs))
		for _, permissionID := range uniquePositiveIDs(assignment.DirectPermissionIDs) {
			direct[permissionID] = struct{}{}
		}
		for _, permissionID := range uniquePositiveIDs(assignment.MenuPermissionIDs) {
			totalRelations++
			if totalRelations > authorizationDerivedRelationMax {
				return fmt.Errorf("replace derived role permissions exceeds relation limit")
			}
			if _, ok := direct[permissionID]; ok {
				bothPairs = append(bothPairs, [2]int64{assignment.RoleID, permissionID})
				continue
			}
			menuOnlyRows = append(menuOnlyRows, []any{nextID(), assignment.RoleID, permissionID, rolePermissionSourceMenu, operatorID})
		}
	}
	roleIDs = uniquePositiveIDs(roleIDs)
	if len(roleIDs) == 0 {
		return nil
	}
	if len(roleIDs) > authorizationSetQueryMaxIDs {
		return fmt.Errorf("replace derived role permissions exceeds role limit")
	}
	query, args, err := sqlx.In(`UPDATE sys_role_permission SET source = 'DIRECT', updateTime = NOW() WHERE roleId IN (?) AND source = 'BOTH'`, roleIDs)
	if err != nil {
		return err
	}
	if err := r.exec(ctx, query, args...); err != nil {
		return err
	}
	query, args, err = sqlx.In(`DELETE FROM sys_role_permission WHERE roleId IN (?) AND source = 'MENU'`, roleIDs)
	if err != nil {
		return err
	}
	if err := r.exec(ctx, query, args...); err != nil {
		return err
	}
	for _, chunk := range chunkRolePermissionPairs(bothPairs, authorizationBulkInsertChunkSize) {
		clauses := make([]string, 0, len(chunk))
		args = make([]any, 0, len(chunk)*2)
		for _, pair := range chunk {
			clauses = append(clauses, `(roleId = ? AND permissionId = ?)`)
			args = append(args, pair[0], pair[1])
		}
		if err := r.exec(ctx, `UPDATE sys_role_permission SET source = 'BOTH', updateTime = NOW() WHERE source = 'DIRECT' AND (`+strings.Join(clauses, ` OR `)+`)`, args...); err != nil {
			return err
		}
	}
	return r.bulkInsert(ctx, `INSERT INTO sys_role_permission (id, roleId, permissionId, source, creatorId, createTime, updateTime) VALUES `, `(?, ?, ?, ?, ?, NOW(), NOW())`, menuOnlyRows)
}

func (r *Repository) GrantTemporaryPermission(ctx context.Context, userID int64, permissionCode string, expireAt *time.Time, source, reason string, grantedBy int64, nextID func() int64) error {
	permissionID, err := r.findPermissionIDByCode(ctx, permissionCode)
	if err != nil {
		return err
	}
	if permissionID <= 0 {
		return fmt.Errorf("temporary permission code not found: %s", permissionCode)
	}
	return r.exec(ctx, `
INSERT INTO sys_user_permission (id, userId, permissionId, type, expireTime, source, reason, grantedBy, creatorId, updaterId, createTime, updateTime, isDeleted)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)
ON DUPLICATE KEY UPDATE
	type = VALUES(type),
	expireTime = VALUES(expireTime),
	source = VALUES(source),
	reason = VALUES(reason),
	grantedBy = VALUES(grantedBy),
	updaterId = VALUES(updaterId),
	updateTime = NOW(),
	isDeleted = 0`,
		nextID(), userID, permissionID, temporaryPermissionType(expireAt), expireAt, strings.TrimSpace(source), strings.TrimSpace(reason), grantedBy, grantedBy, grantedBy)
}

func (r *Repository) RevokeTemporaryPermission(ctx context.Context, userID int64, permissionCode string) error {
	permissionID, err := r.findPermissionIDByCode(ctx, permissionCode)
	if err != nil || permissionID <= 0 {
		return err
	}
	return r.exec(ctx, `UPDATE sys_user_permission SET isDeleted = 1, updateTime = NOW() WHERE userId = ? AND permissionId = ? AND isDeleted = 0`, userID, permissionID)
}

func (r *Repository) ExtendTemporaryPermission(ctx context.Context, userID int64, permissionCode string, expireAt *time.Time, reason string) error {
	permissionID, err := r.findPermissionIDByCode(ctx, permissionCode)
	if err != nil || permissionID <= 0 {
		return err
	}
	return r.exec(ctx, `UPDATE sys_user_permission SET expireTime = ?, type = ?, reason = ?, updateTime = NOW() WHERE userId = ? AND permissionId = ? AND isDeleted = 0`,
		expireAt, temporaryPermissionType(expireAt), strings.TrimSpace(reason), userID, permissionID)
}

func (r *Repository) ListUserTemporaryPermissions(ctx context.Context, userID int64) ([]domain.TemporaryPermissionRecord, error) {
	query := `
SELECT
	sup.userId AS user_id,
	sp.code AS permission_code,
	sp.name AS permission_name,
	CASE WHEN sup.type THEN 1 ELSE 0 END AS type,
	sup.expireTime AS expire_at,
	COALESCE(sup.source, '') AS source,
	COALESCE(sup.reason, '') AS reason,
	COALESCE(sup.grantedBy, 0) AS granted_by,
	sup.createTime AS granted_at,
	sup.updateTime AS updated_at
FROM sys_user_permission sup
JOIN sys_permission sp ON sp.id = sup.permissionId
WHERE sup.userId = ? AND sup.isDeleted = 0 AND sp.isDeleted = 0
ORDER BY sup.id DESC`
	exec := store.SQLXExecutor(ctx, r.db)
	var items []domain.TemporaryPermissionRecord
	if err := sqlx.SelectContext(ctx, exec, &items, r.rebind(exec, query), userID); err != nil {
		return nil, fmt.Errorf("list user temporary permissions: %w", err)
	}
	return items, nil
}

func (r *Repository) ListExpiredTemporaryPermissionUserIDsPage(ctx context.Context, afterUserID int64, limit int) ([]int64, error) {
	if afterUserID < 0 {
		return nil, fmt.Errorf("list expired temporary permission users page cursor is invalid")
	}
	if limit <= 0 || limit > authorizationSetQueryChunkSize {
		return nil, fmt.Errorf("list expired temporary permission users page limit is invalid")
	}
	query := `
SELECT DISTINCT userId
FROM sys_user_permission
WHERE userId > ? AND isDeleted = 0 AND type AND expireTime IS NOT NULL AND expireTime <= NOW()
ORDER BY userId ASC
LIMIT ?`
	exec := store.SQLXExecutor(ctx, r.db)
	ids := make([]int64, 0, limit)
	if err := sqlx.SelectContext(ctx, exec, &ids, r.rebind(exec, query), afterUserID, limit); err != nil {
		return nil, fmt.Errorf("list expired temporary permission users page: %w", err)
	}
	return uniquePositiveIDs(ids), nil
}

func (r *Repository) CleanupExpiredTemporaryPermissionsByUserIDs(ctx context.Context, userIDs []int64) error {
	ids := uniquePositiveIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > authorizationSetQueryChunkSize {
		return fmt.Errorf("cleanup expired temporary permissions exceeds user limit")
	}
	query, args, err := sqlx.In(`
UPDATE sys_user_permission
SET isDeleted = 1, updateTime = NOW()
WHERE userId IN (?) AND isDeleted = 0 AND type AND expireTime IS NOT NULL AND expireTime <= NOW()`, ids)
	if err != nil {
		return err
	}
	return r.exec(ctx, query, args...)
}
func (r *Repository) TemporaryPermissionStats(ctx context.Context) (*domain.TemporaryPermissionStats, error) {
	query := `
SELECT
	COUNT(1) AS total_active,
	COALESCE(SUM(CASE WHEN type THEN 1 ELSE 0 END), 0) AS temporary,
	COALESCE(SUM(CASE WHEN NOT type THEN 1 ELSE 0 END), 0) AS permanent,
	COALESCE(SUM(CASE WHEN type AND expireTime IS NOT NULL AND expireTime <= ` + r.temporaryPermissionDeadlineSQL() + ` THEN 1 ELSE 0 END), 0) AS expiring_soon
FROM sys_user_permission
WHERE isDeleted = 0 AND (NOT type OR expireTime IS NULL OR expireTime > NOW())`
	exec := store.SQLXExecutor(ctx, r.db)
	var item domain.TemporaryPermissionStats
	if err := sqlx.GetContext(ctx, exec, &item, r.rebind(exec, query)); err != nil {
		return nil, fmt.Errorf("temporary permission stats: %w", err)
	}
	return &item, nil
}

func (r *Repository) ListPermissionCodes(ctx context.Context) ([]string, error) {
	query := `SELECT code FROM sys_permission WHERE isDeleted = 0 AND status = 0 ORDER BY code ASC`
	exec := store.SQLXExecutor(ctx, r.db)
	var values []string
	if err := sqlx.SelectContext(ctx, exec, &values, r.rebind(exec, query)); err != nil {
		return nil, fmt.Errorf("list permission codes: %w", err)
	}
	return values, nil
}

func (r *Repository) FindPermissionCodeByID(ctx context.Context, permissionID int64) (string, error) {
	query := `SELECT code FROM sys_permission WHERE id = ? AND isDeleted = 0 AND status = 0 LIMIT 1`
	exec := store.SQLXExecutor(ctx, r.db)
	var code string
	if err := sqlx.GetContext(ctx, exec, &code, r.rebind(exec, query), permissionID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("find permission code by id: %w", err)
	}
	return strings.TrimSpace(code), nil
}

func (r *Repository) FindPermissionIDByCode(ctx context.Context, permissionCode string) (int64, error) {
	return r.findPermissionIDByCode(ctx, permissionCode)
}

func (r *Repository) exec(ctx context.Context, query string, args ...any) error {
	exec := store.SQLXExecutor(ctx, r.db)
	if _, err := exec.ExecContext(ctx, r.rebind(exec, query), args...); err != nil {
		return fmt.Errorf("authorization repository exec: %w", err)
	}
	return nil
}

func (r *Repository) bulkInsert(ctx context.Context, prefix string, valueClause string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	exec := store.SQLXExecutor(ctx, r.db)
	for start := 0; start < len(rows); start += authorizationBulkInsertChunkSize {
		end := start + authorizationBulkInsertChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		clauses := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*len(rows[start]))
		for _, row := range rows[start:end] {
			clauses = append(clauses, valueClause)
			args = append(args, row...)
		}
		query := prefix + strings.Join(clauses, ", ")
		if _, err := exec.ExecContext(ctx, r.rebind(exec, query), args...); err != nil {
			return fmt.Errorf("authorization repository bulk insert: %w", err)
		}
	}
	return nil
}

func (r *Repository) count(ctx context.Context, query string, args ...any) (int, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	var total int
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, query), args...); err != nil {
		return 0, fmt.Errorf("authorization repository count: %w", err)
	}
	return total, nil
}

func findOne[T any](
	ctx context.Context,
	db store.SQLX,
	rebind func(store.SQLX, string) string,
	query string,
	args ...any,
) (*T, error) {
	exec := store.SQLXExecutor(ctx, db)
	var result T
	if err := sqlx.GetContext(ctx, exec, &result, rebind(exec, query), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("authorization repository find one: %w", err)
	}
	return &result, nil
}

func (r *Repository) findPermissionIDByCode(ctx context.Context, code string) (int64, error) {
	query := `SELECT id FROM sys_permission WHERE code = ? AND isDeleted = 0 AND status = 0 LIMIT 1`
	exec := store.SQLXExecutor(ctx, r.db)
	var permissionID int64
	if err := sqlx.GetContext(ctx, exec, &permissionID, r.rebind(exec, query), strings.TrimSpace(code)); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("find permission id by code: %w", err)
	}
	return permissionID, nil
}

func temporaryPermissionType(expireAt *time.Time) int {
	if expireAt != nil {
		return 1
	}
	return 0
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
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

func normalizedFeatureCodes(values []string) ([]string, error) {
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
	if len(result) > authorizationFeatureCodeMax {
		return nil, fmt.Errorf("permission feature filter exceeds %d", authorizationFeatureCodeMax)
	}
	sort.Strings(result)
	return result, nil
}

func chunkInt64s(values []int64, size int) [][]int64 {
	if size <= 0 || len(values) == 0 {
		return nil
	}
	result := make([][]int64, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		result = append(result, values[start:end])
	}
	return result
}

func chunkRolePermissionPairs(values [][2]int64, size int) [][][2]int64 {
	if size <= 0 || len(values) == 0 {
		return nil
	}
	result := make([][][2]int64, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		result = append(result, values[start:end])
	}
	return result
}

func rolePermissionRows(roleID int64, directPermissionIDs, menuPermissionIDs []int64, operatorID int64, nextID func() int64) [][]any {
	if roleID <= 0 {
		return nil
	}
	sources := make(map[int64]string)
	for _, permissionID := range uniquePositiveIDs(directPermissionIDs) {
		sources[permissionID] = rolePermissionSourceDirect
	}
	for _, permissionID := range uniquePositiveIDs(menuPermissionIDs) {
		switch sources[permissionID] {
		case rolePermissionSourceDirect, rolePermissionSourceBoth:
			sources[permissionID] = rolePermissionSourceBoth
		default:
			sources[permissionID] = rolePermissionSourceMenu
		}
	}
	rows := make([][]any, 0, len(sources))
	for permissionID, source := range sources {
		rows = append(rows, []any{nextID(), roleID, permissionID, source, operatorID})
	}
	return rows
}

func pageArgs(current, size int64) (int64, int64) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}
	if size > 200 {
		size = 200
	}
	return current, size
}

func nullableInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
