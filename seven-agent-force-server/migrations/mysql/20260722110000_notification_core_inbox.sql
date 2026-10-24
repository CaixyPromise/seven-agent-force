-- +goose Up
-- Goal-2 is additive: V1 delivery records remain external-delivery history and
-- are never inferred as user inbox messages.

CREATE TABLE IF NOT EXISTS sysNotification (
    id BIGINT NOT NULL PRIMARY KEY,
    notificationId VARCHAR(96) NOT NULL COMMENT '逻辑通知外部标识',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    eventKey VARCHAR(128) NOT NULL COMMENT '业务事件键',
    idempotencyKey VARCHAR(191) NOT NULL COMMENT '业务幂等键',
    requestFingerprint CHAR(64) NOT NULL COMMENT '规范化请求SHA-256摘要',
    audienceJson JSON NOT NULL COMMENT '用户和角色受众快照',
    category VARCHAR(64) NOT NULL COMMENT '通知分类',
    priority VARCHAR(32) NOT NULL COMMENT '业务优先级',
    mandatory TINYINT NOT NULL DEFAULT 0 COMMENT '是否强制通知',
    title VARCHAR(512) NOT NULL COMMENT '不可变标题快照',
    content TEXT NOT NULL COMMENT '不可变纯文本正文快照',
    deepLink VARCHAR(512) NULL COMMENT '受限站内深链',
    actionType VARCHAR(32) NOT NULL DEFAULT '' COMMENT '冻结动作类型',
    actionLabelSnapshot VARCHAR(128) NOT NULL DEFAULT '' COMMENT '冻结动作文案',
    actionStyle VARCHAR(32) NOT NULL DEFAULT '' COMMENT '冻结动作样式',
    scheduleAt DATETIME NULL COMMENT '延迟物化时间',
    expiresAt DATETIME NULL COMMENT '过期时间',
    traceId VARCHAR(128) NULL COMMENT '链路追踪标识',
    status VARCHAR(32) NOT NULL COMMENT '逻辑通知状态',
    creatorId BIGINT NULL COMMENT '业务创建人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationExternalId (notificationId),
    UNIQUE KEY ukNotificationIdempotency (scopeId, eventKey, idempotencyKey),
    KEY idxNotificationScopeStatusTime (scopeId, status, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='逻辑通知';

CREATE TABLE IF NOT EXISTS sysNotificationRecipient (
    id BIGINT NOT NULL PRIMARY KEY,
    recipientId VARCHAR(96) NOT NULL COMMENT '收件人投影外部标识',
    notificationId BIGINT NOT NULL COMMENT '逻辑通知内部标识',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    userId BIGINT NOT NULL COMMENT '收件用户标识',
    eventKey VARCHAR(128) NOT NULL COMMENT '业务事件键快照',
    category VARCHAR(64) NOT NULL COMMENT '通知分类快照',
    priority VARCHAR(32) NOT NULL COMMENT '业务优先级快照',
    mandatory TINYINT NOT NULL DEFAULT 0 COMMENT '是否强制通知',
    title VARCHAR(512) NOT NULL COMMENT '收件人标题快照',
    content TEXT NOT NULL COMMENT '收件人纯文本正文快照',
    deepLink VARCHAR(512) NULL COMMENT '收件人站内深链快照',
    actionType VARCHAR(32) NOT NULL DEFAULT '' COMMENT '冻结动作类型',
    actionLabelSnapshot VARCHAR(128) NOT NULL DEFAULT '' COMMENT '冻结动作文案',
    actionStyle VARCHAR(32) NOT NULL DEFAULT '' COMMENT '冻结动作样式',
    expiresAt DATETIME NULL COMMENT '过期时间',
    firstSeenAt DATETIME NULL COMMENT '首次可见时间',
    readAt DATETIME NULL COMMENT '阅读时间',
    archivedAt DATETIME NULL COMMENT '归档时间',
    mailboxVersion BIGINT NOT NULL COMMENT '收件箱可见状态版本',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationRecipientId (recipientId),
    UNIQUE KEY ukNotificationRecipientUser (notificationId, userId),
    KEY idxNotificationRecipientInboxPage (scopeId, userId, archivedAt, createTime, id),
    KEY idxNotificationRecipientUnread (scopeId, userId, archivedAt, readAt, expiresAt),
    KEY idxNotificationRecipientMailboxVersion (scopeId, userId, mailboxVersion)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户收件箱投影';

CREATE TABLE IF NOT EXISTS sysNotificationMaterializationTask (
    id BIGINT NOT NULL PRIMARY KEY,
    taskId VARCHAR(96) NOT NULL COMMENT '物化任务外部标识',
    notificationId BIGINT NOT NULL COMMENT '逻辑通知内部标识',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    audienceJson JSON NOT NULL COMMENT '规范化受众快照',
    materializationCursor VARCHAR(512) NOT NULL COMMENT '可恢复物化游标',
    status VARCHAR(32) NOT NULL COMMENT '任务状态',
    materializedCount BIGINT NOT NULL DEFAULT 0 COMMENT '已物化收件人计数',
    retryCount INT NOT NULL DEFAULT 0 COMMENT '失败或重领次数',
    nextRunAt DATETIME NOT NULL COMMENT '下次可执行时间',
    leaseOwner VARCHAR(128) NULL COMMENT '当前任务租约工作者',
    leaseToken VARCHAR(64) NULL COMMENT '当前任务围栏令牌',
    leaseUntil DATETIME NULL COMMENT '当前任务租约到期时间',
    lastError TEXT NULL COMMENT '最近物化错误',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationMaterializationTaskId (taskId),
    UNIQUE KEY ukNotificationMaterializationNotification (notificationId),
    KEY idxNotificationMaterializationReady (status, nextRunAt, leaseUntil, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知受众物化任务';

-- +goose Down
-- Forward-only migration: logical notifications and inbox audit history are not
-- dropped automatically. Disable the feature before a manual rollback plan.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification core inbox migration cannot be rolled back automatically';
