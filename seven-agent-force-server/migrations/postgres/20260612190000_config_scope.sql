-- +goose Up
CREATE TABLE IF NOT EXISTS sys_role_config_scope (
  id BIGINT PRIMARY KEY,
  "roleId" BIGINT NOT NULL,
  "groupCode" VARCHAR(64) NOT NULL,
  "configKey" VARCHAR(128) NOT NULL DEFAULT '',
  "canRead" SMALLINT NOT NULL DEFAULT 1,
  "canWrite" SMALLINT NOT NULL DEFAULT 0,
  "canDelete" SMALLINT NOT NULL DEFAULT 0,
  "createdBy" BIGINT,
  "createTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "updatedBy" BIGINT,
  "updateTime" TIMESTAMPTZ NOT NULL DEFAULT now(),
  "isDeleted" SMALLINT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS "uk_role_config_scope" ON sys_role_config_scope ("roleId", "groupCode", "configKey");
CREATE INDEX IF NOT EXISTS idx_role_config_scope_role ON sys_role_config_scope ("roleId", "isDeleted");
CREATE INDEX IF NOT EXISTS idx_role_config_scope_group ON sys_role_config_scope ("groupCode", "configKey", "isDeleted");

-- +goose StatementBegin
DO $config_scope_permissions$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
      SELECT v.id, v.code, v.name, 'API', v.method, v.path, 0, v.description, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
      FROM (VALUES
        (1900301060, 'system:config:scope:query', 'system config scope query', 'GET', '/config-scopes/roles/:roleId', 'system config scope query'),
        (1900301061, 'system:config:scope:assign', 'system config scope assign', 'POST', '/config-scopes/roles/:roleId', 'system config scope assign')
      ) AS v(id, code, name, method, path, description)
      WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = v.code AND existing."isDeleted" = 0)
    $sql$;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL AND to_regclass('public.sys_role') IS NOT NULL AND to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
      SELECT 190030106000 + r.id + p.id, r.id, p.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM sys_role r
      JOIN sys_permission p ON p."isDeleted" = 0 AND p.id BETWEEN 1900301060 AND 1900301061
      WHERE r."isDeleted" = 0 AND r.code = 'SUPER_ADMIN'
        AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing."roleId" = r.id AND existing."permissionId" = p.id)
    $sql$;
  END IF;
END
$config_scope_permissions$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $config_scope_permissions$
BEGIN
  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    EXECUTE 'DELETE FROM sys_role_permission WHERE "permissionId" BETWEEN 1900301060 AND 1900301061';
  END IF;
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE 'DELETE FROM sys_permission WHERE id BETWEEN 1900301060 AND 1900301061';
  END IF;
END
$config_scope_permissions$;
-- +goose StatementEnd

DROP TABLE IF EXISTS sys_role_config_scope;
