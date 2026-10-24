-- +goose Up
-- Expiry keeps the recipient audit row but creates one durable visible-state
-- transition so open inbox clients can remove an item through mailbox delta.

ALTER TABLE "public"."sysNotificationRecipient"
    ADD COLUMN "expiredAt" timestamp with time zone;

CREATE INDEX IF NOT EXISTS "idxNotificationRecipientExpiry"
    ON "public"."sysNotificationRecipient" ("scopeId", "expiredAt", "expiresAt", "id");

-- +goose Down
-- Forward-only: retain expiry audit data and disable the worker before any
-- manually reviewed rollback. Failing here keeps the Goose version unchanged.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'forward-only notification expiry migration cannot be rolled back automatically';
END
$$;
-- +goose StatementEnd
