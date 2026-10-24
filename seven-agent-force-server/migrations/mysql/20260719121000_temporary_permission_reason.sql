-- +goose Up
ALTER TABLE sys_user_permission
  ADD COLUMN reason VARCHAR(500) NULL COMMENT '临时权限变更原因' AFTER source;

-- +goose Down
ALTER TABLE sys_user_permission DROP COLUMN reason;
