package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

func (r *Repository) FindAccessUser(ctx context.Context, userID int64) (*domain.AccessUserRecord, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	var record domain.AccessUserRecord
	err := sqlx.GetContext(ctx, exec, &record, r.rebind(exec, `
SELECT id AS user_id, userAccount AS username, status
FROM sys_user
WHERE id = ? AND isDeleted = 0
LIMIT 1`), userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find access explain user: %w", err)
	}
	return &record, nil
}

func (r *Repository) ListAccessRoleSources(ctx context.Context, userID int64) ([]domain.AccessRoleSourceRecord, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	records := []domain.AccessRoleSourceRecord{}
	if err := sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, `
SELECT
  sr.id AS role_id, sr.code AS role_code, sr.name AS role_name,
  sr.status AS role_status, COALESCE(sr.dataScope, 5) AS role_data_scope,
  COALESCE(sr.systemKey, '') AS role_system_key,
  'DIRECT_USER' AS assignment_source,
  0 AS post_id, '' AS post_code, '' AS post_name, 0 AS post_dept_id, 0 AS post_org_id
FROM sys_user_role sur
JOIN sys_role sr ON sr.id = sur.roleId
WHERE sur.userId = ? AND sur.isDeleted = 0 AND sr.isDeleted = 0
ORDER BY sr.id, sur.id
LIMIT ?`), userID, authorizationSetQueryMaxIDs+1); err != nil {
		return nil, fmt.Errorf("list direct access role sources: %w", err)
	}
	if len(records) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list access role sources: direct role set exceeds %d", authorizationSetQueryMaxIDs)
	}

	type postSourceRow struct {
		PostID     int64  `db:"post_id"`
		PostCode   string `db:"post_code"`
		PostName   string `db:"post_name"`
		PostDeptID int64  `db:"post_dept_id"`
		PostOrgID  int64  `db:"post_org_id"`
	}
	var posts []postSourceRow
	if err := sqlx.SelectContext(ctx, exec, &posts, r.rebind(exec, `
SELECT sp.id AS post_id, sp.code AS post_code, sp.name AS post_name,
       COALESCE(sp.deptId, 0) AS post_dept_id, COALESCE(sp.orgId, 0) AS post_org_id
FROM sys_user_position sup
JOIN sys_post sp ON sp.id = sup.postId AND sp.isDeleted = 0
WHERE sup.userId = ? AND sup.isDeleted = 0
ORDER BY sp.id, sup.id
LIMIT ?`), userID, authorizationSetQueryMaxIDs+1); err != nil {
		return nil, fmt.Errorf("list access posts: %w", err)
	}
	if len(posts) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list access role sources: post set exceeds %d", authorizationSetQueryMaxIDs)
	}
	postIDs := make([]int64, 0, len(posts))
	postByID := make(map[int64]postSourceRow, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.PostID)
		postByID[post.PostID] = post
	}
	if len(postIDs) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list access role sources: post set exceeds %d", authorizationSetQueryMaxIDs)
	}
	type postRoleRow struct {
		PostID        int64  `db:"post_id"`
		RoleID        int64  `db:"role_id"`
		RoleCode      string `db:"role_code"`
		RoleName      string `db:"role_name"`
		RoleStatus    int    `db:"role_status"`
		RoleDataScope int    `db:"role_data_scope"`
		RoleSystemKey string `db:"role_system_key"`
	}
	for _, ids := range chunkInt64s(postIDs, authorizationSetQueryChunkSize) {
		remaining := authorizationSetQueryMaxIDs - len(records)
		query, args, err := sqlx.In(`
SELECT spr.postId AS post_id,
       sr.id AS role_id, sr.code AS role_code, sr.name AS role_name,
       sr.status AS role_status, COALESCE(sr.dataScope, 5) AS role_data_scope,
       COALESCE(sr.systemKey, '') AS role_system_key
FROM sys_post_role spr
JOIN sys_role sr ON sr.id = spr.roleId
WHERE spr.postId IN (?) AND sr.isDeleted = 0
  AND COALESCE(sr.systemKey, '') <> ?
ORDER BY spr.postId, sr.id
LIMIT ?`, ids, domain.AuthorizationRootSystemKey, remaining+1)
		if err != nil {
			return nil, err
		}
		var roleRows []postRoleRow
		if err := sqlx.SelectContext(ctx, exec, &roleRows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list access post roles: %w", err)
		}
		if len(roleRows) > remaining {
			return nil, fmt.Errorf("list access role sources: accumulated role source set exceeds %d", authorizationSetQueryMaxIDs)
		}
		for _, row := range roleRows {
			post, ok := postByID[row.PostID]
			if !ok {
				continue
			}
			records = append(records, domain.AccessRoleSourceRecord{
				RoleID: row.RoleID, RoleCode: row.RoleCode, RoleName: row.RoleName,
				RoleStatus: row.RoleStatus, RoleDataScope: row.RoleDataScope, RoleSystemKey: row.RoleSystemKey,
				AssignmentSource: "POST", PostID: post.PostID, PostCode: post.PostCode, PostName: post.PostName,
				PostDeptID: post.PostDeptID, PostOrgID: post.PostOrgID,
			})
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].RoleID != records[j].RoleID {
			return records[i].RoleID < records[j].RoleID
		}
		if records[i].AssignmentSource != records[j].AssignmentSource {
			return records[i].AssignmentSource < records[j].AssignmentSource
		}
		return records[i].PostID < records[j].PostID
	})
	return records, nil
}

