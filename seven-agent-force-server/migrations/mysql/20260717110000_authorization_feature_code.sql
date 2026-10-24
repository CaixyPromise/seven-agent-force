-- +goose Up
ALTER TABLE sys_menu
  ADD COLUMN featureCode VARCHAR(64) NULL COMMENT '菜单所属功能能力编码' AFTER permission;

ALTER TABLE sys_permission
  ADD COLUMN featureCode VARCHAR(64) NULL COMMENT '权限所属功能能力编码' AFTER code;

UPDATE sys_menu
SET featureCode = 'docker.admin'
WHERE isDeleted = 0
  AND (path = '/system/docker' OR path LIKE '/system/docker/%');

UPDATE sys_menu
SET featureCode = 'platform.control'
WHERE isDeleted = 0 AND path = '/system/platform';

UPDATE sys_menu
SET featureCode = 'federation.hub'
WHERE isDeleted = 0 AND path = '/system/hub-node';

UPDATE sys_permission
SET featureCode = 'docker.admin'
WHERE isDeleted = 0 AND code LIKE 'admin:docker:%';

UPDATE sys_permission
SET featureCode = 'platform.control'
WHERE isDeleted = 0 AND code LIKE 'system:platform:%';

UPDATE sys_permission
SET featureCode = 'federation.hub'
WHERE isDeleted = 0 AND code LIKE 'system:hub-node:%';

SET @operatorId := 0;
SET @hubPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 1900600000) FROM sys_permission);

UPDATE sys_permission existing
JOIN (
  SELECT 1 AS sortOrder, 'system:hub-node:list' AS code, 'Hub节点列表' AS name, 'GET' AS method, '/system/hub/nodes' AS path, '查询Hub节点列表' AS description
  UNION ALL SELECT 2, 'system:hub-node:add', '创建Hub节点', 'POST', '/system/hub/nodes', '创建或复制Hub节点'
  UNION ALL SELECT 3, 'system:hub-node:query', 'Hub节点详情', 'GET', '/system/hub/nodes/:nodeCode', '查询Hub节点详情'
  UNION ALL SELECT 4, 'system:hub-node:edit', '编辑Hub节点', 'PUT', '/system/hub/nodes/:nodeCode', '编辑Hub节点'
  UNION ALL SELECT 5, 'system:hub-node:status', '启停Hub节点', 'PUT', '/system/hub/nodes/:nodeCode/status', '启用或停用Hub节点'
  UNION ALL SELECT 6, 'system:hub-node:test', '测试Hub节点连接', 'POST', '/system/hub/nodes/:nodeCode/connection-test', '测试Hub节点连接'
  UNION ALL SELECT 7, 'system:hub-node:user:list', 'Node用户列表', 'GET', '/system/hub/nodes/:nodeCode/users', '查询Node用户列表'
  UNION ALL SELECT 8, 'system:hub-node:user:query', 'Node用户详情', 'GET', '/system/hub/nodes/:nodeCode/users/:userId', '查询Node用户详情'
  UNION ALL SELECT 9, 'system:hub-node:user:status', '修改Node用户状态', 'PUT', '/system/hub/nodes/:nodeCode/users/:userId/status', '修改Node用户状态'
  UNION ALL SELECT 10, 'system:hub-node:session:list', 'Node用户会话', 'GET', '/system/hub/nodes/:nodeCode/users/:userId/sessions', '查询Node用户会话'
  UNION ALL SELECT 11, 'system:hub-node:session:revoke', '撤销Node用户会话', 'POST', '/system/hub/nodes/:nodeCode/users/:userId/sessions/revoke', '撤销Node用户会话'
  UNION ALL SELECT 12, 'system:hub-node:policy:query', 'Node登录策略', 'GET', '/system/hub/nodes/:nodeCode/login-policy', '查询Node登录策略'
  UNION ALL SELECT 13, 'system:hub-node:policy:apply', '应用Node登录策略', 'POST', '/system/hub/nodes/:nodeCode/login-policy/apply', '应用Node登录策略'
  UNION ALL SELECT 14, 'system:hub-node:federation:query', 'Node联邦连接', 'GET', '/system/hub/nodes/:nodeCode/federation', '查询Node联邦连接'
  UNION ALL SELECT 15, 'system:hub-node:federation:apply', '编排Node联邦连接', 'POST', '/system/hub/nodes/:nodeCode/federation/provision', '编排Node联邦连接'
) item ON existing.code = item.code
SET existing.featureCode = 'federation.hub',
    existing.name = item.name,
    existing.resourceType = 'API',
    existing.method = item.method,
    existing.path = item.path,
    existing.status = 0,
    existing.description = item.description,
    existing.updateTime = NOW(),
    existing.isDeleted = 0;

