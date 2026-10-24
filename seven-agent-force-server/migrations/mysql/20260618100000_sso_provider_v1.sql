-- +goose Up
CREATE TABLE IF NOT EXISTS sysSsoClient (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    clientId VARCHAR(128) NOT NULL COMMENT '客户端标识',
    clientName VARCHAR(128) NOT NULL COMMENT '客户端名称',
    clientType VARCHAR(32) NOT NULL COMMENT '客户端类型',
    clientAuthMethod VARCHAR(32) NOT NULL DEFAULT 'none' COMMENT '客户端认证方式',
    grantTypesJson JSON NOT NULL COMMENT '授权模式 JSON',
    scopesJson JSON NOT NULL COMMENT '允许的 scope JSON',
    requirePkce TINYINT NOT NULL DEFAULT 1 COMMENT '是否强制 PKCE',
    requireConsent TINYINT NOT NULL DEFAULT 0 COMMENT '是否要求 consent',
    trustedFirstParty TINYINT NOT NULL DEFAULT 0 COMMENT '是否为可信首方客户端',
    accessTokenTtlSec INT NOT NULL DEFAULT 1800 COMMENT 'Access Token 有效期秒数',
    refreshTokenTtlSec INT NOT NULL DEFAULT 2592000 COMMENT 'Refresh Token 有效期秒数',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    metadataJson JSON NULL COMMENT '客户端元数据 JSON',
    creatorId BIGINT NULL COMMENT '创建人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updaterId BIGINT NULL COMMENT '更新人',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysSsoClient_clientId_deleted (clientId, isDeleted),
    KEY idx_sysSsoClient_type_status_deleted (clientType, status, isDeleted)
) COMMENT='SSO 客户端表';

CREATE TABLE IF NOT EXISTS sysSsoClientRedirectUri (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    clientId VARCHAR(128) NOT NULL COMMENT '客户端标识',
    redirectUri VARCHAR(512) NOT NULL COMMENT '登录回调地址',
    postLogoutRedirectUri VARCHAR(512) NULL COMMENT '登出回调地址',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    creatorId BIGINT NULL COMMENT '创建人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updaterId BIGINT NULL COMMENT '更新人',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysSsoClientRedirectUri_client_uri_deleted (clientId, redirectUri, isDeleted),
    KEY idx_sysSsoClientRedirectUri_client_status_deleted (clientId, status, isDeleted)
) COMMENT='SSO 客户端回调地址表';

CREATE TABLE IF NOT EXISTS sysSsoClientSecret (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    clientId VARCHAR(128) NOT NULL COMMENT '客户端标识',
    secretHash VARCHAR(255) NOT NULL COMMENT '客户端密钥哈希',
    secretHint VARCHAR(128) NULL COMMENT '密钥提示',
    expiresAt DATETIME NULL COMMENT '过期时间',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 DISABLED',
    creatorId BIGINT NULL COMMENT '创建人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updaterId BIGINT NULL COMMENT '更新人',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    KEY idx_sysSsoClientSecret_client_status_deleted (clientId, status, isDeleted)
) COMMENT='SSO 客户端密钥表';

CREATE TABLE IF NOT EXISTS sysSsoAuthorizationCode (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    code VARCHAR(255) NOT NULL COMMENT '授权码',
    clientId VARCHAR(128) NOT NULL COMMENT '客户端标识',
    userId BIGINT NOT NULL COMMENT '用户 ID',
    sessionId VARCHAR(128) NOT NULL COMMENT '会话 ID',
    redirectUri VARCHAR(512) NOT NULL COMMENT '回调地址',
    scopesJson JSON NOT NULL COMMENT '授权 scope JSON',
    codeChallenge VARCHAR(255) NULL COMMENT 'PKCE code challenge',
    codeChallengeMethod VARCHAR(32) NULL COMMENT 'PKCE challenge 方法',
    nonce VARCHAR(255) NULL COMMENT 'OIDC nonce',
    acr VARCHAR(64) NULL COMMENT '认证上下文级别',
    amrJson JSON NULL COMMENT '认证方式 JSON',
    expiresAt DATETIME NOT NULL COMMENT '过期时间',
    consumedAt DATETIME NULL COMMENT '消费时间',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 CONSUMED，2 EXPIRED',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysSsoAuthorizationCode_code_deleted (code, isDeleted),
    KEY idx_sysSsoAuthorizationCode_client_user_status (clientId, userId, status)
) COMMENT='SSO 授权码表';

CREATE TABLE IF NOT EXISTS sysSsoConsentGrant (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    userId BIGINT NOT NULL COMMENT '用户 ID',
    clientId VARCHAR(128) NOT NULL COMMENT '客户端标识',
    scopesJson JSON NOT NULL COMMENT '授权 scope JSON',
    grantedAt DATETIME NOT NULL COMMENT '授权时间',
    revokedAt DATETIME NULL COMMENT '撤销时间',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 REVOKED',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysSsoConsentGrant_user_client_deleted (userId, clientId, isDeleted)
) COMMENT='SSO consent 授权表';

