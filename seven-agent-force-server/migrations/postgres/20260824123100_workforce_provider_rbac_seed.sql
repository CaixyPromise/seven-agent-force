-- +goose Up
-- +goose StatementBegin
DO $workforce_model_credential_rbac$
BEGIN
  INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
  SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.ordinal), candidate.code, candidate.name,
         'API', candidate.method, candidate.path, 0, candidate.description,
         CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false
  FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412210000000) AS id FROM sys_permission) base
  CROSS JOIN (VALUES
    (1, 'workforce:model-credential:list', '查看模型密钥', 'GET', '/workforce/model-credentials', '查看当前组织的模型密钥'),
    (2, 'workforce:model-credential:create', '新增模型密钥', 'POST', '/workforce/model-credentials', '加密保存模型密钥'),
    (3, 'workforce:model-credential:update', '编辑模型密钥', 'PUT', '/workforce/model-credentials/{id}', '编辑地址、密钥和模型目录'),
    (4, 'workforce:model-credential:status', '启停模型密钥', 'PUT', '/workforce/model-credentials/{id}/status', '启用或停用模型密钥'),
    (5, 'workforce:model-credential:usage', '查看模型调用', 'GET', '/workforce/model-credentials/{id}/invocations', '查看安全的模型调用统计'),
    (6, 'workforce:model-credential:delete', '删除模型密钥', 'DELETE', '/workforce/model-credentials/{id}', '删除模型密钥')
  ) AS candidate(ordinal, code, name, method, path, description)
  WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code=candidate.code AND existing."isDeleted"=false);

  INSERT INTO sys_menu (id, name, "parentId", "sortOrder", path, component, icon, type, permission,
                        "isFrame", "isCache", visible, hierarchy, level, status,
                        "creatorId", "createTime", "updaterId", "updateTime", "isDeleted", remark)
  SELECT base.id + 1, '模型中心', parent.id, 10, '/workforce/model-credentials', 'workforce/model-credentials/index',
         'ApiOutlined', 'C', 'workforce:model-credential:list', true, true, true,
         CONCAT(COALESCE(NULLIF(parent.hierarchy, ''), '/1'), '/', base.id + 1), parent.level + 1, 0,
         0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, false, '模型调用地址、密钥与模型目录'
  FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412210000100) AS id FROM sys_menu) base
  JOIN sys_menu parent ON parent.path='/system/workforce' AND parent."isDeleted"=false
  WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path='/workforce/model-credentials' AND existing."isDeleted"=false);

  INSERT INTO sys_menu_permission (id, "menuId", "permissionId", "creatorId", "createTime")
  SELECT base.id + ROW_NUMBER() OVER (ORDER BY permission.id), menu.id, permission.id, 0, CURRENT_TIMESTAMP
  FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412210000200) AS id FROM sys_menu_permission) base
  JOIN sys_menu menu ON menu.path='/workforce/model-credentials' AND menu."isDeleted"=false
  JOIN sys_permission permission ON permission.code LIKE 'workforce:model-credential:%' AND permission."isDeleted"=false
  WHERE NOT EXISTS (SELECT 1 FROM sys_menu_permission existing WHERE existing."menuId"=menu.id AND existing."permissionId"=permission.id);

  INSERT INTO sys_role_menu (id, "roleId", "menuId", "createTime", "updateTime")
  SELECT base.id + ROW_NUMBER() OVER (ORDER BY role.id), role.id, menu.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
  FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412210000300) AS id FROM sys_role_menu) base
  JOIN sys_role role ON role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
  JOIN sys_menu menu ON menu.path='/workforce/model-credentials' AND menu."isDeleted"=false
  WHERE NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing."roleId"=role.id AND existing."menuId"=menu.id);

  INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
  SELECT base.id + ROW_NUMBER() OVER (ORDER BY role.id,permission.id), role.id, permission.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
  FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082412210000400) AS id FROM sys_role_permission) base
  JOIN sys_role role ON role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
  JOIN sys_permission permission ON permission.code LIKE 'workforce:model-credential:%' AND permission."isDeleted"=false
  WHERE NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing."roleId"=role.id AND existing."permissionId"=permission.id);
END
$workforce_model_credential_rbac$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
