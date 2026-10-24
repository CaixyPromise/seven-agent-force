-- +goose Up
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT item.id, item.code, item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1900300901 AS id, 'admin:sso:session:list' AS code, 'SSO会话列表' AS name, 'GET' AS method, '/sso/admin/users/{userId}/sessions' AS path, '查询用户SSO会话' AS description
  UNION ALL SELECT 1900300902, 'admin:sso:session:kick', 'SSO会话踢出', 'POST', '/sso/admin/users/{userId}/sessions/{sessionId}/kick', '踢出用户SSO会话'
  UNION ALL SELECT 1900300903, 'admin:sso:device:list', 'SSO设备列表', 'GET', '/sso/admin/users/{userId}/devices', '查询用户SSO设备'
  UNION ALL SELECT 1900300904, 'admin:sso:device:kick', 'SSO设备踢出', 'POST', '/sso/admin/users/{userId}/devices/{deviceId}/kick', '踢出用户SSO设备'
  UNION ALL SELECT 1900300905, 'admin:ops:module:list', '模块运行列表', 'GET', '/ops/modules', '查询模块运行列表'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code AND existing.isDeleted = 0);

INSERT IGNORE INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT 190030090000 + r.id + p.id, r.id, p.id, 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.isDeleted = 0 AND p.code IN (
  'admin:sso:session:list',
  'admin:sso:session:kick',
  'admin:sso:device:list',
  'admin:sso:device:kick',
  'admin:ops:module:list'
)
WHERE r.isDeleted = 0 AND r.code = 'SUPER_ADMIN';

-- +goose Down
DELETE FROM sys_role_permission
WHERE permissionId IN (
  SELECT id FROM sys_permission WHERE code IN (
    'admin:sso:session:list',
    'admin:sso:session:kick',
    'admin:sso:device:list',
    'admin:sso:device:kick',
    'admin:ops:module:list'
  )
);

DELETE FROM sys_permission WHERE code IN (
  'admin:sso:session:list',
  'admin:sso:session:kick',
  'admin:sso:device:list',
  'admin:sso:device:kick',
  'admin:ops:module:list'
);
