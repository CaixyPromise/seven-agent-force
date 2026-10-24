-- +goose Up
-- Goal-6.2 mirrors the MySQL versioned scene workspace. It is additive to
-- the V1 scene-binding table and never stores dynamic recipients or secrets.

CREATE TABLE IF NOT EXISTS "public"."sysNotificationSceneDefinition" (
    "id" BIGINT PRIMARY KEY,
    "scopeId" character varying(128) NOT NULL,
    "sceneCode" character varying(96) NOT NULL,
    "sceneName" character varying(128) NOT NULL,
    "receiverKind" character varying(32) NOT NULL,
    "currentDraftRevisionId" BIGINT,
    "currentPublishedRevisionId" BIGINT,
    "version" integer NOT NULL DEFAULT 1,
    "creatorId" BIGINT,
    "updaterId" BIGINT,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" smallint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationSceneDefinitionScopeCodeKind"
    ON "public"."sysNotificationSceneDefinition" ("scopeId", "sceneCode", "receiverKind", "isDeleted");
CREATE INDEX IF NOT EXISTS "idxNotificationSceneDefinitionScopeUpdate"
    ON "public"."sysNotificationSceneDefinition" ("scopeId", "updateTime");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationSceneRevision" (
    "id" BIGINT PRIMARY KEY,
    "sceneDefinitionId" BIGINT NOT NULL,
    "revisionNo" integer NOT NULL,
    "state" character varying(16) NOT NULL,
    "revisionVersion" integer NOT NULL DEFAULT 1,
    "enabled" boolean NOT NULL DEFAULT TRUE,
    "templateRevisionId" BIGINT NOT NULL,
    "connectionRef" character varying(64),
    "connectionDigest" character varying(128),
    "publishedAt" timestamp with time zone,
    "publishedBy" BIGINT,
    "creatorId" BIGINT,
    "updaterId" BIGINT,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "ukNotificationSceneRevisionDefinitionNo" UNIQUE ("sceneDefinitionId", "revisionNo")
);
CREATE INDEX IF NOT EXISTS "idxNotificationSceneRevisionDefinitionState"
    ON "public"."sysNotificationSceneRevision" ("sceneDefinitionId", "state", "revisionNo");
CREATE INDEX IF NOT EXISTS "idxNotificationSceneRevisionTemplate"
    ON "public"."sysNotificationSceneRevision" ("templateRevisionId");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationSceneRevisionAudit" (
    "id" BIGINT PRIMARY KEY,
    "sceneDefinitionId" BIGINT NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "action" character varying(48) NOT NULL,
    "fromRevisionNo" integer,
    "toRevisionNo" integer,
    "errorCode" character varying(64),
    "actorId" BIGINT,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS "idxNotificationSceneRevisionAuditDefinition"
    ON "public"."sysNotificationSceneRevisionAudit" ("sceneDefinitionId", "createTime");
CREATE INDEX IF NOT EXISTS "idxNotificationSceneRevisionAuditScope"
    ON "public"."sysNotificationSceneRevisionAudit" ("scopeId", "createTime");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationSceneSnapshot" (
    "id" BIGINT PRIMARY KEY,
    "notificationId" BIGINT NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "sceneCode" character varying(96) NOT NULL,
    "receiverKind" character varying(32) NOT NULL,
    "sceneDefinitionId" BIGINT NOT NULL,
    "sceneRevisionId" BIGINT NOT NULL,
    "templateDefinitionId" BIGINT NOT NULL,
    "templateRevisionId" BIGINT NOT NULL,
    "connectionRef" character varying(64),
    "connectionDigest" character varying(128),
    "templateContentDigest" character varying(128) NOT NULL,
    "renderedDigest" character varying(128) NOT NULL,
    "variableDigest" character varying(128) NOT NULL,
    "resolution" character varying(32) NOT NULL,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "ukNotificationSceneSnapshotNotificationKind" UNIQUE ("notificationId", "receiverKind")
);
CREATE INDEX IF NOT EXISTS "idxNotificationSceneSnapshotSceneRevision"
    ON "public"."sysNotificationSceneSnapshot" ("sceneRevisionId");
CREATE INDEX IF NOT EXISTS "idxNotificationSceneSnapshotScope"
    ON "public"."sysNotificationSceneSnapshot" ("scopeId", "createTime");

ALTER TABLE "public"."sysNotificationDelivery"
    ADD COLUMN IF NOT EXISTS "sceneSnapshotId" BIGINT;
CREATE INDEX IF NOT EXISTS "idxNotificationDeliverySceneSnapshot"
    ON "public"."sysNotificationDelivery" ("sceneSnapshotId");

-- +goose Down
-- Forward-only: accepted scene snapshots and immutable revision history must
-- remain available for delivery audit and idempotency evidence.
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'forward-only notification scene revision migration cannot be rolled back automatically';
END
$$;
-- +goose StatementEnd
