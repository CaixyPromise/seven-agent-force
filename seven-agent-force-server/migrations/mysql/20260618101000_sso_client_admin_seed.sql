-- +goose Up
SET @operatorId := 0;
SET @ssoPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 1900301100) FROM sys_permission);

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT @ssoPermissionBaseId + ROW_NUMBER() OVER (ORDER BY item.sortOrder), item.code, item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1 AS sortOrder, 'system:sso-client:list' AS code, 'SSO客户端列表' AS name, 'GET' AS method, '/sso/admin/clients' AS path, '分页查询SSO客户端列表' AS description
  UNION ALL SELECT 2, 'system:sso-client:query', 'SSO客户端详情', 'GET', '/sso/admin/clients/{clientId}', '查询SSO客户端详情'
  UNION ALL SELECT 3, 'system:sso-client:add', '创建SSO客户端', 'POST', '/sso/admin/clients', '创建SSO客户端'
  UNION ALL SELECT 4, 'system:sso-client:edit', '编辑SSO客户端', 'PUT', '/sso/admin/clients/{clientId}', '编辑SSO客户端'
  UNION ALL SELECT 5, 'system:sso-client:status', '启停SSO客户端', 'PUT', '/sso/admin/clients/{clientId}/status', '启用或停用SSO客户端'
  UNION ALL SELECT 6, 'system:sso-client:redirect:list', '查询SSO回调地址', 'GET', '/sso/admin/clients/{clientId}/redirect-uris', '查询SSO客户端回调地址'
  UNION ALL SELECT 7, 'system:sso-client:redirect:edit', '编辑SSO回调地址', 'PUT', '/sso/admin/clients/{clientId}/redirect-uris', '替换SSO客户端回调地址'
  UNION ALL SELECT 8, 'system:sso-client:secret:list', '查询SSO密钥', 'GET', '/sso/admin/clients/{clientId}/secrets', '查询SSO客户端密钥摘要'
  UNION ALL SELECT 9, 'system:sso-client:secret:generate', '生成SSO密钥', 'POST', '/sso/admin/clients/{clientId}/secrets', '生成SSO客户端密钥'
  UNION ALL SELECT 10, 'system:sso-client:secret:disable', '停用SSO密钥', 'PUT', '/sso/admin/clients/{clientId}/secrets/{secretId}/status', '停用SSO客户端密钥'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code AND existing.isDeleted = 0);

SET @ssoMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000000) FROM sys_menu);

INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, isFrame, isCache, visible, hierarchy, level, status, creatorId, createTime, updaterId, updateTime, isDeleted, remark)
SELECT @ssoMenuBaseId + 1, '安全中心', 1, 115, '/system/security', 'Layout', 'SafetyOutlined', 'M', NULL, 1, 1, 1, CONCAT('/1/', @ssoMenuBaseId + 1), 2, 0, @operatorId, NOW(), @operatorId, NOW(), 0, '系统安全管理分组'
WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/security' AND existing.isDeleted = 0);

UPDATE sys_menu
SET name = '安全中心', parentId = 1, sortOrder = 115, component = 'Layout', icon = 'SafetyOutlined', type = 'M',
    visible = 1, status = 0, updateTime = NOW()
WHERE path = '/system/security' AND isDeleted = 0;

SET @securityMenuId := (SELECT id FROM sys_menu WHERE path = '/system/security' AND isDeleted = 0 ORDER BY id LIMIT 1);

INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, isFrame, isCache, visible, hierarchy, level, status, creatorId, createTime, updaterId, updateTime, isDeleted, remark)
SELECT @ssoMenuBaseId + 2, 'SSO客户端', @securityMenuId, 10, '/system/sso-client', 'system/sso-client/index', 'SafetyOutlined', 'C', 'system:sso-client:list', 1, 1, 1, CONCAT('/1/', @securityMenuId, '/', @ssoMenuBaseId + 2), 3, 0, @operatorId, NOW(), @operatorId, NOW(), 0, 'OIDC客户端管理'
WHERE @securityMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/sso-client' AND existing.isDeleted = 0);

UPDATE sys_menu child
JOIN sys_menu parent ON parent.path = '/system/security' AND parent.isDeleted = 0
SET child.name = 'SSO客户端',
    child.parentId = parent.id,
    child.sortOrder = 10,
    child.component = 'system/sso-client/index',
    child.icon = 'SafetyOutlined',
    child.type = 'C',
    child.permission = 'system:sso-client:list',
    child.visible = 1,
    child.status = 0,
    child.updateTime = NOW()
WHERE child.path = '/system/sso-client' AND child.isDeleted = 0;

SET @ssoClientMenuId := (SELECT id FROM sys_menu WHERE path = '/system/sso-client' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @menuPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000100) FROM sys_menu_permission);

INSERT INTO sys_menu_permission (id, menuId, permissionId, creatorId, createTime)
SELECT @menuPermissionBaseId + ROW_NUMBER() OVER (ORDER BY p.id), @ssoClientMenuId, p.id, @operatorId, NOW()
FROM sys_permission p
WHERE p.code IN (
  'system:sso-client:list',
  'system:sso-client:query',
  'system:sso-client:add',
  'system:sso-client:edit',
  'system:sso-client:status',
  'system:sso-client:redirect:list',
  'system:sso-client:redirect:edit',
  'system:sso-client:secret:list',
  'system:sso-client:secret:generate',
  'system:sso-client:secret:disable'
)
AND @ssoClientMenuId IS NOT NULL
AND NOT EXISTS (
  SELECT 1 FROM sys_menu_permission existing WHERE existing.menuId = @ssoClientMenuId AND existing.permissionId = p.id
);

SET @roleMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000200) FROM sys_role_menu);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT @roleMenuBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId), candidate.roleId, candidate.menuId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path IN ('/system/security', '/system/sso-client') AND m.isDeleted = 0
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing.roleId = r.id AND existing.menuId = m.id)
) candidate;

SET @rolePermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000300) FROM sys_role_permission);

INSERT INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT @rolePermissionBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.permissionId), candidate.roleId, candidate.permissionId, @operatorId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, p.id AS permissionId
  FROM sys_role r
  JOIN sys_permission p ON p.isDeleted = 0 AND p.code IN (
    'system:sso-client:list',
    'system:sso-client:query',
    'system:sso-client:add',
    'system:sso-client:edit',
    'system:sso-client:status',
    'system:sso-client:redirect:list',
    'system:sso-client:redirect:edit',
    'system:sso-client:secret:list',
    'system:sso-client:secret:generate',
    'system:sso-client:secret:disable'
  )
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing.roleId = r.id AND existing.permissionId = p.id)
) candidate;

INSERT INTO sysSsoClient (
  clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson,
  requirePkce, requireConsent, trustedFirstParty, accessTokenTtlSec, refreshTokenTtlSec,
  status, metadataJson, creatorId, updaterId, isDeleted
)
SELECT
  'authorization-console',
  'Authorization Console',
  'PUBLIC',
  'none',
  JSON_ARRAY('authorization_code', 'refresh_token'),
  JSON_ARRAY('openid', 'profile', 'email', 'offline_access'),
  1,
  0,
  1,
  1800,
  2592000,
  0,
  JSON_OBJECT('seed', 'sso-provider-v1'),
  @operatorId,
  @operatorId,
  0
WHERE NOT EXISTS (SELECT 1 FROM sysSsoClient existing WHERE existing.clientId = 'authorization-console' AND existing.isDeleted = 0);

-- +goose Down
-- Stable seed data is intentionally retained on rollback to avoid deleting
-- existing security menus, role bindings, or later operator-maintained grants.
SELECT 1;
