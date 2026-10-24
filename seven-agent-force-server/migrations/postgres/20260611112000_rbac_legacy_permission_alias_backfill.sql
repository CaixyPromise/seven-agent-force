-- +goose Up
-- Backfill canonical RBAC admin permissions for databases that already had the
-- legacy create/update/delete/assign permission codes before the Go route
-- contract switched to add/edit/remove/grant.

-- +goose StatementBegin
DO $rbac_alias_backfill$
BEGIN
  IF to_regclass('public.sys_role_permission') IS NOT NULL
     AND to_regclass('public.sys_permission') IS NOT NULL
     AND to_regclass('public.sys_role') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_role_permission ("roleId", "permissionId", "creatorId", "createTime", "updateTime")
      SELECT DISTINCT source."roleId", target.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM (
        SELECT rp."roleId", mapping.new_code
        FROM sys_role_permission rp
        JOIN sys_permission old_perm ON old_perm.id = rp."permissionId" AND old_perm."isDeleted" = 0
        JOIN (
          VALUES
            ('system:role:create', 'system:role:add'),
            ('system:role:update', 'system:role:edit'),
            ('system:role:delete', 'system:role:remove'),
            ('system:role:assign', 'system:role:grant'),
            ('system:menu:create', 'system:menu:add'),
            ('system:menu:update', 'system:menu:edit'),
            ('system:menu:delete', 'system:menu:remove'),
            ('system:permission:create', 'system:permission:add'),
            ('system:permission:update', 'system:permission:edit'),
            ('system:permission:delete', 'system:permission:remove')
        ) AS mapping(old_code, new_code) ON mapping.old_code = old_perm.code
        JOIN sys_role role ON role.id = rp."roleId" AND role."isDeleted" = 0
      ) source
      JOIN sys_permission target ON target.code = source.new_code AND target."isDeleted" = 0
      WHERE NOT EXISTS (
        SELECT 1
        FROM sys_role_permission existing
        WHERE existing."roleId" = source."roleId" AND existing."permissionId" = target.id
      )
    $sql$;

    EXECUTE $sql$
      INSERT INTO sys_role_permission ("roleId", "permissionId", "creatorId", "createTime", "updateTime")
      SELECT DISTINCT role.id, permission.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM sys_role role
      JOIN sys_permission permission ON permission."isDeleted" = 0 AND permission.code IN (
        '*',
        'system:role:list', 'system:role:query', 'system:role:add', 'system:role:edit', 'system:role:remove', 'system:role:grant',
        'system:menu:list', 'system:menu:query', 'system:menu:add', 'system:menu:edit', 'system:menu:remove',
        'system:permission:list', 'system:permission:query', 'system:permission:add', 'system:permission:edit', 'system:permission:remove',
        'system:menu:permission:list', 'system:menu:permission:assign',
        'system:user-role:assign'
      )
      WHERE role."isDeleted" = 0
        AND role.code = 'SUPER_ADMIN'
        AND NOT EXISTS (
          SELECT 1
          FROM sys_role_permission existing
          WHERE existing."roleId" = role.id AND existing."permissionId" = permission.id
        )
    $sql$;
  END IF;
END
$rbac_alias_backfill$;
-- +goose StatementEnd

-- +goose Down
-- Intentionally no-op: removing permission grants on rollback can revoke
-- legitimate assignments made after the backfill.
