-- +goose Up
-- CONFIG_ASSET owns exactly one active sys_file_reference per configuration.
-- The existing userId/scopeId active-slot key remains the operator audit slot;
-- this generated key is deliberately independent of the operator so a second
-- administrator cannot create a parallel active asset for the same configId.
ALTER TABLE sys_file_reference
  ADD COLUMN activeConfigAssetBizId BIGINT
    GENERATED ALWAYS AS (
      CASE WHEN isDeleted = 0 AND bizType = 'CONFIG_ASSET' THEN bizId ELSE NULL END
    ) STORED,
  ADD UNIQUE KEY uk_file_reference_active_config_asset (activeConfigAssetBizId),
  ADD KEY idx_file_reference_config_asset_lookup (bizType, bizId, scopeId, isDeleted);

-- Create the reviewed browser metadata group only on installations that do not
-- already have it. Existing groups are left untouched so local permission
-- policy is never silently widened by this migration.
INSERT INTO sys_config_group (groupCode, groupName, module, permissionCode, sortOrder, status, isDeleted)
SELECT 'SEVEN_FRONTEND_METADATA', '前端元数据', 'system', NULL, 0, 1, 0
WHERE NOT EXISTS (
  SELECT 1 FROM sys_config_group WHERE groupCode = 'SEVEN_FRONTEND_METADATA'
);

-- These empty typed rows are the only canonical browser consumers for custom
-- logo/favicon. Values can later become only /api/config-assets/{configId}
-- through the atomic configuration command; no URL is seeded or migrated.
INSERT INTO sys_config (
  groupId, configKey, configValue, valueType, configDesc,
  isSensitive, isSystemConfig, requiredLogin, isReadonly, isEnabled,
  effectType, extJson, uiWidget, validationJson, exposure, sensitivity,
  schemaVersion, version, createdBy, updatedBy, isDeleted
)
SELECT
  g.id, 'loginLogo', '', 'IMAGE', '登录页品牌图标（受控配置资产）',
  0, 0, 0, 0, 1,
  'realtime', NULL, 'IMAGE_UPLOAD', NULL, 'PUBLIC', 'NORMAL',
  1, 1, 0, 0, 0
FROM sys_config_group g
WHERE g.groupCode = 'SEVEN_FRONTEND_METADATA' AND g.isDeleted = 0
  AND NOT EXISTS (
    SELECT 1 FROM sys_config c WHERE c.groupId = g.id AND c.configKey = 'loginLogo'
  );

INSERT INTO sys_config (
  groupId, configKey, configValue, valueType, configDesc,
  isSensitive, isSystemConfig, requiredLogin, isReadonly, isEnabled,
  effectType, extJson, uiWidget, validationJson, exposure, sensitivity,
  schemaVersion, version, createdBy, updatedBy, isDeleted
)
SELECT
  g.id, 'favicon', '', 'IMAGE', '浏览器站点图标（受控配置资产）',
  0, 0, 0, 0, 1,
  'realtime', NULL, 'IMAGE_UPLOAD', NULL, 'PUBLIC', 'NORMAL',
  1, 1, 0, 0, 0
FROM sys_config_group g
WHERE g.groupCode = 'SEVEN_FRONTEND_METADATA' AND g.isDeleted = 0
  AND NOT EXISTS (
    SELECT 1 FROM sys_config c WHERE c.groupId = g.id AND c.configKey = 'favicon'
  );

-- brandJson.logoUrl was a legacy raw-URL parallel path. Remove it rather
-- than attempting to copy an untrusted address into CONFIG_ASSET. Text title,
-- subtitle and theme fields remain authoritative in sys_platform.brandJson.
UPDATE sys_platform
SET brandJson = JSON_REMOVE(brandJson, '$.logoUrl')
WHERE brandJson IS NOT NULL
  AND JSON_CONTAINS_PATH(brandJson, 'one', '$.logoUrl');

-- +goose Down
-- Do not restore raw legacy URLs or delete seeded configuration rows: either
-- action could resurrect unsafe presentation data or destroy an operator's
-- active asset. Dropping the additive active-slot index/column makes Up
-- repeatable after Goose rollback without touching durable configuration data.
ALTER TABLE sys_file_reference
  DROP INDEX idx_file_reference_config_asset_lookup,
  DROP INDEX uk_file_reference_active_config_asset,
  DROP COLUMN activeConfigAssetBizId;
