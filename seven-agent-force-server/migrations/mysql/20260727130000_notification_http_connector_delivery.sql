-- +goose Up
-- Goal-5.2 stores one immutable operator-selected HTTP connection revision
-- beside each accepted delivery. The row contains no plaintext credential;
-- webhook URLs and signing material remain inside the encrypted envelope.

CREATE TABLE IF NOT EXISTS sysNotificationHTTPDeliverySnapshot (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    deliveryId VARCHAR(96) NOT NULL COMMENT '投递标识',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    channelCode VARCHAR(64) NOT NULL COMMENT '静态连接编码',
    channelType VARCHAR(32) NOT NULL COMMENT 'HTTP或固定群机器人渠道类型',
    channelPriority INT NOT NULL DEFAULT 100 COMMENT '连接优先级快照',
    configJson JSON NOT NULL COMMENT '受控非密钥请求配置快照',
    secretCiphertext TEXT NULL COMMENT '加密连接密钥或Webhook信封',
    secretEdek TEXT NULL COMMENT '连接密钥数据密钥信封',
    secretWrapKeyRef VARCHAR(191) NULL COMMENT '连接密钥包装密钥版本',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationHTTPDeliverySnapshotDelivery (deliveryId),
    KEY idxNotificationHTTPDeliverySnapshotScopeChannel (scopeId, channelCode, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='HTTP连接投递不可变快照';

-- +goose Down
-- Forward-only: accepted delivery evidence and encrypted connection revisions
-- are retained for audit and must not be dropped automatically.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification HTTP connector migration cannot be rolled back automatically';
