-- +goose Up
-- Seed platform menus that already have Go/UI implementations but were missing from RBAC menus.
UPDATE sys_menu
SET parentId = 1, path = '/system/config', permission = 'system:config:query', visible = 1, status = 0, updateTime = NOW()
WHERE name = '配置管理' AND isDeleted = 0;

SET @platform_menu_base_id := (SELECT GREATEST(COALESCE(MAX(id), 0) + 1, 2012232069056864258) FROM sys_menu);

INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, visible, hierarchy, level, status, createTime, updateTime, isDeleted, remark)
SELECT item.id, item.name, 1, item.sortOrder, item.path, item.component, NULL, 'C', item.permission, 1, CONCAT('/1/', item.id), 2, 0, NOW(), NOW(), 0, item.name
FROM (
  SELECT @platform_menu_base_id AS id, '字典管理' AS name, 12 AS sortOrder, '/system/dict' AS path, 'system/dict/index' AS component, 'system:dict:query' AS permission
  UNION ALL SELECT @platform_menu_base_id + 1, '在线用户', 13, '/system/online-user', 'system/online-user/index', 'admin:online:view'
  UNION ALL SELECT @platform_menu_base_id + 2, '应用运行日志', 14, '/system/runtime-log', 'system/runtime-log/index', 'admin:runtime-log:view'
  UNION ALL SELECT @platform_menu_base_id + 3, '文件列表', 15, '/system/files', 'system/files/index', 'system:file:list'
  UNION ALL SELECT @platform_menu_base_id + 4, '文件任务', 16, '/system/file-tasks', 'system/file-tasks/index', 'system:file-task:list'
  UNION ALL SELECT @platform_menu_base_id + 5, '存储策略', 17, '/system/storage', 'system/storage/index', 'system:storage:list'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = item.path AND existing.isDeleted = 0);

SET @platform_role_menu_base_id := (
  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232069056870000)
  FROM sys_role_menu
);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT
  @platform_role_menu_base_id + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId),
  candidate.roleId,
  candidate.menuId,
  NOW(),
  NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.isDeleted = 0
  WHERE r.code = 'SUPER_ADMIN'
    AND r.isDeleted = 0
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
      SELECT 1 FROM sys_role_menu rm WHERE rm.roleId = r.id AND rm.menuId = m.id
    )
) candidate;
