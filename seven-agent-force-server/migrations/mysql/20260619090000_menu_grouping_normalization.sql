-- +goose Up
-- Normalize system menu groups for the UI sidebar and make OAuth client management visible.
SET @operatorId := 0;
SET @rootMenuId := (
  SELECT id
  FROM sys_menu
  WHERE path = '/system' AND isDeleted = 0
  ORDER BY visible DESC, sortOrder ASC, id
  LIMIT 1
);
SET @groupMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012232900000000000) FROM sys_menu);

INSERT INTO sys_menu (
  id, name, parentId, sortOrder, path, component, icon, type, permission,
  isFrame, isCache, visible, hierarchy, level, status,
  creatorId, createTime, updaterId, updateTime, isDeleted, remark
)
SELECT
  @groupMenuBaseId + ROW_NUMBER() OVER (ORDER BY item.sortOrder),
  item.name,
  @rootMenuId,
  item.sortOrder,
  item.path,
  'Layout',
  item.icon,
  'M',
  NULL,
  1,
  1,
  1,
  CONCAT('/1/', @rootMenuId, '/', @groupMenuBaseId + ROW_NUMBER() OVER (ORDER BY item.sortOrder)),
  2,
  0,
  @operatorId,
  NOW(),
  @operatorId,
  NOW(),
  0,
  item.remark
FROM (
  SELECT 10 AS sortOrder, '身份与组织' AS name, '/system/identity' AS path, 'TeamOutlined' AS icon, '用户、组织、部门、岗位管理' AS remark
  UNION ALL SELECT 20, '权限与认证', '/system/access', 'SafetyOutlined', '角色、权限、菜单与 OAuth 客户端管理'
  UNION ALL SELECT 30, '平台运维', '/system/ops', 'ClusterOutlined', 'Docker、可观测性、在线会话与运行日志'
  UNION ALL SELECT 40, '配置与内容', '/system/settings', 'SettingOutlined', '系统配置、字典、文件与存储策略'
  UNION ALL SELECT 50, '审计与日志', '/system/audit', 'FileTextOutlined', '操作审计入口'
) item
WHERE @rootMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = item.path AND existing.isDeleted = 0);

UPDATE sys_menu m
JOIN (
  SELECT 10 AS sortOrder, '身份与组织' AS name, '/system/identity' AS path, 'TeamOutlined' AS icon, '用户、组织、部门、岗位管理' AS remark
  UNION ALL SELECT 20, '权限与认证', '/system/access', 'SafetyOutlined', '角色、权限、菜单与 OAuth 客户端管理'
  UNION ALL SELECT 30, '平台运维', '/system/ops', 'ClusterOutlined', 'Docker、可观测性、在线会话与运行日志'
  UNION ALL SELECT 40, '配置与内容', '/system/settings', 'SettingOutlined', '系统配置、字典、文件与存储策略'
  UNION ALL SELECT 50, '审计与日志', '/system/audit', 'FileTextOutlined', '操作审计入口'
) item ON item.path = m.path
SET m.name = item.name,
    m.parentId = @rootMenuId,
    m.sortOrder = item.sortOrder,
    m.component = 'Layout',
    m.icon = item.icon,
    m.type = 'M',
    m.permission = NULL,
    m.visible = 1,
    m.status = 0,
    m.hierarchy = CONCAT('/1/', @rootMenuId, '/', m.id),
    m.level = 2,
    m.remark = item.remark,
    m.updateTime = NOW()
WHERE m.isDeleted = 0 AND @rootMenuId IS NOT NULL;

