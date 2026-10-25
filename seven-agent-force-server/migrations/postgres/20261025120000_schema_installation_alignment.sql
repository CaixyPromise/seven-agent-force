-- +goose Up

-- Supply the verified-email lookup used by the application and local alignment.
CREATE INDEX IF NOT EXISTS sys_user_verified_email_idx
    ON sys_user (lower("userEmail"), email_verified_at)
    WHERE "isDeleted" = false AND status = 0 AND email_verified_at IS NOT NULL;

-- A renamed legacy table can retain an ACTIVE/DISABLED-only check. Keep a
-- current lifecycle guard before removing the obsolete guard that blocks delete.
-- +goose StatementBegin
DO $model_lifecycle$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'workforce_model_credentials'::regclass
          AND conname IN ('workforce_model_credentials_status_check', 'workforce_model_credentials_status_ck')
    ) THEN
        ALTER TABLE workforce_model_credentials
            ADD CONSTRAINT workforce_model_credentials_status_ck
            CHECK (status IN ('ACTIVE', 'DISABLED', 'DELETED'));
    END IF;
    ALTER TABLE workforce_model_credentials
        DROP CONSTRAINT IF EXISTS workforce_provider_profiles_status_check;
END
$model_lifecycle$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
    RAISE EXCEPTION 'forward-only schema installation alignment: preserve the current lifecycle guard and repair forward';
END
$forward_only$;
-- +goose StatementEnd
