-- +goose Up
ALTER TABLE sysSsoSession
    ADD COLUMN loginMethod VARCHAR(64) NOT NULL DEFAULT 'LOCAL' COMMENT '登录方式：LOCAL/PASSWORD/PASSKEY/TOTP/EXTERNAL_OAUTH' AFTER deviceId,
    ADD COLUMN externalProviderCode VARCHAR(64) NULL COMMENT '外部登录提供方编码' AFTER loginMethod,
    ADD COLUMN externalIdentityId BIGINT NULL COMMENT '外部身份绑定 ID' AFTER externalProviderCode,
    ADD KEY idx_sysSsoSession_external_provider_status_deleted (externalProviderCode, status, isDeleted),
    ADD KEY idx_sysSsoSession_external_identity_status_deleted (externalIdentityId, status, isDeleted);

CREATE TABLE IF NOT EXISTS sysExternalLoginProvider (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    providerCode VARCHAR(64) NOT NULL COMMENT '提供方编码',
    providerName VARCHAR(128) NOT NULL COMMENT '提供方名称',
    protocolType VARCHAR(32) NOT NULL COMMENT '协议类型：OIDC/OAUTH2',
    issuer VARCHAR(512) NULL COMMENT 'OIDC issuer',
    authorizationEndpoint VARCHAR(1024) NOT NULL COMMENT '授权端点',
    tokenEndpoint VARCHAR(1024) NOT NULL COMMENT '令牌端点',
    userinfoEndpoint VARCHAR(1024) NULL COMMENT '用户信息端点',
    jwksUri VARCHAR(1024) NULL COMMENT 'JWKS 地址',
    clientId VARCHAR(255) NOT NULL COMMENT '外部平台客户端 ID',
    clientSecretCiphertext TEXT NULL COMMENT '客户端密钥密文',
    clientSecretEdek TEXT NULL COMMENT '客户端密钥 EDEK',
    clientSecretWrapKeyRef VARCHAR(128) NULL COMMENT '客户端密钥包装密钥引用',
    scopesJson JSON NOT NULL COMMENT '授权范围 JSON',
    redirectUri VARCHAR(1024) NOT NULL COMMENT '回调地址',
    displayName VARCHAR(128) NOT NULL COMMENT '登录页展示名称',
    icon VARCHAR(128) NULL COMMENT '图标编码',
    sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序',
    displayEnabled TINYINT NOT NULL DEFAULT 0 COMMENT '是否展示在登录页',
    loginEnabled TINYINT NOT NULL DEFAULT 0 COMMENT '是否允许登录',
    bindEnabled TINYINT NOT NULL DEFAULT 1 COMMENT '是否允许绑定身份',
    emailAutoBindEnabled TINYINT NOT NULL DEFAULT 0 COMMENT '是否允许 verified email 自动绑定',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    creatorId BIGINT NULL COMMENT '创建人 ID',
    updaterId BIGINT NULL COMMENT '更新人 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysExternalLoginProvider_code_deleted (providerCode, isDeleted),
    KEY idx_sysExternalLoginProvider_display_login_status_deleted (displayEnabled, loginEnabled, status, isDeleted)
) COMMENT='外部登录提供方配置表';

CREATE TABLE IF NOT EXISTS sysExternalProviderMethod (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    providerCode VARCHAR(64) NOT NULL COMMENT '提供方编码',
    methodKey VARCHAR(128) NOT NULL COMMENT '能力方法键',
    capabilityCode VARCHAR(64) NOT NULL COMMENT '能力编码',
    requiredScopesJson JSON NULL COMMENT '所需 scope JSON',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysExternalProviderMethod_provider_method_deleted (providerCode, methodKey, isDeleted),
    KEY idx_sysExternalProviderMethod_capability_status_deleted (capabilityCode, status, isDeleted)
) COMMENT='外部提供方能力方法索引表';

CREATE TABLE IF NOT EXISTS sysExternalUserIdentity (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    providerCode VARCHAR(64) NOT NULL COMMENT '提供方编码',
    externalSubject VARCHAR(255) NOT NULL COMMENT '外部稳定主体 ID',
    userId BIGINT NOT NULL COMMENT '本地用户 ID',
    externalLogin VARCHAR(255) NULL COMMENT '外部登录名',
    externalEmail VARCHAR(255) NULL COMMENT '外部邮箱',
    emailVerified TINYINT NOT NULL DEFAULT 0 COMMENT '外部邮箱是否已验证',
    displayName VARCHAR(255) NULL COMMENT '外部展示名称',
    avatarUrl VARCHAR(1024) NULL COMMENT '外部头像 URL',
    profileJson JSON NULL COMMENT '外部资料 JSON',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED，2 UNLINKED',
    firstLinkedAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '首次绑定时间',
    lastLoginAt DATETIME NULL COMMENT '最近登录时间',
    lastVerifiedAt DATETIME NULL COMMENT '最近验证时间',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    creatorId BIGINT NULL COMMENT '创建人 ID',
    updaterId BIGINT NULL COMMENT '更新人 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysExternalUserIdentity_subject_deleted (providerCode, externalSubject, isDeleted),
    KEY idx_sysExternalUserIdentity_user_provider_deleted (userId, providerCode, isDeleted),
    KEY idx_sysExternalUserIdentity_provider_status_deleted (providerCode, status, isDeleted)
) COMMENT='外部用户身份绑定表';

