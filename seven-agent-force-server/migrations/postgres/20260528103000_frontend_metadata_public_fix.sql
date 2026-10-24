-- +goose Up
-- Fix frontend metadata mistakenly marked as sensitive in existing seed data.
UPDATE sys_config c
SET "isSensitive" = 0
FROM sys_config_group g
WHERE c."groupId" = g.id
  AND g."groupCode" = 'SEVEN_FRONTEND_METADATA'
  AND c."configKey" = 'title'
  AND c."requiredLogin" = 0;
