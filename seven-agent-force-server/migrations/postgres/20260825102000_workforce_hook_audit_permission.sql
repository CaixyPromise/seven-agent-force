-- +goose Up
-- +goose StatementBegin
DO $managed_hooks_audit_rbac$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted")
    SELECT GREATEST(COALESCE(MAX(id),0),2026082510200000000)+1,
           'workforce:hook-audit:list','Hook审计','API','GET','/workforce/hook-audit',0,
           '查看Hook发布、测试、Admission与绑定事实',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false
      FROM sys_permission
     WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code='workforce:hook-audit:list' AND "isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (id,"roleId","permissionId","creatorId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY role.id),role.id,permission.id,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082510200001000) id FROM sys_role_permission) base
      CROSS JOIN sys_role role
      JOIN sys_permission permission ON permission.code='workforce:hook-audit:list' AND permission."isDeleted"=false
     WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
       AND NOT EXISTS (SELECT 1 FROM sys_role_permission rp WHERE rp."roleId"=role.id AND rp."permissionId"=permission.id);
  END IF;
END
$managed_hooks_audit_rbac$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
