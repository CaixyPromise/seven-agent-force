-- +goose Up
CREATE TABLE IF NOT EXISTS sys_config_group (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  groupCode VARCHAR(64) NOT NULL COMMENT '配置分组编码',
  groupName VARCHAR(128) NOT NULL COMMENT '配置分组名称',
  module VARCHAR(64) DEFAULT NULL COMMENT '所属模块',
  permissionCode VARCHAR(1024) DEFAULT NULL COMMENT '读取权限编码',
  sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序顺序',
  status TINYINT NOT NULL DEFAULT 1 COMMENT '状态',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  isDeleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  PRIMARY KEY (id),
  UNIQUE KEY uk_groupCode (groupCode),
  KEY idx_module (module),
  KEY idx_status (status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统配置分组表';

CREATE TABLE IF NOT EXISTS sys_config (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  groupId BIGINT NOT NULL COMMENT '配置分组ID',
  configKey VARCHAR(128) NOT NULL COMMENT '配置键',
  configValue TEXT NOT NULL COMMENT '配置值',
  valueType VARCHAR(16) NOT NULL COMMENT '值类型',
  configDesc VARCHAR(255) DEFAULT NULL COMMENT '配置描述',
  isSensitive TINYINT NOT NULL DEFAULT 0 COMMENT '是否敏感',
  isSystemConfig TINYINT(1) NOT NULL DEFAULT 0 COMMENT '系统内部配置',
  requiredLogin TINYINT(1) NOT NULL DEFAULT 0 COMMENT '读取是否要求登录',
  isReadonly TINYINT NOT NULL DEFAULT 0 COMMENT '是否只读',
  isEnabled TINYINT NOT NULL DEFAULT 1 COMMENT '是否启用',
  effectType VARCHAR(32) DEFAULT NULL COMMENT '生效方式',
  extJson TEXT DEFAULT NULL COMMENT '扩展信息',
  createdBy BIGINT NOT NULL DEFAULT 0 COMMENT '创建人ID',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updatedBy BIGINT NOT NULL DEFAULT 0 COMMENT '更新人ID',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  isDeleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  PRIMARY KEY (id),
  UNIQUE KEY uk_configKey_groupId (configKey, groupId),
  KEY idx_groupId (groupId),
  KEY idx_configKey (configKey),
  KEY idx_enabled (isEnabled, isDeleted),
  KEY idx_sensitive (isSensitive)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统配置表';

CREATE TABLE IF NOT EXISTS sys_config_change_log (
  id BIGINT NOT NULL COMMENT '主键ID',
  configId BIGINT NOT NULL COMMENT '配置ID',
  configKey VARCHAR(255) NOT NULL COMMENT '配置键',
  operationType VARCHAR(20) NOT NULL COMMENT '操作类型',
  oldValue TEXT DEFAULT NULL COMMENT '旧值',
  newValue TEXT DEFAULT NULL COMMENT '新值',
  effectType VARCHAR(20) NOT NULL COMMENT '生效方式',
  status VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT '状态',
  parentLogId BIGINT DEFAULT NULL COMMENT '父级日志ID',
  relatedLogId BIGINT DEFAULT NULL COMMENT '关联日志ID',
  operatorId BIGINT DEFAULT NULL COMMENT '操作人ID',
  operatorName VARCHAR(100) DEFAULT NULL COMMENT '操作人姓名',
  operationTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '操作时间',
  operationReason VARCHAR(500) DEFAULT NULL COMMENT '操作原因',
  appliedBy BIGINT DEFAULT NULL COMMENT '应用人ID',
  appliedTime DATETIME DEFAULT NULL COMMENT '应用时间',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (id),
  KEY idx_config_id (configId),
  KEY idx_config_key (configKey),
  KEY idx_operation_type (operationType),
  KEY idx_status (status),
  KEY idx_parent_log_id (parentLogId),
  KEY idx_related_log_id (relatedLogId),
  KEY idx_operation_time (operationTime),
  KEY idx_operator_id (operatorId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='配置变更审计日志表';

-- +goose Down
-- Intentionally no-op: these tables are shared by system-config runtime code.
