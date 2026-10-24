-- +goose Up
CREATE TABLE IF NOT EXISTS sys_operation_log (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '操作日志ID',
  userId BIGINT DEFAULT NULL COMMENT '用户ID',
  userName VARCHAR(64) DEFAULT NULL COMMENT '用户名',
  nickName VARCHAR(64) DEFAULT NULL COMMENT '用户昵称',
  operationType VARCHAR(64) NOT NULL COMMENT '操作类型',
  operationDesc VARCHAR(255) DEFAULT NULL COMMENT '操作描述',
  methodName VARCHAR(255) DEFAULT NULL COMMENT '方法名称',
  requestMethod VARCHAR(16) DEFAULT NULL COMMENT '请求方法',
  requestUrl VARCHAR(512) DEFAULT NULL COMMENT '请求地址',
  traceId VARCHAR(64) DEFAULT NULL COMMENT '请求链路追踪ID',
  requestParams TEXT DEFAULT NULL COMMENT '请求参数',
  responseResult MEDIUMTEXT DEFAULT NULL COMMENT '响应结果',
  requestIp VARCHAR(64) DEFAULT NULL COMMENT '请求IP',
  requestLocation VARCHAR(128) DEFAULT NULL COMMENT '请求位置',
  userAgent VARCHAR(512) DEFAULT NULL COMMENT '用户代理',
  browser VARCHAR(64) DEFAULT NULL COMMENT '浏览器',
  os VARCHAR(64) DEFAULT NULL COMMENT '操作系统',
  operationTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '操作时间',
  executionTime BIGINT DEFAULT NULL COMMENT '执行耗时毫秒',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：1成功 0失败',
  errorMsg TEXT DEFAULT NULL COMMENT '错误信息',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  isDeleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  PRIMARY KEY (id),
  KEY idx_operation_log_user_time (userId, operationTime),
  KEY idx_operation_log_type_time (operationType, operationTime),
  KEY idx_operation_log_method (requestMethod),
  KEY idx_operation_log_url (requestUrl),
  KEY idx_operation_log_trace_id (traceId),
  KEY idx_operation_log_deleted_time (isDeleted, operationTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='操作日志表';

-- +goose Down
DROP TABLE IF EXISTS sys_operation_log;
