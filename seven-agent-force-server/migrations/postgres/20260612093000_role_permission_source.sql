-- +goose Up
-- Track whether a role permission was granted directly or derived from menu
-- permission bindings. This prevents menu-permission rebind from deleting
-- explicitly granted role permissions.
ALTER TABLE sys_role_permission
  ADD COLUMN IF NOT EXISTS source VARCHAR(16) NOT NULL DEFAULT 'DIRECT';

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
