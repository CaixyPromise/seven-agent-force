-- Optional Framework-side UI integration for the independently hosted
-- Workforce API. This file is deliberately outside seven-framework-server.
-- It is idempotent and grants the Agent workbench to the built-in admin roles.
DO $workforce_menu$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.ordinal), candidate.code, candidate.name,
           'API', candidate.method, candidate.path, 0, candidate.description,
           CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false
    FROM (
      SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412100000000) AS id FROM sys_permission
    ) base
    CROSS JOIN (VALUES
      (1, 'workforce:task:list', 'Workforce任务列表', 'GET', '/workforce/tasks', '查询Agent任务'),
      (2, 'workforce:task:query', 'Workforce任务详情', 'GET', '/workforce/tasks/{id}', '查询任务详情和事件'),
      (3, 'workforce:task:create', '创建Workforce任务', 'POST', '/workforce/tasks', '创建Agent任务'),
      (4, 'workforce:task:cancel', '取消Workforce任务', 'POST', '/workforce/tasks/{id}/cancel', '取消Agent任务'),
      (5, 'workforce:artifact:list', 'Workforce产物列表', 'GET', '/workforce/tasks/{id}/artifacts', '查询可信任务产物'),
      (6, 'workforce:artifact:download', '下载Workforce产物', 'GET', '/workforce/tasks/{id}/artifacts/{artifactId}/download', '下载可信任务产物')
    ) AS candidate(ordinal, code, name, method, path, description)
    WHERE NOT EXISTS (
      SELECT 1 FROM sys_permission existing
      WHERE existing.code = candidate.code AND existing."isDeleted" = false
    );
  END IF;

  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    INSERT INTO sys_menu (id, name, "parentId", "sortOrder", path, component, icon, type, permission,
                          "isFrame", "isCache", visible, hierarchy, level, status,
                          "creatorId", "createTime", "updaterId", "updateTime", "isDeleted", remark)
    SELECT base.id + 1, 'Agent工作台', parent.id, 5, '/workforce/tasks', 'workforce/tasks/index',
           'DeploymentUnitOutlined', 'C', 'workforce:task:list', true, true, true,
           CONCAT(COALESCE(NULLIF(parent.hierarchy, ''), '/1'), '/', base.id + 1), parent.level + 1, 0,
           0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, false, 'Seven Agent Force Agent任务与可信产物工作台'
    FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412100000100) AS id FROM sys_menu) base
    JOIN sys_menu parent ON parent.path = '/system' AND parent."isDeleted" = false
    WHERE NOT EXISTS (
      SELECT 1 FROM sys_menu existing WHERE existing.path = '/workforce/tasks' AND existing."isDeleted" = false
    );
  END IF;

  IF to_regclass('public.sys_menu_permission') IS NOT NULL THEN
    INSERT INTO sys_menu_permission (id, "menuId", "permissionId", "creatorId", "createTime")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.menu_id, candidate.permission_id),
           candidate.menu_id, candidate.permission_id, 0, CURRENT_TIMESTAMP
    FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412100000200) AS id FROM sys_menu_permission) base
    CROSS JOIN (
      SELECT menu.id AS menu_id, permission.id AS permission_id
      FROM sys_menu menu
      JOIN sys_permission permission ON permission.code LIKE 'workforce:%' AND permission."isDeleted" = false
      WHERE menu.path = '/workforce/tasks' AND menu."isDeleted" = false
        AND NOT EXISTS (
          SELECT 1 FROM sys_menu_permission existing
          WHERE existing."menuId" = menu.id AND existing."permissionId" = permission.id
        )
    ) candidate;
  END IF;

  IF to_regclass('public.sys_role_menu') IS NOT NULL THEN
    INSERT INTO sys_role_menu (id, "roleId", "menuId", "createTime", "updateTime")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.role_id, candidate.menu_id),
           candidate.role_id, candidate.menu_id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
    FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412100000300) AS id FROM sys_role_menu) base
    CROSS JOIN (
      SELECT role.id AS role_id, menu.id AS menu_id
      FROM sys_role role
      JOIN sys_menu menu ON menu.path = '/workforce/tasks' AND menu."isDeleted" = false
      WHERE role.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN') AND role."isDeleted" = false
        AND NOT EXISTS (
          SELECT 1 FROM sys_role_menu existing
          WHERE existing."roleId" = role.id AND existing."menuId" = menu.id
        )
    ) candidate;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.role_id, candidate.permission_id),
           candidate.role_id, candidate.permission_id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
    FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412100000400) AS id FROM sys_role_permission) base
    CROSS JOIN (
      SELECT role.id AS role_id, permission.id AS permission_id
      FROM sys_role role
      JOIN sys_permission permission ON permission.code LIKE 'workforce:%' AND permission."isDeleted" = false
      WHERE role.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN') AND role."isDeleted" = false
        AND NOT EXISTS (
          SELECT 1 FROM sys_role_permission existing
          WHERE existing."roleId" = role.id AND existing."permissionId" = permission.id
        )
    ) candidate;
  END IF;
END
$workforce_menu$;
