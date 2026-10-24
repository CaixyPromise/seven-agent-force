-- +goose Up

ALTER TABLE "sys_upload_task"
  ADD COLUMN "scopeId" character varying(128),
  ADD COLUMN "credentialId" character varying(64),
  ADD COLUMN "credentialVersion" integer NOT NULL DEFAULT 0,
  ADD COLUMN "protectedUntil" timestamp with time zone,
  ADD COLUMN "credentialExpireAt" timestamp with time zone,
  ADD COLUMN "revokedAt" timestamp with time zone;

CREATE UNIQUE INDEX "uk_upload_credential_id"
  ON "sys_upload_task" ("credentialId")
  WHERE "credentialId" IS NOT NULL;
CREATE INDEX "idx_upload_credential_authority"
  ON "sys_upload_task" ("userId", "scopeId", "fileId", "status", "credentialVersion", "credentialExpireAt");
CREATE INDEX "idx_upload_credential_protection"
  ON "sys_upload_task" ("fileId", "status", "protectedUntil", "revokedAt");

ALTER TABLE "sys_file_chunk_upload"
  ADD COLUMN "scopeId" character varying(128),
  ADD COLUMN "uploadTaskId" character varying(64);
CREATE INDEX "idx_chunk_upload_task" ON "sys_file_chunk_upload" ("uploadTaskId");
CREATE INDEX "idx_chunk_scope_user_status"
  ON "sys_file_chunk_upload" ("scopeId", "userId", "status", "expireTime");

ALTER TABLE "sys_file_reference"
  ADD COLUMN "scopeId" character varying(128);
DROP INDEX "idx_3095349_uk_user_biz_active";
CREATE UNIQUE INDEX "uk_file_reference_active_slot"
  ON "sys_file_reference" ("userId", "scopeId", "bizType", "bizId")
  WHERE "isDeleted" = false;
CREATE INDEX "idx_file_reference_scope_file"
  ON "sys_file_reference" ("scopeId", "fileId", "isDeleted");

-- +goose Down

DROP INDEX "idx_file_reference_scope_file";
DROP INDEX "uk_file_reference_active_slot";
CREATE UNIQUE INDEX "idx_3095349_uk_user_biz_active"
  ON "sys_file_reference" ("userId", "bizType", "bizId")
  WHERE "isDeleted" = false;
ALTER TABLE "sys_file_reference" DROP COLUMN "scopeId";

DROP INDEX "idx_chunk_scope_user_status";
DROP INDEX "idx_chunk_upload_task";
ALTER TABLE "sys_file_chunk_upload"
  DROP COLUMN "uploadTaskId",
  DROP COLUMN "scopeId";

DROP INDEX "idx_upload_credential_protection";
DROP INDEX "idx_upload_credential_authority";
DROP INDEX "uk_upload_credential_id";
ALTER TABLE "sys_upload_task"
  DROP COLUMN "revokedAt",
  DROP COLUMN "credentialExpireAt",
  DROP COLUMN "protectedUntil",
  DROP COLUMN "credentialVersion",
  DROP COLUMN "credentialId",
  DROP COLUMN "scopeId";
