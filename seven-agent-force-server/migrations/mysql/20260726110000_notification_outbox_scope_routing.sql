-- +goose Up
-- Notification Outbox ownership has two dimensions: module and stable
-- installation/Hub/Node scope. Filtering scope in SQL before LIMIT prevents a
-- foreign backlog from being claimed or starving the local relay.
--
-- Historical rows intentionally remain NULL. Runtime compatibility lets only
-- the legacy local scope drain them; a Hub/Node must not start consuming until
-- its historical rows have been drained or explicitly attributed.
ALTER TABLE sys_outbox_event
  ADD COLUMN scopeId VARCHAR(128) DEFAULT NULL COMMENT '通知事件所属范围' AFTER eventOwner;

CREATE INDEX idx_outbox_owner_scope_type_status_retry
  ON sys_outbox_event (eventOwner, scopeId, eventType, status, nextRetryAt, leaseUntil, createTime);

-- +goose Down
DROP INDEX idx_outbox_owner_scope_type_status_retry ON sys_outbox_event;

ALTER TABLE sys_outbox_event
  DROP COLUMN scopeId;