CREATE TABLE IF NOT EXISTS sysSsoSession (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    sessionId VARCHAR(128) NOT NULL COMMENT '会话 ID',
    userId BIGINT NOT NULL COMMENT '用户 ID',
    clientId VARCHAR(128) NOT NULL COMMENT '客户端标识',
    deviceId VARCHAR(128) NULL COMMENT '设备 ID',
    loginIp VARCHAR(64) NULL COMMENT '登录 IP',
    userAgent VARCHAR(1024) NULL COMMENT '用户代理',
    acr VARCHAR(64) NULL COMMENT '认证上下文级别',
    amrJson JSON NULL COMMENT '认证方式 JSON',
    loginAt DATETIME NOT NULL COMMENT '登录时间',
    lastAccessAt DATETIME NULL COMMENT '最近访问时间',
    expiresAt DATETIME NOT NULL COMMENT '过期时间',
    revokedAt DATETIME NULL COMMENT '撤销时间',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 REVOKED，2 EXPIRED',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysSsoSession_sessionId_deleted (sessionId, isDeleted),
    KEY idx_sysSsoSession_user_status_deleted (userId, status, isDeleted)
) COMMENT='SSO 会话表';

CREATE TABLE IF NOT EXISTS sysSsoRefreshTokenFamily (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    familyId VARCHAR(128) NOT NULL COMMENT '刷新令牌族 ID',
    sessionId VARCHAR(128) NOT NULL COMMENT '会话 ID',
    clientId VARCHAR(128) NOT NULL COMMENT '客户端标识',
    userId BIGINT NOT NULL COMMENT '用户 ID',
    currentTokenHash VARCHAR(255) NOT NULL COMMENT '当前刷新令牌哈希',
    previousTokenHash VARCHAR(255) NULL COMMENT '上一枚刷新令牌哈希',
    reuseDetected TINYINT NOT NULL DEFAULT 0 COMMENT '是否检测到 reuse',
    rotatedAt DATETIME NULL COMMENT '最近轮转时间',
    expiresAt DATETIME NOT NULL COMMENT '过期时间',
    revokedAt DATETIME NULL COMMENT '撤销时间',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0 ACTIVE，1 REVOKED，2 EXPIRED',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysSsoRefreshTokenFamily_family_deleted (familyId, isDeleted),
    KEY idx_sysSsoRefreshTokenFamily_currentTokenHash_deleted (currentTokenHash, isDeleted)
) COMMENT='SSO 刷新令牌族表';

CREATE TABLE IF NOT EXISTS sysSsoIssuerKey (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    kid VARCHAR(128) NOT NULL COMMENT 'kid 标识',
    algorithm VARCHAR(32) NOT NULL COMMENT '签名算法',
    publicKeyPem TEXT NOT NULL COMMENT '公钥内容',
    privateKeyCiphertext TEXT NULL COMMENT '私钥密文',
    keyStatus VARCHAR(32) NOT NULL COMMENT '密钥状态：ACTIVE/NEXT/RETIRED',
    activateAt DATETIME NULL COMMENT '启用时间',
    retireAt DATETIME NULL COMMENT '退役时间',
    metadataJson JSON NULL COMMENT '扩展元数据 JSON',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sysSsoIssuerKey_kid_deleted (kid, isDeleted)
) COMMENT='SSO issuer 密钥表';

CREATE TABLE IF NOT EXISTS sysSsoAuditLog (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键标识',
    eventType VARCHAR(64) NOT NULL COMMENT '事件类型',
    clientId VARCHAR(128) NULL COMMENT '客户端标识',
    userId BIGINT NULL COMMENT '用户 ID',
    sessionId VARCHAR(128) NULL COMMENT '会话 ID',
    deviceId VARCHAR(128) NULL COMMENT '设备 ID',
    tenantId VARCHAR(128) NULL COMMENT '租户 ID',
    loginIp VARCHAR(64) NULL COMMENT '登录 IP',
    userAgent VARCHAR(1024) NULL COMMENT '用户代理',
    result VARCHAR(32) NOT NULL COMMENT '事件结果',
    reasonCode VARCHAR(64) NULL COMMENT '原因编码',
    detailJson JSON NULL COMMENT '事件详情 JSON',
    traceId VARCHAR(128) NULL COMMENT '追踪 ID',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    PRIMARY KEY (id),
    KEY idx_sysSsoAuditLog_trace_deleted (traceId, isDeleted)
) COMMENT='SSO 审计日志表';

-- +goose Down
DROP TABLE IF EXISTS sysSsoAuditLog;
DROP TABLE IF EXISTS sysSsoIssuerKey;
DROP TABLE IF EXISTS sysSsoRefreshTokenFamily;
DROP TABLE IF EXISTS sysSsoSession;
DROP TABLE IF EXISTS sysSsoConsentGrant;
DROP TABLE IF EXISTS sysSsoAuthorizationCode;
DROP TABLE IF EXISTS sysSsoClientSecret;
DROP TABLE IF EXISTS sysSsoClientRedirectUri;
DROP TABLE IF EXISTS sysSsoClient;
