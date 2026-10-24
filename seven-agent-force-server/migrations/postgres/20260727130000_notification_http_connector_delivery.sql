-- +goose Up
-- Goal-5.2 stores one immutable operator-selected HTTP connection revision
-- beside each accepted delivery. The row contains no plaintext credential;
-- webhook URLs and signing material remain inside the encrypted envelope.

CREATE TABLE IF NOT EXISTS "public"."sysNotificationHTTPDeliverySnapshot" (
    "id" BIGINT PRIMARY KEY,
    "deliveryId" character varying(96) NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "channelType" character varying(32) NOT NULL,
    "channelPriority" integer NOT NULL DEFAULT 100,
    "configJson" jsonb NOT NULL,
    "secretCiphertext" text,
    "secretEdek" text,
    "secretWrapKeyRef" character varying(191),
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS "ukNotificationHTTPDeliverySnapshotDelivery"
    ON "public"."sysNotificationHTTPDeliverySnapshot" ("deliveryId");
CREATE INDEX IF NOT EXISTS "idxNotificationHTTPDeliverySnapshotScopeChannel"
    ON "public"."sysNotificationHTTPDeliverySnapshot" ("scopeId", "channelCode", "createTime");

-- +goose Down
-- Forward-only: accepted delivery evidence and encrypted connection revisions
-- are retained for audit and must not be dropped automatically.
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'forward-only notification HTTP connector migration cannot be rolled back automatically';
END
$$;
-- +goose StatementEnd
