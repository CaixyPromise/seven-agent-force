-- +goose Up
-- Keep the PostgreSQL upgrade path aligned with the shared Outbox ownership
-- and fencing contract. The clean-install baseline applies this regular
-- migration after its snapshot marker.
ALTER TABLE "public"."sys_outbox_event"
  ADD COLUMN IF NOT EXISTS "eventOwner" character varying(64) NOT NULL DEFAULT 'unassigned',
  ADD COLUMN IF NOT EXISTS "leaseOwner" character varying(128),
  ADD COLUMN IF NOT EXISTS "leaseToken" character varying(64),
  ADD COLUMN IF NOT EXISTS "leaseUntil" timestamp with time zone;

ALTER TABLE "public"."sys_message_consume_log"
  ADD COLUMN IF NOT EXISTS "leaseOwner" character varying(128),
  ADD COLUMN IF NOT EXISTS "leaseToken" character varying(64),
  ADD COLUMN IF NOT EXISTS "leaseUntil" timestamp with time zone,
  ADD COLUMN IF NOT EXISTS "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP;

UPDATE "public"."sys_outbox_event"
SET "eventOwner" = CASE
  WHEN "eventType" IN ('UPLOAD_TASK_READY', 'UPLOAD_TASK_UPLOADED', 'FILE_PROCESS_TASK') THEN 'file'
  WHEN "eventType" = 'notification.dispatch' THEN 'notification'
  ELSE 'unassigned'
END
WHERE "eventOwner" = 'unassigned';

CREATE INDEX IF NOT EXISTS "idx_outbox_owner_type_status_retry"
  ON "public"."sys_outbox_event" ("eventOwner", "eventType", "status", "nextRetryAt", "leaseUntil", "createTime");

CREATE INDEX IF NOT EXISTS "idx_message_consume_status_lease"
  ON "public"."sys_message_consume_log" ("status", "leaseUntil");

-- +goose Down
DROP INDEX IF EXISTS "public"."idx_message_consume_status_lease";
DROP INDEX IF EXISTS "public"."idx_outbox_owner_type_status_retry";

ALTER TABLE "public"."sys_message_consume_log"
  DROP COLUMN IF EXISTS "updateTime",
  DROP COLUMN IF EXISTS "leaseUntil",
  DROP COLUMN IF EXISTS "leaseToken",
  DROP COLUMN IF EXISTS "leaseOwner";

ALTER TABLE "public"."sys_outbox_event"
  DROP COLUMN IF EXISTS "leaseUntil",
  DROP COLUMN IF EXISTS "leaseToken",
  DROP COLUMN IF EXISTS "leaseOwner",
  DROP COLUMN IF EXISTS "eventOwner";
