-- +goose Up
-- Seed platform menus that already have Go/UI implementations but were missing from RBAC menus.
UPDATE sys_menu
SET "parentId" = 1, path = '/system/config', permission = 'system:config:query', visible = 1, status = 0, "updateTime" = NOW()
WHERE name = '配置管理' AND "isDeleted" = 0;

WITH base AS (
  SELECT GREATEST(COALESCE(MAX(id), 0) + 1, 2012232069056864258) AS id FROM sys_menu
),
items AS (
  SELECT base.id AS id, '字典管理' AS name, 12 AS sort_order, '/system/dict' AS path, 'system/dict/index' AS component, 'system:dict:query' AS permission FROM base
  UNION ALL SELECT base.id + 1, '在线用户', 13, '/system/online-user', 'system/online-user/index', 'admin:online:view' FROM base
  UNION ALL SELECT base.id + 2, '应用运行日志', 14, '/system/runtime-log', 'system/runtime-log/index', 'admin:runtime-log:view' FROM base
  UNION ALL SELECT base.id + 3, '文件列表', 15, '/system/files', 'system/files/index', 'system:file:list' FROM base
  UNION ALL SELECT base.id + 4, '文件任务', 16, '/system/file-tasks', 'system/file-tasks/index', 'system:file-task:list' FROM base
  UNION ALL SELECT base.id + 5, '存储策略', 17, '/system/storage', 'system/storage/index', 'system:storage:list' FROM base
)
INSERT INTO sys_menu (id, name, "parentId", "sortOrder", path, component, icon, type, permission, visible, hierarchy, level, status, "createTime", "updateTime", "isDeleted", remark)
SELECT item.id, item.name, 1, item.sort_order, item.path, item.component, NULL, 'C', item.permission, 1, '/1/' || item.id::text, 2, 0, NOW(), NOW(), 0, item.name
FROM items item
WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = item.path AND existing."isDeleted" = 0);

WITH base AS (
  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232069056870000) AS id FROM sys_role_menu
),
candidates AS (
  SELECT r.id AS role_id, m.id AS menu_id
  FROM sys_role r
  JOIN sys_menu m ON m."isDeleted" = 0
  WHERE r.code = 'SUPER_ADMIN'
    AND r."isDeleted" = 0
    AND m.path IN (
      '/system/config',
      '/system/dict',
      '/system/online-user',
      '/system/runtime-log',
      '/system/files',
      '/system/file-tasks',
      '/system/storage'
    )
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
