-- +goose Up
-- Persist request trace IDs in operation logs so audit records can be joined
-- with HTTP responses and structured runtime logs.
SET @hasOperationLogTable := (
  SELECT COUNT(1)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_operation_log'
);
SET @hasOperationLogTraceID := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_operation_log'
    AND COLUMN_NAME = 'traceId'
);
SET @sql := IF(
  @hasOperationLogTable > 0 AND @hasOperationLogTraceID = 0,
  'ALTER TABLE sys_operation_log ADD COLUMN traceId VARCHAR(64) DEFAULT NULL COMMENT ''请求链路追踪ID'' AFTER requestUrl',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @hasOperationLogTraceIDIndex := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_operation_log'
    AND INDEX_NAME = 'idx_operation_log_trace_id'
);
SET @sql := IF(
  @hasOperationLogTable > 0 AND @hasOperationLogTraceIDIndex = 0,
  'ALTER TABLE sys_operation_log ADD KEY idx_operation_log_trace_id (traceId)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- +goose Down
SET @hasOperationLogTable := (
  SELECT COUNT(1)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_operation_log'
);
SET @hasOperationLogTraceIDIndex := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_operation_log'
    AND INDEX_NAME = 'idx_operation_log_trace_id'
);
SET @sql := IF(
  @hasOperationLogTable > 0 AND @hasOperationLogTraceIDIndex > 0,
  'ALTER TABLE sys_operation_log DROP KEY idx_operation_log_trace_id',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @hasOperationLogTraceID := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_operation_log'
    AND COLUMN_NAME = 'traceId'
);
SET @sql := IF(
  @hasOperationLogTable > 0 AND @hasOperationLogTraceID > 0,
  'ALTER TABLE sys_operation_log DROP COLUMN traceId',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