INSERT INTO sys_permission (id, code, featureCode, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT @hubPermissionBaseId + ROW_NUMBER() OVER (ORDER BY item.sortOrder), item.code, 'federation.hub', item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1 AS sortOrder, 'system:hub-node:list' AS code, 'Hub节点列表' AS name, 'GET' AS method, '/system/hub/nodes' AS path, '查询Hub节点列表' AS description
  UNION ALL SELECT 2, 'system:hub-node:add', '创建Hub节点', 'POST', '/system/hub/nodes', '创建或复制Hub节点'
  UNION ALL SELECT 3, 'system:hub-node:query', 'Hub节点详情', 'GET', '/system/hub/nodes/:nodeCode', '查询Hub节点详情'
  UNION ALL SELECT 4, 'system:hub-node:edit', '编辑Hub节点', 'PUT', '/system/hub/nodes/:nodeCode', '编辑Hub节点'
  UNION ALL SELECT 5, 'system:hub-node:status', '启停Hub节点', 'PUT', '/system/hub/nodes/:nodeCode/status', '启用或停用Hub节点'
  UNION ALL SELECT 6, 'system:hub-node:test', '测试Hub节点连接', 'POST', '/system/hub/nodes/:nodeCode/connection-test', '测试Hub节点连接'
  UNION ALL SELECT 7, 'system:hub-node:user:list', 'Node用户列表', 'GET', '/system/hub/nodes/:nodeCode/users', '查询Node用户列表'
  UNION ALL SELECT 8, 'system:hub-node:user:query', 'Node用户详情', 'GET', '/system/hub/nodes/:nodeCode/users/:userId', '查询Node用户详情'
  UNION ALL SELECT 9, 'system:hub-node:user:status', '修改Node用户状态', 'PUT', '/system/hub/nodes/:nodeCode/users/:userId/status', '修改Node用户状态'
  UNION ALL SELECT 10, 'system:hub-node:session:list', 'Node用户会话', 'GET', '/system/hub/nodes/:nodeCode/users/:userId/sessions', '查询Node用户会话'
  UNION ALL SELECT 11, 'system:hub-node:session:revoke', '撤销Node用户会话', 'POST', '/system/hub/nodes/:nodeCode/users/:userId/sessions/revoke', '撤销Node用户会话'
  UNION ALL SELECT 12, 'system:hub-node:policy:query', 'Node登录策略', 'GET', '/system/hub/nodes/:nodeCode/login-policy', '查询Node登录策略'
  UNION ALL SELECT 13, 'system:hub-node:policy:apply', '应用Node登录策略', 'POST', '/system/hub/nodes/:nodeCode/login-policy/apply', '应用Node登录策略'
  UNION ALL SELECT 14, 'system:hub-node:federation:query', 'Node联邦连接', 'GET', '/system/hub/nodes/:nodeCode/federation', '查询Node联邦连接'
  UNION ALL SELECT 15, 'system:hub-node:federation:apply', '编排Node联邦连接', 'POST', '/system/hub/nodes/:nodeCode/federation/provision', '编排Node联邦连接'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code);

