-- +goose Up
-- Goal-6.3 keeps normal delivery management content-free. Sensitive and
-- one-time content has a separate, audited diagnostic path; short-lived
-- secrets are never stored in the normal delivery payload or rendered fields.

DROP PROCEDURE IF EXISTS ensureNotificationDeliveryDiagnosticColumns;
-- +goose StatementBegin
CREATE PROCEDURE ensureNotificationDeliveryDiagnosticColumns()
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = 'sysNotificationDelivery'
          AND column_name = 'contentTier'
    ) THEN
        ALTER TABLE sysNotificationDelivery
            ADD COLUMN contentTier VARCHAR(32) NOT NULL DEFAULT 'SENSITIVE'
            COMMENT '内容敏感级别：PUBLIC、SENSITIVE或SECRET_EPHEMERAL'
            AFTER renderedMarkdown;
    END IF;
END
-- +goose StatementEnd
CALL ensureNotificationDeliveryDiagnosticColumns();
DROP PROCEDURE ensureNotificationDeliveryDiagnosticColumns;

CREATE TABLE IF NOT EXISTS sysNotificationDeliveryEphemeralContent (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    deliveryId VARCHAR(128) NOT NULL COMMENT '投递稳定标识',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    ciphertext MEDIUMTEXT NOT NULL COMMENT '短期内容密文',
    edek MEDIUMTEXT NOT NULL COMMENT '短期内容数据密钥密文',
    wrapKeyRef VARCHAR(128) NOT NULL COMMENT '主密钥引用',
    expiresAt DATETIME NOT NULL COMMENT '内容和投递的失效时间',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationDeliveryEphemeralContentDelivery (deliveryId),
    KEY idxNotificationDeliveryEphemeralContentScopeExpiry (scopeId, expiresAt)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知短期秘密加密内容';

CREATE TABLE IF NOT EXISTS sysNotificationDeliveryDiagnosticAudit (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    deliveryId VARCHAR(128) NOT NULL COMMENT '投递稳定标识',
    actorId BIGINT NOT NULL COMMENT '查看操作者',
    contentTier VARCHAR(32) NOT NULL COMMENT '内容敏感级别',
    reasonCode VARCHAR(32) NOT NULL COMMENT '查看原因枚举',
    ticketReference VARCHAR(128) NULL COMMENT '工单或事件引用',
    resultCode VARCHAR(48) NOT NULL COMMENT '允许、拒绝、过期或传输拒绝结果',
    traceId VARCHAR(128) NULL COMMENT '请求追踪标识',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    KEY idxNotificationDeliveryDiagnosticAuditScopeTime (scopeId, createTime),
    KEY idxNotificationDeliveryDiagnosticAuditDeliveryTime (deliveryId, createTime),
    KEY idxNotificationDeliveryDiagnosticAuditActorTime (actorId, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知投递内容诊断审计';

INSERT IGNORE INTO sys_permission (
    id, code, name, resourceType, method, path, status, description,
    creatorId, createTime, updaterId, updateTime, isDeleted
) VALUES
    (1900301064, 'system:notification:delivery:diagnostic', '诊断通知投递内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '逐条诊断通知投递内容，需原因、受保护连接和按内容级别授权', 0, NOW(), 0, NOW(), 0),
    (1900301065, 'system:notification:delivery:content:public', '查看公开通知内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '查看公开级别通知内容', 0, NOW(), 0, NOW(), 0),
    (1900301066, 'system:notification:delivery:content:sensitive', '查看敏感通知内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '查看敏感级别通知内容，需二次确认', 0, NOW(), 0, NOW(), 0),
    (1900301067, 'system:notification:delivery:content:secret-ephemeral', '查看短期秘密通知内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '查看未过期短期秘密，需二次确认', 0, NOW(), 0, NOW(), 0);

-- These capabilities are intentionally not granted to ordinary operators or
-- developers. The authorization root can delegate them explicitly only after
-- the organization has accepted the diagnostic policy.
INSERT IGNORE INTO sys_role_permission (
    id, roleId, permissionId, source, creatorId, createTime, updateTime
)
SELECT 190030106400 + r.id + p.id, r.id, p.id, 'DIRECT', 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.id BETWEEN 1900301064 AND 1900301067 AND p.isDeleted = 0
WHERE r.systemKey = 'AUTHORIZATION_ROOT' AND r.isDeleted = 0;

-- +goose Down
-- Forward-only: audit history and encrypted TTL envelopes are evidence for
-- delivery incidents and must not be deleted automatically.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification delivery diagnostics migration cannot be rolled back automatically';
