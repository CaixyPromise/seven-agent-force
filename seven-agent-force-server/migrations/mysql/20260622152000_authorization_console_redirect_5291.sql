-- +goose Up
INSERT INTO sysSsoClientRedirectUri (
  clientId, redirectUri, postLogoutRedirectUri, status, creatorId, updaterId, isDeleted
)
SELECT
  'authorization-console',
  'http://127.0.0.1:5291/oidc/callback/authorization-console',
  NULL,
  0,
  0,
  0,
  0
WHERE NOT EXISTS (
  SELECT 1 FROM sysSsoClientRedirectUri existing
  WHERE existing.clientId = 'authorization-console'
    AND existing.redirectUri = 'http://127.0.0.1:5291/oidc/callback/authorization-console'
    AND existing.isDeleted = 0
);

-- +goose Down
UPDATE sysSsoClientRedirectUri
SET isDeleted = 1, updateTime = CURRENT_TIMESTAMP
WHERE clientId = 'authorization-console'
  AND redirectUri = 'http://127.0.0.1:5291/oidc/callback/authorization-console'
  AND isDeleted = 0;
