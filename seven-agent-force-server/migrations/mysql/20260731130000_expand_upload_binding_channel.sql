-- +goose Up
-- DG4.2 preserves the existing server-owned upload audit channel. The current
-- values include the scope-source marker and are longer than the historical
-- 16-character field. This is an in-place widening only: no table rename,
-- copy, dual write, or compatibility path is introduced.
ALTER TABLE sys_upload_task
  MODIFY COLUMN bindingChannel VARCHAR(64) DEFAULT NULL;

-- +goose Down
-- Narrowing can truncate committed audit values. Repair forward instead.
SIGNAL SQLSTATE '45000'
  SET MESSAGE_TEXT = 'upload binding channel expansion is forward-only';
