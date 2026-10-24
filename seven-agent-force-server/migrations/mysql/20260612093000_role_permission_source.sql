-- +goose Up
-- Track whether a role permission was granted directly or derived from menu
-- permission bindings. This prevents menu-permission rebind from deleting
-- explicitly granted role permissions.
SET @hasRolePermissionSource := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_role_permission'
    AND COLUMN_NAME = 'source'
);
SET @sql := IF(
  @hasRolePermissionSource = 0,
  'ALTER TABLE sys_role_permission ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT ''DIRECT'' COMMENT ''授权来源：DIRECT/MENU/BOTH'' AFTER permissionId',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE sys_role_permission rp
SET source = 'MENU'
WHERE EXISTS (
  SELECT 1
  FROM sys_role_menu rm
  JOIN sys_menu_permission mp ON mp.menuId = rm.menuId
  WHERE rm.roleId = rp.roleId AND mp.permissionId = rp.permissionId
);

-- +goose Down
-- Intentionally no-op: dropping provenance would reintroduce stale permission
-- recompute behavior and can lose explicit grant semantics.
