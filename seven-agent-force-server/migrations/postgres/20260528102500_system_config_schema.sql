-- +goose Up
CREATE TABLE IF NOT EXISTS sys_config_group (
  id BIGSERIAL PRIMARY KEY,
  "groupCode" VARCHAR(64) NOT NULL,
  "groupName" VARCHAR(128) NOT NULL,
  module VARCHAR(64),
  "permissionCode" VARCHAR(1024),
  "sortOrder" INT NOT NULL DEFAULT 0,
  status SMALLINT NOT NULL DEFAULT 1,
  "createTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "updateTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "isDeleted" BOOLEAN NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX IF NOT EXISTS "uk_sys_config_group_groupCode" ON sys_config_group ("groupCode");
CREATE INDEX IF NOT EXISTS idx_sys_config_group_module ON sys_config_group (module);
CREATE INDEX IF NOT EXISTS idx_sys_config_group_status ON sys_config_group (status, "isDeleted");

CREATE TABLE IF NOT EXISTS sys_config (
  id BIGSERIAL PRIMARY KEY,
  "groupId" BIGINT NOT NULL,
  "configKey" VARCHAR(128) NOT NULL,
  "configValue" TEXT NOT NULL,
  "valueType" VARCHAR(16) NOT NULL,
  "configDesc" VARCHAR(255),
  "isSensitive" SMALLINT NOT NULL DEFAULT 0,
  "isSystemConfig" BOOLEAN NOT NULL DEFAULT false,
  "requiredLogin" BOOLEAN NOT NULL DEFAULT false,
  "isReadonly" SMALLINT NOT NULL DEFAULT 0,
  "isEnabled" SMALLINT NOT NULL DEFAULT 1,
  "effectType" VARCHAR(32),
  "extJson" TEXT,
  "createdBy" BIGINT NOT NULL DEFAULT 0,
  "createTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "updatedBy" BIGINT NOT NULL DEFAULT 0,
  "updateTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "isDeleted" BOOLEAN NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX IF NOT EXISTS "uk_sys_config_configKey_groupId" ON sys_config ("configKey", "groupId");
CREATE INDEX IF NOT EXISTS idx_sys_config_group_id ON sys_config ("groupId");
CREATE INDEX IF NOT EXISTS idx_sys_config_config_key ON sys_config ("configKey");
CREATE INDEX IF NOT EXISTS idx_sys_config_enabled ON sys_config ("isEnabled", "isDeleted");
CREATE INDEX IF NOT EXISTS idx_sys_config_sensitive ON sys_config ("isSensitive");

CREATE TABLE IF NOT EXISTS sys_config_change_log (
  id BIGINT PRIMARY KEY,
  "configId" BIGINT NOT NULL,
  "configKey" VARCHAR(255) NOT NULL,
  "operationType" VARCHAR(20) NOT NULL,
  "oldValue" TEXT,
  "newValue" TEXT,
  "effectType" VARCHAR(20) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  "parentLogId" BIGINT,
  "relatedLogId" BIGINT,
  "operatorId" BIGINT,
  "operatorName" VARCHAR(100),
  "operationTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "operationReason" VARCHAR(500),
  "appliedBy" BIGINT,
  "appliedTime" TIMESTAMPTZ,
  "createTime" TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_config_id ON sys_config_change_log ("configId");
CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_config_key ON sys_config_change_log ("configKey");
CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_operation_type ON sys_config_change_log ("operationType");
CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_status ON sys_config_change_log (status);
CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_parent_log_id ON sys_config_change_log ("parentLogId");
CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_related_log_id ON sys_config_change_log ("relatedLogId");
CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_operation_time ON sys_config_change_log ("operationTime");
CREATE INDEX IF NOT EXISTS idx_sys_config_change_log_operator_id ON sys_config_change_log ("operatorId");

-- +goose Down
-- Intentionally no-op: these tables are shared by system-config runtime code.
