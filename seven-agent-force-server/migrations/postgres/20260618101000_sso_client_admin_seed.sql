-- +goose Up
-- +goose StatementBegin
DO $sso_client_admin_seed$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      WITH base AS (
        SELECT GREATEST(COALESCE(MAX(id), 0), 1900301100) AS id FROM sys_permission
      ),
      candidates AS (
        SELECT *
        FROM (VALUES
          (1, 'system:sso-client:list', 'SSO客户端列表', 'GET', '/sso/admin/clients', '分页查询SSO客户端列表'),
          (2, 'system:sso-client:query', 'SSO客户端详情', 'GET', '/sso/admin/clients/{clientId}', '查询SSO客户端详情'),
          (3, 'system:sso-client:add', '创建SSO客户端', 'POST', '/sso/admin/clients', '创建SSO客户端'),
          (4, 'system:sso-client:edit', '编辑SSO客户端', 'PUT', '/sso/admin/clients/{clientId}', '编辑SSO客户端'),
          (5, 'system:sso-client:status', '启停SSO客户端', 'PUT', '/sso/admin/clients/{clientId}/status', '启用或停用SSO客户端'),
          (6, 'system:sso-client:redirect:list', '查询SSO回调地址', 'GET', '/sso/admin/clients/{clientId}/redirect-uris', '查询SSO客户端回调地址'),
          (7, 'system:sso-client:redirect:edit', '编辑SSO回调地址', 'PUT', '/sso/admin/clients/{clientId}/redirect-uris', '替换SSO客户端回调地址'),
          (8, 'system:sso-client:secret:list', '查询SSO密钥', 'GET', '/sso/admin/clients/{clientId}/secrets', '查询SSO客户端密钥摘要'),
          (9, 'system:sso-client:secret:generate', '生成SSO密钥', 'POST', '/sso/admin/clients/{clientId}/secrets', '生成SSO客户端密钥'),
          (10, 'system:sso-client:secret:disable', '停用SSO密钥', 'PUT', '/sso/admin/clients/{clientId}/secrets/{secretId}/status', '停用SSO客户端密钥')
        ) AS v(sort_order, code, name, method, path, description)
        WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = v.code AND existing."isDeleted" = 0)
      )
      INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
      SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidates.sort_order),
             candidates.code, candidates.name, 'API', candidates.method, candidates.path, 0,
             candidates.description, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
      FROM candidates
      CROSS JOIN base
    $sql$;
  END IF;

  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    EXECUTE $sql$
      WITH base AS (
        SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000000) AS id FROM sys_menu
      )
      INSERT INTO sys_menu (id, name, "parentId", "sortOrder", path, component, icon, type, permission, "isFrame", "isCache", visible, hierarchy, level, status, "creatorId", "createTime", "updaterId", "updateTime", "isDeleted", remark)
      SELECT base.id + 1, '安全中心', 1, 115, '/system/security', 'Layout', 'SafetyOutlined', 'M', NULL, 1, 1, 1, CONCAT('/1/', base.id + 1), 2, 0, 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, 0, '系统安全管理分组'
      FROM base
      WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/security' AND existing."isDeleted" = 0)
    $sql$;

    EXECUTE $sql$
      UPDATE sys_menu
      SET name = '安全中心',
          "parentId" = 1,
          "sortOrder" = 115,
          component = 'Layout',
          icon = 'SafetyOutlined',
          type = 'M',
          visible = 1,
          status = 0,
          "updateTime" = CURRENT_TIMESTAMP
      WHERE path = '/system/security' AND "isDeleted" = 0
    $sql$;

    EXECUTE $sql$
      WITH base AS (
        SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000000) AS id FROM sys_menu
      )
      INSERT INTO sys_menu (id, name, "parentId", "sortOrder", path, component, icon, type, permission, "isFrame", "isCache", visible, hierarchy, level, status, "creatorId", "createTime", "updaterId", "updateTime", "isDeleted", remark)
      SELECT base.id + 1, 'SSO客户端', parent.id, 10, '/system/sso-client', 'system/sso-client/index', 'SafetyOutlined', 'C', 'system:sso-client:list', 1, 1, 1, CONCAT('/1/', parent.id, '/', base.id + 1), 3, 0, 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, 0, 'OIDC客户端管理'
      FROM base
      JOIN sys_menu parent ON parent.path = '/system/security' AND parent."isDeleted" = 0
      WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/sso-client' AND existing."isDeleted" = 0)
    $sql$;

    EXECUTE $sql$
      UPDATE sys_menu child
      SET name = 'SSO客户端',
          "parentId" = parent.id,
          "sortOrder" = 10,
          component = 'system/sso-client/index',
          icon = 'SafetyOutlined',
          type = 'C',
          permission = 'system:sso-client:list',
          visible = 1,
          status = 0,
          "updateTime" = CURRENT_TIMESTAMP
      FROM sys_menu parent
      WHERE child.path = '/system/sso-client'
        AND child."isDeleted" = 0
        AND parent.path = '/system/security'
        AND parent."isDeleted" = 0
    $sql$;
  END IF;

  IF to_regclass('public.sys_menu_permission') IS NOT NULL
     AND to_regclass('public.sys_menu') IS NOT NULL
     AND to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      WITH base AS (
        SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000100) AS id FROM sys_menu_permission
      ),
      candidates AS (
        SELECT m.id AS menu_id, p.id AS permission_id
        FROM sys_menu m
        JOIN sys_permission p ON p.code IN (
          'system:sso-client:list',
          'system:sso-client:query',
          'system:sso-client:add',
          'system:sso-client:edit',
          'system:sso-client:status',
          'system:sso-client:redirect:list',
          'system:sso-client:redirect:edit',
          'system:sso-client:secret:list',
          'system:sso-client:secret:generate',
          'system:sso-client:secret:disable'
        ) AND p."isDeleted" = 0
        WHERE m.path = '/system/sso-client'
          AND m."isDeleted" = 0
          AND NOT EXISTS (
            SELECT 1 FROM sys_menu_permission existing WHERE existing."menuId" = m.id AND existing."permissionId" = p.id
          )
      )
      INSERT INTO sys_menu_permission (id, "menuId", "permissionId", "creatorId", "createTime")
      SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidates.menu_id, candidates.permission_id),
             candidates.menu_id, candidates.permission_id, 0, CURRENT_TIMESTAMP
      FROM candidates
      CROSS JOIN base
    $sql$;
  END IF;

  IF to_regclass('public.sys_role_menu') IS NOT NULL
     AND to_regclass('public.sys_role') IS NOT NULL
     AND to_regclass('public.sys_menu') IS NOT NULL THEN
    EXECUTE $sql$
      WITH base AS (
        SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000200) AS id FROM sys_role_menu
      ),
      candidates AS (
        SELECT r.id AS role_id, m.id AS menu_id
        FROM sys_role r
        JOIN sys_menu m ON m.path IN ('/system/security', '/system/sso-client') AND m."isDeleted" = 0
        WHERE r.code = 'SUPER_ADMIN'
          AND r."isDeleted" = 0
          AND NOT EXISTS (
            SELECT 1 FROM sys_role_menu existing WHERE existing."roleId" = r.id AND existing."menuId" = m.id
          )
      )
      INSERT INTO sys_role_menu (id, "roleId", "menuId", "createTime", "updateTime")
      SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidates.role_id, candidates.menu_id),
             candidates.role_id, candidates.menu_id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM candidates
      CROSS JOIN base
    $sql$;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL
     AND to_regclass('public.sys_role') IS NOT NULL
     AND to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      WITH base AS (
        SELECT GREATEST(COALESCE(MAX(id), 0), 2012232800000000300) AS id FROM sys_role_permission
      ),
      candidates AS (
        SELECT r.id AS role_id, p.id AS permission_id
        FROM sys_role r
        JOIN sys_permission p ON p."isDeleted" = 0 AND p.code IN (
          'system:sso-client:list',
          'system:sso-client:query',
          'system:sso-client:add',
          'system:sso-client:edit',
          'system:sso-client:status',
          'system:sso-client:redirect:list',
          'system:sso-client:redirect:edit',
          'system:sso-client:secret:list',
          'system:sso-client:secret:generate',
          'system:sso-client:secret:disable'
        )
        WHERE r.code = 'SUPER_ADMIN'
          AND r."isDeleted" = 0
          AND NOT EXISTS (
            SELECT 1 FROM sys_role_permission existing WHERE existing."roleId" = r.id AND existing."permissionId" = p.id
          )
      )
      INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
      SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidates.role_id, candidates.permission_id),
             candidates.role_id, candidates.permission_id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM candidates
      CROSS JOIN base
    $sql$;
  END IF;

  IF to_regclass('public."sysSsoClient"') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO "sysSsoClient" (
        "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
        "requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
        status, "metadataJson", "creatorId", "updaterId", "isDeleted"
      )
      SELECT
        'authorization-console',
        'Authorization Console',
        'PUBLIC',
        'none',
        '["authorization_code","refresh_token"]'::jsonb,
        '["openid","profile","email","offline_access"]'::jsonb,
        1,
        0,
        1,
        1800,
        2592000,
        0,
        '{"seed":"sso-provider-v1"}'::jsonb,
        0,
        0,
        0
      WHERE NOT EXISTS (
        SELECT 1 FROM "sysSsoClient" existing WHERE existing."clientId" = 'authorization-console' AND existing."isDeleted" = 0
      )
    $sql$;
  END IF;
END
$sso_client_admin_seed$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $sso_client_admin_seed$
BEGIN
  -- Stable seed data is intentionally retained on rollback to avoid deleting
  -- existing security menus, role bindings, or later operator-maintained grants.
  PERFORM 1;
END
$sso_client_admin_seed$;
-- +goose StatementEnd
