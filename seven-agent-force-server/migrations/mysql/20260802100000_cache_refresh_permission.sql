-- +goose Up
-- Protected DG6.3 action only: it refreshes reviewed application caches via
-- a durable V3 outbox protocol and is not a generic Redis/cache clear API.
INSERT IGNORE INTO sys_permission (
  id, code, name, resourceType, method, path, status, description,
  creatorId, createTime, updaterId, updateTime, isDeleted
) VALUES
  (1900301068, 'system:cache:refresh', '刷新应用缓存', 'API', 'POST', '/system/cache/refresh', 0, '提交受治理的应用缓存刷新操作', 0, NOW(), 0, NOW(), 0);

INSERT IGNORE INTO sys_role_permission (
  id, roleId, permissionId, source, creatorId, createTime, updateTime
)
SELECT 190030106800 + r.id + p.id, r.id, p.id, 'DIRECT', 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.id = 1900301068 AND p.isDeleted = 0
WHERE r.systemKey = 'AUTHORIZATION_ROOT' AND r.isDeleted = 0;

-- +goose Down
-- This seed uses INSERT IGNORE and cannot know whether the permission or a
-- role grant existed before this migration. Deleting by permission ID would
-- therefore remove a legitimate operator-managed grant. Keep the migration
-- forward-only and require an explicit, audited operational removal instead.
SIGNAL SQLSTATE '45000'
  SET MESSAGE_TEXT = 'forward-only migration: cache refresh permission must not be removed automatically';
