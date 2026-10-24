-- +goose Up
-- Backfill canonical RBAC admin permissions for databases that already had the
-- legacy create/update/delete/assign permission codes before the Go route
-- contract switched to add/edit/remove/grant.

INSERT INTO sys_role_permission (roleId, permissionId, creatorId, createTime, updateTime)
SELECT DISTINCT source.roleId, target.id, 0, NOW(), NOW()
FROM (
  SELECT rp.roleId, mapping.newCode
  FROM sys_role_permission rp
  JOIN sys_permission oldPerm ON oldPerm.id = rp.permissionId AND oldPerm.isDeleted = 0
  JOIN (
    SELECT 'system:role:create' AS oldCode, 'system:role:add' AS newCode
    UNION ALL SELECT 'system:role:update', 'system:role:edit'
    UNION ALL SELECT 'system:role:delete', 'system:role:remove'
    UNION ALL SELECT 'system:role:assign', 'system:role:grant'
    UNION ALL SELECT 'system:menu:create', 'system:menu:add'
    UNION ALL SELECT 'system:menu:update', 'system:menu:edit'
    UNION ALL SELECT 'system:menu:delete', 'system:menu:remove'
    UNION ALL SELECT 'system:permission:create', 'system:permission:add'
    UNION ALL SELECT 'system:permission:update', 'system:permission:edit'
    UNION ALL SELECT 'system:permission:delete', 'system:permission:remove'
  ) mapping ON mapping.oldCode = oldPerm.code
  JOIN sys_role role ON role.id = rp.roleId AND role.isDeleted = 0
) source
JOIN sys_permission target ON target.code = source.newCode AND target.isDeleted = 0
WHERE NOT EXISTS (
  SELECT 1
  FROM sys_role_permission existing
  WHERE existing.roleId = source.roleId AND existing.permissionId = target.id
);

INSERT INTO sys_role_permission (roleId, permissionId, creatorId, createTime, updateTime)
SELECT DISTINCT role.id, permission.id, 0, NOW(), NOW()
FROM sys_role role
JOIN sys_permission permission ON permission.isDeleted = 0 AND permission.code IN (
  '*',
  'system:role:list', 'system:role:query', 'system:role:add', 'system:role:edit', 'system:role:remove', 'system:role:grant',
  'system:menu:list', 'system:menu:query', 'system:menu:add', 'system:menu:edit', 'system:menu:remove',
  'system:permission:list', 'system:permission:query', 'system:permission:add', 'system:permission:edit', 'system:permission:remove',
  'system:menu:permission:list', 'system:menu:permission:assign',
  'system:user-role:assign'
)
WHERE role.isDeleted = 0
  AND role.code = 'SUPER_ADMIN'
  AND NOT EXISTS (
    SELECT 1
    FROM sys_role_permission existing
    WHERE existing.roleId = role.id AND existing.permissionId = permission.id
  );

-- +goose Down
-- Intentionally no-op: removing permission grants on rollback can revoke
-- legitimate assignments made after the backfill.
