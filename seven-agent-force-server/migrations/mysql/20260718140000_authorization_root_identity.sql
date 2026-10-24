-- +goose Up
ALTER TABLE sys_role
  ADD COLUMN systemKey VARCHAR(64) NULL COMMENT '稳定系统角色标识' AFTER code,
  ADD UNIQUE KEY uk_sys_role_system_key (systemKey);

UPDATE sys_role
SET systemKey = 'AUTHORIZATION_ROOT',
    updateTime = NOW()
WHERE code = 'SUPER_ADMIN' AND isDeleted = 0;

CREATE TEMPORARY TABLE authorization_root_migration_guard (
  rootCount INT NOT NULL,
  rootPostRelationCount INT NOT NULL,
  CONSTRAINT RBAC_ROOT_INVALID_COUNT CHECK (rootCount = 1),
  CONSTRAINT RBAC_ROOT_POST_RELATION_EXISTS CHECK (rootPostRelationCount = 0)
);

INSERT INTO authorization_root_migration_guard (rootCount, rootPostRelationCount)
SELECT
  (SELECT COUNT(1) FROM sys_role WHERE systemKey = 'AUTHORIZATION_ROOT' AND isDeleted = 0),
  (SELECT COUNT(1)
   FROM sys_post_role spr
   JOIN sys_role sr ON sr.id = spr.roleId
   WHERE sr.systemKey = 'AUTHORIZATION_ROOT');

DROP TEMPORARY TABLE authorization_root_migration_guard;

CREATE TABLE sys_security_bootstrap (
  bootstrapKey VARCHAR(64) NOT NULL COMMENT '初始化记录键',
  rootRoleId BIGINT NOT NULL COMMENT '授权安全根角色ID',
  rootRoleCode VARCHAR(50) NOT NULL COMMENT '初始化时固化的外部角色编码',
  initializedAt DATETIME NOT NULL COMMENT '初始化时间',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (bootstrapKey),
  UNIQUE KEY uk_sys_security_bootstrap_root_role (rootRoleId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='安全根初始化记录';

INSERT INTO sys_security_bootstrap (bootstrapKey, rootRoleId, rootRoleCode, initializedAt, createTime, updateTime)
SELECT 'AUTHORIZATION_ROOT', sr.id, sr.code, NOW(), NOW(), NOW()
FROM sys_role sr
WHERE sr.systemKey = 'AUTHORIZATION_ROOT'
  AND sr.isDeleted = 0
  AND EXISTS (SELECT 1 FROM sys_user su WHERE su.isDeleted = 0)
ON DUPLICATE KEY UPDATE bootstrapKey = VALUES(bootstrapKey);

-- +goose Down
DROP TABLE IF EXISTS sys_security_bootstrap;

ALTER TABLE sys_role
  DROP INDEX uk_sys_role_system_key,
  DROP COLUMN systemKey;
