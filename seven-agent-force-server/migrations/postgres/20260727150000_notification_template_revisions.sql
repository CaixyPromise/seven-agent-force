-- +goose Up
-- Goal-6.1 adds a versioned authoring workspace beside the legacy
-- sysNotificationTemplate runtime configuration. These tables are not read by
-- the existing dispatch path and therefore cannot alter current sends.

CREATE TABLE IF NOT EXISTS "public"."sysNotificationTemplateDefinition" (
    "id" BIGINT PRIMARY KEY,
    "scopeId" character varying(128) NOT NULL,
    "templateCode" character varying(96) NOT NULL,
    "templateName" character varying(128) NOT NULL,
    "locale" character varying(32) NOT NULL DEFAULT 'zh-CN',
    "currentDraftRevisionId" BIGINT,
    "currentPublishedRevisionId" BIGINT,
    "version" integer NOT NULL DEFAULT 1,
    "creatorId" BIGINT,
    "updaterId" BIGINT,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" smallint NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationTemplateDefinitionScopeCode"
    ON "public"."sysNotificationTemplateDefinition" ("scopeId", "templateCode", "isDeleted");
CREATE INDEX IF NOT EXISTS "idxNotificationTemplateDefinitionScopeUpdate"
    ON "public"."sysNotificationTemplateDefinition" ("scopeId", "updateTime");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationTemplateRevision" (
    "id" BIGINT PRIMARY KEY,
    "templateDefinitionId" BIGINT NOT NULL,
    "revisionNo" integer NOT NULL,
    "state" character varying(16) NOT NULL,
    "revisionVersion" integer NOT NULL DEFAULT 1,
    "subjectTemplate" character varying(512),
    "textTemplate" text,
    "htmlTemplate" text,
    "markdownTemplate" text,
    "primaryActionLabelTemplate" character varying(128) NOT NULL DEFAULT ''::character varying,
    "variableSchemaJson" jsonb NOT NULL,
    "contentDigest" character varying(128) NOT NULL,
    "publishedAt" timestamp with time zone,
    "publishedBy" BIGINT,
    "creatorId" BIGINT,
    "updaterId" BIGINT,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "ukNotificationTemplateRevisionDefinitionNo" UNIQUE ("templateDefinitionId", "revisionNo")
);

CREATE INDEX IF NOT EXISTS "idxNotificationTemplateRevisionDefinitionState"
    ON "public"."sysNotificationTemplateRevision" ("templateDefinitionId", "state", "revisionNo");

-- This audit table stores metadata only. It intentionally has no template
-- body, variable value, preview, recipient, credential, route or provider data.
CREATE TABLE IF NOT EXISTS "public"."sysNotificationTemplateRevisionAudit" (
    "id" BIGINT PRIMARY KEY,
    "templateDefinitionId" BIGINT NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "action" character varying(48) NOT NULL,
    "fromRevisionNo" integer,
    "toRevisionNo" integer,
    "actorId" BIGINT,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS "idxNotificationTemplateRevisionAuditDefinition"
    ON "public"."sysNotificationTemplateRevisionAudit" ("templateDefinitionId", "createTime");
CREATE INDEX IF NOT EXISTS "idxNotificationTemplateRevisionAuditScope"
    ON "public"."sysNotificationTemplateRevisionAudit" ("scopeId", "createTime");

-- +goose Down
-- Forward-only: published-template history and its audit evidence must not be
-- removed automatically.
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'forward-only notification template revision migration cannot be rolled back automatically';
END
$$;
-- +goose StatementEnd
