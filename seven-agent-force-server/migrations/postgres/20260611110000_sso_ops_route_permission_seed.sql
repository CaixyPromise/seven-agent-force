-- +goose Up
-- +goose StatementBegin
DO $sso_ops_permissions$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
      SELECT v.id, v.code, v.name, 'API', v.method, v.path, 0, v.description, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0
      FROM (VALUES
        (1900300901, 'admin:sso:session:list', 'SSO会话列表', 'GET', '/sso/admin/users/{userId}/sessions', '查询用户SSO会话'),
        (1900300902, 'admin:sso:session:kick', 'SSO会话踢出', 'POST', '/sso/admin/users/{userId}/sessions/{sessionId}/kick', '踢出用户SSO会话'),
        (1900300903, 'admin:sso:device:list', 'SSO设备列表', 'GET', '/sso/admin/users/{userId}/devices', '查询用户SSO设备'),
        (1900300904, 'admin:sso:device:kick', 'SSO设备踢出', 'POST', '/sso/admin/users/{userId}/devices/{deviceId}/kick', '踢出用户SSO设备'),
        (1900300905, 'admin:ops:module:list', '模块运行列表', 'GET', '/ops/modules', '查询模块运行列表')
      ) AS v(id, code, name, method, path, description)
      WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = v.code AND existing."isDeleted" = 0)
    $sql$;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL AND to_regclass('public.sys_role') IS NOT NULL AND to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
      SELECT 190030090000 + r.id + p.id, r.id, p.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM sys_role r
      JOIN sys_permission p ON p."isDeleted" = 0 AND p.code IN (
        'admin:sso:session:list',
        'admin:sso:session:kick',
        'admin:sso:device:list',
        'admin:sso:device:kick',
        'admin:ops:module:list'
      )
      WHERE r."isDeleted" = 0 AND r.code = 'SUPER_ADMIN'
        AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing."roleId" = r.id AND existing."permissionId" = p.id)
    $sql$;
  END IF;
END
$sso_ops_permissions$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $sso_ops_permissions$
BEGIN
  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    EXECUTE $sql$
      DELETE FROM sys_role_permission
      WHERE "permissionId" IN (
        SELECT id FROM sys_permission WHERE code IN (
          'admin:sso:session:list',
          'admin:sso:session:kick',
          'admin:sso:device:list',
          'admin:sso:device:kick',
          'admin:ops:module:list'
        )
      )
    $sql$;
  END IF;
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    EXECUTE $sql$
      DELETE FROM sys_permission WHERE code IN (
        'admin:sso:session:list',
        'admin:sso:session:kick',
        'admin:sso:device:list',
        'admin:sso:device:kick',
        'admin:ops:module:list'
      )
    $sql$;
  END IF;
END
$sso_ops_permissions$;
-- +goose StatementEnd
