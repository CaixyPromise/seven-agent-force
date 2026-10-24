-- +goose Up
SET @operatorId := 0;
SET @platformPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 1900301300) FROM sys_permission);

UPDATE sys_permission existing
JOIN (
  SELECT 1 AS sortOrder, 'system:platform:list' AS code, '平台列表' AS name, 'GET' AS method, '/platform/admin/platforms' AS path, '分页查询平台列表' AS description
  UNION ALL SELECT 2, 'system:platform:query', '平台详情', 'GET', '/platform/admin/platforms/:platformCode', '查询平台详情'
  UNION ALL SELECT 3, 'system:platform:add', '创建平台', 'POST', '/platform/admin/platforms', '创建平台配置'
  UNION ALL SELECT 4, 'system:platform:edit', '编辑平台', 'PUT', '/platform/admin/platforms/:platformCode', '编辑平台配置'
  UNION ALL SELECT 5, 'system:platform:status', '启停平台', 'PUT', '/platform/admin/platforms/:platformCode/status', '启用或停用平台'
  UNION ALL SELECT 6, 'system:platform:login-method:edit', '编辑平台登录方式', 'PUT', '/platform/admin/platforms/:platformCode/login-methods', '替换平台登录方式'
  UNION ALL SELECT 7, 'system:platform:source-rule:edit', '编辑平台来源规则', 'PUT', '/platform/admin/platforms/:platformCode/source-rules', '替换平台来源规则'
  UNION ALL SELECT 8, 'system:platform:default-role:edit', '编辑平台默认角色', 'PUT', '/platform/admin/platforms/:platformCode/default-roles', '替换平台默认角色'
) item ON existing.code = item.code
SET existing.name = item.name,
    existing.resourceType = 'API',
    existing.method = item.method,
    existing.path = item.path,
    existing.status = 0,
    existing.description = item.description,
    existing.updateTime = NOW(),
    existing.isDeleted = 0;

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT @platformPermissionBaseId + ROW_NUMBER() OVER (ORDER BY item.sortOrder), item.code, item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1 AS sortOrder, 'system:platform:list' AS code, '平台列表' AS name, 'GET' AS method, '/platform/admin/platforms' AS path, '分页查询平台列表' AS description
  UNION ALL SELECT 2, 'system:platform:query', '平台详情', 'GET', '/platform/admin/platforms/:platformCode', '查询平台详情'
  UNION ALL SELECT 3, 'system:platform:add', '创建平台', 'POST', '/platform/admin/platforms', '创建平台配置'
  UNION ALL SELECT 4, 'system:platform:edit', '编辑平台', 'PUT', '/platform/admin/platforms/:platformCode', '编辑平台配置'
  UNION ALL SELECT 5, 'system:platform:status', '启停平台', 'PUT', '/platform/admin/platforms/:platformCode/status', '启用或停用平台'
  UNION ALL SELECT 6, 'system:platform:login-method:edit', '编辑平台登录方式', 'PUT', '/platform/admin/platforms/:platformCode/login-methods', '替换平台登录方式'
  UNION ALL SELECT 7, 'system:platform:source-rule:edit', '编辑平台来源规则', 'PUT', '/platform/admin/platforms/:platformCode/source-rules', '替换平台来源规则'
  UNION ALL SELECT 8, 'system:platform:default-role:edit', '编辑平台默认角色', 'PUT', '/platform/admin/platforms/:platformCode/default-roles', '替换平台默认角色'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code);

SET @rootMenuId := (
  SELECT id
  FROM sys_menu
  WHERE path = '/system' AND isDeleted = 0
  ORDER BY visible DESC, sortOrder ASC, id
  LIMIT 1
);
SET @accessMenuId := (SELECT id FROM sys_menu WHERE path = '/system/access' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @platformMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000000000) FROM sys_menu);

INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, isFrame, isCache, visible, hierarchy, level, status, creatorId, createTime, updaterId, updateTime, isDeleted, remark)
SELECT @platformMenuBaseId + 1, '平台管理', @accessMenuId, 60, '/system/platform', 'system/platform/index', 'AppstoreOutlined', 'C', 'system:platform:list', 1, 1, 1, CONCAT('/1/', @rootMenuId, '/', @accessMenuId, '/', @platformMenuBaseId + 1), 3, 0, @operatorId, NOW(), @operatorId, NOW(), 0, '平台入口、登录策略与默认权限管理'
WHERE @accessMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/platform' AND existing.isDeleted = 0);

UPDATE sys_menu child
JOIN sys_menu parent ON parent.path = '/system/access' AND parent.isDeleted = 0
LEFT JOIN sys_menu root ON root.path = '/system' AND root.isDeleted = 0
SET child.name = '平台管理',
    child.parentId = parent.id,
    child.sortOrder = 60,
    child.component = 'system/platform/index',
    child.icon = 'AppstoreOutlined',
    child.type = 'C',
    child.permission = 'system:platform:list',
    child.visible = 1,
    child.status = 0,
    child.hierarchy = CONCAT('/1/', COALESCE(root.id, parent.parentId), '/', parent.id, '/', child.id),
    child.level = 3,
    child.updateTime = NOW()