func (r *Repository) ListAccessGrantRecords(ctx context.Context, userID int64) ([]domain.AccessGrantRecord, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	roleSources, err := r.ListAccessRoleSources(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(roleSources) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list access grant records: role source set exceeds %d", authorizationSetQueryMaxIDs)
	}
	sourcesByRole := make(map[int64][]domain.AccessRoleSourceRecord)
	roleIDs := make([]int64, 0, len(roleSources))
	for _, source := range roleSources {
		if _, exists := sourcesByRole[source.RoleID]; !exists {
			roleIDs = append(roleIDs, source.RoleID)
		}
		sourcesByRole[source.RoleID] = append(sourcesByRole[source.RoleID], source)
	}
	roleIDs = uniquePositiveIDs(roleIDs)
	records := make([]domain.AccessGrantRecord, 0)

	type rolePermissionRow struct {
		RoleID           int64  `db:"role_id"`
		PermissionID     int64  `db:"permission_id"`
		PermissionCode   string `db:"permission_code"`
		PermissionName   string `db:"permission_name"`
		PermissionStatus int    `db:"permission_status"`
		FeatureCode      string `db:"feature_code"`
	}
	type roleMenuRow struct {
		RoleID         int64  `db:"role_id"`
		MenuID         int64  `db:"menu_id"`
		MenuName       string `db:"menu_name"`
		MenuStatus     int    `db:"menu_status"`
		MenuPermission string `db:"menu_permission"`
		MenuFeature    string `db:"menu_feature"`
	}
	roleMenus := make([]roleMenuRow, 0)
	for _, ids := range chunkInt64s(roleIDs, authorizationSetQueryChunkSize) {
		remaining := authorizationSetQueryMaxIDs - len(records)
		query, args, err := sqlx.In(`
SELECT rp.roleId AS role_id,
       p.id AS permission_id, p.code AS permission_code, p.name AS permission_name,
       p.status AS permission_status, COALESCE(p.featureCode, '') AS feature_code
FROM sys_role_permission rp
JOIN sys_permission p ON p.id = rp.permissionId AND p.isDeleted = 0
WHERE rp.roleId IN (?) AND COALESCE(rp.source, 'DIRECT') IN ('DIRECT', 'BOTH')
ORDER BY rp.roleId, p.id
LIMIT ?`, ids, remaining+1)
		if err != nil {
			return nil, err
		}
		var rows []rolePermissionRow
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list access direct role grants: %w", err)
		}
		for _, row := range rows {
			for _, source := range sourcesByRole[row.RoleID] {
				if len(records) >= authorizationSetQueryMaxIDs {
					return nil, fmt.Errorf("list access grant records: accumulated grant set exceeds %d", authorizationSetQueryMaxIDs)
				}
				records = append(records, grantRecordFromRoleSource(source, domain.AccessGrantRecord{
					PermissionID: row.PermissionID, PermissionCode: row.PermissionCode,
					PermissionName: row.PermissionName, PermissionStatus: row.PermissionStatus,
					FeatureCode: row.FeatureCode, GrantSource: "ROLE_DIRECT",
				}))
			}
		}
		menuRemaining := authorizationSetQueryMaxIDs - len(roleMenus)
		query, args, err = sqlx.In(`
SELECT rm.roleId AS role_id,
       m.id AS menu_id, m.name AS menu_name, m.status AS menu_status,
       COALESCE(m.permission, '') AS menu_permission,
       COALESCE(m.featureCode, '') AS menu_feature
FROM sys_role_menu rm
JOIN sys_menu m ON m.id = rm.menuId AND m.isDeleted = 0
WHERE rm.roleId IN (?)
ORDER BY rm.roleId, m.id
LIMIT ?`, ids, menuRemaining+1)
		if err != nil {
			return nil, err
		}
		var menus []roleMenuRow
		if err := sqlx.SelectContext(ctx, exec, &menus, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list access role menus: %w", err)
		}
		if len(menus) > menuRemaining {
			return nil, fmt.Errorf("list access grant records: role menu set exceeds %d", authorizationSetQueryMaxIDs)
		}
		roleMenus = append(roleMenus, menus...)
	}

	menuIDs := make([]int64, 0, len(roleMenus))
	fallbackCodes := make([]string, 0, len(roleMenus))
	for _, menu := range roleMenus {
		menuIDs = append(menuIDs, menu.MenuID)
		if code := strings.TrimSpace(menu.MenuPermission); code != "" {
			fallbackCodes = append(fallbackCodes, code)
		}
	}
	menuIDs = uniquePositiveIDs(menuIDs)
	type menuPermissionRow struct {
		MenuID           int64  `db:"menu_id"`
		PermissionID     int64  `db:"permission_id"`
		PermissionCode   string `db:"permission_code"`
		PermissionName   string `db:"permission_name"`
		PermissionStatus int    `db:"permission_status"`
		FeatureCode      string `db:"feature_code"`
	}
	permissionsByMenu := make(map[int64][]menuPermissionRow)
	menuPermissionCount := 0
	for _, ids := range chunkInt64s(menuIDs, authorizationSetQueryChunkSize) {
		remaining := authorizationSetQueryMaxIDs - menuPermissionCount
		query, args, err := sqlx.In(`
SELECT mp.menuId AS menu_id,
       p.id AS permission_id, p.code AS permission_code, p.name AS permission_name,
       p.status AS permission_status, COALESCE(p.featureCode, '') AS feature_code
FROM sys_menu_permission mp
JOIN sys_permission p ON p.id = mp.permissionId AND p.isDeleted = 0
WHERE mp.menuId IN (?)
ORDER BY mp.menuId, p.id
LIMIT ?`, ids, remaining+1)
		if err != nil {
			return nil, err
		}
		var rows []menuPermissionRow
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list access menu permissions: %w", err)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("list access grant records: menu permission set exceeds %d", authorizationSetQueryMaxIDs)
		}
		menuPermissionCount += len(rows)
		for _, row := range rows {
			permissionsByMenu[row.MenuID] = append(permissionsByMenu[row.MenuID], row)
		}
	}
	type permissionCodeRow struct {
		PermissionID   int64  `db:"permission_id"`
		PermissionCode string `db:"permission_code"`
		PermissionName string `db:"permission_name"`
		FeatureCode    string `db:"feature_code"`
	}
	permissionByCode := make(map[string]permissionCodeRow)
	codes := uniqueNonBlankStrings(fallbackCodes)
	if len(codes) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list access grant records: menu permission code set exceeds %d", authorizationSetQueryMaxIDs)
	}
	for _, chunk := range chunkStrings(codes, authorizationSetQueryChunkSize) {
		remaining := authorizationSetQueryMaxIDs - len(permissionByCode)
		query, args, err := sqlx.In(`
SELECT id AS permission_id, code AS permission_code, name AS permission_name,
       COALESCE(featureCode, '') AS feature_code
FROM sys_permission
WHERE code IN (?) AND isDeleted = 0
ORDER BY code, id
LIMIT ?`, chunk, remaining+1)
		if err != nil {
			return nil, err
		}
		var rows []permissionCodeRow
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("list access permissions by code: %w", err)
		}
		if len(rows) > remaining {
			return nil, fmt.Errorf("list access grant records: permission code result exceeds %d", authorizationSetQueryMaxIDs)
		}
		for _, row := range rows {
			permissionByCode[strings.TrimSpace(row.PermissionCode)] = row
		}
	}
	for _, menu := range roleMenus {
		for _, permission := range permissionsByMenu[menu.MenuID] {
			for _, source := range sourcesByRole[menu.RoleID] {
				if len(records) >= authorizationSetQueryMaxIDs {
					return nil, fmt.Errorf("list access grant records: accumulated grant set exceeds %d", authorizationSetQueryMaxIDs)
				}
				feature := strings.TrimSpace(permission.FeatureCode)
				if feature == "" {
					feature = menu.MenuFeature
				}
				records = append(records, grantRecordFromRoleSource(source, domain.AccessGrantRecord{
					PermissionID: permission.PermissionID, PermissionCode: permission.PermissionCode,
					PermissionName: permission.PermissionName, PermissionStatus: permission.PermissionStatus,
					FeatureCode: feature, GrantSource: "MENU_DERIVED",
					MenuID: menu.MenuID, MenuName: menu.MenuName, MenuStatus: menu.MenuStatus,
				}))
			}
		}
		code := strings.TrimSpace(menu.MenuPermission)
		if code == "" {
			continue
		}
		permission := permissionByCode[code]
		name := permission.PermissionName
		if strings.TrimSpace(name) == "" {
			name = menu.MenuName
		}
		feature := strings.TrimSpace(permission.FeatureCode)
		if feature == "" {
			feature = menu.MenuFeature
		}
		for _, source := range sourcesByRole[menu.RoleID] {
			if len(records) >= authorizationSetQueryMaxIDs {
				return nil, fmt.Errorf("list access grant records: accumulated grant set exceeds %d", authorizationSetQueryMaxIDs)
			}
			records = append(records, grantRecordFromRoleSource(source, domain.AccessGrantRecord{
				PermissionID: permission.PermissionID, PermissionCode: code,
				PermissionName: name, PermissionStatus: 0, FeatureCode: feature,
				GrantSource: "MENU_DERIVED", MenuID: menu.MenuID,
				MenuName: menu.MenuName, MenuStatus: menu.MenuStatus,
			}))
		}
	}

	remaining := authorizationSetQueryMaxIDs - len(records)
	var temporary []domain.AccessGrantRecord
	if err := sqlx.SelectContext(ctx, exec, &temporary, r.rebind(exec, `
SELECT p.id AS permission_id, p.code AS permission_code, p.name AS permission_name,
       p.status AS permission_status, COALESCE(p.featureCode, '') AS feature_code,
       'TEMPORARY' AS grant_source,
       0 AS role_id, '' AS role_code, '' AS role_name, 0 AS role_status, '' AS assignment_source,
       0 AS post_id, '' AS post_code, '' AS post_name, 0 AS post_dept_id, 0 AS post_org_id,
       0 AS menu_id, '' AS menu_name, 0 AS menu_status,
       COALESCE(up.grantedBy, 0) AS granted_by, COALESCE(up.source, '') AS source,
       CASE WHEN up.type THEN 1 ELSE 0 END AS permission_type, up.expireTime AS expire_at
FROM sys_user_permission up
JOIN sys_permission p ON p.id = up.permissionId AND p.isDeleted = 0
WHERE up.userId = ? AND up.isDeleted = 0
ORDER BY p.code, p.id, up.id
LIMIT ?`), userID, remaining+1); err != nil {
		return nil, fmt.Errorf("list access temporary grants: %w", err)
	}
	if len(temporary) > remaining {
		return nil, fmt.Errorf("list access grant records: accumulated grant set exceeds %d", authorizationSetQueryMaxIDs)
	}
	records = append(records, temporary...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].PermissionCode != records[j].PermissionCode {
			return records[i].PermissionCode < records[j].PermissionCode
		}
		if records[i].GrantSource != records[j].GrantSource {
			return records[i].GrantSource < records[j].GrantSource
		}
		if records[i].RoleID != records[j].RoleID {
			return records[i].RoleID < records[j].RoleID
		}
		if records[i].AssignmentSource != records[j].AssignmentSource {
			return records[i].AssignmentSource < records[j].AssignmentSource
		}
		if records[i].PostID != records[j].PostID {
			return records[i].PostID < records[j].PostID
		}
		return records[i].MenuID < records[j].MenuID
	})
	return records, nil
}

