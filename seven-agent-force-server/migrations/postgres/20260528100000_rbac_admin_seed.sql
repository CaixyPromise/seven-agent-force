-- +goose Up
-- +goose StatementBegin
DO $rbac$
BEGIN
  IF to_regclass('public.sys_role') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_role (id, name, code, "dataScope", status, type, "sortOrder", remark, "creatorId", "createTime", "updaterId", "updateTime", "isDeleted")
      SELECT 1900300001, '超级管理员', 'SUPER_ADMIN', 1, 0, 1, 0, 'RBAC admin seed role', 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, 0
      WHERE NOT EXISTS (SELECT 1 FROM sys_role WHERE code = 'SUPER_ADMIN' AND "isDeleted" = 0)
    $sql$;
  END IF;

  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
      SELECT v.id, v.code, v.name, v.resource_type, v.method, v.path, 0, v.description, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
      FROM (VALUES
        (1900300100, '*', '全部权限', 'API', '*', '*', '超级管理员全部权限'),
        (1900300101, 'system:role:list', '角色列表', 'API', 'GET', '/system/role/page', '分页查询角色'),
        (1900300102, 'system:role:query', '角色详情', 'API', 'GET', '/system/role/{id}', '查询角色详情与授权'),
        (1900300103, 'system:role:add', '新增角色', 'API', 'POST', '/system/role', '新增角色'),
        (1900300104, 'system:role:edit', '编辑角色', 'API', 'PUT', '/system/role', '编辑角色与菜单授权'),
        (1900300105, 'system:role:remove', '删除角色', 'API', 'DELETE', '/system/role/{id}', '删除角色'),
        (1900300106, 'system:role:grant', '角色权限分配', 'API', 'POST', '/system/role/permissions/assign', '分配角色权限资源'),
        (1900300111, 'system:menu:list', '菜单列表', 'API', 'GET', '/system/menu/tree', '查询菜单树'),
        (1900300112, 'system:menu:query', '菜单详情', 'API', 'GET', '/system/menu/{id}', '查询菜单详情'),
        (1900300113, 'system:menu:add', '新增菜单', 'API', 'POST', '/system/menu', '新增菜单'),
        (1900300114, 'system:menu:edit', '编辑菜单', 'API', 'PUT', '/system/menu', '编辑菜单'),
        (1900300115, 'system:menu:remove', '删除菜单', 'API', 'DELETE', '/system/menu/{id}', '删除菜单'),
        (1900300121, 'system:permission:list', '权限资源列表', 'API', 'GET', '/system/menu/permissions', '查询权限资源列表'),
        (1900300122, 'system:permission:query', '权限资源详情', 'API', 'GET', '/system/menu/permissions/{permissionId}', '查询权限资源详情'),
        (1900300123, 'system:permission:add', '新增权限资源', 'API', 'POST', '/system/menu/permissions', '新增权限资源'),
        (1900300124, 'system:permission:edit', '编辑权限资源', 'API', 'PUT', '/system/menu/permissions/{permissionId}', '编辑权限资源'),
        (1900300125, 'system:permission:remove', '删除权限资源', 'API', 'DELETE', '/system/menu/permissions/{permissionId}', '删除权限资源'),
        (1900300131, 'system:menu:permission:list', '菜单权限绑定列表', 'API', 'GET', '/system/menu/{menuId}/permissions', '查询菜单权限绑定'),
        (1900300132, 'system:menu:permission:assign', '菜单权限绑定', 'API', 'POST', '/system/menu/{menuId}/permissions', '绑定菜单权限'),
        (1900300133, 'system:user-role:assign', '用户角色分配', 'API', 'POST', '/system/role/user-roles/assign', '分配用户角色')
      ) AS v(id, code, name, resource_type, method, path, description)
      WHERE NOT EXISTS (SELECT 1 FROM sys_permission p WHERE p.code = v.code AND p."isDeleted" = 0)
    $sql$;
  END IF;

  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_menu (id, name, "parentId", "sortOrder", path, component, icon, type, permission, status, visible, "isFrame", "isCache", remark, "creatorId", "updaterId", "createTime", "updateTime", "isDeleted")
      SELECT v.id, v.name, v.parent_id, v.sort_order, v.path, v.component, v.icon, v.type, v.permission, 0, 0, 0, 0, v.remark, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
      FROM (VALUES
        (1900300200, '系统管理', 0, 100, '/system', NULL, 'SettingOutlined', 'M', NULL, 'RBAC admin seed parent menu'),
        (1900300201, '角色管理', 1900300200, 10, '/system/role', '/system/role', 'TeamOutlined', 'C', 'system:role:list', '角色管理'),
        (1900300202, '菜单管理', 1900300200, 20, '/system/menu', '/system/menu', 'MenuOutlined', 'C', 'system:menu:list', '菜单管理'),
        (1900300203, '权限管理', 1900300200, 30, '/system/permission', '/system/permission', 'SafetyOutlined', 'C', 'system:permission:list', '权限资源管理')
      ) AS v(id, name, parent_id, sort_order, path, component, icon, type, permission, remark)
      WHERE NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.id = v.id AND m."isDeleted" = 0)
    $sql$;
  END IF;

  IF to_regclass('public.sys_menu_permission') IS NOT NULL AND to_regclass('public.sys_menu') IS NOT NULL AND to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_menu_permission (id, "menuId", "permissionId", "creatorId", "createTime")
      SELECT 190030030000 + m.id + p.id, m.id, p.id, 0, CURRENT_TIMESTAMP
      FROM sys_menu m
      JOIN sys_permission p ON p."isDeleted" = 0 AND (
        (m.id = 1900300201 AND p.code LIKE 'system:role:%')
        OR (m.id = 1900300202 AND (p.code LIKE 'system:menu:%' OR p.code LIKE 'system:permission:%'))
        OR (m.id = 1900300203 AND p.code LIKE 'system:permission:%')
      )
      WHERE m.id IN (1900300201, 1900300202, 1900300203)
        AND m."isDeleted" = 0
        AND NOT EXISTS (SELECT 1 FROM sys_menu_permission existing WHERE existing."menuId" = m.id AND existing."permissionId" = p.id)
    $sql$;
  END IF;

  IF to_regclass('public.sys_role_menu') IS NOT NULL AND to_regclass('public.sys_role') IS NOT NULL AND to_regclass('public.sys_menu') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_role_menu (id, "roleId", "menuId", "updaterId", "createTime", "updateTime")
      SELECT 190030040000 + r.id + m.id, r.id, m.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM sys_role r
      JOIN sys_menu m ON m.id IN (1900300200, 1900300201, 1900300202, 1900300203) AND m."isDeleted" = 0
      WHERE r."isDeleted" = 0 AND r.code = 'SUPER_ADMIN'
        AND NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing."roleId" = r.id AND existing."menuId" = m.id)
    $sql$;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL AND to_regclass('public.sys_role') IS NOT NULL AND to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
      SELECT 190030050000 + r.id + p.id, r.id, p.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM sys_role r
      JOIN sys_permission p ON p."isDeleted" = 0 AND p.code IN (
        '*',
        'system:role:list', 'system:role:query', 'system:role:add', 'system:role:edit', 'system:role:remove', 'system:role:grant',
        'system:menu:list', 'system:menu:query', 'system:menu:add', 'system:menu:edit', 'system:menu:remove',
        'system:permission:list', 'system:permission:query', 'system:permission:add', 'system:permission:edit', 'system:permission:remove',
        'system:menu:permission:list', 'system:menu:permission:assign',
        'system:user-role:assign'
      )
      WHERE r."isDeleted" = 0 AND r.code = 'SUPER_ADMIN'
        AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing."roleId" = r.id AND existing."permissionId" = p.id)
    $sql$;
  END IF;
