-- +goose Up

ALTER TABLE docker_operation_event
  DROP CONSTRAINT IF EXISTS fk_docker_operation_event_operation;

-- PK, uk_docker_operation_event_sequence, required-column NOT NULL constraints,
-- and chk_docker_operation_event_integrity_status intentionally remain.

-- +goose Down

-- This contract migration is forward-only. Restore by reconciling data and
-- applying a separately reviewed forward migration; never silently re-enable
-- cascading deletes after the application-integrity boundary has changed.
-- +goose StatementBegin
DO $dg3$
BEGIN
  RAISE EXCEPTION 'DG3 foreign-key removal is forward-only';
END
$dg3$;
-- +goose StatementEnd
