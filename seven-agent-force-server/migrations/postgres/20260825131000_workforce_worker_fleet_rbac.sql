-- +goose Up
-- +goose StatementBegin
DO $workforce_worker_rbac$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.ordinal), candidate.code, candidate.name,
           'API', candidate.method, candidate.path, 0, candidate.description,
           CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513100000000) id FROM sys_permission) base
      CROSS JOIN (VALUES
        (1,'workforce:worker:create','创建Worker','POST','/workforce/workers','创建Worker并签发一次性机器凭证'),
        (2,'workforce:worker:list','Worker列表','GET','/workforce/workers','查询平台Worker'),
        (3,'workforce:worker:query','Worker详情','GET','/workforce/workers/{id}','查询Worker身份、资源、镜像和Assignment'),
        (4,'workforce:worker:operate','操作Worker','PUT','/workforce/workers/{id}/desired-state','Drain、启用或停用Worker'),
        (5,'workforce:worker:token:rotate','轮换Worker凭证','POST','/workforce/workers/{id}/token-rotations','撤销旧凭证并签发新凭证')
      ) candidate(ordinal,code,name,method,path,description)
     WHERE NOT EXISTS (SELECT 1 FROM sys_permission p WHERE p.code=candidate.code AND p."isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    INSERT INTO sys_menu (id,name,"parentId","sortOrder",path,component,icon,type,permission,
                          "isFrame","isCache",visible,hierarchy,level,status,
                          "creatorId","createTime","updaterId","updateTime","isDeleted",remark)
    SELECT base.id+1,'Worker',parent.id,40,'/workforce/workers','workforce/workers/index',
           'ClusterOutlined','C','workforce:worker:list',true,true,true,
           CONCAT(COALESCE(NULLIF(parent.hierarchy,''),'/1'),'/',base.id+1),parent.level+1,0,
           0,CURRENT_TIMESTAMP,0,CURRENT_TIMESTAMP,false,'Seven Workforce执行工作机管理'
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513100000100) id FROM sys_menu) base
      JOIN sys_menu parent ON parent.path='/system/workforce' AND parent."isDeleted"=false
     WHERE NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path='/workforce/workers' AND m."isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (id,"roleId","permissionId","creatorId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY role.id,permission.id),role.id,permission.id,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513100000200) id FROM sys_role_permission) base
      CROSS JOIN sys_role role
      JOIN sys_permission permission ON permission.code LIKE 'workforce:worker:%' AND permission."isDeleted"=false
     WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
       AND NOT EXISTS (SELECT 1 FROM sys_role_permission rp WHERE rp."roleId"=role.id AND rp."permissionId"=permission.id);
  END IF;

  IF to_regclass('public.sys_menu_permission') IS NOT NULL THEN
    INSERT INTO sys_menu_permission (id,"menuId","permissionId","creatorId","createTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY permission.id),menu.id,permission.id,0,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513100000250) id FROM sys_menu_permission) base
      JOIN sys_menu menu ON menu.path='/workforce/workers' AND menu."isDeleted"=false
      CROSS JOIN sys_permission permission
     WHERE permission.code LIKE 'workforce:worker:%' AND permission."isDeleted"=false
       AND NOT EXISTS (SELECT 1 FROM sys_menu_permission mp WHERE mp."menuId"=menu.id AND mp."permissionId"=permission.id);
  END IF;

  IF to_regclass('public.sys_role_menu') IS NOT NULL THEN
    INSERT INTO sys_role_menu (id,"roleId","menuId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY role.id),role.id,menu.id,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513100000300) id FROM sys_role_menu) base
      CROSS JOIN sys_role role
      JOIN sys_menu menu ON menu.path='/workforce/workers' AND menu."isDeleted"=false
     WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
       AND NOT EXISTS (SELECT 1 FROM sys_role_menu rm WHERE rm."roleId"=role.id AND rm."menuId"=menu.id);
  END IF;
END
$workforce_worker_rbac$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
