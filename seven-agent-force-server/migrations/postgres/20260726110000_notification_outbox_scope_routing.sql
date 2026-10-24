-- +goose Up
-- Keep the PostgreSQL path aligned with the notification relay's two-part
-- ownership boundary: module plus stable installation/Hub/Node scope.
-- Existing NULL rows are local-only compatibility work; a Hub/Node may not
-- consume them until they are drained or explicitly attributed.
ALTER TABLE "public"."sys_outbox_event"
  ADD COLUMN IF NOT EXISTS "scopeId" character varying(128);

CREATE INDEX IF NOT EXISTS "idx_outbox_owner_scope_type_status_retry"
  ON "public"."sys_outbox_event" ("eventOwner", "scopeId", "eventType", "status", "nextRetryAt", "leaseUntil", "createTime");

-- +goose Down
DROP INDEX IF EXISTS "public"."idx_outbox_owner_scope_type_status_retry";

ALTER TABLE "public"."sys_outbox_event"
  DROP COLUMN IF EXISTS "scopeId";
