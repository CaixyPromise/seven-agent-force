-- +goose Up
SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sysExternalOAuthLoginState ADD COLUMN provisioningAuthorityId VARCHAR(96) NULL COMMENT ''平台注册授权ID'' AFTER platformCode',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND column_name = 'provisioningAuthorityId'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sysExternalOAuthLoginState ADD KEY idxSysExternalOAuthLoginStateProvisioningAuthorityId (provisioningAuthorityId)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND index_name = 'idxSysExternalOAuthLoginStateProvisioningAuthorityId'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- +goose Down
SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sysExternalOAuthLoginState DROP INDEX idxSysExternalOAuthLoginStateProvisioningAuthorityId', 'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND index_name = 'idxSysExternalOAuthLoginStateProvisioningAuthorityId'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sysExternalOAuthLoginState DROP COLUMN provisioningAuthorityId', 'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND column_name = 'provisioningAuthorityId'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
