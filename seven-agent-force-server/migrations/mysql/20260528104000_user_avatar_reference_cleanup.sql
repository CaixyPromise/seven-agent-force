-- +goose Up
-- Remove stale avatar URLs that point to deleted or missing file references.
UPDATE sys_user u
LEFT JOIN sys_file_reference r
  ON r.id = CAST(SUBSTRING_INDEX(u.userAvatar, 'referenceId=', -1) AS UNSIGNED)
  AND r.isDeleted = 0
SET u.userAvatar = NULL
WHERE u.userAvatar LIKE '%referenceId=%'
  AND r.id IS NULL;
