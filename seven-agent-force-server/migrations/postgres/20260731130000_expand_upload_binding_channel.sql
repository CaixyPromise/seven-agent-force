-- +goose Up
-- DG4.2 preserves the existing server-owned upload audit channel. The current
-- values include the scope-source marker and are longer than the historical
-- 16-character field. This is an in-place widening only: no table rename,
-- copy, dual write, or compatibility path is introduced.
ALTER TABLE "sys_upload_task"
  ALTER COLUMN "bindingChannel" TYPE character varying(64);

-- +goose Down
-- Narrowing can truncate committed audit values. Repair forward instead.
-- +goose StatementBegin
DO $dg4_protected$
BEGIN
    RAISE EXCEPTION 'upload binding channel expansion is forward-only';
END
$dg4_protected$;
-- +goose StatementEnd
