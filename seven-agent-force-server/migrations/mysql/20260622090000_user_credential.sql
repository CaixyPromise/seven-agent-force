-- +goose Up
CREATE TABLE IF NOT EXISTS sys_user_credential (
  id BIGINT NOT NULL COMMENT '凭证ID',
  userId BIGINT NOT NULL COMMENT '用户ID',
  credentialType VARCHAR(32) NOT NULL COMMENT '凭证类型：PASSWORD/TOTP/PASSKEY/RECOVERY_CODE',
  credentialKey VARCHAR(255) NOT NULL COMMENT '凭证键，同一类型下用于区分具体凭证',
  secretHash VARCHAR(255) DEFAULT NULL COMMENT '密钥哈希，密码和恢复码使用',
  secretCiphertext TEXT DEFAULT NULL COMMENT '密钥密文，TOTP等加密材料使用',
  credentialPayloadJson TEXT DEFAULT NULL COMMENT '凭证扩展载荷JSON',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0启用 1禁用 2已消费 3已失效',
  verifiedAt DATETIME DEFAULT NULL COMMENT '验证时间',
  lastUsedAt DATETIME DEFAULT NULL COMMENT '最后使用时间',
  invalidatedAt DATETIME DEFAULT NULL COMMENT '失效时间',
  metadataJson TEXT DEFAULT NULL COMMENT '扩展元数据JSON',
  mustChangePassword TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否必须修改密码',
  passwordChangedAt DATETIME DEFAULT NULL COMMENT '密码修改时间',
  creatorId BIGINT DEFAULT NULL COMMENT '创建人ID',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updaterId BIGINT DEFAULT NULL COMMENT '更新人ID',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  isDeleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_credential_scope (userId, credentialType, credentialKey, isDeleted),
  UNIQUE KEY uk_credential_type_key (credentialType, credentialKey, isDeleted),
  KEY idx_user_credential_user_type_status (userId, credentialType, status, isDeleted),
  KEY idx_user_credential_status (status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户凭证表';

-- +goose Down
DROP TABLE IF EXISTS sys_user_credential;
