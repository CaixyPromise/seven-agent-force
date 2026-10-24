-- +goose Up
UPDATE sys_role
SET type = 1, updateTime = NOW()
WHERE code = 'SUPER_ADMIN'
  AND isDeleted = 0
  AND type = 0;

-- +goose Down
UPDATE sys_role
SET type = 0, updateTime = NOW()
WHERE code = 'SUPER_ADMIN'
  AND isDeleted = 0
  AND remark = 'RBAC admin seed role'
  AND type = 1;
