-- +goose Up
-- Keep live legacy schemas aligned with the canonical sys_permission
-- definition introduced by 20260424090000_system_user_admin.sql.
-- +goose StatementBegin
DO $permission_description_length$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    ALTER TABLE sys_permission ALTER COLUMN description TYPE VARCHAR(255);
  END IF;
END
$permission_description_length$;
-- +goose StatementEnd

-- +goose Down
-- Intentionally no-op: shrinking this column can fail or truncate operator
-- authored permission descriptions.
