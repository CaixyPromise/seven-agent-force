-- +goose Up
ALTER TABLE sys_role
  ADD COLUMN grantRevision BIGINT NOT NULL DEFAULT 0 COMMENT '角色授权快照修订号' AFTER dataScope;

CREATE TABLE sys_role_grant_request (
  roleId BIGINT NOT NULL COMMENT '角色ID',
  idempotencyKey VARCHAR(128) NOT NULL COMMENT '客户端幂等键',
  requestHash CHAR(64) NOT NULL COMMENT '规范化请求摘要',
  resultRevision BIGINT NOT NULL COMMENT '提交后的授权修订号',
  impactedUserCount INT NOT NULL DEFAULT 0 COMMENT '受影响用户数',
  changed TINYINT NOT NULL DEFAULT 0 COMMENT '是否产生授权变化',
  operatorId BIGINT NULL COMMENT '操作用户ID',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (roleId, idempotencyKey),
  KEY idx_sys_role_grant_request_create_time (createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色授权原子提交幂等记录';

-- +goose Down
DROP TABLE IF EXISTS sys_role_grant_request;

ALTER TABLE sys_role DROP COLUMN grantRevision;