WHERE child.path = '/system/platform' AND child.isDeleted = 0;

SET @platformMenuId := (SELECT id FROM sys_menu WHERE path = '/system/platform' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @menuPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000000100) FROM sys_menu_permission);

INSERT INTO sys_menu_permission (id, menuId, permissionId, creatorId, createTime)
SELECT @menuPermissionBaseId + ROW_NUMBER() OVER (ORDER BY p.id), @platformMenuId, p.id, @operatorId, NOW()
FROM sys_permission p
WHERE p.code LIKE 'system:platform:%'
AND @platformMenuId IS NOT NULL
AND NOT EXISTS (
  SELECT 1 FROM sys_menu_permission existing WHERE existing.menuId = @platformMenuId AND existing.permissionId = p.id
);

SET @roleMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000000200) FROM sys_role_menu);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT @roleMenuBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId), candidate.roleId, candidate.menuId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path IN ('/system/access', '/system/platform') AND m.isDeleted = 0
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing.roleId = r.id AND existing.menuId = m.id)
) candidate;

SET @rolePermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000000300) FROM sys_role_permission);

INSERT INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT @rolePermissionBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.permissionId), candidate.roleId, candidate.permissionId, @operatorId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, p.id AS permissionId
  FROM sys_role r
  JOIN sys_permission p ON p.isDeleted = 0 AND p.code LIKE 'system:platform:%'
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing.roleId = r.id AND existing.permissionId = p.id)
) candidate;

INSERT INTO sysPlatform (platformCode, platformName, platformType, description, defaultRedirectUrl, allowAutoRegister, isDefault, brandJson, settingsJson, status, creatorId, updaterId)
SELECT 'seven-admin', 'Seven 管理后台', 'ADMIN', 'Seven 默认管理后台平台', 'http://127.0.0.1:5291/', 0, 1,
       JSON_OBJECT('title', 'Seven', 'subtitle', '统一身份认证系统', 'theme', 'blue-cyan'),
       JSON_OBJECT('seed', 'platform-management-v1'), 0, @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysPlatform WHERE platformCode = 'seven-admin' AND isDeleted = 0);

INSERT INTO sysPlatformSsoClient (platformCode, clientId, creatorId, updaterId)
SELECT 'seven-admin', 'authorization-console', @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysPlatformSsoClient WHERE platformCode = 'seven-admin' AND clientId = 'authorization-console' AND isDeleted = 0);

INSERT INTO sysPlatformSourceRule (platformCode, matchType, matchValue, priority, creatorId, updaterId)
SELECT 'seven-admin', 'CLIENT_ID', 'authorization-console', 100, @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysPlatformSourceRule WHERE platformCode = 'seven-admin' AND matchType = 'CLIENT_ID' AND matchValue = 'authorization-console' AND isDeleted = 0);

INSERT INTO sysPlatformSourceRule (platformCode, matchType, matchValue, priority, creatorId, updaterId)
SELECT 'seven-admin', 'HOST', '127.0.0.1:5291', 50, @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysPlatformSourceRule WHERE platformCode = 'seven-admin' AND matchType = 'HOST' AND matchValue = '127.0.0.1:5291' AND isDeleted = 0);

INSERT INTO sysPlatformLoginMethod (platformCode, methodType, providerCode, displayName, icon, sortOrder, displayEnabled, loginEnabled, creatorId, updaterId)
SELECT candidate.platformCode, candidate.methodType, candidate.providerCode, candidate.displayName, candidate.icon, candidate.sortOrder, 1, 1, @operatorId, @operatorId
FROM (
  SELECT 'seven-admin' AS platformCode, 'PASSWORD' AS methodType, '' AS providerCode, '账号密码登录' AS displayName, 'LockOutlined' AS icon, 10 AS sortOrder
  UNION ALL SELECT 'seven-admin', 'PASSKEY', '', '通行密钥', 'KeyOutlined', 20
  UNION ALL SELECT 'seven-admin', 'EXTERNAL_OAUTH', 'github', 'GitHub', 'GithubOutlined', 30
  UNION ALL SELECT 'seven-admin', 'EXTERNAL_OAUTH', 'google', 'Google', 'GoogleOutlined', 40
) candidate
WHERE NOT EXISTS (
  SELECT 1 FROM sysPlatformLoginMethod existing
  WHERE existing.platformCode = candidate.platformCode
    AND existing.methodType = candidate.methodType
    AND existing.providerCode = candidate.providerCode
    AND existing.isDeleted = 0
);

-- +goose Down
-- Stable seed data is intentionally retained on rollback to avoid deleting
-- existing security menus, role bindings, or later operator-maintained grants.
SELECT 1;
