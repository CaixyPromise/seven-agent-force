-- +goose Up
-- Close the Docker operations permission model with three dedicated ops roles.

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT item.id, item.code, item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1900100501 AS id, 'admin:docker:container:terminal' AS code, 'Docker容器终端' AS name, 'GET' AS method, '/admin/docker/containers/{id}/terminal' AS path, '打开Docker容器终端' AS description
  UNION ALL SELECT 1900100502, 'admin:docker:volume:list', 'Docker Volume列表', 'GET', '/admin/docker/volumes', '查询Docker Volume列表'
  UNION ALL SELECT 1900100503, 'admin:docker:volume:create', '创建Docker Volume', 'POST', '/admin/docker/volumes', '创建Docker Volume'
  UNION ALL SELECT 1900100504, 'admin:docker:volume:prune', '清理Docker Volume', 'POST', '/admin/docker/volumes/prune/*', '清理未使用Docker Volume'
  UNION ALL SELECT 1900100505, 'admin:docker:volume:query', 'Docker Volume详情', 'GET', '/admin/docker/volumes/{name}', '查询Docker Volume详情'
  UNION ALL SELECT 1900100506, 'admin:docker:volume:delete', '删除Docker Volume', 'DELETE', '/admin/docker/volumes/{name}', '删除Docker Volume'
  UNION ALL SELECT 1900100507, 'admin:docker:network:list', 'Docker Network列表', 'GET', '/admin/docker/networks', '查询Docker Network列表'
  UNION ALL SELECT 1900100508, 'admin:docker:network:create', '创建Docker Network', 'POST', '/admin/docker/networks', '创建Docker Network'
  UNION ALL SELECT 1900100509, 'admin:docker:network:prune', '清理Docker Network', 'POST', '/admin/docker/networks/prune/*', '清理未使用Docker Network'
  UNION ALL SELECT 1900100510, 'admin:docker:network:query', 'Docker Network详情', 'GET', '/admin/docker/networks/{id}', '查询Docker Network详情'
  UNION ALL SELECT 1900100511, 'admin:docker:network:delete', '删除Docker Network', 'DELETE', '/admin/docker/networks/{id}', '删除Docker Network'
  UNION ALL SELECT 1900100512, 'admin:docker:network:connect', '连接Docker Network', 'POST', '/admin/docker/networks/{id}/connect', '连接容器到Docker Network'
  UNION ALL SELECT 1900100513, 'admin:docker:network:disconnect', '断开Docker Network', 'POST', '/admin/docker/networks/{id}/disconnect', '断开容器与Docker Network'
  UNION ALL SELECT 1900100514, 'admin:docker:config:query', 'Docker配置查询', 'GET', '/admin/docker/daemon/config', '查询Docker daemon配置'
  UNION ALL SELECT 1900100515, 'admin:docker:config:validate', 'Docker配置校验', 'POST', '/admin/docker/daemon/config/validate', '校验Docker daemon配置'
  UNION ALL SELECT 1900100516, 'admin:docker:config:update', 'Docker配置更新', 'PUT', '/admin/docker/daemon/config', '保存Docker daemon配置'
  UNION ALL SELECT 1900100517, 'admin:docker:config:restart', 'Docker daemon重启', 'POST', '/admin/docker/daemon/restart', '重启Docker daemon'
  UNION ALL SELECT 1900100518, 'admin:docker:registry:sync', '同步Docker Registry', 'POST', '/admin/docker/registries/{id}/sync', '同步Docker Registry'
  UNION ALL SELECT 1900100519, 'admin:docker:registry:delete', '删除Docker Registry', 'DELETE', '/admin/docker/registries/{id}', '删除本地Docker Registry配置'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code AND existing.isDeleted = 0);

INSERT INTO sys_role (id, name, code, dataScope, status, type, sortOrder, remark, creatorId, createTime, updaterId, updateTime, isDeleted)
SELECT item.id, item.name, item.code, 5, 0, 3, item.sortOrder, item.remark, 0, NOW(), 0, NOW(), 0
FROM (
  SELECT 2012232600000000001 AS id, '运维实习生' AS name, 'OPS_INTERN' AS code, 60 AS sortOrder, 'Docker operations read-only role' AS remark
  UNION ALL SELECT 2012232600000000002, '运维工程师', 'OPS_ENGINEER', 61, 'Docker operations controlled-action role'
  UNION ALL SELECT 2012232600000000003, '运维管理员', 'OPS_ADMIN', 62, 'Docker operations administrator role'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_role existing WHERE existing.code = item.code AND existing.isDeleted = 0);

SET @docker_menu_permission_base_id := (
  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232600000000100)
  FROM sys_menu_permission
);

