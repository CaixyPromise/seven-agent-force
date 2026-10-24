-- +goose Up
-- +goose StatementBegin
DO $rbac$
BEGIN
  IF to_regclass('public.sys_role') IS NOT NULL THEN
    EXECUTE 'UPDATE sys_role SET type = 1, "updateTime" = CURRENT_TIMESTAMP WHERE code = ''SUPER_ADMIN'' AND "isDeleted" = 0 AND type = 0';
  END IF;
END
$rbac$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $rbac$
BEGIN
  IF to_regclass('public.sys_role') IS NOT NULL THEN
    EXECUTE 'UPDATE sys_role SET type = 0, "updateTime" = CURRENT_TIMESTAMP WHERE code = ''SUPER_ADMIN'' AND "isDeleted" = 0 AND remark = ''RBAC admin seed role'' AND type = 1';
  END IF;
END
$rbac$;
-- +goose StatementEnd
