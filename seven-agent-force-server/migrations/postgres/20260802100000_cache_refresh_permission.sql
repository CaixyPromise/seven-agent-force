-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (
      id, code, name, "resourceType", method, path, status, description,
      "creatorId", "createTime", "updaterId", "updateTime", "isDeleted"
    ) VALUES
      (1900301068, 'system:cache:refresh', '刷新应用缓存', 'API', 'POST', '/system/cache/refresh', 0, '提交受治理的应用缓存刷新操作', 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, FALSE)
    ON CONFLICT DO NOTHING;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL
     AND to_regclass('public.sys_role') IS NOT NULL
     AND to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (
      id, "roleId", "permissionId", source, "creatorId", "createTime", "updateTime"
    )
    SELECT 190030106800 + r.id + p.id, r.id, p.id, 'DIRECT', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
    FROM sys_role r
    JOIN sys_permission p ON p.id = 1900301068 AND p."isDeleted" = FALSE
    WHERE r."systemKey" = 'AUTHORIZATION_ROOT' AND r."isDeleted" = FALSE
      AND NOT EXISTS (
        SELECT 1 FROM sys_role_permission existing
        WHERE existing."roleId" = r.id AND existing."permissionId" = p.id
      )
    ON CONFLICT DO NOTHING;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  -- Up uses ON CONFLICT DO NOTHING and does not record ownership of a
  -- pre-existing permission or grant. A generic Down would risk deleting an
  -- operator-managed authorization, so this migration is intentionally
  -- forward-only and requires an explicit audited removal procedure.
  RAISE EXCEPTION 'forward-only migration: cache refresh permission must not be removed automatically';
END $$;
-- +goose StatementEnd
