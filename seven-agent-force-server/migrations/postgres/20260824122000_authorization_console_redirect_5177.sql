-- +goose Up
INSERT INTO sys_sso_client_redirect_uri (
  id, "clientId", "redirectUri", "postLogoutRedirectUri", status,
  "creatorId", "updaterId", "createTime", "updateTime", "isDeleted"
)
SELECT
  GREATEST(COALESCE(MAX(id), 0), 2026082412200000000) + 1,
  'authorization-console',
  'http://127.0.0.1:5177/oidc/callback/authorization-console',
  NULL,
  0,
  0,
  0,
  CURRENT_TIMESTAMP,
  CURRENT_TIMESTAMP,
  0
FROM sys_sso_client_redirect_uri
WHERE NOT EXISTS (
  SELECT 1
  FROM sys_sso_client_redirect_uri existing
  WHERE existing."clientId" = 'authorization-console'
    AND existing."redirectUri" = 'http://127.0.0.1:5177/oidc/callback/authorization-console'
    AND existing."isDeleted" = 0
);

-- +goose Down
UPDATE sys_sso_client_redirect_uri
SET "isDeleted" = 1, "updateTime" = CURRENT_TIMESTAMP
WHERE "clientId" = 'authorization-console'
  AND "redirectUri" = 'http://127.0.0.1:5177/oidc/callback/authorization-console'
  AND "isDeleted" = 0;
