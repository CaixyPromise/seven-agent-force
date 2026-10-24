-- +goose Up
-- Seed the Docker operations menu so dynamic menu/button regressions can cover the Docker workbench.
SET @docker_menu_id := (
  SELECT COALESCE(
    (SELECT id FROM sys_menu WHERE path = '/system/docker' AND isDeleted = 0 LIMIT 1),
    GREATEST(COALESCE((SELECT MAX(id) + 1 FROM sys_menu), 0), 2012232407482671109)
  )
);

INSERT INTO sys_menu (
  id,
  name,
  parentId,
  sortOrder,
  path,
  component,
  icon,
  type,
  permission,
  visible,
  hierarchy,
  level,
  status,
  createTime,
  updateTime,
  isDeleted,
  remark
)
SELECT
  @docker_menu_id,
  'Docker运维',
  1,
  15,
  '/system/docker',
  'system/docker/index',
  'DockerOutlined',
  'C',
  'admin:docker:container:list',
  1,
  CONCAT('/1/', @docker_menu_id),
  2,
  0,
  NOW(),
  NOW(),
  0,
  'Docker运维工作台'
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path = '/system/docker' AND isDeleted = 0);

UPDATE sys_menu
SET
  parentId = 1,
  sortOrder = 15,
  component = 'system/docker/index',
  icon = 'DockerOutlined',
  type = 'C',
  permission = 'admin:docker:container:list',
  visible = 1,
  status = 0,
  hierarchy = CONCAT('/1/', id),
  level = 2,
  updateTime = NOW(),
  remark = IF(COALESCE(remark, '') = '', 'Docker运维工作台', remark)
WHERE path = '/system/docker' AND isDeleted = 0;

SET @docker_role_menu_base_id := (
  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232407482671200)
  FROM sys_role_menu
);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT
  @docker_role_menu_base_id + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId),
  candidate.roleId,
  candidate.menuId,
  NOW(),
  NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path = '/system/docker' AND m.isDeleted = 0
  WHERE r.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN')
    AND r.isDeleted = 0
    AND NOT EXISTS (
      SELECT 1 FROM sys_role_menu rm WHERE rm.roleId = r.id AND rm.menuId = m.id
    )
) candidate;

-- +goose Down
-- Intentionally no-op: this migration may normalize a pre-existing Docker menu.
-- Rolling back by deleting role-menu bindings or soft-deleting the menu would
-- remove valid operator access in initialized environments.
SELECT 1;
