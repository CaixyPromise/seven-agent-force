-- +goose Up
-- Goal-2 is additive: legacy V1 delivery history is not guessed into a local
-- user's inbox, and no existing delivery rows are rewritten.

-- Notification recipient resolution accepts only an address whose ownership
-- has been proven. Historical addresses intentionally remain unverified.
ALTER TABLE "public"."sys_user"
    ADD COLUMN IF NOT EXISTS "email_verified_at" timestamp with time zone,
    ADD COLUMN IF NOT EXISTS "email_version" BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS "email_verification_source" character varying(64) NOT NULL DEFAULT '';

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_sys_user_email_version_positive'
          AND conrelid = 'public.sys_user'::regclass
    ) THEN
        ALTER TABLE "public"."sys_user"
            ADD CONSTRAINT "chk_sys_user_email_version_positive" CHECK ("email_version" > 0);
    END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS "idxUserVerifiedEmail"
    ON "public"."sys_user" (lower("userEmail"), "email_verified_at")
    WHERE "isDeleted" = FALSE AND "status" = 0 AND "email_verified_at" IS NOT NULL;

CREATE TABLE IF NOT EXISTS "public"."sysNotification" (
    "id" BIGINT PRIMARY KEY,
    "notificationId" character varying(96) NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "eventKey" character varying(128) NOT NULL,
    "idempotencyKey" character varying(191) NOT NULL,
    "requestFingerprint" character(64) NOT NULL,
    "audienceJson" jsonb NOT NULL,
    "category" character varying(64) NOT NULL,
    "priority" character varying(32) NOT NULL,
    "mandatory" boolean NOT NULL DEFAULT FALSE,
    "title" character varying(512) NOT NULL,
    "content" text NOT NULL,
    "deepLink" character varying(512),
    "actionType" character varying(32) NOT NULL DEFAULT '',
    "actionLabelSnapshot" character varying(128) NOT NULL DEFAULT '',
    "actionStyle" character varying(32) NOT NULL DEFAULT '',
    "scheduleAt" timestamp with time zone,
    "expiresAt" timestamp with time zone,
    "traceId" character varying(128),
    "status" character varying(32) NOT NULL,
    "creatorId" BIGINT,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationExternalId"
    ON "public"."sysNotification" ("notificationId");
CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationIdempotency"
    ON "public"."sysNotification" ("scopeId", "eventKey", "idempotencyKey");
CREATE INDEX IF NOT EXISTS "idxNotificationScopeStatusTime"
    ON "public"."sysNotification" ("scopeId", "status", "createTime");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationRecipient" (
    "id" BIGINT PRIMARY KEY,
    "recipientId" character varying(96) NOT NULL,
    "notificationId" BIGINT NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "userId" BIGINT NOT NULL,
    "eventKey" character varying(128) NOT NULL,
    "category" character varying(64) NOT NULL,
    "priority" character varying(32) NOT NULL,
    "mandatory" boolean NOT NULL DEFAULT FALSE,
    "title" character varying(512) NOT NULL,
    "content" text NOT NULL,
    "deepLink" character varying(512),
    "actionType" character varying(32) NOT NULL DEFAULT '',
    "actionLabelSnapshot" character varying(128) NOT NULL DEFAULT '',
    "actionStyle" character varying(32) NOT NULL DEFAULT '',
    "expiresAt" timestamp with time zone,
    "firstSeenAt" timestamp with time zone,
    "readAt" timestamp with time zone,
    "archivedAt" timestamp with time zone,
    "mailboxVersion" BIGINT NOT NULL,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationRecipientId"
    ON "public"."sysNotificationRecipient" ("recipientId");
CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationRecipientUser"
    ON "public"."sysNotificationRecipient" ("notificationId", "userId");
CREATE INDEX IF NOT EXISTS "idxNotificationRecipientInboxPage"
    ON "public"."sysNotificationRecipient" ("scopeId", "userId", "archivedAt", "createTime", "id");
CREATE INDEX IF NOT EXISTS "idxNotificationRecipientUnread"
    ON "public"."sysNotificationRecipient" ("scopeId", "userId", "archivedAt", "readAt", "expiresAt");
CREATE INDEX IF NOT EXISTS "idxNotificationRecipientMailboxVersion"
    ON "public"."sysNotificationRecipient" ("scopeId", "userId", "mailboxVersion");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationMaterializationTask" (
    "id" BIGINT PRIMARY KEY,
    "taskId" character varying(96) NOT NULL,
    "notificationId" BIGINT NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "audienceJson" jsonb NOT NULL,
    "materializationCursor" character varying(512) NOT NULL,
    "status" character varying(32) NOT NULL,
    "materializedCount" BIGINT NOT NULL DEFAULT 0,
    "retryCount" integer NOT NULL DEFAULT 0,
    "nextRunAt" timestamp with time zone NOT NULL,
    "leaseOwner" character varying(128),
    "leaseToken" character varying(64),
    "leaseUntil" timestamp with time zone,
    "lastError" text,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationMaterializationTaskId"
    ON "public"."sysNotificationMaterializationTask" ("taskId");
CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationMaterializationNotification"
    ON "public"."sysNotificationMaterializationTask" ("notificationId");
CREATE INDEX IF NOT EXISTS "idxNotificationMaterializationReady"
    ON "public"."sysNotificationMaterializationTask" ("status", "nextRunAt", "leaseUntil", "createTime");

-- +goose Down
-- Forward-only migration: use a manual, audited data-retention plan rather
-- than dropping logical notification or recipient audit data automatically.
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'forward-only notification core inbox migration cannot be rolled back automatically';
END
$$;
-- +goose StatementEnd
