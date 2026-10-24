-- +goose Up
-- +goose StatementBegin
DO $workforce_open_integration_rbac$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.ordinal),candidate.code,candidate.name,
           'API',candidate.method,candidate.path,0,candidate.description,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026083010100000000) id FROM sys_permission) base
      CROSS JOIN (VALUES
        (1,'workforce:open-integration:read','查看开放接入','GET','/workforce/open-integrations','查看外部应用和安全投递摘要'),
        (2,'workforce:open-integration:create','创建开放接入','POST','/workforce/open-integrations','创建外部应用并一次性签发凭证'),
        (3,'workforce:open-integration:operate','操作开放接入','POST','/workforce/open-integrations/{id}/enable','启用或停用外部应用')
      ) candidate(ordinal,code,name,method,path,description)
     WHERE NOT EXISTS (SELECT 1 FROM sys_permission p WHERE p.code=candidate.code AND p."isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    INSERT INTO sys_menu (id,name,"parentId","sortOrder",path,component,icon,type,permission,
                          "isFrame","isCache",visible,hierarchy,level,status,
                          "creatorId","createTime","updaterId","updateTime","isDeleted",remark)
    SELECT base.id+1,'开放接入',parent.id,60,'/workforce/open-integrations','workforce/open-integrations/index',
           'ApiOutlined','C','workforce:open-integration:read',true,true,true,
           CONCAT(COALESCE(NULLIF(parent.hierarchy,''),'/1'),'/',base.id+1),parent.level+1,0,
           0,CURRENT_TIMESTAMP,0,CURRENT_TIMESTAMP,false,'管理外部应用和任务回调'
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026083010100000100) id FROM sys_menu) base
      JOIN sys_menu parent ON parent.path='/system/workforce' AND parent."isDeleted"=false
     WHERE NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path='/workforce/open-integrations' AND m."isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (id,"roleId","permissionId","creatorId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY role.id,permission.id),role.id,permission.id,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026083010100000200) id FROM sys_role_permission) base
      CROSS JOIN sys_role role
      JOIN sys_permission permission ON permission.code LIKE 'workforce:open-integration:%' AND permission."isDeleted"=false
     WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
       AND NOT EXISTS (SELECT 1 FROM sys_role_permission rp WHERE rp."roleId"=role.id AND rp."permissionId"=permission.id);
  END IF;

  IF to_regclass('public.sys_menu_permission') IS NOT NULL THEN
    INSERT INTO sys_menu_permission (id,"menuId","permissionId","creatorId","createTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY permission.id),menu.id,permission.id,0,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026083010100000250) id FROM sys_menu_permission) base
      JOIN sys_menu menu ON menu.path='/workforce/open-integrations' AND menu."isDeleted"=false
      CROSS JOIN sys_permission permission
     WHERE permission.code LIKE 'workforce:open-integration:%' AND permission."isDeleted"=false
       AND NOT EXISTS (SELECT 1 FROM sys_menu_permission mp WHERE mp."menuId"=menu.id AND mp."permissionId"=permission.id);
  END IF;

  IF to_regclass('public.sys_role_menu') IS NOT NULL THEN
    INSERT INTO sys_role_menu (id,"roleId","menuId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY role.id),role.id,menu.id,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026083010100000300) id FROM sys_role_menu) base
      CROSS JOIN sys_role role
      JOIN sys_menu menu ON menu.path='/workforce/open-integrations' AND menu."isDeleted"=false
     WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
       AND NOT EXISTS (SELECT 1 FROM sys_role_menu rm WHERE rm."roleId"=role.id AND rm."menuId"=menu.id);
  END IF;
END
$workforce_open_integration_rbac$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
