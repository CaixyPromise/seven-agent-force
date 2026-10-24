-- +goose Up
-- Keep configuration ownership aligned with the scoped Outbox/RabbitMQ
-- boundary. Historical NULL rows are a local-only compatibility case.
ALTER TABLE "public"."sysNotificationTemplate"
  ADD COLUMN IF NOT EXISTS "scopeId" character varying(128);

ALTER TABLE "public"."sysNotificationSceneBinding"
  ADD COLUMN IF NOT EXISTS "scopeId" character varying(128);

CREATE INDEX IF NOT EXISTS "idx_notification_template_scope_scene_status"
  ON "public"."sysNotificationTemplate" ("scopeId", "sceneCode", "channelType", "locale", "status", "isDeleted");

CREATE INDEX IF NOT EXISTS "idx_notification_scene_binding_scope_scene_enabled"
  ON "public"."sysNotificationSceneBinding" ("scopeId", "sceneCode", "enabled", "priority", "isDeleted");

-- +goose Down
DROP INDEX IF EXISTS "public"."idx_notification_scene_binding_scope_scene_enabled";
DROP INDEX IF EXISTS "public"."idx_notification_template_scope_scene_status";

ALTER TABLE "public"."sysNotificationSceneBinding"
  DROP COLUMN IF EXISTS "scopeId";

ALTER TABLE "public"."sysNotificationTemplate"
  DROP COLUMN IF EXISTS "scopeId";
