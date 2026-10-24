-- +goose Up
CREATE TABLE IF NOT EXISTS sys_role_config_scope (
  id BIGINT NOT NULL COMMENT '主键ID',
  roleId BIGINT NOT NULL COMMENT '角色ID',
  groupCode VARCHAR(64) NOT NULL COMMENT '配置分组编码',
  configKey VARCHAR(128) NOT NULL DEFAULT '' COMMENT '配置键，空表示整个分组',
  canRead TINYINT(1) NOT NULL DEFAULT 1 COMMENT '可读',
  canWrite TINYINT(1) NOT NULL DEFAULT 0 COMMENT '可写',
  canDelete TINYINT(1) NOT NULL DEFAULT 0 COMMENT '可删除',
  createdBy BIGINT DEFAULT NULL COMMENT '创建人ID',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updatedBy BIGINT DEFAULT NULL COMMENT '更新人ID',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  isDeleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  PRIMARY KEY (id),
  UNIQUE KEY uk_role_config_scope (roleId, groupCode, configKey),
  KEY idx_role_config_scope_role (roleId, isDeleted),
  KEY idx_role_config_scope_group (groupCode, configKey, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色配置访问范围表';

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT item.id, item.code, item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1900301060 AS id, 'system:config:scope:query' AS code, 'system config scope query' AS name, 'GET' AS method, '/config-scopes/roles/:roleId' AS path, 'system config scope query' AS description
  UNION ALL SELECT 1900301061, 'system:config:scope:assign', 'system config scope assign', 'POST', '/config-scopes/roles/:roleId', 'system config scope assign'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code AND existing.isDeleted = 0);

INSERT IGNORE INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT 190030106000 + r.id + p.id, r.id, p.id, 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.isDeleted = 0 AND p.id BETWEEN 1900301060 AND 1900301061
WHERE r.isDeleted = 0 AND r.code = 'SUPER_ADMIN';

-- +goose Down
DELETE FROM sys_role_permission
WHERE permissionId IN (SELECT id FROM sys_permission WHERE id BETWEEN 1900301060 AND 1900301061);

DELETE FROM sys_permission WHERE id BETWEEN 1900301060 AND 1900301061;

DROP TABLE IF EXISTS sys_role_config_scope;
