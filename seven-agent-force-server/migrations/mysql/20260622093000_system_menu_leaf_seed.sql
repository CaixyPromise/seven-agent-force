-- +goose Up
SET @operatorId := 0;
SET @systemMenuId := (SELECT id FROM sys_menu WHERE path = '/system' AND parentId = 0 AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @identityMenuId := (SELECT id FROM sys_menu WHERE path = '/system/identity' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @opsMenuId := (SELECT id FROM sys_menu WHERE path = '/system/ops' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @settingsMenuId := (SELECT id FROM sys_menu WHERE path = '/system/settings' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @auditMenuId := (SELECT id FROM sys_menu WHERE path = '/system/audit' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @menuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233100000000000) FROM sys_menu);

INSERT INTO sys_menu (
  id, name, parentId, sortOrder, path, component, icon, type, permission,
  isFrame, isCache, visible, hierarchy, level, status,
  creatorId, createTime, updaterId, updateTime, isDeleted, remark
)
SELECT
  @menuBaseId + ROW_NUMBER() OVER (ORDER BY item.path),
  item.name,
  item.parentId,
  item.sortOrder,
  item.path,
  item.component,
  item.icon,
  'C',
  item.permission,
  1,
  1,
  1,
  CONCAT('/1/', @systemMenuId, '/', item.parentId, '/', @menuBaseId + ROW_NUMBER() OVER (ORDER BY item.path)),
  3,
  0,
  @operatorId,
  NOW(),
  @operatorId,
  NOW(),
  0,
  item.remark
FROM (
  SELECT @identityMenuId AS parentId, 10 AS sortOrder, '/system/user' AS path, '用户管理' AS name, 'system/user/index' AS component, 'UserOutlined' AS icon, 'system:user:list' AS permission, '用户管理' AS remark
  UNION ALL SELECT @identityMenuId, 30, '/system/organization', '组织管理', 'system/organization/index', 'ApartmentOutlined', 'system:org:list', '组织管理'
  UNION ALL SELECT @identityMenuId, 40, '/system/department', '部门管理', 'system/department/index', 'BankOutlined', 'system:dept:list', '部门管理'
  UNION ALL SELECT @identityMenuId, 50, '/system/post', '岗位管理', 'system/post/index', 'IdcardOutlined', 'system:post:list', '岗位管理'
  UNION ALL SELECT @settingsMenuId, 10, '/system/config', '配置管理', 'system/config/index', 'SettingOutlined', 'system:config:query', '配置管理'
  UNION ALL SELECT @opsMenuId, 20, '/system/observability', '可观测性中心', 'system/observability/index', 'RadarChartOutlined', 'admin:observability:view', '可观测性中心'
  UNION ALL SELECT @auditMenuId, 10, '/system/operation-log', '操作审计', 'system/operation-log/index', 'FileTextOutlined', 'admin:log:view', '操作审计'
) item
WHERE item.parentId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = item.path AND existing.isDeleted = 0);

UPDATE sys_menu child
JOIN (
  SELECT @identityMenuId AS parentId, 10 AS sortOrder, '/system/user' AS path, '用户管理' AS name, 'system/user/index' AS component, 'UserOutlined' AS icon, 'system:user:list' AS permission, '用户管理' AS remark
  UNION ALL SELECT @identityMenuId, 20, '/system/organization-management', '组织架构', 'system/organization-management/index', 'ApartmentOutlined', 'system:org:list', '组织、部门、岗位聚合管理'
  UNION ALL SELECT @identityMenuId, 30, '/system/organization', '组织管理', 'system/organization/index', 'ApartmentOutlined', 'system:org:list', '组织管理'
  UNION ALL SELECT @identityMenuId, 40, '/system/department', '部门管理', 'system/department/index', 'BankOutlined', 'system:dept:list', '部门管理'
  UNION ALL SELECT @identityMenuId, 50, '/system/post', '岗位管理', 'system/post/index', 'IdcardOutlined', 'system:post:list', '岗位管理'
  UNION ALL SELECT @settingsMenuId, 10, '/system/config', '配置管理', 'system/config/index', 'SettingOutlined', 'system:config:query', '配置管理'
  UNION ALL SELECT @opsMenuId, 20, '/system/observability', '可观测性中心', 'system/observability/index', 'RadarChartOutlined', 'admin:observability:view', '可观测性中心'
  UNION ALL SELECT @auditMenuId, 10, '/system/operation-log', '操作审计', 'system/operation-log/index', 'FileTextOutlined', 'admin:log:view', '操作审计'
) item ON item.path = child.path
SET child.name = item.name,
    child.parentId = item.parentId,
    child.sortOrder = item.sortOrder,
    child.component = item.component,
    child.icon = item.icon,
    child.type = 'C',
    child.permission = item.permission,
    child.visible = 1,
    child.status = 0,
    child.hierarchy = CONCAT('/1/', @systemMenuId, '/', item.parentId, '/', child.id),
    child.level = 3,
    child.remark = item.remark,
    child.updateTime = NOW()
WHERE child.isDeleted = 0
  AND item.parentId IS NOT NULL;

SET @roleMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233100000000200) FROM sys_role_menu);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT
  @roleMenuBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId),
  candidate.roleId,
  candidate.menuId,
  NOW(),
  NOW()
FROM (
  SELECT DISTINCT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.isDeleted = 0 AND m.status = 0 AND m.path LIKE '/system%'
  WHERE r.code = 'SUPER_ADMIN'
    AND r.isDeleted = 0
    AND NOT EXISTS (
      SELECT 1 FROM sys_role_menu existing
      WHERE existing.roleId = r.id AND existing.menuId = m.id
    )
) candidate;

-- +goose Down
DELETE rm
FROM sys_role_menu rm
JOIN sys_role r ON r.id = rm.roleId AND r.code = 'SUPER_ADMIN'
JOIN sys_menu m ON m.id = rm.menuId
WHERE m.path IN (
  '/system/user',
  '/system/organization',
  '/system/department',
  '/system/post',
  '/system/config',
  '/system/observability',
  '/system/operation-log'
);
