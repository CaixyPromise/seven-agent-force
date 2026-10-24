-- +goose Up
ALTER TABLE sysExternalOAuthLoginState
    ADD COLUMN bindUserId BIGINT NULL COMMENT '主动绑定当前用户ID' AFTER redirectAfterLogin;

-- +goose Down
ALTER TABLE sysExternalOAuthLoginState
    DROP COLUMN bindUserId;
