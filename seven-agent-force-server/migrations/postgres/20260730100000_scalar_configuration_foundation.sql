-- +goose Up
CREATE TABLE IF NOT EXISTS sys_dict_type (
  id BIGSERIAL PRIMARY KEY,
  "dictCode" VARCHAR(64) NOT NULL,
  "dictName" VARCHAR(128) NOT NULL,
  "dictDesc" VARCHAR(255),
  module VARCHAR(64),
  status SMALLINT NOT NULL DEFAULT 1,
  "requiredLogin" BOOLEAN NOT NULL DEFAULT false,
  "sortOrder" INT NOT NULL DEFAULT 0,
  "isSystem" SMALLINT NOT NULL DEFAULT 0,
  "createdBy" BIGINT NOT NULL DEFAULT 0,
  "createTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "updatedBy" BIGINT NOT NULL DEFAULT 0,
  "updateTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "isDeleted" BOOLEAN NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_sys_dict_type_dictCode" ON sys_dict_type ("dictCode");

CREATE TABLE IF NOT EXISTS sys_dict_item (
  id BIGSERIAL PRIMARY KEY,
  "dictTypeId" BIGINT NOT NULL,
  "itemValue" VARCHAR(64) NOT NULL,
  "itemLabel" VARCHAR(128) NOT NULL,
  "itemDesc" VARCHAR(255),
  "sortOrder" INT NOT NULL DEFAULT 0,
  status SMALLINT NOT NULL DEFAULT 1,
  "extJson" TEXT,
  "createdBy" BIGINT NOT NULL DEFAULT 0,
  "createTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "updatedBy" BIGINT NOT NULL DEFAULT 0,
  "updateTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "isDeleted" BOOLEAN NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_sys_dict_item_type_value" ON sys_dict_item ("dictTypeId", "itemValue");

ALTER TABLE sys_config
  ADD COLUMN IF NOT EXISTS "uiWidget" VARCHAR(32) NOT NULL DEFAULT 'INPUT',
  ADD COLUMN IF NOT EXISTS "validationJson" TEXT,
  ADD COLUMN IF NOT EXISTS exposure VARCHAR(20) NOT NULL DEFAULT 'INTERNAL',
  ADD COLUMN IF NOT EXISTS sensitivity VARCHAR(20) NOT NULL DEFAULT 'NORMAL',
  ADD COLUMN IF NOT EXISTS "schemaVersion" INT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

UPDATE sys_config
SET "valueType" = CASE UPPER("valueType")
  WHEN 'INT' THEN 'INTEGER'
  WHEN 'BOOL' THEN 'BOOLEAN'
  WHEN 'ARRAY' THEN 'MULTI_ENUM'
  ELSE UPPER("valueType")
END,
"uiWidget" = CASE UPPER("valueType")
  WHEN 'INT' THEN 'INPUT_NUMBER'
  WHEN 'INTEGER' THEN 'INPUT_NUMBER'
  WHEN 'BOOL' THEN 'SWITCH'
  WHEN 'BOOLEAN' THEN 'SWITCH'
  WHEN 'ENUM' THEN 'SELECT'
  WHEN 'ARRAY' THEN 'MULTI_SELECT'
  WHEN 'MULTI_ENUM' THEN 'MULTI_SELECT'
  WHEN 'JSON' THEN 'CONTROLLED_JSON'
  ELSE 'INPUT'
END,
exposure = 'INTERNAL',
sensitivity = CASE WHEN "isSensitive" = 1 THEN 'SENSITIVE' ELSE 'NORMAL' END,
"schemaVersion" = 1,
version = 1;

UPDATE sys_config c
SET exposure = 'AUTHENTICATED'
FROM sys_config_group g
WHERE g.id = c."groupId"
  AND g."groupCode" = 'SEVEN_FRONTEND_METADATA'
  AND c."configKey" = 'title';

ALTER TABLE sys_config_change_log
  ADD COLUMN IF NOT EXISTS "oldValueProtected" BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS "newValueProtected" BOOLEAN NOT NULL DEFAULT false;

CREATE SEQUENCE IF NOT EXISTS sys_config_change_log_id_seq;
ALTER SEQUENCE sys_config_change_log_id_seq OWNED BY sys_config_change_log.id;
ALTER TABLE sys_config_change_log ALTER COLUMN id SET DEFAULT nextval('sys_config_change_log_id_seq');
SELECT setval(
  'sys_config_change_log_id_seq',
  GREATEST(COALESCE(MAX(id), 0), 1),
  COALESCE(MAX(id), 0) > 0
) FROM sys_config_change_log;

UPDATE sys_config_change_log l
SET "oldValue" = CASE WHEN c."isSensitive" = 1 THEN '[PROTECTED]' ELSE l."oldValue" END,
    "newValue" = CASE WHEN c."isSensitive" = 1 THEN '[PROTECTED]' ELSE l."newValue" END,
    "oldValueProtected" = CASE WHEN c."isSensitive" = 1 THEN true ELSE false END,
    "newValueProtected" = CASE WHEN c."isSensitive" = 1 THEN true ELSE false END
FROM sys_config c
WHERE c.id = l."configId";

UPDATE sys_config_change_log l
SET status = 'rolled_back',
    "operationReason" = 'scalar migration: sensitive pending value retired'
FROM sys_config c
WHERE c.id = l."configId" AND c."isSensitive" = 1 AND l.status = 'pending';

ALTER TABLE sys_dict_type
  ADD COLUMN IF NOT EXISTS "valueType" VARCHAR(20) NOT NULL DEFAULT 'STRING',
  ADD COLUMN IF NOT EXISTS "uiWidget" VARCHAR(32) NOT NULL DEFAULT 'SELECT',
  ADD COLUMN IF NOT EXISTS "validationJson" TEXT,
  ADD COLUMN IF NOT EXISTS exposure VARCHAR(20) NOT NULL DEFAULT 'INTERNAL',
  ADD COLUMN IF NOT EXISTS sensitivity VARCHAR(20) NOT NULL DEFAULT 'NORMAL',
  ADD COLUMN IF NOT EXISTS "schemaVersion" INT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE sys_dict_item
  ADD COLUMN IF NOT EXISTS "colorToken" VARCHAR(32),
  ADD COLUMN IF NOT EXISTS "iconToken" VARCHAR(64),
  ADD COLUMN IF NOT EXISTS "presentationVersion" INT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

UPDATE sys_dict_type SET exposure = 'INTERNAL', sensitivity = 'NORMAL', "schemaVersion" = 1, version = 1;
UPDATE sys_dict_type SET exposure = 'PUBLIC' WHERE "dictCode" = 'gender';

-- Legacy upgrade schemas store extJson as TEXT while the clean-install baseline
-- stores it as JSON. Cast both representations through TEXT before parsing so
-- malformed historical TEXT rows are skipped just like MySQL JSON_VALID rows.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pg_temp.dc23_try_jsonb(value TEXT)
RETURNS JSONB
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
  RETURN value::jsonb;
EXCEPTION WHEN others THEN
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

WITH parsed AS (
  SELECT id, pg_temp.dc23_try_jsonb("extJson"::text) AS payload
  FROM sys_dict_item
  WHERE "extJson" IS NOT NULL AND BTRIM("extJson"::text) <> ''
)
UPDATE sys_dict_item AS item
SET "colorToken" = CASE WHEN parsed.payload->>'color' IN ('gray','blue','pink','green','orange','red','purple') THEN parsed.payload->>'color' END,
    "iconToken" = CASE WHEN parsed.payload->>'icon' IN ('unknown','male','female','check','close','info') THEN parsed.payload->>'icon' END
FROM parsed
WHERE item.id = parsed.id AND parsed.payload IS NOT NULL;

-- +goose Down
-- Additive contract migration is intentionally non-destructive.
SELECT 1;
