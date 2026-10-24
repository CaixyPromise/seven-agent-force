-- +goose Up
CREATE TABLE IF NOT EXISTS sysPlatform (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    platformCode VARCHAR(64) NOT NULL COMMENT '平台编码',
    platformName VARCHAR(128) NOT NULL COMMENT '平台名称',
    platformType VARCHAR(32) NOT NULL DEFAULT 'ADMIN' COMMENT '平台类型：ADMIN/PORTAL/API',
    description VARCHAR(512) NULL COMMENT '平台说明',
    defaultRedirectUrl VARCHAR(1024) NULL COMMENT '默认登录后跳转地址',
    allowAutoRegister TINYINT NOT NULL DEFAULT 0 COMMENT '是否允许自动创建用户',
    isDefault TINYINT NOT NULL DEFAULT 0 COMMENT '是否默认平台',
    defaultDeptId BIGINT NULL COMMENT '默认部门 ID',
    brandJson JSON NULL COMMENT '登录页品牌配置 JSON',
    settingsJson JSON NULL COMMENT '平台扩展设置 JSON',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    creatorId BIGINT NULL COMMENT '创建人 ID',
    updaterId BIGINT NULL COMMENT '更新人 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysPlatform_code_deleted (platformCode, isDeleted),
    KEY idx_sysPlatform_status_deleted (status, isDeleted),
    KEY idx_sysPlatform_default_status_deleted (isDefault, status, isDeleted)
) COMMENT='平台配置表';

CREATE TABLE IF NOT EXISTS sysPlatformSsoClient (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    platformCode VARCHAR(64) NOT NULL COMMENT '平台编码',
    clientId VARCHAR(128) NOT NULL COMMENT 'SSO 客户端标识',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    creatorId BIGINT NULL COMMENT '创建人 ID',
    updaterId BIGINT NULL COMMENT '更新人 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysPlatformSsoClient_platform_client_deleted (platformCode, clientId, isDeleted),
    KEY idx_sysPlatformSsoClient_client_status_deleted (clientId, status, isDeleted)
) COMMENT='平台与 SSO 客户端关联表';

CREATE TABLE IF NOT EXISTS sysPlatformSourceRule (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    platformCode VARCHAR(64) NOT NULL COMMENT '平台编码',
    matchType VARCHAR(32) NOT NULL COMMENT '匹配类型：CLIENT_ID/REDIRECT_HOST/REDIRECT_PREFIX/HOST/ORIGIN/REFERER_HOST',
    matchValue VARCHAR(1024) NOT NULL COMMENT '匹配值',
    priority INT NOT NULL DEFAULT 0 COMMENT '优先级，数值越大越优先',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    creatorId BIGINT NULL COMMENT '创建人 ID',
    updaterId BIGINT NULL COMMENT '更新人 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysPlatformSourceRule_platform_type_value_deleted (platformCode, matchType, matchValue(512), isDeleted),
    KEY idx_sysPlatformSourceRule_type_status_priority_deleted (matchType, status, priority, isDeleted)
) COMMENT='平台来源匹配规则表';

CREATE TABLE IF NOT EXISTS sysPlatformLoginMethod (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    platformCode VARCHAR(64) NOT NULL COMMENT '平台编码',
    methodType VARCHAR(32) NOT NULL COMMENT '登录方式：PASSWORD/PASSKEY/EXTERNAL_OAUTH',
    providerCode VARCHAR(64) NOT NULL DEFAULT '' COMMENT '外部登录提供方编码，非外部登录为空字符串',
    displayName VARCHAR(128) NOT NULL COMMENT '登录页展示名称',
    icon VARCHAR(128) NULL COMMENT '图标编码',
    sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序',
    displayEnabled TINYINT NOT NULL DEFAULT 1 COMMENT '是否在登录页展示',
    loginEnabled TINYINT NOT NULL DEFAULT 1 COMMENT '是否允许登录',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    creatorId BIGINT NULL COMMENT '创建人 ID',
    updaterId BIGINT NULL COMMENT '更新人 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysPlatformLoginMethod_platform_method_provider_deleted (platformCode, methodType, providerCode, isDeleted),
    KEY idx_sysPlatformLoginMethod_platform_display_login_deleted (platformCode, displayEnabled, loginEnabled, isDeleted)
) COMMENT='平台登录方式配置表';

CREATE TABLE IF NOT EXISTS sysPlatformDefaultRole (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    platformCode VARCHAR(64) NOT NULL COMMENT '平台编码',
    roleId BIGINT NOT NULL COMMENT '默认角色 ID',
    autoAssignEnabled TINYINT NOT NULL DEFAULT 0 COMMENT '是否允许自动注册分配',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    creatorId BIGINT NULL COMMENT '创建人 ID',
    updaterId BIGINT NULL COMMENT '更新人 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysPlatformDefaultRole_platform_role_deleted (platformCode, roleId, isDeleted),
    KEY idx_sysPlatformDefaultRole_platform_status_deleted (platformCode, status, isDeleted)
) COMMENT='平台默认角色配置表';

SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sysExternalOAuthLoginState ADD COLUMN platformCode VARCHAR(64) NULL COMMENT ''平台编码'' AFTER providerCode',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND column_name = 'platformCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

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

SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sysExternalOAuthLoginState ADD KEY idx_sysExternalOAuthLoginState_platform_status_deleted (platformCode, status, isDeleted)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND index_name = 'idx_sysExternalOAuthLoginState_platform_status_deleted'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sysSsoSession ADD COLUMN platformCode VARCHAR(64) NULL COMMENT ''平台编码'' AFTER clientId',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysSsoSession' AND column_name = 'platformCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sysSsoSession ADD KEY idx_sysSsoSession_platformCode_status_deleted (platformCode, status, isDeleted)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'sysSsoSession' AND index_name = 'idx_sysSsoSession_platformCode_status_deleted'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sys_user ADD COLUMN registerPlatformCode VARCHAR(64) NULL COMMENT ''注册来源平台编码''',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sys_user' AND column_name = 'registerPlatformCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sys_user ADD COLUMN registerProviderCode VARCHAR(64) NULL COMMENT ''注册来源外部登录提供方编码''',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sys_user' AND column_name = 'registerProviderCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- +goose Down
SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sys_user DROP COLUMN registerProviderCode', 'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sys_user' AND column_name = 'registerProviderCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sys_user DROP COLUMN registerPlatformCode', 'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sys_user' AND column_name = 'registerPlatformCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sysSsoSession DROP INDEX idx_sysSsoSession_platformCode_status_deleted', 'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'sysSsoSession' AND index_name = 'idx_sysSsoSession_platformCode_status_deleted'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sysSsoSession DROP COLUMN platformCode', 'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysSsoSession' AND column_name = 'platformCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sysExternalOAuthLoginState DROP INDEX idx_sysExternalOAuthLoginState_platform_status_deleted', 'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND index_name = 'idx_sysExternalOAuthLoginState_platform_status_deleted'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

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

SET @sql := (
    SELECT IF(COUNT(*) > 0, 'ALTER TABLE sysExternalOAuthLoginState DROP COLUMN platformCode', 'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysExternalOAuthLoginState' AND column_name = 'platformCode'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

DROP TABLE IF EXISTS sysPlatformDefaultRole;
DROP TABLE IF EXISTS sysPlatformLoginMethod;
DROP TABLE IF EXISTS sysPlatformSourceRule;
DROP TABLE IF EXISTS sysPlatformSsoClient;
DROP TABLE IF EXISTS sysPlatform;
