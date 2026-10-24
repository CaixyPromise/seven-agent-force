-- +goose Up
-- Goal-4 is additive. A third-party enterprise member is not a platform user,
-- so its encrypted delivery target and provider attempt history live outside
-- the inbox-recipient projection.

ALTER TABLE "public"."sysNotificationChannel"
    ADD COLUMN IF NOT EXISTS "scopeId" character varying(128),
    ADD COLUMN IF NOT EXISTS "scopeType" character varying(32) NOT NULL DEFAULT 'SYSTEM',
    ADD COLUMN IF NOT EXISTS "organizationId" BIGINT,
    ADD COLUMN IF NOT EXISTS "enterpriseConnectionId" BIGINT,
    ADD COLUMN IF NOT EXISTS "enterpriseConnectionVersion" BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "enterpriseConnectionDigest" character(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS "idxNotificationChannelOrganization"
    ON "public"."sysNotificationChannel" ("organizationId", "channelType", "status")
    WHERE "isDeleted" = 0;

ALTER TABLE "public"."sysNotificationDelivery"
    ADD COLUMN IF NOT EXISTS "organizationId" BIGINT,
    ADD COLUMN IF NOT EXISTS "notificationId" BIGINT,
    ADD COLUMN IF NOT EXISTS "externalTargetId" BIGINT,
    ADD COLUMN IF NOT EXISTS "providerReference" character varying(191),
    ADD COLUMN IF NOT EXISTS "presentationType" character varying(32) NOT NULL DEFAULT 'TEXT',
    ADD COLUMN IF NOT EXISTS "actionLabelSnapshot" character varying(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS "actionUrlSnapshot" character varying(1024) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS "renderedPayloadDigest" character(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS "idxNotificationDeliveryNotification"
    ON "public"."sysNotificationDelivery" ("notificationId", "createTime");
CREATE INDEX IF NOT EXISTS "idxNotificationDeliveryExternalTarget"
    ON "public"."sysNotificationDelivery" ("externalTargetId", "createTime");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationExternalTarget" (
    "id" BIGINT PRIMARY KEY,
    "externalTargetId" character varying(96) NOT NULL,
    "notificationId" BIGINT NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "organizationId" BIGINT,
    "userId" BIGINT,
    "externalIdentityId" BIGINT,
    "externalIdentityVersion" BIGINT NOT NULL DEFAULT 0,
    "enterpriseConnectionId" BIGINT,
    "enterpriseConnectionVersion" BIGINT NOT NULL DEFAULT 0,
    "authorityDigest" character varying(128) NOT NULL DEFAULT '',
    "grantType" character varying(32) NOT NULL DEFAULT '',
    "connectionRef" character varying(64) NOT NULL,
    "providerCode" character varying(32) NOT NULL,
    "identityKind" character varying(32) NOT NULL,
    "subjectCiphertext" text NOT NULL,
    "subjectEdek" text NOT NULL,
    "subjectWrapKeyRef" character varying(191) NOT NULL,
    "subjectDigest" character(64) NOT NULL,
    "subjectDigestKeyRef" character varying(191) NOT NULL,
    "providerParamsJson" jsonb,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationExternalTargetId"
    ON "public"."sysNotificationExternalTarget" ("externalTargetId");
CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationExternalTargetSemantic"
    ON "public"."sysNotificationExternalTarget" ("notificationId", "connectionRef", "identityKind", "subjectDigest");
CREATE INDEX IF NOT EXISTS "idxNotificationExternalTargetNotification"
    ON "public"."sysNotificationExternalTarget" ("notificationId", "createTime");
CREATE INDEX IF NOT EXISTS "idxNotificationExternalTargetScopeConnection"
    ON "public"."sysNotificationExternalTarget" ("scopeId", "connectionRef", "createTime");
CREATE INDEX IF NOT EXISTS "idxNotificationExternalTargetAuthority"
    ON "public"."sysNotificationExternalTarget" ("organizationId", "externalIdentityId", "enterpriseConnectionId", "createTime");

ALTER TABLE "public"."sysNotification"
    ADD COLUMN IF NOT EXISTS "organizationId" BIGINT;

CREATE TABLE IF NOT EXISTS "public"."sysNotificationDeliveryAttempt" (
    "id" BIGINT PRIMARY KEY,
    "attemptId" character varying(96) NOT NULL,
    "deliveryId" character varying(96) NOT NULL,
    "attemptNo" integer NOT NULL,
    "status" character varying(32) NOT NULL,
    "failureClass" character varying(64),
    "providerReference" character varying(191),
    "diagnostic" character varying(128),
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationDeliveryAttemptId"
    ON "public"."sysNotificationDeliveryAttempt" ("attemptId");
CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationDeliveryAttemptNumber"
    ON "public"."sysNotificationDeliveryAttempt" ("deliveryId", "attemptNo");
CREATE INDEX IF NOT EXISTS "idxNotificationDeliveryAttemptDelivery"
    ON "public"."sysNotificationDeliveryAttempt" ("deliveryId", "createTime");

-- +goose Down
-- Forward-only: external delivery audit records and encrypted targets are not
-- dropped automatically. Failing here keeps the Goose version unchanged.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'forward-only notification external delivery migration cannot be rolled back automatically';
END
$$;
-- +goose StatementEnd
