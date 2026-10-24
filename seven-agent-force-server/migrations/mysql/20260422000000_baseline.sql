-- +goose Up
CREATE TABLE IF NOT EXISTS sys_storage_strategy (
  id BIGINT NOT NULL COMMENT '策略ID',
  strategyName VARCHAR(64) NOT NULL COMMENT '策略名称',
  providerType VARCHAR(32) NOT NULL COMMENT '存储提供商类型',
  isDefault TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否默认',
  isEnabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用',
  runState VARCHAR(16) NOT NULL DEFAULT 'ACTIVE' COMMENT '运行态',
  priority INT NOT NULL DEFAULT 0 COMMENT '优先级',
  configCiphertext TEXT NOT NULL COMMENT '配置密文',
  configEdek VARCHAR(512) NOT NULL COMMENT '加密DEK',
  wrapKeyRef VARCHAR(64) NOT NULL COMMENT '主密钥引用',
  healthCheckUrl VARCHAR(255) DEFAULT NULL,
  healthStatus TINYINT DEFAULT 1,
  lastHealthCheck DATETIME DEFAULT NULL,
  failureCount INT NOT NULL DEFAULT 0,
  totalRequests INT NOT NULL DEFAULT 0,
  failureRateThreshold DECIMAL(5,2) NOT NULL DEFAULT 10.00,
  windowStartTime DATETIME DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_strategyName (strategyName),
  KEY idx_providerType (providerType),
  KEY idx_isDefault (isDefault),
  KEY idx_runState (runState),
  KEY idx_healthStatus (healthStatus, priority)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='存储策略配置表';

CREATE TABLE IF NOT EXISTS sys_file_info (
  id BIGINT NOT NULL COMMENT '文件ID',
  fileInnerName VARCHAR(255) NOT NULL COMMENT '文件内部名称',
  fileSize BIGINT UNSIGNED NOT NULL COMMENT '文件大小',
  fileSha256 CHAR(64) DEFAULT NULL COMMENT 'SHA256',
  fileCrc32c VARCHAR(16) DEFAULT NULL,
  hashAlgorithm VARCHAR(32) DEFAULT NULL,
  contentType VARCHAR(64) NOT NULL COMMENT 'MIME',
  fileMetadata TEXT DEFAULT NULL,
  thumbnailData TEXT DEFAULT NULL,
  storageStrategyId BIGINT DEFAULT NULL,
  storagePath VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(32) DEFAULT NULL,
  scanStatus VARCHAR(32) DEFAULT NULL,
  integrityStatus VARCHAR(32) DEFAULT NULL,
  integrityCheckedAt DATETIME DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  deletedTime DATETIME DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_sha256_size (fileSha256, fileSize),
  KEY idx_createTime (createTime),
  KEY idx_fileInnerName (fileInnerName),
  KEY idx_sha256 (fileSha256),
  KEY idx_storage_strategy (storageStrategyId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='文件信息表';

CREATE TABLE IF NOT EXISTS sys_file_reference (
  id BIGINT NOT NULL COMMENT '主键ID',
  fileId BIGINT NOT NULL COMMENT '文件ID',
  userId BIGINT NOT NULL COMMENT '用户ID',
  displayName VARCHAR(128) NOT NULL COMMENT '展示名称',
  bizType VARCHAR(50) NOT NULL COMMENT '业务类型',
  bizId BIGINT NOT NULL COMMENT '业务ID',
  visitUrl VARCHAR(255) DEFAULT NULL,
  accessLevel TINYINT DEFAULT 0,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  visitStrategy VARCHAR(32) NOT NULL DEFAULT 'PRIVATE_PREVIEW',
  accessScope VARCHAR(32) NOT NULL DEFAULT 'OWNER_ONLY',
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_biz_active (userId, bizType, bizId, isDeleted),
  KEY idx_user_business (userId, bizType, bizId),
  KEY idx_fileId (fileId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='文件引用表';

CREATE TABLE IF NOT EXISTS sys_upload_task (
  id VARCHAR(64) NOT NULL,
  userId BIGINT NOT NULL,
  bizType INT DEFAULT NULL,
  bizId BIGINT DEFAULT NULL,
  fileName VARCHAR(255) DEFAULT NULL,
  contentType VARCHAR(128) DEFAULT NULL,
  storageStrategyId BIGINT DEFAULT NULL,
  objectKeyStaging VARCHAR(512) NOT NULL,
  objectKeyClean VARCHAR(512) NOT NULL,
  status VARCHAR(32) NOT NULL,
  uploadMode VARCHAR(16) DEFAULT NULL,
  multipartUploadId VARCHAR(128) DEFAULT NULL,
  partSize INT DEFAULT NULL,
  totalParts INT DEFAULT NULL,
  expectedSize BIGINT DEFAULT NULL,
  expectedSha256 CHAR(64) DEFAULT NULL,
  expectedCrc32c VARCHAR(16) DEFAULT NULL,
  actualSize BIGINT DEFAULT NULL,
  etag VARCHAR(128) DEFAULT NULL,
  serverCrc32c VARCHAR(16) DEFAULT NULL,
  failureCategory VARCHAR(32) DEFAULT NULL,
  failureReason VARCHAR(512) DEFAULT NULL,
  fileId BIGINT DEFAULT NULL,
  bindingToken VARCHAR(128) DEFAULT NULL,
  bindingChannel VARCHAR(16) DEFAULT NULL,
  expireAt DATETIME DEFAULT NULL,
  userIp VARCHAR(64) DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_user (userId),
  KEY idx_status (status),
  KEY idx_expire (expireAt),
  KEY idx_file (fileId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='上传任务表';

CREATE TABLE IF NOT EXISTS sys_file_chunk_upload (
  id BIGINT NOT NULL COMMENT '分块上传ID',
  uploadId VARCHAR(64) NOT NULL COMMENT '上传事务ID',
  userId BIGINT NOT NULL COMMENT '用户ID',
  fileName VARCHAR(255) NOT NULL COMMENT '文件名',
  contentType VARCHAR(128) DEFAULT NULL COMMENT 'MIME',
  fileSize BIGINT NOT NULL COMMENT '文件大小',
  chunkSize INT NOT NULL COMMENT '分块大小',
  totalChunks INT NOT NULL COMMENT '总分块数',
  uploadedChunks TEXT DEFAULT NULL COMMENT '已上传分块JSON',
  chunkSha256Map TEXT DEFAULT NULL COMMENT '分块SHA256映射',
  fileSha256 CHAR(64) DEFAULT NULL COMMENT '文件SHA256',
  expectedCrc32c VARCHAR(16) DEFAULT NULL,
  serverCrc32c VARCHAR(16) DEFAULT NULL,
  storageStrategyId BIGINT NOT NULL,
  tempStoragePath VARCHAR(512) DEFAULT NULL,
  cloudUploadId VARCHAR(128) DEFAULT NULL,
  partETagsMap TEXT DEFAULT NULL,
  bizType VARCHAR(50) DEFAULT NULL,
  bizId BIGINT DEFAULT NULL,
  status TINYINT NOT NULL DEFAULT 0,
  expireTime DATETIME NOT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_chunk_upload_id (uploadId),
  KEY idx_chunk_user_status (userId, status, expireTime),
  KEY idx_chunk_expire (expireTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='文件分块上传表';

CREATE TABLE IF NOT EXISTS sys_file_process_task (
  id BIGINT NOT NULL,
  fileId BIGINT NOT NULL,
  taskType VARCHAR(32) NOT NULL,
  taskParams TEXT DEFAULT NULL,
  pipelineId VARCHAR(64) DEFAULT NULL,
  nodeId VARCHAR(64) DEFAULT NULL,
  idempotencyKey VARCHAR(128) DEFAULT NULL,
  dedupKey VARCHAR(128) DEFAULT NULL,
  replayToken VARCHAR(128) DEFAULT NULL,
  dependsOn VARCHAR(512) DEFAULT NULL,
  attempt INT NOT NULL DEFAULT 0,
  status TINYINT NOT NULL DEFAULT 0,
  retryCount INT NOT NULL DEFAULT 0,
  maxRetry INT NOT NULL DEFAULT 3,
  errorMsg TEXT DEFAULT NULL,
  resultData TEXT DEFAULT NULL,
  priority INT NOT NULL DEFAULT 0,
  mqMessageId VARCHAR(128) DEFAULT NULL,
  nextRetryTime DATETIME DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  startTime DATETIME DEFAULT NULL,
  finishTime DATETIME DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_process_task_idempotency (idempotencyKey),
  KEY idx_fileId (fileId),
  KEY idx_status_priority (status, priority),
  KEY idx_taskType (taskType),
  KEY idx_nextRetryTime (nextRetryTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='文件处理任务表';

CREATE TABLE IF NOT EXISTS sys_file_process_run (
  id BIGINT NOT NULL,
  taskId BIGINT NOT NULL,
  fileId BIGINT NOT NULL,
  taskType VARCHAR(32) NOT NULL,
  status TINYINT NOT NULL,
  attempt INT NOT NULL DEFAULT 0,
  errorMsg TEXT DEFAULT NULL,
  resultData TEXT DEFAULT NULL,
  startedAt DATETIME NOT NULL,
  finishedAt DATETIME DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_run_task (taskId),
  KEY idx_run_file (fileId),
  KEY idx_run_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='文件处理运行记录表';

CREATE TABLE IF NOT EXISTS sys_file_integrity_audit (
  id BIGINT NOT NULL,
  fileId BIGINT NOT NULL,
  storageStrategyId BIGINT DEFAULT NULL,
  expectedSha256 CHAR(64) DEFAULT NULL,
  actualSha256 CHAR(64) DEFAULT NULL,
  status VARCHAR(32) NOT NULL,
  errorMsg VARCHAR(512) DEFAULT NULL,
  auditTime DATETIME NOT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_integrity_file (fileId),
  KEY idx_integrity_status (status, auditTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='文件完整性审计表';

CREATE TABLE IF NOT EXISTS sys_storage_alert_log (
  id BIGINT NOT NULL,
  strategyId BIGINT NOT NULL,
  alertType VARCHAR(32) NOT NULL,
  alertLevel VARCHAR(16) NOT NULL,
  message VARCHAR(512) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'OPEN',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_storage_alert_strategy (strategyId, status),
  KEY idx_storage_alert_type (alertType, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='存储告警日志表';

CREATE TABLE IF NOT EXISTS sys_file_binding_task (
  id BIGINT NOT NULL,
  fileId BIGINT NOT NULL,
  userId BIGINT NOT NULL,
  bizType INT NOT NULL,
  bizId BIGINT DEFAULT NULL,
  bindingToken VARCHAR(128) NOT NULL,
  channel VARCHAR(16) NOT NULL,
  status VARCHAR(32) NOT NULL,
  attemptCount INT NOT NULL DEFAULT 0,
  nextRetryTime DATETIME DEFAULT NULL,
  lastError VARCHAR(512) DEFAULT NULL,
  fileName VARCHAR(255) DEFAULT NULL,
  displayName VARCHAR(255) DEFAULT NULL,
  visitStrategy VARCHAR(32) DEFAULT NULL,
  accessScope VARCHAR(32) DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_file_binding_file_token (fileId, bindingToken),
  KEY idx_file_binding_status_retry (status, nextRetryTime),
  KEY idx_file_binding_user (userId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='文件业务绑定任务表';

CREATE TABLE IF NOT EXISTS sys_outbox_event (
  id BIGINT NOT NULL,
  eventId VARCHAR(64) NOT NULL,
  eventType VARCHAR(64) NOT NULL,
  aggregateType VARCHAR(64) NOT NULL,
  aggregateId VARCHAR(64) NOT NULL,
  payload TEXT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
  retryCount INT NOT NULL DEFAULT 0,
  nextRetryAt DATETIME DEFAULT NULL,
  errorMsg VARCHAR(512) DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_outbox_event_id (eventId),
  KEY idx_outbox_status_retry (status, nextRetryAt),
  KEY idx_outbox_aggregate (aggregateType, aggregateId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Outbox事务消息';

CREATE TABLE IF NOT EXISTS sys_message_consume_log (
  id BIGINT NOT NULL,
  messageId VARCHAR(128) NOT NULL,
  consumer VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  detail VARCHAR(512) DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_message_consume (messageId, consumer),
  KEY idx_message_consume_time (createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息消费幂等日志';

INSERT INTO sys_storage_strategy (
  id, strategyName, providerType, isDefault, isEnabled, runState, priority,
  configCiphertext, configEdek, wrapKeyRef, healthStatus, failureCount,
  totalRequests, failureRateThreshold, windowStartTime, createTime, updateTime, isDeleted
)
SELECT
  2026041901001, 'local-default', 'LOCAL', 1, 1, 'ACTIVE', 100,
  '', '', '', 1, 0, 0, 10.00, NOW(), NOW(), NOW(), 0
WHERE NOT EXISTS (
  SELECT 1 FROM sys_storage_strategy WHERE isDefault = 1 AND isDeleted = 0
);

-- +goose Down
DROP TABLE IF EXISTS sys_message_consume_log;
DROP TABLE IF EXISTS sys_outbox_event;
DROP TABLE IF EXISTS sys_file_binding_task;
DROP TABLE IF EXISTS sys_storage_alert_log;
DROP TABLE IF EXISTS sys_file_integrity_audit;
DROP TABLE IF EXISTS sys_file_process_run;
DROP TABLE IF EXISTS sys_file_process_task;
DROP TABLE IF EXISTS sys_file_chunk_upload;
DROP TABLE IF EXISTS sys_upload_task;
DROP TABLE IF EXISTS sys_file_reference;
DROP TABLE IF EXISTS sys_file_info;
DROP TABLE IF EXISTS sys_storage_strategy;
