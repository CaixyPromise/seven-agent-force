-- +goose Up
-- Goal-3 adds a per-user mailbox sequence. Existing recipient rows are kept;
-- their prior global IDs are deterministically reseeded before delta routes are
-- enabled so the new sequence is never inferred from a global ID generator.

CREATE TABLE IF NOT EXISTS sysNotificationMailbox (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    userId BIGINT NOT NULL COMMENT '收件用户标识',
    mailboxKey VARCHAR(96) NOT NULL COMMENT '非授权收件箱缓存标识',
    changeSequence BIGINT NOT NULL DEFAULT 0 COMMENT '收件箱严格串行变更序列',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationMailboxOwner (scopeId, userId),
    UNIQUE KEY ukNotificationMailboxKey (mailboxKey)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户收件箱同步水位';

-- The nested derived table keeps this compatible with MySQL's target-table
-- update restriction while assigning deterministic values to historical rows.
UPDATE sysNotificationRecipient AS recipient
JOIN (
    SELECT ranked.id, ranked.sequenceValue
    FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY scopeId, userId ORDER BY createTime ASC, id ASC) AS sequenceValue
        FROM sysNotificationRecipient
    ) AS ranked
) AS seeded ON seeded.id = recipient.id
SET recipient.mailboxVersion = seeded.sequenceValue;

INSERT INTO sysNotificationMailbox (scopeId, userId, mailboxKey, changeSequence)
SELECT seeded.scopeId,
       seeded.userId,
       CONCAT('mbx_', SHA2(CONCAT(seeded.scopeId, ':', seeded.userId), 256)),
       seeded.changeSequence
FROM (
    SELECT scopeId, userId, MAX(mailboxVersion) AS changeSequence
    FROM sysNotificationRecipient
    GROUP BY scopeId, userId
) AS seeded
ON DUPLICATE KEY UPDATE
    changeSequence = GREATEST(sysNotificationMailbox.changeSequence, VALUES(changeSequence)),
    updateTime = CURRENT_TIMESTAMP;

-- +goose Down
-- Forward-only: disabling the inbox synchronization feature is safe, but
-- mailbox audit state is not automatically deleted.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification mailbox sync migration cannot be rolled back automatically';
