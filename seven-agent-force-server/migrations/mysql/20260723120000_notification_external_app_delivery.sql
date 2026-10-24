-- +goose Up
-- Goal-4 is additive. A third-party enterprise member is not a platform user,
-- so its encrypted delivery target and provider attempt history live outside
-- the inbox-recipient projection.

ALTER TABLE sysNotificationChannel
    ADD COLUMN scopeId VARCHAR(128) NULL COMMENT '企业应用连接所属部署、Hub或Node作用域' AFTER channelType,
    ADD COLUMN scopeType VARCHAR(32) NOT NULL DEFAULT 'SYSTEM' COMMENT '系统或组织范围',
    ADD COLUMN organizationId BIGINT NULL COMMENT '组织范围身份',
    ADD COLUMN enterpriseConnectionId BIGINT NULL COMMENT '企业身份连接',
    ADD COLUMN enterpriseConnectionVersion BIGINT NOT NULL DEFAULT 0 COMMENT '冻结连接版本',
    ADD COLUMN enterpriseConnectionDigest CHAR(64) NOT NULL DEFAULT '' COMMENT '冻结连接摘要',
    ADD KEY idxNotificationChannelOrganization (organizationId, channelType, status);

ALTER TABLE sysNotificationDelivery
    ADD COLUMN organizationId BIGINT NULL COMMENT '组织身份边界' AFTER requestDigest,
    ADD COLUMN notificationId BIGINT NULL COMMENT '语义逻辑通知内部标识' AFTER organizationId,
    ADD COLUMN externalTargetId BIGINT NULL COMMENT '加密第三方目标快照内部标识' AFTER notificationId,
    ADD COLUMN providerReference VARCHAR(191) NULL COMMENT '脱敏平台受理引用' AFTER lastError,
    ADD COLUMN presentationType VARCHAR(32) NOT NULL DEFAULT 'TEXT' COMMENT '冻结渠道表现',
    ADD COLUMN actionLabelSnapshot VARCHAR(128) NOT NULL DEFAULT '' COMMENT '冻结动作文案',
    ADD COLUMN actionUrlSnapshot VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '冻结外部动作地址',
    ADD COLUMN renderedPayloadDigest CHAR(64) NOT NULL DEFAULT '' COMMENT '冻结渠道载荷摘要';

CREATE INDEX idxNotificationDeliveryNotification
    ON sysNotificationDelivery (notificationId, createTime);
CREATE INDEX idxNotificationDeliveryExternalTarget
    ON sysNotificationDelivery (externalTargetId, createTime);

CREATE TABLE IF NOT EXISTS sysNotificationExternalTarget (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    externalTargetId VARCHAR(96) NOT NULL COMMENT '外部目标诊断标识',
    notificationId BIGINT NOT NULL COMMENT '逻辑通知内部标识',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    organizationId BIGINT NULL COMMENT '组织身份边界',
    userId BIGINT NULL COMMENT '平台用户',
    externalIdentityId BIGINT NULL COMMENT '企业外部身份',
    externalIdentityVersion BIGINT NOT NULL DEFAULT 0 COMMENT '冻结身份版本',
    enterpriseConnectionId BIGINT NULL COMMENT '企业身份连接',
    enterpriseConnectionVersion BIGINT NOT NULL DEFAULT 0 COMMENT '冻结连接版本',
    authorityDigest VARCHAR(128) NOT NULL DEFAULT '' COMMENT '冻结授权摘要',
    grantType VARCHAR(32) NOT NULL DEFAULT '' COMMENT '冻结授权类型',
    connectionRef VARCHAR(64) NOT NULL COMMENT '企业应用连接编码',
    providerCode VARCHAR(32) NOT NULL COMMENT '企业应用提供方编码',
    identityKind VARCHAR(32) NOT NULL COMMENT '第三方成员身份类型',
    subjectCiphertext TEXT NOT NULL COMMENT '加密第三方成员标识',
    subjectEdek TEXT NOT NULL COMMENT '成员标识数据密钥信封',
    subjectWrapKeyRef VARCHAR(191) NOT NULL COMMENT '成员标识密钥版本',
    subjectDigest CHAR(64) NOT NULL COMMENT '带密钥的成员标识摘要',
    subjectDigestKeyRef VARCHAR(191) NOT NULL COMMENT '成员摘要密钥版本',
    providerParamsJson JSON NULL COMMENT '已解析可选参数不可变快照',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationExternalTargetId (externalTargetId),
    UNIQUE KEY ukNotificationExternalTargetSemantic (notificationId, connectionRef, identityKind, subjectDigest),
    KEY idxNotificationExternalTargetNotification (notificationId, createTime),
    KEY idxNotificationExternalTargetScopeConnection (scopeId, connectionRef, createTime),
    KEY idxNotificationExternalTargetAuthority (organizationId, externalIdentityId, enterpriseConnectionId, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='第三方企业应用加密投递目标';

ALTER TABLE sysNotification
    ADD COLUMN organizationId BIGINT NULL COMMENT '组织身份边界' AFTER scopeId;

CREATE TABLE IF NOT EXISTS sysNotificationDeliveryAttempt (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    attemptId VARCHAR(96) NOT NULL COMMENT '投递尝试诊断标识',
    deliveryId VARCHAR(96) NOT NULL COMMENT '投递标识',
    attemptNo INT NOT NULL COMMENT '尝试序号',
    status VARCHAR(32) NOT NULL COMMENT '受理、失败或不确定状态',
    failureClass VARCHAR(64) NULL COMMENT '脱敏失败分类',
    providerReference VARCHAR(191) NULL COMMENT '脱敏平台受理引用',
    diagnostic VARCHAR(128) NULL COMMENT '稳定脱敏诊断代码',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    UNIQUE KEY ukNotificationDeliveryAttemptId (attemptId),
    UNIQUE KEY ukNotificationDeliveryAttemptNumber (deliveryId, attemptNo),
    KEY idxNotificationDeliveryAttemptDelivery (deliveryId, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='第三方企业应用投递尝试';

-- +goose Down
-- Forward-only: external delivery audit records and encrypted targets are not
-- dropped automatically. Failing here keeps Goose at the applied version so a
-- later Up cannot replay the non-idempotent ALTER TABLE statements.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification external delivery migration cannot be rolled back automatically';
