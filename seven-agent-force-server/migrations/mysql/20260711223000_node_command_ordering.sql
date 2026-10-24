-- +goose Up
ALTER TABLE sys_user
  ADD COLUMN statusVersion BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '用户状态单调版本' AFTER status;

ALTER TABLE sysSsoRefreshTokenFamily
  ADD INDEX idx_sysSsoRefreshTokenFamily_user_status_deleted_createTime (userId, status, isDeleted, createTime);

-- +goose Down
ALTER TABLE sysSsoRefreshTokenFamily
  DROP INDEX idx_sysSsoRefreshTokenFamily_user_status_deleted_createTime;

ALTER TABLE sys_user
  DROP COLUMN statusVersion;
