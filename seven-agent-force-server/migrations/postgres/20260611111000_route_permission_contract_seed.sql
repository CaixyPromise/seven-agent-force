-- +goose Up
-- +goose StatementBegin
DO $route_permissions$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
      SELECT v.id, v.code, v.name, 'API', v.method, v.path, 0, v.description, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
      FROM (VALUES
        (1900301001, 'admin:log:clean', 'admin log clean', 'POST', '/admin/logs/operation/clean', 'admin log clean'),
        (1900301002, 'admin:log:delete', 'admin log delete', 'POST', '/admin/logs/operation/deleteByTimeRange', 'admin log delete'),
        (1900301003, 'admin:log:export', 'admin log export', 'GET', '/admin/logs/operation/export', 'admin log export'),
        (1900301004, 'admin:log:view', 'admin log view', 'GET', '/admin/logs/operation', 'admin log view'),
        (1900301005, 'admin:online:kick', 'admin online kick', 'POST', '/admin/kick/:userId', 'admin online kick'),
        (1900301006, 'admin:temp-permission:cleanup', 'admin temp-permission cleanup', 'POST', '/admin/temp-permission/cleanup', 'admin temp-permission cleanup'),
        (1900301007, 'admin:temp-permission:extend', 'admin temp-permission extend', 'PUT', '/admin/temp-permission/extend', 'admin temp-permission extend'),
        (1900301008, 'admin:temp-permission:grant', 'admin temp-permission grant', 'POST', '/admin/temp-permission/grant', 'admin temp-permission grant'),
        (1900301009, 'admin:temp-permission:query', 'admin temp-permission query', 'GET', '/admin/temp-permission/list', 'admin temp-permission query'),
        (1900301010, 'admin:temp-permission:revoke', 'admin temp-permission revoke', 'DELETE', '/admin/temp-permission/revoke', 'admin temp-permission revoke'),
        (1900301011, 'admin:temp-permission:stats', 'admin temp-permission stats', 'GET', '/admin/temp-permission/statistics', 'admin temp-permission stats'),
        (1900301012, 'auth:user:info', 'auth user info', 'GET', '/admin/logs/operation/my', 'auth user info'),
        (1900301013, 'system:config:add', 'system config add', 'POST', '/config', 'system config add'),
        (1900301014, 'system:config:apply', 'system config apply', 'POST', '/config/apply-pending', 'system config apply'),
        (1900301015, 'system:config:delete', 'system config delete', 'POST', '/config/delete', 'system config delete'),
        (1900301016, 'system:config:edit', 'system config edit', 'POST', '/config/update', 'system config edit'),
        (1900301017, 'system:config:group:add', 'system config group add', 'POST', '/config-groups', 'system config group add'),
        (1900301018, 'system:config:group:delete', 'system config group delete', 'POST', '/config-groups/delete', 'system config group delete'),
        (1900301019, 'system:config:group:edit', 'system config group edit', 'POST', '/config-groups/update', 'system config group edit'),
        (1900301020, 'system:config:group:query', 'system config group query', 'GET', '/config-groups/page', 'system config group query'),
        (1900301021, 'system:config:query', 'system config query', 'GET', '/config/:id', 'system config query'),
        (1900301022, 'system:config:rollback', 'system config rollback', 'POST', '/config/rollback', 'system config rollback'),
        (1900301023, 'system:config:sensitive', 'system config sensitive', 'POST', '/config/:id/sensitive/reveal', 'system config sensitive'),
        (1900301024, 'system:dept:add', 'system dept add', 'POST', '/system/dept', 'system dept add'),
        (1900301025, 'system:dept:edit', 'system dept edit', 'PUT', '/system/dept', 'system dept edit'),
        (1900301026, 'system:dept:query', 'system dept query', 'GET', '/system/dept/:deptId/children', 'system dept query'),
        (1900301027, 'system:dept:remove', 'system dept remove', 'DELETE', '/system/dept/:id', 'system dept remove'),
        (1900301028, 'system:dept:view', 'system dept view', 'GET', '/system/dept/tree/enabled', 'system dept view'),
        (1900301029, 'system:dict:add', 'system dict add', 'POST', '/dict-type/add', 'system dict add'),
        (1900301030, 'system:dict:delete', 'system dict delete', 'POST', '/dict-type/delete', 'system dict delete'),
        (1900301031, 'system:dict:edit', 'system dict edit', 'POST', '/dict-type/update', 'system dict edit'),
        (1900301032, 'system:dict:query', 'system dict query', 'GET', '/dict-type/:id', 'system dict query'),
        (1900301033, 'system:file-task:list', 'system file-task list', 'GET', '/file-process-task', 'system file-task list'),
        (1900301034, 'system:file-task:retry', 'system file-task retry', 'POST', '/file-process-task/:id/retry', 'system file-task retry'),
        (1900301035, 'system:file:delete', 'system file delete', 'POST', '/file-manage/:id/delete', 'system file delete'),
        (1900301036, 'system:file:edit', 'system file edit', 'POST', '/file-manage/references/:id/access-level', 'system file edit'),
        (1900301037, 'system:file:list', 'system file list', 'GET', '/file-manage/list', 'system file list'),
        (1900301038, 'system:file:query', 'system file query', 'GET', '/file-manage/:id', 'system file query'),
        (1900301039, 'system:post:role', 'system post role', 'GET', '/system/post/:postId/roles', 'system post role'),
        (1900301040, 'system:storage:add', 'system storage add', 'POST', '/storage-strategy', 'system storage add'),
        (1900301041, 'system:storage:delete', 'system storage delete', 'POST', '/storage-strategy/:id/delete', 'system storage delete'),
        (1900301042, 'system:storage:edit', 'system storage edit', 'POST', '/storage-strategy/update', 'system storage edit'),
        (1900301043, 'system:storage:list', 'system storage list', 'GET', '/storage-strategy', 'system storage list'),
        (1900301044, 'system:user:query', 'system user query', 'GET', '/user/get/:id', 'system user query'),
        (1900301045, 'system:user:status', 'system user status', 'POST', '/user/status/:id', 'system user status')
      ) AS v(id, code, name, method, path, description)
      WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = v.code AND existing."isDeleted" = 0)
    $sql$;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL AND to_regclass('public.sys_role') IS NOT NULL AND to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
      SELECT 190030100000 + r.id + p.id, r.id, p.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM sys_role r
      JOIN sys_permission p ON p."isDeleted" = 0 AND p.id BETWEEN 1900301001 AND 1900301045
      WHERE r."isDeleted" = 0 AND r.code = 'SUPER_ADMIN'
        AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing."roleId" = r.id AND existing."permissionId" = p.id)
    $sql$;
  END IF;
END
$route_permissions$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $route_permissions$
BEGIN
  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    EXECUTE 'DELETE FROM sys_role_permission WHERE "permissionId" BETWEEN 1900301001 AND 1900301045';
  END IF;
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE 'DELETE FROM sys_permission WHERE id BETWEEN 1900301001 AND 1900301045';
  END IF;
END
$route_permissions$;
-- +goose StatementEnd
