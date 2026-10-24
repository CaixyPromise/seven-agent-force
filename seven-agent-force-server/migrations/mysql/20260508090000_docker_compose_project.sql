-- +goose Up
SET @hasTargetId := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'docker_operation'
    AND COLUMN_NAME = 'targetId'
);
SET @sql := IF(
  @hasTargetId = 0,
  'ALTER TABLE docker_operation ADD COLUMN targetId VARCHAR(128) DEFAULT NULL COMMENT ''目标稳定ID'' AFTER targetType',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @hasTargetIndex := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'docker_operation'
    AND INDEX_NAME = 'idx_docker_operation_target'
);
SET @sql := IF(
  @hasTargetIndex = 0,
  'ALTER TABLE docker_operation ADD KEY idx_docker_operation_target (targetType, targetId, targetName, updateTime)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS docker_compose_project (
  id BIGINT NOT NULL COMMENT 'Compose项目内部ID',
  projectId VARCHAR(128) NOT NULL COMMENT 'Compose项目稳定ID',
  projectName VARCHAR(128) NOT NULL COMMENT 'Docker Compose项目名',
  workingDir VARCHAR(1024) DEFAULT NULL COMMENT '工作目录',
  configFilesJson TEXT DEFAULT NULL COMMENT '配置文件列表JSON',
  composeYaml MEDIUMTEXT DEFAULT NULL COMMENT 'Compose YAML',
  description VARCHAR(512) DEFAULT NULL COMMENT '描述',
  status VARCHAR(32) NOT NULL DEFAULT 'unknown' COMMENT '项目状态',
  lastPreviewJson MEDIUMTEXT DEFAULT NULL COMMENT '最近一次策略预览JSON',
  lastValidationJson MEDIUMTEXT DEFAULT NULL COMMENT '最近一次校验结果JSON',
  lastOperationId BIGINT DEFAULT NULL COMMENT '最近一次操作ID',
  source VARCHAR(32) NOT NULL DEFAULT 'MANAGED' COMMENT '来源：MANAGED/DISCOVERED',
  createdBy BIGINT DEFAULT NULL COMMENT '创建人',
  deleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_docker_compose_project_id_deleted (projectId, deleted),
  UNIQUE KEY uk_docker_compose_project_name_deleted (projectName, deleted),
  KEY idx_docker_compose_project_status (status, deleted),
  KEY idx_docker_compose_project_operation (lastOperationId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Docker Compose项目';

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100033, 'admin:docker:compose:project:list', 'Docker Compose项目列表', 'API', 'GET', '/admin/docker/compose/projects', 0, '查询Docker Compose项目列表', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:project:list');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100034, 'admin:docker:compose:project:query', 'Docker Compose项目详情', 'API', 'GET', '/admin/docker/compose/projects/{id}', 0, '查询Docker Compose项目详情', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:project:query');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100035, 'admin:docker:compose:project:save', '保存Docker Compose项目', 'API', 'POST', '/admin/docker/compose/projects', 0, '创建或更新Docker Compose项目', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:project:save');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100039, 'admin:docker:compose:project:create', '创建Docker Compose项目', 'API', 'POST', '/admin/docker/compose/projects', 0, '创建Docker Compose项目', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:project:create');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100040, 'admin:docker:compose:project:update', '更新Docker Compose项目', 'API', 'PUT', '/admin/docker/compose/projects/{id}/compose', 0, '更新Docker Compose项目配置', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:project:update');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100036, 'admin:docker:compose:workspace:check', '检查Docker Compose工作目录', 'API', 'POST', '/admin/docker/compose/workspace/check', 0, '检查Docker Compose工作目录', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:workspace:check');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100037, 'admin:docker:compose:yaml:validate', '校验Docker Compose YAML', 'API', 'POST', '/admin/docker/compose/yaml/validate', 0, '校验Docker Compose YAML并返回解析结果', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:yaml:validate');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100038, 'admin:docker:compose:dockerfile:preview', '预览Dockerfile构建', 'API', 'POST', '/admin/docker/compose/dockerfile/preview', 0, '预览Dockerfile构建配置', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:dockerfile:preview');

INSERT IGNORE INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT 190030000000 + r.id + p.id, r.id, p.id, 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON (p.code LIKE 'admin:docker:compose:project:%' OR p.code IN ('admin:docker:compose:workspace:check', 'admin:docker:compose:yaml:validate', 'admin:docker:compose:dockerfile:preview')) AND p.isDeleted = 0
WHERE r.isDeleted = 0 AND r.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN');

-- +goose Down
DELETE FROM sys_role_permission WHERE permissionId IN (SELECT id FROM sys_permission WHERE code LIKE 'admin:docker:compose:project:%' OR code IN ('admin:docker:compose:workspace:check', 'admin:docker:compose:yaml:validate', 'admin:docker:compose:dockerfile:preview'));
DELETE FROM sys_permission WHERE code LIKE 'admin:docker:compose:project:%' OR code IN ('admin:docker:compose:workspace:check', 'admin:docker:compose:yaml:validate', 'admin:docker:compose:dockerfile:preview');
DROP TABLE IF EXISTS docker_compose_project;
ALTER TABLE docker_operation DROP KEY idx_docker_operation_target;
ALTER TABLE docker_operation DROP COLUMN targetId;
