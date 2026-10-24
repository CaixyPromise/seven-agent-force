-- +goose Up
-- Expiry keeps the recipient audit row but creates one durable visible-state
-- transition so open inbox clients can remove an item through mailbox delta.

ALTER TABLE sysNotificationRecipient
    ADD COLUMN expiredAt DATETIME NULL COMMENT '收件箱可见性到期处理时间' AFTER expiresAt;

CREATE INDEX idxNotificationRecipientExpiry
    ON sysNotificationRecipient (scopeId, expiredAt, expiresAt, id);

-- +goose Down
-- Forward-only: retain expiry audit data and disable the worker before any
-- manually reviewed rollback. Failing here is deliberate: a successful no-op
-- Down would make Goose lower its recorded version and then re-run the
-- non-idempotent ALTER TABLE on the next Up.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification expiry migration cannot be rolled back automatically';