func grantRecordFromRoleSource(source domain.AccessRoleSourceRecord, grant domain.AccessGrantRecord) domain.AccessGrantRecord {
	grant.RoleID = source.RoleID
	grant.RoleCode = source.RoleCode
	grant.RoleName = source.RoleName
	grant.RoleStatus = source.RoleStatus
	grant.AssignmentSource = source.AssignmentSource
	grant.PostID = source.PostID
	grant.PostCode = source.PostCode
	grant.PostName = source.PostName
	grant.PostDeptID = source.PostDeptID
	grant.PostOrgID = source.PostOrgID
	return grant
}

func uniqueNonBlankStrings(values []string) []string {
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
	sort.Strings(result)
	return result
}

func chunkStrings(values []string, size int) [][]string {
	if size <= 0 || len(values) == 0 {
		return nil
	}
	result := make([][]string, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		result = append(result, values[start:end])
	}
	return result
}

func (r *Repository) ListAccessRoleDeptRecords(ctx context.Context, roleIDs []int64) ([]domain.AccessRoleDeptRecord, error) {
	ids := uniquePositiveIDs(roleIDs)
	if len(ids) == 0 {
		return []domain.AccessRoleDeptRecord{}, nil
	}
	query, args, err := sqlx.In(`
SELECT roleId AS role_id, deptId AS dept_id
FROM sys_role_dept
WHERE roleId IN (?)
ORDER BY roleId, deptId
LIMIT ?`, ids, authorizationSetQueryMaxIDs+1)
	if err != nil {
		return nil, err
	}
	exec := store.SQLXExecutor(ctx, r.db)
	records := []domain.AccessRoleDeptRecord{}
	if err := sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, query), args...); err != nil {
		return nil, fmt.Errorf("list access role departments: %w", err)
	}
	if len(records) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list access role departments: result exceeds %d", authorizationSetQueryMaxIDs)
	}
	return records, nil
}

