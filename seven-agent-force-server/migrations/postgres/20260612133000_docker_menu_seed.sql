-- +goose Up
-- Seed the Docker operations menu so dynamic menu/button regressions can cover the Docker workbench.
WITH docker_menu_id AS (
  SELECT COALESCE(
    (SELECT id FROM sys_menu WHERE path = '/system/docker' AND "isDeleted" = 0 LIMIT 1),
    GREATEST(COALESCE((SELECT MAX(id) + 1 FROM sys_menu), 0), 2012232407482671109)
  ) AS id
),
inserted AS (
  INSERT INTO sys_menu (
    id,
    name,
    "parentId",
    "sortOrder",
    path,
    component,
    icon,
    type,
    permission,
    visible,
    hierarchy,
    level,
    status,
    "createTime",
    "updateTime",
    "isDeleted",
    remark
  )
  SELECT
    docker_menu_id.id,
    'Docker运维',
    1,
    15,
    '/system/docker',
    'system/docker/index',
    'DockerOutlined',
    'C',
    'admin:docker:container:list',
    1,
    '/1/' || docker_menu_id.id::text,
    2,
    0,
    NOW(),
    NOW(),
    0,
    'Docker运维工作台'
  FROM docker_menu_id
  WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path = '/system/docker' AND "isDeleted" = 0)
  RETURNING id
)
UPDATE sys_menu
SET
  "parentId" = 1,
  "sortOrder" = 15,
  component = 'system/docker/index',
  icon = 'DockerOutlined',
  type = 'C',
  permission = 'admin:docker:container:list',
  visible = 1,
  status = 0,
  hierarchy = '/1/' || id::text,
  level = 2,
  "updateTime" = NOW(),
  remark = CASE WHEN COALESCE(remark, '') = '' THEN 'Docker运维工作台' ELSE remark END
WHERE path = '/system/docker' AND "isDeleted" = 0;

WITH base AS (
  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232407482671200) AS id FROM sys_role_menu
),
candidates AS (
  SELECT r.id AS role_id, m.id AS menu_id
  FROM sys_role r
  JOIN sys_menu m ON m.path = '/system/docker' AND m."isDeleted" = 0
  WHERE r.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN')
    AND r."isDeleted" = 0
    AND NOT EXISTS (
      SELECT 1 FROM sys_role_menu rm WHERE rm."roleId" = r.id AND rm."menuId" = m.id
    )
)
INSERT INTO sys_role_menu (id, "roleId", "menuId", "createTime", "updateTime")
SELECT
  base.id + ROW_NUMBER() OVER (ORDER BY candidates.role_id, candidates.menu_id),
  candidates.role_id,
  candidates.menu_id,
  NOW(),
  NOW()
FROM candidates
CROSS JOIN base;

-- +goose Down
-- Intentionally no-op: this migration may normalize a pre-existing Docker menu.
-- Rolling back by deleting role-menu bindings or soft-deleting the menu would
-- remove valid operator access in initialized environments.
SELECT 1;
