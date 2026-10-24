-- +goose Up
-- Normalize system menu groups for the UI sidebar and make OAuth client management visible.
-- +goose StatementBegin
DO $menu_grouping_normalization$
DECLARE
  root_menu_id BIGINT;
  group_menu_base_id BIGINT;
  new_child_menu_base_id BIGINT;
BEGIN
  SELECT id INTO root_menu_id
  FROM sys_menu
  WHERE path = '/system' AND "isDeleted" = 0
  ORDER BY visible DESC, "sortOrder" ASC, id
  LIMIT 1;

  IF root_menu_id IS NULL THEN
    RETURN;
  END IF;

  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232900000000000) INTO group_menu_base_id FROM sys_menu;

  WITH items AS (
    SELECT *
    FROM (VALUES
      (10, '身份与组织', '/system/identity', 'TeamOutlined', '用户、组织、部门、岗位管理'),
      (20, '权限与认证', '/system/access', 'SafetyOutlined', '角色、权限、菜单与 OAuth 客户端管理'),
      (30, '平台运维', '/system/ops', 'ClusterOutlined', 'Docker、可观测性、在线会话与运行日志'),
      (40, '配置与内容', '/system/settings', 'SettingOutlined', '系统配置、字典、文件与存储策略'),
      (50, '审计与日志', '/system/audit', 'FileTextOutlined', '操作审计入口')
    ) AS item(sort_order, name, path, icon, remark)
  )
  INSERT INTO sys_menu (
    id, name, "parentId", "sortOrder", path, component, icon, type, permission,
    "isFrame", "isCache", visible, hierarchy, level, status,
    "creatorId", "createTime", "updaterId", "updateTime", "isDeleted", remark
  )
  SELECT
    group_menu_base_id + ROW_NUMBER() OVER (ORDER BY item.sort_order),
    item.name,
    root_menu_id,
    item.sort_order,
    item.path,
    'Layout',
    item.icon,
    'M',
    NULL,
    1,
    1,
    1,
    CONCAT('/1/', root_menu_id, '/', group_menu_base_id + ROW_NUMBER() OVER (ORDER BY item.sort_order)),
    2,
    0,
    0,
    CURRENT_TIMESTAMP,
    0,
    CURRENT_TIMESTAMP,
    0,
    item.remark
  FROM items item
  WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = item.path AND existing."isDeleted" = 0);

  WITH items AS (
    SELECT *
    FROM (VALUES
      (10, '身份与组织', '/system/identity', 'TeamOutlined', '用户、组织、部门、岗位管理'),
      (20, '权限与认证', '/system/access', 'SafetyOutlined', '角色、权限、菜单与 OAuth 客户端管理'),
      (30, '平台运维', '/system/ops', 'ClusterOutlined', 'Docker、可观测性、在线会话与运行日志'),
      (40, '配置与内容', '/system/settings', 'SettingOutlined', '系统配置、字典、文件与存储策略'),
      (50, '审计与日志', '/system/audit', 'FileTextOutlined', '操作审计入口')
    ) AS item(sort_order, name, path, icon, remark)
  )
  UPDATE sys_menu m
  SET name = item.name,
      "parentId" = root_menu_id,
      "sortOrder" = item.sort_order,
      component = 'Layout',
      icon = item.icon,
      type = 'M',
      permission = NULL,
      visible = 1,
      status = 0,
      hierarchy = CONCAT('/1/', root_menu_id, '/', m.id),
      level = 2,
      remark = item.remark,
      "updateTime" = CURRENT_TIMESTAMP
  FROM items item
  WHERE item.path = m.path AND m."isDeleted" = 0;

  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232900000000100) INTO new_child_menu_base_id FROM sys_menu;

  INSERT INTO sys_menu (
    id, name, "parentId", "sortOrder", path, component, icon, type, permission,
    "isFrame", "isCache", visible, hierarchy, level, status,
    "creatorId", "createTime", "updaterId", "updateTime", "isDeleted", remark
  )
  SELECT
    new_child_menu_base_id + 1,
    '组织架构',
    parent.id,
    20,
    '/system/organization-management',
    'system/organization-management/index',
    'ApartmentOutlined',
    'C',
    'system:org:list',
    1,
    1,
    1,
    CONCAT('/1/', root_menu_id, '/', parent.id, '/', new_child_menu_base_id + 1),
    3,
    0,
    0,
    CURRENT_TIMESTAMP,
    0,
    CURRENT_TIMESTAMP,
    0,
    '组织、部门、岗位聚合管理'
  FROM sys_menu parent
  WHERE parent.path = '/system/identity'
    AND parent."isDeleted" = 0
    AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/organization-management' AND existing."isDeleted" = 0);

  WITH groups AS (
    SELECT path, id FROM sys_menu WHERE path IN ('/system/identity', '/system/access', '/system/ops', '/system/settings', '/system/audit') AND "isDeleted" = 0
  ),
  targets AS (
    SELECT (SELECT id FROM groups WHERE path = '/system/identity') AS parent_id, 10 AS sort_order, '/system/user' AS path, '用户管理' AS name, 'UserOutlined' AS icon, 'system:user:list' AS permission
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/identity'), 20, '/system/organization-management', '组织架构', 'ApartmentOutlined', 'system:org:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/identity'), 30, '/system/organization', '组织管理', 'ApartmentOutlined', 'system:org:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/identity'), 40, '/system/department', '部门管理', 'BankOutlined', 'system:dept:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/identity'), 50, '/system/post', '岗位管理', 'IdcardOutlined', 'system:post:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/access'), 10, '/system/role', '角色管理', 'TeamOutlined', 'system:role:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/access'), 20, '/system/menu', '菜单管理', 'MenuOutlined', 'system:menu:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/access'), 30, '/system/permission', '权限资源', 'SafetyOutlined', 'system:permission:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/access'), 40, '/system/sso-client', 'OAuth 客户端', 'SafetyOutlined', 'system:sso-client:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/ops'), 10, '/system/docker', 'Docker 运维', 'ClusterOutlined', 'admin:docker:container:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/ops'), 20, '/system/observability', '可观测性中心', 'RadarChartOutlined', 'admin:observability:view'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/ops'), 30, '/system/online-user', '在线用户', 'GlobalOutlined', 'admin:online:view'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/ops'), 40, '/system/runtime-log', '应用运行日志', 'FileTextOutlined', 'admin:runtime-log:view'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/settings'), 10, '/system/config', '配置管理', 'SettingOutlined', 'system:config:query'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/settings'), 20, '/system/dict', '字典管理', 'SettingOutlined', 'system:dict:query'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/settings'), 30, '/system/files', '文件列表', 'FileTextOutlined', 'system:file:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/settings'), 40, '/system/file-tasks', '文件任务', 'FileTextOutlined', 'system:file-task:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/settings'), 50, '/system/storage', '存储策略', 'FileTextOutlined', 'system:storage:list'
    UNION ALL SELECT (SELECT id FROM groups WHERE path = '/system/audit'), 10, '/system/operation-log', '操作审计', 'FileTextOutlined', 'admin:log:view'
  )
  UPDATE sys_menu child
  SET name = target.name,
      "parentId" = target.parent_id,
      "sortOrder" = target.sort_order,
      icon = target.icon,
      type = 'C',
      permission = target.permission,
      visible = 1,
      status = 0,
      hierarchy = CONCAT('/1/', root_menu_id, '/', target.parent_id, '/', child.id),
      level = 3,
      "updateTime" = CURRENT_TIMESTAMP
  FROM targets target
  WHERE target.path = child.path
    AND child."isDeleted" = 0
    AND target.parent_id IS NOT NULL;

  WITH canonical AS (
    SELECT path, MIN(id) AS keep_id
    FROM sys_menu
    WHERE "isDeleted" = 0
      AND path IN (
        '/system/role',
        '/system/menu',
        '/system/permission',
        '/system/user',
        '/system/organization',
        '/system/department',
        '/system/post',
        '/system/config',
        '/system/dict',
        '/system/online-user',
        '/system/runtime-log',
        '/system/files',
        '/system/file-tasks',
        '/system/storage',
        '/system/docker',
        '/system/observability',
        '/system/operation-log',
        '/system/sso-client',
        '/system/organization-management'
      )
    GROUP BY path
    HAVING COUNT(*) > 1
  )
  UPDATE sys_menu duplicate
  SET visible = 0,
      "sortOrder" = duplicate."sortOrder" + 1000,
      "updateTime" = CURRENT_TIMESTAMP
  FROM canonical
  WHERE duplicate.path = canonical.path
    AND duplicate.id <> canonical.keep_id
    AND duplicate."isDeleted" = 0;

  WITH base AS (
    SELECT GREATEST(COALESCE(MAX(id), 0), 2012232900000000200) AS id FROM sys_role_menu
  ),
  candidates AS (
    SELECT DISTINCT rm."roleId" AS role_id, parent.id AS menu_id
    FROM sys_role_menu rm
    JOIN sys_menu child ON child.id = rm."menuId" AND child."isDeleted" = 0
    JOIN sys_menu parent ON parent.id = child."parentId" AND parent."isDeleted" = 0
    WHERE parent.path IN ('/system/identity', '/system/access', '/system/ops', '/system/settings', '/system/audit')
    UNION
    SELECT r.id AS role_id, m.id AS menu_id
    FROM sys_role r
    JOIN sys_menu m ON m.path IN (
      '/system/identity', '/system/access', '/system/ops', '/system/settings', '/system/audit',
      '/system/sso-client'
    ) AND m."isDeleted" = 0
    WHERE r.code = 'SUPER_ADMIN' AND r."isDeleted" = 0
  )
  INSERT INTO sys_role_menu (id, "roleId", "menuId", "createTime", "updateTime")
  SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidates.role_id, candidates.menu_id),
         candidates.role_id,
         candidates.menu_id,
         CURRENT_TIMESTAMP,
         CURRENT_TIMESTAMP
  FROM candidates
  CROSS JOIN base
  WHERE NOT EXISTS (
    SELECT 1 FROM sys_role_menu existing WHERE existing."roleId" = candidates.role_id AND existing."menuId" = candidates.menu_id
  );
END
$menu_grouping_normalization$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $menu_grouping_normalization$
BEGIN
  -- Intentionally no-op: this migration normalizes operator-maintained menus.
  PERFORM 1;
END
$menu_grouping_normalization$;
-- +goose StatementEnd
