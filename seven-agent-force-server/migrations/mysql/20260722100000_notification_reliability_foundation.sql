-- +goose Up
-- Shared outbox events are consumed by multiple modules. Persist the owner and
-- lease/fencing fields so a relay cannot claim or complete another module's
-- event, including after a stale worker is reclaimed.
ALTER TABLE sys_outbox_event
  ADD COLUMN eventOwner VARCHAR(64) NOT NULL DEFAULT 'unassigned' COMMENT 'Outbox事件归属模块' AFTER eventId,
  ADD COLUMN leaseOwner VARCHAR(128) DEFAULT NULL COMMENT '当前租约工作者' AFTER errorMsg,
  ADD COLUMN leaseToken VARCHAR(64) DEFAULT NULL COMMENT '当前租约围栏令牌' AFTER leaseOwner,
  ADD COLUMN leaseUntil DATETIME DEFAULT NULL COMMENT '当前租约到期时间' AFTER leaseToken;

ALTER TABLE sys_message_consume_log
  ADD COLUMN leaseOwner VARCHAR(128) DEFAULT NULL COMMENT '当前消费租约工作者' AFTER detail,
  ADD COLUMN leaseToken VARCHAR(64) DEFAULT NULL COMMENT '当前消费租约围栏令牌' AFTER leaseOwner,
  ADD COLUMN leaseUntil DATETIME DEFAULT NULL COMMENT '当前消费租约到期时间' AFTER leaseToken,
  ADD COLUMN updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间' AFTER createTime;

-- Only known historical event types are assigned automatically. Unknown rows
-- remain unassigned and are intentionally not eligible for a module relay.
UPDATE sys_outbox_event
SET eventOwner = CASE
  WHEN eventType IN ('UPLOAD_TASK_READY', 'UPLOAD_TASK_UPLOADED', 'FILE_PROCESS_TASK') THEN 'file'
  WHEN eventType = 'notification.dispatch' THEN 'notification'
  ELSE 'unassigned'
END
WHERE eventOwner = 'unassigned';

CREATE INDEX idx_outbox_owner_type_status_retry
  ON sys_outbox_event (eventOwner, eventType, status, nextRetryAt, leaseUntil, createTime);

CREATE INDEX idx_message_consume_status_lease
  ON sys_message_consume_log (status, leaseUntil);

-- +goose Down
DROP INDEX idx_message_consume_status_lease ON sys_message_consume_log;
DROP INDEX idx_outbox_owner_type_status_retry ON sys_outbox_event;

ALTER TABLE sys_message_consume_log
  DROP COLUMN updateTime,
  DROP COLUMN leaseUntil,
  DROP COLUMN leaseToken,
  DROP COLUMN leaseOwner;

ALTER TABLE sys_outbox_event
  DROP COLUMN leaseUntil,
  DROP COLUMN leaseToken,
  DROP COLUMN leaseOwner,
  DROP COLUMN eventOwner;