SET @rootMenuId := (SELECT id FROM sys_menu WHERE path = '/system' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @accessMenuId := (SELECT id FROM sys_menu WHERE path = '/system/access' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @hubMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000001000) FROM sys_menu);

INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, featureCode, isFrame, isCache, visible, hierarchy, level, status, creatorId, createTime, updaterId, updateTime, isDeleted, remark)
SELECT @hubMenuBaseId + 1, 'Hub节点管理', @accessMenuId, 70, '/system/hub-node', 'system/hub-node/index', 'DeploymentUnitOutlined', 'C', 'system:hub-node:list', 'federation.hub', 1, 1, 1, CONCAT('/1/', @rootMenuId, '/', @accessMenuId, '/', @hubMenuBaseId + 1), 3, 0, @operatorId, NOW(), @operatorId, NOW(), 0, '管理联邦Node连接、用户状态、会话和登录策略'
WHERE @accessMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/hub-node' AND existing.isDeleted = 0);

UPDATE sys_menu child
JOIN sys_menu parent ON parent.path = '/system/access' AND parent.isDeleted = 0
LEFT JOIN sys_menu root ON root.path = '/system' AND root.isDeleted = 0
SET child.name = 'Hub节点管理',
    child.parentId = parent.id,
    child.sortOrder = 70,
    child.component = 'system/hub-node/index',
    child.icon = 'DeploymentUnitOutlined',
    child.type = 'C',
    child.permission = 'system:hub-node:list',
    child.featureCode = 'federation.hub',
    child.visible = 1,
    child.status = 0,
    child.hierarchy = CONCAT('/1/', COALESCE(root.id, parent.parentId), '/', parent.id, '/', child.id),
    child.level = 3,
    child.updateTime = NOW()
WHERE child.path = '/system/hub-node' AND child.isDeleted = 0;

SET @hubMenuId := (SELECT id FROM sys_menu WHERE path = '/system/hub-node' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @hubMenuPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000001100) FROM sys_menu_permission);

INSERT INTO sys_menu_permission (id, menuId, permissionId, creatorId, createTime)
SELECT @hubMenuPermissionBaseId + ROW_NUMBER() OVER (ORDER BY p.id), @hubMenuId, p.id, @operatorId, NOW()
FROM sys_permission p
WHERE p.code LIKE 'system:hub-node:%' AND p.isDeleted = 0 AND @hubMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu_permission existing WHERE existing.menuId = @hubMenuId AND existing.permissionId = p.id);

SET @hubRoleMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000001200) FROM sys_role_menu);
INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT @hubRoleMenuBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId), candidate.roleId, candidate.menuId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path IN ('/system/access', '/system/hub-node') AND m.isDeleted = 0
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing.roleId = r.id AND existing.menuId = m.id)
) candidate;

SET @hubRolePermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233200000001300) FROM sys_role_permission);
INSERT INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT @hubRolePermissionBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.permissionId), candidate.roleId, candidate.permissionId, @operatorId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, p.id AS permissionId
  FROM sys_role r
  JOIN sys_permission p ON p.isDeleted = 0 AND p.code LIKE 'system:hub-node:%'
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing.roleId = r.id AND existing.permissionId = p.id)
) candidate;

-- +goose Down
DELETE FROM sys_role_permission WHERE permissionId IN (SELECT id FROM sys_permission WHERE code LIKE 'system:hub-node:%');
DELETE FROM sys_role_menu WHERE menuId IN (SELECT id FROM sys_menu WHERE path = '/system/hub-node');
DELETE FROM sys_menu_permission WHERE menuId IN (SELECT id FROM sys_menu WHERE path = '/system/hub-node');
DELETE FROM sys_menu WHERE path = '/system/hub-node';
DELETE FROM sys_permission WHERE code LIKE 'system:hub-node:%';
ALTER TABLE sys_permission DROP COLUMN featureCode;
ALTER TABLE sys_menu DROP COLUMN featureCode;
