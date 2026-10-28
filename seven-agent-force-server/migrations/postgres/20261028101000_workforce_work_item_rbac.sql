-- +goose Up
INSERT INTO sys_permission(id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted")
SELECT base.id+ROW_NUMBER() OVER(ORDER BY v.n),v.code,v.name,'API',v.method,v.path,0,v.name,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false
FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026102810100000000) id FROM sys_permission) base
CROSS JOIN (VALUES
 (1,'workforce:work-item:create','创建开发工作单','POST','/workforce/projects/{projectId}/work-items'),
 (2,'workforce:work-item:query','查看开发工作单','GET','/workforce/work-items/{id}'),
 (3,'workforce:work-item:plan:approve','确认开发方案','POST','/workforce/work-items/{id}/plan-decisions'),
 (4,'workforce:work-item:cancel','停止开发工作单','POST','/workforce/work-items/{id}/cancel')
) v(n,code,name,method,path)
WHERE NOT EXISTS(SELECT 1 FROM sys_permission p WHERE p.code=v.code AND p."isDeleted"=false);
INSERT INTO sys_role_permission(id,"roleId","permissionId","creatorId","createTime","updateTime")
SELECT base.id+ROW_NUMBER() OVER(ORDER BY r.id,p.id),r.id,p.id,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026102810100000100) id FROM sys_role_permission) base
CROSS JOIN sys_role r JOIN sys_permission p ON p.code LIKE 'workforce:work-item:%' AND p."isDeleted"=false
WHERE r.code IN('SUPER_ADMIN','SYSTEM_ADMIN') AND r."isDeleted"=false
AND NOT EXISTS(SELECT 1 FROM sys_role_permission rp WHERE rp."roleId"=r.id AND rp."permissionId"=p.id);
-- +goose Down
SELECT 1;
