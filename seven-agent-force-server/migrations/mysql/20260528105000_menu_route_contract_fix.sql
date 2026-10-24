-- +goose Up
-- Align RBAC menu routes with current system-ui routes.
UPDATE sys_menu
SET path = '/system/organization', updateTime = NOW()
WHERE name = '组织管理' AND path = '/system/org' AND isDeleted = 0;

UPDATE sys_menu
SET path = '/system/department', updateTime = NOW()
WHERE name = '部门管理' AND path = '/system/dept' AND isDeleted = 0;

UPDATE sys_menu
SET path = '/system/operation-log', permission = 'admin:log:view', updateTime = NOW()
WHERE name = '操作日志' AND path = '/system/log' AND isDeleted = 0;

UPDATE sys_menu
SET path = '/system/config', permission = 'system:config:query', updateTime = NOW()
WHERE name = '配置管理' AND (path IS NULL OR path = '') AND isDeleted = 0;
