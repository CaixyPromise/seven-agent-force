-- +goose Up
SET @hasComposeFilePath := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'docker_compose_project'
    AND COLUMN_NAME = 'composeFilePath'
);
SET @sql := IF(
  @hasComposeFilePath = 0,
  'ALTER TABLE docker_compose_project ADD COLUMN composeFilePath VARCHAR(1024) DEFAULT NULL COMMENT ''实际Compose文件路径'' AFTER composeYaml',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @hasFileManifestJson := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'docker_compose_project'
    AND COLUMN_NAME = 'fileManifestJson'
);
SET @sql := IF(
  @hasFileManifestJson = 0,
  'ALTER TABLE docker_compose_project ADD COLUMN fileManifestJson MEDIUMTEXT DEFAULT NULL COMMENT ''项目文件清单JSON'' AFTER composeFilePath',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100036, 'admin:docker:compose:workspace:check', '检查Docker Compose工作目录', 'API', 'POST', '/admin/docker/compose/workspace/check', 0, '检查Docker Compose工作目录', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:workspace:check');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100037, 'admin:docker:compose:yaml:validate', '校验Docker Compose YAML', 'API', 'POST', '/admin/docker/compose/yaml/validate', 0, '校验Docker Compose YAML并返回解析结果', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:yaml:validate');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100038, 'admin:docker:compose:dockerfile:preview', '预览Dockerfile构建', 'API', 'POST', '/admin/docker/compose/dockerfile/preview', 0, '预览Dockerfile构建配置', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:dockerfile:preview');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100039, 'admin:docker:compose:project:create', '创建Docker Compose项目', 'API', 'POST', '/admin/docker/compose/projects', 0, '创建Docker Compose项目', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:project:create');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100040, 'admin:docker:compose:project:update', '更新Docker Compose项目', 'API', 'PUT', '/admin/docker/compose/projects/{id}/compose', 0, '更新Docker Compose项目配置', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:project:update');

INSERT IGNORE INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT 190030000000 + r.id + p.id, r.id, p.id, 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.code IN ('admin:docker:compose:workspace:check', 'admin:docker:compose:yaml:validate', 'admin:docker:compose:dockerfile:preview', 'admin:docker:compose:project:create', 'admin:docker:compose:project:update') AND p.isDeleted = 0
WHERE r.isDeleted = 0 AND r.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN');

-- +goose Down
DELETE FROM sys_role_permission WHERE permissionId IN (SELECT id FROM sys_permission WHERE code IN ('admin:docker:compose:workspace:check', 'admin:docker:compose:yaml:validate', 'admin:docker:compose:dockerfile:preview', 'admin:docker:compose:project:create', 'admin:docker:compose:project:update'));
DELETE FROM sys_permission WHERE code IN ('admin:docker:compose:workspace:check', 'admin:docker:compose:yaml:validate', 'admin:docker:compose:dockerfile:preview', 'admin:docker:compose:project:create', 'admin:docker:compose:project:update');
ALTER TABLE docker_compose_project
  DROP COLUMN fileManifestJson,
  DROP COLUMN composeFilePath;
