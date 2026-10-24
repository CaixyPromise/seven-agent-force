-- +goose Up
ALTER TABLE sys_user_permission
  ADD COLUMN reason VARCHAR(500) NULL;

COMMENT ON COLUMN sys_user_permission.reason IS '临时权限变更原因';

-- +goose Down
ALTER TABLE sys_user_permission DROP COLUMN IF EXISTS reason;
