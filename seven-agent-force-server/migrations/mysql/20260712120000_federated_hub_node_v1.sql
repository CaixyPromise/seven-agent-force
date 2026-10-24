-- +goose Up
DROP PROCEDURE IF EXISTS task7FederatedIdentityPreflight;
-- +goose StatementBegin
CREATE PROCEDURE task7FederatedIdentityPreflight()
BEGIN
  IF EXISTS (
    SELECT 1
    FROM sysExternalUserIdentity i
    JOIN sysExternalLoginProvider p ON p.providerCode = i.providerCode AND p.isDeleted = 0
    WHERE i.isDeleted = 0 AND p.protocolType = 'OIDC'
      AND (
        p.issuer IS NULL OR TRIM(p.issuer) = '' OR OCTET_LENGTH(TRIM(p.issuer)) > 512
        OR NOT REGEXP_LIKE(TRIM(p.issuer), '^https?://[^/?#@[:space:]]+(/[^?#[:space:]]*)?$', 'c')
      )
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Task 7 unsafe legacy OIDC issuer; repair provider issuer before retry';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM sysExternalUserIdentity i
    JOIN sysExternalLoginProvider p ON p.providerCode = i.providerCode AND p.isDeleted = 0
    WHERE i.isDeleted = 0 AND p.protocolType = 'OIDC'
    GROUP BY TRIM(p.issuer), i.externalSubject
    HAVING COUNT(*) > 1
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Task 7 duplicate legacy OIDC issuer and subject; resolve identities before retry';
  END IF;
END;
-- +goose StatementEnd
CALL task7FederatedIdentityPreflight();
DROP PROCEDURE IF EXISTS task7FederatedIdentityPreflight;

SET @task7SQL = IF(
  EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND COLUMN_NAME = 'externalIssuer'),
  'SELECT 1',
  'ALTER TABLE sysExternalUserIdentity ADD COLUMN externalIssuer VARCHAR(512) NULL COMMENT ''经验证的OIDC issuer'' AFTER providerCode'
);
PREPARE task7Statement FROM @task7SQL;
EXECUTE task7Statement;
DEALLOCATE PREPARE task7Statement;

SET @task7SQL = IF(
  EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND COLUMN_NAME = 'externalIssuerDigest'),
  'SELECT 1',
  'ALTER TABLE sysExternalUserIdentity ADD COLUMN externalIssuerDigest BINARY(32) GENERATED ALWAYS AS (UNHEX(SHA2(externalIssuer, 256))) STORED AFTER externalIssuer'
);
PREPARE task7Statement FROM @task7SQL;
EXECUTE task7Statement;
DEALLOCATE PREPARE task7Statement;

UPDATE sysExternalUserIdentity i
JOIN sysExternalLoginProvider p ON p.providerCode = i.providerCode AND p.isDeleted = 0
SET i.externalIssuer = TRIM(p.issuer)
WHERE i.isDeleted = 0 AND p.protocolType = 'OIDC' AND p.issuer IS NOT NULL AND TRIM(p.issuer) <> '' AND i.externalIssuer IS NULL;

SET @task7SQL = IF(
  EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND INDEX_NAME = 'uk_sysExternalUserIdentity_issuer_subject_deleted'),
  'SELECT 1',
  'ALTER TABLE sysExternalUserIdentity ADD UNIQUE KEY uk_sysExternalUserIdentity_issuer_subject_deleted (externalIssuerDigest, externalSubject, isDeleted)'
);
PREPARE task7Statement FROM @task7SQL;
EXECUTE task7Statement;
DEALLOCATE PREPARE task7Statement;

