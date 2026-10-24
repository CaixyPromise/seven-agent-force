-- +goose Up
-- CONFIG_ASSET owns exactly one active sys_file_reference per configuration.
-- Unlike the existing (userId, scopeId, bizType, bizId) slot index, this key
-- is independent of the binding operator and therefore closes cross-operator
-- parallel references for the same server-derived configId.
CREATE UNIQUE INDEX "uk_file_reference_active_config_asset"
  ON "sys_file_reference" ("bizId")
  WHERE "isDeleted" = false AND "bizType" = 'CONFIG_ASSET';
CREATE INDEX "idx_file_reference_config_asset_lookup"
  ON "sys_file_reference" ("bizType", "bizId", "scopeId")
  WHERE "isDeleted" = false;

-- Create the reviewed browser metadata group only when absent. Existing group
-- policy stays intact rather than being silently widened during upgrade.
INSERT INTO "sys_config_group" ("groupCode", "groupName", module, "permissionCode", "sortOrder", status, "isDeleted")
SELECT 'SEVEN_FRONTEND_METADATA', '前端元数据', 'system', NULL, 0, 1, false
WHERE NOT EXISTS (
  SELECT 1 FROM "sys_config_group" WHERE "groupCode" = 'SEVEN_FRONTEND_METADATA'
);

-- Empty typed rows are the sole canonical browser consumers for custom
-- logo/favicon. Their value can only become a server-generated same-origin
-- /api/config-assets/{configId} path via the atomic config command.
INSERT INTO "sys_config" (
  "groupId", "configKey", "configValue", "valueType", "configDesc",
  "isSensitive", "isSystemConfig", "requiredLogin", "isReadonly", "isEnabled",
  "effectType", "extJson", "uiWidget", "validationJson", exposure, sensitivity,
  "schemaVersion", version, "createdBy", "updatedBy", "isDeleted"
)
SELECT
  g.id, 'loginLogo', '', 'IMAGE', '登录页品牌图标（受控配置资产）',
  0, false, false, 0, 1,
  'realtime', NULL, 'IMAGE_UPLOAD', NULL, 'PUBLIC', 'NORMAL',
  1, 1, 0, 0, false
FROM "sys_config_group" g
WHERE g."groupCode" = 'SEVEN_FRONTEND_METADATA' AND g."isDeleted" = false
  AND NOT EXISTS (
    SELECT 1 FROM "sys_config" c WHERE c."groupId" = g.id AND c."configKey" = 'loginLogo'
  );

INSERT INTO "sys_config" (
  "groupId", "configKey", "configValue", "valueType", "configDesc",
  "isSensitive", "isSystemConfig", "requiredLogin", "isReadonly", "isEnabled",
  "effectType", "extJson", "uiWidget", "validationJson", exposure, sensitivity,
  "schemaVersion", version, "createdBy", "updatedBy", "isDeleted"
)
SELECT
  g.id, 'favicon', '', 'IMAGE', '浏览器站点图标（受控配置资产）',
  0, false, false, 0, 1,
  'realtime', NULL, 'IMAGE_UPLOAD', NULL, 'PUBLIC', 'NORMAL',
  1, 1, 0, 0, false
FROM "sys_config_group" g
WHERE g."groupCode" = 'SEVEN_FRONTEND_METADATA' AND g."isDeleted" = false
  AND NOT EXISTS (
    SELECT 1 FROM "sys_config" c WHERE c."groupId" = g.id AND c."configKey" = 'favicon'
  );

-- The old raw URL field is explicitly retired. Do not migrate arbitrary URL,
-- blob/data URI or physical-path content into the CONFIG_ASSET contract.
UPDATE sys_platform
SET "brandJson" = ("brandJson"::jsonb - 'logoUrl')::json
WHERE "brandJson" IS NOT NULL
  AND jsonb_typeof("brandJson"::jsonb) = 'object'
  AND "brandJson"::jsonb ? 'logoUrl';

-- +goose Down
-- Do not resurrect raw URLs or delete operator-managed configuration rows.
-- This rollback only removes the additive active-slot indexes, so a subsequent
-- Goose Up is repeatable while durable configuration remains intact.
DROP INDEX "idx_file_reference_config_asset_lookup";
DROP INDEX "uk_file_reference_active_config_asset";
