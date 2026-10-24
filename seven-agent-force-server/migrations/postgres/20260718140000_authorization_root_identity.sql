-- +goose Up
ALTER TABLE sys_role ADD COLUMN "systemKey" VARCHAR(64);
CREATE UNIQUE INDEX uk_sys_role_system_key ON sys_role ("systemKey");

UPDATE sys_role
SET "systemKey" = 'AUTHORIZATION_ROOT',
    "updateTime" = CURRENT_TIMESTAMP
WHERE code = 'SUPER_ADMIN' AND "isDeleted" = 0;

-- +goose StatementBegin
DO $authorization_root_guard$
BEGIN
  IF (SELECT COUNT(1) FROM sys_role WHERE "systemKey" = 'AUTHORIZATION_ROOT' AND "isDeleted" = 0) <> 1 THEN
    RAISE EXCEPTION 'RBAC_ROOT_INVALID_COUNT';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM sys_post_role spr
    JOIN sys_role sr ON sr.id = spr."roleId"
    WHERE sr."systemKey" = 'AUTHORIZATION_ROOT'
  ) THEN
    RAISE EXCEPTION 'RBAC_ROOT_POST_RELATION_EXISTS';
  END IF;
END
$authorization_root_guard$;
-- +goose StatementEnd

CREATE TABLE sys_security_bootstrap (
  "bootstrapKey" VARCHAR(64) PRIMARY KEY,
  "rootRoleId" BIGINT NOT NULL UNIQUE,
  "rootRoleCode" VARCHAR(50) NOT NULL,
  "initializedAt" TIMESTAMPTZ NOT NULL,
  "createTime" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updateTime" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO sys_security_bootstrap ("bootstrapKey", "rootRoleId", "rootRoleCode", "initializedAt", "createTime", "updateTime")
SELECT 'AUTHORIZATION_ROOT', sr.id, sr.code, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM sys_role sr
WHERE sr."systemKey" = 'AUTHORIZATION_ROOT'
  AND sr."isDeleted" = 0
  AND EXISTS (SELECT 1 FROM sys_user su WHERE su."isDeleted" = 0)
ON CONFLICT ("bootstrapKey") DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS sys_security_bootstrap;
DROP INDEX IF EXISTS uk_sys_role_system_key;
ALTER TABLE sys_role DROP COLUMN IF EXISTS "systemKey";