func (r *Repository) ListAccessMemberships(ctx context.Context, userID int64) ([]domain.AccessMembershipRecord, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	records := []domain.AccessMembershipRecord{}
	if err := sqlx.SelectContext(ctx, exec, &records, r.rebind(exec, `
SELECT 'ORG' AS kind, suo.orgId AS id, suo.orgId AS org_id, '' AS hierarchy
FROM sys_user_org suo
JOIN sys_org so ON so.id = suo.orgId AND so.isDeleted = 0
WHERE suo.userId = ? AND suo.isDeleted = 0
ORDER BY suo.orgId, suo.id
LIMIT ?`), userID, authorizationSetQueryMaxIDs+1); err != nil {
		return nil, fmt.Errorf("list access organization memberships: %w", err)
	}
	if len(records) > authorizationSetQueryMaxIDs {
		return nil, fmt.Errorf("list access memberships: organization set exceeds %d", authorizationSetQueryMaxIDs)
	}
	remaining := authorizationSetQueryMaxIDs - len(records)
	var departments []domain.AccessMembershipRecord
	if err := sqlx.SelectContext(ctx, exec, &departments, r.rebind(exec, `
SELECT 'DEPT' AS kind, sud.deptId AS id, sd.orgId AS org_id, COALESCE(sd.hierarchy, '') AS hierarchy
FROM sys_user_dept sud
JOIN sys_dept sd ON sd.id = sud.deptId AND sd.isDeleted = 0
WHERE sud.userId = ?
ORDER BY sud.deptId, sud.id
LIMIT ?`), userID, remaining+1); err != nil {
		return nil, fmt.Errorf("list access department memberships: %w", err)
	}
	if len(departments) > remaining {
		return nil, fmt.Errorf("list access memberships: accumulated membership set exceeds %d", authorizationSetQueryMaxIDs)
	}
	records = append(records, departments...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].ID < records[j].ID
	})
	return records, nil
}
