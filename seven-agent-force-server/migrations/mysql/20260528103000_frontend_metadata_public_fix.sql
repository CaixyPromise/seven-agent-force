-- +goose Up
-- Fix frontend metadata mistakenly marked as sensitive in existing seed data.
UPDATE sys_config c
JOIN sys_config_group g ON c.groupId = g.id
SET c.isSensitive = 0
WHERE g.groupCode = 'SEVEN_FRONTEND_METADATA'
  AND c.configKey = 'title'
  AND c.requiredLogin = 0;
