-- +goose Up
ALTER TABLE sys_role
  ADD COLUMN "grantRevision" BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN sys_role."grantRevision" IS '角色授权快照修订号';

CREATE TABLE sys_role_grant_request (
  "roleId" BIGINT NOT NULL,
  "idempotencyKey" VARCHAR(128) NOT NULL,
  "requestHash" CHAR(64) NOT NULL,
  "resultRevision" BIGINT NOT NULL,
  "impactedUserCount" INTEGER NOT NULL DEFAULT 0,
  changed SMALLINT NOT NULL DEFAULT 0,
  "operatorId" BIGINT NULL,
  "createTime" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("roleId", "idempotencyKey")
);

CREATE INDEX idx_sys_role_grant_request_create_time
  ON sys_role_grant_request ("createTime");

-- +goose Down
DROP TABLE IF EXISTS sys_role_grant_request;

ALTER TABLE sys_role DROP COLUMN IF EXISTS "grantRevision";
