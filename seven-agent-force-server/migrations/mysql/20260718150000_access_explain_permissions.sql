-- +goose Up
INSERT IGNORE INTO sys_permission (
  id, code, name, resourceType, method, path, status, description,
  creatorId, createTime, updaterId, updateTime, isDeleted
) VALUES
  (1900301062, 'system:user:access:query', '查询用户有效权限', 'API', 'GET', '/system/user/:id/effective-access', 0, '查询目标用户的有效角色、数据范围和权限来源', 0, NOW(), 0, NOW(), 0),
  (1900301063, 'system:user:access:explain', '解释用户权限', 'API', 'GET', '/system/user/:id/access-explain', 0, '解释目标用户指定权限的允许或拒绝原因', 0, NOW(), 0, NOW(), 0);

INSERT IGNORE INTO sys_role_permission (
  id, roleId, permissionId, source, creatorId, createTime, updateTime
)
SELECT 190030106200 + r.id + p.id, r.id, p.id, 'DIRECT', 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.id BETWEEN 1900301062 AND 1900301063 AND p.isDeleted = 0
WHERE r.systemKey = 'AUTHORIZATION_ROOT' AND r.isDeleted = 0;

-- +goose Down
DELETE FROM sys_role_permission
WHERE permissionId IN (1900301062, 1900301063);

DELETE FROM sys_permission
WHERE id IN (1900301062, 1900301063)
  AND code IN ('system:user:access:query', 'system:user:access:explain');
