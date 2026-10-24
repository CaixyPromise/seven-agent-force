-- +goose Up
-- Private, server-only recovery payloads for CONFIG_ASSET audit rows. They
-- remain opaque TEXT at the database boundary; only the configuration domain
-- parses the versioned envelope and no management/history query projects it.
ALTER TABLE "sys_config_change_log"
  ADD COLUMN IF NOT EXISTS "oldAssetSnapshot" TEXT NULL,
  ADD COLUMN IF NOT EXISTS "newAssetSnapshot" TEXT NULL;

-- +goose Down
-- Preserve durable private recovery evidence. The additive columns remain so
-- a Down/Up cycle cannot erase a historical CONFIG_ASSET snapshot and make a
-- later rollback appear to succeed by guessing current state.
SELECT 1;