SET @identityMenuId := (SELECT id FROM sys_menu WHERE path = '/system/identity' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @accessMenuId := (SELECT id FROM sys_menu WHERE path = '/system/access' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @opsMenuId := (SELECT id FROM sys_menu WHERE path = '/system/ops' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @settingsMenuId := (SELECT id FROM sys_menu WHERE path = '/system/settings' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @auditMenuId := (SELECT id FROM sys_menu WHERE path = '/system/audit' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @newChildMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012232900000000100) FROM sys_menu);

INSERT INTO sys_menu (
  id, name, parentId, sortOrder, path, component, icon, type, permission,
  isFrame, isCache, visible, hierarchy, level, status,
  creatorId, createTime, updaterId, updateTime, isDeleted, remark
)
SELECT
  @newChildMenuBaseId + 1,
  '组织架构',
  @identityMenuId,
  20,
  '/system/organization-management',
  'system/organization-management/index',
  'ApartmentOutlined',
  'C',
  'system:org:list',
  1,
  1,
  1,
  CONCAT('/1/', @rootMenuId, '/', @identityMenuId, '/', @newChildMenuBaseId + 1),
  3,
  0,
  @operatorId,
  NOW(),
  @operatorId,
  NOW(),
  0,
  '组织、部门、岗位聚合管理'
WHERE @identityMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/organization-management' AND existing.isDeleted = 0);

UPDATE sys_menu child
JOIN (
  SELECT @identityMenuId AS parentId, 10 AS sortOrder, '/system/user' AS path, '用户管理' AS name, 'UserOutlined' AS icon, 'system:user:list' AS permission
  UNION ALL SELECT @identityMenuId, 20, '/system/organization-management', '组织架构', 'ApartmentOutlined', 'system:org:list'
  UNION ALL SELECT @identityMenuId, 30, '/system/organization', '组织管理', 'ApartmentOutlined', 'system:org:list'
  UNION ALL SELECT @identityMenuId, 40, '/system/department', '部门管理', 'BankOutlined', 'system:dept:list'
  UNION ALL SELECT @identityMenuId, 50, '/system/post', '岗位管理', 'IdcardOutlined', 'system:post:list'
  UNION ALL SELECT @accessMenuId, 10, '/system/role', '角色管理', 'TeamOutlined', 'system:role:list'
  UNION ALL SELECT @accessMenuId, 20, '/system/menu', '菜单管理', 'MenuOutlined', 'system:menu:list'
  UNION ALL SELECT @accessMenuId, 30, '/system/permission', '权限资源', 'SafetyOutlined', 'system:permission:list'
  UNION ALL SELECT @accessMenuId, 40, '/system/sso-client', 'OAuth 客户端', 'SafetyOutlined', 'system:sso-client:list'
  UNION ALL SELECT @opsMenuId, 10, '/system/docker', 'Docker 运维', 'ClusterOutlined', 'admin:docker:container:list'
  UNION ALL SELECT @opsMenuId, 20, '/system/observability', '可观测性中心', 'RadarChartOutlined', 'admin:observability:view'
  UNION ALL SELECT @opsMenuId, 30, '/system/online-user', '在线用户', 'GlobalOutlined', 'admin:online:view'
  UNION ALL SELECT @opsMenuId, 40, '/system/runtime-log', '应用运行日志', 'FileTextOutlined', 'admin:runtime-log:view'
  UNION ALL SELECT @settingsMenuId, 10, '/system/config', '配置管理', 'SettingOutlined', 'system:config:query'
  UNION ALL SELECT @settingsMenuId, 20, '/system/dict', '字典管理', 'SettingOutlined', 'system:dict:query'
  UNION ALL SELECT @settingsMenuId, 30, '/system/files', '文件列表', 'FileTextOutlined', 'system:file:list'
  UNION ALL SELECT @settingsMenuId, 40, '/system/file-tasks', '文件任务', 'FileTextOutlined', 'system:file-task:list'
  UNION ALL SELECT @settingsMenuId, 50, '/system/storage', '存储策略', 'FileTextOutlined', 'system:storage:list'
  UNION ALL SELECT @auditMenuId, 10, '/system/operation-log', '操作审计', 'FileTextOutlined', 'admin:log:view'
) target ON target.path = child.path
SET child.name = target.name,
    child.parentId = target.parentId,
    child.sortOrder = target.sortOrder,
    child.icon = target.icon,
    child.type = 'C',
    child.permission = target.permission,
    child.visible = 1,
    child.status = 0,
    child.hierarchy = CONCAT('/1/', @rootMenuId, '/', target.parentId, '/', child.id),
    child.level = 3,
    child.updateTime = NOW()
WHERE child.isDeleted = 0 AND target.parentId IS NOT NULL;

UPDATE sys_menu duplicate
JOIN (
  SELECT path, MIN(id) AS keepId
  FROM sys_menu
  WHERE isDeleted = 0
    AND path IN (
      '/system/role',
      '/system/menu',
      '/system/permission',
      '/system/user',
      '/system/organization',
      '/system/department',
      '/system/post',
      '/system/config',
      '/system/dict',
      '/system/online-user',
      '/system/runtime-log',
      '/system/files',
      '/system/file-tasks',
      '/system/storage',
      '/system/docker',
      '/system/observability',
      '/system/operation-log',
      '/system/sso-client',
      '/system/organization-management'
    )
  GROUP BY path
  HAVING COUNT(*) > 1
) canonical ON canonical.path = duplicate.path
SET duplicate.visible = 0,
    duplicate.sortOrder = duplicate.sortOrder + 1000,
    duplicate.updateTime = NOW()
WHERE duplicate.id <> canonical.keepId
  AND duplicate.isDeleted = 0;

SET @roleMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012232900000000200) FROM sys_role_menu);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT @roleMenuBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId), candidate.roleId, candidate.menuId, NOW(), NOW()
FROM (
  SELECT DISTINCT rm.roleId, parent.id AS menuId
  FROM sys_role_menu rm
  JOIN sys_menu child ON child.id = rm.menuId AND child.isDeleted = 0
  JOIN sys_menu parent ON parent.id = child.parentId AND parent.isDeleted = 0
  WHERE parent.path IN ('/system/identity', '/system/access', '/system/ops', '/system/settings', '/system/audit')
  UNION
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path IN (
    '/system/identity', '/system/access', '/system/ops', '/system/settings', '/system/audit',
    '/system/sso-client'
  ) AND m.isDeleted = 0
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
) candidate
WHERE NOT EXISTS (
  SELECT 1 FROM sys_role_menu existing WHERE existing.roleId = candidate.roleId AND existing.menuId = candidate.menuId
);

-- +goose Down
-- Intentionally no-op: this migration normalizes operator-maintained menus.
SELECT 1;