CREATE TABLE IF NOT EXISTS sysExternalOAuthLoginState (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    stateId VARCHAR(128) NOT NULL COMMENT '状态 ID',
    providerCode VARCHAR(64) NOT NULL COMMENT '提供方编码',
    loginTransactionId VARCHAR(128) NULL COMMENT 'SSO 登录交易 ID',
    redirectAfterLogin VARCHAR(1024) NULL COMMENT '登录后跳转地址',
    stateHash VARCHAR(255) NOT NULL COMMENT 'state 哈希',
    nonceHash VARCHAR(255) NULL COMMENT 'nonce 哈希',
    codeVerifierCiphertext TEXT NULL COMMENT 'PKCE verifier 密文',
    codeVerifierEdek TEXT NULL COMMENT 'PKCE verifier EDEK',
    codeVerifierWrapKeyRef VARCHAR(128) NULL COMMENT 'PKCE verifier 包装密钥引用',
    issuer VARCHAR(512) NULL COMMENT '绑定 issuer',
    redirectUri VARCHAR(1024) NOT NULL COMMENT '绑定回调地址',
    expiresAt DATETIME NOT NULL COMMENT '过期时间',
    consumedAt DATETIME NULL COMMENT '消费时间',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 CONSUMED，2 EXPIRED',
    loginIp VARCHAR(64) NULL COMMENT '登录 IP',
    userAgent VARCHAR(1024) NULL COMMENT '用户代理',
    traceId VARCHAR(128) NULL COMMENT '追踪 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysExternalOAuthLoginState_stateId_deleted (stateId, isDeleted),
    KEY idx_sysExternalOAuthLoginState_stateHash_status_deleted (stateHash, status, isDeleted),
    KEY idx_sysExternalOAuthLoginState_provider_status_expire_deleted (providerCode, status, expiresAt, isDeleted)
) COMMENT='外部 OAuth 登录状态表';

CREATE TABLE IF NOT EXISTS sysExternalOAuthToken (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    providerCode VARCHAR(64) NOT NULL COMMENT '提供方编码',
    identityId BIGINT NOT NULL COMMENT '外部身份绑定 ID',
    userId BIGINT NOT NULL COMMENT '本地用户 ID',
    tokenPurpose VARCHAR(32) NOT NULL COMMENT '令牌用途：LOGIN/API',
    scopeJson JSON NULL COMMENT '授权范围 JSON',
    scopeHash VARCHAR(128) NOT NULL COMMENT '授权范围哈希',
    tokenSetCiphertext TEXT NOT NULL COMMENT '令牌集密文',
    tokenSetEdek TEXT NOT NULL COMMENT '令牌集 EDEK',
    tokenSetWrapKeyRef VARCHAR(128) NOT NULL COMMENT '令牌集包装密钥引用',
    accessExpiresAt DATETIME NULL COMMENT 'access token 过期时间',
    refreshExpiresAt DATETIME NULL COMMENT 'refresh token 过期时间',
    lastRefreshAt DATETIME NULL COMMENT '最近刷新时间',
    revokedAt DATETIME NULL COMMENT '撤销时间',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 REVOKED，2 EXPIRED，3 REFRESH_FAILED',
    version INT NOT NULL DEFAULT 0 COMMENT '乐观锁版本',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysExternalOAuthToken_identity_purpose_scope_deleted (identityId, tokenPurpose, scopeHash, isDeleted),
    KEY idx_sysExternalOAuthToken_provider_status_deleted (providerCode, status, isDeleted),
    KEY idx_sysExternalOAuthToken_user_status_deleted (userId, status, isDeleted)
) COMMENT='外部 OAuth 令牌保险库表';

-- +goose Down
DROP TABLE IF EXISTS sysExternalOAuthToken;
DROP TABLE IF EXISTS sysExternalOAuthLoginState;
DROP TABLE IF EXISTS sysExternalUserIdentity;
DROP TABLE IF EXISTS sysExternalProviderMethod;
DROP TABLE IF EXISTS sysExternalLoginProvider;

ALTER TABLE sysSsoSession
  DROP INDEX idx_sysSsoSession_external_provider_status_deleted,
  DROP INDEX idx_sysSsoSession_external_identity_status_deleted,
  DROP COLUMN loginMethod,
  DROP COLUMN externalProviderCode,
  DROP COLUMN externalIdentityId;