END
$rbac$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $rbac$
BEGIN
  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    EXECUTE $sql$
      DELETE FROM sys_role_permission
      WHERE "permissionId" IN (
        SELECT id FROM sys_permission WHERE code IN (
          '*',
          'system:role:list', 'system:role:query', 'system:role:add', 'system:role:edit', 'system:role:remove', 'system:role:grant',
          'system:menu:list', 'system:menu:query', 'system:menu:add', 'system:menu:edit', 'system:menu:remove',
          'system:permission:list', 'system:permission:query', 'system:permission:add', 'system:permission:edit', 'system:permission:remove',
          'system:menu:permission:list', 'system:menu:permission:assign',
          'system:user-role:assign'
        )
      )
    $sql$;
  END IF;
  IF to_regclass('public.sys_role_menu') IS NOT NULL THEN
    EXECUTE 'DELETE FROM sys_role_menu WHERE "menuId" IN (1900300200, 1900300201, 1900300202, 1900300203)';
  END IF;
  IF to_regclass('public.sys_menu_permission') IS NOT NULL THEN
    EXECUTE 'DELETE FROM sys_menu_permission WHERE "menuId" IN (1900300201, 1900300202, 1900300203)';
  END IF;
  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    EXECUTE 'DELETE FROM sys_menu WHERE id IN (1900300200, 1900300201, 1900300202, 1900300203)';
  END IF;
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      DELETE FROM sys_permission WHERE code IN (
        '*',
        'system:role:list', 'system:role:query', 'system:role:add', 'system:role:edit', 'system:role:remove', 'system:role:grant',
        'system:menu:list', 'system:menu:query', 'system:menu:add', 'system:menu:edit', 'system:menu:remove',
        'system:permission:list', 'system:permission:query', 'system:permission:add', 'system:permission:edit', 'system:permission:remove',
        'system:menu:permission:list', 'system:menu:permission:assign',
        'system:user-role:assign'
      )
    $sql$;
  END IF;
END
$rbac$;
-- +goose StatementEnd
