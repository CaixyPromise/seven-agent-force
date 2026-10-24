-- +goose Up
ALTER TABLE sysExternalLoginProvider
    ADD COLUMN accountAutoCreateEnabled TINYINT NOT NULL DEFAULT 0 COMMENT '是否允许 verified email 自动创建本地用户' AFTER emailAutoBindEnabled;

-- +goose Down
ALTER TABLE sysExternalLoginProvider
    DROP COLUMN accountAutoCreateEnabled;