INSERT INTO sys_menu_permission (id, menuId, permissionId, creatorId, createTime)
SELECT
  @docker_menu_permission_base_id + ROW_NUMBER() OVER (ORDER BY candidate.menuId, candidate.permissionId),
  candidate.menuId,
  candidate.permissionId,
  0,
  NOW()
FROM (
  SELECT m.id AS menuId, p.id AS permissionId
  FROM sys_menu m
  JOIN sys_permission p ON p.code LIKE 'admin:docker:%' AND p.isDeleted = 0
  WHERE m.path = '/system/docker'
    AND m.isDeleted = 0
    AND NOT EXISTS (
      SELECT 1 FROM sys_menu_permission mp WHERE mp.menuId = m.id AND mp.permissionId = p.id
    )
) candidate;

SET @docker_ops_role_menu_base_id := (
  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232600000000200)
  FROM sys_role_menu
);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT
  @docker_ops_role_menu_base_id + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId),
  candidate.roleId,
  candidate.menuId,
  NOW(),
  NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path = '/system/docker' AND m.isDeleted = 0
  WHERE r.code IN ('OPS_INTERN', 'OPS_ENGINEER', 'OPS_ADMIN')
    AND r.isDeleted = 0
    AND NOT EXISTS (
      SELECT 1 FROM sys_role_menu rm WHERE rm.roleId = r.id AND rm.menuId = m.id
    )
) candidate;

SET @docker_ops_role_permission_base_id := (
  SELECT GREATEST(COALESCE(MAX(id), 0), 2012232600000000300)
  FROM sys_role_permission
);

INSERT INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT
  @docker_ops_role_permission_base_id + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.permissionId),
  candidate.roleId,
  candidate.permissionId,
  0,
  NOW(),
  NOW()
FROM (
  SELECT r.id AS roleId, p.id AS permissionId
  FROM sys_role r
  JOIN sys_permission p ON p.isDeleted = 0
  WHERE r.isDeleted = 0
    AND (
      (r.code = 'OPS_INTERN' AND p.code IN (
        'admin:docker:container:list',
        'admin:docker:container:query',
        'admin:docker:container:logs',
        'admin:docker:image:list',
        'admin:docker:image:query',
        'admin:docker:image:containers',
        'admin:docker:compose:project:list',
        'admin:docker:compose:project:query',
        'admin:docker:compose:validate',
        'admin:docker:operation:list',
        'admin:docker:operation:query',
        'admin:docker:operation:stream',
        'admin:docker:volume:list',
        'admin:docker:volume:query',
        'admin:docker:network:list',
        'admin:docker:network:query',
        'admin:docker:registry:list',
        'admin:docker:config:query'
      ))
      OR (r.code = 'OPS_ENGINEER' AND p.code IN (
        'admin:docker:container:list',
        'admin:docker:container:query',
        'admin:docker:container:logs',
        'admin:docker:container:create',
        'admin:docker:container:start',
        'admin:docker:container:stop',
        'admin:docker:container:restart',
        'admin:docker:image:list',
        'admin:docker:image:query',
        'admin:docker:image:containers',
        'admin:docker:image:pull',
        'admin:docker:image:tag',
        'admin:docker:image:startup-preview',
        'admin:docker:compose:project:list',
        'admin:docker:compose:project:query',
        'admin:docker:compose:project:create',
        'admin:docker:compose:project:update',
        'admin:docker:compose:validate',
        'admin:docker:compose:up',
        'admin:docker:compose:workspace:check',
        'admin:docker:compose:yaml:validate',
        'admin:docker:compose:dockerfile:preview',
        'admin:docker:operation:list',
        'admin:docker:operation:query',
        'admin:docker:operation:stream',
        'admin:docker:operation:cancel',
        'admin:docker:volume:list',
        'admin:docker:volume:query',
        'admin:docker:volume:create',
        'admin:docker:network:list',
        'admin:docker:network:query',
        'admin:docker:network:create',
        'admin:docker:network:connect',
        'admin:docker:network:disconnect',
        'admin:docker:registry:list',
        'admin:docker:registry:test',
        'admin:docker:config:query',
        'admin:docker:config:validate'
      ))
      OR (r.code = 'OPS_ADMIN' AND p.code LIKE 'admin:docker:%')
      OR (r.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN') AND p.code LIKE 'admin:docker:%')
    )
    AND NOT EXISTS (
      SELECT 1 FROM sys_role_permission rp WHERE rp.roleId = r.id AND rp.permissionId = p.id
    )
) candidate;

-- +goose Down
-- Intentionally no-op: this migration seeds stable production roles and fills
-- missing Docker route permissions. Removing them on rollback would break live
-- operator access in initialized environments.
SELECT 1;