CREATE TABLE IF NOT EXISTS sysExternalManagedProviderCommand (
  providerCode VARCHAR(64) NOT NULL COMMENT '系统托管Provider编码',
  connectionVersion VARCHAR(128) NOT NULL COMMENT '连接版本',
  requestHash CHAR(64) NOT NULL COMMENT '完整配置请求摘要',
  createdAt DATETIME(6) NOT NULL COMMENT '首次应用时间',
  PRIMARY KEY (providerCode, connectionVersion)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Node系统托管OIDC Provider命令账本';

CREATE TABLE IF NOT EXISTS sysFederatedNode (
  id BIGINT NOT NULL COMMENT '主键',
  nodeCode VARCHAR(64) NOT NULL COMMENT '稳定节点编码',
  nodeName VARCHAR(128) NOT NULL COMMENT '节点名称',
  status TINYINT NOT NULL DEFAULT 1 COMMENT '状态:0启用,1禁用',
  discoveryType VARCHAR(16) NOT NULL COMMENT '发现类型:STATIC,CONSUL',
  serviceName VARCHAR(128) NULL COMMENT 'Consul服务名称',
  managementBaseUrl VARCHAR(2048) NULL COMMENT '静态管理地址',
  hubIssuer VARCHAR(512) NOT NULL COMMENT 'Hub公开OIDC issuer',
  oidcClientId VARCHAR(128) NULL COMMENT '系统托管OIDC客户端ID',
  oidcClientSecretCiphertext TEXT NULL COMMENT 'OIDC客户端密钥密文',
  oidcClientSecretEdek TEXT NULL COMMENT 'OIDC客户端密钥封装DEK',
  oidcClientSecretWrapKeyRef VARCHAR(128) NULL COMMENT 'OIDC客户端密钥包装密钥引用',
  managementBearerCiphertext TEXT NULL COMMENT '管理Bearer密文',
  managementBearerEdek TEXT NULL COMMENT '管理Bearer封装DEK',
  managementBearerWrapKeyRef VARCHAR(128) NULL COMMENT '管理Bearer包装密钥引用',
  capabilitiesJson TEXT NULL COMMENT '安全能力快照JSON',
  connectionStatus VARCHAR(16) NOT NULL DEFAULT 'PENDING' COMMENT '连接状态:PENDING,ACTIVE,ERROR',
  connectionVersion VARCHAR(128) NULL COMMENT '稳定连接版本',
  connectionRequestHash CHAR(64) NULL COMMENT '连接重放请求哈希',
  targetRevision BIGINT NOT NULL DEFAULT 1 COMMENT '路由与管理凭证单调修订号',
  issuerLockedAt DATETIME(6) NULL COMMENT '首次激活时间及issuer永久锁标记',
  lastConnectionError VARCHAR(512) NULL COMMENT '脱敏连接错误',
  lastConnectionTraceId VARCHAR(128) NULL COMMENT '远端追踪ID',
  lastHealthyAt DATETIME(6) NULL COMMENT '最近健康时间',
  createdAt DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '创建时间',
  updatedAt DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) COMMENT '更新时间',
  isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '软删除标记',
  activeKey TINYINT GENERATED ALWAYS AS (CASE WHEN isDeleted = 0 THEN 0 ELSE NULL END) STORED,
  PRIMARY KEY (id),
  UNIQUE KEY uk_sysFederatedNode_nodeCode_active (nodeCode, activeKey),
  KEY idx_sysFederatedNode_status_updatedAt (status, updatedAt),
  KEY idx_sysFederatedNode_connectionStatus (connectionStatus)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Hub联邦节点注册表';

CREATE TABLE IF NOT EXISTS sysFederatedNodeConnectionCommand (
  nodeCode VARCHAR(64) NOT NULL COMMENT '稳定节点编码',
  connectionVersion VARCHAR(128) NOT NULL COMMENT '客户端提供的稳定连接版本',
  requestHash CHAR(64) NOT NULL COMMENT '连接请求哈希',
  targetRevision BIGINT NOT NULL COMMENT '接受命令时的目标修订号',
  terminalState VARCHAR(16) NOT NULL COMMENT '命令状态:PENDING,ACTIVE,ERROR,SUPERSEDED',
  createdAt DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '创建时间',
  updatedAt DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) COMMENT '更新时间',
  PRIMARY KEY (nodeCode, connectionVersion),
  KEY idx_sysFederatedNodeConnectionCommand_state_updatedAt (terminalState, updatedAt)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Hub节点连接命令元数据账本';

INSERT INTO sysFederatedNodeConnectionCommand (nodeCode, connectionVersion, requestHash, targetRevision, terminalState, createdAt, updatedAt)
SELECT nodeCode, connectionVersion, COALESCE(connectionRequestHash, REPEAT('0', 64)), targetRevision,
       CASE connectionStatus WHEN 'ACTIVE' THEN 'ACTIVE' WHEN 'ERROR' THEN 'ERROR' ELSE 'PENDING' END,
       createdAt, updatedAt
FROM sysFederatedNode
WHERE isDeleted = 0 AND connectionVersion IS NOT NULL AND connectionVersion <> ''
ON DUPLICATE KEY UPDATE updatedAt = sysFederatedNodeConnectionCommand.updatedAt;

-- +goose Down
DROP PROCEDURE IF EXISTS task7FederatedIdentityPreflight;
DROP TABLE IF EXISTS sysFederatedNodeConnectionCommand;
DROP TABLE IF EXISTS sysFederatedNode;
DROP TABLE IF EXISTS sysExternalManagedProviderCommand;

SET @task7SQL = IF(
  EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND INDEX_NAME = 'uk_sysExternalUserIdentity_issuer_subject_deleted'),
  'ALTER TABLE sysExternalUserIdentity DROP INDEX uk_sysExternalUserIdentity_issuer_subject_deleted',
  'SELECT 1'
);
PREPARE task7Statement FROM @task7SQL;
EXECUTE task7Statement;
DEALLOCATE PREPARE task7Statement;

SET @task7SQL = IF(
  EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND COLUMN_NAME = 'externalIssuerDigest'),
  'ALTER TABLE sysExternalUserIdentity DROP COLUMN externalIssuerDigest',
  'SELECT 1'
);
PREPARE task7Statement FROM @task7SQL;
EXECUTE task7Statement;
DEALLOCATE PREPARE task7Statement;

SET @task7SQL = IF(
  EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND COLUMN_NAME = 'externalIssuer'),
  'ALTER TABLE sysExternalUserIdentity DROP COLUMN externalIssuer',
  'SELECT 1'
);
PREPARE task7Statement FROM @task7SQL;
EXECUTE task7Statement;
DEALLOCATE PREPARE task7Statement;
