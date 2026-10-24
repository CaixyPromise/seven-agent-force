-- +goose Up
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT item.id, item.code, item.name, item.resourceType, item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1900301001 AS id, 'admin:log:clean' AS code, 'admin log clean' AS name, 'API' AS resourceType, 'POST' AS method, '/admin/logs/operation/clean' AS path, 'admin log clean' AS description
  UNION ALL SELECT 1900301002, 'admin:log:delete', 'admin log delete', 'API', 'POST', '/admin/logs/operation/deleteByTimeRange', 'admin log delete'
  UNION ALL SELECT 1900301003, 'admin:log:export', 'admin log export', 'API', 'GET', '/admin/logs/operation/export', 'admin log export'
  UNION ALL SELECT 1900301004, 'admin:log:view', 'admin log view', 'API', 'GET', '/admin/logs/operation', 'admin log view'
  UNION ALL SELECT 1900301005, 'admin:online:kick', 'admin online kick', 'API', 'POST', '/admin/kick/:userId', 'admin online kick'
  UNION ALL SELECT 1900301006, 'admin:temp-permission:cleanup', 'admin temp-permission cleanup', 'API', 'POST', '/admin/temp-permission/cleanup', 'admin temp-permission cleanup'
  UNION ALL SELECT 1900301007, 'admin:temp-permission:extend', 'admin temp-permission extend', 'API', 'PUT', '/admin/temp-permission/extend', 'admin temp-permission extend'
  UNION ALL SELECT 1900301008, 'admin:temp-permission:grant', 'admin temp-permission grant', 'API', 'POST', '/admin/temp-permission/grant', 'admin temp-permission grant'
  UNION ALL SELECT 1900301009, 'admin:temp-permission:query', 'admin temp-permission query', 'API', 'GET', '/admin/temp-permission/list', 'admin temp-permission query'
  UNION ALL SELECT 1900301010, 'admin:temp-permission:revoke', 'admin temp-permission revoke', 'API', 'DELETE', '/admin/temp-permission/revoke', 'admin temp-permission revoke'
  UNION ALL SELECT 1900301011, 'admin:temp-permission:stats', 'admin temp-permission stats', 'API', 'GET', '/admin/temp-permission/statistics', 'admin temp-permission stats'
  UNION ALL SELECT 1900301012, 'auth:user:info', 'auth user info', 'API', 'GET', '/admin/logs/operation/my', 'auth user info'
  UNION ALL SELECT 1900301013, 'system:config:add', 'system config add', 'API', 'POST', '/config', 'system config add'
  UNION ALL SELECT 1900301014, 'system:config:apply', 'system config apply', 'API', 'POST', '/config/apply-pending', 'system config apply'
  UNION ALL SELECT 1900301015, 'system:config:delete', 'system config delete', 'API', 'POST', '/config/delete', 'system config delete'
  UNION ALL SELECT 1900301016, 'system:config:edit', 'system config edit', 'API', 'POST', '/config/update', 'system config edit'
  UNION ALL SELECT 1900301017, 'system:config:group:add', 'system config group add', 'API', 'POST', '/config-groups', 'system config group add'
  UNION ALL SELECT 1900301018, 'system:config:group:delete', 'system config group delete', 'API', 'POST', '/config-groups/delete', 'system config group delete'
  UNION ALL SELECT 1900301019, 'system:config:group:edit', 'system config group edit', 'API', 'POST', '/config-groups/update', 'system config group edit'
  UNION ALL SELECT 1900301020, 'system:config:group:query', 'system config group query', 'API', 'GET', '/config-groups/page', 'system config group query'
  UNION ALL SELECT 1900301021, 'system:config:query', 'system config query', 'API', 'GET', '/config/:id', 'system config query'
  UNION ALL SELECT 1900301022, 'system:config:rollback', 'system config rollback', 'API', 'POST', '/config/rollback', 'system config rollback'
  UNION ALL SELECT 1900301023, 'system:config:sensitive', 'system config sensitive', 'API', 'POST', '/config/:id/sensitive/reveal', 'system config sensitive'
  UNION ALL SELECT 1900301024, 'system:dept:add', 'system dept add', 'API', 'POST', '/system/dept', 'system dept add'
  UNION ALL SELECT 1900301025, 'system:dept:edit', 'system dept edit', 'API', 'PUT', '/system/dept', 'system dept edit'
  UNION ALL SELECT 1900301026, 'system:dept:query', 'system dept query', 'API', 'GET', '/system/dept/:deptId/children', 'system dept query'
  UNION ALL SELECT 1900301027, 'system:dept:remove', 'system dept remove', 'API', 'DELETE', '/system/dept/:id', 'system dept remove'
  UNION ALL SELECT 1900301028, 'system:dept:view', 'system dept view', 'API', 'GET', '/system/dept/tree/enabled', 'system dept view'
  UNION ALL SELECT 1900301029, 'system:dict:add', 'system dict add', 'API', 'POST', '/dict-type/add', 'system dict add'
  UNION ALL SELECT 1900301030, 'system:dict:delete', 'system dict delete', 'API', 'POST', '/dict-type/delete', 'system dict delete'
  UNION ALL SELECT 1900301031, 'system:dict:edit', 'system dict edit', 'API', 'POST', '/dict-type/update', 'system dict edit'
  UNION ALL SELECT 1900301032, 'system:dict:query', 'system dict query', 'API', 'GET', '/dict-type/:id', 'system dict query'
  UNION ALL SELECT 1900301033, 'system:file-task:list', 'system file-task list', 'API', 'GET', '/file-process-task', 'system file-task list'
  UNION ALL SELECT 1900301034, 'system:file-task:retry', 'system file-task retry', 'API', 'POST', '/file-process-task/:id/retry', 'system file-task retry'
  UNION ALL SELECT 1900301035, 'system:file:delete', 'system file delete', 'API', 'POST', '/file-manage/:id/delete', 'system file delete'
  UNION ALL SELECT 1900301036, 'system:file:edit', 'system file edit', 'API', 'POST', '/file-manage/references/:id/access-level', 'system file edit'
  UNION ALL SELECT 1900301037, 'system:file:list', 'system file list', 'API', 'GET', '/file-manage/list', 'system file list'
  UNION ALL SELECT 1900301038, 'system:file:query', 'system file query', 'API', 'GET', '/file-manage/:id', 'system file query'
  UNION ALL SELECT 1900301039, 'system:post:role', 'system post role', 'API', 'GET', '/system/post/:postId/roles', 'system post role'
  UNION ALL SELECT 1900301040, 'system:storage:add', 'system storage add', 'API', 'POST', '/storage-strategy', 'system storage add'
  UNION ALL SELECT 1900301041, 'system:storage:delete', 'system storage delete', 'API', 'POST', '/storage-strategy/:id/delete', 'system storage delete'
  UNION ALL SELECT 1900301042, 'system:storage:edit', 'system storage edit', 'API', 'POST', '/storage-strategy/update', 'system storage edit'
  UNION ALL SELECT 1900301043, 'system:storage:list', 'system storage list', 'API', 'GET', '/storage-strategy', 'system storage list'
  UNION ALL SELECT 1900301044, 'system:user:query', 'system user query', 'API', 'GET', '/user/get/:id', 'system user query'
  UNION ALL SELECT 1900301045, 'system:user:status', 'system user status', 'API', 'POST', '/user/status/:id', 'system user status'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code AND existing.isDeleted = 0);

INSERT IGNORE INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT 190030100000 + r.id + p.id, r.id, p.id, 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.isDeleted = 0 AND p.id BETWEEN 1900301001 AND 1900301045
WHERE r.isDeleted = 0 AND r.code = 'SUPER_ADMIN';

-- +goose Down
DELETE FROM sys_role_permission
WHERE permissionId IN (SELECT id FROM sys_permission WHERE id BETWEEN 1900301001 AND 1900301045);

DELETE FROM sys_permission WHERE id BETWEEN 1900301001 AND 1900301045;
