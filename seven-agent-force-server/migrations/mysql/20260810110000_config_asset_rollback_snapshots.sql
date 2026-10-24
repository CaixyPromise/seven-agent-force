-- +goose Up
-- Private, server-only recovery payloads for CONFIG_ASSET audit rows. These
-- are intentionally TEXT rather than a response-facing JSON contract: the
-- configuration repository is the sole reader and never maps them to a VO.
-- MySQL installations in the supported local upgrade range do not all accept
-- ADD COLUMN IF NOT EXISTS. Use information_schema + a prepared statement so
-- the migration is still repeatable after its non-destructive Goose Down.
SET @dc23_snapshot_sql = (
  SELECT IF(COUNT(*) = 0,
    'ALTER TABLE sys_config_change_log ADD COLUMN oldAssetSnapshot TEXT NULL COMMENT ''CONFIG_ASSET private previous binding snapshot''',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'sys_config_change_log' AND column_name = 'oldAssetSnapshot'
);
PREPARE dc23_snapshot_stmt FROM @dc23_snapshot_sql;
EXECUTE dc23_snapshot_stmt;
DEALLOCATE PREPARE dc23_snapshot_stmt;

SET @dc23_snapshot_sql = (
  SELECT IF(COUNT(*) = 0,
    'ALTER TABLE sys_config_change_log ADD COLUMN newAssetSnapshot TEXT NULL COMMENT ''CONFIG_ASSET private resulting binding snapshot''',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'sys_config_change_log' AND column_name = 'newAssetSnapshot'
);
PREPARE dc23_snapshot_stmt FROM @dc23_snapshot_sql;
EXECUTE dc23_snapshot_stmt;
DEALLOCATE PREPARE dc23_snapshot_stmt;

-- +goose Down
-- Do not destroy recoverable audit state during a schema rollback. Leaving
-- additive private columns in place makes Down/Up repeatable and prevents a
-- Goose downgrade from silently turning future history recovery into a guess.
SELECT 1;
