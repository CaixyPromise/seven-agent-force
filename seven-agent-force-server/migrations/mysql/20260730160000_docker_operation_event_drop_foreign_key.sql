-- +goose Up

SET @dg3_drop_fk_sql = IF(
  (
    SELECT COUNT(1)
    FROM information_schema.referential_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'docker_operation_event'
      AND constraint_name = 'fk_docker_operation_event_operation'
  ) = 1,
  'ALTER TABLE docker_operation_event DROP FOREIGN KEY fk_docker_operation_event_operation',
  'SELECT 1'
);
PREPARE dg3_drop_fk_stmt FROM @dg3_drop_fk_sql;
EXECUTE dg3_drop_fk_stmt;
DEALLOCATE PREPARE dg3_drop_fk_stmt;

-- PK, uk_docker_operation_event_sequence, required-column NOT NULL constraints,
-- and chk_docker_operation_event_integrity_status intentionally remain.

-- +goose Down

-- This contract migration is forward-only. Restore by reconciling data and
-- applying a separately reviewed forward migration; never silently re-enable
-- cascading deletes after the application-integrity boundary has changed.
SIGNAL SQLSTATE '45000'
  SET MESSAGE_TEXT = 'DG3 foreign-key removal is forward-only';
