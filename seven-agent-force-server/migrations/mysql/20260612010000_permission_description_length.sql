-- +goose Up
-- Keep live legacy schemas aligned with the canonical sys_permission
-- definition introduced by 20260424090000_system_user_admin.sql.
ALTER TABLE sys_permission MODIFY COLUMN description VARCHAR(255) DEFAULT NULL;

-- +goose Down
-- Intentionally no-op: shrinking this column can fail or truncate operator
-- authored permission descriptions.
