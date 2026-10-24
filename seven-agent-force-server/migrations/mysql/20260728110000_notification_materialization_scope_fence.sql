-- +goose Up
-- Scope is the leading selector for every materialization worker claim. Keep a
-- dedicated index for already-upgraded databases without rewriting audit data.
CREATE INDEX idxNotificationMaterializationScopeReady
    ON sysNotificationMaterializationTask (scopeId, status, nextRunAt, leaseUntil, createTime);

-- +goose Down
DROP INDEX idxNotificationMaterializationScopeReady
    ON sysNotificationMaterializationTask;
