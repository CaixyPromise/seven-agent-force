-- +goose Up
-- Remove stale avatar URLs that point to deleted or missing file references.
UPDATE sys_user u
SET "userAvatar" = NULL
WHERE u."userAvatar" LIKE '%referenceId=%'
  AND NOT EXISTS (
    SELECT 1
    FROM sys_file_reference r
    WHERE r.id = NULLIF(substring(u."userAvatar" FROM 'referenceId=([0-9]+)'), '')::bigint
      AND r."isDeleted" = 0
  );
