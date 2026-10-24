-- +goose Up
ALTER TABLE sys_user
  ADD COLUMN statusCommandHash CHAR(64) NULL COMMENT '节点状态命令哈希' AFTER statusVersion;

-- +goose Down
ALTER TABLE sys_user
  DROP COLUMN statusCommandHash;
