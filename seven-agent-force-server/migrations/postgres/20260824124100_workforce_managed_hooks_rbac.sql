-- +goose Up
-- +goose StatementBegin
DO $managed_hooks_rbac$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.ordinal), candidate.code, candidate.name, 'API', candidate.method, candidate.path, 0, candidate.description, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false
    FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082412410000000) id FROM sys_permission) base
    CROSS JOIN (VALUES
      (1,'workforce:hook-package:list','Hook列表','GET','/workforce/hook-packages','查询Hook Package'),
      (2,'workforce:hook-package:query','Hook详情','GET','/workforce/hook-packages/{id}','查询Hook Package详情'),
      (3,'workforce:hook-package:create','创建Hook','POST','/workforce/hook-packages','创建Hook Package'),
      (4,'workforce:hook-package:update','编辑Hook','PUT','/workforce/hook-revisions/{id}','编辑Hook草稿'),
      (5,'workforce:hook-package:validate','验证Hook','POST','/workforce/hook-revisions/{id}/validate','验证Hook草稿'),
      (6,'workforce:hook-package:test','测试Hook','POST','/workforce/hook-revisions/{id}/test-runs','运行Hook Fixture测试'),
      (7,'workforce:hook-package:publish','发布Hook','POST','/workforce/hook-revisions/{id}/publish','发布不可变Hook版本')
    ) candidate(ordinal,code,name,method,path,description)
    WHERE NOT EXISTS (SELECT 1 FROM sys_permission p WHERE p.code=candidate.code AND p."isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    INSERT INTO sys_menu (id,name,"parentId","sortOrder",path,component,icon,type,permission,"isFrame","isCache",visible,hierarchy,level,status,"creatorId","createTime","updaterId","updateTime","isDeleted",remark)
    SELECT base.id+candidate.ordinal,candidate.name,parent.id,candidate.sort_order,candidate.path,candidate.component,candidate.icon,'C',candidate.permission,true,true,true,CONCAT(COALESCE(NULLIF(parent.hierarchy,''),'/1'),'/',base.id+candidate.ordinal),parent.level+1,0,0,CURRENT_TIMESTAMP,0,CURRENT_TIMESTAMP,false,candidate.remark
    FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082412410001000) id FROM sys_menu) base
    JOIN sys_menu parent ON parent.path='/system/workforce' AND parent."isDeleted"=false
    CROSS JOIN (VALUES
      (1,20,'Hook 管理','/workforce/hooks','workforce/hooks/index','CodeOutlined','workforce:hook-package:list','Hook 编辑、测试和发布')
    ) candidate(ordinal,sort_order,name,path,component,icon,permission,remark)
    WHERE NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path=candidate.path AND m."isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (id,"roleId","permissionId","creatorId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER()OVER(ORDER BY role.id,p.id),role.id,p.id,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
    FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082412410002000) id FROM sys_role_permission) base
    CROSS JOIN sys_role role CROSS JOIN sys_permission p
    WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN')
      AND role."isDeleted"=false
      AND p.code LIKE 'workforce:hook-package:%'
      AND p."isDeleted"=false
      AND NOT EXISTS(SELECT 1 FROM sys_role_permission rp WHERE rp."roleId"=role.id AND rp."permissionId"=p.id);
  END IF;

  IF to_regclass('public.sys_role_menu') IS NOT NULL THEN
    INSERT INTO sys_role_menu (id,"roleId","menuId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER()OVER(ORDER BY role.id,menu.id),role.id,menu.id,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
    FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082412410003000) id FROM sys_role_menu) base
    CROSS JOIN sys_role role CROSS JOIN sys_menu menu
    WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false AND menu.path='/workforce/hooks' AND menu."isDeleted"=false
      AND NOT EXISTS(SELECT 1 FROM sys_role_menu rm WHERE rm."roleId"=role.id AND rm."menuId"=menu.id);
  END IF;
END
$managed_hooks_rbac$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
